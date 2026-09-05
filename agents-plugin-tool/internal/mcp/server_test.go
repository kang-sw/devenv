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

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdoc"
)

// TestMain defaults WS_RSRC_ROOT to the shipped rsrc tree so agent registration
// can load delegate-orientation (260611 Phase 6b moved it off the wsprompt
// go:embed bundle). Tests that exercise a custom rsrc tree override it per-test
// with t.Setenv.
//
// It also defaults WS_CACHE_HOME to a throwaway temp dir so the file-backed
// session store (keys/<key>.json under the cache root) never reads or writes the
// developer's real ~/.cache during the suite. Tests that assert specific cache
// paths still override it per-test with t.Setenv (last write wins).
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
	return m.Run()
}

func TestFormatBroadDocumentationFindGroupsEvidence(t *testing.T) {
	specs := []wsdoc.SpecInfo{{Path: "ai-docs/spec/plugin-runtime.md", MatchScore: 18, Matches: []wsdoc.MatchEvidence{{Line: 18, MatchedTerms: []string{"marketplace"}, Snippet: "marketplace release packaging"}}}}
	text := formatSpecFind("wsflow installer marketplace release packaging", specs)
	if !strings.Contains(text, `1 candidate spec for query="wsflow installer marketplace release packaging"`) || !strings.Contains(text, "ai-docs/spec/plugin-runtime.md	score=18	hits=1") || !strings.Contains(text, "  18: marketplace release packaging") {
		t.Fatalf("spec find text = %q", text)
	}
	if strings.Contains(text, "matched:") {
		t.Fatalf("spec find text included matched line: %q", text)
	}

	models := []wsdoc.MentalModelInfo{{Path: "ai-docs/mental-model/runtime.md", MatchScore: 9, Matches: []wsdoc.MatchEvidence{{Line: 7, MatchedTerms: []string{"runtime", "cli"}, Snippet: "runtime CLI mirror"}}}}
	text = formatMentalModelFind("runtime readable CLI mirror", models)
	if !strings.Contains(text, `1 candidate mental model for query="runtime readable CLI mirror"`) || !strings.Contains(text, "ai-docs/mental-model/runtime.md	score=9	hits=1") || !strings.Contains(text, "  7: runtime CLI mirror") {
		t.Fatalf("mental model find text = %q", text)
	}
}

