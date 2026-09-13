package wsdoc

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestParseBlockedByEdge pins the typed-edge value shape: a bare stem is a
// whole-ticket prerequisite (Phase 0), a `#<phaseN>` suffix names a producer
// phase (with an optional `phase` word), and a spec that is not a ticket stem or
// carries a non-positive phase does not parse.
func TestParseBlockedByEdge(t *testing.T) {
	for _, tc := range []struct {
		spec      string
		wantOK    bool
		wantStem  string
		wantPhase int
	}{
		{spec: "260101-feat-a", wantOK: true, wantStem: "260101-feat-a", wantPhase: 0},
		{spec: "260101-feat-a#2", wantOK: true, wantStem: "260101-feat-a", wantPhase: 2},
		{spec: "260101-feat-a#phase3", wantOK: true, wantStem: "260101-feat-a", wantPhase: 3},
		{spec: "  260101-feat-a#2  ", wantOK: true, wantStem: "260101-feat-a", wantPhase: 2},
		{spec: "not-a-stem", wantOK: false},
		{spec: "260101-feat-a#0", wantOK: false},
		{spec: "260101-feat-a#-1", wantOK: false},
		{spec: "260101-feat-a#x", wantOK: false},
		{spec: "", wantOK: false},
	} {
		t.Run(tc.spec, func(t *testing.T) {
			edge, ok := parseBlockedByEdge(tc.spec)
			if ok != tc.wantOK {
				t.Fatalf("parse(%q) ok = %v, want %v", tc.spec, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if edge.Stem != tc.wantStem || edge.Phase != tc.wantPhase {
				t.Fatalf("parse(%q) = {%q, %d}, want {%q, %d}", tc.spec, edge.Stem, edge.Phase, tc.wantStem, tc.wantPhase)
			}
		})
	}
}

// TestBlockedByEntriesShapes pins that the raw edge list is recovered from every
// frontmatter shape the parser produces (scalar, list, map keys), and that a
// trailing comment on a list item is stripped while a `#<phaseN>` suffix — which
// has no leading space — survives.
func TestBlockedByEntriesShapes(t *testing.T) {
	scalar := blockedByEntries("260101-feat-a")
	if len(scalar) != 1 || scalar[0] != "260101-feat-a" {
		t.Fatalf("scalar = %v", scalar)
	}
	list := blockedByEntries([]string{"260101-feat-a#2", "260101-feat-b # note", ""})
	if len(list) != 2 || list[0] != "260101-feat-a#2" || list[1] != "260101-feat-b" {
		t.Fatalf("list = %v", list)
	}
	mapForm := blockedByEntries(map[string]string{"260101-feat-b": "", "260101-feat-a": ""})
	if len(mapForm) != 2 || mapForm[0] != "260101-feat-a" || mapForm[1] != "260101-feat-b" {
		t.Fatalf("map form (sorted) = %v", mapForm)
	}
	if blockedByEntries(map[string]string{}) != nil {
		t.Fatal("empty map should normalise to nil (absent edge)")
	}
}

func prereqWithPhases(title string, phase1Result, phase2Result bool) string {
	body := "---\ntitle: " + title + "\n---\n\n# " + title + "\n\n## Phases\n\n### Phase 1: A\n"
	if phase1Result {
		body += "\n### Result (abc1234) - 2026-01-01\n\nlanded\n"
	}
	body += "\n### Phase 2: B\n"
	if phase2Result {
		body += "\n### Result (def5678) - 2026-01-01\n\nlanded\n"
	}
	return body
}

// TestDispatchBlockForPredicate pins the dispatch-time hard gate: the code-level
// landed-predicate, computed live, over the whole board. A .done prerequisite
// clears; a bare-stem edge to a ready-but-unexecuted prerequisite blocks; a
// phase-targeted edge clears exactly when the named producer phase carries a
// ### Result (the phase-granular interleave); a missing or malformed edge
// blocks; and a ticket with no blocked-by edge is never gated (soft related: is
// not consulted).
func TestDispatchBlockForPredicate(t *testing.T) {
	for _, tc := range []struct {
		name        string
		consumerFM  string
		prereqs     map[string]string // stem -> "status|body"
		wantBlocked bool
		wantStem    string
		reasonHas   string
	}{
		{
			name:        "no blocked-by is never gated",
			consumerFM:  "related:\n  260101-feat-a: partner\n",
			prereqs:     map[string]string{"260101-feat-a": "ready|" + prereqWithPhases("A", false, false)},
			wantBlocked: false,
		},
		{
			name:        "bare stem, prerequisite done, clears",
			consumerFM:  "blocked-by: 260101-feat-a\n",
			prereqs:     map[string]string{"260101-feat-a": ".done|" + prereqWithPhases("A", true, true)},
			wantBlocked: false,
		},
		{
			name:        "bare stem, prerequisite in ready, blocks",
			consumerFM:  "blocked-by: 260101-feat-a\n",
			prereqs:     map[string]string{"260101-feat-a": "ready|" + prereqWithPhases("A", true, true)},
			wantBlocked: true,
			wantStem:    "260101-feat-a",
			reasonHas:   "not .done/",
		},
		{
			name:        "phase edge clears when the named phase has a Result",
			consumerFM:  "blocked-by: 260101-feat-a#2\n",
			prereqs:     map[string]string{"260101-feat-a": "ready|" + prereqWithPhases("A", false, true)},
			wantBlocked: false,
		},
		{
			name:        "phase edge blocks when the named phase has no Result",
			consumerFM:  "blocked-by: 260101-feat-a#2\n",
			prereqs:     map[string]string{"260101-feat-a": "ready|" + prereqWithPhases("A", true, false)},
			wantBlocked: true,
			wantStem:    "260101-feat-a",
			reasonHas:   "phase 2 carries no ### Result",
		},
		{
			name:        "prerequisite absent blocks",
			consumerFM:  "blocked-by: 260101-feat-absent\n",
			prereqs:     map[string]string{},
			wantBlocked: true,
			wantStem:    "260101-feat-absent",
			reasonHas:   "resolves to no ticket",
		},
		{
			name:        "malformed edge blocks",
			consumerFM:  "blocked-by: not-a-stem\n",
			prereqs:     map[string]string{},
			wantBlocked: true,
			wantStem:    "not-a-stem",
			reasonHas:   "malformed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for stem, spec := range tc.prereqs {
				parts := strings.SplitN(spec, "|", 2)
				status, body := parts[0], parts[1]
				mustWrite(t, root, filepath.Join("ai-docs", "tickets", statusDirs[status], stem+".md"), body)
			}
			consumerRel := "ai-docs/tickets/ready/260101-feat-consumer.md"
			mustWrite(t, root, filepath.FromSlash(consumerRel),
				"---\ntitle: Consumer\n"+tc.consumerFM+"---\n\n# Consumer\n\n## Phases\n\n### Phase 1: X\n")
			info, err := TicketAt(root, consumerRel)
			if err != nil {
				t.Fatalf("TicketAt: %v", err)
			}
			block, err := DispatchBlockFor(root, info)
			if err != nil {
				t.Fatalf("DispatchBlockFor: %v", err)
			}
			if tc.wantBlocked {
				if block == nil {
					t.Fatalf("want dispatch block on %q, got nil", tc.name)
				}
				if block.BlockingStem != tc.wantStem {
					t.Fatalf("blocking stem = %q, want %q", block.BlockingStem, tc.wantStem)
				}
				if tc.reasonHas != "" && !strings.Contains(block.Reason, tc.reasonHas) {
					t.Fatalf("reason = %q, want it to contain %q", block.Reason, tc.reasonHas)
				}
			} else if block != nil {
				t.Fatalf("want no dispatch block, got %+v", block)
			}
		})
	}
}

