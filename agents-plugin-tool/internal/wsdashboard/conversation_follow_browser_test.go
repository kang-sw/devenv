package wsdashboard

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// TestConversationSendFollowBrowser exercises the real composer, scroll owner
// and live-region stream. The daemon has only disposable data and no provider.
func TestConversationSendFollowBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for disposable Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("OWN test_pid=%d (daemon is in-process)", os.Getpid())
	home := t.TempDir()
	for _, key := range []string{"HOME", "CLAUDE_CONFIG_DIR", "CODEX_HOME", "WS_CACHE_HOME", "WS_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		dir := filepath.Join(home, key)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv(key, dir)
	}
	t.Setenv("WS_MCP_DASHBOARD_DEV_SOURCE", "")
	f := startPageFixture(t, func(o *DaemonOptions) { o.Home = os.Getenv("HOME") })
	// A long real reply provides overflow without CSS or geometry substitutes.
	updates := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stage := r.URL.Query().Get("stage")
		if stage != "initial" && stage != "reply" && stage != "growth" && stage != "cancelled" {
			http.Error(w, "unknown fixture stage", http.StatusBadRequest)
			return
		}
		n := map[string]int{"initial": 35, "reply": 43, "growth": 51, "cancelled": 59}[stage]
		var text strings.Builder
		for i := 0; i < n; i++ {
			fmt.Fprintf(&text, "Conversation paragraph %02d. Real streamed reply text for the bottom-follow browser fixture.\n\n", i)
		}
		text.WriteString("TAIL-" + stage)
		f.d.snapshots.SetLive(f.dir, "livesession", wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 3600000,
			Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, Reply: &wslive.Reply{Text: text.String()}}})
		f.d.broker.wake(liveDep("livesession"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer updates.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/conversation-follow.browser.cjs")
	cmd.Env = append(os.Environ(), "FOLLOW_BASE="+f.base, "FOLLOW_UPDATES="+updates.URL, "FOLLOW_SHOTS="+os.Getenv("WS_SEND_FOLLOW_SHOTS"))
	cmd.WaitDelay = 5 * time.Second
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Logf("OWN node_pid=%d fixture=%s updates=%s isolated_home=%s", cmd.Process.Pid, f.base, updates.URL, home)
	err = cmd.Wait()
	t.Log(out.String())
	if err != nil {
		t.Fatalf("conversation follow Chromium: %v", err)
	}
	t.Logf("OWN node_pid=%d reaped", cmd.Process.Pid)
}
