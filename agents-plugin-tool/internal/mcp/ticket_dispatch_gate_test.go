package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdoc"
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

// TestTicketsQueryJSONDispatchBlockedContract pins the public dispatch_blocked
// JSON contract on the point-resolve projection (format:"json"): the outer
// dispatch_blocked field name and its nested blocking_stem / reason field names
// and shape. A serialization rename or reshape of this cross-ticket scheduling
// fact — the field consumers key on — fails here.
func TestTicketsQueryJSONDispatchBlockedContract(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260101-feat-prereq.md",
		"---\ntitle: Prereq\n---\n# Prereq\n\n## Phases\n\n### Phase 1: A\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260101-feat-consumer.md",
		"---\ntitle: Consumer\nblocked-by: 260101-feat-prereq\n---\n# Consumer\n\n## Phases\n\n### Phase 1: X\n")
	initGit(t, root)

	resp := callScopedTool(t, root, 1, "tickets.query", map[string]any{
		"ticket_stem": "260101-feat-consumer", "format": "json",
	})

	// Raw field-name pins so a json-tag rename fails directly, not just via the
	// typed unmarshal below.
	if !strings.Contains(resp, `"dispatch_blocked"`) {
		t.Fatalf("json point-resolve is missing the dispatch_blocked field:\n%s", resp)
	}
	if !strings.Contains(resp, `"blocking_stem"`) || !strings.Contains(resp, `"reason"`) {
		t.Fatalf("dispatch_blocked object is missing blocking_stem/reason field names:\n%s", resp)
	}

	// Locally-declared json tags: if production renames a tag, this local struct
	// keeps the old name, the field stays zero/nil, and the assertions below fail.
	var payload struct {
		DispatchBlocked *struct {
			BlockingStem string `json:"blocking_stem"`
			Reason       string `json:"reason"`
		} `json:"dispatch_blocked"`
	}
	if err := json.Unmarshal([]byte(resp), &payload); err != nil {
		t.Fatalf("point-resolve json did not parse: %v\n%s", err, resp)
	}
	if payload.DispatchBlocked == nil {
		t.Fatalf("dispatch_blocked absent or under a renamed key after unmarshal:\n%s", resp)
	}
	if payload.DispatchBlocked.BlockingStem != "260101-feat-prereq" {
		t.Fatalf("blocking_stem = %q, want the unlanded prerequisite stem", payload.DispatchBlocked.BlockingStem)
	}
	if strings.TrimSpace(payload.DispatchBlocked.Reason) == "" {
		t.Fatalf("reason field empty or under a renamed key:\n%s", resp)
	}
}

// TestTicketsQueryBlockedMarkerInBothProjections pins the advisory body-marker
// surface tickets.query gained: the verbatim `## Blocked` heading rides both the
// discovery listing and the point-resolve, in compact text (as a blocked_marker
// line) and in JSON (as blocked_headings), a nonstandard suffix is returned
// uninterpreted, a ticket without the marker carries none, and — because the
// blocked ticket declares no blocked-by edge — the marker never becomes the
// typed dispatch_blocked hard gate.
func TestTicketsQueryBlockedMarkerInBothProjections(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260201-feat-blocked.md",
		"---\ntitle: Blocked widget\n---\n# Blocked widget\n\n## Blocked (2026-09-16) — owner smoke\n\nOwner-only.\n\n## Phases\n\n### Phase 1: X\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260202-feat-clean.md",
		"---\ntitle: Clean widget\n---\n# Clean widget\n\n## Phases\n\n### Phase 1: Y\n")
	initGit(t, root)

	const marker = "## Blocked (2026-09-16) — owner smoke"

	// Discovery listing (compact): the blocked ticket carries the marker line and
	// the clean ticket does not; the advisory marker is not the typed gate.
	discovery := callScopedTool(t, root, 1, "tickets.query", map[string]any{"query": "widget"})
	if !strings.Contains(discovery, "blocked_marker: "+marker) {
		t.Fatalf("discovery listing missing verbatim blocked_marker:\n%s", discovery)
	}
	if strings.Contains(discovery, "dispatch_blocked") {
		t.Fatalf("body marker must not compute the dispatch gate on discovery:\n%s", discovery)
	}
	if strings.Count(discovery, "blocked_marker:") != 1 {
		t.Fatalf("clean ticket must carry no blocked_marker line:\n%s", discovery)
	}

	// Discovery listing (JSON): the field name and verbatim value.
	discoveryJSON := callScopedTool(t, root, 2, "tickets.query", map[string]any{"query": "widget", "format": "json"})
	if !strings.Contains(discoveryJSON, `"blocked_headings"`) {
		t.Fatalf("discovery json missing blocked_headings field:\n%s", discoveryJSON)
	}
	var listing []struct {
		Stem            string   `json:"stem"`
		BlockedHeadings []string `json:"blocked_headings"`
	}
	if err := json.Unmarshal([]byte(discoveryJSON), &listing); err != nil {
		t.Fatalf("discovery json did not parse: %v\n%s", err, discoveryJSON)
	}
	for _, row := range listing {
		switch row.Stem {
		case "260201-feat-blocked":
			if len(row.BlockedHeadings) != 1 || row.BlockedHeadings[0] != marker {
				t.Fatalf("blocked ticket json headings = %#v", row.BlockedHeadings)
			}
		case "260202-feat-clean":
			if len(row.BlockedHeadings) != 0 {
				t.Fatalf("clean ticket json must carry no headings: %#v", row.BlockedHeadings)
			}
		}
	}

	// Point-resolve carries the same evidence in both modes.
	resolve := callScopedTool(t, root, 3, "tickets.query", map[string]any{"ticket_stem": "260201-feat-blocked"})
	if !strings.Contains(resolve, "blocked_marker: "+marker) {
		t.Fatalf("point-resolve compact missing blocked_marker:\n%s", resolve)
	}
	resolveJSON := callScopedTool(t, root, 4, "tickets.query", map[string]any{"ticket_stem": "260201-feat-blocked", "format": "json"})
	if !strings.Contains(resolveJSON, `"blocked_headings"`) || !strings.Contains(resolveJSON, marker) {
		t.Fatalf("point-resolve json missing verbatim blocked_headings:\n%s", resolveJSON)
	}
}

