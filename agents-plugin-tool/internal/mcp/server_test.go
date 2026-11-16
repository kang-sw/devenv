package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// TestMain defaults WS_RSRC_ROOT to the shipped rsrc tree so playbook and
// convention loads resolve bundled resources. Tests that exercise a custom
// rsrc tree override it per-test with t.Setenv.
//
// It also defaults WS_CACHE_HOME to a throwaway temp dir so the file-backed
// session store (sessions/<key>/ under the cache root) never reads or writes the
// developer's real ~/.cache during the suite. Tests that assert specific cache
// paths still override it per-test with t.Setenv (last write wins).
//
// WS_CONFIG_HOME always points at an empty temp dir, so tests that assert
// builtin defaults (agent tiers, worktree_pool) never read the developer's
// ~/.ws/config.json. It is unconditional: an inherited WS_CONFIG_HOME is a
// real config just the same. Tests that need a config home set their own.
func TestMain(m *testing.M) {
	if os.Getenv("WS_RSRC_ROOT") == "" {
		_ = os.Setenv("WS_RSRC_ROOT", shippedRsrcRootForTest())
	}
	os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
	if os.Getenv("WS_CACHE_HOME") == "" {
		dir, err := os.MkdirTemp("", "ws-mcp-test-cache-")
		if err != nil {
			fmt.Fprintf(os.Stderr, "TestMain: create temp cache home: %v\n", err)
			return 1
		}
		defer os.RemoveAll(dir)
		_ = os.Setenv("WS_CACHE_HOME", dir)
	}
	configHome, err := os.MkdirTemp("", "ws-mcp-test-config-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: create temp config home: %v\n", err)
		return 1
	}
	defer os.RemoveAll(configHome)
	_ = os.Setenv("WS_CONFIG_HOME", configHome)
	return m.Run()
}

func toolPropertiesByName(t *testing.T, listLine, toolName string) map[string]any {
	t.Helper()
	var listResp map[string]any
	if err := json.Unmarshal([]byte(listLine), &listResp); err != nil {
		t.Fatal(err)
	}
	result, _ := listResp["result"].(map[string]any)
	listedTools, _ := result["tools"].([]any)
	for _, rawTool := range listedTools {
		tool, _ := rawTool.(map[string]any)
		name, _ := tool["name"].(string)
		if name != toolName {
			continue
		}
		schema, _ := tool["inputSchema"].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		return properties
	}
	t.Fatalf("tools/list missing %s: %s", toolName, listLine)
	return nil
}

// toolEntryTextByName returns the marshaled JSON of the named tool's full
// tools/list entry (name, description, inputSchema), for substring checks
// against the description/schema text a client actually sees.
func toolEntryTextByName(t *testing.T, listLine, toolName string) string {
	t.Helper()
	var listResp map[string]any
	if err := json.Unmarshal([]byte(listLine), &listResp); err != nil {
		t.Fatal(err)
	}
	result, _ := listResp["result"].(map[string]any)
	listedTools, _ := result["tools"].([]any)
	for _, rawTool := range listedTools {
		tool, _ := rawTool.(map[string]any)
		name, _ := tool["name"].(string)
		if name != toolName {
			continue
		}
		entry, err := json.Marshal(tool)
		if err != nil {
			t.Fatal(err)
		}
		return string(entry)
	}
	t.Fatalf("tools/list missing %s: %s", toolName, listLine)
	return ""
}

func toolNameListed(t *testing.T, listLine, toolName string) bool {
	t.Helper()
	var listResp map[string]any
	if err := json.Unmarshal([]byte(listLine), &listResp); err != nil {
		t.Fatal(err)
	}
	result, _ := listResp["result"].(map[string]any)
	listedTools, _ := result["tools"].([]any)
	for _, rawTool := range listedTools {
		tool, _ := rawTool.(map[string]any)
		name, _ := tool["name"].(string)
		if name == toolName {
			return true
		}
	}
	return false
}

func canonicalTestPath(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	if evaluated, err := filepath.EvalSymlinks(abs); err == nil {
		abs = evaluated
	}
	return abs
}

func serveStdioWithSession(t *testing.T, server *Server, root, input string, out *bytes.Buffer) error {
	t.Helper()
	key, _ := parseLoginResponse(t, callLogin(t, server, 900001, root, nil))
	return server.ServeStdio(context.Background(), strings.NewReader(withSessionKeyInToolCalls(t, input, key)), out)
}

func withSessionKeyInToolCalls(t *testing.T, input, key string) string {
	t.Helper()
	var out []string
	for _, line := range strings.Split(strings.TrimRight(input, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("parse test MCP line: %v\n%s", err, line)
		}
		if payload["method"] == "tools/call" {
			params, _ := payload["params"].(map[string]any)
			name, _ := params["name"].(string)
			if name != "ferrule" {
				args, _ := params["arguments"].(map[string]any)
				if args == nil {
					args = map[string]any{}
					params["arguments"] = args
				}
				delete(args, "root")
				if _, exists := args["session_key"]; !exists {
					args["session_key"] = key
				}
			}
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal test MCP line: %v", err)
		}
		out = append(out, string(raw))
	}
	return strings.Join(out, "\n") + "\n"
}

func TestServeStdioDefaultsToLeadToolsWithoutRootAuthorityDetection(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	for _, name := range []string{"playbook.render", "git.commit", "config.list"} {
		if !strings.Contains(byID["1"], name) {
			t.Fatalf("default profile hid lead tool %s: %s", name, byID["1"])
		}
	}
}

