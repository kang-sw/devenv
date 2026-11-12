package wsdashboard

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The HTML preview's capability route:
//
//	GET /s/<reply-id>/fs/cap/<capability>/<path>
//
// serves the file at path, relative to the session's project root, for a
// preview card's sandboxed frame. An HTML or SVG file from a workspace may
// run script, so the card loads it in an iframe with sandbox="allow-scripts"
// and no allow-same-origin: its origin is opaque, it cannot read the
// dashboard page, and it sends no cookie. The route therefore cannot be
// admitted by the LAN token; it is admitted by the capability its path
// carries (previewCapability in access.go), which the preview route mints
// when the page opens an HTML or SVG preview. Because the capability is a
// path segment, the document's relative assets (its style sheets, scripts,
// images, and media) resolve under the same prefix and load with it.
//
// A capability is the expiry time and a MAC over the session's reply-id,
// its project root, and that expiry, under a key the daemon draws at start
// and never stores:
//
//   - it reads only the root it was minted for: under another session, or
//     once the session's root has changed, the MAC does not verify;
//   - it stops working fsCapabilityTTL after it was minted, and at a daemon
//     restart; the card asks for a fresh one when it reloads;
//   - it is minted only for the session's own root, never for a root the
//     explorer's "change root" picked (the preview route shows such a
//     file's source instead);
//   - it never reads a file whose path from the root has a dot-named
//     segment (.env, .git/, .ssh/), where secrets usually live, and, for a
//     viewer other than a browser on this machine (localViewer), a file
//     git reports ignored either (framedDenied); the preview route mints
//     none for such a file and shows its source instead. A browser on this machine
//     already reads every file of the root through the explorer, and a
//     paged document or a build output usually sits in an ignored folder,
//     so the ignore rule guards only a LAN viewer. The sandboxed frame's
//     own requests come from the same browser to the same Host (the
//     capability URL is a path, resolved against the session page's
//     origin), so they keep the local answer the page got.
//
// Every response carries a Content-Security-Policy that:
//
//   - sandboxes the document itself ("sandbox allow-scripts"), so the file
//     opened outside the card's frame, in a tab of its own, still runs in
//     an opaque origin and never in the daemon's;
//   - limits every load directive to the capability's own path prefix (and
//     data: or blob: where a document makes its own), so the document loads
//     nothing from another origin or any other dashboard route, and allows
//     no fetch at all (connect-src 'none');
//   - allows no form target and no base URL change.
//
// A capability is a bearer secret, and the document it serves can read it:
// a script sees it in location.href and can carry it off, by navigating its
// tab to another origin or by a channel no policy closes (WebRTC, a DNS
// prefetch). So the route keeps a file's bytes from a script whoever holds
// the capability, not only from the document:
//
//   - a request with Sec-Fetch-Dest: empty (fetch, XMLHttpRequest, a
//     beacon) is refused, whoever makes it, as the policy refuses the
//     document's own (capRefused);
//   - a request whose Origin is another origin's is refused (capRefused);
//   - Access-Control-Allow-Origin is "null", and only on a request from an
//     opaque origin (Origin: null) for a module script, a style sheet, or a
//     font (Sec-Fetch-Dest script, style, or font): the CORS loads an opaque
//     document needs for its own files (capCORS). A page gets those only as
//     code it runs, rules it applies, and a face it draws, and nosniff with
//     the route's own types keeps any other file from loading as one;
//   - the session page's policy (frame-src 'self', sessionPageCSP) stops a
//     preview's frame from navigating itself to another origin; a document
//     in a tab of its own still can.
//
// What another origin holding a capability still gets is what any page gets
// from a cross-origin load: a script run for its side effects, a style
// sheet's rules, an image or a frame it cannot read, never a file's bytes.
// Sec-Fetch-Site cannot tell the document from that page: Chromium marks
// every request an opaque-origin document makes cross-site, as it marks
// another site's, and a sandboxed frame on another site sends Origin "null"
// too. Every refusal answers 404.
//
// The file view's page route:
//
//	GET /s/<reply-id>/fs/view/<capability>/<path>
//
// is the same route for an HTML file the file view opens as a page in a tab
// of its own (handleFile redirects to it), with the same containment,
// policy, and checks and a capability of the same shape, which verifies on
// its own route only (the MAC covers the route), and four differences:
//
//   - an image is served under the image route's policy, so an SVG runs no
//     script even opened on its own;
//   - an HTML file must be UTF-8 text without NUL bytes and at most
//     fileViewMax, as the file view requires, and any other file at most
//     fsViewAssetMax;
//   - it refuses a dot-named path but serves a file git ignores
//     (dotDenied): with no fetch, nosniff, and the route's own types, a
//     page reads a file's bytes only as a script, a style sheet, or media
//     (an image, a font, audio, video), and a paged document's folder is
//     often ignored whole;
//   - a document navigation (Sec-Fetch-Dest: document) with a capability
//     that no longer verifies, a reload after the expiry or a daemon
//     restart, is redirected to the file view of the same path, which mints
//     a fresh one behind the access guard.
//
// The policy carries no frame-ancestors: the board never frames the file
// view (it opens a tab), the sandboxed document's own requests carry an
// opaque Origin every write route refuses, and Chromium blocks every
// frame-ancestors source, even *, for a frame whose parent is opaque, so
// any value would break a document's frames of its own files.

