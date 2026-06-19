package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// sessionKeyPattern validates the word-chain session key format: 4 lowercase words + 2-digit suffix.
var sessionKeyPattern = regexp.MustCompile(`^[a-z]+(-[a-z]+){3}-[0-9]{2}$`)

// callLogin issues a ws.ferrule (session-bootstrap) MCP call and returns the raw response line.
func callLogin(t *testing.T, server *Server, id int, root string, extra map[string]any) string {
	t.Helper()
	args := map[string]any{"root": root}
	for k, v := range extra {
		args[k] = v
	}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"ws.ferrule","arguments":%s}}`, id, raw)
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(line), &out); err != nil {
		t.Fatalf("ServeStdio error: %v", err)
	}
	resp := strings.TrimSpace(out.String())
	if resp == "" {
		t.Fatalf("got empty response for ws.ferrule id=%d", id)
	}
	return resp
}

// callToolOnce issues a single tools/call MCP request and returns the raw response line.
func callToolOnce(t *testing.T, server *Server, id int, name string, args map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, id, name, raw)
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(line), &out); err != nil {
		t.Fatalf("ServeStdio error: %v", err)
	}
	return strings.TrimSpace(out.String())
}

// parseLoginResponse extracts the session_key and root from a ws.ferrule text response.
func parseLoginResponse(t *testing.T, respLine string) (key, root string) {
	t.Helper()
	text := toolText(t, respLine)
	for _, segment := range strings.Split(text, "\n") {
		segment = strings.TrimSpace(segment)
		if strings.HasPrefix(segment, "session_key:") {
			key = strings.TrimSpace(strings.TrimPrefix(segment, "session_key:"))
		} else if strings.HasPrefix(segment, "root:") {
			root = strings.TrimSpace(strings.TrimPrefix(segment, "root:"))
		}
	}
	if key == "" {
		t.Fatalf("no session_key in login response: %s", text)
	}
	if root == "" {
		t.Fatalf("no root in login response: %s", text)
	}
	return key, root
}

// canonicalRootForTest returns the canonical form of root for test comparison.
// It mirrors server-side canonicalGitRoot by running git rev-parse --show-toplevel
// via the runGitOutput helper already defined in server_test.go (same package).
func canonicalRootForTest(t *testing.T, root string) string {
	t.Helper()
	out := runGitOutput(t, root, "rev-parse", "--show-toplevel")
	return filepath.Clean(strings.TrimSpace(string(out)))
}

// --- Test 1: ws.ferrule returns a valid key and correct canonical root ---

func TestLeadLoginReturnsKeyAndRoot(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root1 := t.TempDir()
	root2 := t.TempDir()
	initGit(t, root1)
	initGit(t, root2)

	canonical1 := canonicalRootForTest(t, root1)
	canonical2 := canonicalRootForTest(t, root2)

	server := NewServer(root1, "test")

	// Login for root1
	resp1 := callLogin(t, server, 1, root1, nil)
	if toolIsError(t, resp1) {
		t.Fatalf("ws.ferrule unexpectedly returned isError: %s", resp1)
	}
	key1, gotRoot1 := parseLoginResponse(t, resp1)
	if !sessionKeyPattern.MatchString(key1) {
		t.Fatalf("key %q does not match word-chain format", key1)
	}
	// Root round-trip: returned root must equal the canonical form of root1.
	if gotRoot1 != canonical1 {
		t.Fatalf("login root = %q, want canonical %q", gotRoot1, canonical1)
	}

	// Login for root2
	resp2 := callLogin(t, server, 2, root2, nil)
	if toolIsError(t, resp2) {
		t.Fatalf("ws.ferrule for root2 unexpectedly returned isError: %s", resp2)
	}
	key2, gotRoot2 := parseLoginResponse(t, resp2)
	if !sessionKeyPattern.MatchString(key2) {
		t.Fatalf("key2 %q does not match word-chain format", key2)
	}
	// Root round-trip for root2.
	if gotRoot2 != canonical2 {
		t.Fatalf("login root2 = %q, want canonical %q", gotRoot2, canonical2)
	}

	// Two keys must be distinct (distinct logins → distinct keys).
	if key1 == key2 {
		t.Fatalf("two logins returned the same key: %q", key1)
	}

	// JSON format branch: key and root must be present and correctly formatted.
	respJSON := callLogin(t, server, 3, root1, map[string]any{"format": "json"})
	if toolIsError(t, respJSON) {
		t.Fatalf("ws.ferrule json format returned isError: %s", respJSON)
	}
	jsonText := toolText(t, respJSON)
	var parsed struct {
		SessionKey string `json:"session_key"`
		Root       string `json:"root"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonText)), &parsed); err != nil {
		t.Fatalf("could not parse json login response: %v\ntext: %s", err, jsonText)
	}
	if !sessionKeyPattern.MatchString(parsed.SessionKey) {
		t.Fatalf("json session_key %q does not match format", parsed.SessionKey)
	}
	if parsed.Root != canonical1 {
		t.Fatalf("json root = %q, want canonical %q", parsed.Root, canonical1)
	}
}

