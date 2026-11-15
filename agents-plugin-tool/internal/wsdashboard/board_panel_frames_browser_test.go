package wsdashboard

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Run native script-continuity checks only against disposable fixture sessions.
func TestBoardPanelFrameReuseBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	SetDevAssets(nil)
	f := startPageFixture(t, func(o *DaemonOptions) {
		o.Port = 0
		o.LAN = false
		o.bindHost = "127.0.0.1"
	})
	writeSnapshot(t, f.dir, "livesession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 3600000})

	panel := func(id, version string) string {
		// The load token detects a restart even if the counter overtakes its
		// old value before the browser driver samples it. No host DOM access.
		return fmt.Sprintf(`<p>%s %s</p><script>window.panelRun={load:Math.random().toString(36),ticks:0,version:%q};setInterval(function(){window.panelRun.ticks++;},20);</script>`, id, version, version)
	}
	st := &wswhiteboard.State{Seq: 1, Topics: []wswhiteboard.TopicDoc{
		{Name: "BodyAndPanel", Body: "Small topic body", Changed: 1, Entries: []wswhiteboard.Entry{{ID: "e1", Kind: wswhiteboard.EntryPanel, PanelID: "p1", Recap: "Body topic panel", Text: panel("p1", "original")}}},
		{Name: "PanelOnly", Entries: []wswhiteboard.Entry{{ID: "e2", Kind: wswhiteboard.EntryPanel, PanelID: "p2", Recap: "Panel-only topic panel", Text: panel("p2", "original")}}},
	}}
	pages := map[string]PageContent{}
	render := func(name string) {
		view := wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: "livesession"})
		regions, err := wswhiteboard.RenderRegions(view)
		if err != nil {
			t.Fatal(err)
		}
		pages[name] = PageContent{HasBoard: true, Seq: st.Seq, Board: regions}
	}
	render("base")
	for i, topic := range []int{0, 1} {
		st.Seq++
		st.Topics[topic].Entries = append(st.Topics[topic].Entries, wswhiteboard.Entry{ID: fmt.Sprintf("e%d", 10+i), Kind: wswhiteboard.EntryAgent, Text: fmt.Sprintf("Unrelated log %d", i)})
		render([]string{"log-body", "log-only"}[i])
	}
	for i, topic := range []int{0, 1} {
		st.Seq++
		id := fmt.Sprintf("p%d", i+1)
		// A real overwrite appends a new entry version under the same panel id.
		st.Topics[topic].Entries = append(st.Topics[topic].Entries, wswhiteboard.Entry{ID: fmt.Sprintf("e%d", 20+i), Kind: wswhiteboard.EntryPanel, PanelID: id, Recap: "Replacement panel", Text: panel(id, "replacement")})
		render([]string{"overwrite-body", "overwrite-only"}[i])
	}
	f.src.setPage("livesession", pages["base"])
	f.d.Index().Refresh()
	updates := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, ok := pages[r.URL.Query().Get("state")]
		if !ok {
			http.Error(w, "unknown fixture state", http.StatusBadRequest)
			return
		}
		f.src.setPage("livesession", page)
		f.d.Index().Refresh()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer updates.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/board-panel-frames.browser.cjs", f.base, updates.URL)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium panel frame reuse acceptance: %v", err)
	}
}
