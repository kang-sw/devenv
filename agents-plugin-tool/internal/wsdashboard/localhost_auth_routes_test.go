package wsdashboard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsholder"
)

// No fixture transport is used here: these callers must not acquire a cookie
// merely because an unrelated regression fixture registered their origin.
func rawAuthClient(t *testing.T) *http.Client {
	t.Helper()
	tr := &http.Transport{Proxy: nil}
	t.Cleanup(tr.CloseIdleConnections)
	return &http.Client{Transport: tr, Timeout: 5 * time.Second}
}

func TestLocalhostAuthMachineRoutesWithoutCookie(t *testing.T) {
	f := startPageFixture(t)
	client := rawAuthClient(t)
	nudge, err := json.Marshal(NudgeBody{ReplyID: "livesession", Project: f.project})
	if err != nil {
		t.Fatal(err)
	}
	for _, bearer := range []string{"", "Bearer " + f.d.Endpoint().Token} {
		for _, tc := range []struct {
			method, path, body string
			want               int
		}{
			{"GET", "/health", "", 200},
			{"POST", "/nudge", string(nudge), 204},
			{"POST", "/nudge", `{"reply_id":"unknown"}`, 404},
			{"GET", "/session-hire/roster?requester=livesession", "", 200},
		} {
			req, err := http.NewRequest(tc.method, f.base+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			if bearer != "" {
				req.Header.Set("Authorization", bearer)
			}
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != tc.want {
				t.Fatalf("%s %s bearer=%q: %d %s", tc.method, tc.path, bearer, res.StatusCode, raw)
			}
		}
	}
	// Shutdown retains the endpoint bearer check, not the browser cookie.
	d := startTestDaemon(t, t.TempDir(), "1.0.0")
	for _, token := range []string{"", "wrong", d.Endpoint().Token} {
		req, _ := http.NewRequest("POST", endpointURL(d.Endpoint(), "/shutdown"), nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		want := 401
		if token == d.Endpoint().Token {
			want = 202
		}
		if res.StatusCode != want {
			t.Fatalf("shutdown token %q = %d want %d", token, res.StatusCode, want)
		}
	}
	waitDone(t, d)
}

func TestLocalhostAuthMachineExemptionsStayLocal(t *testing.T) {
	d := authFixture(t)
	d.lan = true
	token, err := d.lanToken()
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct{ method, path string }{
		{"GET", "/health"}, {"HEAD", "/health"}, {"POST", "/nudge"},
		{"GET", "/session-hire/roster"}, {"GET", "/pwa/manifest.webmanifest"},
		{"HEAD", "/pwa/icon-192.png"},
	} {
		for _, caller := range []struct {
			name, peer, host, origin, site, accept string
			wantPass                               bool
		}{
			{"LAN machine", "192.168.7.30:50000", "192.168.7.20:43210", "", "", "", false},
			{"LAN browser", "192.168.7.30:50000", "192.168.7.20:43210", "http://192.168.7.20:43210", "same-origin", "", false},
			{"rebound Origin", "127.0.0.1:50000", "evil.example:43210", "http://evil.example:43210", "same-origin", "", false},
			{"rebound fetch without Origin", "127.0.0.1:50000", "evil.example:43210", "", "", "*/*", false},
			{"rebound legacy navigation", "127.0.0.1:50000", "evil.example:43210", "", "", "text/html", false},
			{"loopback machine", "127.0.0.1:50000", "127.0.0.1:43210", "", "", "", true},
			{"IPv6 machine", "[::1]:50000", "localhost:43210", "", "", "", true},
			{"legacy machine Host", "127.0.0.1:50000", "machine.example:43210", "", "", "", false},
			{"local browser", "127.0.0.1:50000", "127.0.0.1:43210", "http://127.0.0.1:43210", "same-origin", "", true},
		} {
			for _, withToken := range []bool{false, true} {
				r := authRequest(route.method, route.path, "")
				r.RemoteAddr, r.Host = caller.peer, caller.host
				for key, value := range map[string]string{"Origin": caller.origin, "Sec-Fetch-Site": caller.site, "Accept": caller.accept} {
					if value != "" {
						r.Header.Set(key, value)
					}
				}
				if withToken {
					r.AddCookie(&http.Cookie{Name: lanTokenCookie, Value: token})
				}
				// A sentinel isolates the outer guard from each route's own checks.
				w := httptest.NewRecorder()
				d.accessGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
				want := http.StatusUnauthorized
				if caller.wantPass || withToken {
					want = http.StatusNoContent
				}
				if w.Code != want {
					t.Errorf("%s %s %s LAN token=%v: %d %s, want %d", route.method, route.path, caller.name, withToken, w.Code, w.Body.String(), want)
				}
			}
		}
	}
}

// The real worker capability handler is covered by the unchanged pi_forward
// tests. This probe pins the new outer mux boundary: it must forward the
// bearer untouched to that handler, without accepting it on browser routes.
type authAgentRouteProbe struct{}

func (*authAgentRouteProbe) Start(func(string, string)) {}
func (*authAgentRouteProbe) Close()                     {}
func (*authAgentRouteProbe) InboxChanged(string)        {}
func (*authAgentRouteProbe) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/agents/by-token/") && r.Header.Get("Authorization") != "Bearer worker-capability" {
			http.Error(w, "capability required", 401)
			return
		}
		w.WriteHeader(204)
	})
}

