package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Every conversation row kind rendered with the shipped CSS, in Chromium: the
// agent rows share the mail rows' frame (time column, chevron, one-line
// preview), whiteboard updates are never folded, and a compaction pill keeps
// 1em above and below while the other non-chat rows keep 0.2em. WS_CONVO_SHOTS, when set
// to a directory, also receives dark and light screenshots.
func TestConversationRowGrammarBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run isolated Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	assets, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	d := assets.data
	log := wswhiteboard.Effect{Kind: "log", Topic: "composer-ux", EntryID: "e7", Field: "text", After: "Mention memo result. The test failure was in the test tooling."}
	wb, ok := wslive.NewWhiteboardEffects([]wswhiteboard.Effect{log})
	if !ok {
		t.Fatal("effects")
	}
	ring := []wslive.RingRecord{
		ringRecord(1, wslive.RingEntry{Kind: "user", Origin: "terminal", Text: "please check the failing test"}),
		ringRecord(2, wslive.RingEntry{Kind: "reply", Text: "On it. Looking at the failure now."}),
		ringRecord(3, wslive.RingEntry{Kind: wslive.RingWake, Cause: "subagent_report", Source: "Diagnose selection [B] test failure", Text: "The failure is a cross-realm comparison.\nsecond line"}),
		ringRecord(4, wslive.RingEntry{Kind: wslive.RingWake, Cause: "subagent_done", Source: "Merge viewed re-fix head", Caption: "finished"}),
		ringRecord(5, wslive.RingEntry{Kind: wslive.RingWake, Cause: "background_exit", Source: "shell", Caption: "exited 0"}),
		ringRecord(6, wslive.RingEntry{Kind: wslive.RingMail, Direction: "sent", Peer: "EXEC 3", Title: "[B] result. My Claude agent confirmed the result", HistoryKey: "h1"}),
		ringRecord(7, wslive.RingEntry{Kind: wslive.RingMail, Direction: "recv", Peer: "EXEC 2", Title: "1074 answer with verified source coverage", HistoryKey: "h2"}),
		ringRecord(8, wslive.RingEntry{Kind: "tools", Tools: toolsSummary()}),
		ringRecord(9, wb),
		ringRecord(10, wslive.RingEntry{Kind: "notice", Notice: &wslive.Notice{Kind: "usage_limit", Text: "limit hit"}}),
		ringRecord(11, wslive.RingEntry{Kind: wslive.RingWake, Cause: "mail", Text: boardNudgeEnvelope + "Whiteboard lead nudge (unblocked).\n- One: ready to proceed; resolved targets: plan"}),
		// a compaction pill between tool and turn-footer rows, for the spacing check
		ringRecord(12, wslive.RingEntry{Kind: "tools", Tools: toolsSummary()}),
		ringRecord(13, wslive.RingEntry{Kind: wslive.RingTurn, Origin: "terminal", DurationMs: 39_000}),
		ringRecord(14, wslive.RingEntry{Kind: wslive.RingCompact, Trigger: wslive.TriggerManual}),
		ringRecord(15, wslive.RingEntry{Kind: wslive.RingWake, Cause: "subagent_report", Source: "Merge landing root-guide follow-up", Text: "Merged."}),
		ringRecord(16, wslive.RingEntry{Kind: wslive.RingTurn, Origin: "terminal", DurationMs: 0}),
	}
	page := PageContent{MailBodies: map[string]string{"h1": "body one", "h2": "body two"}}
	body := presentationHTML(t, ring, page, nil)
	doc := `<!doctype html><html data-theme="THEME"><head><meta charset="utf-8"><style>` + string(d.WhiteboardCSS) + string(d.CSS) + `</style></head><body class="dash live">` +
		string(d.WhiteboardSprite) + string(d.Sprite) + string(d.LucideSprite) +
		`<div class="conv" style="width:620px">` + body + `</div></body></html>`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, node, "testdata/conversation-row-grammar.browser.cjs", filepath.Join(dir, "page.html"), os.Getenv("WS_CONVO_SHOTS")).CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("row grammar: %v", err)
	}
}
