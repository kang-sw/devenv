package wsdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Legacy planned markers (`🚧`) are a retired spec mechanism. The advisory below
// is a migration aid only: it names the marker-carrying spec and routes the
// reader to the resolution. No code path may fail, error, or exit non-zero
// because a marker was found.
//
// The predicate keys on the marker's syntactic *shape at line start*, not on a
// bare `🚧` contains-check. Prose that documents the mechanism (for example
// `ai-docs/spec/documentation-system.md`) embeds the literal marker shapes
// inside inline code mid-line; a contains-anywhere predicate reports those
// files as carrying markers, which is a false positive.
var (
	legacyMarkerHeadingRE = regexp.MustCompile(`^#{1,6}\s+🚧`)
	legacyMarkerCalloutRE = regexp.MustCompile(`^>\s*\[!\s*[A-Za-z]+\s*\]\s*Planned\s+🚧`)
	legacyMarkerListRE    = regexp.MustCompile(`^-\s+🚧\s`)
	legacySpecPathRE      = regexp.MustCompile(`ai-docs/spec/[A-Za-z0-9._/-]+\.md`)
)

// legacyMarker is one marker-carrying line plus the spec anchor stem that line
// declares (empty when the marker line carries no anchor).
type legacyMarker struct {
	Line   string
	Anchor string
}

// legacyMarkerLines returns the legacy planned markers in a spec document body.
// It deliberately does not call markerContext: that helper's looseness
// (`planned`/`wip` substrings anywhere in a line) still serves specs.find match
// scoring and must stay byte-identical, while this predicate must not inherit
// it.
func legacyMarkerLines(text string) []legacyMarker {
	out := []legacyMarker{}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !legacyMarkerHeadingRE.MatchString(trimmed) &&
			!legacyMarkerCalloutRE.MatchString(trimmed) &&
			!legacyMarkerListRE.MatchString(trimmed) {
			continue
		}
		marker := legacyMarker{Line: trimmed}
		if match := specAnchorRE.FindStringSubmatch(trimmed); len(match) == 2 {
			marker.Anchor = match[1]
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
type legacyMarkerResolver struct {
	tickets []legacyMarkerTicket
}

// newLegacyMarkerResolver scans live tickets (idea/todo/ready) once. Ticket read
// failures are skipped rather than propagated: the advisory is advisory.
func newLegacyMarkerResolver(root string) *legacyMarkerResolver {
	resolver := &legacyMarkerResolver{}
	tickets, err := TicketsList(root, TicketListOptions{Statuses: []string{"idea", "todo", "ready"}})
	if err != nil {
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
		collectLegacyMarkerRefs(refs, specImpactSection(root, ticket.Path))
		if len(refs) == 0 {
			continue
		}
		resolver.tickets = append(resolver.tickets, legacyMarkerTicket{
			Stem:   ticket.Stem,
			Status: ticket.Status,
			refs:   refs,
		})
	}
	sort.Slice(resolver.tickets, func(i, j int) bool {
		return resolver.tickets[i].Stem < resolver.tickets[j].Stem
	})
	return resolver
}

// collectLegacyMarkerRefs harvests exact spec file paths and spec anchor stems
// from one reference string. Both `{#YYMMDD-slug}` and bare `YYMMDD-slug` forms
// are collected; matching later requires exact equality, so a loose token
// harvest here cannot widen the match rule.
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
// section, or "" when the ticket has none or cannot be read.
func specImpactSection(root, relPath string) string {
	if strings.TrimSpace(relPath) == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relPath)))
	if err != nil {
		return ""
	}
	var b strings.Builder
	inSection := false
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			inSection = strings.HasPrefix(trimmed, "## Spec Impact")
			continue
		}
		if inSection {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// Advise returns the compat note for one spec file, or "" when that file
// carries no legacy marker. Matching is deliberately narrow: a ticket matches
// only when it names the exact spec path or one of these markers' own anchor
// stems. Sibling anchors in the same file do not match — a spec file can carry
// dozens of unrelated anchors, and file-level matching would attribute a marker
// to every ticket that touches the file.
func (r *legacyMarkerResolver) Advise(specPath string, markers []legacyMarker) string {
	if len(markers) == 0 {
		return ""
	}
	matched := []string{}
	if r != nil {
		path := filepath.ToSlash(specPath)
		for _, ticket := range r.tickets {
			if !ticket.matches(path, markers) {
				continue
			}
			matched = append(matched, fmt.Sprintf("%s [%s]", ticket.Stem, ticket.Status))
		}
	}
	sort.Strings(matched)
	if len(matched) > 0 {
		return fmt.Sprintf(
			"legacy planned marker (retired mechanism): %d marker(s); live tickets referencing this spec: %s — move the marker text into the ticket's ## Spec Impact, then strip the marker. Advisory only; this never blocks a commit.",
			len(markers), strings.Join(matched, ", "))
	}
	return fmt.Sprintf(
		"legacy planned marker (retired mechanism): %d marker(s); no live ticket references this spec — the marker is orphaned; strip it, keeping the described behavior as an ordinary implemented entry if it shipped. Advisory only; this never blocks a commit.",
		len(markers))
}

func (t legacyMarkerTicket) matches(specPath string, markers []legacyMarker) bool {
	if t.refs[specPath] {
		return true
	}
	for _, marker := range markers {
		if marker.Anchor != "" && t.refs[marker.Anchor] {
			return true
		}
	}
	return false
}
