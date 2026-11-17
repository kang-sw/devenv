package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Phase 3a: stripModeGatedRegion pure-logic tests (TDD) -------------------

func TestStripModeGatedRegion_NoMarker(t *testing.T) {
	body := "line one\nline two\nline three"
	// Without any marker, the body should be unchanged regardless of keepContent.
	if got := stripModeGatedRegion(body, true); got != body {
		t.Fatalf("keepContent=true mutated marker-free body:\n got: %q\nwant: %q", got, body)
	}
	if got := stripModeGatedRegion(body, false); got != body {
		t.Fatalf("keepContent=false mutated marker-free body:\n got: %q\nwant: %q", got, body)
	}
}

func TestStripModeGatedRegion_KeepContent(t *testing.T) {
	// keepContent=true: marker lines removed, inner content kept.
	body := "before\n<!-- ws:fresh-only:start -->\nbootstrap line\n<!-- ws:fresh-only:end -->\nafter"
	want := "before\nbootstrap line\nafter"
	if got := stripModeGatedRegion(body, true); got != want {
		t.Fatalf("keepContent=true:\n got: %q\nwant: %q", got, want)
	}
}

func TestStripModeGatedRegion_StripContent(t *testing.T) {
	// keepContent=false: marker lines AND inner content removed.
	body := "before\n<!-- ws:fresh-only:start -->\nbootstrap line\n<!-- ws:fresh-only:end -->\nafter"
	want := "before\nafter"
	if got := stripModeGatedRegion(body, false); got != want {
		t.Fatalf("keepContent=false:\n got: %q\nwant: %q", got, want)
	}
}

func TestStripModeGatedRegion_MultipleLines(t *testing.T) {
	// Multiple inner lines in the gated region.
	body := "before\n<!-- ws:fresh-only:start -->\nline A\nline B\nline C\n<!-- ws:fresh-only:end -->\nafter"
	wantKeep := "before\nline A\nline B\nline C\nafter"
	wantStrip := "before\nafter"
	if got := stripModeGatedRegion(body, true); got != wantKeep {
		t.Fatalf("multi-line keep:\n got: %q\nwant: %q", got, wantKeep)
	}
	if got := stripModeGatedRegion(body, false); got != wantStrip {
		t.Fatalf("multi-line strip:\n got: %q\nwant: %q", got, wantStrip)
	}
}

func TestStripModeGatedRegion_MultipleRegions(t *testing.T) {
	// Multiple gated regions in one body.
	body := "a\n<!-- ws:fresh-only:start -->\nR1\n<!-- ws:fresh-only:end -->\nb\n<!-- ws:fresh-only:start -->\nR2\n<!-- ws:fresh-only:end -->\nc"
	wantKeep := "a\nR1\nb\nR2\nc"
	wantStrip := "a\nb\nc"
	if got := stripModeGatedRegion(body, true); got != wantKeep {
		t.Fatalf("multi-region keep:\n got: %q\nwant: %q", got, wantKeep)
	}
	if got := stripModeGatedRegion(body, false); got != wantStrip {
		t.Fatalf("multi-region strip:\n got: %q\nwant: %q", got, wantStrip)
	}
}

func TestStripModeGatedRegion_UnclosedMarker(t *testing.T) {
	// Unclosed start marker: trailing lines treated per keepContent.
	body := "before\n<!-- ws:fresh-only:start -->\norphaned line\nno end marker"
	wantKeep := "before\norphaned line\nno end marker"
	wantStrip := "before"
	if got := stripModeGatedRegion(body, true); got != wantKeep {
		t.Fatalf("unclosed keep:\n got: %q\nwant: %q", got, wantKeep)
	}
	if got := stripModeGatedRegion(body, false); got != wantStrip {
		t.Fatalf("unclosed strip:\n got: %q\nwant: %q", got, wantStrip)
	}
}

