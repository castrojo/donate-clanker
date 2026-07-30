package pod

import (
	"context"
	"fmt"
	"net"
	"time"
)

func waitForEndpoint(ctx context.Context, address string, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("model endpoint %s did not become ready within %s", address, timeout)
		case <-ticker.C:
		}
	}
}
