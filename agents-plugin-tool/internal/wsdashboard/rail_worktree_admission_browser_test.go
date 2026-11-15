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
	"sync"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

// Opt-in like rail_hover_browser_test.go, but no StartDaemon: only production
// renderers/route and a private httptest transport, with no provider sources.
func TestRailWorktreeAdmissionBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run cached Chromium acceptance")
	}
	home := t.TempDir()
	for _, name := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "CODEX_HOME", "CLAUDE_CONFIG_DIR", "WS_CACHE_ROOT"} {
		t.Setenv(name, filepath.Join(home, name))
	}
	SetDevAssets(nil)
	base, root := worktreeRepo(t)
	paths := map[string]string{}
	for _, name := range []string{"agent-a82f81f3", "tmp.Xk2", "manual-check"} {
		paths[name] = filepath.Join(base, name)
		gitRun(t, root, "worktree", "add", "-q", "-b", name, paths[name])
	}
	dir := t.TempDir()
	stats := &streamStats{}
	d := &Daemon{dir: dir, log: io.Discard, stats: stats, broker: newBroker(stats), index: NewIndex(IndexOptions{Dir: dir}), layout: openRailLayoutStore(dir), workspaces: openWorkspaceStore(filepath.Join(dir, "workspaces.json"))}
	d.rail = newRailLister(func(root string, regs []Registration) (RepoWorktrees, error) {
		return (gitWorktreeSource{}).ReadRepoWorktrees(root, regs)
	}, nil, make(chan struct{}))
	d.rail.admission = newRailAdmissionTracker(d.layout)
	if _, err := d.workspaces.add(root, false); err != nil {
		t.Fatal(err)
	}
	if err := d.recordUserWorktree(root, paths["manual-check"], "manual-check"); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	conns := map[*websocket.Conn]bool{}
	missingPath := ""
	refresh := func() {
		list := d.rail.readRepo(root, nil)
		d.rail.mu.Lock()
		d.rail.repos[root] = list
		d.rail.mu.Unlock()
	}
	refresh()
	render := func() renderedRegion {
		r, err := renderRailRegion(d.railView(d.pages()), "en")
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
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path != "/" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"entries":[],"memos":[],"writable":true}`)
			return
		}
		page, err := renderShellPage(shellPageData{Lang: "en", Src: "/fixture-frame", Title: "Worktree admission fixture", regions: regionSet{regionRail: render()}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, page)
	})
	mux.HandleFunc("/fixture-frame", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<!doctype html><title>Isolated fixture</title>")
	})
	mux.HandleFunc(railAdmissionRoute, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		d.handleRailAdmission(w, r)
	})
	mux.HandleFunc(railRoute, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		refresh()
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
		case "reset":
			_, err := d.layout.mutateAdmissions(func([]railAdmissionRecord) ([]railAdmissionRecord, error) { return nil, nil })
			if err != nil {
				t.Fatal(err)
			}
			if err := d.recordUserWorktree(root, paths["manual-check"], "manual-check"); err != nil {
				t.Fatal(err)
			}
			refresh()
			send()
		case "replace":
			p := paths["agent-a82f81f3"]
			old := p + fmt.Sprintf("-old-%d", time.Now().UnixNano())
			if err := os.Rename(p, old); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(p, 0700); err != nil {
				t.Fatal(err)
			}
			marker, err := os.ReadFile(filepath.Join(old, ".git"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(p, ".git"), marker, 0600); err != nil {
				t.Fatal(err)
			}
		case "missing":
			p := paths["agent-a82f81f3"]
			missingPath = p + fmt.Sprintf("-missing-%d", time.Now().UnixNano())
			if err := os.Rename(p, missingPath); err != nil {
				t.Fatal(err)
			}
			refresh()
			send()
		case "restore":
			if err := os.Rename(missingPath, paths["agent-a82f81f3"]); err != nil {
				t.Fatal(err)
			}
			refresh()
			send()
		case "state":
			json.NewEncoder(w).Encode(admissionRecords(t, d.layout))
			return
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
	reference, err := filepath.Abs("../../../ai-docs/tickets/assets/261115-feat-ws-dashboard-worktree-detection-approval/847-worktree-approval.html")
	if err != nil {
		t.Fatal(err)
	}
	artifacts := "/tmp/ws-worktree-admission-evidence"
	if err := os.MkdirAll(artifacts, 0755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/rail-worktree-admission.browser.cjs", server.URL, reference, artifacts)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("production admission browser: %v", err)
	}
}
