package mcp

// prompt_override_test.go — integration tests for the override-marker engine
// (260619-feat-ws-prompt-override-marker-engine; Phase 1 engine, Phase 2 seed).
//
// Coverage:
//   1. No-override: seed renders, markers stripped.
//   2. Per-harness override: body replaced for the matching harness; a different
//      harness still receives its seed.
//   3. "all" override: applies when no harness-specific override is stored.
//   4. Empty-seed slot: renders stored override or nothing when none is set.
//   5. Production-path case: override stored via the real resolver/session store
//      is honored at playbook.render time (mirrors
//      TestPreferMercenaryOnOffRenderGuidanceProductionPath).
//   6. Phase 2 shipped manual: DelegationSection is absent, while the
//      UserPreferenceSection marker remains the shipped freeform slot.
//
// Unit-level cases (1–4) drive renderPlaybookBody with an injected fake
// overrideLookupFn.  Case 5 uses the production dispatch (callToolOnce on
// playbook.render) so the server-side resolver wiring is exercised end-to-end.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// ---------------------------------------------------------------------------
// Test fixture: playbook with override-points
// ---------------------------------------------------------------------------

// overridePlaybookContent is a minimal playbook containing:
//   - A non-empty seed override-point (SeedSection).
//   - An empty-seed extension slot (ExtSlot).
//
// It declares no template variables; seed text is static prose.
const overridePlaybookContent = `---
kind: print
delegates: false
---
# Override Test Playbook

Before section.

<!-- ws:override:SeedSection desc="a seeded override point" -->
original seed text
<!-- ws:/override:SeedSection -->

Between sections.

<!-- ws:override:ExtSlot desc="an empty extension slot" -->
<!-- ws:/override:ExtSlot -->

After sections.
`

// buildOverrideTestTree creates a minimal rsrc tree containing the override
// fixture playbook and returns the rsrc root path.
func buildOverrideTestTree(t *testing.T) string {
	t.Helper()
	return buildTestRsrcTree(t, map[string]string{
		"override-pb/override-pb.md": overridePlaybookContent,
	})
}

// staticLookup returns an overrideLookupFn that resolves from a static map
// keyed by "<pointId>/<harness>".  An absent key returns ("", false).
func staticLookup(m map[string]string) overrideLookupFn {
	return func(pointId, harness string) (string, bool) {
		v, ok := m[pointId+"/"+harness]
		return v, ok
	}
}

// ---------------------------------------------------------------------------
// Case 1: no-override → seed renders, markers stripped
// ---------------------------------------------------------------------------

func TestOverrideNoOverrideSeedRenders(t *testing.T) {
	rsrcRoot := buildOverrideTestTree(t)
	s := newTestServerWithHarness(t, "claude")

	// nil lookup: every override-point falls back to its seed.
	body, _, err := renderPlaybookBody(s, rsrcRoot, "override-pb", nil, wsconfig.Options{}, "", "", false, "", nil)
	if err != nil {
		t.Fatalf("renderPlaybookBody: %v", err)
	}

	// Seed text must appear in output.
	if !strings.Contains(body, "original seed text") {
		t.Errorf("seed text must appear in no-override render:\n%s", body)
	}

	// Marker syntax must NOT appear in output.
	if strings.Contains(body, "ws:override:") {
		t.Errorf("marker syntax must not appear in rendered output:\n%s", body)
	}
	if strings.Contains(body, "ws:/override:") {
		t.Errorf("close marker must not appear in rendered output:\n%s", body)
	}

	// Static structural content must remain intact, including the content between
	// the two marker blocks.
	if !strings.Contains(body, "Before section.") {
		t.Errorf("content before marker block must be preserved:\n%s", body)
	}
	if !strings.Contains(body, "Between sections.") {
		t.Errorf("content between marker blocks must be preserved:\n%s", body)
	}
	if !strings.Contains(body, "After sections.") {
		t.Errorf("content after marker block must be preserved:\n%s", body)
	}
}

// ---------------------------------------------------------------------------
// Case 2: per-harness override → body replaced for matching harness only
// ---------------------------------------------------------------------------

func TestOverridePerHarnessReplacement(t *testing.T) {
	rsrcRoot := buildOverrideTestTree(t)

	lookup := staticLookup(map[string]string{
		"SeedSection/claude": "claude-specific override text",
	})

	// Claude harness: should get the per-harness override.
	sClaude := newTestServerWithHarness(t, "claude")
	bodyClaude, _, err := renderPlaybookBody(sClaude, rsrcRoot, "override-pb", nil, wsconfig.Options{}, "", "", false, "", lookup)
	if err != nil {
		t.Fatalf("renderPlaybookBody (claude): %v", err)
	}
	if !strings.Contains(bodyClaude, "claude-specific override text") {
		t.Errorf("claude render must show per-harness override:\n%s", bodyClaude)
	}
	if strings.Contains(bodyClaude, "original seed text") {
		t.Errorf("claude render must not show seed when override is present:\n%s", bodyClaude)
	}

	// Codex harness: no codex-specific or "all" override → must fall back to seed.
	sCodex := newTestServerWithHarness(t, "codex")
	bodyCodex, _, err := renderPlaybookBody(sCodex, rsrcRoot, "override-pb", nil, wsconfig.Options{}, "", "", false, "", lookup)
	if err != nil {
		t.Fatalf("renderPlaybookBody (codex): %v", err)
	}
	if !strings.Contains(bodyCodex, "original seed text") {
		t.Errorf("codex render must fall back to seed when no codex/all override:\n%s", bodyCodex)
	}
	if strings.Contains(bodyCodex, "claude-specific override text") {
		t.Errorf("codex render must not see claude-specific override:\n%s", bodyCodex)
	}

	// Marker syntax must be absent in both outputs.
	for _, body := range []string{bodyClaude, bodyCodex} {
		if strings.Contains(body, "ws:override:") || strings.Contains(body, "ws:/override:") {
			t.Errorf("marker syntax must never appear in rendered output:\n%s", body)
		}
	}
}

// ---------------------------------------------------------------------------
// Case 3: "all" override → applies when no harness-specific override exists
// ---------------------------------------------------------------------------

