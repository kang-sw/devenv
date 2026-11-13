package wsdashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// The session rail's workspace routes, served by a daemon that keeps stored
// workspaces (DaemonOptions.WorkspacesFile). Each is a POST with a JSON
// body that needs the daemon's own Origin (sameOrigin) and nothing more, as
// the git drawer's push, pull, and fetch do: a LAN viewer holding the token
// can use them, since the folder a viewer names is on the daemon's machine
// whichever machine the browser is on.
//
//   - /rail/workspaces/preview {"path"} reads a pasted path as the daemon's
//     OS reads it (workspacepath.go) and answers what adding it would do,
//     or why it is refused; it changes nothing.
//   - /rail/workspaces/add {"path"} adds the workspace the path resolves
//     to: the repository's root worktree for a path inside a repository,
//     else the folder itself.
//   - /rail/workspaces/forget {"path"} removes a stored workspace from the
//     list. It never touches the folder.
//   - /rail/worktrees/options {"workspace"} answers a repository
//     workspace's branches and default folder for the add-worktree form.
//   - /rail/worktrees/add {"workspace","branch","new","base","folder"} runs
//     git worktree add in a repository workspace (railworktreeadd.go).
//   - /rail/worktrees/delete/preview {"workspace","path"} and
//     /rail/worktrees/delete {"workspace","path","force","token"} check and
//     run git worktree remove of a linked worktree (railworktreedelete.go).
//     Unlike the others they also need a browser on this machine: a delete
//     discards files.
//
// The worktree routes act only on a repository the rail shows: a stored
// workspace, or the repository of a listed session. Every answer's text is
// in the page's language, for the popover or a toast.

const (
	workspacePreviewRoute = "/rail/workspaces/preview"
	workspaceAddRoute     = "/rail/workspaces/add"
	workspaceForgetRoute  = "/rail/workspaces/forget"
	worktreeOptionsRoute  = "/rail/worktrees/options"
	worktreeAddRoute      = "/rail/worktrees/add"

	// workspaceBodyLimit bounds a workspace route's body.
	workspaceBodyLimit = 16 << 10
)

// workspaceRoutes registers the routes on mux.
func (d *Daemon) workspaceRoutes(mux *http.ServeMux) {
	mux.HandleFunc(workspacePreviewRoute, d.handleWorkspacePreview)
	mux.HandleFunc(workspaceAddRoute, d.handleWorkspaceAdd)
	mux.HandleFunc(workspaceForgetRoute, d.handleWorkspaceForget)
	mux.HandleFunc(worktreeOptionsRoute, d.handleWorktreeOptions)
	mux.HandleFunc(worktreeAddRoute, d.handleWorktreeAdd)
	mux.HandleFunc(worktreeDeletePreviewRoute, d.handleWorktreeDeletePreview)
	mux.HandleFunc(worktreeDeleteRoute, d.handleWorktreeDelete)
}

// workspaceBody runs the routes' shared checks and decodes the body into
// into. On a refusal it has answered the request and ok is false.
func workspaceBody(w http.ResponseWriter, r *http.Request, into any) (ok bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, workspaceBodyLimit))
	if err != nil {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return false
	}
	if err := json.Unmarshal(raw, into); err != nil {
		http.Error(w, "malformed request body", http.StatusBadRequest)
		return false
	}
	return true
}

// workspacePathRequest is the body of the routes that take a pasted path.
type workspacePathRequest struct {
	Path string `json:"path"`
}

// workspacePreview is the preview route's answer. With OK unset, Refusal
// names why the path is refused and Note says so; else Path is the path as
// read, Root the workspace an add would concern, Result what it would do,
// and Note a line saying so when that is not plainly adding Path.
type workspacePreview struct {
	OK      bool   `json:"ok"`
	Refusal string `json:"refusal,omitempty"`
	Path    string `json:"path,omitempty"`
	Root    string `json:"root,omitempty"`
	Repo    bool   `json:"repo,omitempty"`
	Result  string `json:"result,omitempty"`
	Note    string `json:"note,omitempty"`
}

// workspaceAddAnswer is the add route's answer: what the add did, the workspace
// concerned, and the toast's text.
type workspaceAddAnswer struct {
	Result  string `json:"result"`
	Root    string `json:"root"`
	Repo    bool   `json:"repo"`
	Message string `json:"message"`
}

// readWorkspaceFolder reads a pasted path as an existing folder on the
// daemon's machine.
func readWorkspaceFolder(raw string, host pathHost) (string, pathRefusal) {
	path, refusal := normalizeWorkspacePath(raw, host)
	if refusal != "" {
		return "", refusal
	}
	info, err := os.Stat(path)
	switch {
	case err != nil:
		return path, pathMissing
	case !info.IsDir():
		return path, pathNotDir
	}
	return filepath.Clean(path), ""
}

