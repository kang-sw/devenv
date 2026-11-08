package wsdashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestReadGitChanges pins the changed-file read on a real repository: staged
// files (added, modified, deleted, binary), unstaged ones, a file changed on
// both sides in both groups, untracked entries (a directory once), each
// with its line delta, and the branch standing.
func TestReadGitChanges(t *testing.T) {
	work := filepath.Join(t.TempDir(), "work")
	pushRepos(t, work, 0)
	gitCommit(t, work, "b")
	gitCommit(t, work, "c")
	writeFile(t, filepath.Join(work, "a"), "x\ny\n")
	writeFile(t, filepath.Join(work, "s"), "1\n2\n")
	writeFile(t, filepath.Join(work, "b"), "b\nmore\n")
	writeFile(t, filepath.Join(work, "bin"), "\x00\x01\x02")
	gitRun(t, work, "add", "s", "b", "bin")
	gitRun(t, work, "rm", "-q", "c")
	writeFile(t, filepath.Join(work, "b"), "b\nmore\nagain\n")
	writeFile(t, filepath.Join(work, "u"), "u")
	writeFile(t, filepath.Join(work, "d", "x"), "x")

	c, err := readGitChanges(work)
	if err != nil {
		t.Fatal(err)
	}
	if c.Branch != "main" || c.Detached || c.Upstream != "origin/main" || c.Ahead != 2 || c.Behind != 0 || c.Truncated {
		t.Fatalf("standing = %+v", c)
	}
	wantStaged := []GitFile{{"b", "M", 2, 1}, {"bin", "A", -1, -1}, {"c", "D", 0, 1}, {"s", "A", 2, 0}}
	wantUnstaged := []GitFile{{"a", "M", 2, 1}, {"b", "M", 1, 0}}
	wantUntracked := []GitFile{{"d/", "?", 0, 0}, {"u", "?", 0, 0}}
	if !reflect.DeepEqual(c.Staged, wantStaged) {
		t.Errorf("staged = %+v, want %+v", c.Staged, wantStaged)
	}
	if !reflect.DeepEqual(c.Unstaged, wantUnstaged) {
		t.Errorf("unstaged = %+v, want %+v", c.Unstaged, wantUnstaged)
	}
	if !reflect.DeepEqual(c.Untracked, wantUntracked) {
		t.Errorf("untracked = %+v, want %+v", c.Untracked, wantUntracked)
	}

	gitRun(t, work, "checkout", "-q", "--detach")
	if c, err = readGitChanges(work); err != nil || !c.Detached || c.Branch == "" || c.Upstream != "" {
		t.Fatalf("detached = %+v, %v", c, err)
	}
	if _, err := readGitChanges(t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("non-repository: err = %v", err)
	}
}

// TestFillGitChanges pins the status parse on its own: an unmerged path is
// unstaged with status U, an ignored one is left out, a path with spaces
// keeps them, and each group stops at gitFilesMax, marking the list cut.
func TestFillGitChanges(t *testing.T) {
	c := GitChanges{}
	fillGitChanges(&c, []byte("UU m\x00AA n\x00!! ign\x00 M with space\x00"), nil, map[string][2]int{"with space": {3, 4}})
	want := []GitFile{{"m", "U", 0, 0}, {"n", "U", 0, 0}, {"with space", "M", 3, 4}}
	if !reflect.DeepEqual(c.Unstaged, want) || len(c.Staged) != 0 || len(c.Untracked) != 0 || c.Truncated {
		t.Fatalf("changes = %+v", c)
	}
	var many strings.Builder
	for i := range gitFilesMax + 5 {
		fmt.Fprintf(&many, "?? f%04d\x00", i)
	}
	c = GitChanges{}
	fillGitChanges(&c, []byte(many.String()), nil, nil)
	if len(c.Untracked) != gitFilesMax || !c.Truncated {
		t.Fatalf("untracked %d, truncated %v", len(c.Untracked), c.Truncated)
	}
}

