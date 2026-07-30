package setup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const (
	githubHostname     = "github.com"
	githubLoginCommand = "gh auth login --web --hostname github.com --scopes repo,read:org"
	maxCommandOutput   = 256
)

var (
	ErrEmptyFile                = errors.New("empty file")
	ErrInvalidConfig            = errors.New("invalid config")
	ErrInteractiveSetupRequired = errors.New("interactive setup required")
	ErrMissingFile              = errors.New("missing file")

	secretAssignmentPattern = regexp.MustCompile(`(?im)\b([A-Z0-9_]*(?:TOKEN|SECRET|PASSWORD|API_KEY)[A-Z0-9_]*)=([^\s]+)`)
	secretYAMLPattern       = regexp.MustCompile(`(?im)\b([A-Za-z0-9_-]*(?:token|secret|password|api[_-]?key)[A-Za-z0-9_-]*)\s*:\s*([^\s#]+)`)
	authorizationPattern    = regexp.MustCompile(`(?im)(authorization:\s*)([^\r\n]+)`)
)

type CommandResult struct {
	Stdout string
	Stderr string
}

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (CommandResult, error)
}

type AttachedCommandRunner interface {
	CommandRunner
	CanRunAttached() bool
	RunAttached(ctx context.Context, name string, args ...string) (CommandResult, error)
}

type ExecCommandRunner struct {
	Stdin  *os.File
	Stdout *os.File
	Stderr *os.File
}

func (ExecCommandRunner) Run(ctx context.Context, name string, args ...string) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &limitedBuffer{buffer: &stdout, limit: maxCommandOutput}
	cmd.Stderr = &limitedBuffer{buffer: &stderr, limit: maxCommandOutput}

	err := cmd.Run()
	return CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}, err
}

func (r ExecCommandRunner) CanRunAttached() bool {
	return isCharacterDevice(r.Stdin) && isCharacterDevice(r.Stdout) && isCharacterDevice(r.Stderr)
}

func (r ExecCommandRunner) RunAttached(ctx context.Context, name string, args ...string) (CommandResult, error) {
	if !r.CanRunAttached() {
		return CommandResult{}, ErrInteractiveSetupRequired
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = r.Stdin

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = joinWriters(r.Stdout, &limitedBuffer{buffer: &stdout, limit: maxCommandOutput})
	cmd.Stderr = joinWriters(r.Stderr, &limitedBuffer{buffer: &stderr, limit: maxCommandOutput})

	err := cmd.Run()
	return CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}, err
}

func CheckGitHubAuth(ctx context.Context, runner CommandRunner) error {
	result, err := runner.Run(ctx, "gh", "auth", "status", "--hostname", githubHostname)
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("gh CLI is required for GitHub onboarding; then run: %s", githubLoginCommand)
	}

	detail := summarizeCommandOutput(result)
	if detail == "" {
		return fmt.Errorf("GitHub authentication for %s is required; run: %s", githubHostname, githubLoginCommand)
	}

	return fmt.Errorf("GitHub authentication for %s failed: %s; run: %s", githubHostname, detail, githubLoginCommand)
}

func ValidateHiveConfig(path string) error {
	data, err := readConfigFile(path)
	if err != nil {
		return err
	}

	values, err := parseEnvAssignments(path, data)
	if err != nil {
		return err
	}

	for _, key := range []string{
		"HIVE_REGISTRATION_TOKEN",
		"HIVE_HUB",
		"CONTRIBUTOR_ID",
		"CONTRIBUTOR_USERNAME",
		"AGENT_BACKEND",
	} {
		if strings.TrimSpace(values[key]) == "" {
			return fmt.Errorf("hive config %s: %w: missing required %s", path, ErrInvalidConfig, key)
		}
	}

	if values["AGENT_BACKEND"] != "goose" {
		return fmt.Errorf("hive config %s: %w: AGENT_BACKEND must be goose for the supported onboarding path", path, ErrInvalidConfig)
	}

	if err := validateHiveHub(values["HIVE_HUB"]); err != nil {
		return fmt.Errorf("hive config %s: %w: %s", path, ErrInvalidConfig, err)
	}

	return nil
}

