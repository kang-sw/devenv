package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// The chat blocks' action pill rendered with the shipped CSS, in Chromium:
// invisible and inert at rest, shown on hover over its block and on
// keyboard focus without moving anything, placed on the bubble's
// bottom-right edge outside the bubble (whose paint a finished reply
// clips), its disabled actions' tooltips legible, and pinned with its
// confirmation while a copy confirms. WS_CHAT_ACTIONS_SHOTS, when set to a
// directory, also receives the design mockup's states as dark screenshots.
func TestChatBlockActionsBrowser(t *testing.T) {
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
	ring := []wslive.RingRecord{
		ringRecord(1, wslive.RingEntry{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard, Text: "원격 테스트 돌려보고 통과하면 '열린 질문을 카드 아래로 옮기기' develop에 푸시해 줘."}),
		ringRecord(2, wslive.RingEntry{Kind: wslive.RingReply, Text: "원격 테스트부터 돌리겠습니다."}),
		ringRecord(3, wslive.RingEntry{Kind: "tools", Tools: toolsSummary()}),
		ringRecord(4, wslive.RingEntry{Kind: wslive.RingReply, Text: "원격 테스트를 통과해서(Node가 필요한 2건만 예외) '열린 질문을 카드 아래로 옮기기'를 develop에 푸시했습니다 (`d69105645`). 지금 데몬을 교체하고 있고, 끝나면 확인을 요청드리겠습니다.\n\nWhiteboard: [http://127.0.0.1:29417/s/example-session](http://127.0.0.1:29417/s/example-session)\n\nActive ticket 261115-feat-ws-whiteboard-panel-first-stream has phases remaining on its retained impl branch — next cycle: wsflow:lead-run continues it."}),
		ringRecord(5, wslive.RingEntry{Kind: wslive.RingTurn, Origin: wslive.OriginTerminal, Done: 2, DurationMs: 16_000}),
		ringRecord(6, wslive.RingEntry{Kind: wslive.RingUser, Origin: wslive.OriginTerminal, Text: "확인했어. 이제 메모 패널 쪽 작업으로 넘어가자.\n목업부터 보여줘."}),
		ringRecord(7, wslive.RingEntry{Kind: wslive.RingReply, Text: "메모 패널 목업을 두 가지 배치로 만들겠습니다. 완료되면 경로와 스크린샷을 드리겠습니다."}),
		ringRecord(8, wslive.RingEntry{Kind: wslive.RingTurn, Origin: wslive.OriginTerminal, DurationMs: 4_000}),
	}
	view := historyView(ring, "", PageContent{}, sessionLookup{})
	view.Lang = "ko"
	set, err := renderSessionRegions(view, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc := `<!doctype html><html data-theme="THEME"><head><meta charset="utf-8"><style>` + string(d.WhiteboardCSS) + string(d.CSS) + `</style></head><body class="dash live">` +
		string(d.WhiteboardSprite) + string(d.Sprite) + string(d.LucideSprite) +
		`<div id="frame" style="width:760px">` + string(set[regionEntries].HTML) + `</div></body></html>`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, node, "testdata/chat-actions.browser.cjs", filepath.Join(dir, "page.html"), os.Getenv("WS_CHAT_ACTIONS_SHOTS")).CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("chat actions: %v", err)
	}
}
