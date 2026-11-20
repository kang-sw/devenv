package wswhiteboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// This fixture composes the production regions and assets without a daemon or
// shared store. WS_TOPIC_STATE_SHOTS keeps the acceptance captures.
func TestTopicStateCompactBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	now := time.Now().UTC()
	st := &State{}
	for _, row := range []struct {
		name, label, state, attrs string
		elapsed                   int
	}{
		{"impl", "리뷰 스윕 — 누적 커밋 재검토", StateImplementing, `owner="pi 작업자 5" eta_minutes="60"`, 5},
		{"over", "보드 카드 접기 상태 유지", StateImplementing, `owner="codex 작업자 2" eta_minutes="45"`, 72},
		{"extended", "긴 작업의 ETA 연장", StateImplementing, `owner="작업자 3" eta_minutes="120"`, 135},
		{"research", "폴링 비용과 이벤트 전달 조사", StateResearching, `owner="sonnet 조사자" eta_minutes="30"`, 0},
		{"discuss", "다음 작업 범위 논의", StateDiscussing, "", 0},
		{"wait", "선행 작업 완료 대기", StateWaiting, "", 0},
		{"confirm", "구현 결과 확인", StateAwaitingConfirmation, "", 0},
		{"narrow", "리뷰 스윕 — 수리 5건 병렬 진행과 재검토 대기열 정리, 긴 제목은 먼저 말줄임", StateImplementing, `owner="pi 작업자 5" eta_minutes="60"`, 5},
	} {
		at := now.Add(-time.Duration(row.elapsed) * time.Minute)
		applyAt(t, st, fmt.Sprintf(`<wb:topic name="%s" label="%s"><wb:log>컴팩트 상태 뱃지 검증</wb:log><wb:state value="%s" %s/></wb:topic>`, row.name, row.label, row.state, row.attrs), at)
	}
	applyAt(t, st, `<wb:topic name="over"><wb:eta minutes="60" reason="추가 검증"/></wb:topic>`, now)
	applyAt(t, st, `<wb:topic name="extended"><wb:eta minutes="150" reason="추가 검증"/><wb:eta minutes="180" reason="최종 확인"/></wb:topic>`, now)
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "state-fixture", Location: time.UTC})
	view.Lang = "ko"
	// Exercise the compact bar with the same dependency and ticket siblings as
	// the mockup, independently of external resolvers.
	for i := range view.Topics {
		if view.Topics[i].Name == "narrow" {
			view.Topics[i].Quiet = false
			view.Topics[i].Dependencies = []DependencyView{{Token: "exec:fixture", Label: "선행 작업"}}
			view.Topics[i].Tickets = []TicketBadge{{Stem: "260101-feat-example", Status: TicketReady}}
		}
	}
	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	assets, err := PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	page := `<!DOCTYPE html><html><head><style>` + string(assets.CSS) + `</style></head><body>` + string(assets.Sprite) + `<main>` + string(regions.Board) + `</main><aside>` + string(regions.Panels) + `</aside><script>` + string(wsi18n.Script(view.Lang)) + `</script><script>` + string(assets.AnswersScript) + `</script><script>(function(){` + string(assets.FormScript) + `})();</script></body></html>`
	dir := t.TempDir()
	file := filepath.Join(dir, "board.html")
	if err := os.WriteFile(file, []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	shots := os.Getenv("WS_TOPIC_STATE_SHOTS")
	if shots == "" {
		shots = dir
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/topic-state-compact.browser.cjs", file, shots, fmt.Sprint(now.UnixMilli()))
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("compact state browser: %v", err)
	}
}
