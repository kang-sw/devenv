package mcp

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdoc"
)

func TestTicketChecklist(t *testing.T) {
	accepted := []string{"feat", "bug", "refactor", "chore", "research", "workset", "epic"}
	phases := []string{"content", "intent"}

	// Every accepted (type, phase) combination returns non-empty text and no error.
	for _, tt := range accepted {
		for _, ph := range phases {
			text, err := wsdoc.TicketChecklist(tt, ph)
			if err != nil {
				t.Errorf("TicketChecklist(%q, %q) returned unexpected error: %v", tt, ph, err)
			}
			if text == "" {
				t.Errorf("TicketChecklist(%q, %q) returned empty text", tt, ph)
			}
		}
	}

	// phase:"content" is category-invariant; assert every item's text travels
	// verbatim (a truncated/dropped item would otherwise still pass).
	contentFragments := []string{
		"forward-compatibility guardrail, and verification expectation; include suggested implementation strategy only when it was agreed, constrains implementation, or is needed to recover the intended contract",
		"exclude source-local edit notes unless they are settled constraints",
		"Open Decision Queue",
	}
	first, _ := wsdoc.TicketChecklist(accepted[0], "content")
	for _, frag := range contentFragments {
		if !strings.Contains(first, frag) {
			t.Errorf("TicketChecklist(_, \"content\") missing expected fragment %q", frag)
		}
	}
	for _, tt := range accepted[1:] {
		got, _ := wsdoc.TicketChecklist(tt, "content")
		if got != first {
			t.Errorf("TicketChecklist(%q, \"content\") differs from TicketChecklist(%q, \"content\"); expected identical content", tt, accepted[0])
		}
	}

	// phase:"intent", non-epic/workset category (e.g. "feat"): 6 items, no
	// epic/workset-conditional branch, closing items numbered 5/6. Assert
	// every item's text travels verbatim.
	featIntent, _ := wsdoc.TicketChecklist("feat", "intent")
	featFragments := []string{
		"checking every settled decision, contract, agreed API/type/event/UI sketch, rejected alternative, constraint, forward-compatibility guardrail, and verification expectation, and confirming nothing unconfirmed was written",
		"fresh implementer",
		"materially different caller-visible, workflow, API, or verification result",
		"preserved literally, not prose-flattened",
		"Result Forward note",
		"5. Fix confirmed gaps in-place",
		"6. Present a brief correction summary",
	}
	for _, frag := range featFragments {
		if !strings.Contains(featIntent, frag) {
			t.Errorf("TicketChecklist(\"feat\", \"intent\") missing expected fragment %q", frag)
		}
	}
	if strings.Contains(featIntent, "stayed out of the epic") || strings.Contains(featIntent, "parent-child semantics") {
		t.Error(`TicketChecklist("feat", "intent") unexpectedly contains an epic/workset-conditional branch`)
	}

	// epic/workset-conditional branch text appears only for its own category,
	// with closing items renumbered to 6/7 (item 5 inserted ahead of them).
	epicIntent, _ := wsdoc.TicketChecklist("epic", "intent")
	epicFragments := []string{
		"5. For `epic`, check that detailed implementation material stayed out of the epic and moved to a child-ticket invocation",
		"6. Fix confirmed gaps in-place",
		"7. Present a brief correction summary",
	}
	for _, frag := range epicFragments {
		if !strings.Contains(epicIntent, frag) {
			t.Errorf("TicketChecklist(\"epic\", \"intent\") missing expected fragment %q", frag)
		}
	}
	if strings.Contains(epicIntent, "parent-child semantics") {
		t.Error(`TicketChecklist("epic", "intent") unexpectedly contains the workset-specific branch text`)
	}
	if strings.Contains(epicIntent, "5. Fix confirmed gaps in-place") || strings.Contains(epicIntent, "6. Present a brief correction summary") {
		t.Error(`TicketChecklist("epic", "intent") miscounted renumbering; closing items must be 6/7, not 5/6`)
	}

	worksetIntent, _ := wsdoc.TicketChecklist("workset", "intent")
	worksetFragments := []string{
		"5. For `workset`, check that it did not create parent-child semantics, decomposition ownership, or implementation phases",
		"6. Fix confirmed gaps in-place",
		"7. Present a brief correction summary",
	}
	for _, frag := range worksetFragments {
		if !strings.Contains(worksetIntent, frag) {
			t.Errorf("TicketChecklist(\"workset\", \"intent\") missing expected fragment %q", frag)
		}
	}
	if strings.Contains(worksetIntent, "stayed out of the epic") {
		t.Error(`TicketChecklist("workset", "intent") unexpectedly contains the epic-specific branch text`)
	}
	if strings.Contains(worksetIntent, "5. Fix confirmed gaps in-place") || strings.Contains(worksetIntent, "6. Present a brief correction summary") {
		t.Error(`TicketChecklist("workset", "intent") miscounted renumbering; closing items must be 6/7, not 5/6`)
	}

	// Unknown phase returns an error mentioning the accepted set.
	_, err := wsdoc.TicketChecklist("feat", "bogus")
	if err == nil {
		t.Error(`TicketChecklist("feat", "bogus") expected error, got nil`)
	} else if !strings.Contains(err.Error(), "unknown ticket checklist phase") {
		t.Errorf("TicketChecklist(\"feat\", \"bogus\") error %q does not contain \"unknown ticket checklist phase\"", err.Error())
	}

	// Unknown type returns an error mentioning the accepted set.
	_, err = wsdoc.TicketChecklist("invalid", "content")
	if err == nil {
		t.Error(`TicketChecklist("invalid", "content") expected error, got nil`)
	} else if !strings.Contains(err.Error(), "unknown ticket type") {
		t.Errorf("TicketChecklist(\"invalid\", \"content\") error %q does not contain \"unknown ticket type\"", err.Error())
	}

	// Empty type and empty phase are also invalid (mirrors the TicketTemplate("")
	// boundary-input precedent in tickets_template_test.go).
	_, err = wsdoc.TicketChecklist("", "content")
	if err == nil {
		t.Error(`TicketChecklist("", "content") expected error, got nil`)
	}
	_, err = wsdoc.TicketChecklist("feat", "")
	if err == nil {
		t.Error(`TicketChecklist("feat", "") expected error, got nil`)
	}
}
