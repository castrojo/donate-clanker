package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/projectbluefin/donate-clanker/internal/config"
	"github.com/projectbluefin/donate-clanker/internal/engine"
	"github.com/projectbluefin/donate-clanker/internal/hive"
	"github.com/projectbluefin/donate-clanker/internal/pod"
	"github.com/projectbluefin/donate-clanker/internal/profile"
	"github.com/projectbluefin/donate-clanker/internal/setup"
)

func TestRunOrdersFoundationSteps(t *testing.T) {
	var calls []string
	deps := testDependencies(&calls)
	opts := testOptions()

	if err := run(context.Background(), opts, deps); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	want := []string{"detect", "github", "goose", "hive", "catalog", "creds", "mounts", "create-pod", "start-model", "start-worker", "wait", "close"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("run() calls = %#v, want %#v", calls, want)
	}
}

func TestRunStopsOnSetupFailure(t *testing.T) {
	var calls []string
	deps := testDependencies(&calls)
	deps.ensureHiveSetup = func(context.Context, setup.SetupOptions) error {
		calls = append(calls, "hive")
		return errors.New("setup failed")
	}

	err := run(context.Background(), testOptions(), deps)
	if err == nil || err.Error() != "setup failed" {
		t.Fatalf("run() error = %v, want setup failed", err)
	}
	if want := []string{"detect", "github", "goose", "hive"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("run() calls = %#v, want %#v", calls, want)
	}
}

func TestRunPassesNonInteractiveToHiveSetup(t *testing.T) {
	var calls []string
	deps := testDependencies(&calls)
	var got setup.SetupOptions
	deps.ensureHiveSetup = func(_ context.Context, opts setup.SetupOptions) error {
		calls = append(calls, "hive")
		got = opts
		return nil
	}

	opts := testOptions()
	opts.NonInteractive = true
	if err := run(context.Background(), opts, deps); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !got.NonInteractive {
		t.Fatalf("ensureHiveSetup options = %#v, want NonInteractive=true", got)
	}
}

func TestRunPassesHiveCommitOverrideToSetup(t *testing.T) {
	var calls []string
	deps := testDependencies(&calls)
	var got setup.SetupOptions
	deps.ensureHiveSetup = func(_ context.Context, opts setup.SetupOptions) error {
		calls = append(calls, "hive")
		got = opts
		return nil
	}

	opts := testOptions()
	opts.HiveCommit = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := run(context.Background(), opts, deps); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got.Commit != opts.HiveCommit {
		t.Fatalf("ensureHiveSetup options = %#v, want Commit=%q", got, opts.HiveCommit)
	}
}

func TestRunCleansUpWhenWorkerStartFails(t *testing.T) {
	var calls []string
	deps := testDependencies(&calls)
	deps.startWorker = func(context.Context, podHandle, pod.WorkerSpec) (engine.Process, error) {
		calls = append(calls, "start-worker")
		return nil, errors.New("worker failed")
	}

	err := run(context.Background(), testOptions(), deps)
	if err == nil || err.Error() != "worker failed" {
		t.Fatalf("run() error = %v, want worker failed", err)
	}
	if want := []string{"detect", "github", "goose", "hive", "catalog", "creds", "mounts", "create-pod", "start-model", "start-worker", "close"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("run() calls = %#v, want %#v", calls, want)
	}
}

func TestRunSignalsWorkerOnInterrupt(t *testing.T) {
	var calls []string
	deps := testDependencies(&calls)
	ctx, cancel := context.WithCancel(context.Background())
	process := &blockingProcess{release: make(chan struct{}), calls: &calls}
	deps.startWorker = func(context.Context, podHandle, pod.WorkerSpec) (engine.Process, error) {
		calls = append(calls, "start-worker")
		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
			close(process.release)
		}()
		return process, nil
	}
	deps.notifyContext = func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
		return context.WithCancel(parent)
	}

	err := run(ctx, testOptions(), deps)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run() error = %v, want context.Canceled", err)
	}
	if !process.signaled {
		t.Fatal("run() did not signal worker on interrupt")
	}
	if want := []string{"detect", "github", "goose", "hive", "catalog", "creds", "mounts", "create-pod", "start-model", "start-worker", "signal", "close"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("run() calls = %#v, want %#v", calls, want)
	}
}

