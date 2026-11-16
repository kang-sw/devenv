package wsdashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func gitInitFixture(t *testing.T) (*pageFixture, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	f := startPageFixture(t, withWorkspaces(t), func(o *DaemonOptions) {
		o.Worktrees = gitWorktreeSource{}
		o.Agents = &startOrderAgents{}
	})
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	folder := mkDir(t, filepath.Join(base, "notes"))
	if _, err := f.d.workspaces.add(folder, false); err != nil {
		t.Fatal(err)
	}
	return f, folder
}

func postGitInit(t *testing.T, f *pageFixture, path string) (int, string) {
	t.Helper()
	code, _, raw := postJSONWith(t, &http.Client{Timeout: 15 * time.Second, Transport: localhostTestHTTPTransport()}, f.base+workspaceGitInitRoute, f.base, pathBody(t, "path", path))
	return code, raw
}

func TestGitInitListedFolderAndUnbornRail(t *testing.T) {
	f, folder := gitInitFixture(t)
	writeFile(t, filepath.Join(folder, "keep.txt"), "not committed")
	code, raw := postGitInit(t, f, folder)
	var answer workspaceAddAnswer
	if err := json.Unmarshal([]byte(raw), &answer); err != nil || code != http.StatusOK || !answer.Repo || answer.Root != folder || answer.Message != "Made notes a Git repository" {
		t.Fatalf("git init = %d %s (%v)", code, raw, err)
	}
	if out, err := exec.Command("git", "-C", folder, "rev-parse", "--verify", "HEAD").CombinedOutput(); err == nil {
		t.Fatalf("init created a commit: %s", out)
	}
	out, err := exec.Command("git", "-C", folder, "ls-files").Output()
	if err != nil || len(out) != 0 {
		t.Fatalf("init staged files: %s (%v)", out, err)
	}
	if got, err := os.ReadFile(filepath.Join(folder, "keep.txt")); err != nil || string(got) != "not committed" {
		t.Fatalf("init changed existing files: %q (%v)", got, err)
	}
	stored := f.d.workspaces.workspaces()
	if len(stored) != 1 || !stored[0].Repo {
		t.Fatalf("workspace not promoted: %+v", stored)
	}
	// The production lister runs only with an open display watch. An unborn
	// HEAD must still produce the root row and its normal new-session button.
	release := f.d.rail.watch()
	defer release()
	waitFor(t, "unborn root worktree in rail", func() bool {
		view := f.d.railView(f.d.pages())
		for _, ws := range view.Workspaces {
			if ws.Path == folder && len(ws.Worktrees) == 1 {
				wt := ws.Worktrees[0]
				return ws.Repo && wt.Root && !wt.Detached && wt.Branch == headBranch(t, folder)
			}
		}
		return false
	})
	region, err := renderRailRegion(f.d.railView(f.d.pages()), "ko")
	if err != nil {
		t.Fatal(err)
	}
	section := railSection(t, string(region.HTML), folder)
	for _, want := range []string{`class="rnew"`, `class="radd"`, `href="#i-repo"`, `· 루트`} {
		if !strings.Contains(section, want) {
			t.Errorf("unborn rail lacks %s: %s", want, section)
		}
	}
	if strings.Contains(section, `class="rgit"`) {
		t.Fatal("git-init button remains on repository bar")
	}
}

