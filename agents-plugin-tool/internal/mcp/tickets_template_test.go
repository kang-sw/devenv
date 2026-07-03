package mcp

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdoc"
)

func TestTicketTemplate(t *testing.T) {
	// Each of the 7 accepted type values returns non-empty text and no error.
	accepted := []string{"feat", "bug", "refactor", "chore", "research", "workset", "epic"}
	for _, tt := range accepted {
		text, err := wsdoc.TicketTemplate(tt)
		if err != nil {
			t.Errorf("TicketTemplate(%q) returned unexpected error: %v", tt, err)
		}
		if text == "" {
			t.Errorf("TicketTemplate(%q) returned empty text", tt)
		}
	}

	// feat, bug, refactor, chore all return identical content (same template).
	actionable := []string{"feat", "bug", "refactor", "chore"}
	first, _ := wsdoc.TicketTemplate(actionable[0])
	for _, tt := range actionable[1:] {
		got, _ := wsdoc.TicketTemplate(tt)
		if got != first {
			t.Errorf("TicketTemplate(%q) differs from TicketTemplate(%q); expected identical content", tt, actionable[0])
		}
	}

	// research return includes ## Background and no ## Phases heading.
	researchText, _ := wsdoc.TicketTemplate("research")
	if !strings.Contains(researchText, "## Background") {
		t.Error("TicketTemplate(\"research\") does not contain \"## Background\"")
	}
	if strings.Contains(researchText, "## Phases") {
		t.Error("TicketTemplate(\"research\") unexpectedly contains \"## Phases\"")
	}

	// workset return includes ## Tickets heading.
	worksetText, _ := wsdoc.TicketTemplate("workset")
	if !strings.Contains(worksetText, "## Tickets") {
		t.Error("TicketTemplate(\"workset\") does not contain \"## Tickets\"")
	}

	// epic return includes ## Child Tickets heading.
	epicText, _ := wsdoc.TicketTemplate("epic")
	if !strings.Contains(epicText, "## Child Tickets") {
		t.Error("TicketTemplate(\"epic\") does not contain \"## Child Tickets\"")
	}

	// feat return includes ## Phases heading.
	featText, _ := wsdoc.TicketTemplate("feat")
	if !strings.Contains(featText, "## Phases") {
		t.Error("TicketTemplate(\"feat\") does not contain \"## Phases\"")
	}

	// Unknown type returns an error with "unknown ticket type".
	_, err := wsdoc.TicketTemplate("invalid")
	if err == nil {
		t.Error("TicketTemplate(\"invalid\") expected error, got nil")
	} else if !strings.Contains(err.Error(), "unknown ticket type") {
		t.Errorf("TicketTemplate(\"invalid\") error %q does not contain \"unknown ticket type\"", err.Error())
	}

	// Empty type string returns an error.
	_, err = wsdoc.TicketTemplate("")
	if err == nil {
		t.Error("TicketTemplate(\"\") expected error, got nil")
	}
}
