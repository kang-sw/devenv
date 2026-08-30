package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

// reviewTrackNudgeMarker is the distinctive prefix of reviewTrackNudge's
// advisory text (review_track_alarm.go).
const reviewTrackNudgeMarker = "Review-track branch is not configured"

// extractMintedSessionKey pulls the minted lead session key out of a
// workflow_manual FRESH-with-root response's "## Session Key" section,
// mirroring TestWorkflowManualFreshModeWithRoot's extraction shape
// (session_state_test.go).
func extractMintedSessionKey(t *testing.T, resp string) string {
	t.Helper()
	inSection := false
	for _, line := range strings.Split(resp, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Session Key" {
			inSection = true
			continue
		}
		if inSection {
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "#") {
				break
			}
			return trimmed
		}
	}
	t.Fatalf("could not extract minted key from response:\n%s", resp)
	return ""
}

// TestServeStdioWorkflowManualReviewTrackNudgeFiresOnceThenQuiets covers the
// once-per-session shape of the new review-track config nudge: an unset
// AGENTS.md `review-track` field fires the nudge on the first
// workflow_manual call (FRESH-with-root) and must not repeat on a second
// CONTINUE call using the same minted session key. It also proves the nudge
// never blocks the underlying call: the FRESH response still carries a
// "## Session Key" section alongside the nudge.
func TestServeStdioWorkflowManualReviewTrackNudgeFiresOnceThenQuiets(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	reviewNudgeTestRepo(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))

	s := NewServer(root, "test")

	freshResp := callToolWithKey(t, s, 1, freshBootstrapKey, "workflow_manual", map[string]any{"root": root})
	if !strings.Contains(freshResp, reviewTrackNudgeMarker) {
		t.Fatalf("workflow_manual FRESH-with-root should carry the review-track nudge when review-track is unset: %s", freshResp)
	}
	if !strings.Contains(freshResp, "## Session Key") {
		t.Fatalf("workflow_manual FRESH-with-root should still succeed (no error/block) alongside the nudge: %s", freshResp)
	}
	key := extractMintedSessionKey(t, freshResp)

	continueResp := callToolWithKey(t, s, 2, key, "workflow_manual", nil)
	if strings.Contains(continueResp, reviewTrackNudgeMarker) {
		t.Fatalf("workflow_manual CONTINUE should not repeat the review-track nudge in the same session: %s", continueResp)
	}
	if !strings.Contains(continueResp, "## Session Key") {
		t.Fatalf("workflow_manual CONTINUE should still succeed (no error/block): %s", continueResp)
	}
}

// TestServeStdioWorkflowManualReviewTrackNudgeSilentWhenConfigured proves the
// nudge's silent case: once AGENTS.md declares a non-empty `review-track`
// field, neither the FRESH-with-root call nor a subsequent CONTINUE call
// surfaces the advisory.
func TestServeStdioWorkflowManualReviewTrackNudgeSilentWhenConfigured(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	reviewNudgeTestRepo(t, root)
	mustWrite(t, root, "AGENTS.md", "# AGENTS.md\n\n## Workflow\n\n### Review Policy\nreview-track: main\nrelease-boundary: absent\nrendezvous-backend: canary\n")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))

	s := NewServer(root, "test")

	freshResp := callToolWithKey(t, s, 1, freshBootstrapKey, "workflow_manual", map[string]any{"root": root})
	if strings.Contains(freshResp, reviewTrackNudgeMarker) {
		t.Fatalf("workflow_manual FRESH-with-root should stay silent when review-track is configured: %s", freshResp)
	}
	key := extractMintedSessionKey(t, freshResp)

	continueResp := callToolWithKey(t, s, 2, key, "workflow_manual", nil)
	if strings.Contains(continueResp, reviewTrackNudgeMarker) {
		t.Fatalf("workflow_manual CONTINUE should stay silent when review-track is configured: %s", continueResp)
	}
}
