package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scopedTicketRepo builds a committed board and hides every todo/ ticket except
// the keep list with a --no-cone sparse-checkout, then asserts the hide landed
// as intended (both directions) before the test body runs.
func scopedTicketRepo(t *testing.T, tickets map[string]string, keep ...string) string {
	t.Helper()
	return scopedTicketRepoDirs(t, []string{"todo"}, tickets, keep...)
}

// scopedTicketRepoDirs is scopedTicketRepo with the excluded status directories
// spelled out, for the cases that need a hidden ticket outside todo/.
func scopedTicketRepoDirs(t *testing.T, excludeDirs []string, tickets map[string]string, keep ...string) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	for rel, body := range tickets {
		mustWrite(t, root, rel, body)
	}
	initGit(t, root)
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "board")

	args := []string{"sparse-checkout", "set", "--no-cone", "/*"}
	for _, dir := range excludeDirs {
		args = append(args, "!/ai-docs/tickets/"+dir+"/*")
	}
	kept := map[string]bool{}
	for _, rel := range keep {
		kept[rel] = true
		args = append(args, "/"+rel)
	}
	runGit(t, root, args...)

	for rel := range tickets {
		hidden := false
		for _, dir := range excludeDirs {
			if strings.HasPrefix(rel, "ai-docs/tickets/"+dir+"/") && !kept[rel] {
				hidden = true
			}
		}
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if hidden && err == nil {
			t.Fatalf("fixture: %s should have been hidden but is on disk", rel)
		}
		if !hidden && err != nil {
			t.Fatalf("fixture: %s should have stayed on disk but is missing (%v)", rel, err)
		}
	}
	return root
}

// callScopedTool issues one authenticated tools/call against root and returns
// the response's single text block.
func callScopedTool(t *testing.T, root string, id int, name string, args map[string]any) string {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params":  map[string]any{"name": name, "arguments": args},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, string(payload)+"\n", &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	return toolText(t, byID[fmt.Sprint(id)])
}

// TestTicketsListScopeAnnotationTextOnly pins the two halves of the settled
// annotation shape: the aggregate hidden count is a text-mode-only trailing
// line (CommitResult.Advisories' json:"-" precedent), and JSON mode's content
// block stays a bare array byte-identical to an unscoped board carrying the
// same visible tickets.
func TestTicketsListScopeAnnotationTextOnly(t *testing.T) {
	useLeadProfile(t)
	scoped := scopedTicketRepo(t, map[string]string{
		"ai-docs/tickets/todo/260101-feat-visible.md": "---\ntitle: Visible\n---\n# Visible\n",
		"ai-docs/tickets/todo/260102-feat-hidden.md":  "---\ntitle: Hidden\n---\n# Hidden\n",
	}, "ai-docs/tickets/todo/260101-feat-visible.md")

	control := t.TempDir()
	mustWrite(t, control, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, control, "ai-docs/tickets/todo/260101-feat-visible.md", "---\ntitle: Visible\n---\n# Visible\n")
	initGit(t, control)

	text := callScopedTool(t, scoped, 1, "tickets.query", nil)
	if !strings.Contains(text, "260101-feat-visible") {
		t.Fatalf("tickets.list text lost the visible ticket:\n%s", text)
	}
	if strings.Contains(text, "260102-feat-hidden") {
		t.Fatalf("tickets.list is a discovery surface and must not list hidden tickets:\n%s", text)
	}
	if !strings.Contains(text, "scope: 1 ticket(s) hidden by this worktree's sparse-checkout scope (core.sparseCheckout)") {
		t.Fatalf("tickets.list text missing the hidden-count annotation:\n%s", text)
	}

	scopedJSON := callScopedTool(t, scoped, 2, "tickets.query", map[string]any{"format": "json"})
	controlJSON := callScopedTool(t, control, 3, "tickets.query", map[string]any{"format": "json"})
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(scopedJSON), &parsed); err != nil {
		t.Fatalf("tickets.list json mode is not a bare array: %v\n%s", err, scopedJSON)
	}
	if scopedJSON != controlJSON {
		t.Fatalf("tickets.list json mode changed under a scope:\nscoped:  %s\ncontrol: %s", scopedJSON, controlJSON)
	}
}