func TestPromptRenderToolRemoved(t *testing.T) {
	for _, tc := range []struct {
		name      string
		noAgent   string
		namespace string
	}{
		{name: "full ws", noAgent: "", namespace: ""},
		{name: "wsflow", noAgent: "1", namespace: "wsflow"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useLeadProfile(t)
			root := t.TempDir()
			mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
			initGit(t, root)
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			t.Setenv("WS_MCP_NO_AGENT", tc.noAgent)
			t.Setenv("WS_MCP_NAMESPACE", tc.namespace)

			input := strings.Join([]string{
				`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
				`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"prompt.render","arguments":{"stem":"code-reviewer"}}}`,
			}, "\n") + "\n"

			var out bytes.Buffer
			if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
				t.Fatalf("ServeStdio returned error: %v", err)
			}
			byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))

			if toolNameListed(t, byID["1"], "prompt.render") {
				t.Fatalf("tools/list exposed removed prompt.render tool in %s mode: %s", tc.name, byID["1"])
			}

			var callResp struct {
				Error *struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(byID["2"]), &callResp); err != nil {
				t.Fatalf("unmarshal call response: %v", err)
			}
			if callResp.Error == nil {
				t.Fatalf("prompt.render in %s mode did not return JSON-RPC error: %s", tc.name, byID["2"])
			}
			if !strings.Contains(callResp.Error.Message, "prompt.render") {
				t.Fatalf("prompt.render error missing tool name: %s", callResp.Error.Message)
			}
		})
	}
}

func TestNamespaceTermsSubstitution(t *testing.T) {
	nonManglingCases := []struct {
		input string
		want  string
	}{
		{"use ws/tickets.query here", "use wsflow/tickets.query here"},
		{"call ws:lead-run skill", "call wsflow:lead-run skill"},
		{"rows: many items", "rows: many items"},
		{"news/feed here", "news/feed here"},
		{"workflows/steps", "workflows/steps"},
		{"newws/path", "newws/path"},
		{"ws/tool and ws:skill together", "wsflow/tool and wsflow:skill together"},
	}
	for _, tc := range nonManglingCases {
		got := wsNamespaceRef.ReplaceAllString(tc.input, "wsflow"+"$1")
		if got != tc.want {
			t.Errorf("wsNamespaceRef.ReplaceAllString(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}

	synth := wsNamespaceRef.ReplaceAllString("invoke ws:some-skill here", "wsflow"+"$1")
	if synth != "invoke wsflow:some-skill here" {
		t.Errorf("ws: substitution failed: %q", synth)
	}
}

// TestWsflowPlaybookRenderAllLegacyStemsFromRsrc verifies every legacy
// render-eligible stem still renders through wsflow playbook.render after the
// prompt.render MCP tool is removed.
func TestWsflowPlaybookRenderAllLegacyStemsFromRsrc(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_MCP_NO_AGENT", "1")
	t.Setenv("WS_MCP_NAMESPACE", "wsflow")
	t.Setenv("WS_RSRC_ROOT", shippedRsrcRootForTest())
	s := NewServer(root, "test")

	for _, stem := range []string{
		"reference-discovery", "code-reviewer",
	} {
		t.Run(stem, func(t *testing.T) {
			context := map[string]string{"bridge_probe": "context for " + stem}
			path, _, err := renderPlaybook(s, shippedRsrcRootForTest(), root, stem, context, wsconfig.Options{}, "", "", "", nil, "")
			if err != nil {
				t.Fatalf("renderPlaybook(%s): %v", stem, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read rendered %s: %v", stem, err)
			}
			text := string(data)
			if strings.TrimSpace(text) == "" {
				t.Fatalf("rendered %s is empty", stem)
			}
			if strings.Contains(text, "{{.") {
				t.Errorf("rendered %s has an unsubstituted placeholder:\n%s", stem, text)
			}
			if !strings.Contains(text, "## Render Context") || !strings.Contains(text, "- bridge_probe: context for "+stem) {
				t.Errorf("rendered %s did not append legacy context block:\n%s", stem, text)
			}
		})
	}
}

func TestServeStdioLogsCancellationNotificationsWhenEnabled(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	debugLog := filepath.Join(t.TempDir(), "mcp-debug.jsonl")
	t.Setenv("WS_MCP_DEBUG_LOG", debugLog)

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":"wait-1","method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"wait-1","reason":"user interrupt"}}`,
	}, "\n")

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected notification to produce no response, got %d lines\n%s", len(lines), out.String())
	}

	logBytes, err := os.ReadFile(debugLog)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logBytes)
	if !strings.Contains(logText, `"event":"request.received"`) ||
		!strings.Contains(logText, `"id":"wait-1"`) ||
		!strings.Contains(logText, `"event":"notification.cancelled"`) ||
		!strings.Contains(logText, `"request_id":"wait-1"`) ||
		!strings.Contains(logText, `"reason":"user interrupt"`) {
		t.Fatalf("debug log missing cancellation evidence:\n%s", logText)
	}
}

func TestServeStdioExposesCancellationNotificationsInDebugEvents(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":"wait-2","method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"wait-2","reason":"user interrupt"}}`,
		`{"jsonrpc":"2.0","id":"debug","method":"tools/call","params":{"name":"runtime.debug_events","arguments":{"limit":5}}}`,
	}, "\n")

	var out bytes.Buffer
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	debugText := toolText(t, byID["debug"])
	if !strings.Contains(debugText, `"event":"notification.cancelled"`) ||
		!strings.Contains(debugText, `"request_id":"wait-2"`) ||
		!strings.Contains(debugText, `"reason":"user interrupt"`) {
		t.Fatalf("runtime.debug_events missing cancellation evidence:\n%s", debugText)
	}
}

// TestKeyedScopeGatesRestrictedTools is the post-fold replacement for the old
// WS_MCP_TOOL_PROFILE-driven TestServeStdioFiltersToolsByProfile. The env profile
// no longer gates the served tool surface; tool-permission containment for a
// restricted scope flows entirely through the capability scope minted into a
// session key. This preserves the original intent (a leaf scope cannot reach
// restricted tools) against the keyed call gate, and confirms tools/list now
// advertises the full lead surface regardless of any restricted scope.
func TestKeyedScopeGatesRestrictedTools(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	leafKey, err := server.sessions.mint(root, roleLeaf, "")
	if err != nil {
		t.Fatalf("mint leaf key: %v", err)
	}

	// tools/list advertises the full lead surface (schema visibility is advisory;
	// the keyed call gate is the enforcement). Restricted tools remain visible.
	listResp := callToolsList(t, server)
	for _, name := range []string{"config.tune", "config.list", "runtime.read"} {
		if !strings.Contains(listResp, name) {
			t.Fatalf("tools/list must advertise full lead surface, missing %s: %s", name, listResp)
		}
	}
	if strings.Contains(listResp, "ws.setup") {
		t.Fatalf("tools/list exposed deleted setup mutation tool: %s", listResp)
	}

	// A leaf-scoped key is rejected by the keyed gate for restricted tools.
	deniedTier := callToolOnce(t, server, 3, "config.tune", map[string]any{
		"session_key": leafKey,
		"key":         "agents.tier",
		"value":       map[string]any{"tier": "small", "model": "gpt-5.2"},
	})
	if !strings.Contains(deniedTier, "tool not available") {
		t.Fatalf("leaf key not rejected for config.tune: %s", deniedTier)
	}
	deniedShow := callToolOnce(t, server, 4, "config.list", map[string]any{
		"session_key": leafKey,
	})
	if !strings.Contains(deniedShow, "tool not available") {
		t.Fatalf("leaf key not rejected for config.list: %s", deniedShow)
	}

	// Read-only runtime/cache discovery is permitted for a leaf scope.
	allowedInfo := callToolOnce(t, server, 5, "runtime.read", map[string]any{
		"session_key": leafKey,
	})
	if !strings.Contains(allowedInfo, "version:") {
		t.Fatalf("leaf key wrongly rejected runtime.read: %s", allowedInfo)
	}
	allowedAPIList := callToolOnce(t, server, 6, "api.list", map[string]any{
		"session_key": leafKey,
	})
	if strings.Contains(allowedAPIList, "tool not available") {
		t.Fatalf("leaf key wrongly rejected api.list: %s", allowedAPIList)
	}
}

// TestExplicitAllowedToolsCannotBypassEffectiveRole verifies that the
// WS_MCP_ALLOWED_TOOLS visibility allowlist cannot regain a tool that the keyed
// capability scope denies. After the WS_MCP_TOOL_PROFILE fold, the allowlist is a
// schema-visibility filter only; the keyed call gate remains the role authority,
// so an allowlisted-but-scope-denied tool is still rejected on call.
func TestExplicitAllowedToolsCannotBypassEffectiveRole(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_MCP_ALLOWED_TOOLS", "runtime.read,config.list")

	server := NewServer(root, "test")
	leafKey, err := server.sessions.mint(root, roleLeaf, "")
	if err != nil {
		t.Fatalf("mint leaf key: %v", err)
	}

	// The allowlist constrains tools/list visibility (only allowlisted tools show).
	listResp := callToolsList(t, server)
	if !strings.Contains(listResp, "runtime.read") {
		t.Fatalf("allowlist hid allowlisted runtime.read: %s", listResp)
	}

	// runtime.read is both allowlisted and scope-permitted: it is callable.
	allowedInfo := callToolOnce(t, server, 2, "runtime.read", map[string]any{
		"session_key": leafKey,
	})
	if !strings.Contains(allowedInfo, "version:") {
		t.Fatalf("allowlist+leaf wrongly rejected runtime.read: %s", allowedInfo)
	}

	// config.list is allowlisted but DENIED by the leaf scope. The keyed gate
	// must still reject it — the allowlist cannot regain it.
	deniedShow := callToolOnce(t, server, 4, "config.list", map[string]any{
		"session_key": leafKey,
	})
	if !strings.Contains(deniedShow, "tool not available") {
		t.Fatalf("allowlist let leaf-denied config.list through: %s", deniedShow)
	}
}

func useLeadProfile(t *testing.T) {
	t.Helper()
	t.Setenv("WS_MCP_ALLOWED_TOOLS", "")
}

func mustWrite(t *testing.T, root, rel, text string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertStringAbsentFromTree(t *testing.T, root, needle string) {
	t.Helper()
	if strings.TrimSpace(root) == "" {
		return
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > 1<<20 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), needle) {
			t.Fatalf("found persisted setup root %q in %s", needle, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func snapshotTreeForTest(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return snapshot
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.IsDir() {
			snapshot[rel] = "<dir>"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot[rel] = string(data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func initGit(t *testing.T, root string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, string(out))
	}
	runGit(t, root, "config", "core.autocrlf", "false")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
}

// headBranch resolves the current branch name of a test repo. git.commit now
// requires an expected_branch argument, and `git init` picks its default branch
// from the test runner's init.defaultBranch config (main or master), so tests
// resolve it here rather than hard-coding a branch into the tool arguments.
func headBranch(t *testing.T, root string) string {
	t.Helper()
	return strings.TrimSpace(string(runGitOutput(t, root, "symbolic-ref", "--short", "HEAD")))
}

func initTicketRepo(t *testing.T, stem string) string {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, root, filepath.Join("ai-docs/tickets/todo", stem+".md"), "---\ntitle: Demo\n---\n# Demo\n")
	initGit(t, root)
	return root
}

func TestServeStdioTicketToolsRejectSpecStemArgument(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/todo/260504-demo.md", "---\ntitle: Demo\n---\n# Demo\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"tickets.query","arguments":{"spec_stem":"260504-demo"}}}` + "\n"

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	text := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["1"])
	if !strings.Contains(text, "ticket_stem") || !strings.Contains(out.String(), `"isError":true`) {
		t.Fatalf("tickets.query accepted spec_stem argument: %s", out.String())
	}
}

