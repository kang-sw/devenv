package wsdashboard

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kang-sw/devenv/internal/wsi18n"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Outside-root file links. A file link anywhere on the dashboard, outside
// the session's project root, opens in the preview
// card, and an HTML one in a tab of its own, for a browser on the daemon's
// machine only:
//
//	POST /s/<reply-id>/fs/outside {"path": <abs>}       what the card shows; mints the capability
//	GET  /s/<reply-id>/fs/ocap/<capability>/<path>      the sandboxed frame's document and its files
//	GET  /s/<reply-id>/fs/oview/<capability>/<path>     an HTML file as a page in a tab of its own
//
// The POST is the only way in. It answers a browser on this machine
// (localViewer; anyone else gets 404) that sent the daemon's own Origin
// (sameOrigin; else 403). There is no board/live-ring provenance check:
// older messages, user messages, tool output and file mentions work alike.
// Shared markup reaches LAN viewers too, so no capability is rendered into
// it; it is minted here, on the click. Home-relative paths expand here only.
//
// What a link grants (grantOutside) is the folder that holds the linked
// file, so an HTML file's relative style sheets and images load, with
// these limits, all on the path resolved through every symlink:
//
//   - a folder too wide to hand out grants the linked file alone
//     (wideFolder): a volume root, the user's home, or a folder directly
//     under either;
//   - a path at or under an always-denied credential location (outsideDenied)
//     never opens, by written/resolved path or file identity, whatever links
//     it; Linux process environment files are denied too;
//   - below the granted folder, a path with a dot-named segment is refused
//     as the in-root capability route refuses it for a browser on this
//     machine (framedDenied, which every viewer of these routes is), so a
//     file git ignores is served, except the linked file itself, which the
//     lead named: it opens dot-named too. The folder's own position is not
//     judged, so a link into ~/.cache/proj opens;
//   - every read stays inside the granted folder once resolved, and a
//     symlinked file is refused (openContained).
//
// The answer is the preview route's (fsPreviewAnswer). Text, markdown, and
// a patch come back in it, read under the grant by the POST itself, so no
// route hands their bytes to a fetch; an HTML or SVG file is a frame on the
// ocap route, an image or a video loads from it, and View is the oview URL
// of an HTML file. Markdown renders with no link rule: its relative links
// and images stay path text, as under any root that is not the session's.
//
// An outside capability is the in-root one's shape (fs_capability.go) with
// the grant in it:
//
//	<expiry>.<mac>.<d|f>.<base64url folder>.<base64url file name>
//
// d grants the folder and f the file alone. The MAC covers the route part,
// the session, its project root, the grant, and the expiry, under the same
// daemon key, so the serving route learns the folder and the one exempt
// file from the URL and trusts neither until the MAC verifies. The route
// parts differ from the in-root ones, so neither capability verifies on the
// other's routes. Serving is loopback-only too: the guard admits the shapes
// without a cookie for sandboxed loads (previewCapability), but the handler
// answers 404 to any request that is not a localViewer, so a capability
// that leaked reads nothing from another host. Everything else is the
// in-root routes' (capRefused, capCORS, the policy, serveCapabilityFile).
//
// A document navigation to the oview route with a capability that no
// longer verifies (a reload after the expiry or a daemon restart) requires
// the localhost login before showing the re-mint page: a daemon-origin page
// whose script posts the mint for the
// path the URL names and replaces itself with the fresh URL, or says the
// file is no longer available when the mint is refused. A GET never mints.
// The path it posts comes from a URL nobody verified; the mint's own checks
// are what gate it, and the page holds it as text.

const (
	fsOutCapRoutePart  = "/fs/ocap/"
	fsOutViewRoutePart = "/fs/oview/"
	// fsOutsidePathMax bounds the folder and file name a capability carries:
	// the route writes its own URL prefix into every directive of its policy.
	fsOutsidePathMax = 1024
)

// outsideDenied are the always-denied folders and files, relative to the
// user's home.
var outsideDenied = []string{
	".ssh", ".gnupg", ".aws", ".config/gh", ".netrc", ".docker",
	".kube", ".git-credentials", ".codex/auth.json", ".claude/.credentials.json",
	".config/gcloud", ".azure", ".npmrc", ".pypirc", ".cargo/credentials", ".cargo/credentials.toml",
	".vault-token", ".password-store", "Library/Keychains",
	".cache/" + wsstate.DefaultCacheDirName + "/dashboard",
}

