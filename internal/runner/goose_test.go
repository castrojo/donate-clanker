package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareTaskPromptPreservesPolicyWhitespace(t *testing.T) {
	policy := "  inspect local repository evidence first.\nUse Context7 opportunistically.  \n"
	assignment := "Task ID: hive-123\nTitle: Fix the bug\n\nFollow the original assignment exactly."

	prompt := PrepareTaskPrompt(policy, assignment)

	want := localPolicyHeading + "\n" + policy + "\n\n" + hiveAssignmentHeading + "\n" + assignment
	if prompt != want {
		t.Fatalf("PrepareTaskPrompt() = %q, want %q", prompt, want)
	}
}

func TestPrepareTaskPromptReturnsAssignmentWhenPolicyEmpty(t *testing.T) {
	assignment := "Task ID: hive-123\nTitle: Fix the bug"

	if got := PrepareTaskPrompt("\n \t", assignment); got != assignment {
		t.Fatalf("PrepareTaskPrompt() = %q, want %q", got, assignment)
	}
}

func TestGooseRunUsesHeadlessCommandAndStagesConfig(t *testing.T) {
	t.Setenv("PATH", os.Getenv("PATH"))
	t.Setenv("HIVE_CONFIG_DIR", "/host/hive")
	t.Setenv("GITHUB_CONFIG_DIR", "/host/github")
	t.Setenv("GH_TOKEN", "gho_host_token")
	t.Setenv("GITHUB_TOKEN", "gho_host_token")

	fake := &fakeCommandRunner{
		output: CommandOutput{Combined: "Task finished cleanly\n"},
	}
	task := Task{
		Prompt:        "Task ID: hive-123\nTitle: Fix the bug",
		Workspace:     repoScratch(t, "workspace"),
		Provider:      "openai",
		Model:         "Qwen3.6-35B-A3B",
		OpenAIBaseURL: "http://127.0.0.1:8000/v1",
		OpenAIAPIKey:  "local",
		GitHubToken:   "ghs_secret_token",
		RuntimeDir:    repoScratch(t, "runtime"),
		BundledConfig: []byte("provider: openai\nbase_url: http://127.0.0.1:8000/v1\n"),
	}
	goose := Goose{
		Command: "goose",
		Policy:  "inspect local repository evidence first.\nUse Context7 opportunistically.",
		Runner:  fake,
	}

	result, err := goose.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Result != "completed" {
		t.Fatalf("Run().Result = %q, want completed", result.Result)
	}
	if got := result.Summary; got != "Task finished cleanly" {
		t.Fatalf("Run().Summary = %q, want task summary", got)
	}

	if len(fake.calls) != 1 {
		t.Fatalf("Run() command count = %d, want 1", len(fake.calls))
	}
	call := fake.calls[0]
	if call.Name != "goose" {
		t.Fatalf("Run() command name = %q, want goose", call.Name)
	}
	if call.Dir != filepath.Clean(task.Workspace) {
		t.Fatalf("Run() dir = %q, want %q", call.Dir, task.Workspace)
	}

	wantArgs := []string{
		"run",
		"--no-session",
		"--provider", "openai",
		"--model", "Qwen3.6-35B-A3B",
	}
	for _, want := range wantArgs {
		if !contains(call.Args, want) {
			t.Fatalf("Run() args = %#v, missing %q", call.Args, want)
		}
	}
	wantPrompt := localPolicyHeading + "\n" + goose.Policy + "\n\n" + hiveAssignmentHeading + "\n" + task.Prompt
	if !contains(call.Args, wantPrompt) {
		t.Fatalf("Run() args do not contain prepared prompt %q: %#v", wantPrompt, call.Args)
	}

	wantEnv := map[string]string{
		"GOOSE_PROVIDER":        "openai",
		"GOOSE_MODEL":           "Qwen3.6-35B-A3B",
		"GOOSE_THINKING_EFFORT": "off",
		"OPENAI_BASE_URL":       "http://127.0.0.1:8000/v1",
		"OPENAI_API_KEY":        "local",
		"GH_TOKEN":              "ghs_secret_token",
		"GITHUB_TOKEN":          "ghs_secret_token",
		"WORKSPACE":             filepath.Clean(task.Workspace),
	}
	for key, want := range wantEnv {
		if got := call.Env[key]; got != want {
			t.Fatalf("Run() env[%q] = %q, want %q", key, got, want)
		}
	}
	for _, key := range []string{"HIVE_CONFIG_DIR", "GITHUB_CONFIG_DIR", "GH_CONFIG_DIR"} {
		if got := call.Env[key]; got != "" {
			t.Fatalf("Run() env[%q] = %q, want empty", key, got)
		}
	}
	if got := call.Env["GH_TOKEN"]; got == "gho_host_token" {
		t.Fatalf("Run() inherited host GH_TOKEN %q", got)
	}
	if got := call.Env["GITHUB_TOKEN"]; got == "gho_host_token" {
		t.Fatalf("Run() inherited host GITHUB_TOKEN %q", got)
	}
	homeDir := filepath.Join(filepath.Clean(task.RuntimeDir), "home")
	if got := call.Env["HOME"]; got != homeDir {
		t.Fatalf("Run() env[HOME] = %q, want %q", got, homeDir)
	}
	if got := call.Env["XDG_CONFIG_HOME"]; got != filepath.Join(homeDir, ".config") {
		t.Fatalf("Run() env[XDG_CONFIG_HOME] = %q", got)
	}

	stagedConfig, err := os.ReadFile(filepath.Join(homeDir, ".config", "goose", "config.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(staged config) error = %v", err)
	}
	if string(stagedConfig) != string(task.BundledConfig) {
		t.Fatalf("staged config = %q, want %q", stagedConfig, task.BundledConfig)
	}
}

