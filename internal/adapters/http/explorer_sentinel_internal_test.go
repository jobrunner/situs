package httpapi

import (
	"strings"
	"testing"
)

// The substitution is silent by nature: strings.Replace on a sentinel that is
// not there returns the document unchanged and reports nothing. hostus shipped
// exactly that bug once — the page rendered, every route test stayed green,
// and the footer showed the raw placeholder. This test watches the asset, not
// the output, so renaming the sentinel in one of the two places fails here.
func TestExplorerAsset_TraegtDenVersionsPlatzhalterGenauEinmal(t *testing.T) {
	if n := strings.Count(string(explorerAsset), explorerVersionSentinel); n != 1 {
		t.Errorf("das eingebettete explorer.html enthaelt %q %d-mal, erwartet genau einmal", explorerVersionSentinel, n)
	}
}
