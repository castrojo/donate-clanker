package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
)

const policyMaxSize = 64 * 1024

const (
	DefaultGooseProvider       = "openai"
	DefaultGooseModel          = "local"
	DefaultGooseOpenAIBaseURL  = "http://127.0.0.1:8000/v1"
	DefaultGooseOpenAIAPIKey   = "local"
	DefaultGooseThinkingEffort = "off"
)

var (
	ErrMissingFile  = errors.New("missing file")
	ErrEmptyFile    = errors.New("empty file")
	ErrFileTooLarge = errors.New("file too large")
)

func LoadBundledGooseConfig(path string) ([]byte, error) {
	return loadBytes(path, 0)
}

func LoadLocalAgentPolicy(path string) (string, error) {
	data, err := loadBytes(path, policyMaxSize)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func DefaultGooseEnvironment(model string) map[string]string {
	if model == "" {
		model = DefaultGooseModel
	}

	return map[string]string{
		"GOOSE_PROVIDER":        DefaultGooseProvider,
		"GOOSE_MODEL":           model,
		"GOOSE_THINKING_EFFORT": DefaultGooseThinkingEffort,
		"OPENAI_BASE_URL":       DefaultGooseOpenAIBaseURL,
		"OPENAI_API_KEY":        DefaultGooseOpenAIAPIKey,
	}
}

func loadBytes(path string, maxSize int64) ([]byte, error) {
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
	if maxSize > 0 && int64(len(data)) > maxSize {
		return nil, fmt.Errorf("%s: %w", path, ErrFileTooLarge)
	}
	return data, nil
}
