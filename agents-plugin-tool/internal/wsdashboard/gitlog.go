package wsdashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// The git drawer's commit graph and commit panel reads: the graph of every
// local branch and its upstream, a page at a time with the lane layout the
// daemon computes (GET /s/<reply-id>/git/log?offset=<n>), and one commit's
// metadata and changed-file list (GET /s/<reply-id>/git/commit/<oid>). Both
// are read-only GETs under the same rules as the status read: no fetch, no
// optional locks, local refs only, and nothing the page sends reaches git's
// argv but a validated hex object id.
//
// The log answer (GitLog) is the contract the page's SVG graph draws:
//
//	{"head": "<oid of the root's HEAD, or empty>",
//	 "commits": [{"oid", "parents": [...], "subject", "author", "time": <ms>,
//	              "col", "color", "down": [{"from", "to", "color"}, ...],
//	              "refs": [{"name", "kind", "head", "impl", "goal", "text",
//	                        "worktree"}, ...]}, ...],
//	 "width": <columns this page uses>, "more": <next offset, 0 when none>}
//
// Rows are in --topo-order, newest first. A row's node sits in column col
// (0 is leftmost); down lists the line segments between this row and the
// next one, each from a column at this row to a column at the next row, so
// a page draws by stacking rows and the first row of a later page continues
// the last row of the page before it (the daemon lays out every page from
// the top of the log). color is a lane's stable index, which the page maps
// onto its palette; a lane keeps its color while it continues through first
// parents.

const (
	// gitLogPage is the number of commits one log page holds.
	gitLogPage = 200
	// gitLogMaxOffset bounds the offset a log request may ask for, since
	// every page is laid out from the top.
	gitLogMaxOffset = 20000
	// gitCommitFilesMax bounds a commit's changed-file list.
	gitCommitFilesMax = 1000
)

// GitLog is one page of the commit graph.
type GitLog struct {
	// Head is the session root's HEAD commit, "" when HEAD names none.
	Head    string         `json:"head"`
	Commits []GitLogCommit `json:"commits"`
	// Width is the number of columns this page's nodes and segments use.
	Width int `json:"width"`
	// More is the offset of the next page, 0 when this page is the last.
	More int `json:"more"`
}

// GitLogCommit is one row of the graph.
type GitLogCommit struct {
	OID     string   `json:"oid"`
	Parents []string `json:"parents"`
	Subject string   `json:"subject"`
	Author  string   `json:"author"`
	// Time is the author time, epoch ms.
	Time int64 `json:"time"`
	// Col is the node's column and Color its lane's color index.
	Col   int `json:"col"`
	Color int `json:"color"`
	// Down is the segments between this row and the next.
	Down []GitLane   `json:"down"`
	Refs []GitLogRef `json:"refs,omitempty"`
}

// GitLane is one line segment between two adjacent rows: from a column at
// the upper row to a column at the lower one.
type GitLane struct {
	From  int `json:"from"`
	To    int `json:"to"`
	Color int `json:"color"`
}

// GitLogRef is a label on a row: a local branch, a remote-tracking branch
// some local branch has as its upstream, or a worktree's detached HEAD.
type GitLogRef struct {
	// Name is the branch's short name ("develop", "origin/develop"); empty
	// for a detached HEAD.
	Name string `json:"name"`
	// Kind is "branch", "upstream", or "detached".
	Kind string `json:"kind"`
	// Head marks the session root's own checkout: its branch, or its
	// detached HEAD.
	Head bool `json:"head,omitempty"`
	// Impl, Goal, and Text are a local branch's handle abbreviation, as the
	// drawer handle shows it (GitHandle).
	Impl bool   `json:"impl,omitempty"`
	Goal bool   `json:"goal,omitempty"`
	Text string `json:"text,omitempty"`
	// Worktree is the path of the worktree that has this branch (or this
	// detached HEAD) checked out, "" when none.
	Worktree string `json:"worktree,omitempty"`
}

// laneCommit is a row the lane layout reads: a commit and its parents.
type laneCommit struct {
	oid     string
	parents []string
}

// laneRows is the layout of rows: each row's node column and color, and the
// segments between it and the next row.
type laneRows struct {
	cols, colors []int
	down         [][]GitLane
}

