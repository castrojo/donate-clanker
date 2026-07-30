package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

type Preference string

const (
	PreferenceAuto   Preference = "auto"
	PreferencePodman Preference = "podman"
	PreferenceDocker Preference = "docker"
)

var (
	ErrNoEngine          = errors.New("no supported container engine found")
	ErrUnavailableEngine = errors.New("requested engine unavailable")
)

type CommandResult struct {
	Stdout string
	Stderr string
}

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (CommandResult, error)
	Start(ctx context.Context, name string, args ...string) (Process, error)
}

type Process interface {
	Wait() error
	Signal(os.Signal) error
	StdoutStderr() io.ReadCloser
}

type Engine interface {
	Name() string
	Version(context.Context) error
	PodCreate(context.Context, PodSpec) error
	Run(context.Context, RunSpec) (Process, error)
	Stop(context.Context, string) error
	Remove(context.Context, string) error
	RemovePod(context.Context, string) error
}

type PodSpec struct {
	Name          string
	Labels        map[string]string
	HostPort      int
	ContainerPort int
}

type RunSpec struct {
	Name          string
	Image         string
	PodName       string
	JoinContainer string
	Detach        bool
	WorkDir       string
	Command       []string
	Env           map[string]string
	Mounts        []Mount
	Labels        map[string]string
	HostIP        string
	HostPort      int
	ContainerPort int
}

type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

func Detect(ctx context.Context, preference Preference) (Engine, error) {
	return DetectWithRunner(ctx, preference, ExecCommandRunner{})
}

func DetectWithRunner(ctx context.Context, preference Preference, runner CommandRunner) (Engine, error) {
	candidates := []Engine{
		NewPodman(runner),
		NewDocker(runner),
	}

	if preference != "" && preference != PreferenceAuto {
		engine := engineByName(preference, runner)
		if engine == nil {
			return nil, fmt.Errorf("%w: %s", ErrUnavailableEngine, preference)
		}
		if err := engine.Version(ctx); err != nil {
			return nil, fmt.Errorf("%s: %w", engine.Name(), ErrUnavailableEngine)
		}
		return engine, nil
	}

	for _, candidate := range candidates {
		if err := candidate.Version(ctx); err == nil {
			return candidate, nil
		}
	}

	return nil, ErrNoEngine
}

func engineByName(preference Preference, runner CommandRunner) Engine {
	switch strings.ToLower(string(preference)) {
	case string(PreferencePodman):
		return NewPodman(runner)
	case string(PreferenceDocker):
		return NewDocker(runner)
	default:
		return nil
	}
}

func sortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
