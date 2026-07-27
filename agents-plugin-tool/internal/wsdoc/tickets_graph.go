package wsdoc

import (
	"fmt"
	"sort"
	"strings"
)

// This file holds TicketVerify's cross-file half: every other guardrail in
// tickets_verify.go is intra-file, while these checks resolve `parent:` and
// `related:` against the rest of the board. Cross-file reference resolution is
// still a pure function of the file set, so it belongs to the same
// deterministic floor — but every output here is non-blocking, because a
// commit is reversible and the consumer is an agent that can act on a returned
// remedy. Nothing in this file reads a ticket body; `parent:` is the sole
// authority for the child edge set.

const (
	// graphRowCap bounds the child rows rendered under one ancestor; the
	// hidden remainder collapses into a per-status overflow line.
	graphRowCap = 5
	// graphIntegrityCap bounds the integrity advisories per *verified ticket*,
	// mirroring the row cap — which is itself per subject (per ancestor).
	// Capping per call instead would let one ticket's advisories crowd out
	// another's entirely, and nothing in the advisory names which ticket lost
	// them.
	graphIntegrityCap = 5
	// advisoryWrapWidth is a presentation detail only; tests assert
	// substrings and line shapes, never a wrapped column.
	advisoryWrapWidth = 76
)

// Row sort order is settled: open reads ready -> todo -> idea and closed reads
// .done -> .dropped, so the fixed-width status column scans monotonically.
// ticketStatusRank is the single source of that ordering — see sortedChildren.

const actionAllChildrenClosed = "Check whether this epic can be closed. Read its `## Completion Criteria` first - \"all children closed\" is not itself the closure test."

const actionIdeaOnlyRemaining = "Every accepted child has landed; only idea/ children remain. Check whether this epic can be closed - read its `## Completion Criteria`, which may permit closure with the remaining children deferred."

// noteParentAlreadyClosed is worded path-neutrally on purpose: the same block
// renders on an ordinary todo/-path commit where nothing closed at all, so it
// must not assert when (or whether) a child closed.
const noteParentAlreadyClosed = "This parent is already closed. No action needed. If its `### Result` should mention this work, edit that Result; do not reopen the parent."

// verifiedTicket is one ticket-shaped path TicketVerify accepted, carrying the
// status directory the path sits in. The status is what gates the sibling
// listing — deliberately the path's directory rather than staged-rename
// detection, to avoid coupling with the staged-rename bug ticket.
type verifiedTicket struct {
	Path   string
	Status string
	Stem   string
}

// ticketGraph is the single whole-board load that serves both halves of the
// pass: the ancestor walk needs each ancestor's frontmatter and child set,
// which is the same input the integrity checks resolve against.
type ticketGraph struct {
	byStem      map[string]TicketInfo // one entry per stem; the most-open copy wins
	byPath      map[string]TicketInfo // every scanned file, keyed by board-relative path
	children    map[string][]string   // parent stem -> child stems
	specAnchors map[string]bool       // {#YYMMDD-slug} anchors under ai-docs/spec/
}

// verifiedInfo resolves the graph entry for a verified path. It prefers the
// exact file over byStem's most-open pick, because the integrity checks take
// the verified ticket's own frontmatter as subject: on a duplicate-stem board
// byStem may hold a different copy, whose frontmatter is not what was verified.
// byStem's most-open preference is a board-half concern and stays there.
func (g *ticketGraph) verifiedInfo(ticket verifiedTicket) (TicketInfo, bool) {
	if info, ok := g.byPath[ticket.Path]; ok {
		return info, true
	}
	info, ok := g.byStem[ticket.Stem]
	return info, ok
}