func TestGitInitSessionFolderAndRefusals(t *testing.T) {
	f, folder := gitInitFixture(t)
	// A listed session's plain folder is eligible without being stored.
	if code, raw := postGitInit(t, f, f.project); code != http.StatusOK {
		t.Fatalf("session folder = %d %s", code, raw)
	}
	unlisted := t.TempDir()
	for _, path := range []string{unlisted, filepath.Join(folder, "unknown"), "relative", ""} {
		if code, raw := postGitInit(t, f, path); code != http.StatusNotFound {
			t.Errorf("unlisted %q = %d %s", path, code, raw)
		}
	}
	if holdsGitDir(unlisted) {
		t.Fatal("unlisted folder initialized")
	}
	// The stored list still calls notes plain, but its parent became a repo
	// since the button was drawn. Never create a nested repository there.
	gitRun(t, filepath.Dir(folder), "init", "-q")
	if code, raw := postGitInit(t, f, folder); code != http.StatusConflict || !strings.Contains(raw, "already inside") {
		t.Fatalf("folder inside repo = %d %s", code, raw)
	}
	if holdsGitDir(folder) {
		t.Fatal("created a nested repository")
	}
	if code, _ := postGitInit(t, f, f.project); code < 400 {
		t.Fatal("already initialized session folder accepted")
	}
}

func TestGitInitGuards(t *testing.T) {
	f, folder := gitInitFixture(t)
	for _, tc := range []struct {
		name, method, remote, origin string
		status                       int
	}{
		{"remote", "POST", "192.0.2.2:1234", f.base, http.StatusNotFound},
		{"no-origin", "POST", "127.0.0.1:1234", "", http.StatusForbidden},
		{"cross-origin", "POST", "127.0.0.1:1234", "http://evil.example", http.StatusForbidden},
		{"get", "GET", "127.0.0.1:1234", f.base, http.StatusMethodNotAllowed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, f.base+workspaceGitInitRoute, strings.NewReader(pathBody(t, "path", folder)))
			r.RemoteAddr = tc.remote
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			f.d.handleWorkspaceGitInit(w, r)
			if w.Code != tc.status {
				t.Errorf("got %d %s, want %d", w.Code, w.Body, tc.status)
			}
		})
	}
	if holdsGitDir(folder) {
		t.Fatal("refused request initialized folder")
	}
}

func TestGitInitGitNotFound(t *testing.T) {
	f, folder := gitInitFixture(t)
	t.Setenv("PATH", t.TempDir())
	code, raw := postGitInit(t, f, folder)
	if code != http.StatusBadGateway || strings.TrimSpace(raw) != "Git was not found. Install Git and try again." {
		t.Fatalf("missing git = %d %s", code, raw)
	}
	if holdsGitDir(folder) {
		t.Fatal("missing git initialized folder")
	}
}

func TestGitInitExactCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	folder := t.TempDir()
	cmd := gitInitCommand(ctx, folder, "init")
	if !reflect.DeepEqual(cmd.Args[1:], []string{"init"}) || cmd.Dir != folder {
		t.Fatalf("not exactly git init in folder: %v, %s", cmd.Args, cmd.Dir)
	}
}

func TestGitInitRailMarkup(t *testing.T) {
	view := RailView{CanAdd: true, CanSpawn: true, Workspaces: []RailWorkspace{
		{Name: "notes", Path: "/workspace/notes"},
		{Name: "repo", Path: "/workspace/repo", Repo: true},
	}}
	reg, err := renderRailRegion(view, "ko")
	if err != nil {
		t.Fatal(err)
	}
	folder := railSection(t, string(reg.HTML), "/workspace/notes")
	for _, want := range []string{`class="rgit"`, `title="git init하기" aria-label="git init하기"`, `aria-haspopup="dialog" aria-expanded="false"`, `href="#i-gitinit"`} {
		if !strings.Contains(folder, want) {
			t.Errorf("plain folder lacks %s: %s", want, folder)
		}
	}
	if strings.Contains(folder, `class="rnew"`) || strings.Contains(railSection(t, string(reg.HTML), "/workspace/repo"), `class="rgit"`) {
		t.Fatal("git init and new-session buttons placed on wrong bars")
	}
	view.CanAdd = false
	reg, err = renderRailRegion(view, "ko")
	if err != nil || strings.Contains(string(reg.HTML), `class="rgit"`) {
		t.Fatalf("no workspace routes but git init shown (%v)", err)
	}
}