// callToolNoKey issues a single tools/call MCP request WITHOUT injecting a
// session_key — needed to test the fresh-mode path of ws.workflow_manual.
func callToolNoKey(t *testing.T, server *Server, id int, name string, args map[string]any) string {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	// Do NOT inject session_key (contrast with callToolWithKey).
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params":  map[string]any{"name": name, "arguments": args},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(string(raw)+"\n"), &out); err != nil {
		t.Fatalf("ServeStdio(%s) error: %v", name, err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	return toolText(t, byID[fmt.Sprint(id)])
}

// --- Phase 3a: ws.workflow_manual integration tests --------------------------

func TestWorkflowManualFreshMode(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")

	// Fresh mode: call with the reserved sentinel key (not keyless). The sentinel
	// is never minted, so the keyed lead-only gate sees a lookup-miss and skips;
	// the call reaches the handler, which maps the sentinel to fresh mode.
	resp := callToolWithKey(t, server, 5001, freshBootstrapKey, "workflow_manual", nil)
	if !strings.HasPrefix(resp, workflowManualReadFullNotice+"\n\n") {
		t.Fatalf("fresh mode must start with the full-result notice: %s", resp)
	}

	// No "Session State" section in fresh mode.
	if strings.Contains(resp, "Session State") {
		t.Errorf("fresh mode: unexpected Session State section in response:\n%s", resp)
	}
}

func TestWorkflowManualFreshModeWithRoot(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")

	// Fresh-with-root mode: sentinel key + root supplied. The handler must mint a
	// lead session key inline, strip the fresh-only block, and return Session Key
	// and Session State sections.
	resp := callToolWithKey(t, server, 5010, freshBootstrapKey, "workflow_manual", map[string]any{
		"root": root,
	})

	// (a) Session Key section must be present.
	if !strings.Contains(resp, "## Session Key") {
		t.Errorf("fresh-with-root: ## Session Key section absent from response:\n%s", resp)
	}
	// (c) Session State section must be present.
	if !strings.Contains(resp, "## Session State") {
		t.Errorf("fresh-with-root: ## Session State section absent from response:\n%s", resp)
	}
	// (d) Extract the minted key from the response and verify it resolves to a lead
	//     session entry. The key appears on the first non-empty line after the
	//     "## Session Key" heading.
	var mintedKey string
	inSessionKeySection := false
	for _, line := range strings.Split(resp, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Session Key" {
			inSessionKeySection = true
			continue
		}
		if inSessionKeySection {
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "#") {
				// Next section — key not found
				break
			}
			mintedKey = trimmed
			break
		}
	}
	if mintedKey == "" {
		t.Fatalf("fresh-with-root: could not extract minted key from response:\n%s", resp)
	}
	entry, ok := server.sessions.lookup(mintedKey)
	if !ok {
		t.Fatalf("fresh-with-root: minted key %q does not resolve to a session entry", mintedKey)
	}
	if entry.scope != roleLead {
		t.Errorf("fresh-with-root: minted key scope = %q; want %q", entry.scope, roleLead)
	}
}

func TestWorkflowManualKeylessRejected(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")

	// Keyless call must be rejected with a required session_key error.
	resp := callToolNoKey(t, server, 5002, "workflow_manual", nil)

	if !strings.Contains(resp, "session_key") {
		t.Errorf("keyless: response must mention session_key, got:\n%s", resp)
	}
}

func TestWorkflowManualDelegateKeyBlocked(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")

	// Mint a delegate-scoped key directly.
	delegateKey, err := server.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatalf("mint delegate key: %v", err)
	}

	// ws.workflow_manual must be rejected for delegate keys at the keyed gate.
	// The gate returns an RPC-level error (-32601), not a toolText response, so
	// read the raw JSON line (same pattern as session_auth_test.go assertGateError).
	rawResp := callToolOnce(t, server, 5003, "workflow_manual", map[string]any{
		"session_key": delegateKey,
	})

	// Must receive the lead-only profile rejection (JSON-RPC error -32601).
	if !strings.Contains(rawResp, "tool not available in current") {
		t.Errorf("delegate key: expected lead-only rejection, got:\n%s", rawResp)
	}
	// The rejection must carry the keyed-gate RPC error code (-32601), not a soft
	// text response — confirm the code is present in the raw JSON-RPC error.
	if !strings.Contains(rawResp, "-32601") {
		t.Errorf("delegate key: expected JSON-RPC error code -32601, got:\n%s", rawResp)
	}
	// Must NOT contain a manual body.
	if strings.Contains(rawResp, "Session State") {
		t.Errorf("delegate key: manual body must be absent from rejection response:\n%s", rawResp)
	}
}

