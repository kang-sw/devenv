package wswhiteboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

func panelHeaderRowsState(t *testing.T) *State {
	t.Helper()
	st := &State{}
	day := time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)
	for _, step := range []struct {
		minute   int
		commands string
	}{
		{0, `<wb:topic name="wb-panel-rows" label="화이트보드 패널 헤더 2행 레이아웃"><wb:log>초안 구조 설계 완료</wb:log></wb:topic>`},
		{10, panelCommandXML("wb-panel-rows", `name="mockup-v2" recap="p55 · 인터랙티브 HTML 목업 v1 초안 (agy.medium) — 1차 시안 및 구조 검토"`, "### 1차 시안\n\n- 기본 구조 정리\n- 반응형 그리드 초안\n")},
		{25, logXML("wb-panel-rows", "사용자가 타이틀 우선 2행 레이아웃(Option A) 확정")},
		{35, panelCommandXML("wb-panel-rows", `name="mockup-v2" recap="p55 · 인터랙티브 HTML 목업 v2 완성 (agy.medium) — 타이틀 우선 2행 헤더 및 메타데이터 바 분리"`, "### 구현 완료\n\n- Row 1: 전체 너비 패널 타이틀 + 우측 접기/펼치기 셰브론\n- Row 2: 패널 ID · 타임스탬프 · 버전 · 액션 바\n")},
		{42, panelCommandXML("wb-panel-rows", `name="collapsed-long" recap="p56 · 접힌 상태 긴 한국어 패널 제목 테스트 — 좁은 도킹 창(420px)에서도 제목이 3~4줄로 찌그러지지 않고 자연스럽게 2줄 이내로 줄바꿈"`, "### 접힌 패널 본문\n\n- 확인용 본문\n")},
		{48, panelCommandXML("wb-panel-rows", `recap="p57 · 본문 없는 리캡 전용 패널(.msg) — 2행 메타데이터 바 정렬 확인"`, "")},
	} {
		env := testEnv()
		env.Now = day.Add(time.Duration(step.minute) * time.Minute)
		if _, problems := Apply(st, step.commands, env); len(problems) > 0 {
			t.Fatalf("Apply(%s): %v", step.commands, problems)
		}
	}
	return st
}

// TestPanelHeaderRowsBrowser verifies the Option A title-first 2-row panel
// header layout in Chromium across wide (1280px) and narrow (360–420px) widths in
// light and dark themes, including dashboard docked-pane styling.
func TestPanelHeaderRowsBrowser(t *testing.T) {
	view := BuildView(panelHeaderRowsState(t), ViewInput{Layout: LayoutFull, ReplyID: "panel-header-rows", Location: time.UTC})
	view.Lang = "ko"
	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	assets, err := PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	dashBoardCSS, err := os.ReadFile(filepath.Join("..", "wsdashboard", "assets", "dashboard-board.css"))
	if err != nil {
		t.Fatal(err)
	}
	maxSVG, err := os.ReadFile(filepath.Join("..", "wsdashboard", "assets", "lucide", "icons", "maximize-2.svg"))
	if err != nil {
		t.Fatal(err)
	}
	minSVG, err := os.ReadFile(filepath.Join("..", "wsdashboard", "assets", "lucide", "icons", "minimize-2.svg"))
	if err != nil {
		t.Fatal(err)
	}
	extractSymbol := func(id string, raw []byte) string {
		s := string(raw)
		start := strings.Index(s, ">")
		end := strings.LastIndex(s, "</svg>")
		if start < 0 || end <= start {
			t.Fatalf("unexpected svg for %s", id)
		}
		return `<symbol id="` + id + `" viewBox="0 0 24 24">` + s[start+1:end] + `</symbol>`
	}
	dashSprite := `<svg width="0" height="0" style="position:absolute" aria-hidden="true">` +
		extractSymbol("l-maximize-2", maxSVG) + extractSymbol("l-minimize-2", minSVG) + `</svg>`

	standalonePage := `<!doctype html><html><head><meta charset="utf-8"><style>
body { margin: 0; padding: 16px; }
main { max-width: 900px; margin: 0 auto; }
</style><style>` + string(assets.CSS) + `</style></head><body>` + string(assets.Sprite) + `<main>` + string(regions.Board) + `</main><script>` + string(wsi18n.Script(view.Lang)) + `</script><script>` + string(assets.AnswersScript) + `</script><script>(function(){` + string(assets.FormScript) + `})();</script></body></html>`

	dockedPage := `<!doctype html><html><head><meta charset="utf-8"><style>
body { margin: 0; padding: 8px; }
.gal-pane { width: 100%; }
</style><style>` + string(assets.CSS) + "\n" + string(dashBoardCSS) + `</style></head><body class="dash live">` + string(assets.Sprite) + dashSprite + `<div class="gal-pane">` + string(regions.Board) + `</div><script>` + string(wsi18n.Script(view.Lang)) + `</script><script>` + string(assets.AnswersScript) + `</script><script>(function(){` + string(assets.FormScript) + `})();</script></body></html>`

	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/standalone":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(standalonePage))
			return
		case "/docked":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(dockedPage))
			return
		}
		if data, kind, ok := FrameAsset(strings.TrimPrefix(r.URL.Path, "/")); ok {
			w.Header().Set("Content-Type", kind)
			_, _ = w.Write(data)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/panel-header-rows.browser.cjs", srv.URL, os.Getenv("WS_PANEL_HEADER_SHOTS"))
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("panel header rows browser: %v", err)
	}
}
