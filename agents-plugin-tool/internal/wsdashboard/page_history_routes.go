// Page history routes and frame helpers serve archived content.

package wsdashboard

import (
	"encoding/json"
	"fmt"
	"html/template"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// historyPageMax bounds one history page's n.
const historyPageMax = 4 * wslive.RingCap

// historyAnswer is the history route's answer: the rendered entries, oldest
// first, the cursor for the next older page (empty when the page held no
// entry), and whether no older entry remains, so the page stops asking.
type historyAnswer struct {
	HTML   string `json:"html"`
	Before string `json:"before"`
	End    bool   `json:"end"`
}

// handleHistory serves /s/<reply-id>/history?before=<entry name>&n=<count>:
// the n entries (default wslive.RingCap) older than before, exclusive, or
// the newest n when before is omitted. before is only ever a sort cursor,
// never a path, and must have an entry name's grammar.
func (d *Daemon) handleHistory(w http.ResponseWriter, r *http.Request, replyID string) {
	query := r.URL.Query()
	before := query.Get("before")
	if before != "" && !wslive.ValidRingName(before) {
		http.Error(w, "before is not a conversation entry name", http.StatusBadRequest)
		return
	}
	n := wslive.RingCap
	if raw := query.Get("n"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			http.Error(w, "n must be a positive count", http.StatusBadRequest)
			return
		}
		n = min(v, historyPageMax)
	}
	lang, _ := requestLang(r)
	cv, page, ok, err := d.pagesIn(lang).history(replyID, before, n)
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	if err == nil {
		var a *dashboardAssets
		if a, err = loadDashboardAssets(); err == nil {
			cv.Lang = lang
			err = renderEntries(a, d.entries, replyID, &cv)
		}
	}
	if err != nil {
		fmt.Fprintf(d.log, "dashboard: history %s: %v\n", replyID, err)
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	raw, _ := json.Marshal(historyAnswer{HTML: renderedEntries(cv.Entries), Before: page.Before, End: page.End})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(raw)
}

// closedPageMax bounds one closed-topic page's n: a page that re-fetches
// every closed topic it had shown after a board swap asks for them at once.
const closedPageMax = 10 * wswhiteboard.ClosedCap

// handleClosedTopics serves /s/<reply-id>/closed?from=<index>&n=<count>: n
// (default wswhiteboard.ClosedCap) of the closed topics the board leaves
// past its cap, from index from in close order, the most recently closed
// first, rendered as the board renders them (ClosedTopicsSource), with the
// next show-more control when more remain. It is read-only and serves what
// the session page shows, so it takes the page's own access rule.
func (d *Daemon) handleClosedTopics(w http.ResponseWriter, r *http.Request, replyID string) {
	query := r.URL.Query()
	from, err := strconv.Atoi(query.Get("from"))
	if err != nil || from < 1 {
		http.Error(w, "from must be a positive index", http.StatusBadRequest)
		return
	}
	n := wswhiteboard.ClosedCap
	if raw := query.Get("n"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			http.Error(w, "n must be a positive count", http.StatusBadRequest)
			return
		}
		n = min(v, closedPageMax)
	}
	// The daemon's source is agentSessions, which forwards to the configured
	// one; a source without closed topics answers not found.
	src, hasClosed := d.source.(ClosedTopicsSource)
	var s Session
	ok := hasClosed && d.index != nil && ValidReplyID(replyID)
	if ok {
		s, ok = d.index.Session(replyID)
	}
	var html template.HTML
	if ok {
		lang, _ := requestLang(r)
		html, ok = src.ReadClosedTopics(s.Registration, lang, from, n)
	}
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	raw, _ := json.Marshal(map[string]string{"html": string(wswhiteboard.ThemeFrames(html, requestTheme(r)))})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(raw)
}