func TestOverrideAllBucketFallback(t *testing.T) {
	rsrcRoot := buildOverrideTestTree(t)

	lookup := staticLookup(map[string]string{
		"SeedSection/all": "all-harness override text",
	})

	// Claude harness: no claude-specific override → "all" applies.
	sClaude := newTestServerWithHarness(t, "claude")
	bodyClaude, _, err := renderPlaybookBody(sClaude, rsrcRoot, "override-pb", nil, wsconfig.Options{}, "", "", false, "", lookup)
	if err != nil {
		t.Fatalf("renderPlaybookBody (claude): %v", err)
	}
	if !strings.Contains(bodyClaude, "all-harness override text") {
		t.Errorf("claude render must pick up all-bucket override:\n%s", bodyClaude)
	}
	if strings.Contains(bodyClaude, "original seed text") {
		t.Errorf("claude render must not show seed when all-bucket override is set:\n%s", bodyClaude)
	}

	// Codex harness: same expectation.
	sCodex := newTestServerWithHarness(t, "codex")
	bodyCodex, _, err := renderPlaybookBody(sCodex, rsrcRoot, "override-pb", nil, wsconfig.Options{}, "", "", false, "", lookup)
	if err != nil {
		t.Fatalf("renderPlaybookBody (codex): %v", err)
	}
	if !strings.Contains(bodyCodex, "all-harness override text") {
		t.Errorf("codex render must pick up all-bucket override:\n%s", bodyCodex)
	}

	// When both harness-specific AND all are present, harness-specific wins.
	lookupBoth := staticLookup(map[string]string{
		"SeedSection/claude": "claude wins",
		"SeedSection/all":    "all-bucket",
	})
	bodyBoth, _, err := renderPlaybookBody(sClaude, rsrcRoot, "override-pb", nil, wsconfig.Options{}, "", "", false, "", lookupBoth)
	if err != nil {
		t.Fatalf("renderPlaybookBody (both): %v", err)
	}
	if !strings.Contains(bodyBoth, "claude wins") {
		t.Errorf("harness-specific must win over all-bucket:\n%s", bodyBoth)
	}
	if strings.Contains(bodyBoth, "all-bucket") {
		t.Errorf("all-bucket must not appear when harness-specific is set:\n%s", bodyBoth)
	}
}

// ---------------------------------------------------------------------------
// Case 4: empty-seed slot → renders override or nothing when none is set
// ---------------------------------------------------------------------------

func TestOverrideEmptySeedSlot(t *testing.T) {
	rsrcRoot := buildOverrideTestTree(t)
	s := newTestServerWithHarness(t, "claude")

	// No override stored: extension slot must render nothing (empty body).
	bodyNoOverride, _, err := renderPlaybookBody(s, rsrcRoot, "override-pb", nil, wsconfig.Options{}, "", "", false, "", nil)
	if err != nil {
		t.Fatalf("renderPlaybookBody (no override): %v", err)
	}
	// The slot had no seed content, so it should vanish entirely.
	if strings.Contains(bodyNoOverride, "ws:override:ExtSlot") || strings.Contains(bodyNoOverride, "ws:/override:ExtSlot") {
		t.Errorf("extension slot markers must be stripped even when no override:\n%s", bodyNoOverride)
	}

	// With an override stored for ExtSlot: the override text must appear.
	lookup := staticLookup(map[string]string{
		"ExtSlot/claude": "injected extension text",
	})
	bodyWithOverride, _, err := renderPlaybookBody(s, rsrcRoot, "override-pb", nil, wsconfig.Options{}, "", "", false, "", lookup)
	if err != nil {
		t.Fatalf("renderPlaybookBody (with override): %v", err)
	}
	if !strings.Contains(bodyWithOverride, "injected extension text") {
		t.Errorf("stored override must appear for empty-seed slot:\n%s", bodyWithOverride)
	}
	if strings.Contains(bodyWithOverride, "ws:override:") {
		t.Errorf("marker syntax must not appear after override substitution:\n%s", bodyWithOverride)
	}
}

// ---------------------------------------------------------------------------
// Marker-stripping robustness — no override marker syntax may ever survive
// ---------------------------------------------------------------------------

// assertNoMarkerSyntax fails the test if any override marker syntax survives in s.
func assertNoMarkerSyntax(t *testing.T, label, s string) {
	t.Helper()
	if strings.Contains(s, overrideOpenPrefix) || strings.Contains(s, overrideClosePrefix) {
		t.Errorf("%s: override marker syntax survived in output:\n%s", label, s)
	}
}

// TestApplyOverrideMarkersCloseSpacingLeniency verifies the close marker is
// recognized with the same spacing leniency as the open marker: a close marker
// with no space before `-->` must still close the block and be stripped, not
// swept into the seed and re-emitted.
func TestApplyOverrideMarkersCloseSpacingLeniency(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"close no space", "<!-- ws:override:A -->\nseed\n<!-- ws:/override:A-->"},
		{"open no space", "<!-- ws:override:A-->\nseed\n<!-- ws:/override:A -->"},
		{"both no space", "<!-- ws:override:A-->\nseed\n<!-- ws:/override:A-->"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// nil lookup → seed renders, both markers stripped.
			got := applyOverrideMarkers(tc.body, "claude", nil)
			if !strings.Contains(got, "seed") {
				t.Errorf("seed must survive:\n%s", got)
			}
			assertNoMarkerSyntax(t, tc.name, got)

			// With an override → seed replaced, markers stripped.
			lookup := staticLookup(map[string]string{"A/claude": "override"})
			got2 := applyOverrideMarkers(tc.body, "claude", lookup)
			if !strings.Contains(got2, "override") || strings.Contains(got2, "seed") {
				t.Errorf("override must replace seed:\n%s", got2)
			}
			assertNoMarkerSyntax(t, tc.name+" override", got2)
		})
	}
}

// TestApplyOverrideMarkersNesting verifies a nested override block inside a seed
// is processed recursively so no inner marker line survives in the output.
func TestApplyOverrideMarkersNesting(t *testing.T) {
	body := strings.Join([]string{
		"<!-- ws:override:Outer -->",
		"outer-pre",
		"<!-- ws:override:Inner -->",
		"inner-seed",
		"<!-- ws:/override:Inner -->",
		"outer-post",
		"<!-- ws:/override:Outer -->",
	}, "\n")

	// nil lookup → both seeds render, all four marker lines stripped.
	got := applyOverrideMarkers(body, "claude", nil)
	assertNoMarkerSyntax(t, "nested nil", got)
	for _, want := range []string{"outer-pre", "inner-seed", "outer-post"} {
		if !strings.Contains(got, want) {
			t.Errorf("nested nil: %q must survive:\n%s", want, got)
		}
	}

	// Inner override set → inner seed replaced, no markers survive.
	lookup := staticLookup(map[string]string{"Inner/claude": "inner-override"})
	got2 := applyOverrideMarkers(body, "claude", lookup)
	assertNoMarkerSyntax(t, "nested inner override", got2)
	if !strings.Contains(got2, "inner-override") || strings.Contains(got2, "inner-seed") {
		t.Errorf("inner override must replace inner seed:\n%s", got2)
	}
	if !strings.Contains(got2, "outer-pre") || !strings.Contains(got2, "outer-post") {
		t.Errorf("outer seed content must survive:\n%s", got2)
	}

	// Outer override set → whole outer block (including inner markers) replaced.
	lookupOuter := staticLookup(map[string]string{"Outer/claude": "outer-override"})
	got3 := applyOverrideMarkers(body, "claude", lookupOuter)
	assertNoMarkerSyntax(t, "nested outer override", got3)
	if !strings.Contains(got3, "outer-override") {
		t.Errorf("outer override must appear:\n%s", got3)
	}
	for _, gone := range []string{"outer-pre", "inner-seed", "outer-post"} {
		if strings.Contains(got3, gone) {
			t.Errorf("outer override must replace entire block; %q leaked:\n%s", gone, got3)
		}
	}
}