const (
	// fsCapabilityTTL is how long a minted capability is served.
	fsCapabilityTTL = 15 * time.Minute
	fsCapRoutePart  = "/fs/cap/"
	fsViewRoutePart = "/fs/view/"
	// fsViewAssetMax bounds a file other than HTML the page route serves.
	fsViewAssetMax = 64 << 20
)

// capTypes are the content types the capability route serves by extension;
// any other file is application/octet-stream. The table is the route's own,
// so the answer does not depend on the host's MIME registry.
var capTypes = map[string]string{
	".html":  "text/html; charset=utf-8",
	".htm":   "text/html; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".json":  "application/json",
	".map":   "application/json",
	".txt":   "text/plain; charset=utf-8",
	".md":    "text/plain; charset=utf-8",
	".csv":   "text/csv; charset=utf-8",
	".xml":   "application/xml",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
	".avif":  "image/avif",
	".bmp":   "image/bmp",
	".ico":   "image/x-icon",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".mp4":   "video/mp4",
	".webm":  "video/webm",
	".ogg":   "video/ogg",
	".ogv":   "video/ogg",
	".mp3":   "audio/mpeg",
	".wav":   "audio/wav",
	".wasm":  "application/wasm",
}

// fsCapKey returns the daemon's capability key, drawn on first use.
func (d *Daemon) fsCapKey() []byte {
	d.fsCapOnce.Do(func() {
		d.fsCapSecret = make([]byte, 32)
		if _, err := rand.Read(d.fsCapSecret); err != nil {
			// No key: every capability then fails to verify.
			d.fsCapSecret = nil
		}
	})
	return d.fsCapSecret
}

// fsCapNow is the capability clock; a test replaces fsNow.
func (d *Daemon) fsCapNow() time.Time {
	if d.fsNow != nil {
		return d.fsNow()
	}
	return time.Now()
}

// fsCapMAC is the MAC of a capability for the route part (fsCapRoutePart
// or fsViewRoutePart) and the session replyID with project root root that
// expires at the Unix time expiry.
func fsCapMAC(key []byte, part, replyID, root string, expiry int64) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(part))
	m.Write([]byte{0})
	m.Write([]byte(replyID))
	m.Write([]byte{0})
	m.Write([]byte(filepath.Clean(root)))
	m.Write([]byte{0})
	m.Write([]byte(strconv.FormatInt(expiry, 10)))
	return hex.EncodeToString(m.Sum(nil)[:16])
}

// mintFSCapability mints a capability route capability for the session
// replyID and its project root root; empty when the daemon has no key.
func (d *Daemon) mintFSCapability(replyID, root string) string {
	return d.mintCapability(fsCapRoutePart, replyID, root)
}

// mintCapability mints a capability for the route part, the session
// replyID, and its project root root; empty when the daemon has no key. A
// capability verifies on its own route only, so a preview's capability
// cannot read, for a LAN viewer, a file git ignores, which the page route
// serves.
func (d *Daemon) mintCapability(part, replyID, root string) string {
	key := d.fsCapKey()
	if key == nil {
		return ""
	}
	expiry := d.fsCapNow().Add(fsCapabilityTTL).Unix()
	return strconv.FormatInt(expiry, 36) + "-" + fsCapMAC(key, part, replyID, root, expiry)
}

// validCapability reports whether capability is one the daemon minted for
// the route part, the session replyID, and root and has not expired.
func (d *Daemon) validCapability(part, capability, replyID, root string) bool {
	key := d.fsCapKey()
	exp, mac, ok := strings.Cut(capability, "-")
	if key == nil || !ok || len(exp) > 13 {
		return false
	}
	expiry, err := strconv.ParseInt(exp, 36, 64)
	// Only the spelling the mint wrote verifies, so the capability the
	// route echoes into its policy is one the daemon made.
	if err != nil || strconv.FormatInt(expiry, 36) != exp || expiry <= d.fsCapNow().Unix() {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(mac), []byte(fsCapMAC(key, part, replyID, root, expiry))) == 1
}

