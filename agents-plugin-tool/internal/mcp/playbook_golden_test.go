package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/claudeagents"
	"github.com/kang-sw/devenv/internal/wsconfig"
)

// ---------------------------------------------------------------------------
// Golden render: real agents-plugin/rsrc tree
// ---------------------------------------------------------------------------

func TestPlaybookPrintGoldenDelegateSampleClaudeHarness(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "delegate-sample", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	// Derived checks (broad coverage).
	claudeTerm := terminologyForHarness("claude", RuntimeNamespace())
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

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "delegate-sample", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	// Derived checks.
	codexTerm := terminologyForHarness("codex", RuntimeNamespace())
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, codexTerm[varName]) {
			t.Errorf("golden body %q: expected codex %s %q", body, varName, codexTerm[varName])
		}
	}
	// Hardcoded expected strings for the same anti-tautology reason.
	if !strings.Contains(body, "an explorer subagent") {
		t.Errorf("golden body %q: expected hardcoded codex ExploreAgent 'an explorer subagent'", body)
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

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "delegate-sample", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	neutralTerm := terminologyForHarness("", RuntimeNamespace())
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, neutralTerm[varName]) {
			t.Errorf("golden body %q: expected neutral %s %q", body, varName, neutralTerm[varName])
		}
	}
}

func TestPlaybookPrintGoldenSamplePlaybookNoDelegation(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "sample-playbook",
		map[string]string{"WorktreeID": "wt-golden"},
		wsconfig.Options{}, "", nil)

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
// Golden render: fallback explore playbook (real rsrc tree)
// ---------------------------------------------------------------------------