// TestApplyOverrideMarkersUnclosedMarkerPreservesContent verifies an unclosed
// open marker (EOF with no matching close) is NOT treated as a block: its line
// and all following content are emitted unchanged, even when an override is
// stored for that pointId (no silent consumption or truncation).
func TestApplyOverrideMarkersUnclosedMarkerPreservesContent(t *testing.T) {
	body := strings.Join([]string{
		"before",
		"<!-- ws:override:Dangling -->",
		"content after open marker",
		"more content",
	}, "\n")

	// Even with an override stored, the unclosed block must not consume content.
	lookup := staticLookup(map[string]string{"Dangling/claude": "should-not-apply"})
	got := applyOverrideMarkers(body, "claude", lookup)

	for _, want := range []string{"before", "content after open marker", "more content"} {
		if !strings.Contains(got, want) {
			t.Errorf("unclosed marker must preserve %q:\n%s", want, got)
		}
	}
	// The override must NOT be applied for an unclosed block.
	if strings.Contains(got, "should-not-apply") {
		t.Errorf("override must not apply to an unclosed (malformed) block:\n%s", got)
	}
	// The dangling open line is emitted verbatim here (it is not a valid block),
	// which is acceptable: the content-preservation guarantee is the contract.
	// What matters for fix #2 is that nothing was consumed/truncated.
}

// ---------------------------------------------------------------------------
// Case 5: production-path — override stored via resolver is honored at render
// ---------------------------------------------------------------------------

// TestOverrideProductionPath verifies that an override stored through the real
// wsconfig resolver/session store is resolved by playbook.render at render time
// and that marker syntax never appears in the output file.
//
// This mirrors TestPreferMercenaryOnOffRenderGuidanceProductionPath in structure:
// sequential callToolOnce calls on the same *Server guarantee session-write
// visibility, and the override is seeded directly via the resolver (not via
// config.prompt.set which belongs to the sibling ticket).
func TestOverrideProductionPath(t *testing.T) {
	useLeadProfile(t)

	rsrcRoot := buildOverrideTestTree(t)
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)

	// Separate git repo as the session-bound worktree root.
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	s := NewServer(root, "test")
	// Set harness to "claude" so per-harness lookups resolve predictably.
	s.observeHarness("test", "claude")

	// Bootstrap a lead session key.
	key, _ := parseLoginResponse(t, callLogin(t, s, 900100, root, nil))

	// Seed an override directly through the resolver (Phase 1 — no setter tool yet).
	// The key is "prompt.SeedSection.claude" as per the storage convention.
	// ExplicitScope: ScopeSession forces the write into the per-key session store;
	// without it, the unregistered prompt.* key would default to project scope and
	// the test would never exercise the session→resolver→render path.
	adapter := sessionConfigAdapter{s: s.sessions}
	resolver := wsconfig.NewResolver(wsconfig.Options{}, nil, adapter, adapter)
	if err := resolver.Set("prompt.SeedSection.claude", "production-path override", wsconfig.SetOptions{
		ExplicitScope: wsconfig.ScopeSession,
		SessionKey:    key,
	}); err != nil {
		t.Fatalf("seed override via resolver: %v", err)
	}

	// Call playbook.render through the production dispatch path.
	renderResp := callToolOnce(t, s, 1, "playbook.render", map[string]any{
		"name":        "override-pb",
		"session_key": key,
	})
	renderText := toolText(t, renderResp)
	// First line of text response is the path.
	renderedPath := strings.SplitN(strings.TrimSpace(renderText), "\n", 2)[0]
	renderedBody, err := os.ReadFile(renderedPath)
	if err != nil {
		t.Fatalf("read rendered playbook: %v", err)
	}
	body := string(renderedBody)

	// The override must appear in the output.
	if !strings.Contains(body, "production-path override") {
		t.Errorf("production-path override must appear in rendered output:\n%s", body)
	}

	// The original seed must not appear (it was replaced).
	if strings.Contains(body, "original seed text") {
		t.Errorf("original seed must not appear when override is set:\n%s", body)
	}

	// Marker syntax must never appear in the rendered file.
	if strings.Contains(body, "ws:override:") || strings.Contains(body, "ws:/override:") {
		t.Errorf("marker syntax must not appear in rendered output file:\n%s", body)
	}
}

// TestOverridePrintProductionPath mirrors TestOverrideProductionPath for the
// playbook.print dispatch path: it stores an override via the session-scope
// resolver and then calls playbook.print with the same session_key, proving the
// print-path override-lookup wiring (and the print inputSchema's session_key
// advertisement) resolves overrides end-to-end. Marker syntax must never appear.
func TestOverridePrintProductionPath(t *testing.T) {
	useLeadProfile(t)

	rsrcRoot := buildOverrideTestTree(t)
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	s := NewServer(root, "test")
	s.observeHarness("test", "claude")

	// Bootstrap a lead session key.
	key, _ := parseLoginResponse(t, callLogin(t, s, 900200, root, nil))

	// Seed a session-scoped override through the resolver.
	adapter := sessionConfigAdapter{s: s.sessions}
	resolver := wsconfig.NewResolver(wsconfig.Options{}, nil, adapter, adapter)
	if err := resolver.Set("prompt.SeedSection.claude", "print-path override", wsconfig.SetOptions{
		ExplicitScope: wsconfig.ScopeSession,
		SessionKey:    key,
	}); err != nil {
		t.Fatalf("seed override via resolver: %v", err)
	}

	// Call playbook.print through the production dispatch path, passing session_key.
	printResp := callToolOnce(t, s, 1, "playbook.print", map[string]any{
		"name":        "override-pb",
		"session_key": key,
	})
	body := toolText(t, printResp)

	// The override must appear; the original seed must not; markers must be gone.
	if !strings.Contains(body, "print-path override") {
		t.Errorf("print-path override must appear in inline output:\n%s", body)
	}
	if strings.Contains(body, "original seed text") {
		t.Errorf("original seed must not appear when override is set:\n%s", body)
	}
	if strings.Contains(body, "ws:override:") || strings.Contains(body, "ws:/override:") {
		t.Errorf("marker syntax must not appear in print output:\n%s", body)
	}
}

