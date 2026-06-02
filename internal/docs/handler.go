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

// scalarHTML embeds the Scalar API reference UI via CDN.
// Version is pinned to the @1 major range; upgrade by bumping the tag and
// regenerating the SRI hash (sha384) at https://www.srihash.org/.
const scalarHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <title>EventPulse API Reference</title>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
</head>
<body>
  <script
    id="api-reference"
    data-url="/openapi.json"
    crossorigin="anonymous"
    src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@1"></script>
</body>
</html>`

// HandleUI serves the Scalar interactive API reference.
func HandleUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(scalarHTML))
}
