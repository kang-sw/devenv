// Page routes serve the index, session, and shell views.

package wsdashboard

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// handleIndex serves / as an empty-selection session layout. The shell
// owns the rail and Settings; its frame has an empty main panel, not a
// missing-session finding or a silently chosen session.
func (d *Daemon) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !readMethod(w, r) {
		return
	}
	if !framedRequest(r) {
		d.serveShell(w, r, nil, "/?frame=1")
		return
	}
	lang, pref := requestLang(r)
	view := IndexView{Version: d.endpoint.Version, Lang: lang, LangPref: pref, Theme: requestTheme(r)}
	html, err := renderIndexPage(view, nil, nil, localViewer(r), true)
	if err == nil {
		admitShellFrame(w, frameAncestorsSelf)
		writeHTML(w, html)
		return
	}
	fmt.Fprintf(d.log, "dashboard: render index: %v\n", err)
	http.Error(w, "render failed", http.StatusInternalServerError)
}

// handleSessionRoutes serves everything under /s/: a session page, its SSE
// stream, its image, file view, files, history, entry text, closed-topic,
// topic-history, worktree pool,
// git status, log, and commit, attachment, and file browser routes, its
// message, cancel, steer, interrupt, system-open, file browser host,
// outside-link mint, git
// refresh, git push, pull, and fetch, worktree cleanup, and display label
// routes, and the shared assets a page loads by relative path.
func (d *Daemon) handleSessionRoutes(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/s/")
	// The host anchor is not a session, including at the page's redirect
	// path. Refuse before any session handler or method-specific response.
	if replyID, sub, _ := strings.Cut(rest, "/"); replyID == LocalBrowseAnchor &&
		!strings.HasPrefix(sub, "fs/") && sub != "terminals" && !strings.HasPrefix(sub, "terminals/") {
		http.NotFound(w, r)
		return
	}
	if replyID, sub, _ := strings.Cut(rest, "/"); sub == "message" || sub == "cancel" || sub == "steer" || sub == "interrupt" {
		d.handleInboxWrite(w, r, replyID, sub)
		return
	} else if sub == "notes" {
		d.handleNotes(w, r, replyID)
		return
	} else if sub == "open" {
		d.handleSystemOpen(w, r, replyID)
		return
	} else if sub == "terminals" || strings.HasPrefix(sub, "terminals/") {
		d.handleTerminals(w, r, replyID, strings.TrimPrefix(sub, "terminals"))
		return
	} else if sub == "fs/act" {
		d.handleFSAct(w, r, replyID)
		return
	} else if sub == "fs/outside" {
		d.handleFSOutside(w, r, replyID)
		return
	} else if sub == "git" {
		d.handleGitRefresh(w, r, replyID)
		return
	} else if action, ok := strings.CutPrefix(sub, "git/"); ok && gitActions[action] {
		d.handleGitAction(w, r, replyID, action)
		return
	} else if sub == "worktrees/cleanup" {
		d.handleWorktreeCleanup(w, r, replyID)
		return
	} else if sub == "hiring" {
		d.handleHiringFlag(w, r, replyID)
		return
	} else if sub == "label" {
		d.handleSessionLabel(w, r, replyID)
		return
	} else if sub == "web" || strings.HasPrefix(sub, "web/") {
		d.handleWeb(w, r, replyID, strings.TrimPrefix(sub, "web"))
		return
	}
	if !readMethod(w, r) {
		return
	}
	if strings.HasPrefix(rest, "dashboard-") && (strings.HasSuffix(rest, ".js") || strings.HasSuffix(rest, ".css")) {
		a, err := loadDashboardAssets()
		if err != nil {
			http.Error(w, "assets unavailable", http.StatusInternalServerError)
			return
		}
		file, ok := a.files[rest]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", file.ctype)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write([]byte(file.body))
		return
	}
	if name, _ := wswhiteboard.MermaidAsset(); rest == name {
		serveMermaid(w)
		return
	}
	if data, ctype, ok := wswhiteboard.FrameAsset(rest); ok {
		serveFrameAsset(w, data, ctype)
		return
	}
	if name, _ := XtermAsset(); rest == name {
		serveXterm(w)
		return
	}
	if name, _ := TermSymbolsAsset(); rest == name {
		serveTermSymbols(w)
		return
	}
	if rest == harnessFontName() {
		serveHarnessFont(w)
		return
	}
	if data, ok := GuideAsset(rest); ok {
		serveGuideAsset(w, data)
		return
	}
	replyID, sub, slash := strings.Cut(rest, "/")
	switch {
	case !slash:
		d.handleSessionPage(w, r, replyID)
	case sub == "" && ValidReplyID(replyID):
		// The page loads its assets by relative path, so it is served at
		// one URL only.
		http.Redirect(w, r, "/s/"+replyID, http.StatusMovedPermanently)
	case sub == "events":
		d.handleSessionEvents(w, r, replyID)
	case sub == "image":
		d.handleImage(w, r, replyID)
	case sub == "file":
		d.handleFile(w, r, replyID)
	case sub == "files":
		d.handleFiles(w, r, replyID)
	case sub == "history":
		d.handleHistory(w, r, replyID)
	case sub == "entry":
		d.handleEntryText(w, r, replyID)
	case sub == "closed":
		d.handleClosedTopics(w, r, replyID)
	case sub == "topic":
		d.handleTopicHistory(w, r, replyID)
	case sub == "board.zip":
		d.handleBoardArchive(w, r, replyID)
	case sub == "tickets/idea":
		d.handleTicketIdeas(w, r, replyID)
	case sub == "tickets/graph":
		d.handleTicketGraph(w, r, replyID)
	case sub == "worktrees":
		d.handleWorktrees(w, r, replyID)
	case sub == "git/status":
		d.handleGitStatus(w, r, replyID)
	case sub == "git/log":
		d.handleGitLog(w, r, replyID)
	case strings.HasPrefix(sub, "git/commit/") && strings.HasSuffix(sub, "/diff"):
		d.handleGitCommitDiff(w, r, replyID, strings.TrimSuffix(strings.TrimPrefix(sub, "git/commit/"), "/diff"))
	case strings.HasPrefix(sub, "git/commit/"):
		d.handleGitCommit(w, r, replyID, strings.TrimPrefix(sub, "git/commit/"))
	case sub == "git/worktree/diff":
		d.handleGitWorktreeDiff(w, r, replyID)
	case strings.HasPrefix(sub, "attachment/"):
		d.handleAttachment(w, r, replyID, strings.TrimPrefix(sub, "attachment/"))
	case strings.HasPrefix(sub, "fs/"):
		d.handleFSRoutes(w, r, replyID, strings.TrimPrefix(sub, "fs/"))
	default:
		http.NotFound(w, r)
	}
}

