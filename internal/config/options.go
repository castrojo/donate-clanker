package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultReadinessTimeout = 2 * time.Minute

type EnginePreference string

const (
	EngineAuto   EnginePreference = "auto"
	EnginePodman EnginePreference = "podman"
	EngineDocker EnginePreference = "docker"
)

var (
	ErrMissingWorkspace   = errors.New("missing workspace")
	ErrInvalidEngine      = errors.New("invalid engine")
	ErrInvalidRuntime     = errors.New("invalid container runtime")
	ErrProfileUnsupported = errors.New("profile selection is unavailable until the FSDK helper launch contract is published")
)

type Options struct {
	Engine                 EnginePreference
	ContainerRuntime       string
	StrictSandbox          bool
	Workspace              string
	Profile                string
	Model                  string
	CacheDir               string
	HiveConfigDir          string
	GooseConfigPath        string
	ProfileCatalogPath     string
	ProfileCatalogExplicit bool
	HelperImage            string
	ContributorImage       string
	HiveSourceDir          string
	HiveCommit             string
	NonInteractive         bool
	ReadinessTimeout       time.Duration
	ModelContainerPort     int
}

func Parse(args []string, env map[string]string) (Options, error) {
	defaults, err := defaultOptions(env)
	if err != nil {
		return Options{}, err
	}
	profileCatalogExplicit := strings.TrimSpace(env["DONATE_CLANKER_PROFILE_CATALOG"]) != ""

	fs := flag.NewFlagSet("donate-clanker", flag.ContinueOnError)
	fs.SetOutput(nil)

	engine := fs.String("engine", string(defaults.Engine), "container engine: auto, podman, docker")
	workspace := fs.String("workspace", defaults.Workspace, "workspace to mount read/write")
	profile := fs.String("profile", defaults.Profile, "reserved curated profile id (currently unsupported until the FSDK helper launch contract is published)")
	model := fs.String("model", defaults.Model, "explicit model override")
	cacheDir := fs.String("cache-dir", defaults.CacheDir, "persistent model cache directory")
	hiveConfigDir := fs.String("hive-config-dir", defaults.HiveConfigDir, "Hive config directory")
	gooseConfigPath := fs.String("goose-config", defaults.GooseConfigPath, "validated Goose local config path")
	profileCatalogPath := fs.String("profile-catalog", defaults.ProfileCatalogPath, "profile catalog path reserved for future native profile support")
	helperImage := fs.String("helper-image", defaults.HelperImage, "helper image reference")
	contributorImage := fs.String("contributor-image", defaults.ContributorImage, "worker image reference")
	hiveSourceDir := fs.String("hive-source-dir", defaults.HiveSourceDir, "upstream Hive checkout location")
	hiveCommit := fs.String("hive-commit", defaults.HiveCommit, "immutable upstream Hive commit override (40-hex SHA)")
	nonInteractive := fs.Bool("non-interactive", defaults.NonInteractive, "disable prompts")
	readinessTimeout := fs.Duration("readiness-timeout", defaults.ReadinessTimeout, "model readiness timeout")

	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	if flagProvided(fs, "profile-catalog") {
		profileCatalogExplicit = true
	}

	if strings.TrimSpace(*workspace) == "" {
		return Options{}, ErrMissingWorkspace
	}
	if trimmedProfile := strings.TrimSpace(*profile); trimmedProfile != "" {
		return Options{}, fmt.Errorf("%w: %q", ErrProfileUnsupported, trimmedProfile)
	}

	normalizedEngine, err := parseEnginePreference(*engine)
	if err != nil {
		return Options{}, err
	}

	return Options{
		Engine:                 normalizedEngine,
		ContainerRuntime:       defaults.ContainerRuntime,
		StrictSandbox:          defaults.StrictSandbox,
		Workspace:              normalizePath(*workspace),
		Profile:                strings.TrimSpace(*profile),
		Model:                  strings.TrimSpace(*model),
		CacheDir:               normalizePath(*cacheDir),
		HiveConfigDir:          normalizePath(*hiveConfigDir),
		GooseConfigPath:        normalizePath(*gooseConfigPath),
		ProfileCatalogPath:     normalizePath(*profileCatalogPath),
		ProfileCatalogExplicit: profileCatalogExplicit,
		HelperImage:            strings.TrimSpace(*helperImage),
		ContributorImage:       strings.TrimSpace(*contributorImage),
		HiveSourceDir:          normalizePath(*hiveSourceDir),
		HiveCommit:             strings.TrimSpace(*hiveCommit),
		NonInteractive:         *nonInteractive,
		ReadinessTimeout:       *readinessTimeout,
		ModelContainerPort:     8000,
	}, nil
}

