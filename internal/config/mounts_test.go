package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolveMountsRejectsHomeWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	opts := minimalMountOptions(t, home)
	opts.Workspace = home

	_, err := ResolveMounts(opts)
	if !errors.Is(err, ErrUnsafeMount) {
		t.Fatalf("ResolveMounts() error = %v, want ErrUnsafeMount", err)
	}
}

func TestResolveMountsCanonicalizesWorkspaceSymlink(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)

	workspace := filepath.Join(root, "workspace-real")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	link := filepath.Join(root, "workspace-link")
	if err := os.Symlink(workspace, link); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	opts := minimalMountOptions(t, root)
	opts.Workspace = link

	mounts, err := ResolveMounts(opts)
	if err != nil {
		t.Fatalf("ResolveMounts() error = %v", err)
	}
	if got := mounts[0].HostPath; got != filepath.Clean(workspace) {
		t.Fatalf("ResolveMounts() workspace = %q, want %q", got, workspace)
	}
}

func TestResolveMountsReturnsOnlyWorkspaceAndCache(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	opts := minimalMountOptions(t, root)
	opts.HiveConfigDir = filepath.Join(root, "missing-hive")

	mounts, err := ResolveMounts(opts)
	if err != nil {
		t.Fatalf("ResolveMounts() error = %v", err)
	}

	got := []string{mounts[0].ContainerPath, mounts[1].ContainerPath}
	want := []string{WorkspaceMountPath, CacheMountPath}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolveMounts() container paths = %#v, want %#v", got, want)
	}
}

func TestResolveMountsCreatesPrivateCacheDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	opts := minimalMountOptions(t, root)
	opts.CacheDir = filepath.Join(root, "cache-dir")

	mounts, err := ResolveMounts(opts)
	if err != nil {
		t.Fatalf("ResolveMounts() error = %v", err)
	}

	info, err := os.Stat(opts.CacheDir)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("cache mode = %o, want 700", got)
	}
	if mounts[1].HostPath != filepath.Clean(opts.CacheDir) {
		t.Fatalf("ResolveMounts() cache mount = %q, want %q", mounts[1].HostPath, opts.CacheDir)
	}
}

func minimalMountOptions(t *testing.T, root string) Options {
	t.Helper()

	workspace := filepath.Join(root, "workspace")
	cacheDir := filepath.Join(root, ".cache", "ramalama")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", workspace, err)
	}
	return Options{
		Workspace:     workspace,
		HiveConfigDir: filepath.Join(root, ".config", "hive"),
		CacheDir:      cacheDir,
	}
}