// handleSessionPage serves /s/<reply-id>: the iframe shell, which keeps the
// rail and the top header across session switches and loads each session
// page in a frame of its own, or with ?frame=1 that session page as the
// shell frames it (framedRequest).
func (d *Daemon) handleSessionPage(w http.ResponseWriter, r *http.Request, replyID string) {
	frame := framedRequest(r)
	lang, pref := requestLang(r)
	var view SessionView
	var regions regionSet
	var ok bool
	var readErr error
	if frame {
		// The frame's read is also its stream's first frame: it renders
		// through the shared page, held for the stream the frame opens next,
		// so a page open costs one read pass instead of two.
		src := d.sessionPageSourceWith(replyID, lang, requestTheme(r), true)
		page := d.cache.acquire(src.key)
		pr := d.cache.renderFresh(page, src)
		readErr, ok = pr.err, pr.ok && pr.view != nil
		if readErr == nil && ok {
			view, regions = *pr.view, pr.regions
			d.cache.holdFor(src.key, page, pageHandoffHold)
		} else {
			d.cache.release(src.key, page)
		}
	} else {
		reader := d.pagesIn(lang)
		reader.freshSnapshots = true
		view, _, _, ok, readErr = reader.readSessionWith(replyID, nil, true, true)
	}
	if readErr != nil {
		http.Error(w, "session temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	if !ok {
		if ValidReplyID(replyID) {
			d.serveFindingSession(w, r, replyID, lang)
			return
		}
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	view.Lang, view.LangPref, view.Theme = lang, pref, requestTheme(r)
	if !frame {
		d.serveShell(w, r, &view, "/s/"+replyID+"?frame=1")
		return
	}
	// The shared render already set Git and rendered the regions, which
	// depend on neither Worktrees, Holders nor LangPref.
	view.Worktrees = d.worktrees != nil
	view.Holders = d.holders != nil
	view.CanEditSettings = view.Hosted && !view.IsChild && localViewer(r)
	err := errors.New("session regions failed to render")
	if regions != nil {
		var html string
		page := sessionPageData{SessionView: view, SysOpen: localViewer(r), Frame: true, regions: regions}
		if d.offersTerminals(r) {
			page.Terminal, _ = XtermAsset()
		}
		if html, err = renderSessionPageData(page); err == nil {
			admitShellFrame(w, sessionPageCSP(true))
			writeHTML(w, html)
			return
		}
	}
	fmt.Fprintf(d.log, "dashboard: render session %s: %v\n", replyID, err)
	http.Error(w, "render failed", http.StatusInternalServerError)
}

// serveShell serves the iframe shell whose first frame loads src: the
// session rail, rendered as its stream renders it (railPageSource), and the
// top header, holding view's head when the shell opens on a session; with
// no view, the root has an empty header and no selected session.
func (d *Daemon) serveShell(w http.ResponseWriter, r *http.Request, view *SessionView, src string) {
	d.serveShellStatus(w, r, view, src, http.StatusOK)
}

func (d *Daemon) serveShellStatus(w http.ResponseWriter, r *http.Request, view *SessionView, src string, status int) {
	lang, pref := requestLang(r)
	page := shellPageData{Lang: lang, LangPref: pref, Theme: requestTheme(r), SysOpen: localViewer(r), Src: src, Title: "Werkschaft (alpha)", Version: d.endpoint.Version, LAN: d.lanAccessFor(r)}
	if view != nil {
		page.RID = view.ReplyID
		page.Title = view.Label
		if page.Title == "" {
			page.Title = view.ReplyID
		}
		page.Title += " - Werkschaft (alpha)"
	}
	rail, err := renderRailRegion(d.railView(d.pagesIn(lang)), lang)
	if err == nil {
		page.regions = regionSet{regionRail: rail}
		var html string
		if html, err = renderShellPage(page, view); err == nil {
			w.Header().Set("Content-Security-Policy", sessionPageCSP(false))
			if status != http.StatusOK {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				w.WriteHeader(status)
			}
			writeHTML(w, html)
			return
		}
	}
	fmt.Fprintf(d.log, "dashboard: render shell: %v\n", err)
	http.Error(w, "render failed", http.StatusInternalServerError)
}

// framedRequest reports a request for the page a shell frames (?frame=1).
func framedRequest(r *http.Request) bool { return r.URL.Query().Get("frame") == "1" }