// handleBoardArchive serves /s/<reply-id>/board.zip: the session's board
// folder as a zip download, named by boardArchiveName. A session without a
// board answers not found.
func (d *Daemon) handleBoardArchive(w http.ResponseWriter, r *http.Request, replyID string) {
	src, has := d.source.(BoardArchiveSource)
	var s Session
	ok := has && d.index != nil && ValidReplyID(replyID)
	if ok {
		s, ok = d.index.Session(replyID)
	}
	var data []byte
	if ok {
		var err error
		data, ok, err = src.BoardArchive(s.Registration)
		if err != nil {
			http.Error(w, "board unavailable: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if !ok {
		http.Error(w, "no board", http.StatusNotFound)
		return
	}
	var branch string
	if b := d.git.badge(s.Project); b != nil && b.State == gitOK && !b.Detached {
		branch = b.Branch
	}
	name := boardArchiveName(s, branch, time.Now())
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

// boardArchiveName is whiteboard_<repository>_<branch>_<label>_<YYMMDD_hhmm>.zip:
// the main repository's directory name (a linked worktree's branch already
// tells it apart), the branch, dropped with its separator when there is
// none, the display label or else the reply-id, and now in the daemon's
// local time. Filename-unsafe characters become "-".
func boardArchiveName(s Session, branch string, now time.Time) string {
	repo := railRepoRoot(s.Registration)
	if repo == "" {
		repo = s.Project
	}
	label := s.Label
	if label == "" {
		label = s.ReplyID
	}
	parts := []string{"whiteboard", filepath.Base(filepath.Clean(repo))}
	if branch != "" {
		parts = append(parts, branch)
	}
	parts = append(parts, label)
	for i, part := range parts {
		parts[i] = strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f || strings.ContainsRune(`/\:*?"<>|`, r) {
				return '-'
			}
			return r
		}, part)
	}
	return strings.Join(parts, "_") + "_" + now.Format("060102_1504") + ".zip"
}

// serveMermaid serves the shared mermaid script. Its name carries its hash,
// so it never changes under a name and the browser may cache it for good.
func serveMermaid(w http.ResponseWriter) {
	_, data := wswhiteboard.MermaidAsset()
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(data)
}

// serveFrameAsset serves a shared topic-body frame asset
// (wswhiteboard.FrameAsset). Its name carries its hash, so, like the mermaid
// script, the browser may cache it for good.
func serveFrameAsset(w http.ResponseWriter, data []byte, ctype string) {
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

func readMethod(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// frameAncestorsNone refuses every framing of a daemon page. A framed page's
// own script posts carry the daemon's Origin, so the same-origin and loopback
// guards on the write routes would pass a clickjacked board answer, message,
// steer, cancel, interrupt, or system open; only refusing the frame stops it.
// The one exception is a page the iframe shell frames (?frame=1,
// admitShellFrame), which only a page of the daemon's own origin may frame.
const frameAncestorsNone = "frame-ancestors 'none'"

// sessionPageCSP is the session page's policy: frame-src 'self', so no
// frame on the page (a preview's sandboxed document, a topic body, a
// shell's session frame) can navigate itself to another origin, where a
// preview's document would carry its capability (fs_capability.go), and
// the page's own framing: the shell refuses every framing
// (frameAncestorsNone), and the page a shell frames (frame) admits its
// own origin only. A topic body's srcdoc frame loads under it.
func sessionPageCSP(frame bool) string {
	if frame {
		return "frame-src 'self'; " + frameAncestorsSelf
	}
	return "frame-src 'self'; " + frameAncestorsNone
}

// frameAncestorsSelf admits framing by a page of the daemon's own origin
// only: the iframe shell framing a page (?frame=1, admitShellFrame).
const frameAncestorsSelf = "frame-ancestors 'self'"

// admitShellFrame sets the framing headers of a page the iframe shell
// frames: csp, which admits the daemon's own origin as the page's framer
// (frameAncestorsSelf), and X-Frame-Options SAMEORIGIN in place of
// writeHTML's DENY.
func admitShellFrame(w http.ResponseWriter, csp string) {
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
}

// writeHTML writes a daemon HTML page. A caller that sets its own
// Content-Security-Policy includes frameAncestorsNone in it (or, for a page
// the shell frames, frameAncestorsSelf: admitShellFrame); without one the
// page gets frameAncestorsNone alone, and X-Frame-Options DENY unless the
// caller set SAMEORIGIN. The one daemon page that does not go through it is
// the web card's wrapper frame (writeWebFrame, webcard_routes.go), which the
// session page frames: it sends frame-ancestors 'self' and X-Frame-Options
// SAMEORIGIN instead.
func writeHTML(w http.ResponseWriter, html string) { writeHTMLStatus(w, http.StatusOK, html) }

// writeHTMLStatus is writeHTML with the status the page answers.
func writeHTMLStatus(w http.ResponseWriter, status int, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if w.Header().Get("Content-Security-Policy") == "" {
		w.Header().Set("Content-Security-Policy", frameAncestorsNone)
	}
	if w.Header().Get("X-Frame-Options") == "" {
		w.Header().Set("X-Frame-Options", "DENY")
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(html))
}