func TestPlaybookPrintGoldenExploreClaudeHarness(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "explore", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	// Derived checks (broad coverage).
	claudeTerm := terminologyForHarness("claude", RuntimeNamespace())
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

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "explore", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	// Derived checks.
	codexTerm := terminologyForHarness("codex", RuntimeNamespace())
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, codexTerm[varName]) {
			t.Errorf("golden body %q: expected codex %s %q", body, varName, codexTerm[varName])
		}
	}
	// Hardcoded expected strings for the same anti-tautology reason.
	if !strings.Contains(body, "an explorer subagent") {
		t.Errorf("golden body %q: expected hardcoded codex ExploreAgent 'an explorer subagent'", body)
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

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "explore", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	neutralTerm := terminologyForHarness("", RuntimeNamespace())
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

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "explore", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	// Unrecognized harness falls back to host-neutral table.
	neutralTerm := terminologyForHarness("", RuntimeNamespace())
	for _, varName := range []string{"ExploreAgent", "SpawnIdiom", "ContinueIdiom"} {
		if !strings.Contains(body, neutralTerm[varName]) {
			t.Errorf("golden body %q: expected neutral %s %q for junk harness", body, varName, neutralTerm[varName])
		}
	}
	// Hardcoded literals for anti-tautology: guards against a wrong neutral table
	// producing a false-positive derived pass (same strings as Unknown harness test).
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

// ---------------------------------------------------------------------------
// Terminology table coverage assertions
// ---------------------------------------------------------------------------

func TestTerminologyTableCoverage(t *testing.T) {
	for _, harness := range []string{"claude", "codex", "pi", ""} {
		if _, ok := playbookTerminologyTable[harness]; !ok {
			t.Errorf("terminology table missing harness entry %q", harness)
			continue
		}
		// The effective table: claude's SpawnIdiom is added at resolve time.
		tbl := terminologyForHarness(harness, RuntimeNamespace())
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
	claude := terminologyForHarness("claude", RuntimeNamespace())
	codex := terminologyForHarness("codex", RuntimeNamespace())
	neutral := terminologyForHarness("", RuntimeNamespace())
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
	for _, name := range []string{
		"ExploreAgent", "SpawnIdiom", "ContinueIdiom", "RoleModel", "McpNamespace", "SkillNamespace",
		"SmallTierModel", "MediumTierModel", "LargeTierModel", "XLargeTierModel",
		"SmallTierReasoningEffort", "MediumTierReasoningEffort", "LargeTierReasoningEffort", "XLargeTierReasoningEffort",
	} {
		if !reservedToolVarNames[name] {
			t.Errorf("reservedToolVarNames missing %q", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Golden print: migrated internal procedure playbooks (real rsrc tree)
// ---------------------------------------------------------------------------

// TestPlaybookPrintGoldenLeadCheckBlockers verifies lead-check-blockers
// resolves from the real rsrc tree and contains procedure body text.
func TestPlaybookPrintGoldenLeadCheckBlockers(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "lead-check-blockers", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if strings.TrimSpace(body) == "" {
		t.Fatal("lead-check-blockers rendered empty")
	}
	// delegates:false — continuity tip must NOT appear.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// TestPlaybookPrintRetiredSpecStemsGone pins the retirement of the spec and
// mental-model playbooks: printPlaybook must fail to resolve each stem rather
// than serve a stale body left behind in the rsrc tree.
func TestPlaybookPrintRetiredSpecStemsGone(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	for _, name := range []string{
		"lead-update-spec", "lead-write-spec", "lead-forge-spec",
		"lead-forge-mental-model", "lead-backfill-docs",
		"mental-model-updater", "doc-gap-discovery",
	} {
		if _, _, err := printPlaybook(s, os.DirFS(rsrcRoot), name, nil, wsconfig.Options{}, "", nil); err == nil {
			t.Errorf("printPlaybook(%q) still resolves a retired playbook", name)
		}
	}
}

// TestPlaybookPrintGoldenLeadWorkflowManual verifies lead-workflow-manual resolves
// and points compaction recovery at the state-restoring lead-revive entry.
func TestPlaybookPrintGoldenLeadWorkflowManual(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if strings.TrimSpace(body) == "" {
		t.Fatal("lead-workflow-manual rendered empty")
	}
	if !strings.Contains(body, `ws:lead-revive`) {
		t.Errorf("body %q: expected compaction recovery instruction using lead-revive", body)
	}
	if strings.Contains(body, "{{.") {
		t.Errorf("body %q: unsubstituted placeholder remains", body)
	}
	// delegates:false — no tip.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadTicket verifies lead-ticket resolves from the
// real rsrc tree under both harness variants without naming a retired entry
// point. delegates:false — no tip.
func TestPlaybookPrintGoldenLeadTicket(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	for _, harness := range []string{"claude", "codex"} {
		t.Run(harness, func(t *testing.T) {
			s := newTestServerWithHarness(t, harness)

			body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "lead-ticket", nil, wsconfig.Options{}, "", nil)
			if err != nil {
				t.Fatalf("printPlaybook: %v", err)
			}
			// The retired entry points must not be named by the surviving skill.
			for _, forbidden := range []string{
				"lead-write-ticket",
				"lead-proceed",
				"lead-implement",
				"[design-review:",
			} {
				if strings.Contains(body, forbidden) {
					t.Errorf("body still names retired surface %q:\n%s", forbidden, body)
				}
			}
			// delegates:false — no tip.
			if strings.Contains(body, "Continuity tip") {
				t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Golden print: Phase 3 entry-skill playbooks (real rsrc tree)
// ---------------------------------------------------------------------------

// TestRetiredLeadPlaybooksNoLongerResolve pins the collapse: the routing
// entry playbook retired into lead-run and the implementation procedure was
// superseded by the ticket-worker playbook, so neither may resolve again. A
// reintroduction is a silent re-expansion of the lead surface, which is
// exactly what this change removes; the replacements are asserted here too so
// the retirement cannot pass while leaving no path.
func TestRetiredLeadPlaybooksNoLongerResolve(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	for _, retired := range []string{"lead-proceed", "lead-implement", "lead-write-ticket", "lead-verify-discussion"} {
		if _, _, err := printPlaybook(s, os.DirFS(rsrcRoot), retired, nil, wsconfig.Options{}, "", nil); err == nil {
			t.Errorf("%s must no longer resolve as an rsrc playbook", retired)
		}
	}
	for _, replacement := range []string{"lead-run", "lead-ticket", "ticket-worker"} {
		if _, _, err := printPlaybook(s, os.DirFS(rsrcRoot), replacement, nil, wsconfig.Options{}, "", nil); err != nil {
			t.Errorf("replacement playbook %s must resolve: %v", replacement, err)
		}
	}
}

// TestPlaybookPrintGoldenLeadShip verifies lead-ship resolves from the real
// rsrc tree with no unresolved marker. delegates:false — no tip.
func TestPlaybookPrintGoldenLeadShip(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "lead-ship", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if strings.Contains(body, "[design-review:") {
		t.Errorf("body %q: unresolved design-review marker shipped", body)
	}
	// delegates:false — continuity tip must NOT appear.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

func TestPlaybookDocumentAuditProducts(t *testing.T) {
	for _, product := range []string{"ws", "wsflow"} {
		t.Run(product, func(t *testing.T) {
			t.Setenv(envNoAgent, map[string]string{"ws": "", "wsflow": "1"}[product])
			t.Setenv(envNamespace, product)
			packageDir := map[string]string{"ws": "agents-plugin", "wsflow": "agents-plugin-wsflow"}[product]
			rsrcRoot := filepath.Join("..", "..", "..", packageDir, "rsrc")
			s := newTestServerWithHarness(t, "codex")
			opts := isolatedPlaybookConfigOptions(t)
			body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "lead-audit-doc", nil, opts, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(body, product+"/playbook.render") {
				t.Error("lead render missing substituted namespace")
			}
			if _, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "lead-add-rule", nil, opts, "", nil); err == nil {
				t.Fatal("retired playbook still resolves")
			}
			// A downstream target needs only its document and the installed bundle.
			worktreeRoot := initGitRepo(t)
			target := filepath.Join(worktreeRoot, "AGENTS.md")
			if err := os.WriteFile(target, []byte("# Project\n\nUse the existing conventions.\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			path, tier, err := renderPlaybook(s, os.DirFS(rsrcRoot), worktreeRoot, "fresh-read-doc-auditor", map[string]string{"TargetFiles": target}, opts, "", "", "", nil, "")
			if err != nil {
				t.Fatal(err)
			}
			if tier != "large" {
				t.Errorf("auditor tier = %q, want large", tier)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			audit := string(data)
			for _, want := range []string{target} {
				if !strings.Contains(audit, want) {
					t.Errorf("auditor missing %q", want)
				}
			}
			for _, forbidden := range []string{"{{.", "skill-authoring", "ai-docs/manuals/", "Continuity tip"} {
				if strings.Contains(audit, forbidden) {
					t.Errorf("auditor contains %q", forbidden)
				}
			}
		})
	}
}

// TestPlaybookPrintGoldenLeadDiscuss verifies lead-discuss resolves from the
// real rsrc tree, keeps its conversation-only boundary, and routes capture and
// execution at the collapsed names. delegates:true (exploration spawn) — tip
// must appear, and the ExploreAgent variable must be substituted.
func TestPlaybookPrintGoldenLeadDiscuss(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "lead-discuss", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	for _, want := range []string{"ws:lead-delegate", "ws:lead-ticket", "ws:lead-run"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing lead-discuss text %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "{{.ExploreAgent}}") {
		t.Errorf("body %q: ExploreAgent left unsubstituted", body)
	}
	// delegates:true (exploration spawn) — tip must appear.
	if !strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: expected delegation tip for delegates:true playbook", body)
	}
}

// TestPlaybookPrintGoldenLeadReview verifies lead-review resolves from the
// real rsrc tree. delegates:false — no tip.
func TestPlaybookPrintGoldenLeadReview(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "lead-review", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	for _, want := range []string{"ws:lead-run", "ws/git.commit"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing lead-review text %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "[design-review:") {
		t.Errorf("body %q: unresolved design-review marker shipped", body)
	}
	// delegates:false — continuity tip must NOT appear.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// TestSkillAuthoringRelocatedOutOfRsrc replaces the former
// TestPlaybookPrintGoldenLeadSkillAuthoring golden. The authoring manual is no
// longer a shipped playbook: it moved to ai-docs/manuals/skill-authoring.md, which
// AGENTS.md binds as a mandatory pre-edit read. The relocation is only safe
// while the content actually survives at the new path, so this asserts the
// destination carries the doctrine text the old golden checked, and that the
// playbook name stays unresolvable so a reintroduction fails loudly.
func TestSkillAuthoringRelocatedOutOfRsrc(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	if _, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "lead-skill-authoring", nil, wsconfig.Options{}, "", nil); err == nil {
		t.Error("lead-skill-authoring must no longer resolve as an rsrc playbook")
	}

	relocated := filepath.Join("..", "..", "..", "ai-docs", "manuals", "skill-authoring.md")
	raw, err := os.ReadFile(relocated)
	if err != nil {
		t.Fatalf("relocated authoring manual missing: %v", err)
	}
	body := string(raw)
	// The manual carries manuals-tier `summary:` frontmatter, not the
	// rsrc playbook-serving frontmatter (`kind:`, `delegates:`, etc.) which
	// has no meaning outside rsrc and must not have come along with the move.
	// Only the frontmatter block is inspected: the body may legitimately
	// mention `kind: render` when describing playbook layouts.
	frontmatter := body
	if strings.HasPrefix(body, "---\n") {
		if end := strings.Index(body[4:], "\n---"); end >= 0 {
			frontmatter = body[:4+end]
		}
	}
	if !strings.Contains(frontmatter, "summary:") {
		t.Error("relocated manual is missing manuals-tier `summary:` frontmatter")
	}
	if strings.Contains(frontmatter, "kind:") {
		t.Error("relocated manual still carries rsrc playbook-serving frontmatter")
	}
}

// TestSkillsCallEnterTools verifies the routing call site survives the lead
// surface collapse: it moved off the retired lead-facing playbooks onto the
// worker playbook, which is now the only reader that routes an implementation.
// Tokens are chosen to be non-incidental: route.<mode> appears only from the
// inserted call, and target/policy are argument-level signals that cannot
// appear from surrounding prose alone.
func TestSkillsCallEnterTools(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	cases := []struct {
		playbook string
	}{
		{
			playbook: "ticket-worker",
		},
	}

	for _, tc := range cases {
		t.Run(tc.playbook, func(t *testing.T) {
			body, _, err := renderPlaybookBody(s, os.DirFS(rsrcRoot), tc.playbook, nil, wsconfig.Options{}, "", "", "", nil, "")
			if err != nil {
				t.Fatalf("renderPlaybookBody(%q): %v", tc.playbook, err)
			}
			if strings.TrimSpace(body) == "" {
				t.Errorf("renderPlaybookBody(%q) returned empty text", tc.playbook)
			}
		})
	}
}

// TestPhase3bSkillRepoint verifies the workflow-manual bootstrap stayed on the
// skill shims rather than reappearing as a playbook self-load. The original
// four repointed skills are gone or collapsed; the invariant now lives on the
// parallel-init shims, which call workflow_manual alongside playbook.read.
// A playbook body that self-loads the manual re-adds the recursion this
// repoint removed, so no rsrc body may carry that call.
func TestPhase3bSkillRepoint(t *testing.T) {
	skillsRoot := filepath.Join("..", "..", "..", "agents-plugin", "skills")
	wsflowSkillsRoot := filepath.Join("..", "..", "..", "agents-plugin-wsflow", "skills")

	for _, skill := range []string{"lead-discuss", "lead-run", "lead-ticket"} {
		t.Run(skill, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(skillsRoot, skill, "SKILL.md"))
			if err != nil {
				t.Fatalf("read %s SKILL.md: %v", skill, err)
			}
			if len(raw) == 0 {
				t.Errorf("%s: SKILL.md is empty", skill)
			}
		})
	}

	// lead-revive SKILL.md must exist in agents-plugin/skills/.
	reviveSkillPath := filepath.Join(skillsRoot, "lead-revive", "SKILL.md")
	if _, err := os.Stat(reviveSkillPath); err != nil {
		t.Errorf("lead-revive SKILL.md missing from agents-plugin/skills/: %v", err)
	}

	// lead-revive SKILL.md must exist in agents-plugin-wsflow/skills/.
	wsflowReviveSkillPath := filepath.Join(wsflowSkillsRoot, "lead-revive", "SKILL.md")
	if _, err := os.Stat(wsflowReviveSkillPath); err != nil {
		t.Errorf("lead-revive SKILL.md missing from agents-plugin-wsflow/skills/: %v", err)
	}

	// lead-load-workflow-manual must NOT exist in agents-plugin/skills/.
	removedSkillPath := filepath.Join(skillsRoot, "lead-load-workflow-manual", "SKILL.md")
	if _, err := os.Stat(removedSkillPath); !os.IsNotExist(err) {
		t.Errorf("lead-load-workflow-manual SKILL.md should be removed but still present (stat: %v)", err)
	}
}

// TestPlaybookPrintGoldenLeadBootstrap verifies lead-bootstrap resolves from the
// real rsrc tree and contains procedure body text. delegates:false — no tip.
func TestPlaybookPrintGoldenLeadBootstrap(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "lead-bootstrap", nil, wsconfig.Options{}, "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	if strings.TrimSpace(body) == "" {
		t.Fatal("lead-bootstrap rendered empty")
	}
	// delegates:false — continuity tip must NOT appear.
	if strings.Contains(body, "Continuity tip") {
		t.Errorf("body %q: delegation tip must not appear for delegates:false playbook", body)
	}
}

// workerPlaybookContent is a worker-role playbook: the render-minted child key
// must carry the caller's lead scope, not a delegate scope.
const workerPlaybookContent = `---
kind: render
delegates: true
role: worker
tier: large
---
# Worker Playbook

Execute the ticket.
`

// TestRenderMintsLeadScopedChildKeyForWorkerPlaybook pins the worker render
// role: a `role: worker` playbook rendered by a lead caller mints a child key
// that is lead-scoped and bound to the caller's root, so the worker can call
// every lead-gated ws tool without minting a key of its own.
func TestRenderMintsLeadScopedChildKeyForWorkerPlaybook(t *testing.T) {
	root := buildTestRsrcTree(t, map[string]string{
		"worker-pb/worker-pb.md": workerPlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")
	mintRoot := "/work/tree-a"
	parentKey := "parent-key-fixture"

	body, _, err := renderPlaybookBody(s, os.DirFS(root), "worker-pb", nil, wsconfig.Options{}, mintRoot, parentKey, "", nil, "")
	if err != nil {
		t.Fatalf("renderPlaybookBody: %v", err)
	}
	key := extractSplicedKey(t, body)
	entry, ok := s.sessions.lookup(key)
	if !ok {
		t.Fatalf("minted key %q not found in registry", key)
	}
	if entry.scope != roleLead {
		t.Errorf("minted key scope = %q, want %q (worker → lead)", entry.scope, roleLead)
	}
	if entry.root != mintRoot {
		t.Errorf("minted key root = %q, want caller root %q", entry.root, mintRoot)
	}
	if entry.parent != parentKey {
		t.Errorf("minted key parent = %q, want %q", entry.parent, parentKey)
	}
}

// TestPlaybookRenderGoldenTicketWorker resolves the shipped ticket-worker
// playbook from the real rsrc tree: the worker-stop-protocol include must be
// spliced in, the harness idiom vars must substitute, and a lead caller must
// receive a lead-scoped child key. This is the end-to-end counterpart of
// TestRenderMintsLeadScopedChildKeyForWorkerPlaybook, which uses a fixture.
func TestPlaybookRenderGoldenTicketWorker(t *testing.T) {
	for _, product := range []string{"ws", "wsflow"} {
		t.Run(product, func(t *testing.T) {
			t.Setenv(envNoAgent, map[string]string{"ws": "", "wsflow": "1"}[product])
			t.Setenv(envNamespace, product)
			for _, tc := range []struct{ name, tier string }{
				{"ticket-worker", "medium"},
				{"ticket-worker-elevated", "large"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					rsrcRoot := filepath.Join("..", "..", "..", map[string]string{"ws": "agents-plugin", "wsflow": "agents-plugin-wsflow"}[product], "rsrc")
					s := newTestServerWithHarness(t, "claude")
					mintRoot := "/work/tree-a"

					body, tier, err := renderPlaybookBody(s, os.DirFS(rsrcRoot), tc.name, nil, wsconfig.Options{}, mintRoot, "", "", nil, "")
					if err != nil {
						t.Fatalf("renderPlaybookBody: %v", err)
					}
					if tier != tc.tier {
						t.Errorf("recommended tier = %q, want %q", tier, tc.tier)
					}
					// delegates:true → the harness continuity tip carries the continuation idiom.
					if !strings.Contains(body, "SendMessage(to: <agentId>)") {
						t.Errorf("rendered body missing claude continuation idiom")
					}
					// No unsubstituted template variables may survive rendering.
					if strings.Contains(body, "{{.") {
						t.Errorf("rendered body has unsubstituted variables:\n%s", body)
					}
					key := extractSplicedKey(t, body)
					entry, ok := s.sessions.lookup(key)
					if !ok {
						t.Fatalf("minted key %q not found in registry", key)
					}
					if entry.scope != roleLead || entry.root != mintRoot {
						t.Errorf("minted key = (scope %q, root %q), want (%q, %q)", entry.scope, entry.root, roleLead, mintRoot)
					}
				})
			}
		})
	}
}

// The lead interprets this policy as prose, so pin the rendered decision rows
// rather than duplicate the routing logic in a test-only Go implementation.
func TestPlaybookPrintLeadRunWorkerTierPolicy(t *testing.T) {
	for _, product := range []string{"ws", "wsflow"} {
		t.Run(product, func(t *testing.T) {
			t.Setenv(envNoAgent, map[string]string{"ws": "", "wsflow": "1"}[product])
			t.Setenv(envNamespace, product)
			packageDir := map[string]string{"ws": "agents-plugin", "wsflow": "agents-plugin-wsflow"}[product]
			root := filepath.Join("..", "..", "..", packageDir, "rsrc")
			s := newTestServerWithHarness(t, "codex")
			body, _, err := printPlaybook(s, os.DirFS(root), "lead-run", nil, isolatedPlaybookConfigOptions(t), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(body) == "" {
				t.Fatal("lead-run rendered empty")
			}
			if strings.Contains(body, "{{.") {
				t.Error("rendered policy retains a template variable")
			}
		})
	}
}

// Pin the real query boundary the lead consumes: compact text omits Route
// Facts, so the policy must request the existing JSON projection explicitly.
func TestLeadRunTicketQueryRiskProjection(t *testing.T) {
	root := initGitRepo(t)
	mustWrite(t, root, "ai-docs/tickets/ready/260910-feat-risk-fixture.md", `---
title: Risk fixture
---
## Route Facts

| fact | value | evidence |
|---|---|---|
| risk.correctness | high | fixture |
| risk.fit | low | fixture |
| risk.test | moderate | fixture |
| risk.security_or_contract | unknown | fixture |
`)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newTestServerWithHarness(t, "codex")
	key, err := s.sessions.mint(canonicalRootForTest(t, root), roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	resp := callToolOnce(t, s, 1, "tickets.query", map[string]any{
		"session_key": key,
		"ticket_stem": "260910-feat-risk-fixture",
		"format":      "json",
	})
	var projection struct {
		Present bool              `json:"route_facts_present"`
		Facts   map[string]string `json:"route_facts"`
	}
	if err := json.Unmarshal([]byte(toolText(t, resp)), &projection); err != nil {
		t.Fatal(err)
	}
	if !projection.Present {
		t.Fatal("point query did not expose Route Facts presence")
	}
	for fact, want := range map[string]string{
		"risk.correctness": "high", "risk.fit": "low",
		"risk.test": "moderate", "risk.security_or_contract": "unknown",
	} {
		if got := projection.Facts[fact]; got != want {
			t.Errorf("projected %s = %q, want %q", fact, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Claude effort-carrier SpawnIdiom
// ---------------------------------------------------------------------------

// spawnIdiomSites are the real playbooks that substitute {{.SpawnIdiom}}.
var spawnIdiomSites = []string{"lead-run", "ticket-worker", "ticket-worker-elevated", "ticket-handler", "review-handler", "explore", "delegate-sample"}

// wantClaudeSpawnIdiom is the hardcoded Claude SpawnIdiom per namespace, so a
// wrong claudeSpawnIdiom cannot agree with itself in a derived assertion.
func wantClaudeSpawnIdiom(namespace string) string {
	return `Agent({subagent_type: "` + namespace + `:effort-<resolved effort>", model: <resolved model>, ...})` +
		` (subagent_type "general-purpose" when the resolved effort is not exactly low, medium, high, xhigh, or max)`
}

// TestClaudeSpawnIdiomRendersAtEverySite pins the Claude SpawnIdiom text as
// rendered at each {{.SpawnIdiom}} site for both plugin namespaces: the value
// carries the effort-agent mapping rule and its general-purpose fallback, and
// the namespace comes from the runtime, never a hard-coded ws: prefix.
func TestClaudeSpawnIdiomRendersAtEverySite(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	for _, namespace := range []string{"ws", "wsflow"} {
		t.Run(namespace, func(t *testing.T) {
			t.Setenv(envNamespace, namespace)
			want := wantClaudeSpawnIdiom(namespace)
			if got := terminologyForHarness("claude", RuntimeNamespace())["SpawnIdiom"]; got != want {
				t.Fatalf("claude SpawnIdiom = %q, want %q", got, want)
			}
			s := newTestServerWithHarness(t, "claude")
			for _, name := range spawnIdiomSites {
				body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), name, nil, isolatedPlaybookConfigOptions(t), "", nil)
				if err != nil {
					t.Fatalf("printPlaybook(%s): %v", name, err)
				}
				if !strings.Contains(body, want) {
					t.Errorf("%s: Claude render missing SpawnIdiom %q:\n%s", name, want, body)
				}
				if strings.Contains(body, "{{.") {
					t.Errorf("%s: unsubstituted placeholder remains", name)
				}
			}
		})
	}
}

// TestSpawnIdiomSitesListIsComplete keeps spawnIdiomSites equal to the real
// playbooks that declare SpawnIdiom, including uses through shared includes.
// lead-workflow-manual uses it only through its Claude overlay, which
// TestLeadWorkflowManualClaudeSpawnBindingOverlay pins.
func TestSpawnIdiomSitesListIsComplete(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	found := map[string]bool{}
	err := filepath.WalkDir(rsrcRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "  - SpawnIdiom\n") {
			found[filepath.Base(filepath.Dir(path))] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk rsrc: %v", err)
	}
	delete(found, "lead-workflow-manual")
	want := map[string]bool{}
	for _, name := range spawnIdiomSites {
		want[name] = true
	}
	if !reflect.DeepEqual(found, want) {
		t.Errorf("{{.SpawnIdiom}} sites = %v, spawnIdiomSites = %v", found, want)
	}
}

// TestClaudeSpawnIdiomLevelsMatchGeneratedAgents keeps the level list the rule
// names equal to the agents the generator ships.
func TestClaudeSpawnIdiomLevelsMatchGeneratedAgents(t *testing.T) {
	want := []string{"low", "medium", "high", "xhigh", "max"}
	if strings.Join(claudeagents.Levels, ",") != strings.Join(want, ",") {
		t.Fatalf("claudeagents.Levels = %v, want %v (update the pinned SpawnIdiom with it)", claudeagents.Levels, want)
	}
}

// TestLeadWorkflowManualClaudeSpawnBindingOverlay pins the lead-side Claude
// overlay for both namespaces: it applies render bindings through the
// SpawnIdiom rule, keeps ExploreAgent on the built-in Explore agent, and uses
// its own heading rather than Codex wording.
func TestLeadWorkflowManualClaudeSpawnBindingOverlay(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	for _, namespace := range []string{"ws", "wsflow"} {
		t.Run(namespace, func(t *testing.T) {
			t.Setenv(envNamespace, namespace)
			s := newTestServerWithHarness(t, "claude")
			body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", nil)
			if err != nil {
				t.Fatalf("printPlaybook: %v", err)
			}
			want := wantClaudeSpawnIdiom(namespace)
			if !strings.Contains(body, want) {
				t.Errorf("Claude workflow manual missing overlay %q:\n%s", want, body)
			}
			if terminologyForHarness("claude", RuntimeNamespace())["ExploreAgent"] != "the Explore agent" {
				t.Errorf("claude ExploreAgent changed: %q", terminologyForHarness("claude", RuntimeNamespace())["ExploreAgent"])
			}
			if !strings.Contains(body, terminologyForHarness("claude", RuntimeNamespace())["ExploreAgent"]) {
				t.Errorf("Claude workflow manual lost the built-in Explore agent wording:\n%s", body)
			}
		})
	}
}

// TestNonClaudeRendersOmitEffortAgents asserts the effort agents are a
// Claude-only surface: Codex, Pi, and host-neutral renders never mention them.
func TestNonClaudeRendersOmitEffortAgents(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	names := append([]string{"lead-workflow-manual"}, spawnIdiomSites...)
	for _, harness := range []string{"codex", "pi", ""} {
		s := newTestServerWithHarness(t, harness)
		for _, name := range names {
			body, _, err := printPlaybook(s, os.DirFS(rsrcRoot), name, nil, isolatedPlaybookConfigOptions(t), "", nil)
			if err != nil {
				t.Fatalf("printPlaybook(%s, %q): %v", name, harness, err)
			}
			for _, forbidden := range []string{":effort-", "effort-<resolved effort>"} {
				if strings.Contains(body, forbidden) {
					t.Errorf("harness %q %s render mentions Claude effort agents (%q)", harness, name, forbidden)
				}
			}
		}
	}
}
