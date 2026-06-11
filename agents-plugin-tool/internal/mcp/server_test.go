package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdoc"
)

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

func assertCompactActorID(t *testing.T, actorID, authority string) {
	t.Helper()
	prefix := authority + "-"
	if !strings.HasPrefix(actorID, prefix) || len(strings.TrimPrefix(actorID, prefix)) != 8 {
		t.Fatalf("actor id %q does not match compact %s-prefixed shape", actorID, authority)
	}
	for _, r := range strings.TrimPrefix(actorID, prefix) {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			t.Fatalf("actor id %q contains non-lowercase base36 payload character %q", actorID, r)
		}
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

func TestServeStdioConfigShow(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	cache := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cache)

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.show","arguments":{"format":"json"}}}`+"\n",
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

	if _, err := wsconfig.SetAgentsTier(wsconfig.Options{}, "light", "", "gemini-3-1-pro", "low"); err != nil {
		t.Fatalf("SetAgentsTier returned error: %v", err)
	}
	out.Reset()
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.show","arguments":{"format":"json"}}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	showAfter := toolText(t, byID["2"])
	if !strings.Contains(showAfter, `"backend":"gemini"`) || !strings.Contains(showAfter, `"model":"gemini-3-1-pro"`) || !strings.Contains(showAfter, `"effort":"low"`) {
		t.Fatalf("config.show response missing tier mapping: %s", byID["2"])
	}

	out.Reset()
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"config.show","arguments":{}}}`+"\n",
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

func TestServeStdioConfigAgentsTier(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.agents_tier","arguments":{"tier":"light","model":"gemini-3-1-pro"}}}`+"\n",
	), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	configText := toolText(t, byID["1"])
	if !strings.Contains(configText, `"backend":"gemini"`) || !strings.Contains(configText, `"model":"gemini-3-1-pro"`) {
		t.Fatalf("config response missing tier mapping: %s", byID["1"])
	}

}

func TestAPIRuntimeUsesObservedHarness(t *testing.T) {
	server := NewServer(t.TempDir(), "test")
	server.observeHarness("test", "claude")
	runtime, ok := server.apiRuntime().(wsagentAPIRuntime)
	if !ok {
		t.Fatalf("apiRuntime returned %T", server.apiRuntime())
	}
	if runtime.harness != "claude" {
		t.Fatalf("api runtime harness = %q", runtime.harness)
	}
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
	if !strings.Contains(byID["1"], "agents.register") || !strings.Contains(byID["1"], "agents.call") || !strings.Contains(byID["1"], "config.show") {
		t.Fatalf("default profile hid lead tools: %s", byID["1"])
	}
}

func TestWsflowOnlyToolHiddenInFullWsMode(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	// Full ws mode: WS_MCP_NO_AGENT is NOT set.
	t.Setenv("WS_MCP_NO_AGENT", "")
	t.Setenv("WS_MCP_NAMESPACE", "")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"prompt.render","arguments":{"stem":"code-reviewer"}}}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))

	// tools/list must NOT include prompt.render in full ws mode.
	list := byID["1"]
	if strings.Contains(list, "prompt.render") {
		t.Fatalf("tools/list exposed wsflow-only tool prompt.render in full ws mode: %s", list)
	}

	// Explicit call must return a JSON-RPC error (not isError content).
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
		t.Fatalf("prompt.render in full ws mode did not return JSON-RPC error: %s", byID["2"])
	}
	if !strings.Contains(callResp.Error.Message, "prompt.render") {
		t.Fatalf("prompt.render full ws error missing tool name: %s", callResp.Error.Message)
	}
}

