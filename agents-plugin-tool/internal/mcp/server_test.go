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
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wsstore"
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

func TestRawPublicActorOwnedToolSchemasOmitRoot(t *testing.T) {
	actorOwnedTools := 0
	for _, tool := range tools() {
		name, _ := tool["name"].(string)
		schema, _ := tool["inputSchema"].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		if strings.HasPrefix(name, "agents.") || name == "subquery" {
			actorOwnedTools++
			if _, ok := properties["root"]; ok {
				t.Fatalf("raw public actor-owned tool %s advertises root", name)
			}
			continue
		}
		if name == "git.status" {
			if _, ok := properties["root"]; !ok {
				t.Fatalf("non-agent root-aware tool %s no longer advertises root", name)
			}
		}
	}
	if actorOwnedTools == 0 {
		t.Fatal("raw tools list has no public actor-owned tools")
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

func TestServeStdioToolsListAndCall(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	mustWrite(t, root, "ai-docs/spec/demo.md", "---\ntitle: Demo\nfeatures:\n  - planned [260503-feat-demo/p1]\n---\n# Demo\n\n## Feature {#260503-spec-demo}\n\nSpec discovery text.\n")
	mustWrite(t, root, "ai-docs/mental-model/workflow.md", "---\ndomain: workflow\ndescription: Workflow model\nsources:\n  - ai-docs/spec/demo.md#260503-spec-demo\n---\n# Workflow\n\nReferences {#260503-spec-demo} with discovery text.\n")
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
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"runtime.info","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"git.status","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"runtime.debug_events","arguments":{"limit":10}}}`,
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"config.show","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"tickets.find","arguments":{"mentions_ticket_stem":"260503-epic-demo"}}}`,
		`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"specs.find","arguments":{"spec_stem":"260503-spec-demo","ticket_stem":"260503-feat-demo","query":"discovery"}}}`,
		`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"mental_models.find","arguments":{"spec_stem":"260503-spec-demo","domain":"workflow","query":"discovery"}}}`,
		`{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"specs.find","arguments":{"query":"discovery","format":"json"}}}`,
		`{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"mental_models.find","arguments":{"query":"discovery","format":"json"}}}`,
		`{"jsonrpc":"2.0","id":15,"method":"tools/call","params":{"name":"references.trace","arguments":{"spec_stem":"260503-spec-demo"}}}`,
	}, "\n")

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
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
	if !strings.Contains(byID["2"], "agents.call") {
		t.Fatalf("tools/list missing agents.call: %s", byID["2"])
	}
	if strings.Contains(byID["2"], "agents.call_async") {
		t.Fatalf("tools/list still includes agents.call_async: %s", byID["2"])
	}
	if strings.Contains(byID["2"], "agents.oneshot") {
		t.Fatalf("tools/list still includes agents.oneshot: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "subquery") {
		t.Fatalf("tools/list missing subquery: %s", byID["2"])
	}
	subqueryProperties := toolPropertiesByName(t, byID["2"], "subquery")
	if _, ok := subqueryProperties["root"]; ok {
		t.Fatalf("subquery publicly advertises root in schema: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "path.generate") {
		t.Fatalf("tools/list missing path.generate: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "runtime.info") {
		t.Fatalf("tools/list missing runtime.info: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "ws.setup") {
		t.Fatalf("tools/list missing ws.setup: %s", byID["2"])
	}
	setupProperties := toolPropertiesByName(t, byID["2"], "ws.setup")
	if _, ok := setupProperties["format"]; ok {
		t.Fatalf("ws.setup publicly advertises hidden format property: %s", byID["2"])
	}
	if _, ok := setupProperties["root"]; !ok {
		t.Fatalf("ws.setup schema missing root property: %s", byID["2"])
	}
	if strings.Contains(byID["2"], "session.set_default_root") || strings.Contains(byID["2"], "session.get_default_root") {
		t.Fatalf("tools/list still advertises session root compatibility tools: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "config.agents_tier") {
		t.Fatalf("tools/list missing config.agents_tier: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], `"effort"`) || !strings.Contains(byID["2"], `""`) || !strings.Contains(byID["2"], `"xhigh"`) {
		t.Fatalf("tools/list missing config.agents_tier effort schema values: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "config.show") {
		t.Fatalf("tools/list missing config.show: %s", byID["2"])
	}
	if !strings.Contains(byID["2"], "\"prompts\"") {
		t.Fatalf("tools/list missing prompts field: %s", byID["2"])
	}
	toolsResult, _ := listResp["result"].(map[string]any)
	listedTools, _ := toolsResult["tools"].([]any)
	for _, rawTool := range listedTools {
		tool, _ := rawTool.(map[string]any)
		name, _ := tool["name"].(string)
		if !strings.HasPrefix(name, "agents.") {
			continue
		}
		schema, _ := tool["inputSchema"].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		if _, ok := properties["root"]; ok {
			t.Fatalf("agents tool %s publicly advertises root in schema: %s", name, byID["2"])
		}
	}
	for _, tool := range []string{"agents.wait", "agents.result", "agents.status", "agents.tail", "agents.debug.tail", "agents.debug.stdout", "agents.debug.stderr", "agents.debug.runtime_log", "agents.debug.events", "agents.cancel", "git.status", "git.diff", "git.log", "git.merge_base", "git.commit", "tickets.list", "tickets.find", "tickets.status", "specs.list", "specs.find", "specs.status", "mental_models.find", "mental_models.status", "references.trace"} {
		if !strings.Contains(byID["2"], tool) {
			t.Fatalf("tools/list missing %s: %s", tool, byID["2"])
		}
	}
	if !strings.Contains(byID["2"], `"mental_model_notes"`) {
		t.Fatalf("tools/list missing git.commit mental_model_notes schema: %s", byID["2"])
	}
	if strings.Contains(byID["2"], "agents.recall") {
		t.Fatalf("tools/list should not advertise agents.recall: %s", byID["2"])
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
	if !strings.Contains(byID["6"], "prompt_bundle") || !strings.Contains(byID["6"], "code-reviewer") {
		t.Fatalf("runtime.info response missing prompt bundle: %s", byID["6"])
	}
	if !strings.Contains(toolText(t, byID["7"]), "dirty:") || !strings.Contains(toolText(t, byID["7"]), "ai-docs/") {
		t.Fatalf("git.status response missing readable status: %s", byID["7"])
	}
	if !strings.Contains(toolText(t, byID["8"]), `"event":"request.received"`) {
		t.Fatalf("runtime.debug_events missing request evidence: %s", byID["8"])
	}
	configText := toolText(t, byID["9"])
	if !strings.Contains(configText, "path:") || !strings.Contains(configText, `config.json`) || !strings.Contains(configText, "model_aliases:") {
		t.Fatalf("config.show response missing path/config: %s", byID["9"])
	}
	ticketsText := toolText(t, byID["10"])
	if !strings.Contains(ticketsText, "260503-feat-demo") || !strings.Contains(ticketsText, "mentions_ticket_stem") {
		t.Fatalf("tickets.find response missing mention result: %s", byID["10"])
	}
	specsText := toolText(t, byID["11"])
	if !strings.Contains(specsText, "1 candidate spec for query=\"discovery\"") || !strings.Contains(specsText, "ai-docs/spec/demo.md\tscore=") || strings.Contains(specsText, "matched:") {
		t.Fatalf("specs.find response missing spec result: %s", byID["11"])
	}
	mentalModelsText := toolText(t, byID["12"])
	if !strings.Contains(mentalModelsText, "1 candidate mental model for query=\"discovery\"") || !strings.Contains(mentalModelsText, "ai-docs/mental-model/workflow.md\tscore=") || strings.Contains(mentalModelsText, "matched:") {
		t.Fatalf("mental_models.find response missing result: %s", byID["12"])
	}
	if !strings.Contains(byID["13"], "matches") || !strings.Contains(byID["13"], "matched_terms") {
		t.Fatalf("specs.find json missing evidence: %s", byID["13"])
	}
	if !strings.Contains(byID["14"], "matches") || !strings.Contains(byID["14"], "matched_terms") {
		t.Fatalf("mental_models.find json missing evidence: %s", byID["14"])
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
	_, layout, err := wsagent.NewManager(wsagent.Options{}).Register(wsagent.RegisterOptions{Root: root, Name: "impl"})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Dir(layout.CurrentStdout), filepath.Base(layout.CurrentStdout), "stdout old\nstdout new\n")
	mustWrite(t, filepath.Dir(layout.CurrentRuntimeLog), filepath.Base(layout.CurrentRuntimeLog), "runtime old\nruntime new\n")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agents.debug.stdout","arguments":{"name":"impl","lines":1}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agents.debug.runtime_log","arguments":{"name":"impl","lines":1}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"agents.debug.tail","arguments":{"name":"impl","lines":1}}}`,
	}, "\n")

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
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
	_, layout, err := wsagent.NewManager(wsagent.Options{}).Register(wsagent.RegisterOptions{Root: root, Name: "impl"})
	if err != nil {
		t.Fatal(err)
	}
	largeOutput := strings.Repeat("x", 5000)
	line := `{"type":"event","aggregated_output":"` + largeOutput + `"}`
	mustWrite(t, filepath.Dir(layout.CurrentStdout), filepath.Base(layout.CurrentStdout), line+"\n")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agents.tail","arguments":{"name":"impl","lines":1}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agents.debug.tail","arguments":{"name":"impl","lines":1}}}`,
	}, "\n")

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
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

	out.Reset()
	server := NewServer(root, "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ws.setup","arguments":{"method":"lead-workflow-bootstrap","root":%q}}}`+"\n", root)), &out); err != nil {
		t.Fatalf("ServeStdio setup returned error: %v", err)
	}
	out.Reset()
	if err := server.ServeStdio(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"agents.register","arguments":{"name":"survey","tier":"light"}}}`+"\n"), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	status, err := wsagent.NewManager(wsagent.Options{}).StatusScoped(root, "survey", server.currentActorID())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "backend: gemini") || !strings.Contains(status, "model: gemini-3-1-pro") {
		t.Fatalf("registered status missing configured backend/model:\n%s", status)
	}
}

