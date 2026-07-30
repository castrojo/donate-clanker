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

type fakeCommandRunner struct {
	runResults  map[string]error
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
		return CommandResult{}, err
	}
	return CommandResult{}, nil
}

func (f *fakeCommandRunner) Start(_ context.Context, _ string, _ ...string) (Process, error) {
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
