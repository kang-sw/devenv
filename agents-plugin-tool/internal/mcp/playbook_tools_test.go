package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// buildTestRsrcTree creates a minimal rsrc tree for testing playbook tools.
// playbooks maps relative paths to file content.
// Returns the root path with a freshly generated manifest.json.
func buildTestRsrcTree(t *testing.T, playbooks map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for relPath, content := range playbooks {
		full := filepath.Join(root, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", relPath, err)
		}
	}
	m, err := wsrsrc.GenerateManifest(root)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if err := wsrsrc.WriteManifest(root, m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	return root
}

// newTestServerWithHarness creates a Server bound to a temp root with the given harness.
func newTestServerWithHarness(t *testing.T, harness string) *Server {
	t.Helper()
	s := NewServer(t.TempDir(), "test")
	if harness != "" {
		s.observeHarness("test", harness)
	}
	return s
}

// initGitRepo creates a git repository in a temp dir and returns its path.
// Required for renderPlaybook tests since GeneratePaths calls gitIdentity.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmds := [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	}
	for _, c := range cmds {
		out, err := exec.Command(c[0], c[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", c[1:], err, out)
		}
	}
	return dir
}

// asPlaybookError reports whether err or any error in its chain matches type T.
func asPlaybookError[T error](err error, target *T) bool {
	if err == nil {
		return false
	}
	return errors.As(err, target)
}

// writeTestFile writes content to root/relPath, creating parent dirs.
func writeTestFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

// ---------------------------------------------------------------------------
// Fixture playbook content strings
// ---------------------------------------------------------------------------

const (
	// plainPlaybookContent: non-delegate, one custom variable.
	plainPlaybookContent = `---
kind: print
delegates: false
variables:
  - WorktreeID
---
# Plain Playbook

Worktree: {{.WorktreeID}}
`

	// delegatePlaybookContent: delegates:true with all terminology vars.
	// NOTE: kind:render is advisory metadata only — the loader does not restrict
	// by kind, so this fixture is valid for use with printPlaybook too. kind is
	// not a tool-routing gate; it is surfaced in PlaybookMeta for caller inspection.
	delegatePlaybookContent = `---
kind: render
delegates: true
variables:
  - ExploreAgent
  - SpawnIdiom
  - ContinueIdiom
---
# Delegate Playbook

Explore: {{.ExploreAgent}}
Spawn: {{.SpawnIdiom}}
Continue: {{.ContinueIdiom}}
`

	// modelAliasPlaybookContent: declares one model alias variable.
	modelAliasPlaybookContent = `---
kind: print
delegates: false
variables:
  - CoreModel
---
# Model Alias Playbook

Model: {{.CoreModel}}
`

	// noVarsPlaybookContent: no variables, static content.
	noVarsPlaybookContent = `---
kind: print
delegates: false
---
# No-Vars Playbook

Static content only.
`
)

// ---------------------------------------------------------------------------
// playbook.print — golden harness rendering
// ---------------------------------------------------------------------------

func TestPlaybookPrintUnknownHarness(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "") // no harness → host-neutral

	body, err := printPlaybook(s, rsrcRoot, "delegate-pb", nil, wsconfig.Options{})
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	neutral := terminologyForHarness("")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, neutral[varName]) {
			t.Errorf("body %q: expected neutral %s %q", body, varName, neutral[varName])
		}
	}
	// Placeholders must be substituted.
	if strings.Contains(body, "{{.") {
		t.Errorf("body %q: unsubstituted placeholder remains", body)
	}
}

func TestPlaybookPrintClaudeHarness(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")

	body, err := printPlaybook(s, rsrcRoot, "delegate-pb", nil, wsconfig.Options{})
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	claudeTerm := terminologyForHarness("claude")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, claudeTerm[varName]) {
			t.Errorf("body %q: expected claude %s %q", body, varName, claudeTerm[varName])
		}
	}
	// Codex terms must NOT appear for any var (proves harness selection on all vars).
	codexTerm := terminologyForHarness("codex")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if claudeTerm[varName] != codexTerm[varName] {
			if strings.Contains(body, codexTerm[varName]) {
				t.Errorf("body %q: codex %s term %q must not appear in claude render", body, varName, codexTerm[varName])
			}
		}
	}
}

