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
//   6. Phase 2 shipped seed: the DelegationSection marker in the real
//      lead-workflow-manual renders its seed posture (markers stripped) and an
//      override replaces only that posture, leaving manual structure intact.
//
// Unit-level cases (1–4) drive renderPlaybookBody with an injected fake
// overrideLookupFn.  Case 5 uses the production dispatch (callToolOnce on
// playbook.render) so the server-side resolver wiring is exercised end-to-end.

import (
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
	body, _, err := renderPlaybookBody(s, rsrcRoot, "override-pb", nil, wsconfig.Options{}, "", "", false, nil)
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
	bodyClaude, _, err := renderPlaybookBody(sClaude, rsrcRoot, "override-pb", nil, wsconfig.Options{}, "", "", false, lookup)
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
	bodyCodex, _, err := renderPlaybookBody(sCodex, rsrcRoot, "override-pb", nil, wsconfig.Options{}, "", "", false, lookup)
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
	bodyClaude, _, err := renderPlaybookBody(sClaude, rsrcRoot, "override-pb", nil, wsconfig.Options{}, "", "", false, lookup)
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
	bodyCodex, _, err := renderPlaybookBody(sCodex, rsrcRoot, "override-pb", nil, wsconfig.Options{}, "", "", false, lookup)
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
	bodyBoth, _, err := renderPlaybookBody(sClaude, rsrcRoot, "override-pb", nil, wsconfig.Options{}, "", "", false, lookupBoth)
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
	bodyNoOverride, _, err := renderPlaybookBody(s, rsrcRoot, "override-pb", nil, wsconfig.Options{}, "", "", false, nil)
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
	bodyWithOverride, _, err := renderPlaybookBody(s, rsrcRoot, "override-pb", nil, wsconfig.Options{}, "", "", false, lookup)
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
// Case 6: Phase 2 — shipped DelegationSection seed in lead-workflow-manual
// ---------------------------------------------------------------------------

// TestShippedDelegationSectionSeedAndOverride verifies the first shipped override
// marker. With no override the seed posture renders (markers stripped); an
// override on DelegationSection replaces only the posture body and leaves the
// rest of the manual intact. It runs against the real shipped rsrc tree, so it
// also guards the manifest regen for the edited manual.
func TestShippedDelegationSectionSeedAndOverride(t *testing.T) {
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "claude")

	const seedPhrase = "Delegate to preserve lead execution context"

	// No override → seed posture renders, markers stripped, structure intact.
	seedBody, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, wsconfig.Options{}, nil)
	if err != nil {
		t.Fatalf("printPlaybook (seed): %v", err)
	}
	if !strings.Contains(seedBody, seedPhrase) {
		t.Errorf("seed posture must render with no override:\n%s", seedBody)
	}
	assertNoMarkerSyntax(t, "shipped seed", seedBody)
	assertManualStructureIntact(t, "shipped seed", seedBody)

	// Override DelegationSection (all bucket) → posture replaced, seed gone,
	// structure intact, markers stripped.
	lookup := staticLookup(map[string]string{
		"DelegationSection/all": "ALWAYS delegate aggressively to conserve context.",
	})
	ovBody, _, err := printPlaybook(s, rsrcRoot, "lead-workflow-manual", nil, wsconfig.Options{}, lookup)
	if err != nil {
		t.Fatalf("printPlaybook (override): %v", err)
	}
	if !strings.Contains(ovBody, "ALWAYS delegate aggressively to conserve context.") {
		t.Errorf("override posture must replace the seed:\n%s", ovBody)
	}
	if strings.Contains(ovBody, seedPhrase) {
		t.Errorf("seed posture must not appear when overridden:\n%s", ovBody)
	}
	assertNoMarkerSyntax(t, "shipped override", ovBody)
	assertManualStructureIntact(t, "shipped override", ovBody)
}

// assertManualStructureIntact bounds the override replacement region: the
// far-above heading, the `### Delegation posture` heading (immediately above the
// override block, outside it), and the following `Scoped Exploration` section
// must all survive both seed and override renders. A mis-scoped marker that
// swallowed the heading or the next section would fail here.
func assertManualStructureIntact(t *testing.T, label, body string) {
	t.Helper()
	for _, want := range []string{"WS Workflow Primitives", "### Delegation posture", "Scoped Exploration"} {
		if !strings.Contains(body, want) {
			t.Errorf("%s: manual structure must remain intact, missing %q:\n%s", label, want, body)
		}
	}
}
