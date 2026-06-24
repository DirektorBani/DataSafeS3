package openapi

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"

	yaml "go.yaml.in/yaml/v2"
)

//go:embed openapi.yaml
var specYAML []byte

//go:embed swagger-ui-dist/*
var swaggerUIAssets embed.FS

var specJSON []byte

func init() {
	var doc any
	if err := yaml.Unmarshal(specYAML, &doc); err != nil {
		panic("openapi: invalid embedded yaml: " + err.Error())
	}
	b, err := json.Marshal(normalizeYAML(doc))
	if err != nil {
		panic("openapi: json marshal: " + err.Error())
	}
	specJSON = b
}

// normalizeYAML converts map[interface{}]interface{} from yaml.v2 into JSON-safe types.
func normalizeYAML(v any) any {
	switch x := v.(type) {
	case map[interface{}]interface{}:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[fmt.Sprint(k)] = normalizeYAML(val)
		}
		return out
	case map[string]any:
		for k, val := range x {
			x[k] = normalizeYAML(val)
		}
		return x
	case []interface{}:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = normalizeYAML(val)
		}
		return out
	default:
		return v
	}
}

// Register mounts public OpenAPI and Swagger UI routes on mux (no auth).
func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/openapi.json", serveJSON)
	mux.HandleFunc("GET /api/v1/openapi.yaml", serveYAML)
	mux.HandleFunc("GET /api/v1/docs", serveSwaggerUI)
	mux.HandleFunc("GET /api/v1/docs/assets/swagger-ui.css", serveSwaggerUICSS)
	mux.HandleFunc("GET /api/v1/docs/assets/swagger-ui-bundle.js", serveSwaggerUIBundleJS)
	mux.HandleFunc("GET /api/v1/docs/assets/swagger-ui-init.js", serveSwaggerUIInitJS)
}

func serveJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(specJSON)
}

func serveYAML(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(specYAML)
}

func serveSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerHTML))
}

func serveSwaggerUICSS(w http.ResponseWriter, _ *http.Request) {
	serveSwaggerUIAsset(w, "swagger-ui.css", "text/css; charset=utf-8")
}

func serveSwaggerUIBundleJS(w http.ResponseWriter, _ *http.Request) {
	serveSwaggerUIAsset(w, "swagger-ui-bundle.js", "application/javascript; charset=utf-8")
}

func serveSwaggerUIInitJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(swaggerInitJS))
}

func serveSwaggerUIAsset(w http.ResponseWriter, name, contentType string) {
	b, err := swaggerUIAssets.ReadFile("swagger-ui-dist/" + name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(b)
}

// SpecJSON returns the embedded OpenAPI document as JSON (for tests).
func SpecJSON() []byte {
	return specJSON
}

// SpecYAML returns the embedded OpenAPI document as YAML.
func SpecYAML() []byte {
	return specYAML
}

const swaggerInitJS = `window.addEventListener('DOMContentLoaded', function () {
  window.ui = SwaggerUIBundle({
    url: '/api/v1/openapi.json',
    dom_id: '#swagger-ui',
    deepLinking: true,
    persistAuthorization: true,
    presets: [SwaggerUIBundle.presets.apis],
    layout: 'BaseLayout',
  });
});
`

const swaggerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1"/>
  <title>DataSafeS3 Community API</title>
  <link rel="stylesheet" href="/api/v1/docs/assets/swagger-ui.css"/>
</head>
<body>
<div id="swagger-ui"></div>
<script src="/api/v1/docs/assets/swagger-ui-bundle.js"></script>
<script src="/api/v1/docs/assets/swagger-ui-init.js"></script>
</body>
</html>`