// TestDispatchBlockForIgnoresSageStamp pins the decisive design property: the
// landed-predicate is computed live from directory state and phase Results, and
// is never read from a content-hashed sage-review stamp. A prerequisite still in
// ready/ that carries completed sage stamps (and a design-reviewed hash) must
// still block — the stamp says the ticket's text passed review, which is not the
// same as the ticket having landed — and a .done/ prerequisite clears even with
// a blocked sage stamp. If the gate ever started consulting the stamp, one of
// these flips.
func TestDispatchBlockForIgnoresSageStamp(t *testing.T) {
	root := t.TempDir()
	// Unlanded (ready/) prerequisite whose sage stamps all read completed.
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "ready", "260101-feat-stamped.md"),
		"---\ntitle: Stamped\nsage-review-design: completed\nsage-review-completeness: completed\nsage-review-design-reviewed: deadbeefdeadbeef\nsage-review-completeness-reviewed: deadbeefdeadbeef\n---\n\n# Stamped\n\n## Phases\n\n### Phase 1: A\n")
	consumerRel := "ai-docs/tickets/ready/260101-feat-consumer.md"
	mustWrite(t, root, filepath.FromSlash(consumerRel),
		"---\ntitle: Consumer\nblocked-by: 260101-feat-stamped\n---\n\n# Consumer\n\n## Phases\n\n### Phase 1: X\n")
	info, err := TicketAt(root, consumerRel)
	if err != nil {
		t.Fatalf("TicketAt: %v", err)
	}
	block, err := DispatchBlockFor(root, info)
	if err != nil {
		t.Fatalf("DispatchBlockFor: %v", err)
	}
	if block == nil || block.BlockingStem != "260101-feat-stamped" {
		t.Fatalf("a completed sage stamp on a ready/ prerequisite cleared the gate: %+v", block)
	}

	// A .done/ prerequisite clears even when its sage stamp reads blocked: the
	// predicate keys on directory state, not the stamp.
	root2 := t.TempDir()
	mustWrite(t, root2, filepath.Join("ai-docs", "tickets", ".done", "260101-feat-stamped.md"),
		"---\ntitle: Stamped\nsage-review-design: blocked\n---\n\n# Stamped\n")
	mustWrite(t, root2, filepath.FromSlash(consumerRel),
		"---\ntitle: Consumer\nblocked-by: 260101-feat-stamped\n---\n\n# Consumer\n\n## Phases\n\n### Phase 1: X\n")
	info2, err := TicketAt(root2, consumerRel)
	if err != nil {
		t.Fatalf("TicketAt: %v", err)
	}
	block2, err := DispatchBlockFor(root2, info2)
	if err != nil {
		t.Fatalf("DispatchBlockFor: %v", err)
	}
	if block2 != nil {
		t.Fatalf("a .done/ prerequisite with a blocked sage stamp was gated: %+v", block2)
	}
}