// ---------------------------------------------------------------------------
// Case 6: Phase 2 — shipped lead-workflow-manual override surface
// ---------------------------------------------------------------------------

// TestShippedWorkflowManualOmitsDelegationSection verifies that the shipped
// workflow manual no longer carries the legacy posture override marker or seed.
func TestShippedWorkflowManualOmitsDelegationSection(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}
	for _, forbidden := range []string{
		"DelegationSection",
		"### Delegation posture",
		"Delegate all work to subagents for this session.",
		"Dispatch:",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("workflow manual must not contain removed delegation surface %q:\n%s", forbidden, body)
		}
	}
	if !strings.Contains(body, "### User preferences") {
		t.Errorf("workflow manual must keep the user-preferences heading:\n%s", body)
	}
	assertNoMarkerSyntax(t, "shipped manual", body)
	assertManualStructureIntact(t, "shipped manual", body)
}

// TestShippedUserPreferenceSectionEmptySlotAndOverride verifies the shipped
// user-preference extension slot. With no override it renders the static
// default-preferences seed text (260702); an all-harness override appends
// preference guidance without replacing delegation posture.
func TestShippedUserPreferenceSectionEmptySlotAndOverride(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "codex")

	const preferenceText = "User preferences:\n- Prefer conventional terminology when user wording is imprecise."

	baseBody, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", nil)
	if err != nil {
		t.Fatalf("printPlaybook (base): %v", err)
	}
	if !strings.Contains(baseBody, "### User preferences") {
		t.Errorf("base render must keep the user-preferences heading:\n%s", baseBody)
	}
	if strings.Contains(baseBody, "Prefer conventional terminology") {
		t.Errorf("empty user-preference slot must not render preference text without override:\n%s", baseBody)
	}
	assertNoMarkerSyntax(t, "user preference base", baseBody)
	assertManualStructureIntact(t, "user preference base", baseBody)

	lookup := staticLookup(map[string]string{
		"UserPreferenceSection/all": preferenceText,
	})
	ovBody, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", lookup)
	if err != nil {
		t.Fatalf("printPlaybook (override): %v", err)
	}
	if !strings.Contains(ovBody, preferenceText) {
		t.Errorf("user-preference override must render:\n%s", ovBody)
	}
	if strings.Contains(ovBody, "DelegationSection") || strings.Contains(ovBody, "Delegate all work to subagents for this session.") {
		t.Errorf("user-preference override must not restore removed delegation posture:\n%s", ovBody)
	}
	assertNoMarkerSyntax(t, "user preference override", ovBody)
	assertManualStructureIntact(t, "user preference override", ovBody)
}

// TestWorkflowLangInjectionIntoUserPreferenceSection verifies that a non-empty
// workflowLang generates a language-binding instruction in the ### User preferences
// seed, and that an empty workflowLang leaves the section empty.
func TestWorkflowLangInjectionIntoUserPreferenceSection(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	// Empty workflowLang → section stays empty (no lang instruction).
	emptyBody, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", nil)
	if err != nil {
		t.Fatalf("printPlaybook (empty lang): %v", err)
	}
	if strings.Contains(emptyBody, "Respond to the user in") {
		t.Errorf("empty workflowLang must not inject lang instruction:\n%s", emptyBody)
	}
	assertNoMarkerSyntax(t, "empty lang", emptyBody)

	// Non-empty workflowLang → instruction appears in User preferences seed.
	langBody, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "Korean", nil)
	if err != nil {
		t.Fatalf("printPlaybook (Korean): %v", err)
	}
	if !strings.Contains(langBody, "Respond to the user in Korean") {
		t.Errorf("workflowLang=Korean must inject lang instruction:\n%s", langBody)
	}
	if !strings.Contains(langBody, "### User preferences") {
		t.Errorf("User preferences heading must survive lang injection:\n%s", langBody)
	}
	assertNoMarkerSyntax(t, "Korean lang", langBody)
	assertManualStructureIntact(t, "Korean lang", langBody)
}

// TestShippedManualSessionSetupAndUserPreferenceSectionsAreNotThin verifies the
// 260702 fix: the shipped Session setup section states the ferrule
// redundant-mint consequence (a second call for the same root mints a new
// session identity with empty state, stranding prior agenda/todo state), and
// the User preferences section is never fully empty in the default render
// (no override, no workflow.lang configured).
func TestShippedManualSessionSetupAndUserPreferenceSectionsAreNotThin(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", nil)
	if err != nil {
		t.Fatalf("printPlaybook: %v", err)
	}

	if !strings.Contains(body, "### Session setup") {
		t.Fatalf("workflow manual must keep the Session setup heading:\n%s", body)
	}
	if !strings.Contains(body, "mints a brand-new session key with empty state") {
		t.Errorf("Session setup must state the redundant-mint consequence:\n%s", body)
	}
	if !strings.Contains(body, "stranding any agenda, todo, or session-tree state") {
		t.Errorf("Session setup must name the stranded state kinds:\n%s", body)
	}

	// Extract the User preferences section body (between its heading and the
	// next heading) and assert it is not empty/whitespace-only.
	idx := strings.Index(body, "### User preferences")
	if idx < 0 {
		t.Fatalf("workflow manual must keep the User preferences heading:\n%s", body)
	}
	rest := body[idx+len("### User preferences"):]
	if next := strings.Index(rest, "\n### "); next >= 0 {
		rest = rest[:next]
	}
	if strings.TrimSpace(rest) == "" {
		t.Errorf("User preferences section must not be empty in the default render:\n%s", body)
	}
	assertNoMarkerSyntax(t, "session setup and user preferences", body)
	assertManualStructureIntact(t, "session setup and user preferences", body)
}

// assertManualStructureIntact bounds the override replacement region: the
// far-above heading, the user-preference slot, and the following
// `Scoped Exploration` section must all survive both seed and override renders.
// A mis-scoped marker that swallowed adjacent sections would fail here.
func assertManualStructureIntact(t *testing.T, label, body string) {
	t.Helper()
	for _, want := range []string{"WS Workflow Primitives", "### User preferences", "Scoped Exploration"} {
		if !strings.Contains(body, want) {
			t.Errorf("%s: manual structure must remain intact, missing %q:\n%s", label, want, body)
		}
	}
}

// ---------------------------------------------------------------------------
// Case 7: config.prompt.set end-to-end — setter drives real override through
// to render, matching the brief's integration-test contract.
// ---------------------------------------------------------------------------

