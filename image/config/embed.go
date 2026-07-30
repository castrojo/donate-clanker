package imageconfig

import _ "embed"

//go:embed models.json
var bundledModelsJSON []byte

func BundledModelsJSON() []byte {
	return append([]byte(nil), bundledModelsJSON...)
}
