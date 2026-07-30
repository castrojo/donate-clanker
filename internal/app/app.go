package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/projectbluefin/donate-clanker/internal/config"
	"github.com/projectbluefin/donate-clanker/internal/engine"
	"github.com/projectbluefin/donate-clanker/internal/hive"
	"github.com/projectbluefin/donate-clanker/internal/pod"
	"github.com/projectbluefin/donate-clanker/internal/profile"
	"github.com/projectbluefin/donate-clanker/internal/setup"
)

var (
	ErrMissingHelperImage      = errors.New("missing helper image")
	ErrMissingContributorImage = errors.New("missing contributor image")
)

type podHandle interface {
	Close(context.Context) error
	WorkerEndpointURL() string
}

type dependencies struct {
	detectEngine       func(context.Context, engine.Preference) (engine.Engine, error)
	checkGitHubAuth    func(context.Context, setup.CommandRunner) error
	checkGooseConfig   func(string) error
	ensureHiveSetup    func(context.Context, setup.SetupOptions) error
	loadCredentials    func(string) (hive.Credentials, error)
	resolveMounts      func(config.Options) ([]config.Mount, error)
	loadBundledCatalog func() (profile.Catalog, error)
	loadCatalog        func(string) (profile.Catalog, error)
	createPod          func(context.Context, engine.Engine, pod.Spec) (podHandle, error)
	startModel         func(context.Context, podHandle, pod.ModelSpec) error
	startWorker        func(context.Context, podHandle, pod.WorkerSpec) (engine.Process, error)
	notifyContext      func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)
	commandRunner      setup.CommandRunner
	stdout             io.Writer
	now                func() time.Time
}

func Run(ctx context.Context, opts config.Options) error {
	return run(ctx, opts, defaultDependencies())
}

func defaultDependencies() dependencies {
	return dependencies{
		detectEngine: func(ctx context.Context, preference engine.Preference) (engine.Engine, error) {
			return engine.Detect(ctx, preference)
		},
		checkGitHubAuth:  setup.CheckGitHubAuth,
		checkGooseConfig: setup.CheckGooseLocalConfig,
		ensureHiveSetup:  setup.EnsureHiveSetup,
		loadCredentials: func(path string) (hive.Credentials, error) {
			return hive.LoadCredentials(path, nil)
		},
		resolveMounts:      config.ResolveMounts,
		loadBundledCatalog: profile.LoadBundled,
		loadCatalog:        profile.Load,
		createPod: func(ctx context.Context, eng engine.Engine, spec pod.Spec) (podHandle, error) {
			return pod.Create(ctx, eng, spec)
		},
		startModel: func(ctx context.Context, handle podHandle, spec pod.ModelSpec) error {
			real, ok := handle.(*pod.Handle)
			if !ok {
				return fmt.Errorf("unexpected pod handle %T", handle)
			}
			return pod.StartModel(ctx, real, spec)
		},
		startWorker: func(ctx context.Context, handle podHandle, spec pod.WorkerSpec) (engine.Process, error) {
			real, ok := handle.(*pod.Handle)
			if !ok {
				return nil, fmt.Errorf("unexpected pod handle %T", handle)
			}
			return pod.StartWorker(ctx, real, spec)
		},
		notifyContext: signal.NotifyContext,
		commandRunner: setup.ExecCommandRunner{
			Stdin:  os.Stdin,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		},
		stdout: os.Stdout,
		now:    time.Now,
	}
}

