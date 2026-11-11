package wsdashboard

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
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
//     segment (.env, .git/, .ssh/) or that git reports ignored, where
//     secrets usually live (capDenied); the preview route mints none for
//     such a file and shows its source instead.
//
// Every response carries a Content-Security-Policy that:
//
//   - sandboxes the document itself ("sandbox allow-scripts"), so the file
//     opened outside the card's frame, in a tab of its own, still runs in
//     an opaque origin and never in the daemon's;
//   - limits every fetch directive to the capability's own path prefix (and
//     data: or blob: where a document makes its own), so the frame can
//     request neither another origin nor any other dashboard route;
//   - allows no form target and no base URL change.
//
// Access-Control-Allow-Origin is "*": a module script or a fetch from the
// frame's opaque origin is a CORS request, and the capability in the path,
// not the origin, is what admits a read. Every refusal answers 404.

const (
	// fsCapabilityTTL is how long a minted capability is served.
	fsCapabilityTTL = 15 * time.Minute
	fsCapRoutePart  = "/fs/cap/"
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

// fsCapMAC is the MAC of a capability for the session replyID with project
// root root that expires at the Unix time expiry.
func fsCapMAC(key []byte, replyID, root string, expiry int64) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(replyID))
	m.Write([]byte{0})
	m.Write([]byte(filepath.Clean(root)))
	m.Write([]byte{0})
	m.Write([]byte(strconv.FormatInt(expiry, 10)))
	return hex.EncodeToString(m.Sum(nil)[:16])
}

// mintFSCapability mints a capability for the session replyID and its
// project root root; empty when the daemon has no key.
func (d *Daemon) mintFSCapability(replyID, root string) string {
	key := d.fsCapKey()
	if key == nil {
		return ""
	}
	expiry := d.fsCapNow().Add(fsCapabilityTTL).Unix()
	return strconv.FormatInt(expiry, 36) + "-" + fsCapMAC(key, replyID, root, expiry)
}

// validFSCapability reports whether capability is one the daemon minted
// for the session replyID and root and has not expired.
func (d *Daemon) validFSCapability(capability, replyID, root string) bool {
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
	return subtle.ConstantTimeCompare([]byte(mac), []byte(fsCapMAC(key, replyID, root, expiry))) == 1
}

// fsCapabilityURL is the capability route URL of the file rel under the
// session replyID's root, with a fresh capability.
func (d *Daemon) fsCapabilityURL(replyID, root, rel string) string {
	segs := strings.Split(filepath.ToSlash(rel), "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return "/s/" + replyID + fsCapRoutePart + d.mintFSCapability(replyID, root) + "/" + strings.Join(segs, "/")
}

// previewCapability reports a read of the capability route by its shape
// alone: the one session route LAN mode serves without the token, since
// the sandboxed frame it serves sends no cookie. The route itself checks
// the capability (handleFSCapability) and answers 404 without a valid one.
func previewCapability(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	rest, ok := strings.CutPrefix(r.URL.Path, "/s/")
	if !ok {
		return false
	}
	replyID, sub, ok := strings.Cut(rest, "/")
	return ok && ValidReplyID(replyID) && strings.HasPrefix(sub, strings.TrimPrefix(fsCapRoutePart, "/"))
}

// capHost admits the Host values the capability route writes into its
// policy: a host name or address with an optional port, and nothing a
// policy would read as a separator.
var capHost = regexp.MustCompile(`^[A-Za-z0-9.\-\[\]:]+$`)

// fsCapabilityCSP is the capability route's policy for the path prefix
// prefix (an absolute URL ending in a slash); see the file comment.
func fsCapabilityCSP(prefix string) string {
	return "sandbox allow-scripts; default-src " + prefix +
		"; script-src " + prefix + " 'unsafe-inline'" +
		"; style-src " + prefix + " 'unsafe-inline'" +
		"; img-src " + prefix + " data: blob:" +
		"; media-src " + prefix + " data: blob:" +
		"; font-src " + prefix + " data:" +
		"; connect-src " + prefix +
		"; object-src 'none'; base-uri 'none'; form-action 'none'"
}

// handleFSCapability serves the capability route; rest is the path after
// /fs/cap/, the capability and the file's path.
func (d *Daemon) handleFSCapability(w http.ResponseWriter, r *http.Request, replyID, rest string) {
	capability, path, ok := strings.Cut(rest, "/")
	if !ok || path == "" || d.index == nil || !ValidReplyID(replyID) || !capHost.MatchString(r.Host) {
		fsNotFound(w)
		return
	}
	s, found := d.index.Session(replyID)
	if !found || s.Project == "" || !d.validFSCapability(capability, replyID, s.Project) {
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
	if capDenied(s.Project, rel) {
		fsNotFound(w)
		return
	}
	ctype, ok := capTypes[strings.ToLower(filepath.Ext(rel))]
	if !ok {
		ctype = "application/octet-stream"
	}
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", fsCapabilityCSP("http://"+r.Host+"/s/"+replyID+fsCapRoutePart+capability+"/"))
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	http.ServeContent(w, r, "", info.ModTime(), f)
}

// capDenied reports a file inside root, at rel, that the capability route
// still refuses: one with a dot-named segment in its path from the root,
// as written or once resolved through its symlinks, or one git reports
// ignored. The whole root stays the read scope, so a document's assets in
// a parent folder load, but a framed document's script cannot fetch a
// secret from where secrets usually live. It fails closed: when the ignore
// check fails inside a repository the file is refused; only a root in no
// repository has no ignored set.
func capDenied(root, rel string) bool {
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
	dir, name := filepath.Dir(full), filepath.Base(full)
	if ignored, repo := gitIgnored(dir, []string{name}); repo {
		return ignored[name]
	}
	return !outsideRepo(dir)
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
