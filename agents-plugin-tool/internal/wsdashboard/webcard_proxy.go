package wsdashboard

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// The web card's proxy (the card kind is webcard.js; its dashboard routes
// are webcard_routes.go). A web card shows a page of a localhost dev server,
// or a static file under the session root, inside the dashboard, with a
// small injected script (webinject.js) that picks an element and collects
// console errors for the composer.
//
// Each target (a dev server's origin, or a registered directory) gets its
// own listener on an ephemeral 127.0.0.1 port, so the proxied page runs on
// an origin of its own: its cookies and storage work, and the origin
// boundary keeps its scripts away from the dashboard's routes. The page
// talks to the dashboard only by postMessage, through the card's wrapper
// frame (webcard_routes.go).
//
// A listener:
//
//   - binds 127.0.0.1 explicitly, never bindHostFor, which is every
//     interface in LAN mode, and passes hostGuard first, so a DNS-rebinding
//     page cannot read a registered directory under its own host name;
//   - refuses a request whose Origin is present and not the listener's own
//     (any loopback name on its port), or whose Sec-Fetch-Site is
//     cross-site, before anything reaches the target, WebSocket upgrades
//     included. Its own Origin is rewritten to the target's; rewriting any
//     other would launder a foreign page's Origin past the dev server's own
//     checks;
//   - rewrites the upstream Host to the target's, strips the dashboard's
//     own cookies and an Authorization carrying the dashboard's bearer, and
//     forwards every other cookie and header;
//   - rewrites an absolute Location naming the target to the proxy origin
//     and refuses a redirect to any other origin;
//   - strips the target's X-Frame-Options and the frame-ancestors directive
//     of its policies, and appends its own frame-ancestors naming the
//     dashboard under every loopback name, so a dev page renders in the card
//     and no foreign site can frame the proxy;
//   - injects <script src> of webInjectPath, which it serves itself, into
//     an HTML response (an encoded one only when gzip, which it decodes).
//
// The registry (webRegistry) holds one entry per target, reused by every
// card on it, with at most webListenerCap live listeners. A listener with
// no request in flight, no open upgraded connection, and no live card (a
// wrapper heartbeat within webLiveWindow) for webIdleClose closes; the next
// wrapper load recreates it on the same port when it can, so the page's
// origin, and the storage keyed to it, survive. Daemon.Close closes every
// listener, and the registry then refuses new ones. Registrations live in
// daemon memory only.

const (
	// webListenerCap bounds the live listeners; a new one past it is
	// refused and the card shows why.
	webListenerCap = 16
	// webIdleClose is how long a listener with nothing in flight and no
	// live card stays open.
	webIdleClose = 30 * time.Minute
	// webLiveWindow is how recent a card's heartbeat keeps its target
	// live; the wrapper beats every minute.
	webLiveWindow = 3 * time.Minute
	// webSweepEvery is the idle check's period.
	webSweepEvery = time.Minute
	// webInjectPath is the injected script's reserved path on every proxy
	// origin. Its dot-named segment can never name a served static file
	// (dotDenied).
	webInjectPath = "/.ws-web/inject.js"
	// webInjectPort is the placeholder webinject.js carries for the
	// daemon's port, which the listener bakes in.
	webInjectPort = `"__WS_DASHBOARD_PORT__"`
	// webHTMLMax bounds an HTML body the proxy reads to inject into; a
	// larger one passes uninjected.
	webHTMLMax = 16 << 20
)

var (
	errWebClosed = errors.New("the dashboard is shutting down")
	errWebCap    = fmt.Errorf("at most %d web card targets can be open at once: close a web card on another page first", webListenerCap)
	// errWebRedirect is the proxy's refusal of a redirect to another
	// origin than the target's own.
	errWebRedirect = errors.New("the page redirected to another origin, which a web card does not follow: open it in a tab")
)

// webRefusal is a target the registry refuses to register: not a localhost
// page, the dashboard's own port, a path outside the session root. The
// registration route answers it 400 with its text.
type webRefusal struct{ msg string }

