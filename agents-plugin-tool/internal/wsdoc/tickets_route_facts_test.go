package wsdoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTicketRouteFactsProjection pins the parse the implementation route
// depends on: rows keyed by their first column, header and alignment rows
// skipped by shape, the section ending at the next heading, and a present but
// empty section reported as present-with-no-rows so a caller can tell "never
// populated" from "populated wrongly".
func TestTicketRouteFactsProjection(t *testing.T) {
	for _, tc := range []struct {
		name        string
		body        string
		wantPresent bool
		wantFacts   map[string]string
	}{
		{
			name:        "no section",
			body:        "# T\n\n## Phases\n\n### Phase 1: X\n",
			wantPresent: false,
		},
		{
			name:        "populated table",
			body:        "# T\n\n## Route Facts\n\n| fact | value | evidence |\n|---|---|---|\n| scope.span | multi-file | a.go, b.go |\n| risk.correctness | high | touches the parser |\n\n## Phases\n\n| fact | value |\n| scope.span | ignored-after-heading |\n",
			wantPresent: true,
			wantFacts:   map[string]string{"scope.span": "multi-file", "risk.correctness": "high"},
		},
		{
			name:        "present but empty",
			body:        "# T\n\n## Route Facts\n\nNot populated yet.\n\n## Phases\n",
			wantPresent: true,
		},
		{
			name:        "alignment row and blank value skipped",
			body:        "# T\n\n## Route Facts\n\n| fact | value |\n|:---:|:---|\n| scope.span |  |\n| risk.fit | low |\n",
			wantPresent: true,
			wantFacts:   map[string]string{"risk.fit": "low"},
		},
		{
			// A capitalized header cell is a table the author wrote, not a
			// fact; reading it as one would fail the whole section on a
			// difference no reader can see.
			name:        "header row skipped whatever its case",
			body:        "# T\n\n## Route Facts\n\n| Fact | Value |\n|---|---|\n| risk.fit | low |\n",
			wantPresent: true,
			wantFacts:   map[string]string{"risk.fit": "low"},
		},
		{
			// Any heading closes the section, not only a sibling `## ` one: a
			// result or edition block landing after the table must not have
			// its rows absorbed into the facts.
			name:        "sub-heading ends the section",
			body:        "# T\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| risk.fit | low |\n\n### Result (abc1234)\n\n| fact | value |\n| risk.fit | high |\n",
			wantPresent: true,
			wantFacts:   map[string]string{"risk.fit": "low"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			present, facts := ticketRouteFacts(tc.body)
			if present != tc.wantPresent {
				t.Fatalf("present = %v, want %v", present, tc.wantPresent)
			}
			if len(facts) != len(tc.wantFacts) {
				t.Fatalf("facts = %v, want %v", facts, tc.wantFacts)
			}
			for key, want := range tc.wantFacts {
				if facts[key] != want {
					t.Fatalf("facts[%q] = %q, want %q", key, facts[key], want)
				}
			}
		})
	}
}

// TestTicketAtProjectsRouteFacts pins that the path-addressed entry point the
// route resolver uses goes through the same reader as every other ticket
// query, status included.
func TestTicketAtProjectsRouteFacts(t *testing.T) {
	root := t.TempDir()
	rel := "ai-docs/tickets/ready/260101-feat-sample.md"
	mustWrite(t, root, filepath.FromSlash(rel),
		"---\ntitle: Sample\n---\n\n# Sample\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| scope.surface | internal |\n\n## Phases\n\n### Phase 1: Do it\n")
	info, err := TicketAt(root, rel)
	if err != nil {
		t.Fatalf("TicketAt: %v", err)
	}
	if info.Status != "ready" || info.Stem != "260101-feat-sample" || info.Title != "Sample" {
		t.Fatalf("projection = %+v", info)
	}
	if !info.RouteFactsPresent || info.RouteFacts["scope.surface"] != "internal" {
		t.Fatalf("route facts = %v present=%v", info.RouteFacts, info.RouteFactsPresent)
	}
	if len(info.Phases) != 1 {
		t.Fatalf("phases = %+v, want the one declared phase", info.Phases)
	}
	if _, err := TicketAt(root, "ai-docs/tickets/ready/260101-feat-absent.md"); !os.IsNotExist(err) {
		t.Fatalf("absent ticket error = %v, want a not-exist error", err)
	}
}

// TestSageGateReadyRefusesMissingRouteFacts pins the ready/ promotion gate:
// a ticket the route resolver could not route is refused before any posture
// question, and the categories that never reach an implementation run are
// exempt. Presence is the whole check.
func TestSageGateReadyRefusesMissingRouteFacts(t *testing.T) {
	for _, tc := range []struct {
		name       string
		stem       string
		routeFacts bool
		wantAction string
	}{
		{name: "actionable without facts", stem: "260101-feat-sample", wantAction: "stop_missing_route_facts"},
		{name: "actionable with facts", stem: "260101-feat-sample", routeFacts: true, wantAction: "run"},
		{name: "epic exempt", stem: "260101-epic-sample", wantAction: "run"},
		{name: "research exempt", stem: "260101-research-sample", wantAction: "skip"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			body := "---\ntitle: Sample\nsage-review-design: required\nsage-review-completeness: required\n---\n\n# Sample\n\n"
			if tc.routeFacts {
				body += sageRouteFactsSection
			}
			body += "Body text.\n"
			mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", tc.stem+".md"), body)
			res, err := SageGate(root, SageGateOptions{TicketStem: tc.stem, Landing: "ready"}, "auto")
			if err != nil {
				t.Fatalf("SageGate: %v", err)
			}
			if res.Action != tc.wantAction {
				t.Fatalf("action = %q, want %q", res.Action, tc.wantAction)
			}
		})
	}
}

