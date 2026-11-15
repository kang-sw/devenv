package wsdashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Opt in with explicit cached dependencies; this fixture never uses a daemon.
func TestBoardViewedUpdatePerfBrowser(t *testing.T) {
	playwright, chromium := os.Getenv("WS_VIEWED_PERF_PLAYWRIGHT"), os.Getenv("WS_VIEWED_PERF_CHROMIUM")
	if playwright == "" || chromium == "" {
		t.Skip("set WS_VIEWED_PERF_PLAYWRIGHT and WS_VIEWED_PERF_CHROMIUM to cached local paths")
	}
	for _, path := range []string{playwright, chromium} {
		if !filepath.IsAbs(path) {
			t.Fatalf("browser dependency path must be absolute: %q", path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	SetDevAssets(nil)
	f := startPageFixture(t, func(o *DaemonOptions) {
		o.Tracker = &fakeTracker{}
		o.Port = 0
		o.LAN = false
		o.bindHost = "127.0.0.1"
	})
	writeSnapshot(t, f.dir, "livesession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 3600000})
	order := []string{"base"}
	for k := 1; k <= 12; k++ {
		order = append(order, fmt.Sprintf("u%d", k))
	}
	order = append(order, "img", "vis")
	variants := map[string]any{"order": order}
	for _, variant := range order {
		st := perfBoardState(variant)
		view := wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: "livesession"})
		regions, err := wswhiteboard.RenderRegions(view)
		if err != nil {
			t.Fatal(err)
		}
		variants[variant] = string(regions.Board)
		if variant == "base" {
			f.src.setPage("livesession", PageContent{HasBoard: true, Seq: 100, Board: regions})
			f.src.mu.Lock()
			f.src.boards["livesession"] = st
			f.src.mu.Unlock()
		}
	}
	f.d.Index().Refresh()
	raw, err := json.Marshal(variants)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "variants.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/viewed-update-perf.browser.cjs", f.base, path)
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("board viewed update performance browser: %v", err)
	}
}

func perfBoardState(variant string) *wswhiteboard.State {
	updates := 0
	if variant == "img" || variant == "vis" {
		updates = 12
	} else if len(variant) > 1 && variant[0] == 'u' {
		updates, _ = strconv.Atoi(variant[1:])
	}
	// BuildView sorts entry timestamps before Changed; keep times equal so
	// only the explicit update bumps reorder topics, not the late panel.
	st := &wswhiteboard.State{Seq: 100 + updates}
	for i := 0; i < 120; i++ {
		name := fmt.Sprintf("T%03d", i)
		topic := wswhiteboard.TopicDoc{Name: name, Changed: 100 - i}
		for j := 0; j < 12; j++ {
			id := i*12 + j + 1
			e := wswhiteboard.Entry{ID: fmt.Sprintf("e%d", id), Kind: wswhiteboard.EntryAgent, Text: fmt.Sprintf("%s log entry %d: stable progress notes.", name, j), At: "2026-11-15T00:00:00Z"}
			if j%4 == 0 {
				e.Kind, e.PanelID = wswhiteboard.EntryPanel, fmt.Sprintf("p%d", id)
				e.Recap = fmt.Sprintf("%s panel %d", name, j)
				e.Text = fmt.Sprintf(`<div style="height:200px;padding:12px;box-sizing:border-box"><h2>%s panel %d</h2><p>Stable reading material and progress context.</p></div>`, name, j)
				if variant == "vis" && (i == 1 || i == 2) && j == 0 {
					e.Text += `<p>Visible panel text revised.</p>`
				}
			}
			topic.Entries = append(topic.Entries, e)
		}
		for k := 1; k <= updates; k++ {
			if i == (k*7)%100 {
				topic.Entries = append(topic.Entries, wswhiteboard.Entry{ID: fmt.Sprintf("e%d", 1440+k), Kind: wswhiteboard.EntryAgent, Text: fmt.Sprintf("Agent update %d", k), At: "2026-11-15T00:00:00Z"})
				topic.Changed = 100 + k
			}
		}
		if i == 60 && (variant == "img" || variant == "vis") {
			topic.Entries = append(topic.Entries, wswhiteboard.Entry{ID: "e1453", Kind: wswhiteboard.EntryPanel, PanelID: "p1453", Recap: "Late-growing image simulation", At: "2026-11-15T00:00:00Z", Text: `<div style="height:200px"><h2>Late image</h2><p>This panel grows after hydration.</p></div><script>setTimeout(function(){var n=document.createElement('div');n.style.height='400px';n.textContent='Late image content';(document.querySelector('.doc-body')||document.body).appendChild(n);},700);</script>`})
		}
		st.Topics = append(st.Topics, topic)
		if i >= 100 {
			st.ClosedTopics = append(st.ClosedTopics, name)
		}
	}
	return st
}