func (e *webRefusal) Error() string { return e.msg }

// webRegistry is the web card targets and their listeners (see the file
// comment).
type webRegistry struct {
	mu     sync.Mutex
	byKey  map[string]*webEntry
	byID   map[string]*webEntry
	closed bool
	// port is the daemon's own port: a target on it is refused, and every
	// listener's frame-ancestors names the dashboard on it.
	port int
	// token is the daemon's bearer (Endpoint.Token), which the proxy never
	// forwards.
	token string
	log   io.Writer
	// now, idle, and live are the idle clock, which a test replaces.
	now        func() time.Time
	idle, live time.Duration
	transport  *http.Transport
	stop       chan struct{}
}

// webEntry is one registered target: a dev server's origin (target), or a
// registered directory (dir, resolved). base is what a pick's url starts
// with: the target's origin, or the directory relative to the session root.
// port is the last port its listener bound, which a recreated listener
// binds again when it can.
type webEntry struct {
	id, key string
	target  *url.URL
	dir     string
	base    string
	port    int
	ln      *webListener
	beat    time.Time
}

// webListener is a target's live listener.
type webListener struct {
	g     *webRegistry
	e     *webEntry
	ln    net.Listener
	srv   *http.Server
	port  int
	proxy *httputil.ReverseProxy
	// inflight counts the requests being served, an upgraded connection
	// included while it is open; active is when the last one ended.
	// conns are the upgraded connections, which closing the server leaves
	// open. All guarded by g.mu.
	inflight int
	active   time.Time
	conns    map[*webTrackedConn]struct{}
}

func newWebRegistry(port int, token string, log io.Writer) *webRegistry {
	if log == nil {
		log = io.Discard
	}
	return &webRegistry{
		byKey: map[string]*webEntry{},
		byID:  map[string]*webEntry{},
		port:  port,
		token: token,
		log:   log,
		now:   time.Now,
		idle:  webIdleClose,
		live:  webLiveWindow,
		transport: &http.Transport{
			// Never an environment proxy: the target is this machine.
			Proxy:       nil,
			DialContext: (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			// A loopback dev server over https serves a self-signed
			// certificate; the target is this machine, so there is no
			// network path to authenticate.
			TLSClientConfig:     &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // loopback-only targets
			DisableCompression:  true,
			MaxIdleConnsPerHost: 8,
			IdleConnTimeout:     90 * time.Second,
		},
		stop: make(chan struct{}),
	}
}

// run closes idle listeners every period until the registry closes.
func (g *webRegistry) run(every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-g.stop:
			return
		case <-t.C:
			g.sweep()
		}
	}
}

// sweep closes every listener idle past g.idle. A request in flight, an
// open upgraded connection, or a live card keeps a listener's idle clock at
// zero.
func (g *webRegistry) sweep() {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	for _, e := range g.byKey {
		l := e.ln
		if l == nil {
			continue
		}
		if l.inflight > 0 || len(l.conns) > 0 || now.Sub(e.beat) < g.live {
			l.active = now
			continue
		}
		if now.Sub(l.active) >= g.idle {
			fmt.Fprintf(g.log, "dashboard: web card listener %s on port %d closed after %s idle\n", e.key, l.port, g.idle)
			g.closeListener(l)
		}
	}
}

// close closes every listener and refuses new ones from now on.
func (g *webRegistry) close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return
	}
	g.closed = true
	for _, e := range g.byKey {
		if e.ln != nil {
			g.closeListener(e.ln)
		}
	}
	close(g.stop)
	g.transport.CloseIdleConnections()
}

// closeListener closes l, its server, and its upgraded connections; the
// entry keeps its port. The caller holds g.mu.
func (g *webRegistry) closeListener(l *webListener) {
	if l.e.ln == l {
		l.e.ln = nil
	}
	_ = l.srv.Close()
	// Serve runs on its own goroutine and Close closes only the listeners
	// it has registered: closing it here releases the port at once.
	_ = l.ln.Close()
	// The raw connection: the tracked Close takes g.mu, held here.
	for c := range l.conns {
		_ = c.Conn.Close()
	}
	l.conns = nil
}