// TestConfigPromptSetEndToEnd verifies the config.prompt.set tool dispatch path
// end-to-end against the shipped lead-workflow-manual:
//
//  1. Calling config.prompt.set with a lead session key writes the override
//     through the real layered config resolver.
//  2. Rendering lead-workflow-manual via buildOverrideLookup + printPlaybook
//     (the same path used by playbook.print dispatch) shows the stored override
//     in the shipped UserPreferenceSection slot.
//  3. Marker syntax is absent; manual structure is intact.
//  4. Harness-exact match vs all-bucket: a harness-specific override (claude)
//     wins over a previously stored all-bucket override when both are present.
func TestConfigPromptSetEndToEnd(t *testing.T) {
	useLeadProfile(t)

	// Use the real shipped rsrc tree so lead-workflow-manual loads with its real
	// UserPreferenceSection marker.
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)

	// Separate git repo as the session-bound worktree root.
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	// Set harness to "claude" so per-harness lookups resolve predictably.
	s.observeHarness("test", "claude")

	// Bootstrap a lead session key.
	key, _ := parseLoginResponse(t, callLogin(t, s, 910100, root, nil))

	const overrideText = "Use concise Korean status updates when reporting workflow state."

	// --- Baseline: without any override, custom preference is absent ---
	baseBody, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", buildOverrideLookup(s, key))
	if err != nil {
		t.Fatalf("printPlaybook (baseline): %v", err)
	}
	if strings.Contains(baseBody, overrideText) {
		t.Errorf("baseline: override text must not appear before any override is set:\n%s", baseBody)
	}

	// --- Set override via the real config.prompt.set dispatch ---
	// Scope: session (explicit) so the test exercises the session-scope write path
	// and does not touch the project-scope file on disk.
	setResp := callToolOnce(t, s, 1, "config.prompt.set", map[string]any{
		"session_key": key,
		"pointId":     "UserPreferenceSection",
		"harness":     "claude",
		"prompt":      overrideText,
		"scope":       "session",
	})
	setText := toolText(t, setResp)
	// Confirm the tool returned the expected confirmation line.
	if !strings.Contains(setText, "prompt override set: UserPreferenceSection/claude") {
		t.Fatalf("config.prompt.set confirmation missing: %s", setText)
	}
	if !strings.Contains(setText, "scope: session") {
		t.Fatalf("config.prompt.set must report session scope: %s", setText)
	}

	// --- Render with the override active ---
	overrideBody, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", buildOverrideLookup(s, key))
	if err != nil {
		t.Fatalf("printPlaybook (after set): %v", err)
	}

	// Override text must appear.
	if !strings.Contains(overrideBody, overrideText) {
		t.Errorf("config.prompt.set: stored override must appear in render:\n%s", overrideBody)
	}

	// Marker syntax must be absent; manual structure intact.
	assertNoMarkerSyntax(t, "config.prompt.set render", overrideBody)
	assertManualStructureIntact(t, "config.prompt.set render", overrideBody)

	// --- Harness-exact vs all-bucket precedence ---
	// Store an all-bucket override. Since a claude-specific override already exists,
	// the per-harness key must win and the all-bucket text must not appear.
	const allOverrideText = "All-harness override — should lose to claude-specific."
	allSetResp := callToolOnce(t, s, 2, "config.prompt.set", map[string]any{
		"session_key": key,
		"pointId":     "UserPreferenceSection",
		"harness":     "*",
		"prompt":      allOverrideText,
		"scope":       "session",
	})
	allSetText := toolText(t, allSetResp)
	// Confirm the harness normalization: "*" is stored as "all".
	if !strings.Contains(allSetText, "prompt override set: UserPreferenceSection/all") {
		t.Fatalf("config.prompt.set (all bucket) confirmation missing: %s", allSetText)
	}

	// Render again — claude-specific override must still win.
	precedenceBody, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", buildOverrideLookup(s, key))
	if err != nil {
		t.Fatalf("printPlaybook (precedence): %v", err)
	}
	if !strings.Contains(precedenceBody, overrideText) {
		t.Errorf("harness-specific (claude) override must win over all-bucket:\n%s", precedenceBody)
	}
	if strings.Contains(precedenceBody, allOverrideText) {
		t.Errorf("all-bucket text must not appear when harness-specific override is set:\n%s", precedenceBody)
	}
	assertNoMarkerSyntax(t, "config.prompt.set precedence render", precedenceBody)
	assertManualStructureIntact(t, "config.prompt.set precedence render", precedenceBody)
}

// TestConfigPromptUnsetSessionScope verifies ticket 260702-bug-config-unset-asymmetry:
// config.prompt.unset now supports scope: "session", removing only the
// session-scoped override and falling back to the next-broader scope (project
// here) rather than being forced through a global-scope detour or being
// unsupported entirely.
func TestConfigPromptUnsetSessionScope(t *testing.T) {
	useLeadProfile(t)

	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	s.observeHarness("test", "claude")
	key, _ := parseLoginResponse(t, callLogin(t, s, 910200, root, nil))

	// Seed a project-scope override beneath the session override so the
	// post-unset fallback lands somewhere concrete rather than the seed default.
	projectSetResp := callToolOnce(t, s, 1, "config.prompt.set", map[string]any{
		"session_key": key,
		"pointId":     "UserPreferenceSection",
		"harness":     "claude",
		"prompt":      "project-scope fallback text",
		"scope":       "project",
	})
	if !strings.Contains(toolText(t, projectSetResp), "scope: project") {
		t.Fatalf("project-scope seed set failed: %s", projectSetResp)
	}

	sessionSetResp := callToolOnce(t, s, 2, "config.prompt.set", map[string]any{
		"session_key": key,
		"pointId":     "UserPreferenceSection",
		"harness":     "claude",
		"prompt":      "session-scope override text",
		"scope":       "session",
	})
	if !strings.Contains(toolText(t, sessionSetResp), "scope: session") {
		t.Fatalf("session-scope set failed: %s", sessionSetResp)
	}

	// Session override wins while present.
	beforeUnset, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", buildOverrideLookup(s, key))
	if err != nil {
		t.Fatalf("printPlaybook (before unset): %v", err)
	}
	if !strings.Contains(beforeUnset, "session-scope override text") {
		t.Fatalf("session override must win before unset:\n%s", beforeUnset)
	}

	// Unset at session scope — must not require a global-scope detour and must
	// not clear the value to empty.
	unsetResp := callToolOnce(t, s, 3, "config.prompt.unset", map[string]any{
		"session_key": key,
		"pointId":     "UserPreferenceSection",
		"harness":     "claude",
		"scope":       "session",
	})
	unsetText := toolText(t, unsetResp)
	if strings.Contains(unsetText, `"isError":true`) {
		t.Fatalf("config.prompt.unset with scope=session must succeed: %s", unsetResp)
	}
	if !strings.Contains(unsetText, "prompt override cleared: UserPreferenceSection/claude (scope: session)") {
		t.Fatalf("config.prompt.unset confirmation missing: %s", unsetText)
	}

	// Falls back to the project-scope value, not to an empty override.
	afterUnset, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, isolatedPlaybookConfigOptions(t), "", buildOverrideLookup(s, key))
	if err != nil {
		t.Fatalf("printPlaybook (after unset): %v", err)
	}
	if strings.Contains(afterUnset, "session-scope override text") {
		t.Fatalf("session override must be gone after unset:\n%s", afterUnset)
	}
	if !strings.Contains(afterUnset, "project-scope fallback text") {
		t.Fatalf("unset must fall back to the next-broader (project) scope, not empty:\n%s", afterUnset)
	}
}

