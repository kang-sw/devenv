package wsdashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kang-sw/devenv/internal/wsi18n"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// The file browser routes: the session page's Files tab lists a folder and
// its preview cards show a file, read-only.
//
//	GET  /s/<reply-id>/fs/list?dir=<dir>[&root=<abs>]     one directory's entries
//	GET  /s/<reply-id>/fs/preview?path=<p>[&root=<abs>]   what a preview card shows (text=1: as plain text)
//	GET  /s/<reply-id>/fs/raw?path=<p>[&root=<abs>]       an image's or a video's bytes
//	GET  /s/<reply-id>/fs/find, /fs/search                name and content search (fs_search.go)
//	GET  /s/<reply-id>/fs/cap/<capability>/<p>            the sandboxed HTML preview (fs_capability.go)
//	POST /s/<reply-id>/fs/act                             open or reveal on the host (fs_act_route.go)
//
// The browsing root is the session's project root, the root the file view,
// the file-link check, and the files route use. Every viewer the access
// layer admits browses it, a LAN token holder included, as the file view
// is served. A request may name another root with the root parameter (the
// explorer's "change root"): that reaches outside the session root, so it
// is served only to a browser on this machine (localViewer) for a request
// no other site started (crossSiteFetch), checked before the filesystem is
// touched, under the files route's rules for an absolute directory
// (browseRootParam).
//
// Every read stays inside the chosen root. A directory is listed only when
// it stays inside the root lexically and once resolved through every
// symlink, and is itself no symlink (containedDir); a file is read through
// the file view's openContained, so a symlinked file, a path through a
// symlinked directory that leads outside, and a file swapped after the
// check are refused. Every refusal answers 404, so the routes tell nothing
// about which paths exist.

const (
	// fsListMax bounds the entries one listing carries; a longer directory
	// is cut and says so (more).
	fsListMax = 2000
	// fsIgnoreTimeout bounds the git read that marks ignored entries.
	fsIgnoreTimeout = 3 * time.Second
)

// fsEntry is one entry of a listing.
type fsEntry struct {
	Name string `json:"name"`
	Dir  bool   `json:"dir,omitempty"`
	// Link marks a symlink: it is listed and never opened or descended.
	Link bool `json:"link,omitempty"`
	// Ignored marks an entry git's ignore rules exclude, and .git itself.
	Ignored bool `json:"ign,omitempty"`
	// Status is the entry's git status mark (gitMarks): "M", "A", "U", or
	// "D" on a file, "U" on an untracked directory, and "*" on a directory
	// that holds changes.
	Status string `json:"st,omitempty"`
	// Gone marks an entry that is not on disk: a deleted file, or a
	// directory whose changes are all deletions. It is listed for its mark
	// and never opened.
	Gone bool `json:"gone,omitempty"`
}

// fsListAnswer is the listing route's answer. Root is the browsing root as
// an absolute path with forward slashes, Name its base name, Session
// whether it is the session's own root, Dir the listed directory relative
// to Root ("" is the root itself), and Repo whether git told which entries
// are ignored. Abs is the listed directory's absolute path in the host's
// own spelling and Sep the host's path separator: an entry's absolute path,
// which the row menu copies, is Abs + Sep + its name.
type fsListAnswer struct {
	Root    string    `json:"root"`
	Name    string    `json:"name"`
	Session bool      `json:"session"`
	Dir     string    `json:"dir"`
	Abs     string    `json:"abs"`
	Sep     string    `json:"sep"`
	Repo    bool      `json:"repo"`
	More    bool      `json:"more,omitempty"`
	Entries []fsEntry `json:"entries"`
}

// fsPreviewAnswer is the preview route's answer. Kind picks what the card
// draws:
//
//   - "image", "video": Src is the raw route's URL of the file;
//   - "frame": an HTML or SVG file; Frame is its capability route URL, for
//     an iframe with sandbox="allow-scripts", and TTL the seconds the
//     capability still lives;
//   - "markdown": HTML is the file rendered by the board's markdown
//     renderer in its escaped mode;
//   - "diff": Diff is the patch a .diff or .patch file holds, one entry per
//     file, as the commit window's diff answer;
//   - "text": Text is the file; Source marks an HTML or SVG file shown as
//     its source because the root is not the session's;
//   - "none": the card shows Reason, in the page's language.
type fsPreviewAnswer struct {
	Kind   string        `json:"kind"`
	Rel    string        `json:"rel"`
	Abs    string        `json:"abs"`
	Size   int64         `json:"size"`
	Src    string        `json:"src,omitempty"`
	Frame  string        `json:"frame,omitempty"`
	TTL    int           `json:"ttl,omitempty"`
	HTML   string        `json:"html,omitempty"`
	Diff   []GitFileDiff `json:"diff,omitempty"`
	Text   string        `json:"text,omitempty"`
	Source bool          `json:"source,omitempty"`
	Reason string        `json:"reason,omitempty"`
}

