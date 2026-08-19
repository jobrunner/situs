package httpapi

import (
	"embed"
	"net/http"
)

// The spec lives next to the code and is compiled into the binary, so GET
// /openapi can never drift from the deployed build. It is served as YAML —
// exactly the embedded bytes — which keeps a YAML library out of the module.
//
// A byte-identical copy lives at api/openapi/openapi.yaml for external tooling
// (oasdiff, codegen); TestOpenAPICopiesAreIdentical enforces that.
//
//go:embed openapi.yaml
var openAPIFS embed.FS

// specFileName is the embedded spec, and the basename of its mirrored copy.
const specFileName = "openapi.yaml"

// openAPISpec returns the embedded spec bytes.
func openAPISpec() ([]byte, error) { return openAPIFS.ReadFile(specFileName) }

// handleOpenAPI serves the embedded spec.
func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	body, err := openAPISpec()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, CodeInternalError, "failed to load the OpenAPI specification")
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	if _, err := w.Write(body); err != nil {
		s.logger.Error("writing the OpenAPI specification", "error", err)
	}
}