func run(ctx context.Context, opts config.Options, deps dependencies) error {
	if opts.HelperImage == "" {
		return ErrMissingHelperImage
	}
	if opts.ContributorImage == "" {
		return ErrMissingContributorImage
	}

	signalCtx, stopSignals := deps.notifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	eng, err := deps.detectEngine(signalCtx, engine.Preference(opts.Engine))
	if err != nil {
		return err
	}

	if err := deps.checkGitHubAuth(signalCtx, deps.commandRunner); err != nil {
		return err
	}
	if err := deps.checkGooseConfig(opts.GooseConfigPath); err != nil {
		return err
	}
	if err := deps.ensureHiveSetup(signalCtx, setup.SetupOptions{
		ConfigPath:     filepath.Join(opts.HiveConfigDir, "contributor.env"),
		RepoDir:        opts.HiveSourceDir,
		Commit:         opts.HiveCommit,
		NonInteractive: opts.NonInteractive,
		Runner:         deps.commandRunner,
	}); err != nil {
		return err
	}

	resolvedModel, err := resolveModel(opts, deps)
	if err != nil {
		return err
	}

	creds, err := deps.loadCredentials(filepath.Join(opts.HiveConfigDir, "contributor.env"))
	if err != nil {
		return err
	}

	mounts, err := deps.resolveMounts(opts)
	if err != nil {
		return err
	}

	handle, err := deps.createPod(signalCtx, eng, pod.Spec{
		NamePrefix:    fmt.Sprintf("donate-clanker-%d", deps.now().UnixNano()),
		ModelHostPort: 0,
		ContainerPort: opts.ModelContainerPort,
		Labels: map[string]string{
			"app.kubernetes.io/name": "donate-clanker",
		},
	})
	if err != nil {
		return err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = handle.Close(cleanupCtx)
	}()

	helperMounts, workerMounts := splitMounts(mounts)
	if err := deps.startModel(signalCtx, handle, pod.ModelSpec{
		Image:            opts.HelperImage,
		Mounts:           helperMounts,
		ReadinessTimeout: opts.ReadinessTimeout,
	}); err != nil {
		return err
	}

	workerEnv := workerEnvironment(resolvedModel, handle.WorkerEndpointURL(), creds)

	process, err := deps.startWorker(signalCtx, handle, pod.WorkerSpec{
		Image:   opts.ContributorImage,
		Env:     workerEnv,
		Mounts:  workerMounts,
		WorkDir: config.WorkspaceMountPath,
	})
	if err != nil {
		return err
	}

	output := process.StdoutStderr()
	if output != nil {
		go func() {
			defer output.Close()
			_, _ = io.Copy(deps.stdout, output)
		}()
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- process.Wait()
	}()

	select {
	case err := <-waitCh:
		return err
	case <-signalCtx.Done():
		_ = process.Signal(os.Interrupt)
		return signalCtx.Err()
	}
}

func resolveModel(opts config.Options, deps dependencies) (string, error) {
	if opts.Model != "" {
		return opts.Model, nil
	}
	if opts.Profile == "" {
		return config.DefaultGooseModel, nil
	}

	var (
		catalog profile.Catalog
		err     error
	)
	if opts.ProfileCatalogExplicit {
		catalog, err = deps.loadCatalog(opts.ProfileCatalogPath)
	} else {
		catalog, err = deps.loadBundledCatalog()
	}
	if err != nil {
		return "", err
	}
	if _, ok := catalog[opts.Profile]; !ok {
		return "", fmt.Errorf("unknown profile %q", opts.Profile)
	}
	return opts.Profile, nil
}

func splitMounts(mounts []config.Mount) ([]config.Mount, []config.Mount) {
	helperMounts := make([]config.Mount, 0, 1)
	workerMounts := make([]config.Mount, 0, 1)
	for _, mount := range mounts {
		switch mount.ContainerPath {
		case config.CacheMountPath:
			helperMounts = append(helperMounts, mount)
		case config.WorkspaceMountPath:
			workerMounts = append(workerMounts, mount)
		}
	}
	return helperMounts, workerMounts
}

func workerEnvironment(model, endpoint string, creds hive.Credentials) map[string]string {
	env := config.DefaultGooseEnvironment(model)
	env["OPENAI_BASE_URL"] = endpoint
	env["WORKSPACE"] = config.WorkspaceMountPath
	env["HIVE_REGISTRATION_TOKEN"] = creds.RegistrationToken
	env["HIVE_WS_URL"] = creds.WSURL
	if creds.CLIBackend != "" {
		env["AGENT_BACKEND"] = creds.CLIBackend
	}
	return env
}