// pathRefusalText is a path refusal's line in lang; path is the path as
// read, when the refusal came after the read.
func pathRefusalText(lang string, refusal pathRefusal, path string) string {
	switch refusal {
	case pathEmpty:
		return wsi18n.T(lang, "rail.ws.refuse.empty")
	case pathShare:
		return wsi18n.T(lang, "rail.ws.refuse.share")
	case pathDistro:
		return wsi18n.T(lang, "rail.ws.refuse.distro")
	case pathMissing:
		return wsi18n.T(lang, "rail.ws.refuse.missing", path)
	case pathNotDir:
		return wsi18n.T(lang, "rail.ws.refuse.notdir", path)
	}
	return wsi18n.T(lang, "rail.ws.refuse.relative")
}

// workspaceAddText is an add's line in lang: the toast after an add.
func workspaceAddText(lang string, a workspaceAdd) string {
	switch {
	case a.Result == workspaceMerged:
		return wsi18n.T(lang, "rail.ws.merged", a.Worktree, a.Root)
	case a.Result == workspaceKnown && a.Worktree != "":
		return wsi18n.T(lang, "rail.ws.known.worktree", a.Worktree, a.Root)
	case a.Result == workspaceKnown:
		return wsi18n.T(lang, "rail.ws.known", a.Root)
	case a.Root != a.Path:
		return wsi18n.T(lang, "rail.ws.added.root", a.Root)
	}
	return wsi18n.T(lang, "rail.ws.added", a.Root)
}

// workspacePreviewText is the preview's note in lang for an add that has
// not happened: empty when the add would plainly add the pasted path.
func workspacePreviewText(lang string, a workspaceAdd) string {
	switch {
	case a.Result == workspaceMerged:
		return wsi18n.T(lang, "rail.ws.preview.merged", a.Worktree, a.Root)
	case a.Result == workspaceKnown:
		return workspaceAddText(lang, a)
	case a.Root != a.Path:
		return wsi18n.T(lang, "rail.ws.preview.root", a.Root)
	}
	return ""
}