func TestServeStdioConfigAgentsTierUsesDetectedHarness(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260513-feat-harness-local-agent-tier-config")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(t.TempDir(), "test")
	var out bytes.Buffer
	initializeInput := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"Claude Code","version":"test"}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(initializeInput), &out); err != nil {
		t.Fatalf("ServeStdio initialize returned error: %v", err)
	}

	out.Reset()
	configInput := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.agents_tier","arguments":{"tier":"core","backend":"codex","model":"gpt-5.4","effort":"medium"}}}`
	if err := server.ServeStdio(context.Background(), strings.NewReader(configInput), &out); err != nil {
		t.Fatalf("ServeStdio config returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	configText := toolText(t, byID["2"])
	if !strings.Contains(configText, `"claude":{"backend":"codex","model":"gpt-5.4","effort":"medium"}`) {
		t.Fatalf("config response missing claude harness mapping: %s", byID["2"])
	}

	out.Reset()
	registerInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"agents.register","arguments":{"root":%q,"name":"reviewer","model":"core"}}}`, root)
	if err := server.ServeStdio(context.Background(), strings.NewReader(registerInput), &out); err != nil {
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

func TestServeStdioConfigAgentsTierOmittedEffortClearsExistingEffort(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260513-feat-agent-tier-effort-config")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	var out bytes.Buffer
	server := NewServer(root, "test")
	inputs := []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"config.agents_tier","arguments":{"tier":"core","harness":"codex","model":"gpt-5.5","effort":"medium"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"config.agents_tier","arguments":{"tier":"core","harness":"codex","model":"gpt-5.4"}}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"agents.register","arguments":{"root":%q,"name":"reviewer","model":"core"},"_meta":{"x-codex-turn-metadata":{"workspaces":{%q:{}}}}}}`, root, root),
	}
	for _, input := range inputs {
		out.Reset()
		if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
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

func TestServeStdioNoAgentModeHidesAgentBackedTools(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_MCP_NO_AGENT", "1")
	t.Setenv("WS_MCP_NAMESPACE", "wsflow")
	t.Setenv("WS_MCP_SETUP_TOOL", "setup")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"api.list","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"agents.call","arguments":{"name":"impl","prompt":"work"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"setup","arguments":{"format":"json"}}}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	server := NewServer(root, "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	list := byID["1"]
	for _, hidden := range []string{"agents.call", "agents.register", "agents.debug.tail", "subquery", "config.agents_tier", "api.ask", "api.ask_async", "api.status", "api.result", "api.cancel", "ws.setup"} {
		if strings.Contains(list, hidden) {
			t.Fatalf("tools/list exposed hidden no-agent tool %s: %s", hidden, list)
		}
	}
	for _, visible := range []string{"api.list", "config.show", "tickets.list", "setup"} {
		if !strings.Contains(list, visible) {
			t.Fatalf("tools/list missing no-agent visible tool %s: %s", visible, list)
		}
	}
	if _, ok := toolPropertiesByName(t, list, "setup")["format"]; ok {
		t.Fatalf("setup alias publicly advertises hidden format property: %s", list)
	}
	if strings.Contains(list, "ws MCP") || !strings.Contains(list, "wsflow MCP") {
		t.Fatalf("tools/list did not use namespace override in descriptions: %s", list)
	}
	if toolIsError(t, byID["2"]) {
		t.Fatalf("api.list should remain callable in no-agent mode: %s", byID["2"])
	}
	if !strings.Contains(byID["3"], "wsflow agentless mode disables agent-backed tool: agents.call") {
		t.Fatalf("hidden tool did not return clear no-agent error: %s", byID["3"])
	}
	if !strings.Contains(toolText(t, byID["4"]), `"source":"setup"`) {
		t.Fatalf("setup alias did not dispatch to setup state: %s", byID["4"])
	}

	var badRootOut bytes.Buffer
	badRootServer := NewServer(filepath.Join(t.TempDir(), "missing"), "test")
	if err := badRootServer.ServeStdio(context.Background(), strings.NewReader(
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"tickets.list","arguments":{}}}`+"\n",
	), &badRootOut); err != nil {
		t.Fatalf("bad root ServeStdio returned error: %v", err)
	}
	badRootByID := responseLinesByID(t, strings.Split(strings.TrimSpace(badRootOut.String()), "\n"))
	if !strings.Contains(toolText(t, badRootByID["5"]), "call setup with root") {
		t.Fatalf("root guidance did not use setup alias: %s", badRootByID["5"])
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

func TestWsflowModeAdvertisesAndServesPromptRender(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_MCP_NO_AGENT", "1")
	t.Setenv("WS_MCP_NAMESPACE", "wsflow")
	t.Setenv("WS_MCP_SETUP_TOOL", "setup")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"prompt.render","arguments":{"root":%q,"stem":"code-reviewer"}}}`, root),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"prompt.render","arguments":{"root":%q,"stem":"code-reviewer","context":{"reviewer_scope":"correctness only"}}}}`, root),
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))

	// tools/list must include prompt.render in wsflow mode.
	list := byID["1"]
	if !strings.Contains(list, "prompt.render") {
		t.Fatalf("tools/list missing prompt.render in wsflow mode: %s", list)
	}

	// Render without context: file must exist, contain wsflow/, not contain ws/.
	if toolIsError(t, byID["2"]) {
		t.Fatalf("prompt.render returned error in wsflow mode: %s", byID["2"])
	}
	promptPath := strings.TrimSpace(toolText(t, byID["2"]))
	if _, err := os.Stat(promptPath); err != nil {
		t.Fatalf("rendered prompt file does not exist at %q: %v", promptPath, err)
	}
	rendered, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read rendered prompt: %v", err)
	}
	renderedText := string(rendered)
	if strings.Contains(renderedText, "ws/") {
		t.Fatalf("rendered prompt still contains 'ws/' after substitution:\n%s", renderedText)
	}
	if !strings.Contains(renderedText, "wsflow/") {
		t.Fatalf("rendered prompt missing 'wsflow/' after substitution:\n%s", renderedText)
	}

	// Render with context: file must contain ## Render Context with injected key/value.
	if toolIsError(t, byID["3"]) {
		t.Fatalf("prompt.render with context returned error: %s", byID["3"])
	}
	contextPath := strings.TrimSpace(toolText(t, byID["3"]))
	contextRendered, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("read context-rendered prompt: %v", err)
	}
	contextText := string(contextRendered)
	if !strings.Contains(contextText, "## Render Context") {
		t.Fatalf("rendered prompt missing ## Render Context block:\n%s", contextText)
	}
	if !strings.Contains(contextText, "- reviewer_scope: correctness only") {
		t.Fatalf("rendered prompt missing injected context key/value:\n%s", contextText)
	}
}

func TestServeStdioSetupRootAndExplicitOverride(t *testing.T) {
	useLeadProfile(t)
	rootA := initTicketRepo(t, "260505-feat-alpha")
	rootB := initTicketRepo(t, "260505-feat-beta")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(t.TempDir(), "test")
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(
		fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ws.setup","arguments":{"root":%q,"format":"json"}}}`+"\n", rootA),
	), &out); err != nil {
		t.Fatalf("set default ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	var setResponse struct {
		SessionDefaultRoot string `json:"session_default_root"`
		Root               string `json:"root"`
		HasRoot            bool   `json:"has_root"`
	}
	if err := json.Unmarshal([]byte(toolText(t, byID["1"])), &setResponse); err != nil {
		t.Fatalf("ws.setup response is not JSON: %v\n%s", err, byID["1"])
	}
	if setResponse.Root != canonicalTestPath(t, rootA) || !setResponse.HasRoot || setResponse.SessionDefaultRoot != canonicalTestPath(t, rootA) {
		t.Fatalf("ws.setup response mismatch: %s", byID["1"])
	}

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"tickets.list","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ws.setup","arguments":{"format":"json"}}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"tickets.list","arguments":{"root":%q}}}`, rootB),
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"agents.register","arguments":{"name":"session-agent","model":"light"}}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"agents.register","arguments":{"root":%q,"name":"explicit-agent","model":"light"}}}`, rootB),
	}, "\n")

	out.Reset()
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !strings.Contains(toolText(t, byID["2"]), "260505-feat-alpha") || strings.Contains(toolText(t, byID["2"]), "260505-feat-beta") {
		t.Fatalf("session default root was not used for root-omitted call: %s", byID["2"])
	}
	var getResponse struct {
		SessionDefaultRoot string `json:"session_default_root"`
		HasSessionDefault  bool   `json:"has_session_default"`
		Root               string `json:"root"`
		HasRoot            bool   `json:"has_root"`
	}
	if err := json.Unmarshal([]byte(toolText(t, byID["3"])), &getResponse); err != nil {
		t.Fatalf("ws.setup response is not JSON: %v\n%s", err, byID["3"])
	}
	if !getResponse.HasRoot || getResponse.Root != canonicalTestPath(t, rootA) || !getResponse.HasSessionDefault || getResponse.SessionDefaultRoot != canonicalTestPath(t, rootA) {
		t.Fatalf("ws.setup response mismatch: %s", byID["3"])
	}
	if !strings.Contains(toolText(t, byID["4"]), "260505-feat-beta") || strings.Contains(toolText(t, byID["4"]), "260505-feat-alpha") {
		t.Fatalf("explicit root did not override session default: %s", byID["4"])
	}
	setupGateText := toolText(t, byID["5"])
	if !toolIsError(t, byID["5"]) || !strings.Contains(setupGateText, "setup required") || !strings.Contains(setupGateText, `ws.setup(id: "<actor-id>")`) {
		t.Fatalf("root-omitted agents.register without actor was not setup-gated: %s", byID["5"])
	}
	if strings.Contains(setupGateText, "lead-workflow-manual") || strings.Contains(setupGateText, "lead-workflow-bootstrap") || strings.Contains(setupGateText, `root: "<absolute-working-directory>"`) {
		t.Fatalf("root-omitted agents.register setup gate was too verbose: %s", byID["5"])
	}
	if toolIsError(t, byID["6"]) {
		t.Fatalf("explicit-root agents.register compatibility failed: %s", byID["6"])
	}
	if _, err := wsagent.NewManager(wsagent.Options{}).Status(rootB, "explicit-agent"); err != nil {
		t.Fatalf("explicit-root agents.register did not use explicit root: %v", err)
	}
}