func TestFerruleWithParentSessionKeyRecordsParent(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root1 := t.TempDir()
	root2 := t.TempDir()
	initGit(t, root1)
	initGit(t, root2)
	server := NewServer(root1, "test")

	parentResp := callLogin(t, server, 1, root1, nil)
	if toolIsError(t, parentResp) {
		t.Fatalf("primary ws.ferrule returned isError: %s", parentResp)
	}
	parentKey, _ := parseLoginResponse(t, parentResp)

	childResp := callLogin(t, server, 2, root2, map[string]any{"parent_session_key": parentKey})
	if toolIsError(t, childResp) {
		t.Fatalf("child ws.ferrule returned isError: %s", childResp)
	}
	childKey, _ := parseLoginResponse(t, childResp)
	childEntry, ok := server.sessions.lookup(childKey)
	if !ok {
		t.Fatalf("lookup child key %q failed", childKey)
	}
	if childEntry.parent != parentKey {
		t.Fatalf("child parent = %q, want %q", childEntry.parent, parentKey)
	}
	if childEntry.scope != roleLead {
		t.Fatalf("child scope = %q, want %q", childEntry.scope, roleLead)
	}
}

func TestFerruleWithoutParentSessionKeyMintsParentlessKey(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root := t.TempDir()
	initGit(t, root)
	server := NewServer(root, "test")

	resp := callLogin(t, server, 1, root, nil)
	if toolIsError(t, resp) {
		t.Fatalf("ws.ferrule returned isError: %s", resp)
	}
	key, _ := parseLoginResponse(t, resp)
	entry, ok := server.sessions.lookup(key)
	if !ok {
		t.Fatalf("lookup key %q failed", key)
	}
	if entry.parent != "" {
		t.Fatalf("parent = %q, want empty", entry.parent)
	}
}

func TestFerruleUnknownParentSessionKeyErrorsWithoutMint(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root := t.TempDir()
	initGit(t, root)
	server := NewServer(root, "test")
	before := sessionKeyFileCount(t, server)

	resp := callLogin(t, server, 1, root, map[string]any{"parent_session_key": "missing-parent-00"})
	if !toolIsError(t, resp) {
		t.Fatalf("unknown parent_session_key should return tool error: %s", resp)
	}
	if text := toolText(t, resp); !strings.Contains(text, `parent_session_key "missing-parent-00" is not a known session key`) {
		t.Fatalf("unknown parent error text = %q", text)
	}
	after := sessionKeyFileCount(t, server)
	if after != before {
		t.Fatalf("session key count grew after unknown parent: before=%d after=%d", before, after)
	}
}

func TestFerruleEmptyParentSessionKeyBehavesAsAbsent(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root := t.TempDir()
	initGit(t, root)
	server := NewServer(root, "test")

	resp := callLogin(t, server, 1, root, map[string]any{"parent_session_key": "   "})
	if toolIsError(t, resp) {
		t.Fatalf("empty parent_session_key should behave as absent, got isError: %s", resp)
	}
	key, _ := parseLoginResponse(t, resp)
	entry, ok := server.sessions.lookup(key)
	if !ok {
		t.Fatalf("lookup key %q failed", key)
	}
	if entry.parent != "" {
		t.Fatalf("parent = %q, want empty", entry.parent)
	}
}