func TestPlaybookPrintCodexHarness(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "codex")

	body, err := printPlaybook(s, rsrcRoot, "delegate-pb", nil, wsconfig.Options{})
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	codexTerm := terminologyForHarness("codex")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, codexTerm[varName]) {
			t.Errorf("body %q: expected codex %s %q", body, varName, codexTerm[varName])
		}
	}
}

// ---------------------------------------------------------------------------
// playbook.print — delegation tip injection
// ---------------------------------------------------------------------------

func TestPlaybookPrintDelegatesTipPresent(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")

	body, err := printPlaybook(s, rsrcRoot, "delegate-pb", nil, wsconfig.Options{})
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: expected delegation tip for delegates:true playbook", body)
	}
	// Tip must include the claude ContinueIdiom.
	claudeTerm := terminologyForHarness("claude")
	if !strings.Contains(body, claudeTerm["ContinueIdiom"]) {
		t.Errorf("body %q: expected tip to include claude ContinueIdiom %q", body, claudeTerm["ContinueIdiom"])
	}
}

func TestPlaybookPrintDelegatesTipAbsent(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"plain-pb/plain-pb.md": plainPlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")

	body, err := printPlaybook(s, rsrcRoot, "plain-pb", map[string]string{"WorktreeID": "wt-123"}, wsconfig.Options{})
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// ---------------------------------------------------------------------------
// playbook.print — caller context substitution
// ---------------------------------------------------------------------------

func TestPlaybookPrintCallerContextSubstituted(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"plain-pb/plain-pb.md": plainPlaybookContent,
	})
	s := newTestServerWithHarness(t, "")

	body, err := printPlaybook(s, rsrcRoot, "plain-pb", map[string]string{"WorktreeID": "wt-abc"}, wsconfig.Options{})
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "wt-abc") {
		t.Errorf("body %q: expected caller context value 'wt-abc' substituted", body)
	}
	if strings.Contains(body, "{{.WorktreeID}}") {
		t.Errorf("body %q: placeholder should have been substituted", body)
	}
}

// ---------------------------------------------------------------------------
// playbook.print — no-vars fast path
// ---------------------------------------------------------------------------

func TestPlaybookPrintNoVarsPlaybook(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"novars/novars.md": noVarsPlaybookContent,
	})
	s := newTestServerWithHarness(t, "")

	body, err := printPlaybook(s, rsrcRoot, "novars", nil, wsconfig.Options{})
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "Static content only") {
		t.Errorf("body %q: expected static content", body)
	}
}

// ---------------------------------------------------------------------------
// playbook.render — writes tmp file, returns path
// ---------------------------------------------------------------------------

func TestPlaybookRenderWritesTmpFile(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"delegate-pb/delegate-pb.md": delegatePlaybookContent,
	})
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)

	s := newTestServerWithHarness(t, "claude")

	path, err := renderPlaybook(s, rsrcRoot, worktreeRoot, "delegate-pb", nil, wsconfig.Options{CacheHome: cacheHome})
	if err != nil {
		t.Fatalf("renderPlaybook: %v", err)
	}
	if path == "" {
		t.Fatal("renderPlaybook returned empty path")
	}

	// File must exist and contain the rendered content.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered file: %v", err)
	}
	body := string(data)
	claudeTerm := terminologyForHarness("claude")
	if !strings.Contains(body, claudeTerm["ExploreAgent"]) {
		t.Errorf("file body %q: expected claude ExploreAgent %q", body, claudeTerm["ExploreAgent"])
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("file body %q: expected delegation tip", body)
	}
}

// ---------------------------------------------------------------------------
// Model alias — config-sourced resolution (no baked model names)
// ---------------------------------------------------------------------------

func TestPlaybookPrintModelAliasFromConfig(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"model-pb/model-pb.md": modelAliasPlaybookContent,
	})
	s := newTestServerWithHarness(t, "")

	// Write a config with a unique, recognizable model name.
	cacheHome := t.TempDir()
	uniqueModel := "test-custom-model-xyz-9999"
	if _, err := wsconfig.SetAgentsTierForHarness(wsconfig.Options{CacheHome: cacheHome}, "core", "custom-backend", uniqueModel, ""); err != nil {
		t.Fatalf("SetAgentsTierForHarness: %v", err)
	}

	// Render using the custom config.
	body, err := printPlaybook(s, rsrcRoot, "model-pb", nil, wsconfig.Options{CacheHome: cacheHome})
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	// The model name must come from config, not be baked in.
	if !strings.Contains(body, uniqueModel) {
		t.Errorf("body %q: expected config-sourced model name %q", body, uniqueModel)
	}
}