// handleWorkspacePreview serves the preview route: always 200 with a
// workspacePreview once the request passed the shared checks.
func (d *Daemon) handleWorkspacePreview(w http.ResponseWriter, r *http.Request) {
	var req workspacePathRequest
	if !workspaceBody(w, r, &req) {
		return
	}
	lang, _ := requestLang(r)
	path, refusal := readWorkspaceFolder(req.Path, d.pathHost)
	if refusal != "" {
		writeJSON(w, http.StatusOK, workspacePreview{Refusal: string(refusal), Path: path, Note: pathRefusalText(lang, refusal, path)})
		return
	}
	a, err := d.workspaces.add(path, true)
	if err != nil {
		http.Error(w, wsi18n.T(lang, "rail.ws.storefailed"), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, workspacePreview{OK: true, Path: a.Path, Root: a.Root, Repo: a.Repo, Result: string(a.Result), Note: workspacePreviewText(lang, a)})
}

// handleWorkspaceAdd serves the add route: 200 with a workspaceAddAnswer (a
// path that changes nothing is a 200 too, its message saying so), 409 with
// the reason for a refused path, and 500 when the list could not be
// written. The rail shows the result at once.
func (d *Daemon) handleWorkspaceAdd(w http.ResponseWriter, r *http.Request) {
	var req workspacePathRequest
	if !workspaceBody(w, r, &req) {
		return
	}
	lang, _ := requestLang(r)
	path, refusal := readWorkspaceFolder(req.Path, d.pathHost)
	if refusal != "" {
		http.Error(w, pathRefusalText(lang, refusal, path), http.StatusConflict)
		return
	}
	a, err := d.workspaces.add(path, false)
	if err != nil {
		fmt.Fprintf(d.log, "dashboard: add workspace: %v\n", err)
		http.Error(w, wsi18n.T(lang, "rail.ws.storefailed"), http.StatusInternalServerError)
		return
	}
	d.broker.wake(railListRepoKey)
	writeJSON(w, http.StatusOK, workspaceAddAnswer{Result: string(a.Result), Root: a.Root, Repo: a.Repo, Message: workspaceAddText(lang, a)})
}

// handleWorkspaceForget serves the forget route: 204 once the entry is out
// of the list, 404 for a path that is not in it.
func (d *Daemon) handleWorkspaceForget(w http.ResponseWriter, r *http.Request) {
	var req workspacePathRequest
	if !workspaceBody(w, r, &req) {
		return
	}
	lang, _ := requestLang(r)
	if req.Path == "" || !filepath.IsAbs(req.Path) {
		http.Error(w, "unknown workspace", http.StatusNotFound)
		return
	}
	ok, err := d.workspaces.forget(req.Path)
	if err != nil {
		fmt.Fprintf(d.log, "dashboard: forget workspace: %v\n", err)
		http.Error(w, wsi18n.T(lang, "rail.ws.storefailed"), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "unknown workspace", http.StatusNotFound)
		return
	}
	d.broker.wake(railListRepoKey)
	w.WriteHeader(http.StatusNoContent)
}

// railRepository is the repository workspace path names when the rail shows
// one there: a stored repository, or the repository of a listed session.
func (d *Daemon) railRepository(path string) (string, bool) {
	if path == "" || !filepath.IsAbs(path) {
		return "", false
	}
	path = filepath.Clean(path)
	for _, ws := range d.workspaces.workspaces() {
		if ws.Repo && ws.Path == path {
			return path, true
		}
	}
	if d.index == nil {
		return "", false
	}
	for _, s := range d.index.Sessions() {
		reg := s.Registration
		if s.Unlisted {
			continue
		}
		if root := railRepoRoot(reg); root != "" && filepath.Clean(root) == path {
			return path, true
		}
	}
	return "", false
}

// worktreeOptionsRequest is the options route's body.
type worktreeOptionsRequest struct {
	Workspace string `json:"workspace"`
}

// handleWorktreeOptions serves the options route: 200 with the
// worktreeOptions, 404 for a path that is no repository the rail shows, 502
// with git's line when the read failed.
func (d *Daemon) handleWorktreeOptions(w http.ResponseWriter, r *http.Request) {
	var req worktreeOptionsRequest
	if !workspaceBody(w, r, &req) {
		return
	}
	now := time.Now()
	root, ok := d.railRepository(req.Workspace)
	if !ok {
		http.Error(w, "unknown workspace", http.StatusNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitActionTimeout)
	defer cancel()
	opts, err := readWorktreeOptions(ctx, root, now)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, opts)
}

// worktreeAddRequest is the add-worktree route's body. Folder is a pasted
// path, read as the add-workspace form's is; empty takes the default.
type worktreeAddRequest struct {
	Workspace string `json:"workspace"`
	Branch    string `json:"branch"`
	New       bool   `json:"new"`
	Base      string `json:"base"`
	Folder    string `json:"folder"`
}

// worktreeAddAnswer is the add-worktree route's answer.
type worktreeAddAnswer struct {
	Path    string `json:"path"`
	Branch  string `json:"branch"`
	Message string `json:"message"`
}

// worktreeRefusalText is a worktree refusal's line in lang.
func worktreeRefusalText(lang string, refusal worktreeRefusal, req worktreeAddRequest, folder string) string {
	switch refusal {
	case worktreeBranchTaken:
		return wsi18n.T(lang, "rail.wt.refuse.exists", req.Branch)
	case worktreeNoBranch:
		return wsi18n.T(lang, "rail.wt.refuse.nobranch", req.Branch)
	case worktreeCheckedOut:
		return wsi18n.T(lang, "rail.wt.refuse.checkedout", req.Branch)
	case worktreeNoBase:
		return wsi18n.T(lang, "rail.wt.refuse.nobase", req.Base)
	case worktreeFolderInUse:
		return wsi18n.T(lang, "rail.wt.refuse.folderinuse", folder)
	case worktreeBadFolder:
		return wsi18n.T(lang, "rail.ws.refuse.relative")
	}
	return wsi18n.T(lang, "rail.wt.refuse.badbranch")
}

// handleWorktreeAdd serves the add-worktree route: 200 with a
// worktreeAddAnswer once git added it, 404 for a path that is no repository the
// rail shows, 409 with the reason when the daemon refused the request or
// another git action of the root runs, and 502 with git's first meaningful
// line when git failed. The rail reads the repository's worktrees again at
// once.
func (d *Daemon) handleWorktreeAdd(w http.ResponseWriter, r *http.Request) {
	var req worktreeAddRequest
	if !workspaceBody(w, r, &req) {
		return
	}
	lang, _ := requestLang(r)
	now := time.Now()
	root, ok := d.railRepository(req.Workspace)
	if !ok {
		http.Error(w, "unknown workspace", http.StatusNotFound)
		return
	}
	add := worktreeRequest{Branch: req.Branch, New: req.New, Base: req.Base}
	if req.Folder != "" {
		folder, refusal := normalizeWorkspacePath(req.Folder, d.pathHost)
		if refusal != "" {
			http.Error(w, pathRefusalText(lang, refusal, folder), http.StatusConflict)
			return
		}
		add.Folder = folder
	}
	release, ok := d.beginGitAction(root)
	if !ok {
		http.Error(w, wsi18n.T(lang, "git.busy"), http.StatusConflict)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitActionTimeout)
	folder, refusal, err := addWorktree(ctx, root, add, now)
	cancel()
	release()
	switch {
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	case refusal != "":
		http.Error(w, worktreeRefusalText(lang, refusal, req, add.Folder), http.StatusConflict)
		return
	}
	d.rail.reread(root)
	d.broker.wake(railListRepoKey)
	writeJSON(w, http.StatusOK, worktreeAddAnswer{Path: folder, Branch: req.Branch, Message: wsi18n.T(lang, "rail.wt.added", req.Branch, folder)})
}
