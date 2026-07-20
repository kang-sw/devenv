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

	// phase:"content" is category-invariant and includes the Open Decision Queue mention.
	first, _ := wsdoc.TicketChecklist(accepted[0], "content")
	if !strings.Contains(first, "Open Decision Queue") {
		t.Error(`TicketChecklist(_, "content") does not mention "Open Decision Queue"`)
	}
	for _, tt := range accepted[1:] {
		got, _ := wsdoc.TicketChecklist(tt, "content")
		if got != first {
			t.Errorf("TicketChecklist(%q, \"content\") differs from TicketChecklist(%q, \"content\"); expected identical content", tt, accepted[0])
		}
	}

	// phase:"intent" carries the "fresh implementer" generative test verbatim.
	featIntent, _ := wsdoc.TicketChecklist("feat", "intent")
	if !strings.Contains(featIntent, "fresh implementer") {
		t.Error(`TicketChecklist("feat", "intent") does not contain "fresh implementer"`)
	}

	// epic/workset-conditional branch text appears only for its own category.
	epicIntent, _ := wsdoc.TicketChecklist("epic", "intent")
	if !strings.Contains(epicIntent, "stayed out of the epic") {
		t.Error(`TicketChecklist("epic", "intent") does not contain the epic-specific branch text`)
	}
	if strings.Contains(epicIntent, "parent-child semantics") {
		t.Error(`TicketChecklist("epic", "intent") unexpectedly contains the workset-specific branch text`)
	}

	worksetIntent, _ := wsdoc.TicketChecklist("workset", "intent")
	if !strings.Contains(worksetIntent, "parent-child semantics") {
		t.Error(`TicketChecklist("workset", "intent") does not contain the workset-specific branch text`)
	}
	if strings.Contains(worksetIntent, "stayed out of the epic") {
		t.Error(`TicketChecklist("workset", "intent") unexpectedly contains the epic-specific branch text`)
	}

	if strings.Contains(featIntent, "stayed out of the epic") || strings.Contains(featIntent, "parent-child semantics") {
		t.Error(`TicketChecklist("feat", "intent") unexpectedly contains an epic/workset-conditional branch`)
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
}