func TestWorkflowManualContinueMode(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 5100, root, nil))

	// Enter implement mode to populate agenda+todos.
	enter := callToolWithKey(t, server, 5101, key, "route.resolve_implement", implementSkipDocsArgs("text"))
	if !strings.Contains(enter, "Implementation Verdict") {
		t.Fatalf("route.resolve_implement unexpected: %s", enter)
	}

	// Continue mode: key present and resolves.
	resp := callToolWithKey(t, server, 5102, key, "workflow_manual", nil)

	// Session State section must be present.
	if !strings.Contains(resp, "Session State") {
		t.Errorf("continue mode: Session State section absent:\n%s", resp)
	}
	// Agenda content present: renderSessionState emits "### agenda: <key>" headings.
	if !strings.Contains(resp, "### agenda: implement") {
		t.Errorf("continue mode: agenda heading '### agenda: implement' absent:\n%s", resp)
	}
	// Todo summary content present (Route and Prep are active).
	if !strings.Contains(resp, "Route") {
		t.Errorf("continue mode: todo 'Route' absent from summary:\n%s", resp)
	}
	if !strings.Contains(resp, "Prep") {
		t.Errorf("continue mode: todo 'Prep' absent from summary:\n%s", resp)
	}
}

func TestWorkflowManualTodoInstructionPreview(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 5110, root, nil))
	longInstruction := "Restore this todo instruction through the workflow manual summary path without showing the extra full detail."
	wantPreview := "Restore this todo instruction through the workflow manual su"
	wantTail := "mmary path without showing the extra full detail."

	if got := callToolWithKey(t, server, 5111, key, "todo.add", map[string]any{
		"key":         "restore",
		"title":       "Restore instruction",
		"instruction": longInstruction,
	}); !strings.Contains(got, "todo added: restore") {
		t.Fatalf("append unexpected: %s", got)
	}

	resp := callToolWithKey(t, server, 5112, key, "workflow_manual", nil)
	if !strings.Contains(resp, "### Todos") {
		t.Fatalf("workflow manual response missing todo summary:\n%s", resp)
	}
	if !strings.Contains(resp, "- [ ] {restore} Restore instruction\n      "+wantPreview) {
		t.Fatalf("workflow manual missing instruction preview:\n%s", resp)
	}
	if strings.Contains(resp, wantTail) {
		t.Fatalf("workflow manual rendered instruction tail, want preview only:\n%s", resp)
	}
}

func TestWorkflowManualUnknownKey(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheDir)

	server := NewServer(root, "test")

	// Use a syntactically valid but never-minted key.
	badKey := "no-such-key-here"
	resp := callToolWithKey(t, server, 5200, badKey, "workflow_manual", nil)

	// Must contain the no-restorable-state notice.
	if !strings.Contains(resp, "no restorable state for session key") {
		t.Errorf("unknown key: no-restorable-state notice absent:\n%s", resp)
	}
	// Fail-loud renders NO manual body, so the always-shown per-root rule and the
	// ws.ferrule mention it carries must be absent — a non-lead caller reaching
	// fail-loud (any unregistered key) must not learn the lead self-bootstrap call.
	if strings.Contains(resp, "ferrule") {
		t.Errorf("unknown key: manual body / ferrule mention must be absent in fail-loud:\n%s", resp)
	}
	// The recovery pointer names only the lead-revive skill (no ferrule/sentinel).
	if !strings.Contains(resp, "lead-revive") {
		t.Errorf("unknown key: fail-loud notice should point to lead-revive recovery:\n%s", resp)
	}
	// Must NOT have minted a key record. Use os.Stat on the specific record path so
	// the check is meaningful even when the sessions/ directory was never created.
	_, statErr := os.Stat(filepath.Join(cacheDir, sessionsDirName, badKey))
	if !os.IsNotExist(statErr) {
		t.Errorf("unknown key: key record was minted (stat: %v)", statErr)
	}
}