func TestServeStdioActorSetupBootstrapAndRecovery(t *testing.T) {
	useLeadProfile(t)
	rootA := initTicketRepo(t, "260524-feat-actor-alpha")
	rootB := initTicketRepo(t, "260524-feat-actor-beta")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(rootB, "test")
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ws.setup","arguments":{"format":"json"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agents.register","arguments":{"name":"before-setup","model":"light"}}}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	var stateBefore struct {
		ActorID  string `json:"actor_id"`
		HasActor bool   `json:"has_actor"`
	}
	if err := json.Unmarshal([]byte(toolText(t, byID["1"])), &stateBefore); err != nil {
		t.Fatalf("setup state before bootstrap is not JSON: %v\n%s", err, byID["1"])
	}
	if stateBefore.HasActor || stateBefore.ActorID != "" {
		t.Fatalf("id-less setup minted actor authority: %s", byID["1"])
	}
	if !toolIsError(t, byID["2"]) || !strings.Contains(toolText(t, byID["2"]), "setup required") {
		t.Fatalf("root-omitted agent call before actor was not gated: %s", byID["2"])
	}
	out.Reset()
	if err := server.ServeStdio(context.Background(), strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ws.setup","arguments":{"method":"lead-workflow-bootstrap","root":%q,"format":"json"}}}`+"\n", rootA)), &out); err != nil {
		t.Fatalf("ServeStdio bootstrap returned error: %v", err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	var setup struct {
		Root             string `json:"root"`
		ActorID          string `json:"actor_id"`
		HasActor         bool   `json:"has_actor"`
		ActorAuthority   string `json:"actor_authority"`
		RecoveryGuidance string `json:"recovery_guidance"`
	}
	if err := json.Unmarshal([]byte(toolText(t, byID["3"])), &setup); err != nil {
		t.Fatalf("bootstrap setup is not JSON: %v\n%s", err, byID["3"])
	}
	if setup.Root != canonicalTestPath(t, rootA) || !setup.HasActor || setup.ActorID == "" || setup.ActorAuthority != "lead" || !strings.Contains(setup.RecoveryGuidance, setup.ActorID) {
		t.Fatalf("bootstrap setup response mismatch: %s", byID["3"])
	}
	assertCompactActorID(t, setup.ActorID, "lead")
	out.Reset()
	if err := server.ServeStdio(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"agents.register","arguments":{"name":"after-setup","model":"light"}}}`+"\n"), &out); err != nil {
		t.Fatalf("ServeStdio after setup returned error: %v", err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if toolIsError(t, byID["4"]) {
		t.Fatalf("root-omitted agents.register after actor setup failed: %s", byID["4"])
	}
	if _, err := wsagent.NewManager(wsagent.Options{}).StatusScoped(rootA, "after-setup", server.currentActorID()); err != nil {
		t.Fatalf("root-omitted agents.register did not use actor root: %v", err)
	}

	out.Reset()
	recoveryServer := NewServer(rootB, "test")
	if err := recoveryServer.ServeStdio(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"agents.register","arguments":{"name":"fresh-before-recovery","model":"light"}}}`+"\n"), &out); err != nil {
		t.Fatalf("ServeStdio pre-recovery returned error: %v", err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !toolIsError(t, byID["5"]) || !strings.Contains(toolText(t, byID["5"]), "setup required") {
		t.Fatalf("fresh server did not require actor recovery before root-omitted agent call: %s", byID["5"])
	}
	out.Reset()
	if err := recoveryServer.ServeStdio(context.Background(), strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"ws.setup","arguments":{"id":%q,"format":"json"}}}`+"\n", strings.ToUpper(setup.ActorID))), &out); err != nil {
		t.Fatalf("ServeStdio recovery returned error: %v", err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	var recovered struct {
		Root           string `json:"root"`
		ActorID        string `json:"actor_id"`
		HasActor       bool   `json:"has_actor"`
		ActorAuthority string `json:"actor_authority"`
	}
	if err := json.Unmarshal([]byte(toolText(t, byID["6"])), &recovered); err != nil {
		t.Fatalf("recovery setup is not JSON: %v\n%s", err, byID["6"])
	}
	if recovered.Root != canonicalTestPath(t, rootA) || recovered.ActorID != setup.ActorID || !recovered.HasActor || recovered.ActorAuthority != "lead" {
		t.Fatalf("recovery setup response mismatch: %s", byID["6"])
	}
	out.Reset()
	if err := recoveryServer.ServeStdio(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"agents.register","arguments":{"name":"fresh-after-recovery","model":"light"}}}`+"\n"), &out); err != nil {
		t.Fatalf("ServeStdio post-recovery returned error: %v", err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if toolIsError(t, byID["7"]) {
		t.Fatalf("root-omitted agents.register after actor recovery failed: %s", byID["7"])
	}
	if _, err := wsagent.NewManager(wsagent.Options{}).StatusScoped(rootA, "fresh-after-recovery", recovered.ActorID); err != nil {
		t.Fatalf("recovered actor did not bind root for agent register: %v", err)
	}
}

func TestServeStdioSetupActorIDCollisionRetries(t *testing.T) {
	useLeadProfile(t)
	existingRoot := initTicketRepo(t, "260525-feat-existing-actor")
	targetRoot := initTicketRepo(t, "260525-feat-actor-collision")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	existingStore, err := wsstore.NewManager(wsstore.Options{}).Open(existingRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := existingStore.UpsertActor(context.Background(), wsstore.Actor{ActorID: "lead-aaaaaaaa", Authority: "lead", RootPath: canonicalTestPath(t, existingRoot), WorktreeKey: existingStore.Layout().WorktreeKey, Status: "active", Pinned: true}); err != nil {
		t.Fatal(err)
	}
	if err := existingStore.Close(); err != nil {
		t.Fatal(err)
	}
	targetStore, err := wsstore.NewManager(wsstore.Options{}).Open(targetRoot)
	if err != nil {
		t.Fatal(err)
	}
	if targetStore.Layout().WorktreeKey == "" || targetStore.Layout().WorktreeKey == existingStore.Layout().WorktreeKey {
		t.Fatalf("test setup did not create distinct worktree keys: existing=%q target=%q", existingStore.Layout().WorktreeKey, targetStore.Layout().WorktreeKey)
	}
	if _, ok, err := targetStore.Actor(context.Background(), "lead-aaaaaaaa"); err != nil || ok {
		t.Fatalf("target store should not contain the colliding actor locally: ok=%t err=%v", ok, err)
	}
	if err := targetStore.Close(); err != nil {
		t.Fatal(err)
	}
	oldGenerate := generateActorPayload
	calls := 0
	generateActorPayload = func(length int) (string, error) {
		calls++
		if calls == 1 {
			return "aaaaaaaa", nil
		}
		return "bbbbbbbb", nil
	}
	t.Cleanup(func() { generateActorPayload = oldGenerate })

	input := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ws.setup","arguments":{"method":"lead-workflow-bootstrap","root":%q,"format":"json"}}}`+"\n", targetRoot)
	var out bytes.Buffer
	if err := NewServer(targetRoot, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	var setup struct {
		ActorID string `json:"actor_id"`
	}
	if err := json.Unmarshal([]byte(toolText(t, byID["1"])), &setup); err != nil {
		t.Fatalf("setup response is not JSON: %v\n%s", err, byID["1"])
	}
	if setup.ActorID != "lead-bbbbbbbb" || calls != 2 {
		t.Fatalf("cache-wide collision retry mismatch: actor_id=%q calls=%d response=%s", setup.ActorID, calls, byID["1"])
	}
}

func TestServeStdioActorSetupRejectsCWDPlaceholder(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ws.setup","arguments":{"method":"lead-workflow-bootstrap","root":"<cwd>"}}}` + "\n"
	var out bytes.Buffer
	if err := NewServer(t.TempDir(), "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !toolIsError(t, byID["1"]) || !strings.Contains(toolText(t, byID["1"]), "absolute repository path") {
		t.Fatalf("cwd placeholder was not rejected with actionable guidance: %s", byID["1"])
	}
}

func TestServeStdioSetupFencesFollowingBatchRequest(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260524-feat-actor-batch")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	input := strings.Join([]string{
		fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ws.setup","arguments":{"method":"lead-workflow-bootstrap","root":%q,"format":"json"}}}`, root),
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agents.register","arguments":{"name":"batch-after-setup","model":"light"}}}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"id":1`) || !strings.Contains(lines[1], `"id":2`) {
		t.Fatalf("setup fence did not preserve setup-before-register response order:\n%s", out.String())
	}
	byID := responseLinesByID(t, lines)
	if toolIsError(t, byID["2"]) {
		t.Fatalf("batched agents.register after setup failed: %s", byID["2"])
	}
	var setup struct {
		ActorID string `json:"actor_id"`
	}
	if err := json.Unmarshal([]byte(toolText(t, byID["1"])), &setup); err != nil {
		t.Fatal(err)
	}
	if _, err := wsagent.NewManager(wsagent.Options{}).StatusScoped(root, "batch-after-setup", setup.ActorID); err != nil {
		t.Fatalf("batched register did not use actor-bound root: %v", err)
	}
}

func TestServeStdioChildActorPromptInjection(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260524-feat-child-actor")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	var out bytes.Buffer
	setupInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ws.setup","arguments":{"method":"lead-workflow-bootstrap","root":%q,"format":"json"}}}`+"\n", root)
	if err := server.ServeStdio(context.Background(), strings.NewReader(setupInput), &out); err != nil {
		t.Fatalf("ServeStdio setup returned error: %v", err)
	}
	out.Reset()
	registerInput := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agents.register","arguments":{"name":"child-worker","model":"light"}}}` + "\n"
	if err := server.ServeStdio(context.Background(), strings.NewReader(registerInput), &out); err != nil {
		t.Fatalf("ServeStdio register returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if toolIsError(t, byID["2"]) {
		t.Fatalf("actor-bound agents.register failed: %s", byID["2"])
	}
	agent, err := wsagent.NewManager(wsagent.Options{}).AgentScoped(root, "child-worker", server.currentActorID())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(agent.ChildActorID, "delegate-") || agent.ChildActorAuthority != "delegate" {
		t.Fatalf("child actor metadata mismatch: id=%q authority=%q", agent.ChildActorID, agent.ChildActorAuthority)
	}
	assertCompactActorID(t, agent.ChildActorID, "delegate")
	layout, _, _, err := wsstate.NewManager(wsstate.Options{}).Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(layout.AgentsDir, "*", agent.SystemPromptPath))
	if err != nil {
		t.Fatal(err)
	}
	var system string
	for _, systemPath := range matches {
		raw, err := os.ReadFile(systemPath)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), agent.ChildActorID) {
			system = string(raw)
			break
		}
	}
	if system == "" {
		t.Fatalf("system prompt for child-worker not found under %s", layout.AgentsDir)
	}
	if !strings.Contains(system, `ws.setup`) || !strings.Contains(system, agent.ChildActorID) {
		t.Fatalf("system prompt missing child actor setup instruction:\n%s", system)
	}
	if strings.Contains(system, "lead-workflow-bootstrap") {
		t.Fatalf("child system prompt exposed lead bootstrap method:\n%s", system)
	}

	out.Reset()
	recoverInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ws.setup","arguments":{"id":%q,"format":"json"}}}`+"\n", agent.ChildActorID)
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(recoverInput), &out); err != nil {
		t.Fatalf("ServeStdio child recovery returned error: %v", err)
	}
	byID = responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	var recovered struct {
		ActorID        string `json:"actor_id"`
		ActorAuthority string `json:"actor_authority"`
		Root           string `json:"root"`
	}
	if err := json.Unmarshal([]byte(toolText(t, byID["3"])), &recovered); err != nil {
		t.Fatalf("child recovery setup is not JSON: %v\n%s", err, byID["3"])
	}
	if recovered.ActorID != agent.ChildActorID || recovered.ActorAuthority != "delegate" || recovered.Root != canonicalTestPath(t, root) {
		t.Fatalf("child recovery mismatch: %s", byID["3"])
	}
}