func TestRunScopesWorkerMountsAndEnv(t *testing.T) {
	var calls []string
	deps := testDependencies(&calls)
	var modelSpec pod.ModelSpec
	var workerSpec pod.WorkerSpec
	deps.loadCredentials = func(string) (hive.Credentials, error) {
		calls = append(calls, "creds")
		return hive.Credentials{RegistrationToken: "hive-secret", WSURL: "wss://example.invalid/api/contribute/ws", CLIBackend: "goose"}, nil
	}
	deps.startModel = func(_ context.Context, _ podHandle, spec pod.ModelSpec) error {
		calls = append(calls, "start-model")
		modelSpec = spec
		return nil
	}
	deps.startWorker = func(_ context.Context, _ podHandle, spec pod.WorkerSpec) (engine.Process, error) {
		calls = append(calls, "start-worker")
		workerSpec = spec
		return &successfulProcess{calls: &calls}, nil
	}

	if err := run(context.Background(), testOptions(), deps); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if len(modelSpec.Mounts) != 1 || modelSpec.Mounts[0].ContainerPath != config.CacheMountPath {
		t.Fatalf("startModel mounts = %#v, want only cache", modelSpec.Mounts)
	}
	if len(workerSpec.Mounts) != 1 || workerSpec.Mounts[0].ContainerPath != config.WorkspaceMountPath {
		t.Fatalf("startWorker mounts = %#v, want only workspace", workerSpec.Mounts)
	}
	for _, key := range []string{"HIVE_CONFIG_DIR", "GITHUB_CONFIG_DIR", "GH_TOKEN", "GITHUB_TOKEN", "GH_CONFIG_DIR"} {
		if got := workerSpec.Env[key]; got != "" {
			t.Fatalf("worker env[%q] = %q, want empty", key, got)
		}
	}
	for key, want := range map[string]string{
		"HIVE_REGISTRATION_TOKEN": "hive-secret",
		"HIVE_WS_URL":             "wss://example.invalid/api/contribute/ws",
		"AGENT_BACKEND":           "goose",
		"WORKSPACE":               config.WorkspaceMountPath,
		"OPENAI_BASE_URL":         "http://127.0.0.1:8000/v1",
	} {
		if got := workerSpec.Env[key]; got != want {
			t.Fatalf("worker env[%q] = %q, want %q", key, got, want)
		}
	}
}

func testDependencies(calls *[]string) dependencies {
	return dependencies{
		detectEngine: func(context.Context, engine.Preference) (engine.Engine, error) {
			*calls = append(*calls, "detect")
			return fakeEngine{}, nil
		},
		checkGitHubAuth: func(context.Context, setup.CommandRunner) error {
			*calls = append(*calls, "github")
			return nil
		},
		checkGooseConfig: func(string) error {
			*calls = append(*calls, "goose")
			return nil
		},
		ensureHiveSetup: func(context.Context, setup.SetupOptions) error {
			*calls = append(*calls, "hive")
			return nil
		},
		loadCredentials: func(string) (hive.Credentials, error) {
			*calls = append(*calls, "creds")
			return hive.Credentials{RegistrationToken: "hive-secret", WSURL: "wss://example.invalid/api/contribute/ws", CLIBackend: "goose"}, nil
		},
		resolveMounts: func(config.Options) ([]config.Mount, error) {
			*calls = append(*calls, "mounts")
			return []config.Mount{
				{HostPath: "/workspace", ContainerPath: config.WorkspaceMountPath},
				{HostPath: "/cache", ContainerPath: config.CacheMountPath},
			}, nil
		},
		loadBundledCatalog: func() (profile.Catalog, error) {
			*calls = append(*calls, "catalog")
			return profile.Catalog{"Qwen3.5-4B": {}}, nil
		},
		loadCatalog: func(string) (profile.Catalog, error) {
			*calls = append(*calls, "catalog")
			return profile.Catalog{"Qwen3.5-4B": {}}, nil
		},
		createPod: func(context.Context, engine.Engine, pod.Spec) (podHandle, error) {
			*calls = append(*calls, "create-pod")
			return &fakeHandle{calls: calls}, nil
		},
		startModel: func(context.Context, podHandle, pod.ModelSpec) error {
			*calls = append(*calls, "start-model")
			return nil
		},
		startWorker: func(context.Context, podHandle, pod.WorkerSpec) (engine.Process, error) {
			*calls = append(*calls, "start-worker")
			return &successfulProcess{calls: calls}, nil
		},
		notifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
			return context.WithCancel(parent)
		},
		commandRunner: fakeCommandRunner{},
		stdout:        io.Discard,
		now:           func() time.Time { return time.Unix(1, 0) },
	}
}

func testOptions() config.Options {
	return config.Options{
		Engine:             config.EngineAuto,
		Workspace:          "/workspace",
		Profile:            "Qwen3.5-4B",
		CacheDir:           "/cache",
		HiveConfigDir:      "/hive",
		GooseConfigPath:    "/goose/config.yaml",
		ProfileCatalogPath: "/catalog.json",
		HelperImage:        "helper@sha256:abc",
		ContributorImage:   "worker:dev",
		HiveSourceDir:      "/hive-src",
		ReadinessTimeout:   time.Second,
		ModelContainerPort: 8000,
	}
}

