package wsdashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestLayoutLanes pins the lane layout on a hand-made graph: a merge opens
// a lane for its second parent, a branch tip takes a new lane and color, a
// branch forked from a commit another lane continues through runs beside
// it to that commit and meets it there, two lanes waiting for one commit
// meet at its row, and a root commit closes its lane.
//
//	M merges D and C; F is a branch tip on D; C and D fork from B; A is the
//	root.
func TestLayoutLanes(t *testing.T) {
	rows := []laneCommit{
		{"M", []string{"D", "C"}},
		{"F", []string{"D"}},
		{"C", []string{"B"}},
		{"D", []string{"B"}},
		{"B", []string{"A"}},
		{"A", nil},
	}
	got := layoutLanes(rows)
	if want := []int{0, 2, 1, 0, 0, 0}; !reflect.DeepEqual(got.cols, want) {
		t.Errorf("cols = %v, want %v", got.cols, want)
	}
	if want := []int{0, 2, 1, 0, 0, 0}; !reflect.DeepEqual(got.colors, want) {
		t.Errorf("colors = %v, want %v", got.colors, want)
	}
	want := [][]GitLane{
		{{0, 0, 0}, {0, 1, 1}},            // under M: first parent straight down, second parent out to lane 1
		{{0, 0, 0}, {1, 1, 1}, {2, 2, 2}}, // under F, which opened lane 2
		{{0, 0, 0}, {1, 1, 1}, {2, 0, 2}}, // under C: F's lane bends into D's node
		{{0, 0, 0}, {1, 0, 1}},            // under D: C's lane bends into B's node
		{{0, 0, 0}},                       // under B
		{},                                // under A: the root closed the last lane
	}
	if !reflect.DeepEqual(got.down, want) {
		t.Errorf("down = %v, want %v", got.down, want)
	}

	// A lane freed in the middle is reused by the next new lane.
	got = layoutLanes([]laneCommit{
		{"X", []string{"P"}},
		{"Y", []string{"Q"}},
		{"Z", []string{"R"}},
		{"Q", nil},
		{"W", []string{"S"}},
	})
	if want := []int{0, 1, 2, 1, 1}; !reflect.DeepEqual(got.cols, want) {
		t.Errorf("reuse: cols = %v, want %v", got.cols, want)
	}
	if want := []GitLane{{0, 0, 0}, {2, 2, 2}}; !reflect.DeepEqual(got.down[3], want) {
		t.Errorf("reuse: under the root Q = %v, want %v", got.down[3], want)
	}
}

// checkLanes asserts the graph's lines are continuous: parents come after
// their children; every segment under a row starts at its node or at a lane
// arriving from the row above; every lane arriving from above and not
// ending at the node continues below; and the node starts one segment per
// parent.
func checkLanes(t *testing.T, cs []GitLogCommit) {
	t.Helper()
	pos := map[string]int{}
	for i, c := range cs {
		pos[c.OID] = i
	}
	for i, c := range cs {
		for _, p := range c.Parents {
			if j, ok := pos[p]; ok && j <= i {
				t.Errorf("row %d (%s): parent %s at row %d", i, c.Subject, p, j)
			}
		}
		arriving := map[int]bool{}
		if i > 0 {
			for _, s := range cs[i-1].Down {
				if s.To != c.Col {
					arriving[s.To] = true
				}
			}
		}
		fromNode := 0
		continues := map[int]bool{}
		for _, s := range c.Down {
			switch {
			case s.From == c.Col:
				fromNode++
			case arriving[s.From]:
				continues[s.From] = true
			default:
				t.Errorf("row %d (%s): segment %+v starts at no lane", i, c.Subject, s)
			}
		}
		if fromNode != len(c.Parents) {
			t.Errorf("row %d (%s): %d segments leave the node, want one per parent (%d)", i, c.Subject, fromNode, len(c.Parents))
		}
		for k := range arriving {
			if !continues[k] {
				t.Errorf("row %d (%s): the lane arriving in column %d ends", i, c.Subject, k)
			}
		}
	}
}

