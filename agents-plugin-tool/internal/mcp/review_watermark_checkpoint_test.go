package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsreview"
)

// reviewNudgeTestRepo inits a repo on an explicit "main" branch (avoiding any
// dependency on the test runner's init.defaultBranch git config, since
// wsreview.ResolveTrack's pre-④ fallback needs a resolvable local main or
// master ref when no origin remote exists) with one seed commit.
func reviewNudgeTestRepo(t *testing.T, root string) {
	t.Helper()
	initGit(t, root)
	runGit(t, root, "checkout", "-b", "main")
	mustWrite(t, root, "README.md", "root\n")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-m", "root commit")
}

// reviewNudgeTestCommit adds one trivial commit to root's current branch and
// returns its full SHA.
func reviewNudgeTestCommit(t *testing.T, root, label string) string {
	t.Helper()
	mustWrite(t, root, label+".txt", label+"\n")
	runGit(t, root, "add", label+".txt")
	runGit(t, root, "commit", "-m", label)
	return strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
}

// reviewNudgeSeedStaleMarker stamps a pass entry at current HEAD, then adds
// aheadCommits more commits so the ledger marker is stale by that many
// commits over whatever branch is current at call time.
func reviewNudgeSeedStaleMarker(t *testing.T, root string, aheadCommits int) {
	t.Helper()
	head := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	if err := wsreview.Append(root, wsreview.Entry{Base: head, Head: head, Verdict: wsreview.VerdictPass}); err != nil {
		t.Fatalf("seed review ledger: %v", err)
	}
	for i := 0; i < aheadCommits; i++ {
		reviewNudgeTestCommit(t, root, fmt.Sprintf("c%d", i))
	}
}

// reviewNudgeRestamp appends a fresh pass entry at current HEAD, simulating a
// just-completed sweep that advances the marker to the current tip.
func reviewNudgeRestamp(t *testing.T, root string) {
	t.Helper()
	head := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	if err := wsreview.Append(root, wsreview.Entry{Base: head, Head: head, Verdict: wsreview.VerdictPass}); err != nil {
		t.Fatalf("restamp review ledger: %v", err)
	}
}

// TestServeStdioTicketsCloseReviewWatermarkNudgeSurfacesAndQuiets covers the
// first of the four cheap-checkpoint call sites: tickets.close must carry a
// "review-watermark:" advisory when the track (here, local "main" fallback)
// is stale past the ledger marker by more than the size threshold, and must
// stay silent right after the ledger is restamped to the current tip.
func TestServeStdioTicketsCloseReviewWatermarkNudgeSurfacesAndQuiets(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	reviewNudgeTestRepo(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	reviewNudgeSeedStaleMarker(t, root, wsreview.SizeThresholdCommits+2)

	stemStale := "260101-feat-review-watermark-close-nudge-stale"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stemStale+".md"), "---\ntitle: Stale\n---\n\nBody.\n")
	stemFresh := "260101-feat-review-watermark-close-nudge-fresh"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stemFresh+".md"), "---\ntitle: Fresh\n---\n\nBody.\n")

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	staleResp := callToolWithKey(t, server, 2, key, "tickets.close", map[string]any{"stem": stemStale, "status": "done"})
	if !strings.Contains(staleResp, "review-watermark:") {
		t.Fatalf("tickets.close should carry the review-watermark nudge for a stale range: %s", staleResp)
	}

	reviewNudgeRestamp(t, root)

	freshResp := callToolWithKey(t, server, 3, key, "tickets.close", map[string]any{"stem": stemFresh, "status": "done"})
	if strings.Contains(freshResp, "review-watermark:") {
		t.Fatalf("tickets.close should stay silent right after the ledger was restamped: %s", freshResp)
	}
}

