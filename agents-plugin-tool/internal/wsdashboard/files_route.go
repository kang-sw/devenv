package wsdashboard

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// The files route: GET /s/<reply-id>/files?q=<query> answers the composer's
// @ file mention popup with at most filesMax candidates, best first, as
// {"mode", "dir", "items": [{"path", "dir"}]}. The query picks the mode:
//
//   - Root mode, for any query not written absolute: a fuzzy search over the
//     session root's file list, paths relative to the root with forward
//     slashes. The list is git's (tracked plus untracked files, minus what
//     the ignore rules exclude), or a capped walk when git cannot list the
//     root, plus every directory those files sit in. It is cached per root
//     for filesListTTL. The query only filters that list; it never reaches
//     the filesystem. Root mode is served to every viewer the access layer
//     admits, a LAN token holder included, as the file view is.
//
//   - Abs mode, for a query starting with a slash in either direction or a
//     drive letter and colon (absQuery): one directory listing, the
//     directory the query names up to its last slash, its entries filtered
//     by the rest, like a shell's path completion; never recursive, since
//     a search under something like D:/ has no bound. It reaches outside
//     the root, so it is served only to a browser on this machine
//     (localViewer) for a request no other site started (crossSiteFetch),
//     checked before the filesystem is touched, under the image route's
//     rules for an outside path: a UNC, device, or NT-namespace spelling is
//     refused on the string, and the directory must be absolute on a plain
//     volume both as written and once resolved through its symlinks. Its
//     answer carries Cross-Origin-Resource-Policy: same-origin.
//
// Every refusal answers 404, so the route tells nothing about which paths
// exist.

const (
	// filesMax bounds the candidates one answer carries.
	filesMax = 50
	// filesQueryMax bounds the query in bytes.
	filesQueryMax = 4096
	// filesListTTL is how long a root's file list is served before it is
	// listed again; filesListKeep, how long an unasked root's list is kept.
	filesListTTL  = 10 * time.Second
	filesListKeep = 6 * filesListTTL
	// filesGitTimeout bounds the git listing of one root.
	filesGitTimeout = 5 * time.Second
	// filesWalkDepth and filesWalkMax bound the walk that lists a root git
	// cannot: the depth of the deepest entry it lists, and the entries it
	// visits.
	filesWalkDepth = 8
	filesWalkMax   = 20000
)

// fileItem is one candidate of the route's answer.
type fileItem struct {
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
}

// filesAnswer is the route's answer; Dir is the listed directory of abs
// mode.
type filesAnswer struct {
	Mode  string     `json:"mode"`
	Dir   string     `json:"dir,omitempty"`
	Items []fileItem `json:"items"`
}

// fileCandidate is one entry of a root's file list: a root-relative path
// with forward slashes and no trailing slash, and its depth in segments.
type fileCandidate struct {
	path  string
	dir   bool
	depth int
}

// fileListCache holds each root's file list for filesListTTL. A root's
// entry is listed under its own lock, so a burst of keystrokes against one
// root runs one listing and other roots never wait on it.
type fileListCache struct {
	mu      sync.Mutex
	entries map[string]*fileListEntry
	// now is the clock and list lists one root; time.Now and listRootFiles
	// unless a test replaces them.
	now  func() time.Time
	list func(root string) []fileCandidate
}

type fileListEntry struct {
	// used is when the root was last asked for, under the cache's mu.
	used time.Time

	mu     sync.Mutex
	at     time.Time
	filled bool
	items  []fileCandidate
}

func newFileListCache() *fileListCache {
	return &fileListCache{entries: map[string]*fileListEntry{}, now: time.Now, list: listRootFiles}
}

// get returns root's file list, sorted shallowest first (sortCandidates),
// listing it when the cached one is missing or older than filesListTTL.
func (c *fileListCache) get(root string) []fileCandidate {
	now := c.now()
	c.mu.Lock()
	for r, e := range c.entries {
		if now.Sub(e.used) > filesListKeep {
			delete(c.entries, r)
		}
	}
	e := c.entries[root]
	if e == nil {
		e = &fileListEntry{}
		c.entries[root] = e
	}
	e.used = now
	c.mu.Unlock()

	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.filled || now.Sub(e.at) >= filesListTTL {
		e.items, e.at, e.filled = c.list(root), now, true
	}
	return e.items
}

// listRootFiles lists root's files through git, so the ignore rules keep
// build output and dependency trees out, or through a capped walk when git
// cannot (not a repository, no git, a timeout), with the directories they
// sit in.
func listRootFiles(root string) []fileCandidate {
	if root == "" {
		return nil
	}
	paths, err := gitListFiles(root)
	if err != nil {
		paths = walkFiles(root, filesWalkDepth, filesWalkMax)
	}
	return fileCandidates(paths)
}