func TestFormatBroadDocumentationFindBoundsEvidenceAndGuidesZeroResults(t *testing.T) {
	matches := []wsdoc.MatchEvidence{}
	for i := 1; i <= 5; i++ {
		matches = append(matches, wsdoc.MatchEvidence{Line: i, MatchedTerms: []string{"workflow"}, Snippet: fmt.Sprintf("workflow line %d", i)})
	}
	specs := []wsdoc.SpecInfo{{Path: "ai-docs/spec/a.md", MatchScore: 5, Matches: matches}}
	text := formatSpecFind("workflow", specs)
	if !strings.Contains(text, "showing subset") || strings.Count(text, "workflow line") != maxFindTextEvidencePerDoc {
		t.Fatalf("bounded spec find text = %q", text)
	}

	text = formatSpecFind("absent phrase", nil)
	if !strings.Contains(text, "0 candidate specs") || !strings.Contains(text, "retry with shorter noun phrases") {
		t.Fatalf("zero-result text = %q", text)
	}
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

func TestServeStdioConfigShow(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cache := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cache)

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.list","arguments":{"format":"json"}}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	showBefore := toolText(t, byID["1"])
	var before struct {
		Path   string `json:"path"`
		Config struct {
			SchemaVersion int `json:"schema_version"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(showBefore), &before); err != nil {
		t.Fatalf("config.show json response is not JSON: %v\n%s", err, showBefore)
	}
	wantConfigPath := filepath.Join(canonicalTestPath(t, cache), "config.json")
	if before.Path != wantConfigPath || before.Config.SchemaVersion != 1 {
		t.Fatalf("config.show json response mismatch: %s", byID["1"])
	}

	if _, err := wsconfig.SetAgentsTier(wsconfig.Options{}, "small", "", "claude-sonnet-4", "low"); err != nil {
		t.Fatalf("SetAgentsTier returned error: %v", err)
	}
	out.Reset()
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.list","arguments":{"format":"json"}}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	showAfter := toolText(t, byID["2"])
	if !strings.Contains(showAfter, `"backend":"claude"`) || !strings.Contains(showAfter, `"model":"claude-sonnet-4"`) || !strings.Contains(showAfter, `"effort":"low"`) {
		t.Fatalf("config.show response missing tier mapping: %s", byID["2"])
	}

	out.Reset()
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"config.list","arguments":{}}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if readable := toolText(t, byID["3"]); !strings.Contains(readable, "effort=low") {
		t.Fatalf("readable config.show missing effort: %s", readable)
	}
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

func TestServeStdioConfigAgentsTier(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","value":{"tier":"small","model":"claude-sonnet-4"}}}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	configText := toolText(t, byID["1"])
	if !strings.Contains(configText, `"backend":"claude"`) || !strings.Contains(configText, `"model":"claude-sonnet-4"`) {
		t.Fatalf("config response missing tier mapping: %s", byID["1"])
	}

}

func TestServeStdioDefaultsToLeadToolsWithoutRootAuthorityDetection(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	mustEnableMercenary(t)

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !strings.Contains(byID["1"], "mercenary.register") || !strings.Contains(byID["1"], "mercenary.call") || !strings.Contains(byID["1"], "config.list") {
		t.Fatalf("default profile hid lead tools: %s", byID["1"])
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
		{"use ws/specs.query here", "use wsflow/specs.query here"},
		{"call ws:lead-implement skill", "call wsflow:lead-implement skill"},
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
		"reference-discovery", "plan-populator-survey",
		"plan-populator-research", "code-reviewer", "mental-model-updater",
	} {
		t.Run(stem, func(t *testing.T) {
			context := map[string]string{"bridge_probe": "context for " + stem}
			if stem == "plan-populator-survey" || stem == "plan-populator-research" {
				for key, value := range shippedPlanPopulatorContext() {
					context[key] = value
				}
			}
			path, _, err := renderPlaybook(s, shippedRsrcRootForTest(), root, stem, context, wsconfig.Options{}, "", "", false, "", nil)
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
			if strings.Contains(text, "ws.mercenary.") || strings.Contains(text, "exec.") {
				t.Errorf("rendered %s exposed hidden full-ws guidance:\n%s", stem, text)
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
	mustEnableMercenary(t)

	server := NewServer(root, "test")
	leafKey, err := server.sessions.mint(root, roleLeaf, "")
	if err != nil {
		t.Fatalf("mint leaf key: %v", err)
	}

	// tools/list advertises the full lead surface (schema visibility is advisory;
	// the keyed call gate is the enforcement). Restricted tools remain visible.
	listResp := callToolsList(t, server)
	for _, name := range []string{"mercenary.status", "config.tune", "config.list", "runtime.read"} {
		if !strings.Contains(listResp, name) {
			t.Fatalf("tools/list must advertise full lead surface, missing %s: %s", name, listResp)
		}
	}
	if strings.Contains(listResp, "ws.setup") {
		t.Fatalf("tools/list exposed deleted setup mutation tool: %s", listResp)
	}

	// A leaf-scoped key is rejected by the keyed gate for restricted tools.
	deniedStatus := callToolOnce(t, server, 2, "mercenary.status", map[string]any{
		"session_key": leafKey,
		"name":        "impl",
	})
	if !strings.Contains(deniedStatus, "tool not available") {
		t.Fatalf("leaf key not rejected for ws.mercenary.status: %s", deniedStatus)
	}
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
	t.Setenv("WS_MCP_ALLOWED_TOOLS", "runtime.read,ws.mercenary.status,config.list")

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

	// ws.mercenary.status and config.list are allowlisted but DENIED by the leaf scope.
	// The keyed gate must still reject them — the allowlist cannot regain them.
	deniedStatus := callToolOnce(t, server, 3, "mercenary.status", map[string]any{
		"session_key": leafKey,
		"name":        "impl",
	})
	if !strings.Contains(deniedStatus, "tool not available") {
		t.Fatalf("allowlist let leaf-denied ws.mercenary.status through: %s", deniedStatus)
	}
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

func mustMarshalForTest(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
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

// TestServeStdioSpecsQueryPointResolveMatchesRemovedStatusShape is the specs
// counterpart of TestServeStdioTicketsQueryPointResolveMatchesRemovedStatusShape:
// specs.query(spec_stem: X) alone (no query, no ticket_stem) must error on a
// not-found anchor exactly like the removed specs.status did, and a found
// anchor's text/JSON must use SpecAnchorStatus's spec_stem/locations/files
// shape (formatSpecStatus), not SpecsFind's per-file formatSpecs listing.
func TestServeStdioSpecsQueryPointResolveMatchesRemovedStatusShape(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/demo.md", "# Demo\n\n## Feature {#260504-spec-demo}\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	notFoundInput := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"specs.query","arguments":{"spec_stem":"260504-missing"}}}` + "\n"
	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := serveStdioWithSession(t, server, root, notFoundInput, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	if !strings.Contains(out.String(), `"isError":true`) || !strings.Contains(out.String(), "spec anchor not found: 260504-missing") {
		t.Fatalf("specs.query(spec_stem:) not-found case did not error like the removed specs.status: %s", out.String())
	}

	textInput := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"specs.query","arguments":{"spec_stem":"260504-spec-demo"}}}` + "\n"
	out.Reset()
	if err := serveStdioWithSession(t, server, root, textInput, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	text := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["2"])
	if !strings.Contains(text, "spec_stem:") || !strings.Contains(text, "locations:") || !strings.Contains(text, "files:") {
		t.Fatalf("specs.query(spec_stem:) text did not use the removed specs.status's spec_stem/locations/files shape: %s", text)
	}

	jsonInput := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"specs.query","arguments":{"spec_stem":"260504-spec-demo","format":"json"}}}` + "\n"
	out.Reset()
	if err := serveStdioWithSession(t, server, root, jsonInput, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	jsonText := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["3"])
	var obj map[string]any
	if err := json.Unmarshal([]byte(jsonText), &obj); err != nil {
		t.Fatalf("specs.query(spec_stem:) json mode is not a bare object like the removed specs.status: %v\n%s", err, jsonText)
	}
	if obj["spec_stem"] != "260504-spec-demo" {
		t.Fatalf("specs.query(spec_stem:) json object missing expected spec_stem: %s", jsonText)
	}
}

func TestServeStdioMentalModelToolsRejectSpecStemOnStatus(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/mental-model/workflow.md", "---\ndomain: workflow\n---\n# Workflow\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"mental_models.status","arguments":{"spec_stem":"260504-spec-demo"}}}` + "\n"

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	text := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["1"])
	if !strings.Contains(text, "domain or path") || !strings.Contains(out.String(), `"isError":true`) {
		t.Fatalf("mental_models.status accepted spec_stem argument: %s", out.String())
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

// mustEnableMercenary writes the global WS_CONFIG_HOME/config.json override
// {"workflow.prefer_mercenary":"on"}. Tests that call ws.mercenary.* tools need
// this because the builtin default is hide.
func mustEnableMercenary(t *testing.T) {
	t.Helper()
	configHome := os.Getenv("WS_CONFIG_HOME")
	if configHome == "" {
		configHome = t.TempDir()
		t.Setenv("WS_CONFIG_HOME", configHome)
	}
	if err := os.MkdirAll(configHome, 0755); err != nil {
		t.Fatalf("mustEnableMercenary: mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configHome, "config.json"),
		[]byte(`{"schema_version":1,"overrides":{"workflow.prefer_mercenary":"on"}}`), 0644); err != nil {
		t.Fatalf("mustEnableMercenary: write config: %v", err)
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
	mustWrite(t, root, "ai-docs/spec/demo.md", "---\ntitle: Demo\nfeatures:\n  - planned [260503-feat-demo/p1]\n---\n# Demo\n\n## Feature {#260503-spec-demo}\n\nSpec discovery text.\n")
	mustWrite(t, root, "ai-docs/mental-model/workflow.md", "---\ndomain: workflow\ndescription: Workflow model\nsources:\n  - ai-docs/spec/demo.md#260503-spec-demo\n---\n# Workflow\n\nReferences {#260503-spec-demo} with discovery text.\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260503-feat-demo.md", "---\ntitle: Demo ticket\n---\n# Demo\n\nMentions 260503-epic-demo.\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	mustEnableMercenary(t)

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
		`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"specs.query","arguments":{"spec_stem":"260503-spec-demo","ticket_stem":"260503-feat-demo","query":"discovery"}}}`,
		`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"mental_models.query","arguments":{"spec_stem":"260503-spec-demo","domain":"workflow","query":"discovery"}}}`,
		`{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"specs.query","arguments":{"query":"discovery","format":"json"}}}`,
		`{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"mental_models.query","arguments":{"query":"discovery","format":"json"}}}`,
		`{"jsonrpc":"2.0","id":15,"method":"tools/call","params":{"name":"references.trace","arguments":{"spec_stem":"260503-spec-demo"}}}`,
	}, "\n")

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 15 {
		t.Fatalf("expected 15 responses, got %d\n%s", len(lines), out.String())
	}
	byID := responseLinesByID(t, lines)

	var listResp map[string]any
	if err := json.Unmarshal([]byte(byID["2"]), &listResp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(byID["2"], "project_tree") {
		t.Fatalf("tools/list missing project_tree: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "mercenary.call") {
		t.Fatalf("tools/list missing ws.mercenary.call: %s", byID["2"])
	}
	if strings.Contains(byID["2"], "ws.mercenary.call_async") {
		t.Fatalf("tools/list still includes ws.mercenary.call_async: %s", byID["2"])
	}
	if strings.Contains(byID["2"], "ws.mercenary.oneshot") {
		t.Fatalf("tools/list still includes ws.mercenary.oneshot: %s", byID["2"])
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
	}
	rootAwareTools := []string{
		"api.list",
		"git.status", "git.diff", "git.log", "git.merge_base", "git.commit",
		"project_tree", "spec_stem.generate", "spec_index.verify", "specs.query",
		"mental_models.list", "mental_models.query", "mental_models.status", "references.trace",
		"tickets.query", "path.generate", "playbook.render",
		"mercenary.register", "mercenary.call", "mercenary.wait", "mercenary.result", "mercenary.status",
		"mercenary.interrupt", "mercenary.tail", "mercenary.debug.tail", "mercenary.debug.stdout",
		"mercenary.debug.stderr", "mercenary.debug.runtime_log", "mercenary.debug.events",
		"mercenary.cancel", "mercenary.print", "mercenary.erase",
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
	// Unit 4: ws.mercenary.register prompts/tier/model fields removed from schema.
	// Verify system_prompt_text is still present and prompts is absent.
	if strings.Contains(byID["2"], "\"prompts\"") {
		t.Fatalf("tools/list ws.mercenary.register schema still has removed 'prompts' field: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "\"system_prompt_text\"") {
		t.Fatalf("tools/list ws.mercenary.register schema missing system_prompt_text: %s", byID["2"])
	}
	for _, tool := range []string{"mercenary.wait", "mercenary.result", "mercenary.status", "mercenary.tail", "mercenary.debug.tail", "mercenary.debug.stdout", "mercenary.debug.stderr", "mercenary.debug.runtime_log", "mercenary.debug.events", "mercenary.cancel", "git.status", "git.diff", "git.log", "git.merge_base", "git.commit", "tickets.query", "specs.query", "mental_models.query", "mental_models.status", "references.trace"} {
		if !strings.Contains(byID["2"], tool) {
			t.Fatalf("tools/list missing %s: %s", tool, byID["2"])
		}
	}
	if !strings.Contains(byID["2"], `"mental_model_notes"`) {
		t.Fatalf("tools/list missing git.commit mental_model_notes schema: %s", byID["2"])
	}
	if strings.Contains(byID["2"], "ws.mercenary.recall") {
		t.Fatalf("tools/list should not advertise ws.mercenary.recall: %s", byID["2"])
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
	if !strings.Contains(byID["4"], "Implementation Playbook") {
		t.Fatalf("infra response missing impl-playbook: %s", byID["4"])
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
	specsText := toolText(t, byID["11"])
	if !strings.Contains(specsText, "1 candidate spec for query=\"discovery\"") || !strings.Contains(specsText, "ai-docs/spec/demo.md\tscore=") || strings.Contains(specsText, "matched:") {
		t.Fatalf("specs.query response missing spec result: %s", byID["11"])
	}
	mentalModelsText := toolText(t, byID["12"])
	if !strings.Contains(mentalModelsText, "1 candidate mental model for query=\"discovery\"") || !strings.Contains(mentalModelsText, "ai-docs/mental-model/workflow.md\tscore=") || strings.Contains(mentalModelsText, "matched:") {
		t.Fatalf("mental_models.query response missing result: %s", byID["12"])
	}
	if !strings.Contains(byID["13"], "matches") || !strings.Contains(byID["13"], "matched_terms") {
		t.Fatalf("specs.query json missing evidence: %s", byID["13"])
	}
	if !strings.Contains(byID["14"], "matches") || !strings.Contains(byID["14"], "matched_terms") {
		t.Fatalf("mental_models.query json missing evidence: %s", byID["14"])
	}
	referencesText := toolText(t, byID["15"])
	if !strings.Contains(referencesText, "input: spec") || !strings.Contains(referencesText, "tickets:") || !strings.Contains(referencesText, "mental_models:") {
		t.Fatalf("references.trace response missing graph result: %s", byID["15"])
	}
}

func TestServeStdioAgentDebugToolCalls(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cache := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cache)
	mustEnableMercenary(t)
	_, layout, err := wsagent.NewManager(wsagent.Options{}).Register(wsagent.RegisterOptions{Root: root, Name: "impl"})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Dir(layout.CurrentStdout), filepath.Base(layout.CurrentStdout), "stdout old\nstdout new\n")
	mustWrite(t, filepath.Dir(layout.CurrentRuntimeLog), filepath.Base(layout.CurrentRuntimeLog), "runtime old\nruntime new\n")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"mercenary.debug.stdout","arguments":{"name":"impl","lines":1}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"mercenary.debug.runtime_log","arguments":{"name":"impl","lines":1}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"mercenary.debug.tail","arguments":{"name":"impl","lines":1}}}`,
	}, "\n")

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 responses, got %d\n%s", len(lines), out.String())
	}
	byID := responseLinesByID(t, lines)
	if got := toolText(t, byID["1"]); got != "stdout new\n" {
		t.Fatalf("stdout debug response = %q", got)
	}
	if got := toolText(t, byID["2"]); got != "runtime new\n" {
		t.Fatalf("runtime debug response = %q", got)
	}
	if got := toolText(t, byID["3"]); !strings.Contains(got, "== stdout ==") || !strings.Contains(got, "stdout new") || !strings.Contains(got, "== runtime ==") {
		t.Fatalf("debug tail response mismatch: %q", got)
	}
}

func TestServeStdioAgentTailIsBoundedButDebugTailIsRaw(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cache := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cache)
	mustEnableMercenary(t)
	_, layout, err := wsagent.NewManager(wsagent.Options{}).Register(wsagent.RegisterOptions{Root: root, Name: "impl"})
	if err != nil {
		t.Fatal(err)
	}
	largeOutput := strings.Repeat("x", 5000)
	line := `{"type":"event","aggregated_output":"` + largeOutput + `"}`
	mustWrite(t, filepath.Dir(layout.CurrentStdout), filepath.Base(layout.CurrentStdout), line+"\n")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"mercenary.tail","arguments":{"name":"impl","lines":1}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"mercenary.debug.tail","arguments":{"name":"impl","lines":1}}}`,
	}, "\n")

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses, got %d\n%s", len(lines), out.String())
	}
	byID := responseLinesByID(t, lines)
	normal := toolText(t, byID["1"])
	if !strings.Contains(normal, "ws-tail truncated field aggregated_output") || strings.Contains(normal, largeOutput) {
		t.Fatalf("normal tail was not bounded: %q", normal)
	}
	debug := toolText(t, byID["2"])
	if !strings.Contains(debug, largeOutput) || strings.Contains(debug, "ws-tail truncated") {
		t.Fatalf("debug tail was not raw: %q", debug)
	}
}

func TestServeStdioConfigAgentsTierUsesDetectedHarness(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260513-feat-harness-local-agent-tier-config")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	mustEnableMercenary(t)

	server := NewServer(t.TempDir(), "test")
	var out bytes.Buffer
	initializeInput := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"Claude Code","version":"test"}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(initializeInput), &out); err != nil {
		t.Fatalf("ServeStdio initialize returned error: %v", err)
	}

	out.Reset()
	configInput := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","value":{"tier":"medium","backend":"codex","model":"gpt-5.4","effort":"medium"}}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(configInput), &out); err != nil {
		t.Fatalf("ServeStdio config returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	configText := toolText(t, byID["2"])
	if !strings.Contains(configText, `"claude":{"backend":"codex","model":"gpt-5.4","effort":"medium"}`) {
		t.Fatalf("config response missing claude harness mapping: %s", byID["2"])
	}

	out.Reset()
	registerInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"mercenary.register","arguments":{"root":%q,"name":"reviewer","model":"medium"}}}`, root)
	if err := serveStdioWithSession(t, server, root, registerInput, &out); err != nil {
		t.Fatalf("ServeStdio register returned error: %v", err)
	}
	status, err := wsagent.NewManager(wsagent.Options{}).Status(root, "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "harness: claude") || !strings.Contains(status, "backend: codex") || !strings.Contains(status, "model: gpt-5.4") || !strings.Contains(status, "effort: medium") {
		t.Fatalf("registered status missing configured claude harness mapping:\n%s", status)
	}
}

// TestServeStdioConfigAgentsTierUsesDetectedPiHarness mirrors
// TestServeStdioConfigAgentsTierUsesDetectedHarness for the Pi harness: an
// `initialize` request whose clientInfo.name is exactly "ws-pi-bridge" must
// be detected via the structured clientInfo check
// (detectHarnessFromInitializeParams) and used as the config.tune harness
// default. Unlike the Claude-detection sibling test, this does not exercise
// mercenary.register: Open Decision #3 keeps mercenary untouched and
// unreachable from Pi in this phase.
func TestServeStdioConfigAgentsTierUsesDetectedPiHarness(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260513-feat-harness-local-agent-tier-config")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	var out bytes.Buffer
	initializeInput := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"ws-pi-bridge","version":"test"}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(initializeInput), &out); err != nil {
		t.Fatalf("ServeStdio initialize returned error: %v", err)
	}

	out.Reset()
	configInput := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","value":{"tier":"medium","backend":"pi","model":"pi-model","effort":"medium"}}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(configInput), &out); err != nil {
		t.Fatalf("ServeStdio config returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	configText := toolText(t, byID["2"])
	if !strings.Contains(configText, `"pi":{"backend":"pi","model":"pi-model","effort":"medium"}`) {
		t.Fatalf("config response missing pi harness mapping: %s", byID["2"])
	}
}

// TestServeStdioConfigAgentsTierAcceptsTierSynonyms guards the 260814 config
// surface swap against a regression: config.tune must NOT enum-validate the
// agents.tier `tier` up front. The removed config.agents_tier passed the raw
// string to SetAgentsTierForHarness, whose normalizedTier accepts documented
// synonyms (light/core/deep, haiku/sonnet/opus) case/whitespace-insensitively
// (see wsconfig TestResolveAgentLegacyTierSynonyms). The surviving CLI path
// still does. So config.tune must too.
func TestServeStdioConfigAgentsTierAcceptsTierSynonyms(t *testing.T) {
	cases := []struct {
		name         string
		tier         string
		canonicalKey string
	}{
		{"synonym-core", "core", "medium"},
		{"synonym-deep", "deep", "large"},
		{"synonym-sonnet", "sonnet", "medium"},
		{"case-insensitive", "Small", "small"},
		{"whitespace", "  large  ", "large"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useLeadProfile(t)
			root := initTicketRepo(t, "260513-feat-harness-local-agent-tier-config")
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

			server := NewServer(root, "test")
			var out bytes.Buffer
			model := "model-" + tc.canonicalKey
			configInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","harness":"claude","value":{"tier":%q,"model":%q}}}}`, tc.tier, model)
			if err := server.ServeStdio(context.Background(), strings.NewReader(configInput+"\n"), &out); err != nil {
				t.Fatalf("ServeStdio config returned error: %v", err)
			}
			byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
			resp := byID["2"]
			if strings.Contains(resp, `"isError":true`) {
				t.Fatalf("config.tune rejected tier synonym %q (should accept, matching CLI): %s", tc.tier, resp)
			}
			// The synonym must normalize to its canonical tier key and store the
			// harness mapping there. toolText unescapes the inner config JSON.
			configText := toolText(t, resp)
			if !strings.Contains(configText, `"`+tc.canonicalKey+`"`) || !strings.Contains(configText, model) {
				t.Fatalf("config.tune tier synonym %q did not land under canonical tier %q with model %q: %s", tc.tier, tc.canonicalKey, model, configText)
			}
		})
	}
}