// videoTypes are the video containers the raw route streams, by extension.
var videoTypes = map[string]string{
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".ogg":  "video/ogg",
	".ogv":  "video/ogg",
}

func videoType(name string) (string, bool) {
	ctype, ok := videoTypes[strings.ToLower(filepath.Ext(name))]
	return ctype, ok
}

// framedFile reports a file the preview renders in the sandboxed frame:
// HTML, and SVG, which can carry script and is treated like it.
func framedFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm", ".svg":
		return true
	}
	return false
}

// browseRootParam validates a root parameter: a directory written absolute
// on a plain volume, both as written and once resolved through its
// symlinks, under the files route's rules for an absolute directory
// (listAbsDir). It returns the path cleaned.
func browseRootParam(raw string) (string, bool) {
	if raw == "" || len(raw) > filesQueryMax || strings.ContainsRune(raw, 0) || networkPath(raw) {
		return "", false
	}
	native := filepath.FromSlash(raw)
	if !filepath.IsAbs(native) || !driveOrRootPath(native) {
		return "", false
	}
	clean := filepath.Clean(native)
	if deviceElement(clean) {
		return "", false
	}
	real, err := filepath.EvalSymlinks(clean)
	if err != nil || networkPath(real) || !driveOrRootPath(real) {
		return "", false
	}
	if info, err := os.Stat(real); err != nil || !info.IsDir() {
		return "", false
	}
	return clean, true
}

// browseRoot is the browsing root of a read on the session replyID: the
// session's project root, or the root parameter for a browser on this
// machine. session reports the session's own root, the only one an HTML
// preview capability is minted for.
func (d *Daemon) browseRoot(r *http.Request, replyID string) (root string, session, ok bool) {
	if d.index == nil || !ValidReplyID(replyID) {
		return "", false, false
	}
	s, found := d.index.Session(replyID)
	if !found || s.Project == "" {
		return "", false, false
	}
	raw := r.URL.Query().Get("root")
	if raw == "" {
		return s.Project, true, true
	}
	if !localViewer(r) || crossSiteFetch(r) {
		return "", false, false
	}
	root, ok = browseRootParam(raw)
	if !ok {
		return "", false, false
	}
	if root == filepath.Clean(s.Project) {
		return s.Project, true, true
	}
	return root, false, true
}

// containedDir resolves dir, relative to root or absolute inside it ("" is
// root itself), to a directory the listing may read: inside root lexically
// and once resolved through every symlink, and no symlink itself. rel is
// its path relative to root, "" for root.
func containedDir(root, dir string) (full, rel string, ok bool) {
	full = filepath.Clean(root)
	if dir != "" && dir != "." {
		rel, ok = relativeInside(root, dir)
		if !ok {
			return "", "", false
		}
		full = filepath.Join(root, rel)
		if linfo, err := os.Lstat(full); err != nil || !linfo.IsDir() {
			return "", "", false
		}
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", false
	}
	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		return "", "", false
	}
	if r, err := filepath.Rel(realRoot, real); err != nil || (r != "." && !insideRel(r)) {
		return "", "", false
	}
	if info, err := os.Stat(real); err != nil || !info.IsDir() {
		return "", "", false
	}
	return full, rel, true
}

// gitIgnored reports which of names, entries of the directory dir, git's
// ignore rules exclude; repo is false when git cannot tell (no repository,
// no git, a timeout). It asks from dir itself, so a nested repository or
// worktree answers by its own rules. A tracked file is never ignored.
func gitIgnored(dir string, names []string) (ignored map[string]bool, repo bool) {
	ctx, cancel := context.WithTimeout(context.Background(), fsIgnoreTimeout)
	defer cancel()
	if len(names) == 0 {
		_, err := (&gitBadgeRunner{timeout: fsIgnoreTimeout}).RunGit(ctx, dir, "rev-parse", "--is-inside-work-tree")
		return nil, err == nil
	}
	cmd := badgeGitCommand(ctx, dir, "check-ignore", "-z", "--stdin")
	cmd.Stdin = strings.NewReader(strings.Join(names, "\x00") + "\x00")
	out, err := cmd.Output()
	// Exit 1 says none of the names is ignored; anything else failed.
	if err != nil {
		if exit, isExit := err.(interface{ ExitCode() int }); !isExit || exit.ExitCode() != 1 || ctx.Err() != nil {
			return nil, false
		}
	}
	ignored = map[string]bool{}
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			ignored[p] = true
		}
	}
	return ignored, true
}