// TestDispatchBlockForMultipleEdges pins that the gate iterates every blocked-by
// edge and reports the actual unlanded one, rather than only checking the first
// edge or short-circuiting on the first landed edge. The unlanded prerequisite
// is placed second so a first-edge-only bug would wrongly clear.
func TestDispatchBlockForMultipleEdges(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", ".done", "260101-feat-landed.md"),
		"---\ntitle: Landed\n---\n\n# Landed\n")
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "ready", "260101-feat-pending.md"),
		"---\ntitle: Pending\n---\n\n# Pending\n\n## Phases\n\n### Phase 1: A\n")
	consumerRel := "ai-docs/tickets/ready/260101-feat-consumer.md"
	mustWrite(t, root, filepath.FromSlash(consumerRel),
		"---\ntitle: Consumer\nblocked-by:\n  - 260101-feat-landed\n  - 260101-feat-pending\n---\n\n# Consumer\n\n## Phases\n\n### Phase 1: X\n")
	info, err := TicketAt(root, consumerRel)
	if err != nil {
		t.Fatalf("TicketAt: %v", err)
	}
	if len(info.BlockedBy) != 2 {
		t.Fatalf("consumer should carry two blocked-by edges, got %v", info.BlockedBy)
	}
	block, err := DispatchBlockFor(root, info)
	if err != nil {
		t.Fatalf("DispatchBlockFor: %v", err)
	}
	if block == nil || block.BlockingStem != "260101-feat-pending" {
		t.Fatalf("multi-edge gate should report the unlanded second edge, got %+v", block)
	}
}