// TestPullGit pins the pull on real repositories: dirty tracked files and a
// diverged branch are refused and move nothing, untracked files alone do
// not make the tree dirty, a branch behind fast-forwards to its upstream,
// one already there finds nothing to pull, and a detached HEAD and a branch
// without an upstream are refused.
func TestPullGit(t *testing.T) {
	base := t.TempDir()
	work := filepath.Join(base, "work")
	origin := pushRepos(t, work, 0)
	other := filepath.Join(base, "other")
	gitRun(t, base, "clone", "-q", origin, other)
	gitCommit(t, other, "theirs")
	gitRun(t, other, "push", "-q", "origin", "main")

	head := revParse(t, work, "HEAD")
	writeFile(t, filepath.Join(work, "a"), "changed")
	if outcome, err := pullGit(work); err != nil || outcome != pullDirty {
		t.Fatalf("dirty: pullGit = %v, %v; want pullDirty", outcome, err)
	}
	gitRun(t, work, "add", "a")
	if outcome, err := pullGit(work); err != nil || outcome != pullDirty {
		t.Fatalf("staged: pullGit = %v, %v; want pullDirty", outcome, err)
	}
	gitRun(t, work, "reset", "-q", "--hard")
	if got := revParse(t, work, "HEAD"); got != head {
		t.Fatalf("a refused pull moved HEAD to %s", got)
	}

	writeFile(t, filepath.Join(work, "untracked"), "u")
	if outcome, err := pullGit(work); err != nil || outcome != pullRan {
		t.Fatalf("behind: pullGit = %v, %v; want pullRan", outcome, err)
	}
	if got, want := revParse(t, work, "HEAD"), revParse(t, origin, "main"); got != want {
		t.Fatalf("HEAD after the pull = %s, want %s", got, want)
	}
	if outcome, err := pullGit(work); err != nil || outcome != pullCurrent {
		t.Fatalf("current: pullGit = %v, %v; want pullCurrent", outcome, err)
	}

	gitCommit(t, other, "theirs2")
	gitRun(t, other, "push", "-q", "origin", "main")
	gitCommit(t, work, "mine")
	head = revParse(t, work, "HEAD")
	if outcome, err := pullGit(work); err != nil || outcome != pullDiverged {
		t.Fatalf("diverged: pullGit = %v, %v; want pullDiverged", outcome, err)
	}
	if got := revParse(t, work, "HEAD"); got != head {
		t.Fatalf("a diverged pull moved HEAD to %s", got)
	}

	gitRun(t, work, "checkout", "-q", "-b", "topic")
	if outcome, err := pullGit(work); err != nil || outcome != pullNoUpstream {
		t.Errorf("no upstream: pullGit = %v, %v; want pullNoUpstream", outcome, err)
	}
	gitRun(t, work, "checkout", "-q", "--detach")
	if outcome, err := pullGit(work); err != nil || outcome != pullDetached {
		t.Errorf("detached: pullGit = %v, %v; want pullDetached", outcome, err)
	}
}

// TestFetchGit: a fetch brings the upstream's new commit, which the next
// read shows as behind, and moves nothing else.
func TestFetchGit(t *testing.T) {
	base := t.TempDir()
	work := filepath.Join(base, "work")
	origin := pushRepos(t, work, 0)
	other := filepath.Join(base, "other")
	gitRun(t, base, "clone", "-q", origin, other)
	gitCommit(t, other, "theirs")
	gitRun(t, other, "push", "-q", "origin", "main")

	head := revParse(t, work, "HEAD")
	if b := readGitBadge(work); b.Behind != 0 {
		t.Fatalf("behind before the fetch = %d", b.Behind)
	}
	if err := fetchGit(work); err != nil {
		t.Fatal(err)
	}
	if b := readGitBadge(work); b.Behind != 1 || b.Ahead != 0 {
		t.Fatalf("badge after the fetch = %+v", b)
	}
	if got := revParse(t, work, "HEAD"); got != head {
		t.Fatalf("a fetch moved HEAD to %s", got)
	}
	if err := fetchGit(t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("non-repository: err = %v", err)
	}
}

