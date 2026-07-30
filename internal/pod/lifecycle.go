package pod

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"time"

	"github.com/castrojo/donate-clanker/internal/config"
	"github.com/castrojo/donate-clanker/internal/engine"
)

func StartModel(ctx context.Context, handle *Handle, spec ModelSpec) error {
	if spec.Image == "" {
		return errors.New("missing helper image")
	}

	_, err := handle.engine.Run(ctx, engine.RunSpec{
		Name:          handle.helperName,
		Image:         spec.Image,
		Detach:        true,
		Env:           spec.Env,
		Mounts:        toEngineMounts(spec.Mounts),
		Command:       spec.Command,
		Labels:        map[string]string{"app.kubernetes.io/part-of": "donate-clanker"},
		PodName:       handle.podName,
		HostIP:        "127.0.0.1",
		HostPort:      handle.modelHostPort,
		ContainerPort: handle.modelPort,
	})
	if err != nil {
		return err
	}

	timeout := spec.ReadinessTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return waitForEndpoint(ctx, net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", handle.modelHostPort)), timeout)
}

func StartWorker(ctx context.Context, handle *Handle, spec WorkerSpec) (engine.Process, error) {
	if spec.Image == "" {
		return nil, errors.New("missing contributor image")
	}

	return handle.engine.Run(ctx, engine.RunSpec{
		Name:          handle.workerName,
		Image:         spec.Image,
		PodName:       handle.podName,
		JoinContainer: handle.helperName,
		Env:           spec.Env,
		Mounts:        toEngineMounts(spec.Mounts),
		Command:       spec.Command,
		WorkDir:       firstNonEmpty(spec.WorkDir, config.WorkspaceMountPath),
		Labels:        map[string]string{"app.kubernetes.io/part-of": "donate-clanker"},
	})
}

func (h *Handle) Close(ctx context.Context) error {
	if h == nil || h.closed {
		return nil
	}
	h.closed = true

	var errs []error
	for _, removal := range []func(context.Context) error{
		func(ctx context.Context) error { return h.engine.Stop(ctx, h.workerName) },
		func(ctx context.Context) error { return h.engine.Remove(ctx, h.workerName) },
		func(ctx context.Context) error { return h.engine.Stop(ctx, h.helperName) },
		func(ctx context.Context) error { return h.engine.Remove(ctx, h.helperName) },
		func(ctx context.Context) error { return h.engine.RemovePod(ctx, h.podName) },
	} {
		if err := removal(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func toEngineMounts(mounts []config.Mount) []engine.Mount {
	converted := make([]engine.Mount, 0, len(mounts))
	for _, mount := range mounts {
		converted = append(converted, engine.Mount{
			Source:   filepath.Clean(mount.HostPath),
			Target:   mount.ContainerPath,
			ReadOnly: mount.ReadOnly,
		})
	}
	return converted
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