// TestServeStdioTicketsQueryPointResolveMatchesRemovedStatusShape pins the
// 260903 collapse's byte-identical requirement for the point-resolve call
// shape (ticket_stem alone, no query/mentions_ticket_stem/statuses): it must
// error on a not-found stem exactly like the removed tickets.status did,
// rather than silently succeeding with TicketsFind's empty-array discovery
// shape, and a found stem's JSON must be a bare object, not an array.
func TestServeStdioTicketsQueryPointResolveMatchesRemovedStatusShape(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/todo/260504-demo.md", "---\ntitle: Demo\n---\n# Demo\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	notFoundInput := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"tickets.query","arguments":{"ticket_stem":"260504-missing"}}}` + "\n"
	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := serveStdioWithSession(t, server, root, notFoundInput, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	if !strings.Contains(out.String(), `"isError":true`) || !strings.Contains(out.String(), "ticket not found: 260504-missing") {
		t.Fatalf("tickets.query(ticket_stem:) not-found case did not error like the removed tickets.status: %s", out.String())
	}

	jsonInput := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"tickets.query","arguments":{"ticket_stem":"260504-demo","format":"json"}}}` + "\n"
	out.Reset()
	if err := serveStdioWithSession(t, server, root, jsonInput, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	text := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["2"])
	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		t.Fatalf("tickets.query(ticket_stem:) json mode is not a bare object like the removed tickets.status: %v\n%s", err, text)
	}
	if obj["stem"] != "260504-demo" {
		t.Fatalf("tickets.query(ticket_stem:) json object missing expected stem: %s", text)
	}
}

func TestTicketsQueryPointResolveIncludesTerminalStatuses(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	for _, status := range []string{".done", ".dropped"} {
		mustWrite(t, root, "ai-docs/tickets/"+status+"/260504-"+status[1:]+".md", "---\ntitle: Terminal\n---\n# Terminal\n")
	}
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	for _, status := range []string{".done", ".dropped"} {
		stem := "260504-" + status[1:]
		for _, format := range []string{"", "json"} {
			args := map[string]any{"ticket_stem": stem, "format": format, "include_done": false, "include_dropped": false}
			got := callScopedTool(t, root, 1, "tickets.query", args)
			if !strings.Contains(got, stem) || !strings.Contains(got, status) {
				t.Fatalf("point resolve %s (%s): %s", status, format, got)
			}
		}
	}
	for _, args := range []map[string]any{{}, {"query": "Terminal"}, {"statuses": []string{"todo"}, "ticket_stem": "260504-done"}} {
		got := callScopedTool(t, root, 1, "tickets.query", args)
		if strings.Contains(got, "260504-done") || strings.Contains(got, "260504-dropped") {
			t.Fatalf("discovery included terminal ticket by default: %s", got)
		}
	}
}

// callToolsList issues a single tools/list request and returns the raw response
// line. Used by capability-scope tests to assert advertised schema visibility.
func callToolsList(t *testing.T, server *Server) string {
	t.Helper()
	line := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(line), &out); err != nil {
		t.Fatalf("ServeStdio error: %v", err)
	}
	return strings.TrimSpace(out.String())
}

func TestPathGenerateAdvertisesAndAllocatesPlanPaths(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "README.md", "# Test\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	listResp := callToolsList(t, server)
	kindProperty, _ := toolPropertiesByName(t, listResp, "path.generate")["kind"].(map[string]any)
	enumValues, _ := kindProperty["enum"].([]any)
	for _, want := range []string{"review", "prompt", "plan"} {
		found := false
		for _, raw := range enumValues {
			if raw == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("path.generate kind enum missing %q: %s", want, listResp)
		}
	}

	key, err := server.sessions.mint(canonicalRootForTest(t, root), roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	callResp := callToolOnce(t, server, 2, "path.generate", map[string]any{
		"kind":        "plan",
		"stems":       []string{"../bad stem"},
		"session_key": key,
	})
	text := strings.TrimSpace(toolText(t, callResp))
	canonicalText := canonicalTestPath(t, text)
	wantDir := canonicalTestPath(t, filepath.Join(root, "ai-docs", ".plans"))
	if !strings.HasPrefix(canonicalText, wantDir+string(os.PathSeparator)) {
		t.Fatalf("plan path %q is not under %q", text, wantDir)
	}
	if !regexp.MustCompile(`[0-9]{4}-[0-9]{2}` + regexp.QuoteMeta(string(os.PathSeparator)) + `[0-9]{2}-[0-9]{4}-bad-stem\.md$`).MatchString(canonicalText) {
		t.Fatalf("plan path %q does not match expected timestamped shape", text)
	}
	if info, err := os.Stat(text); err != nil || info.IsDir() {
		t.Fatalf("reserved plan path %q stat=%v err=%v", text, info, err)
	}
}

func responseLinesByID(t *testing.T, lines []string) map[string]string {
	t.Helper()
	byID := make(map[string]string, len(lines))
	for _, line := range lines {
		var resp struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatal(err)
		}
		byID[rawIDForTest(t, resp.ID)] = line
	}
	return byID
}

func rawIDForTest(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		return strconv.Itoa(number)
	}
	return string(raw)
}

func TestServeStdioPingPreservesIDsAndIsNotAdvertised(t *testing.T) {
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":17,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":"ping-string-id","method":"ping"}`,
		`{"jsonrpc":"2.0","id":18,"method":"tools/list","params":{}}`,
	}, "\n")

	var out bytes.Buffer
	if err := NewServer(t.TempDir(), "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 responses, got %d\n%s", len(lines), out.String())
	}
	byID := responseLinesByID(t, lines)
	for _, test := range []struct {
		lookupID string
		rawID    string
	}{
		{lookupID: "17", rawID: `17`},
		{lookupID: "ping-string-id", rawID: `"ping-string-id"`},
	} {
		id := test.lookupID
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal([]byte(byID[id]), &envelope); err != nil {
			t.Fatalf("decode ping response for id %q: %v", id, err)
		}
		if len(envelope) != 3 {
			t.Fatalf("ping response for id %q has unexpected fields: %s", id, byID[id])
		}
		if string(envelope["jsonrpc"]) != `"2.0"` {
			t.Fatalf("ping response for id %q has unexpected jsonrpc: %s", id, byID[id])
		}
		if string(envelope["id"]) != test.rawID {
			t.Fatalf("ping response did not preserve raw id %s: %s", test.rawID, byID[id])
		}
		if string(envelope["result"]) != `{}` {
			t.Fatalf("ping response for id %q did not return an empty object: %s", id, byID[id])
		}
	}
	if toolNameListed(t, byID["18"], "ping") {
		t.Fatalf("tools/list advertised base-protocol ping as a tool: %s", byID["18"])
	}
}

