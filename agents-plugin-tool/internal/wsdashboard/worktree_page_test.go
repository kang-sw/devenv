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
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsholder"
)

func worktreePageFixture(t *testing.T, options ...func(*DaemonOptions)) (*pageFixture, string, string) {
	t.Helper()
	base, root := worktreeRepo(t)
	linked := filepath.Join(base, "work tree #한")
	gitRun(t, root, "worktree", "add", "-q", "-b", "topic/linked", linked)
	for _, path := range []string{root, linked} {
		if err := os.WriteFile(filepath.Join(path, "worktree.txt"), []byte(path), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	options = append(options, withWorkspaces(t), func(o *DaemonOptions) { o.Worktrees = gitWorktreeSource{} })
	f := startPageFixture(t, options...)
	if _, err := f.d.workspaces.add(root, false); err != nil {
		t.Fatal(err)
	}
	return f, root, linked
}

func TestWorktreePageGuardsAndTemplate(t *testing.T) {
	// This route reads Git and renders cold assets; the health client's
	// 500 ms deadline is too short under the race detector.
	client := &http.Client{Timeout: 10 * time.Second, Transport: localhostTestHTTPTransport()}
	get := func(t *testing.T, endpoint string) (int, string) {
		t.Helper()
		resp, err := client.Get(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, string(raw)
	}
	f, root, linked := worktreePageFixture(t)
	for i, path := range []string{root, linked} {
		route := worktreePagePath(path)
		code, shell := get(t, f.base+route)
		if code != http.StatusOK || !strings.Contains(shell, `data-shell="1"`) || !strings.Contains(shell, route+"?frame=1") {
			t.Fatalf("worktree shell = %d, expected its own frame", code)
		}
		code, page := get(t, f.base+route+"?frame=1")
		markup := strings.Split(page, "<script>")[0]
		if code != http.StatusOK {
			t.Fatalf("worktree frame = %d: %s", code, page)
		}
		branch := []string{"main", "topic/linked"}[i]
		if !strings.Contains(markup, `</svg>`+branch+`</b>`) {
			t.Errorf("cold-load header lacks branch %s", branch)
		}
		for _, want := range []string{`data-mode="worktree"`, `data-rid=""`, `data-worktree="` + filepath.ToSlash(path) + `"`, `id="files"`, `id="bcol"`, `data-region="board"`, `worktree-path`, `>worktree</span>`, `>local only</span>`} {
			if !strings.Contains(markup, want) {
				t.Errorf("worktree page missing %s", want)
			}
		}
		if !strings.Contains(markup, `id="chead" class="thead"`) || !strings.Contains(markup, `class="sum fsum"`) {
			t.Error("worktree files header must use the session's shared panel header, not the whiteboard chead")
		}
		for _, absent := range []string{`id="composer"`, `id="convo"`, `id="tabbar"`, `id="r-tree"`, `class="topic"`, `class="sname"`, `class="host `} {
			if strings.Contains(markup, absent) {
				t.Errorf("worktree page leaked %s", absent)
			}
		}
	}
	if code, _ := get(t, f.base+worktreePagePath(f.project)+"?frame=1"); code != http.StatusNotFound {
		t.Errorf("unlisted directory = %d, want 404", code)
	}
	for _, suffix := range []string{"", "?frame=1"} {
		r := httptest.NewRequest(http.MethodGet, worktreePagePath(root)+suffix, nil)
		r.RemoteAddr = "192.0.2.20:1234"
		r.Host = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		f.d.handleWorktreePage(w, r)
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "local only") {
			t.Errorf("remote worktree page = %d %s", w.Code, w.Body.String())
		}
	}
	for _, headers := range []map[string]string{{"Origin": "http://evil.example"}, {"Sec-Fetch-Site": "cross-site"}} {
		r := httptest.NewRequest(http.MethodGet, worktreePagePath(root), nil)
		r.RemoteAddr = "127.0.0.1:1234"
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		f.d.handleWorktreePage(w, r)
		if w.Code != http.StatusForbidden {
			t.Errorf("untrusted local request = %d", w.Code)
		}
	}
	gitRun(t, linked, "checkout", "--detach", "-q")
	if code, page := get(t, f.base+worktreePagePath(linked)+"?frame=1"); code != http.StatusOK || !strings.Contains(strings.Split(page, "<script>")[0], `</svg>detached HEAD</b>`) {
		t.Errorf("detached header = %d, want detached branch marker", code)
	}
	pooled := filepath.Join(filepath.Dir(root), ".ws-worktrees", filepath.Base(root), "hidden")
	gitRun(t, root, "worktree", "add", "-q", "-b", "pooled", pooled)
	if code, _ := get(t, f.base+worktreePagePath(pooled)+"?frame=1"); code != http.StatusNotFound {
		t.Errorf("rail-excluded pooled worktree = %d, want 404", code)
	}
	// Stored workspaces need no agent session. Live workspace roots use the
	// very same rail inventory after the workspace is forgotten.
	register(t, f.dir, "worktreelive", root, time.Now())
	f.d.Index().Refresh()
	if _, err := f.d.workspaces.forget(root); err != nil {
		t.Fatal(err)
	}
	if code, _ := get(t, f.base+worktreePagePath(root)+"?frame=1"); code != http.StatusOK {
		t.Errorf("live workspace worktree = %d", code)
	}
}

// Compare the shared header and explorer-only frame using real page consumers.
func TestWorktreePageFileCardStyleBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	f, root, _ := worktreePageFixture(t)
	for i := 0; i < 55; i++ {
		dir := filepath.Join(root, fmt.Sprintf("folder-%02d", i))
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "child.txt"), []byte("style fixture\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("file-%02d.txt", i)), []byte("style fixture\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/worktree-filecard-style.browser.cjs", f.base, worktreePagePath(root))
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("worktree file-card style acceptance: %v", err)
	}
}

func TestExplorerCardFramelessTabWrapBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	f, root, _ := worktreePageFixture(t)
	// Tabs are labelled by their folder; a long name makes 12 tabs overflow one row.
	if err := os.Mkdir(filepath.Join(root, "devenv-explorer-workspace"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/explorer-tab-wrap.browser.cjs", f.base, worktreePagePath(root))
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("explorer frameless tab-wrap acceptance: %v", err)
	}
}

func TestExplorerNarrowTreeHomeAndPanelWidthBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	f, root, _ := worktreePageFixture(t)
	nested := filepath.Join(root, "alpha", "beta")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "deep.txt"), []byte("deep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/explorer-narrow-home.browser.cjs", f.base, worktreePagePath(root))
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("explorer narrow tree, home and panel width acceptance: %v", err)
	}
}

func TestExplorerTabRowAndFloatWidthBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	f, root, _ := worktreePageFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/explorer-tabrow-float.browser.cjs", f.base, worktreePagePath(root))
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("explorer tab row and floating width acceptance: %v", err)
	}
}

// This is also the visual acceptance fixture: only disposable directories,
// holder processes and ports are used. Screenshots stay outside the repository.
func TestWorktreePageBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	dir, python := termHoldersDir(t)
	manager := wsholder.New(wsholder.Options{Dir: dir, Python: python})
	f, root, linked := worktreePageFixture(t, withHolders(manager), func(o *DaemonOptions) { o.terminalShell = termTestShell })
	paths, _ := json.Marshal([]string{root, linked})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/worktree-page.browser.cjs", f.base, string(paths))
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("worktree Chromium acceptance: %v", err)
	}
}
