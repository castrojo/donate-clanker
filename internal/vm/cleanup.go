package vm

import (
	"context"
	"errors"
	"os"
	"time"
)

func Cleanup(ctx context.Context, process Process, timeout time.Duration) error {
	if process == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = DefaultCleanupTimeout
	}
	_ = process.Signal(os.Interrupt)
	wait := make(chan error, 1)
	go func() { wait <- process.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-wait:
		return err
	case <-timer.C:
		_ = process.Kill()
		grace := time.NewTimer(100 * time.Millisecond)
		defer grace.Stop()
		select {
		case err := <-wait:
			return err
		case <-grace.C:
			return errors.New("process did not exit after kill")
		}
	case <-ctx.Done():
		_ = process.Kill()
		return errors.Join(ctx.Err(), errors.New("process did not exit after cancellation"))
	}
}
