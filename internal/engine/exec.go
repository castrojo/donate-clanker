package engine

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
)

type ExecCommandRunner struct{}

func (ExecCommandRunner) Run(ctx context.Context, name string, args ...string) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}, err
}

func (ExecCommandRunner) Start(ctx context.Context, name string, args ...string) (Process, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	reader, writer := io.Pipe()
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Start(); err != nil {
		_ = writer.Close()
		_ = reader.Close()
		return nil, err
	}

	return &execProcess{
		cmd:    cmd,
		reader: reader,
		writer: writer,
	}, nil
}

type execProcess struct {
	cmd       *exec.Cmd
	reader    *io.PipeReader
	writer    *io.PipeWriter
	closeOnce sync.Once
}

func (p *execProcess) Wait() error {
	err := p.cmd.Wait()
	p.closeOutput()
	return err
}

func (p *execProcess) Signal(sig os.Signal) error {
	if p.cmd.Process == nil {
		return os.ErrProcessDone
	}
	return p.cmd.Process.Signal(sig)
}

func (p *execProcess) StdoutStderr() io.ReadCloser {
	return p.reader
}

func (p *execProcess) closeOutput() {
	p.closeOnce.Do(func() {
		_ = p.writer.Close()
	})
}

type completedProcess struct {
	output string
	err    error
}

func (p *completedProcess) Wait() error { return p.err }
func (p *completedProcess) Signal(os.Signal) error {
	return os.ErrProcessDone
}
func (p *completedProcess) StdoutStderr() io.ReadCloser {
	return io.NopCloser(bytes.NewBufferString(p.output))
}