// gitListFiles lists the tracked and the untracked but not ignored files
// under root, relative to it.
func gitListFiles(root string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), filesGitTimeout)
	defer cancel()
	out, err := (&gitBadgeRunner{timeout: filesGitTimeout}).RunGit(ctx, root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// walkFiles lists the files under root, relative to it with forward slashes,
// visiting at most maxEntries entries and listing none deeper than maxDepth
// segments. It skips dot-directories and never follows a symlinked
// directory (WalkDir does not); a symlink is listed as a file, as git lists
// one.
func walkFiles(root string, maxDepth, maxEntries int) []string {
	// WalkDir does not follow a symlinked root either; the root itself is
	// the session's, so it is resolved first.
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	var paths []string
	visited := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if path == root {
			return err
		}
		if err != nil {
			return nil
		}
		if visited >= maxEntries {
			return fs.SkipAll
		}
		visited++
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") || strings.Count(rel, "/")+1 >= maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	return paths
}

// fileCandidates turns a file list into the cached candidates: each file
// once, plus every directory a file sits in, sorted by sortCandidates.
func fileCandidates(paths []string) []fileCandidate {
	dirs := map[string]bool{}
	files := map[string]bool{}
	for _, p := range paths {
		p = strings.TrimSuffix(p, "/")
		if p == "" {
			continue
		}
		files[p] = true
		for i := strings.LastIndexByte(p, '/'); i > 0; i = strings.LastIndexByte(p[:i], '/') {
			if dirs[p[:i]] {
				break
			}
			dirs[p[:i]] = true
		}
	}
	out := make([]fileCandidate, 0, len(files)+len(dirs))
	for d := range dirs {
		out = append(out, fileCandidate{path: d, dir: true, depth: strings.Count(d, "/") + 1})
	}
	for f := range files {
		if !dirs[f] {
			out = append(out, fileCandidate{path: f, depth: strings.Count(f, "/") + 1})
		}
	}
	sortCandidates(out)
	return out
}

// sortCandidates orders candidates as an empty query lists them: shallowest
// first, directories before files, then by path.
func sortCandidates(cs []fileCandidate) {
	sort.Slice(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.depth != b.depth {
			return a.depth < b.depth
		}
		if a.dir != b.dir {
			return a.dir
		}
		return a.path < b.path
	})
}

// rankRoot is root mode's answer over a root's sorted candidates: the first
// filesMax for an empty query, else the fuzzy matches of the whole relative
// path, best first.
func rankRoot(cs []fileCandidate, q string) []fileItem {
	items := make([]fileItem, 0, min(len(cs), filesMax))
	if q == "" {
		for _, c := range cs[:min(len(cs), filesMax)] {
			items = append(items, fileItem{Path: c.path, Dir: c.dir})
		}
		return items
	}
	type hit struct {
		c     fileCandidate
		score int
	}
	var hits []hit
	for _, c := range cs {
		if s, ok := fuzzyScore(q, c.path); ok {
			hits = append(hits, hit{c, s})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		return betterMatch(hits[i].score, hits[j].score, hits[i].c.path, hits[j].c.path)
	})
	for _, h := range hits[:min(len(hits), filesMax)] {
		items = append(items, fileItem{Path: h.c.path, Dir: h.c.dir})
	}
	return items
}

// betterMatch orders fuzzy matches: higher score, then shorter, then
// lexicographic.
func betterMatch(sa, sb int, a, b string) bool {
	if sa != sb {
		return sa > sb
	}
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}

// absQuery reports whether q is written absolute: it starts with a slash in
// either direction, or with a drive letter and colon followed by a slash or
// nothing (C: alone lists C:\).
func absQuery(q string) bool {
	if q == "" {
		return false
	}
	if q[0] == '/' || q[0] == '\\' {
		return true
	}
	return len(q) >= 2 && q[1] == ':' && ('a' <= q[0] && q[0] <= 'z' || 'A' <= q[0] && q[0] <= 'Z') &&
		(len(q) == 2 || q[2] == '/' || q[2] == '\\')
}