func TestServeStdioSetupRootDoesNotPersistAcrossServers(t *testing.T) {
	useLeadProfile(t)
	rootA := initTicketRepo(t, "260505-feat-alpha")
	rootB := initTicketRepo(t, "260505-feat-beta")
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheRoot)

	firstInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ws.setup","arguments":{"root":%q}}}`+"\n", rootA)
	var firstOut bytes.Buffer
	if err := NewServer(rootB, "test").ServeStdio(context.Background(), strings.NewReader(firstInput), &firstOut); err != nil {
		t.Fatalf("first ServeStdio returned error: %v", err)
	}

	secondInput := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"tickets.list","arguments":{}}}` + "\n"
	var secondOut bytes.Buffer
	if err := NewServer(rootB, "test").ServeStdio(context.Background(), strings.NewReader(secondInput), &secondOut); err != nil {
		t.Fatalf("second ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(secondOut.String()), "\n"))
	if !strings.Contains(toolText(t, byID["2"]), "260505-feat-beta") || strings.Contains(toolText(t, byID["2"]), "260505-feat-alpha") {
		t.Fatalf("session default root leaked across server instances: %s", byID["2"])
	}
	assertStringAbsentFromTree(t, cacheRoot, canonicalTestPath(t, rootA))
	assertStringAbsentFromTree(t, rootA, canonicalTestPath(t, rootA))
	assertStringAbsentFromTree(t, rootB, canonicalTestPath(t, rootA))
}

func TestServeStdioServerRootTakesPrecedenceOverProjectEnv(t *testing.T) {
	useLeadProfile(t)
	envRoot := initTicketRepo(t, "260505-feat-env")
	serverRoot := initTicketRepo(t, "260505-feat-server")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_MCP_PROJECT_ROOT", envRoot)

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"tickets.list","arguments":{}}}` + "\n"
	var out bytes.Buffer
	if err := NewServer(serverRoot, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !strings.Contains(toolText(t, byID["1"]), "260505-feat-server") || strings.Contains(toolText(t, byID["1"]), "260505-feat-env") {
		t.Fatalf("server root did not take precedence over project env: %s", byID["1"])
	}
}

