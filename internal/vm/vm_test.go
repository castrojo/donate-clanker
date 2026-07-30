package vm

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func testSpec() Spec {
	return Spec{
		RunnerImage: "sha256:" + strings.Repeat("a", 64),
		GuestImage:  "sha256:" + strings.Repeat("b", 64),
		Kernel:      "/run/donate-clanker/kernel",
		Initrd:      "/run/donate-clanker/initrd",
		GuestRootFS: "/run/donate-clanker/rootfs.ext4",
		Overlay:     "/run/donate-clanker/overlay.qcow2",
		StateDir:    "/home/test/.local/state/donate-clanker/run-1",
		RunID:       "run-1",
	}
}

func TestQEMUArgsContainOnlyGuestInputs(t *testing.T) {
	args, err := testSpec().QEMUArgs()
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(args, "\x00")
	for _, forbidden := range []string{"/workspace", "/home/test", "docker.sock", "podman.sock", ".git"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("QEMU args contain forbidden host input %q: %s", forbidden, got)
		}
	}
	for _, required := range []string{"-machine\x00microvm,accel=kvm", "-netdev\x00user,id=net", "virtio-serial-device"} {
		if !strings.Contains(got, required) {
			t.Fatalf("QEMU args missing %q: %s", required, got)
		}
	}
}

func TestRunnerArgsDoNotExposeHostSockets(t *testing.T) {
	args, err := testSpec().RunnerArgs()
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(args, "\x00")
	for _, forbidden := range []string{"docker.sock", "podman.sock", "/workspace", "/home/test/.config"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("runner args contain forbidden host input %q", forbidden)
		}
	}
}

func TestBootstrapRejectsUnknownFieldsAndMalformedValues(t *testing.T) {
	_, err := DecodeBootstrap(strings.NewReader(`{"version":1,"hive_endpoint":"wss://hive","registration_token":"secret","backend":"goose","run_id":"r","extra":"x"}`))
	if err == nil {
		t.Fatal("expected unknown field error")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("error leaked credential: %v", err)
	}
	if err := (Bootstrap{Version: 1, HiveEndpoint: "http://hive", RegistrationToken: "secret", Backend: "goose", RunID: "r"}).Validate(); err == nil {
		t.Fatal("expected endpoint validation error")
	}
}

func TestWaitReadyRequiresOrderedStages(t *testing.T) {
	var statuses strings.Builder
	for _, stage := range readinessStages {
		statuses.WriteString(`{"version":1,"type":"`)
		statuses.WriteString(stage)
		statuses.WriteString("\"}\n")
	}
	if err := WaitReady(context.Background(), strings.NewReader(statuses.String()), time.Second); err != nil {
		t.Fatal(err)
	}
	if err := WaitReady(context.Background(), strings.NewReader(`{"version":1,"type":"network"}`), time.Millisecond); err == nil {
		t.Fatal("expected out-of-order status error")
	}
}

type fakeProcess struct {
	mu           sync.Mutex
	signaled     bool
	killed       bool
	ignoreSignal bool
	done         chan struct{}
}

func (p *fakeProcess) Wait() error { <-p.done; return nil }
func (p *fakeProcess) Signal(os.Signal) error {
	p.mu.Lock()
	p.signaled = true
	p.mu.Unlock()
	if !p.ignoreSignal {
		close(p.done)
	}
	return nil
}
func (p *fakeProcess) Kill() error {
	p.mu.Lock()
	p.killed = true
	p.mu.Unlock()
	select {
	case <-p.done:
	default:
		close(p.done)
	}
	return nil
}

func TestCleanupIsBoundedAndIdempotent(t *testing.T) {
	p := &fakeProcess{done: make(chan struct{})}
	if err := Cleanup(context.Background(), p, time.Second); err != nil {
		t.Fatal(err)
	}
	p = &fakeProcess{done: make(chan struct{}), ignoreSignal: true}
	if err := Cleanup(context.Background(), p, 5*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.killed {
		t.Fatal("cleanup did not kill an unresponsive process")
	}
}

func TestVMStartUsesFakeCommand(t *testing.T) {
	p := &fakeProcess{done: make(chan struct{})}
	var got []string
	vm, err := New(testSpec(), CommandFunc(func(_ context.Context, arguments []string) (Process, error) {
		got = append([]string(nil), arguments...)
		return p, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := vm.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0] != "-machine" {
		t.Fatalf("unexpected command args: %v", got)
	}
	_ = p.Kill()
}

func TestSendBootstrap(t *testing.T) {
	var out bytes.Buffer
	b := Bootstrap{Version: 1, HiveEndpoint: "wss://hive", RegistrationToken: "secret", Backend: "goose", RunID: "r"}
	if err := SendBootstrap(&out, b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"version":1`) || !strings.Contains(out.String(), `"run_id":"r"`) {
		t.Fatalf("unexpected envelope: %s", out.String())
	}
}