func toolIsError(t *testing.T, line string) bool {
	t.Helper()
	var resp struct {
		Result struct {
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Result.IsError
}

func toolText(t *testing.T, line string) string {
	t.Helper()
	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Result.Content) != 1 {
		t.Fatalf("unexpected content in response: %s", line)
	}
	return resp.Result.Content[0].Text
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	_ = runGitOutput(t, root, args...)
}

func runGitOutput(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return out
}

func TestServeStdioToolsListAndCall(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260503-feat-demo.md", "---\ntitle: Demo ticket\n---\n# Demo\n\nMentions 260503-epic-demo.\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"project_tree","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"infra.read","arguments":{"name":"impl-playbook"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"path.generate","arguments":{"kind":"review","stems":["direct"]}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"runtime.read","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"git.status","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"runtime.debug_events","arguments":{"limit":10}}}`,
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"config.list","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"tickets.query","arguments":{"mentions_ticket_stem":"260503-epic-demo"}}}`,
	}, "\n")

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 10 {
		t.Fatalf("expected 10 responses, got %d\n%s", len(lines), out.String())
	}
	byID := responseLinesByID(t, lines)

	var listResp map[string]any
	if err := json.Unmarshal([]byte(byID["2"]), &listResp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(byID["2"], "project_tree") {
		t.Fatalf("tools/list missing project_tree: %s", byID["2"])
	}
	if strings.Contains(byID["2"], "subquery") {
		t.Fatalf("tools/list still advertises removed subquery tool: %s", byID["2"])
	}
	toolsResult, _ := listResp["result"].(map[string]any)
	listedTools, _ := toolsResult["tools"].([]any)
	loginProperties := toolPropertiesByName(t, byID["2"], "ferrule")
	if _, ok := loginProperties["root"]; !ok {
		t.Fatalf("ws.ferrule schema missing root bootstrap parameter: %s", byID["2"])
	}
	// Tools that may advertise a "root" bootstrap parameter: ws.ferrule (the
	// primary session-bootstrap tool) and ws.workflow_manual (lead-only fresh-start
	// shortcut that mints a key inline when root is supplied alongside the sentinel).
	rootParamAllowed := map[string]bool{
		"ferrule":         true,
		"workflow_manual": true,
	}
	for _, rawTool := range listedTools {
		tool, _ := rawTool.(map[string]any)
		name, _ := tool["name"].(string)
		if rootParamAllowed[name] {
			continue
		}
		schema, _ := tool["inputSchema"].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		if _, ok := properties["root"]; ok {
			t.Fatalf("non-login tool %s publicly advertises root in schema: %s", name, byID["2"])
		}
		if name == "review.marker" || name == "review.stamp" {
			key, _ := properties["session_key"].(map[string]any)
			if key["type"] != "string" {
				t.Fatalf("%s session_key schema = %v, want string", name, key)
			}
			want := []string{"session_key"}
			if name == "review.stamp" {
				want = []string{"base", "head", "verdict", "session_key"}
			}
			required, _ := schema["required"].([]any)
			if len(required) != len(want) {
				t.Fatalf("%s required = %v, want %v", name, required, want)
			}
			for i, field := range want {
				if required[i] != field {
					t.Fatalf("%s required = %v, want %v", name, required, want)
				}
			}
		}
	}
	rootAwareTools := []string{
		"api.list",
		"git.status", "git.diff", "git.log", "git.merge_base", "git.commit",
		"project_tree",
		"review.marker", "review.stamp",
		"tickets.query", "path.generate", "playbook.render",
	}
	for _, name := range rootAwareTools {
		properties := toolPropertiesByName(t, byID["2"], name)
		if _, ok := properties["session_key"]; !ok {
			t.Fatalf("root-aware tool %s schema missing session_key: %s", name, byID["2"])
		}
	}
	if !strings.Contains(byID["2"], "path.generate") {
		t.Fatalf("tools/list missing path.generate: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "runtime.read") {
		t.Fatalf("tools/list missing runtime.read: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "ferrule") {
		t.Fatalf("tools/list missing ws.ferrule: %s", byID["2"])
	}
	if strings.Contains(byID["2"], "session.set_default_root") || strings.Contains(byID["2"], "session.get_default_root") {
		t.Fatalf("tools/list still advertises session root compatibility tools: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "config.tune") {
		t.Fatalf("tools/list missing config.tune: %s", byID["2"])
	}
	// The generic config.tune advertises the key-addressed write contract; the
	// per-key value schema (agents.tier tier/effort domains) is now discoverable
	// at runtime through config.list's catalog rather than the static tools/list
	// schema (260814 Phase 2 collapse).
	if !strings.Contains(byID["2"], `"key"`) || !strings.Contains(byID["2"], "agents.tier") {
		t.Fatalf("tools/list config.tune schema missing key-addressed write contract: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "config.list") {
		t.Fatalf("tools/list missing config.list: %s", byID["2"])
	}
	for _, tool := range []string{"git.status", "git.diff", "git.log", "git.merge_base", "git.commit", "tickets.query"} {
		if !strings.Contains(byID["2"], tool) {
			t.Fatalf("tools/list missing %s: %s", tool, byID["2"])
		}
	}
	// The spec and mental-model tool surface retired with its layer, and
	// git.commit's doc trailers went with it. tools/list must advertise none
	// of them, or an agent calls a tool that no longer dispatches.
	for _, gone := range []string{"spec_stem.generate", "spec_index.verify", "specs.query", "mental_models.list", "mental_models.query", "mental_models.status", "references.trace", `"mental_model_notes"`, `"updated_specs"`, `"updated_mental_models"`} {
		if strings.Contains(byID["2"], gone) {
			t.Fatalf("tools/list still advertises retired surface %s: %s", gone, byID["2"])
		}
	}
	// exec.* tools are permanently hidden from the public MCP surface in all modes.
	for _, execTool := range []string{"exec.spawn", "exec.shell", "exec.status", "exec.result", "exec.abort", "exec.raw.tail", "exec.raw.read", "exec.raw.grep"} {
		if strings.Contains(byID["2"], execTool) {
			t.Fatalf("tools/list should not advertise exec tool %s: %s", execTool, byID["2"])
		}
	}
	if !strings.Contains(byID["3"], "tickets:") {
		t.Fatalf("project_tree response missing tickets: %s", byID["3"])
	}
	if text := toolText(t, byID["4"]); strings.TrimSpace(text) == "" || toolIsError(t, byID["4"]) {
		t.Fatalf("infra response failed to load impl-playbook: %s", byID["4"])
	}
	if !strings.Contains(byID["5"], "review-paths") || !strings.Contains(byID["5"], "-direct.md") {
		t.Fatalf("path.generate response missing review path: %s", byID["5"])
	}
	if !strings.Contains(byID["6"], "version:") || !strings.Contains(byID["6"], "source_commit:") {
		t.Fatalf("runtime.read response missing version/source_commit: %s", byID["6"])
	}
	if !strings.Contains(toolText(t, byID["7"]), "dirty:") || !strings.Contains(toolText(t, byID["7"]), "ai-docs/") {
		t.Fatalf("git.status response missing readable status: %s", byID["7"])
	}
	if !strings.Contains(toolText(t, byID["8"]), `"event":"request.received"`) {
		t.Fatalf("runtime.debug_events missing request evidence: %s", byID["8"])
	}
	configText := toolText(t, byID["9"])
	if !strings.Contains(configText, "path:") || !strings.Contains(configText, `config.json`) || !strings.Contains(configText, "model_aliases:") {
		t.Fatalf("config.list response missing path/config: %s", byID["9"])
	}
	ticketsText := toolText(t, byID["10"])
	if !strings.Contains(ticketsText, "260503-feat-demo") || !strings.Contains(ticketsText, "mentions_ticket_stem") {
		t.Fatalf("tickets.query response missing mention result: %s", byID["10"])
	}
}

func TestServeStdioNoAgentModeHidesAgentBackedTools(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_MCP_NO_AGENT", "1")
	t.Setenv("WS_MCP_NAMESPACE", "wsflow")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"api.list","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"exec.spawn","arguments":{"cmd":"true"}}}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	list := byID["1"]
	// config.tune/config.list are generic and always visible; the no-agent cut
	// for a full-ws-only knob lives per-key inside config.tune's dispatch and
	// config.list's catalog, not in a hidden per-knob tool.
	for _, hidden := range []string{"exec.spawn", "subquery"} {
		if strings.Contains(list, hidden) {
			t.Fatalf("tools/list exposed hidden no-agent tool %s: %s", hidden, list)
		}
	}
	for _, visible := range []string{"api.list", "config.list", "config.tune", "config.resolve_agent", "tickets.query", "playbook.read", "playbook.render"} {
		if !strings.Contains(list, visible) {
			t.Fatalf("tools/list missing no-agent visible tool %s: %s", visible, list)
		}
	}
	if !strings.Contains(list, "wsflow") {
		t.Fatalf("tools/list did not use namespace override in descriptions: %s", list)
	}
	if strings.Contains(list, "Full ws") || strings.Contains(list, "full ws") {
		t.Fatalf("tools/list retained full-ws-only playbook wording in wsflow mode: %s", list)
	}
	if strings.Contains(list, "[ws note") {
		t.Fatalf("tools/list named the ws product in the peer-lead stamp in wsflow mode: %s", list)
	}
	if toolIsError(t, byID["2"]) {
		t.Fatalf("api.list should remain callable in no-agent mode: %s", byID["2"])
	}
	if !strings.Contains(byID["3"], "wsflow agentless mode disables agent-backed tool: exec.spawn") {
		t.Fatalf("hidden tool did not return clear no-agent error: %s", byID["3"])
	}
}

func TestWsflowModePlaybookRenderAbsorbsPromptRenderContext(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_MCP_NO_AGENT", "1")
	t.Setenv("WS_MCP_NAMESPACE", "wsflow")
	t.Setenv("WS_RSRC_ROOT", shippedRsrcRootForTest())

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"playbook.render","arguments":{"name":"code-reviewer","context":{"reviewer_scope":"correctness only","note":"see ws/tickets.query for details"}}}}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !toolNameListed(t, byID["1"], "playbook.render") {
		t.Fatalf("wsflow tools/list missing playbook.render: %s", byID["1"])
	}
	if toolNameListed(t, byID["1"], "prompt.render") {
		t.Fatalf("wsflow tools/list exposed removed prompt.render: %s", byID["1"])
	}

	if toolIsError(t, byID["2"]) {
		t.Fatalf("playbook.render code-reviewer returned error: %s", byID["2"])
	}
	codeReviewerPath := strings.SplitN(strings.TrimSpace(toolText(t, byID["2"])), "\n", 2)[0]
	codeReviewerData, err := os.ReadFile(codeReviewerPath)
	if err != nil {
		t.Fatalf("read code-reviewer render: %v", err)
	}
	codeReviewerText := string(codeReviewerData)
	for _, want := range []string{"wsflow/", "## Render Context", "- note: see ws/tickets.query for details", "- reviewer_scope: correctness only"} {
		if !strings.Contains(codeReviewerText, want) {
			t.Fatalf("code-reviewer playbook render missing %q:\n%s", want, codeReviewerText)
		}
	}
}

func TestPlaybookRenderReturnsResolvedNativeBindings(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "global"))
	root := initGitRepo(t)
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"model-pb/model-pb.md": modelAliasPlaybookContent,
	})
	cacheHome := t.TempDir()
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)
	t.Setenv("WS_CACHE_HOME", cacheHome)

	s := NewServer(root, "test")
	s.observeHarness("test", "codex")
	leadKey, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))
	rendered := callToolOnce(t, s, 2, "playbook.render", map[string]any{"name": "model-pb", "session_key": leadKey})
	if toolIsError(t, rendered) {
		t.Fatalf("playbook.render returned error: %s", rendered)
	}
	responseText := toolText(t, rendered)
	lines := strings.Split(strings.TrimSpace(responseText), "\n")
	if len(lines) != 4 || !strings.HasSuffix(lines[0], "-model-pb.md") {
		t.Fatalf("render metadata path/tier shape = %q", responseText)
	}
	if got, want := strings.Join(lines[1:], "\n"), "recommended-tier: medium\nrecommended-model: gpt-6-luna\nrecommended-reasoning-effort: xhigh"; got != want {
		t.Fatalf("render metadata = %q, want %q", got, want)
	}

	printed := callToolOnce(t, s, 3, "playbook.read", map[string]any{"name": "model-pb", "session_key": leadKey})
	if toolIsError(t, printed) {
		t.Fatalf("playbook.read returned error: %s", printed)
	}
	if got := toolText(t, printed); strings.Contains(got, "recommended-model:") || strings.Contains(got, "recommended-reasoning-effort:") || !strings.HasSuffix(strings.TrimSpace(got), "recommended-tier: medium") {
		t.Fatalf("playbook.read contract changed: %q", got)
	}
}

