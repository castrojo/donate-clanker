package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrRuntimeUnavailable = errors.New("requested container runtime unavailable")

type runtimeProber interface {
	ProbeRuntime(context.Context, string) error
}

func ResolveRuntime(ctx context.Context, eng Engine, requested string, strict bool) (string, string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "auto"
	}
	target := requested
	if target == "auto" {
		target = "runsc"
	}

	prober, ok := eng.(runtimeProber)
	if eng.Name() != "podman" || !ok {
		return runtimeUnavailable(target, eng.Name(), strict)
	}
	if err := prober.ProbeRuntime(ctx, target); err != nil {
		return runtimeUnavailable(target, eng.Name(), strict)
	}
	return target, "", nil
}

func runtimeUnavailable(requested, engineName string, strict bool) (string, string, error) {
	message := fmt.Sprintf("warning: container runtime %q is unavailable with %s; using the default runtime", requested, engineName)
	if strict {
		return "", "", fmt.Errorf("%w: %s", ErrRuntimeUnavailable, requested)
	}
	return "", message, nil
}