// TestPlaybookPrintModelAliasVariesWithConfig verifies that changing the config
// changes the model in the output — proving config-sourced resolution.
func TestPlaybookPrintModelAliasVariesWithConfig(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"model-pb/model-pb.md": modelAliasPlaybookContent,
	})
	s := newTestServerWithHarness(t, "")

	cacheA := t.TempDir()
	modelA := "model-variant-aaa"
	if _, err := wsconfig.SetAgentsTierForHarness(wsconfig.Options{CacheHome: cacheA}, "core", "", modelA, ""); err != nil {
		t.Fatalf("config A: %v", err)
	}

	cacheB := t.TempDir()
	modelB := "model-variant-bbb"
	if _, err := wsconfig.SetAgentsTierForHarness(wsconfig.Options{CacheHome: cacheB}, "core", "", modelB, ""); err != nil {
		t.Fatalf("config B: %v", err)
	}

	bodyA, err := printPlaybook(s, rsrcRoot, "model-pb", nil, wsconfig.Options{CacheHome: cacheA})
	if err != nil {
		t.Fatalf("printPlaybook A: %v", err)
	}
	bodyB, err := printPlaybook(s, rsrcRoot, "model-pb", nil, wsconfig.Options{CacheHome: cacheB})
	if err != nil {
		t.Fatalf("printPlaybook B: %v", err)
	}

	if !strings.Contains(bodyA, modelA) {
		t.Errorf("bodyA %q: expected model %q from config A", bodyA, modelA)
	}
	if !strings.Contains(bodyB, modelB) {
		t.Errorf("bodyB %q: expected model %q from config B", bodyB, modelB)
	}
	if bodyA == bodyB {
		t.Error("different configs produced identical output — model alias resolution not config-driven")
	}
}

// ---------------------------------------------------------------------------
// Loud failure paths
// ---------------------------------------------------------------------------

func TestPlaybookPrintMissingManifest(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "pb/pb.md", "---\nkind: print\n---\nbody\n")
	// No manifest written.

	s := newTestServerWithHarness(t, "")
	_, err := printPlaybook(s, root, "pb", nil, wsconfig.Options{})
	if err == nil {
		t.Fatal("expected error for missing manifest, got nil")
	}
	var missing wsrsrc.ErrManifestMissing
	if !asPlaybookError(err, &missing) {
		t.Errorf("expected ErrManifestMissing, got %T: %v", err, err)
	}
}

func TestPlaybookPrintSchemaMismatch(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "pb/pb.md", "---\nkind: print\n---\nbody\n")
	writeTestFile(t, root, "manifest.json", `{"schema_version":999,"files":{"pb/pb.md":"deadbeef"}}`)

	s := newTestServerWithHarness(t, "")
	_, err := printPlaybook(s, root, "pb", nil, wsconfig.Options{})
	if err == nil {
		t.Fatal("expected error for schema mismatch, got nil")
	}
	var mismatch wsrsrc.ErrSchemaMismatch
	if !asPlaybookError(err, &mismatch) {
		t.Errorf("expected ErrSchemaMismatch, got %T: %v", err, err)
	}
}

func TestPlaybookPrintUndeclaredCallerVar(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"plain-pb/plain-pb.md": plainPlaybookContent,
	})
	s := newTestServerWithHarness(t, "")

	_, err := printPlaybook(s, rsrcRoot, "plain-pb",
		map[string]string{"WorktreeID": "wt", "Undeclared": "oops"},
		wsconfig.Options{})
	if err == nil {
		t.Fatal("expected ErrUndeclaredVar for undeclared caller var, got nil")
	}
	var undecl wsrsrc.ErrUndeclaredVar
	if !asPlaybookError(err, &undecl) {
		t.Errorf("expected ErrUndeclaredVar, got %T: %v", err, err)
	}
	if undecl.Name != "Undeclared" {
		t.Errorf("ErrUndeclaredVar.Name = %q, want Undeclared", undecl.Name)
	}
}

