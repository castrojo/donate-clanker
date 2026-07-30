package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEnsureHiveSetupReturnsWhenConfigAlreadyValid(t *testing.T) {
	root := t.TempDir()
	configPath := writeHiveConfig(t, filepath.Join(root, "contributor.env"))

	runner := &fakeSetupRunner{}
	err := EnsureHiveSetup(context.Background(), SetupOptions{
		ConfigPath: configPath,
		RepoDir:    filepath.Join(root, "hive-src"),
		Runner:     runner,
	})
	if err != nil {
		t.Fatalf("EnsureHiveSetup() error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("EnsureHiveSetup() calls = %#v, want none", runner.calls)
	}
}

func TestEnsureHiveSetupRunsUpstreamSetupWhenConfigMissing(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "hive-src")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "Justfile"), []byte("contribute-setup:\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	configPath := filepath.Join(root, "contributor.env")
	runner := newPinnedHiveRunner(repoDir)
	runner.canRunAttached = true
	runner.onRunAttached = func(name string, args []string) {
		if name == "just" {
			writeHiveConfig(t, configPath)
		}
	}

	err := EnsureHiveSetup(context.Background(), SetupOptions{
		ConfigPath: configPath,
		RepoDir:    repoDir,
		Runner:     runner,
	})
	if err != nil {
		t.Fatalf("EnsureHiveSetup() error = %v", err)
	}

	want := []string{
		"git -C " + repoDir + " status --porcelain",
		"git -C " + repoDir + " remote get-url origin",
		"git -C " + repoDir + " fetch --depth 1 origin " + defaultHiveCommit,
		"git -C " + repoDir + " checkout --detach -f FETCH_HEAD",
		"git -C " + repoDir + " rev-parse HEAD",
		"attached just --working-directory " + repoDir + " --justfile " + filepath.Join(repoDir, "Justfile") + " contribute-setup goose",
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("EnsureHiveSetup() calls = %#v, want %#v", runner.calls, want)
	}
}

func TestEnsureHiveSetupInitializesPinnedCheckoutWhenSourceMissing(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "hive-src")
	configPath := filepath.Join(root, "contributor.env")

	runner := &fakeSetupRunner{
		canRunAttached: true,
		onRunAttached: func(name string, args []string) {
			if name == "just" {
				writeHiveConfig(t, configPath)
			}
		},
		runCommand: newCommandResultRunner(map[string]CommandResult{
			"git init " + repoDir: {},
			"git -C " + repoDir + " remote add origin https://github.com/kubestellar/hive": {},
			"git -C " + repoDir + " fetch --depth 1 origin " + defaultHiveCommit:           {},
			"git -C " + repoDir + " checkout --detach -f FETCH_HEAD":                       {},
			"git -C " + repoDir + " rev-parse HEAD":                                        {Stdout: defaultHiveCommit + "\n"},
		}),
	}

	err := EnsureHiveSetup(context.Background(), SetupOptions{
		ConfigPath: configPath,
		RepoDir:    repoDir,
		Runner:     runner,
	})
	if err != nil {
		t.Fatalf("EnsureHiveSetup() error = %v", err)
	}

	want := []string{
		"git init " + repoDir,
		"git -C " + repoDir + " remote add origin https://github.com/kubestellar/hive",
		"git -C " + repoDir + " fetch --depth 1 origin " + defaultHiveCommit,
		"git -C " + repoDir + " checkout --detach -f FETCH_HEAD",
		"git -C " + repoDir + " rev-parse HEAD",
		"attached just --working-directory " + repoDir + " --justfile " + filepath.Join(repoDir, "Justfile") + " contribute-setup goose",
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("EnsureHiveSetup() calls = %#v, want %#v", runner.calls, want)
	}
}

func TestEnsureHiveSetupRejectsMutableCommitReference(t *testing.T) {
	root := t.TempDir()
	runner := &fakeSetupRunner{canRunAttached: true}

	err := EnsureHiveSetup(context.Background(), SetupOptions{
		ConfigPath: filepath.Join(root, "contributor.env"),
		RepoDir:    filepath.Join(root, "hive-src"),
		Commit:     "v2",
		Runner:     runner,
	})
	if err == nil {
		t.Fatal("EnsureHiveSetup() error = nil, want invalid commit error")
	}
	if got := err.Error(); !strings.Contains(got, "40-character commit SHA") || !strings.Contains(got, "v2") {
		t.Fatalf("EnsureHiveSetup() error = %q, want immutable commit guidance", got)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("EnsureHiveSetup() calls = %#v, want none", runner.calls)
	}
}