func TestServeStdioInvalidServerRootDoesNotFallBackToProjectEnv(t *testing.T) {
	useLeadProfile(t)
	envRoot := initTicketRepo(t, "260505-feat-env")
	serverRoot := filepath.Join(t.TempDir(), "missing")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_MCP_PROJECT_ROOT", envRoot)

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"tickets.list","arguments":{}}}` + "\n"
	var out bytes.Buffer
	if err := NewServer(serverRoot, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !toolIsError(t, byID["1"]) || strings.Contains(toolText(t, byID["1"]), "260505-feat-env") || !strings.Contains(toolText(t, byID["1"]), "ws.setup") {
		t.Fatalf("invalid server root fell back to project env or lacked setup guidance: %s", byID["1"])
	}
}

func TestServeStdioCodexWorkspaceMetadataRootFallback(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260505-feat-metadata")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	input := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"tickets.list","arguments":{},"_meta":{"x-codex-turn-metadata":{"workspaces":{%q:{}}}}}}`+"\n", root)
	var out bytes.Buffer
	if err := NewServer(t.TempDir(), "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !strings.Contains(toolText(t, byID["1"]), "260505-feat-metadata") {
		t.Fatalf("metadata workspace root was not used: %s", byID["1"])
	}
}

func TestServeStdioInitializeDetectsClaudeHarnessForAgentAlias(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260508-feat-claude-harness")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	initializeInput := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"Claude Code","version":"test"}}}`
	registerInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agents.register","arguments":{"root":%q,"name":"reviewer","model":"core"}}}`, root)
	checkInput := strings.Join([]string{
		fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"agents.status","arguments":{"root":%q,"name":"reviewer"}}}`, root),
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ws.setup","arguments":{}}}`,
	}, "\n")

	var out bytes.Buffer
	server := NewServer(t.TempDir(), "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(initializeInput), &out); err != nil {
		t.Fatalf("ServeStdio initialize returned error: %v", err)
	}
	out.Reset()
	if err := server.ServeStdio(context.Background(), strings.NewReader(registerInput), &out); err != nil {
		t.Fatalf("ServeStdio register returned error: %v", err)
	}
	out.Reset()
	if err := server.ServeStdio(context.Background(), strings.NewReader(checkInput), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	status := toolText(t, byID["3"])
	if !strings.Contains(status, "harness: claude") || !strings.Contains(status, "backend: claude") || !strings.Contains(status, "model: sonnet") {
		t.Fatalf("status missing claude alias resolution:\n%s", status)
	}
	session := toolText(t, byID["4"])
	if !strings.Contains(session, "session_harness: claude") {
		t.Fatalf("session did not report claude harness: %s", session)
	}
}