func TestPlaybookPrintUnprovidedVar(t *testing.T) {
	// WorktreeID is declared and used in body but neither caller nor tool provides it.
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"plain-pb/plain-pb.md": plainPlaybookContent,
	})
	s := newTestServerWithHarness(t, "")

	_, err := printPlaybook(s, rsrcRoot, "plain-pb", map[string]string{}, wsconfig.Options{})
	if err == nil {
		t.Fatal("expected ErrUnprovidedVar for missing required var, got nil")
	}
	var unprov wsrsrc.ErrUnprovidedVar
	if !asPlaybookError(err, &unprov) {
		t.Errorf("expected ErrUnprovidedVar, got %T: %v", err, err)
	}
	if unprov.Name != "WorktreeID" {
		t.Errorf("ErrUnprovidedVar.Name = %q, want WorktreeID", unprov.Name)
	}
}

func TestPlaybookPrintDanglingInclude(t *testing.T) {
	// Playbook declares includes: [dangling] but dangling.md is not in the tree.
	// The include resolution should fail and propagate the error through printPlaybook.
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		// Only the playbook file; dangling.md is intentionally absent so the
		// manifest will not list it, causing ErrFileMissing from resolveIncludes.
		"dangle-pb/dangle-pb.md": "---\nkind: print\ndelegates: false\nincludes:\n  - dangling\n---\nbody\n",
	})
	s := newTestServerWithHarness(t, "")

	_, err := printPlaybook(s, rsrcRoot, "dangle-pb", nil, wsconfig.Options{})
	if err == nil {
		t.Fatal("expected error for dangling include, got nil")
	}
	// The error message must contain the missing include stem name.
	if !strings.Contains(err.Error(), "dangling") {
		t.Errorf("error %q: expected include stem 'dangling' in message", err)
	}
	// The wrapped underlying error must be ErrFileMissing (the manifest does not
	// list dangling.md since it was never written to the tree).
	var fileMissing wsrsrc.ErrFileMissing
	if !asPlaybookError(err, &fileMissing) {
		t.Errorf("expected ErrFileMissing (via errors.As), got %T: %v", err, err)
	}
}

// ---------------------------------------------------------------------------
// MCP dispatch: tool surface (no wsflow gate, no no-agent gate)
// ---------------------------------------------------------------------------

func TestPlaybookToolsInLeadToolNames(t *testing.T) {
	names := LeadToolNames()
	has := func(name string) bool {
		for _, n := range names {
			if n == name {
				return true
			}
		}
		return false
	}
	if !has("playbook.print") {
		t.Error("playbook.print missing from LeadToolNames")
	}
	if !has("playbook.render") {
		t.Error("playbook.render missing from LeadToolNames")
	}
}

func TestPlaybookToolsNotWsflowOnly(t *testing.T) {
	if wsflowOnlyTool("playbook.print") {
		t.Error("playbook.print is incorrectly marked as wsflow-only")
	}
	if wsflowOnlyTool("playbook.render") {
		t.Error("playbook.render is incorrectly marked as wsflow-only")
	}
}

func TestPlaybookToolsNotNoAgentHidden(t *testing.T) {
	if noAgentHiddenTool("playbook.print") {
		t.Error("playbook.print is incorrectly hidden in no-agent mode")
	}
	if noAgentHiddenTool("playbook.render") {
		t.Error("playbook.render is incorrectly hidden in no-agent mode")
	}
}

func TestPlaybookToolsVisibleInToolsList(t *testing.T) {
	listed := map[string]bool{}
	for _, tool := range tools() {
		name, _ := tool["name"].(string)
		listed[name] = true
	}
	for _, want := range []string{"playbook.print", "playbook.render"} {
		if !listed[want] {
			t.Errorf("tool %q missing from tools() list", want)
		}
	}
}

func TestPlaybookToolsSchemaNameRequired(t *testing.T) {
	for _, tool := range tools() {
		name, _ := tool["name"].(string)
		if name != "playbook.print" && name != "playbook.render" {
			continue
		}
		schema, _ := tool["inputSchema"].(map[string]any)
		required, _ := schema["required"].([]string)
		found := false
		for _, r := range required {
			if r == "name" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tool %q schema: 'name' not in required %v", name, required)
		}
	}
}

// ---------------------------------------------------------------------------
// MCP dispatch: end-to-end via callTool
// ---------------------------------------------------------------------------

