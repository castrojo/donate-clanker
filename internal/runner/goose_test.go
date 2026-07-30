package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/projectbluefin/donate-clanker/internal/contract"
)

func TestPrepareTaskPromptPreservesPolicyWhitespace(t *testing.T) {
	policy := "  inspect local repository evidence first.\nUse Context7 opportunistically.  \n"
	contractSection := "## Agent contract\n- Read AGENTS.md first."
	assignment := "Task ID: hive-123\nTitle: Fix the bug\n\nFollow the original assignment exactly."

	prompt := PrepareTaskPrompt(policy, contractSection, assignment)

	want := localPolicyHeading + "\n" + policy + "\n\n" + contractSection + "\n\n" + hiveAssignmentHeading + "\n" + assignment
	if prompt != want {
		t.Fatalf("PrepareTaskPrompt() = %q, want %q", prompt, want)
	}
}

func TestPrepareTaskPromptReturnsContractAndAssignmentWhenPolicyEmpty(t *testing.T) {
	contractSection := "## Agent contract\n- Read AGENTS.md first."
	assignment := "Task ID: hive-123\nTitle: Fix the bug"

	want := contractSection + "\n\n" + hiveAssignmentHeading + "\n" + assignment
	if got := PrepareTaskPrompt("\n \t", contractSection, assignment); got != want {
		t.Fatalf("PrepareTaskPrompt() = %q, want %q", got, want)
	}
}

func TestGooseRunFailsBeforeCommandWhenContractDocumentMissing(t *testing.T) {
	fake := &fakeCommandRunner{}
	workspace := repoScratch(t, "missing-workspace")
	runtimeDir := repoScratch(t, "missing-runtime")
	goose := Goose{
		Runner:   fake,
		Contract: testContractManifest(),
	}

	_, err := goose.Run(context.Background(), Task{
		Prompt:        "Task ID: hive-123\nTitle: Fix the bug",
		Workspace:     workspace,
		Provider:      "openai",
		Model:         "local",
		OpenAIBaseURL: "http://127.0.0.1:8000/v1",
		OpenAIAPIKey:  "local",
		RuntimeDir:    runtimeDir,
		BundledConfig: []byte("provider: openai\n"),
	})
	if err == nil {
		t.Fatal("Run() error = nil, want failure")
	}

	var execErr *ExecutionError
	if !errors.As(err, &execErr) {
		t.Fatalf("Run() error = %T, want *ExecutionError", err)
	}
	if !strings.Contains(execErr.Error(), "AGENTS.md") {
		t.Fatalf("Run() error = %q, want missing document path", execErr.Error())
	}
	if len(fake.calls) != 0 {
		t.Fatalf("Run() command count = %d, want 0", len(fake.calls))
	}
	if _, statErr := os.Stat(filepath.Join(runtimeDir, "home")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Run() staged config despite contract failure, stat error = %v", statErr)
	}
}

func TestGooseRunFailsBeforeCommandWhenContractManifestUnvalidated(t *testing.T) {
	fake := &fakeCommandRunner{}
	workspace := repoScratch(t, "invalid-manifest-workspace")
	runtimeDir := repoScratch(t, "invalid-manifest-runtime")
	goose := Goose{
		Runner:   fake,
		Contract: contract.Manifest{},
	}

	_, err := goose.Run(context.Background(), Task{
		Prompt:        "Task ID: hive-123\nTitle: Fix the bug",
		Workspace:     workspace,
		Provider:      "openai",
		Model:         "local",
		OpenAIBaseURL: "http://127.0.0.1:8000/v1",
		OpenAIAPIKey:  "local",
		RuntimeDir:    runtimeDir,
		BundledConfig: []byte("provider: openai\n"),
	})
	if err == nil {
		t.Fatal("Run() error = nil, want failure")
	}

	var execErr *ExecutionError
	if !errors.As(err, &execErr) {
		t.Fatalf("Run() error = %T, want *ExecutionError", err)
	}
	if !strings.Contains(execErr.Error(), "unsupported version") {
		t.Fatalf("Run() error = %q, want manifest validation failure", execErr.Error())
	}
	if len(fake.calls) != 0 {
		t.Fatalf("Run() command count = %d, want 0", len(fake.calls))
	}
	if _, statErr := os.Stat(filepath.Join(runtimeDir, "home")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Run() staged config despite contract failure, stat error = %v", statErr)
	}
}

