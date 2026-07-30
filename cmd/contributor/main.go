package main

import (
	"context"
	"flag"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/projectbluefin/donate-clanker/internal/config"
	"github.com/projectbluefin/donate-clanker/internal/contract"
	"github.com/projectbluefin/donate-clanker/internal/hive"
	"github.com/projectbluefin/donate-clanker/internal/runner"
)

const defaultRuntimeDir = "/var/lib/donate-clanker"

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("contributor", flag.ContinueOnError)
	fs.SetOutput(nil)

	workspace := fs.String("workspace", config.WorkspaceMountPath, "mounted workspace path")
	bundledGooseConfig := fs.String("bundled-goose-config", "/etc/donate-clanker/goose.yaml", "bundled Goose config path")
	policyFile := fs.String("policy-file", "/etc/donate-clanker/local-agent-policy.md", "bundled policy path")
	hiveConfigDir := fs.String("hive-config-dir", firstNonEmpty(os.Getenv("HIVE_CONFIG_DIR"), config.HiveMountPath), "mounted Hive config directory")
	runtimeDir := fs.String("runtime-dir", firstNonEmpty(os.Getenv("DONATE_CLANKER_RUNTIME_DIR"), defaultRuntimeDir), "writable Goose runtime directory")
	gooseBinary := fs.String("goose-binary", "goose", "goose CLI binary")

	if err := fs.Parse(args); err != nil {
		return err
	}

	bundledConfig, err := config.LoadBundledGooseConfig(*bundledGooseConfig)
	if err != nil {
		return err
	}
	policy, err := config.LoadLocalAgentPolicy(*policyFile)
	if err != nil {
		return err
	}
	manifest, err := contract.LoadBundled()
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Clean(*workspace)); err != nil {
		return err
	}

	creds, err := hive.LoadCredentials(filepath.Join(filepath.Clean(*hiveConfigDir), "contributor.env"), currentEnvironment())
	if err != nil {
		return err
	}
	clearWorkerCredentialEnvironment()

	provider := firstNonEmpty(os.Getenv("GOOSE_PROVIDER"), config.DefaultGooseProvider)
	model := firstNonEmpty(os.Getenv("GOOSE_MODEL"), creds.Model, config.DefaultGooseModel)
	creds.Model = model
	if creds.CLIBackend == "" {
		creds.CLIBackend = "goose"
	}

	handler := contributorHandler{
		baseTask: runner.Task{
			Workspace:     filepath.Clean(*workspace),
			Provider:      provider,
			Model:         model,
			OpenAIBaseURL: firstNonEmpty(os.Getenv("OPENAI_BASE_URL"), config.DefaultGooseOpenAIBaseURL),
			OpenAIAPIKey:  firstNonEmpty(os.Getenv("OPENAI_API_KEY"), config.DefaultGooseOpenAIAPIKey),
			RuntimeDir:    filepath.Clean(*runtimeDir),
			BundledConfig: bundledConfig,
		},
		goose: runner.Goose{
			Command:  *gooseBinary,
			Policy:   policy,
			Contract: manifest,
		},
		observationWriter: os.Stderr,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return hive.NewClient().Run(ctx, creds, &handler)
}

type contributorHandler struct {
	baseTask          runner.Task
	goose             runner.Goose
	observationWriter io.Writer
	now               func() time.Time

	mu     sync.Mutex
	active *activeTask
}

type activeTask struct {
	assignment hive.Assignment
}

func (h *contributorHandler) Handle(ctx context.Context, assignment hive.Assignment) (hive.TaskReport, error) {
	now := h.now
	if now == nil {
		now = time.Now
	}
	startedAt := now()

	h.mu.Lock()
	active := &activeTask{assignment: assignment}
	h.active = active
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if h.active == active {
			h.active = nil
		}
		h.mu.Unlock()
	}()

	task := h.baseTask
	task.Prompt = assignment.Verbatim()
	task.GitHubToken = assignment.GitHubToken
	var err error
	task.RuntimeDir, err = runner.TaskRuntimeDir(task.RuntimeDir, assignment.TaskID)
	if err != nil {
		return hive.TaskReport{}, err
	}
	result, err := h.goose.Run(ctx, task)
	if cleanupErr := os.RemoveAll(task.RuntimeDir); cleanupErr != nil {
		writeTaskCleanupError(h.observationWriter, cleanupErr)
	}
	writeTaskObservation(h.observationWriter, assignment, startedAt, now(), ctx, err)

	return hive.TaskReport{
		Result:  result.Result,
		Summary: result.Summary,
		Output:  result.Output,
	}, err
}

func (h *contributorHandler) Refresh(_ context.Context, assignment hive.Assignment) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.active == nil || h.active.assignment.TaskID != assignment.TaskID {
		return nil
	}
	h.active.assignment = assignment
	return nil
}

func currentEnvironment() map[string]string {
	values := map[string]string{}
	for _, key := range []string{
		"HIVE_REGISTRATION_TOKEN",
		"HIVE_HUB",
		"HIVE_WS_URL",
		"CONTRIBUTOR_ID",
		"CONTRIBUTOR_USERNAME",
		"AGENT_BACKEND",
		"AGENT_MODEL",
		"GOOSE_MODEL",
	} {
		values[key] = os.Getenv(key)
	}
	return values
}

func clearWorkerCredentialEnvironment() {
	for _, key := range []string{
		"HIVE_REGISTRATION_TOKEN",
		"HIVE_HUB",
		"HIVE_WS_URL",
		"CONTRIBUTOR_ID",
		"CONTRIBUTOR_USERNAME",
		"HIVE_CONFIG_DIR",
		"GH_TOKEN",
		"GITHUB_TOKEN",
		"GH_CONFIG_DIR",
		"GITHUB_CONFIG_DIR",
	} {
		_ = os.Unsetenv(key)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func writeTaskCleanupError(writer io.Writer, err error) {
	if writer == nil || err == nil {
		return
	}
	_, _ = io.WriteString(writer, "task runtime cleanup failed: "+err.Error()+"\n")
}
