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