// fsCapabilityURL is the capability route URL of the file rel under the
// session replyID's root, with a fresh capability.
func (d *Daemon) fsCapabilityURL(replyID, root, rel string) string {
	return d.capabilityURL(fsCapRoutePart, d.mintFSCapability(replyID, root), replyID, rel)
}

// fsViewURL is the page route URL of the file rel under the session
// replyID's root, with a fresh capability; empty when the daemon has no
// capability key.
func (d *Daemon) fsViewURL(replyID, root, rel string) string {
	capability := d.mintCapability(fsViewRoutePart, replyID, root)
	if capability == "" {
		return ""
	}
	return d.capabilityURL(fsViewRoutePart, capability, replyID, rel)
}

// capabilityURL is the URL under the route part (fsCapRoutePart or
// fsViewRoutePart) of the file rel in the session replyID with capability.
func (d *Daemon) capabilityURL(part, capability, replyID, rel string) string {
	segs := strings.Split(filepath.ToSlash(rel), "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return "/s/" + replyID + part + capability + "/" + strings.Join(segs, "/")
}

// previewCapability reports a read of the capability route or the page
// route by its shape alone: the session routes LAN mode serves without the
// token, since the sandboxed document they serve sends no cookie. The route
// itself checks the capability (handleFSCapability) and answers 404 without
// a valid one, and to a fetch or another origin's request (capRefused) in
// LAN mode as on loopback.
func previewCapability(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	rest, ok := strings.CutPrefix(r.URL.Path, "/s/")
	if !ok {
		return false
	}
	replyID, sub, ok := strings.Cut(rest, "/")
	return ok && ValidReplyID(replyID) &&
		(strings.HasPrefix(sub, strings.TrimPrefix(fsCapRoutePart, "/")) || strings.HasPrefix(sub, strings.TrimPrefix(fsViewRoutePart, "/")))
}

// capRefused reports a request the capability and page routes refuse
// whatever capability it carries (see the file comment): a fetch
// (Sec-Fetch-Dest: empty), whose answer a script reads, or one whose Origin
// is neither the daemon's own nor an opaque document's ("null").
func capRefused(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" && origin != "null" && !sameOrigin(r) {
		return true
	}
	return r.Header.Get("Sec-Fetch-Dest") == "empty"
}

// capCORS reports a request the routes answer with
// Access-Control-Allow-Origin: null: an opaque document's CORS load of a
// module script, a style sheet, or a font, which it cannot load without
// one. A fetch is refused (capRefused), and an image, media, or a JSON
// module gets no grant, so no page reads their bytes (see the file comment).
func capCORS(r *http.Request) bool {
	if r.Header.Get("Origin") != "null" {
		return false
	}
	switch r.Header.Get("Sec-Fetch-Dest") {
	case "script", "style", "font":
		return true
	}
	return false
}

// capHost admits the Host values the capability route writes into its
// policy: a host name or address with an optional port, and nothing a
// policy would read as a separator.
var capHost = regexp.MustCompile(`^[A-Za-z0-9.\-\[\]:]+$`)

// fsCapabilityCSP is the capability and page routes' policy for the path
// prefix prefix (an absolute URL ending in a slash); see the file comment.
func fsCapabilityCSP(prefix string) string {
	return "sandbox allow-scripts; default-src " + prefix +
		"; script-src " + prefix + " 'unsafe-inline'" +
		"; style-src " + prefix + " 'unsafe-inline'" +
		"; img-src " + prefix + " data: blob:" +
		"; media-src " + prefix + " data: blob:" +
		"; font-src " + prefix + " data:" +
		"; connect-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'"
}

// handleFSCapability serves the capability route, or with view the page
// route; rest is the path after /fs/cap/ or /fs/view/, the capability and
// the file's path.
func (d *Daemon) handleFSCapability(w http.ResponseWriter, r *http.Request, replyID, rest string, view bool) {
	capability, path, ok := strings.Cut(rest, "/")
	if !ok || path == "" || capRefused(r) || d.index == nil || !ValidReplyID(replyID) || !capHost.MatchString(r.Host) {
		fsNotFound(w)
		return
	}
	s, found := d.index.Session(replyID)
	if !found || s.Project == "" {
		fsNotFound(w)
		return
	}
	part := fsCapRoutePart
	if view {
		part = fsViewRoutePart
	}
	if !d.validCapability(part, capability, replyID, s.Project) {
		if view && r.Header.Get("Sec-Fetch-Dest") == "document" {
			// A reload of the page after its capability expired: the file
			// view, behind the access guard, checks the path and mints anew.
			w.Header().Set("Cache-Control", "no-store")
			http.Redirect(w, r, FileSrc(replyID, path), http.StatusFound)
			return
		}
		fsNotFound(w)
		return
	}
	// The path is the URL's, so it is relative to the root by construction;
	// an absolute spelling is refused rather than resolved.
	native := filepath.FromSlash(path)
	if filepath.IsAbs(native) || isRooted(native) || filepath.VolumeName(native) != "" {
		fsNotFound(w)
		return
	}
	f, rel, info, ok := openContained(s.Project, path)
	if !ok {
		fsNotFound(w)
		return
	}
	defer f.Close()
	if (view && dotDenied(s.Project, rel)) || (!view && framedDenied(r, s.Project, rel)) {
		fsNotFound(w)
		return
	}
	d.serveCapabilityFile(w, r, f, rel, info, "http://"+r.Host+"/s/"+replyID+part+capability+"/", view)
}

