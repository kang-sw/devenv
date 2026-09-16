package wsrationale

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// stemRE is the ticket-stem scanner, new to this tool and deliberately NOT the
// six-authoring-category form. The category segment is any lowercase word,
// because closed inventory is immutable and history names stems in retired
// categories (workset, todo, design, perf, test, idea) that commits still
// mention. See the ticket Record model note: this form recognizes ~52% of
// stem-naming commits against ~45% for the six-category form.
var stemRE = regexp.MustCompile(`\b\d{6}-[a-z]+-[a-z0-9]+(?:-[a-z0-9]+)*\b`)

// extractStems returns the distinct ticket stems named in a block of text, in
// first-seen order.
func extractStems(text string) []string {
	matches := stemRE.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

// hasGlobMeta reports whether a pattern carries any glob metacharacter. A
// pattern with none is a segment-prefix match (see matchPath); a pattern with
// any is a full segment-wise glob.
func hasGlobMeta(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

// validateGlob rejects a syntactically invalid glob so a bad pattern is an
// error, not a silent empty result. `**` is validated by collapsing it to a
// single `*` before delegating to path.Match's syntax check.
func validateGlob(pattern string) error {
	if strings.TrimSpace(pattern) == "" {
		return fmt.Errorf("path glob may not be empty")
	}
	if !hasGlobMeta(pattern) {
		return nil
	}
	probe := strings.ReplaceAll(pattern, "**", "*")
	for _, seg := range strings.Split(probe, "/") {
		if _, err := path.Match(seg, ""); err != nil {
			return fmt.Errorf("invalid path glob %q: %w", pattern, err)
		}
	}
	return nil
}

// matchPath reports whether a repo-relative path matches one glob pattern.
// A metacharacter-free pattern is a leading segment-prefix match
// (`internal/mcp` matches `internal/mcp/server.go`). A pattern with `*`/`**`
// is a whole-path segment-wise match where `*` stays within one segment and
// `**` spans zero or more segments.
func matchPath(pattern, p string) bool {
	pattern = strings.Trim(pattern, "/")
	p = strings.Trim(p, "/")
	if !hasGlobMeta(pattern) {
		patSegs := strings.Split(pattern, "/")
		pathSegs := strings.Split(p, "/")
		if len(patSegs) > len(pathSegs) {
			return false
		}
		for i, seg := range patSegs {
			if pathSegs[i] != seg {
				return false
			}
		}
		return true
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(p, "/"))
}

// matchSegments matches segment lists with `**` spanning zero or more segments.
func matchSegments(pat, seg []string) bool {
	if len(pat) == 0 {
		return len(seg) == 0
	}
	if pat[0] == "**" {
		// `**` matches zero or more path segments.
		for i := 0; i <= len(seg); i++ {
			if matchSegments(pat[1:], seg[i:]) {
				return true
			}
		}
		return false
	}
	if len(seg) == 0 {
		return false
	}
	if ok, _ := path.Match(pat[0], seg[0]); !ok {
		return false
	}
	return matchSegments(pat[1:], seg[1:])
}

// matchAnyGlob reports whether any of the given paths matches any of the
// patterns.
func matchAnyGlob(patterns, paths []string) bool {
	for _, pattern := range patterns {
		for _, p := range paths {
			if matchPath(pattern, p) {
				return true
			}
		}
	}
	return false
}