func TestWsflowConfigAgentsTierWriteRoundTripsIntoPlaybookRenderRecommendedModel(t *testing.T) {
	useLeadProfile(t)
	root := initGitRepo(t)
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"model-pb/model-pb.md": modelAliasPlaybookContent,
	})
	cacheHome := t.TempDir()
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)
	t.Setenv("WS_CACHE_HOME", cacheHome)
	t.Setenv(envNoAgent, "1")
	t.Setenv(envNamespace, "wsflow")

	s := NewServer(root, "test")
	s.observeHarness("test", "codex")
	leadKey, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))

	written := callToolOnce(t, s, 2, "config.tune", map[string]any{
		"key":     "agents.tier",
		"harness": "codex",
		"value": map[string]any{
			"tier":   "medium",
			"model":  "gpt-5.7-wsflow-roundtrip",
			"effort": "xhigh",
		},
	})
	if toolIsError(t, written) {
		t.Fatalf("wsflow config.tune agents.tier write returned error: %s", written)
	}

	rendered := callToolOnce(t, s, 3, "playbook.render", map[string]any{"name": "model-pb", "session_key": leadKey})
	if toolIsError(t, rendered) {
		t.Fatalf("wsflow playbook.render returned error: %s", rendered)
	}
	responseText := toolText(t, rendered)
	if !strings.Contains(responseText, "recommended-model: gpt-5.7-wsflow-roundtrip") {
		t.Fatalf("playbook.render recommended-model did not reflect wsflow config.agents_tier write:\n%s", responseText)
	}
	if !strings.Contains(responseText, "recommended-reasoning-effort: xhigh") {
		t.Fatalf("playbook.render recommended-reasoning-effort did not reflect wsflow config.agents_tier write:\n%s", responseText)
	}
}