func loadTicketGraph(root string) (*ticketGraph, error) {
	tickets, err := scanTickets(root, ticketScanOptions{IncludeDone: true, IncludeDropped: true})
	if err != nil {
		return nil, err
	}
	specs, err := scanSpecs(root)
	if err != nil {
		return nil, err
	}
	graph := &ticketGraph{
		byStem:      make(map[string]TicketInfo, len(tickets)),
		byPath:      make(map[string]TicketInfo, len(tickets)),
		children:    map[string][]string{},
		specAnchors: map[string]bool{},
	}
	for _, ticket := range tickets {
		graph.byPath[ticket.Path] = ticket
	}
	for _, ticket := range tickets {
		// scanTickets is sorted by ticketStatusRank, so first-wins keeps the
		// most-open copy when the same stem exists in two status directories.
		// That direction matters: an abnormal duplicate then degrades toward
		// "still open" rather than producing a false closure nudge.
		if _, seen := graph.byStem[ticket.Stem]; seen {
			continue
		}
		graph.byStem[ticket.Stem] = ticket
	}
	for _, ticket := range tickets {
		// byStem keeps one entry per stem, so the child edge must follow it. A
		// stem present in two status directories is an abnormal board (git mv
		// is atomic), but left unguarded it duplicates the child row, inflates
		// the child count, and can fire the tier-1 closure ACTION while an open
		// copy of that child still exists.
		if graph.byStem[ticket.Stem].Path != ticket.Path {
			continue
		}
		if parent := strings.TrimSpace(ticket.Parent); parent != "" {
			graph.children[parent] = append(graph.children[parent], ticket.Stem)
		}
	}
	for _, spec := range specs {
		for _, anchor := range spec.Anchors {
			if anchor.SpecStem != "" {
				graph.specAnchors[anchor.SpecStem] = true
			}
		}
	}
	return graph, nil
}

// boardAncestor is one deduplicated ancestor entry. depth is the first
// occurrence's depth (the label a reader already saw), while gated is ORed
// across occurrences: if any verified path that reached this ancestor sits
// under .done/ or .dropped/, the sibling listing is warranted.
type boardAncestor struct {
	stem  string
	depth int
	gated bool
}

// ticketGraphAdvisories is the entry point for the cross-file pass. It returns
// the graph-load error unchanged; swallowing it is TicketVerify's job, and the
// swallow is the named degrade-to-silence path.
func ticketGraphAdvisories(root string, verified []verifiedTicket) ([]VerifyAdvisory, error) {
	if len(verified) == 0 {
		return nil, nil
	}
	graph, err := loadTicketGraph(root)
	if err != nil {
		return nil, err
	}

	verifiedStems := map[string]bool{}
	for _, ticket := range verified {
		verifiedStems[ticket.Stem] = true
	}

	var integrity []VerifyAdvisory
	var ancestors []boardAncestor
	index := map[string]int{}

	for _, ticket := range verified {
		info, ok := graph.verifiedInfo(ticket)
		if !ok {
			// The path shape passed but no ticket file backs it (a bad status
			// directory, say). The intra-file guardrails already report that.
			continue
		}
		chain, cycle := walkAncestors(graph, ticket.Stem)
		// The cap is applied per verified ticket, not across the call, so one
		// ticket's advisories can never crowd out another's.
		integrity = append(integrity, capIntegrityAdvisories(integrityAdvisories(graph, info, cycle))...)
		if len(cycle) > 0 {
			// Ancestor status is undefined on a cyclic chain, so the CHECK:
			// advisory is this ticket's whole board output.
			continue
		}
		gated := isClosedTicketStatus(ticket.Status)
		for depth, stem := range chain {
			if at, seen := index[stem]; seen {
				if gated {
					ancestors[at].gated = true
				}
				continue
			}
			index[stem] = len(ancestors)
			ancestors = append(ancestors, boardAncestor{stem: stem, depth: depth + 1, gated: gated})
		}
	}

	var out []VerifyAdvisory
	if block := renderBoardBlock(graph, ancestors, verifiedStems); block != "" {
		out = append(out, VerifyAdvisory{Kind: AdvisoryKindBoard, Text: block})
	}
	return append(out, integrity...), nil
}

