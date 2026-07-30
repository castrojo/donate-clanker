package contract

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAcceptsBundledShape(t *testing.T) {
	manifest, err := LoadBundled()
	if err != nil {
		t.Fatalf("LoadBundled() error = %v", err)
	}

	if manifest.Version != supportedVersion {
		t.Fatalf("manifest.Version = %d, want %d", manifest.Version, supportedVersion)
	}
	if got, want := len(manifest.RequiredDocuments), 3; got != want {
		t.Fatalf("len(manifest.RequiredDocuments) = %d, want %d", got, want)
	}
	if got, want := manifest.RequiredDocuments[0].Path, "AGENTS.md"; got != want {
		t.Fatalf("first document path = %q, want %q", got, want)
	}
	if got, want := manifest.RequiredDocuments[0].Heading, "Required repository document: AGENTS.md"; got != want {
		t.Fatalf("first document heading = %q, want %q", got, want)
	}
	if got, want := manifest.Rules, []string{
		"Read AGENTS.md first, then docs/SKILL.md.",
		"If you discover a durable workaround, pattern, or convention, update the nearest docs/skills/*.md file in the same change.",
		"Use the repository validation command family: go test ./..., git diff --check, gofmt -l ., just --justfile just/61-donate-clanker.just --list.",
	}; !slicesEqual(got, want) {
		t.Fatalf("manifest.Rules = %#v, want %#v", got, want)
	}
	if got, want := manifest.ValidationCommands, []string{
		"go test ./...",
		"git diff --check",
		"gofmt -l .",
		"just --justfile just/61-donate-clanker.just --list",
	}; !slicesEqual(got, want) {
		t.Fatalf("manifest.ValidationCommands = %#v, want %#v", got, want)
	}
}

func TestLoadRejectsMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{
			name: "missing-version",
			json: `{"required_documents":[{"path":"AGENTS.md","heading":"AGENTS.md"}],"rules":["rule"],"validation_commands":["go test ./..."]}`,
		},
		{
			name: "missing-rules",
			json: `{"version":1,"required_documents":[{"path":"AGENTS.md","heading":"AGENTS.md"}],"validation_commands":["go test ./..."]}`,
		},
		{
			name: "missing-validation-commands",
			json: `{"version":1,"required_documents":[{"path":"AGENTS.md","heading":"AGENTS.md"}],"rules":["rule"]}`,
		},
		{
			name: "missing-document-heading",
			json: `{"version":1,"required_documents":[{"path":"AGENTS.md"}],"rules":["rule"],"validation_commands":["go test ./..."]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load([]byte(tt.json))
			if err == nil {
				t.Fatal("Load() error = nil, want error")
			}
		})
	}
}

func TestLoadRejectsAbsoluteAndTraversalPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
		want error
	}{
		{name: "absolute", path: "/etc/passwd", want: ErrAbsolutePath},
		{name: "traversal", path: "../AGENTS.md", want: ErrTraversalPath},
		{name: "cleaned-traversal", path: "docs/../../AGENTS.md", want: ErrTraversalPath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load([]byte(`{"version":1,"required_documents":[{"path":"` + tt.path + `","heading":"AGENTS.md"}],"rules":["rule"],"validation_commands":["go test ./..."]}`))
			if err == nil {
				t.Fatal("Load() error = nil, want error")
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("Load() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestLoadDocumentsRejectsMissingAndEmptyFiles(t *testing.T) {
	manifest := Manifest{
		Version: supportedVersion,
		RequiredDocuments: []RequiredDocument{
			{Path: "missing.md", Heading: "Missing"},
			{Path: "empty.md", Heading: "Empty"},
		},
		Rules:              []string{"rule"},
		ValidationCommands: []string{"go test ./..."},
	}
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "empty.md"), []byte(" \n\t"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := manifest.LoadDocuments(workspace)
	if err == nil {
		t.Fatal("LoadDocuments() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "missing.md") {
		t.Fatalf("LoadDocuments() error = %v, want relative path", err)
	}
	if strings.Contains(err.Error(), " \n\t") {
		t.Fatalf("LoadDocuments() error leaked file contents: %v", err)
	}

	manifest.RequiredDocuments[0].Path = "empty.md"
	manifest.RequiredDocuments[1].Path = "missing.md"
	_, err = manifest.LoadDocuments(workspace)
	if err == nil {
		t.Fatal("LoadDocuments() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "empty.md") {
		t.Fatalf("LoadDocuments() error = %v, want relative path", err)
	}
}

func TestLoadDocumentsRejectsInvalidManifestPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
		want error
	}{
		{name: "absolute", path: "/etc/passwd", want: ErrAbsolutePath},
		{name: "traversal", path: "../AGENTS.md", want: ErrTraversalPath},
		{name: "cleaned-traversal", path: "docs/../../AGENTS.md", want: ErrTraversalPath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := Manifest{
				Version: supportedVersion,
				RequiredDocuments: []RequiredDocument{
					{Path: tt.path, Heading: "Invalid"},
				},
				Rules:              []string{"rule"},
				ValidationCommands: []string{"go test ./..."},
			}

			_, err := manifest.LoadDocuments(t.TempDir())
			if err == nil {
				t.Fatal("LoadDocuments() error = nil, want error")
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("LoadDocuments() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestLoadDocumentsRejectsSymlinkedRequiredDocumentOutsideWorkspace(t *testing.T) {
	manifest := Manifest{
		Version: supportedVersion,
		RequiredDocuments: []RequiredDocument{
			{Path: "AGENTS.md", Heading: "Agents"},
		},
		Rules:              []string{"rule"},
		ValidationCommands: []string{"go test ./..."},
	}
	workspace := t.TempDir()
	outside := t.TempDir()
	outsidePath := filepath.Join(outside, "AGENTS.md")
	mustWrite(t, outsidePath, "outside content")
	if err := os.Symlink(outsidePath, filepath.Join(workspace, "AGENTS.md")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	_, err := manifest.LoadDocuments(workspace)
	if err == nil {
		t.Fatal("LoadDocuments() error = nil, want error")
	}
	if !errors.Is(err, ErrTraversalPath) {
		t.Fatalf("LoadDocuments() error = %v, want %v", err, ErrTraversalPath)
	}
	if !strings.Contains(err.Error(), "AGENTS.md") {
		t.Fatalf("LoadDocuments() error = %v, want relative path", err)
	}
	if strings.Contains(err.Error(), workspace) || strings.Contains(err.Error(), outside) {
		t.Fatalf("LoadDocuments() error leaked absolute path: %v", err)
	}
}

func TestLoadDocumentsRejectsSymlinkedParentOutsideWorkspace(t *testing.T) {
	manifest := Manifest{
		Version: supportedVersion,
		RequiredDocuments: []RequiredDocument{
			{Path: "docs/SKILL.md", Heading: "Skill"},
		},
		Rules:              []string{"rule"},
		ValidationCommands: []string{"go test ./..."},
	}
	workspace := t.TempDir()
	outside := t.TempDir()
	safePath := filepath.Join(workspace, "safe", "SKILL.md")
	mustWrite(t, safePath, "workspace content")
	if err := os.Symlink(safePath, filepath.Join(outside, "SKILL.md")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "docs")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	_, err := manifest.LoadDocuments(workspace)
	if err == nil {
		t.Fatal("LoadDocuments() error = nil, want error")
	}
	if !errors.Is(err, ErrTraversalPath) {
		t.Fatalf("LoadDocuments() error = %v, want %v", err, ErrTraversalPath)
	}
	if !strings.Contains(err.Error(), "docs/SKILL.md") {
		t.Fatalf("LoadDocuments() error = %v, want relative path", err)
	}
	if strings.Contains(err.Error(), workspace) || strings.Contains(err.Error(), outside) {
		t.Fatalf("LoadDocuments() error leaked absolute path: %v", err)
	}
}

func TestLoadDocumentsRejectsNonRegularFile(t *testing.T) {
	manifest := Manifest{
		Version: supportedVersion,
		RequiredDocuments: []RequiredDocument{
			{Path: "AGENTS.md", Heading: "Agents"},
		},
		Rules:              []string{"rule"},
		ValidationCommands: []string{"go test ./..."},
	}
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "AGENTS.md"), 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	_, err := manifest.LoadDocuments(workspace)
	if err == nil {
		t.Fatal("LoadDocuments() error = nil, want error")
	}
	if !errors.Is(err, ErrNotRegularFile) {
		t.Fatalf("LoadDocuments() error = %v, want %v", err, ErrNotRegularFile)
	}
	if !strings.Contains(err.Error(), "AGENTS.md") {
		t.Fatalf("LoadDocuments() error = %v, want relative path", err)
	}
	if strings.Contains(err.Error(), workspace) {
		t.Fatalf("LoadDocuments() error leaked workspace path: %v", err)
	}
}

func TestLoadDocumentsSanitizesUnreadableFileErrors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission-denied read checks are unreliable when running as root")
	}

	manifest := Manifest{
		Version: supportedVersion,
		RequiredDocuments: []RequiredDocument{
			{Path: "docs/locked.md", Heading: "Locked"},
		},
		Rules:              []string{"rule"},
		ValidationCommands: []string{"go test ./..."},
	}
	workspace := t.TempDir()
	path := filepath.Join(workspace, "docs", "locked.md")
	mustWrite(t, path, "locked content")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(path, 0o644)
	})

	_, err := manifest.LoadDocuments(workspace)
	if err == nil {
		t.Fatal("LoadDocuments() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "docs/locked.md") {
		t.Fatalf("LoadDocuments() error = %v, want relative path", err)
	}
	if strings.Contains(err.Error(), workspace) {
		t.Fatalf("LoadDocuments() error leaked workspace path: %v", err)
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("LoadDocuments() error = %v, want permission denied", err)
	}
}

func TestLoadDocumentsPreservesManifestOrder(t *testing.T) {
	manifest := Manifest{
		Version: supportedVersion,
		RequiredDocuments: []RequiredDocument{
			{Path: "docs/second.md", Heading: "Second"},
			{Path: "docs/first.md", Heading: "First"},
		},
		Rules:              []string{"rule"},
		ValidationCommands: []string{"go test ./..."},
	}
	workspace := t.TempDir()
	mustWrite(t, filepath.Join(workspace, "docs", "first.md"), "first content")
	mustWrite(t, filepath.Join(workspace, "docs", "second.md"), "second content")

	documents, err := manifest.LoadDocuments(workspace)
	if err != nil {
		t.Fatalf("LoadDocuments() error = %v", err)
	}
	if got, want := len(documents), 2; got != want {
		t.Fatalf("len(documents) = %d, want %d", got, want)
	}
	if got, want := documents[0].Path, "docs/second.md"; got != want {
		t.Fatalf("documents[0].Path = %q, want %q", got, want)
	}
	if got, want := documents[1].Path, "docs/first.md"; got != want {
		t.Fatalf("documents[1].Path = %q, want %q", got, want)
	}
}

func TestPromptSectionIncludesRulesAndDocumentHeadings(t *testing.T) {
	manifest := Manifest{
		Version: supportedVersion,
		RequiredDocuments: []RequiredDocument{
			{Path: "AGENTS.md", Heading: "Required repository document: AGENTS.md"},
			{Path: "docs/SKILL.md", Heading: "Required repository document: docs/SKILL.md"},
		},
		Rules: []string{
			"Read AGENTS.md first, then docs/SKILL.md.",
			"Update docs/skills/*.md files when you learn something durable.",
		},
		ValidationCommands: []string{"go test ./...", "git diff --check"},
	}
	section := manifest.PromptSection([]Document{
		{Path: "AGENTS.md", Heading: "Required repository document: AGENTS.md", Content: "agents content"},
		{Path: "docs/SKILL.md", Heading: "Required repository document: docs/SKILL.md", Content: "skill content"},
	})

	for _, want := range []string{
		"## Agent contract",
		"Read AGENTS.md first, then docs/SKILL.md.",
		"git diff --check",
		"Required repository document: AGENTS.md",
		"agents content",
		"Required repository document: docs/SKILL.md",
		"skill content",
	} {
		if !strings.Contains(section, want) {
			t.Fatalf("PromptSection() missing %q in %q", want, section)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func slicesEqual[T comparable](got, want []T) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