// TestServeStdioDoesNotBlockToolsListBehindLongCall pins that ServeStdio
// dispatches concurrently: a tools/call that blocks (here exec.result waiting
// on a still-running job) must not delay a tools/list that arrives after it.
// A serialized dispatch loop would answer id 1 first and stall the client.
func TestServeStdioDoesNotBlockToolsListBehindLongCall(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	var spawnOut bytes.Buffer
	spawnServer := NewServer(root, "test")
	// This test's own pass condition depends on the spawned job still being
	// "running" when the exec.result call below (timeout_seconds:2) is
	// issued, not just on a nominal duration: if the job finishes first,
	// exec.result returns immediately and can win the first-output-line race
	// against tools/list, flipping the assertion below for a reason unrelated
	// to the dispatch-ordering behavior this test pins. mcpLongShellArgs's
	// ~6s nominal duration leaves under ~1s of slack after the 5s
	// ForegroundWindow launch plus this function's own pipe/session setup, so
	// use mcpAbortShellArgs's much longer duration instead for a wide margin
	// (same rationale as its doc comment). Reap it via t.Cleanup — LIFO,
	// registered after the WS_CACHE_HOME t.Setenv above so it runs first —
	// so the still-running job doesn't race root's t.TempDir() RemoveAll on
	// Windows (see reapExecKeys and TestExecMCPRunningLargeAndAbort).
	if err := serveStdioWithSession(t, spawnServer, root, toolCallLine(t, 1, "exec.shell", mcpAbortShellArgs())+"\n", &spawnOut); err != nil {
		t.Fatal(err)
	}
	running := execKeyFromText(t, toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(spawnOut.String()), "\n"))["1"]))
	t.Cleanup(func() { reapExecKeys(t, spawnServer, root, []string{running}) })

	reader, writer := io.Pipe()
	outReader, outWriter := io.Pipe()
	done := make(chan error, 1)
	streamServer := NewServer(root, "test")
	go func() {
		done <- streamServer.ServeStdio(context.Background(), reader, outWriter)
		_ = outWriter.Close()
	}()

	key, _ := parseLoginResponse(t, callLogin(t, streamServer, 900002, root, nil))
	fmt.Fprintln(writer, fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"exec.result","arguments":{"exec_key":%q,"timeout_seconds":2,"session_key":%q}}}`, running, key))
	fmt.Fprintln(writer, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)

	lineCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(outReader)
		// The tools/list line is ~60 KB, near the default 64 KB token cap;
		// past it the scanner stops and the test would misreport a block.
		scanner.Buffer(nil, 16<<20)
		for scanner.Scan() {
			select {
			case lineCh <- scanner.Text():
			default:
			}
		}
	}()
	select {
	case line := <-lineCh:
		if !strings.Contains(line, `"id":2`) || !strings.Contains(line, "tools") {
			t.Fatalf("first response was not tools/list while the long call was running: %s", line)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("tools/list was blocked behind a long-running tools/call")
	}
	_ = writer.Close()
	_ = reader.Close()
	// ServeStdio exits only after the in-flight exec.result returns: up to
	// its 2s timeout plus a final poll of the job record (file I/O). A loaded
	// Windows runner overran a 3s bound here with this path unchanged, so the
	// bound only has to tell a slow exit from one that never comes.
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("ServeStdio did not exit after input close")
	}
}

func TestSubqueryToolRemovedFromListAndCallRejected(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"subquery","arguments":{"question":"where is X?"}}}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if strings.Contains(byID["1"], `"subquery"`) {
		t.Fatalf("tools/list still advertises subquery: %s", byID["1"])
	}
	if !strings.Contains(byID["2"], "not available") && !strings.Contains(byID["2"], "unknown") {
		t.Fatalf("subquery tools/call was not rejected: %s", byID["2"])
	}
}

func TestAPIListDomains(t *testing.T) {
	root := t.TempDir()
	useLeadProfile(t)
	initGit(t, root)
	server := NewServer(root, "test")
	domains, err := apiListDomains(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 0 {
		t.Fatalf("empty deps domains = %v", domains)
	}
	mustWrite(t, root, "ai-docs/.deps/go/README.md", "go")
	mustWrite(t, root, "ai-docs/.deps/.hidden/README.md", "hidden")
	mustWrite(t, root, "ai-docs/.deps/python/README.md", "python")
	mustWrite(t, root, "ai-docs/.deps/file.txt", "not dir")
	before := snapshotTreeForTest(t, filepath.Join(root, "ai-docs", ".deps"))

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"api.list","arguments":{}}}` + "\n"
	var out bytes.Buffer
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	got := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["1"])
	if got != "go\npython\n" {
		t.Fatalf("api.list = %q", got)
	}
	if after := snapshotTreeForTest(t, filepath.Join(root, "ai-docs", ".deps")); !reflect.DeepEqual(after, before) {
		t.Fatalf("api.list mutated ai-docs/.deps: before=%v after=%v", before, after)
	}
}

