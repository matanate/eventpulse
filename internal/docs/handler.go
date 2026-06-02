package docs

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"gopkg.in/yaml.v3"
)

//go:embed openapi.yaml
var specYAML []byte

var specJSON []byte

func init() {
	// yaml.v3 → any → json.Marshal: safe for all scalar types the spec uses.
	// Binary YAML scalars (!!binary) are intentionally excluded from openapi.yaml.
	var v any
	if err := yaml.Unmarshal(specYAML, &v); err != nil {
		panic("docs: invalid openapi.yaml: " + err.Error())
	}
	b, err := json.Marshal(v)
	if err != nil {
		panic("docs: cannot marshal openapi spec to JSON: " + err.Error())
	}
	specJSON = b
}

// HandleSpec serves the OpenAPI 3.1 specification as JSON.
func HandleSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(specJSON)
}

