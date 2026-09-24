package wsdoc

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestContainsOpenDecisionQueueHeadingRule pins the promotion gate's match
// rule: the exact level-2 line, trailing whitespace allowed, outside fenced
// code blocks. A ticket that documents the section in an example fence (or
// names it inline) must not be refused.
func TestContainsOpenDecisionQueueHeadingRule(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want bool
	}{
		{"exact heading", "# T\n\n## Open Decision Queue\n\n(1) x\n", true},
		{"trailing whitespace", "# T\n\n## Open Decision Queue  \t\n", true},
		{"crlf line ending", "# T\r\n\r\n## Open Decision Queue\r\n", true},
		{"heading after a closed fence", "```text\nexample\n```\n\n## Open Decision Queue\n", true},
		{"leading indentation", "# T\n\n ## Open Decision Queue\n", false},
		{"level-3 heading", "### Open Decision Queue\n", false},
		{"level-1 heading", "# Open Decision Queue\n", false},
		{"suffixed heading", "## Open Decision Queue (last id: 3)\n", false},
		{"inline mention", "A temporary `## Open Decision Queue` section holds items.\n", false},
		{"backtick fence", "```text\n## Open Decision Queue\n```\n", false},
		{"tilde fence", "~~~\n## Open Decision Queue\n~~~\n", false},
		{"indented fence", "   ```\n## Open Decision Queue\n   ```\n", false},
		{"shorter inner fence does not close", "````md\n```\n## Open Decision Queue\n```\n````\n", false},
		{"other fence char does not close", "```\n~~~\n## Open Decision Queue\n```\n", false},
		{"unclosed fence runs to end", "```\n## Open Decision Queue\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := containsOpenDecisionQueue(tc.text); got != tc.want {
				t.Fatalf("containsOpenDecisionQueue(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

const odqSection = "## Open Decision Queue\n\nLast ID: 1\n\n(1) [open] Pick the retry bound.\n\n"

// TestTicketsMoveReadyRefusesOpenDecisionQueue pins the tickets.move half of
// the gate: a pending section refuses the ready/ move as a genuine no-op (no
// git call, no byte change, no move), and the same ticket moves once the
// section is deleted.
func TestTicketsMoveReadyRefusesOpenDecisionQueue(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-odq"
	rel := filepath.Join("ai-docs", "tickets", "todo", stem+".md")
	abs := filepath.Join(root, rel)
	pending := "---\ntitle: Sample\nsage-review-design: pending\nsage-review-completeness: pending\n---\n\n# Sample\n\n" + odqSection + sageRouteFactsSection + "Body.\n"
	mustWrite(t, root, rel, pending)
	runner := &mockGitRunner{}

	_, err := TicketsMove(root, runner, TicketMoveOptions{TicketStem: stem, To: "ready", SageReview: "auto"})
	if err == nil {
		t.Fatal("TicketsMove to ready with a pending Open Decision Queue section must be refused")
	}
	if !strings.Contains(err.Error(), "## Open Decision Queue") || !strings.Contains(err.Error(), "delete the section") {
		t.Fatalf("error = %q, want it to name the section and the remedy", err.Error())
	}
	if len(runner.calls) != 0 {
		t.Fatalf("git was called %d time(s); the refusal must be a no-op", len(runner.calls))
	}
	if got := readFileString(t, abs); got != pending {
		t.Fatalf("ticket bytes changed after refusal:\n%s", got)
	}

	settled := strings.Replace(pending, odqSection, "", 1)
	mustWrite(t, root, rel, settled)
	res, err := TicketsMove(root, runner, TicketMoveOptions{TicketStem: stem, To: "ready", SageReview: "auto"})
	if err != nil {
		t.Fatalf("TicketsMove after deleting the section: %v", err)
	}
	if res.NewPath != "ai-docs/tickets/ready/"+stem+".md" {
		t.Fatalf("new path = %q, want the ready/ path", res.NewPath)
	}
}

// TestTicketsMoveReadyIgnoresFencedOpenDecisionQueue pins the false-positive
// guard end to end: a ticket quoting the heading inside a fence still moves.
func TestTicketsMoveReadyIgnoresFencedOpenDecisionQueue(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-odqfence"
	rel := filepath.Join("ai-docs", "tickets", "todo", stem+".md")
	mustWrite(t, root, rel, "---\ntitle: Sample\n---\n\n# Sample\n\n```text\n## Open Decision Queue\n\n(1) <decision>\n```\n\n"+sageRouteFactsSection+"Body.\n")
	if _, err := TicketsMove(root, &mockGitRunner{}, TicketMoveOptions{TicketStem: stem, To: "ready", SageReview: "auto"}); err != nil {
		t.Fatalf("fenced heading must not refuse the move: %v", err)
	}
}

// TestTicketsMoveTodoIgnoresOpenDecisionQueue pins the gate's scope: todo/ is
// ordinary accepted board state for an epic and an actionable ticket alike, so
// a pending section there is allowed.
func TestTicketsMoveTodoIgnoresOpenDecisionQueue(t *testing.T) {
	for _, stem := range []string{"260101-feat-odqtodo", "260101-epic-odqtodo"} {
		t.Run(stem, func(t *testing.T) {
			root := t.TempDir()
			rel := filepath.Join("ai-docs", "tickets", "idea", stem+".md")
			mustWrite(t, root, rel, "---\ntitle: Sample\nsage-review-design: skipped\n---\n\n# Sample\n\n"+odqSection+"Body.\n")
			if _, err := TicketsMove(root, &mockGitRunner{}, TicketMoveOptions{TicketStem: stem, To: "todo", SageReview: "auto"}); err != nil {
				t.Fatalf("a todo/ move must not be gated on the queue section: %v", err)
			}
		})
	}
}

// TestSageGateRefusesOpenDecisionQueue pins the sage_gate half: both gated
// landings stop with stop_open_decision_queue ahead of the route-facts,
// posture-skip, and posture-resolution branches, and the stop writes nothing.
// The ungated landings (idea, and the research/workset todo exemptions) keep
// their behavior.
func TestSageGateRefusesOpenDecisionQueue(t *testing.T) {
	for _, tc := range []struct {
		name    string
		stem    string
		landing string
		front   string
		facts   bool
		want    string
	}{
		// No route facts: the queue stop precedes stop_missing_route_facts.
		{"actionable ready before route facts", "260101-feat-odq", "ready", "sage-review-design: pending\nsage-review-completeness: pending\n", false, "stop_open_decision_queue"},
		{"actionable ready with facts", "260101-feat-odq", "ready", "sage-review-design: pending\nsage-review-completeness: pending\n", true, "stop_open_decision_queue"},
		{"epic todo landing", "260101-epic-odq", "todo", "sage-review-design: pending\n", false, "stop_open_decision_queue"},
		// A skipped design posture would otherwise skip; the retained
		// epic-at-ready branch is gated too.
		{"epic ready before posture skip", "260101-epic-odq", "ready", "sage-review-design: skipped\n", false, "stop_open_decision_queue"},
		{"research todo exemption", "260101-research-odq", "todo", "", false, "skip"},
		{"workset todo exemption", "260101-workset-odq", "todo", "", false, "skip"},
		{"idea landing", "260101-feat-odq", "idea", "", false, "skip"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			rel := filepath.Join("ai-docs", "tickets", "todo", tc.stem+".md")
			body := "---\ntitle: Sample\n" + tc.front + "---\n\n# Sample\n\n" + odqSection
			if tc.facts {
				body += sageRouteFactsSection
			}
			body += "Body.\n"
			mustWrite(t, root, rel, body)

			res, err := SageGate(root, SageGateOptions{TicketStem: tc.stem, Landing: tc.landing}, "required")
			if err != nil {
				t.Fatalf("SageGate: %v", err)
			}
			if res.Action != tc.want {
				t.Fatalf("action = %q, want %q", res.Action, tc.want)
			}
			if got := readFileString(t, filepath.Join(root, rel)); got != body {
				t.Fatalf("ticket bytes changed:\n%s", got)
			}
		})
	}
}

// TestSageGateIgnoresFencedOpenDecisionQueue: a fenced heading reaches the
// ordinary posture path.
func TestSageGateIgnoresFencedOpenDecisionQueue(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-odqfence"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"),
		"---\ntitle: Sample\nsage-review-design: required\nsage-review-completeness: required\n---\n\n# Sample\n\n~~~\n## Open Decision Queue\n~~~\n\n"+sageRouteFactsSection+"Body.\n")
	res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "ready"}, "auto")
	if err != nil {
		t.Fatalf("SageGate: %v", err)
	}
	if res.Action != "run" {
		t.Fatalf("action = %q, want run", res.Action)
	}
}