func TestAgentBackedAPIToolsRemovedFromMCP(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	server := NewServer(root, "test")
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"api.ask","arguments":{"prompt":"How do modules work?"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"api.ask_async","arguments":{"prompt":"How do modules work?"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"api.status","arguments":{"api_job_key":"job"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"api.result","arguments":{"api_job_key":"job"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"api.cancel","arguments":{"api_job_key":"job"}}}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !strings.Contains(byID["1"], "api.list") {
		t.Fatalf("tools/list missing retained api.list: %s", byID["1"])
	}
	for _, removed := range []string{"api.ask", "api.ask_async", "api.status", "api.result", "api.cancel"} {
		if strings.Contains(byID["1"], removed) {
			t.Fatalf("tools/list still advertises removed tool %s: %s", removed, byID["1"])
		}
	}
	for id := 2; id <= 6; id++ {
		line := byID[strconv.Itoa(id)]
		if !strings.Contains(line, `"code":-32602`) || !strings.Contains(line, "unknown tool") {
			t.Fatalf("removed api tool call did not fail as unknown tool: %s", line)
		}
	}
}

func TestExecToolsListNoAgentAndMCPFlow(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "README.md", "x\n")
	mustWrite(t, root, "sub/.keep", "x\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		toolCallLine(t, 2, "exec.spawn", map[string]any{"cmd": os.Args[0], "args": []string{"-test.run=TestMCPExecHelperProcess", "--", "flow"}, "env": map[string]string{"GO_WANT_MCP_EXEC_HELPER": "1"}, "working_dir": "sub"}),
		toolCallLine(t, 9, "exec.shell", mcpShellShapeArgs()),
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
		t.Fatal(err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	// exec.* tools are permanently hidden from the public MCP surface in all modes.
	for _, name := range []string{"exec.spawn", "exec.shell", "exec.status", "exec.result", "exec.abort", "exec.raw.tail", "exec.raw.read", "exec.raw.grep"} {
		if strings.Contains(byID["1"], name) {
			t.Fatalf("tools/list should not advertise exec tool %s: %s", name, byID["1"])
		}
	}
	text := toolText(t, byID["2"])
	shellText := toolText(t, byID["9"])
	if !strings.Contains(shellText, "shell-shape") || !containsResolvedPath(shellText, root) {
		t.Fatalf("shell response = %s", byID["9"])
	}
	if strings.HasPrefix(strings.TrimSpace(text), "{") || !strings.Contains(text, "status: succeeded") || !containsResolvedPath(text, filepath.Join(root, "sub")) || !strings.Contains(text, "========== stdout ==========") || !strings.Contains(text, "========== stderr ==========") || !strings.Contains(text, "err") {
		t.Fatalf("spawn response = %s", byID["2"])
	}
	launch := execToolResponse{ExecKey: execKeyFromText(t, text)}

	input = strings.Join([]string{
		fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"exec.status","arguments":{"exec_key":%q}}}`, launch.ExecKey),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"exec.result","arguments":{"exec_key":%q}}}`, launch.ExecKey),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"exec.raw.tail","arguments":{"exec_key":%q,"stream":"stdout","lines":1}}}`, launch.ExecKey),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"exec.raw.read","arguments":{"exec_key":%q,"stream":"stderr","offset":0,"limit":20}}}`, launch.ExecKey),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"exec.raw.grep","arguments":{"exec_key":%q,"pattern":"beta42"}}}`, launch.ExecKey),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"exec.raw.grep","arguments":{"exec_key":%q,"pattern":"beta[0-9]+","regex":true}}}`, launch.ExecKey),
	}, "\n") + "\n"
	out.Reset()
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
		t.Fatal(err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	for id, want := range map[string]string{"3": "result_ready:", "4": "========== stdout ==========", "5": "beta42", "6": "next_offset:", "7": "beta42", "8": "beta42"} {
		if !strings.Contains(byID[id], want) {
			t.Fatalf("response %s missing %s: %s", id, want, byID[id])
		}
	}
	tailText := toolText(t, byID["5"])
	if strings.HasPrefix(strings.TrimSpace(tailText), "{") || !strings.Contains(tailText, "exec_key: ") || !strings.Contains(tailText, "stream: stdout") || !strings.Contains(tailText, "========== text ==========") {
		t.Fatalf("tail response was not readable text: %s", byID["5"])
	}
	readText := toolText(t, byID["6"])
	if strings.HasPrefix(strings.TrimSpace(readText), "{") || !strings.Contains(readText, "stream: stderr") || !strings.Contains(readText, "offset: 0") || !strings.Contains(readText, "========== text ==========") {
		t.Fatalf("read response was not readable text: %s", byID["6"])
	}
	for _, id := range []string{"7", "8"} {
		grepText := toolText(t, byID[id])
		if strings.HasPrefix(strings.TrimSpace(grepText), "{") || !strings.Contains(grepText, "stream: stdout") || !strings.Contains(grepText, "matches: 1") || !strings.Contains(grepText, "========== matches ==========") {
			t.Fatalf("grep response %s was not readable text: %s", id, byID[id])
		}
	}

	t.Setenv("WS_MCP_NO_AGENT", "1")
	t.Setenv("WS_MCP_NAMESPACE", "wsflow")
	out.Reset()
	input = `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"exec.spawn","arguments":{"cmd":"echo"}}}` + "\n" + `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"exec.raw.tail","arguments":{"exec_key":"exec-1-0000000000000000"}}}` + "\n"
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
		t.Fatal(err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if strings.Contains(byID["1"], "exec.spawn") || strings.Contains(byID["1"], "exec.raw.tail") {
		t.Fatalf("no-agent listed exec tools: %s", byID["1"])
	}
	if !strings.Contains(byID["2"], "agentless mode disables") || !strings.Contains(byID["3"], "agentless mode disables") {
		t.Fatalf("no-agent calls not rejected: %s\n%s", byID["2"], byID["3"])
	}
}

func TestExecMCPRunningLargeAndAbort(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "README.md", "x\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	var execKeys []string
	// t.Cleanup is LIFO across ALL cleanups registered in this test, not just
	// t.TempDir()'s own. Registering the reap here — after the WS_CACHE_HOME
	// t.Setenv and its backing t.TempDir("cache") above, and after root's own
	// t.TempDir() earlier still — makes it the LAST registration, so it is the
	// FIRST to run: before WS_CACHE_HOME is restored, before the cache dir is
	// removed, and before root's own RemoveAll. That ordering matters twice
	// over: exec.status/exec.abort resolve the job store through WS_CACHE_HOME
	// and root, so reaping must happen while both are still the test's live
	// values; and every exec job this test spawns (the running/aborted job and
	// the large job) must reach a terminal status before root's RemoveAll
	// runs, because a still-"running" job on Windows holds root as its
	// child's CWD and keeps open stdout/stderr/combined file handles under
	// it — RemoveAll racing that live process fails with "The process cannot
	// access the file because it is being used by another process" (POSIX
	// unlink tolerates this; Windows does not). Registering the reap earlier
	// (e.g. immediately after root's t.TempDir()) would run it too early:
	// LIFO would then unwind WS_CACHE_HOME's restore and the cache dir's
	// RemoveAll first, and exec.status/exec.abort would report the job "not
	// found" against the now-reset cache home.
	t.Cleanup(func() { reapExecKeys(t, server, root, execKeys) })

	input := toolCallLine(t, 1, "exec.shell", mcpAbortShellArgs()) + "\n"
	var out bytes.Buffer
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatal(err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	text := toolText(t, byID["1"])
	if strings.HasPrefix(strings.TrimSpace(text), "{") || !strings.Contains(text, "status: running") || !strings.Contains(text, "exec.ask") || strings.Contains(text, "done") {
		t.Fatalf("running launch response = %s", byID["1"])
	}
	running := execToolResponse{ExecKey: execKeyFromText(t, text)}
	execKeys = append(execKeys, running.ExecKey)
	out.Reset()
	input = strings.Join([]string{
		fmt.Sprintf(`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"exec.result","arguments":{"exec_key":%q}}}`, running.ExecKey),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"exec.result","arguments":{"exec_key":%q,"timeout_seconds":0}}}`, running.ExecKey),
	}, "\n") + "\n"
	started := time.Now()
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatal(err)
	}
	// Both result calls are non-blocking (default and explicit timeout_seconds:0),
	// so they must not wait for the long-running job. The job has ~25s left at
	// this point, so a regression that blocked on completion would take that
	// long; the generous 5s ceiling cleanly catches it while tolerating
	// serveStdioWithSession setup jitter under full-suite scheduling pressure
	// (observed ~1.16s spikes that tripped the old sub-second budget).
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("non-blocking result calls took %s", elapsed)
	}
	resultByID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	for _, id := range []string{"10", "11"} {
		resultText := toolText(t, resultByID[id])
		if strings.Contains(resultByID[id], `"isError":true`) || strings.HasPrefix(strings.TrimSpace(resultText), "{") || !strings.Contains(resultText, "status: running") || !strings.Contains(resultText, "guidance:") || strings.Contains(resultText, "done") {
			t.Fatalf("non-blocking result %s = %s", id, resultByID[id])
		}
	}
	out.Reset()
	input = fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"exec.abort","arguments":{"exec_key":%q}}}`, running.ExecKey) + "\n"
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var abortText string
	for time.Now().Before(deadline) {
		out.Reset()
		input = fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"exec.status","arguments":{"exec_key":%q}}}`, running.ExecKey) + "\n"
		if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
			t.Fatal(err)
		}
		abortText = toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["3"])
		if strings.Contains(abortText, "status: cancelled") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(abortText, "status: cancelled") {
		t.Fatalf("abort status = %s", abortText)
	}

	out.Reset()
	input = toolCallLine(t, 4, "exec.shell", mcpLargeShellArgs()) + "\n"
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatal(err)
	}
	launchText := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["4"])
	large := execToolResponse{ExecKey: execKeyFromText(t, launchText)}
	execKeys = append(execKeys, large.ExecKey)

	// Do not assert combined_bytes off the synchronous launch response: on a
	// loaded Windows runner, powershell cold-start can exceed the 5s
	// ForegroundWindow (execjob.ForegroundWindow), so the launch can still
	// return "status: running" / "combined_bytes: 0" while the job keeps
	// writing. Drive exec.result with a generous timeout so it polls
	// (execjob.ResultWithTimeout) until the job reaches a terminal status —
	// only then is combined_bytes final and exec.abort's t.Cleanup-time reap
	// guaranteed a closed file handle.
	out.Reset()
	input = fmt.Sprintf(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"exec.result","arguments":{"exec_key":%q,"timeout_seconds":30}}}`, large.ExecKey) + "\n"
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatal(err)
	}
	largeText := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["5"])
	if strings.Contains(largeText, strings.Repeat("x", 100)) || !strings.Contains(largeText, "status: succeeded") || !strings.Contains(largeText, "combined_bytes: 5000") || !strings.Contains(largeText, "exec.raw.*") {
		t.Fatalf("large response = %s", largeText)
	}
}

