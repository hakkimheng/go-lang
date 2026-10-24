// Package docs embeds the OpenAPI contract and serves a Swagger UI for it.
//
// The spec in openapi.yaml is hand-written and is the source of truth for the
// API's shape. internal/api has a test that fails if the routes registered in
// code and the paths described here ever disagree.
package docs

import (
	"embed"
	"fmt"
	"net/http"
)

// swaggerUIVersion is pinned, and the asset tags below carry Subresource
// Integrity hashes for this exact release. Bumping the version means
// regenerating both hashes, e.g.
//
//	curl -sfL https://cdn.jsdelivr.net/npm/swagger-ui-dist@VERSION/swagger-ui.css |
//	  openssl dgst -sha384 -binary | openssl base64 -A
const (
	swaggerUIVersion = "5.33.1"
	cssIntegrity     = "sha384-Ov4/wv3j2bmct8cDc5X4ngJZohVPzEmc6uDPH8WeljUxO5vtoykvMEfbu9Vh6RaW"
	bundleIntegrity  = "sha384-ZPehFMQommnnuaZ4rpxgkgTT2DKFVp4hZC/7pLit+9Lek9T1YGSo23eHFbvNkXkw"
)

// Paths at which the handlers below expect to be mounted.
const (
	SpecPath = "/openapi.yaml"
	UIPath   = "/docs"
)

//go:embed openapi.yaml
var files embed.FS

// Spec returns the raw OpenAPI document. Tests use this to compare the
// documented paths against the routes the server actually registers.
func Spec() []byte {
	b, err := files.ReadFile("openapi.yaml")
	if err != nil {
		// Unreachable: go:embed fails at compile time if the file is missing.
		panic(fmt.Sprintf("read embedded openapi.yaml: %v", err))
	}
	return b
}

// SpecHandler serves the OpenAPI document itself.
func SpecHandler() http.HandlerFunc {
	spec := Spec()
	return func(w http.ResponseWriter, _ *http.Request) {
		// application/yaml is the registered media type as of RFC 9512.
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(spec)
	}
}

// UIHandler serves a Swagger UI page pointed at SpecPath.
func UIHandler() http.HandlerFunc {
	page := []byte(fmt.Sprintf(uiTemplate, swaggerUIVersion, cssIntegrity, swaggerUIVersion, bundleIntegrity, SpecPath))
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	}
}

// The UI assets load from a CDN rather than being vendored, which keeps the
// repository small at the cost of needing network access in the browser. See
// the README for how to vendor them for an air-gapped environment.
const uiTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>restgo API reference</title>
  <link rel="stylesheet"
        href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@%s/swagger-ui.css"
        integrity="%s"
        crossorigin="anonymous">
  <style>body { margin: 0; }</style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@%s/swagger-ui-bundle.js"
          integrity="%s"
          crossorigin="anonymous"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: %q,
      dom_id: "#swagger-ui",
      deepLinking: true,
      tryItOutEnabled: true,
      // Keeps a pasted bearer token across page reloads.
      persistAuthorization: true,
      defaultModelsExpandDepth: 1,
    });
  </script>
</body>
</html>
`
