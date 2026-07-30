package vm

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultControlChannel = "org.projectbluefin.donate-clanker.bootstrap"
	DefaultMemoryMiB      = 2048
	DefaultCPUs           = 2
	DefaultReadyTimeout   = 2 * time.Minute
	DefaultCleanupTimeout = 10 * time.Second
)

var digestPattern = regexp.MustCompile(`^(sha256:[0-9a-f]{64}|.+@sha256:[0-9a-f]{64})$`)

// Spec describes one disposable donate-clanker microVM.
type Spec struct {
	RunnerImage      string
	GuestImage       string
	Kernel           string
	Initrd           string
	GuestKernel      string
	GuestRootFS      string
	Overlay          string
	StateDir         string
	Channel          string
	ControlChannel   string
	Architecture     string
	KVM              string
	RunID            string
	MemoryMiB        int
	CPUs             int
	ReadyTimeout     time.Duration
	ReadinessTimeout time.Duration
	CleanupTimeout   time.Duration
}

// VMSpec is retained as a descriptive alias for callers constructing a VM.
type VMSpec = Spec

func (s Spec) Validate() error {
	if !digestPattern.MatchString(s.RunnerImage) {
		return errors.New("runner image must be an immutable sha256 digest")
	}
	if !digestPattern.MatchString(s.GuestImage) {
		return errors.New("guest image must be an immutable sha256 digest")
	}
	for name, value := range map[string]string{"kernel": s.Kernel, "initrd": s.Initrd, "overlay": s.Overlay, "state directory": s.StateDir} {
		if name == "initrd" {
			continue
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("missing %s", name)
		}
	}
	if strings.TrimSpace(s.GuestRootFS) == "" {
		return errors.New("missing guest root filesystem")
	}
	if filepath.IsAbs(s.StateDir) == false || filepath.Clean(s.StateDir) == string(filepath.Separator) {
		return errors.New("state directory must be a safe absolute path")
	}
	if s.Channel == "" {
		return errors.New("missing control channel")
	}
	if s.MemoryMiB < 0 || s.CPUs < 0 {
		return errors.New("memory and CPUs cannot be negative")
	}
	if s.Architecture != "" && s.Architecture != "amd64" && s.Architecture != "arm64" {
		return fmt.Errorf("unsupported architecture %q", s.Architecture)
	}
	return nil
}

func (s Spec) normalized() Spec {
	if s.Kernel == "" {
		s.Kernel = s.GuestKernel
	}
	if s.Initrd == "" {
		s.Initrd = s.GuestRootFS
	}
	if s.Channel == "" {
		s.Channel = s.ControlChannel
	}
	if s.Channel == "" {
		s.Channel = DefaultControlChannel
	}
	if s.MemoryMiB == 0 {
		s.MemoryMiB = DefaultMemoryMiB
	}
	if s.CPUs == 0 {
		s.CPUs = DefaultCPUs
	}
	if s.ReadyTimeout <= 0 {
		s.ReadyTimeout = s.ReadinessTimeout
	}
	if s.ReadyTimeout <= 0 {
		s.ReadyTimeout = DefaultReadyTimeout
	}
	if s.CleanupTimeout <= 0 {
		s.CleanupTimeout = DefaultCleanupTimeout
	}
	if s.KVM == "" {
		s.KVM = "/dev/kvm"
	}
	return s
}

// QEMUArgs returns the fixed, minimal microVM argument vector.
func (s Spec) QEMUArgs() ([]string, error) {
	s = s.normalized()
	if err := s.Validate(); err != nil {
		return nil, err
	}
	args := []string{
		"-machine", "microvm,accel=kvm",
		"-cpu", "host",
		"-m", strconv.Itoa(s.MemoryMiB),
		"-smp", strconv.Itoa(s.CPUs),
		"-kernel", s.Kernel,
		"-append", "console=ttyS0 root=/dev/vda rw init=/sbin/donate-clanker-init panic=-1",
		"-drive", "file=" + s.GuestRootFS + ",if=none,format=raw,readonly=on,id=root",
		"-device", "virtio-blk-device,drive=root",
		"-drive", "file=" + s.Overlay + ",if=none,format=qcow2,id=overlay",
		"-device", "virtio-blk-device,drive=overlay",
		"-chardev", "socket,id=control,path=" + s.Channel,
		"-device", "virtio-serial-device",
		"-device", "virtserialport,chardev=control,name=" + s.Channel,
		"-netdev", "user,id=net",
		"-device", "virtio-net-device,netdev=net",
		"-nographic",
	}
	if s.Initrd != "" {
		args = append(args, "-initrd", s.Initrd)
	}
	return args, nil
}

func (s Spec) BuildQEMUArgs() ([]string, error) { return s.QEMUArgs() }

// RunnerArgs returns the rootless runner invocation. Only the per-run state
// directory is shared; no workspace, home, or container-engine socket is.
func (s Spec) RunnerArgs() ([]string, error) {
	s = s.normalized()
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return []string{
		"run", "--rm", "--name", "donate-clanker-vm-" + s.RunID,
		"--device", s.KVM,
		"--network", "none",
		"--mount", "type=bind,src=" + s.StateDir + ",dst=/run/donate-clanker,rw",
		s.RunnerImage,
		"qemu-system-" + runnerArch(s.Architecture),
	}, nil
}

func (s Spec) BuildRunnerCommand() ([]string, error) { return s.RunnerArgs() }

func runnerArch(arch string) string {
	if arch == "arm64" {
		return "aarch64"
	}
	return "x86_64"
}
