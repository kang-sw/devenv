// playbook_render_test.go checks playbook rendering and resource errors.
package mcp

import (
	"errors"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsrsrc"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

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

	path, _, err := renderPlaybook(s, os.DirFS(rsrcRoot), worktreeRoot, "delegate-pb", nil, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil, "")
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
	claudeTerm := terminologyForHarness("claude", RuntimeNamespace())
	if !strings.Contains(body, claudeTerm["ExploreAgent"]) {
		t.Errorf("file body %q: expected claude ExploreAgent %q", body, claudeTerm["ExploreAgent"])
	}
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("file body %q: expected delegation tip", body)
	}
}

// TestPlaybookPrintGoldenLeadWorkflowManualRendersExploreAgent renders the real
// lead-workflow-manual under both claude and codex harness contexts with an
// isolated (default) config and verifies the per-harness Explore agent term
// materializes with no {{. placeholder remaining. The manual names only the
// tier for Explore, so no per-harness model name is expected in the body.
func TestPlaybookPrintGoldenLeadWorkflowManualRendersExploreAgent(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")

	for _, harness := range []string{"claude", "codex"} {
		t.Run(harness, func(t *testing.T) {
			s := newTestServerWithHarness(t, harness)
			body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", nil)
			if err != nil {
				t.Fatalf("printPlaybook: %v", err)
			}
			if want := terminologyForHarness(harness, RuntimeNamespace())["ExploreAgent"]; !strings.Contains(body, want) {
				t.Errorf("body missing substituted ExploreAgent %q", want)
			}
			if strings.Contains(body, "{{.") {
				t.Errorf("body %q: unsubstituted placeholder remains", body)
			}
		})
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
	_, _, err := printPlaybook(s, os.DirFS(root), "pb", nil, wsconfig.Options{}, "", nil)
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
	_, _, err := printPlaybook(s, os.DirFS(root), "pb", nil, wsconfig.Options{}, "", nil)
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

	_, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "plain-pb",
		map[string]string{"WorktreeID": "wt", "Undeclared": "oops"},
		wsconfig.Options{}, "", nil)

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

	_, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "plain-pb", map[string]string{}, wsconfig.Options{}, "", nil)
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

	_, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "dangle-pb", nil, wsconfig.Options{}, "", nil)
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

func TestPlaybookPrintWsflowProductModeFiltersHiddenGuidance(t *testing.T) {
	t.Setenv(envNoAgent, "1")
	t.Setenv(envNamespace, "wsflow")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	t.Setenv("WS_SKILLS_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "skills"))
	s := newTestServerWithHarness(t, "codex")
	configOpts := isolatedPlaybookConfigOptions(t)

	assertCleanWsflowManual := func(label, body string) {
		t.Helper()
		for _, forbidden := range []string{fullOnlyStart, fullOnlyEnd, wsflowOnlyStart, wsflowOnlyEnd, "exec.", "Full ws", "full ws", "ws:override:", "ws:/override:"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s: wsflow playbook output contains forbidden %q:\n%s", label, forbidden, body)
			}
		}
		// Exclude HTML comment lines: inert Markdown marker tokens are not
		// namespace notation, so filtering them avoids false positives.
		bodyLinesWithoutComments := strings.Join(func() []string {
			var out []string
			for _, line := range strings.Split(body, "\n") {
				if !strings.HasPrefix(strings.TrimSpace(line), "<!--") {
					out = append(out, line)
				}
			}
			return out
		}(), "\n")
		if regexp.MustCompile(`\bws[/:]`).MatchString(bodyLinesWithoutComments) {
			t.Fatalf("%s: wsflow playbook output contains bare ws namespace notation:\n%s", label, body)
		}
		if strings.Contains(body, "{{.") {
			t.Fatalf("%s: wsflow playbook output contains unsubstituted placeholder:\n%s", label, body)
		}
		for _, want := range []string{"wsflow/", "wsflow:"} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s: wsflow playbook output missing %q:\n%s", label, want, body)
			}
		}
	}

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "lead-workflow-manual", nil, configOpts, "", mustOverrideLookup(t, s, ""))
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	assertCleanWsflowManual("prefer-subagent off", body)
	if strings.Contains(body, "Use `wsflow:lead-delegate` for bounded investigation") {
		t.Fatalf("wsflow workflow manual must not append lead-prefer-subagent while preference is off:\n%s", body)
	}

	resolver := wsconfig.NewResolver(configOpts, builtinConfigDefaults(), nil, nil)
	if err := resolver.Set(wsconfig.ItemWorkflowPreferSubagent, "on", wsconfig.SetOptions{}); err != nil {
		t.Fatalf("enable workflow.prefer_subagent: %v", err)
	}
	bodyOn, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "lead-workflow-manual", nil, configOpts, "", mustOverrideLookup(t, s, ""))
	if err != nil {
		t.Fatalf("printPlaybook on: %v", err)
	}
	assertCleanWsflowManual("prefer-subagent on", bodyOn)
	for _, want := range []string{
		"Use `wsflow:lead-delegate` for bounded investigation",
		"wsflow:lead-delegate",
	} {
		if !strings.Contains(bodyOn, want) {
			t.Fatalf("wsflow workflow manual with prefer-subagent on missing %q:\n%s", want, bodyOn)
		}
	}
}

