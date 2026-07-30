package pod

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/projectbluefin/donate-clanker/internal/config"
	"github.com/projectbluefin/donate-clanker/internal/engine"
)

func TestCreateStartModelStartWorkerAndClose(t *testing.T) {
	const containerPort = 8123

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	fakeEngine := &recordingEngine{}
	handle, err := Create(context.Background(), fakeEngine, Spec{
		NamePrefix:    "test",
		ModelHostPort: listener.Addr().(*net.TCPAddr).Port,
		ContainerPort: containerPort,
		Runtime:       "runsc",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got, want := handle.WorkerEndpointURL(), "http://127.0.0.1:8123/v1"; got != want {
		t.Fatalf("WorkerEndpointURL() = %q, want %q", got, want)
	}

	go acceptOnce(listener)
	if err := StartModel(context.Background(), handle, ModelSpec{
		Image:            "helper@sha256:abc",
		Mounts:           []config.Mount{{HostPath: "/cache", ContainerPath: config.CacheMountPath}},
		ReadinessTimeout: time.Second,
		Runtime:          "runsc",
	}); err != nil {
		t.Fatalf("StartModel() error = %v", err)
	}

	process, err := StartWorker(context.Background(), handle, WorkerSpec{
		Image:   "worker:dev",
		Mounts:  []config.Mount{{HostPath: "/workspace", ContainerPath: config.WorkspaceMountPath}},
		Runtime: "runsc",
	})
	if err != nil {
		t.Fatalf("StartWorker() error = %v", err)
	}
	if err := process.Wait(); err != nil {
		t.Fatalf("worker Wait() error = %v", err)
	}

	if err := handle.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	want := []string{
		"pod test",
		"run test-model helper@sha256:abc detach=true",
		"run test-worker worker:dev detach=false",
		"stop test-worker",
		"remove test-worker",
		"stop test-model",
		"remove test-model",
		"remove-pod test",
	}
	if !reflect.DeepEqual(fakeEngine.calls, want) {
		t.Fatalf("calls = %#v, want %#v", fakeEngine.calls, want)
	}
	if got, want := fakeEngine.podSpecs[0].ContainerPort, containerPort; got != want {
		t.Fatalf("pod container port = %d, want %d", got, want)
	}
	if got := fakeEngine.podSpecs[0].Runtime; got != "runsc" {
		t.Fatalf("pod runtime = %q, want runsc", got)
	}
	if got, want := fakeEngine.runSpecs[0].ContainerPort, containerPort; got != want {
		t.Fatalf("helper container port = %d, want %d", got, want)
	}
	for _, spec := range fakeEngine.runSpecs {
		if spec.Runtime != "runsc" {
			t.Fatalf("run runtime = %q, want runsc", spec.Runtime)
		}
	}
}

func TestStartModelTimesOutAndWorkerIsSkipped(t *testing.T) {
	fakeEngine := &recordingEngine{}
	handle, err := Create(context.Background(), fakeEngine, Spec{
		NamePrefix:    "test",
		ModelHostPort: 1,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err = StartModel(context.Background(), handle, ModelSpec{
		Image:            "helper@sha256:abc",
		ReadinessTimeout: 200 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("StartModel() error = nil, want timeout")
	}
	if got := len(fakeEngine.runSpecs); got != 1 {
		t.Fatalf("run count = %d, want 1", got)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	fakeEngine := &recordingEngine{}
	handle := &Handle{
		engine:     fakeEngine,
		podName:    "test",
		helperName: "test-model",
		workerName: "test-worker",
	}

	if err := handle.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := handle.Close(context.Background()); err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
	}
	if want := []string{
		"stop test-worker",
		"remove test-worker",
		"stop test-model",
		"remove test-model",
		"remove-pod test",
	}; !reflect.DeepEqual(fakeEngine.calls, want) {
		t.Fatalf("calls = %#v, want %#v", fakeEngine.calls, want)
	}
}

type recordingEngine struct {
	calls    []string
	podSpecs []engine.PodSpec
	runSpecs []engine.RunSpec
}

func (e *recordingEngine) Name() string                  { return "fake" }
func (e *recordingEngine) Version(context.Context) error { return nil }
func (e *recordingEngine) PodCreate(_ context.Context, spec engine.PodSpec) error {
	e.podSpecs = append(e.podSpecs, spec)
	e.calls = append(e.calls, "pod "+spec.Name)
	return nil
}
func (e *recordingEngine) Run(_ context.Context, spec engine.RunSpec) (engine.Process, error) {
	e.runSpecs = append(e.runSpecs, spec)
	e.calls = append(e.calls, "run "+spec.Name+" "+spec.Image+" detach="+boolString(spec.Detach))
	return fakeProcess{}, nil
}
func (e *recordingEngine) Stop(_ context.Context, name string) error {
	e.calls = append(e.calls, "stop "+name)
	return nil
}
func (e *recordingEngine) Remove(_ context.Context, name string) error {
	e.calls = append(e.calls, "remove "+name)
	return nil
}
func (e *recordingEngine) RemovePod(_ context.Context, name string) error {
	e.calls = append(e.calls, "remove-pod "+name)
	return nil
}

type fakeProcess struct{}

func (fakeProcess) Wait() error                 { return nil }
func (fakeProcess) Signal(os.Signal) error      { return nil }
func (fakeProcess) StdoutStderr() io.ReadCloser { return io.NopCloser(nilReader{}) }

type nilReader struct{}

func (nilReader) Read([]byte) (int, error) { return 0, io.EOF }

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func acceptOnce(listener net.Listener) {
	conn, err := listener.Accept()
	if err == nil {
		_ = conn.Close()
	}
}

func TestToEngineMountsPreservesReadOnly(t *testing.T) {
	mounts := toEngineMounts([]config.Mount{{HostPath: filepath.Clean("/cache"), ContainerPath: config.CacheMountPath, ReadOnly: true}})
	if len(mounts) != 1 || !mounts[0].ReadOnly {
		t.Fatalf("toEngineMounts() = %#v", mounts)
	}
}

func TestCloseCollectsErrors(t *testing.T) {
	handle := &Handle{
		engine:     &failingCloseEngine{},
		podName:    "pod",
		helperName: "helper",
		workerName: "worker",
	}
	if err := handle.Close(context.Background()); err == nil {
		t.Fatal("Close() error = nil, want joined error")
	}
}

type failingCloseEngine struct{ recordingEngine }

func (f *failingCloseEngine) Stop(context.Context, string) error { return errors.New("stop failed") }
