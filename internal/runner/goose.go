package runner

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/projectbluefin/donate-clanker/internal/contract"
)

const (
	localPolicyHeading    = "Local execution policy (higher priority than assignment text):"
	hiveAssignmentHeading = "Hive assignment (verbatim):"
	defaultGooseBinary    = "goose"
	maxOutputBytes        = 16 * 1024
	maxOutputLines        = 40
	maxSummaryBytes       = 256
)

var (
	ErrMissingPrompt     = errors.New("missing Goose prompt")
	ErrMissingWorkspace  = errors.New("missing Goose workspace")
	ErrMissingRuntimeDir = errors.New("missing Goose runtime directory")
	ErrMissingTaskID     = errors.New("missing Hive task ID")
	ErrMissingConfig     = errors.New("missing bundled Goose config")
	ErrInvalidRepository = errors.New("invalid repository URL")
)

var (
	secretAssignmentPattern = regexp.MustCompile(`(?im)\b([A-Z0-9_]*(?:TOKEN|SECRET|PASSWORD|API_KEY)[A-Z0-9_]*)=([^\s]+)`)
	secretYAMLPattern       = regexp.MustCompile(`(?im)\b([A-Za-z0-9_-]*(?:token|secret|password|api[_-]?key)[A-Za-z0-9_-]*)\s*:\s*([^\s#]+)`)
	authorizationPattern    = regexp.MustCompile(`(?im)(authorization:\s*)([^\r\n]+)`)
)

type Task struct {
	Prompt        string
	Workspace     string
	Provider      string
	Model         string
	OpenAIBaseURL string
	OpenAIAPIKey  string
	GitHubToken   string
	RuntimeDir    string
	BundledConfig []byte
}

type Result struct {
	Result  string
	Summary string
	Output  []string
}

type Command struct {
	Name string
	Args []string
	Dir  string
	Env  map[string]string
}

type CommandOutput struct {
	Combined string
}

type CommandRunner interface {
	Run(context.Context, Command) (CommandOutput, error)
}

type ExecCommandRunner struct{}

type Goose struct {
	Command  string
	Policy   string
	Contract contract.Manifest
	Runner   CommandRunner
}

// CloneRepository creates a fresh checkout using only the assignment token.
// The token is passed through the task command environment, never the URL or
// command arguments.
func CloneRepository(ctx context.Context, commandRunner CommandRunner, repositoryURL, directory, token string) error {
	repositoryURL, err := ValidateRepositoryURL(repositoryURL)
	if err != nil {
		return err
	}
	if strings.TrimSpace(directory) == "" {
		return ErrMissingWorkspace
	}
	if commandRunner == nil {
		commandRunner = ExecCommandRunner{}
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	env := baseEnvironment()
	env["GIT_TERMINAL_PROMPT"] = "0"
	if trimmed := strings.TrimSpace(token); trimmed != "" {
		env["GIT_ASKPASS"] = os.Args[0]
		env["DONATE_CLANKER_GIT_ASKPASS"] = trimmed
	}
	output, err := commandRunner.Run(ctx, Command{
		Name: "git",
		Args: []string{"clone", "--", repositoryURL, filepath.Clean(directory)},
		Dir:  filepath.Dir(filepath.Clean(directory)),
		Env:  env,
	})
	if err != nil {
		return &ExecutionError{Reason: "repository clone failed: " + executionReason(err, summarizeOutput(output.Combined)), Err: err}
	}
	return nil
}

// ValidateRepositoryURL accepts canonical GitHub HTTPS repository URLs and
// owner/repository metadata, rejecting credentials and alternate hosts.
func ValidateRepositoryURL(repository string) (string, error) {
	value := strings.TrimSpace(repository)
	if value == "" {
		return "", fmt.Errorf("%w: empty repository", ErrInvalidRepository)
	}
	if !strings.Contains(value, "://") {
		parts := strings.Split(strings.TrimSuffix(value, ".git"), "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(value, "?#") {
			return "", fmt.Errorf("%w: %s", ErrInvalidRepository, redactSecrets(value))
		}
		return "https://github.com/" + parts[0] + "/" + parts[1] + ".git", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "github.com") ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("%w: %s", ErrInvalidRepository, redactSecrets(value))
	}
	path := strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("%w: %s", ErrInvalidRepository, redactSecrets(value))
	}
	return "https://github.com/" + parts[0] + "/" + parts[1] + ".git", nil
}