// TestTicketsQueryBlockedMarkerAndDispatchGateAreDistinctLines pins the ticket's
// "visibly distinct" decision positively: a ticket that carries BOTH an unlanded
// blocked-by edge and a body `## Blocked` marker renders the typed gate as a
// dispatch_blocked line and the advisory marker as a separate blocked_marker
// line, so a caller can never conflate the two.
func TestTicketsQueryBlockedMarkerAndDispatchGateAreDistinctLines(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260301-feat-prereq.md",
		"---\ntitle: Prereq\n---\n# Prereq\n\n## Phases\n\n### Phase 1: A\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260302-feat-both.md",
		"---\ntitle: Both\nblocked-by: 260301-feat-prereq\n---\n# Both\n\n## Blocked (2026-09-16) — owner smoke\n\nOwner-only.\n\n## Phases\n\n### Phase 1: X\n")
	initGit(t, root)

	resolve := callScopedTool(t, root, 1, "tickets.query", map[string]any{"ticket_stem": "260302-feat-both"})
	if !strings.Contains(resolve, "dispatch_blocked: 260301-feat-prereq") {
		t.Fatalf("typed gate line missing:\n%s", resolve)
	}
	if !strings.Contains(resolve, "blocked_marker: ## Blocked (2026-09-16) — owner smoke") {
		t.Fatalf("advisory marker line missing:\n%s", resolve)
	}
	// The two facts must be on separate lines, not fused into one.
	if !strings.Contains(resolve, "dispatch_blocked:") || !strings.Contains(resolve, "\n  blocked_marker:") {
		t.Fatalf("gate and marker must render as distinct lines:\n%s", resolve)
	}
}

func TestTicketsQueryPointResolveDispatchGateFailsOpen(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/ready/260101-feat-consumer.md",
		"---\ntitle: Consumer\nblocked-by: 260101-feat-prereq\n---\n# Consumer\n")
	initGit(t, root)
	server := NewServer(root, "test")
	called := false
	server.dispatchBlock = func(_ string, info wsdoc.TicketInfo, _ []string) (*wsdoc.DispatchBlock, error) {
		called = true
		if info.Stem != "260101-feat-consumer" || len(info.BlockedBy) != 1 {
			t.Fatalf("gate received wrong resolved ticket: %+v", info)
		}
		return nil, errors.New("injected dispatch scan failure")
	}
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"tickets.query","arguments":{"ticket_stem":"260101-feat-consumer"}}}` + "\n"
	var out bytes.Buffer
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatal(err)
	}
	raw := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["1"]
	resp := toolText(t, raw)
	if !called || strings.Contains(raw, `"isError":true`) || strings.Contains(resp, "injected dispatch scan failure") {
		t.Fatalf("dispatch scan failure was not swallowed: %s", raw)
	}
	if !strings.Contains(resp, "260101-feat-consumer") || strings.Contains(resp, "dispatch_blocked") {
		t.Fatalf("point resolve did not return the ticket without dispatch_blocked: %s", resp)
	}
}

// Point resolution scans terminal directories too, so a read error there must
// be reported rather than mistaken for a missing ticket.
func TestTicketsQueryPointResolveTerminalScanError(t *testing.T) {
	if runtime.GOOS == "windows" {
		// The scan error is injected via chmod 0o000 to deny read, which Windows
		// does not honor (its file mode only toggles the read-only bit, which
		// gates writes, not reads), so the "unreadable" file stays readable.
		t.Skip("chmod 0o000 read-denial is POSIX-only")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root; chmod 000 does not deny read")
	}
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260101-feat-prereq.md",
		"---\ntitle: Prereq\n---\n# Prereq\n\n## Phases\n\n### Phase 1: A\n")
	mustWrite(t, root, "ai-docs/tickets/ready/260101-feat-consumer.md",
		"---\ntitle: Consumer\nblocked-by: 260101-feat-prereq\n---\n# Consumer\n\n## Phases\n\n### Phase 1: X\n")
	// An unreadable file in .dropped/ makes the point-resolve scan fail.
	junkRel := "ai-docs/tickets/.dropped/260101-feat-unreadable.md"
	mustWrite(t, root, junkRel, "---\ntitle: Junk\n---\n# Junk\n")
	junkPath := filepath.Join(root, junkRel)
	if err := os.Chmod(junkPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(junkPath, 0o644) })
	initGit(t, root)

	resp := callScopedTool(t, root, 1, "tickets.query", map[string]any{"ticket_stem": "260101-feat-consumer"})

	if !strings.Contains(resp, "permission denied") {
		t.Fatalf("point-resolve did not report the terminal scan error:\n%s", resp)
	}
}