// TestConfigPromptSetValidationAndDefaultScope covers the setter's input-validation
// guards and the omitted-scope path (DefaultScope → project), which the happy-path
// end-to-end test does not exercise.
func TestConfigPromptSetValidationAndDefaultScope(t *testing.T) {
	useLeadProfile(t)

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	s := NewServer(root, "test")
	s.observeHarness("test", "claude")
	key, _ := parseLoginResponse(t, callLogin(t, s, 920100, root, nil))

	// --- Validation negatives: each must return an isError response with a clear message. ---
	negatives := []struct {
		label   string
		args    map[string]any
		wantMsg string
	}{
		{
			label:   "empty session_key",
			args:    map[string]any{"session_key": "", "pointId": "DelegationSection", "harness": "claude", "prompt": "x"},
			wantMsg: "session_key is required",
		},
		{
			label:   "empty pointId",
			args:    map[string]any{"session_key": key, "pointId": "", "harness": "claude", "prompt": "x"},
			wantMsg: "pointId must be non-empty",
		},
		{
			label:   "invalid harness",
			args:    map[string]any{"session_key": key, "pointId": "DelegationSection", "harness": "vscode", "prompt": "x"},
			wantMsg: "harness must be one of",
		},
		{
			label:   "empty prompt",
			args:    map[string]any{"session_key": key, "pointId": "DelegationSection", "harness": "claude", "prompt": "   "},
			wantMsg: "prompt must be non-empty",
		},
	}
	for i, tc := range negatives {
		resp := callToolOnce(t, s, 1000+i, "config.prompt.set", tc.args)
		if !strings.Contains(resp, `"isError":true`) {
			t.Errorf("%s: expected isError:true response, got: %s", tc.label, resp)
		}
		if msg := toolText(t, resp); !strings.Contains(msg, tc.wantMsg) {
			t.Errorf("%s: error message %q must contain %q", tc.label, msg, tc.wantMsg)
		}
	}

	// --- Default scope: omitting scope resolves to project for unregistered prompt.* keys. ---
	defResp := callToolOnce(t, s, 1100, "config.prompt.set", map[string]any{
		"session_key": key,
		"pointId":     "DelegationSection",
		"harness":     "codex",
		"prompt":      "default-scope override text",
	})
	if defText := toolText(t, defResp); !strings.Contains(defText, "scope: project") {
		t.Errorf("omitted scope must resolve to project, got: %s", defText)
	}
}

// TestConfigPromptListEnumeratesDeclaredPoints verifies the read-only
// config.prompt listing: it enumerates the two declared override-points in the
// test tree (SeedSection, ExtSlot) with their descs, annotates the one seeded
// override with its harness + session scope, shows the unset point as having no
// overrides, and ends with the ws:lead-tune pointer.
func TestConfigPromptListEnumeratesDeclaredPoints(t *testing.T) {
	useLeadProfile(t)

	rsrcRoot := buildOverrideTestTree(t)
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	s := NewServer(root, "test")
	s.observeHarness("test", "claude")

	key, _ := parseLoginResponse(t, callLogin(t, s, 900200, root, nil))

	// Seed one session-scope override so the listing has an annotated value.
	adapter := sessionConfigAdapter{s: s.sessions}
	resolver := wsconfig.NewResolver(wsconfig.Options{}, nil, adapter, adapter)
	if err := resolver.Set("prompt.SeedSection.claude", "seeded override value", wsconfig.SetOptions{
		ExplicitScope: wsconfig.ScopeSession,
		SessionKey:    key,
	}); err != nil {
		t.Fatalf("seed override via resolver: %v", err)
	}

	resp := callToolOnce(t, s, 1, "config.prompt", map[string]any{
		"session_key": key,
	})
	text := toolText(t, resp)

	wantSubstrings := []string{
		"SeedSection",
		"a seeded override point",
		"ExtSlot",
		"an empty extension slot",
		"harness=claude scope=session",
		"(no overrides set)",
		"ws:lead-tune",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(text, want) {
			t.Errorf("config.prompt listing missing %q:\n%s", want, text)
		}
	}

	// ExtSlot has no override seeded, so its block must carry "(no overrides set)";
	// SeedSection must carry the seeded annotation. Verify ordering is stable
	// (ExtSlot sorts before SeedSection).
	if idxExt, idxSeed := strings.Index(text, "ExtSlot"), strings.Index(text, "SeedSection"); idxExt < 0 || idxSeed < 0 || idxExt > idxSeed {
		t.Errorf("expected ExtSlot to sort before SeedSection in listing:\n%s", text)
	}
}

// TestConfigPromptListIncludesShippedUserPreferenceSection verifies that the
// listing discovers shipped override-point markers from the rsrc tree rather
// than from a curated schema.
func TestConfigPromptListIncludesShippedUserPreferenceSection(t *testing.T) {
	useLeadProfile(t)

	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900300, root, nil))

	resp := callToolOnce(t, s, 1, "config.prompt", map[string]any{
		"session_key": key,
	})
	text := toolText(t, resp)

	for _, want := range []string{
		"UserPreferenceSection",
		"user standing preferences for communication, terminology, and workflow behavior",
		"PreferSubagentInvocationGuidance",
		"harness-specific forked subagent invocation guidance",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("shipped config.prompt listing missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{
		"DelegationSection",
		"lead delegation eagerness and context-saving stance",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("shipped config.prompt listing must not expose removed %q:\n%s", forbidden, text)
		}
	}
	if strings.Contains(text, "PreferSubagent"+"CodexBinding") {
		t.Fatalf("shipped config.prompt listing must not expose Codex-specific point id:\n%s", text)
	}
}

func TestConfigTuningShippedPromptKnobsOmitDelegationSection(t *testing.T) {
	useLeadProfile(t)

	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900301, root, nil))

	resp := callToolOnce(t, s, 1, "config.tuning", map[string]any{
		"session_key": key,
		"format":      "json",
	})
	catalog := parseTuningCatalogResponse(t, resp)

	requireTuningKnob(t, catalog, "prompt.UserPreferenceSection")
	requireTuningKnob(t, catalog, "prompt.PreferSubagentInvocationGuidance")
	if knob := findTuningKnob(catalog, "prompt.DelegationSection"); knob != nil {
		t.Fatalf("config.tuning must not expose removed DelegationSection marker: %+v", *knob)
	}
}