func sessionKeyFileCount(t *testing.T, server *Server) int {
	t.Helper()
	dir, err := server.sessions.keysDir()
	if err != nil {
		t.Fatalf("keysDir: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}
	return count
}

// --- Test 2: valid session_key resolves the bound root; concurrent calls do not clobber ---

func TestSessionKeyResolvesRoot(t *testing.T) {
	useLeadProfile(t)
	root1 := t.TempDir()
	root2 := t.TempDir()
	initGit(t, root1)
	initGit(t, root2)

	canonical1 := canonicalRootForTest(t, root1)
	canonical2 := canonicalRootForTest(t, root2)

	// Create distinct sentinel untracked files in each root so git.status
	// responses differ and we can verify which root each call resolved against.
	mustWrite(t, root1, "session-marker-root1.txt", "marker1\n")
	mustWrite(t, root2, "session-marker-root2.txt", "marker2\n")

	server := NewServer(root1, "test")

	// Mint keys for both roots directly to avoid round-trip parsing.
	key1, err := server.sessions.mint(canonical1, roleLead, "")
	if err != nil {
		t.Fatalf("mint root1: %v", err)
	}
	key2, err := server.sessions.mint(canonical2, roleLead, "")
	if err != nil {
		t.Fatalf("mint root2: %v", err)
	}

	// Serial checks: key1 must surface root1's marker, key2 must surface root2's.
	resp1 := callToolOnce(t, server, 1, "git.status", map[string]any{"session_key": key1})
	if toolIsError(t, resp1) {
		t.Fatalf("git.status with key1 returned isError: %s", resp1)
	}
	if text1 := toolText(t, resp1); !strings.Contains(text1, "session-marker-root1.txt") {
		t.Fatalf("key1 git.status missing root1 marker; got: %s", text1)
	}

	resp2 := callToolOnce(t, server, 2, "git.status", map[string]any{"session_key": key2})
	if toolIsError(t, resp2) {
		t.Fatalf("git.status with key2 returned isError: %s", resp2)
	}
	if text2 := toolText(t, resp2); !strings.Contains(text2, "session-marker-root2.txt") {
		t.Fatalf("key2 git.status missing root2 marker; got: %s", text2)
	}

	// Concurrent calls: N goroutines each using key1 and N using key2.
	// Every response must contain the marker for that key's expected root.
	const workers = 8
	var wg sync.WaitGroup
	type result struct {
		id  int
		err string
	}
	errs := make([]string, workers*2)

	for i := 0; i < workers; i++ {
		idx := i
		wg.Add(2)
		go func() {
			defer wg.Done()
			var buf bytes.Buffer
			line := fmt.Sprintf(
				`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"git.status","arguments":{"session_key":%q}}}`,
				idx*2, key1)
			_ = server.ServeStdio(context.Background(), strings.NewReader(line), &buf)
			resp := strings.TrimSpace(buf.String())
			var r struct {
				Result struct {
					IsError bool `json:"isError"`
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"result"`
			}
			if jerr := json.Unmarshal([]byte(resp), &r); jerr != nil {
				errs[idx*2] = fmt.Sprintf("key1 worker %d parse error: %v", idx, jerr)
				return
			}
			if r.Result.IsError {
				errs[idx*2] = fmt.Sprintf("key1 worker %d got isError: %s", idx, resp)
				return
			}
			text := ""
			if len(r.Result.Content) > 0 {
				text = r.Result.Content[0].Text
			}
			if !strings.Contains(text, "session-marker-root1.txt") {
				errs[idx*2] = fmt.Sprintf("key1 worker %d resolved wrong root; response: %s", idx, text)
			}
		}()
		go func() {
			defer wg.Done()
			var buf bytes.Buffer
			line := fmt.Sprintf(
				`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"git.status","arguments":{"session_key":%q}}}`,
				idx*2+1, key2)
			_ = server.ServeStdio(context.Background(), strings.NewReader(line), &buf)
			resp := strings.TrimSpace(buf.String())
			var r struct {
				Result struct {
					IsError bool `json:"isError"`
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"result"`
			}
			if jerr := json.Unmarshal([]byte(resp), &r); jerr != nil {
				errs[idx*2+1] = fmt.Sprintf("key2 worker %d parse error: %v", idx, jerr)
				return
			}
			if r.Result.IsError {
				errs[idx*2+1] = fmt.Sprintf("key2 worker %d got isError: %s", idx, resp)
				return
			}
			text := ""
			if len(r.Result.Content) > 0 {
				text = r.Result.Content[0].Text
			}
			if !strings.Contains(text, "session-marker-root2.txt") {
				errs[idx*2+1] = fmt.Sprintf("key2 worker %d resolved wrong root; response: %s", idx, text)
			}
		}()
	}
	wg.Wait()
	for _, e := range errs {
		if e != "" {
			t.Error(e)
		}
	}
}

// --- Test 3: unknown session_key returns the unknown_session re-login contract ---

func TestUnknownSessionKeyReturnsError(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)

	server := NewServer(root, "test")

	const bogusKey = "word-word-word-word-00"
	resp := callToolOnce(t, server, 1, "git.status", map[string]any{"session_key": bogusKey})

	// Must be a toolTextResponse isError, NOT a JSON-RPC error (must have "result" field).
	var raw struct {
		Result *struct {
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct{ Code int } `json:"error"`
	}
	if err := json.Unmarshal([]byte(resp), &raw); err != nil {
		t.Fatalf("could not parse response: %v\n%s", err, resp)
	}
	if raw.Error != nil {
		t.Fatalf("got JSON-RPC error instead of toolTextResponse isError: %s", resp)
	}
	if raw.Result == nil || !raw.Result.IsError {
		t.Fatalf("expected isError:true in result, got: %s", resp)
	}
	text := toolText(t, resp)
	if !strings.Contains(text, "unknown_session") {
		t.Fatalf("error text missing 'unknown_session' token: %q", text)
	}
	// 260617 scrub: the recovery hint must NOT leak the bootstrap tool name and
	// must route the lead to the manual instead.
	if strings.Contains(text, "ws.lead.login") || strings.Contains(text, "ws.ferrule") {
		t.Fatalf("error text must not leak the bootstrap tool name: %q", text)
	}
	if !strings.Contains(text, "workflow-manual") {
		t.Fatalf("error text must route recovery to ws:workflow-manual: %q", text)
	}
}

// --- Test 4: capability-scoped keys restrict tools; lead key allows all ---

func TestCapabilityScopedKeyGatesTools(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)

	server := NewServer(root, "test")

	// git.commit is blocked for roleLeaf.
	leafKey, err := server.sessions.mint(root, roleLeaf, "")
	if err != nil {
		t.Fatalf("mint leaf key: %v", err)
	}
	// ws.mercenary.register is blocked for roleDelegate.
	delegateKey, err := server.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatalf("mint delegate key: %v", err)
	}
	leadKey, err := server.sessions.mint(root, roleLead, "")
	if err != nil {
		t.Fatalf("mint lead key: %v", err)
	}

	assertGateError := func(t *testing.T, label string, resp string, wantCode int) {
		t.Helper()
		var r struct {
			Error *struct{ Code int } `json:"error"`
		}
		if err := json.Unmarshal([]byte(resp), &r); err != nil {
			t.Fatalf("%s: parse response: %v\n%s", label, err, resp)
		}
		if r.Error == nil {
			t.Fatalf("%s: expected JSON-RPC error, got: %s", label, resp)
		}
		if r.Error.Code != wantCode {
			t.Fatalf("%s: error code = %d, want %d; response: %s", label, r.Error.Code, wantCode, resp)
		}
	}

	// leaf key: git.commit must be denied with -32601.
	deniedLeafResp := callToolOnce(t, server, 1, "git.commit", map[string]any{
		"session_key": leafKey,
		"paths":       []string{"nonexistent.txt"},
		"title":       "test",
		"ai_context":  []string{"test"},
	})
	assertGateError(t, "leaf/git.commit", deniedLeafResp, -32601)

	// delegate key: config.agents_tier must be denied with -32601 (config.* prefix).
	// ws.mercenary.* tools are also blocked for delegates but hit actorGate before the
	// keyed gate, producing a toolTextResponse error rather than -32601.
	deniedDelegateResp := callToolOnce(t, server, 2, "config.agents_tier", map[string]any{
		"session_key": delegateKey,
		"tier":        "core",
	})
	assertGateError(t, "delegate/config.agents_tier", deniedDelegateResp, -32601)

	// Non-lead key calling the bootstrap tool must be denied (self-bootstrap escalation block).
	deniedLoginResp := callToolOnce(t, server, 3, "ws.ferrule", map[string]any{
		"session_key": leafKey,
		"root":        root,
	})
	assertGateError(t, "leaf/bootstrap escalation", deniedLoginResp, -32601)

	// Delegate key calling the bootstrap tool must also be denied.
	deniedDelegateLoginResp := callToolOnce(t, server, 4, "ws.ferrule", map[string]any{
		"session_key": delegateKey,
		"root":        root,
	})
	assertGateError(t, "delegate/bootstrap escalation", deniedDelegateLoginResp, -32601)

	// Lead key must NOT be rejected by the capability gate for git.commit
	// (may fail for other reasons such as missing files, but not -32601 from the gate).
	allowedResp := callToolOnce(t, server, 5, "git.commit", map[string]any{
		"session_key": leadKey,
		"paths":       []string{"nonexistent.txt"},
		"title":       "test",
		"ai_context":  []string{"test"},
	})
	var rAllowed struct {
		Error *struct{ Code int } `json:"error"`
	}
	if err := json.Unmarshal([]byte(allowedResp), &rAllowed); err != nil {
		t.Fatalf("parse allowed response: %v\n%s", err, allowedResp)
	}
	if rAllowed.Error != nil && rAllowed.Error.Code == -32601 {
		t.Fatalf("lead key must NOT be blocked by capability gate (-32601): %s", allowedResp)
	}

	// Keyless caller must still be able to call the bootstrap tool (normal bootstrap path).
	keylessLoginResp := callLogin(t, server, 6, root, nil)
	if toolIsError(t, keylessLoginResp) {
		t.Fatalf("keyless ws.ferrule must succeed (additive guarantee): %s", keylessLoginResp)
	}
}

