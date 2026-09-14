package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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

// TestTicketsQueryPointResolveDispatchGateFailsOpen verifies the documented
// fail-open contract at the server point-resolve seam (server.go's
// `blockErr == nil` guard): when the dispatch gate's live whole-board scan
// errors, the gate degrades silently — the point-resolve still returns the
// ticket, does not surface an error, and simply omits dispatch_blocked, rather
// than turning a plain "where is this stem" resolve into a failure.
//
// The scan error is injected with an unreadable ticket file in .dropped/: only
// DispatchBlockFor's boardByStem scan reads .dropped/ (and .done/), while the
// consumer's own point-resolve scan (ready/todo/idea by default) does not — so
// the ticket still resolves while the gate's scan fails. The prerequisite is
// left unlanded in ready/, so absent the injected error the gate WOULD emit a
// dispatch_blocked line (see TestTicketsQueryPointResolveDispatchBlocked); its
// absence here is therefore the fail-open path, not a no-op.
func TestTicketsQueryPointResolveDispatchGateFailsOpen(t *testing.T) {
	if runtime.GOOS == "windows" {
		// The scan error is injected via chmod 0o000 to deny read, which Windows
		// does not honor (its file mode only toggles the read-only bit, which
		// gates writes, not reads), so the "unreadable" file stays readable, the
		// board scan succeeds, and the gate correctly emits dispatch_blocked —
		// making the injection, not the fail-open contract, the thing that fails.
		// The fail-open path itself is platform-independent Go; POSIX coverage
		// below is sufficient.
		t.Skip("chmod 0o000 read-denial is POSIX-only; fail-open path covered on POSIX")
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
	// An unreadable file in .dropped/ (scanned only by the gate's whole-board
	// scan) makes boardByStem's readTicket fail, forcing DispatchBlockFor to
	// return an error.
	junkRel := "ai-docs/tickets/.dropped/260101-feat-unreadable.md"
	mustWrite(t, root, junkRel, "---\ntitle: Junk\n---\n# Junk\n")
	junkPath := filepath.Join(root, junkRel)
	if err := os.Chmod(junkPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(junkPath, 0o644) })
	initGit(t, root)

	resp := callScopedTool(t, root, 1, "tickets.query", map[string]any{"ticket_stem": "260101-feat-consumer"})

	// The ticket still resolves (fail-open did not propagate the scan error as a
	// call error): an error response would carry the failure text, not the stem.
	if !strings.Contains(resp, "260101-feat-consumer") {
		t.Fatalf("point-resolve did not return the ticket when the gate scan failed (error was not swallowed):\n%s", resp)
	}
	// The gate degraded silently: no dispatch_blocked despite an unlanded
	// prerequisite, because the scan errored.
	if strings.Contains(resp, "dispatch_blocked") {
		t.Fatalf("gate must degrade silently on a scan error, but emitted a dispatch_blocked line:\n%s", resp)
	}
}