// TestTicketsStatusResolvesHiddenStemOverMCP is the resolution half of the same
// boundary: the explicit-stem surfaces report hidden-but-found, marked hidden.
//
// The out-of-scope stem is deliberately "260102-feat-shadow", and the flag
// assertion is on the bracketed "[hidden]" token: a stem containing the word
// "hidden" makes a bare strings.Contains(text, "hidden") true no matter what
// formatTickets renders, so the flag could be dropped entirely and the test
// would still pass.
func TestTicketsStatusResolvesHiddenStemOverMCP(t *testing.T) {
	useLeadProfile(t)
	root := scopedTicketRepo(t, map[string]string{
		"ai-docs/tickets/todo/260101-feat-visible.md": "---\ntitle: Visible\n---\n# Visible\n",
		"ai-docs/tickets/todo/260102-feat-shadow.md":  "---\ntitle: Shadow\n---\n# Shadow\n",
	}, "ai-docs/tickets/todo/260101-feat-visible.md")

	status := callScopedTool(t, root, 1, "tickets.query", map[string]any{"ticket_stem": "260102-feat-shadow"})
	if !strings.Contains(status, "260102-feat-shadow") {
		t.Fatalf("tickets.status did not resolve the out-of-scope ticket:\n%s", status)
	}
	if !strings.Contains(status, "[hidden]") {
		t.Fatalf("tickets.status did not mark the resolved ticket hidden:\n%s", status)
	}

	find := callScopedTool(t, root, 2, "tickets.query", map[string]any{"ticket_stem": "260102-feat-shadow"})
	if !strings.Contains(find, "260102-feat-shadow") {
		t.Fatalf("tickets.query(ticket_stem:) did not resolve the out-of-scope ticket:\n%s", find)
	}
	if !strings.Contains(find, "[hidden]") {
		t.Fatalf("tickets.query(ticket_stem:) did not mark the resolved ticket hidden:\n%s", find)
	}

	// A visible ticket must not carry the flag, so "[hidden]" is proven to
	// track TicketInfo.Hidden rather than being present unconditionally.
	visible := callScopedTool(t, root, 3, "tickets.query", map[string]any{"ticket_stem": "260101-feat-visible"})
	if strings.Contains(visible, "[hidden]") {
		t.Fatalf("a checked-out ticket was marked hidden:\n%s", visible)
	}

	// The query form stays a discovery surface: it lists nothing hidden and
	// carries the aggregate count instead.
	query := callScopedTool(t, root, 4, "tickets.query", map[string]any{"query": "Shadow"})
	if strings.Contains(query, "260102-feat-shadow") {
		t.Fatalf("tickets.query(query:) listed a hidden ticket:\n%s", query)
	}
	if !strings.Contains(query, "scope: 1 ticket(s) hidden") {
		t.Fatalf("tickets.query(query:) missing the hidden-count annotation:\n%s", query)
	}
}

