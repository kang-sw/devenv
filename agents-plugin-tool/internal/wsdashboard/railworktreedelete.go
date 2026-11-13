package wsdashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsi18n"
)

// Deleting a worktree from the session rail: a linked worktree of a
// repository the rail shows, through a check (the preview route, which
// changes nothing) and a submit that checks again before it runs git
// worktree remove from the primary worktree. The branch is always kept.
// Both routes need a browser on this machine and the daemon's own Origin
// (TrustedWrite): a delete discards files, so the LAN token alone never
// reaches them, and a viewer elsewhere gets 404 as if they did not exist.
//
// A worktree is refused outright when it is the primary worktree, is not in
// the repository's git worktree list, is gone from disk, sits under a ws
// worktree pool (the pool roots the rail's source resolves and, whatever the
// configuration, the default and the legacy in-tree pool roots:
// worktree.remove owns every pooled worktree), is locked, or holds a live
// session or a terminal that may still run (its folder at or below the
// worktree; a terminal by the folder it started in). An ended session and
// an exited terminal there are listed only.
//
// A delete needs force when it discards work: changed or untracked files,
// commits a detached HEAD holds that no branch, tag, or remote ref reaches,
// or initialized submodules (git itself refuses those without --force). A
// branch worktree's commits stay on its kept branch, so they need nothing.
// The check's token sums what force covers (HEAD, the changed paths, the
// unreached count, submodules); a forced submit is refused unless its token
// still matches, so a force covers only the state the user saw. git runs
// with --force only when the fresh check needs it; otherwise git's own
// guards stay on.

const (
	worktreeDeletePreviewRoute = "/rail/worktrees/delete/preview"
	worktreeDeleteRoute        = "/rail/worktrees/delete"

	// worktreeDirtyShown bounds the changed paths a check lists.
	worktreeDirtyShown = 20
)

// Refusals of a delete (worktreeCheck.Refusal).
const (
	deleteUnknown = "unknown" // not in the repository's worktree list
	deletePrimary = "primary" // the repository's primary worktree
	deletePooled  = "pooled"  // under a ws worktree pool
	deleteMissing = "missing" // its folder is gone
	deleteLocked  = "locked"  // git worktree lock holds it
	deleteBusy    = "busy"    // a live session or a running terminal is in it
	// The submit's own refusals: the delete needs force and the submit
	// carried none, or what force covers changed since the check.
	deleteForce   = "force"
	deleteChanged = "changed"
)

// errTerminalsUnknown fails a check made before the daemon's first terminal
// scan finished: a terminal a predecessor left could be missed.
var errTerminalsUnknown = errors.New("terminals not yet known")

// worktreeDeleteRequest is both delete routes' body: the repository the
// rail shows (its workspace path), the worktree as its row names it, and,
// for the submit, the force the modal confirmed with the check's token.
type worktreeDeleteRequest struct {
	Workspace string `json:"workspace"`
	Path      string `json:"path"`
	Force     bool   `json:"force"`
	Token     string `json:"token"`
}

// worktreeCheck is a delete's fresh check: the preview's answer, and the
// body of a refused submit's 409.
type worktreeCheck struct {
	// Path is the worktree as git lists it; Folder is its last element, the
	// name the modal asks the user to type.
	Path   string `json:"path"`
	Folder string `json:"folder"`
	// Branch is the checked-out branch, which a delete keeps; Detached
	// marks a detached HEAD (Branch then empty).
	Branch   string `json:"branch,omitempty"`
	Detached bool   `json:"detached,omitempty"`
	// Locked marks a locked worktree, LockReason the reason git records.
	Locked     bool   `json:"locked,omitempty"`
	LockReason string `json:"lock_reason,omitempty"`
	// Dirty counts the changed and untracked entries (an untracked folder
	// is one); DirtyPaths are the first worktreeDirtyShown of them.
	Dirty      int      `json:"dirty"`
	DirtyPaths []string `json:"dirty_paths,omitempty"`
	// Unreached counts the commits a detached HEAD holds that no branch,
	// tag, or remote ref reaches; zero for a branch worktree.
	Unreached int `json:"unreached"`
	// Submodules marks initialized submodules.
	Submodules bool `json:"submodules,omitempty"`
	// Holders are the sessions and terminals in the worktree.
	Holders []worktreeHolder `json:"holders,omitempty"`
	// Force marks a delete that discards work (Dirty, Unreached, or
	// Submodules): the submit must carry force and Token.
	Force bool   `json:"force"`
	Token string `json:"token,omitempty"`
	// Refusal names why the delete is refused, empty when it is not; Note
	// is its line in the page's language.
	Refusal string `json:"refusal,omitempty"`
	Note    string `json:"note,omitempty"`

	// primary is the primary worktree, where git worktree remove runs.
	primary string
}

