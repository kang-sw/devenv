package wsdoc

import (
	"strings"
	"testing"
)

func TestResearchOutcomeLedgerTemplateAndConvention(t *testing.T) {
	template, err := TicketTemplate("research")
	if err != nil {
		t.Fatal(err)
	}
	convention, err := ReadConvention("ticket-conventions")
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"template": template, "convention": convention} {
		if strings.TrimSpace(text) == "" {
			t.Errorf("%s is empty", name)
		}
	}
}

func TestResearchChecklistAuthorityBoundary(t *testing.T) {
	for _, phase := range []string{"content", "intent"} {
		text, err := TicketChecklist("research", phase)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"Outcome Ledger", "Verified Findings", "Confirmed Decisions", "Proposals", "Open Questions", "non-authoritative", "literally"} {
			if !strings.Contains(text, want) {
				t.Errorf("research %s missing %q", phase, want)
			}
		}
		if strings.Contains(text, "Exclude anything unconfirmed") || strings.Contains(text, "no unconfirmed mechanism choice") {
			t.Errorf("research %s still forbids labeled investigation output", phase)
		}
	}
}

func TestNonResearchChecklistsRemainVerbatim(t *testing.T) {
	want := map[string]string{
		"content": `1. Capture every settled decision, contract, agreed API/type/event/UI sketch (literal, not prose-flattened), rejected alternative, constraint, forward-compatibility guardrail, and verification expectation; include suggested implementation strategy only when it was agreed, constrains implementation, or is needed to recover the intended contract.
2. Exclude anything unconfirmed — return it to the Open Decision Queue instead of writing it; exclude source-local edit notes unless they are settled constraints.
3. When the user approved a visual reference, check that text mockups (HTML/SVG) are linked by path at their tracked ai-docs/assets/mockups/ location, binary references remain in the ticket's tracked assets folder and are linked by path, and the verification requires a side-by-side comparison of a result capture against it.`,
		"intent": `1. Test: could a fresh implementer build a materially different caller-visible, workflow, API, or verification result from the settled discussion without contradicting the ticket? If yes, capture the missing settled decision.
2. Check that API/type/event/UI sketches were preserved literally, not prose-flattened.
3. Check that no unconfirmed mechanism choice, future-scope hint, Result Forward note, or focus "Next" line was written.`,
	}
	for _, category := range []string{"feat", "bug", "refactor", "chore", "epic"} {
		for phase, expected := range want {
			got, err := TicketChecklist(category, phase)
			if err != nil || got != expected {
				t.Errorf("%s %s checklist changed: %q (%v)", category, phase, got, err)
			}
		}
	}
}
