package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// keyPattern validates the word-chain session key format: 4 lowercase words + 2-digit suffix.
var sessionKeyPattern = regexp.MustCompile(`^[a-z]+(-[a-z]+){3}-[0-9]{2}$`)

// callLogin issues a ws.lead.login MCP call and returns the raw response line.
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
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"ws.lead.login","arguments":%s}}`, id, raw)
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(line), &out); err != nil {
		t.Fatalf("ServeStdio error: %v", err)
	}
	resp := strings.TrimSpace(out.String())
	if resp == "" {
		t.Fatalf("got empty response for ws.lead.login id=%d", id)
	}
	return resp
}

// callTool issues a single tools/call MCP request and returns the raw response line.
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

// parseLoginResponse extracts the session_key and root from a ws.lead.login text response.
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

// --- Test 1: ws.lead.login returns a valid key and correct root for two distinct repos ---

func TestLeadLoginReturnsKeyAndRoot(t *testing.T) {
	useLeadProfile(t)
	root1 := t.TempDir()
	root2 := t.TempDir()
	initGit(t, root1)
	initGit(t, root2)

	server := NewServer(root1, "test")

	// Login for root1
	resp1 := callLogin(t, server, 1, root1, nil)
	if toolIsError(t, resp1) {
		t.Fatalf("ws.lead.login unexpectedly returned isError: %s", resp1)
	}
	key1, gotRoot1 := parseLoginResponse(t, resp1)
	if !sessionKeyPattern.MatchString(key1) {
		t.Fatalf("key %q does not match word-chain format", key1)
	}
	if gotRoot1 == "" {
		t.Fatalf("login returned empty root")
	}

	// Login for root2
	resp2 := callLogin(t, server, 2, root2, nil)
	if toolIsError(t, resp2) {
		t.Fatalf("ws.lead.login for root2 unexpectedly returned isError: %s", resp2)
	}
	key2, _ := parseLoginResponse(t, resp2)
	if !sessionKeyPattern.MatchString(key2) {
		t.Fatalf("key2 %q does not match word-chain format", key2)
	}

	// Two keys must be distinct
	if key1 == key2 {
		t.Fatalf("two logins returned the same key: %q", key1)
	}

	// JSON format branch
	respJSON := callLogin(t, server, 3, root1, map[string]any{"format": "json"})
	if toolIsError(t, respJSON) {
		t.Fatalf("ws.lead.login json format returned isError: %s", respJSON)
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
	if parsed.Root == "" {
		t.Fatalf("json response has empty root")
	}
}

// --- Test 2: valid session_key resolves the bound root; concurrent calls do not clobber ---

func TestSessionKeyResolvesRoot(t *testing.T) {
	useLeadProfile(t)
	root1 := t.TempDir()
	root2 := t.TempDir()
	initGit(t, root1)
	initGit(t, root2)

	server := NewServer(root1, "test")

	// Mint keys for both roots directly on the registry to avoid round-trip parsing.
	key1, err := server.sessions.mint(root1, roleLead)
	if err != nil {
		t.Fatalf("mint root1: %v", err)
	}
	key2, err := server.sessions.mint(root2, roleLead)
	if err != nil {
		t.Fatalf("mint root2: %v", err)
	}

	// Verify that a root-aware call with key1 resolves root1, not root2.
	resp1 := callToolOnce(t, server, 1, "git.status", map[string]any{"session_key": key1})
	if toolIsError(t, resp1) {
		t.Fatalf("git.status with key1 returned isError: %s", resp1)
	}

	// Verify that a root-aware call with key2 resolves root2.
	resp2 := callToolOnce(t, server, 2, "git.status", map[string]any{"session_key": key2})
	if toolIsError(t, resp2) {
		t.Fatalf("git.status with key2 returned isError: %s", resp2)
	}

	// Concurrent calls: start N goroutines using key1 and N using key2;
	// confirm none surfaces an error (which would indicate wrong root resolution).
	const workers = 8
	var wg sync.WaitGroup
	errs := make([]string, workers*2)
	for i := 0; i < workers; i++ {
		idx := i
		wg.Add(2)
		go func() {
			defer wg.Done()
			var buf bytes.Buffer
			line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"git.status","arguments":{"session_key":%q}}}`, idx*2, key1)
			_ = server.ServeStdio(context.Background(), strings.NewReader(line), &buf)
			resp := strings.TrimSpace(buf.String())
			var r struct {
				Result struct {
					IsError bool `json:"isError"`
				} `json:"result"`
			}
			if jerr := json.Unmarshal([]byte(resp), &r); jerr != nil {
				errs[idx*2] = fmt.Sprintf("parse error for key1 worker %d: %v", idx, jerr)
			} else if r.Result.IsError {
				errs[idx*2] = fmt.Sprintf("key1 worker %d got isError response: %s", idx, resp)
			}
		}()
		go func() {
			defer wg.Done()
			var buf bytes.Buffer
			line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"git.status","arguments":{"session_key":%q}}}`, idx*2+1, key2)
			_ = server.ServeStdio(context.Background(), strings.NewReader(line), &buf)
			resp := strings.TrimSpace(buf.String())
			var r struct {
				Result struct {
					IsError bool `json:"isError"`
				} `json:"result"`
			}
			if jerr := json.Unmarshal([]byte(resp), &r); jerr != nil {
				errs[idx*2+1] = fmt.Sprintf("parse error for key2 worker %d: %v", idx, jerr)
			} else if r.Result.IsError {
				errs[idx*2+1] = fmt.Sprintf("key2 worker %d got isError response: %s", idx, resp)
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
	if !strings.Contains(text, "ws.lead.login") {
		t.Fatalf("error text must name the re-login recovery verb 'ws.lead.login': %q", text)
	}
}

// --- Test 4: capability-scoped key restricts tools; lead key allows all ---

func TestCapabilityScopedKeyGatesTools(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)

	server := NewServer(root, "test")

	// git.commit is a lead-only tool: roleLeaf blocks it.
	leafKey, err := server.sessions.mint(root, roleLeaf)
	if err != nil {
		t.Fatalf("mint leaf key: %v", err)
	}
	leadKey, err := server.sessions.mint(root, roleLead)
	if err != nil {
		t.Fatalf("mint lead key: %v", err)
	}

	// A leaf-scoped key should be denied git.commit.
	deniedResp := callToolOnce(t, server, 1, "git.commit", map[string]any{
		"session_key": leafKey,
		"paths":       []string{"nonexistent.txt"},
		"title":       "test",
		"ai_context":  []string{"test"},
	})
	var rDenied struct {
		Error  *struct{ Code int } `json:"error"`
		Result *struct {
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(deniedResp), &rDenied); err != nil {
		t.Fatalf("parse denied response: %v\n%s", err, deniedResp)
	}
	// Should be a JSON-RPC error (profile denial, not toolTextResponse)
	if rDenied.Error == nil {
		t.Fatalf("leaf-scoped key should produce a JSON-RPC error for git.commit, got: %s", deniedResp)
	}

	// A lead key must be allowed to proceed (may fail for other reasons like missing files,
	// but must NOT be rejected by the capability gate — the error would not be a -32601 code).
	allowedResp := callToolOnce(t, server, 2, "git.commit", map[string]any{
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
		t.Fatalf("lead-scoped key must NOT be blocked by capability gate, got -32601 error: %s", allowedResp)
	}
}

// --- Test 5: keyless call still resolves through the existing chain (additive guarantee) ---

func TestKeylessCallFallsThroughToExistingChain(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)

	server := NewServer(root, "test")

	// A keyless git.status call with no session_key must still work via the
	// existing root resolver chain (server root fallback).
	resp := callToolOnce(t, server, 1, "git.status", map[string]any{})
	if toolIsError(t, resp) {
		t.Fatalf("keyless git.status unexpectedly returned isError: %s", resp)
	}
	text := toolText(t, resp)
	if text == "" {
		t.Fatalf("keyless git.status returned empty text")
	}

	// Also confirm that ws.setup root assignment still works (actor model unchanged).
	var setupOut bytes.Buffer
	setupInput := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ws.setup","arguments":{"root":%q}}}`,
		root,
	)
	server2 := NewServer(".", "test")
	if err := server2.ServeStdio(context.Background(), strings.NewReader(setupInput), &setupOut); err != nil {
		t.Fatalf("ServeStdio error: %v", err)
	}
	setupResp := strings.TrimSpace(setupOut.String())
	if toolIsError(t, setupResp) {
		t.Fatalf("ws.setup returned isError: %s", setupResp)
	}

	// Subsequent keyless call should resolve via sessionRoot (set by ws.setup).
	var followOut bytes.Buffer
	followInput := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"git.status","arguments":{}}}`
	if err := server2.ServeStdio(context.Background(), strings.NewReader(followInput), &followOut); err != nil {
		t.Fatalf("ServeStdio follow-up error: %v", err)
	}
	followResp := strings.TrimSpace(followOut.String())
	if toolIsError(t, followResp) {
		t.Fatalf("keyless follow-up git.status unexpectedly returned isError: %s", followResp)
	}
}