func TestServeStdioConfigAgentsTierOmittedEffortClearsExistingEffort(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260513-feat-agent-tier-effort-config")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	mustEnableMercenary(t)

	var out bytes.Buffer
	server := NewServer(root, "test")
	inputs := []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","harness":"codex","value":{"tier":"medium","model":"gpt-5.5","effort":"medium"}}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.tune","arguments":{"key":"agents.tier","harness":"codex","value":{"tier":"medium","model":"gpt-5.4"}}}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"mercenary.register","arguments":{"root":%q,"name":"reviewer","model":"medium"},"_meta":{"x-codex-turn-metadata":{"workspaces":{%q:{}}}}}}`, root, root),
	}
	for _, input := range inputs {
		out.Reset()
		if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
			t.Fatalf("ServeStdio returned error: %v", err)
		}
	}
	status, err := wsagent.NewManager(wsagent.Options{}).Status(root, "reviewer")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "model: gpt-5.4") || strings.Contains(status, "effort: medium") {
		t.Fatalf("registered status did not clear effort after model update:\n%s", status)
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
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"mercenary.call","arguments":{"name":"impl","prompt":"work"}}}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	list := byID["1"]
	// config.tune/config.list are generic and always visible; the no-agent cut
	// for full-ws-only knobs (workflow.prefer_mercenary) now lives per-key inside
	// config.tune's dispatch and config.list's catalog, not in a hidden per-knob
	// tool (260814 Phase 2 collapse). See TestPreferMercenaryHiddenInNoAgentMode.
	for _, hidden := range []string{"mercenary.call", "mercenary.register", "mercenary.debug.tail"} {
		if strings.Contains(list, hidden) {
			t.Fatalf("tools/list exposed hidden no-agent tool %s: %s", hidden, list)
		}
	}
	for _, visible := range []string{"api.list", "config.list", "config.tune", "tickets.query", "playbook.read", "playbook.render"} {
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
	if toolIsError(t, byID["2"]) {
		t.Fatalf("api.list should remain callable in no-agent mode: %s", byID["2"])
	}
	if !strings.Contains(byID["3"], "wsflow agentless mode disables agent-backed tool: mercenary.call") {
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
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"playbook.render","arguments":{"name":"code-reviewer","context":{"reviewer_scope":"correctness only","note":"see ws/specs.query for details"}}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"playbook.render","arguments":{"name":"plan-populator-survey","context":{"target_kind":"ticket","ticket_path":"ai-docs/tickets/ready/260628-feat-demo.md","selected_phase":"Phase 2: Rework planner playbooks around ticket-to-plan","inline_contract":"","plan_path":"ai-docs/.plans/2026-06/28-1200-demo.md","note":"legacy extra context"}}}}`,
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
	for _, want := range []string{"wsflow/", "## Render Context", "- note: see ws/specs.query for details", "- reviewer_scope: correctness only"} {
		if !strings.Contains(codeReviewerText, want) {
			t.Fatalf("code-reviewer playbook render missing %q:\n%s", want, codeReviewerText)
		}
	}
	if strings.Contains(codeReviewerText, "ws.mercenary.") || strings.Contains(codeReviewerText, "exec.") {
		t.Fatalf("code-reviewer playbook render exposed hidden full-ws guidance:\n%s", codeReviewerText)
	}

	if toolIsError(t, byID["3"]) {
		t.Fatalf("playbook.render plan-populator-survey returned error: %s", byID["3"])
	}
	planPath := strings.SplitN(strings.TrimSpace(toolText(t, byID["3"])), "\n", 2)[0]
	planData, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read plan-populator-survey render: %v", err)
	}
	planText := string(planData)
	for _, want := range []string{
		"recommended-tier: medium",
		"## Render Context",
		"- note: legacy extra context",
		"- Ticket path: `ai-docs/tickets/ready/260628-feat-demo.md`",
		"- Selected phase: `Phase 2: Rework planner playbooks around ticket-to-plan`",
		"- Plan path: `ai-docs/.plans/2026-06/28-1200-demo.md`",
		"## Relevant Ticket Contract",
		"## Implementation Plan",
		"[escalate-to-research]",
	} {
		if want == "recommended-tier: medium" {
			if !strings.Contains(toolText(t, byID["3"]), want) {
				t.Fatalf("playbook.render response missing %q: %s", want, toolText(t, byID["3"]))
			}
			continue
		}
		if !strings.Contains(planText, want) {
			t.Fatalf("plan-populator-survey playbook render missing %q:\n%s", want, planText)
		}
	}
	if strings.Contains(planText, "- brief_path:") || strings.Contains(planText, "brief path") {
		t.Fatalf("plan-populator-survey playbook render retained brief-path dependency:\n%s", planText)
	}
}