// graphRepo builds a repository for the graph reads and returns its work
// tree, the impl branch's worktree, and the detached worktree:
//
//   - origin/main is one commit behind (theirs) and main several ahead;
//   - main holds a no-ff merge of side, forked from the first commit;
//   - impl/main/261109-feat-x forks from b and is checked out in a worktree;
//   - a second worktree is detached at b.
func graphRepo(t *testing.T) (work, implWT, detWT string) {
	t.Helper()
	base := t.TempDir()
	work = filepath.Join(base, "work")
	origin := pushRepos(t, work, 0)
	other := filepath.Join(base, "other")
	gitRun(t, base, "clone", "-q", origin, other)
	gitCommit(t, other, "theirs")
	gitRun(t, other, "push", "-q", "origin", "main")
	gitRun(t, work, "fetch", "-q")
	gitRun(t, work, "checkout", "-q", "-b", "side")
	gitCommit(t, work, "s1")
	gitCommit(t, work, "s2")
	gitRun(t, work, "checkout", "-q", "main")
	gitCommit(t, work, "b")
	gitRun(t, work, "merge", "-q", "--no-ff", "-m", "merge side", "side")
	gitCommit(t, work, "c")
	implWT = filepath.Join(base, "impl")
	gitRun(t, work, "worktree", "add", "-q", "-b", "impl/main/261109-feat-x", implWT, "main~2")
	gitCommit(t, implWT, "x1")
	detWT = filepath.Join(base, "det")
	gitRun(t, work, "worktree", "add", "-q", "--detach", detWT, "main~2")
	return work, implWT, detWT
}

func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// TestReadGitLog pins the graph read on a real repository: every local
// branch and its upstream are in it, with their labels (the root's branch
// marked head, an impl branch's handle abbreviation, each checkout's
// worktree, a detached worktree's HEAD, the upstream's commit main does not
// have), and its lines are continuous down to the root commit.
func TestReadGitLog(t *testing.T) {
	work, implWT, detWT := graphRepo(t)
	g, err := readGitLog(work, 0, gitLogPage)
	if err != nil {
		t.Fatal(err)
	}
	if g.More != 0 || g.Head != revParse(t, work, "HEAD") || g.Width < 2 {
		t.Fatalf("log = head %s, more %d, width %d", g.Head, g.More, g.Width)
	}
	by := map[string]GitLogCommit{}
	var subjects []string
	for _, c := range g.Commits {
		by[c.Subject] = c
		subjects = append(subjects, c.Subject)
		if c.Author != "t" || c.Time <= 0 || len(c.OID) < 40 {
			t.Errorf("commit %+v", c)
		}
	}
	for _, s := range []string{"a", "theirs", "s1", "s2", "b", "merge side", "c", "x1"} {
		if _, ok := by[s]; !ok {
			t.Errorf("commit %q missing from %v", s, subjects)
		}
	}
	if len(g.Commits) != 8 {
		t.Errorf("commits = %v", subjects)
	}
	checkLanes(t, g.Commits)
	if last := g.Commits[len(g.Commits)-1]; last.Subject != "a" || len(last.Down) != 0 {
		t.Errorf("last row = %+v, want the root commit with no lane below", last)
	}

	refs := func(subject string) []GitLogRef { return by[subject].Refs }
	if r := refs("c"); len(r) != 1 || r[0].Name != "main" || r[0].Kind != "branch" || !r[0].Head || r[0].Impl || !sameDir(t, r[0].Worktree, work) {
		t.Errorf("refs of c = %+v", r)
	}
	if r := refs("theirs"); len(r) != 1 || r[0].Name != "origin/main" || r[0].Kind != "upstream" || r[0].Head || r[0].Worktree != "" {
		t.Errorf("refs of theirs = %+v", r)
	}
	if r := refs("x1"); len(r) != 1 || r[0].Name != "impl/main/261109-feat-x" || !r[0].Impl || r[0].Goal || r[0].Text != "261109-feat-x" || !sameDir(t, r[0].Worktree, implWT) {
		t.Errorf("refs of x1 = %+v", r)
	}
	if r := refs("s2"); len(r) != 1 || r[0].Name != "side" || r[0].Worktree != "" || r[0].Text != "" {
		t.Errorf("refs of s2 = %+v", r)
	}
	if r := refs("b"); len(r) != 1 || r[0].Kind != "detached" || r[0].Name != "" || r[0].Head || !sameDir(t, r[0].Worktree, detWT) {
		t.Errorf("refs of b = %+v", r)
	}
	if m := by["merge side"]; len(m.Parents) != 2 || by["s2"].Col == m.Col {
		t.Errorf("merge = %+v, side tip in column %d", m, by["s2"].Col)
	}
	if by["x1"].Col == by["c"].Col {
		t.Errorf("the impl branch shares main's column %d", by["c"].Col)
	}

	// A detached root marks its HEAD; its branches still draw.
	gitRun(t, work, "checkout", "-q", "--detach", "main~1")
	if g, err = readGitLog(work, 0, gitLogPage); err != nil {
		t.Fatal(err)
	}
	for _, c := range g.Commits {
		for _, r := range c.Refs {
			if r.Head && (r.Kind != "detached" || c.OID != g.Head || !sameDir(t, r.Worktree, work)) {
				t.Errorf("head ref %+v on %s (head %s)", r, c.Subject, g.Head)
			}
		}
	}
	if len(g.Commits) != 8 {
		t.Errorf("detached root: %d commits", len(g.Commits))
	}

	if _, err := readGitLog(t.TempDir(), 0, gitLogPage); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("non-repository: err = %v", err)
	}
}

