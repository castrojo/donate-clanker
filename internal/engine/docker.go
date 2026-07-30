package engine

import (
	"context"
	"fmt"
	"strings"
)

type dockerEngine struct {
	runner CommandRunner
}

func NewDocker(runner CommandRunner) Engine {
	return dockerEngine{runner: runner}
}

func (e dockerEngine) Name() string { return "docker" }

func (e dockerEngine) Version(ctx context.Context) error {
	_, err := e.runner.Run(ctx, "docker", "version")
	return err
}

func (e dockerEngine) PodCreate(context.Context, PodSpec) error {
	return nil
}

func (e dockerEngine) Run(ctx context.Context, spec RunSpec) (Process, error) {
	args := []string{"run"}
	if spec.Detach {
		args = append(args, "-d", "--rm")
	} else {
		args = append(args, "--rm")
	}
	args = append(args, "--name", spec.Name)
	if spec.JoinContainer != "" {
		args = append(args, "--network", "container:"+spec.JoinContainer)
	}
	if spec.WorkDir != "" {
		args = append(args, "--workdir", spec.WorkDir)
	}
	for _, key := range sortedMapKeys(spec.Labels) {
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, spec.Labels[key]))
	}
	for _, mount := range spec.Mounts {
		args = append(args, "--mount", dockerMountArg(mount))
	}
	for _, key := range sortedMapKeys(spec.Env) {
		args = append(args, "--env", fmt.Sprintf("%s=%s", key, spec.Env[key]))
	}
	if spec.HostPort > 0 && spec.ContainerPort > 0 && spec.JoinContainer == "" {
		args = append(args, "-p", fmt.Sprintf("%s:%d:%d", defaultHostIP(spec.HostIP), spec.HostPort, spec.ContainerPort))
	}
	args = append(args, spec.Image)
	args = append(args, spec.Command...)

	if spec.Detach {
		result, err := e.runner.Run(ctx, "docker", args...)
		return &completedProcess{output: strings.TrimSpace(result.Stdout), err: err}, err
	}
	return e.runner.Start(ctx, "docker", args...)
}

func (e dockerEngine) Stop(ctx context.Context, name string) error {
	_, err := e.runner.Run(ctx, "docker", "stop", name)
	return err
}

func (e dockerEngine) Remove(ctx context.Context, name string) error {
	_, err := e.runner.Run(ctx, "docker", "rm", "-f", name)
	return err
}

func (e dockerEngine) RemovePod(context.Context, string) error {
	return nil
}

func dockerMountArg(mount Mount) string {
	options := []string{fmt.Sprintf("type=bind,src=%s,dst=%s", mount.Source, mount.Target)}
	if mount.ReadOnly {
		options = append(options, "readonly")
	}
	return strings.Join(options, ",")
}
