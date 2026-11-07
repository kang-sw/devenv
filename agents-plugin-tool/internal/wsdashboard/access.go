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

// LAN access. With LAN mode off the daemon binds loopback only and every
// route passes the loopback Host guard (hostGuard). With it on the daemon
// binds every interface and admits a request when either holds:
//
//   - its peer address and its Host header are both loopback: a browser on
//     this machine, which keeps the token-free localhost URI of the pointer
//     line working, and requiring a loopback Host as well still refuses a
//     DNS-rebinding page;
//   - it carries the LAN access token.
//
// One route needs neither: GET and HEAD of the shared mermaid script
// (publicAsset), which the Now slot's sandboxed frame loads from an opaque
// origin that sends no cookie. That frame also inlines its images as data:
// URLs (ImageDataURL, up to 2 MiB); a larger image falls back to the image
// route, which a LAN viewer's frame cannot load.
//
// The LAN token is not the endpoint file's shutdown token and is never
// written to the endpoint file, so a LAN browser never holds the shutdown
// credential. It persists in its own owner-only file in the state directory,
// so a daemon replaced by an upgrade keeps the token a second device
// bookmarked; deleting the file issues a new one.
//
// The token travels as the "token" query parameter of the address the
// localhost index page shows. A request that passes by it gets the token back
// as an HttpOnly cookie, which the page's links, its SSE stream (EventSource
// sends cookies but cannot set headers), its images, and its message and
// cancel posts then carry on their own, so no rendered URL or region ever
// holds the token.

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

// lanGuard is LAN mode's access guard (see the file comment).
func (d *Daemon) lanGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if localViewer(r) || publicAsset(r) {
			next.ServeHTTP(w, r)
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
		http.Error(w, "this dashboard needs its LAN access token: on the machine it runs on, open its index page at http://localhost:"+strconv.Itoa(d.endpoint.Port)+"/ for the address that carries the token", http.StatusUnauthorized)
	})
}

// publicAsset reports a read of the shared mermaid script, the one route
// LAN mode serves without the token. A board's Now renders in a sandboxed
// frame whose opaque origin sends no cookie (wswhiteboard framedNow), and a
// mermaid block there loads this script itself. The script is the vendored
// library under a content-hash name, so serving it reveals nothing about any
// session.
func publicAsset(r *http.Request) bool {
	name, _ := wswhiteboard.MermaidAsset()
	return (r.Method == http.MethodGet || r.Method == http.MethodHead) && r.URL.Path == "/s/"+name
}

func tokenMatches(got, token string) bool {
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

// localViewer reports whether r comes from a browser on this machine: a
// loopback peer that addressed the daemon by a loopback name.
func localViewer(r *http.Request) bool {
	return loopbackPeer(r.RemoteAddr) && loopbackHost(r.Host)
}

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

// LANAccess is what the index page shows a local viewer in LAN mode: the
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