func TestWorkflowManualGitCommitReinjection(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 5300, root, nil))

	// Enter implement mode to populate todos.
	callToolWithKey(t, server, 5301, key, "route.resolve_implement", implementSkipDocsArgs("text"))

	// Stage a file and commit.
	testFile := filepath.Join(root, "test-p3a.txt")
	if err := os.WriteFile(testFile, []byte("p3a test\n"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	// git add
	{
		var out bytes.Buffer
		payload := map[string]any{
			"jsonrpc": "2.0",
			"id":      5302,
			"method":  "tools/call",
			"params": map[string]any{"name": "git.commit", "arguments": map[string]any{
				"session_key":     key,
				"paths":           []any{"test-p3a.txt"},
				"title":           "test(p3a): re-injection test",
				"ai_context":      []any{"Phase 3a git.commit re-injection test"},
				"expected_branch": headBranch(t, root),
			}},
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := server.ServeStdio(context.Background(), strings.NewReader(string(raw)+"\n"), &out); err != nil {
			t.Fatalf("git.commit error: %v", err)
		}
		byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
		commitResp := toolText(t, byID["5302"])

		// Assert that the commit response contains a todo summary fragment.
		if !strings.Contains(commitResp, "## TODO(ws reminder: update this if stale)") {
			t.Errorf("git.commit re-injection: reminder heading absent from response:\n%s", commitResp)
		}
		if !strings.Contains(commitResp, "Route") && !strings.Contains(commitResp, "- [") {
			t.Errorf("git.commit re-injection: todo summary absent from response:\n%s", commitResp)
		}

		// The session-key tip trailer must land after the TODO summary (key
		// line last, for maximal recency/salience).
		wantTip := fmt.Sprintf("tip: preserve this session key: %s during compaction", key)
		if !strings.Contains(commitResp, wantTip) {
			t.Errorf("git.commit re-injection: session-key tip absent from response:\n%s", commitResp)
		}
		if idx := strings.Index(commitResp, "## TODO("); idx == -1 || strings.Index(commitResp, wantTip) < idx {
			t.Errorf("git.commit re-injection: session-key tip must follow the TODO summary trailer:\n%s", commitResp)
		}
	}

	// Also assert: a commit with no todos appends nothing extra.
	// Create a new session with no todos, commit, and check the output shape.
	root2 := t.TempDir()
	initGit(t, root2)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server2 := NewServer(root2, "test")
	key2, _ := parseLoginResponse(t, callLogin(t, server2, 5400, root2, nil))

	testFile2 := filepath.Join(root2, "test-p3a-notodo.txt")
	if err := os.WriteFile(testFile2, []byte("no todo\n"), 0o644); err != nil {
		t.Fatalf("write test file2: %v", err)
	}
	{
		var out bytes.Buffer
		payload := map[string]any{
			"jsonrpc": "2.0",
			"id":      5401,
			"method":  "tools/call",
			"params": map[string]any{"name": "git.commit", "arguments": map[string]any{
				"session_key":     key2,
				"paths":           []any{"test-p3a-notodo.txt"},
				"title":           "test(p3a): no-todo commit",
				"ai_context":      []any{"no-todo test"},
				"expected_branch": headBranch(t, root2),
			}},
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := server2.ServeStdio(context.Background(), strings.NewReader(string(raw)+"\n"), &out); err != nil {
			t.Fatalf("git.commit(no-todo) error: %v", err)
		}
		byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
		commitResp := toolText(t, byID["5401"])
		// No todo summary section appended.
		if strings.Contains(commitResp, "Todo (post-commit)") || strings.Contains(commitResp, "TODO(ws reminder: update this if stale)") {
			t.Errorf("git.commit(no-todo): unexpected Todo section:\n%s", commitResp)
		}
		// The session-key tip still appends even with no todos.
		wantTip := fmt.Sprintf("tip: preserve this session key: %s during compaction", key2)
		if !strings.Contains(commitResp, wantTip) {
			t.Errorf("git.commit(no-todo): session-key tip absent from response:\n%s", commitResp)
		}
	}
}

func TestAppendSessionKeyTip(t *testing.T) {
	const key = "scarily-imminent-ploy-plated-84"
	withKey := appendSessionKeyTip("commit ok", key)
	wantTip := fmt.Sprintf("tip: preserve this session key: %s during compaction", key)
	if !strings.Contains(withKey, wantTip) {
		t.Errorf("appendSessionKeyTip: tip absent for non-empty key:\n%s", withKey)
	}
	if !strings.HasPrefix(withKey, "commit ok") {
		t.Errorf("appendSessionKeyTip: original text not preserved:\n%s", withKey)
	}

	noKey := appendSessionKeyTip("commit ok", "")
	if noKey != "commit ok" {
		t.Errorf("appendSessionKeyTip: expected no-op for empty key, got:\n%s", noKey)
	}
}

// --- 260702: ws.workflow_state integration tests -----------------------------

func TestWorkflowStateReturnsSessionStateOnly(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 5500, root, nil))

	// Enter implement mode to populate agenda+todos.
	enter := callToolWithKey(t, server, 5501, key, "route.resolve_implement", implementSkipDocsArgs("text"))
	if !strings.Contains(enter, "Implementation Verdict") {
		t.Fatalf("route.resolve_implement unexpected: %s", enter)
	}

	manualResp := callToolWithKey(t, server, 5502, key, "workflow_manual", nil)
	stateResp := callToolWithKey(t, server, 5503, key, "workflow_state", nil)

	// The workflow_state output must exactly equal the "## Session State" section
	// workflow_manual renders for the same session at the same point in time.
	manualIdx := strings.Index(manualResp, "## Session State")
	if manualIdx < 0 {
		t.Fatalf("workflow_manual response missing Session State section:\n%s", manualResp)
	}
	wantState := manualResp[manualIdx:]
	if stateResp != wantState {
		t.Fatalf("workflow_state mismatch.\ngot:\n%s\nwant (workflow_manual's Session State suffix):\n%s", stateResp, wantState)
	}

	// No manual reference/primitives text: workflow_state must be substantially
	// shorter than the full workflow_manual render, and must not carry manual-only
	// fragments (e.g. the always-shown per-root rule or ferrule mention).
	if len(stateResp) >= len(manualResp) {
		t.Fatalf("workflow_state (%d bytes) not shorter than workflow_manual (%d bytes)", len(stateResp), len(manualResp))
	}
	if strings.Contains(stateResp, "ferrule") {
		t.Errorf("workflow_state leaked manual body content:\n%s", stateResp)
	}

	// Session State content itself is present (agenda + todos).
	if !strings.Contains(stateResp, "### agenda: implement") {
		t.Errorf("workflow_state missing agenda heading:\n%s", stateResp)
	}
	if !strings.Contains(stateResp, "### Todos") {
		t.Errorf("workflow_state missing Todos heading:\n%s", stateResp)
	}
}

// TestWorkflowStateByteIdenticalToWorkflowManualWhenNotesPresent is the
// notes-present sibling of TestWorkflowStateReturnsSessionStateOnly: once
// facet B (260823) makes wsnote.Compute always render a non-empty "# Notes"
// block, workflow_state's resolved-record branch must append the identical
// computeNotes(rec.Root) text workflow_manual's CONTINUE branch does, or the
// two responses silently diverge only when a note actually exists.
func TestWorkflowStateByteIdenticalToWorkflowManualWhenNotesPresent(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 5505, root, nil))

	callToolWithKey(t, server, 5506, key, "note.write", map[string]any{
		"layer": "worktree",
		"notes": []any{map[string]any{"key": "state.parity", "value": "seen by both tools", "priority": 1}},
	})

	manualResp := callToolWithKey(t, server, 5507, key, "workflow_manual", nil)
	stateResp := callToolWithKey(t, server, 5508, key, "workflow_state", nil)

	manualIdx := strings.Index(manualResp, "## Session State")
	if manualIdx < 0 {
		t.Fatalf("workflow_manual response missing Session State section:\n%s", manualResp)
	}
	wantState := manualResp[manualIdx:]
	if stateResp != wantState {
		t.Fatalf("workflow_state mismatch with notes present.\ngot:\n%s\nwant (workflow_manual's Session State suffix):\n%s", stateResp, wantState)
	}
	if !strings.Contains(stateResp, "state.parity") || !strings.Contains(stateResp, "seen by both tools") {
		t.Fatalf("workflow_state missing the written note:\n%s", stateResp)
	}
}