// ownPort reports whether port is the daemon's own or a live listener's.
// The caller holds g.mu.
func (g *webRegistry) ownPort(port int) bool {
	if port == g.port {
		return true
	}
	for _, e := range g.byKey {
		if e.ln != nil && e.ln.port == port {
			return true
		}
	}
	return false
}

// webTarget checks a dev-server link against the allow-list: http or https
// on localhost, 127.0.0.1, or [::1] exactly, with no user info. It returns
// the target's origin (scheme and host:port, the port made explicit) and
// the path the card opens on it.
func webTarget(raw string) (*url.URL, string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Opaque != "" {
		return nil, "", &webRefusal{"a web card opens an http or https page on localhost, 127.0.0.1, or [::1]"}
	}
	host := strings.ToLower(u.Hostname())
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return nil, "", &webRefusal{"a web card opens an http or https page on localhost, 127.0.0.1, or [::1]"}
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[u.Scheme]
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return nil, "", &webRefusal{"the link's port is not a port"}
	}
	target := &url.URL{Scheme: u.Scheme, Host: net.JoinHostPort(host, port)}
	inner := u.EscapedPath()
	if inner == "" {
		inner = "/"
	}
	if u.RawQuery != "" {
		inner += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		inner += "#" + u.EscapedFragment()
	}
	return target, inner, nil
}

// registerURL registers the dev-server page at raw and makes sure its
// listener runs. It returns the entry and the path to open on the proxy
// origin. A refused target is a *webRefusal; a full registry errWebCap.
func (g *webRegistry) registerURL(raw string) (*webEntry, string, error) {
	target, inner, err := webTarget(raw)
	if err != nil {
		return nil, "", err
	}
	port, _ := strconv.Atoi(target.Port())
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, "", errWebClosed
	}
	if g.ownPort(port) {
		return nil, "", &webRefusal{fmt.Sprintf("port %d is the dashboard's own", port)}
	}
	key := target.String()
	e, created := g.entry(key, func(e *webEntry) { e.target, e.base = target, key })
	if _, err := g.ensure(e); err != nil {
		if created {
			g.forget(e)
		}
		return nil, "", err
	}
	return e, inner, nil
}

// registerDir registers the directory of the file at p (relative to root,
// or absolute inside it; a directory itself registers as is) and makes sure
// its listener runs. The directory is resolved through its symlinks and
// must lie in root resolved the same way; the root itself counts, for a page
// at its top. It returns the entry and the path to open on the proxy origin.
func (g *webRegistry) registerDir(root, p string) (*webEntry, string, error) {
	refuse := &webRefusal{"a web card opens a file under the session root"}
	rel, ok := relativeInside(root, p)
	if !ok {
		return nil, "", refuse
	}
	full := filepath.Join(filepath.Clean(root), rel)
	info, err := os.Stat(full)
	if err != nil {
		return nil, "", refuse
	}
	dir, inner := full, "/"
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return nil, "", refuse
		}
		dir, inner = filepath.Dir(full), "/"+url.PathEscape(filepath.Base(full))
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, "", refuse
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, "", refuse
	}
	base, err := filepath.Rel(realRoot, realDir)
	if err != nil || (base != "." && !insideRel(base)) {
		return nil, "", refuse
	}
	if base == "." {
		base = ""
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, "", errWebClosed
	}
	e, created := g.entry("dir:"+realDir, func(e *webEntry) { e.dir, e.base = realDir, filepath.ToSlash(base) })
	if _, err := g.ensure(e); err != nil {
		if created {
			g.forget(e)
		}
		return nil, "", err
	}
	return e, inner, nil
}

// entry returns the entry for key, creating it with fill when absent. The
// caller holds g.mu.
func (g *webRegistry) entry(key string, fill func(*webEntry)) (*webEntry, bool) {
	if e := g.byKey[key]; e != nil {
		return e, false
	}
	buf := make([]byte, 12)
	_, _ = rand.Read(buf)
	e := &webEntry{id: hex.EncodeToString(buf), key: key}
	fill(e)
	g.byKey[key], g.byID[e.id] = e, e
	return e, true
}