// reapExecKeys aborts and waits for every exec job this test spawned to reach
// a terminal status. It is registered via t.Cleanup as the last registration
// in TestExecMCPRunningLargeAndAbort (see the comment at that call site for
// why) so it runs, LIFO, before WS_CACHE_HOME is restored and before Go's own
// TempDir RemoveAll cleanups, which on Windows cannot unlink a directory a
// live child process still holds as its CWD or has open file handles under.
// exec.abort on an already-terminal job is a documented no-op
// (execjob.Abort), so calling it unconditionally for every key — including
// one already confirmed cancelled by the test body — is safe.
func reapExecKeys(t *testing.T, server *Server, root string, keys []string) {
	t.Helper()
	for _, key := range keys {
		var out bytes.Buffer
		abortInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":990,"method":"tools/call","params":{"name":"exec.abort","arguments":{"exec_key":%q}}}`, key) + "\n"
		if err := serveStdioWithSession(t, server, root, abortInput, &out); err != nil {
			t.Logf("reapExecKeys: abort %s: %v", key, err)
			continue
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			out.Reset()
			statusInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":991,"method":"tools/call","params":{"name":"exec.status","arguments":{"exec_key":%q}}}`, key) + "\n"
			if err := serveStdioWithSession(t, server, root, statusInput, &out); err != nil {
				t.Logf("reapExecKeys: status %s: %v", key, err)
				break
			}
			text := out.String()
			if strings.Contains(text, "status: succeeded") || strings.Contains(text, "status: failed") || strings.Contains(text, "status: cancelled") {
				break
			}
			if time.Now().After(deadline) {
				t.Logf("reapExecKeys: %s did not reach terminal status before cleanup deadline: %s", key, text)
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func TestExecMCPResultReadableJSONStdoutAndTimeout(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "README.md", "x\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")

	jsonArgs := map[string]any{"command": `printf '{"ok":true}\n'`}
	if runtime.GOOS == "windows" {
		jsonArgs = map[string]any{"shell": "powershell", "command": `Write-Output '{"ok":true}'`}
	}
	var out bytes.Buffer
	if err := serveStdioWithSession(t, server, root, toolCallLine(t, 1, "exec.shell", jsonArgs)+"\n", &out); err != nil {
		t.Fatal(err)
	}
	text := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["1"])
	if strings.HasPrefix(strings.TrimSpace(text), "{") || !strings.Contains(text, "========== stdout ==========") || !strings.Contains(text, `{"ok":true}`) || strings.Contains(text, `\"ok\"`) {
		t.Fatalf("json stdout response = %s", text)
	}

	out.Reset()
	if err := serveStdioWithSession(t, server, root, toolCallLine(t, 2, "exec.shell", mcpLongShellArgs())+"\n", &out); err != nil {
		t.Fatal(err)
	}
	running := execKeyFromText(t, toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["2"]))
	// Reap before root's t.TempDir() cleanup, LIFO (registered after the
	// WS_CACHE_HOME t.Setenv above so it runs first), matching
	// TestExecMCPRunningLargeAndAbort: on an early t.Fatal below, or if the
	// generous timeout_seconds budget below is ever exhausted, a still-live
	// child holding root as its CWD would otherwise race Windows' RemoveAll.
	t.Cleanup(func() { reapExecKeys(t, server, root, []string{running}) })
	out.Reset()
	// Do not race mcpLongShellArgs's ~6s nominal duration against a tight
	// timeout_seconds budget: on a loaded Windows runner the synchronous
	// exec.shell launch can itself consume most of execjob.ForegroundWindow
	// (5s) before the job even starts running its command, so a thin
	// remaining margin (the previous timeout_seconds:3, ~2s of slack) flakes
	// under CI load. Use a generous multiple of ForegroundWindow + the job's
	// nominal duration so exec.result's poll loop (execjob.ResultWithTimeout)
	// reaches the job's terminal status well before the budget expires,
	// matching the precedent pattern in TestExecMCPRunningLargeAndAbort.
	input := fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"exec.result","arguments":{"exec_key":%q,"timeout_seconds":30}}}`, running) + "\n"
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatal(err)
	}
	waitText := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["3"])
	if !strings.Contains(waitText, "status: succeeded") || !strings.Contains(waitText, "done") {
		t.Fatalf("timeout result = %s", waitText)
	}
}

type execToolResponse struct {
	ExecKey string `json:"exec_key"`
}

func execKeyFromText(t *testing.T, text string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^exec_key: (exec-(?:[0-9a-f]{8}|[0-9]+-[0-9a-f]{16}))$`)
	match := re.FindStringSubmatch(text)
	if len(match) != 2 {
		t.Fatalf("exec_key not found in text: %s", text)
	}
	return match[1]
}

func toolCallLine(t *testing.T, id int, name string, arguments map[string]any) string {
	t.Helper()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      name,
			"arguments": arguments,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func mcpShellShapeArgs() map[string]any {
	if runtime.GOOS == "windows" {
		return map[string]any{"command": "cd && echo shell-shape"}
	}
	return map[string]any{"command": "pwd; printf shell-shape"}
}

func mcpLongShellArgs() map[string]any {
	if runtime.GOOS == "windows" {
		return map[string]any{"command": "echo start & ping -n 7 127.0.0.1 >NUL & echo done"}
	}
	return map[string]any{"command": "echo start; sleep 6; echo done"}
}

// mcpAbortShellArgs is a deliberately long-running job for abort tests. It must
// outlive the 5s ForegroundWindow plus the non-blocking result calls and the
// abort round-trip by a wide margin, so the abort always lands while the job is
// still running instead of racing its natural completion (the prior sleep-6
// command left only ~1s after the foreground window, which flaked to
// "status: succeeded" under full-suite load). The abort terminates it promptly,
// so the test does not actually wait the full duration.
func mcpAbortShellArgs() map[string]any {
	if runtime.GOOS == "windows" {
		return map[string]any{"command": "echo start & ping -n 31 127.0.0.1 >NUL & echo done"}
	}
	return map[string]any{"command": "echo start; sleep 30; echo done"}
}

func mcpLargeShellArgs() map[string]any {
	if runtime.GOOS == "windows" {
		return map[string]any{"shell": "powershell", "command": "[Console]::Out.Write(('x' * 5000))"}
	}
	return map[string]any{"command": "i=0; while [ $i -lt 5000 ]; do printf x; i=$((i+1)); done"}
}

// containsResolvedPath reports whether haystack mentions directory p, tolerating
// the gap between the path t.TempDir() returns and the form a shell prints for
// the same directory. On Windows CI the temp root can come back as an 8.3 short
// name (C:\Users\RUNNER~1\...) while `cd` prints the long name
// (C:\Users\runneradmin\...); on macOS t.TempDir() lives under a /var ->
// /private/var symlink. filepath.EvalSymlinks canonicalizes both forms, so we
// accept either the raw or the resolved path. The assertions match against
// toolText() output, which json-decodes the content "text" field, so Windows
// paths carry native single backslashes with no extra escaping.
func containsResolvedPath(haystack, p string) bool {
	if strings.Contains(haystack, p) {
		return true
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil && resolved != p {
		return strings.Contains(haystack, resolved)
	}
	return false
}

func TestMCPExecHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_EXEC_HELPER") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" && i+1 < len(args) {
			switch args[i+1] {
			case "flow":
				wd, _ := os.Getwd()
				_, _ = os.Stdout.WriteString(wd + "\nalpha\nbeta42\n")
				_, _ = os.Stderr.WriteString("err\n")
			default:
				_, _ = os.Stdout.WriteString(args[i+1] + "\n")
			}
			os.Exit(0)
		}
	}
	os.Exit(2)
}