// layoutLanes lays the rows (in topo order, children first) out in lanes.
// A lane waits for one commit. A row's node takes the leftmost lane waiting
// for it, or the leftmost free lane when none waits (a branch tip); every
// other lane waiting for it ends there, drawn as a segment into the node.
// The node's lane then waits for its first parent, keeping its color; each
// other parent joins the lane already waiting for it, or opens a new lane
// with a new color. Lanes never shift: a free lane in the middle stays
// empty until a new lane takes it. Two lanes may wait for the same commit
// (a branch forked from a commit another lane continues through), and they
// meet at that commit's row, so a branch's line runs beside its base down
// to its fork point. The last row's segments run straight down, toward
// rows the layout did not read.
func layoutLanes(rows []laneCommit) laneRows {
	type lane struct {
		oid   string // the commit it waits for; "" when free
		color int
		srcs  []int // the columns at the current row its segments start at
	}
	var lanes []lane
	next := 0
	free := func(skip int) int {
		for k := range lanes {
			if lanes[k].oid == "" && k != skip {
				return k
			}
		}
		lanes = append(lanes, lane{})
		return len(lanes) - 1
	}
	out := laneRows{cols: make([]int, len(rows)), colors: make([]int, len(rows)), down: make([][]GitLane, len(rows))}
	// emit records the segments under row i, toward a next row whose node
	// is target in column col (target "" when there is no next row).
	emit := func(i int, target string, col int) {
		segs := []GitLane{}
		for k, l := range lanes {
			if l.oid == "" {
				continue
			}
			to := k
			if l.oid == target {
				to = col
			}
			for _, s := range l.srcs {
				segs = append(segs, GitLane{From: s, To: to, Color: l.color})
			}
		}
		out.down[i] = segs
	}
	for i, c := range rows {
		col := -1
		for k, l := range lanes {
			if l.oid == c.oid {
				col = k
				break
			}
		}
		color := 0
		if col < 0 {
			col = free(-1)
			color = next
			next++
		} else {
			color = lanes[col].color
		}
		if i > 0 {
			emit(i-1, c.oid, col)
		}
		out.cols[i], out.colors[i] = col, color
		for k := range lanes {
			if lanes[k].oid == c.oid {
				lanes[k] = lane{}
			} else if lanes[k].oid != "" {
				lanes[k].srcs = []int{k}
			}
		}
		if len(c.parents) > 0 {
			lanes[col] = lane{oid: c.parents[0], color: color, srcs: []int{col}}
		}
		for _, p := range c.parents[min(1, len(c.parents)):] {
			joined := false
			for k := range lanes {
				if lanes[k].oid == p {
					lanes[k].srcs = append(lanes[k].srcs, col)
					joined = true
					break
				}
			}
			if !joined {
				k := free(col)
				lanes[k] = lane{oid: p, color: next, srcs: []int{col}}
				next++
			}
		}
		for len(lanes) > 0 && lanes[len(lanes)-1].oid == "" {
			lanes = lanes[:len(lanes)-1]
		}
	}
	if n := len(rows); n > 0 {
		emit(n-1, "", 0)
	}
	return out
}

// gitLogRefs is what the log reads from the refs and worktrees: the revs
// the walk starts from, the labels by commit, and the root's HEAD.
type gitLogRefs struct {
	revs   []string
	labels map[string][]GitLogRef
	head   string
}