func TestEnsureHiveSetupRejectsUnexpectedOriginBeforeExecution(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "hive-src")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	runner := &fakeSetupRunner{
		canRunAttached: true,
		runCommand: newCommandResultRunner(map[string]CommandResult{
			"git -C " + repoDir + " status --porcelain":    {},
			"git -C " + repoDir + " remote get-url origin": {Stdout: "https://example.com/fork/hive\n"},
		}),
	}

	err := EnsureHiveSetup(context.Background(), SetupOptions{
		ConfigPath: filepath.Join(root, "contributor.env"),
		RepoDir:    repoDir,
		Runner:     runner,
	})
	if err == nil {
		t.Fatal("EnsureHiveSetup() error = nil, want origin mismatch")
	}
	if got := err.Error(); !strings.Contains(got, "points at") || !strings.Contains(got, "example.com/fork/hive") {
		t.Fatalf("EnsureHiveSetup() error = %q, want origin mismatch detail", got)
	}
	want := []string{
		"git -C " + repoDir + " status --porcelain",
		"git -C " + repoDir + " remote get-url origin",
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("EnsureHiveSetup() calls = %#v, want %#v", runner.calls, want)
	}
}

func TestEnsureHiveSetupVerifiesPinnedCommitBeforeExecution(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "hive-src")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	runner := &fakeSetupRunner{
		canRunAttached: true,
		runCommand: newCommandResultRunner(map[string]CommandResult{
			"git -C " + repoDir + " status --porcelain":                          {},
			"git -C " + repoDir + " remote get-url origin":                       {Stdout: defaultHiveRepoURL + "\n"},
			"git -C " + repoDir + " fetch --depth 1 origin " + defaultHiveCommit: {},
			"git -C " + repoDir + " checkout --detach -f FETCH_HEAD":             {},
			"git -C " + repoDir + " rev-parse HEAD":                              {Stdout: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"},
		}),
	}

	err := EnsureHiveSetup(context.Background(), SetupOptions{
		ConfigPath: filepath.Join(root, "contributor.env"),
		RepoDir:    repoDir,
		Runner:     runner,
	})
	if err == nil {
		t.Fatal("EnsureHiveSetup() error = nil, want pin verification error")
	}
	if got := err.Error(); !strings.Contains(got, "verify pinned hive source") || !strings.Contains(got, "want") {
		t.Fatalf("EnsureHiveSetup() error = %q, want pin verification detail", got)
	}
	for _, call := range runner.calls {
		if strings.HasPrefix(call, "attached just ") {
			t.Fatalf("EnsureHiveSetup() ran upstream setup before commit verification: %#v", runner.calls)
		}
	}
}

func TestEnsureHiveSetupRequiresPreSeedInNonInteractiveMode(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "hive-src")

	runner := &fakeSetupRunner{canRunAttached: true}
	err := EnsureHiveSetup(context.Background(), SetupOptions{
		ConfigPath:     filepath.Join(root, "contributor.env"),
		RepoDir:        repoDir,
		NonInteractive: true,
		Runner:         runner,
	})
	if !errors.Is(err, ErrInteractiveSetupRequired) {
		t.Fatalf("EnsureHiveSetup() error = %v, want ErrInteractiveSetupRequired", err)
	}
	if got := err.Error(); !strings.Contains(got, "pre-seed") || !strings.Contains(got, "kubestellar/hive") {
		t.Fatalf("EnsureHiveSetup() error = %q, want pre-seed guidance", got)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("EnsureHiveSetup() calls = %#v, want none", runner.calls)
	}
}

func TestEnsureHiveSetupRequiresAttachedTerminalWhenConfigMissing(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "hive-src")

	runner := &fakeSetupRunner{}
	err := EnsureHiveSetup(context.Background(), SetupOptions{
		ConfigPath: filepath.Join(root, "contributor.env"),
		RepoDir:    repoDir,
		Runner:     runner,
	})
	if !errors.Is(err, ErrInteractiveSetupRequired) {
		t.Fatalf("EnsureHiveSetup() error = %v, want ErrInteractiveSetupRequired", err)
	}
	if got := err.Error(); !strings.Contains(got, "interactive terminal") {
		t.Fatalf("EnsureHiveSetup() error = %q, want interactive terminal guidance", got)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("EnsureHiveSetup() calls = %#v, want none", runner.calls)
	}
}

