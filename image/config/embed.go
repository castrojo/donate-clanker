package imageconfig

import _ "embed"

//go:embed models.json
var bundledModelsJSON []byte

//go:embed agent-contract.json
var bundledAgentContractJSON []byte

func BundledModelsJSON() []byte {
	return append([]byte(nil), bundledModelsJSON...)
}

func BundledAgentContractJSON() []byte {
	return append([]byte(nil), bundledAgentContractJSON...)
}
