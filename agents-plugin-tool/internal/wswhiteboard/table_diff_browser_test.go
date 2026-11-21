package wswhiteboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const tableDiffV1 = "| # | 시나리오 ID | Werkschaft 트리거 (로컬/사이드카) | 포즈 패밀리 | 표정 / 눈꺼풀 / 시선 | 핵심 모션 & FX |\n|---|---|---|---|---|---|\n| 7 | `gloom_fail` | 테스트·빌드 실패 / 워커 에러 / 턴 실패 | A (어깨 축 처짐) | 멘붕(`gloom`) · 무거운 반눈 · 느린 표류 시선 | 상체 주저앉음 + 느린 한숨 호흡 + 회전 `🌀` |\n| 8 | `celebrate_done` | 커밋·머지·테스트 PASS / 작업 완료 | A + `arm-raised` | 반짝(`happy`) · 초승달/반짝 눈 | 2단 폴짝 점프 + 양팔 만세 + 별/반짝이 FX |\n"
const tableDiffV2 = "| # | 시나리오 ID | Werkschaft 트리거 (로컬/사이드카) | 포즈 패밀리 / 프롭 | 표정 / 눈꺼풀 / 시선 | 핵심 모션 & FX |\n|---|---|---|---|---|---|\n| 7 | `gloom_fail` | 테스트·빌드 실패 / 워커 에러 / 턴 실패 | A (어깨 축 처짐) | 멘붕(`gloom`) · 무거운 반눈 · 느린 표류 시선 | 상체 주저앉음 + 느린 한숨 호흡 + 회전 `🌀` |\n| 8 | `celebrate_done` | 커밋·머지·테스트 PASS / 작업 완료 | A + `arm-raised` | 반짝(`happy`) · 초승달/반짝 눈 | 2단 폴짝 점프 + 양팔 만세 + 별/반짝이 FX |\n| 9 | **`subagent_dispatch`** | **서브에이전트 스폰 (`agent.spawn`)** | **A + `arm-raised`(`socket:hand`) + `prop-drone`** | 기본/자신감 · **날아가는 드론 추적 시선(위쪽)** | 손에 드론을 들어 살짝 웅크렸다가 위로 휙 던짐 → 드론이 상단 `hud-roster`로 포물선 비행 후 **퐁!** 흡수 |\n| 10 | **`mailbox_bonk`** | **메일박스 수신 (워커 보고 · 피어 메일 · 너지)** | **A + `arm-headrub`(폴백 `arm-raised`) + `prop-mail`** | **아야!(`wince` `>_<`, 폴백 `flustered`)** · 순간 움찔 후 메일 쪽 시선 | 뒤통수 쪽에서 날아온 봉투가 **확 꽂힘(2프레임 히트스톱 + 고개 홱 젖힘)** → 뒤통수 감싸며 \"아야!\" + 별/충격 FX (연속 수신 시 합쳐서 처리) |\n"

func TestTableDiffRenderBrowser(t *testing.T) {
	a, err := loadAssets()
	if err != nil {
		t.Fatal(err)
	}
	p := newPageData(View{Layout: LayoutFull, Lang: "ko"}, a)
	pages := map[string]string{}
	for _, tc := range []struct{ name, old, cur string }{{"/add", tableDiffV1, tableDiffV2}, {"/remove", tableDiffV2, tableDiffV1}} {
		e := EntryView{Entry: Entry{ID: "e254", Text: tc.cur}, Diff: diffPanel(tc.old, tc.cur)}
		body := string(p.renderPanel(e))
		if !strings.Contains(body, "<!--wbpdg:0-->") {
			t.Fatalf("missing table diff: %s", body)
		}
		pages[tc.name] = `<!doctype html><html data-pdg-compare="true"><head><meta charset="utf-8"><style>` + string(a.css) + `</style></head><body><div class="doc-body md">` + body + `</div><script>window.wsT=function(x){return x};</script><script>` + string(a.panelDiff) + `</script></body></html>`
	}
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(pages[r.URL.Path]))
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/table-diff.browser.cjs", srv.URL, os.Getenv("WS_TABLE_DIFF_SHOTS"))
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatal(err)
	}
}
