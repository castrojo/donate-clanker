package vm

import (
	"context"
	"os"
	"os/exec"
	"sync"
)

type Process interface {
	Wait() error
	Signal(os.Signal) error
	Kill() error
}

type Command interface {
	Start(context.Context, []string) (Process, error)
}

type CommandFunc func(context.Context, []string) (Process, error)

func (f CommandFunc) Start(ctx context.Context, args []string) (Process, error) {
	return f(ctx, args)
}

type ExecCommand struct{ Program string }

func (c ExecCommand) Start(ctx context.Context, args []string) (Process, error) {
	cmd := exec.CommandContext(ctx, c.Program, args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return execProcess{cmd}, nil
}

type execProcess struct{ *exec.Cmd }

func (p execProcess) Signal(signal os.Signal) error { return p.Process.Signal(signal) }
func (p execProcess) Kill() error                   { return p.Process.Kill() }

type VM struct {
	spec    Spec
	command Command
	process Process
	mu      sync.Mutex
	closed  bool
}

func New(spec Spec, command Command) (*VM, error) {
	spec = spec.normalized()
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if command == nil {
		return nil, os.ErrInvalid
	}
	return &VM{spec: spec, command: command}, nil
}

func (v *VM) Start(ctx context.Context) error {
	args, err := v.spec.QEMUArgs()
	if err != nil {
		return err
	}
	process, err := v.command.Start(ctx, args)
	if err != nil {
		return err
	}
	v.mu.Lock()
	v.process = process
	v.mu.Unlock()
	return nil
}

func (v *VM) Process() Process {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.process
}

func (v *VM) Close(ctx context.Context) error {
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return nil
	}
	v.closed = true
	process := v.process
	v.mu.Unlock()
	if process == nil {
		return nil
	}
	return Cleanup(ctx, process, v.spec.CleanupTimeout)
}