// forget drops an entry the registry could not start. The caller holds
// g.mu.
func (g *webRegistry) forget(e *webEntry) {
	delete(g.byKey, e.key)
	delete(g.byID, e.id)
}

// frame is the wrapper route's lookup: the entry registered as id, its
// listener started or recreated, and its port. A wrapper load counts as a
// live card. A nil entry is an unknown id.
func (g *webRegistry) frame(id string) (*webEntry, int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	e := g.byID[id]
	if e == nil {
		return nil, 0, nil
	}
	e.beat = g.now()
	port, err := g.ensure(e)
	return e, port, err
}

// heartbeat marks the target registered as id live; false for an unknown
// id.
func (g *webRegistry) heartbeat(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	e := g.byID[id]
	if e == nil {
		return false
	}
	e.beat = g.now()
	return true
}

// ensure starts e's listener when it has none: on its last port when that
// is free, else on an ephemeral one. The caller holds g.mu.
func (g *webRegistry) ensure(e *webEntry) (int, error) {
	if g.closed {
		return 0, errWebClosed
	}
	if e.ln != nil {
		return e.ln.port, nil
	}
	live := 0
	for _, o := range g.byKey {
		if o.ln != nil {
			live++
		}
	}
	if live >= webListenerCap {
		return 0, errWebCap
	}
	var ln net.Listener
	var err error
	if e.port > 0 {
		ln, err = net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(e.port)))
	}
	if ln == nil {
		if ln, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
			return 0, fmt.Errorf("the web card could not bind a loopback port: %w", err)
		}
	}
	l := &webListener{g: g, e: e, ln: ln, port: ln.Addr().(*net.TCPAddr).Port, active: g.now(), conns: map[*webTrackedConn]struct{}{}}
	if e.target != nil {
		l.proxy = g.reverseProxy(e.target)
	}
	l.srv = &http.Server{Handler: hostGuard(l), ReadHeaderTimeout: 10 * time.Second}
	e.ln, e.port = l, l.port
	go func() { _ = l.srv.Serve(ln) }()
	return l.port, nil
}

// webHostKey carries the inbound Host (the proxy origin as the browser
// addressed it) to the response rewrite.
type webHostKey struct{}

// ServeHTTP serves one request on a listener (see the file comment).
func (l *webListener) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g := l.g
	g.mu.Lock()
	l.inflight++
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		l.inflight--
		l.active = g.now()
		g.mu.Unlock()
	}()
	if webForeign(r, l.port) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	if r.URL.Path == webInjectPath {
		l.serveInject(w, r)
		return
	}
	if l.e.target == nil {
		l.serveStatic(w, r)
		return
	}
	ctx := context.WithValue(r.Context(), webHostKey{}, r.Host)
	l.proxy.ServeHTTP(&webTrackWriter{ResponseWriter: w, l: l}, r.WithContext(ctx))
}

// webForeign reports a request the proxy refuses before it reaches the
// target: Sec-Fetch-Site cross-site, or an Origin present and not the
// listener's own (an opaque "null" included). A request with no Origin, a
// navigation, passes.
func webForeign(r *http.Request, port int) bool {
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return true
	}
	for _, o := range r.Header.Values("Origin") {
		if !webOwnOrigin(o, port) {
			return true
		}
	}
	return false
}

// webOwnOrigin reports whether origin is a listener's own: http, a loopback
// name, and the listener's port.
func webOwnOrigin(origin string, port int) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" {
		return false
	}
	return loopbackHost(u.Host) && u.Port() == strconv.Itoa(port)
}

// webAncestors is every listener's frame-ancestors policy: the dashboard
// on the daemon's port under each loopback name the viewer may use (the
// wrapper, the session page, and any shell framing it are all the
// dashboard's origin).
func (g *webRegistry) webAncestors() string {
	p := strconv.Itoa(g.port)
	return "frame-ancestors http://localhost:" + p + " http://127.0.0.1:" + p + " http://[::1]:" + p
}

