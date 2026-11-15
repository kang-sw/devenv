package wsdashboard

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsholder"
)

func TestTerminalLocalBrowseAnchorGuards(t *testing.T) {
	manager := wsholder.New(wsholder.Options{Dir: filepath.Join(t.TempDir(), wsholder.DirName)})
	f := startPageFixture(t, lanMode, withHolders(manager))
	local, lan := fsViewers(t, f)
	base := "/s/" + LocalBrowseAnchor + "/terminals"
	for _, route := range terminalRoutes {
		target := strings.Replace(route.path, "/s/livesession/", "/s/"+LocalBrowseAnchor+"/", 1)
		want := http.StatusNotFound
		switch {
		case target == base && route.method == http.MethodGet:
			want = http.StatusOK
		case target == base && route.method == http.MethodPost:
			want = http.StatusBadRequest
		case strings.HasSuffix(target, "/ws"):
			want = http.StatusUpgradeRequired
		}
		if rec := serve(f.d, local, route.method, target, route.body); rec.Code != want {
			t.Errorf("local anchor %s %s = %d %s, want %d", route.method, target, rec.Code, rec.Body.String(), want)
		}
		for _, v := range []viewer{lan, {peer: local.peer, host: lan.host, cookie: lan.cookie}, {peer: lan.peer, host: local.host, cookie: lan.cookie}} {
			if rec := serve(f.d, v, route.method, target, route.body); rec.Code != http.StatusNotFound || rec.Body.String() != "404 page not found\n" {
				t.Errorf("non-local anchor %s %s = %d %q, want bare 404", route.method, target, rec.Code, rec.Body.String())
			}
		}
		for _, headers := range []map[string]string{{"Origin": "http://evil.example"}, {"Origin": "null"}, {"Sec-Fetch-Site": "cross-site"}, {"Origin": "http://" + local.host, "Sec-Fetch-Site": "cross-site"}} {
			if rec := termRequest(f.d, local.peer, local.host, route.method, target, route.body, headers); rec.Code != http.StatusForbidden {
				t.Errorf("anchor %s with %v = %d, want 403", target, headers, rec.Code)
			}
		}
		if route.method == http.MethodPost {
			if rec := termRequest(f.d, local.peer, local.host, route.method, target, route.body, nil); rec.Code != http.StatusForbidden {
				t.Errorf("anchor POST without Origin = %d, want 403", rec.Code)
			}
		}
	}
	for _, body := range []string{`{}`, `{"folder":""}`, `{"folder":"root"}`, `{"folder":"worktree"}`, `{"folder":"path"}`, `{"folder":"path","path":"relative"}`, `{"folder":"path","path":"~"}`} {
		if rec := serve(f.d, local, http.MethodPost, base, body); rec.Code != http.StatusBadRequest {
			t.Errorf("anchor create %s = %d, want 400", body, rec.Code)
		}
	}
	for _, folder := range []string{"", "root", "worktree"} {
		body, _ := json.Marshal(terminalCreate{Folder: folder, Path: f.project})
		if rec := serve(f.d, local, http.MethodPost, base, string(body)); rec.Code != http.StatusBadRequest {
			t.Errorf("anchor create folder %q with an existing path = %d, want 400", folder, rec.Code)
		}
	}
	writeFile(t, filepath.Join(f.project, "file.txt"), "not a directory")
	for _, dir := range []string{filepath.Join(f.project, "missing"), filepath.Join(f.project, "file.txt")} {
		body, _ := json.Marshal(terminalCreate{Folder: "path", Path: dir})
		if rec := serve(f.d, local, http.MethodPost, base, string(body)); rec.Code != http.StatusBadRequest {
			t.Errorf("anchor create path %q = %d, want 400", dir, rec.Code)
		}
	}
	for _, dir := range []string{"", "relative", "~", filepath.Join(f.project, "missing"), filepath.Join(f.project, "file.txt")} {
		if rec := serve(f.d, local, http.MethodGet, base+"?folder="+url.QueryEscape(dir), ""); rec.Code != http.StatusBadRequest {
			t.Errorf("anchor list folder %q = %d, want 400", dir, rec.Code)
		}
	}
	if got := manager.List().Holders; len(got) != 0 {
		t.Errorf("refused anchor requests started holders: %+v", got)
	}
	const ws = "/s/" + LocalBrowseAnchor + "/terminals/hguardtest/ws"
	for _, c := range []struct {
		headers map[string]string
		want    int
	}{
		{map[string]string{"Origin": "http://" + local.host}, http.StatusNotFound},
		{map[string]string{"Origin": "http://evil.example"}, http.StatusForbidden},
		{nil, http.StatusForbidden},
	} {
		if got := handshake(t, f.base, ws, local.host, c.headers); got != c.want {
			t.Errorf("anchor WebSocket handshake with %v = %d, want %d", c.headers, got, c.want)
		}
	}
}

