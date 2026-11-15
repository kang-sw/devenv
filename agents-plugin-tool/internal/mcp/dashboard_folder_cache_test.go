package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Opt-in CPU fixture: one isolated daemon, a 112-topic board and an open
// stream. Non-board snapshot changes wake it four times per second. All
// credentials, state, config, and listeners belong to the test, not the host.
func TestDashboardFolderIdleCPUFixture(t *testing.T) {
	profile := os.Getenv("WS_DASHBOARD_IDLE_PROFILE")
	if profile == "" {
		t.Skip("opt-in 10-second isolated daemon profile")
	}
	t.Setenv("WS_MCP_NO_DASHBOARD", "1")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	f := newWhiteboardFixture(t)
	if err := whiteboardFolder(f.root, f.key).Update(nil, func(s *wswhiteboard.State) error {
		for i := 0; i < 112; i++ {
			s.Topics = append(s.Topics, wswhiteboard.TopicDoc{Name: fmt.Sprintf("topic-%d", i), Body: "idle fixture topic"})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	src := &DashboardSessionSource{store: newSessionStore(), stateDir: func() (string, error) { return dir, nil }}
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root, HeartbeatAt: time.Now().UTC().Format(time.RFC3339)}
	if err := wsdashboard.WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	writeSnapshot := func(i int) {
		t.Helper()
		snap := wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 30000}
		// Change a non-board field; updatedAt alone is intentionally ignored
		// by the session fingerprint, so a pure heartbeat is not a wake.
		snap.Lead.Phase = "idle"
		snap.Lead.Caption = fmt.Sprintf("fixture-%d", i)
		data, err := json.Marshal(snap)
		path := wslive.SnapshotPath(wsdashboard.SessionDir(dir, reg.ReplyID))
		if err == nil {
			err = os.WriteFile(path+".tmp", data, 0600)
		}
		if err == nil {
			err = wsstate.AtomicReplaceFile(path+".tmp", path)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	writeSnapshot(0)
	daemon, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{Dir: dir, Sessions: src, Version: "test", PollInterval: time.Hour, Pprof: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(daemon.Close)
	daemon.Index().Refresh()
	daemon.Index().Settle()
	stream := openWhiteboardStreamForTest(t, daemon, reg.ReplyID)
	waitWhiteboardRegionForTest(t, stream, "board", "topic-0")
	// Settle the board-render memo before measuring wake-driven reads.
	readPageOK(t, src, reg)
	readPageOK(t, src, reg)
	beforeRenders := boardRenders(src)
	out, err := os.Create(profile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { out.Close() })
	if err := pprof.StartCPUProfile(out); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pprof.StopCPUProfile)
	start := time.Now()
	ticker := time.NewTicker(250 * time.Millisecond)
	for i := 1; i <= 40; i++ {
		<-ticker.C
		writeSnapshot(i)
		daemon.Index().Reread(reg.ReplyID)
		daemon.Index().Settle()
	}
	ticker.Stop()
	time.Sleep(300 * time.Millisecond) // let the last shared page read land
	pprof.StopCPUProfile()
	out.Close()
	url := fmt.Sprintf("http://127.0.0.1:%d/debug/pprof/streams", daemon.Endpoint().Port)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	stats, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profile+".streams.json", stats, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("112 topics, 40 non-board wakes, elapsed=%s, board rebuilds=%d; counters=%s", time.Since(start), boardRenders(src)-beforeRenders, stats)
	// Preserve the identity of the isolated fixture alongside its profile.
	if err := os.WriteFile(profile+".fixture.txt", []byte(fmt.Sprintf("state=%s\ncache=%s\nconfig=%s\nport=%d\n", dir, os.Getenv("WS_CACHE_HOME"), os.Getenv("WS_CONFIG_HOME"), daemon.Endpoint().Port)), 0600); err != nil {
		t.Fatal(err)
	}
}
