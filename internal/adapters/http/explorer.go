package httpapi

import (
	_ "embed"
	"net/http"
)

// The explorer page is embedded, not read from disk: situs ships as a single
// binary, and a page that needs a file next to the executable is a page that
// is missing wherever the binary was copied to.
//
// Embedded as []byte rather than an embed.FS, for the same reason docs.go
// does it: a single file resolved at compile time has no read that could
// fail at runtime, and therefore no error branch no test could reach.
//
//go:embed explorer.html
var explorerPage []byte

// handleExplorer serves the API explorer at the root.
//
// It lives at "/" because a locally started service whose bare address shows
// something usable needs no explanation. gorilla/mux matches this exactly,
// not as a prefix, so unknown paths stay 404 instead of silently returning
// the page.
func (s *Server) handleExplorer(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(explorerPage); err != nil {
		s.logger.Error("writing the explorer page", "error", err)
	}
}