// TestBlockedByPromotionWarning pins the promotion-time advisory warning: the
// soft, non-blocking layer over the in-between the closure allows. A typed
// prerequisite in ready/ but not code-landed warns; a .done/ prerequisite, a
// phase-targeted prerequisite whose named phase carries a ### Result, a soft
// related: edge, and an absent blocked-by all stay silent. Computed live from
// board state, never from a stamp.
func TestBlockedByPromotionWarning(t *testing.T) {
	for _, tc := range []struct {
		name       string
		consumerFM string
		prereqs    map[string]string // stem -> "status|body"
		wantWarn   bool
		mentions   []string
	}{
		{
			name:       "ready-but-unexecuted bare stem warns",
			consumerFM: "blocked-by: 260101-feat-a\n",
			prereqs:    map[string]string{"260101-feat-a": "ready|" + prereqWithPhases("A", false, false)},
			wantWarn:   true,
			mentions:   []string{"260101-feat-a"},
		},
		{
			name:       "done prerequisite is silent",
			consumerFM: "blocked-by: 260101-feat-a\n",
			prereqs:    map[string]string{"260101-feat-a": ".done|" + prereqWithPhases("A", true, true)},
			wantWarn:   false,
		},
		{
			name:       "phase edge with Result on the named phase is silent",
			consumerFM: "blocked-by: 260101-feat-a#2\n",
			prereqs:    map[string]string{"260101-feat-a": "ready|" + prereqWithPhases("A", false, true)},
			wantWarn:   false,
		},
		{
			name:       "phase edge without Result on the named phase warns",
			consumerFM: "blocked-by: 260101-feat-a#2\n",
			prereqs:    map[string]string{"260101-feat-a": "ready|" + prereqWithPhases("A", true, false)},
			wantWarn:   true,
			mentions:   []string{"260101-feat-a#2"},
		},
		{
			name:       "multi-edge names every pending prerequisite (join path)",
			consumerFM: "blocked-by:\n  - 260101-feat-a\n  - 260101-feat-b\n",
			prereqs: map[string]string{
				"260101-feat-a": "ready|" + prereqWithPhases("A", false, false),
				"260101-feat-b": "ready|" + prereqWithPhases("B", false, false),
			},
			wantWarn: true,
			mentions: []string{"260101-feat-a", "260101-feat-b"},
		},
		{
			name:       "malformed edge stays silent here (closure/dispatch gate owns it)",
			consumerFM: "blocked-by: not-a-stem\n",
			prereqs:    map[string]string{},
			wantWarn:   false,
		},
		{
			name:       "soft related edge never warns",
			consumerFM: "related:\n  260101-feat-a: partner\n",
			prereqs:    map[string]string{"260101-feat-a": "ready|" + prereqWithPhases("A", false, false)},
			wantWarn:   false,
		},
		{
			name:       "absent prerequisite is silent (closure/dispatch gate owns it)",
			consumerFM: "blocked-by: 260101-feat-absent\n",
			prereqs:    map[string]string{},
			wantWarn:   false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for stem, spec := range tc.prereqs {
				parts := strings.SplitN(spec, "|", 2)
				status, body := parts[0], parts[1]
				mustWrite(t, root, filepath.Join("ai-docs", "tickets", statusDirs[status], stem+".md"), body)
			}
			consumerRel := "ai-docs/tickets/ready/260101-feat-consumer.md"
			mustWrite(t, root, filepath.FromSlash(consumerRel),
				"---\ntitle: Consumer\n"+tc.consumerFM+"---\n\n# Consumer\n\n## Phases\n\n### Phase 1: X\n")
			warning, err := blockedByPromotionWarning(root, filepath.Join(root, filepath.FromSlash(consumerRel)))
			if err != nil {
				t.Fatalf("blockedByPromotionWarning: %v", err)
			}
			if tc.wantWarn {
				if warning == "" {
					t.Fatalf("want an advisory warning on %q, got none", tc.name)
				}
				for _, mention := range tc.mentions {
					if !strings.Contains(warning, mention) {
						t.Fatalf("warning = %q, want it to mention %q", warning, mention)
					}
				}
			} else if warning != "" {
				t.Fatalf("want no advisory warning, got %q", warning)
			}
		})
	}
}