func TestResolveModelUsesBundledCatalogOutsideCheckout(t *testing.T) {
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	scratch := appScratch(t, "cwd-bundled")
	if err := os.Chdir(scratch); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	opts, err := config.Parse([]string{"--profile", "Qwen3.5-4B"}, map[string]string{
		"HOME": "/home/tester",
		"PWD":  "/work/repo",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if got, want := opts.ProfileCatalogPath, filepath.Join(scratch, "image", "config", "models.json"); got != want {
		t.Fatalf("Parse() profile catalog path = %q, want %q", got, want)
	}
	if opts.ProfileCatalogExplicit {
		t.Fatal("Parse() unexpectedly marked default profile catalog as explicit")
	}

	model, err := resolveModel(opts, dependencies{
		loadBundledCatalog: profile.LoadBundled,
		loadCatalog: func(string) (profile.Catalog, error) {
			t.Fatal("resolveModel() unexpectedly used explicit profile catalog override")
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("resolveModel() error = %v", err)
	}
	if model != "Qwen3.5-4B" {
		t.Fatalf("resolveModel() model = %q, want %q", model, "Qwen3.5-4B")
	}
}

func TestResolveModelHonorsExplicitProfileCatalogOverride(t *testing.T) {
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	scratch := appScratch(t, "cwd-override")
	if err := os.Chdir(scratch); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	catalogPath := writeAppScratchFile(t, filepath.Join("catalogs", "override.json"), []byte(`{
		"custom": {
			"context_size": 32768,
			"thinking": false,
			"runtime_args": ["--thinking", "false"]
		}
	}`))

	opts, err := config.Parse([]string{"--profile", "custom", "--profile-catalog", catalogPath}, map[string]string{
		"HOME": "/home/tester",
		"PWD":  "/work/repo",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !opts.ProfileCatalogExplicit {
		t.Fatal("Parse() did not mark --profile-catalog override as explicit")
	}

	model, err := resolveModel(opts, dependencies{
		loadBundledCatalog: func() (profile.Catalog, error) {
			t.Fatal("resolveModel() unexpectedly used bundled catalog")
			return nil, nil
		},
		loadCatalog: profile.Load,
	})
	if err != nil {
		t.Fatalf("resolveModel() error = %v", err)
	}
	if model != "custom" {
		t.Fatalf("resolveModel() model = %q, want %q", model, "custom")
	}
}

type fakeHandle struct {
	calls *[]string
}

func (h *fakeHandle) Close(context.Context) error {
	*h.calls = append(*h.calls, "close")
	return nil
}

func (h *fakeHandle) WorkerEndpointURL() string { return "http://127.0.0.1:8000/v1" }

type successfulProcess struct {
	calls *[]string
}

func (p *successfulProcess) Wait() error {
	*p.calls = append(*p.calls, "wait")
	return nil
}
func (p *successfulProcess) Signal(os.Signal) error { return nil }
func (p *successfulProcess) StdoutStderr() io.ReadCloser {
	return io.NopCloser(noopReader{})
}

type blockingProcess struct {
	release  chan struct{}
	signaled bool
	calls    *[]string
}

func (p *blockingProcess) Wait() error {
	<-p.release
	return nil
}
func (p *blockingProcess) Signal(os.Signal) error {
	p.signaled = true
	if p.calls != nil {
		*p.calls = append(*p.calls, "signal")
	}
	return nil
}
func (p *blockingProcess) StdoutStderr() io.ReadCloser { return io.NopCloser(noopReader{}) }

type noopReader struct{}

func (noopReader) Read([]byte) (int, error) { return 0, io.EOF }

type fakeEngine struct{}

func (fakeEngine) Name() string                                                { return "fake" }
func (fakeEngine) Version(context.Context) error                               { return nil }
func (fakeEngine) PodCreate(context.Context, engine.PodSpec) error             { return nil }
func (fakeEngine) Run(context.Context, engine.RunSpec) (engine.Process, error) { return nil, nil }
func (fakeEngine) Stop(context.Context, string) error                          { return nil }
func (fakeEngine) Remove(context.Context, string) error                        { return nil }
func (fakeEngine) RemovePod(context.Context, string) error                     { return nil }

func appScratch(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "internal", "app", ".app-test", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(repoRoot(t), "internal", "app", ".app-test")) })
	return dir
}

func writeAppScratchFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(appScratch(t, "files"), name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

type fakeCommandRunner struct{}

func (fakeCommandRunner) Run(context.Context, string, ...string) (setup.CommandResult, error) {
	return setup.CommandResult{}, nil
}
