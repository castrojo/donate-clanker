package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	WorkspaceMountPath = "/workspace"
	HiveMountPath      = "/config/hive"
	GitHubMountPath    = "/config/github"
	CacheMountPath     = "/cache/ramalama"
)

var (
	ErrMissingMountSource = errors.New("missing mount source")
	ErrUnsafeMount        = errors.New("unsafe mount path")
)

type Mount struct {
	HostPath       string
	ContainerPath  string
	ReadOnly       bool
	SELinuxRelabel string
}

func ResolveMounts(opts Options) ([]Mount, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	home = filepath.Clean(home)

	workspace, err := canonicalExistingPath(opts.Workspace)
	if err != nil {
		return nil, err
	}
	if err := ensureSafeMountPath(home, workspace, "workspace"); err != nil {
		return nil, err
	}

	cacheDir, err := ensurePrivateDir(opts.CacheDir)
	if err != nil {
		return nil, err
	}
	if err := ensureSafeMountPath(home, cacheDir, "cache"); err != nil {
		return nil, err
	}

	return []Mount{
		{HostPath: workspace, ContainerPath: WorkspaceMountPath, SELinuxRelabel: "z"},
		{HostPath: cacheDir, ContainerPath: CacheMountPath, SELinuxRelabel: "z"},
	}, nil
}

func canonicalExistingPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", ErrMissingMountSource
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%s: %w", abs, ErrMissingMountSource)
		}
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func ensurePrivateDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(abs, 0o700); err != nil {
		return "", err
	}
	return canonicalExistingPath(abs)
}

func ensureSafeMountPath(home, path, label string) error {
	if samePath(home, path) {
		return fmt.Errorf("%s: %w: %s cannot be the home directory", path, ErrUnsafeMount, label)
	}

	info, err := os.Stat(path)
	if err == nil && info.Mode()&os.ModeSocket != 0 {
		return fmt.Errorf("%s: %w: sockets cannot be mounted", path, ErrUnsafeMount)
	}
	if strings.HasSuffix(path, ".sock") {
		return fmt.Errorf("%s: %w: sockets cannot be mounted", path, ErrUnsafeMount)
	}

	return nil
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}
