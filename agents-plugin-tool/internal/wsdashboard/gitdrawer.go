package wsdashboard

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// The git drawer's reads: the session root's changed files, which the page
// asks for each time the drawer opens and after each action (GET
// /s/<reply-id>/git/status), and the title of the ticket an impl or goal
// branch's stem names, which the handle's tooltip shows. Neither ever fetches
// or takes git's optional locks. The drawer's actions (push, pull, fetch) are
// in gitpush.go.

const (
	// gitFilesMax bounds each group of the changed-file list.
	gitFilesMax = 300
)

// GitChanges is the drawer's changed-file list for one root, with the branch
// standing its action panel shows.
type GitChanges struct {
	// Branch is the branch name, or the short commit id when Detached.
	Branch   string `json:"branch"`
	Detached bool   `json:"detached,omitempty"`
	Upstream string `json:"upstream,omitempty"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
	// Staged, Unstaged, and Untracked are the changed files, by path. A file
	// changed in both the index and the working tree is in both groups; an
	// unmerged path is unstaged, with status "U".
	Staged    []GitFile `json:"staged"`
	Unstaged  []GitFile `json:"unstaged"`
	Untracked []GitFile `json:"untracked"`
	// Truncated reports a group cut at gitFilesMax.
	Truncated bool `json:"truncated,omitempty"`
}

// GitFile is one changed file: its path, its one-letter status (git's X or Y
// column: M, A, D, T, U; "?" untracked), and its line delta from git diff
// --numstat (Add and Del -1 for a binary file, 0 for an untracked one).
type GitFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Add    int    `json:"add"`
	Del    int    `json:"del"`
}

// readGitChanges reads root's branch standing and changed files: one status
// and two numstat diffs (worktree against index, index against HEAD), with
// renames off so each path counts on its own.
func readGitChanges(root string) (GitChanges, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitReadTimeout)
	defer cancel()
	run := &gitBadgeRunner{}
	st, ok := wsgit.BranchSync(ctx, run, root)
	if !ok {
		if run.err == nil {
			return GitChanges{}, fmt.Errorf("HEAD names no commit")
		}
		return GitChanges{}, fmt.Errorf("%s", gitErrText(run.err))
	}
	c := GitChanges{Branch: st.Head, Upstream: st.Upstream, Ahead: st.Ahead, Behind: st.Behind,
		Staged: []GitFile{}, Unstaged: []GitFile{}, Untracked: []GitFile{}}
	if st.Head == "" {
		c.Branch, c.Detached = st.OID, true
	}
	status, err := run.RunGit(ctx, root, "--no-optional-locks", "status", "--porcelain=v1", "-z", "--no-renames", "--untracked-files=normal")
	if err != nil {
		return GitChanges{}, fmt.Errorf("%s", gitErrText(err))
	}
	unstaged, err := run.RunGit(ctx, root, "--no-optional-locks", "diff", "--numstat", "-z", "--no-renames")
	if err != nil {
		return GitChanges{}, fmt.Errorf("%s", gitErrText(err))
	}
	staged, err := run.RunGit(ctx, root, "--no-optional-locks", "diff", "--cached", "--numstat", "-z", "--no-renames")
	if err != nil {
		return GitChanges{}, fmt.Errorf("%s", gitErrText(err))
	}
	fillGitChanges(&c, status, parseNumstat(staged), parseNumstat(unstaged))
	return c, nil
}

// fillGitChanges sorts the entries of a `status --porcelain=v1 -z` output
// into c's groups, each file with its delta from the numstat of its side.
func fillGitChanges(c *GitChanges, status []byte, staged, unstaged map[string][2]int) {
	add := func(group *[]GitFile, path, code string, delta map[string][2]int) {
		if len(*group) >= gitFilesMax {
			c.Truncated = true
			return
		}
		f := GitFile{Path: path, Status: code}
		if d, ok := delta[path]; ok {
			f.Add, f.Del = d[0], d[1]
		}
		*group = append(*group, f)
	}
	fields := strings.Split(string(status), "\x00")
	for i := 0; i < len(fields); i++ {
		e := fields[i]
		if len(e) < 4 {
			continue
		}
		x, y, path := e[0], e[1], e[3:]
		if x == 'R' || x == 'C' {
			i++ // the original path follows; renames are off, so this is defensive
		}
		switch {
		case x == '?' && y == '?':
			add(&c.Untracked, path, "?", nil)
		case x == '!' && y == '!':
		case x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D'):
			add(&c.Unstaged, path, "U", unstaged)
		default:
			if x != ' ' {
				add(&c.Staged, path, string(x), staged)
			}
			if y != ' ' {
				add(&c.Unstaged, path, string(y), unstaged)
			}
		}
	}
}

// parseNumstat reads a `diff --numstat -z --no-renames` output: path to
// added and deleted lines, -1 for both on a binary file.
func parseNumstat(out []byte) map[string][2]int {
	m := map[string][2]int{}
	for _, e := range strings.Split(string(out), "\x00") {
		a, rest, ok := strings.Cut(e, "\t")
		if !ok {
			continue
		}
		d, path, ok := strings.Cut(rest, "\t")
		if !ok || path == "" {
			continue
		}
		na, errA := strconv.Atoi(a)
		nd, errD := strconv.Atoi(d)
		if errA != nil || errD != nil {
			na, nd = -1, -1
		}
		m[path] = [2]int{na, nd}
	}
	return m
}

// handleGitStatus serves GET /s/<reply-id>/git/status: the session root's
// changed files as JSON (GitChanges), or 502 with git's reason.
func (d *Daemon) handleGitStatus(w http.ResponseWriter, r *http.Request, replyID string) {
	if d.index == nil || !ValidReplyID(replyID) {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	s, ok := d.index.Session(replyID)
	if !ok || s.Project == "" {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	c, err := readGitChanges(s.Project)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	raw, _ := json.Marshal(c)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(raw)
}

// GitHandle is the drawer handle's abbreviated branch label: an
// "impl/<parent>/<stem>" branch is Impl, a "goal/<parent>/<stem>" one Goal,
// and either one whose parent names the other kind ("goal/.../impl/...",
// "impl/goal/...") is both; Text is then the last segment, the stem. Any
// other branch keeps its whole name, and a detached HEAD its short commit id.
type GitHandle struct {
	Impl, Goal bool
	Text       string
}

// branchHandle is branch's handle label.
func branchHandle(branch string, detached bool) GitHandle {
	segs := strings.Split(branch, "/")
	if detached || len(segs) < 2 || (segs[0] != "impl" && segs[0] != "goal") || segs[len(segs)-1] == "" {
		return GitHandle{Text: branch}
	}
	h := GitHandle{Text: segs[len(segs)-1]}
	for _, s := range segs[:len(segs)-1] {
		switch s {
		case "impl":
			h.Impl = true
		case "goal":
			h.Goal = true
		}
	}
	return h
}

// Handle is the badge's handle label; the zero label for a badge that read
// no branch.
func (b GitBadge) Handle() GitHandle {
	if b.State != gitOK {
		return GitHandle{}
	}
	return branchHandle(b.Branch, b.Detached)
}

const (
	// ticketTitleTTL is how long a looked-up title answers before the next
	// lookup reads the file again.
	ticketTitleTTL = time.Minute
	// ticketTitleMax bounds the cache's entries; a full cache starts over.
	ticketTitleMax = 256
	// ticketHeadMax bounds the bytes of a ticket file a lookup reads.
	ticketHeadMax = 16 << 10
)

// ticketTitles caches the ticket titles the badge reads look up.
var ticketTitles = &ticketTitleCache{entries: map[string]ticketTitleEntry{}}

// ticketTitleCache maps a root and a stem to the title of the ticket file
// <root>/ai-docs/tickets/<status>/<stem>.md, "" when there is none.
type ticketTitleCache struct {
	mu      sync.Mutex
	entries map[string]ticketTitleEntry
}

type ticketTitleEntry struct {
	title string
	at    time.Time
}

// title is the ticket title for stem under root, from the cache while
// fresh.
func (c *ticketTitleCache) title(root, stem string, now time.Time) string {
	if stem == "" || strings.ContainsAny(stem, `/\`) || strings.HasPrefix(stem, ".") {
		return ""
	}
	key := gitKey(root) + "\x00" + stem
	c.mu.Lock()
	e, ok := c.entries[key]
	c.mu.Unlock()
	if ok && now.Sub(e.at) < ticketTitleTTL {
		return e.title
	}
	title := lookupTicketTitle(root, stem)
	c.mu.Lock()
	if len(c.entries) >= ticketTitleMax {
		c.entries = map[string]ticketTitleEntry{}
	}
	c.entries[key] = ticketTitleEntry{title: title, at: now}
	c.mu.Unlock()
	return title
}

// lookupTicketTitle finds <stem>.md in a status directory of root's ticket
// board and reads its title: the front matter's title scalar, else its first
// level-one heading, from the file's first ticketHeadMax bytes.
func lookupTicketTitle(root, stem string) string {
	base := filepath.Join(root, "ai-docs", "tickets")
	dirs, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		f, err := os.Open(filepath.Join(base, dir.Name(), stem+".md"))
		if err != nil {
			continue
		}
		title := ticketFileTitle(io.LimitReader(f, ticketHeadMax))
		f.Close()
		return title
	}
	return ""
}

// ticketFileTitle reads a ticket's title from its head.
func ticketFileTitle(r io.Reader) string {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), ticketHeadMax)
	first, front := true, false
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if first {
			first = false
			if strings.TrimSpace(line) == "---" {
				front = true
				continue
			}
		}
		if front {
			if strings.TrimSpace(line) == "---" {
				front = false
				continue
			}
			if v, ok := strings.CutPrefix(line, "title:"); ok {
				if t := unquoteScalar(strings.TrimSpace(v)); t != "" {
					return t
				}
			}
			continue
		}
		if v, ok := strings.CutPrefix(line, "# "); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// unquoteScalar drops a YAML scalar's matching quotes.
func unquoteScalar(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		if v[0] == '"' {
			if u, err := strconv.Unquote(v); err == nil {
				return u
			}
		}
		return v[1 : len(v)-1]
	}
	return v
}
