package domain

import (
	"strings"
	"testing"
)

// FuzzOriginPatternAcceptedEntryMatchesABrowserOrigin is the fuzzing
// counterpart to TestEveryAcceptedOriginPatternMatchesItsOwnCanonicalOrigin.
//
// That table test only catches a variant of "parser accepts an entry no
// browser Origin can ever equal" once someone has already found the variant
// and written it into the table — which is exactly how an eighth instance of
// the same bug class slipped through a seventh review round (userinfo in the
// authority, "https://user@example.com"). A fuzzer generating arbitrary
// allow-list entries and checking the same invariant does not need the
// variant enumerated in advance: any input ParseOriginPattern accepts, whose
// mechanically-derived canonical origin the pattern then fails to match, is a
// new instance of the class and fails the run — corpus-independent.
//
// Seeded from TestEveryAcceptedOriginPatternMatchesItsOwnCanonicalOrigin's and
// TestEveryAcceptedWildcardMatchesAConcreteBrowserOrigin's tables plus the
// userinfo case those tests were written before: a fuzzer never runs "cold"
// against a bug class this codebase has already paid to learn about.
//
// Without -fuzz, `go test` only replays this seed corpus (plus anything saved
// under testdata/fuzz/) — fast, deterministic, and exactly what `make verify`
// needs. `-fuzz` is what actually generates new inputs; that is an explicit,
// separate invocation (see the CORS report), never part of the default run.
func FuzzOriginPatternAcceptedEntryMatchesABrowserOrigin(f *testing.F) {
	seeds := []string{
		"https://example.com",
		"https://example.com:8443",
		"http://localhost:5173",
		"https://[::1]:8443",
		"https://[::1]",
		"HTTPS://Example.COM",
		"HtTp://LocalHost:5173",
		"https://example.com:1",
		"https://example.com:65535",
		"https://example.com:443",
		"http://example.com:80",
		"https://example.com:80",
		"http://example.com:443",
		"ftp://example.com:21",
		"  https://spaced.example.com  ",
		"https://*.example.com",
		"http://*.internal.example.org:8080",
		" https://*.spaced.example.com ",
		"https://user@example.com",
		"https://user:pass@example.com",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		p, err := ParseOriginPattern(raw)
		if err != nil {
			// Not an accepted entry — nothing to check.
			return
		}

		trimmed := strings.TrimSpace(raw)
		origin, err := ParseOrigin(trimmed)
		if err != nil {
			// Only a wildcard entry can be accepted by ParseOriginPattern while
			// being rejected by ParseOrigin itself (the "*" in the host).
			// Substitute a concrete label, exactly like the wildcard is
			// resolved at match time.
			if !strings.Contains(trimmed, "*") {
				t.Fatalf("ParseOriginPattern(%q) accepted, but ParseOrigin(%q) rejected it "+
					"with no \"*\" to explain the difference: %v", raw, trimmed, err)
			}
			concrete := strings.Replace(trimmed, "*", "sub", 1)
			origin, err = ParseOrigin(concrete)
			if err != nil {
				t.Fatalf("ParseOriginPattern(%q) accepted, but the concrete substitution "+
					"ParseOrigin(%q) still failed: %v", raw, concrete, err)
			}
		}

		// browserSent is exactly what a browser sends for this origin:
		// scheme://host[:port], nothing else — no userinfo, no path, no query,
		// no fragment. If ParseOriginPattern accepted raw, its canonical origin
		// must be something a real browser could actually send.
		browserSent := origin.Scheme + "://" + origin.Host
		if origin.Port != "" {
			browserSent += ":" + origin.Port
		}

		if !p.MatchesString(browserSent) {
			t.Fatalf("ParseOriginPattern(%q) accepted, but its own canonical origin %q is never "+
				"matched — this pattern can never be hit by a real browser request", raw, browserSent)
		}
	})
}
