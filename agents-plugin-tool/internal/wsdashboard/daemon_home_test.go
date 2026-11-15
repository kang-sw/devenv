package wsdashboard

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

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
	d.planUsage.codexBinary = filepath.Join(t.TempDir(), "nonexistent-codex")
	response := httptest.NewRecorder()
	d.handlePlanUsage(response, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/rail/usage", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"entries":[]`) || requests.Load() != 0 || !d.planUsage.lastQuery.IsZero() {
		t.Fatalf("explicit empty home queried account usage: status=%d calls=%d body=%s", response.Code, requests.Load(), response.Body.String())
	}
	if d.planUsage.home != home {
		t.Fatal("explicit home ignored")
	}
}
