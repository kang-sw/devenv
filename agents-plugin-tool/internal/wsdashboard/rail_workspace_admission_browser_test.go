package wsdashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func TestRailWorkspaceAdmissionBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	home := t.TempDir()
	for _, name := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "WS_CACHE_ROOT"} {
		t.Setenv(name, filepath.Join(home, name))
	}
	SetDevAssets(nil)
	dir := t.TempDir()
	workspacesFile := filepath.Join(dir, "dashboard", "workspaces.json")
	stats := &streamStats{}
	d := &Daemon{dir: dir, log: io.Discard, stats: stats, broker: newBroker(stats), index: NewIndex(IndexOptions{Dir: dir, Source: newFakeSource()}), layout: openRailLayoutStore(dir), workspaces: openWorkspaceStore(workspacesFile), wsAdmission: openWorkspaceAdmissionStore(WorkspaceAdmissionFilePath(workspacesFile))}
	d.rail = newRailLister(nil, nil, make(chan struct{}))
	d.rail.admission = newRailAdmissionTracker(d.layout)
	paths := make([]string, 30)
	for i := range paths {
		paths[i] = mkDir(t, filepath.Join(t.TempDir(), fmt.Sprintf("temp-root-%02d", i)))
		register(t, dir, fmt.Sprintf("session%02d", i), paths[i], time.Now())
	}
	d.index.Refresh()
	d.index.Settle()
	explicit := mkDir(t, filepath.Join(t.TempDir(), "explicit-ws"))
	if _, err := d.workspaces.add(explicit, false); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	refuseWorkspace, worktreeCalls := false, 0
	conns := map[*websocket.Conn]bool{}
	render := func() renderedRegion {
		r, err := renderRailRegion(d.railView(d.pagesIn("ko")), "ko")
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	send := func() {
		r := render()
		for c := range conns {
			_ = websocket.JSON.Send(c, map[string]any{"event": "region", "data": map[string]any{"name": "rail", "hash": r.Hash, "html": r.HTML}})
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/s/") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"entries":[],"memos":[],"writable":true}`)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		page, err := renderShellPage(shellPageData{Lang: "ko", Src: "/fixture-frame", Title: "Workspace admission fixture", regions: regionSet{regionRail: render()}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, page)
	})
	mux.HandleFunc("/fixture-frame", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "<!doctype html><title>Fixture</title>") })
	mux.HandleFunc(workspaceAdmissionRoute, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if refuseWorkspace {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"applied":false}`)
			return
		}
		d.handleWorkspaceAdmission(w, r)
	})
	mux.HandleFunc(railAdmissionRoute, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		worktreeCalls++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"applied":false,"stale":[]}`)
	})
	mux.HandleFunc(railRoute, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		d.layout.mu.Lock()
		d.layout.gen++
		d.layout.mu.Unlock()
		send()
		w.WriteHeader(204)
	})
	mux.Handle(railEventsRoute, websocket.Handler(func(c *websocket.Conn) {
		mu.Lock()
		conns[c] = true
		mu.Unlock()
		defer func() { mu.Lock(); delete(conns, c); mu.Unlock(); c.Close() }()
		for {
			var incoming string
			if websocket.Message.Receive(c, &incoming) != nil {
				return
			}
		}
	}))
	mux.HandleFunc("/fixture-control", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Query().Get("op") {
		case "refuse":
			refuseWorkspace = true
		case "allow":
			refuseWorkspace = false
		case "calls":
			json.NewEncoder(w).Encode(worktreeCalls)
			return
		case "empty":
			for _, ws := range d.workspaces.workspaces() {
				if _, err := d.workspaces.forget(ws.Path); err != nil {
					t.Fatal(err)
				}
			}
			send()
		case "hide":
			// An empty index render demonstrates that row-less drafts are retained.
			d.index = NewIndex(IndexOptions{Dir: t.TempDir(), Source: newFakeSource()})
			send()
		case "refresh":
			d.layout.mu.Lock()
			d.layout.gen++
			d.layout.mu.Unlock()
			send()
		}
		json.NewEncoder(w).Encode(paths)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		for c := range conns {
			c.Close()
		}
	}()
	shots := "/tmp/rawshots-p2"
	if err := os.MkdirAll(shots, 0755); err != nil {
		t.Fatal(err)
	}
	reference, err := filepath.Abs("../../../ai-docs/assets/mockups/261120/rail-workspace-admission/workspace-admission-v1.html")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/rail-workspace-admission.browser.cjs", server.URL, reference, shots)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("workspace admission browser: %v", err)
	}
}