// listAbsDir is abs mode's answer for q: the directory q names up to its
// last slash, cleaned and with forward slashes, and its entries matching the
// rest of q. It refuses a UNC, device, or NT-namespace spelling and a
// directory not absolute on a plain volume before the filesystem is
// touched, and the same once the directory is resolved through its
// symlinks. Dot-entries are listed only for a query starting with a dot; a
// symlink's dir is its target's.
func listAbsDir(q string) (string, []fileItem, bool) {
	if networkPath(q) {
		return "", nil, false
	}
	dirPart, base := q+"/", ""
	if i := strings.LastIndexAny(q, `/\`); i >= 0 {
		dirPart, base = q[:i+1], q[i+1:]
	}
	native := filepath.FromSlash(dirPart)
	if !filepath.IsAbs(native) || !driveOrRootPath(native) {
		return "", nil, false
	}
	clean := filepath.Clean(native)
	real, err := filepath.EvalSymlinks(clean)
	if err != nil || networkPath(real) || !driveOrRootPath(real) {
		return "", nil, false
	}
	if info, err := os.Stat(real); err != nil || !info.IsDir() {
		return "", nil, false
	}
	entries, err := os.ReadDir(real)
	if err != nil {
		return "", nil, false
	}
	type hit struct {
		name  string
		dir   bool
		score int
	}
	var hits []hit
	dots := strings.HasPrefix(base, ".")
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") && !dots {
			continue
		}
		score, ok := fuzzyScore(base, name)
		if !ok {
			continue
		}
		dir := e.IsDir()
		if e.Type()&fs.ModeSymlink != 0 {
			info, err := os.Stat(filepath.Join(real, name))
			dir = err == nil && info.IsDir()
		}
		hits = append(hits, hit{name, dir, score})
	}
	sort.Slice(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if base == "" {
			if a.dir != b.dir {
				return a.dir
			}
			return a.name < b.name
		}
		return betterMatch(a.score, b.score, a.name, b.name)
	})
	items := make([]fileItem, 0, min(len(hits), filesMax))
	for _, h := range hits[:min(len(hits), filesMax)] {
		items = append(items, fileItem{Path: filepath.ToSlash(filepath.Join(clean, h.name)), Dir: h.dir})
	}
	return filepath.ToSlash(clean), items, true
}

// The fuzzy scorer's weights: each matched rune scores fuzzyMatch, plus
// fuzzyBoundary at a path boundary (the start, or after a separator) or
// fuzzyCamel at a lower-to-upper transition, plus fuzzyBasename inside the
// last path segment, plus fuzzyConsecutive right after the previous match.
// The candidate's rune count over fuzzyLengthDiv is taken off the total.
const (
	fuzzyMatch       = 16
	fuzzyBoundary    = 10
	fuzzyCamel       = 8
	fuzzyBasename    = 8
	fuzzyConsecutive = 12
	fuzzyLengthDiv   = 4
)

// fuzzyScore scores candidate against query, a case-insensitive
// subsequence match; ok is false when query is not a subsequence of
// candidate. An empty query matches everything with score 0. The score is
// the best placement of query's runes (a dynamic program over query and
// candidate positions) under the weights above.
func fuzzyScore(query, candidate string) (int, bool) {
	q := lowerRunes(query)
	if len(q) == 0 {
		return 0, true
	}
	orig := []rune(candidate)
	c := lowerRunes(candidate)
	matched := 0
	for _, r := range c {
		if matched < len(q) && r == q[matched] {
			matched++
		}
	}
	if matched < len(q) {
		return 0, false
	}

	n := len(c)
	base := 0
	for j := n - 1; j >= 0; j-- {
		if c[j] == '/' || c[j] == '\\' {
			base = j + 1
			break
		}
	}
	bonus := make([]int, n)
	for j := range bonus {
		switch {
		case j == 0 || fuzzySeparator(c[j-1]):
			bonus[j] += fuzzyBoundary
		case unicode.IsLower(orig[j-1]) && unicode.IsUpper(orig[j]):
			bonus[j] += fuzzyCamel
		}
		if j >= base {
			bonus[j] += fuzzyBasename
		}
	}

	// prev[j] is the best score of query[:i] with query[i-1] matched at j,
	// or none when it cannot be.
	const none = -1 << 30
	prev, cur := make([]int, n), make([]int, n)
	for j := range prev {
		prev[j] = none
		if c[j] == q[0] {
			prev[j] = fuzzyMatch + bonus[j]
		}
	}
	for i := 1; i < len(q); i++ {
		gapBest := none // best prev[k] for k <= j-2
		for j := range cur {
			if j >= 2 && prev[j-2] > gapBest {
				gapBest = prev[j-2]
			}
			cur[j] = none
			if c[j] != q[i] {
				continue
			}
			s := gapBest
			if j >= 1 && prev[j-1] != none && prev[j-1]+fuzzyConsecutive > s {
				s = prev[j-1] + fuzzyConsecutive
			}
			if s != none {
				cur[j] = s + fuzzyMatch + bonus[j]
			}
		}
		prev, cur = cur, prev
	}
	best := none
	for _, s := range prev {
		best = max(best, s)
	}
	return best - n/fuzzyLengthDiv, true
}

func lowerRunes(s string) []rune {
	rs := []rune(s)
	for i, r := range rs {
		rs[i] = unicode.ToLower(r)
	}
	return rs
}

func fuzzySeparator(r rune) bool {
	switch r {
	case '/', '\\', '_', '-', '.', ' ':
		return true
	}
	return false
}

// handleFiles serves the files route of one session.
func (d *Daemon) handleFiles(w http.ResponseWriter, r *http.Request, replyID string) {
	if d.index == nil || !ValidReplyID(replyID) {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	s, ok := d.index.Session(replyID)
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	q := r.URL.Query().Get("q")
	if len(q) > filesQueryMax || strings.ContainsRune(q, 0) {
		http.Error(w, "files not found", http.StatusNotFound)
		return
	}
	var answer filesAnswer
	if absQuery(q) {
		if !localViewer(r) || crossSiteFetch(r) {
			http.Error(w, "files not found", http.StatusNotFound)
			return
		}
		dir, items, ok := listAbsDir(q)
		if !ok {
			http.Error(w, "files not found", http.StatusNotFound)
			return
		}
		answer = filesAnswer{Mode: "abs", Dir: dir, Items: items}
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	} else {
		answer = filesAnswer{Mode: "root", Items: rankRoot(d.files.get(s.Project), q)}
	}
	raw, _ := json.Marshal(answer)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(raw)
}