func TestGooseRunFailsBeforeCommandWhenContractDocumentEmpty(t *testing.T) {
	fake := &fakeCommandRunner{}
	workspace := repoScratch(t, "empty-workspace")
	runtimeDir := repoScratch(t, "empty-runtime")
	manifest := testContractManifest()
	writeContractDocuments(t, workspace, manifest)
	mustWriteFile(t, filepath.Join(workspace, "docs", "SKILL.md"), " \n\t")
	goose := Goose{
		Runner:   fake,
		Contract: manifest,
	}

	_, err := goose.Run(context.Background(), Task{
		Prompt:        "Task ID: hive-123\nTitle: Fix the bug",
		Workspace:     workspace,
		Provider:      "openai",
		Model:         "local",
		OpenAIBaseURL: "http://127.0.0.1:8000/v1",
		OpenAIAPIKey:  "local",
		RuntimeDir:    runtimeDir,
		BundledConfig: []byte("provider: openai\n"),
	})
	if err == nil {
		t.Fatal("Run() error = nil, want failure")
	}

	var execErr *ExecutionError
	if !errors.As(err, &execErr) {
		t.Fatalf("Run() error = %T, want *ExecutionError", err)
	}
	if !strings.Contains(execErr.Error(), "docs/SKILL.md") {
		t.Fatalf("Run() error = %q, want empty document path", execErr.Error())
	}
	if len(fake.calls) != 0 {
		t.Fatalf("Run() command count = %d, want 0", len(fake.calls))
	}
	if _, statErr := os.Stat(filepath.Join(runtimeDir, "home")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Run() staged config despite contract failure, stat error = %v", statErr)
	}
}

