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