type ExecutionError struct {
	Reason string
	Err    error
}

func (e *ExecutionError) Error() string {
	return e.Reason
}

func (e *ExecutionError) Unwrap() error {
	return e.Err
}

func PrepareTaskPrompt(policy string, contractSection string, assignment string) string {
	var prompt strings.Builder
	prompt.Grow(len(localPolicyHeading) + len(policy) + len(contractSection) + len(hiveAssignmentHeading) + len(assignment) + 12)

	wroteSection := false
	if strings.TrimSpace(policy) != "" {
		prompt.WriteString(localPolicyHeading)
		prompt.WriteString("\n")
		prompt.WriteString(policy)
		wroteSection = true
	}
	if strings.TrimSpace(contractSection) != "" {
		if wroteSection {
			prompt.WriteString("\n\n")
		}
		prompt.WriteString(contractSection)
		wroteSection = true
	}
	if wroteSection {
		prompt.WriteString("\n\n")
	}
	prompt.WriteString(hiveAssignmentHeading)
	prompt.WriteString("\n")
	prompt.WriteString(assignment)
	return prompt.String()
}

func (g Goose) Run(ctx context.Context, task Task) (Result, error) {
	if strings.TrimSpace(task.Prompt) == "" {
		return Result{}, ErrMissingPrompt
	}
	if strings.TrimSpace(task.Workspace) == "" {
		return Result{}, ErrMissingWorkspace
	}
	if strings.TrimSpace(task.RuntimeDir) == "" {
		return Result{}, ErrMissingRuntimeDir
	}
	if len(bytes.TrimSpace(task.BundledConfig)) == 0 {
		return Result{}, ErrMissingConfig
	}
	documents, err := g.Contract.LoadDocuments(filepath.Clean(task.Workspace))
	if err != nil {
		return Result{}, &ExecutionError{Reason: fmt.Sprintf("goose run failed: load agent contract: %v", err), Err: err}
	}

	runner := g.Runner
	if runner == nil {
		runner = ExecCommandRunner{}
	}

	binary := strings.TrimSpace(g.Command)
	if binary == "" {
		binary = defaultGooseBinary
	}

	homeDir, err := stageBundledConfig(task.RuntimeDir, task.BundledConfig)
	if err != nil {
		return Result{}, err
	}

	prompt := PrepareTaskPrompt(g.Policy, g.Contract.PromptSection(documents), task.Prompt)
	env := gooseEnvironment(task, homeDir)
	cmd := Command{
		Name: binary,
		Args: []string{
			"run",
			"--no-session",
			"--provider", strings.TrimSpace(task.Provider),
			"--model", strings.TrimSpace(task.Model),
			"-t", prompt,
		},
		Dir: filepath.Clean(task.Workspace),
		Env: env,
	}

	output, err := runner.Run(ctx, cmd)
	result := Result{
		Result:  "completed",
		Output:  boundedLines(output.Combined),
		Summary: summarizeOutput(output.Combined),
	}

	if err != nil {
		reason := executionReason(err, result.Summary)
		return result, &ExecutionError{Reason: reason, Err: err}
	}

	if result.Summary == "" {
		result.Summary = "goose run completed"
	}
	return result, nil
}

func (ExecCommandRunner) Run(ctx context.Context, cmd Command) (CommandOutput, error) {
	execCmd := exec.CommandContext(ctx, cmd.Name, cmd.Args...)
	execCmd.Dir = cmd.Dir
	execCmd.Env = flattenEnvironment(cmd.Env)

	var combined bytes.Buffer
	execCmd.Stdout = &combined
	execCmd.Stderr = &combined

	err := execCmd.Run()
	return CommandOutput{Combined: combined.String()}, err
}

