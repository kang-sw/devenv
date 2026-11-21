package wsdashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDaemonScratchCapture(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	d, err := StartDaemon(DaemonOptions{Dir: t.TempDir(), ScratchCapture: true, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	if d.planUsage != nil {
		t.Fatal("scratch capture initialized the account usage store")
	}
	cookie, err := d.localhostCookieValue()
	if err != nil {
		t.Fatal(err)
	}

	get := func(path string, result any) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
		request.RemoteAddr = "127.0.0.1:1234"
		request.Header.Set("Sec-Fetch-Site", "same-origin")
		request.AddCookie(&http.Cookie{Name: localhostCookieName, Value: cookie})
		response := httptest.NewRecorder()
		d.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s: HTTP %d: %s", path, response.Code, response.Body.String())
		}
		if err := json.Unmarshal(response.Body.Bytes(), result); err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
	}
	var usage struct {
		Entries []planUsageEntry
		History planUsageHistoryProjection
	}
	get("/rail/usage?refresh=1&history=1", &usage)
	if usage.Entries == nil || len(usage.Entries) != 0 || usage.History.Status != "empty" {
		t.Fatalf("scratch usage = %+v", usage)
	}
	var roots browseRoots
	get("/s/host0/fs/roots", &roots)
	if roots.Host != "scratch" || roots.Home != home || roots.Volumes == nil || len(roots.Volumes) != 0 {
		t.Fatalf("scratch roots = %+v, want home %s and no volumes", roots, home)
	}
}

func TestDaemonExplicitHomeDoesNotQueryUsage(t *testing.T) {
	// Fake login-presence documents deliberately live outside the explicit
	// home. No real account files, provider executable, or endpoint are used.
	inherited := t.TempDir()
	for _, name := range []string{"HOME", "CLAUDE_CONFIG_DIR", "CODEX_HOME", "WS_CACHE_HOME", "WS_CONFIG_HOME"} {
		t.Setenv(name, inherited)
	}
	if err := os.WriteFile(filepath.Join(inherited, "auth.json"), []byte(`{"auth_mode":"chatgpt"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inherited, ".credentials.json"), []byte(`{"claudeAiOauth":{"subscriptionType":"fixture"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	d, err := StartDaemon(DaemonOptions{Dir: t.TempDir(), Home: home, Port: 0, bindHost: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	var requests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer upstream.Close()
	d.planUsage.usageURL = upstream.URL
	response := httptest.NewRecorder()
	d.handlePlanUsage(response, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/rail/usage", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"entries":[]`) || requests.Load() != 0 || !d.planUsage.lastQuery.IsZero() {
		t.Fatalf("explicit empty home queried account usage: status=%d calls=%d body=%s", response.Code, requests.Load(), response.Body.String())
	}
	if d.planUsage.home != home {
		t.Fatal("explicit home ignored")
	}
}