func TestLeadPreferSubagentInvocationGuidanceUsesCodexBuiltinPromptOverride(t *testing.T) {
	useLeadProfile(t)

	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)

	sCodex := NewServer(root, "test")
	sCodex.observeHarness("test", "codex")
	codexPrintResp := callToolOnce(t, sCodex, 900309, "playbook.print", map[string]any{
		"name": "lead-prefer-subagent",
	})
	if codexPrintText := toolText(t, codexPrintResp); !strings.Contains(codexPrintText, "spawn_agent(fork_context:true, message:<prompt>)") {
		t.Fatalf("codex playbook.print without session_key must include builtin binding:\n%s", codexPrintText)
	}

	keyCodex, _ := parseLoginResponse(t, callLogin(t, sCodex, 900310, root, nil))

	codexBody, _, err := printPlaybook(sCodex, rsrcRoot, "lead-prefer-subagent", nil, wsconfig.Options{}, "", buildOverrideLookup(sCodex, keyCodex))
	if err != nil {
		t.Fatalf("printPlaybook codex: %v", err)
	}
	for _, want := range []string{
		"spawn_agent(fork_context:true, message:<prompt>)",
		"retry untyped with `fork_context:true`",
		"`agent_type: explorer`",
		"`agent_type: worker`",
	} {
		if !strings.Contains(codexBody, want) {
			t.Fatalf("codex render missing %q:\n%s", want, codexBody)
		}
	}
	assertNoMarkerSyntax(t, "lead-prefer-subagent codex", codexBody)

	sClaude := NewServer(root, "test")
	sClaude.observeHarness("test", "claude")
	claudePrintResp := callToolOnce(t, sClaude, 900312, "playbook.print", map[string]any{
		"name": "lead-prefer-subagent",
	})
	if claudePrintText := toolText(t, claudePrintResp); strings.Contains(claudePrintText, "spawn_agent(fork_context:true, message:<prompt>)") {
		t.Fatalf("claude playbook.print without session_key must not include codex binding:\n%s", claudePrintText)
	}

	keyClaude, _ := parseLoginResponse(t, callLogin(t, sClaude, 900311, root, nil))

	claudeBody, _, err := printPlaybook(sClaude, rsrcRoot, "lead-prefer-subagent", nil, wsconfig.Options{}, "", buildOverrideLookup(sClaude, keyClaude))
	if err != nil {
		t.Fatalf("printPlaybook claude: %v", err)
	}
	for _, forbidden := range []string{
		"spawn_agent(fork_context:true, message:<prompt>)",
		"`agent_type: explorer`",
		"`agent_type: worker`",
	} {
		if strings.Contains(claudeBody, forbidden) {
			t.Fatalf("claude render must not include codex binding %q:\n%s", forbidden, claudeBody)
		}
	}
	assertNoMarkerSyntax(t, "lead-prefer-subagent claude", claudeBody)
}