// TestReadGitLogPaging: pages laid out from the top stack into the whole
// log, so the lanes continue across every page boundary, and the last page
// says there is no more.
func TestReadGitLogPaging(t *testing.T) {
	work, _, _ := graphRepo(t)
	full, err := readGitLog(work, 0, gitLogPage)
	if err != nil {
		t.Fatal(err)
	}
	var stacked []GitLogCommit
	offset, pages := 0, 0
	for {
		g, err := readGitLog(work, offset, 3)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		if len(g.Commits) == 0 || len(g.Commits) > 3 || g.Width > full.Width {
			t.Fatalf("page at %d: %d commits, width %d", offset, len(g.Commits), g.Width)
		}
		stacked = append(stacked, g.Commits...)
		if g.More == 0 {
			break
		}
		if g.More != offset+3 {
			t.Fatalf("page at %d: more = %d", offset, g.More)
		}
		offset = g.More
	}
	if pages != 3 {
		t.Errorf("pages = %d, want 3", pages)
	}
	if !reflect.DeepEqual(stacked, full.Commits) {
		t.Fatalf("stacked pages differ from the whole log:\n%+v\n%+v", stacked, full.Commits)
	}
	checkLanes(t, stacked)
	// The boundary itself: the last row of the first page leads into the
	// first row of the second.
	if into := stacked[2].Down; len(into) == 0 {
		t.Errorf("no lane leaves the first page")
	}
}

// TestReadGitCommit pins the commit read: metadata and the changed files
// with status, rename source, and line delta (binary -1); a merge diffs
// against its first parent and a root commit against the empty tree; an
// unknown or malformed id names no commit.
func TestReadGitCommit(t *testing.T) {
	work := filepath.Join(t.TempDir(), "work")
	pushRepos(t, work, 0)
	lines := strings.Repeat("line one\nline two\nline three\nline four\n", 4)
	writeFile(t, filepath.Join(work, "old.txt"), lines)
	writeFile(t, filepath.Join(work, "gone"), "1\n2\n")
	writeFile(t, filepath.Join(work, "mod"), "m\n")
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-q", "-m", "seed")
	gitRun(t, work, "mv", "old.txt", "new.txt")
	writeFile(t, filepath.Join(work, "new.txt"), lines+"tail\n")
	gitRun(t, work, "rm", "-q", "gone")
	writeFile(t, filepath.Join(work, "mod"), "m\nn\n")
	writeFile(t, filepath.Join(work, "bin"), "\x00\x01\x02")
	gitRun(t, work, "add", ".")
	gitRun(t, work, "commit", "-q", "-m", "subject line\n\nbody one\nbody two")

	head := revParse(t, work, "HEAD")
	c, err := readGitCommit(work, head[:10])
	if err != nil {
		t.Fatal(err)
	}
	if c.OID != head || c.Base != revParse(t, work, "HEAD~1") || !reflect.DeepEqual(c.Parents, []string{c.Base}) ||
		c.Message != "subject line\n\nbody one\nbody two" || c.Author != "t" || c.AuthorEmail != "t@example.com" ||
		c.Committer != "t" || c.CommitterEmail != "t@example.com" || c.AuthorTime <= 0 || c.CommitTime <= 0 || c.Truncated {
		t.Fatalf("commit = %+v", c)
	}
	want := []GitCommitFile{
		{Path: "bin", Status: "A", Add: -1, Del: -1},
		{Path: "gone", Status: "D", Add: 0, Del: 2},
		{Path: "mod", Status: "M", Add: 1, Del: 0},
		{Path: "new.txt", Status: "R", From: "old.txt", Add: 1, Del: 0},
	}
	if !reflect.DeepEqual(c.Files, want) {
		t.Errorf("files = %+v, want %+v", c.Files, want)
	}

	// A merge shows what it brought into its first parent's line.
	gitRun(t, work, "checkout", "-q", "-b", "side")
	gitCommit(t, work, "s1")
	gitRun(t, work, "checkout", "-q", "main")
	gitCommit(t, work, "m1")
	gitRun(t, work, "merge", "-q", "--no-ff", "-m", "merge", "side")
	if c, err = readGitCommit(work, revParse(t, work, "HEAD")); err != nil {
		t.Fatal(err)
	}
	if len(c.Parents) != 2 || c.Base != revParse(t, work, "HEAD^1") || !reflect.DeepEqual(c.Files, []GitCommitFile{{Path: "s1", Status: "A", Add: 1, Del: 0}}) {
		t.Errorf("merge = %+v", c)
	}

	root := revParse(t, work, "HEAD~4")
	if c, err = readGitCommit(work, root); err != nil {
		t.Fatal(err)
	}
	if len(c.Parents) != 0 || c.Base != "" || !reflect.DeepEqual(c.Files, []GitCommitFile{{Path: "a", Status: "A", Add: 1, Del: 0}}) {
		t.Errorf("root = %+v", c)
	}

	for _, oid := range []string{"0000000000", "HEAD", "main", "--all", "abc", "ab-cd", strings.Repeat("a", 65), revParse(t, work, "HEAD^{tree}")} {
		if _, err := readGitCommit(work, oid); err != errNoCommit {
			t.Errorf("readGitCommit(%q) err = %v, want errNoCommit", oid, err)
		}
	}
	// A hex-shaped branch name is no object id.
	if head := revParse(t, work, "HEAD"); !strings.HasPrefix(head, "abcdef") {
		gitRun(t, work, "branch", "abcdef")
		if _, err := readGitCommit(work, "abcdef"); err != errNoCommit {
			t.Errorf("a branch named abcdef: err = %v, want errNoCommit", err)
		}
	}
	if _, err := readGitCommit(t.TempDir(), "abcd"); err == nil || err == errNoCommit {
		t.Errorf("non-repository: err = %v", err)
	}
}