func TestEnsureHiveSetupRedactsAttachedFailureOutput(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "hive-src")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "Justfile"), []byte("contribute-setup:\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := newPinnedHiveRunner(repoDir)
	runner.canRunAttached = true
	runner.attachedResult = CommandResult{
		Stderr: "registration failed HIVE_REGISTRATION_TOKEN=super-secret-token",
	}
	runner.attachedErr = errors.New("exit status 1")

	err := EnsureHiveSetup(context.Background(), SetupOptions{
		ConfigPath: filepath.Join(root, "contributor.env"),
		RepoDir:    repoDir,
		Runner:     runner,
	})
	if err == nil {
		t.Fatal("EnsureHiveSetup() error = nil, want error")
	}
	if got := err.Error(); !strings.Contains(got, "registration failed") {
		t.Fatalf("EnsureHiveSetup() error = %q, want attached stderr detail", got)
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatalf("EnsureHiveSetup() error leaked secret: %q", err)
	}
}

func TestEnsureHiveSetupRejectsInvalidExistingConfig(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "contributor.env")
	if err := os.WriteFile(configPath, []byte("HIVE_REGISTRATION_TOKEN=\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := EnsureHiveSetup(context.Background(), SetupOptions{
		ConfigPath: configPath,
		RepoDir:    filepath.Join(root, "hive-src"),
		Runner:     &fakeSetupRunner{},
	})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("EnsureHiveSetup() error = %v, want ErrInvalidConfig", err)
	}
}

type fakeSetupRunner struct {
	calls              []string
	onRun              func(name string, args []string)
	onRunAttached      func(name string, args []string)
	canRunAttached     bool
	result             CommandResult
	err                error
	attachedResult     CommandResult
	attachedErr        error
	runCommand         func(name string, args []string) (CommandResult, error)
	runAttachedCommand func(name string, args []string) (CommandResult, error)
}

func (f *fakeSetupRunner) Run(_ context.Context, name string, args ...string) (CommandResult, error) {
	call := name
	for _, arg := range args {
		call += " " + arg
	}
	f.calls = append(f.calls, call)
	if f.onRun != nil {
		f.onRun(name, args)
	}
	if f.runCommand != nil {
		return f.runCommand(name, args)
	}
	return f.result, f.err
}

func (f *fakeSetupRunner) CanRunAttached() bool {
	return f.canRunAttached
}

func (f *fakeSetupRunner) RunAttached(_ context.Context, name string, args ...string) (CommandResult, error) {
	call := "attached " + name
	for _, arg := range args {
		call += " " + arg
	}
	f.calls = append(f.calls, call)
	if f.onRunAttached != nil {
		f.onRunAttached(name, args)
	}
	if f.runAttachedCommand != nil {
		return f.runAttachedCommand(name, args)
	}
	return f.attachedResult, f.attachedErr
}

func newPinnedHiveRunner(repoDir string) *fakeSetupRunner {
	results := map[string]CommandResult{
		fmt.Sprintf("git -C %s status --porcelain", repoDir):                           {},
		fmt.Sprintf("git -C %s remote get-url origin", repoDir):                        {Stdout: defaultHiveRepoURL + "\n"},
		fmt.Sprintf("git -C %s fetch --depth 1 origin %s", repoDir, defaultHiveCommit): {},
		fmt.Sprintf("git -C %s checkout --detach -f FETCH_HEAD", repoDir):              {},
		fmt.Sprintf("git -C %s rev-parse HEAD", repoDir):                               {Stdout: defaultHiveCommit + "\n"},
	}
	return &fakeSetupRunner{
		runCommand: newCommandResultRunner(results),
	}
}

func newCommandResultRunner(results map[string]CommandResult) func(name string, args []string) (CommandResult, error) {
	return func(name string, args []string) (CommandResult, error) {
		call := name
		for _, arg := range args {
			call += " " + arg
		}
		result, ok := results[call]
		if !ok {
			return CommandResult{}, fmt.Errorf("unexpected command: %s", call)
		}
		return result, nil
	}
}

func writeHiveConfig(t *testing.T, path string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte("HIVE_REGISTRATION_TOKEN=redacted\nHIVE_HUB=wss://example.invalid/contribute\nCONTRIBUTOR_ID=123\nCONTRIBUTOR_USERNAME=tester\nAGENT_BACKEND=goose\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
