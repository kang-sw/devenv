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
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	for rel, body := range tickets {
		mustWrite(t, root, rel, body)
	}
	initGit(t, root)
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "board")

	args := []string{"sparse-checkout", "set", "--no-cone", "/*", "!/ai-docs/tickets/todo/*"}
	kept := map[string]bool{}
	for _, rel := range keep {
		kept[rel] = true
		args = append(args, "/"+rel)
	}
	runGit(t, root, args...)

	for rel := range tickets {
		hidden := strings.HasPrefix(rel, "ai-docs/tickets/todo/") && !kept[rel]
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

	text := callScopedTool(t, scoped, 1, "tickets.list", nil)
	if !strings.Contains(text, "260101-feat-visible") {
		t.Fatalf("tickets.list text lost the visible ticket:\n%s", text)
	}
	if strings.Contains(text, "260102-feat-hidden") {
		t.Fatalf("tickets.list is a discovery surface and must not list hidden tickets:\n%s", text)
	}
	if !strings.Contains(text, "scope: 1 ticket(s) hidden by this worktree's sparse-checkout scope (core.sparseCheckout)") {
		t.Fatalf("tickets.list text missing the hidden-count annotation:\n%s", text)
	}

	scopedJSON := callScopedTool(t, scoped, 2, "tickets.list", map[string]any{"format": "json"})
	controlJSON := callScopedTool(t, control, 3, "tickets.list", map[string]any{"format": "json"})
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
func TestTicketsStatusResolvesHiddenStemOverMCP(t *testing.T) {
	useLeadProfile(t)
	root := scopedTicketRepo(t, map[string]string{
		"ai-docs/tickets/todo/260101-feat-visible.md": "---\ntitle: Visible\n---\n# Visible\n",
		"ai-docs/tickets/todo/260102-feat-hidden.md":  "---\ntitle: Hidden\n---\n# Hidden\n",
	}, "ai-docs/tickets/todo/260101-feat-visible.md")

	status := callScopedTool(t, root, 1, "tickets.status", map[string]any{"ticket_stem": "260102-feat-hidden"})
	if !strings.Contains(status, "260102-feat-hidden") || !strings.Contains(status, "hidden") {
		t.Fatalf("tickets.status did not report the hidden ticket as hidden-but-found:\n%s", status)
	}

	find := callScopedTool(t, root, 2, "tickets.find", map[string]any{"ticket_stem": "260102-feat-hidden"})
	if !strings.Contains(find, "260102-feat-hidden") || !strings.Contains(find, "hidden") {
		t.Fatalf("tickets.find(ticket_stem:) did not resolve the hidden ticket:\n%s", find)
	}
	// The query form stays a discovery surface: it lists nothing hidden and
	// carries the aggregate count instead.
	query := callScopedTool(t, root, 3, "tickets.find", map[string]any{"query": "Hidden"})
	if strings.Contains(query, "260102-feat-hidden") {
		t.Fatalf("tickets.find(query:) listed a hidden ticket:\n%s", query)
	}
	if !strings.Contains(query, "scope: 1 ticket(s) hidden") {
		t.Fatalf("tickets.find(query:) missing the hidden-count annotation:\n%s", query)
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