func TestPlaybookPrintMCPDispatch(t *testing.T) {
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"novars/novars.md": noVarsPlaybookContent,
	})
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)

	srv := NewServer(t.TempDir(), "test")
	// req.Params is the JSON for the tools/call params object:
	// {"name": "<tool-name>", "arguments": {...}}
	reqParams, _ := json.Marshal(map[string]any{
		"name": "playbook.print",
		"arguments": map[string]any{
			"name": "novars",
		},
	})
	req := request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  reqParams,
	}
	resp := srv.callTool(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("callTool error: %v", resp.Error.Message)
	}
	result, _ := resp.Result.(map[string]any)
	if result["isError"] == true {
		if content, ok := result["content"].([]map[string]string); ok && len(content) > 0 {
			t.Fatalf("callTool isError: %s", content[0]["text"])
		}
		t.Fatal("callTool returned isError")
	}
	content, _ := result["content"].([]map[string]string)
	if len(content) == 0 {
		t.Fatal("callTool returned no content")
	}
	if !strings.Contains(content[0]["text"], "Static content only") {
		t.Errorf("callTool result %q: expected playbook content", content[0]["text"])
	}
}

// ---------------------------------------------------------------------------
// Golden render: real agents-plugin/rsrc tree
// ---------------------------------------------------------------------------

func TestPlaybookPrintGoldenDelegateSampleClaudeHarness(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, err := printPlaybook(s, rsrcRoot, "delegate-sample", nil, wsconfig.Options{})
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	// Derived checks (broad coverage).
	claudeTerm := terminologyForHarness("claude")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, claudeTerm[varName]) {
			t.Errorf("golden body %q: expected claude %s %q", body, varName, claudeTerm[varName])
		}
	}
	// Hardcoded expected strings to guard against a wrong terminology table
	// (both sides of a derived assertion would agree even if the table were wrong).
	if !strings.Contains(body, "the Explore agent") {
		t.Errorf("golden body %q: expected hardcoded claude ExploreAgent 'the Explore agent'", body)
	}
	if !strings.Contains(body, "SendMessage(to: <agentId>)") {
		t.Errorf("golden body %q: expected hardcoded claude ContinueIdiom 'SendMessage(to: <agentId>)'", body)
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("golden body %q: expected delegation tip (delegates:true)", body)
	}
}

func TestPlaybookPrintGoldenDelegateSampleCodexHarness(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "codex")

	body, err := printPlaybook(s, rsrcRoot, "delegate-sample", nil, wsconfig.Options{})
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	// Derived checks.
	codexTerm := terminologyForHarness("codex")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, codexTerm[varName]) {
			t.Errorf("golden body %q: expected codex %s %q", body, varName, codexTerm[varName])
		}
	}
	// Hardcoded expected strings for the same anti-tautology reason.
	if !strings.Contains(body, "a search agent") {
		t.Errorf("golden body %q: expected hardcoded codex ExploreAgent 'a search agent'", body)
	}
	if !strings.Contains(body, "resuming the agent using its task id") {
		t.Errorf("golden body %q: expected hardcoded codex ContinueIdiom", body)
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("golden body %q: expected delegation tip (delegates:true)", body)
	}
}

func TestPlaybookPrintGoldenDelegateSampleUnknownHarness(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "") // host-neutral

	body, err := printPlaybook(s, rsrcRoot, "delegate-sample", nil, wsconfig.Options{})
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	neutralTerm := terminologyForHarness("")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, neutralTerm[varName]) {
			t.Errorf("golden body %q: expected neutral %s %q", body, varName, neutralTerm[varName])
		}
	}
}

func TestPlaybookPrintGoldenSamplePlaybookNoDelegation(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, err := printPlaybook(s, rsrcRoot, "sample-playbook",
		map[string]string{"WorktreeID": "wt-golden"},
		wsconfig.Options{})
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if !strings.Contains(body, "wt-golden") {
		t.Errorf("golden body %q: expected WorktreeID substituted", body)
	}
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("golden body %q: delegation tip must not appear for delegates:false", body)
	}
}

// ---------------------------------------------------------------------------
// Golden render: explore playbook (real rsrc tree)
// ---------------------------------------------------------------------------

func TestPlaybookPrintGoldenExploreClaudeHarness(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, err := printPlaybook(s, rsrcRoot, "explore", nil, wsconfig.Options{})
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	// Derived checks (broad coverage).
	claudeTerm := terminologyForHarness("claude")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, claudeTerm[varName]) {
			t.Errorf("golden body %q: expected claude %s %q", body, varName, claudeTerm[varName])
		}
	}
	// Hardcoded expected strings to guard against a wrong terminology table.
	if !strings.Contains(body, "the Explore agent") {
		t.Errorf("golden body %q: expected hardcoded claude ExploreAgent 'the Explore agent'", body)
	}
	if !strings.Contains(body, "SendMessage(to: <agentId>)") {
		t.Errorf("golden body %q: expected hardcoded claude ContinueIdiom 'SendMessage(to: <agentId>)'", body)
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("golden body %q: expected delegation tip (delegates:true)", body)
	}
}

