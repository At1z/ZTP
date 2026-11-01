package handler

import (
	_ "embed"
	"net/http"
)

// openAPISpec is the OpenAPI description of the API, served as-is.
//
//go:embed openapi.yaml
var openAPISpec []byte

// swaggerUIPage renders the spec with Swagger UI loaded from a CDN.
const swaggerUIPage = `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <title>Notification system API</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui-bundle.js"></script>
  <script>SwaggerUIBundle({url: "/openapi.yaml", dom_id: "#swagger-ui"});</script>
</body>
</html>`

func openAPIHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(openAPISpec)
}

func docsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerUIPage))
}