func TestPlaybookPrintLeadTuneUsesWorkflowPreferenceCatalogKnobs(t *testing.T) {
	t.Setenv(envNoAgent, "")
	t.Setenv(envNamespace, "")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "codex")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "lead-tune", nil, isolatedPlaybookConfigOptions(t), "", mustOverrideLookup(t, s, ""))
	if err != nil {
		t.Fatalf("printPlaybook lead-tune: %v", err)
	}
	if strings.TrimSpace(body) == "" {
		t.Fatal("lead-tune rendered empty")
	}
}

func TestPlaybookPrintWsflowLeadTuneOmitsFullWsOnlyCatalogKnobs(t *testing.T) {
	t.Setenv(envNoAgent, "1")
	t.Setenv(envNamespace, "wsflow")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "codex")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "lead-tune", nil, isolatedPlaybookConfigOptions(t), "", mustOverrideLookup(t, s, ""))
	if err != nil {
		t.Fatalf("printPlaybook lead-tune wsflow: %v", err)
	}
	if !strings.Contains(body, "wsflow/config.list") {
		t.Fatal("lead-tune render missing substituted namespace")
	}
	for _, forbidden := range []string{
		"Full ws",
		"full ws",
		"ws:override:",
		"ws:/override:",
		"{{.",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("wsflow lead-tune render contains forbidden %q:\n%s", forbidden, body)
		}
	}
	if regexp.MustCompile(`\bws[/:]`).MatchString(body) {
		t.Fatalf("wsflow lead-tune render contains bare ws namespace notation:\n%s", body)
	}
}

func TestProductModeBlockSelection(t *testing.T) {
	t.Setenv(envNamespace, "wsflow")
	input := strings.Join([]string{
		"shared text",
		fullOnlyStart,
		"full-only text",
		fullOnlyEnd,
		wsflowOnlyStart,
		"wsflow-only text",
		wsflowOnlyEnd,
	}, "\n")

	t.Setenv(envNoAgent, "1")
	wsflow := renderProductModePlaybookBody(input, NoAgentMode())
	for _, forbidden := range []string{"full-only text", fullOnlyStart, wsflowOnlyStart} {
		if strings.Contains(wsflow, forbidden) {
			t.Fatalf("wsflow render contains forbidden %q:\n%s", forbidden, wsflow)
		}
	}
	for _, want := range []string{"shared text", "wsflow-only text"} {
		if !strings.Contains(wsflow, want) {
			t.Fatalf("wsflow render missing %q:\n%s", want, wsflow)
		}
	}

	t.Setenv(envNoAgent, "")
	full := renderProductModePlaybookBody(input, NoAgentMode())
	if strings.Contains(full, "wsflow-only text") || strings.Contains(full, fullOnlyStart) || strings.Contains(full, wsflowOnlyStart) {
		t.Fatalf("full render kept wsflow-only text or marker comments:\n%s", full)
	}
	if !strings.Contains(full, "full-only text") {
		t.Fatalf("full render omitted full-only content:\n%s", full)
	}
}

func TestRenderPlaybookWsflowProductModeUsesShippedDelegate(t *testing.T) {
	t.Setenv(envNoAgent, "1")
	t.Setenv(envNamespace, "wsflow")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	s := newTestServerWithHarness(t, "codex")

	path, tier, err := renderPlaybook(s, os.DirFS(rsrcRoot), worktreeRoot, "implementer", shippedImplementerContext(), wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil, "")
	if err != nil {
		t.Fatalf("renderPlaybook: %v", err)
	}
	if tier != "medium" {
		t.Fatalf("implementer recommended tier = %q, want medium", tier)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered playbook: %v", err)
	}
	body := string(data)
	if strings.Contains(body, "Continuity tip") {
		t.Fatalf("rendered implementer output must not include delegation continuity tip:\n%s", body)
	}
	for _, forbidden := range []string{fullOnlyStart, fullOnlyEnd, wsflowOnlyStart, wsflowOnlyEnd, "exec.", "showsflow", "knowsflow", "followsflow", "workflowsflow"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("rendered wsflow delegate contains forbidden %q:\n%s", forbidden, body)
		}
	}
	if regexp.MustCompile(`\bws[/:]`).MatchString(body) {
		t.Fatalf("rendered wsflow delegate contains bare ws namespace notation:\n%s", body)
	}
}