// TestBlockedByPromotionClosure pins the promotion-closure half bound to
// tickets.move(to: "ready"): a typed prerequisite must already be in ready/ or
// .done/. It is status-only — a ready-but-unexecuted prerequisite passes the
// closure (the dispatch gate catches the unlanded case), an idea/todo one is
// refused, and an absent one is refused. A ticket with no blocked-by edge, or
// only a soft related: edge, promotes unchanged.
func TestBlockedByPromotionClosure(t *testing.T) {
	for _, tc := range []struct {
		name       string
		consumerFM string
		prereqs    map[string]string // stem -> status dir
		wantErr    bool
		errHas     string
	}{
		{
			name:       "prerequisite in ready passes (status-only)",
			consumerFM: "blocked-by: 260101-feat-a\n",
			prereqs:    map[string]string{"260101-feat-a": "ready"},
			wantErr:    false,
		},
		{
			name:       "prerequisite done passes",
			consumerFM: "blocked-by: 260101-feat-a\n",
			prereqs:    map[string]string{"260101-feat-a": ".done"},
			wantErr:    false,
		},
		{
			name:       "prerequisite in todo is refused",
			consumerFM: "blocked-by: 260101-feat-a\n",
			prereqs:    map[string]string{"260101-feat-a": "todo"},
			wantErr:    true,
			errHas:     "not ready/ or .done/",
		},
		{
			name:       "prerequisite absent is refused",
			consumerFM: "blocked-by: 260101-feat-absent\n",
			prereqs:    map[string]string{},
			wantErr:    true,
			errHas:     "resolves to no ticket",
		},
		{
			name:       "no blocked-by promotes (no regression)",
			consumerFM: "related:\n  260101-feat-a: partner\n",
			prereqs:    map[string]string{"260101-feat-a": "todo"},
			wantErr:    false,
		},
		{
			name:       "multi-edge refuses on the unlanded second edge",
			consumerFM: "blocked-by:\n  - 260101-feat-a\n  - 260101-feat-b\n",
			prereqs:    map[string]string{"260101-feat-a": ".done", "260101-feat-b": "idea"},
			wantErr:    true,
			errHas:     "260101-feat-b",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for stem, dir := range tc.prereqs {
				mustWrite(t, root, filepath.Join("ai-docs", "tickets", statusDirs[dir], stem+".md"),
					"---\ntitle: Prereq\n---\n\n# Prereq\n")
			}
			stem := "260101-feat-consumer"
			mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
				"---\ntitle: Consumer\n"+tc.consumerFM+"---\n\n# Consumer\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| scope.span | single-file | a.go |\n")
			_, err := TicketsMove(root, &mockGitRunner{}, TicketMoveOptions{TicketStem: stem, To: "ready"})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want a promotion refusal, got success")
				}
				if tc.errHas != "" && !strings.Contains(err.Error(), tc.errHas) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.errHas)
				}
			} else if err != nil {
				t.Fatalf("want promotion to succeed, got %v", err)
			}
		})
	}
}
