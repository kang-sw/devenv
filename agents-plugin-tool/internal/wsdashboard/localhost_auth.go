package wsdashboard

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	localhostSecretFile   = "localhost-secret"
	localhostCookieName   = "ws_dashboard_localhost"
	localhostCookieMaxAge = 400 * 24 * 60 * 60
	loginCodeLifetime     = 10 * time.Minute
	loginCodeCapacity     = 256
	loginFailureLimit     = 10
	loginFailureWindow    = time.Minute
)

type loginCode struct {
	value   string
	expires time.Time
}

// localhostAuth's zero value is gated. The clock is injectable, never an
// authentication bypass. Its mutex serializes mint, consumption and throttling.
type localhostAuth struct {
	mu        sync.Mutex
	now       func() time.Time
	codes     []loginCode // oldest first, at most loginCodeCapacity
	failures  int
	window    time.Time
	windowSet bool
	// Clipboard attempts have their own budget, independent of exchanges.
	clipboardLast     time.Time
	clipboardWindow   time.Time
	clipboardAttempts int
	clipboardStarted  bool
}

func (a *localhostAuth) clock() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

// localhostCookieValue re-reads the persisted secret so deletion rotates it
// immediately. Sharing tokenMu also serializes concurrent first-time minting.
func (d *Daemon) localhostCookieValue() (string, error) {
	d.tokenMu.Lock()
	defer d.tokenMu.Unlock()
	raw, err := os.ReadFile(filepath.Join(d.dir, localhostSecretFile))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	secret := strings.TrimSpace(string(raw))
	if secret == "" {
		secret, err = newToken()
		if err != nil {
			return "", err
		}
		if err := writePrivateFile(d.dir, localhostSecretFile, []byte(secret+"\n")); err != nil {
			return "", err
		}
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("ws-dashboard-localhost-cookie-v1"))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (d *Daemon) localhostAuthenticated(r *http.Request) bool {
	if !localViewer(r) {
		return false
	}
	c, err := r.Cookie(localhostCookieName)
	if err != nil {
		return false
	}
	value, err := d.localhostCookieValue()
	return err == nil && tokenMatches(c.Value, value)
}

func (d *Daemon) setLocalhostCookie(w http.ResponseWriter) error {
	value, err := d.localhostCookieValue()
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: localhostCookieName, Value: value, Path: "/",
		MaxAge: localhostCookieMaxAge, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	return nil
}

// accessExempt is deliberately method- and path-specific. Each machine or
// capability handler keeps its existing credential/origin checks.
func accessExempt(r *http.Request) bool {
	read := r.Method == http.MethodGet || r.Method == http.MethodHead
	switch r.URL.Path {
	case "/auth/status", "/auth/exchange", "/auth/link", "/auth/cookie", "/auth/clipboard":
		return true // each auth handler checks its own caller
	case "/health":
		return read
	case ProxyPath, "/session-hire/roster":
		return r.Method == http.MethodGet
	case "/shutdown", "/nudge":
		return r.Method == http.MethodPost
	}
	return (r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/agents/by-token/") &&
		strings.TrimPrefix(r.URL.Path, "/agents/by-token/") != "") ||
		(read && strings.HasPrefix(r.URL.Path, pwaPrefix)) || publicAsset(r) || previewCapability(r)
}

func loginNavigationRequest(r *http.Request) bool {
	if (r.Method != http.MethodGet && r.Method != http.MethodHead) || wsUpgrade(r) {
		return false
	}
	if mode := r.Header.Get("Sec-Fetch-Mode"); mode != "" {
		return mode == "navigate"
	}
	// Fetch metadata, when supplied, is authoritative over Accept.
	for key := range r.Header {
		if strings.HasPrefix(strings.ToLower(key), "sec-fetch-") {
			return false
		}
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html")
}

func loginRequired(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login_required"})
}

func (d *Daemon) refuseLogin(w http.ResponseWriter, r *http.Request, invalid bool) {
	w.Header().Set("Cache-Control", "no-store")
	if loginNavigationRequest(r) {
		d.serveLoginGate(w, r, invalid)
		return
	}
	loginRequired(w)
}

// validLoginPath rejects external and browser-normalized authority URLs too.
func validLoginPath(path string) bool {
	decoded, err := url.PathUnescape(path)
	if err != nil || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") ||
		strings.IndexFunc(decoded, unicode.IsControl) >= 0 {
		return false
	}
	u, err := url.Parse(path)
	return err == nil && !u.IsAbs() && u.Host == "" && u.Opaque == "" &&
		strings.HasPrefix(u.Path, "/") && !strings.HasPrefix(u.Path, "//") && !strings.Contains(u.Path, "\\")
}

// loginNext is shared with the self-contained gate. It retains path/query,
// but never echoes a code into the next navigation.
func loginNext(r *http.Request) string {
	u := *r.URL
	u.Scheme, u.Host, u.Fragment = "", "", ""
	q := u.Query()
	q.Del("login")
	u.RawQuery = q.Encode()
	next := u.String()
	if !validLoginPath(next) {
		return "/"
	}
	return next
}

// LoginLink mints a single-use, ten-minute browser link without HTTP. The
// secret never appears in a URL, and outstanding links are bounded in memory.
func (d *Daemon) LoginLink(path string) (string, error) {
	if !validLoginPath(path) {
		return "", errors.New("invalid login path")
	}
	code, err := newToken()
	if err != nil {
		return "", err
	}
	a := &d.auth
	a.mu.Lock()
	now := a.clock()
	a.prune(now)
	if len(a.codes) >= loginCodeCapacity {
		a.codes = a.codes[1:]
	}
	a.codes = append(a.codes, loginCode{value: code, expires: now.Add(loginCodeLifetime)})
	a.mu.Unlock()
	u, _ := url.Parse(path)
	u.Scheme, u.Host = "http", "127.0.0.1:"+strconv.Itoa(d.endpoint.Port)
	q := u.Query()
	q.Set("login", code)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (a *localhostAuth) prune(now time.Time) {
	kept := a.codes[:0]
	for _, code := range a.codes {
		if now.Before(code.expires) {
			kept = append(kept, code)
		}
	}
	a.codes = kept
}

// consume returns a retry interval when the daemon-wide failed-exchange
// budget is exhausted. A blocked attempt cannot consume a valid code.
func (d *Daemon) consumeLoginCode(code string) (bool, int) {
	a := &d.auth
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.clock()
	if !a.windowSet || !now.Before(a.window.Add(loginFailureWindow)) {
		a.window, a.failures, a.windowSet = now, 0, true
	}
	if a.failures >= loginFailureLimit {
		return false, max(1, int((a.window.Add(loginFailureWindow).Sub(now)+time.Second-1)/time.Second))
	}
	a.prune(now)
	for i, candidate := range a.codes {
		if tokenMatches(code, candidate.value) {
			a.codes = append(a.codes[:i], a.codes[i+1:]...)
			return true, 0
		}
	}
	a.failures++
	return false, 0
}

func authRateLimited(w http.ResponseWriter, retry int) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Retry-After", strconv.Itoa(retry))
	writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
}

// loginNavigation handles only HTML navigation; fetches and WebSocket
// handshakes never exchange credentials from a query parameter.
func (d *Daemon) loginNavigation(w http.ResponseWriter, r *http.Request) bool {
	if !loginNavigationRequest(r) || !r.URL.Query().Has("login") {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	ok, retry := d.consumeLoginCode(r.URL.Query().Get("login"))
	if !ok && d.localhostAuthenticated(r) {
		http.Redirect(w, r, loginNext(r), http.StatusSeeOther)
		return true
	}
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(retry))
		d.serveLoginGateState(w, r, false, true)
	} else if !ok {
		d.refuseLogin(w, r, true)
	} else if err := d.setLocalhostCookie(w); err != nil {
		http.Error(w, "credential unavailable", http.StatusServiceUnavailable)
	} else {
		http.Redirect(w, r, loginNext(r), http.StatusSeeOther)
	}
	return true
}

func (d *Daemon) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !d.localhostAuthenticated(r) {
		loginRequired(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

func decodeAuthBody(w http.ResponseWriter, r *http.Request, body any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := json.NewDecoder(r.Body).Decode(body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return false
	}
	return true
}

func (d *Daemon) handleAuthExchange(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !localViewer(r) || !sameOrigin(r) || crossSiteFetch(r) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}
	var body struct {
		Code string `json:"code"`
		Next string `json:"next"`
	}
	if !decodeAuthBody(w, r, &body) {
		return
	}
	if body.Next == "" {
		body.Next = "/"
	}
	if !validLoginPath(body.Next) {
		http.Error(w, "invalid next", http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(body.Code)
	// Pasted links supply the code only; next remains the gate's validated
	// destination, not a destination controlled by clipboard contents.
	if u, err := url.Parse(code); err == nil && u.Scheme == "http" && loopbackHost(u.Host) && u.Port() == strconv.Itoa(d.endpoint.Port) {
		code = u.Query().Get("login")
	}
	ok, retry := d.consumeLoginCode(code)
	if retry > 0 {
		authRateLimited(w, retry)
		return
	}
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_code"})
		return
	}
	if err := d.setLocalhostCookie(w); err != nil {
		http.Error(w, "credential unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"next": body.Next})
}

// machineAuth is the native cookie/link boundary: a local process with the
// endpoint credential, never a browser Origin (even an explicitly empty one).
func (d *Daemon) machineAuth(r *http.Request) bool {
	got, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return loopbackPeer(r.RemoteAddr) && len(r.Header.Values("Origin")) == 0 &&
		bearer && tokenMatches(got, d.endpoint.Token)
}

func (d *Daemon) handleAuthLink(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !d.machineAuth(r) {
		if !d.localhostAuthenticated(r) {
			loginRequired(w)
			return
		}
		if local, own := TrustedWrite(r); !local || !own || crossSiteFetch(r) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
	}
	var body struct {
		Path string `json:"path"`
	}
	if !decodeAuthBody(w, r, &body) {
		return
	}
	if body.Path == "" {
		body.Path = "/"
	}
	if !validLoginPath(body.Path) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	link, err := d.LoginLink(body.Path)
	if err != nil {
		http.Error(w, "login link unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": link})
}

func (d *Daemon) handleAuthCookie(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !d.machineAuth(r) {
		loginRequired(w)
		return
	}
	value, err := d.localhostCookieValue()
	if err != nil {
		http.Error(w, "credential unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": localhostCookieName, "value": value, "maxAge": localhostCookieMaxAge})
}