// readGitLogRefs reads root's local branches, the upstreams they have, the
// worktrees' checkouts, and root's own HEAD.
func readGitLogRefs(ctx context.Context, run *gitBadgeRunner, root string) (gitLogRefs, error) {
	refs := gitLogRefs{labels: map[string][]GitLogRef{}}
	// HEAD may name no commit (an unborn branch): the graph then has no
	// head mark, and its branches still draw.
	if out, err := run.RunGit(ctx, root, "--no-optional-locks", "rev-parse", "--verify", "--quiet", "HEAD"); err == nil {
		refs.head = strings.TrimSpace(string(out))
	}
	headRef := ""
	if out, err := run.RunGit(ctx, root, "--no-optional-locks", "rev-parse", "--symbolic-full-name", "HEAD"); err == nil {
		headRef = strings.TrimSpace(string(out))
	}
	out, err := run.RunGit(ctx, root, "--no-optional-locks", "for-each-ref", "--format=%(refname)%00%(objectname)%00%(upstream)", "refs/heads", "refs/remotes")
	if err != nil {
		return refs, fmt.Errorf("%s", gitErrText(err))
	}
	oids := map[string]string{}
	var locals, ups []string
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, "\x00")
		if len(f) != 3 || f[1] == "" {
			continue
		}
		oids[f[0]] = f[1]
		if strings.HasPrefix(f[0], "refs/heads/") {
			locals = append(locals, f[0])
			if strings.HasPrefix(f[2], "refs/remotes/") {
				ups = append(ups, f[2])
			}
		}
	}
	worktrees := readWorktreeCheckouts(ctx, run, root)
	seen := map[string]bool{}
	for _, ref := range locals {
		name := strings.TrimPrefix(ref, "refs/heads/")
		h := branchHandle(name, false)
		l := GitLogRef{Name: name, Kind: "branch", Head: ref == headRef, Worktree: worktrees[ref]}
		if h.Impl || h.Goal {
			l.Impl, l.Goal, l.Text = h.Impl, h.Goal, h.Text
		}
		refs.labels[oids[ref]] = append(refs.labels[oids[ref]], l)
		refs.revs = append(refs.revs, ref)
		seen[ref] = true
	}
	for _, ref := range ups {
		oid, ok := oids[ref]
		if !ok || seen[ref] {
			continue // an upstream whose ref is gone, or a second branch tracking it
		}
		seen[ref] = true
		refs.labels[oid] = append(refs.labels[oid], GitLogRef{Name: strings.TrimPrefix(ref, "refs/remotes/"), Kind: "upstream"})
		refs.revs = append(refs.revs, ref)
	}
	// Detached checkouts: the root's own HEAD, and every worktree's.
	detached := map[string]string{}
	for key, path := range worktrees {
		if oid, ok := strings.CutPrefix(key, "detached:"); ok {
			detached[oid] = path
		}
	}
	if headRef == "HEAD" && refs.head != "" {
		if _, ok := detached[refs.head]; !ok {
			detached[refs.head] = ""
		}
	}
	// In a fixed order: the walk's revs order its ties, and every page of
	// one log must walk alike.
	dets := make([]string, 0, len(detached))
	for oid := range detached {
		if isHexOID(oid) {
			dets = append(dets, oid)
		}
	}
	sort.Strings(dets)
	for _, oid := range dets {
		head := headRef == "HEAD" && oid == refs.head
		refs.labels[oid] = append(refs.labels[oid], GitLogRef{Kind: "detached", Head: head, Worktree: detached[oid]})
		refs.revs = append(refs.revs, oid)
	}
	return refs, nil
}

// readWorktreeCheckouts reads `git worktree list --porcelain`: each
// worktree's path by the branch ref it has checked out, or by
// "detached:<oid>" for a detached one. A failed read marks nothing.
func readWorktreeCheckouts(ctx context.Context, run *gitBadgeRunner, root string) map[string]string {
	m := map[string]string{}
	out, err := run.RunGit(ctx, root, "--no-optional-locks", "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return m
	}
	path, oid := "", ""
	for _, e := range strings.Split(string(out), "\x00") {
		switch {
		case e == "":
			path, oid = "", ""
		case strings.HasPrefix(e, "worktree "):
			path = filepath.FromSlash(strings.TrimPrefix(e, "worktree "))
		case strings.HasPrefix(e, "HEAD "):
			oid = strings.TrimPrefix(e, "HEAD ")
		case strings.HasPrefix(e, "branch "):
			m[strings.TrimPrefix(e, "branch ")] = path
		case e == "detached" && oid != "":
			m["detached:"+oid] = path
		}
	}
	return m
}

// readGitLog reads the page of root's graph that starts at offset, holding
// at most page commits. The walk reads every commit above the page too, and
// one past it, so the page's lanes are those of the whole log.
func readGitLog(root string, offset, page int) (GitLog, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitReadTimeout)
	defer cancel()
	run := &gitBadgeRunner{}
	refs, err := readGitLogRefs(ctx, run, root)
	if err != nil {
		return GitLog{}, err
	}
	g := GitLog{Head: refs.head, Commits: []GitLogCommit{}}
	if len(refs.revs) == 0 {
		return g, nil
	}
	args := []string{"--no-optional-locks", "log", "--topo-order", "--no-show-signature", "-z",
		"--format=%H%x1f%P%x1f%an%x1f%at%x1f%s", "-n", strconv.Itoa(offset + page + 1)}
	out, err := run.RunGit(ctx, root, append(args, refs.revs...)...)
	if err != nil {
		return GitLog{}, fmt.Errorf("%s", gitErrText(err))
	}
	var all []GitLogCommit
	var rows []laneCommit
	for _, rec := range strings.Split(string(out), "\x00") {
		f := strings.SplitN(strings.TrimPrefix(rec, "\n"), "\x1f", 5)
		if len(f) != 5 || f[0] == "" {
			continue
		}
		c := GitLogCommit{OID: f[0], Parents: strings.Fields(f[1]), Author: f[2], Subject: f[4]}
		if s, err := strconv.ParseInt(f[3], 10, 64); err == nil {
			c.Time = s * 1000
		}
		all = append(all, c)
		rows = append(rows, laneCommit{oid: c.OID, parents: c.Parents})
	}
	lay := layoutLanes(rows)
	end := min(len(all), offset+page)
	if len(all) > offset+page {
		g.More = offset + page
	}
	for i := offset; i < end; i++ {
		c := all[i]
		c.Col, c.Color, c.Down, c.Refs = lay.cols[i], lay.colors[i], lay.down[i], refs.labels[c.OID]
		g.Width = max(g.Width, c.Col+1)
		for _, s := range c.Down {
			g.Width = max(g.Width, s.From+1, s.To+1)
		}
		g.Commits = append(g.Commits, c)
	}
	return g, nil
}

