package mcp

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdoc"
)

func TestTicketChecklist(t *testing.T) {
	accepted := []string{"feat", "bug", "refactor", "chore", "research", "epic"}
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

	// Non-research content stays unchanged; assert every item's text travels
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
		if tt == "research" {
			continue
		}
		got, _ := wsdoc.TicketChecklist(tt, "content")
		if got != first {
			t.Errorf("TicketChecklist(%q, \"content\") differs from TicketChecklist(%q, \"content\"); expected identical content", tt, accepted[0])
		}
	}

	// phase:"intent" is the three-item conversation-fidelity check and is
	// shared by non-research categories: the capture enumeration, the epic shape rules,
	// and the fix-then-summarize procedure moved out. Assert every surviving
	// item's text travels verbatim, and that the dropped items stay dropped.
	featIntent, _ := wsdoc.TicketChecklist("feat", "intent")
	intentFragments := []string{
		"1. Test: could a fresh implementer build a materially different caller-visible, workflow, API, or verification result from the settled discussion without contradicting the ticket? If yes, capture the missing settled decision.",
		"2. Check that API/type/event/UI sketches were preserved literally, not prose-flattened.",
		"3. Check that no unconfirmed mechanism choice, future-scope hint, Result Forward note, or focus \"Next\" line was written.",
	}
	for _, frag := range intentFragments {
		if !strings.Contains(featIntent, frag) {
			t.Errorf("TicketChecklist(\"feat\", \"intent\") missing expected fragment %q", frag)
		}
	}
	if got := len(strings.Split(strings.TrimSpace(featIntent), "\n")); got != 3 {
		t.Errorf("TicketChecklist(\"feat\", \"intent\") has %d items, want exactly 3:\n%s", got, featIntent)
	}
	dropped := []string{
		"Re-read the written/edited ticket against the conversation",
		"stayed out of the epic",
		"parent-child semantics",
		"Fix confirmed gaps in-place",
		"Present a brief correction summary",
	}
	for _, frag := range dropped {
		if strings.Contains(featIntent, frag) {
			t.Errorf("TicketChecklist(\"feat\", \"intent\") still carries retired item text %q", frag)
		}
	}
	for _, tt := range accepted[1:] {
		if tt == "research" {
			continue
		}
		got, _ := wsdoc.TicketChecklist(tt, "intent")
		if got != featIntent {
			t.Errorf("TicketChecklist(%q, \"intent\") differs from TicketChecklist(\"feat\", \"intent\"); the intent checklist is category-invariant", tt)
		}
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