// serveCapabilityFile writes the opened file f, at rel under its root with
// stat info, as the capability route serves one, or with view as the page
// route does, under the policy for the path prefix prefix (see the file
// comment). The in-root routes and the outside-link routes (fs_outside.go)
// share it once each has checked its own capability and refusals.
func (d *Daemon) serveCapabilityFile(w http.ResponseWriter, r *http.Request, f io.ReadSeeker, rel string, info os.FileInfo, prefix string, view bool) {
	ext := strings.ToLower(filepath.Ext(rel))
	ctype, ok := capTypes[ext]
	if !ok {
		ctype = "application/octet-stream"
	}
	csp := fsCapabilityCSP(prefix)
	content := f
	if view {
		if _, image := imageType(rel); image {
			csp = imageContentSecurityPolicy
		}
		switch {
		case ext == ".html" || ext == ".htm":
			data, ok := readViewable(f, info)
			if !ok {
				fsNotFound(w)
				return
			}
			content = bytes.NewReader(data)
		case info.Size() > fsViewAssetMax:
			fsNotFound(w)
			return
		}
	}
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", csp)
	if capCORS(r) {
		h.Set("Access-Control-Allow-Origin", "null")
	}
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	http.ServeContent(w, r, "", info.ModTime(), content)
}

// framedDenied reports a file inside root, at rel, that the capability
// route refuses for r: one dotDenied refuses for a browser on this machine
// (localViewer), and one capDenied refuses for any other viewer (see the
// file comment).
func framedDenied(r *http.Request, root, rel string) bool {
	if localViewer(r) {
		return dotDenied(root, rel)
	}
	return capDenied(root, rel)
}

// capDenied reports a file inside root, at rel, that the capability route
// refuses for a LAN viewer: one dotDenied refuses, or one git reports ignored. The
// whole root stays the read scope, so a document's assets in a parent
// folder load, but a framed document cannot load a secret from where
// secrets usually live, even as a script or a style sheet. It fails closed: when the ignore check fails
// inside a repository the file is refused; only a root in no repository
// has no ignored set.
func capDenied(root, rel string) bool {
	if dotDenied(root, rel) {
		return true
	}
	full := filepath.Join(filepath.Clean(root), rel)
	dir, name := filepath.Dir(full), filepath.Base(full)
	if ignored, repo := gitIgnored(dir, []string{name}); repo {
		return ignored[name]
	}
	return !outsideRepo(dir)
}

// dotDenied reports a file inside root, at rel, with a dot-named segment
// in its path from the root, as written or once resolved through its
// symlinks (.env, .git/, .ssh/), or one it cannot resolve: the page
// route's refusal (see the file comment). It does not refuse a file git
// ignores, as capDenied does: a page cannot fetch, and with nosniff and
// the route's own types it can read a file's bytes only as a script, a
// style sheet, or media, while a paged document's folder is often ignored
// as a whole.
func dotDenied(root, rel string) bool {
	full := filepath.Join(filepath.Clean(root), rel)
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return true
	}
	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		return true
	}
	realRel, err := filepath.Rel(realRoot, real)
	if err != nil {
		return true
	}
	for _, p := range []string{rel, realRel} {
		for _, seg := range strings.Split(filepath.ToSlash(p), "/") {
			if strings.HasPrefix(seg, ".") {
				return true
			}
		}
	}
	return false
}

// outsideRepo reports that git places dir in no repository. gitIgnored
// answers no repository for a failure too; this tells the two apart, so a
// failed ignore check inside a repository refuses the read.
func outsideRepo(dir string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), fsIgnoreTimeout)
	defer cancel()
	_, err := (&gitBadgeRunner{timeout: fsIgnoreTimeout}).RunGit(ctx, dir, "rev-parse", "--is-inside-work-tree")
	return err != nil && ctx.Err() == nil && strings.Contains(err.Error(), "not a git repository")
}