func defaultOptions(env map[string]string) (Options, error) {
	home := strings.TrimSpace(env["HOME"])
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return Options{}, err
		}
	}

	workspace := strings.TrimSpace(env["DONATE_CLANKER_WORKSPACE"])
	if workspace == "" {
		workspace = strings.TrimSpace(env["PWD"])
	}

	profile := strings.TrimSpace(env["DONATE_CLANKER_PROFILE"])
	model := strings.TrimSpace(env["DONATE_CLANKER_MODEL"])
	if model == "" {
		model = strings.TrimSpace(env["GOOSE_MODEL"])
	}

	containerRuntime, err := parseContainerRuntime(env["CLANKER_CONTAINER_RUNTIME"])
	if err != nil {
		return Options{}, err
	}

	return Options{
		Engine:             EngineAuto,
		ContainerRuntime:   containerRuntime,
		StrictSandbox:      strings.TrimSpace(env["CLANKER_STRICT_SANDBOX"]) == "1",
		Workspace:          workspace,
		Profile:            profile,
		Model:              model,
		CacheDir:           firstNonEmpty(env["DONATE_CLANKER_CACHE_DIR"], filepath.Join(home, ".local", "state", "donate-clanker", "cache", "ramalama")),
		HiveConfigDir:      firstNonEmpty(env["DONATE_CLANKER_HIVE_CONFIG_DIR"], filepath.Join(home, ".config", "hive")),
		GooseConfigPath:    firstNonEmpty(env["DONATE_CLANKER_GOOSE_CONFIG"], filepath.Join(home, ".config", "goose", "config.yaml")),
		ProfileCatalogPath: firstNonEmpty(env["DONATE_CLANKER_PROFILE_CATALOG"], filepath.Join("image", "config", "models.json")),
		HelperImage:        strings.TrimSpace(env["DONATE_CLANKER_HELPER_IMAGE"]),
		ContributorImage:   strings.TrimSpace(env["DONATE_CLANKER_CONTRIBUTOR_IMAGE"]),
		HiveSourceDir:      firstNonEmpty(env["DONATE_CLANKER_HIVE_SOURCE_DIR"], filepath.Join(home, ".local", "state", "donate-clanker", "hive-src")),
		HiveCommit:         strings.TrimSpace(env["DONATE_CLANKER_HIVE_COMMIT"]),
		NonInteractive:     strings.EqualFold(strings.TrimSpace(env["DONATE_CLANKER_NON_INTERACTIVE"]), "true"),
		ReadinessTimeout:   defaultReadinessTimeout,
	}, nil
}

func parseContainerRuntime(raw string) (string, error) {
	runtime := strings.TrimSpace(raw)
	if runtime == "" {
		return "auto", nil
	}
	if strings.ContainsAny(runtime, " \t\r\n\x00") {
		return "", fmt.Errorf("%w: %q", ErrInvalidRuntime, raw)
	}
	return runtime, nil
}

func parseEnginePreference(raw string) (EnginePreference, error) {
	switch EnginePreference(strings.ToLower(strings.TrimSpace(raw))) {
	case "", EngineAuto:
		return EngineAuto, nil
	case EnginePodman:
		return EnginePodman, nil
	case EngineDocker:
		return EngineDocker, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrInvalidEngine, raw)
	}
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func flagProvided(fs *flag.FlagSet, name string) bool {
	provided := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			provided = true
		}
	})
	return provided
}
