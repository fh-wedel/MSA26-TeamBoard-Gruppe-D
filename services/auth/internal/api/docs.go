package api

import (
	_ "embed"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed openapi.yaml
var openAPISpec []byte

// mountDocs registers the public API documentation routes: the raw OpenAPI
// spec and a Swagger UI shell that renders it. Paths are namespaced under
// /auth so the existing Traefik router can forward them without a prefix
// collision with the other services. No auth — the docs are public.
func mountDocs(r chi.Router) {
	r.Get("/auth/openapi.yaml", serveOpenAPISpec)
	r.Get("/auth/docs", serveDocsUI("TeamBoard Auth API", "/auth/openapi.yaml"))
}

func serveOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openAPISpec)
}

func serveDocsUI(title, specURL string) http.HandlerFunc {
	page := swaggerUIPage(title, specURL)
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(page)
	}
}

// swaggerUIPage builds a self-contained Swagger UI page that loads the spec
// from specURL. The UI assets are pulled from the unpkg CDN, so viewing the
// docs requires internet access at view time.
func swaggerUIPage(title, specURL string) []byte {
	return []byte(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1"/>
  <title>` + title + `</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"/>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js" crossorigin></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: '` + specURL + `',
      dom_id: '#swagger-ui',
      deepLinking: true,
      presets: [SwaggerUIBundle.presets.apis]
    });
  </script>
</body>
</html>`)
}
