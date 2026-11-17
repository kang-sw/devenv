package wsdashboard

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Browser access is deny-by-default in both modes. A local viewer needs the
// persisted localhost credential; a non-local viewer in LAN mode needs the
// separate LAN token. Neither credential grants access in the other branch.
// Loopback mode also keeps hostGuard's DNS-rebinding defense. Machine/PWA
// exemptions require a loopback peer and Host (accessExempt); route-level
// checks and auth routes' own checks still apply.
//
// Sandboxed frames have opaque origins and send no cookie, so these read
// routes use fixed assets or path capabilities instead:
//
//   - GET and HEAD of the shared mermaid script and the shared frame
//     stylesheet and script (publicAsset), which a topic body's sandboxed
//     frame loads. That frame also inlines its images as
//     data: URLs (ImageDataURL, up to 2 MiB); a larger image is relayed by
//     the same-origin host page through the protected image route.
//   - GET and HEAD of the HTML preview's capability route
//     (previewCapability, fs_capability.go), which a preview card's
//     sandboxed frame loads with its relative assets, and of the file
//     view's page route, the same route for an HTML file opened in a tab.
//     It is admitted by the short-lived, root-scoped capability its path
//     carries instead: the guard passes the route by its shape and the
//     route answers 404 without a valid capability, and to a fetch or
//     another origin's request with one (capRefused).
//
// The LAN token is not the endpoint file's shutdown token and is never
// written to the endpoint file, so a LAN browser never holds the shutdown
// credential. It persists in its own owner-only file in the state directory,
// so a daemon replaced by an upgrade keeps the token a second device
// bookmarked; deleting the file issues a new one.
//
// The token travels as the "token" query parameter of the address the
// localhost Settings card shows. A request that passes by it gets the token back
// as an HttpOnly cookie, which the page's links, its stream (a WebSocket
// handshake, like an EventSource, sends cookies but cannot set headers), its
// images, and its message and cancel posts then carry on their own, so no
// rendered URL or region ever holds the token.

const (
	// LANTokenFileName names the LAN access token's file in the state
	// directory.
	LANTokenFileName = "lan-token"

	lanTokenParam  = "token"
	lanTokenCookie = "ws_dashboard_token"
	// lanCookieMaxAge keeps the cookie across browser restarts; browsers cap
	// a cookie's lifetime at 400 days.
	lanCookieMaxAge = 400 * 24 * 60 * 60
)

// lanToken returns the LAN access token, minting and persisting one when its
// file is missing or blank. It reads the file on every call, so deleting the
// file rotates the token without a daemon restart.
func (d *Daemon) lanToken() (string, error) {
	d.tokenMu.Lock()
	defer d.tokenMu.Unlock()
	raw, err := os.ReadFile(filepath.Join(d.dir, LANTokenFileName))
	switch {
	case err == nil:
		if token := strings.TrimSpace(string(raw)); token != "" {
			return token, nil
		}
	case !errors.Is(err, fs.ErrNotExist):
		return "", err
	}
	token, err := newToken()
	if err != nil {
		return "", err
	}
	if err := writePrivateFile(d.dir, LANTokenFileName, []byte(token+"\n")); err != nil {
		return "", err
	}
	fmt.Fprintf(d.log, "dashboard: issued a new LAN access token\n")
	return token, nil
}