// worktreeHolder is a session or terminal whose folder is in a worktree.
type worktreeHolder struct {
	// Kind is "session" or "terminal".
	Kind string `json:"kind"`
	// ID is the session's reply-id or the terminal's id. Label is the
	// session's label, or the reply-id of the session page a terminal was
	// opened from; Harness the session's harness.
	ID      string `json:"id"`
	Label   string `json:"label,omitempty"`
	Harness string `json:"harness,omitempty"`
	// Live marks one that blocks the delete: a live session, a terminal
	// that may still run. An ended session or an exited terminal is listed
	// only.
	Live bool `json:"live"`
}

// worktreeDeleteAnswer is the submit's answer once git removed the worktree.
type worktreeDeleteAnswer struct {
	Path    string `json:"path"`
	Branch  string `json:"branch,omitempty"`
	Message string `json:"message"`
}

// deleteListEntry is one worktree of git worktree list --porcelain, with
// what a delete checks.
type deleteListEntry struct {
	Path, Head, Branch string
	Detached, Prunable bool
	Locked             bool
	LockReason         string
}

// readDeleteList reads root's worktrees, the primary first.
func readDeleteList(ctx context.Context, root string) ([]deleteListEntry, error) {
	out, err := gitOutput(ctx, root, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var entries []deleteListEntry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			entries = append(entries, deleteListEntry{Path: filepath.Clean(strings.TrimSpace(p))})
			continue
		}
		if len(entries) == 0 {
			continue
		}
		e := &entries[len(entries)-1]
		switch {
		case strings.HasPrefix(line, "HEAD "):
			e.Head = strings.TrimSpace(strings.TrimPrefix(line, "HEAD "))
		case strings.HasPrefix(line, "branch "):
			e.Branch = strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(line, "branch ")), "refs/heads/")
		case line == "detached":
			e.Detached = true
		case line == "locked" || strings.HasPrefix(line, "locked "):
			e.Locked = true
			e.LockReason = strings.TrimSpace(strings.TrimPrefix(line, "locked"))
		case line == "prunable" || strings.HasPrefix(line, "prunable "):
			e.Prunable = true
		}
	}
	return entries, nil
}