// TestTicketsListScopeAnnotationSuppressedWhenFilterSelectsNothing pins the
// annotation's purpose: it must distinguish "hidden by the scope" from
// "filtered out by your own arguments", so a status filter that selects no
// status at all must not be blamed on the scope.
func TestTicketsListScopeAnnotationSuppressedWhenFilterSelectsNothing(t *testing.T) {
	useLeadProfile(t)
	// A hidden .done ticket is what makes this bite: without one, the count
	// would be zero for a reason unrelated to the archive gating.
	root := scopedTicketRepoDirs(t, []string{"todo", ".done"}, map[string]string{
		"ai-docs/tickets/todo/260101-feat-visible.md": "---\ntitle: Visible\n---\n# Visible\n",
		"ai-docs/tickets/todo/260102-feat-shadow.md":  "---\ntitle: Shadow\n---\n# Shadow\n",
		"ai-docs/tickets/.done/260103-feat-old.md":    "---\ntitle: Old\ncompleted: 2026-01-01\n---\n# Old\n",
	}, "ai-docs/tickets/todo/260101-feat-visible.md")

	// wsdoc.ticketStatuses drops an explicit "done" unless include_done is set,
	// so this listing is empty because of the caller's own arguments. Blaming
	// the scope would invert the annotation's purpose.
	text := callScopedTool(t, root, 1, "tickets.query", map[string]any{"statuses": []any{"done"}})
	if strings.Contains(text, "scope:") {
		t.Fatalf("an archive listing gated off by include_done was blamed on the scope:\n%s", text)
	}

	// Positive control: with the gate opened, the same request must report the
	// hidden archive ticket - so the suppression above is the gating, not a
	// blanket silence.
	opened := callScopedTool(t, root, 2, "tickets.query", map[string]any{"statuses": []any{"done"}, "include_done": true})
	if !strings.Contains(opened, "scope: 1 ticket(s) hidden") {
		t.Fatalf("include_done listing lost the hidden-count annotation:\n%s", opened)
	}
}

// TestTicketsCloseDeliversPartialMutationNoticeOverMCP pins the delivery path,
// not the notice text. wsdoc.TicketsClose populates PartialMutationNotice when
// its non-idempotent writes landed before the git move failed, but the tool case
// used to return toolTextResponse(id, "", err) and drop the result — so the
// caller was told to widen the scope and retry, was not told the file had
// already changed, and the retry appended a second `## Resolution` section.
//
// The failure is induced by occupying the destination status directory with a
// regular file rather than by a cross-scope `git mv`, so the test does not
// depend on the host's git version: the real trigger is git < 2.42, where
// check-rules is absent and the destination pre-flight fails open, which cannot
// be reproduced on a host whose git has the subcommand.
func TestTicketsCloseDeliversPartialMutationNoticeOverMCP(t *testing.T) {
	useLeadProfile(t)
	root := scopedTicketRepo(t, map[string]string{
		"ai-docs/tickets/ready/260101-feat-a.md":     "---\ntitle: A\n---\n# A\n",
		"ai-docs/tickets/todo/260102-feat-shadow.md": "---\ntitle: Shadow\n---\n# Shadow\n",
	})
	if err := os.WriteFile(filepath.Join(root, "ai-docs", "tickets", ".done"), []byte("occupied\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	text := callScopedTool(t, root, 1, "tickets.close", map[string]any{
		"stem":       "260101-feat-a",
		"status":     "done",
		"resolution": "Closed for the fixture.",
	})
	if !strings.Contains(text, "partial-mutation:") {
		t.Fatalf("tickets.close dropped the write-then-reject notice:\n%s", text)
	}
	// The actionable half: a blind retry would duplicate the appended section.
	if !strings.Contains(text, "## Resolution") {
		t.Fatalf("the delivered notice does not name the non-idempotent write:\n%s", text)
	}
}

func TestProjectTreeCarriesScopeAnnotation(t *testing.T) {
	useLeadProfile(t)
	root := scopedTicketRepo(t, map[string]string{
		"ai-docs/tickets/todo/260101-feat-visible.md": "---\ntitle: Visible\n---\n# Visible\n",
		"ai-docs/tickets/todo/260102-feat-hidden.md":  "---\ntitle: Hidden\n---\n# Hidden\n",
	}, "ai-docs/tickets/todo/260101-feat-visible.md")

	var out bytes.Buffer
	server := NewServer(root, "test")
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"project_tree","arguments":{}}}` + "\n"
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	text := toolText(t, byID["1"])
	if !strings.Contains(text, "scope: 1 ticket(s) hidden by this worktree's sparse-checkout scope") {
		t.Fatalf("project_tree missing the hidden-count annotation:\n%s", text)
	}
}
