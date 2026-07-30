package pod

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/projectbluefin/donate-clanker/internal/config"
	"github.com/projectbluefin/donate-clanker/internal/engine"
)

const DefaultContainerPort = 8000

type Spec struct {
	NamePrefix    string
	Labels        map[string]string
	ModelHostPort int
	ContainerPort int
}

type ModelSpec struct {
	Image            string
	Env              map[string]string
	Mounts           []config.Mount
	Command          []string
	ReadinessTimeout time.Duration
}

type WorkerSpec struct {
	Image   string
	Env     map[string]string
	Mounts  []config.Mount
	Command []string
	WorkDir string
}

type Handle struct {
	engine        engine.Engine
	podName       string
	helperName    string
	workerName    string
	modelHostPort int
	modelPort     int
	closed        bool
}

func Create(ctx context.Context, eng engine.Engine, spec Spec) (*Handle, error) {
	hostPort := spec.ModelHostPort
	if hostPort == 0 {
		var err error
		hostPort, err = allocateHostPort()
		if err != nil {
			return nil, err
		}
	}

	containerPort := spec.ContainerPort
	if containerPort == 0 {
		containerPort = DefaultContainerPort
	}

	name := spec.NamePrefix
	if name == "" {
		name = fmt.Sprintf("donate-clanker-%d", time.Now().UnixNano())
	}

	if err := eng.PodCreate(ctx, engine.PodSpec{
		Name:          name,
		Labels:        spec.Labels,
		HostPort:      hostPort,
		ContainerPort: containerPort,
	}); err != nil {
		return nil, err
	}

	return &Handle{
		engine:        eng,
		podName:       name,
		helperName:    name + "-model",
		workerName:    name + "-worker",
		modelHostPort: hostPort,
		modelPort:     containerPort,
	}, nil
}

func (h *Handle) WorkerEndpointURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/v1", h.modelPort)
}

func allocateHostPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