// walkAncestors follows `parent:` upward at unbounded depth. On a cycle it
// returns the full revisiting path (starting at stem) and an empty chain, so
// the caller can report the cycle without having to reconstruct it. An
// unresolvable parent ends the walk; whether that end may be announced as
// "No further ancestors." is decided per ancestor by chainEndsAt.
func walkAncestors(graph *ticketGraph, stem string) (chain []string, cycle []string) {
	seen := map[string]bool{stem: true}
	path := []string{stem}
	current := stem
	for {
		info, ok := graph.byStem[current]
		if !ok {
			return chain, nil
		}
		parent := strings.TrimSpace(info.Parent)
		if parent == "" {
			return chain, nil
		}
		if seen[parent] {
			return nil, append(path, parent)
		}
		if _, ok := graph.byStem[parent]; !ok {
			return chain, nil
		}
		seen[parent] = true
		path = append(path, parent)
		chain = append(chain, parent)
		current = parent
	}
}

// chainEndsAt reports whether this ancestor genuinely terminates its chain,
// which is a property of the ancestor's own frontmatter rather than of the
// verify call. An ancestor carrying a `parent:` that does not resolve ends the
// walk without ending the chain, and because the integrity checks by design
// never inspect ancestors, nothing else in the output would mention that
// dangling edge — so the closing claim must be withheld for that chain alone.
func chainEndsAt(graph *ticketGraph, info TicketInfo) bool {
	parent := strings.TrimSpace(info.Parent)
	if parent == "" {
		return true
	}
	_, resolved := graph.byStem[parent]
	return resolved
}

// integrityAdvisories takes the verified ticket's own frontmatter as subject
// and never inspects ancestors: a dangling `related:` on an ancestor is that
// ancestor's problem, reported when a commit touches it.
func integrityAdvisories(graph *ticketGraph, info TicketInfo, cycle []string) []VerifyAdvisory {
	var out []VerifyAdvisory

	parent := strings.TrimSpace(info.Parent)
	parentInfo, parentResolved := graph.byStem[parent]
	if parent != "" && !parentResolved {
		out = append(out, VerifyAdvisory{
			Kind: AdvisoryKindFix,
			Text: wrapAdvisory("FIX:   ", fmt.Sprintf("parent: `%s` resolves to no ticket stem. Correct or remove the entry.", parent)),
		})
	}

	// `related:` resolves against ticket stems UNION spec anchor stems;
	// pointing a related: at a spec anchor is an established pattern, so a
	// ticket-only resolver would flag deliberate references. Map iteration is
	// random, so the keys are sorted for a deterministic advisory order.
	stems := make([]string, 0, len(info.Related))
	for stem := range info.Related {
		stems = append(stems, stem)
	}
	sort.Strings(stems)
	for _, stem := range stems {
		if _, ok := graph.byStem[stem]; ok {
			continue
		}
		if graph.specAnchors[stem] {
			continue
		}
		out = append(out, VerifyAdvisory{
			Kind: AdvisoryKindFix,
			Text: wrapAdvisory("FIX:   ", fmt.Sprintf("related: `%s` resolves to no ticket stem and no spec anchor. Correct or remove the entry.", stem)),
		})
	}

	// A cycle is CHECK: rather than FIX: because which edge is the wrong one is
	// exactly a judgment call, so the message reports the path and stops.
	if len(cycle) > 0 {
		quoted := make([]string, 0, len(cycle))
		for _, stem := range cycle {
			quoted = append(quoted, "`"+stem+"`")
		}
		out = append(out, VerifyAdvisory{
			Kind: AdvisoryKindCheck,
			Text: wrapAdvisory("CHECK: ", fmt.Sprintf("parent: the chain from `%s` forms a cycle: %s. Ancestor status is undefined on a cyclic chain, so no parent board is shown; decide which parent edge is wrong.", info.Stem, strings.Join(quoted, " -> "))),
		})
	}

	// `parent:` resolves against ticket stems only, so a resolved parent always
	// has a stem the category regex can read.
	if parentResolved {
		if match := ticketCategoryRE.FindStringSubmatch(parentInfo.Stem); len(match) == 2 && match[1] != "epic" {
			out = append(out, VerifyAdvisory{
				Kind: AdvisoryKindCheck,
				Text: wrapAdvisory("CHECK: ", fmt.Sprintf("parent: `%s` resolves to a ticket whose category is `%s`, not `epic`. A parent must be an epic; confirm the intended parent.", parentInfo.Stem, match[1])),
			})
		}
	}

	return out
}

