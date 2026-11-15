package mcp

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdoc"
)

func TestTicketTemplate(t *testing.T) {
	// Each of the 6 accepted type values returns non-empty text and no error.
	accepted := []string{"feat", "bug", "refactor", "chore", "research", "epic"}
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

// TestFormatTicketCreateRendersScopeWarning: the out-of-scope create warning
// reaches the text response, and a create without one renders no warning line.
func TestFormatTicketCreateRendersScopeWarning(t *testing.T) {
	with := formatTicketCreate(wsdoc.TicketCreateResult{Path: "p.md", Tip: "tip", Warning: "outside scope"})
	if !strings.Contains(with, "Created p.md\nwarning: outside scope\nTip: tip\n") {
		t.Fatalf("warning not rendered:\n%s", with)
	}
	if without := formatTicketCreate(wsdoc.TicketCreateResult{Path: "p.md", Tip: "tip"}); strings.Contains(without, "warning:") {
		t.Fatalf("warning line without a warning:\n%s", without)
	}
}