// TestBranchHandle pins the handle's abbreviation.
func TestBranchHandle(t *testing.T) {
	for _, c := range []struct {
		branch   string
		detached bool
		want     GitHandle
	}{
		{"impl/develop/261108-feat-x", false, GitHandle{Impl: true, Text: "261108-feat-x"}},
		{"impl/feature/nested/stem", false, GitHandle{Impl: true, Text: "stem"}},
		{"impl/stem", false, GitHandle{Impl: true, Text: "stem"}},
		{"goal/main/ship-it", false, GitHandle{Goal: true, Text: "ship-it"}},
		{"goal/main/g/impl/goal/main/g/fix-it", false, GitHandle{Impl: true, Goal: true, Text: "fix-it"}},
		{"impl/goal/main/g/fix-it", false, GitHandle{Impl: true, Goal: true, Text: "fix-it"}},
		{"develop", false, GitHandle{Text: "develop"}},
		{"delegate/git-drawer", false, GitHandle{Text: "delegate/git-drawer"}},
		{"feature/impl/x", false, GitHandle{Text: "feature/impl/x"}},
		{"impl/", false, GitHandle{Text: "impl/"}},
		{"impl", false, GitHandle{Text: "impl"}},
		{"abc1234", true, GitHandle{Text: "abc1234"}},
	} {
		if got := branchHandle(c.branch, c.detached); got != c.want {
			t.Errorf("branchHandle(%q, %v) = %+v, want %+v", c.branch, c.detached, got, c.want)
		}
	}
	if h := (GitBadge{State: gitPending}).Handle(); h != (GitHandle{}) {
		t.Errorf("a pending badge's handle = %+v", h)
	}
}

// TestTicketTitles pins the ticket-title lookup: the front matter's title
// (quotes dropped), a first heading without one, nothing for a ticket with
// neither, an unknown stem, or a stem that would leave the board; and the
// cache, which answers until its time to live ends.
func TestTicketTitles(t *testing.T) {
	root := t.TempDir()
	tickets := filepath.Join(root, "ai-docs", "tickets")
	writeFile(t, filepath.Join(tickets, "ready", "261108-feat-x.md"), "---\ntitle: \"The x feature: \\\"quoted\\\"\"\nparent: y\n---\n\n# Heading\n")
	writeFile(t, filepath.Join(tickets, ".done", "261101-old.md"), "# Old one\n\nbody\n")
	writeFile(t, filepath.Join(tickets, "idea", "261102-none.md"), "---\nrelated:\n  title: nested\n---\nbody\n")
	writeFile(t, filepath.Join(root, "ai-docs", "secret.md"), "# not a ticket\n")

	c := &ticketTitleCache{entries: map[string]ticketTitleEntry{}}
	now := time.Now()
	for stem, want := range map[string]string{
		"261108-feat-x": `The x feature: "quoted"`,
		"261101-old":    "Old one",
		"261102-none":   "",
		"261103-absent": "",
		"../secret":     "",
		"..":            "",
		"":              "",
	} {
		if got := c.title(root, stem, now); got != want {
			t.Errorf("title(%q) = %q, want %q", stem, got, want)
		}
	}
	writeFile(t, filepath.Join(tickets, "ready", "261108-feat-x.md"), "---\ntitle: renamed\n---\n")
	if got := c.title(root, "261108-feat-x", now.Add(ticketTitleTTL/2)); got != `The x feature: "quoted"` {
		t.Errorf("a fresh cached title = %q", got)
	}
	if got := c.title(root, "261108-feat-x", now.Add(2*ticketTitleTTL)); got != "renamed" {
		t.Errorf("a stale cached title = %q, want renamed", got)
	}
}

// TestReadGitBadgeResolvesTheTicket: an impl branch's read carries the
// title of the ticket its stem names in the root's board.
func TestReadGitBadgeResolvesTheTicket(t *testing.T) {
	work := filepath.Join(t.TempDir(), "work")
	pushRepos(t, work, 0)
	gitRun(t, work, "checkout", "-q", "-b", "impl/main/261108-feat-y")
	writeFile(t, filepath.Join(work, "ai-docs", "tickets", "todo", "261108-feat-y.md"), "---\ntitle: The y feature\n---\n")
	if b := readGitBadge(work); b.State != gitOK || b.Ticket != "The y feature" || !b.Handle().Impl {
		t.Fatalf("badge = %+v", b)
	}
	gitRun(t, work, "checkout", "-q", "main")
	if b := readGitBadge(work); b.Ticket != "" {
		t.Fatalf("a plain branch's ticket = %q", b.Ticket)
	}
}