// TestSageGateTodoLandingIgnoresRouteFacts pins that the refusal is scoped to
// the ready/ landing: facts are a promotion-time requirement, and a design
// review at todo/ runs long before they exist.
func TestSageGateTodoLandingIgnoresRouteFacts(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", "260101-feat-sample.md"),
		"---\ntitle: Sample\nsage-review-design: required\n---\n\n# Sample\n\nBody text.\n")
	res, err := SageGate(root, SageGateOptions{TicketStem: "260101-feat-sample", Landing: "todo"}, "auto")
	if err != nil {
		t.Fatalf("SageGate: %v", err)
	}
	if res.Action != "run" {
		t.Fatalf("todo landing action = %q, want run", res.Action)
	}
}

// TestTicketsMoveReadyTipsMissingRouteFacts pins the soft counterpart on the
// move itself: tickets.move never blocks, so a lead moving a ticket directly
// still learns the ticket cannot be routed yet.
func TestTicketsMoveReadyTipsMissingRouteFacts(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join("ai-docs", "tickets", "todo", "260101-feat-sample.md")
	mustWrite(t, root, rel, "---\ntitle: Sample\nspec: 260101-sample\n---\n\n# Sample\n\nBody.\n")
	res, err := TicketsMove(root, &mockGitRunner{}, TicketMoveOptions{TicketStem: "260101-feat-sample", To: "ready"})
	if err != nil {
		t.Fatalf("TicketsMove: %v", err)
	}
	if !strings.Contains(res.Tip, "No ## Route Facts section") {
		t.Fatalf("tip = %q, want the missing-route-facts advisory", res.Tip)
	}
}

// TestTicketsMoveReadyStaysSilentWhenFactsAreNotOwed pins the other side of the
// advisory: the tip fires on a real gap, not on every `ready/` move. A ticket
// carrying the section, and a category the ready gate exempts, both move
// quietly.
func TestTicketsMoveReadyStaysSilentWhenFactsAreNotOwed(t *testing.T) {
	cases := []struct {
		name string
		stem string
		body string
	}{
		{
			name: "section present",
			stem: "260101-feat-sample",
			body: "---\ntitle: Sample\nspec: 260101-sample\n---\n\n# Sample\n\nBody.\n\n## Route Facts\n\n| fact | value | evidence |\n|---|---|---|\n| scope.span | single-file | a.go |\n",
		},
		{
			name: "exempt category",
			stem: "260101-research-sample",
			body: "---\ntitle: Sample\n---\n\n# Sample\n\nBody.\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			rel := filepath.Join("ai-docs", "tickets", "todo", tc.stem+".md")
			mustWrite(t, root, rel, tc.body)
			res, err := TicketsMove(root, &mockGitRunner{}, TicketMoveOptions{TicketStem: tc.stem, To: "ready"})
			if err != nil {
				t.Fatalf("TicketsMove: %v", err)
			}
			if strings.Contains(res.Tip, "No ## Route Facts section") {
				t.Fatalf("tip = %q, want no missing-route-facts advisory", res.Tip)
			}
		})
	}
}

// TestTicketAtRefusesPathsOutsideTheBoard pins the confinement: the ticket path
// reaching TicketAt is caller-supplied, and nothing outside the board is a
// ticket. Each case asserts the guard's own refusal rather than any error, so a
// path that would otherwise read cleanly cannot pass by accident.
func TestTicketAtRefusesPathsOutsideTheBoard(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, filepath.FromSlash("AGENTS.md"), "secret\n")
	// Both of these are readable files: without the guard they would be
	// projected as tickets, so they are what the refusal has to catch.
	mustWrite(t, root, filepath.FromSlash("ai-docs/tickets/ready/notes.txt"), "not a ticket\n")
	for _, rel := range []string{
		"AGENTS.md",
		"../AGENTS.md",
		"ai-docs/tickets/../../AGENTS.md",
		"ai-docs/tickets/ready/",
		"ai-docs/tickets/ready/notes.txt",
	} {
		_, err := TicketAt(root, rel)
		if err == nil {
			t.Fatalf("TicketAt(%q) succeeded; want a not-a-ticket-path refusal", rel)
		}
		if !strings.Contains(err.Error(), "not a ticket path") {
			t.Fatalf("TicketAt(%q) err = %v, want the confinement refusal", rel, err)
		}
	}
}

// TestTicketAtAcceptsAnAbsolutePathUnderTheRoot pins the other side of the
// confinement: the board test is about where the ticket is, not how the caller
// spelled the path, and a caller relaying a path from elsewhere may well hold
// the absolute one.
func TestTicketAtAcceptsAnAbsolutePathUnderTheRoot(t *testing.T) {
	root := t.TempDir()
	rel := "ai-docs/tickets/ready/260101-feat-sample.md"
	mustWrite(t, root, filepath.FromSlash(rel), "# Sample\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| risk.fit | low |\n")
	info, err := TicketAt(root, filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("TicketAt(absolute): %v", err)
	}
	if info.Path != rel || info.Status != "ready" || info.RouteFacts["risk.fit"] != "low" {
		t.Fatalf("absolute path projected as %+v, want the same ticket as the relative form", info)
	}
	// A path outside the root stays refused whichever form it takes.
	if _, err := TicketAt(root, filepath.Join(t.TempDir(), "ai-docs", "tickets", "ready", "260101-feat-other.md")); err == nil {
		t.Fatal("an absolute path outside the root was accepted; want a refusal")
	}
}