func TestKeylessRootAwareCallRequiresSessionKey(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	server := NewServer(root, "test")

	for _, args := range []map[string]any{
		{},
		{"root": root},
	} {
		resp := callToolOnce(t, server, 1, "git.status", args)
		if !toolIsError(t, resp) {
			t.Fatalf("keyless git.status should be a tool error for args %#v: %s", args, resp)
		}
		text := toolText(t, resp)
		if !strings.Contains(text, "mandatory_session_key") || !strings.Contains(text, "workflow-manual") {
			t.Fatalf("keyless error missing mandatory session-key guidance (manual route) for args %#v: %q", args, text)
		}
	}
}

func TestSetupCallsReturnUnknownTool(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	server := NewServer(root, "test")

	for _, args := range []map[string]any{
		{"root": root},
		{"method": "lead-workflow-bootstrap", "root": root},
	} {
		resp := callToolOnce(t, server, 1, "ws.setup", args)
		var raw struct {
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(resp), &raw); err != nil {
			t.Fatalf("parse ws.setup response: %v\n%s", err, resp)
		}
		if raw.Error == nil || raw.Error.Code != -32602 || !strings.Contains(raw.Error.Message, "unknown tool") {
			t.Fatalf("ws.setup should return unknown-tool JSON-RPC error: %s", resp)
		}
	}
}

