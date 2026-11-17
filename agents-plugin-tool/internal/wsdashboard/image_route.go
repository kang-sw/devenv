package wsdashboard

import (
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// The image route: GET /s/<reply-id>/image?path=<path> serves an image file a
// board item references, only when the file resolves inside the session's
// project root (its registration's root path), for LAN/token viewers.
// path is a path relative to that root, or an absolute path inside it,
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
// Local viewers may also read outside images, including symlink escapes,
// ../ and ~/ paths. Both written and resolved sensitive home paths are
// denied (localPath), and both names must retain an image extension.
// Every image response is refused, with the same 404, to a request whose
// Sec-Fetch-Site is same-site or cross-site, and carries
// Cross-Origin-Resource-Policy: same-origin, so another site open in the
// same browser cannot probe the filesystem through an <img> load or error.
// Sec-Fetch-Site: none is accepted: it is a navigation the user started
// (a reload of a tab the link menu opened, the browser's own open image in
// new tab, a pasted URL), not another site's page, and localViewer still
// holds the route to this machine.
// A LAN viewer, token or not, gets the 404 every other refusal gets.
// Local viewers are also served the images the user attached to messages,
// by their absolute path in a session's inbox attachments directory
// (attachmentImage), which the deny set would refuse with the rest of the
// daemon's state directory; LAN viewers see those only through the
// attachment route.
//
// The route streams the file as it is, never re-encoded: a served page
// points at it by URL and no response inlines image bytes. The ETag is the
// file's ImageVersion, and a URL whose &v= matches it is cached as
// immutable. A missing file costs a stat (imageMissing) before any symlink
// resolution or deny-set build.

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
	if !ValidReplyID(replyID) || strings.HasPrefix(path, "~/") {
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
		if strings.HasPrefix(path, "~/") {
			_, ok = imageType(path)
			abs = path
		}
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

// crossSiteFetch reports a request another site's page started: its
// Sec-Fetch-Site is present and neither same-origin nor none. none is a
// navigation the user started (typed, pasted, reloaded, or the browser's own
// open image in new tab), so refusing it would 404 the image the menu opened.
func crossSiteFetch(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
		return false
	}
	return true
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

// ImageVersion is an image file's version: its modification time and size.
// A served page appends it to an image route URL as &v=, which the route
// otherwise ignores, so an edited image gets a new URL. mcp servedViewInput
// writes the same format; the two must agree for the immutable cache.
func ImageVersion(info os.FileInfo) string {
	return strconv.FormatInt(info.ModTime().UnixNano(), 36) + "." + strconv.FormatInt(info.Size(), 36)
}

// imageCacheImmutable is the route's cache policy for a URL whose &v= is the
// served file's current ImageVersion: that URL names this content, so the
// browser keeps it without asking again. Any other URL is revalidated
// (no-cache) against the ETag, which is the same version.
const imageCacheImmutable = "private, max-age=31536000, immutable"

// attachmentImage reports path as an image the user attached to a message
// from the page: written absolute, directly in a session's inbox attachments
// directory under the daemon's state directory dir, with an attachment name
// (wsinbox.AttachmentName). The state directory is otherwise always denied
// (outsideDenied), since it holds the daemon's credentials; an attachment's
// name grammar never names one of them. Any session's attachment qualifies,
// not only the route's own session's: a loopback viewer may open each
// session's attachment route anyway. It reads no file.
func attachmentImage(dir, path string) (string, bool) {
	if dir == "" || path == "" || strings.ContainsRune(path, 0) || networkPath(path) {
		return "", false
	}
	native := filepath.FromSlash(path)
	if !filepath.IsAbs(native) {
		return "", false
	}
	native = filepath.Clean(native)
	// <dir>/sessions/<reply-id>/inbox/attachments/<name>
	replyID := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(native))))
	if !ValidReplyID(replyID) {
		return "", false
	}
	if _, ok := wsinbox.AttachmentName(wsinbox.AttachmentsDir(sessionInbox(dir, replyID)), native); !ok {
		return "", false
	}
	return native, true
}