// statusMarks reads a `git status --porcelain=v1 -z --no-renames` output
// for the directory at prefix (its path from the repository's top, with a
// trailing slash, or "" for the top): each changed entry of that directory
// by name, with its mark. A file is "M" modified, "A" added, "D" deleted,
// or "U" untracked; a directory is "U" when git reports it untracked as a
// whole and "*" when it holds changes. gone names the entries whose every
// change is a deletion, which may not be on disk any more.
func statusMarks(status []byte, prefix string) (marks map[string]string, dirs, gone map[string]bool) {
	marks, dirs, gone = map[string]string{}, map[string]bool{}, map[string]bool{}
	kept := map[string]bool{}
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
		untracked := x == '?' && y == '?'
		if untracked && strings.HasSuffix(path, "/") && strings.HasPrefix(prefix, path) {
			// The listed directory is inside an untracked one: all of it is.
			marks[""] = "U"
			continue
		}
		rest, ok := strings.CutPrefix(path, prefix)
		if !ok || rest == "" {
			continue
		}
		name, deeper, nested := strings.Cut(rest, "/")
		deleted := !untracked && (x == 'D' || y == 'D') && x != 'A' && y != 'A'
		switch {
		case nested && deeper == "" && untracked:
			marks[name], dirs[name] = "U", true
		case nested:
			if marks[name] != "U" {
				marks[name] = "*"
			}
			dirs[name] = true
		case untracked:
			marks[name] = "U"
		case deleted:
			marks[name] = "D"
		case x == 'A' || y == 'A':
			// y is 'A' for a file added with intent only (git add -N).
			marks[name] = "A"
		default:
			marks[name] = "M"
		}
		if deleted {
			if !kept[name] {
				gone[name] = true
			}
		} else {
			kept[name] = true
			delete(gone, name)
		}
	}
	return marks, dirs, gone
}

// gitMarks reads the git status marks of the entries of the directory dir,
// asked from dir itself, as gitIgnored asks; nil when git cannot tell.
func gitMarks(dir string) (marks map[string]string, dirs, gone map[string]bool) {
	ctx, cancel := context.WithTimeout(context.Background(), fsIgnoreTimeout)
	defer cancel()
	run := &gitBadgeRunner{timeout: fsIgnoreTimeout}
	prefix, err := run.RunGit(ctx, dir, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, nil, nil
	}
	status, err := run.RunGit(ctx, dir, "--no-optional-locks", "status", "--porcelain=v1", "-z", "--no-renames", "--untracked-files=normal", "--", ".")
	if err != nil {
		return nil, nil, nil
	}
	return statusMarks(status, strings.TrimRight(string(prefix), "\r\n"))
}

// listDir lists the directory full: directories first, then by name
// without case, at most fsListMax entries, each with its ignore and git
// status marks. An entry git reports deleted is listed too, marked gone.
func listDir(full string) (entries []fsEntry, more, repo bool, err error) {
	read, err := os.ReadDir(full)
	if err != nil {
		return nil, false, false, err
	}
	marks, markDirs, gone := gitMarks(full)
	entries = make([]fsEntry, 0, len(read))
	for _, e := range read {
		link := e.Type()&fs.ModeSymlink != 0
		entries = append(entries, fsEntry{Name: e.Name(), Dir: e.IsDir() && !link, Link: link})
		delete(gone, e.Name())
	}
	for name := range gone {
		entries = append(entries, fsEntry{Name: name, Dir: markDirs[name], Gone: true})
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		if la, lb := strings.ToLower(a.Name), strings.ToLower(b.Name); la != lb {
			return la < lb
		}
		return a.Name < b.Name
	})
	if len(entries) > fsListMax {
		entries, more = entries[:fsListMax], true
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Name != ".git" {
			names = append(names, e.Name)
		}
	}
	ignored, repo := gitIgnored(full, names)
	for i := range entries {
		e := &entries[i]
		// The repository's own directory is no content of the tree: it is
		// marked as ignored entries are, so the default view hides it.
		e.Ignored = e.Name == ".git" || ignored[e.Name]
		switch mark := marks[e.Name]; {
		case e.Ignored:
		case mark != "" && (e.Dir == markDirs[e.Name] || e.Gone):
			e.Status = mark
		case mark != "" && e.Dir:
			// A directory git reports as one path, a submodule: it holds
			// the change whatever its letter.
			e.Status = "*"
		case marks[""] != "":
			e.Status = marks[""]
		}
	}
	return entries, more, repo, nil
}

