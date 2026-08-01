package vm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const BootstrapVersion = 2

// Bootstrap is the single host-to-guest control message. Version 2 adds the
// Goose credential passthrough so a contributor's own model access reaches the
// agent without mounting host configuration into the VM.
type Bootstrap struct {
	Version           int    `json:"version"`
	HiveEndpoint      string `json:"hive_endpoint"`
	RegistrationToken string `json:"registration_token"`
	Backend           string `json:"backend"`
	RunID             string `json:"run_id"`
	GooseProvider     string `json:"goose_provider,omitempty"`
	GooseModel        string `json:"goose_model,omitempty"`
	ProviderSecret    string `json:"provider_secret,omitempty"`
}

type Status struct {
	Version int    `json:"version"`
	Type    string `json:"type"`
	Detail  string `json:"detail,omitempty"`
}

func (b Bootstrap) Validate() error {
	if b.Version != BootstrapVersion {
		return fmt.Errorf("unsupported bootstrap version %d (this launcher speaks version %d)", b.Version, BootstrapVersion)
	}
	if b.HiveEndpoint == "" || b.RegistrationToken == "" || b.Backend == "" || b.RunID == "" {
		return errors.New("bootstrap is missing a required field")
	}
	if !strings.HasPrefix(b.HiveEndpoint, "wss://") && !strings.HasPrefix(b.HiveEndpoint, "https://") {
		return errors.New("bootstrap endpoint must use HTTPS or WSS")
	}
	if b.Backend != "goose" {
		return fmt.Errorf("unsupported backend %q: donate-clanker runs Goose only", b.Backend)
	}
	return nil
}

func ValidateBootstrap(b Bootstrap) error { return b.Validate() }

func SendBootstrap(w io.Writer, b Bootstrap) error {
	if err := b.Validate(); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	return enc.Encode(b)
}

func DecodeBootstrap(r io.Reader) (Bootstrap, error) {
	dec := json.NewDecoder(io.LimitReader(r, 64<<10))
	dec.DisallowUnknownFields()
	var b Bootstrap
	if err := dec.Decode(&b); err != nil {
		return Bootstrap{}, err
	}
	if err := b.Validate(); err != nil {
		return Bootstrap{}, err
	}
	return b, nil
}

var readinessStages = []string{"boot", "control_ack", "network", "hive", "worker_ready"}

func WaitReady(ctx context.Context, r io.Reader, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = DefaultReadyTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	lines := make(chan []byte)
	errs := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			select {
			case lines <- line:
			case <-ctx.Done():
				return
			}
		}
		errs <- scanner.Err()
	}()
	stage := 0
	for stage < len(readinessStages) {
		select {
		case <-ctx.Done():
			return fmt.Errorf("readiness timeout waiting for %s: %w", readinessStages[stage], ctx.Err())
		case line := <-lines:
			var status Status
			if err := json.Unmarshal(line, &status); err != nil {
				return fmt.Errorf("invalid readiness status: %w", err)
			}
			if status.Version != BootstrapVersion || status.Type != readinessStages[stage] {
				return fmt.Errorf("unexpected readiness status %q, want %q", status.Type, readinessStages[stage])
			}
			stage++
		case err := <-errs:
			if err == nil {
				return io.ErrUnexpectedEOF
			}
			return err
		}
	}
	return nil
}

func WaitForReadiness(ctx context.Context, r io.Reader, timeout time.Duration) error {
	return WaitReady(ctx, r, timeout)
}