// OutsideLinkSource is a PageSource that also lists the files outside the
// session's project root its board links: the absolute path the renderer
// resolved (FileAction.Abs) for every file target of the board's topic
// bodies and items that is not inside the root, the capped closed topics
// included. This is render metadata, not an authorization boundary: local
// viewers may open unlisted paths too. ok is false when the session is gone.
type OutsideLinkSource interface {
	ReadOutsideLinks(reg Registration) (abs []string, ok bool)
}

// RecordOutsideLinks wraps a served page's link rules (FileActions,
// OutsideImage) so every file target they resolve outside the session root
// is also handed to record, by its absolute path. A render under the
// wrapped rules lists what the page links outside the root.
func RecordOutsideLinks(actions, outsideImage func(path string) wswhiteboard.FileAction, record func(abs string)) (func(path string) wswhiteboard.FileAction, func(path string) wswhiteboard.FileAction) {
	return func(path string) wswhiteboard.FileAction {
			a := actions(path)
			if a.Rel == "" && a.Abs != "" {
				record(a.Abs)
			}
			return a
		}, func(path string) wswhiteboard.FileAction {
			a := outsideImage(path)
			if a.Outside && a.Abs != "" {
				record(a.Abs)
			}
			return a
		}
}

// outsideGrant is what one outside link opens: the folder Root, resolved
// through every symlink, the linked file's Name in it, and Only when the
// folder is too wide and the file alone is granted.
type outsideGrant struct {
	Root, Name string
	Only       bool
}

// homeDirs are the user's home as the environment names it and as it
// resolves; none when the daemon cannot tell.
func homeDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return nil
	}
	home = filepath.Clean(home)
	homes := []string{home}
	if real, err := filepath.EvalSymlinks(home); err == nil && real != home {
		homes = append(homes, real)
	}
	return homes
}

// atOrBelow reports whether path is base or an entry under it, without
// regard to case: a case-insensitive volume reads ~/.SSH as ~/.ssh.
func atOrBelow(base, path string) bool {
	rel, err := filepath.Rel(strings.ToLower(base), strings.ToLower(path))
	return err == nil && (rel == "." || insideRel(rel))
}

// alwaysDenied reports a cleaned absolute path at or under an always-denied
// folder of a home in homes. String checks are only the fast path: file
// identities also cover filesystem aliases that EvalSymlinks leaves intact
// (Unicode folding and macOS firmlinks). Without a home, deny these names
// anywhere in the path. Linux process environments are denied separately.
func alwaysDenied(path string, homes []string) bool {
	if deniedProcEnviron(path) {
		return true
	}
	if len(homes) == 0 {
		slashed := "/" + strings.ToLower(filepath.ToSlash(path)) + "/"
		for _, name := range outsideDenied {
			if strings.Contains(slashed, "/"+strings.ToLower(name)+"/") {
				return true
			}
		}
	}
	for _, base := range deniedBasePaths(homes) {
		if atOrBelow(base, path) {
			return true
		}
	}
	return deniedFileIdentity(path, homes)
}

// wideFolder reports a resolved folder too wide to grant: a volume root, a
// home in homes, or a folder directly under either. Without a home every
// folder is too wide.
func wideFolder(dir string, homes []string) bool {
	if len(homes) == 0 {
		return true
	}
	parent := filepath.Dir(dir)
	if parent == dir || filepath.Dir(parent) == parent {
		return true
	}
	for _, home := range homes {
		if strings.EqualFold(dir, home) || strings.EqualFold(parent, home) {
			return true
		}
	}
	return false
}

