package wsdoc

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// BlockedByEdge is one parsed `blocked-by:` frontmatter entry: a typed, optional
// dependency-landing prerequisite. Phase == 0 means the whole prerequisite
// ticket must be in `.done/`; Phase > 0 means that phase of the prerequisite
// must carry a `### Result` (or the whole ticket be `.done/`). The typed edge,
// not prose, names which producer phase the landed-predicate checks.
type BlockedByEdge struct {
	Stem  string
	Phase int
	Raw   string
}

// DispatchBlock names the first `blocked-by:` prerequisite of a ticket that is
// not landed by the live code-level predicate. It is a point-in-time
// cross-ticket scheduling fact, computed live at the scheduling moment and never
// read from a content-hashed review stamp, so it is populated only on the
// single-stem point-resolve projection (not on every discovery query, which
// would be noise).
type DispatchBlock struct {
	BlockingStem string `json:"blocking_stem"`
	Reason       string `json:"reason"`
}

// blockedByEntries normalises every frontmatter shape the hand-rolled parser
// (frontmatter.go) can produce for `blocked-by:` into the raw edge-spec list
// TicketInfo advertises. The scalar and list forms are the expected shapes; the
// nested-map form is normalised by key so a hand-authored map does not resolve
// to nil. List items never pass through cleanScalar in the parser, so a trailing
// " # comment" is stripped here (a `#<phaseN>` suffix has no leading space, so
// cleanScalar leaves it intact).
func blockedByEntries(value any) []string {
	switch typed := value.(type) {
	case []string:
		out := []string{}
		for _, item := range typed {
			item = cleanScalar(item)
			if item != "" {
				out = append(out, item)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case map[string]string:
		out := []string{}
		for key := range typed {
			key = strings.TrimSpace(key)
			if key != "" {
				out = append(out, key)
			}
		}
		if len(out) == 0 {
			return nil
		}
		sort.Strings(out)
		return out
	case string:
		item := cleanScalar(typed)
		if item == "" {
			return nil
		}
		return []string{item}
	default:
		return nil
	}
}

// parseBlockedByEdge parses one raw edge spec. A bare `<stem>` yields Phase 0; a
// `<stem>#<phaseN>` suffix yields that phase number. The suffix accepts an
// optional `phase` word (`#2` and `#phase2` are equivalent) so the author is not
// forced to remember one spelling. A spec whose stem is not a ticket stem, or
// whose phase suffix is not a positive integer, does not parse — the caller
// treats that as a block rather than silently ignoring a malformed prerequisite.
func parseBlockedByEdge(spec string) (BlockedByEdge, bool) {
	raw := strings.TrimSpace(spec)
	if raw == "" {
		return BlockedByEdge{}, false
	}
	stem := raw
	phase := 0
	if idx := strings.Index(raw, "#"); idx >= 0 {
		stem = strings.TrimSpace(raw[:idx])
		suffix := strings.TrimSpace(raw[idx+1:])
		suffix = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(suffix), "phase"))
		n, err := strconv.Atoi(suffix)
		if err != nil || n < 1 {
			return BlockedByEdge{}, false
		}
		phase = n
	}
	if !ticketStemRE.MatchString(stem) {
		return BlockedByEdge{}, false
	}
	return BlockedByEdge{Stem: stem, Phase: phase, Raw: raw}, true
}

// phaseResultPresent reports whether a ticket has phase n and whether that phase
// carries a `### Result`. Phase numbers are read from the `### Phase N:` heading
// text (TicketPhase.Heading), so `blocked-by: <stem>#<N>` targets the producer's
// declared phase N rather than its position in the slice.
func phaseResultPresent(phases []TicketPhase, n int) (found, resultPresent bool) {
	for _, phase := range phases {
		if num, ok := phaseNumberOf(phase.Heading); ok && num == n {
			return true, phase.ResultPresent
		}
	}
	return false, false
}

// phaseNumberOf reads the leading integer from a `Phase N: ...` heading (the
// form TicketPhase.Heading carries after `### ` is stripped in ticketPhases).
func phaseNumberOf(heading string) (int, bool) {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(heading), "Phase "))
	i := 0
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(rest[:i])
	if err != nil {
		return 0, false
	}
	return n, true
}

