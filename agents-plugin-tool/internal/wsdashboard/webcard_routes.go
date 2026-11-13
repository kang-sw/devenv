package wsdashboard

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// The web card's dashboard routes (the proxy is webcard_proxy.go), all
// under /s/<reply-id>/web and all for a browser on this machine only
// (localViewer: a localhost proxy is an SSRF and execution surface):
//
//   - POST /s/<reply-id>/web registers a target, {url} (a localhost dev
//     server's page) or {path} (an HTML file under the session root, whose
//     directory is registered), and answers {id, kind ("dev" or "static"),
//     base, path}: the opaque registration id (the same target keeps its
//     id), what a pick's url starts with (the target's origin, or the
//     directory relative to the session root, "" for the root), and the path
//     to open on the proxy origin. The card never stores the id; a restored
//     card registers again. Gated by TrustedWrite (not local 404, another
//     origin 403); a refused target answers 400 with why, a full registry
//     or a bind failure 503.
//   - GET /s/<reply-id>/web/<id>?p=<path> is the card's wrapper frame. The
//     card's iframe loads it by a plain navigation, which sends no Origin,
//     so sameOrigin cannot gate it: it takes localViewer (else 404) plus
//     Sec-Fetch-Site same-origin and Sec-Fetch-Dest iframe (else 403). It
//     starts or recreates the target's listener and frames the proxied page
//     at p on the proxy origin. An unknown id answers 404 and the card
//     registers again once.
//   - POST /s/<reply-id>/web/<id>/beat is the wrapper's heartbeat, every
//     minute while it is open, which keeps the target live (webLiveWindow).
//     TrustedWrite-gated; an unknown id answers 404.
//
// The wrapper's response is the one exception to writeHTML's frame refusal
// (frameAncestorsNone and X-Frame-Options DENY, pages.go): the session page
// frames it, so it sends frame-ancestors 'self' and SAMEORIGIN instead. It
// has no write control and a foreign origin still cannot frame it, so the
// clickjacking hazard writeHTML guards against (a framed page whose own
// script posts) does not apply. Its policy's frame-src names only its own
// card's proxy origin; the session page itself stays at frame-src 'self'.
//
// The postMessage protocol. Every message is an object whose wsWeb field
// names its type. The proxied page (the injected webinject.js) is the
// wrapper's inner frame, a grandchild of the session page, so its posts
// reach the card only through the wrapper's relay (webframe.js):
//
//   - inner -> wrapper: posted to the dashboard origin only (http, the
//     page's own host name, the daemon's port), never "*". The wrapper
//     relays a message whose source is its inner frame and whose origin is
//     the proxy origin up to its parent, with targetOrigin its own origin.
//   - host -> wrapper -> inner: the card posts to its iframe's window with
//     targetOrigin its own origin; the wrapper relays a message whose source
//     is its parent and whose origin is its own down to the inner frame with
//     targetOrigin the proxy origin; the injected script takes only a
//     message from its parent with the dashboard origin.
//   - The card host takes a message only when its origin is the page's own
//     and its source is the card's iframe window; a direct post from the
//     inner frame (another source) is dropped.
//
// Types, wrapper -> host: {wsWeb: "frame", proxy} once the wrapper loads
// (proxy is the proxy origin); {wsWeb: "gone"} when a heartbeat finds the id
// unknown. Inner -> host (relayed): {wsWeb: "ready", path} once the script
// runs (path: the page's location path, search, and hash on the proxy
// origin); {wsWeb: "pick", path, selector, text} for a picked element;
// {wsWeb: "picking", on} when pick mode turns on or off on the page's side
// (a pick or Escape ends it); {wsWeb: "errors", count} when the error count
// changes; {wsWeb: "console", path, lines} answering an attach. Host ->
// inner (relayed): {wsWeb: "pick", on} turns pick mode on or off;
// {wsWeb: "attach"} asks for the collected errors, which the script sends
// and then clears, resetting its count.

// webRequestLimit bounds a registration body.
const webRequestLimit = 8 << 10

// webInnerSandbox is the wrapper's inner frame's sandbox: everything a dev
// page uses for itself, and no top navigation, so a page cannot navigate the
// dashboard away. allow-same-origin keeps the proxy's origin (its storage
// and cookies), which differs from the wrapper's, so the page cannot reach
// into the wrapper to lift the sandbox.
const webInnerSandbox = "allow-scripts allow-same-origin allow-forms allow-popups allow-modals allow-downloads allow-popups-to-escape-sandbox"

// webRegistration is the registration route's answer.
type webRegistration struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Base string `json:"base"`
	Path string `json:"path"`
}

