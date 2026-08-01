// Package imageconfig embeds the container payload that must stay in lockstep
// with the Go launcher.
//
// The Goose configuration lives here rather than in the image alone because
// Hive's contributor entrypoint unconditionally overwrites
// ~/.config/goose/config.yaml at every startup. donate-clanker therefore points
// Goose at a controlled root via GOOSE_PATH_ROOT and ships this file there.
package imageconfig

import _ "embed"

//go:embed goose.yaml
var bundledGooseConfig []byte

//go:embed local-agent-policy.md
var bundledLocalAgentPolicy []byte

// BundledGooseConfig returns the controlled Goose configuration.
func BundledGooseConfig() []byte {
	return append([]byte(nil), bundledGooseConfig...)
}

// BundledLocalAgentPolicy returns the local execution policy text.
func BundledLocalAgentPolicy() []byte {
	return append([]byte(nil), bundledLocalAgentPolicy...)
}