// pathWithin reports whether child is at or below parent, comparing cleaned
// paths at directory boundaries, case-insensitively on Windows.
func pathWithin(parent, child string) bool {
	p, c := filepath.Clean(parent), filepath.Clean(child)
	if runtime.GOOS == "windows" {
		p, c = strings.ToLower(p), strings.ToLower(c)
	}
	if p == c {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(c, strings.TrimSuffix(p, sep)+sep)
}

// samePath reports two paths that are one once cleaned, case-insensitively
// on Windows.
func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// worktreePoolRoots are the pool roots a dashboard delete refuses under:
// pools (the repository's, as the rail's source resolves its worktree_pool)
// and, whatever the configuration, the default pool root beside primary and
// the legacy in-tree one under it, so a changed configuration never exposes
// an old pool's worktrees.
func worktreePoolRoots(pools []string, primary string) []string {
	roots := append([]string(nil), pools...)
	return append(roots,
		filepath.Join(filepath.Dir(primary), ".ws-worktrees", filepath.Base(primary)),
		filepath.Join(primary, ".ws-worktrees"))
}

// worktreePooled reports a path at or below one of roots.
func worktreePooled(roots []string, path string) bool {
	for _, r := range roots {
		if r != "" && pathWithin(r, path) {
			return true
		}
	}
	return false
}

// repoPools are the pool roots of the repository root as the rail's source
// resolves them, read now; none without a source. A failed read fails the
// check: a pooled worktree must never pass for want of its pool. It calls
// the source off the lister's goroutine, beside its reads: a source read is
// a git list and a config lookup, holding no state of its own.
func (d *Daemon) repoPools(root string) ([]string, error) {
	if d.rail == nil || d.rail.read == nil {
		return nil, nil
	}
	var regs []Registration
	if d.index != nil {
		for _, s := range d.index.Sessions() {
			if r := railRepoRoot(s.Registration); !s.Unlisted && r != "" && samePath(r, root) {
				regs = append(regs, s.Registration)
			}
		}
	}
	got, err := d.rail.read(root, regs)
	if err != nil {
		return nil, fmt.Errorf("read the worktree pool of %s: %w", root, err)
	}
	return got.Pools, nil
}

// worktreeHolds reports a folder at or below the worktree target, compared
// as given and again with symlinks resolved (realTarget is target's).
func worktreeHolds(target, realTarget, folder string) bool {
	if folder == "" || !filepath.IsAbs(folder) {
		return false
	}
	if pathWithin(target, folder) {
		return true
	}
	real, err := filepath.EvalSymlinks(folder)
	return err == nil && pathWithin(realTarget, real)
}

// worktreeHolders are the sessions and terminals in the worktree target
// (see worktreeHolder), the live ones first. An ended session the index page
// does not list is left out.
func worktreeHolders(target string, sessions []Session, terms []TerminalInfo, now time.Time) []worktreeHolder {
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		realTarget = target
	}
	var out []worktreeHolder
	for _, s := range sessions {
		reg := s.Registration
		live := reg.Live(now)
		if (!live && s.Unlisted) || !worktreeHolds(target, realTarget, reg.Project) {
			continue
		}
		out = append(out, worktreeHolder{Kind: "session", ID: reg.ReplyID, Label: s.Label, Harness: reg.Harness, Live: live})
	}
	for _, t := range terms {
		if !worktreeHolds(target, realTarget, t.Cwd) {
			continue
		}
		// A terminal the daemon is not attached to may still run.
		out = append(out, worktreeHolder{Kind: "terminal", ID: t.ID, Label: t.Session, Live: t.State != "exited"})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Live != out[j].Live {
			return out[i].Live
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind == "session"
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// worktreeSubmodules reports initialized submodules in the worktree at
// path, as git worktree remove finds them before it refuses without
// --force: a modules folder in the worktree's own git directory, or a
// submodule .gitmodules names whose folder holds a .git. A gitlink
// .gitmodules does not name is missed; git then refuses the unforced remove
// itself, so a miss costs an error, never work.
func worktreeSubmodules(ctx context.Context, path string) bool {
	if gitdir, err := gitOutput(ctx, path, "rev-parse", "--absolute-git-dir"); err == nil {
		if info, err := os.Stat(filepath.Join(gitdir, "modules")); err == nil && info.IsDir() {
			return true
		}
	}
	// No .gitmodules, or no entry in it, fails the read: no submodules.
	out, err := gitOutput(ctx, path, "config", "--file", ".gitmodules", "--get-regexp", `^submodule\..*\.path$`)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		_, sub, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok || sub == "" {
			continue
		}
		if _, err := os.Lstat(filepath.Join(path, filepath.FromSlash(sub), ".git")); err == nil {
			return true
		}
	}
	return false
}

// worktreeToken sums what force covers in a check: HEAD, every changed
// path, the unreached count, and submodules.
func worktreeToken(c worktreeCheck, head string, dirty []string) string {
	sorted := append([]string(nil), dirty...)
	sort.Strings(sorted)
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%d\x00%t\x00", c.Path, head, c.Unreached, c.Submodules)
	for _, p := range sorted {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}

// checkWorktreeDelete checks a delete of the worktree at path in the
// repository root, as the preview shows it and the submit judges it. A
// structural refusal (unknown, primary, pooled, missing) answers before the
// worktree is read; a lock and the holders are judged after it, so the
// modal shows what the worktree holds. err is a read that failed: the check
// then stands on nothing, and the caller refuses.
func (d *Daemon) checkWorktreeDelete(ctx context.Context, root, path string, now time.Time) (worktreeCheck, error) {
	c := worktreeCheck{Path: path, Folder: lastElem(path)}
	if path == "" || !filepath.IsAbs(path) {
		c.Refusal = deleteUnknown
		return c, nil
	}
	c.Path = filepath.Clean(path)
	entries, err := readDeleteList(ctx, root)
	if err != nil {
		return c, err
	}
	if len(entries) == 0 {
		return c, fmt.Errorf("git listed no worktree in %s", root)
	}
	c.primary = entries[0].Path
	at := -1
	for i, e := range entries {
		if samePath(e.Path, c.Path) {
			at = i
			break
		}
	}
	if at < 0 {
		c.Refusal = deleteUnknown
		return c, nil
	}
	e := entries[at]
	c.Path, c.Folder = e.Path, lastElem(e.Path)
	c.Branch, c.Detached = e.Branch, e.Detached || e.Branch == ""
	if at == 0 {
		c.Refusal = deletePrimary
		return c, nil
	}
	pools, err := d.repoPools(root)
	if err != nil {
		return c, err
	}
	if worktreePooled(worktreePoolRoots(pools, c.primary), c.Path) {
		c.Refusal = deletePooled
		return c, nil
	}
	if info, err := os.Stat(c.Path); e.Prunable || err != nil || !info.IsDir() {
		c.Refusal = deleteMissing
		return c, nil
	}
	c.Locked, c.LockReason = e.Locked, e.LockReason

	run := &gitBadgeRunner{timeout: gitActionTimeout}
	dirty, err := wsgit.PorcelainPaths(ctx, run, c.Path)
	if err != nil {
		return c, fmt.Errorf("read the status of %s: %w", c.Path, err)
	}
	c.Dirty = len(dirty)
	c.DirtyPaths = dirty[:min(len(dirty), worktreeDirtyShown)]
	if c.Detached {
		c.Unreached, err = wsgit.CountUnreachedCommits(ctx, run, c.Path, "HEAD", "--branches", "--tags", "--remotes")
		if err != nil {
			return c, fmt.Errorf("count the commits of %s: %w", c.Path, err)
		}
	}
	c.Submodules = worktreeSubmodules(ctx, c.Path)
	c.Force = c.Dirty > 0 || c.Unreached > 0 || c.Submodules
	c.Token = worktreeToken(c, e.Head, dirty)

	var sessions []Session
	if d.index != nil {
		sessions = d.index.Sessions()
	}
	var terms []TerminalInfo
	if d.terminals != nil {
		if !d.terminals.ready() {
			return c, errTerminalsUnknown
		}
		terms = d.terminals.list()
	}
	c.Holders = worktreeHolders(c.Path, sessions, terms, now)
	switch {
	case c.Locked:
		c.Refusal = deleteLocked
	case len(c.Holders) > 0 && c.Holders[0].Live:
		c.Refusal = deleteBusy
	}
	return c, nil
}

// worktreeDeleteNote is a refusal's line in lang; empty for none.
func worktreeDeleteNote(lang string, c worktreeCheck) string {
	switch c.Refusal {
	case deleteUnknown:
		return wsi18n.T(lang, "rail.wt.del.unknown", c.Path)
	case deletePrimary:
		return wsi18n.T(lang, "rail.wt.del.primary")
	case deletePooled:
		return wsi18n.T(lang, "rail.wt.del.pooled", c.Folder)
	case deleteMissing:
		return wsi18n.T(lang, "rail.wt.del.missing", c.Path)
	case deleteLocked:
		return wsi18n.T(lang, "rail.wt.del.locked", c.Folder)
	case deleteBusy:
		return wsi18n.T(lang, "rail.wt.del.busy", c.Folder)
	case deleteForce:
		return wsi18n.T(lang, "rail.wt.del.force", c.Folder)
	case deleteChanged:
		return wsi18n.T(lang, "rail.wt.del.changed", c.Folder)
	}
	return ""
}

// worktreeDeleteBody runs the delete routes' gate ahead of the workspace
// routes' shared checks (workspaceBody): a browser on this machine, else
// 404 as if the route did not exist; then the method, the daemon's own
// Origin (403 without it), and the body.
func worktreeDeleteBody(w http.ResponseWriter, r *http.Request, into any) bool {
	if local, _ := TrustedWrite(r); !local {
		http.NotFound(w, r)
		return false
	}
	return workspaceBody(w, r, into)
}

// worktreeCheckFailed answers a check whose read failed: 503 while the
// terminals are not yet known, else 502 with the failure's line.
func worktreeCheckFailed(w http.ResponseWriter, lang string, err error) {
	if errors.Is(err, errTerminalsUnknown) {
		http.Error(w, wsi18n.T(lang, "rail.wt.del.terminals"), http.StatusServiceUnavailable)
		return
	}
	http.Error(w, gitErrText(err), http.StatusBadGateway)
}

// handleWorktreeDeletePreview serves the preview route: 200 with the
// worktreeCheck (a refused delete is a 200 too, its refusal and note saying
// why), 404 for a viewer elsewhere or a workspace that is no repository the
// rail shows, and 502 or 503 when the check could not be read.
func (d *Daemon) handleWorktreeDeletePreview(w http.ResponseWriter, r *http.Request) {
	var req worktreeDeleteRequest
	if !worktreeDeleteBody(w, r, &req) {
		return
	}
	lang, _ := requestLang(r)
	root, ok := d.railRepository(req.Workspace)
	if !ok {
		http.Error(w, "unknown workspace", http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitActionTimeout)
	defer cancel()
	c, err := d.checkWorktreeDelete(ctx, root, req.Path, time.Now())
	if err != nil {
		worktreeCheckFailed(w, lang, err)
		return
	}
	c.Note = worktreeDeleteNote(lang, c)
	writeJSON(w, http.StatusOK, c)
}

// handleWorktreeDelete serves the submit route. It checks the worktree
// again and answers 409 with that worktreeCheck when the delete is refused,
// needs force the submit did not carry, or was forced on a token the
// worktree no longer matches; 409 with text while another git action of the
// repository runs; 404 as the preview does; 502 with git's line when the
// check or git failed. Once git removed the worktree it answers 200 with a
// worktreeDeleteAnswer, and the rail reads the repository's worktrees again.
func (d *Daemon) handleWorktreeDelete(w http.ResponseWriter, r *http.Request) {
	var req worktreeDeleteRequest
	if !worktreeDeleteBody(w, r, &req) {
		return
	}
	lang, _ := requestLang(r)
	root, ok := d.railRepository(req.Workspace)
	if !ok {
		http.Error(w, "unknown workspace", http.StatusNotFound)
		return
	}
	release, ok := d.beginGitAction(root)
	if !ok {
		http.Error(w, wsi18n.T(lang, "git.busy"), http.StatusConflict)
		return
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), gitActionTimeout)
	defer cancel()
	c, err := d.checkWorktreeDelete(ctx, root, req.Path, time.Now())
	if err != nil {
		worktreeCheckFailed(w, lang, err)
		return
	}
	if c.Refusal == "" && c.Force {
		switch {
		case !req.Force:
			c.Refusal = deleteForce
		case req.Token != c.Token:
			c.Refusal = deleteChanged
		}
	}
	if c.Refusal != "" {
		c.Note = worktreeDeleteNote(lang, c)
		writeJSON(w, http.StatusConflict, c)
		return
	}
	args := []string{"worktree", "remove"}
	if c.Force {
		args = append(args, "--force")
	}
	if err := runGitAction(ctx, c.primary, append(args, "--", c.Path)...); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	fmt.Fprintf(d.log, "dashboard: deleted the worktree %s (force %t)\n", c.Path, c.Force)
	d.rail.reread(root)
	d.broker.wake(railListRepoKey)
	msg := wsi18n.T(lang, "rail.wt.deleted", c.Path)
	if c.Branch != "" {
		msg = wsi18n.T(lang, "rail.wt.deleted.branch", c.Path, c.Branch)
	}
	writeJSON(w, http.StatusOK, worktreeDeleteAnswer{Path: c.Path, Branch: c.Branch, Message: msg})
}
