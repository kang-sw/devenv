package mcp

import (
	"strings"
	"testing"
)

// TestTicketsQueryPointResolveDispatchBlocked pins the dispatch-time hard gate's
// binding to tickets.query's single-stem point-resolve: a consumer whose
// blocked-by prerequisite has not landed carries a dispatch_blocked line naming
// the blocking stem, while a prerequisite already in .done/ clears it. The field
// is a scheduling fact computed live at the resolve, not read from a review
// stamp.
func TestTicketsQueryPointResolveDispatchBlocked(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260101-feat-prereq.md", "---\ntitle: Prereq\n---\n# Prereq\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260101-feat-consumer.md",
		"---\ntitle: Consumer\nblocked-by: 260101-feat-prereq\n---\n# Consumer\n\n## Phases\n\n### Phase 1: X\n")
	initGit(t, root)

	blocked := callScopedTool(t, root, 1, "tickets.query", map[string]any{"ticket_stem": "260101-feat-consumer"})
	if !strings.Contains(blocked, "dispatch_blocked: 260101-feat-prereq") {
		t.Fatalf("point-resolve of a blocked consumer missing dispatch_blocked:\n%s", blocked)
	}

	// The prerequisite's own point-resolve declares no blocked-by, so it is never
	// gated — proves the field tracks the edge rather than appearing everywhere.
	prereq := callScopedTool(t, root, 2, "tickets.query", map[string]any{"ticket_stem": "260101-feat-prereq"})
	if strings.Contains(prereq, "dispatch_blocked") {
		t.Fatalf("a ticket with no blocked-by edge was gated:\n%s", prereq)
	}

	// A discovery query is not a single-stem projection, so the scheduling fact
	// is deliberately absent there (populated only in point-resolve mode to avoid
	// noise on every listing).
	discovery := callScopedTool(t, root, 3, "tickets.query", map[string]any{"query": "Consumer"})
	if strings.Contains(discovery, "dispatch_blocked") {
		t.Fatalf("discovery listing must not compute the dispatch gate:\n%s", discovery)
	}
}

// TestTicketsQueryPointResolveDispatchClearsWhenLanded is the cleared half: once
// the prerequisite is in .done/, the same point-resolve carries no
// dispatch_blocked line.
func TestTicketsQueryPointResolveDispatchClearsWhenLanded(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/tickets/.done/260101-feat-prereq.md", "---\ntitle: Prereq\n---\n# Prereq\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260101-feat-consumer.md",
		"---\ntitle: Consumer\nblocked-by: 260101-feat-prereq\n---\n# Consumer\n\n## Phases\n\n### Phase 1: X\n")
	initGit(t, root)

	resolved := callScopedTool(t, root, 1, "tickets.query", map[string]any{"ticket_stem": "260101-feat-consumer", "include_done": true})
	if strings.Contains(resolved, "dispatch_blocked") {
		t.Fatalf("a landed (.done/) prerequisite must clear the dispatch gate:\n%s", resolved)
	}
}