// GitCommit is one commit's panel: its metadata and the files it changed
// against Base, its first parent (the empty tree for a root commit, Base
// ""), so a merge shows what it brought into its first parent's line.
type GitCommit struct {
	OID            string          `json:"oid"`
	Parents        []string        `json:"parents"`
	Base           string          `json:"base"`
	Message        string          `json:"message"`
	Author         string          `json:"author"`
	AuthorEmail    string          `json:"authorEmail"`
	AuthorTime     int64           `json:"authorTime"`
	Committer      string          `json:"committer"`
	CommitterEmail string          `json:"committerEmail"`
	CommitTime     int64           `json:"commitTime"`
	Files          []GitCommitFile `json:"files"`
	// Truncated reports a file list cut at gitCommitFilesMax.
	Truncated bool `json:"truncated,omitempty"`
}

// GitCommitFile is one changed file of a commit: its path, its status
// (A, M, D, T, R renamed, C copied), the path it was renamed or copied
// from, and its line delta (Add and Del -1 for a binary file).
type GitCommitFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	From   string `json:"from,omitempty"`
	Add    int    `json:"add"`
	Del    int    `json:"del"`
}

// isHexOID reports whether s is a 4 to 64 character hex object id, the only
// form the commit routes accept, so no ref expression or option reaches
// git's argv.
func isHexOID(s string) bool {
	if len(s) < 4 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
			return false
		}
	}
	return true
}

// errNoCommit is a commit read's answer for an id that names no commit.
var errNoCommit = fmt.Errorf("no such commit")

// readGitCommit reads the commit oid names (a hex id, full or abbreviated)
// in root: errNoCommit when it names none.
func readGitCommit(root, oid string) (GitCommit, error) {
	if !isHexOID(oid) {
		return GitCommit{}, errNoCommit
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitReadTimeout)
	defer cancel()
	run := &gitBadgeRunner{}
	out, err := run.RunGit(ctx, root, "--no-optional-locks", "rev-parse", "--verify", "--quiet", oid+"^{commit}")
	if err != nil {
		if _, e := run.RunGit(ctx, root, "--no-optional-locks", "rev-parse", "--git-dir"); e != nil {
			return GitCommit{}, fmt.Errorf("%s", gitErrText(e))
		}
		return GitCommit{}, errNoCommit
	}
	full := strings.TrimSpace(string(out))
	out, err = run.RunGit(ctx, root, "--no-optional-locks", "show", "-s", "--no-show-signature",
		"--format=%H%x00%P%x00%an%x00%ae%x00%at%x00%cn%x00%ce%x00%ct%x00%B", full)
	if err != nil {
		return GitCommit{}, fmt.Errorf("%s", gitErrText(err))
	}
	f := strings.SplitN(string(out), "\x00", 9)
	if len(f) != 9 {
		return GitCommit{}, fmt.Errorf("malformed commit read")
	}
	c := GitCommit{OID: f[0], Parents: strings.Fields(f[1]), Author: f[2], AuthorEmail: f[3],
		Committer: f[5], CommitterEmail: f[6], Message: strings.TrimRight(f[8], "\n"), Files: []GitCommitFile{}}
	if s, err := strconv.ParseInt(f[4], 10, 64); err == nil {
		c.AuthorTime = s * 1000
	}
	if s, err := strconv.ParseInt(f[7], 10, 64); err == nil {
		c.CommitTime = s * 1000
	}
	diff := []string{"--no-optional-locks", "diff-tree", "-r", "-z", "-M", "--no-commit-id"}
	var trees []string
	if len(c.Parents) > 0 {
		c.Base = c.Parents[0]
		trees = []string{c.Base, c.OID}
	} else {
		trees = []string{"--root", c.OID}
	}
	status, err := run.RunGit(ctx, root, append(append(diff, "--name-status"), trees...)...)
	if err != nil {
		return GitCommit{}, fmt.Errorf("%s", gitErrText(err))
	}
	numstat, err := run.RunGit(ctx, root, append(append(diff, "--numstat"), trees...)...)
	if err != nil {
		return GitCommit{}, fmt.Errorf("%s", gitErrText(err))
	}
	c.Files, c.Truncated = parseCommitFiles(status, parseRenameNumstat(numstat))
	return c, nil
}

