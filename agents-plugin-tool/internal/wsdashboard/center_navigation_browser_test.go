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

// Exercises the production renderer and dashboard assets without a daemon.
func TestCenterNavigationBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for cached Playwright")
	}
	st := &wswhiteboard.State{}
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	for n, name := range []string{"stream", "first-pin", "second-pin"} {
		for i := 0; i < 29; i++ {
			now = now.Add(time.Minute)
			cmd := fmt.Sprintf(`<wb:topic name="%s"><wb:add class="check"><wb:title>Check %02d for %s</wb:title></wb:add></wb:topic>`, name, i, name)
			if _, problems := wswhiteboard.Apply(st, cmd, wswhiteboard.Env{Now: now}); len(problems) != 0 {
				t.Fatal(problems)
			}
		}
		now = now.Add(time.Minute)
		cmd := fmt.Sprintf(`<wb:topic name="%s"><wb:log>Reference {{%d}}</wb:log></wb:topic>`, name, n*29+15)
		if _, problems := wswhiteboard.Apply(st, cmd, wswhiteboard.Env{Now: now}); len(problems) != 0 {
			t.Fatal(problems)
		}
	}
	view := wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: "centernav", Location: time.UTC})
	view.Lang = "en"
	wb, err := wswhiteboard.RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	v := SessionView{ReplyID: "centernav", HasBoard: true, Seq: st.Seq, Board: wb.Board, Panels: wb.Panels, Lang: "en"}
	regions, err := renderSessionRegions(v, nil)
	if err != nil {
		t.Fatal(err)
	}
	html, err := renderSessionPage(v, regions, false, "")
	if err != nil {
		t.Fatal(err)
	}
	rail, err := renderRailRegion(RailView{}, "en")
	if err != nil {
		t.Fatal(err)
	}
	shell, err := renderShellPage(shellPageData{Lang: "en", Src: "/s/centernav", Title: "Navigation fixture", regions: regionSet{regionRail: rail}}, &v)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/fixture", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, shell)
	})
	mux.HandleFunc("/s/centernav", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		themed := html
		if c, err := r.Cookie("ws_dashboard_theme"); err == nil && (c.Value == "dark" || c.Value == "light") {
			themed = strings.Replace(html, "<html ", `<html data-theme="`+c.Value+`" `, 1)
		}
		fmt.Fprint(w, themed)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if data, ct, ok := wswhiteboard.FrameAsset(path.Base(r.URL.Path)); ok {
			w.Header().Set("Content-Type", ct)
			_, _ = w.Write(data)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/center-navigation.browser.cjs", server.URL, os.Getenv("WS_CENTER_NAV_SHOTS"))
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("center navigation browser: %v", err)
	}
}
