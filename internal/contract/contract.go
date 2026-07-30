package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	imageconfig "github.com/projectbluefin/donate-clanker/image/config"
)

const supportedVersion = 1

var (
	ErrMissingFile        = errors.New("missing file")
	ErrEmptyFile          = errors.New("empty file")
	ErrUnsupportedVersion = errors.New("unsupported version")
	ErrMissingField       = errors.New("missing field")
	ErrDuplicatePath      = errors.New("duplicate document path")
	ErrAbsolutePath       = errors.New("absolute path")
	ErrTraversalPath      = errors.New("path escapes workspace")
	ErrNotRegularFile     = errors.New("not a regular file")
)

type Manifest struct {
	Version            int
	RequiredDocuments  []RequiredDocument
	Rules              []string
	ValidationCommands []string
}

type RequiredDocument struct {
	Path    string
	Heading string
}

type Document struct {
	Path    string
	Heading string
	Content string
}

type rawManifest struct {
	Version            int           `json:"version"`
	RequiredDocuments  []rawDocument `json:"required_documents"`
	Rules              []string      `json:"rules"`
	ValidationCommands []string      `json:"validation_commands"`
}

type rawDocument struct {
	Path    string `json:"path"`
	Heading string `json:"heading"`
}

func Load(data []byte) (Manifest, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return Manifest{}, fmt.Errorf("manifest: %w", ErrEmptyFile)
	}

	var raw rawManifest
	if err := json.Unmarshal(data, &raw); err != nil {
		return Manifest{}, err
	}
	return validate(raw)
}

func LoadBundled() (Manifest, error) {
	return Load(imageconfig.BundledAgentContractJSON())
}

func (m Manifest) LoadDocuments(workspace string) ([]Document, error) {
	validated, err := m.validated()
	if err != nil {
		return nil, err
	}
	cleanWorkspace := filepath.Clean(workspace)
	documents := make([]Document, len(validated.RequiredDocuments))
	for i, required := range validated.RequiredDocuments {
		path, err := resolveDocumentPath(cleanWorkspace, required.Path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("%s: %w", required.Path, ErrMissingFile)
			}
			if errors.Is(err, ErrTraversalPath) || errors.Is(err, ErrNotRegularFile) {
				return nil, fmt.Errorf("%s: %w", required.Path, err)
			}
			return nil, sanitizeDocumentReadError(required.Path, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("%s: %w", required.Path, ErrMissingFile)
			}
			return nil, sanitizeDocumentReadError(required.Path, err)
		}
		if len(bytes.TrimSpace(data)) == 0 {
			return nil, fmt.Errorf("%s: %w", required.Path, ErrEmptyFile)
		}
		documents[i] = Document{
			Path:    required.Path,
			Heading: required.Heading,
			Content: string(data),
		}
	}
	return documents, nil
}

func (m Manifest) PromptSection(documents []Document) string {
	var b strings.Builder
	b.WriteString("## Agent contract\n")
	b.WriteString("\n### Rules\n")
	for _, rule := range m.Rules {
		b.WriteString("- ")
		b.WriteString(rule)
		b.WriteByte('\n')
	}
	if len(m.ValidationCommands) > 0 {
		b.WriteString("\n### Validation commands\n")
		for _, command := range m.ValidationCommands {
			b.WriteString("- ")
			b.WriteString(command)
			b.WriteByte('\n')
		}
	}
	for _, document := range documents {
		b.WriteString("\n## ")
		b.WriteString(document.Heading)
		b.WriteByte('\n')
		b.WriteString(document.Content)
		if !strings.HasSuffix(document.Content, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func validate(raw rawManifest) (Manifest, error) {
	requiredDocuments := make([]RequiredDocument, len(raw.RequiredDocuments))
	for i, doc := range raw.RequiredDocuments {
		requiredDocuments[i] = RequiredDocument{
			Path:    doc.Path,
			Heading: doc.Heading,
		}
	}
	return Manifest{
		Version:            raw.Version,
		RequiredDocuments:  requiredDocuments,
		Rules:              append([]string(nil), raw.Rules...),
		ValidationCommands: append([]string(nil), raw.ValidationCommands...),
	}.validated()
}

func validateDocumentPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", ErrMissingField
	}
	if filepath.IsAbs(path) {
		return "", ErrAbsolutePath
	}
	clean := filepath.Clean(path)
	if clean == "." {
		return "", ErrMissingField
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrTraversalPath
	}
	return clean, nil
}

func resolveDocumentPath(workspace, relativePath string) (string, error) {
	workspacePath, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	resolvedWorkspace, err := filepath.EvalSymlinks(workspacePath)
	if err != nil {
		return "", err
	}

	path := workspacePath
	resolvedPath := resolvedWorkspace
	for _, component := range strings.Split(relativePath, string(filepath.Separator)) {
		path = filepath.Join(path, component)
		resolvedPath, err = filepath.EvalSymlinks(path)
		if err != nil {
			return "", err
		}
		if !pathWithin(resolvedWorkspace, resolvedPath) {
			return "", ErrTraversalPath
		}
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", ErrNotRegularFile
	}
	return resolvedPath, nil
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (m Manifest) validated() (Manifest, error) {
	if m.Version != supportedVersion {
		return Manifest{}, fmt.Errorf("version %d: %w", m.Version, ErrUnsupportedVersion)
	}
	if len(m.RequiredDocuments) == 0 {
		return Manifest{}, fmt.Errorf("required_documents: %w", ErrMissingField)
	}
	if len(m.Rules) == 0 {
		return Manifest{}, fmt.Errorf("rules: %w", ErrMissingField)
	}
	if len(m.ValidationCommands) == 0 {
		return Manifest{}, fmt.Errorf("validation_commands: %w", ErrMissingField)
	}

	requiredDocuments := make([]RequiredDocument, len(m.RequiredDocuments))
	seen := make(map[string]struct{}, len(m.RequiredDocuments))
	for i, doc := range m.RequiredDocuments {
		path, err := validateDocumentPath(doc.Path)
		if err != nil {
			return Manifest{}, fmt.Errorf("required_documents[%d].path (%q): %w", i, doc.Path, err)
		}
		if strings.TrimSpace(doc.Heading) == "" {
			return Manifest{}, fmt.Errorf("required_documents[%d].heading: %w", i, ErrMissingField)
		}
		if _, ok := seen[path]; ok {
			return Manifest{}, fmt.Errorf("%s: %w", path, ErrDuplicatePath)
		}
		seen[path] = struct{}{}
		requiredDocuments[i] = RequiredDocument{
			Path:    path,
			Heading: doc.Heading,
		}
	}
	for i, rule := range m.Rules {
		if strings.TrimSpace(rule) == "" {
			return Manifest{}, fmt.Errorf("rules[%d]: %w", i, ErrMissingField)
		}
	}
	for i, command := range m.ValidationCommands {
		if strings.TrimSpace(command) == "" {
			return Manifest{}, fmt.Errorf("validation_commands[%d]: %w", i, ErrMissingField)
		}
	}

	return Manifest{
		Version:            m.Version,
		RequiredDocuments:  requiredDocuments,
		Rules:              append([]string(nil), m.Rules...),
		ValidationCommands: append([]string(nil), m.ValidationCommands...),
	}, nil
}

func sanitizeDocumentReadError(path string, err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return fmt.Errorf("%s: %w", path, pathErr.Err)
	}
	return fmt.Errorf("%s: read failed", path)
}
