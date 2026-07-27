package wsdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Legacy planned markers (`🚧`) belong to a spec mechanism whose retirement is in
// progress. The advisory below is a migration aid only: it names the
// marker-carrying spec and routes the reader to the resolution. No code path may
// fail, error, or exit non-zero because a marker was found.
//
// The predicate keys on the marker's syntactic *shape at line start*, not on a
// bare `🚧` contains-check. Prose that documents the mechanism (for example
// `ai-docs/spec/documentation-system.md`) embeds the literal marker shapes
// inside inline code mid-line; a contains-anywhere predicate reports those
// files as carrying markers, which is a false positive.
//
// "Line start" follows CommonMark, not a bare trim: a line inside a fenced code
// block is never a marker, and four or more leading columns of indentation make
// the line an indented code block, which is also never a marker. Both forms are
// the ordinary markdown idiom for *documenting* the marker syntax, so trimming
// first would flag exactly the documentation this predicate exists to spare.
var (
	legacyMarkerHeadingRE = regexp.MustCompile(`^#{1,6}\s+🚧`)
	legacyMarkerCalloutRE = regexp.MustCompile(`^>\s*\[!\s*[A-Za-z]+\s*\]\s*Planned\s+🚧`)
	legacyMarkerListRE    = regexp.MustCompile(`^-\s+🚧\s`)
	legacySpecPathRE      = regexp.MustCompile(`ai-docs/spec/[A-Za-z0-9._/-]+\.md`)
	// specImpactHeadingRE opens the section on `## Spec Impact` exactly, or on
	// `## Spec Impact` followed by a non-word delimiter (an anchor, a dash, a
	// parenthetical). It deliberately does not open on `## Spec Impacts` or
	// `## Spec Impact Analysis`, which are different sections.
	specImpactHeadingRE = regexp.MustCompile(`^##[ \t]+Spec Impact(?:[ \t]*[^\sA-Za-z0-9].*)?$`)
)

// maxMarkdownBlockIndent is CommonMark's limit: a block-level construct may be
// indented up to three columns. The fourth column opens an indented code block.
const maxMarkdownBlockIndent = 3

// legacyMarker is one marker-carrying line, its 1-based line number, and the
// spec anchor stems that line declares (empty when the line carries none).
type legacyMarker struct {
	Text    string
	Line    int
	Anchors []string
}

// fenceTracker follows CommonMark fenced code blocks (both ``` and ~~~) across a
// line walk. A closing fence must use the same character, be at least as long as
// the opening run, and carry no info string.
type fenceTracker struct {
	open   bool
	char   byte
	length int
}

// step advances the tracker by one line and reports whether that line is inside
// a fenced code block. The fence delimiter lines themselves count as inside.
func (f *fenceTracker) step(line string) bool {
	char, length, rest, isFence := markdownFence(line)
	if f.open {
		if isFence && char == f.char && length >= f.length && strings.TrimSpace(rest) == "" {
			f.open = false
		}
		return true
	}
	if !isFence {
		return false
	}
	// A backtick info string may not itself contain a backtick, so a line such
	// as "``` a ` b" is not a fence opener.
	if char == '`' && strings.Contains(rest, "`") {
		return false
	}
	f.open = true
	f.char = char
	f.length = length
	return true
}

// markdownFence reports whether the line is a code-fence delimiter, returning
// the fence character, its run length, and the trailing info string.
func markdownFence(line string) (byte, int, string, bool) {
	indent, body := splitMarkdownIndent(line)
	if indent > maxMarkdownBlockIndent || body == "" {
		return 0, 0, "", false
	}
	char := body[0]
	if char != '`' && char != '~' {
		return 0, 0, "", false
	}
	run := 0
	for run < len(body) && body[run] == char {
		run++
	}
	if run < 3 {
		return 0, 0, "", false
	}
	return char, run, body[run:], true
}