// dashboardCookies are the daemon's own cookies, which the proxy never
// forwards to a target.
var dashboardCookies = map[string]bool{themeCookie: true, wsi18n.CookieName: true, lanTokenCookie: true}

// reverseProxy is a dev-server listener's proxy to target.
func (g *webRegistry) reverseProxy(target *url.URL) *httputil.ReverseProxy {
	origin := target.Scheme + "://" + target.Host
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			out := pr.Out
			out.Host = target.Host
			stripDashboardCookies(out.Header)
			if auth := out.Header.Get("Authorization"); auth != "" && g.token != "" && auth == "Bearer "+g.token {
				out.Header.Del("Authorization")
			}
			// webForeign admitted only the listener's own Origin.
			if out.Header.Get("Origin") != "" {
				out.Header.Set("Origin", origin)
			}
			out.Header.Set("Accept-Encoding", "identity")
		},
		Transport:     g.transport,
		FlushInterval: -1,
		ModifyResponse: func(res *http.Response) error {
			host, _ := res.Request.Context().Value(webHostKey{}).(string)
			if loc := res.Header.Get("Location"); loc != "" {
				rewritten, ok := webLocation(loc, target, host)
				if !ok {
					return errWebRedirect
				}
				res.Header.Set("Location", rewritten)
			}
			g.frameHeaders(res.Header)
			if res.StatusCode == http.StatusSwitchingProtocols {
				return nil
			}
			return injectResponse(res)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, errWebRedirect) {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			if errors.Is(err, context.Canceled) {
				return
			}
			http.Error(w, "the dev server at "+origin+" did not answer: "+err.Error(), http.StatusBadGateway)
		},
	}
}

// stripDashboardCookies drops the daemon's own cookies from h's Cookie
// headers and keeps every other cookie.
func stripDashboardCookies(h http.Header) {
	vals := h.Values("Cookie")
	if len(vals) == 0 {
		return
	}
	var kept []string
	for _, v := range vals {
		for _, part := range strings.Split(v, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			name, _, _ := strings.Cut(part, "=")
			if !dashboardCookies[strings.TrimSpace(name)] {
				kept = append(kept, part)
			}
		}
	}
	h.Del("Cookie")
	if len(kept) > 0 {
		h.Set("Cookie", strings.Join(kept, "; "))
	}
}

// webLocation rewrites a redirect's Location: a relative one stays, an
// absolute one naming the target's origin moves to the proxy origin
// (http://host, the inbound Host), and any other origin is refused.
func webLocation(loc string, target *url.URL, host string) (string, bool) {
	u, err := url.Parse(loc)
	if err != nil {
		return "", false
	}
	if u.Scheme == "" && u.Host == "" {
		return loc, true
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = target.Scheme
	}
	if !strings.EqualFold(scheme, target.Scheme) || !sameHostPort(u, scheme, target.Host) || host == "" {
		return "", false
	}
	u.Scheme, u.Host = "http", host
	return u.String(), true
}

// sameHostPort reports whether u names hostport, its port made explicit
// from scheme when absent.
func sameHostPort(u *url.URL, scheme, hostport string) bool {
	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[strings.ToLower(scheme)]
	}
	return strings.EqualFold(net.JoinHostPort(strings.ToLower(u.Hostname()), port), hostport)
}

// frameHeaders makes a response frameable by the dashboard only: the
// target's X-Frame-Options goes, the frame-ancestors directive leaves every
// policy (every other directive stays), and the listener's own
// frame-ancestors policy is appended.
func (g *webRegistry) frameHeaders(h http.Header) {
	h.Del("X-Frame-Options")
	for _, name := range []string{"Content-Security-Policy", "Content-Security-Policy-Report-Only"} {
		vals := h.Values(name)
		if len(vals) == 0 {
			continue
		}
		h.Del(name)
		for _, v := range vals {
			if kept := stripFrameAncestors(v); kept != "" {
				h.Add(name, kept)
			}
		}
	}
	h.Add("Content-Security-Policy", g.webAncestors())
}

