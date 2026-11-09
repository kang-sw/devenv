package wsdashboard

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
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
//
// One widening, for a browser on this machine only (localViewer): an image
// outside the root, which a served lead reply or board item names by an
// absolute path (OutsideImage), is served too, under outsideImage's rules.
// Such a response is refused, with the same 404, to a request whose
// Sec-Fetch-Site is present and not same-origin, and carries
// Cross-Origin-Resource-Policy: same-origin, so another site open in the
// same browser cannot probe the filesystem through an <img> load or error.
// A LAN viewer, token or not, gets the 404 every other refusal gets.

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

// OutsideImage is a served page's rule (wswhiteboard.View OutsideImage) for
// an image outside root in the session replyID: a path written absolute,
// with an image extension, lexically outside root, and not a UNC or device
// path gets the image route URL of its absolute path, which the route serves
// to a loopback viewer only. It reads no file. Any other path gets the zero
// FileAction and stays with ImageSrc and FileActions; a relative path that
// leaves root among them, since the decision is on the path as written.
func OutsideImage(root, replyID string) func(path string) wswhiteboard.FileAction {
	return func(path string) wswhiteboard.FileAction {
		abs, ok := outsideImagePath(root, path)
		if !ok || !ValidReplyID(replyID) {
			return wswhiteboard.FileAction{}
		}
		return wswhiteboard.FileAction{
			Href:    "/s/" + replyID + "/image?path=" + url.QueryEscape(filepath.ToSlash(abs)),
			Abs:     abs,
			Image:   true,
			Outside: true,
		}
	}
}

// networkPath reports whether path starts with two separators in either
// slash direction, a Windows UNC path (\\server\share, which would reach
// the network) or device path (\\?\..., \\.\...), or with an NT-namespace
// prefix (\??\, which Windows also reads as absolute and passes through to
// any device, a network share included). It is a pure string check, so it
// runs before the filesystem is touched and on every OS.
func networkPath(path string) bool {
	sep := func(c byte) bool { return c == '/' || c == '\\' }
	if len(path) >= 2 && sep(path[0]) && sep(path[1]) {
		return true
	}
	return len(path) >= 4 && sep(path[0]) && path[1] == '?' && path[2] == '?' && sep(path[3])
}

// driveOrRootPath reports whether the native absolute path is on a plain
// volume: on Windows a drive letter ("C:"), never a UNC, device, or other
// namespace volume, whatever its spelling; on Unix every absolute path,
// which has no volume.
func driveOrRootPath(native string) bool {
	vol := filepath.VolumeName(native)
	return vol == "" || (len(vol) == 2 && vol[1] == ':')
}

// absoluteImageName lexically checks a path written absolute with an image
// extension, refusing a UNC or device path, and returns it native and
// cleaned with its content type. It reads no file.
func absoluteImageName(path string) (string, string, bool) {
	if path == "" || strings.ContainsRune(path, 0) || networkPath(path) {
		return "", "", false
	}
	native := filepath.FromSlash(path)
	if !filepath.IsAbs(native) || !driveOrRootPath(native) {
		return "", "", false
	}
	ctype, ok := imageType(native)
	if !ok {
		return "", "", false
	}
	return filepath.Clean(native), ctype, true
}

// outsideImagePath is absoluteImageName for a path lexically outside root.
func outsideImagePath(root, path string) (string, bool) {
	abs, _, ok := absoluteImageName(path)
	if !ok {
		return "", false
	}
	if _, inside := relativeInside(root, abs); inside {
		return "", false
	}
	return abs, true
}

// resolvedImage resolves the absolute image name abs through every symlink
// to the regular file it names, which must itself carry an image extension
// and not be on a UNC or device path.
func resolvedImage(abs string) (string, bool) {
	real, err := filepath.EvalSymlinks(abs)
	if err != nil || networkPath(real) || !driveOrRootPath(real) {
		return "", false
	}
	if _, ok := imageType(real); !ok {
		return "", false
	}
	// Stat before opening: opening a FIFO would block the handler.
	if info, err := os.Stat(real); err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return real, true
}

// outsideImage resolves path to the regular image file outside root the
// route may serve a loopback viewer, with its content type.
func outsideImage(root, path string) (file, ctype string, ok bool) {
	abs, ok := outsideImagePath(root, path)
	if !ok {
		return "", "", false
	}
	ctype, _ = imageType(abs)
	real, ok := resolvedImage(abs)
	if !ok {
		return "", "", false
	}
	return real, ctype, true
}

// crossSiteFetch reports a request whose Sec-Fetch-Site is present and not
// same-origin: one another site's page started, or one typed or bookmarked.
func crossSiteFetch(r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	return site != "" && site != "same-origin"
}

// imageDataMax bounds the image ImageDataURL inlines; a larger one falls
// back to the image route.
const imageDataMax = 2 << 20

// ImageDataURL is a board image at path (relative to root, or absolute)
// inlined as a data: URL, under the image route's own containment checks
// (containedImage), for a topic body's sandboxed frame: the frame's opaque
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
	rel, err := filepath.Rel(root, resolveAgainst(root, filepath.FromSlash(path)))
	if err != nil || !insideRel(rel) {
		return "", false
	}
	return rel, true
}

// resolveAgainst resolves the native path against root, cleaned: an
// absolute path stays itself and any other joins root, except a rooted one
// (\etc\hosts on Windows, absolute on the current drive rather than
// relative), which takes root's volume. On Unix a rooted path is absolute.
func resolveAgainst(root, native string) string {
	switch {
	case filepath.IsAbs(native):
		return filepath.Clean(native)
	case isRooted(native):
		return filepath.Clean(filepath.VolumeName(root) + native)
	}
	return filepath.Join(root, native)
}

// isRooted reports whether the native path starts at a root directory.
func isRooted(native string) bool { return native != "" && os.IsPathSeparator(native[0]) }

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
	path := r.URL.Query().Get("path")
	file, ctype, ok := containedImage(s.Project, path)
	outside := false
	if !ok && localViewer(r) && !crossSiteFetch(r) {
		file, ctype, ok = outsideImage(s.Project, path)
		outside = ok
	}
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
	if outside {
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
	}
	http.ServeContent(w, r, "", info.ModTime(), f)
}
