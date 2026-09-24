package httpapi

import (
	_ "embed"
	"html"
	"net/http"
	"strings"
)

// The explorer page is embedded, not read from disk: situs ships as a single
// binary, and a page that needs a file next to the executable is a page that
// is missing wherever the binary was copied to.
//
// Embedded as []byte rather than an embed.FS, for the same reason docs.go
// does it: a single file resolved at compile time has no read that could
// fail at runtime, and therefore no error branch no test could reach.
//
// explorerVersionSentinel is the placeholder in explorer.html that the build
// version is substituted for. An HTML comment, so the raw asset stays a valid
// document a browser or an editor can still open.
const explorerVersionSentinel = "<!--situs:version-->"

//go:embed explorer.html
var explorerAsset []byte

// handleExplorer serves the API explorer at the root.
//
// It lives at "/" because a locally started service whose bare address shows
// something usable needs no explanation. gorilla/mux matches this exactly,
// not as a prefix, so unknown paths stay 404 instead of silently returning
// the page.
func (s *Server) handleExplorer(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(s.explorerPage); err != nil {
		s.logger.Error("writing the explorer page", "error", err)
	}
}

// renderExplorer substitutes the build version into the page once, at server
// construction. Not per request: the document is immutable for a given build,
// and not from /v1/info either — a footer that has to ask the index which
// version it is stays empty exactly when the index is the broken part.
//
// The version is escaped as HTML text. In practice it comes from the linker or
// the Docker VERSION build argument, so nothing untrusted reaches it — but
// Options.Version is an exported knob, and the page should not rely on every
// future caller passing something markup-free.
func renderExplorer(version string) []byte {
	return []byte(strings.Replace(string(explorerAsset), explorerVersionSentinel, html.EscapeString(version), 1))
}