// splitMarkdownIndent returns the leading indent width (a tab counts as four
// columns, matching CommonMark) and the remainder of the line with leading
// whitespace and trailing whitespace removed.
func splitMarkdownIndent(line string) (int, string) {
	indent := 0
	i := 0
	for ; i < len(line); i++ {
		switch line[i] {
		case ' ':
			indent++
		case '\t':
			indent += 4
		default:
			return indent, strings.TrimRight(line[i:], " \t\r")
		}
	}
	return indent, ""
}

// legacyMarkerLines returns the legacy planned markers in a spec document body.
// It deliberately does not call markerContext: that helper's looseness
// (`planned`/`wip` substrings anywhere in a line) still serves specs.find match
// scoring and must stay byte-identical, while this predicate must not inherit
// it.
func legacyMarkerLines(text string) []legacyMarker {
	out := []legacyMarker{}
	fence := fenceTracker{}
	for i, line := range strings.Split(text, "\n") {
		if fence.step(line) {
			continue
		}
		indent, body := splitMarkdownIndent(line)
		if body == "" || indent > maxMarkdownBlockIndent {
			continue
		}
		if !legacyMarkerHeadingRE.MatchString(body) &&
			!legacyMarkerCalloutRE.MatchString(body) &&
			!legacyMarkerListRE.MatchString(body) {
			continue
		}
		marker := legacyMarker{Text: body, Line: i + 1}
		// Every anchor on the line counts: a marker heading may declare more
		// than one, and a ticket naming any of them owns the marker.
		for _, match := range specAnchorRE.FindAllStringSubmatch(body, -1) {
			marker.Anchors = append(marker.Anchors, match[1])
		}
		out = append(out, marker)
	}
	return out
}

type legacyMarkerTicket struct {
	Stem   string
	Status string
	refs   map[string]bool
}

// legacyMarkerResolver holds one live-ticket scan per tool call. Resolution runs
// ticket -> spec (which live tickets already own this spec), never spec ->
// ticket, so a marker's own text never has to be parsed for a ticket stem.
//
// incomplete records that the live-ticket scan could not be completed. It exists
// so a read failure never degrades into the orphaned verdict, whose remediation
// is "strip it": an unread ticket is an unknown owner, not an absent one.
type legacyMarkerResolver struct {
	tickets    []legacyMarkerTicket
	incomplete bool
}

// newLegacyMarkerResolver scans live tickets (idea/todo/ready) once. Read
// failures are recorded rather than propagated: the advisory is advisory.
func newLegacyMarkerResolver(root string) *legacyMarkerResolver {
	resolver := &legacyMarkerResolver{}
	tickets, err := TicketsList(root, TicketListOptions{Statuses: []string{"idea", "todo", "ready"}})
	if err != nil {
		resolver.incomplete = true
		return resolver
	}
	for _, ticket := range tickets {
		refs := map[string]bool{}
		for _, ref := range ticket.Specs {
			collectLegacyMarkerRefs(refs, ref)
		}
		for _, ref := range ticket.SpecRemoves {
			collectLegacyMarkerRefs(refs, ref)
		}
		impact, ok := specImpactSection(root, ticket.Path)
		if !ok {
			resolver.incomplete = true
		}
		collectLegacyMarkerRefs(refs, impact)
		if len(refs) == 0 {
			continue
		}
		resolver.tickets = append(resolver.tickets, legacyMarkerTicket{
			Stem:   ticket.Stem,
			Status: ticket.Status,
			refs:   refs,
		})
	}
	sort.SliceStable(resolver.tickets, func(i, j int) bool {
		return resolver.tickets[i].Stem < resolver.tickets[j].Stem
	})
	return resolver
}

// collectLegacyMarkerRefs harvests exact spec file paths and spec anchor stems
// from one reference string. Both `{#YYMMDD-slug}` and bare `YYMMDD-slug` forms
// are collected.
//
// The harvest is deliberately loose, and it does widen matching from "owns" to
// "mentions": it runs over the whole `## Spec Impact` prose, so a spec path or
// stem named only in passing matches, and ticketStemLooseRE also matches ticket
// stems. The property that holds is directional, not exact — a spurious match
// can only add a bystander ticket to the matched branch, and the matched branch
// never instructs deletion. Only the unmatched branch says "strip it", so
// erring toward matching is the safe direction.
func collectLegacyMarkerRefs(refs map[string]bool, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	for _, match := range legacySpecPathRE.FindAllString(text, -1) {
		refs[match] = true
	}
	for _, match := range ticketStemLooseRE.FindAllString(text, -1) {
		refs[match] = true
	}
}

