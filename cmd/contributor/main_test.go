package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/projectbluefin/donate-clanker/internal/contract"
	"github.com/projectbluefin/donate-clanker/internal/hive"
	"github.com/projectbluefin/donate-clanker/internal/runner"
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

	workspace := contributorScratch(t, "workspace")
	runtimeDir := contributorScratch(t, "runtime")
	manifest := testContributorContractManifest()
	writeContributorContractDocuments(t, workspace, manifest)
	handler := &contributorHandler{
		baseTask: runner.Task{
			Workspace:     workspace,
			Provider:      "openai",
			Model:         "test",
			RuntimeDir:    runtimeDir,
			BundledConfig: []byte("GOOSE_PROVIDER: openai\n"),
		},
		goose: runner.Goose{Runner: commandRunner, Contract: manifest},
	}
	if got, want := len(handler.goose.Contract.RequiredDocuments), len(manifest.RequiredDocuments); got != want {
		t.Fatalf("len(handler.goose.Contract.RequiredDocuments) = %d, want %d", got, want)
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

func contributorScratch(t *testing.T, name string) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	dir := filepath.Join(root, "cmd", "contributor", ".contributor-test", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(root, "cmd", "contributor", ".contributor-test")) })
	return dir
}

func testContributorContractManifest() contract.Manifest {
	return contract.Manifest{
		Version: 1,
		RequiredDocuments: []contract.RequiredDocument{
			{Path: "AGENTS.md", Heading: "Required repository document: AGENTS.md"},
			{Path: "docs/SKILL.md", Heading: "Required repository document: docs/SKILL.md"},
		},
		Rules:              []string{"Read AGENTS.md first, then docs/SKILL.md."},
		ValidationCommands: []string{"go test ./cmd/contributor"},
	}
}

func writeContributorContractDocuments(t *testing.T, workspace string, manifest contract.Manifest) {
	t.Helper()
	contents := []string{"Repository contract entrypoint.\n", "Skill router content.\n"}
	for i, doc := range manifest.RequiredDocuments {
		path := filepath.Join(workspace, filepath.FromSlash(doc.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(path, []byte(contents[i]), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}
}