func TestPlaybookPrintGoldenExploreCodexHarness(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "codex")

	body, err := printPlaybook(s, rsrcRoot, "explore", nil, wsconfig.Options{})
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	// Derived checks.
	codexTerm := terminologyForHarness("codex")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, codexTerm[varName]) {
			t.Errorf("golden body %q: expected codex %s %q", body, varName, codexTerm[varName])
		}
	}
	// Hardcoded expected strings for the same anti-tautology reason.
	if !strings.Contains(body, "a search agent") {
		t.Errorf("golden body %q: expected hardcoded codex ExploreAgent 'a search agent'", body)
	}
	if !strings.Contains(body, "resuming the agent using its task id") {
		t.Errorf("golden body %q: expected hardcoded codex ContinueIdiom", body)
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("golden body %q: expected delegation tip (delegates:true)", body)
	}
}

func TestPlaybookPrintGoldenExploreUnknownHarness(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "") // host-neutral

	body, err := printPlaybook(s, rsrcRoot, "explore", nil, wsconfig.Options{})
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	neutralTerm := terminologyForHarness("")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, neutralTerm[varName]) {
			t.Errorf("golden body %q: expected neutral %s %q", body, varName, neutralTerm[varName])
		}
	}
	// Hardcoded expected strings.
	if !strings.Contains(body, "an exploration agent") {
		t.Errorf("golden body %q: expected hardcoded neutral ExploreAgent 'an exploration agent'", body)
	}
	if !strings.Contains(body, "resuming the agent using its returned id") {
		t.Errorf("golden body %q: expected hardcoded neutral ContinueIdiom", body)
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("golden body %q: expected delegation tip (delegates:true)", body)
	}
}

func TestPlaybookPrintGoldenExploreJunkHarness(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "junk-harness-xyz") // unrecognized → neutral

	body, err := printPlaybook(s, rsrcRoot, "explore", nil, wsconfig.Options{})
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	// Unrecognized harness falls back to host-neutral table.
	neutralTerm := terminologyForHarness("")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, neutralTerm[varName]) {
			t.Errorf("golden body %q: expected neutral %s %q for junk harness", body, varName, neutralTerm[varName])
		}
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("golden body %q: expected delegation tip (delegates:true)", body)
	}
}

// ---------------------------------------------------------------------------
// Terminology table coverage assertions
// ---------------------------------------------------------------------------

func TestTerminologyTableCoverage(t *testing.T) {
	for _, harness := range []string{"claude", "codex", ""} {
		tbl, ok := playbookTerminologyTable[harness]
		if !ok {
			t.Errorf("terminology table missing harness entry %q", harness)
			continue
		}
		for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
			v, ok := tbl[varName]
			if !ok || v == "" {
				t.Errorf("terminology[%q][%q] = %q, want non-empty", harness, varName, v)
			}
		}
	}
}

// TestTermsDifferThreeWay asserts three-way distinctness (claude ≠ codex ≠ neutral)
// for each terminology variable, so a copy-paste collapse in the neutral table
// does not go undetected even when tautological golden tests pass.
func TestTermsDifferThreeWay(t *testing.T) {
	claude := terminologyForHarness("claude")
	codex := terminologyForHarness("codex")
	neutral := terminologyForHarness("")
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if claude[varName] == codex[varName] {
			t.Errorf("claude == codex for %q: %q — update terminology table", varName, claude[varName])
		}
		if neutral[varName] == claude[varName] {
			t.Errorf("neutral == claude for %q: %q — update terminology table", varName, neutral[varName])
		}
		if neutral[varName] == codex[varName] {
			t.Errorf("neutral == codex for %q: %q — update terminology table", varName, neutral[varName])
		}
	}
}

func TestReservedToolVarNamesContainsRequiredNames(t *testing.T) {
	for _, name := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom", "LightModel", "CoreModel", "DeepModel"} {
		if !reservedToolVarNames[name] {
			t.Errorf("reservedToolVarNames missing %q", name)
		}
	}
}