func TestGooseRunRedactsAndPropagatesFailure(t *testing.T) {
	fake := &fakeCommandRunner{
		output: CommandOutput{
			Combined: "GH_TOKEN=gho_super_secret\nAuthorization: Bearer demo-token-123\nfatal: push failed\n",
		},
		err: errors.New("exit status 1: GH_TOKEN=gho_super_secret"),
	}
	task := Task{
		Prompt:        "Task ID: hive-123\nTitle: Fix the bug",
		Workspace:     repoScratch(t, "workspace"),
		Provider:      "openai",
		Model:         "local",
		OpenAIBaseURL: "http://127.0.0.1:8000/v1",
		OpenAIAPIKey:  "local",
		RuntimeDir:    repoScratch(t, "runtime"),
		BundledConfig: []byte("provider: openai\n"),
	}

	result, err := Goose{Runner: fake}.Run(context.Background(), task)
	if err == nil {
		t.Fatal("Run() error = nil, want failure")
	}

	var execErr *ExecutionError
	if !errors.As(err, &execErr) {
		t.Fatalf("Run() error = %T, want *ExecutionError", err)
	}
	if strings.Contains(execErr.Error(), "gho_super_secret") || strings.Contains(execErr.Error(), "demo-token-123") {
		t.Fatalf("Run() error leaked secret: %q", execErr.Error())
	}
	if got := strings.Join(result.Output, "\n"); !strings.Contains(got, "[REDACTED]") || strings.Contains(got, "gho_super_secret") || strings.Contains(got, "demo-token-123") {
		t.Fatalf("Run().Output = %q, want redacted secrets", got)
	}
	if got := result.Summary; got != "fatal: push failed" {
		t.Fatalf("Run().Summary = %q, want failure summary", got)
	}
}

type fakeCommandRunner struct {
	calls  []Command
	output CommandOutput
	err    error
}

func (f *fakeCommandRunner) Run(_ context.Context, cmd Command) (CommandOutput, error) {
	envCopy := make(map[string]string, len(cmd.Env))
	for key, value := range cmd.Env {
		envCopy[key] = value
	}
	argsCopy := append([]string(nil), cmd.Args...)
	f.calls = append(f.calls, Command{
		Name: cmd.Name,
		Args: argsCopy,
		Dir:  cmd.Dir,
		Env:  envCopy,
	})
	return f.output, f.err
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func repoScratch(t *testing.T, name string) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	dir := filepath.Join(root, "internal", "runner", ".runner-test", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(root, "internal", "runner", ".runner-test")) })
	return dir
}