func TestConfigTuningCatalogProjectsPromptAndSchemaKnobs(t *testing.T) {
	useLeadProfile(t)

	rsrcRoot := buildOverrideTestTree(t)
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	s.observeHarness("test", "claude")

	key, _ := parseLoginResponse(t, callLogin(t, s, 900400, root, nil))

	adapter := sessionConfigAdapter{s: s.sessions}
	resolver := wsconfig.NewResolver(wsconfig.Options{}, nil, adapter, adapter)
	if err := resolver.Set("prompt.SeedSection.claude", "seeded override value", wsconfig.SetOptions{
		ExplicitScope: wsconfig.ScopeSession,
		SessionKey:    key,
	}); err != nil {
		t.Fatalf("seed override via resolver: %v", err)
	}

	resp := callToolOnce(t, s, 1, "config.tuning", map[string]any{
		"session_key": key,
		"format":      "json",
	})
	catalog := parseTuningCatalogResponse(t, resp)

	textResp := callToolOnce(t, s, 2, "config.tuning", map[string]any{
		"session_key": key,
	})
	text := toolText(t, textResp)
	for _, want := range []string{
		"prompt.SeedSection",
		"harness[claude|codex|*]",
		"scope[session|project|global]",
		`"scope":"session"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("config.tuning text missing %q:\n%s", want, text)
		}
	}

	promptKnob := requireTuningKnob(t, catalog, "prompt.SeedSection")
	if promptKnob.Writer.Tool != "config.prompt.set" || promptKnob.Writer.FixedArguments["pointId"] != "SeedSection" {
		t.Fatalf("prompt knob writer mismatch: %+v", promptKnob.Writer)
	}
	if promptKnob.Reset == nil || promptKnob.Reset.Tool != "config.prompt.unset" || promptKnob.Reset.FixedArguments["pointId"] != "SeedSection" {
		t.Fatalf("prompt knob reset mismatch: %+v", promptKnob.Reset)
	}
	assertFieldEnum(t, promptKnob.SelectorFields, "harness", []string{"claude", "codex", "*"})
	assertFieldEnum(t, promptKnob.SelectorFields, "scope", []string{"session", "project", "global"})
	assertFieldRequired(t, promptKnob.ValueFields, "prompt", true)
	if !strings.Contains(mustMarshalJSON(t, promptKnob.Current), `"scope":"session"`) {
		t.Fatalf("prompt knob current override missing session scope: %+v", promptKnob.Current)
	}

	subagentKnob := requireTuningKnob(t, catalog, "workflow.prefer_subagent")
	if subagentKnob.Writer.Tool != "config.workflow_prefer_subagent" {
		t.Fatalf("workflow.prefer_subagent writer tool mismatch: %+v", subagentKnob.Writer)
	}
	assertFieldEnum(t, subagentKnob.ValueFields, "value", []string{"on", "off"})
	if !strings.Contains(mustMarshalJSON(t, subagentKnob.Current), `"value":"off"`) {
		t.Fatalf("workflow.prefer_subagent default should be cataloged as off: %+v", subagentKnob.Current)
	}

	mercenaryKnob := requireTuningKnob(t, catalog, "workflow.prefer_mercenary")
	if mercenaryKnob.Writer.Tool != "config.workflow_prefer_mercenary" {
		t.Fatalf("workflow.prefer_mercenary writer tool mismatch: %+v", mercenaryKnob.Writer)
	}
	assertFieldEnum(t, mercenaryKnob.ValueFields, "value", []string{"on", "off", "hide"})
	if findTuningField(mercenaryKnob.ValueFields, "enabled") != nil {
		t.Fatalf("workflow.prefer_mercenary catalog must not expose legacy enabled field: %+v", mercenaryKnob.ValueFields)
	}
	if !strings.Contains(mustMarshalJSON(t, mercenaryKnob.Current), `"value":"hide"`) {
		t.Fatalf("workflow.prefer_mercenary default should be cataloged as hide: %+v", mercenaryKnob.Current)
	}

	agentsKnob := requireTuningKnob(t, catalog, "agents.tier")
	assertFieldEnum(t, agentsKnob.SelectorFields, "tier", []string{"small", "medium", "large", "xlarge"})
	assertFieldEnum(t, agentsKnob.ValueFields, "effort", []string{"", "none", "low", "medium", "high", "xhigh"})
}

func TestConfigTuningCatalogNoAgentOmitsFullWsKnobs(t *testing.T) {
	useLeadProfile(t)

	rsrcRoot := buildOverrideTestTree(t)
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)
	t.Setenv("WS_MCP_NO_AGENT", "1")

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900500, root, nil))

	resp := callToolOnce(t, s, 1, "config.tuning", map[string]any{
		"session_key": key,
		"format":      "json",
	})
	catalog := parseTuningCatalogResponse(t, resp)

	requireTuningKnob(t, catalog, "prompt.SeedSection")
	subagentKnob := requireTuningKnob(t, catalog, "workflow.prefer_subagent")
	if subagentKnob.Writer.Tool != "config.workflow_prefer_subagent" {
		t.Fatalf("workflow.prefer_subagent writer tool mismatch in no-agent catalog: %+v", subagentKnob.Writer)
	}
	for _, hidden := range []string{"workflow.prefer_mercenary", "delegation.prefer_mercenary", "agents.tier"} {
		if knob := findTuningKnob(catalog, hidden); knob != nil {
			t.Fatalf("no-agent config.tuning exposed full-ws-only knob %s: %+v", hidden, knob)
		}
	}
}

func parseTuningCatalogResponse(t *testing.T, line string) tuningCatalog {
	t.Helper()
	var catalog tuningCatalog
	if err := json.Unmarshal([]byte(toolText(t, line)), &catalog); err != nil {
		t.Fatalf("parse config.tuning JSON: %v", err)
	}
	return catalog
}

func requireTuningKnob(t *testing.T, catalog tuningCatalog, id string) tuningKnob {
	t.Helper()
	if knob := findTuningKnob(catalog, id); knob != nil {
		return *knob
	}
	t.Fatalf("config.tuning missing knob %q in %+v", id, catalog.Knobs)
	return tuningKnob{}
}

func findTuningKnob(catalog tuningCatalog, id string) *tuningKnob {
	for i := range catalog.Knobs {
		if catalog.Knobs[i].ID == id {
			return &catalog.Knobs[i]
		}
	}
	return nil
}

func assertFieldEnum(t *testing.T, fields []tuningField, name string, want []string) {
	t.Helper()
	field := findTuningField(fields, name)
	if field == nil {
		t.Fatalf("missing field %q in %+v", name, fields)
	}
	if strings.Join(field.Enum, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("field %q enum = %#v, want %#v", name, field.Enum, want)
	}
}

func assertFieldRequired(t *testing.T, fields []tuningField, name string, want bool) {
	t.Helper()
	field := findTuningField(fields, name)
	if field == nil {
		t.Fatalf("missing field %q in %+v", name, fields)
	}
	if field.Required != want {
		t.Fatalf("field %q required = %t, want %t", name, field.Required, want)
	}
}

func findTuningField(fields []tuningField, name string) *tuningField {
	for i := range fields {
		if fields[i].Name == name {
			return &fields[i]
		}
	}
	return nil
}

func mustMarshalJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	return string(raw)
}

// TestParseOverrideOpenMarkerDesc unit-tests the desc-aware open-marker parser
// across desc-present, desc-absent, and non-marker lines.
func TestParseOverrideOpenMarkerDesc(t *testing.T) {
	cases := []struct {
		name        string
		line        string
		wantPointId string
		wantDesc    string
		wantOK      bool
	}{
		{"desc present", `<!-- ws:override:Foo desc="a thing" -->`, "Foo", "a thing", true},
		{"desc absent", `<!-- ws:override:Bar -->`, "Bar", "", true},
		{"empty seed slot no space", `<!-- ws:override:Baz-->`, "Baz", "", true},
		{"non-marker line", `just some prose`, "", "", false},
		{"close marker is not an open marker", `<!-- ws:/override:Foo -->`, "", "", false},
		{"missing terminator", `<!-- ws:override:Foo desc="x"`, "", "", false},
		{"empty pointId", `<!-- ws:override: -->`, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pointId, desc, ok := parseOverrideOpenMarkerDesc(tc.line)
			if ok != tc.wantOK || pointId != tc.wantPointId || desc != tc.wantDesc {
				t.Errorf("parseOverrideOpenMarkerDesc(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tc.line, pointId, desc, ok, tc.wantPointId, tc.wantDesc, tc.wantOK)
			}
		})
	}
}

// TestScanOverridePoints unit-tests the tree scan against a small temp tree:
// desc present, desc absent, dedup across files (first non-empty desc wins), and
// stable sort by PointId.
func TestScanOverridePoints(t *testing.T) {
	root := t.TempDir()
	// File A declares Alpha (with desc) and Gamma (no desc).
	mustWrite(t, root, "a/a.md", `# A
<!-- ws:override:Alpha desc="alpha desc" -->
seed
<!-- ws:/override:Alpha -->
<!-- ws:override:Gamma -->
<!-- ws:/override:Gamma -->
`)
	// File B re-declares Alpha (different desc — must NOT win over A's first
	// non-empty desc) and declares Beta.
	mustWrite(t, root, "b/b.md", `# B
<!-- ws:override:Alpha desc="second alpha desc" -->
<!-- ws:/override:Alpha -->
<!-- ws:override:Beta desc="beta desc" -->
<!-- ws:/override:Beta -->
`)
	// A non-md file must be ignored even if it carries marker text.
	mustWrite(t, root, "c/notes.txt", `<!-- ws:override:Ignored desc="nope" -->`)

	points, err := scanOverridePoints(root)
	if err != nil {
		t.Fatalf("scanOverridePoints: %v", err)
	}

	want := []overridePointDecl{
		{PointId: "Alpha", Desc: "alpha desc"},
		{PointId: "Beta", Desc: "beta desc"},
		{PointId: "Gamma", Desc: ""},
	}
	if len(points) != len(want) {
		t.Fatalf("scanOverridePoints returned %d points, want %d: %+v", len(points), len(want), points)
	}
	for i, w := range want {
		if points[i] != w {
			t.Errorf("point[%d] = %+v, want %+v", i, points[i], w)
		}
	}
}