// --- Test: a key minted on one Server resolves on a fresh Server instance ---
//
// This is the core guarantee of the filesystem-backed store: session continuity
// no longer depends on a shared in-memory registry, so a subagent that starts
// with its own MCP server instance can still resolve a lead-minted key.
func TestSessionKeySurvivesFreshServerInstance(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root := t.TempDir()
	initGit(t, root)
	canonical := canonicalRootForTest(t, root)
	mustWrite(t, root, "fresh-instance-marker.txt", "marker\n")

	// Mint on the "lead" server, then discard it entirely.
	leadServer := NewServer(root, "test")
	key, err := leadServer.sessions.mint(canonical, roleLead, "")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	// A brand-new server shares no in-memory state with leadServer; it must
	// resolve the key purely from the keys/<key>.json file.
	freshServer := NewServer(root, "test")
	resp := callToolOnce(t, freshServer, 1, "git.status", map[string]any{"session_key": key})
	if toolIsError(t, resp) {
		t.Fatalf("fresh-instance git.status with minted key returned isError: %s", resp)
	}
	if text := toolText(t, resp); !strings.Contains(text, "fresh-instance-marker.txt") {
		t.Fatalf("fresh-instance resolution missing root marker; got: %s", text)
	}

	// A path-unsafe key must be rejected as unknown, never resolved to a file.
	bad := callToolOnce(t, freshServer, 2, "git.status", map[string]any{"session_key": "../../etc/passwd"})
	if !toolIsError(t, bad) {
		t.Fatalf("path-unsafe key must be a tool error: %s", bad)
	}
	if text := toolText(t, bad); !strings.Contains(text, "unknown_session") {
		t.Fatalf("path-unsafe key should yield unknown_session, got: %q", text)
	}
}