// TestServeStdioWorkflowManualReviewWatermarkNudgeSurfacesAndQuiets covers the
// second call site (session start): both the FRESH-with-root and CONTINUE
// branches of workflow_manual must carry the nudge on a stale range and go
// quiet once restamped.
func TestServeStdioWorkflowManualReviewWatermarkNudgeSurfacesAndQuiets(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	reviewNudgeTestRepo(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))

	reviewNudgeSeedStaleMarker(t, root, wsreview.SizeThresholdCommits+2)

	s := NewServer(root, "test")

	// workflow_manual injects the raw CheckpointNudge text directly (no
	// "review-watermark:" label prefix, unlike the other three call sites),
	// mirroring the scopeAnnouncement/computeManuals injection shape.
	const nudgeMarker = "review watermark is"

	freshResp := callToolWithKey(t, s, 1, freshBootstrapKey, "workflow_manual", map[string]any{"root": root})
	if !strings.Contains(freshResp, nudgeMarker) {
		t.Fatalf("workflow_manual FRESH-with-root should carry the review-watermark nudge for a stale range: %s", freshResp)
	}

	key, _ := parseLoginResponse(t, callLogin(t, s, 2, root, nil))
	continueRespStale := callToolWithKey(t, s, 3, key, "workflow_manual", nil)
	if !strings.Contains(continueRespStale, nudgeMarker) {
		t.Fatalf("workflow_manual CONTINUE should carry the review-watermark nudge for a stale range: %s", continueRespStale)
	}

	reviewNudgeRestamp(t, root)

	continueRespFresh := callToolWithKey(t, s, 4, key, "workflow_manual", nil)
	if strings.Contains(continueRespFresh, nudgeMarker) {
		t.Fatalf("workflow_manual CONTINUE should stay silent right after the ledger was restamped: %s", continueRespFresh)
	}
}

// TestServeStdioEnterImplementNewSchemaReviewWatermarkNudgeSurfacesAndQuiets
// covers the third call site's hasNewTarget branch (handleEnterImplement).
// The extra commits land on the track branch ("main") before the session's
// working branch is cut from its tip, mirroring
// TestEnterImplementNewSchemaReturnsVerdictAndStoresAgenda's branch setup.
func TestServeStdioEnterImplementNewSchemaReviewWatermarkNudgeSurfacesAndQuiets(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	reviewNudgeTestRepo(t, root)
	reviewNudgeSeedStaleMarker(t, root, wsreview.SizeThresholdCommits+2)
	runGit(t, root, "switch", "-c", "feature/base")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	staleText := callToolWithKey(t, server, 2, key, "enter.implement", implementReadyArgs("text"))
	if !strings.Contains(staleText, "review-watermark:") {
		t.Fatalf("enter.implement (new schema) should carry the review-watermark nudge for a stale range: %s", staleText)
	}

	runGit(t, root, "checkout", "main")
	reviewNudgeRestamp(t, root)
	runGit(t, root, "checkout", "feature/base")

	freshText := callToolWithKey(t, server, 3, key, "enter.implement", implementReadyArgs("text"))
	if strings.Contains(freshText, "review-watermark:") {
		t.Fatalf("enter.implement (new schema) should stay silent right after the ledger was restamped: %s", freshText)
	}
}

// TestServeStdioEnterImplementLegacyReviewWatermarkNudgeSurfacesAndQuiets
// covers the third call site's legacy branch (handleEnter, sole caller is
// enter.implement without a "target" argument).
func TestServeStdioEnterImplementLegacyReviewWatermarkNudgeSurfacesAndQuiets(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	reviewNudgeTestRepo(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	reviewNudgeSeedStaleMarker(t, root, wsreview.SizeThresholdCommits+2)

	staleResp := callToolWithKey(t, server, 2, key, "enter.implement", map[string]any{
		"delegation": "delegated", "need_review": true, "need_doc": false,
	})
	if !strings.Contains(staleResp, "review-watermark:") {
		t.Fatalf("legacy enter.implement should carry the review-watermark nudge for a stale range: %s", staleResp)
	}

	reviewNudgeRestamp(t, root)

	freshResp := callToolWithKey(t, server, 3, key, "enter.implement", map[string]any{
		"delegation": "delegated", "need_review": true, "need_doc": false,
	})
	if strings.Contains(freshResp, "review-watermark:") {
		t.Fatalf("legacy enter.implement should stay silent right after the ledger was restamped: %s", freshResp)
	}
}

// TestServeStdioEnterProceedReviewWatermarkNudgeSurfacesAndQuiets covers the
// fourth call site (handleEnterProceed).
func TestServeStdioEnterProceedReviewWatermarkNudgeSurfacesAndQuiets(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	reviewNudgeTestRepo(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	reviewNudgeSeedStaleMarker(t, root, wsreview.SizeThresholdCommits+2)

	staleText := callToolWithKey(t, server, 2, key, "enter.proceed", proceedReadyArgs("text"))
	if !strings.Contains(staleText, "review-watermark:") {
		t.Fatalf("enter.proceed should carry the review-watermark nudge for a stale range: %s", staleText)
	}

	reviewNudgeRestamp(t, root)

	freshText := callToolWithKey(t, server, 3, key, "enter.proceed", proceedReadyArgs("text"))
	if strings.Contains(freshText, "review-watermark:") {
		t.Fatalf("enter.proceed should stay silent right after the ledger was restamped: %s", freshText)
	}
}

// TestServeStdioWorkflowManualReviewWatermarkNudgeFailsOpenOnUnresolvableTrack
// proves the fail-open contract: when no origin/HEAD and neither main nor
// master exists (here compounded with a detached HEAD), CheckpointNudge must
// never error or block the underlying tool call — it silently omits the
// nudge line.
func TestServeStdioWorkflowManualReviewWatermarkNudgeFailsOpenOnUnresolvableTrack(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	runGit(t, root, "checkout", "-b", "topic")
	mustWrite(t, root, "README.md", "root\n")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-m", "root commit")
	head := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	runGit(t, root, "checkout", "--detach", head)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))

	s := NewServer(root, "test")
	freshResp := callToolWithKey(t, s, 1, freshBootstrapKey, "workflow_manual", map[string]any{"root": root})
	if strings.Contains(freshResp, "review watermark is") {
		t.Fatalf("workflow_manual should stay silent when the review track is unresolvable: %s", freshResp)
	}
	if !strings.Contains(freshResp, "Session Key") {
		t.Fatalf("workflow_manual FRESH-with-root should still succeed (no error/block) when track resolution fails: %s", freshResp)
	}
}

