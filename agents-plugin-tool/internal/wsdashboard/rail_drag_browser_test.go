package wsdashboard

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// TestRailDragReorderBrowser drives the rail's session drag in Chromium
// against a disposable daemon on an ephemeral port, over the owner's
// screenshot rail: four top-level rows in one worktree group, the second a
// parent of five children. The script (testdata/rail-drag.browser.cjs)
// checks that the drop lands where the line shows, downward and upward,
// from the grip and from the row body, in a gap, and that a plain click
// still opens the row. With WS_RAIL_DRAG_SHOTS set to an absolute
// directory it also saves the during/after screenshots there.
func TestRailDragReorderBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	shots := os.Getenv("WS_RAIL_DRAG_SHOTS")
	if shots != "" && !filepath.IsAbs(shots) {
		t.Fatal("WS_RAIL_DRAG_SHOTS must be an absolute directory")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	SetDevAssets(nil)
	dir, root := t.TempDir(), t.TempDir()
	devenv := filepath.Join(root, "devenv")
	// Rail repository recognition needs no Git process.
	if err := os.MkdirAll(filepath.Join(devenv, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	rows := []struct{ id, label, harness, reply string }{
		{"discuss", "DISCUSS", "claude", "가능합니다. 그리고 지금 제안하신 방향이 앞의 안보다 낫습니다."},
		{"mainlead", "메인", "claude", "레일 드래그를 고치는 중입니다."},
		{"execfour", "EXEC 4", "claude", ""},
		{"execfive", "EXEC 5", "claude", ""},
		{"execone", "EXEC 1", "pi", ""},
		{"exectwo", "EXEC 2", "pi", ""},
		{"execthree", "EXEC 3", "pi", ""},
		{"cirow", "CI", "claude", ""},
		{"sosterm", "비상복구", "claude", "플러그인을 다시 불러온 뒤에도 응답합니다."},
	}
	for i, s := range rows {
		at := now.Add(-time.Duration(len(rows)-i) * time.Minute)
		reg := Registration{ReplyID: s.id, Project: devenv, Harness: s.harness, RegisteredAt: stampFor(at), HeartbeatAt: stampFor(now.Add(time.Hour))}
		if err := WriteRegistration(dir, reg); err != nil {
			t.Fatal(err)
		}
		if err := writeSessionLabel(SessionDir(dir, s.id), s.label); err != nil {
			t.Fatal(err)
		}
		if s.reply != "" {
			if err := wslive.AppendRing(wslive.RingDir(SessionDir(dir, s.id)), []wslive.RingEntry{{Kind: wslive.RingReply, Text: s.reply}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	d := startIndexedDaemon(t, dir, newPageFake(), time.Hour, func(o *DaemonOptions) {
		o.Port = 0
		o.LAN = false
		o.bindHost = "127.0.0.1"
		o.noGitBadge = true
	})
	base := endpointURL(d.Endpoint(), "")
	if u, err := url.Parse(base); err != nil || u.Port() == "29417" {
		t.Fatal("fixture must use an ephemeral port, never the shared daemon port")
	}
	waitFor(t, "the rail sessions to be indexed", func() bool {
		_, ok := d.Index().Session("sosterm")
		return ok && d.Index().Loaded()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/rail-drag.browser.cjs", base, shots)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium rail drag acceptance: %v", err)
	}
}