func fsNotFound(w http.ResponseWriter) { http.Error(w, "not found", http.StatusNotFound) }

func writeFSJSON(w http.ResponseWriter, outside bool, v any) {
	raw, _ := json.Marshal(v)
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	if outside {
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
	}
	_, _ = w.Write(raw)
}

// handleFSRoutes serves the file browser's read routes of one session; sub
// is the path under /s/<reply-id>/fs/.
func (d *Daemon) handleFSRoutes(w http.ResponseWriter, r *http.Request, replyID, sub string) {
	switch {
	case sub == "list":
		d.handleFSList(w, r, replyID)
	case sub == "preview":
		d.handleFSPreview(w, r, replyID)
	case sub == "raw":
		d.handleFSRaw(w, r, replyID)
	case sub == "find":
		d.handleFSFind(w, r, replyID)
	case sub == "search":
		d.handleFSSearch(w, r, replyID)
	case strings.HasPrefix(sub, "cap/"):
		d.handleFSCapability(w, r, replyID, strings.TrimPrefix(sub, "cap/"))
	default:
		http.NotFound(w, r)
	}
}

// handleFSList serves the listing route.
func (d *Daemon) handleFSList(w http.ResponseWriter, r *http.Request, replyID string) {
	root, session, ok := d.browseRoot(r, replyID)
	if !ok {
		fsNotFound(w)
		return
	}
	dir := r.URL.Query().Get("dir")
	if len(dir) > filesQueryMax {
		fsNotFound(w)
		return
	}
	full, rel, ok := containedDir(root, dir)
	if !ok {
		fsNotFound(w)
		return
	}
	entries, more, repo, err := listDir(full)
	if err != nil {
		fsNotFound(w)
		return
	}
	clean := filepath.Clean(root)
	writeFSJSON(w, !session, fsListAnswer{Root: filepath.ToSlash(clean), Name: filepath.Base(clean), Session: session,
		Dir: filepath.ToSlash(rel), Abs: full, Sep: string(filepath.Separator), Repo: repo, More: more, Entries: entries})
}

// fsURL is a file browser read route's URL for the file rel under the
// session replyID; root is set only for a root other than the session's.
func fsURL(replyID, route, rel, root string) string {
	u := "/s/" + replyID + "/fs/" + route + "?path=" + url.QueryEscape(filepath.ToSlash(rel))
	if root != "" {
		u += "&root=" + url.QueryEscape(filepath.ToSlash(root))
	}
	return u
}

// patchDiffs splits the patch a .diff or .patch file holds into one
// GitFileDiff per file, in the commit window's shape, unhighlighted. A file
// starts at a "diff --git" line, or, in a patch that has none, at a "--- "
// line the "+++ " line follows. Text before the first file (a mail header,
// a commit message) is not shown; a patch with no file start is one
// nameless file.
func patchDiffs(data []byte) []GitFileDiff {
	lines := strings.SplitAfter(string(data), "\n")
	git := false
	for _, l := range lines {
		if strings.HasPrefix(l, "diff --git ") {
			git = true
			break
		}
	}
	var sections []string
	var cur strings.Builder
	started := false
	for i, l := range lines {
		start := strings.HasPrefix(l, "diff --git ")
		if !git {
			start = strings.HasPrefix(l, "--- ") && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "+++ ")
		}
		if start {
			if started {
				sections = append(sections, cur.String())
			}
			cur.Reset()
			started = true
		}
		if started {
			cur.WriteString(l)
		}
	}
	if started {
		sections = append(sections, cur.String())
	} else {
		sections = []string{string(data)}
	}
	out := make([]GitFileDiff, 0, len(sections))
	for _, section := range sections {
		hunks, info := parsePatch([]byte(section), false)
		d := GitFileDiff{Path: patchPath(section), Binary: info.binary, Hunks: renderHunks(hunks, nil, nil)}
		if info.gitlink {
			d.Notice = "gitlink"
		}
		out = append(out, d)
	}
	return out
}

// patchPath names the file one patch section changes: its "+++ b/<path>"
// line, else the "--- a/<path>" one (a deletion), else the header's.
func patchPath(section string) string {
	var header, minus string
	for _, l := range strings.Split(section, "\n") {
		if strings.HasPrefix(l, "@@ ") {
			break
		}
		switch {
		case strings.HasPrefix(l, "+++ "):
			if p := patchSide(l[4:]); p != "" {
				return p
			}
		case strings.HasPrefix(l, "--- "):
			minus = patchSide(l[4:])
		case strings.HasPrefix(l, "diff --git "):
			if i := strings.LastIndex(l, " b/"); i >= 0 {
				header = strings.TrimRight(l[i+3:], "\r")
			}
		}
	}
	if minus != "" {
		return minus
	}
	return header
}