func TestLocalhostAuthWorkerCapabilityAndBrowserAgentBoundary(t *testing.T) {
	d := authFixture(t)
	d.agents = &authAgentRouteProbe{}
	d.support = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	for _, path := range []string{"/agents/by-token/list", "/agents", "/agents/conversation/send", "/support/stores/store/send"} {
		for _, cookie := range []bool{false, true} {
			r := authRequest("POST", path, `{}`)
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer worker-capability")
			if cookie {
				r.AddCookie(authCookie(t, d))
			}
			w := authServe(d, r)
			want := 401
			if cookie || strings.HasPrefix(path, "/agents/by-token/") {
				want = 204
			}
			if w.Code != want {
				t.Fatalf("%s cookie=%v: %d want %d", path, cookie, w.Code, want)
			}
		}
	}
	r := authRequest("POST", "/agents/by-token/list", `{}`)
	if w := authServe(d, r); w.Code != 401 || !strings.Contains(w.Body.String(), "capability required") {
		t.Fatalf("lost capability check: %d %s", w.Code, w.Body.String())
	}
}

func TestLocalhostAuthWebSocketNeverExchangesNavigationCode(t *testing.T) {
	d := authFixture(t)
	link, code := authLink(t, d, "/events")
	r := authRequest("GET", link, "")
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Connection", "Upgrade")
	r.Header.Set("Accept", "text/html")
	r.Header.Set("Sec-Fetch-Mode", "navigate")
	if w := authServe(d, r); w.Code != 401 || !strings.Contains(w.Body.String(), "login_required") || len(w.Result().Cookies()) != 0 {
		t.Fatalf("upgrade exchanged code or rendered HTML: %d %s", w.Code, w.Body.String())
	}
	if ok, _ := d.consumeLoginCode(code); !ok {
		t.Fatal("WebSocket handshake consumed a navigation code")
	}
}

// authTerminalHost supplies a synthetic, attached terminal for upgrade tests.
// No holder process, shell, harness or network peer is launched.
type authTerminalHost struct {
	*wsholder.Manager
	listing wsholder.Listing
}

func (h *authTerminalHost) List() wsholder.Listing { return h.listing }

func rawAuthHandshake(t *testing.T, base, path, cookie string) int {
	t.Helper()
	conn, err := net.DialTimeout("tcp", strings.TrimPrefix(base, "http://"), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	headers := ""
	if cookie != "" {
		headers = "Cookie: " + cookie + "\r\n"
	}
	_, err = fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nOrigin: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n%s\r\n", path, strings.TrimPrefix(base, "http://"), base, headers)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func TestLocalhostAuthRealStreamUpgradeBoundaries(t *testing.T) {
	meta, _ := json.Marshal(terminalSession{Kind: terminalKind, ReplyID: "livesession", Cwd: t.TempDir()})
	host := &authTerminalHost{Manager: wsholder.New(wsholder.Options{Dir: filepath.Join(t.TempDir(), wsholder.DirName)}),
		listing: wsholder.Listing{Holders: []wsholder.Info{{ID: "hauthprobe", Attached: true, Session: meta}}}}
	f := startPageFixture(t, withHolders(host))
	client := rawAuthClient(t)
	for _, path := range []string{"/events", "/s/livesession/events", "/s/livesession/terminals/hauthprobe/ws"} {
		for _, cookie := range []string{"", localhostCookieName + "=wrong", authCookie(t, f.d).String()} {
			want := 401
			if strings.Contains(cookie, authCookie(t, f.d).Value) {
				want = 101
			}
			if got := rawAuthHandshake(t, f.base, path, cookie); got != want {
				t.Fatalf("handshake %s cookie=%q: %d want %d", path, cookie, got, want)
			}
		}
	}
	for _, path := range []string{"/events", "/s/livesession/events"} {
		for _, authenticated := range []bool{false, true} {
			r, _ := http.NewRequest("GET", f.base+path, nil)
			if authenticated {
				r.AddCookie(authCookie(t, f.d))
			}
			res, err := client.Do(r)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			want := 401
			if authenticated {
				want = 200
			}
			if res.StatusCode != want {
				t.Fatalf("SSE %s authenticated=%v: %d want %d", path, authenticated, res.StatusCode, want)
			}
			if authenticated && res.Header.Get("Content-Type") != "text/event-stream" {
				t.Fatalf("SSE content type: %v", res.Header)
			}
		}
	}
}
