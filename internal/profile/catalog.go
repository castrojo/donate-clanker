package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const defaultContextSize = 32768

var (
	ErrMissingFile          = errors.New("missing file")
	ErrEmptyFile            = errors.New("empty file")
	ErrThinkingEnabled      = errors.New("thinking enabled")
	ErrMissingServerDisable = errors.New("missing server-side thinking disable")
)

type Catalog map[string]Profile

type Profile struct {
	ContextSize int      `json:"context_size"`
	Thinking    bool     `json:"thinking"`
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
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%s: %w", path, ErrEmptyFile)
	}

	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}

	for id, profile := range catalog {
		normalized, err := validateProfile(id, profile)
		if err != nil {
			return nil, err
		}
		catalog[id] = normalized
	}

	return catalog, nil
}

func validateProfile(id string, profile Profile) (Profile, error) {
	if profile.ContextSize == 0 {
		profile.ContextSize = defaultContextSize
	}
	if profile.Thinking {
		return Profile{}, fmt.Errorf("%s: %w", id, ErrThinkingEnabled)
	}
	if !hasRuntimeArgs(profile.RuntimeArgs, "--thinking", "false") {
		return Profile{}, fmt.Errorf("%s: %w", id, ErrMissingServerDisable)
	}
	return profile, nil
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