// patchSide is the path of a "--- " or "+++ " line: without its a/ or b/
// prefix and any tab-separated timestamp; empty for /dev/null.
func patchSide(s string) string {
	s, _, _ = strings.Cut(s, "\t")
	s = strings.TrimSpace(s)
	if s == "/dev/null" {
		return ""
	}
	if len(s) > 2 && (s[0] == 'a' || s[0] == 'b') && s[1] == '/' {
		return s[2:]
	}
	return s
}

// handleFSPreview serves the preview route.
func (d *Daemon) handleFSPreview(w http.ResponseWriter, r *http.Request, replyID string) {
	root, session, ok := d.browseRoot(r, replyID)
	if !ok {
		fsNotFound(w)
		return
	}
	f, rel, info, ok := openContained(root, r.URL.Query().Get("path"))
	if !ok {
		fsNotFound(w)
		return
	}
	defer f.Close()
	outside := ""
	if !session {
		outside = root
	}
	a := fsPreviewAnswer{Rel: filepath.ToSlash(rel), Abs: filepath.Join(filepath.Clean(root), rel), Size: info.Size()}
	lang, _ := requestLang(r)
	_, image := imageType(rel)
	_, video := videoType(rel)
	framed := framedFile(rel)
	// text=1 asks for the file as plain text whatever it is, which a jump
	// to a line needs (a content search's match).
	plain := r.URL.Query().Get("text") == "1"
	switch {
	case framed && session && !plain && !capDenied(root, rel):
		a.Kind = "frame"
		a.Frame, a.TTL = d.fsCapabilityURL(replyID, root, rel), int(fsCapabilityTTL/time.Second)
	case image && !framed:
		a.Kind, a.Src = "image", fsURL(replyID, "raw", rel, outside)
	case video:
		a.Kind, a.Src = "video", fsURL(replyID, "raw", rel, outside)
	case info.Size() > fileViewMax:
		a.Kind, a.Reason = "none", wsi18n.T(lang, "file.reason.large", fileViewMax>>20)
	default:
		data, err := io.ReadAll(io.LimitReader(f, fileViewMax+1))
		switch {
		case err != nil:
			fsNotFound(w)
			return
		case len(data) > fileViewMax:
			a.Kind, a.Reason = "none", wsi18n.T(lang, "file.reason.large", fileViewMax>>20)
		case !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0:
			a.Kind, a.Reason = "none", wsi18n.T(lang, "file.reason.binary")
		default:
			switch ext := strings.ToLower(filepath.Ext(rel)); {
			case plain:
				a.Kind, a.Text = "text", string(data)
			case framed:
				// Under a root that is not the session's, or for a file
				// the capability route refuses (capDenied), no capability
				// is minted, so the file shows as its source.
				a.Kind, a.Text, a.Source = "text", string(data), true
			case ext == ".md" || ext == ".markdown":
				a.Kind = "markdown"
				// A link or image resolves through the session's routes, so
				// under another root it stays path text.
				targets := wswhiteboard.TextTargets{}
				if session {
					targets = fileViewTargets(root, replyID, filepath.Dir(rel))
				}
				a.HTML = string(wswhiteboard.RenderText(string(data), targets))
			case ext == ".diff" || ext == ".patch":
				a.Kind, a.Diff = "diff", patchDiffs(data)
			default:
				a.Kind, a.Text = "text", string(data)
			}
		}
	}
	writeFSJSON(w, !session, a)
}

// handleFSRaw serves the raw route: an image's or a video's bytes through
// http.ServeContent, so a video seeks by Range, with no size cap. An image
// is served as the image route serves one, so an SVG opened on its own runs
// no script in the daemon's origin.
func (d *Daemon) handleFSRaw(w http.ResponseWriter, r *http.Request, replyID string) {
	root, session, ok := d.browseRoot(r, replyID)
	if !ok {
		fsNotFound(w)
		return
	}
	path := r.URL.Query().Get("path")
	ctype, ok := imageType(path)
	if !ok {
		if ctype, ok = videoType(path); !ok {
			fsNotFound(w)
			return
		}
	}
	f, _, info, ok := openContained(root, path)
	if !ok {
		fsNotFound(w)
		return
	}
	defer f.Close()
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", imageContentSecurityPolicy)
	h.Set("Cache-Control", "no-cache")
	h.Set("Referrer-Policy", "no-referrer")
	if !session {
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
	}
	http.ServeContent(w, r, "", info.ModTime(), f)
}