// capIntegrityAdvisories keeps the overflow marker at AdvisoryKindCheck so the
// commit path never appends an amend recipe to a line that carries no remedy.
func capIntegrityAdvisories(items []VerifyAdvisory) []VerifyAdvisory {
	if len(items) <= graphIntegrityCap {
		return items
	}
	out := append([]VerifyAdvisory{}, items[:graphIntegrityCap]...)
	return append(out, VerifyAdvisory{
		Kind: AdvisoryKindCheck,
		Text: fmt.Sprintf("... +%d more", len(items)-graphIntegrityCap),
	})
}

// renderBoardBlock emits a single advisory holding every ancestor entry, or ""
// when nothing rendered — a verified ticket with no parent (most tickets)
// produces no section at all rather than an empty one.
func renderBoardBlock(graph *ticketGraph, ancestors []boardAncestor, verifiedStems map[string]bool) string {
	// closableAbove accumulates in emission order, so an epic child that
	// already rendered as a nearer ancestor carrying an ACTION line can be
	// cross-referenced from a farther ancestor's row.
	closableAbove := map[string]bool{}
	var blocks []string
	for _, ancestor := range ancestors {
		block, actioned := renderAncestor(graph, ancestor, verifiedStems, closableAbove)
		if block == "" {
			continue
		}
		if actioned {
			closableAbove[ancestor.stem] = true
		}
		blocks = append(blocks, block)
	}
	if len(blocks) == 0 {
		return ""
	}
	return "## Parent Board\n\n" + strings.Join(blocks, "\n\n")
}