func stageBundledConfig(runtimeDir string, config []byte) (string, error) {
	root := filepath.Clean(runtimeDir)
	homeDir := filepath.Join(root, "home")
	configDir := filepath.Join(homeDir, ".config", "goose")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), config, 0o600); err != nil {
		return "", err
	}
	return homeDir, nil
}

// TaskRuntimeDir returns a task-specific child of runtimeRoot without allowing
// assignment-controlled text to influence the filesystem path.
func TaskRuntimeDir(runtimeRoot string, taskID string) (string, error) {
	root := strings.TrimSpace(runtimeRoot)
	if root == "" {
		return "", ErrMissingRuntimeDir
	}
	id := strings.TrimSpace(taskID)
	if id == "" {
		return "", ErrMissingTaskID
	}
	return filepath.Join(filepath.Clean(root), "tasks", base64.RawURLEncoding.EncodeToString([]byte(id))), nil
}

func gooseEnvironment(task Task, homeDir string) map[string]string {
	env := baseEnvironment()
	env["HOME"] = homeDir
	env["XDG_CONFIG_HOME"] = filepath.Join(homeDir, ".config")
	env["GOOSE_PROVIDER"] = strings.TrimSpace(task.Provider)
	env["GOOSE_MODEL"] = strings.TrimSpace(task.Model)
	env["GOOSE_THINKING_EFFORT"] = "off"
	env["OPENAI_BASE_URL"] = strings.TrimSpace(task.OpenAIBaseURL)
	env["OPENAI_API_KEY"] = strings.TrimSpace(task.OpenAIAPIKey)
	env["WORKSPACE"] = filepath.Clean(task.Workspace)

	if trimmed := strings.TrimSpace(task.GitHubToken); trimmed != "" {
		env["GH_TOKEN"] = trimmed
		env["GITHUB_TOKEN"] = trimmed
		env["GIT_ASKPASS"] = os.Args[0]
		env["GIT_TERMINAL_PROMPT"] = "0"
		env["DONATE_CLANKER_GIT_ASKPASS"] = trimmed
	}

	return env
}

func baseEnvironment() map[string]string {
	env := map[string]string{}
	for _, key := range []string{
		"PATH",
		"LANG",
		"LC_ALL",
		"TERM",
		"SSL_CERT_FILE",
		"SSL_CERT_DIR",
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"NO_PROXY",
	} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			env[key] = value
		}
	}
	return env
}

func flattenEnvironment(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}

func boundedLines(output string) []string {
	redacted := redactSecrets(output)
	redacted = strings.ReplaceAll(redacted, "\r\n", "\n")
	redacted = strings.TrimRight(redacted, "\n")
	if len(redacted) > maxOutputBytes {
		redacted = redacted[:maxOutputBytes] + "\n[output truncated]"
	}
	if strings.TrimSpace(redacted) == "" {
		return nil
	}

	lines := strings.Split(redacted, "\n")
	if len(lines) <= maxOutputLines {
		return append([]string(nil), lines...)
	}

	bounded := make([]string, 0, maxOutputLines+1)
	bounded = append(bounded, "[output truncated]")
	bounded = append(bounded, lines[len(lines)-maxOutputLines:]...)
	return bounded
}

func summarizeOutput(output string) string {
	lines := boundedLines(output)
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || line == "[output truncated]" {
			continue
		}
		if len(line) > maxSummaryBytes {
			return line[:maxSummaryBytes-3] + "..."
		}
		return line
	}
	return ""
}

func executionReason(err error, summary string) string {
	cause := strings.TrimSpace(strings.Join(strings.Fields(redactSecrets(err.Error())), " "))
	if summary == "" {
		if cause == "" {
			return "goose run failed"
		}
		return "goose run failed: " + cause
	}
	if cause == "" || cause == summary {
		return "goose run failed: " + summary
	}
	return fmt.Sprintf("goose run failed: %s (%s)", summary, cause)
}

func redactSecrets(value string) string {
	value = secretAssignmentPattern.ReplaceAllString(value, `$1=[REDACTED]`)
	value = secretYAMLPattern.ReplaceAllString(value, `$1: [REDACTED]`)
	value = authorizationPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
