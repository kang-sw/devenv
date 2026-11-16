package wsdashboard

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Render real dashboard pages without a daemon, provider, or terminal process.
func TestPanelRecapHeadBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for cached Chromium")
	}
	st := &wswhiteboard.State{}
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	apply := func(command string) {
		t.Helper()
		now = now.Add(time.Minute)
		if _, problems := wswhiteboard.Apply(st, command, wswhiteboard.Env{Now: now}); len(problems) != 0 {
			t.Fatal(problems)
		}
	}
	long := strings.Repeat("패널 recap은 최신 진행 상황을 요약합니다. 구현과 검증을 마쳤고 다음 검토를 기다립니다. ", 8)
	apply(`<wb:topic name="panel-only" label="패널만 있는 토픽"><wb:panel recap="` + long + `"><p>실제 패널 본문: 구현과 검증 완료.</p></wb:panel></wb:topic>`)
	apply(`<wb:topic name="mixed" label="로그 다음에 최신 패널"><wb:log>Earlier agent log — review in progress.</wb:log></wb:topic>`)
	apply(`<wb:topic name="mixed"><wb:panel recap="수리 3/5 병합, 남은 2건은 재검토 대기입니다."><p>Latest panel payload.</p></wb:panel></wb:topic>`)
	pages := map[string]string{}
	render := func(stage string) {
		view := wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: "panelrecap", Location: time.UTC})
		view.Lang = "ko"
		wb, err := wswhiteboard.RenderRegions(view)
		if err != nil {
			t.Fatal(err)
		}
		v := SessionView{ReplyID: "panelrecap", HasBoard: true, Seq: st.Seq, Board: wb.Board, Panels: wb.Panels, Lang: "ko"}
		regions, err := renderSessionRegions(v, nil)
		if err != nil {
			t.Fatal(err)
		}
		pages[stage], err = renderSessionPage(v, regions, false, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	render("panel")
	apply(`<wb:topic name="mixed"><wb:log>Later agent log — all reviews complete.</wb:log></wb:topic>`)
	render("log")
	mux := http.NewServeMux()
	mux.HandleFunc("/s/panelrecap", func(w http.ResponseWriter, r *http.Request) {
		stage := r.URL.Query().Get("stage")
		if stage == "" {
			stage = "panel"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, pages[stage])
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if data, ct, ok := wswhiteboard.FrameAsset(path.Base(r.URL.Path)); ok {
			w.Header().Set("Content-Type", ct)
			w.Write(data)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/panel-recap-head.browser.cjs", server.URL, os.Getenv("WS_PANEL_RECAP_SHOTS"))
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("panel recap head browser: %v", err)
	}
}