func TestGooseRunInjectsPolicyContractDocumentsAndAssignmentInOrder(t *testing.T) {
	fake := &fakeCommandRunner{
		output: CommandOutput{Combined: "Task finished cleanly\n"},
	}
	workspace := repoScratch(t, "ordered-workspace")
	runtimeDir := repoScratch(t, "ordered-runtime")
	manifest := testContractManifest()
	writeContractDocuments(t, workspace, manifest)
	assignment := "Task ID: hive-123\nTitle: Fix the bug\n\nFollow the original assignment exactly.\n  Preserve spacing.\n"
	goose := Goose{
		Command:  "goose",
		Policy:   "inspect local repository evidence first.\nUse Context7 opportunistically.",
		Runner:   fake,
		Contract: manifest,
	}

	_, err := goose.Run(context.Background(), Task{
		Prompt:        assignment,
		Workspace:     workspace,
		Provider:      "openai",
		Model:         "local",
		OpenAIBaseURL: "http://127.0.0.1:8000/v1",
		OpenAIAPIKey:  "local",
		RuntimeDir:    runtimeDir,
		BundledConfig: []byte("provider: openai\n"),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("Run() command count = %d, want 1", len(fake.calls))
	}

	prompt := promptArgument(t, fake.calls[0].Args)
	wantOrder := []string{
		localPolicyHeading,
		goose.Policy,
		"## Agent contract",
		"### Rules",
		"Read AGENTS.md first, then docs/SKILL.md.",
		"Required repository document: AGENTS.md",
		"Repository contract entrypoint.\n",
		"Required repository document: docs/SKILL.md",
		"Skill router content.\n",
		hiveAssignmentHeading,
		assignment,
	}
	assertOrderedSubstrings(t, prompt, wantOrder)
	if !strings.HasSuffix(prompt, hiveAssignmentHeading+"\n"+assignment) {
		t.Fatalf("Run() prompt suffix = %q, want verbatim assignment suffix", prompt)
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
	manifest := testContractManifest()
	workspace := repoScratch(t, "workspace")
	runtimeDir := repoScratch(t, "runtime")
	writeContractDocuments(t, workspace, manifest)
	task := Task{
		Prompt:        "Task ID: hive-123\nTitle: Fix the bug",
		Workspace:     workspace,
		Provider:      "openai",
		Model:         "Qwen3.6-35B-A3B",
		OpenAIBaseURL: "http://127.0.0.1:8000/v1",
		OpenAIAPIKey:  "local",
		GitHubToken:   "ghs_secret_token",
		RuntimeDir:    runtimeDir,
		BundledConfig: []byte("provider: openai\nbase_url: http://127.0.0.1:8000/v1\n"),
	}
	goose := Goose{
		Command:  "goose",
		Policy:   "inspect local repository evidence first.\nUse Context7 opportunistically.",
		Runner:   fake,
		Contract: manifest,
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
	wantPrompt := PrepareTaskPrompt(goose.Policy, manifest.PromptSection(contractDocuments(manifest)), task.Prompt)
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
			Combined: "GH_TOKEN=gho_super_secret\nAuthorization: ******\nfatal: push failed\n",
		},
		err: errors.New("exit status 1: GH_TOKEN=gho_super_secret"),
	}
	manifest := testContractManifest()
	workspace := repoScratch(t, "failure-workspace")
	writeContractDocuments(t, workspace, manifest)
	task := Task{
		Prompt:        "Task ID: hive-123\nTitle: Fix the bug",
		Workspace:     workspace,
		Provider:      "openai",
		Model:         "local",
		OpenAIBaseURL: "http://127.0.0.1:8000/v1",
		OpenAIAPIKey:  "local",
		RuntimeDir:    repoScratch(t, "failure-runtime"),
		BundledConfig: []byte("provider: openai\n"),
	}

	result, err := Goose{Runner: fake, Contract: manifest}.Run(context.Background(), task)
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

func TestTaskRuntimeDirIsolatesAndContainsTaskIDs(t *testing.T) {
	root := repoScratch(t, "task-runtime-root")
	first, err := TaskRuntimeDir(root, "task-one")
	if err != nil {
		t.Fatalf("TaskRuntimeDir() error = %v", err)
	}
	second, err := TaskRuntimeDir(root, "../task-two")
	if err != nil {
		t.Fatalf("TaskRuntimeDir() error = %v", err)
	}

	if first == second {
		t.Fatalf("TaskRuntimeDir() = %q for two task IDs, want distinct paths", first)
	}
	for _, runtimeDir := range []string{first, second} {
		relative, err := filepath.Rel(filepath.Clean(root), runtimeDir)
		if err != nil {
			t.Fatalf("Rel(%q, %q) error = %v", root, runtimeDir, err)
		}
		if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			t.Fatalf("TaskRuntimeDir() = %q, escapes root %q", runtimeDir, root)
		}
	}
}

func TestTaskRuntimeDirRejectsMissingTaskID(t *testing.T) {
	if _, err := TaskRuntimeDir("runtime", " \t"); !errors.Is(err, ErrMissingTaskID) {
		t.Fatalf("TaskRuntimeDir() error = %v, want %v", err, ErrMissingTaskID)
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

func testContractManifest() contract.Manifest {
	return contract.Manifest{
		Version: 1,
		RequiredDocuments: []contract.RequiredDocument{
			{Path: "AGENTS.md", Heading: "Required repository document: AGENTS.md"},
			{Path: "docs/SKILL.md", Heading: "Required repository document: docs/SKILL.md"},
		},
		Rules:              []string{"Read AGENTS.md first, then docs/SKILL.md."},
		ValidationCommands: []string{"go test ./internal/runner"},
	}
}

func writeContractDocuments(t *testing.T, workspace string, manifest contract.Manifest) {
	t.Helper()
	for _, doc := range contractDocuments(manifest) {
		mustWriteFile(t, filepath.Join(workspace, filepath.FromSlash(doc.Path)), doc.Content)
	}
}

func contractDocuments(manifest contract.Manifest) []contract.Document {
	return []contract.Document{
		{Path: manifest.RequiredDocuments[0].Path, Heading: manifest.RequiredDocuments[0].Heading, Content: "Repository contract entrypoint.\n"},
		{Path: manifest.RequiredDocuments[1].Path, Heading: manifest.RequiredDocuments[1].Heading, Content: "Skill router content.\n"},
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func promptArgument(t *testing.T, args []string) string {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-t" {
			return args[i+1]
		}
	}
	t.Fatalf("args = %#v, missing -t prompt", args)
	return ""
}

func assertOrderedSubstrings(t *testing.T, value string, wantOrder []string) {
	t.Helper()
	index := 0
	for _, want := range wantOrder {
		next := strings.Index(value[index:], want)
		if next < 0 {
			t.Fatalf("value missing %q in %q", want, value)
		}
		index += next + len(want)
	}
}
