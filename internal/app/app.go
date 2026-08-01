// Package app is the donate-clanker launcher.
//
// It does three things and nothing else:
//
//  1. host preflight (GitHub auth, Hive registration)
//  2. resolve the contributor's own model credential
//  3. run the VM in the foreground, handing it a bootstrap envelope
//
// Everything the agent does after that — the contributor WebSocket protocol,
// task selection, the tmux session, prompt injection, output capture — belongs
// to Hive. This launcher deliberately owns none of it.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/projectbluefin/donate-clanker/internal/config"
	"github.com/projectbluefin/donate-clanker/internal/credential"
	"github.com/projectbluefin/donate-clanker/internal/setup"
	"github.com/projectbluefin/donate-clanker/internal/vm"
)

var (
	ErrMissingRunnerImage = errors.New("missing VM runner image")
	ErrMissingRunID       = errors.New("missing run identifier")
)

type dependencies struct {
	checkGitHubAuth  func(context.Context, setup.CommandRunner) error
	ensureHiveSetup  func(context.Context, setup.SetupOptions) error
	readHiveEndpoint func(string) (endpoint string, token string, err error)
	resolveCredental func(context.Context, string, string, map[string]string, credential.Runner) (credential.Resolved, error)
	notifyContext    func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)
	commandRunner    setup.CommandRunner
	environment      map[string]string
	stdout           io.Writer
	stderr           io.Writer
	now              func() time.Time
}

// Run performs preflight and then blocks in the foreground until the VM exits
// or the operator interrupts it.
func Run(ctx context.Context, opts config.Options) error {
	return run(ctx, opts, defaultDependencies())
}

func defaultDependencies() dependencies {
	return dependencies{
		checkGitHubAuth:  setup.CheckGitHubAuth,
		ensureHiveSetup:  setup.EnsureHiveSetup,
		readHiveEndpoint: readHiveCredentials,
		resolveCredental: credential.Resolve,
		notifyContext:    signal.NotifyContext,
		commandRunner: setup.ExecCommandRunner{
			Stdin:  os.Stdin,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		},
		environment: credential.HostEnvironment(),
		stdout:      os.Stdout,
		stderr:      os.Stderr,
		now:         time.Now,
	}
}

func run(ctx context.Context, opts config.Options, deps dependencies) error {
	if opts.RunnerImage == "" {
		return fmt.Errorf("%w: set DONATE_CLANKER_VM_RUNNER_IMAGE to a pinned image digest", ErrMissingRunnerImage)
	}

	// Signals are wired before anything long-running so Ctrl-C always reaches
	// the VM. The launcher never detaches or backgrounds the guest.
	signalCtx, stopSignals := deps.notifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	if err := deps.checkGitHubAuth(signalCtx, deps.commandRunner); err != nil {
		return err
	}

	contributorEnv := filepath.Join(opts.HiveConfigDir, "contributor.env")
	if err := deps.ensureHiveSetup(signalCtx, setup.SetupOptions{
		ConfigPath:     contributorEnv,
		RepoDir:        opts.HiveSourceDir,
		Commit:         opts.HiveCommit,
		NonInteractive: opts.NonInteractive,
		Runner:         deps.commandRunner,
	}); err != nil {
		return err
	}

	endpoint, token, err := deps.readHiveEndpoint(contributorEnv)
	if err != nil {
		return err
	}

	resolved, err := deps.resolveCredental(signalCtx, opts.GooseProvider, opts.GooseModel, deps.environment, nil)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(deps.stdout, "✓ model credential resolved for %s (%s)\n", resolved.Provider, resolved.Source)
	// A credential that authenticates GitHub but not Copilot inference would
	// otherwise surface as an opaque agent failure minutes later, inside a
	// guest the contributor cannot easily read. Say it here, where the fix is.
	if resolved.Advisory != "" && deps.stderr != nil {
		_, _ = fmt.Fprintf(deps.stderr, "! %s\n", resolved.Advisory)
	}

	bootstrap := vm.Bootstrap{
		Version:           vm.BootstrapVersion,
		HiveEndpoint:      endpoint,
		RegistrationToken: token,
		Backend:           config.Backend,
		RunID:             fmt.Sprintf("donate-clanker-%d", deps.now().UnixNano()),
		GooseProvider:     resolved.Provider,
		GooseModel:        resolved.Model,
		ProviderSecret:    resolved.Secret,
	}
	if err := bootstrap.Validate(); err != nil {
		return err
	}

	path, err := writeBootstrap(opts.StateDir, bootstrap)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(deps.stdout, path)
	return nil
}

// writeBootstrap persists the envelope for the launcher to hand to the guest.
// The file carries a live credential, so it is created 0600 inside a 0700
// directory and replaced atomically.
func writeBootstrap(stateDir string, bootstrap vm.Bootstrap) (string, error) {
	if bootstrap.RunID == "" {
		return "", ErrMissingRunID
	}
	runDir := filepath.Join(stateDir, "run", bootstrap.RunID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return "", err
	}

	target := filepath.Join(runDir, "bootstrap.json")
	temp, err := os.OpenFile(target+".tmp", os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	if err := vm.SendBootstrap(temp, bootstrap); err != nil {
		_ = temp.Close()
		_ = os.Remove(temp.Name())
		return "", err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(temp.Name())
		return "", err
	}
	if err := os.Rename(temp.Name(), target); err != nil {
		_ = os.Remove(temp.Name())
		return "", err
	}
	return target, nil
}

// readHiveCredentials extracts the endpoint and registration token Hive's
// setup recipe wrote to contributor.env.
func readHiveCredentials(path string) (string, string, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", "", err
	}

	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}

	endpoint := values["HIVE_HUB"]
	if endpoint == "" {
		endpoint = values["HIVE_WS_URL"]
	}
	token := values["HIVE_REGISTRATION_TOKEN"]
	if endpoint == "" || token == "" {
		return "", "", fmt.Errorf("%s: missing HIVE_HUB or HIVE_REGISTRATION_TOKEN", path)
	}
	return endpoint, token, nil
}