// specImpactSection returns the body text of a ticket's `## Spec Impact`
// section, and whether the ticket could be read. A ticket with no such section
// reads successfully and yields "".
//
// Section boundaries are fence-aware: the house commit template quoted inside a
// fenced block carries `## AI Context` / `## Ticket Updates` / `## Spec` lines,
// and a fence-blind scan would close the section on them and drop every
// reference after that point.
func specImpactSection(root, relPath string) (string, bool) {
	if strings.TrimSpace(relPath) == "" {
		return "", true
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relPath)))
	if err != nil {
		return "", false
	}
	var b strings.Builder
	inSection := false
	fence := fenceTracker{}
	for _, line := range strings.Split(string(raw), "\n") {
		if fence.step(line) {
			if inSection {
				b.WriteString(line)
				b.WriteString("\n")
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			inSection = specImpactHeadingRE.MatchString(trimmed)
			continue
		}
		if inSection {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String(), true
}

// advise returns the compat note for one spec file, or "" when that file carries
// no legacy marker. Matching is deliberately narrow: a ticket matches only when
// it names the exact spec path or one of these markers' own anchor stems.
// Sibling anchors in the same file do not match — a spec file can carry dozens
// of unrelated anchors, and file-level matching would attribute a marker to
// every ticket that touches the file.
func (r *legacyMarkerResolver) advise(specPath string, markers []legacyMarker) string {
	if len(markers) == 0 {
		return ""
	}
	matched := []string{}
	incomplete := false
	if r != nil {
		incomplete = r.incomplete
		path := filepath.ToSlash(specPath)
		for _, ticket := range r.tickets {
			if !ticket.matches(path, markers) {
				continue
			}
			matched = append(matched, fmt.Sprintf("%s [%s]", ticket.Stem, ticket.Status))
		}
	}
	sort.SliceStable(matched, func(i, j int) bool { return matched[i] < matched[j] })
	prefix := fmt.Sprintf(
		"legacy planned marker (contract-first planned-entry mechanism being retired by 260726-refactor-retire-spec-planned-marker-mechanism): %d marker(s) %s",
		len(markers), legacyMarkerLineList(markers))
	if len(matched) > 0 {
		return prefix + fmt.Sprintf(
			"; live tickets referencing this spec: %s — move the marker text into the ticket's ## Spec Impact, then strip the marker. Advisory only; this never blocks a commit.",
			strings.Join(matched, ", "))
	}
	if incomplete {
		// A read failure is an unknown owner, not an absent one. Never emit the
		// "strip it" remediation off an incomplete scan.
		return prefix + "; the live ticket scan was incomplete, so marker ownership could not be determined — resolve ownership manually before removing anything. Advisory only; this never blocks a commit."
	}
	return prefix + "; no live ticket references this spec — the marker is orphaned; strip it, keeping the described behavior as an ordinary implemented entry if it shipped, or as an Implementation Gap callout if it did not. Advisory only; this never blocks a commit."
}

// legacyMarkerLineList renders the marker line numbers so the caller told to
// "strip the marker" knows where it is.
func legacyMarkerLineList(markers []legacyMarker) string {
	lines := make([]string, 0, len(markers))
	for _, marker := range markers {
		lines = append(lines, fmt.Sprintf("%d", marker.Line))
	}
	label := "at lines"
	if len(lines) == 1 {
		label = "at line"
	}
	return label + " " + strings.Join(lines, ", ")
}

func (t legacyMarkerTicket) matches(specPath string, markers []legacyMarker) bool {
	if t.refs[specPath] {
		return true
	}
	for _, marker := range markers {
		for _, anchor := range marker.Anchors {
			if anchor != "" && t.refs[anchor] {
				return true
			}
		}
	}
	return false
}