// grantOutside is what a link to the absolute path abs opens (see the file
// comment); false when it opens nothing: no plain absolute path, an
// always-denied one, or no regular file.
func grantOutside(abs string) (outsideGrant, bool) {
	if abs == "" || len(abs) > openTargetMax || strings.ContainsRune(abs, 0) || networkPath(abs) {
		return outsideGrant{}, false
	}
	native := filepath.FromSlash(abs)
	if !filepath.IsAbs(native) || !driveOrRootPath(native) {
		return outsideGrant{}, false
	}
	clean := filepath.Clean(native)
	homes := homeDirs()
	if deviceElement(clean) || alwaysDenied(clean, homes) {
		return outsideGrant{}, false
	}
	real, err := filepath.EvalSymlinks(clean)
	if err != nil || networkPath(real) || !driveOrRootPath(real) || alwaysDenied(real, homes) {
		return outsideGrant{}, false
	}
	// Stat before opening: opening a FIFO would block the handler.
	if info, err := os.Stat(real); err != nil || !info.Mode().IsRegular() {
		return outsideGrant{}, false
	}
	g := outsideGrant{Root: filepath.Dir(real), Name: filepath.Base(real)}
	if len(g.Root)+len(g.Name) > fsOutsidePathMax {
		return outsideGrant{}, false
	}
	g.Only = wideFolder(g.Root, homes)
	return g, true
}