// TestServeStdioTicketsCloseReviewWatermarkNudgeToleratesMalformedLocalConfig
// proves the fail-open contract on the config-parsing side: a malformed
// ai-docs/_review.local.md staleness value must never error the call —
// StalenessKnob falls back to its default, and the nudge still renders.
func TestServeStdioTicketsCloseReviewWatermarkNudgeToleratesMalformedLocalConfig(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	reviewNudgeTestRepo(t, root)
	mustWrite(t, root, filepath.Join("ai-docs", "_review.local.md"), "## Checkpoint Nudge\nstaleness: not-a-number commits\n")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	reviewNudgeSeedStaleMarker(t, root, wsreview.SizeThresholdCommits+2)

	stem := "260101-feat-review-watermark-close-nudge-malformed-config"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", stem+".md"), "---\ntitle: Malformed config\n---\n\nBody.\n")

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	resp := callToolWithKey(t, server, 2, key, "tickets.close", map[string]any{"stem": stem, "status": "done"})
	if !strings.Contains(resp, "review-watermark:") {
		t.Fatalf("tickets.close should still surface the nudge (falling back to the default staleness) despite a malformed config: %s", resp)
	}
}

// TestServeStdioReviewMarkerReadsAbsentAndPresentEntry covers the new
// review.marker tool's plain (non-bootstrap) read path.
func TestServeStdioReviewMarkerReadsAbsentAndPresentEntry(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	reviewNudgeTestRepo(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	absent := callToolWithKey(t, server, 2, key, "review.marker", nil)
	if !strings.Contains(absent, "no entry yet") {
		t.Fatalf("review.marker on an empty ledger should report no entry yet: %s", absent)
	}

	head := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	if err := wsreview.Append(root, wsreview.Entry{Base: head, Head: head, Verdict: wsreview.VerdictPass}); err != nil {
		t.Fatalf("seed ledger: %v", err)
	}
	present := callToolWithKey(t, server, 3, key, "review.marker", nil)
	if !strings.Contains(present, head) || !strings.Contains(present, "pass") {
		t.Fatalf("review.marker should surface the latest entry: %s", present)
	}
}

// TestServeStdioReviewMarkerBootstrapCreatesExactlyOneEntryAndIsIdempotent
// covers review.marker(bootstrap: true): the sole caller-opted-in bootstrap
// trigger, which must be a no-op on a second call.
func TestServeStdioReviewMarkerBootstrapCreatesExactlyOneEntryAndIsIdempotent(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	reviewNudgeTestRepo(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	first := callToolWithKey(t, server, 2, key, "review.marker", map[string]any{"bootstrap": true})
	if !strings.Contains(first, "bootstrapped") {
		t.Fatalf("first review.marker(bootstrap: true) should report a bootstrap: %s", first)
	}

	second := callToolWithKey(t, server, 3, key, "review.marker", map[string]any{"bootstrap": true})
	if strings.Contains(second, "bootstrapped") {
		t.Fatalf("second review.marker(bootstrap: true) must be idempotent (no-op), got: %s", second)
	}

	raw, err := os.ReadFile(wsreview.LedgerPath(root))
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	// Count only entry lines: the Phase-3 banner prepends `#`-prefixed
	// comment lines at first creation, which the parser (and this idempotency
	// assertion) must skip. The invariant is exactly one *entry*, not one raw
	// file line.
	var entries []string
	for _, ln := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		entries = append(entries, ln)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one ledger entry after repeated bootstrap, got %d: %v", len(entries), entries)
	}
}

// TestServeStdioReviewStampRoundTripsThroughMarker covers review.stamp
// appending an entry that review.marker then reads back.
func TestServeStdioReviewStampRoundTripsThroughMarker(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	reviewNudgeTestRepo(t, root)
	base := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	head := reviewNudgeTestCommit(t, root, "extra")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	stampResp := callToolWithKey(t, server, 2, key, "review.stamp", map[string]any{
		"base": base, "head": head, "verdict": "pass",
	})
	if !strings.Contains(stampResp, "stamped") {
		t.Fatalf("review.stamp should confirm the append: %s", stampResp)
	}

	marker := callToolWithKey(t, server, 3, key, "review.marker", nil)
	if !strings.Contains(marker, base) || !strings.Contains(marker, head) || !strings.Contains(marker, "pass") {
		t.Fatalf("review.marker should round-trip the stamped entry: %s", marker)
	}
}

// TestServeStdioReviewMarkerReportsFrontierNotRawTailAfterBlock pins the
// frontier switch at the MCP boundary: after a pass-then-block sequence,
// review.marker (bare read) must still report the pass entry — the
// frontier — not the trailing block entry.
func TestServeStdioReviewMarkerReportsFrontierNotRawTailAfterBlock(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	reviewNudgeTestRepo(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	passBase := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	passHead := reviewNudgeTestCommit(t, root, "pass-head")
	if err := wsreview.Append(root, wsreview.Entry{Base: passBase, Head: passHead, Verdict: wsreview.VerdictPass}); err != nil {
		t.Fatalf("seed pass entry: %v", err)
	}
	blockHead := reviewNudgeTestCommit(t, root, "block-head")
	if err := wsreview.Append(root, wsreview.Entry{Base: passHead, Head: blockHead, Verdict: wsreview.VerdictBlock, Ref: "260901-bug-example"}); err != nil {
		t.Fatalf("seed trailing block entry: %v", err)
	}

	resp := callToolWithKey(t, server, 2, key, "review.marker", nil)
	if !strings.Contains(resp, passHead) || !strings.Contains(resp, "pass") {
		t.Fatalf("review.marker should report the frontier (pass) entry, not the trailing block: %s", resp)
	}
	if strings.Contains(resp, blockHead) {
		t.Fatalf("review.marker should not report the trailing block entry's head: %s", resp)
	}
}

// TestServeStdioReviewMarkerFormatJSONReturnsStructuredFrontier covers the
// new format=json output: structured base/head/verdict fields, sourced from
// the frontier so a caller (the lead-ship gate) can consume the head without
// string-scraping.
func TestServeStdioReviewMarkerFormatJSONReturnsStructuredFrontier(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	reviewNudgeTestRepo(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	base := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	head := reviewNudgeTestCommit(t, root, "extra")
	if err := wsreview.Append(root, wsreview.Entry{Base: base, Head: head, Verdict: wsreview.VerdictPass}); err != nil {
		t.Fatalf("seed pass entry: %v", err)
	}

	resp := callToolWithKey(t, server, 2, key, "review.marker", map[string]any{"format": "json"})
	var got struct {
		Base    string `json:"base"`
		Head    string `json:"head"`
		Verdict string `json:"verdict"`
		Ref     string `json:"ref"`
		Found   bool   `json:"found"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp)), &got); err != nil {
		t.Fatalf("review.marker(format: json) did not return valid JSON: %v\nresponse: %s", err, resp)
	}
	if got.Base != base || got.Head != head || got.Verdict != wsreview.VerdictPass || !got.Found {
		t.Fatalf("review.marker(format: json) = %+v, want base=%q head=%q verdict=pass found=true", got, base, head)
	}
}

// TestServeStdioReviewStampRejectsBlockVerdictWithoutRef mirrors Phase 1's
// Append test coverage shape (ledger_test.go's TestAppendBlockWithEmptyRefFails)
// at the MCP tool boundary: review.stamp must surface Append's own validation
// error as the tool error, not swallow it.
func TestServeStdioReviewStampRejectsBlockVerdictWithoutRef(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	reviewNudgeTestRepo(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))

	head := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	resp := callToolOnce(t, server, 2, "review.stamp", map[string]any{
		"session_key": key, "base": head, "head": head, "verdict": "block",
	})
	if !toolIsError(t, resp) {
		t.Fatalf("review.stamp with verdict=block and no ref should error: %s", resp)
	}
	if !strings.Contains(toolText(t, resp), "requires a non-empty Ref") {
		t.Fatalf("review.stamp block-without-ref error message unexpected: %s", toolText(t, resp))
	}
}
