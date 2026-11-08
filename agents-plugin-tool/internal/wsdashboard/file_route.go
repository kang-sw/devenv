package wsdashboard

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/kang-sw/devenv/internal/wsi18n"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// The file view: GET /s/<reply-id>/file?path=<path> shows a file inside the
// session's project root, the target a served page's file links open. path
// is relative to that root, or absolute inside it, as for the image route,
// whose containment rule it shares: the path must stay inside the root
// lexically, and the file it resolves to after every symlink must stay
// inside the resolved root. On top of that the file itself must not be a
// symlink (Lstat, as the attachment route), and the file opened must be the
// one Lstat saw. Every file inside the root is served, dotfiles and
// git-ignored ones included, to a loopback viewer and a LAN token holder
// alike: the token is the user's own and already lets its holder message
// the session, whose lead reads any file anyway. Every refusal answers 404.
//
// What it shows depends on the file:
//
//   - an image (by extension) is served as the image route serves one, so
//     an SVG runs no script in the daemon's origin;
//   - a markdown file (.md, .markdown) renders through the board's markdown
//     renderer in its escaped mode (raw HTML is text, every destination goes
//     through its allow-list; mermaid stays a code block); its relative links
//     resolve against the file's own directory;
//   - any other UTF-8 text without NUL bytes is served as text/plain under
//     the image route's CSP, so an .html file never runs either;
//   - a binary file, or a text file over fileViewMax, gets a "not viewable"
//     page that offers the path copies only.

// fileViewMax bounds a text or markdown file the file view reads and shows.
const fileViewMax = 2 << 20

// FileSrc is the file view URL of the file at rel, relative to the session
// root, in the session replyID.
func FileSrc(replyID, rel string) string {
	return "/s/" + replyID + "/file?path=" + url.QueryEscape(filepath.ToSlash(rel))
}

// FileActions is a served page's link-action rule (wswhiteboard.View
// FileActions) for the session replyID with project root root: a path
// inside root gets its file view URL, its root-relative path, and its
// absolute path; any other path gets its absolute path alone, which the link
// menu offers to copy. It reads no file, so a path the file view refuses
// once it reads the file (missing, a symlink) still links and answers 404.
func FileActions(root, replyID string) func(path string) wswhiteboard.FileAction {
	return func(path string) wswhiteboard.FileAction {
		if root == "" || path == "" || strings.ContainsRune(path, 0) {
			return wswhiteboard.FileAction{}
		}
		if rel, ok := relativeInside(root, path); ok && ValidReplyID(replyID) {
			return wswhiteboard.FileAction{Href: FileSrc(replyID, rel), Rel: filepath.ToSlash(rel), Abs: filepath.Join(filepath.Clean(root), rel)}
		}
		native := filepath.FromSlash(path)
		if !filepath.IsAbs(native) {
			native = filepath.Join(root, native)
		}
		return wswhiteboard.FileAction{Abs: filepath.Clean(native)}
	}
}

// servedTargets is the served page's markdown link rule for the session
// replyID with project root root: a file target shows its root-relative
// path (its own path outside root) and links through FileActions, and an
// image inside root loads through the image route.
func servedTargets(root, replyID string) wswhiteboard.TextTargets {
	return wswhiteboard.TextTargets{
		FileText: func(path string) string {
			if rel, ok := relativeInside(root, path); ok {
				return filepath.ToSlash(rel)
			}
			return filepath.ToSlash(path)
		},
		ImageSrc:    func(path string) string { return ImageSrc(root, replyID, path) },
		FileActions: FileActions(root, replyID),
	}
}

// fileViewTargets is the markdown file view's link rule: servedTargets with
// a relative path resolved against the file's own directory base (relative
// to root), and a target outside root left as path text, since the page has
// no link menu to copy it from.
func fileViewTargets(root, replyID, base string) wswhiteboard.TextTargets {
	served := servedTargets(root, replyID)
	rebase := func(path string) string {
		if native := filepath.FromSlash(path); !filepath.IsAbs(native) {
			return filepath.Join(base, native)
		}
		return path
	}
	return wswhiteboard.TextTargets{
		FileText: func(path string) string { return served.FileText(rebase(path)) },
		ImageSrc: func(path string) string { return served.ImageSrc(rebase(path)) },
		FileActions: func(path string) wswhiteboard.FileAction {
			if a := served.FileActions(rebase(path)); a.Href != "" {
				return a
			}
			return wswhiteboard.FileAction{}
		},
	}
}

