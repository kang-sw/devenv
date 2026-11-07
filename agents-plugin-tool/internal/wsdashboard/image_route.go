package wsdashboard

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// The image route: GET /s/<reply-id>/image?path=<path> serves an image file a
// board item references, only when the file resolves inside the session's
// project root (its registration's root path), in LAN mode and localhost mode
// alike. path is a path relative to that root, or an absolute path inside it,
// with forward or native slashes.
//
// Containment is checked twice. First lexically, before the filesystem is
// touched, so a path that leaves the root by ".." or names another absolute
// path (a UNC path on Windows would otherwise reach the network) is refused
// unread. Then on the resolved file: both the root and the file are resolved
// through every symlink, and the resolved file must still be inside the
// resolved root, so a symlink inside the root that points outside it is
// refused too. Both the name asked for and the resolved file must carry an
// image extension, so the route never serves a project's other files under an
// image name. Every refusal answers 404, so the route tells nothing about
// which paths outside the root exist.

// imageTypes maps the image extensions the route serves to their content type.
var imageTypes = map[string]string{
	".avif": "image/avif",
	".bmp":  "image/bmp",
	".gif":  "image/gif",
	".ico":  "image/x-icon",
	".jpeg": "image/jpeg",
	".jpg":  "image/jpeg",
	".png":  "image/png",
	".svg":  "image/svg+xml",
	".webp": "image/webp",
}

// imageContentSecurityPolicy keeps an image opened on its own, an SVG above
// all, from running script in the daemon's origin, where it could post to
// the message route.
const imageContentSecurityPolicy = "default-src 'none'; style-src 'unsafe-inline'; sandbox"

func imageType(name string) (string, bool) {
	ctype, ok := imageTypes[strings.ToLower(filepath.Ext(name))]
	return ctype, ok
}

// ImageSrc is the image route URL a served page gives a board image at path
// (relative to root, or absolute) in the session replyID names. It is empty
// when the route would refuse the path on its name alone: it leaves root, or
// it has no image extension; the page then shows the path as text.
func ImageSrc(root, replyID, path string) string {
	if !ValidReplyID(replyID) {
		return ""
	}
	rel, ok := relativeInside(root, path)
	if !ok {
		return ""
	}
	if _, ok := imageType(rel); !ok {
		return ""
	}
	return "/s/" + replyID + "/image?path=" + url.QueryEscape(filepath.ToSlash(rel))
}

// imageDataMax bounds the image ImageDataURL inlines; a larger one falls
// back to the image route.
const imageDataMax = 2 << 20

// ImageDataURL is a board image at path (relative to root, or absolute)
// inlined as a data: URL, under the image route's own containment checks
// (containedImage), for the Now slot's sandboxed frame: the frame's opaque
// origin sends no cookie, so in LAN mode the image route would refuse it. It
// is empty when the route would refuse the path or the file is larger than
// imageDataMax; the page then falls back to the route's URL (ImageSrc). The
// file is read on every render, which is what keeps the inlined image
// current.
func ImageDataURL(root, path string) string {
	file, ctype, ok := containedImage(root, path)
	if !ok {
		return ""
	}
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, imageDataMax+1))
	if err != nil || len(data) > imageDataMax {
		return ""
	}
	return "data:" + ctype + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// relativeInside lexically resolves path against root and returns it
// relative to root, or false when it leaves root. It reads no file.
func relativeInside(root, path string) (string, bool) {
	if root == "" || path == "" || strings.ContainsRune(path, 0) {
		return "", false
	}
	root = filepath.Clean(root)
	native := filepath.FromSlash(path)
	if !filepath.IsAbs(native) {
		native = filepath.Join(root, native)
	}
	rel, err := filepath.Rel(root, filepath.Clean(native))
	if err != nil || !insideRel(rel) {
		return "", false
	}
	return rel, true
}

// insideRel reports whether a filepath.Rel result names an entry strictly
// below its base.
func insideRel(rel string) bool {
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// containedImage resolves path to the regular image file the route may serve
// for root, with its content type.
func containedImage(root, path string) (file, ctype string, ok bool) {
	rel, ok := relativeInside(root, path)
	if !ok {
		return "", "", false
	}
	ctype, ok = imageType(rel)
	if !ok {
		return "", "", false
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", false
	}
	real, err := filepath.EvalSymlinks(filepath.Join(root, rel))
	if err != nil {
		return "", "", false
	}
	if realRel, err := filepath.Rel(realRoot, real); err != nil || !insideRel(realRel) {
		return "", "", false
	}
	if _, ok := imageType(real); !ok {
		return "", "", false
	}
	// Stat before opening: opening a FIFO would block the handler.
	if info, err := os.Stat(real); err != nil || !info.Mode().IsRegular() {
		return "", "", false
	}
	return real, ctype, true
}

// handleImage serves the image route of one session.
func (d *Daemon) handleImage(w http.ResponseWriter, r *http.Request, replyID string) {
	if d.index == nil || !ValidReplyID(replyID) {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	s, ok := d.index.Session(replyID)
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	file, ctype, ok := containedImage(s.Project, r.URL.Query().Get("path"))
	if !ok {
		http.Error(w, "image not found", http.StatusNotFound)
		return
	}
	f, err := os.Open(file)
	if err != nil {
		http.Error(w, "image not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "image not found", http.StatusNotFound)
		return
	}
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", imageContentSecurityPolicy)
	h.Set("Cache-Control", "no-cache")
	h.Set("Referrer-Policy", "no-referrer")
	http.ServeContent(w, r, "", info.ModTime(), f)
}