// renderAncestor returns the ancestor's block and whether it carried an ACTION
// line. An empty block means this ancestor contributes nothing: either it has
// no children, or its sibling listing is suppressed because no verified path
// sits under .done/ or .dropped/.
func renderAncestor(graph *ticketGraph, ancestor boardAncestor, verifiedStems, closableAbove map[string]bool) (string, bool) {
	info := graph.byStem[ancestor.stem]
	children := sortedChildren(graph, ancestor.stem)

	var open, closed, idea, acceptedOpen []TicketInfo
	for _, child := range children {
		switch {
		case isClosedTicketStatus(child.Status):
			closed = append(closed, child)
		case child.Status == "idea":
			idea = append(idea, child)
			open = append(open, child)
		default:
			acceptedOpen = append(acceptedOpen, child)
			open = append(open, child)
		}
	}

	var header, action, note string
	var rows []TicketInfo
	siblingListing := false

	switch {
	case isClosedTicketStatus(info.Status):
		header = "parent already closed"
		note = noteParentAlreadyClosed
	case len(children) == 0:
		return "", false
	case len(open) == 0:
		header = fmt.Sprintf("all %d %s closed", len(children), childTicketNoun(len(children)))
		rows = closed
		action = actionAllChildrenClosed
	case len(acceptedOpen) == 0:
		// conventions make todo/ the accepted backlog, so an epic whose every
		// accepted child has landed is materially different from one in flight.
		header = fmt.Sprintf("%d of %d closed, %d idea/ remaining", len(closed), len(children), len(idea))
		rows = append(append([]TicketInfo{}, closed...), idea...)
		action = actionIdeaOnlyRemaining
	default:
		if !ancestor.gated {
			return "", false
		}
		header = fmt.Sprintf("%d of %d %s still open", len(open), len(children), childTicketNoun(len(children)))
		rows = open
		siblingListing = true
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Parent [%d]: %s [%s] - %s", ancestor.depth, ancestor.stem, info.Status, header)
	if note != "" {
		b.WriteString("\n\n" + wrapText("  NOTE: ", "    ", note))
	}

	shown, hidden := rows, []TicketInfo(nil)
	if len(rows) > graphRowCap {
		shown, hidden = rows[:graphRowCap], rows[graphRowCap:]
	}
	for _, child := range shown {
		b.WriteString("\n" + renderChildRow(child, verifiedStems, closableAbove))
	}
	if len(hidden) > 0 {
		b.WriteString("\n    " + overflowLine(hidden, siblingListing))
	}
	if action != "" {
		b.WriteString("\n\n" + wrapText("  ACTION: ", "    ", action))
	}
	// The closing claim rides the ancestor that terminates its own chain, so a
	// second verified ticket's truncated chain cannot suppress it here (and a
	// truncated chain cannot borrow it).
	if chainEndsAt(graph, info) {
		b.WriteString("\n\n  No further ancestors.")
	}
	return b.String(), action != ""
}

// childTicketNoun keeps the header grammatical on a single-child epic, where
// "all 1 child tickets closed" would read as a formatting bug.
func childTicketNoun(n int) string {
	if n == 1 {
		return "child ticket"
	}
	return "child tickets"
}

// renderChildRow puts status first in a fixed 8-wide column (".dropped" is the
// widest status) so the scanned field never shifts, then the variable-width
// stem. No bullet marker: the indentation already groups the rows.
//
// "(just now)" is restricted to closed rows. Every settled example attaches it
// to a .done row, where it reads as "just closed"; on an open row (a verified
// idea/ child in the idea-only tier) it would read as a closure that did not
// happen — the same false-close assertion the ancestor NOTE is worded to avoid.
func renderChildRow(child TicketInfo, verifiedStems, closableAbove map[string]bool) string {
	row := fmt.Sprintf("    %-8s| %s", child.Status, child.Stem)
	switch {
	case isClosedTicketStatus(child.Status) && verifiedStems[child.Stem]:
		row += "  (just now)"
	case isEpicTicketStem(child.Stem) && closableAbove[child.Stem]:
		row += "  (epic, closable - see above)"
	case isEpicTicketStem(child.Stem):
		row += "  (epic)"
	}
	return row
}

// overflowLine reports per-status counts rather than a single status, because
// the cap can land mid-group and the hidden rows may span several statuses.
// The counts follow the order the hidden rows themselves are in, not a global
// status ranking: the idea-only tier arranges rows closed-first, so a global
// order would contradict the rows printed directly above the line. openOnly
// drops the word "open" for the tiers where hidden rows may be closed, since
// "open" would be false there.
func overflowLine(hidden []TicketInfo, openOnly bool) string {
	counts := map[string]int{}
	var order []string
	for _, child := range hidden {
		if counts[child.Status] == 0 {
			order = append(order, child.Status)
		}
		counts[child.Status]++
	}
	parts := make([]string, 0, len(order))
	for _, status := range order {
		parts = append(parts, fmt.Sprintf("%d %s", counts[status], status))
	}
	word := ""
	if openOnly {
		word = " open"
	}
	return fmt.Sprintf("... +%d more%s (%s)", len(hidden), word, strings.Join(parts, ", "))
}

func sortedChildren(graph *ticketGraph, stem string) []TicketInfo {
	out := make([]TicketInfo, 0, len(graph.children[stem]))
	for _, child := range graph.children[stem] {
		if info, ok := graph.byStem[child]; ok {
			out = append(out, info)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Status != out[j].Status {
			return ticketStatusRank(out[i].Status) < ticketStatusRank(out[j].Status)
		}
		return out[i].Stem < out[j].Stem
	})
	return out
}

func isClosedTicketStatus(status string) bool {
	return status == ".done" || status == ".dropped"
}

func isEpicTicketStem(stem string) bool {
	match := ticketCategoryRE.FindStringSubmatch(stem)
	return len(match) == 2 && match[1] == "epic"
}

// wrapAdvisory renders the settled two-column advisory shape ("FIX:   " /
// "CHECK: " plus a hanging indent) without every call site hand-wrapping a
// format string around stems that vary from 30 to 60 characters.
func wrapAdvisory(prefix, body string) string {
	return wrapText(prefix, strings.Repeat(" ", len(prefix)), body)
}

func wrapText(prefix, continuation, body string) string {
	words := strings.Fields(body)
	if len(words) == 0 {
		return prefix
	}
	var lines []string
	current := prefix + words[0]
	for _, word := range words[1:] {
		if len(current)+1+len(word) > advisoryWrapWidth {
			lines = append(lines, current)
			current = continuation + word
			continue
		}
		current += " " + word
	}
	return strings.Join(append(lines, current), "\n")
}
