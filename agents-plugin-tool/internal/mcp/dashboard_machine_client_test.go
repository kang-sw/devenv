package mcp

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// dashboardCookieForTest uses the production endpoint-bearer route, never an
// auth bypass, so browser-route tests retain their existing Origin checks.
func dashboardCookieForTest(t *testing.T, daemon *wsdashboard.Daemon) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/auth/cookie", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Authorization", "Bearer "+daemon.Endpoint().Token)
	out := httptest.NewRecorder()
	daemon.Handler().ServeHTTP(out, req)
	var cookie struct{ Name, Value string }
	if err := json.Unmarshal(out.Body.Bytes(), &cookie); err != nil || out.Code != http.StatusOK || cookie.Name == "" || cookie.Value == "" {
		t.Fatalf("browser credential: status=%d body=%s err=%v", out.Code, out.Body, err)
	}
	return &http.Cookie{Name: cookie.Name, Value: cookie.Value}
}

func machineClientEndpoint(t *testing.T, server *httptest.Server) wsdashboard.Endpoint {
	t.Helper()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	return wsdashboard.Endpoint{SchemaVersion: 1, Port: n, Token: "endpoint-token"}
}

func TestDashboardSendNudgeEndpointBearer(t *testing.T) {
	body := wsdashboard.NudgeBody{ReplyID: "reply", Project: "/project"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/nudge" {
			t.Errorf("nudge request = %s %s", r.Method, r.URL)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer endpoint-token" {
			t.Errorf("bearer = %q", got)
		}
		if r.Header.Get("Origin") != "" || r.Header.Get("Cookie") != "" {
			t.Error("machine request carried browser credentials")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content type = %q", got)
		}
		var got wsdashboard.NudgeBody
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil || got != body {
			t.Errorf("body = %+v, err = %v", got, err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if err := wsdashboard.SendNudge(context.Background(), server.Client(), machineClientEndpoint(t, server), body); err != nil {
		t.Fatal(err)
	}
}

func TestSessionRosterEndpointBearer(t *testing.T) {
	const replyID = "reply +/id"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/session-hire/roster" || r.URL.Query().Get("requester") != replyID {
			t.Errorf("roster request = %s %s", r.Method, r.URL)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer endpoint-token" {
			t.Errorf("bearer = %q", got)
		}
		if r.Header.Get("Origin") != "" || r.Header.Get("Cookie") != "" {
			t.Error("machine request carried browser credentials")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"mailbox":"id:child","harness":"pi","label":"child","state":"idle"}]`))
	}))
	defer server.Close()
	dir := t.TempDir()
	raw, err := json.Marshal(machineClientEndpoint(t, server))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "endpoint.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewServerWithOptions(ServerOptions{Root: t.TempDir(), Env: []string{}})
	t.Cleanup(s.Close)
	s.dashboard.dir = func() (string, error) { return dir, nil }
	rows, err := s.readSessionRoster(replyID)
	if err != nil || len(rows) != 1 || rows[0].Mailbox != "id:child" || rows[0].Harness != "pi" {
		t.Fatalf("roster = %+v, err = %v", rows, err)
	}
}
