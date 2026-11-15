package wsdashboard

import (
	"net/http"
	"path/filepath"
)

// fsRootChoice is a navigation suggestion, not an authorization grant:
// selecting it still passes through browseRoot and the usual Files guards.
type fsRootChoice struct {
	Path     string `json:"path"`
	Branch   string `json:"branch"`
	Detached bool   `json:"detached"`
}

type fsRootsAnswer struct {
	Roots []fsRootChoice `json:"roots"`
}

// GET /s/<reply-id>/fs/worktrees lists the session root followed by its repository's
// other worktrees. Read the source directly: the rail's cache is populated only
// while a rail stream is open, whereas an explorer can be used independently.
func (d *Daemon) handleFSRoots(w http.ResponseWriter, r *http.Request, replyID string) {
	// Absolute paths and branch names must not reach a remote or cross-site
	// viewer, even when the session itself is otherwise readable.
	if !localViewer(r) || crossSiteFetch(r) || d.index == nil || !ValidReplyID(replyID) {
		fsNotFound(w)
		return
	}
	s, ok := d.index.Session(replyID)
	if !ok || s.Project == "" {
		fsNotFound(w)
		return
	}
	if !readMethod(w, r) {
		return
	}
	root := filepath.Clean(s.Project)
	a := fsRootsAnswer{Roots: []fsRootChoice{{Path: filepath.ToSlash(root)}}}
	if src, ok := d.worktrees.(RepoWorktreeSource); ok {
		if repo, err := src.ReadRepoWorktrees(s.Project, []Registration{s.Registration}); err == nil {
			seen := map[string]bool{root: true}
			for _, wt := range repo.Worktrees {
				path := filepath.Clean(wt.Path)
				choice := fsRootChoice{Path: filepath.ToSlash(path), Branch: wt.Branch, Detached: wt.Detached}
				if path == root {
					a.Roots[0] = choice
					continue
				}
				if seen[path] || wt.Prunable {
					continue
				}
				if _, valid := browseRootParam(path); !valid {
					continue
				}
				seen[path] = true
				a.Roots = append(a.Roots, choice)
			}
		}
	}
	// A plain folder or unavailable Git source still offers the session root.
	writeFSJSON(w, true, a)
}