// TestParseCommitFilesCutsTheList: the file list stops at
// gitCommitFilesMax and says so.
func TestParseCommitFilesCutsTheList(t *testing.T) {
	var b strings.Builder
	for i := 0; i < gitCommitFilesMax+3; i++ {
		b.WriteString("A\x00f" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + "\x00")
	}
	files, cut := parseCommitFiles([]byte(b.String()), nil)
	if len(files) != gitCommitFilesMax || !cut {
		t.Fatalf("files %d, cut %v", len(files), cut)
	}
}

// TestGitLogRoutes pins the graph and commit routes end to end: GET only,
// a known session, a well-formed offset, and an object id that is hex and
// names a commit.
func TestGitLogRoutes(t *testing.T) {
	f := startPageFixture(t)
	work := f.project
	pushRepos(t, work, 0)
	gitCommit(t, work, "b")
	gitClient := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}}
	read := func(path string) (int, string) {
		t.Helper()
		resp, err := gitClient.Get(f.base + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK && resp.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s: content type %q", path, resp.Header.Get("Content-Type"))
		}
		return resp.StatusCode, string(body)
	}

	code, raw := read("/s/livesession/git/log")
	var g GitLog
	if code != http.StatusOK || json.Unmarshal([]byte(raw), &g) != nil || len(g.Commits) != 2 || g.Commits[0].Subject != "b" || g.More != 0 {
		t.Fatalf("log = %d %s", code, raw)
	}
	if code, raw = read("/s/livesession/git/log?offset=1"); code != http.StatusOK || json.Unmarshal([]byte(raw), &g) != nil || len(g.Commits) != 1 || g.Commits[0].Subject != "a" {
		t.Fatalf("log at offset 1 = %d %s", code, raw)
	}
	for _, q := range []string{"-1", "x", "20001"} {
		if code, _ := read("/s/livesession/git/log?offset=" + q); code != http.StatusBadRequest {
			t.Errorf("offset %q = %d, want 400", q, code)
		}
	}

	head := revParse(t, work, "HEAD")
	var c GitCommit
	if code, raw = read("/s/livesession/git/commit/" + head); code != http.StatusOK || json.Unmarshal([]byte(raw), &c) != nil || c.OID != head || len(c.Files) != 1 || c.Files[0].Path != "b" {
		t.Fatalf("commit = %d %s", code, raw)
	}
	for _, oid := range []string{"HEAD", "--output=x", "deadbeef", "", head + "/x", "main"} {
		if code, _ := read("/s/livesession/git/commit/" + oid); code != http.StatusNotFound {
			t.Errorf("commit %q = %d, want 404", oid, code)
		}
	}

	for _, route := range []string{"/s/livesession/git/log", "/s/livesession/git/commit/" + head} {
		if code, _, _ := postJSON(t, f.base+route, f.base, ""); code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s = %d, want 405", route, code)
		}
	}
	for _, route := range []string{"/s/nosuchsession/git/log", "/s/nosuchsession/git/commit/" + head} {
		if code, _ := read(route); code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", route, code)
		}
	}
}
