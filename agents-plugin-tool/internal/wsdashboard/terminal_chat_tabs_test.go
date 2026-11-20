package wsdashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wslive"
)

func TestTerminalChatTabsTemplateAndAssets(t *testing.T) {
	manager := wsholder.New(wsholder.Options{Dir: filepath.Join(t.TempDir(), wsholder.DirName)})
	f := startPageFixture(t, lanMode, withHolders(manager))
	port := strconv.Itoa(f.d.Endpoint().Port)
	local := viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + port, localCookie: true}
	lan := viewer{peer: "192.168.7.30:50000", host: "192.168.7.20:" + port, cookie: readLANToken(t, f.dir)}

	localPage := serve(f.d, local, "GET", "/s/livesession?frame=1", "").Body.String()
	ctabsIdx := strings.Index(localPage, `id="ctabs"`)
	filesIdx := strings.Index(localPage, `data-ctab="files"`)
	addIdx := strings.Index(localPage, `data-ctab-add-term`)
	navEnd := strings.Index(localPage[ctabsIdx:], `</nav>`) + ctabsIdx
	if ctabsIdx < 0 || filesIdx < 0 || addIdx < 0 || !(ctabsIdx < filesIdx && filesIdx < addIdx && addIdx < navEnd) {
		t.Fatalf("expected #ctabs add-terminal button right after files tab (ctabs=%d files=%d add=%d navEnd=%d)", ctabsIdx, filesIdx, addIdx, navEnd)
	}
	if !strings.Contains(localPage, `aria-label="New terminal"`) || !strings.Contains(localPage, `href="#l-square-terminal"`) {
		t.Error("add-terminal button missing accessible label or terminal icon")
	}

	lanPage := serve(f.d, lan, "GET", "/s/livesession?frame=1", "").Body.String()
	if strings.Contains(lanPage, `data-ctab-add-term`) {
		t.Error("LAN viewer page must omit #ctabs add-terminal button")
	}

	boardPage := serve(f.d, local, "GET", "/s/boardsession?frame=1", "").Body.String()
	if strings.Contains(boardPage, `data-ctab-add-term`) {
		t.Error("board-only/history page must omit #ctabs add-terminal button")
	}

	plain := startPageFixture(t, withHolders(newFakeHolders()))
	plainLocal := viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + strconv.Itoa(plain.d.Endpoint().Port), localCookie: true}
	noTermPage := serve(plain.d, plainLocal, "GET", "/s/livesession?frame=1", "").Body.String()
	if strings.Contains(noTermPage, `data-ctab-add-term`) {
		t.Error("session page without terminals must omit #ctabs add-terminal button")
	}
}

func TestTerminalChatTabsBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for disposable Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	dir, python := termHoldersDir(t)
	manager := wsholder.New(wsholder.Options{Dir: dir, Python: python})
	f := startPageFixture(t, withHolders(manager), func(o *DaemonOptions) { o.terminalShell = termTestShell })
	now := time.Now().UnixMilli()
	agent := wslive.Agent{ID: "a1", Label: "worker-tabs", Status: wslive.StatusRunning, StartedAt: now - 15000}
	agent.Phase, agent.TurnStartedAt = wslive.PhaseTool, now-8000
	snap := wslive.Snapshot{
		SchemaVersion: 1,
		Adapter:       "pi",
		UpdatedAt:     now,
		HeartbeatMs:   30000,
		Lead:          wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}},
		Agents:        []wslive.Agent{agent},
	}
	writeSnapshot(t, f.dir, "livesession", &snap)
	if err := wslive.AppendRing(wslive.RingDir(SessionDir(f.dir, "livesession")), []wslive.RingEntry{
		{Kind: wslive.RingUser, Text: "Hello from the session"},
		{Kind: wslive.RingReply, Text: "Ready to inspect terminal tabs."},
	}); err != nil {
		t.Fatal(err)
	}
	f.d.Index().Refresh()
	slot, ok := f.d.OpenHostedSlot(SessionDir(f.dir, "livesession"))
	if !ok {
		t.Fatal("no hosted slot for livesession")
	}

	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = wslive.AppendRing(wslive.RingDir(SessionDir(f.dir, "livesession")), []wslive.RingEntry{
			{Kind: wslive.RingReply, Text: "New reply while terminal tab active"},
		})
		slot.RingAppended()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer control.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/terminal-chat-tabs.browser.cjs", f.base, control.URL)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium chat terminal tabs: %v", err)
	}
}