// TestGitDrawerRoutes pins the drawer's routes end to end on real
// repositories: the status read, and the pull and fetch actions under the
// push route's rules (POST, the daemon's own Origin, a known session and
// action), each answering with the reason the drawer shows.
func TestGitDrawerRoutes(t *testing.T) {
	f := startPageFixture(t, func(o *DaemonOptions) { o.noGitBadge = false })
	f.d.git.gap = 0
	base := t.TempDir()
	work := f.project
	origin := pushRepos(t, work, 0)
	other := filepath.Join(base, "other")
	gitRun(t, base, "clone", "-q", origin, other)
	status := func() GitChanges {
		t.Helper()
		code, raw := get(t, f.base+"/s/livesession/git/status")
		if code != http.StatusOK {
			t.Fatalf("status = %d %s", code, raw)
		}
		var c GitChanges
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatalf("status answer %q: %v", raw, err)
		}
		return c
	}
	post := func(action string) (int, string) {
		t.Helper()
		code, _, raw := postJSON(t, f.base+"/s/livesession/git/"+action, f.base, "")
		return code, strings.TrimSpace(raw)
	}

	writeFile(t, filepath.Join(work, "u"), "u")
	if c := status(); c.Branch != "main" || c.Upstream != "origin/main" || len(c.Untracked) != 1 || c.Untracked[0].Path != "u" {
		t.Fatalf("status = %+v", c)
	}
	if code, _ := get(t, f.base+"/s/nosuchsession/git/status"); code != http.StatusNotFound {
		t.Errorf("status of an unknown session = %d, want 404", code)
	}

	for _, action := range []string{"pull", "fetch"} {
		route := f.base + "/s/livesession/git/" + action
		for _, o := range []string{"", "null", "http://evil.example"} {
			if code, header, _ := postJSON(t, route, o, ""); code != http.StatusForbidden {
				t.Errorf("%s with Origin %q = %d, want 403", action, o, code)
			} else {
				assertNoCORS(t, header)
			}
		}
		if code, _ := get(t, route); code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s = %d, want 405", action, code)
		}
		if code, _, _ := postJSON(t, f.base+"/s/nosuchsession/git/"+action, f.base, ""); code != http.StatusNotFound {
			t.Errorf("%s of an unknown session = %d, want 404", action, code)
		}
	}
	if code, _ := post("commit"); code == http.StatusOK || code == http.StatusNoContent {
		t.Errorf("a commit action answered %d", code)
	}

	gitCommit(t, other, "theirs")
	gitRun(t, other, "push", "-q", "origin", "main")
	if code, raw := post("fetch"); code != http.StatusNoContent {
		t.Fatalf("fetch = %d %q", code, raw)
	}
	if c := status(); c.Behind != 1 {
		t.Fatalf("behind after the fetch = %d", c.Behind)
	}

	writeFile(t, filepath.Join(work, "a"), "changed")
	if code, raw := post("pull"); code != http.StatusConflict || raw != "uncommitted changes: commit or stash them before pulling" {
		t.Fatalf("dirty pull = %d %q", code, raw)
	}
	gitRun(t, work, "checkout", "-q", "--", "a")
	if code, raw := post("pull"); code != http.StatusNoContent {
		t.Fatalf("pull = %d %q", code, raw)
	}
	if got, want := revParse(t, work, "HEAD"), revParse(t, origin, "main"); got != want {
		t.Fatalf("HEAD after the pull = %s, want %s", got, want)
	}
	if code, raw := post("pull"); code != http.StatusOK || raw != "already up to date" {
		t.Fatalf("pull with nothing to pull = %d %q", code, raw)
	}

	gitCommit(t, other, "theirs2")
	gitRun(t, other, "push", "-q", "origin", "main")
	gitCommit(t, work, "mine")
	if code, raw := post("pull"); code != http.StatusConflict || !strings.Contains(raw, "diverged") {
		t.Fatalf("diverged pull = %d %q", code, raw)
	}

	release, _ := f.d.beginGitAction(work)
	if code, raw := post("fetch"); code != http.StatusConflict || !strings.Contains(raw, "another git action") {
		t.Fatalf("fetch while another action runs = %d %q", code, raw)
	}
	release()
}
