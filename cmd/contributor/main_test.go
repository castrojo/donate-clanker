package main

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/castrojo/donate-clanker/internal/hive"
	"github.com/castrojo/donate-clanker/internal/runner"
)

func TestClearWorkerCredentialEnvironmentRemovesHostAuthState(t *testing.T) {
	for key, value := range map[string]string{
		"HIVE_REGISTRATION_TOKEN": "hive-secret",
		"HIVE_HUB":                "wss://example.invalid/contribute",
		"HIVE_WS_URL":             "wss://example.invalid/api/contribute/ws",
		"CONTRIBUTOR_ID":          "c-123",
		"CONTRIBUTOR_USERNAME":    "tester",
		"HIVE_CONFIG_DIR":         "/config/hive",
		"GH_TOKEN":                "gho_host_token",
		"GITHUB_TOKEN":            "gho_host_token",
		"GH_CONFIG_DIR":           "/config/github",
		"GITHUB_CONFIG_DIR":       "/config/github",
		"GOOSE_MODEL":             "local",
		"OPENAI_BASE_URL":         "http://127.0.0.1:8000/v1",
	} {
		t.Setenv(key, value)
	}

	clearWorkerCredentialEnvironment()

	for _, key := range []string{
		"HIVE_REGISTRATION_TOKEN",
		"HIVE_HUB",
		"HIVE_WS_URL",
		"CONTRIBUTOR_ID",
		"CONTRIBUTOR_USERNAME",
		"HIVE_CONFIG_DIR",
		"GH_TOKEN",
		"GITHUB_TOKEN",
		"GH_CONFIG_DIR",
		"GITHUB_CONFIG_DIR",
	} {
		if got := os.Getenv(key); got != "" {
			t.Fatalf("%s = %q, want empty", key, got)
		}
	}

	for key, want := range map[string]string{
		"GOOSE_MODEL":     "local",
		"OPENAI_BASE_URL": "http://127.0.0.1:8000/v1",
	} {
		if got := os.Getenv(key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestContributorHandlerRetriesWithRefreshedToken(t *testing.T) {
	var (
		mu     sync.Mutex
		tokens []string
		calls  int
	)
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})

	commandRunner := runnerFunc(func(ctx context.Context, command runner.Command) (runner.CommandOutput, error) {
		mu.Lock()
		calls++
		call := calls
		tokens = append(tokens, command.Env["GH_TOKEN"])
		mu.Unlock()

		switch call {
		case 1:
			close(firstStarted)
			<-ctx.Done()
			return runner.CommandOutput{}, ctx.Err()
		case 2:
			close(secondStarted)
			return runner.CommandOutput{Combined: "completed"}, nil
		default:
			t.Fatalf("unexpected Goose invocation %d", call)
			return runner.CommandOutput{}, errors.New("unexpected invocation")
		}
	})

	handler := &contributorHandler{
		baseTask: runner.Task{
			Workspace:     t.TempDir(),
			Provider:      "openai",
			Model:         "test",
			RuntimeDir:    t.TempDir(),
			BundledConfig: []byte("GOOSE_PROVIDER: openai\n"),
		},
		goose: runner.Goose{Runner: commandRunner},
	}
	assignment := hive.Assignment{TaskID: "task-1", GitHubToken: "old-token", Prompt: "do work"}

	resultCh := make(chan error, 1)
	go func() {
		_, err := handler.Handle(context.Background(), assignment)
		resultCh <- err
	}()

	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first Goose invocation did not start")
	}
	if err := handler.Refresh(context.Background(), hive.Assignment{
		TaskID: "task-1", GitHubToken: "new-token", Prompt: "do work",
	}); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("refreshed Goose invocation did not start")
	}
	if err := <-resultCh; err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(tokens) != 2 || tokens[0] != "old-token" || tokens[1] != "new-token" {
		t.Fatalf("Goose tokens = %#v, want [old-token new-token]", tokens)
	}
}

type runnerFunc func(context.Context, runner.Command) (runner.CommandOutput, error)

func (f runnerFunc) Run(ctx context.Context, command runner.Command) (runner.CommandOutput, error) {
	return f(ctx, command)
}
