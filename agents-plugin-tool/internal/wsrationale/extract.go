package wsrationale

import (
	"regexp"
	"strings"
)

// wsRun collapses every run of whitespace (including newlines) to a single
// space and trims the ends, so a multi-line record becomes one clean line for
// both matching and display.
var wsRun = regexp.MustCompile(`\s+`)

func collapse(text string) string {
	return strings.TrimSpace(wsRun.ReplaceAllString(text, " "))
}

// section pairs the actual heading line with the block of lines under it (up to
// the next heading at the same-or-shallower level, or end of body).
type section struct {
	heading string
	lines   []string
}

// collectSections returns every section whose heading line matches `match`,
// from just after the heading up to the next heading at the same-or-shallower
// level than `headingPrefix`. A body may repeat `### Result` once per phase, so
// every occurrence is returned.
func collectSections(body string, match func(trimmed string) bool, headingPrefix string) []section {
	lines := strings.Split(body, "\n")
	var out []section
	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if !match(trimmed) {
			i++
			continue
		}
		heading := trimmed
		i++
		start := i
		for i < len(lines) {
			if isBoundaryHeading(strings.TrimSpace(lines[i]), headingPrefix) {
				break
			}
			i++
		}
		out = append(out, section{heading: heading, lines: lines[start:i]})
	}
	return out
}

// exactOrPrefixHeading matches a heading that equals `heading` or begins with
// `heading` followed by a space, so `## Resolution` also matches the dated
// `## Resolution (2026-03-03)` heading tickets.close writes.
func exactOrPrefixHeading(heading string) func(string) bool {
	return func(trimmed string) bool {
		return trimmed == heading || strings.HasPrefix(trimmed, heading+" ")
	}
}

// isBoundaryHeading reports whether a line ends the current section. For a `## `
// block, any `## ` (or shallower `# `) heading ends it. For a `### ` block, any
// heading at level 3 or shallower ends it.
func isBoundaryHeading(trimmed, headingPrefix string) bool {
	if !strings.HasPrefix(trimmed, "#") {
		return false
	}
	// Count the heading level.
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || (level < len(trimmed) && trimmed[level] != ' ') {
		return false
	}
	boundaryLevel := strings.Count(headingPrefix, "#")
	return level <= boundaryLevel
}

// prefixHeadings returns, for the exact-heading matcher, the heading prefixes we
// scan: `## ` for level-2 sections and `### ` for level-3 sections. The caller
// passes the concrete heading text and its prefix.

// numberedItemRe matches a top-level ordered-list marker ("1. ", "21. ", ...).
// Ticket sections such as `## Cross-Child Decisions` use ordered lists with no
// blank line between items; without this, the whole list falls through to
// splitParagraphs as one blank-line-separated paragraph and every item beyond
// the first collapses into a single oversized record (Phase 3 finding on
// 260909-epic-ws-worker-interpreter-refoundation's Cross-Child Decision 16).
var numberedItemRe = regexp.MustCompile(`^\d+\.\s`)

// splitRecords applies the bullet-or-paragraph rule to a block of lines.
//
// A record starts at a line beginning `- ` or a top-level ordered-list marker
// (at column 0) and continues through following lines that start with
// whitespace or `>`; a blank line or the next bullet/ordered-list marker ends
// it. A block with no top-level bullet or ordered-list marker yields one
// record per blank-line-separated paragraph.
func splitRecords(lines []string) []string {
	if blockHasBullet(lines) {
		return splitBullets(lines)
	}
	return splitParagraphs(lines)
}

func blockHasBullet(lines []string) bool {
	for _, line := range lines {
		if isBulletStart(line) {
			return true
		}
	}
	return false
}

// isBulletStart reports whether line opens a new top-level record: a `- `
// bullet or an ordered-list marker.
func isBulletStart(line string) bool {
	if strings.HasPrefix(line, "- ") {
		return true
	}
	return numberedItemRe.MatchString(line)
}

// stripBulletMarker removes the leading `- ` or ordered-list marker from a
// line already confirmed by isBulletStart.
func stripBulletMarker(line string) string {
	if strings.HasPrefix(line, "- ") {
		return strings.TrimPrefix(line, "- ")
	}
	if loc := numberedItemRe.FindStringIndex(line); loc != nil {
		return line[loc[1]:]
	}
	return line
}

func splitBullets(lines []string) []string {
	var records []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			if text := collapse(strings.Join(cur, " ")); text != "" {
				records = append(records, text)
			}
			cur = nil
		}
	}
	for _, line := range lines {
		switch {
		case isBulletStart(line):
			flush()
			cur = []string{stripBulletMarker(line)}
		case strings.TrimSpace(line) == "":
			flush()
		case isContinuation(line):
			if len(cur) > 0 {
				cur = append(cur, strings.TrimSpace(line))
			}
		default:
			// A non-continuation, non-bullet line ends the current bullet and is
			// not itself a record (bullets carry the rationale here).
			flush()
		}
	}
	flush()
	return records
}

func isContinuation(line string) bool {
	if line == "" {
		return false
	}
	if line[0] == ' ' || line[0] == '\t' || line[0] == '>' {
		return true
	}
	return false
}

func splitParagraphs(lines []string) []string {
	var records []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			if text := collapse(strings.Join(cur, " ")); text != "" {
				records = append(records, text)
			}
			cur = nil
		}
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return records
}