// openContained opens the regular file path names inside root for the file
// view, with its path relative to root and its info; false when the file
// view refuses it.
func openContained(root, path string) (*os.File, string, os.FileInfo, bool) {
	rel, ok := relativeInside(root, path)
	if !ok {
		return nil, "", nil, false
	}
	full := filepath.Join(root, rel)
	// Lstat before opening: a symlink is refused, and opening a FIFO would
	// block the handler.
	linfo, err := os.Lstat(full)
	if err != nil || !linfo.Mode().IsRegular() {
		return nil, "", nil, false
	}
	// A symlinked directory on the way may still lead outside the root.
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, "", nil, false
	}
	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		return nil, "", nil, false
	}
	if realRel, err := filepath.Rel(realRoot, real); err != nil || !insideRel(realRel) {
		return nil, "", nil, false
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, "", nil, false
	}
	// The file opened must be the one Lstat saw, not one swapped in since.
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(linfo, info) {
		_ = f.Close()
		return nil, "", nil, false
	}
	return f, rel, info, true
}

// fileViewPage is what the file view's page templates render.
type fileViewPage struct {
	Rel, Abs string
	// HTML is a markdown file's rendered body.
	HTML template.HTML
	// Reason says why a file is not viewable, in the page's language.
	Reason string
	// Lang is the page's language (wsi18n); Theme the pinned theme (empty
	// follows the OS).
	Lang, Theme string
	// Nonce admits the not-viewable page's one inline script.
	Nonce         string
	WhiteboardCSS template.CSS
}

// handleFile serves the file view of one session.
func (d *Daemon) handleFile(w http.ResponseWriter, r *http.Request, replyID string) {
	if d.index == nil || !ValidReplyID(replyID) {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	s, ok := d.index.Session(replyID)
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	f, rel, info, ok := openContained(s.Project, r.URL.Query().Get("path"))
	if !ok {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-cache")
	h.Set("Referrer-Policy", "no-referrer")
	if ctype, ok := imageType(rel); ok {
		h.Set("Content-Type", ctype)
		h.Set("Content-Security-Policy", imageContentSecurityPolicy)
		http.ServeContent(w, r, "", info.ModTime(), f)
		return
	}
	lang, _ := requestLang(r)
	page := fileViewPage{Rel: filepath.ToSlash(rel), Abs: filepath.Join(filepath.Clean(s.Project), rel), Lang: lang, Theme: requestTheme(r)}
	if info.Size() > fileViewMax {
		page.Reason = wsi18n.T(lang, "file.reason.large", fileViewMax>>20)
		d.writeFilePage(w, "file-none", page)
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, fileViewMax+1))
	switch {
	case err != nil:
		http.Error(w, "file not found", http.StatusNotFound)
		return
	case len(data) > fileViewMax:
		page.Reason = wsi18n.T(lang, "file.reason.large", fileViewMax>>20)
		d.writeFilePage(w, "file-none", page)
		return
	case !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0:
		page.Reason = wsi18n.T(lang, "file.reason.binary")
		d.writeFilePage(w, "file-none", page)
		return
	}
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".md", ".markdown":
		page.HTML = wswhiteboard.RenderText(string(data), fileViewTargets(s.Project, replyID, filepath.Dir(rel)))
		d.writeFilePage(w, "file-md", page)
	default:
		h.Set("Content-Type", "text/plain; charset=utf-8")
		h.Set("Content-Security-Policy", imageContentSecurityPolicy)
		http.ServeContent(w, r, "", info.ModTime(), bytes.NewReader(data))
	}
}

// fileMarkdownCSP lets a rendered markdown file load its images from the
// image route and nothing else: no script, no frame, no form target, and no
// page may frame it.
const fileMarkdownCSP = "default-src 'none'; style-src 'unsafe-inline'; img-src 'self'; base-uri 'none'; form-action 'none'; " + frameAncestorsNone

// writeFilePage renders one of the file view's pages: file-md (a rendered
// markdown file) or file-none (a file the view does not show).
func (d *Daemon) writeFilePage(w http.ResponseWriter, name string, page fileViewPage) {
	a, err := loadDashboardAssets()
	if err != nil {
		fmt.Fprintf(d.log, "dashboard: render %s: %v\n", name, err)
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	csp := fileMarkdownCSP
	if name == "file-none" {
		// The page's one inline script copies the paths; nothing else runs.
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err != nil {
			http.Error(w, "render failed", http.StatusInternalServerError)
			return
		}
		page.Nonce = hex.EncodeToString(raw[:])
		csp = "default-src 'none'; style-src 'unsafe-inline'; script-src 'nonce-" + page.Nonce + "'; base-uri 'none'; form-action 'none'; " + frameAncestorsNone
	}
	page.WhiteboardCSS = a.data.WhiteboardCSS
	var buf bytes.Buffer
	if err := a.templateFor(page.Lang).ExecuteTemplate(&buf, name, page); err != nil {
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Security-Policy", csp)
	writeHTML(w, buf.String())
}