func TestSessionMintRoundTripsParent(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer("", "test")

	parentKey := "parent-key-00"
	childKey, err := server.sessions.mint("/work/child", roleDelegate, parentKey)
	if err != nil {
		t.Fatalf("mint child: %v", err)
	}
	childEntry, ok := server.sessions.lookup(childKey)
	if !ok {
		t.Fatalf("lookup child key %q failed", childKey)
	}
	if childEntry.parent != parentKey {
		t.Fatalf("child parent = %q, want %q", childEntry.parent, parentKey)
	}
	if childEntry.scope != roleDelegate {
		t.Fatalf("child scope = %q, want %q", childEntry.scope, roleDelegate)
	}

	rootKey, err := server.sessions.mint("/work/root", roleLead, "")
	if err != nil {
		t.Fatalf("mint root: %v", err)
	}
	rootEntry, ok := server.sessions.lookup(rootKey)
	if !ok {
		t.Fatalf("lookup root key %q failed", rootKey)
	}
	if rootEntry.parent != "" {
		t.Fatalf("root parent = %q, want empty", rootEntry.parent)
	}
	if rootEntry.scope != roleLead {
		t.Fatalf("root scope = %q, want %q", rootEntry.scope, roleLead)
	}
}

func TestRenderPathMintedChildRecordsLeadParent(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"leaf-pb/leaf-pb.md": leafPlaybookContent,
	})
	initGit(t, rsrcRoot)
	server := NewServer(rsrcRoot, "test")
	leadKey, err := server.sessions.mint(rsrcRoot, roleLead, "")
	if err != nil {
		t.Fatalf("mint lead: %v", err)
	}

	resp := callToolOnce(t, server, 1, "playbook.render", map[string]any{
		"session_key":   leadKey,
		"name":          "leaf-pb",
		"root_override": rsrcRoot,
	})
	if toolIsError(t, resp) {
		t.Fatalf("playbook.render returned isError: %s", resp)
	}
	renderedPath := strings.TrimSpace(toolText(t, resp))
	bodyBytes, err := os.ReadFile(renderedPath)
	if err != nil {
		t.Fatalf("read rendered playbook %q: %v", renderedPath, err)
	}
	childKey := extractSplicedKey(t, string(bodyBytes))
	childEntry, ok := server.sessions.lookup(childKey)
	if !ok {
		t.Fatalf("lookup rendered child key %q failed", childKey)
	}
	if childEntry.parent != leadKey {
		t.Fatalf("rendered child parent = %q, want lead key %q", childEntry.parent, leadKey)
	}
	if childEntry.scope != roleLeaf {
		t.Fatalf("rendered child scope = %q, want %q", childEntry.scope, roleLeaf)
	}
	if childEntry.root != rsrcRoot {
		t.Fatalf("rendered child root = %q, want root_override %q", childEntry.root, rsrcRoot)
	}
}

func TestLegacySessionRecordWithoutParentResolves(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	store := newSessionStore()
	dir, err := store.keysDir()
	if err != nil {
		t.Fatalf("keysDir: %v", err)
	}
	const key = "legacy-key-00"
	legacyJSON := `{"schema_version":1,"root":"/legacy/root","scope":"delegate","prefer_mercenary":true}`
	if err := os.WriteFile(store.keyPath(dir, key), []byte(legacyJSON), 0o644); err != nil {
		t.Fatalf("write legacy record: %v", err)
	}
	entry, ok := store.lookup(key)
	if !ok {
		t.Fatalf("legacy key %q did not resolve", key)
	}
	if entry.parent != "" {
		t.Fatalf("legacy parent = %q, want empty", entry.parent)
	}
	if entry.root != "/legacy/root" || entry.scope != roleDelegate || !entry.preferMercenary {
		t.Fatalf("legacy entry = %#v, want root/scope/prefer preserved", entry)
	}
}

func TestSetPreferMercenaryPreservesParent(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	store := newSessionStore()
	parentKey := "parent-key-00"
	key, err := store.mint("/work/child", roleDelegate, parentKey)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if !store.setPreferMercenary(key) {
		t.Fatalf("setPreferMercenary(%q) returned false", key)
	}
	entry, ok := store.lookup(key)
	if !ok {
		t.Fatalf("lookup after setPreferMercenary failed")
	}
	if !entry.preferMercenary {
		t.Fatalf("preferMercenary was not set")
	}
	if entry.parent != parentKey {
		t.Fatalf("parent after setPreferMercenary = %q, want %q", entry.parent, parentKey)
	}
}

func TestKeylessAgentCallRequiresSessionKey(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	server := NewServer(root, "test")

	resp := callToolOnce(t, server, 1, "ws.mercenary.status", map[string]any{"name": "worker"})
	if !toolIsError(t, resp) {
		t.Fatalf("keyless ws.mercenary.status should be a tool error: %s", resp)
	}
	text := toolText(t, resp)
	if !strings.Contains(text, "mandatory_session_key") || !strings.Contains(text, "workflow-manual") {
		t.Fatalf("agent keyless error missing mandatory session-key guidance (manual route): %q", text)
	}
}