func TestServeStdioCodexMetadataDetectsHarnessForAgentAlias(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260508-feat-codex-harness")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	setupInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agents.register","arguments":{"root":%q,"name":"reviewer","model":"core"},"_meta":{"x-codex-turn-metadata":{"workspaces":{%q:{}}}}}}`, root, root)
	checkInput := fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agents.status","arguments":{"root":%q,"name":"reviewer"}}}`, root)
	var out bytes.Buffer
	server := NewServer(t.TempDir(), "test")
	if err := server.ServeStdio(context.Background(), strings.NewReader(setupInput), &out); err != nil {
		t.Fatalf("ServeStdio setup returned error: %v", err)
	}
	out.Reset()
	if err := server.ServeStdio(context.Background(), strings.NewReader(checkInput), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	status := toolText(t, byID["2"])
	if !strings.Contains(status, "harness: codex") || !strings.Contains(status, "backend: codex") || !strings.Contains(status, "model: gpt-5.5") {
		t.Fatalf("status missing codex alias resolution:\n%s", status)
	}
}

func TestServeStdioCodexMultiWorkspaceMetadataRefusesToGuess(t *testing.T) {
	useLeadProfile(t)
	rootA := initTicketRepo(t, "260505-feat-alpha")
	rootB := initTicketRepo(t, "260505-feat-beta")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	input := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"tickets.list","arguments":{},"_meta":{"x-codex-turn-metadata":{"workspaces":{%q:{},%q:{}}}}}}`+"\n", rootA, rootB)
	var out bytes.Buffer
	if err := NewServer(t.TempDir(), "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !toolIsError(t, byID["1"]) || !strings.Contains(toolText(t, byID["1"]), "multiple host workspaces") || !strings.Contains(toolText(t, byID["1"]), "ws.setup") {
		t.Fatalf("multi-workspace metadata did not produce actionable error: %s", byID["1"])
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

func TestServeStdioDelegateProfileRejectsSetupMutation(t *testing.T) {
	t.Setenv("WS_MCP_ALLOWED_TOOLS", "")
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_MCP_TOOL_PROFILE", "delegate")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ws.setup","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"runtime.info","arguments":{}}}`,
	}, "\n")

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if strings.Contains(byID["1"], "ws.setup") {
		t.Fatalf("delegate tools/list exposed setup mutation tool: %s", byID["1"])
	}
	if !strings.Contains(byID["2"], "tool not available") {
		t.Fatalf("delegate tools/call did not reject ws.setup: %s", byID["2"])
	}
	if !strings.Contains(byID["3"], "prompt_bundle") {
		t.Fatalf("delegate tools/call rejected runtime.info: %s", byID["3"])
	}
}

func TestExplicitAllowedToolsCannotBypassEffectiveRole(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_MCP_TOOL_PROFILE", "leaf")
	t.Setenv("WS_MCP_ALLOWED_TOOLS", "agents.status,runtime.info,ws.setup")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agents.status","arguments":{"name":"impl"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"runtime.info","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ws.setup","arguments":{}}}`,
	}, "\n")

	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if strings.Contains(byID["1"], "agents.status") {
		t.Fatalf("explicit allowlist bypassed leaf role in tools/list: %s", byID["1"])
	}
	if strings.Contains(byID["1"], "ws.setup") {
		t.Fatalf("explicit allowlist exposed setup mutation tool: %s", byID["1"])
	}
	if !strings.Contains(byID["1"], "runtime.info") {
		t.Fatalf("explicit allowlist hid runtime.info: %s", byID["1"])
	}
	if !strings.Contains(byID["2"], "tool not available") {
		t.Fatalf("explicit allowlist bypassed leaf role in tools/call: %s", byID["2"])
	}
	if !strings.Contains(byID["3"], "prompt_bundle") {
		t.Fatalf("explicit allowlist rejected allowed runtime.info: %s", byID["3"])
	}
	if !strings.Contains(byID["4"], "tool not available") {
		t.Fatalf("explicit allowlist bypassed leaf role for ws.setup: %s", byID["4"])
	}
}

func TestServeStdioDoesNotBlockToolsListBehindWait(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
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
	go func() {
		done <- NewServer(root, "test").ServeStdio(context.Background(), reader, outWriter)
		_ = outWriter.Close()
	}()

	fmt.Fprintln(writer, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agents.wait","arguments":{"name":"impl","timeout_seconds":2}}}`)
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
		t.Fatal("tools/list was blocked behind agents.wait")
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
	manager := wsagent.NewManager(wsagent.Options{})
	agent, layout, err := manager.Register(wsagent.RegisterOptions{Root: root, Name: "subquery-tmp-test", Ephemeral: true})
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

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"agents.result","arguments":{"name":"subquery-tmp-test"}}}` + "\n"
	var out bytes.Buffer
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !strings.Contains(toolText(t, byID["1"]), "ephemeral answer") {
		t.Fatalf("agents.result response mismatch: %s", byID["1"])
	}
	if _, err := os.Stat(layout.AgentDir); err != nil {
		t.Fatalf("ephemeral agent dir should remain after MCP result for retention cleanup: %v", err)
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

func TestServeStdioGitToolCalls(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
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
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
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
	if err := server.ServeStdio(context.Background(), strings.NewReader(commitInput), &out); err != nil {
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
	if err := server.ServeStdio(context.Background(), strings.NewReader(jsonCommitInput), &out); err != nil {
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
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	text := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["1"])
	if !strings.Contains(text, "exactly one") || !strings.Contains(out.String(), `"isError":true`) {
		t.Fatalf("references.trace accepted ambiguous selectors: %s", out.String())
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

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"api.list","arguments":{}}}` + "\n"
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	got := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["1"])
	if got != "go\npython\n" {
		t.Fatalf("api.list = %q", got)
	}
}