func TestWorkflowStateEmptySessionReturnsEmptyPayloadNotError(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 5510, root, nil))

	// No enter.* call, no todos: session exists but has empty agenda/todos.
	resp := callToolWithKey(t, server, 5511, key, "workflow_state", nil)

	if !strings.Contains(resp, "## Session State") {
		t.Fatalf("workflow_state empty session missing Session State heading:\n%s", resp)
	}
	if !strings.Contains(resp, "(no todos)") {
		t.Errorf("workflow_state empty session missing empty-todos marker:\n%s", resp)
	}
	// Must not be the fail-loud no-restorable-state notice — this is a resolved,
	// merely-empty session, not an unknown key.
	if strings.Contains(resp, "no restorable state for session key") {
		t.Errorf("workflow_state empty session incorrectly returned fail-loud notice:\n%s", resp)
	}
}

func TestWorkflowStateKeylessRejected(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")

	resp := callToolNoKey(t, server, 5520, "workflow_state", nil)

	if !strings.Contains(resp, "session_key") {
		t.Errorf("keyless: response must mention session_key, got:\n%s", resp)
	}
	if strings.Contains(resp, "mint your lead key") {
		t.Errorf("keyless: self-bootstrap fragment must be absent from error response:\n%s", resp)
	}
}

func TestWorkflowStateDelegateKeyBlockedSameAsWorkflowManual(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")

	delegateKey, err := server.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatalf("mint delegate key: %v", err)
	}

	manualRawResp := callToolOnce(t, server, 5530, "workflow_manual", map[string]any{
		"session_key": delegateKey,
	})
	stateRawResp := callToolOnce(t, server, 5531, "workflow_state", map[string]any{
		"session_key": delegateKey,
	})

	// Same lead-only profile rejection shape (RPC-level -32601 error) as
	// workflow_manual for a non-lead scoped key.
	for _, rawResp := range []string{manualRawResp, stateRawResp} {
		if !strings.Contains(rawResp, "tool not available in current") {
			t.Errorf("delegate key: expected lead-only rejection, got:\n%s", rawResp)
		}
		if !strings.Contains(rawResp, "-32601") {
			t.Errorf("delegate key: expected JSON-RPC error code -32601, got:\n%s", rawResp)
		}
	}
	if strings.Contains(stateRawResp, "Session State") {
		t.Errorf("delegate key: workflow_state body must be absent from rejection response:\n%s", stateRawResp)
	}
}

