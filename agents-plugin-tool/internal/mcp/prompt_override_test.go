package mcp

// prompt_override_test.go — integration tests for the override-marker engine
// (Phase 1 of 260619-feat-ws-prompt-override-marker-engine).
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

	// Static structural content must remain intact.
	if !strings.Contains(body, "Before section.") {
		t.Errorf("content before marker block must be preserved:\n%s", body)
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
	adapter := sessionConfigAdapter{s: s.sessions}
	resolver := wsconfig.NewResolver(wsconfig.Options{}, nil, adapter, adapter)
	if err := resolver.Set("prompt.SeedSection.claude", "production-path override", wsconfig.SetOptions{
		SessionKey: key,
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