// openAttachment opens the attachment file at path: a regular file, not a
// symlink, the same file when opened as when it was looked up.
func openAttachment(path string) (*os.File, os.FileInfo, bool) {
	// Lstat before opening: a symlink is refused, and opening a FIFO would
	// block the handler.
	linfo, err := os.Lstat(path)
	if err != nil || !linfo.Mode().IsRegular() {
		return nil, nil, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, false
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(linfo, info) {
		_ = f.Close()
		return nil, nil, false
	}
	return f, info, true
}

// imageMissing reports a path whose file does not exist, by one stat of each
// file the route would look up for it, so a miss costs that stat instead of
// the symlink resolution and deny-set build behind it. For a LAN viewer it
// stats only what containedImage would resolve next: an image name inside
// root. For a local viewer it also stats the outside spelling before the
// deny set's string check, so a denied path's existence (never its content)
// is looked up; the answer is the same 404 either way, and only a
// same-origin loopback viewer gets this far. A path that is not a plain
// absolute one, or names a device, is left to the full checks.
func imageMissing(root, path string, local bool) bool {
	if path == "" || strings.ContainsRune(path, 0) || networkPath(path) {
		return false
	}
	if _, ok := imageType(path); !ok {
		return false
	}
	var candidates []string
	if rel, ok := relativeInside(root, path); ok {
		candidates = append(candidates, filepath.Join(filepath.Clean(root), rel))
	}
	if local {
		// The spelling a local viewer's outside read resolves (localPath).
		if c := resolveAgainst(root, filepath.FromSlash(homePath(path))); len(candidates) == 0 || c != candidates[0] {
			candidates = append(candidates, c)
		}
	}
	if len(candidates) == 0 {
		return false
	}
	for _, c := range candidates {
		if !filepath.IsAbs(c) || !driveOrRootPath(c) || networkPath(c) || deviceElement(c) {
			return false
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); !errors.Is(err, fs.ErrNotExist) {
			return false
		}
	}
	return true
}

// openImage opens the image file the image route serves for path in the
// session whose project root is root, with its content type; false for every
// refusal. A local viewer is served an attachment image, and an image outside
// root under the deny set (localPath); a LAN viewer only an image inside root.
func (d *Daemon) openImage(root, path string, local bool) (*os.File, string, os.FileInfo, bool) {
	if local {
		if file, ok := attachmentImage(d.dir, path); ok {
			ctype, _ := imageType(file)
			f, info, ok := openAttachment(file)
			return f, ctype, info, ok
		}
	}
	if imageMissing(root, path, local) {
		return nil, "", nil, false
	}
	file, ctype, ok := containedImage(root, path)
	deny := newDenySet()
	if local {
		_, real, allowed := deny.localPath(root, path)
		if !allowed {
			ok = false
		} else if !ok {
			ctype, ok = imageType(path)
			if ok {
				file, ok = resolvedImage(real)
			}
		}
	}
	if !ok {
		return nil, "", nil, false
	}
	open := openContained
	if local {
		open = deny.openLocal
	}
	f, _, info, ok := open(filepath.Dir(file), filepath.Base(file))
	return f, ctype, info, ok
}

// handleImage serves the image route of one session.
func (d *Daemon) handleImage(w http.ResponseWriter, r *http.Request, replyID string) {
	if crossSiteFetch(r) {
		http.Error(w, "image not found", http.StatusNotFound)
		return
	}
	if d.index == nil || !ValidReplyID(replyID) {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	s, ok := d.index.Session(replyID)
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	f, ctype, info, ok := d.openImage(s.Project, r.URL.Query().Get("path"), localViewer(r))
	if !ok {
		http.Error(w, "image not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	version := ImageVersion(info)
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", imageContentSecurityPolicy)
	h.Set("ETag", `"`+version+`"`)
	if r.URL.Query().Get("v") == version {
		h.Set("Cache-Control", imageCacheImmutable)
	} else {
		h.Set("Cache-Control", "no-cache")
	}
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	http.ServeContent(w, r, "", info.ModTime(), f)
}