func TestPlaybookRenderReturnsResolvedNativeBindings(t *testing.T) {
	useLeadProfile(t)
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
	if got, want := strings.Join(lines[1:], "\n"), "recommended-tier: medium\nrecommended-model: gpt-5.6-terra\nrecommended-reasoning-effort: high"; got != want {
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

func TestServeStdioInitializeDetectsClaudeHarnessForAgentAlias(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260508-feat-claude-harness")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	mustEnableMercenary(t)

	initializeInput := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"Claude Code","version":"test"}}}`
	registerInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"mercenary.register","arguments":{"root":%q,"name":"reviewer","model":"medium"}}}`, root)
	checkInput := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"mercenary.status","arguments":{"name":"reviewer"}}}`

	var out bytes.Buffer
	server := NewServer(t.TempDir(), "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(initializeInput), &out); err != nil {
		t.Fatalf("ServeStdio initialize returned error: %v", err)
	}
	out.Reset()
	if err := serveStdioWithSession(t, server, root, registerInput, &out); err != nil {
		t.Fatalf("ServeStdio register returned error: %v", err)
	}
	out.Reset()
	if err := serveStdioWithSession(t, server, root, checkInput, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	status := toolText(t, byID["3"])
	if !strings.Contains(status, "harness: claude") || !strings.Contains(status, "backend: claude") || !strings.Contains(status, "model: sonnet") {
		t.Fatalf("status missing claude alias resolution:\n%s", status)
	}
}

func TestServeStdioCodexMetadataDetectsHarnessForAgentAlias(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260508-feat-codex-harness")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	mustEnableMercenary(t)

	setupInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"mercenary.register","arguments":{"root":%q,"name":"reviewer","model":"medium"},"_meta":{"x-codex-turn-metadata":{"workspaces":{%q:{}}}}}}`, root, root)
	checkInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"mercenary.status","arguments":{"root":%q,"name":"reviewer"}}}`, root)
	var out bytes.Buffer
	server := NewServer(t.TempDir(), "test")
	if err := serveStdioWithSession(t, server, root, setupInput, &out); err != nil {
		t.Fatalf("ServeStdio setup returned error: %v", err)
	}
	out.Reset()
	if err := serveStdioWithSession(t, server, root, checkInput, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	status := toolText(t, byID["2"])
	if !strings.Contains(status, "harness: codex") || !strings.Contains(status, "backend: codex") || !strings.Contains(status, "model: gpt-5.6-terra") {
		t.Fatalf("status missing codex alias resolution:\n%s", status)
	}
}

func TestServeStdioDoesNotBlockToolsListBehindWait(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	mustEnableMercenary(t)
	agent, layout, err := wsagent.NewManager(wsagent.Options{}).Register(wsagent.RegisterOptions{Root: root, Name: "impl"})
	if err != nil {
		t.Fatal(err)
	}
	call, err := wsagent.NewManager(wsagent.Options{}).BeginCurrentCall(layout, agent)
	if err != nil {
		t.Fatal(err)
	}
	call.Status = wsagent.CallStatusRunning
	call.PID = os.Getpid()
	if err := os.WriteFile(layout.CurrentStateFile, mustMarshalForTest(t, call), 0o644); err != nil {
		t.Fatal(err)
	}

	reader, writer := io.Pipe()
	outReader, outWriter := io.Pipe()
	done := make(chan error, 1)
	streamServer := NewServer(root, "test")
	go func() {
		done <- streamServer.ServeStdio(context.Background(), reader, outWriter)
		_ = outWriter.Close()
	}()

	key, _ := parseLoginResponse(t, callLogin(t, streamServer, 900002, root, nil))
	fmt.Fprintln(writer, fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"mercenary.wait","arguments":{"name":"impl","timeout_seconds":2,"session_key":%q}}}`, key))
	fmt.Fprintln(writer, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)

	lineCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(outReader)
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
			t.Fatalf("first response was not tools/list while wait was running: %s", line)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("tools/list was blocked behind ws.mercenary.wait")
	}
	_ = writer.Close()
	_ = reader.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("ServeStdio did not exit after input close")
	}
}