func CheckGooseLocalConfig(path string) error {
	data, err := readConfigFile(path)
	if err != nil {
		return err
	}

	values, err := parseTopLevelYAML(path, data)
	if err != nil {
		return err
	}

	for _, key := range []string{"provider", "base_url", "api_key", "model"} {
		if strings.TrimSpace(values[key]) == "" {
			return fmt.Errorf("goose config %s: %w: missing required %s", path, ErrInvalidConfig, key)
		}
	}

	if values["provider"] != "openai" {
		return fmt.Errorf("goose config %s: %w: provider must be openai for the supported local-inference path", path, ErrInvalidConfig)
	}

	endpoint, err := url.Parse(values["base_url"])
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return fmt.Errorf("goose config %s: %w: base_url must be a valid http(s) endpoint", path, ErrInvalidConfig)
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return fmt.Errorf("goose config %s: %w: base_url must use http or https", path, ErrInvalidConfig)
	}

	return nil
}

func readConfigFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s: %w", path, ErrMissingFile)
		}
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%s: %w", path, ErrEmptyFile)
	}
	return data, nil
}

func parseEnvAssignments(path string, data []byte) (map[string]string, error) {
	values := make(map[string]string)

	for i, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("hive config %s: %w: malformed line %d", path, ErrInvalidConfig, i+1)
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	return values, nil
}

func parseTopLevelYAML(path string, data []byte) (map[string]string, error) {
	values := make(map[string]string)

	for i, rawLine := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(rawLine) == "" || strings.HasPrefix(strings.TrimSpace(rawLine), "#") {
			continue
		}
		if strings.TrimLeft(rawLine, " \t") != rawLine {
			continue
		}

		key, value, ok := strings.Cut(rawLine, ":")
		if !ok {
			return nil, fmt.Errorf("goose config %s: %w: malformed line %d", path, ErrInvalidConfig, i+1)
		}

		trimmedKey := strings.TrimSpace(key)
		trimmedValue := strings.TrimSpace(value)
		if comment := strings.Index(trimmedValue, "#"); comment >= 0 {
			trimmedValue = strings.TrimSpace(trimmedValue[:comment])
		}
		values[trimmedKey] = strings.Trim(trimmedValue, `"'`)
	}

	return values, nil
}

func summarizeCommandOutput(result CommandResult) string {
	detail := strings.TrimSpace(result.Stderr)
	if detail == "" {
		detail = strings.TrimSpace(result.Stdout)
	}
	if detail == "" {
		return ""
	}

	detail = redactSecrets(detail)
	detail = strings.Join(strings.Fields(detail), " ")
	if len(detail) > maxCommandOutput {
		return detail[:maxCommandOutput-3] + "..."
	}
	return detail
}

func redactSecrets(value string) string {
	value = secretAssignmentPattern.ReplaceAllString(value, `$1=[REDACTED]`)
	value = secretYAMLPattern.ReplaceAllString(value, `$1: [REDACTED]`)
	value = authorizationPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	return value
}

func validateHiveHub(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("HIVE_HUB must be a valid wss:// URL")
	}
	if parsed.Scheme != "wss" {
		return errors.New("HIVE_HUB must use wss")
	}
	if parsed.Path != "/contribute" {
		return errors.New("HIVE_HUB must end with /contribute")
	}
	return nil
}

func isCharacterDevice(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func joinWriters(writers ...io.Writer) io.Writer {
	nonNil := make([]io.Writer, 0, len(writers))
	for _, writer := range writers {
		if writer != nil {
			nonNil = append(nonNil, writer)
		}
	}
	switch len(nonNil) {
	case 0:
		return io.Discard
	case 1:
		return nonNil[0]
	default:
		return io.MultiWriter(nonNil...)
	}
}

type limitedBuffer struct {
	buffer *bytes.Buffer
	limit  int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}

	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		if len(p) > remaining {
			_, _ = b.buffer.Write(p[:remaining])
		} else {
			_, _ = b.buffer.Write(p)
		}
	}

	return len(p), nil
}