// handleWeb serves the web card routes of one session; rest is the path
// after "web".
func (d *Daemon) handleWeb(w http.ResponseWriter, r *http.Request, replyID, rest string) {
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		d.registerWeb(w, r, replyID)
		return
	}
	id, action, _ := strings.Cut(rest, "/")
	switch action {
	case "":
		d.serveWebFrame(w, r, replyID, id)
	case "beat":
		d.webBeat(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (d *Daemon) registerWeb(w http.ResponseWriter, r *http.Request, replyID string) {
	local, own := TrustedWrite(r)
	if !local {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !own {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	s, ok := d.webSession(replyID)
	if !ok {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	var req struct {
		URL  string `json:"url"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, webRequestLimit)).Decode(&req); err != nil || (req.URL == "") == (req.Path == "") {
		http.Error(w, "the body names a url or a path", http.StatusBadRequest)
		return
	}
	var (
		e     *webEntry
		inner string
		err   error
	)
	if req.URL != "" {
		e, inner, err = d.web.registerURL(req.URL)
	} else {
		e, inner, err = d.web.registerDir(s.Project, req.Path)
	}
	var refusal *webRefusal
	switch {
	case errors.As(err, &refusal):
		http.Error(w, refusal.Error(), http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	kind := "dev"
	if e.target == nil {
		kind = "static"
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, webRegistration{ID: e.id, Kind: kind, Base: e.base, Path: inner})
}

func (d *Daemon) webBeat(w http.ResponseWriter, r *http.Request, id string) {
	local, own := TrustedWrite(r)
	if !local {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !own {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	if !d.web.heartbeat(id) {
		http.Error(w, "unknown web card target", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// webSession is the indexed session replyID, which has a root.
func (d *Daemon) webSession(replyID string) (Session, bool) {
	if d.index == nil || !ValidReplyID(replyID) {
		return Session{}, false
	}
	s, ok := d.index.Session(replyID)
	return s, ok && s.Project != ""
}

func (d *Daemon) serveWebFrame(w http.ResponseWriter, r *http.Request, replyID, id string) {
	if !localViewer(r) {
		http.NotFound(w, r)
		return
	}
	if !readMethod(w, r) {
		return
	}
	if r.Header.Get("Sec-Fetch-Site") != "same-origin" || r.Header.Get("Sec-Fetch-Dest") != "iframe" {
		http.Error(w, "the web card frame loads only inside its card", http.StatusForbidden)
		return
	}
	host := canonicalLoopback(r.Host)
	if _, ok := d.webSession(replyID); !ok || host == "" {
		http.NotFound(w, r)
		return
	}
	e, port, err := d.web.frame(id)
	if e == nil {
		http.Error(w, "unknown web card target", http.StatusNotFound)
		return
	}
	if err != nil {
		writeWebFrame(w, http.StatusServiceUnavailable, "", "<p class=\"err\">"+html.EscapeString(err.Error())+"</p>")
		return
	}
	p := r.URL.Query().Get("p")
	if !webFramePath(p) {
		p = "/"
	}
	// host is request-derived, but hostGuard and localViewer admitted only
	// a loopback name, and canonicalLoopback maps it to one of three fixed
	// spellings, never the raw header text: the frame-src, the inner frame,
	// and the script's dashboard origin all use that same host name, so the
	// card and its proxy stay same-site.
	proxy := "http://" + net.JoinHostPort(host, strconv.Itoa(port))
	frame, err := webAsset("webframe.js")
	if err != nil {
		http.Error(w, "the web card frame script is missing", http.StatusInternalServerError)
		return
	}
	nonce := webNonce()
	body := `<iframe id="f" src="` + html.EscapeString(proxy+p) + `" sandbox="` + webInnerSandbox + `" referrerpolicy="no-referrer"></iframe>` +
		`<script nonce="` + nonce + `">` + string(frame) + `</script>`
	w.Header().Set("Content-Security-Policy", webFrameCSP(nonce, proxy))
	writeWebFrame(w, http.StatusOK, ` data-proxy="`+html.EscapeString(proxy)+`" data-beat="/s/`+html.EscapeString(replyID)+`/web/`+html.EscapeString(id)+`/beat"`, body)
}

// webFrameCSP is the wrapper's policy: its own nonced script, inline style,
// heartbeats to its own origin, frames from its card's proxy origin only,
// and framing by its own origin only.
func webFrameCSP(nonce, proxy string) string {
	return "default-src 'none'; script-src 'nonce-" + nonce + "'; style-src 'unsafe-inline'; connect-src 'self'; frame-src " + proxy +
		"; base-uri 'none'; form-action 'none'; frame-ancestors 'self'"
}

// writeWebFrame writes the wrapper page. It sets its own framing headers,
// the exception to writeHTML's (see the file comment): writeHTML sets
// X-Frame-Options DENY unconditionally. A caller that sets no policy (an
// error page) gets one that frames under its own origin only.
func writeWebFrame(w http.ResponseWriter, status int, attrs, body string) {
	h := w.Header()
	if h.Get("Content-Security-Policy") == "" {
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'self'")
	}
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, `<!doctype html><html`+attrs+`><head><meta charset="utf-8"><style>`+
		`html,body{margin:0;height:100%;overflow:hidden;background:#fff}iframe{display:block;border:0;width:100%;height:100%}`+
		`.err{margin:12px;font:13px/1.4 system-ui,sans-serif;color:#b42318}</style></head><body>`+body+`</body></html>`)
}

// canonicalLoopback is the canonical name of a loopback Host header
// (localhost, 127.0.0.1, or ::1, lower case, port and brackets dropped), ""
// for any other host.
func canonicalLoopback(hostport string) string {
	h := hostport
	if hh, _, err := net.SplitHostPort(hostport); err == nil {
		h = hh
	}
	switch h = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")); h {
	case "localhost", "127.0.0.1", "::1":
		return h
	}
	return ""
}

// webFramePath reports a path the wrapper may open on the proxy origin: one
// starting with a single slash (so it stays on that origin) and holding no
// white space or control character.
func webFramePath(p string) bool {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasPrefix(p, "/\\") || len(p) > 4096 {
		return false
	}
	for _, c := range p {
		if c <= 0x20 || c == 0x7f || c == '\\' {
			return false
		}
	}
	return true
}

func webNonce() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}
