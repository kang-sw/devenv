package mcp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// The knob-off variants only work while each shipped anchor is present: a
// missing anchor silently falls back to appending the old rule beside the new
// one. Pin every anchor to occur exactly once in the rendered shipped body.
func TestTicketReviewAutoAckAnchorsMatchShippedText(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	for _, harness := range []string{"claude", "codex"} {
		t.Run(harness, func(t *testing.T) {
			s := newTestServerWithHarness(t, harness)
			body, _, err := printPlaybook(s, rsrcRoot, "lead-ticket", nil, wsconfig.Options{}, "", nil)
			if err != nil {
				t.Fatalf("printPlaybook: %v", err)
			}
			for i, v := range ticketReviewAutoAckVariants {
				if n := strings.Count(body, v.on); n != 1 {
					t.Errorf("variant %d: shipped anchor occurs %d times, want 1:\n%s", i, n, v.on)
				}
			}
			off := applyTicketReviewAutoAckVariant("lead-ticket", body, false)
			if strings.Contains(off, "auto-confirm") {
				t.Errorf("knob-off body still names auto-confirmation:\n%s", off)
			}
			if got := applyTicketReviewAutoAckVariant("lead-discuss", body, false); got != body {
				t.Errorf("the variant must apply only to lead-ticket")
			}
		})
	}
}

// TestPlaybookReadLeadTicketReviewAutoAckFollowsKnob pins the rendered
// lead-ticket rule under ticket_review_auto_ack: on (builtin) lets the lead
// confirm review-raised announced and technical items and report them as
// auto-confirmed while reviewer `missing` issues and policy questions are still
// asked; off at session, project, or repo scope renders the acknowledgement
// rule for every such item.
func TestPlaybookReadLeadTicketReviewAutoAckFollowsKnob(t *testing.T) {
	read := func(t *testing.T, f *whiteboardFixture, name string) string {
		t.Helper()
		resp := f.call(t, "playbook.read", map[string]any{"name": name})
		if isToolError(resp) {
			t.Fatalf("playbook.read(%s): %s", name, resp)
		}
		return toolText(t, resp)
	}

	f := newWhiteboardFixture(t)
	on := read(t, f, "lead-ticket")
	normalizedOn := strings.Join(strings.Fields(on), " ")
	for _, want := range []string{
		"technical decisions are acknowledged or auto-confirmed (see `### Item classes`) and every policy question is confirmed, rejected, or explicitly deferred",
		"the settlement report lists every auto-confirmed item",
		"an announced or technical item that comes from a fact-populator `decision_gaps:` item or from a reviewer issue marked `autonomous` is confirmed on your own judgment, with no acknowledgement round, and reported as auto-confirmed",
		"A reviewer issue marked `missing` gets a user round whatever class you give it",
		"A trace knock-on of a review-raised item needs its own acknowledgement unless the reviewer issue itself named it",
		"`### Item classes` says which of them you confirm without asking",
		"apart from one auto-confirmed under `### Item classes`",
		"`(05) auto-confirmed - <one-line decision>`",
		"The report may first reach the user in the turn that moves the ticket to `ready/`",
		// Objections and reclasses keep their existing rules.
		"An announced or technical item the user objects to becomes a policy question under its existing ID",
	} {
		if !strings.Contains(normalizedOn, want) {
			t.Errorf("knob on: lead-ticket missing %q", want)
		}
	}
	// The always-policy rule for reviewer `missing` issues is retired;
	// the auto-confirmation limit must not reintroduce it.
	if strings.Contains(on, "is always a policy") || strings.Contains(normalizedOn, "is always a policy") {
		t.Errorf("knob on: lead-ticket reintroduces the retired always-policy phrase")
	}

	// The knob-off text is read through the tool, not rebuilt with the
	// function under test, so a wrong variant cannot make this test agree
	// with itself. Every scope below must render the same text.
	offFixture := newWhiteboardFixture(t)
	if resp := offFixture.call(t, "config.tune", map[string]any{"key": wsconfig.ItemTicketReviewAutoAck, "value": "off", "scope": "session"}); isToolError(resp) {
		t.Fatalf("config.tune off: %s", resp)
	}
	wantOff := read(t, offFixture, "lead-ticket")
	if wantOff == on {
		t.Fatalf("knob off must change the rendered lead-ticket text")
	}
	if strings.Contains(wantOff, "auto-confirm") {
		t.Errorf("knob off: lead-ticket still names auto-confirmation")
	}
	normalizedOff := strings.Join(strings.Fields(wantOff), " ")
	for _, want := range []string{
		"The settle point is reached when the announced defaults and technical decisions are acknowledged and every policy question is confirmed, rejected, or explicitly deferred",
		"the acknowledgement already showed every item, and the trace below",
		"Announced or technical items raised later in the same settlement (a trace knock-on, a new fact-populator gap) need their own acknowledgement.",
		"Send each `decision_gaps:` item to the Open Decision Queue; never resolve one from the populator's evidence alone.",
		"Any Open Decision Queue item the user has not settled, including an announced default or technical decision not yet acknowledged.",
		"This per-round settlement report is not the announced or technical group.",
	} {
		if !strings.Contains(normalizedOff, want) {
			t.Errorf("knob off: lead-ticket missing %q", want)
		}
	}

	discussOn := read(t, f, "lead-discuss")
	for _, scope := range []string{"session", "project", "repo"} {
		t.Run(scope, func(t *testing.T) {
			g := newWhiteboardFixture(t)
			resp := g.call(t, "config.tune", map[string]any{"key": wsconfig.ItemTicketReviewAutoAck, "value": "off", "scope": scope})
			if isToolError(resp) {
				t.Fatalf("config.tune %s: %s", scope, resp)
			}
			if !strings.Contains(toolText(t, resp), "[scope:"+scope+"]") {
				t.Fatalf("knob write must land in %s scope: %s", scope, resp)
			}
			if got := read(t, g, "lead-ticket"); got != wantOff {
				t.Fatalf("%s-scope off: rendered lead-ticket differs from the knob-off text:\n%s", scope, got)
			}
			if got := read(t, g, "lead-discuss"); got != discussOn {
				t.Fatalf("%s-scope off must leave other playbooks unchanged", scope)
			}
		})
	}
}