func TestServeStdioAgentsResultConsumesEphemeralAgent(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	mustEnableMercenary(t)
	manager := wsagent.NewManager(wsagent.Options{})
	agent, layout, err := manager.Register(wsagent.RegisterOptions{Root: root, Name: "ephemeral-tmp-test", Ephemeral: true})
	if err != nil {
		t.Fatal(err)
	}
	call, err := manager.BeginCurrentCall(layout, agent)
	if err != nil {
		t.Fatal(err)
	}
	call.Status = wsagent.CallStatusCompleted
	if err := os.WriteFile(layout.CurrentStateFile, mustMarshalForTest(t, call), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.OutputFile, []byte("ephemeral answer\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"mercenary.result","arguments":{"name":"ephemeral-tmp-test"}}}` + "\n"
	var out bytes.Buffer
	if err := serveStdioWithSession(t, NewServer(root, "test"), root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !strings.Contains(toolText(t, byID["1"]), "ephemeral answer") {
		t.Fatalf("ws.mercenary.result response mismatch: %s", byID["1"])
	}
	if _, err := os.Stat(layout.AgentDir); err != nil {
		t.Fatalf("ephemeral agent dir should remain after MCP result for retention cleanup: %v", err)
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

func TestServeStdioGitToolCalls(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "file.txt", "one\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260503-feat-demo.md", "---\ntitle: Demo\n---\n# Demo\n")
	runGit(t, root, "add", "file.txt", "ai-docs/tickets/todo/260503-feat-demo.md")
	runGit(t, root, "commit", "-m", "initial", "-m", "body text")
	head := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	mustWrite(t, root, "file.txt", "one\ntwo\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260503-feat-demo.md", "---\ntitle: Demo\n---\n# Demo\n\n### Result (abc123) - 2026-05-04\n\nImplemented.\n")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git.diff","arguments":{"mode":"name_only","paths":["file.txt"],"format":"json"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"git.log","arguments":{"limit":1,"include_body":true,"format":"json"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"git.merge_base","arguments":{"base":"HEAD","head":"HEAD","format":"json"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"git.diff","arguments":{"mode":"name_only","paths":["file.txt"]}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"git.log","arguments":{"limit":1,"include_body":true}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"git.merge_base","arguments":{"base":"HEAD","head":"HEAD"}}}`,
	}, "\n")

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 6 {
		t.Fatalf("expected 6 responses, got %d\n%s", len(lines), out.String())
	}
	byID := responseLinesByID(t, lines)

	var diff struct {
		Mode   string   `json:"mode"`
		Paths  []string `json:"paths"`
		Output string   `json:"output"`
	}
	if err := json.Unmarshal([]byte(toolText(t, byID["1"])), &diff); err != nil {
		t.Fatal(err)
	}
	if diff.Mode != "name_only" || !strings.Contains(diff.Output, "file.txt") || len(diff.Paths) != 1 || diff.Paths[0] != "file.txt" {
		t.Fatalf("diff response = %#v", diff)
	}

	var log struct {
		Limit       int  `json:"limit"`
		IncludeBody bool `json:"include_body"`
		Commits     []struct {
			Subject string `json:"subject"`
			Body    string `json:"body"`
		} `json:"commits"`
	}
	if err := json.Unmarshal([]byte(toolText(t, byID["2"])), &log); err != nil {
		t.Fatal(err)
	}
	if log.Limit != 1 || !log.IncludeBody || len(log.Commits) != 1 || log.Commits[0].Subject != "initial" || log.Commits[0].Body != "body text" {
		t.Fatalf("log response = %#v", log)
	}

	var mergeBase struct {
		Base      string `json:"base"`
		Head      string `json:"head"`
		MergeBase string `json:"merge_base"`
	}
	if err := json.Unmarshal([]byte(toolText(t, byID["3"])), &mergeBase); err != nil {
		t.Fatal(err)
	}
	if mergeBase.Base != "HEAD" || mergeBase.Head != "HEAD" || mergeBase.MergeBase != head {
		t.Fatalf("merge-base response = %#v, want hash %s", mergeBase, head)
	}
	if got := toolText(t, byID["4"]); got != "file.txt\n" {
		t.Fatalf("git.diff text response = %q", got)
	}
	logText := toolText(t, byID["5"])
	if !strings.Contains(logText, "commit "+head) || !strings.Contains(logText, "subject: initial") || !strings.Contains(logText, "body text") || strings.Contains(logText, `\"body text\"`) {
		t.Fatalf("git.log text response = %q", logText)
	}
	if got := toolText(t, byID["6"]); !strings.Contains(got, "merge_base: "+head) || !strings.Contains(got, "(HEAD HEAD)") {
		t.Fatalf("git.merge_base text response = %q", got)
	}

	out.Reset()
	commitInput := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["file.txt","ai-docs/tickets/todo/260503-feat-demo.md"],"title":"test: mcp commit","ai_context":["User intent: verify git.commit.","Verification: server test."],"mental_model_notes":["git.commit accepts structured Mental Model Notes."]}}}`
	if err := serveStdioWithSession(t, server, root, commitInput, &out); err != nil {
		t.Fatalf("ServeStdio commit returned error: %v", err)
	}
	commitLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(commitLines) != 1 {
		t.Fatalf("expected 1 commit response, got %d\n%s", len(commitLines), out.String())
	}
	commitByID := responseLinesByID(t, commitLines)

	commitText := toolText(t, commitByID["7"])
	if !strings.Contains(commitText, "commit: ") ||
		!strings.Contains(commitText, "title: test: mcp commit") ||
		!strings.Contains(commitText, "paths:") ||
		!strings.Contains(commitText, "ticket_changes:") ||
		strings.Contains(commitText, `"ticket_changes"`) {
		t.Fatalf("git.commit text response = %q", commitText)
	}
	commitBody := string(runGitOutput(t, root, "log", "-1", "--format=%B"))
	if !strings.Contains(commitBody, "## AI Context\n- User intent: verify git.commit.\n- Verification: server test.\n\n### Mental Model Notes\n- git.commit accepts structured Mental Model Notes.") {
		t.Fatalf("git.commit message missing Mental Model Notes subsection:\n%s", commitBody)
	}

	mustWrite(t, root, "file.txt", "one\ntwo\nthree\n")
	out.Reset()
	jsonCommitInput := `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["file.txt"],"title":"test: mcp commit json","ai_context":["User intent: verify git.commit JSON.","Verification: server test."],"format":"json"}}}`
	if err := serveStdioWithSession(t, server, root, jsonCommitInput, &out); err != nil {
		t.Fatalf("ServeStdio JSON commit returned error: %v", err)
	}
	jsonCommitLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(jsonCommitLines) != 1 {
		t.Fatalf("expected 1 JSON commit response, got %d\n%s", len(jsonCommitLines), out.String())
	}
	jsonCommitByID := responseLinesByID(t, jsonCommitLines)

	var commit struct {
		Hash          string `json:"hash"`
		Title         string `json:"title"`
		TicketChanges []struct {
			Stem        string `json:"stem"`
			ResultAdded bool   `json:"result_added"`
		} `json:"ticket_changes"`
	}
	if err := json.Unmarshal([]byte(toolText(t, jsonCommitByID["8"])), &commit); err != nil {
		t.Fatal(err)
	}
	if commit.Hash == "" || commit.Title != "test: mcp commit json" || len(commit.TicketChanges) != 0 {
		t.Fatalf("commit response = %#v", commit)
	}
}

// unresolvedPhaseTicketBody mirrors
// wsdoc.TestTicketVerifyUnresolvedPhaseIsSoftWarnOnly's fixture: a closed
// ticket with a resolved Phase 1 and an unresolved (non-`[dropped]`) Phase 2,
// which TicketVerify reports as a soft "unresolved-phases" warning, never a
// hard finding.
const unresolvedPhaseTicketBody = "---\ntitle: Closed with open phase\ncompleted: 2026-07-23\n---\n\n" +
	"## Phases\n\n" +
	"### Phase 1: First\n\n" +
	"### Result (abc123) - 2026-07-23\n\n" +
	"Done.\n\n" +
	"### Phase 2: Second\n\n" +
	"Never resolved.\n"

// TestServeStdioGitCommitSurfacesTicketVerifyWarningsAsAdvisories is the
// concrete instance of {#260720-wsdoc-commit-boundary}'s Phase 1 verification
// boundary: a real ticket that trips wsdoc.TicketVerify's soft
// unresolved-phases warning must still commit (Warnings never block OK), and
// the warning text must surface through verifyAdapter into
// wsgit.CommitResult.Advisories and formatGitCommit's text-mode "advisories:"
// block — but never through format:"json", since CommitResult.Advisories
// carries a `json:"-"` tag (see {#260626-git-commit-todo-reinjection}
// precedent).
func TestServeStdioGitCommitSurfacesTicketVerifyWarningsAsAdvisories(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "file.txt", "one\n")
	runGit(t, root, "add", "file.txt")
	runGit(t, root, "commit", "-m", "initial")

	server := NewServer(root, "test")

	textTicketPath := "ai-docs/tickets/.done/260726-feat-openphase-text.md"
	mustWrite(t, root, textTicketPath, unresolvedPhaseTicketBody)

	var out bytes.Buffer
	textInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":[%q],"title":"docs(ticket): close ticket with open phase","ai_context":["User intent: prove unresolved-phases warnings surface as git.commit advisories."]}}}`, textTicketPath)
	if err := serveStdioWithSession(t, server, root, textInput, &out); err != nil {
		t.Fatalf("ServeStdio commit returned error: %v", err)
	}
	textLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(textLines) != 1 {
		t.Fatalf("expected 1 commit response, got %d\n%s", len(textLines), out.String())
	}
	textByID := responseLinesByID(t, textLines)
	if toolIsError(t, textByID["1"]) {
		t.Fatalf("git.commit reported an error for a soft-warning-only ticket: %s", toolText(t, textByID["1"]))
	}
	text := toolText(t, textByID["1"])
	if !strings.Contains(text, "commit: ") {
		t.Fatalf("git.commit text response missing commit hash: %q", text)
	}
	if !strings.Contains(text, "advisories:\n") || !strings.Contains(text, "WARN [unresolved-phases]") {
		t.Fatalf("git.commit text response missing advisories block: %q", text)
	}

	jsonTicketPath := "ai-docs/tickets/.done/260726-feat-openphase-json.md"
	mustWrite(t, root, jsonTicketPath, unresolvedPhaseTicketBody)

	out.Reset()
	jsonInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":[%q],"title":"docs(ticket): close ticket with open phase json","ai_context":["User intent: prove the json:\"-\" tag holds for advisories."],"format":"json"}}}`, jsonTicketPath)
	if err := serveStdioWithSession(t, server, root, jsonInput, &out); err != nil {
		t.Fatalf("ServeStdio JSON commit returned error: %v", err)
	}
	jsonLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(jsonLines) != 1 {
		t.Fatalf("expected 1 JSON commit response, got %d\n%s", len(jsonLines), out.String())
	}
	jsonByID := responseLinesByID(t, jsonLines)
	if toolIsError(t, jsonByID["2"]) {
		t.Fatalf("git.commit reported an error for a soft-warning-only ticket in JSON mode: %s", toolText(t, jsonByID["2"]))
	}
	jsonText := toolText(t, jsonByID["2"])
	if strings.Contains(jsonText, "advisories") || strings.Contains(jsonText, "WARN") {
		t.Fatalf("git.commit JSON response leaked advisories despite the json:\"-\" tag: %q", jsonText)
	}
	var jsonCommit struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal([]byte(jsonText), &jsonCommit); err != nil {
		t.Fatal(err)
	}
	if jsonCommit.Hash == "" {
		t.Fatalf("JSON commit response missing hash: %q", jsonText)
	}
}

// ticketGraphAdvisoryFixture builds the smallest board that exercises all
// three advisory kinds at once: a .done child of a non-epic parent whose every
// child is now closed, carrying a dangling related: key. It yields a board
// block, one fix advisory, and one check advisory, which is what lets the
// commit-path and standalone renderers be compared over identical input.
func ticketGraphAdvisoryFixture(t *testing.T) (string, []string) {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/demo.md", "# Demo\n\n## Anchor {#260101-demo-anchor}\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260726-refactor-graph-parent.md",
		"---\ntitle: Graph parent\n---\n\n# Graph parent\n")
	child := "ai-docs/tickets/.done/260726-feat-graph-child.md"
	mustWrite(t, root, child,
		"---\ntitle: Graph child\ncompleted: 2026-07-27\nparent: 260726-refactor-graph-parent\nrelated:\n  260726-nope-dangling: no such stem\n---\n\n# Graph child\n")
	return root, []string{child}
}

// TestVerifyAdapterAppendsAmendRecipeToFixAdvisoriesOnly pins the one
// presentation difference {#260723-tickets-verify-tool}'s identical-verdict
// guarantee permits: the commit path knows a commit already exists, so a
// mechanical remedy gets the amend recipe. A check or board advisory carries no
// mechanical remedy, so appending it there would be a false instruction.
func TestVerifyAdapterAppendsAmendRecipeToFixAdvisoriesOnly(t *testing.T) {
	root, paths := ticketGraphAdvisoryFixture(t)

	advisories, err := verifyAdapter(root, paths)
	if err != nil {
		t.Fatalf("verifyAdapter returned an error for a non-blocking fixture: %v", err)
	}
	if len(advisories) != 3 {
		t.Fatalf("advisories = %#v, want a board block plus one fix and one check", advisories)
	}

	const recipe = "Then git commit --amend --no-edit."
	var sawFix, sawCheck bool
	for _, advisory := range advisories {
		switch {
		case strings.Contains(advisory, "FIX:"):
			sawFix = true
			if !strings.HasSuffix(advisory, "\n       "+recipe) {
				t.Fatalf("fix advisory missing the amend recipe:\n%s", advisory)
			}
		case strings.Contains(advisory, "CHECK:"):
			sawCheck = true
			if strings.Contains(advisory, "--amend") {
				t.Fatalf("check advisory must not carry the amend recipe:\n%s", advisory)
			}
		default:
			if !strings.HasPrefix(advisory, "## Parent Board") {
				t.Fatalf("unexpected advisory:\n%s", advisory)
			}
			if strings.Contains(advisory, "--amend") {
				t.Fatalf("board advisory must not carry the amend recipe:\n%s", advisory)
			}
		}
	}
	if !sawFix || !sawCheck {
		t.Fatalf("fixture did not produce both a fix and a check advisory: %#v", advisories)
	}
}

// TestFormatTicketVerifyRendersAdvisoriesWithoutAmendRecipe is the standalone
// half of the same comparison: same verdict, same advisories, minus the recipe
// sentence — and advisories must never fire the "should be addressed or
// explicitly accepted" next_instruction, which is wrong for a no-action-needed
// ancestor note.
func TestFormatTicketVerifyRendersAdvisoriesWithoutAmendRecipe(t *testing.T) {
	root, paths := ticketGraphAdvisoryFixture(t)

	result, err := wsdoc.TicketVerify(root, paths)
	if err != nil {
		t.Fatalf("TicketVerify returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("result.OK = false, want true; findings = %#v", result.Findings)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("Warnings = %#v, want none so the next_instruction assertion is non-vacuous", result.Warnings)
	}

	text := formatTicketVerify(result)
	for _, want := range []string{"verify: PASS", "## Parent Board", "Parent [1]: 260726-refactor-graph-parent", "FIX:", "CHECK:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("formatTicketVerify output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "--amend") {
		t.Fatalf("standalone verify output leaked the commit-path amend recipe:\n%s", text)
	}
	if strings.Contains(text, "should be addressed or explicitly accepted") {
		t.Fatalf("advisories wrongly fired the warning next_instruction:\n%s", text)
	}
}

// TestVerifyAdapterDegradesToSilenceOnGraphLoadFailure closes the commit-side
// half of the degrade-to-silence invariant. The wsdoc tests prove no advisories
// are produced; this proves the commit still lands, since wsgit.Commit vetoes
// on a non-nil verifier error and nothing else.
func TestVerifyAdapterDegradesToSilenceOnGraphLoadFailure(t *testing.T) {
	root := t.TempDir()
	// No ai-docs/spec, so the spec-anchor scan fails. The dangling related:
	// would otherwise produce a FIX:, which makes the silence non-vacuous.
	path := "ai-docs/tickets/.done/260726-feat-graph-load-failure.md"
	mustWrite(t, root, path,
		"---\ntitle: Load failure\ncompleted: 2026-07-27\nrelated:\n  260726-nope-dangling: no such stem\n---\n\n# Load failure\n")

	advisories, err := verifyAdapter(root, []string{path})
	if err != nil {
		t.Fatalf("a graph-load failure vetoed the commit: %v", err)
	}
	if len(advisories) != 0 {
		t.Fatalf("advisories = %#v, want none after a graph-load failure", advisories)
	}
}

// TestServeStdioGitCommitSurfacesTicketGraphParentBoard drives the real MCP
// dispatch: a commit closing the last child of an epic must land, keep OK
// unchanged, and carry the ## Parent Board block in the text response — while
// format:"json" stays advisory-free, since CommitResult.Advisories is tagged
// json:"-".
func TestServeStdioGitCommitSurfacesTicketGraphParentBoard(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "file.txt", "one\n")
	runGit(t, root, "add", "file.txt")
	runGit(t, root, "commit", "-m", "initial")

	mustWrite(t, root, "ai-docs/spec/demo.md", "# Demo\n\n## Anchor {#260101-demo-anchor}\n")
	mustWrite(t, root, "ai-docs/tickets/todo/260726-epic-graph-e2e.md",
		"---\ntitle: E2E epic\n---\n\n# E2E epic\n")

	server := NewServer(root, "test")

	textChild := "ai-docs/tickets/.done/260726-feat-graph-e2e-text.md"
	// The dangling related: makes this commit carry both a multi-line board
	// block and a single-block FIX:, which is the adjacency that needs a blank
	// separator in formatGitCommit.
	mustWrite(t, root, textChild,
		"---\ntitle: E2E child\ncompleted: 2026-07-27\nparent: 260726-epic-graph-e2e\nrelated:\n  260726-nope-dangling: no such stem\n---\n\n# E2E child\n")

	var out bytes.Buffer
	textInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":[%q],"title":"docs(ticket): close the last child of an epic","ai_context":["User intent: prove the parent board reaches the git.commit response."]}}}`, textChild)
	if err := serveStdioWithSession(t, server, root, textInput, &out); err != nil {
		t.Fatalf("ServeStdio commit returned error: %v", err)
	}
	textLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(textLines) != 1 {
		t.Fatalf("expected 1 commit response, got %d\n%s", len(textLines), out.String())
	}
	textByID := responseLinesByID(t, textLines)
	if toolIsError(t, textByID["1"]) {
		t.Fatalf("git.commit vetoed a commit whose only output is advisory: %s", toolText(t, textByID["1"]))
	}
	text := toolText(t, textByID["1"])
	for _, want := range []string{"commit: ", "advisories:\n", "## Parent Board", "Parent [1]: 260726-epic-graph-e2e", "(just now)", "ACTION:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("git.commit text response missing %q: %q", want, text)
		}
	}
	// The board block and the FIX: advisory must not run together.
	if !strings.Contains(text, "\n\n  FIX:") {
		t.Fatalf("git.commit text response missing the blank separator before the FIX: advisory: %q", text)
	}
	if !strings.Contains(text, "Then git commit --amend --no-edit.") {
		t.Fatalf("git.commit text response missing the amend recipe: %q", text)
	}

	jsonChild := "ai-docs/tickets/.done/260726-feat-graph-e2e-json.md"
	mustWrite(t, root, jsonChild,
		"---\ntitle: E2E child json\ncompleted: 2026-07-27\nparent: 260726-epic-graph-e2e\n---\n\n# E2E child json\n")

	out.Reset()
	jsonInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":[%q],"title":"docs(ticket): close another child of the epic","ai_context":["User intent: prove the json:\"-\" tag holds for graph advisories."],"format":"json"}}}`, jsonChild)
	if err := serveStdioWithSession(t, server, root, jsonInput, &out); err != nil {
		t.Fatalf("ServeStdio JSON commit returned error: %v", err)
	}
	jsonLines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(jsonLines) != 1 {
		t.Fatalf("expected 1 JSON commit response, got %d\n%s", len(jsonLines), out.String())
	}
	jsonByID := responseLinesByID(t, jsonLines)
	if toolIsError(t, jsonByID["2"]) {
		t.Fatalf("git.commit reported an error in JSON mode: %s", toolText(t, jsonByID["2"]))
	}
	jsonText := toolText(t, jsonByID["2"])
	if strings.Contains(jsonText, "Parent Board") || strings.Contains(jsonText, "advisories") {
		t.Fatalf("git.commit JSON response leaked advisories despite the json:\"-\" tag: %q", jsonText)
	}
}

// aiContextDebugEvents fetches runtime.debug_events (a process-wide ring
// buffer shared by every test in this package's binary — see appendDebugEvent
// in server.go) via a standalone ServeStdio call and returns every
// "git.commit.ai_context_received" event currently in the buffer, in
// insertion order. It is called both before and after the calls under test so
// the test can diff the two snapshots instead of asserting an absolute count,
// which would be polluted both by other tests' git.commit calls and by
// repeated invocations of this same test under `go test -count=N` (all
// sharing the one process-wide buffer).
func aiContextDebugEvents(t *testing.T, server *Server, root string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	input := `{"jsonrpc":"2.0","id":"debug","method":"tools/call","params":{"name":"runtime.debug_events","arguments":{"limit":256}}}`
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio debug_events returned error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 debug_events response, got %d\n%s", len(lines), out.String())
	}
	text := toolText(t, responseLinesByID(t, lines)["debug"])
	var events []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("parse debug event line %q: %v", line, err)
		}
		if event["event"] == "git.commit.ai_context_received" {
			events = append(events, event)
		}
	}
	return events
}

func TestServeStdioGitCommitAIContextConditionsAndDebugEvent(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "file.txt", "one\n")
	runGit(t, root, "add", "file.txt")
	runGit(t, root, "commit", "-m", "initial")
	mustWrite(t, root, "file.txt", "one\ntwo\n")

	server := NewServer(root, "test")

	// Snapshot the debug-event ring buffer before issuing any of this test's
	// git.commit calls (see aiContextDebugEvents doc comment for why this is
	// a diff, not an absolute-count, assertion).
	before := aiContextDebugEvents(t, server, root)

	largeAIContext := make([]string, 0, 60)
	for i := 0; i < 60; i++ {
		largeAIContext = append(largeAIContext, fmt.Sprintf("Entry %d: %s", i, strings.Repeat("context detail", 20)))
	}
	largeAIContextJSON, err := json.Marshal(largeAIContext)
	if err != nil {
		t.Fatal(err)
	}

	commitInput := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["file.txt"],"title":"test: empty ai_context array","ai_context":[]}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["file.txt"],"title":"test: absent ai_context field"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["file.txt"],"title":"test: single empty-string entry","ai_context":[""]}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["file.txt"],"title":"test: single whitespace entry","ai_context":["  "]}}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["file.txt"],"title":"test: large valid ai_context array","ai_context":%s}}}`, largeAIContextJSON),
	}, "\n")

	var out bytes.Buffer
	if err := serveStdioWithSession(t, server, root, commitInput, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 responses, got %d\n%s", len(lines), out.String())
	}
	byID := responseLinesByID(t, lines)

	emptyArrayText := toolText(t, byID["1"])
	if !strings.Contains(byID["1"], `"isError":true`) || !strings.Contains(emptyArrayText, "received an empty array") {
		t.Fatalf("git.commit with empty ai_context array = %s", byID["1"])
	}
	absentFieldText := toolText(t, byID["2"])
	if !strings.Contains(byID["2"], `"isError":true`) || !strings.Contains(absentFieldText, "no ai_context field was received") {
		t.Fatalf("git.commit with absent ai_context field = %s", byID["2"])
	}
	emptyStringEntryText := toolText(t, byID["3"])
	if !strings.Contains(byID["3"], `"isError":true`) || !strings.Contains(emptyStringEntryText, "received 1 entry, all blank") {
		t.Fatalf("git.commit with [\"\"] ai_context = %s", byID["3"])
	}
	whitespaceEntryText := toolText(t, byID["4"])
	if !strings.Contains(byID["4"], `"isError":true`) || !strings.Contains(whitespaceEntryText, "received 1 entry, all blank") {
		t.Fatalf("git.commit with [\"  \"] ai_context = %s", byID["4"])
	}
	// [""] and ["  "] must classify identically ("all blank"), not one as
	// "empty array" and the other as "all blank" — this is the condition the
	// review finding flagged: stringList used to silently drop exact-empty
	// entries before wsgit ever saw them.
	if emptyStringEntryText != whitespaceEntryText {
		t.Fatalf("[\"\"] and [\"  \"] ai_context reported different conditions:\n%q\nvs\n%q", emptyStringEntryText, whitespaceEntryText)
	}
	largeArrayResultText := toolText(t, byID["5"])
	if strings.Contains(byID["5"], `"isError":true`) {
		t.Fatalf("git.commit with a large valid ai_context array was rejected: %s", byID["5"])
	}
	if !strings.Contains(largeArrayResultText, "commit: ") {
		t.Fatalf("git.commit with a large valid ai_context array = %s", largeArrayResultText)
	}

	after := aiContextDebugEvents(t, server, root)
	if len(after) != len(before)+5 {
		t.Fatalf("expected 5 new git.commit.ai_context_received events, got %d before and %d after", len(before), len(after))
	}
	newEvents := after[len(before):]

	// The five git.commit calls above run as concurrent goroutines (see
	// ServeStdio's per-line `go func` in server.go), so their completion
	// order — and thus their append order into the ring buffer — is not
	// guaranteed. Classify the five new events by their own field values
	// instead of assuming array position.
	var emptyArrayEvent, absentFieldEvent, largeArrayEvent map[string]any
	var blankEntryEvents []map[string]any
	for _, event := range newEvents {
		switch {
		case event["present"] == false:
			absentFieldEvent = event
		case event["raw_entry_count"] == float64(0):
			emptyArrayEvent = event
		case event["raw_entry_count"] == float64(1) && event["post_trim_entry_count"] == float64(0):
			blankEntryEvents = append(blankEntryEvents, event)
		case event["raw_entry_count"] == float64(60):
			largeArrayEvent = event
		}
	}
	if emptyArrayEvent == nil || emptyArrayEvent["present"] != true || emptyArrayEvent["raw_bytes"] != float64(0) || emptyArrayEvent["post_trim_entry_count"] != float64(0) {
		t.Fatalf("empty-array debug event = %#v", emptyArrayEvent)
	}
	if absentFieldEvent == nil || absentFieldEvent["raw_entry_count"] != float64(-1) || absentFieldEvent["raw_bytes"] != float64(0) || absentFieldEvent["post_trim_entry_count"] != float64(0) {
		t.Fatalf("absent-field debug event = %#v", absentFieldEvent)
	}
	// One blank-entry event from [""] (raw_bytes 0) and one from ["  "]
	// (raw_bytes 2); both must report post_trim_entry_count 0 — this is the
	// field-accuracy fix: a naive non-empty-string count would have reported
	// 1 here instead of 0.
	if len(blankEntryEvents) != 2 {
		t.Fatalf("expected 2 blank-entry debug events ([\"\"] and [\"  \"]), got %d: %#v", len(blankEntryEvents), blankEntryEvents)
	}
	sawEmptyStringBytes, sawWhitespaceBytes := false, false
	for _, event := range blankEntryEvents {
		if event["present"] != true {
			t.Fatalf("blank-entry debug event missing present=true: %#v", event)
		}
		switch event["raw_bytes"] {
		case float64(0):
			sawEmptyStringBytes = true
		case float64(2):
			sawWhitespaceBytes = true
		}
	}
	if !sawEmptyStringBytes || !sawWhitespaceBytes {
		t.Fatalf("blank-entry debug events missing expected raw_bytes (0 for [\"\"], 2 for [\"  \"]): %#v", blankEntryEvents)
	}
	if largeArrayEvent == nil || largeArrayEvent["present"] != true || largeArrayEvent["post_trim_entry_count"] != float64(60) {
		rawBytes, _ := largeArrayEvent["raw_bytes"].(float64)
		t.Fatalf("large-array debug event = %#v (raw_bytes=%v)", largeArrayEvent, rawBytes)
	}
	if rawBytes, _ := largeArrayEvent["raw_bytes"].(float64); rawBytes < 5000 {
		t.Fatalf("large-array debug event raw_bytes too small: %v", rawBytes)
	}
}