func TestRenderPromptSubstitutionAndAllowlist(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_MCP_NAMESPACE", "wsflow")

	// (a) Namespace substitution replaces ws/ and ws: at word boundaries only.
	//     Tokens like "workflows/", "news/", "rows:" must NOT be mangled.
	nonManglingCases := []struct {
		input string
		want  string
	}{
		{"use ws/specs.find here", "use wsflow/specs.find here"},
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

	// (b) Render code-reviewer; the rendered file must contain wsflow/ (ws/ substituted)
	//     and must not contain bare ws/ tokens.
	path1, err := renderPrompt(root, "code-reviewer", nil)
	if err != nil {
		t.Fatalf("renderPrompt code-reviewer: %v", err)
	}
	data, err := os.ReadFile(path1)
	if err != nil {
		t.Fatalf("read rendered prompt: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "ws/") {
		t.Errorf("rendered code-reviewer still contains 'ws/' after substitution")
	}
	if !strings.Contains(text, "wsflow/") {
		t.Errorf("rendered code-reviewer missing 'wsflow/' after substitution")
	}

	// (b-cont) ws: -> wsflow: substitution: verified via regex unit cases above.
	//         Also confirm a synthetic text with ws: is substituted correctly.
	synth := wsNamespaceRef.ReplaceAllString("invoke ws:some-skill here", "wsflow"+"$1")
	if synth != "invoke wsflow:some-skill here" {
		t.Errorf("ws: substitution failed: %q", synth)
	}

	// (c) Ineligible stem and unknown stem both return errors.
	if _, err := renderPrompt(root, "implementer", nil); err == nil {
		t.Error("renderPrompt with ineligible stem 'implementer' returned nil error")
	} else if !strings.Contains(err.Error(), "not render-eligible") {
		t.Errorf("ineligible stem error message unexpected: %v", err)
	}
	if _, err := renderPrompt(root, "no-such-prompt", nil); err == nil {
		t.Error("renderPrompt with unknown stem 'no-such-prompt' returned nil error")
	} else if !strings.Contains(err.Error(), "not render-eligible") {
		t.Errorf("unknown stem error message unexpected: %v", err)
	}

	// (d) Context values containing ws/ are NOT substituted (context is appended
	//     after the substitution pass).
	path2, err := renderPrompt(root, "code-reviewer", map[string]string{
		"note": "see ws/specs.find for details",
	})
	if err != nil {
		t.Fatalf("renderPrompt with context: %v", err)
	}
	data2, err := os.ReadFile(path2)
	if err != nil {
		t.Fatalf("read context-rendered prompt: %v", err)
	}
	text2 := string(data2)
	if !strings.Contains(text2, "## Render Context") {
		t.Error("context-rendered prompt missing ## Render Context block")
	}
	if !strings.Contains(text2, "ws/specs.find") {
		t.Error("context value containing 'ws/specs.find' was unexpectedly substituted in the context block")
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
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
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
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
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

func TestServeStdioFiltersToolsByProfile(t *testing.T) {
	t.Setenv("WS_MCP_ALLOWED_TOOLS", "")
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_MCP_TOOL_PROFILE", "leaf")
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agents.status","arguments":{"name":"impl"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"runtime.info","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"config.agents_tier","arguments":{"tier":"light","model":"gpt-5.2"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"config.show","arguments":{}}}`,
	}, "\n")

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if strings.Contains(byID["1"], "agents.status") || strings.Contains(byID["1"], "subquery") || strings.Contains(byID["1"], "config.agents_tier") || strings.Contains(byID["1"], "config.show") {
		t.Fatalf("leaf tools/list exposed recursive tools: %s", byID["1"])
	}
	if strings.Contains(byID["1"], "ws.setup") {
		t.Fatalf("leaf tools/list exposed setup mutation tool: %s", byID["1"])
	}
	if !strings.Contains(byID["1"], "runtime.info") {
		t.Fatalf("leaf tools/list hid runtime.info: %s", byID["1"])
	}
	if !strings.Contains(byID["2"], "tool not available") {
		t.Fatalf("leaf tools/call did not reject agents.status: %s", byID["2"])
	}
	if !strings.Contains(byID["3"], "prompt_bundle") {
		t.Fatalf("leaf tools/call rejected runtime.info: %s", byID["3"])
	}
	if !strings.Contains(byID["4"], "tool not available") {
		t.Fatalf("leaf tools/call did not reject config.agents_tier: %s", byID["4"])
	}
	if !strings.Contains(byID["5"], "tool not available") {
		t.Fatalf("leaf tools/call did not reject config.show: %s", byID["5"])
	}
}

func useLeadProfile(t *testing.T) {
	t.Helper()
	t.Setenv("WS_MCP_TOOL_PROFILE", "lead")
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

func initGit(t *testing.T, root string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, string(out))
	}
	runGit(t, root, "config", "core.autocrlf", "false")
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

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"tickets.find","arguments":{"spec_stem":"260504-demo"}}}` + "\n"

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	text := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["1"])
	if !strings.Contains(text, "ticket_stem") || !strings.Contains(out.String(), `"isError":true`) {
		t.Fatalf("tickets.find accepted spec_stem argument: %s", out.String())
	}
}

func TestServeStdioSpecToolsRejectTicketOnlyArgument(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/spec/demo.md", "# Demo\n\n## Feature {#260504-spec-demo}\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"specs.status","arguments":{"ticket_stem":"260504-ticket-demo"}}}` + "\n"

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	text := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["1"])
	if !strings.Contains(text, "spec_stem") || !strings.Contains(out.String(), `"isError":true`) {
		t.Fatalf("specs.status accepted ticket_stem argument: %s", out.String())
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
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	text := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["1"])
	if !strings.Contains(text, "domain or path") || !strings.Contains(out.String(), `"isError":true`) {
		t.Fatalf("mental_models.status accepted spec_stem argument: %s", out.String())
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

type fakeAPIRuntime struct {
	mu          sync.Mutex
	routeCalls  []string
	routeOutput string
	routeErr    error
	managerHits []string
	answers     map[string]string
	errs        map[string]error
	active      map[string]int
	maxActive   map[string]int
	delay       time.Duration
}

func (f *fakeAPIRuntime) Route(ctx context.Context, root, prompt string) (string, error) {
	f.mu.Lock()
	f.routeCalls = append(f.routeCalls, prompt)
	output := f.routeOutput
	err := f.routeErr
	f.mu.Unlock()
	if output == "" {
		output = "go\npython\n"
	}
	return output, err
}

func (f *fakeAPIRuntime) AskManager(ctx context.Context, root, domain, prompt string) (string, error) {
	f.mu.Lock()
	if f.active == nil {
		f.active = map[string]int{}
	}
	if f.maxActive == nil {
		f.maxActive = map[string]int{}
	}
	f.managerHits = append(f.managerHits, domain+":"+prompt)
	f.active[domain]++
	if f.active[domain] > f.maxActive[domain] {
		f.maxActive[domain] = f.active[domain]
	}
	f.mu.Unlock()
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	f.mu.Lock()
	f.active[domain]--
	answer := "answer for " + domain
	if f.answers != nil && f.answers[domain] != "" {
		answer = f.answers[domain]
	}
	err := error(nil)
	if f.errs != nil {
		err = f.errs[domain]
	}
	f.mu.Unlock()
	return answer, err
}

func TestAPIAskExistingDomainMentionStillUsesRouter(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/.deps/ratatui/README.md", "ratatui")
	fake := &fakeAPIRuntime{
		routeOutput: "ratatui\n",
		answers:     map[string]string{"ratatui": "ratatui answer"},
	}
	server := NewServer(root, "test")
	server.api = fake
	text, err := server.askAPI(context.Background(), root, "For ratatui, how do I render a widget?", "")
	if err != nil {
		t.Fatalf("askAPI returned error: %v\n%s", err, text)
	}
	if len(fake.routeCalls) != 1 {
		t.Fatalf("existing domain mention did not invoke pre-router: %v", fake.routeCalls)
	}
	if !strings.Contains(text, "## Domain: ratatui\nratatui answer") {
		t.Fatalf("api.ask response missing existing domain answer:\n%s", text)
	}
}

func TestAPIAskRecoversExistingDomainFromRouterProse(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/.deps/ratatui/README.md", "ratatui")
	fake := &fakeAPIRuntime{
		routeOutput: "Use frame.render_widget for ratatui widgets.\n",
		answers:     map[string]string{"ratatui": "ratatui answer"},
	}
	server := NewServer(root, "test")
	server.api = fake
	text, err := server.askAPI(context.Background(), root, "For ratatui, how do I render a widget?", "")
	if err != nil {
		t.Fatalf("askAPI returned error: %v\n%s", err, text)
	}
	if !strings.Contains(text, "## Domain: ratatui\nratatui answer") {
		t.Fatalf("api.ask response missing recovered existing domain answer:\n%s", text)
	}
}

func TestAPIAskPreRouterPartialFailureBoundaries(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/.deps/go/README.md", "go")
	mustWrite(t, root, "ai-docs/.deps/python/README.md", "python")
	fake := &fakeAPIRuntime{answers: map[string]string{"go": "go ok"}, errs: map[string]error{"python": fmt.Errorf("python unavailable")}}
	server := NewServer(root, "test")
	server.api = fake
	text, err := server.askAPI(context.Background(), root, "Compare clients", "")
	if err != nil {
		t.Fatalf("partial success should not error: %v\n%s", err, text)
	}
	if len(fake.routeCalls) != 1 || !strings.Contains(fake.routeCalls[0], "Existing domains:\ngo\npython\nPrompt: Compare clients") {
		t.Fatalf("pre-router input mismatch: %#v", fake.routeCalls)
	}
	if !strings.Contains(text, "## Domain: go\ngo ok") || !strings.Contains(text, "## Domain: python\nERROR: python unavailable") {
		t.Fatalf("partial response missing boundaries:\n%s", text)
	}
}

func TestAPIAskRejectsMalformedRouterDomainSlug(t *testing.T) {
	root := t.TempDir()
	fake := &fakeAPIRuntime{routeOutput: "1. go\ngo docs\ngo:latest\n"}
	server := NewServer(root, "test")
	server.api = fake
	_, err := server.askAPI(context.Background(), root, "question", "")
	if err == nil || !strings.Contains(err.Error(), "invalid domain") {
		t.Fatalf("expected invalid domain error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "ai-docs", ".deps", "1. go")); !os.IsNotExist(statErr) {
		t.Fatalf("malformed router output created cache directory, stat err=%v", statErr)
	}
}

func TestAPIManagerExpiredUsesRecentUseTimestamp(t *testing.T) {
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-apiManagerTTL + time.Second).Format(time.RFC3339)
	old := now.Add(-apiManagerTTL - time.Second).Format(time.RFC3339)
	if apiManagerExpired(wsagent.Agent{LastCallAt: recent, CreatedAt: old}, now) {
		t.Fatal("recent last call expired")
	}
	if !apiManagerExpired(wsagent.Agent{LastCallAt: old}, now) {
		t.Fatal("old last call did not expire")
	}
	if !apiManagerExpired(wsagent.Agent{LastSeenAt: old}, now) {
		t.Fatal("old last seen did not expire")
	}
}

func TestAPIAskSameDomainCallsSerialize(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/.deps/go/README.md", "go")
	fake := &fakeAPIRuntime{delay: 50 * time.Millisecond}
	server := NewServer(root, "test")
	server.api = fake
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := server.askAPI(context.Background(), root, "question", "go")
			if err != nil {
				t.Errorf("askAPI returned error: %v", err)
			}
		}()
	}
	wg.Wait()
	fake.mu.Lock()
	max := fake.maxActive["go"]
	fake.mu.Unlock()
	if max != 1 {
		t.Fatalf("same-domain calls were not serialized; max active = %d", max)
	}
}
