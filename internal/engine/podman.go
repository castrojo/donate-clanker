package engine

import (
	"context"
	"fmt"
	"strings"
)

type podmanEngine struct {
	runner CommandRunner
}

func NewPodman(runner CommandRunner) Engine {
	return podmanEngine{runner: runner}
}

func (e podmanEngine) Name() string { return "podman" }

func (e podmanEngine) Version(ctx context.Context) error {
	_, err := e.runner.Run(ctx, "podman", "version")
	return err
}

func (e podmanEngine) ProbeRuntime(ctx context.Context, runtime string) error {
	result, err := e.runner.Run(ctx, "podman", "--runtime", runtime, "info", "--format", "{{.Host.OCIRuntime.Name}}")
	if err != nil {
		return err
	}
	if strings.TrimSpace(result.Stdout) != runtime {
		return fmt.Errorf("runtime %q was not selected", runtime)
	}
	return nil
}

func (e podmanEngine) PodCreate(ctx context.Context, spec PodSpec) error {
	var args []string
	if spec.Runtime != "" {
		args = append(args, "--runtime", spec.Runtime)
	}
	args = append(args, "pod", "create", "--replace", "--name", spec.Name)
	for _, key := range sortedMapKeys(spec.Labels) {
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, spec.Labels[key]))
	}
	if spec.HostPort > 0 && spec.ContainerPort > 0 {
		args = append(args, "-p", fmt.Sprintf("127.0.0.1:%d:%d", spec.HostPort, spec.ContainerPort))
	}
	_, err := e.runner.Run(ctx, "podman", args...)
	return err
}

func (e podmanEngine) Run(ctx context.Context, spec RunSpec) (Process, error) {
	args := []string{"run"}
	if spec.Runtime != "" {
		args = append(args, "--runtime", spec.Runtime)
	}
	if spec.Detach {
		args = append(args, "-d")
	} else {
		args = append(args, "--rm")
	}
	args = append(args, "--replace", "--name", spec.Name)
	if spec.PodName != "" {
		args = append(args, "--pod", spec.PodName)
	}
	if spec.WorkDir != "" {
		args = append(args, "--workdir", spec.WorkDir)
	}
	for _, key := range sortedMapKeys(spec.Labels) {
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, spec.Labels[key]))
	}
	for _, mount := range spec.Mounts {
		args = append(args, "--mount", podmanMountArg(mount))
	}
	for _, key := range sortedMapKeys(spec.Env) {
		args = append(args, "--env", fmt.Sprintf("%s=%s", key, spec.Env[key]))
	}
	if spec.PodName == "" && spec.HostPort > 0 && spec.ContainerPort > 0 {
		args = append(args, "-p", fmt.Sprintf("%s:%d:%d", defaultHostIP(spec.HostIP), spec.HostPort, spec.ContainerPort))
	}
	args = append(args, spec.Image)
	args = append(args, spec.Command...)

	if spec.Detach {
		result, err := e.runner.Run(ctx, "podman", args...)
		return &completedProcess{output: strings.TrimSpace(result.Stdout), err: err}, err
	}

	return e.runner.Start(ctx, "podman", args...)
}

func (e podmanEngine) Stop(ctx context.Context, name string) error {
	_, err := e.runner.Run(ctx, "podman", "stop", name)
	return err
}

func (e podmanEngine) Remove(ctx context.Context, name string) error {
	_, err := e.runner.Run(ctx, "podman", "rm", "-f", name)
	return err
}

func (e podmanEngine) RemovePod(ctx context.Context, name string) error {
	_, err := e.runner.Run(ctx, "podman", "pod", "rm", "-f", name)
	return err
}

func podmanMountArg(mount Mount) string {
	options := []string{fmt.Sprintf("type=bind,src=%s,dst=%s", mount.Source, mount.Target)}
	if mount.ReadOnly {
		options = append(options, "ro")
	}
	return strings.Join(options, ",")
}

func defaultHostIP(hostIP string) string {
	if strings.TrimSpace(hostIP) == "" {
		return "127.0.0.1"
	}
	return hostIP
}