func TestServeStdioReferencesTraceRejectsAmbiguousSelectors(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/todo/260504-ticket-demo.md", "---\ntitle: Demo\n---\n# Demo\n")
	mustWrite(t, root, "ai-docs/spec/demo.md", "# Demo\n\n## Feature {#260504-spec-demo}\n")
	mustWrite(t, root, "ai-docs/mental-model/demo.md", "---\ndomain: demo\n---\n# Demo\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"references.trace","arguments":{"ticket_stem":"260504-ticket-demo","spec_stem":"260504-spec-demo"}}}` + "\n"

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	text := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["1"])
	if !strings.Contains(text, "exactly one") || !strings.Contains(out.String(), `"isError":true`) {
		t.Fatalf("references.trace accepted ambiguous selectors: %s", out.String())
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
	largeText := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["4"])
	if strings.Contains(largeText, strings.Repeat("x", 100)) || !strings.Contains(largeText, "combined_bytes: 5000") || !strings.Contains(largeText, "exec.raw.*") {
		t.Fatalf("large response = %s", largeText)
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
	out.Reset()
	input := fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"exec.result","arguments":{"exec_key":%q,"timeout_seconds":3}}}`, running) + "\n"
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

// TestMercenaryDefaultHideAndOnVisibility verifies that:
//   - ws.mercenary.* tools are hidden from tools/list by default (no config),
//   - the generic config.tune writer remains visible so the lead can still
//     toggle workflow.prefer_mercenary (the per-knob config.workflow_prefer_mercenary
//     tool was collapsed into config.tune in 260814 Phase 2), and
//   - after writing workflow.prefer_mercenary=on to global config, ws.mercenary.*
//     tools appear in tools/list.
func TestMercenaryDefaultHideAndOnVisibility(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")

	// Default (unset): ws.mercenary.* must be hidden.
	listResp := callToolsList(t, server)
	if strings.Contains(listResp, `"name":"mercenary.call"`) {
		t.Fatalf("ws.mercenary.call must be hidden by default: %s", listResp)
	}
	if strings.Contains(listResp, `"name":"mercenary.register"`) {
		t.Fatalf("ws.mercenary.register must be hidden by default: %s", listResp)
	}
	if strings.Contains(listResp, `"name":"ws.lead.prefer_mercenary"`) {
		t.Fatalf("removed ws.lead.prefer_mercenary must not be visible: %s", listResp)
	}
	if !strings.Contains(listResp, `"name":"config.tune"`) {
		t.Fatalf("config.tune must be visible even when mercenary hidden (lead toggles workflow.prefer_mercenary through it): %s", listResp)
	}

	// After enabling: ws.mercenary.* must appear in tools/list.
	mustEnableMercenary(t)
	listRespOn := callToolsList(t, server)
	if !strings.Contains(listRespOn, `"name":"mercenary.call"`) {
		t.Fatalf("ws.mercenary.call must be visible when prefer_mercenary=on: %s", listRespOn)
	}
	if !strings.Contains(listRespOn, `"name":"mercenary.register"`) {
		t.Fatalf("ws.mercenary.register must be visible when prefer_mercenary=on: %s", listRespOn)
	}
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
