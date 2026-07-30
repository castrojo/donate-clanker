package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	imageconfig "github.com/castrojo/donate-clanker/image/config"
)

const defaultContextSize = 32768

var (
	ErrMissingFile          = errors.New("missing file")
	ErrEmptyFile            = errors.New("empty file")
	ErrThinkingRequired     = errors.New("thinking must be explicitly false")
	ErrThinkingEnabled      = errors.New("thinking enabled")
	ErrMissingServerDisable = errors.New("missing server-side thinking disable")
)

type Catalog map[string]Profile

type Profile struct {
	ContextSize int      `json:"context_size"`
	Thinking    bool     `json:"thinking"`
	RuntimeArgs []string `json:"runtime_args"`
}

type rawCatalog map[string]rawProfile

type rawProfile struct {
	ContextSize int      `json:"context_size"`
	Thinking    *bool    `json:"thinking"`
	RuntimeArgs []string `json:"runtime_args"`
}

func Load(path string) (Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s: %w", path, ErrMissingFile)
		}
		return nil, err
	}
	return loadBytes(path, data)
}

func LoadBundled() (Catalog, error) {
	return loadBytes("embedded profile catalog", imageconfig.BundledModelsJSON())
}

func loadBytes(source string, data []byte) (Catalog, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%s: %w", source, ErrEmptyFile)
	}

	var raw rawCatalog
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	catalog := make(Catalog, len(raw))
	for id, profile := range raw {
		normalized, err := validateProfile(id, profile)
		if err != nil {
			return nil, err
		}
		catalog[id] = normalized
	}

	return catalog, nil
}

func validateProfile(id string, profile rawProfile) (Profile, error) {
	if profile.ContextSize == 0 {
		profile.ContextSize = defaultContextSize
	}

	switch {
	case profile.Thinking == nil:
		return Profile{}, fmt.Errorf("%s: %w", id, ErrThinkingRequired)
	case *profile.Thinking:
		return Profile{}, fmt.Errorf("%s: %w", id, ErrThinkingEnabled)
	}

	if !hasTrailingArgValue(profile.RuntimeArgs, "--thinking", "false") {
		return Profile{}, fmt.Errorf("%s: %w", id, ErrMissingServerDisable)
	}

	return Profile{
		ContextSize: profile.ContextSize,
		Thinking:    *profile.Thinking,
		RuntimeArgs: profile.RuntimeArgs,
	}, nil
}

func hasRuntimeArgs(args []string, want ...string) bool {
	if len(want) == 0 || len(args) < len(want) {
		return false
	}
	for i := 0; i <= len(args)-len(want); i++ {
		match := true
		for j := range want {
			if args[i+j] != want[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func hasTrailingArgValue(args []string, flag, want string) bool {
	value, ok := trailingArgValue(args, flag)
	return ok && value == want
}

func trailingArgValue(args []string, flag string) (string, bool) {
	var (
		value string
		found bool
	)

	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == flag:
			if i+1 >= len(args) {
				return "", false
			}
			value = args[i+1]
			found = true
			i++
		case len(arg) > len(flag) && arg[:len(flag)] == flag && arg[len(flag)] == '=':
			value = arg[len(flag)+1:]
			found = true
		}
	}

	return value, found
}