// edgeLanded evaluates the code-level landed-predicate for one edge against its
// resolved prerequisite. A prerequisite counts as landed when it is in `.done/`,
// or (for a phase-targeted edge) when the named phase carries a `### Result`.
// Anything else — including a prerequisite still in ready/ but unexecuted — is
// not landed, which is what routes the phase-granular interleave to the dispatch
// gate rather than the promotion closure.
func edgeLanded(edge BlockedByEdge, prereq TicketInfo, found bool) (bool, string) {
	if !found {
		return false, fmt.Sprintf("blocked-by prerequisite %s resolves to no ticket on the board", edge.Stem)
	}
	if prereq.Status == ".done" {
		return true, ""
	}
	if edge.Phase == 0 {
		return false, fmt.Sprintf("blocked-by prerequisite %s is in %s/, not .done/", edge.Stem, prereq.Status)
	}
	present, result := phaseResultPresent(prereq.Phases, edge.Phase)
	if !present {
		return false, fmt.Sprintf("blocked-by prerequisite %s#phase%d names a phase that does not exist", edge.Stem, edge.Phase)
	}
	if !result {
		return false, fmt.Sprintf("blocked-by prerequisite %s phase %d carries no ### Result yet", edge.Stem, edge.Phase)
	}
	return true, ""
}

// DispatchBlockFor computes the dispatch-time hard gate for a consumer ticket:
// the first `blocked-by:` prerequisite not landed by the live code-level
// predicate, or nil when nothing blocks. Absent a `blocked-by:` edge there is no
// hard block — exactly today's behaviour. The whole board (including `.done/`
// and `.dropped/`, and index-hidden entries under a sparse-checkout scope) is
// scanned live so the predicate never goes stale.
func DispatchBlockFor(root string, info TicketInfo) (*DispatchBlock, error) {
	if len(info.BlockedBy) == 0 {
		return nil, nil
	}
	tickets, err := scanTickets(root, ticketScanOptions{
		IncludeDone:    true,
		IncludeDropped: true,
		Resolve:        resolveFull,
	})
	if err != nil {
		return nil, err
	}
	// scanTickets is sorted by ticketStatusRank, so first-wins keeps the
	// most-open copy of a duplicate stem — the conservative pick, since a less
	// landed prerequisite blocks rather than clears.
	byStem := make(map[string]TicketInfo, len(tickets))
	for _, ticket := range tickets {
		if _, seen := byStem[ticket.Stem]; seen {
			continue
		}
		byStem[ticket.Stem] = ticket
	}
	for _, spec := range info.BlockedBy {
		edge, ok := parseBlockedByEdge(spec)
		if !ok {
			return &DispatchBlock{
				BlockingStem: strings.TrimSpace(spec),
				Reason:       fmt.Sprintf("malformed blocked-by entry %q; expected <stem> or <stem>#<phaseN>", spec),
			}, nil
		}
		prereq, found := byStem[edge.Stem]
		if landed, reason := edgeLanded(edge, prereq, found); !landed {
			return &DispatchBlock{BlockingStem: edge.Stem, Reason: reason}, nil
		}
	}
	return nil, nil
}

// blockedByPromotionError is the promotion-closure half of the gate, bound to
// tickets.move(to: "ready"). It enforces only the tool-visible subset: a typed
// prerequisite must already be in ready/ or .done/ before the consumer lands in
// ready/. It is deliberately status-only — the phase-granular `### Result`
// predicate is the dispatch gate's job, and the "or the same batch" case stays
// with the promotion playbook, which moves prerequisites first. It never reads a
// review stamp.
func blockedByPromotionError(root string, scope *ticketScope, absTicketPath string) error {
	edges := blockedByEntries(frontmatter(absTicketPath)["blocked-by"])
	for _, spec := range edges {
		edge, ok := parseBlockedByEdge(spec)
		if !ok {
			return fmt.Errorf("malformed blocked-by entry %q; expected <stem> or <stem>#<phaseN>", spec)
		}
		_, status, _, err := findTicketPath(root, scope, edge.Stem)
		if err != nil {
			return fmt.Errorf("blocked-by prerequisite %s resolves to no ticket; promote or correct it before landing this ticket in ready/", edge.Stem)
		}
		if status != "ready" && status != ".done" {
			return fmt.Errorf("blocked-by prerequisite %s is in %s/, not ready/ or .done/; promote the prerequisite before landing this ticket in ready/", edge.Stem, status)
		}
	}
	return nil
}
