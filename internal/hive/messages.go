package hive

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrInvalidAssignment  = errors.New("invalid hive assignment")
	ErrInvalidCredentials = errors.New("invalid hive credentials")
)

const defaultCLIBackend = "goose"

type Credentials struct {
	RegistrationToken   string
	WSURL               string
	ContributorID       string
	ContributorUsername string
	CLIBackend          string
	Model               string
}

type Assignment struct {
	TaskID            string
	Kind              string
	Repo              string
	Number            int
	Title             string
	URL               string
	Prompt            string
	GitHubToken       string
	TokenExpiresAt    time.Time
	Restrictions      json.RawMessage
	ContributorLabels []string
}

func (a Assignment) Verbatim() string {
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "Task ID: %s\nKind: %s\nRepo: %s\nNumber: %d\nTitle: %s", a.TaskID, a.Kind, a.Repo, a.Number, a.Title)
	if trimmedURL := strings.TrimSpace(a.URL); trimmedURL != "" {
		fmt.Fprintf(&prompt, "\nURL: %s", trimmedURL)
	}
	if strings.TrimSpace(a.Prompt) != "" {
		prompt.WriteString("\n\n")
		prompt.WriteString(a.Prompt)
	}
	return prompt.String()
}

type wsMessage struct {
	Type              string          `json:"type"`
	Seq               int             `json:"seq,omitempty"`
	Nonce             string          `json:"nonce,omitempty"`
	ContributorID     string          `json:"contributor_id,omitempty"`
	TrustTier         string          `json:"trust_tier,omitempty"`
	Permissions       []string        `json:"permissions,omitempty"`
	Reason            string          `json:"reason,omitempty"`
	RegistrationToken string          `json:"registration_token,omitempty"`
	CLIBackend        string          `json:"cli_backend,omitempty"`
	Model             string          `json:"model,omitempty"`
	TaskID            string          `json:"task_id,omitempty"`
	Kind              string          `json:"kind,omitempty"`
	Repo              string          `json:"repo,omitempty"`
	Number            int             `json:"number,omitempty"`
	Title             string          `json:"title,omitempty"`
	URL               string          `json:"url,omitempty"`
	Labels            []string        `json:"labels,omitempty"`
	Prompt            string          `json:"prompt,omitempty"`
	GitHubToken       string          `json:"github_token,omitempty"`
	TokenExpiresAt    string          `json:"token_expires_at,omitempty"`
	Restrictions      json.RawMessage `json:"restrictions,omitempty"`
	Role              string          `json:"role,omitempty"`
	ContribLabels     []string        `json:"contributor_labels,omitempty"`
	Status            string          `json:"status,omitempty"`
	Result            string          `json:"result,omitempty"`
	Summary           string          `json:"summary,omitempty"`
	TmuxOutput        []string        `json:"tmux_output,omitempty"`
	AcceptedModels    []string        `json:"accepted_models,omitempty"`
}

func LoadCredentials(configPath string, env map[string]string) (Credentials, error) {
	values := map[string]string{}

	if trimmedPath := strings.TrimSpace(configPath); trimmedPath != "" {
		loaded, err := readEnvFile(trimmedPath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) || !hasCredentialOverrides(env) {
				return Credentials{}, err
			}
		} else {
			values = loaded
		}
	}

	creds := Credentials{
		RegistrationToken:   firstNonEmpty(env["HIVE_REGISTRATION_TOKEN"], values["HIVE_REGISTRATION_TOKEN"]),
		ContributorID:       firstNonEmpty(env["CONTRIBUTOR_ID"], values["CONTRIBUTOR_ID"]),
		ContributorUsername: firstNonEmpty(env["CONTRIBUTOR_USERNAME"], values["CONTRIBUTOR_USERNAME"]),
		CLIBackend:          firstNonEmpty(env["AGENT_BACKEND"], values["AGENT_BACKEND"], defaultCLIBackend),
		Model:               firstNonEmpty(env["AGENT_MODEL"], env["GOOSE_MODEL"]),
	}

	rawWSURL := firstNonEmpty(env["HIVE_WS_URL"], env["HIVE_HUB"], values["HIVE_HUB"])
	wsURL, err := NormalizeWSURL(rawWSURL)
	if err != nil {
		return Credentials{}, fmt.Errorf("%w: %s", ErrInvalidCredentials, err)
	}
	creds.WSURL = wsURL

	if strings.TrimSpace(creds.RegistrationToken) == "" {
		return Credentials{}, fmt.Errorf("%w: missing HIVE_REGISTRATION_TOKEN", ErrInvalidCredentials)
	}
	if strings.TrimSpace(creds.CLIBackend) == "" {
		return Credentials{}, fmt.Errorf("%w: missing AGENT_BACKEND", ErrInvalidCredentials)
	}

	return creds, nil
}

func NormalizeWSURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("missing HIVE_HUB or HIVE_WS_URL")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("Hive WebSocket URL must be a valid wss:// URL")
	}
	if parsed.Scheme != "wss" {
		return "", errors.New("Hive WebSocket URL must use wss")
	}

	switch strings.TrimRight(parsed.Path, "/") {
	case "/contribute":
		parsed.Path = "/api/contribute/ws"
	case "/api/contribute/ws":
		parsed.Path = "/api/contribute/ws"
	default:
		return "", errors.New("Hive WebSocket URL must end with /contribute or /api/contribute/ws")
	}

	return parsed.String(), nil
}

func assignmentFromMessage(msg wsMessage) (Assignment, error) {
	missing := make([]string, 0, 5)
	if strings.TrimSpace(msg.TaskID) == "" {
		missing = append(missing, "task_id")
	}
	if strings.TrimSpace(msg.Kind) == "" {
		missing = append(missing, "kind")
	}
	if strings.TrimSpace(msg.Repo) == "" {
		missing = append(missing, "repo")
	}
	if msg.Number <= 0 {
		missing = append(missing, "number")
	}
	if strings.TrimSpace(msg.Title) == "" {
		missing = append(missing, "title")
	}
	if len(missing) > 0 {
		return Assignment{}, fmt.Errorf("%w: missing %s", ErrInvalidAssignment, strings.Join(missing, ", "))
	}

	assignment := Assignment{
		TaskID:            msg.TaskID,
		Kind:              msg.Kind,
		Repo:              msg.Repo,
		Number:            msg.Number,
		Title:             msg.Title,
		URL:               strings.TrimSpace(msg.URL),
		Prompt:            msg.Prompt,
		GitHubToken:       msg.GitHubToken,
		Restrictions:      msg.Restrictions,
		ContributorLabels: append([]string(nil), msg.ContribLabels...),
	}
	if expiresAt := strings.TrimSpace(msg.TokenExpiresAt); expiresAt != "" {
		if parsed, err := time.Parse(time.RFC3339, expiresAt); err == nil {
			assignment.TokenExpiresAt = parsed
		}
	}

	return assignment, nil
}

func readEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}

	values := map[string]string{}
	for i, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s: malformed line %d", path, i+1)
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	return values, nil
}

func hasCredentialOverrides(env map[string]string) bool {
	for _, key := range []string{"HIVE_REGISTRATION_TOKEN", "HIVE_HUB", "HIVE_WS_URL"} {
		if strings.TrimSpace(env[key]) != "" {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