func TestWorkflowStateUnknownKeySameFailLoudAsWorkflowManual(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cacheDir := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheDir)

	server := NewServer(root, "test")

	badKey := "no-such-key-here"
	manualResp := callToolWithKey(t, server, 5540, badKey, "workflow_manual", nil)
	stateResp := callToolWithKey(t, server, 5541, badKey, "workflow_state", nil)

	// workflow_state must produce the identical fail-loud notice text that
	// workflow_manual renders for the same unresolvable key (both end with the
	// same "## Session State ... no restorable state ..." notice).
	manualIdx := strings.Index(manualResp, "## Session State")
	if manualIdx < 0 {
		t.Fatalf("workflow_manual fail-loud response missing Session State heading:\n%s", manualResp)
	}
	wantNotice := manualResp[manualIdx:]
	if stateResp != wantNotice {
		t.Fatalf("workflow_state fail-loud mismatch.\ngot:\n%s\nwant (workflow_manual's fail-loud notice):\n%s", stateResp, wantNotice)
	}
	if !strings.Contains(stateResp, "lead-revive") {
		t.Errorf("unknown key: workflow_state fail-loud notice should point to lead-revive recovery:\n%s", stateResp)
	}
	if strings.Contains(stateResp, "ferrule") || strings.Contains(stateResp, "one Git worktree") {
		t.Errorf("unknown key: workflow_state must not leak manual body / ferrule mention:\n%s", stateResp)
	}

	// Must NOT have minted a key record.
	_, statErr := os.Stat(filepath.Join(cacheDir, sessionsDirName, badKey))
	if !os.IsNotExist(statErr) {
		t.Errorf("unknown key: key record was minted (stat: %v)", statErr)
	}
}

func TestWorkflowStateFreshSentinelIsFailLoudNotFresh(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")

	// The fresh-bootstrap sentinel is never a stored record; workflow_state has
	// no FRESH mode, so it must fall through to the same fail-loud path as any
	// other unresolvable key rather than minting or rendering manual content.
	resp := callToolWithKey(t, server, 5550, freshBootstrapKey, "workflow_state", nil)

	if !strings.Contains(resp, "no restorable state for session key") {
		t.Errorf("sentinel key: expected fail-loud notice, got:\n%s", resp)
	}
	if strings.Contains(resp, "mint your lead key") {
		t.Errorf("sentinel key: workflow_state must not render fresh-mode manual content:\n%s", resp)
	}
}

func TestWorkflowStateToolSchema(t *testing.T) {
	useLeadProfile(t)
	server := NewServer(t.TempDir(), "test")
	properties := toolPropertiesByName(t, callToolsList(t, server), "workflow_state")
	if _, ok := properties["session_key"]; !ok {
		t.Fatalf("workflow_state schema missing session_key property: %#v", properties)
	}
}
