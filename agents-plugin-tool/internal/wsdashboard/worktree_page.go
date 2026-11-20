package wsdashboard

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// worktreePagePath carries the absolute path in slash form, with the Unix
// leading slash supplied by /w/ itself. Windows paths retain their drive.
func worktreePagePath(path string) string {
	return (&url.URL{Path: "/w/" + strings.TrimPrefix(filepath.ToSlash(path), "/")}).EscapedPath()
}

// The rail's current worktree inventory is the page allowlist. This does not
// grant filesystem or terminal authority: those requests still pass host0's
// existing explicit-root, local-viewer and host-action guards.
func (d *Daemon) handleWorktreePage(w http.ResponseWriter, r *http.Request) {
	if !readMethod(w, r) {
		return
	}
	if !localBrowseViewer(r) {
		http.Error(w, "worktree pages are local only", http.StatusForbidden)
		return
	}
	lang, pref := requestLang(r)
	railView := d.railOrderedView(d.pagesIn(lang))
	var workspace string
	var selected RailWorktree
	// Bookmarks can be the first request after a restart, before a shell
	// stream warms the lister. Refresh from that same source and projection
	// for both authorization and the branch header (including detached HEAD).
	for _, ws := range railView.Workspaces {
		worktrees := ws.Worktrees
		if ws.Repo && d.rail != nil {
			var regs []Registration
			for _, s := range d.index.Sessions() {
				if !s.Unlisted && railRepoRoot(s.Registration) == ws.Path {
					regs = append(regs, s.Registration)
				}
			}
			if repo := d.rail.readRepo(ws.Path, regs); repo.Read {
				fresh := buildRailView(nil, map[string]railRepo{filepath.Clean(ws.Path): repo},
					[]storedWorkspace{{Path: ws.Path, Repo: true}}, time.Now())
				worktrees = fresh.Workspaces[0].Worktrees
			}
		}
		for _, wt := range worktrees {
			if r.URL.EscapedPath() == worktreePagePath(wt.Path) {
				workspace, selected = ws.Name, wt
				break
			}
		}
		if selected.Path != "" {
			break
		}
	}
	if selected.Path == "" {
		http.NotFound(w, r)
		return
	}
	if !framedRequest(r) {
		d.serveShell(w, r, nil, worktreePagePath(selected.Path)+"?frame=1")
		return
	}
	branch := selected.Branch
	if selected.Detached {
		branch = wsi18n.T(lang, "rail.detached")
	}
	page := sessionPageData{SessionView: SessionView{Lang: lang, LangPref: pref, Theme: requestTheme(r)},
		Worktree: filepath.ToSlash(selected.Path), Workspace: workspace, WorktreeBranch: branch,
		SysOpen: true, Frame: true, regions: regionSet{"board": newRegion("")}}
	if d.offersTerminals(r) {
		page.Terminal, _ = XtermAsset()
	}
	a, err := loadDashboardAssets()
	if err == nil {
		var head bytes.Buffer
		err = a.templateFor(lang).ExecuteTemplate(&head, "worktree-head", page)
		if err == nil {
			page.regions[regionHead] = newRegion(head.String())
			var html string
			html, err = renderSessionPageData(page)
			if err == nil {
				admitShellFrame(w, sessionPageCSP(true))
				writeHTML(w, html)
				return
			}
		}
	}
	fmt.Fprintf(d.log, "dashboard: render worktree: %v\n", err)
	http.Error(w, "render failed", http.StatusInternalServerError)
}