// stripFrameAncestors removes the frame-ancestors directive from every
// policy of one header value (policies separated by commas, directives by
// semicolons); "" when nothing is left.
func stripFrameAncestors(v string) string {
	var policies []string
	for _, policy := range strings.Split(v, ",") {
		var dirs []string
		for _, d := range strings.Split(policy, ";") {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			name, _, _ := strings.Cut(d, " ")
			if strings.EqualFold(strings.TrimSpace(name), "frame-ancestors") {
				continue
			}
			dirs = append(dirs, d)
		}
		if len(dirs) > 0 {
			policies = append(policies, strings.Join(dirs, "; "))
		}
	}
	return strings.Join(policies, ", ")
}

// injectResponse injects the script tag into an HTML response: an
// identity body as it is, a gzip one decoded first and sent without its
// Content-Encoding. Another encoding, a body past webHTMLMax, a HEAD, and a
// bodiless status pass uninjected (the card then shows pick as
// unavailable).
func injectResponse(res *http.Response) error {
	if res.Request != nil && res.Request.Method == http.MethodHead {
		return nil
	}
	if res.StatusCode == http.StatusNoContent || res.StatusCode == http.StatusNotModified || res.StatusCode < 200 {
		return nil
	}
	if mt, _, err := mime.ParseMediaType(res.Header.Get("Content-Type")); err != nil || mt != "text/html" {
		return nil
	}
	enc := strings.ToLower(strings.TrimSpace(res.Header.Get("Content-Encoding")))
	var src io.Reader = res.Body
	switch enc {
	case "", "identity":
	case "gzip", "x-gzip":
		zr, err := gzip.NewReader(res.Body)
		if err != nil {
			return nil
		}
		src = zr
	default:
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(src, webHTMLMax+1))
	if err != nil {
		return err
	}
	if len(body) > webHTMLMax {
		if enc != "" && enc != "identity" {
			// The decoded prefix cannot go back into the encoded stream.
			return errors.New("the page is too large to show in a web card")
		}
		res.Body = struct {
			io.Reader
			io.Closer
		}{io.MultiReader(bytes.NewReader(body), res.Body), res.Body}
		return nil
	}
	_ = res.Body.Close()
	body = injectScript(body)
	res.Body = io.NopCloser(bytes.NewReader(body))
	res.ContentLength = int64(len(body))
	res.Header.Del("Content-Encoding")
	res.Header.Del("Content-Length")
	res.Header.Del("ETag")
	return nil
}

// webInjectTag is the tag the proxy injects: a script from its own
// origin, never inline, which a target's script-src would refuse.
const webInjectTag = `<script src="` + webInjectPath + `"></script>`

// injectScript puts webInjectTag right after the document's <head> start
// tag, else its <html> start tag, else its doctype, else at its start, so it
// runs before the page's own scripts.
func injectScript(body []byte) []byte {
	lower := bytes.ToLower(body)
	at := -1
	for _, tag := range []string{"<head", "<html", "<!doctype"} {
		if i := startTag(lower, tag); i >= 0 {
			if end := bytes.IndexByte(lower[i:], '>'); end >= 0 {
				at = i + end + 1
				break
			}
		}
	}
	if at < 0 {
		at = 0
	}
	out := make([]byte, 0, len(body)+len(webInjectTag))
	out = append(out, body[:at]...)
	out = append(out, webInjectTag...)
	return append(out, body[at:]...)
}

// startTag finds tag (lower case) in lower as a start tag: followed by '>',
// '/', or white space, so <header> is not <head>.
func startTag(lower []byte, tag string) int {
	from := 0
	for {
		i := bytes.Index(lower[from:], []byte(tag))
		if i < 0 {
			return -1
		}
		i += from
		next := i + len(tag)
		if next >= len(lower) {
			return -1
		}
		switch lower[next] {
		case '>', '/', ' ', '\t', '\n', '\r', '\f':
			return i
		}
		from = next
	}
}