// parseCommitFiles reads a `diff-tree -z -M --name-status` output, each
// file with its delta by destination path.
func parseCommitFiles(out []byte, delta map[string][2]int) ([]GitCommitFile, bool) {
	files := []GitCommitFile{}
	f := strings.Split(string(out), "\x00")
	for i := 0; i < len(f); i++ {
		code := f[i]
		if code == "" {
			continue
		}
		file := GitCommitFile{Status: code[:1]}
		if file.Status == "R" || file.Status == "C" {
			if i+2 >= len(f) {
				break
			}
			file.From, file.Path = f[i+1], f[i+2]
			i += 2
		} else {
			if i+1 >= len(f) {
				break
			}
			file.Path = f[i+1]
			i++
		}
		if len(files) >= gitCommitFilesMax {
			return files, true
		}
		if d, ok := delta[file.Path]; ok {
			file.Add, file.Del = d[0], d[1]
		}
		files = append(files, file)
	}
	return files, false
}

// parseRenameNumstat reads a `diff-tree -z -M --numstat` output: the
// destination path to added and deleted lines, -1 for both on a binary
// file. A renamed or copied file's record is "add\tdel\t" followed by its
// source and destination paths as two more fields.
func parseRenameNumstat(out []byte) map[string][2]int {
	m := map[string][2]int{}
	f := strings.Split(string(out), "\x00")
	for i := 0; i < len(f); i++ {
		a, rest, ok := strings.Cut(f[i], "\t")
		if !ok {
			continue
		}
		d, path, ok := strings.Cut(rest, "\t")
		if !ok {
			continue
		}
		if path == "" {
			if i+2 >= len(f) {
				break
			}
			path = f[i+2]
			i += 2
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

// sessionRoot is the root of the session replyID names, for a git read
// route; it answers 404 itself when there is none.
func (d *Daemon) sessionRoot(w http.ResponseWriter, replyID string) (string, bool) {
	if d.index == nil || !ValidReplyID(replyID) {
		http.Error(w, "unknown session", http.StatusNotFound)
		return "", false
	}
	s, ok := d.index.Session(replyID)
	if !ok || s.Project == "" {
		http.Error(w, "unknown session", http.StatusNotFound)
		return "", false
	}
	return s.Project, true
}

// writeGitJSON answers a git read route with v as JSON.
func writeGitJSON(w http.ResponseWriter, v any) {
	raw, _ := json.Marshal(v)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(raw)
}

// handleGitLog serves GET /s/<reply-id>/git/log?offset=<n>: the page of the
// session root's graph at offset (0 when absent), as GitLog JSON; 400 for a
// malformed offset, 502 with git's reason when the read fails.
func (d *Daemon) handleGitLog(w http.ResponseWriter, r *http.Request, replyID string) {
	root, ok := d.sessionRoot(w, replyID)
	if !ok {
		return
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > gitLogMaxOffset {
			http.Error(w, "bad offset", http.StatusBadRequest)
			return
		}
		offset = n
	}
	g, err := readGitLog(root, offset, gitLogPage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeGitJSON(w, g)
}

// handleGitCommit serves GET /s/<reply-id>/git/commit/<oid>: the commit's
// metadata and changed files as GitCommit JSON; 404 for an id that is no
// hex object id or names no commit, 502 with git's reason when the read
// fails.
func (d *Daemon) handleGitCommit(w http.ResponseWriter, r *http.Request, replyID, oid string) {
	root, ok := d.sessionRoot(w, replyID)
	if !ok {
		return
	}
	if !isHexOID(oid) {
		http.Error(w, "no such commit", http.StatusNotFound)
		return
	}
	c, err := readGitCommit(root, oid)
	if err == errNoCommit {
		http.Error(w, "no such commit", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeGitJSON(w, c)
}