func TestAPIAskMCPExactHintSkipsRouter(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "ai-docs/.deps/go/README.md", "go")
	fake := &fakeAPIRuntime{answers: map[string]string{"go": "go answer"}}
	server := NewServer(root, "test")
	server.api = fake
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"api.ask","arguments":{"prompt":"How do modules work?","domain_hint":"go"}}}` + "\n"
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !strings.Contains(byID["1"], "api.ask") || !strings.Contains(byID["1"], "api.list") {
		t.Fatalf("tools/list missing api tools: %s", byID["1"])
	}
	if toolIsError(t, byID["2"]) {
		t.Fatalf("api.ask returned tool error: %s", byID["2"])
	}
	text := toolText(t, byID["2"])
	if len(fake.routeCalls) != 0 {
		t.Fatalf("exact hint invoked pre-router: %v", fake.routeCalls)
	}
	if !strings.Contains(text, "## Domain: go") || !strings.Contains(text, "go answer") {
		t.Fatalf("api.ask response missing boundary/answer:\n%s", text)
	}
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

func TestAPIAskMCPAllDomainFailureReturnsToolErrorWithMetadata(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	fake := &fakeAPIRuntime{errs: map[string]error{"go": fmt.Errorf("go failed"), "python": fmt.Errorf("python failed")}}
	server := NewServer(root, "test")
	server.api = fake
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"api.ask","arguments":{"prompt":"question"}}}` + "\n"
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	if !toolIsError(t, byID["1"]) {
		t.Fatalf("expected api.ask tool error: %s", byID["1"])
	}
	text := toolText(t, byID["1"])
	if !strings.Contains(text, "## Domain: go\nERROR: go failed") ||
		!strings.Contains(text, "## Domain: python\nERROR: python failed") ||
		!strings.Contains(text, "api.ask failed for all resolved domains") {
		t.Fatalf("all failure text missing metadata:\n%s", text)
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
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
	for _, name := range []string{"exec.spawn", "exec.shell", "exec.status", "exec.result", "exec.abort", "exec.raw.tail", "exec.raw.read", "exec.raw.grep"} {
		if !strings.Contains(byID["1"], name) {
			t.Fatalf("tools/list missing %s: %s", name, byID["1"])
		}
	}
	text := toolText(t, byID["2"])
	shellText := toolText(t, byID["9"])
	if !strings.Contains(shellText, "shell-shape") || !strings.Contains(shellText, execToolJSONPath(root)) {
		t.Fatalf("shell response = %s", byID["9"])
	}
	if strings.HasPrefix(strings.TrimSpace(text), "{") || !strings.Contains(text, "status: succeeded") || !strings.Contains(text, execToolJSONPath(filepath.Join(root, "sub"))) || !strings.Contains(text, "========== stdout ==========") || !strings.Contains(text, "========== stderr ==========") || !strings.Contains(text, "err") {
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
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
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
	if err := NewServer(root, "test").ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
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

	input := toolCallLine(t, 1, "exec.shell", mcpLongShellArgs()) + "\n"
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
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
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
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
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var abortText string
	for time.Now().Before(deadline) {
		out.Reset()
		input = fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"exec.status","arguments":{"exec_key":%q}}}`, running.ExecKey) + "\n"
		if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
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
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
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
	if err := server.ServeStdio(context.Background(), strings.NewReader(toolCallLine(t, 1, "exec.shell", jsonArgs)+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	text := toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["1"])
	if strings.HasPrefix(strings.TrimSpace(text), "{") || !strings.Contains(text, "========== stdout ==========") || !strings.Contains(text, `{"ok":true}`) || strings.Contains(text, `\"ok\"`) {
		t.Fatalf("json stdout response = %s", text)
	}

	out.Reset()
	if err := server.ServeStdio(context.Background(), strings.NewReader(toolCallLine(t, 2, "exec.shell", mcpLongShellArgs())+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	running := execKeyFromText(t, toolText(t, responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))["2"]))
	out.Reset()
	input := fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"exec.result","arguments":{"exec_key":%q,"timeout_seconds":3}}}`, running) + "\n"
	if err := server.ServeStdio(context.Background(), strings.NewReader(input), &out); err != nil {
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

func mcpLargeShellArgs() map[string]any {
	if runtime.GOOS == "windows" {
		return map[string]any{"shell": "powershell", "command": "[Console]::Out.Write(('x' * 5000))"}
	}
	return map[string]any{"command": "i=0; while [ $i -lt 5000 ]; do printf x; i=$((i+1)); done"}
}

func execToolJSONPath(path string) string {
	if runtime.GOOS == "windows" {
		return strings.ReplaceAll(path, `\`, `\\`)
	}
	return path
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

func TestServeStdioActorScopedAgentLifecycleAndExplicitRootCompatibility(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260524-feat-actor-lifecycle")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	call := func(id int, payload string) string {
		t.Helper()
		var out bytes.Buffer
		if err := server.ServeStdio(context.Background(), strings.NewReader(payload+"\n"), &out); err != nil {
			t.Fatalf("ServeStdio id %d returned error: %v", id, err)
		}
		byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
		line := byID[fmt.Sprint(id)]
		if toolIsError(t, line) {
			t.Fatalf("tool id %d returned error: %s", id, line)
		}
		return toolText(t, line)
	}

	call(1, fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ws.setup","arguments":{"method":"lead-workflow-bootstrap","root":%q,"format":"json"}}}`, root))
	call(2, fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"agents.register","arguments":{"root":%q,"name":"same","backend":"bogus","model":"global-model"}}}`, root))
	call(3, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"agents.register","arguments":{"name":"same","backend":"bogus","model":"actor-model"}}}`)
	manager := wsagent.NewManager(wsagent.Options{})
	actorID := server.currentActorID()
	state, _, _, err := wsstate.NewManager(wsstate.Options{}).Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := wsstore.NewManager(wsstore.Options{}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	actorSameKey, err := wsstore.AgentInternalKey(actorID, "same")
	if err != nil {
		t.Fatal(err)
	}
	oldActorSame, ok, err := store.AgentDefinition(context.Background(), actorSameKey)
	if err != nil || !ok {
		t.Fatalf("actor same definition ok=%t err=%v", ok, err)
	}
	oldActorSameDir := filepath.Join(state.AgentsDir, oldActorSame.StatePath)
	if err := os.WriteFile(filepath.Join(oldActorSameDir, "history-marker"), []byte("old actor history"), 0o644); err != nil {
		t.Fatal(err)
	}
	call(13, `{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"agents.register","arguments":{"name":"same","backend":"bogus","model":"actor-model-new"}}}`)
	newActorSame, ok, err := store.AgentDefinition(context.Background(), actorSameKey)
	if err != nil || !ok {
		t.Fatalf("new actor same definition ok=%t err=%v", ok, err)
	}
	if newActorSame.StatePath == oldActorSame.StatePath {
		t.Fatalf("actor re-register reused state path %q", newActorSame.StatePath)
	}
	if _, err := os.Stat(filepath.Join(oldActorSameDir, "history-marker")); err != nil {
		t.Fatalf("old actor same history should remain: %v", err)
	}
	globalSameKey, err := wsstore.AgentInternalKey("", "same")
	if err != nil {
		t.Fatal(err)
	}
	globalSame, ok, err := store.AgentDefinition(context.Background(), globalSameKey)
	if err != nil || !ok {
		t.Fatalf("global same definition ok=%t err=%v", ok, err)
	}
	if err := os.WriteFile(filepath.Join(state.AgentsDir, globalSame.StatePath, "output.md"), []byte("global same output\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	newActorSameDir := filepath.Join(state.AgentsDir, newActorSame.StatePath)
	if err := os.WriteFile(filepath.Join(newActorSameDir, "output.md"), []byte("actor same output\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sameCompleted := wsagent.CurrentCall{
		SchemaVersion: 1,
		AgentName:     "same",
		CallSeq:       1,
		ExecutionID:   "same-completed",
		Status:        wsagent.CallStatusCompleted,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		FinishedAt:    time.Now().UTC().Format(time.RFC3339),
		StdoutPath:    "current/stdout",
		StderrPath:    "current/stderr",
	}
	if err := os.WriteFile(filepath.Join(newActorSameDir, "current", "state.json"), mustMarshalForTest(t, sameCompleted), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	globalReady, globalReadyLayout, err := manager.Register(wsagent.RegisterOptions{Root: root, Name: "ready", Backend: "bogus", Model: "global-ready-model"})
	if err != nil {
		t.Fatal(err)
	}
	globalReadyCall, err := manager.BeginCurrentCall(globalReadyLayout, globalReady)
	if err != nil {
		t.Fatal(err)
	}
	globalReadyCall.Status = wsagent.CallStatusCompleted
	if err := os.WriteFile(globalReadyLayout.CurrentStateFile, mustMarshalForTest(t, globalReadyCall), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(globalReadyLayout.OutputFile, []byte("global ready output\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	actorReady, actorReadyLayout, err := manager.Register(wsagent.RegisterOptions{Root: root, ActorID: actorID, Name: "ready", Backend: "bogus", Model: "actor-ready-model"})
	if err != nil {
		t.Fatal(err)
	}
	actorReadyCall, err := manager.BeginCurrentCall(actorReadyLayout, actorReady)
	if err != nil {
		t.Fatal(err)
	}
	actorReadyCall.Status = wsagent.CallStatusCompleted
	if err := os.WriteFile(actorReadyLayout.CurrentStateFile, mustMarshalForTest(t, actorReadyCall), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actorReadyLayout.OutputFile, []byte("actor ready output\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	actorStatus := call(4, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"agents.status","arguments":{"name":"same"}}}`)
	globalStatus := call(5, fmt.Sprintf(`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"agents.status","arguments":{"root":%q,"name":"same"}}}`, root))
	actorPrint := call(14, `{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"agents.print","arguments":{"name":"same"}}}`)
	globalPrint := call(15, fmt.Sprintf(`{"jsonrpc":"2.0","id":15,"method":"tools/call","params":{"name":"agents.print","arguments":{"root":%q,"name":"same"}}}`, root))
	sameWaitText := call(16, `{"jsonrpc":"2.0","id":16,"method":"tools/call","params":{"name":"agents.wait","arguments":{"name":"same","timeout_seconds":5}}}`)
	sameResultText := call(17, `{"jsonrpc":"2.0","id":17,"method":"tools/call","params":{"name":"agents.result","arguments":{"name":"same"}}}`)
	callText := call(6, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"agents.call","arguments":{"name":"same","prompt":"do work"}}}`)
	waitText := call(7, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"agents.wait","arguments":{"name":"ready","timeout_seconds":5}}}`)
	resultText := call(8, `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"agents.result","arguments":{"name":"ready"}}}`)
	tailText := call(9, `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"agents.tail","arguments":{"name":"same","lines":20}}}`)
	cancelText := call(10, `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"agents.cancel","arguments":{"name":"same"}}}`)
	call(11, `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"agents.erase","arguments":{"name":"same"}}}`)
	globalAfterErase := call(12, fmt.Sprintf(`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"agents.status","arguments":{"root":%q,"name":"same"}}}`, root))

	if !strings.Contains(actorStatus, "model: actor-model-new") || strings.Contains(actorStatus, "global-model") {
		t.Fatalf("root-omitted status did not use actor scope:\n%s", actorStatus)
	}
	if !strings.Contains(globalStatus, "model: global-model") || strings.Contains(globalStatus, "actor-model") {
		t.Fatalf("explicit-root status did not use global compatibility scope:\n%s", globalStatus)
	}
	if !strings.Contains(actorPrint, "actor same output") || strings.Contains(actorPrint, "global same output") {
		t.Fatalf("root-omitted print did not use actor scope:\n%s", actorPrint)
	}
	if !strings.Contains(globalPrint, "global same output") || strings.Contains(globalPrint, "actor same output") {
		t.Fatalf("explicit-root print did not use global compatibility scope:\n%s", globalPrint)
	}
	if !strings.Contains(sameWaitText, "agent: same") ||
		!strings.Contains(sameWaitText, "call_status: completed") ||
		!strings.Contains(sameWaitText, "ready: true") {
		t.Fatalf("actor-scoped wait did not read re-registered current instance:\n%s", sameWaitText)
	}
	if !strings.Contains(sameResultText, "actor same output") || strings.Contains(sameResultText, "global same output") {
		t.Fatalf("actor-scoped result did not read re-registered current instance:\n%s", sameResultText)
	}
	if !strings.Contains(callText, "same\trunning") {
		t.Fatalf("actor-scoped call did not start:\n%s", callText)
	}
	if !strings.Contains(waitText, "agent: ready") ||
		!strings.Contains(waitText, "call_status: completed") ||
		!strings.Contains(waitText, "ready: true") ||
		!strings.Contains(waitText, "result_available: true") {
		t.Fatalf("actor-scoped wait mismatch:\n%s", waitText)
	}
	if !strings.Contains(resultText, "actor ready output") || strings.Contains(resultText, "global ready output") {
		t.Fatalf("actor-scoped result status mismatch:\n%s", resultText)
	}
	if !strings.Contains(tailText, "call.started_async") {
		t.Fatalf("actor-scoped tail did not read scoped diagnostics:\n%s", tailText)
	}
	if !strings.Contains(cancelText, "model: actor-model-new") {
		t.Fatalf("actor-scoped cancel/status mismatch:\n%s", cancelText)
	}
	if !strings.Contains(globalAfterErase, "model: global-model") {
		t.Fatalf("actor erase removed or shadowed explicit global agent:\n%s", globalAfterErase)
	}
}

func TestSubqueryHiddenExplicitRootUsesCompatibilityScopeWithoutChildActor(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260525-bug-subquery-root")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	call := func(id int, payload string) string {
		t.Helper()
		var out bytes.Buffer
		if err := server.ServeStdio(context.Background(), strings.NewReader(payload+"\n"), &out); err != nil {
			t.Fatalf("ServeStdio id %d returned error: %v", id, err)
		}
		byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))
		line := byID[fmt.Sprint(id)]
		if toolIsError(t, line) {
			t.Fatalf("tool id %d returned error: %s", id, line)
		}
		return toolText(t, line)
	}

	call(1, fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ws.setup","arguments":{"method":"lead-workflow-bootstrap","root":%q,"format":"json"}}}`, root))
	server.rootMu.RLock()
	boundRoot := server.sessionRoot
	server.rootMu.RUnlock()
	rootlessActorID := server.actorScopeForAgentTool(boundRoot, map[string]any{})
	if rootlessActorID == "" {
		t.Fatal("rootless subquery did not resolve to the current actor scope")
	}
	explicitRootActorID := server.actorScopeForAgentTool(boundRoot, map[string]any{"root": boundRoot})
	if explicitRootActorID != "" {
		t.Fatalf("hidden explicit-root subquery resolved to actor scope %q", explicitRootActorID)
	}
	child, err := server.childActorSetupForSubquery(context.Background(), filepath.Join(root, "does-not-need-to-exist"), explicitRootActorID)
	if err != nil {
		t.Fatal(err)
	}
	if child != (childActorSetup{}) {
		t.Fatalf("explicit-root compatibility subquery received child actor setup: %+v", child)
	}
}