// serveInject serves the injected script with the daemon's port baked in:
// the script posts only to the dashboard's origin, its own host name on
// that port.
func (l *webListener) serveInject(w http.ResponseWriter, r *http.Request) {
	if !readMethod(w, r) {
		return
	}
	src, err := webAsset("webinject.js")
	if err != nil {
		http.Error(w, "the web card script is missing", http.StatusInternalServerError)
		return
	}
	src = bytes.Replace(src, []byte(webInjectPort), []byte(`"`+strconv.Itoa(l.g.port)+`"`), 1)
	h := w.Header()
	h.Set("Content-Type", "text/javascript; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(src)
}

// serveStatic serves a file of a registered directory, read-only. The
// directory must still resolve to itself and the file must resolve inside
// it, both checked on every request, so a symlink swapped in later cannot
// lead out; a dot-named segment (as written or resolved) is refused below
// the directory, never above it.
func (l *webListener) serveStatic(w http.ResponseWriter, r *http.Request) {
	if !readMethod(w, r) {
		return
	}
	dir := l.e.dir
	if real, err := filepath.EvalSymlinks(dir); err != nil || real != dir {
		fsNotFound(w)
		return
	}
	p := r.URL.Path
	if strings.HasSuffix(p, "/") {
		p += "index.html"
	}
	rel := strings.TrimPrefix(path.Clean("/"+p), "/")
	if rel == "" {
		fsNotFound(w)
		return
	}
	f, rel, info, ok := openContained(dir, rel)
	if !ok {
		fsNotFound(w)
		return
	}
	defer f.Close()
	if dotDenied(dir, rel) {
		fsNotFound(w)
		return
	}
	ext := strings.ToLower(filepath.Ext(rel))
	ctype, known := capTypes[ext]
	if !known {
		ctype = "application/octet-stream"
	}
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", l.g.webAncestors())
	var content io.ReadSeeker = f
	if ext == ".html" || ext == ".htm" {
		if info.Size() > webHTMLMax {
			fsNotFound(w)
			return
		}
		data, err := io.ReadAll(io.LimitReader(f, webHTMLMax+1))
		if err != nil || len(data) > webHTMLMax {
			fsNotFound(w)
			return
		}
		content = bytes.NewReader(injectScript(data))
	}
	http.ServeContent(w, r, "", info.ModTime(), content)
}

// webTrackWriter records the connection a WebSocket upgrade hijacks, which
// closing the listener's server leaves open, so the listener closes it.
type webTrackWriter struct {
	http.ResponseWriter
	l *webListener
}

func (t *webTrackWriter) Unwrap() http.ResponseWriter { return t.ResponseWriter }

func (t *webTrackWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	c, brw, err := http.NewResponseController(t.ResponseWriter).Hijack()
	if err != nil {
		return nil, nil, err
	}
	g := t.l.g
	g.mu.Lock()
	defer g.mu.Unlock()
	if t.l.conns == nil {
		// The listener closed meanwhile.
		_ = c.Close()
		return nil, nil, errWebClosed
	}
	tc := &webTrackedConn{Conn: c, l: t.l}
	t.l.conns[tc] = struct{}{}
	return tc, brw, nil
}

// webTrackedConn leaves its listener's upgraded connections when closed.
type webTrackedConn struct {
	net.Conn
	l    *webListener
	once sync.Once
}

func (c *webTrackedConn) Close() error {
	c.once.Do(func() {
		g := c.l.g
		g.mu.Lock()
		delete(c.l.conns, c)
		g.mu.Unlock()
	})
	return c.Conn.Close()
}

// webAsset reads a web card asset the daemon serves outside the page: from
// the working tree with dev assets on, else the embedded copy.
func webAsset(name string) ([]byte, error) {
	if root := devAssets.Load(); root != nil {
		return root.ReadFile(devAssetDir + "/" + name)
	}
	return dashboardFS.ReadFile("assets/" + name)
}