func TestRenderPlaybookShippedImplementerDeclaredContext(t *testing.T) {
	t.Setenv(envNoAgent, "")
	t.Setenv(envNamespace, "")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	s := newTestServerWithHarness(t, "codex")

	path, tier, err := renderPlaybook(s, os.DirFS(rsrcRoot), worktreeRoot, "implementer", shippedImplementerContext(), wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil, "")
	if err != nil {
		t.Fatalf("renderPlaybook: %v", err)
	}
	if tier != "medium" {
		t.Fatalf("implementer recommended tier = %q, want medium", tier)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered playbook: %v", err)
	}
	body := string(data)
	for _, value := range shippedImplementerContext() {
		if !strings.Contains(body, value) {
			t.Fatalf("implementer render missing substituted value %q", value)
		}
	}
	if strings.Contains(body, "{{.") {
		t.Fatal("implementer render contains unresolved variables")
	}
}

func TestRenderPlaybookWsflowLegacyPromptStemsAppendContext(t *testing.T) {
	t.Setenv(envNoAgent, "1")
	t.Setenv(envNamespace, "wsflow")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	s := newTestServerWithHarness(t, "codex")

	codeReviewerPath, _, err := renderPlaybook(s, os.DirFS(rsrcRoot), worktreeRoot, "code-reviewer", map[string]string{
		"note": "see ws/tickets.query for details",
	}, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil, "")

	if err != nil {
		t.Fatalf("renderPlaybook code-reviewer with legacy context: %v", err)
	}
	codeReviewerData, err := os.ReadFile(codeReviewerPath)
	if err != nil {
		t.Fatalf("read code-reviewer render: %v", err)
	}
	codeReviewerBody := string(codeReviewerData)
	for _, want := range []string{"wsflow/", "## Render Context", "- note: see ws/tickets.query for details"} {
		if !strings.Contains(codeReviewerBody, want) {
			t.Fatalf("code-reviewer render missing %q:\n%s", want, codeReviewerBody)
		}
	}
}

func TestRenderPlaybookFullWsStillRejectsUndeclaredContext(t *testing.T) {
	t.Setenv(envNoAgent, "")
	t.Setenv(envNamespace, "")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	s := newTestServerWithHarness(t, "codex")

	if _, _, err := renderPlaybook(s, os.DirFS(rsrcRoot), worktreeRoot, "code-reviewer", map[string]string{
		"note": "ordinary full ws context remains template vars",
	}, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil, ""); err == nil {
		t.Fatal("full ws renderPlaybook accepted undeclared context for code-reviewer")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) {
			t.Fatalf("full ws renderPlaybook error = %T %v, want ErrUndeclaredVar", err, err)
		}
	}

	ctx := shippedImplementerContext()
	ctx["Undeclared"] = "must fail"
	if _, _, err := renderPlaybook(s, os.DirFS(rsrcRoot), worktreeRoot, "implementer", ctx, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil, ""); err == nil {
		t.Fatal("full ws renderPlaybook accepted undeclared context for implementer")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) {
			t.Fatalf("full ws implementer renderPlaybook error = %T %v, want ErrUndeclaredVar", err, err)
		}
	}

	implCtx := shippedImplementerContext()
	implCtx["BriefPath"] = "ai-docs/.plans/legacy-brief.md"
	if _, _, err := renderPlaybook(s, os.DirFS(rsrcRoot), worktreeRoot, "implementer", implCtx, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil, ""); err == nil {
		t.Fatal("full ws renderPlaybook accepted BriefPath for implementer")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) || undeclared.Name != "BriefPath" {
			t.Fatalf("full ws implementer BriefPath error = %T %v, want ErrUndeclaredVar BriefPath", err, err)
		}
	}
}

func TestRenderPlaybookWsflowNonLegacyStemRejectsUndeclaredContext(t *testing.T) {
	t.Setenv(envNoAgent, "1")
	t.Setenv(envNamespace, "wsflow")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	worktreeRoot := initGitRepo(t)
	cacheHome := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cacheHome)
	s := newTestServerWithHarness(t, "codex")

	if _, _, err := renderPlaybook(s, os.DirFS(rsrcRoot), worktreeRoot, "implementer", map[string]string{
		"note": "wsflow non-legacy stems still require declared template vars",
	}, wsconfig.Options{CacheHome: cacheHome}, "", "", "", nil, ""); err == nil {
		t.Fatal("wsflow non-legacy renderPlaybook accepted undeclared context")
	} else {
		var undeclared wsrsrc.ErrUndeclaredVar
		if !errors.As(err, &undeclared) {
			t.Fatalf("wsflow non-legacy renderPlaybook error = %T %v, want ErrUndeclaredVar", err, err)
		}
	}
}
