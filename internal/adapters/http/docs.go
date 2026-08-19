package httpapi

import (
	_ "embed"
	"fmt"
	"net/http"
	"sync"
)

// The Swagger-UI assets are vendored, not pulled from a CDN: situs is a local
// service, and a documentation page that fetches its JavaScript from the network
// is blank exactly when it is needed — in the field, offline. See
// swaggerui/PROVENANCE.md for version, source and checksum.
//
// Embedded into []byte rather than an embed.FS on purpose: a single file per
// variable is resolved at compile time, so there is no read that could fail at
// runtime and no error branch that no test could ever reach.
var (
	//go:embed swaggerui/swagger-ui.css
	swaggerCSS []byte
	//go:embed swaggerui/swagger-ui-bundle.js
	swaggerBundle []byte
)

// docsPage assembles the page once — it is ~1.7 MB, so rebuilding it per request
// would be more than merely wasteful.
//
// CSS and JS are inlined rather than served as their own routes because every
// mounted route must appear in the OpenAPI spec (TestRoutesMatchOpenAPISpec), and
// asset paths have no business in an API contract. The page therefore costs one
// larger response instead of three documented routes.
var docsPage = sync.OnceValue(func() []byte {
	// The spec comes from this same server, so the page stays self-contained: no
	// external origin, and /openapi is the embedded spec of exactly this build.
	return fmt.Appendf(nil, `<!doctype html>
<html lang="de">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>situs — API</title>
<link rel="icon" href="data:image/svg+xml,%%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16'%%3E%%3Ctext y='13' font-size='13'%%3E%%F0%%9F%%8C%%B1%%3C/text%%3E%%3C/svg%%3E">
<style>%s</style>
</head>
<body>
<div id="swagger-ui"></div>
<script>%s</script>
<script>
window.onload = function () {
  SwaggerUIBundle({
    url: "/openapi",
    dom_id: "#swagger-ui",
    deepLinking: true,
    tryItOutEnabled: true,
  });
};
</script>
</body>
</html>
`, swaggerCSS, swaggerBundle)
})

// handleDocs serves the Swagger-UI page for the embedded spec.
func (s *Server) handleDocs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(docsPage()); err != nil {
		s.logger.Error("writing the documentation page", "error", err)
	}
}