// accessGuard covers every mux route, including future additions.
func (d *Daemon) accessGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if accessExempt(r) {
			next.ServeHTTP(w, r)
			return
		}
		if localViewer(r) {
			if d.loginNavigation(w, r) {
				return
			}
			if d.localhostAuthenticated(r) {
				next.ServeHTTP(w, r)
				return
			}
			d.refuseLogin(w, r, false)
			return
		}
		if !d.lan {
			loginRequired(w)
			return
		}
		token, err := d.lanToken()
		if err != nil {
			fmt.Fprintf(d.log, "dashboard: LAN access token unavailable: %v\n", err)
			http.Error(w, "the LAN access token is unavailable", http.StatusServiceUnavailable)
			return
		}
		if tokenMatches(r.URL.Query().Get(lanTokenParam), token) {
			http.SetCookie(w, &http.Cookie{
				Name:     lanTokenCookie,
				Value:    token,
				Path:     "/",
				MaxAge:   lanCookieMaxAge,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
			next.ServeHTTP(w, r)
			return
		}
		if c, err := r.Cookie(lanTokenCookie); err == nil && tokenMatches(c.Value, token) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "this dashboard needs its LAN access token: on the machine it runs on, open Settings (the gear in the session rail footer) at http://localhost:"+strconv.Itoa(d.endpoint.Port)+"/ for the address that carries the token", http.StatusUnauthorized)
	})
}

// publicAsset reports a read of the shared mermaid script or a shared
// topic-body frame asset, the routes LAN mode serves without the token. A
// board's topic body renders in a sandboxed frame whose opaque origin sends
// no cookie (wswhiteboard framedDoc): every frame loads the frame stylesheet
// and script (wswhiteboard.FrameAsset), and a mermaid block there loads the
// mermaid script itself. Each is a fixed build asset under a content-hash
// name, so serving it reveals nothing about any session.
func publicAsset(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	rest, ok := strings.CutPrefix(r.URL.Path, "/s/")
	if !ok {
		return false
	}
	if name, _ := wswhiteboard.MermaidAsset(); rest == name {
		return true
	}
	_, _, ok = wswhiteboard.FrameAsset(rest)
	return ok
}

func tokenMatches(got, token string) bool {
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

// localViewer reports whether r comes from a browser on this machine: a
// loopback peer that addressed the daemon by a loopback name.
func localViewer(r *http.Request) bool {
	return loopbackPeer(r.RemoteAddr) && loopbackHost(r.Host)
}

// TrustedWrite is the gate of every write that carries execution authority
// (a terminal, a holder stop, a hosted lead session's spawn, settings, stop,
// close, and tool approval): local reports a browser on this machine
// (localViewer; a caller answers 404 without it, as if the route did not
// exist), and sameOrigin that the request carries the daemon's own Origin
// (a caller answers 403 without it). Browser routes additionally require the
// localhost cookie at the outer access guard. The LAN token alone never passes.
func TrustedWrite(r *http.Request) (local, own bool) {
	return localViewer(r), sameOrigin(r)
}

// LocalViewer reports whether r comes from a browser on this machine
// (localViewer), for a route outside this package that shows a control only
// to such a viewer.
func LocalViewer(r *http.Request) bool { return localViewer(r) }

// loopbackPeer reports whether a request's remote address is a loopback IP.
func loopbackPeer(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// LANAccess is what Settings shows a local viewer in LAN mode: the
// addresses another device opens, each carrying the token, and the token
// file whose deletion issues a new token.
type LANAccess struct {
	URLs      []string
	TokenFile string
}

// lanAccessFor returns the LAN access block for r, or nil unless LAN mode is
// on and r is a local viewer: the token is shown to this machine only.
func (d *Daemon) lanAccessFor(r *http.Request) *LANAccess {
	if !d.lan || !localViewer(r) {
		return nil
	}
	token, err := d.lanToken()
	if err != nil {
		return nil
	}
	port := strconv.Itoa(d.endpoint.Port)
	var urls []string
	for _, ip := range lanAddresses() {
		urls = append(urls, "http://"+net.JoinHostPort(ip, port)+"/?"+lanTokenParam+"="+token)
	}
	if len(urls) == 0 {
		urls = []string{"http://<this machine's address>:" + port + "/?" + lanTokenParam + "=" + token}
	}
	return &LANAccess{URLs: urls, TokenFile: filepath.Join(d.dir, LANTokenFileName)}
}

// lanAddresses lists this machine's IPv4 addresses another device on the
// network can reach: neither loopback nor link-local.
func lanAddresses() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []string
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipnet.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			continue
		}
		out = append(out, ip.String())
	}
	return out
}
