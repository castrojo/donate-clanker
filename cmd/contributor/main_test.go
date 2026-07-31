package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/projectbluefin/donate-clanker/internal/contract"
	"github.com/projectbluefin/donate-clanker/internal/hive"
	"github.com/projectbluefin/donate-clanker/internal/runner"
)

func TestClearWorkerCredentialEnvironmentRemovesHostAuthState(t *testing.T) {
	for key, value := range map[string]string{
		"HIVE_REGISTRATION_TOKEN":           "hive-secret",
		"HIVE_HUB":                          "wss://example.invalid/contribute",
		"HIVE_WS_URL":                       "wss://example.invalid/api/contribute/ws",
		"CONTRIBUTOR_ID":                    "c-123",
		"CONTRIBUTOR_USERNAME":              "tester",
		"HIVE_CONFIG_DIR":                   "/config/hive",
		"GH_TOKEN":                          "gho_host_token",
		"GITHUB_TOKEN":                      "gho_host_token",
		"GH_CONFIG_DIR":                     "/config/github",
		"GITHUB_CONFIG_DIR":                 "/config/github",
		"DONATE_CLANKER_HIVE_ENDPOINT":      "wss://example.invalid/contribute",
		"DONATE_CLANKER_REGISTRATION_TOKEN": "bootstrap-secret",
		"DONATE_CLANKER_BACKEND":            "goose",
		"DONATE_CLANKER_RUN_ID":             "run-1",
		"GOOSE_MODEL":                       "local",
		"OPENAI_BASE_URL":                   "http://127.0.0.1:8000/v1",
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
		"DONATE_CLANKER_HIVE_ENDPOINT",
		"DONATE_CLANKER_REGISTRATION_TOKEN",
		"DONATE_CLANKER_BACKEND",
		"DONATE_CLANKER_RUN_ID",
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

func TestCurrentEnvironmentMapsVMBootstrapEnvelope(t *testing.T) {
	for _, key := range []string{
		"HIVE_REGISTRATION_TOKEN",
		"HIVE_HUB",
		"HIVE_WS_URL",
		"AGENT_BACKEND",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("DONATE_CLANKER_HIVE_ENDPOINT", "wss://example.invalid/contribute")
	t.Setenv("DONATE_CLANKER_REGISTRATION_TOKEN", "bootstrap-secret")
	t.Setenv("DONATE_CLANKER_BACKEND", "goose")

	values := currentEnvironment()
	for key, want := range map[string]string{
		"HIVE_REGISTRATION_TOKEN": "bootstrap-secret",
		"HIVE_HUB":                "wss://example.invalid/contribute",
		"HIVE_WS_URL":             "wss://example.invalid/contribute",
		"AGENT_BACKEND":           "goose",
	} {
		if got := values[key]; got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestContributorHandlerDoesNotRestartWithRefreshedToken(t *testing.T) {
	var (
		mu     sync.Mutex
		tokens []string
		calls  int
	)
	firstStarted := make(chan struct{})
	finishFirst := make(chan struct{})

	commandRunner := runnerFunc(func(ctx context.Context, command runner.Command) (runner.CommandOutput, error) {
		mu.Lock()
		calls++
		call := calls
		tokens = append(tokens, command.Env["GH_TOKEN"])
		mu.Unlock()

		switch call {
		case 1:
			close(firstStarted)
			select {
			case <-finishFirst:
			case <-ctx.Done():
				return runner.CommandOutput{}, ctx.Err()
			}
			return runner.CommandOutput{Combined: "completed"}, nil
		default:
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
	case err := <-resultCh:
		t.Fatalf("Handle() returned after refresh: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(finishFirst)
	if err := <-resultCh; err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(tokens) != 1 || tokens[0] != "old-token" {
		t.Fatalf("Goose tokens = %#v, want [old-token]", tokens)
	}
}

func TestContributorHandlerUsesTaskIsolatedRuntimeDirectories(t *testing.T) {
	var homes []string
	commandRunner := runnerFunc(func(_ context.Context, command runner.Command) (runner.CommandOutput, error) {
		homes = append(homes, command.Env["HOME"])
		return runner.CommandOutput{Combined: "completed"}, nil
	})

	workspace := contributorScratch(t, "isolated-workspace")
	runtimeRoot := contributorScratch(t, "isolated-runtime")
	manifest := testContributorContractManifest()
	writeContributorContractDocuments(t, workspace, manifest)
	handler := &contributorHandler{
		baseTask: runner.Task{
			Workspace:     workspace,
			Provider:      "openai",
			Model:         "test",
			RuntimeDir:    runtimeRoot,
			BundledConfig: []byte("GOOSE_PROVIDER: openai\n"),
		},
		goose: runner.Goose{Runner: commandRunner, Contract: manifest},
	}

	for _, assignment := range []hive.Assignment{
		{TaskID: "task-one", GitHubToken: "token-one", Prompt: "do work"},
		{TaskID: "../task-two", GitHubToken: "token-two", Prompt: "do other work"},
	} {
		if _, err := handler.Handle(context.Background(), assignment); err != nil {
			t.Fatalf("Handle(%q) error = %v", assignment.TaskID, err)
		}
	}

	if len(homes) != 2 {
		t.Fatalf("Goose invocation count = %d, want 2", len(homes))
	}
	if homes[0] == homes[1] {
		t.Fatalf("Goose runtime homes = %#v, want isolated directories", homes)
	}
	for _, home := range homes {
		relative, err := filepath.Rel(filepath.Clean(runtimeRoot), home)
		if err != nil {
			t.Fatalf("Rel(%q, %q) error = %v", runtimeRoot, home, err)
		}
		if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			t.Fatalf("runtime home %q escapes root %q", home, runtimeRoot)
		}
	}
}

func TestContributorHandlerRemovesTaskRuntimeDirectoryAfterSuccess(t *testing.T) {
	commandRunner := runnerFunc(func(_ context.Context, _ runner.Command) (runner.CommandOutput, error) {
		return runner.CommandOutput{Combined: "completed"}, nil
	})
	workspace := contributorScratch(t, "cleanup-workspace")
	runtimeRoot := contributorScratch(t, "cleanup-runtime")
	manifest := testContributorContractManifest()
	writeContributorContractDocuments(t, workspace, manifest)
	handler := &contributorHandler{
		baseTask: runner.Task{
			Workspace:     workspace,
			Provider:      "openai",
			Model:         "test",
			RuntimeDir:    runtimeRoot,
			BundledConfig: []byte("GOOSE_PROVIDER: openai\n"),
		},
		goose: runner.Goose{Runner: commandRunner, Contract: manifest},
	}
	assignment := hive.Assignment{TaskID: "task-cleanup", GitHubToken: "token", Prompt: "do work"}
	taskRuntimeDir, err := runner.TaskRuntimeDir(runtimeRoot, assignment.TaskID)
	if err != nil {
		t.Fatalf("TaskRuntimeDir() error = %v", err)
	}

	if _, err := handler.Handle(context.Background(), assignment); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if _, err := os.Stat(taskRuntimeDir); !os.IsNotExist(err) {
		t.Fatalf("task runtime directory %q still exists or could not be checked: %v", taskRuntimeDir, err)
	}
}

func TestContributorHandlerCleanupFailureDoesNotChangeSuccessfulResult(t *testing.T) {
	var localOutput bytes.Buffer
	var tasksParent string
	commandRunner := runnerFunc(func(_ context.Context, command runner.Command) (runner.CommandOutput, error) {
		tasksParent = filepath.Dir(filepath.Dir(command.Env["HOME"]))
		if err := os.Chmod(tasksParent, 0o500); err != nil {
			t.Fatalf("Chmod(%q) error = %v", tasksParent, err)
		}
		return runner.CommandOutput{Combined: "completed"}, nil
	})

	workspace := contributorScratch(t, "cleanup-failure-workspace")
	runtimeRoot := contributorScratch(t, "cleanup-failure-runtime")
	manifest := testContributorContractManifest()
	writeContributorContractDocuments(t, workspace, manifest)
	handler := &contributorHandler{
		baseTask: runner.Task{
			Workspace:     workspace,
			Provider:      "openai",
			Model:         "test",
			RuntimeDir:    runtimeRoot,
			BundledConfig: []byte("GOOSE_PROVIDER: openai\n"),
		},
		goose:             runner.Goose{Runner: commandRunner, Contract: manifest},
		observationWriter: &localOutput,
		now: func() time.Time {
			return time.Date(2026, time.July, 30, 1, 2, 3, 0, time.UTC)
		},
	}

	report, err := handler.Handle(context.Background(), hive.Assignment{
		TaskID:      "task-cleanup-warning",
		Kind:        "issue",
		Number:      42,
		GitHubToken: "token",
		Prompt:      "do work",
	})
	if tasksParent != "" {
		if chmodErr := os.Chmod(tasksParent, 0o700); chmodErr != nil {
			t.Fatalf("Chmod(%q) restore error = %v", tasksParent, chmodErr)
		}
	}
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if report.Result != "completed" || report.Summary != "completed" {
		t.Fatalf("report = %#v, want completed result and summary", report)
	}

	lines := strings.Split(strings.TrimSpace(localOutput.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("local output lines = %q, want cleanup warning plus observation", localOutput.String())
	}
	if !strings.Contains(lines[0], "task runtime cleanup failed:") {
		t.Fatalf("cleanup warning = %q, want task runtime cleanup failed prefix", lines[0])
	}

	var recorded taskObservation
	if err := json.Unmarshal([]byte(lines[1]), &recorded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if recorded.Outcome != "success" {
		t.Fatalf("observation outcome = %q, want success", recorded.Outcome)
	}
}

func TestContributorHandlerPropagatesCancellation(t *testing.T) {
	started := make(chan struct{})
	commandRunner := runnerFunc(func(ctx context.Context, _ runner.Command) (runner.CommandOutput, error) {
		close(started)
		<-ctx.Done()
		return runner.CommandOutput{}, ctx.Err()
	})

	workspace := contributorScratch(t, "cancelled-workspace")
	runtimeDir := contributorScratch(t, "cancelled-runtime")
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assignment := hive.Assignment{TaskID: "task-cancelled", GitHubToken: "token", Prompt: "do work"}
	taskRuntimeDir, err := runner.TaskRuntimeDir(runtimeDir, assignment.TaskID)
	if err != nil {
		t.Fatalf("TaskRuntimeDir() error = %v", err)
	}

	resultCh := make(chan error, 1)
	go func() {
		_, err := handler.Handle(ctx, assignment)
		resultCh <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Goose invocation did not start")
	}
	cancel()
	if err := <-resultCh; err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("Handle() error = %v, want cancelled Goose failure", err)
	}
	if _, err := os.Stat(taskRuntimeDir); !os.IsNotExist(err) {
		t.Fatalf("task runtime directory %q still exists or could not be checked: %v", taskRuntimeDir, err)
	}
}

func TestContributorHandlerEmitsObservationBeforeReturningReport(t *testing.T) {
	var observation bytes.Buffer
	commandRunner := runnerFunc(func(_ context.Context, _ runner.Command) (runner.CommandOutput, error) {
		return runner.CommandOutput{Combined: "private model output"}, nil
	})
	handler := newObservationTestHandler(t, commandRunner, &observation)

	report, err := handler.Handle(context.Background(), hive.Assignment{
		TaskID:      "task-observation",
		Kind:        "issue",
		Number:      42,
		Title:       "private title",
		Prompt:      "private prompt",
		GitHubToken: "private token",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if report.Result != "completed" {
		t.Fatalf("report result = %q, want completed", report.Result)
	}

	var recorded taskObservation
	if err := json.Unmarshal(observation.Bytes(), &recorded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if recorded.TaskID != "task-observation" || recorded.Outcome != "success" {
		t.Fatalf("observation = %#v, want task-observation success", recorded)
	}
	if strings.Contains(observation.String(), "private") {
		t.Fatalf("observation contains private task data: %s", observation.String())
	}
}

func TestContributorHandlerObservationWriteFailureDoesNotChangeResult(t *testing.T) {
	commandRunner := runnerFunc(func(_ context.Context, _ runner.Command) (runner.CommandOutput, error) {
		return runner.CommandOutput{Combined: "completed"}, nil
	})
	handler := newObservationTestHandler(t, commandRunner, failingObservationWriter{})

	report, err := handler.Handle(context.Background(), hive.Assignment{
		TaskID:      "task-observation-failure",
		Kind:        "issue",
		Number:      42,
		GitHubToken: "token",
		Prompt:      "do work",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if report.Result != "completed" || report.Summary != "completed" {
		t.Fatalf("report = %#v, want completed result and summary", report)
	}
}

func newObservationTestHandler(t *testing.T, commandRunner runner.CommandRunner, observationWriter io.Writer) *contributorHandler {
	t.Helper()
	workspace := contributorScratch(t, "observation-workspace")
	runtimeDir := contributorScratch(t, "observation-runtime")
	manifest := testContributorContractManifest()
	writeContributorContractDocuments(t, workspace, manifest)
	return &contributorHandler{
		baseTask: runner.Task{
			Workspace:     workspace,
			Provider:      "openai",
			Model:         "test",
			RuntimeDir:    runtimeDir,
			BundledConfig: []byte("GOOSE_PROVIDER: openai\n"),
		},
		goose:             runner.Goose{Runner: commandRunner, Contract: manifest},
		observationWriter: observationWriter,
		now: func() time.Time {
			return time.Date(2026, time.July, 30, 1, 2, 3, 0, time.UTC)
		},
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
