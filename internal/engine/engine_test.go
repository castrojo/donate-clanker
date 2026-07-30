package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"testing"
)

func TestDetectRespectsExplicitSelection(t *testing.T) {
	runner := &fakeCommandRunner{
		runResults: map[string]error{
			"podman version": nil,
		},
	}

	got, err := DetectWithRunner(context.Background(), PreferencePodman, runner)
	if err != nil {
		t.Fatalf("DetectWithRunner() error = %v", err)
	}
	if got.Name() != "podman" {
		t.Fatalf("DetectWithRunner() engine = %q, want podman", got.Name())
	}
}

func TestDetectRejectsUnavailableExplicitEngine(t *testing.T) {
	runner := &fakeCommandRunner{
		runResults: map[string]error{
			"docker version": errors.New("missing"),
		},
	}

	_, err := DetectWithRunner(context.Background(), PreferenceDocker, runner)
	if !errors.Is(err, ErrUnavailableEngine) {
		t.Fatalf("DetectWithRunner() error = %v, want ErrUnavailableEngine", err)
	}
}

func TestDetectFallsBackInDeterministicOrder(t *testing.T) {
	runner := &fakeCommandRunner{
		runResults: map[string]error{
			"podman version": errors.New("missing"),
			"docker version": nil,
		},
	}

	got, err := DetectWithRunner(context.Background(), PreferenceAuto, runner)
	if err != nil {
		t.Fatalf("DetectWithRunner() error = %v", err)
	}
	if got.Name() != "docker" {
		t.Fatalf("DetectWithRunner() engine = %q, want docker", got.Name())
	}
	if want := []string{"podman version", "docker version"}; !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("DetectWithRunner() calls = %#v, want %#v", runner.calls, want)
	}
}

func TestResolveRuntimeAutoProbesPodmanRunsc(t *testing.T) {
	runner := &fakeCommandRunner{
		runResults: map[string]error{
			"podman --runtime runsc info --format {{.Host.OCIRuntime.Name}}": nil,
		},
		runOutputs: map[string]string{
			"podman --runtime runsc info --format {{.Host.OCIRuntime.Name}}": "runsc\n",
		},
	}

	runtime, warning, err := ResolveRuntime(context.Background(), NewPodman(runner), "auto", false)
	if err != nil {
		t.Fatalf("ResolveRuntime() error = %v", err)
	}
	if runtime != "runsc" || warning != "" {
		t.Fatalf("ResolveRuntime() = (%q, %q), want (runsc, empty)", runtime, warning)
	}
	if want := []string{"podman --runtime runsc info --format {{.Host.OCIRuntime.Name}}"}; !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("ResolveRuntime() calls = %#v, want %#v", runner.calls, want)
	}
}

func TestResolveRuntimeFallsBackOrFailsStrictly(t *testing.T) {
	runner := &fakeCommandRunner{
		runResults: map[string]error{
			"podman --runtime runsc info --format {{.Host.OCIRuntime.Name}}": errors.New("runsc unavailable"),
		},
	}

	runtime, warning, err := ResolveRuntime(context.Background(), NewPodman(runner), "auto", false)
	if err != nil {
		t.Fatalf("ResolveRuntime() error = %v", err)
	}
	wantWarning := `warning: container runtime "runsc" is unavailable with podman; using the default runtime`
	if runtime != "" || warning != wantWarning {
		t.Fatalf("ResolveRuntime() = (%q, %q), want (empty, %q)", runtime, warning, wantWarning)
	}

	_, _, err = ResolveRuntime(context.Background(), NewPodman(runner), "auto", true)
	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("ResolveRuntime() error = %v, want ErrRuntimeUnavailable", err)
	}
}

func TestResolveRuntimeDoesNotProbeDocker(t *testing.T) {
	runner := &fakeCommandRunner{}

	runtime, warning, err := ResolveRuntime(context.Background(), NewDocker(runner), "auto", false)
	if err != nil {
		t.Fatalf("ResolveRuntime() error = %v", err)
	}
	wantWarning := `warning: container runtime "runsc" is unavailable with docker; using the default runtime`
	if runtime != "" || warning != wantWarning {
		t.Fatalf("ResolveRuntime() = (%q, %q), want (empty, %q)", runtime, warning, wantWarning)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("ResolveRuntime() calls = %#v, want no Docker runtime probe", runner.calls)
	}

	_, warning, err = ResolveRuntime(context.Background(), NewDocker(runner), "auto", true)
	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("ResolveRuntime() error = %v, want ErrRuntimeUnavailable", err)
	}
	if got := err.Error(); got != "requested container runtime unavailable: runsc" {
		t.Fatalf("ResolveRuntime() error = %q, want %q", got, "requested container runtime unavailable: runsc")
	}
	if warning != "" {
		t.Fatalf("ResolveRuntime() warning = %q, want empty", warning)
	}
}

func TestPodmanRunPassesSelectedRuntime(t *testing.T) {
	runner := &fakeCommandRunner{}

	if _, err := NewPodman(runner).Run(context.Background(), RunSpec{
		Name:    "worker",
		Image:   "worker:dev",
		Runtime: "runsc",
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if want := []string{"podman run --runtime runsc --rm --replace --name worker worker:dev"}; !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("Run() calls = %#v, want %#v", runner.calls, want)
	}
}

func TestPodmanPodCreatePassesSelectedRuntime(t *testing.T) {
	runner := &fakeCommandRunner{}

	if err := NewPodman(runner).PodCreate(context.Background(), PodSpec{
		Name:    "sandbox",
		Runtime: "runsc",
	}); err != nil {
		t.Fatalf("PodCreate() error = %v", err)
	}
	if want := []string{"podman --runtime runsc pod create --replace --name sandbox"}; !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("PodCreate() calls = %#v, want %#v", runner.calls, want)
	}
}

type fakeCommandRunner struct {
	runResults  map[string]error
	runOutputs  map[string]string
	calls       []string
	startErr    error
	startOutput io.ReadCloser
}

func (f *fakeCommandRunner) Run(_ context.Context, name string, args ...string) (CommandResult, error) {
	call := name
	for _, arg := range args {
		call += " " + arg
	}
	f.calls = append(f.calls, call)
	if err, ok := f.runResults[call]; ok {
		return CommandResult{Stdout: f.runOutputs[call]}, err
	}
	return CommandResult{Stdout: f.runOutputs[call]}, nil
}

func (f *fakeCommandRunner) Start(_ context.Context, name string, args ...string) (Process, error) {
	call := name
	for _, arg := range args {
		call += " " + arg
	}
	f.calls = append(f.calls, call)
	if f.startOutput == nil {
		f.startOutput = io.NopCloser(nilReader{})
	}
	return fakeProcess{reader: f.startOutput, err: f.startErr}, f.startErr
}

type fakeProcess struct {
	reader io.ReadCloser
	err    error
}

func (p fakeProcess) Wait() error                 { return p.err }
func (p fakeProcess) Signal(os.Signal) error      { return nil }
func (p fakeProcess) StdoutStderr() io.ReadCloser { return p.reader }

type nilReader struct{}

func (nilReader) Read([]byte) (int, error) { return 0, io.EOF }