// outsideToken matches a capability's spelling; every character of it is
// one a policy source and a URL path segment carry as written.
var outsideToken = regexp.MustCompile(`^[0-9a-z]{1,13}\.[0-9a-f]{32}\.[df]\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)

// outsideEncoding spells a capability's folder and file name; strict, so
// only the spelling the mint wrote decodes.
var outsideEncoding = base64.RawURLEncoding.Strict()

// fsOutMAC is the MAC of an outside capability for the route part, the
// session replyID with project root project, the grant g, and the Unix
// time expiry. No field holds a NUL, so the fields read back one way only.
func fsOutMAC(key []byte, part, replyID, project string, g outsideGrant, expiry int64) string {
	mode := "d"
	if g.Only {
		mode = "f"
	}
	m := hmac.New(sha256.New, key)
	for _, field := range []string{part, replyID, filepath.Clean(project), mode, g.Root, g.Name, strconv.FormatInt(expiry, 10)} {
		m.Write([]byte(field))
		m.Write([]byte{0})
	}
	return hex.EncodeToString(m.Sum(nil)[:16])
}

// mintOutside mints an outside capability for the route part, the session
// replyID with project root project, and the grant g; empty when the
// daemon has no key.
func (d *Daemon) mintOutside(part, replyID, project string, g outsideGrant) string {
	key := d.fsCapKey()
	if key == nil {
		return ""
	}
	expiry := d.fsCapNow().Add(fsCapabilityTTL).Unix()
	mode := "d"
	if g.Only {
		mode = "f"
	}
	return strconv.FormatInt(expiry, 36) + "." + fsOutMAC(key, part, replyID, project, g, expiry) + "." + mode + "." +
		outsideEncoding.EncodeToString([]byte(g.Root)) + "." + outsideEncoding.EncodeToString([]byte(g.Name))
}

// parseOutside reads a capability's fields without verifying them: the
// expiry, the MAC, and the grant it claims.
func parseOutside(capability string) (expiry int64, mac string, g outsideGrant, ok bool) {
	if !outsideToken.MatchString(capability) {
		return 0, "", outsideGrant{}, false
	}
	f := strings.Split(capability, ".")
	expiry, err := strconv.ParseInt(f[0], 36, 64)
	// Only the spelling the mint wrote reads, as for validCapability.
	if err != nil || strconv.FormatInt(expiry, 36) != f[0] {
		return 0, "", outsideGrant{}, false
	}
	root, err := outsideEncoding.DecodeString(f[3])
	if err != nil || !utf8.Valid(root) || bytes.IndexByte(root, 0) >= 0 {
		return 0, "", outsideGrant{}, false
	}
	name, err := outsideEncoding.DecodeString(f[4])
	if err != nil || !utf8.Valid(name) || bytes.IndexByte(name, 0) >= 0 {
		return 0, "", outsideGrant{}, false
	}
	return expiry, f[1], outsideGrant{Root: string(root), Name: string(name), Only: f[2] == "f"}, true
}

// outsideCapability returns the grant of a capability the daemon minted
// for the route part and the session replyID with project root project
// that has not expired.
func (d *Daemon) outsideCapability(part, capability, replyID, project string) (outsideGrant, bool) {
	key := d.fsCapKey()
	expiry, mac, g, ok := parseOutside(capability)
	if key == nil || !ok || expiry <= d.fsCapNow().Unix() {
		return outsideGrant{}, false
	}
	if subtle.ConstantTimeCompare([]byte(mac), []byte(fsOutMAC(key, part, replyID, project, g, expiry))) != 1 {
		return outsideGrant{}, false
	}
	return g, true
}

// fsOutsideRequest is the mint route's body. Unknown fields are ignored.
type fsOutsideRequest struct {
	Path string `json:"path"`
}

// fsOutsideAnswer is the mint route's answer: the preview route's, with
// View the page route URL of an HTML file.
type fsOutsideAnswer struct {
	fsPreviewAnswer
	View string `json:"view,omitempty"`
}

// handleFSOutside serves the mint route of one session (see the file
// comment). Past the Origin check every refusal answers the same 404, so
// the route tells nothing about which paths exist.
func (d *Daemon) handleFSOutside(w http.ResponseWriter, r *http.Request, replyID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !localViewer(r) {
		http.NotFound(w, r)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	if d.index == nil || !ValidReplyID(replyID) {
		fsNotFound(w)
		return
	}
	s, ok := d.index.Session(replyID)
	if !ok || s.Project == "" {
		fsNotFound(w)
		return
	}
	var req fsOutsideRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, openBodyLimit)).Decode(&req); err != nil {
		http.Error(w, "malformed request", http.StatusBadRequest)
		return
	}
	g, ok := grantOutside(homePath(req.Path))
	if !ok {
		fsNotFound(w)
		return
	}
	f, rel, info, ok := openLocalContained(g.Root, g.Name)
	if !ok {
		fsNotFound(w)
		return
	}
	defer f.Close()
	capability := d.mintOutside(fsOutCapRoutePart, replyID, s.Project, g)
	if capability == "" {
		fsNotFound(w)
		return
	}
	a := fsOutsideAnswer{fsPreviewAnswer: fsPreviewAnswer{Rel: filepath.ToSlash(rel), Abs: filepath.Join(g.Root, rel), Size: info.Size()}}
	lang, _ := requestLang(r)
	ext := strings.ToLower(filepath.Ext(rel))
	_, image := imageType(rel)
	_, video := videoType(rel)
	_, audio := audioType(rel)
	switch {
	case framedFile(rel):
		a.Kind = "frame"
		a.Frame, a.TTL = d.capabilityURL(fsOutCapRoutePart, capability, replyID, rel), int(fsCapabilityTTL/time.Second)
		if ext == ".html" || ext == ".htm" {
			a.View = d.capabilityURL(fsOutViewRoutePart, d.mintOutside(fsOutViewRoutePart, replyID, s.Project, g), replyID, rel)
		}
	case image:
		a.Kind, a.Src = "image", d.capabilityURL(fsOutCapRoutePart, capability, replyID, rel)
	case video:
		a.Kind, a.Src = "video", d.capabilityURL(fsOutCapRoutePart, capability, replyID, rel)
	case audio:
		a.Kind, a.Src = "audio", d.capabilityURL(fsOutCapRoutePart, capability, replyID, rel)
	case info.Size() > fileViewMax:
		a.Kind, a.Reason = "none", wsi18n.T(lang, "file.reason.large", fileViewMax>>20)
	default:
		data, err := io.ReadAll(io.LimitReader(f, fileViewMax+1))
		switch {
		case err != nil:
			fsNotFound(w)
			return
		case len(data) > fileViewMax:
			a.Kind, a.Reason = "none", wsi18n.T(lang, "file.reason.large", fileViewMax>>20)
		case !viewableText(data):
			a.Kind, a.Reason, a.ReasonCode = "none", wsi18n.T(lang, "file.reason.binary"), "binary"
		case ext == ".md" || ext == ".markdown":
			// No link rule: a relative link or image stays path text.
			a.Kind, a.HTML = "markdown", string(wswhiteboard.RenderText(string(data), wswhiteboard.TextTargets{}))
		case ext == ".diff" || ext == ".patch":
			a.Kind, a.Diff = "diff", patchDiffs(data)
		default:
			a.Kind, a.Text = "text", string(data)
		}
	}
	writeFSJSON(w, true, a)
}

// handleFSOutsideCapability serves the ocap route, or with view the oview
// route; rest is the path after the route part, the capability and the
// file's path under the granted folder.
func (d *Daemon) handleFSOutsideCapability(w http.ResponseWriter, r *http.Request, replyID, rest string, view bool) {
	capability, path, ok := strings.Cut(rest, "/")
	if !localViewer(r) || !ok || path == "" || capRefused(r) || d.index == nil || !ValidReplyID(replyID) || !capHost.MatchString(r.Host) {
		fsNotFound(w)
		return
	}
	s, found := d.index.Session(replyID)
	if !found || s.Project == "" {
		fsNotFound(w)
		return
	}
	part := fsOutCapRoutePart
	if view {
		part = fsOutViewRoutePart
	}
	g, ok := d.outsideCapability(part, capability, replyID, s.Project)
	if !ok {
		if view && r.Header.Get("Sec-Fetch-Dest") == "document" {
			d.writeOutsideRemint(w, r, replyID, capability, path)
			return
		}
		fsNotFound(w)
		return
	}
	// The path is the URL's, so it is relative to the folder by
	// construction; an absolute spelling is refused rather than resolved.
	native := filepath.FromSlash(path)
	if filepath.IsAbs(native) || isRooted(native) || filepath.VolumeName(native) != "" {
		fsNotFound(w)
		return
	}
	f, rel, info, ok := openLocalContained(g.Root, path)
	if !ok {
		fsNotFound(w)
		return
	}
	defer f.Close()
	// The linked file is the one path the lead named: it alone is served
	// dot-named, and it is all a file-only grant serves.
	if rel != g.Name && (g.Only || framedDenied(r, g.Root, rel)) {
		fsNotFound(w)
		return
	}
	full := filepath.Join(g.Root, rel)
	homes := homeDirs()
	if real, err := filepath.EvalSymlinks(full); err != nil || alwaysDenied(full, homes) || alwaysDenied(real, homes) {
		fsNotFound(w)
		return
	}
	d.serveCapabilityFile(w, r, f, rel, info, "http://"+r.Host+"/s/"+replyID+part+capability+"/", view)
}

// outsideRemintPage is what the re-mint page's template renders.
type outsideRemintPage struct {
	// Abs is the linked file the expired URL names; Mint the mint route,
	// Base the page route's URL prefix a fresh URL must start with, and Rel
	// the URL-escaped path the expired URL showed under its capability.
	Abs, Mint, Base, Rel string
	Lang, Theme, Nonce   string
	WhiteboardCSS        template.CSS
}

// writeOutsideRemint answers a document navigation to the oview route
// whose capability no longer verifies with the re-mint page (see the file
// comment); 404 when the URL names no capability shape at all. Unlike a
// valid capability read, posting the unverified path requires a logged-in page.
func (d *Daemon) writeOutsideRemint(w http.ResponseWriter, r *http.Request, replyID, capability, path string) {
	if !d.localhostAuthenticated(r) {
		if !d.loginNavigation(w, r) {
			d.refuseLogin(w, r, false)
		}
		return
	}
	_, _, g, ok := parseOutside(capability)
	a, err := loadDashboardAssets()
	var raw [16]byte
	if _, rerr := rand.Read(raw[:]); !ok || err != nil || rerr != nil {
		fsNotFound(w)
		return
	}
	segs := strings.Split(path, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	lang, _ := requestLang(r)
	page := outsideRemintPage{
		Abs:  filepath.Join(g.Root, g.Name),
		Mint: "/s/" + replyID + "/fs/outside",
		Base: "/s/" + replyID + fsOutViewRoutePart,
		Rel:  strings.Join(segs, "/"),
		Lang: lang, Theme: requestTheme(r), Nonce: hex.EncodeToString(raw[:]),
		WhiteboardCSS: a.data.WhiteboardCSS,
	}
	var buf bytes.Buffer
	if err := a.templateFor(lang).ExecuteTemplate(&buf, "file-remint", page); err != nil {
		fmt.Fprintf(d.log, "dashboard: render file-remint: %v\n", err)
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	// The page's one inline script posts the mint to the daemon it came
	// from; nothing else runs or loads.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'nonce-"+page.Nonce+"'; connect-src 'self'; base-uri 'none'; form-action 'none'; "+frameAncestorsNone)
	writeHTML(w, buf.String())
}