func anchorTerminalCreate(t *testing.T, f *termFixture, dir string) TerminalInfo {
	t.Helper()
	body, _ := json.Marshal(terminalCreate{Folder: "path", Path: dir})
	code, answer := f.do(t, http.MethodPost, "/s/"+LocalBrowseAnchor+"/terminals", string(body))
	var info TerminalInfo
	if code != http.StatusCreated || json.Unmarshal([]byte(answer), &info) != nil || info.Session != LocalBrowseAnchor || info.Cwd != filepath.Clean(dir) {
		t.Fatalf("anchor create = %d %s, want an anchor terminal in %q", code, answer, dir)
	}
	return info
}

func TestTerminalLocalBrowseAnchorExactFolderAndRestart(t *testing.T) {
	holdersDir, python := termHoldersDir(t)
	f := startTermFixture(t, holdersDir, python, 0)
	work, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(work, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	parent := anchorTerminalCreate(t, f, work)
	child := anchorTerminalCreate(t, f, nested)
	own := f.create(t, `{}`)
	orphan, err := f.d.terminals.create("gonesession", work, termTestShell(), 14, 80)
	if err != nil {
		t.Fatal(err)
	}
	checkLists := func(f *termFixture) {
		t.Helper()
		code, answer := f.do(t, http.MethodGet, "/s/"+LocalBrowseAnchor+"/terminals", "")
		var all terminalList
		if code != http.StatusOK || json.Unmarshal([]byte(answer), &all) != nil || !strings.Contains(answer, `"folders":{}`) {
			t.Fatalf("unfiltered anchor answer = %d %s, want no session folders", code, answer)
		}
		if !all.Anchor || len(all.Terminals) != 2 || len(all.Orphans) != 0 || all.Folders != (terminalFolders{}) {
			t.Errorf("unfiltered anchor list = %+v, want only both anchor terminals and no session folders", all)
		}
		for _, c := range []struct{ dir, id string }{{work + string(filepath.Separator), parent.ID}, {filepath.Join(nested, ".."), parent.ID}, {nested, child.ID}} {
			code, answer := f.do(t, http.MethodGet, "/s/"+LocalBrowseAnchor+"/terminals?folder="+url.QueryEscape(c.dir), "")
			var list terminalList
			if code != http.StatusOK || json.Unmarshal([]byte(answer), &list) != nil || !list.Anchor || len(list.Terminals) != 1 || list.Terminals[0].ID != c.id || len(list.Orphans) != 0 || list.Folders != (terminalFolders{Path: filepath.Clean(c.dir)}) {
				t.Errorf("anchor folder %q = %d %s, want exactly terminal %s and that path only", c.dir, code, answer, c.id)
			}
			if strings.Contains(answer, `"worktree"`) || strings.Contains(answer, `"root"`) {
				t.Errorf("anchor list advertises a session folder: %s", answer)
			}
		}
		list := f.list(t, "livesession")
		if list.Anchor || len(list.Terminals) != 1 || list.Terminals[0].ID != own.ID || len(list.Orphans) != 1 || list.Orphans[0].ID != orphan.ID || list.Folders.Worktree != f.project {
			t.Errorf("real session list = %+v, want own terminal and genuine orphan, no anchor terminals", list)
		}
	}
	checkLists(f)
	legacy, err := json.Marshal(terminalList{Folders: terminalFolders{}})
	if err != nil || !strings.Contains(string(legacy), `"worktree":""`) || strings.Contains(string(legacy), `"anchor"`) {
		t.Errorf("legacy empty-project folders changed: %s (%v)", legacy, err)
	}
	// Attach from a real session is deliberately still holder-id based.
	// The shell itself, not only its descriptor, must start in the folder.
	c := f.open(t, parent.ID, 0)
	c.typeLine(t, "pwd; echo anchor-$((20+2))")
	c.waitText(t, "anchor-22")
	if !strings.Contains(c.text(), work) {
		t.Errorf("anchor shell output = %q, want cwd %q", c.text(), work)
	}
	c.conn.Close()
	f.d.Close()

	// A new isolated fixture daemon rediscovers the same holder-owned shells;
	// no agent session was ever registered for the host anchor.
	f2 := startTermFixture(t, holdersDir, python, 0)
	checkLists(f2)
	watch := f2.open(t, parent.ID, 0)
	watch.waitText(t, "anchor-22")
	watch.typeLine(t, "echo alive-$((2+3))")
	watch.waitText(t, "alive-5")
	if code, answer := f2.do(t, http.MethodPost, "/s/"+LocalBrowseAnchor+"/terminals/"+parent.ID+"/close", ""); code != http.StatusNoContent {
		t.Errorf("anchor close = %d %s, want 204", code, answer)
	}
	if list := f2.list(t, LocalBrowseAnchor); len(list.Terminals) != 1 || list.Terminals[0].ID != child.ID {
		t.Errorf("anchor list after close = %+v, want nested terminal only", list)
	}
}
