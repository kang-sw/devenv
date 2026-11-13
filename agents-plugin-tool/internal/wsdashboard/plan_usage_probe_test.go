package wsdashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPlanUsageWindowClassification(t *testing.T) {
	for _, body := range []string{
		`{"primary":{"usedPercent":31,"windowDurationMins":10080,"resetsAt":2100000000},"secondary":{"usedPercent":82,"windowDurationMins":300}}`,
		`{"secondary":{"usedPercent":31,"windowDurationMins":10080,"resetsAt":"2036-07-18T13:20:00Z"},"primary":{"usedPercent":82,"windowDurationMins":300}}`,
	} {
		var limits codexUsageLimits
		if err := json.Unmarshal([]byte(body), &limits); err != nil {
			t.Fatal(err)
		}
		e, err := codexUsageEntry(limits, "fake")
		if err != nil || len(e.Windows) != 1 || e.Windows[0].Percent != 31 || e.Windows[0].ResetsAt != 2100000000000 {
			t.Fatalf("weekly classification failed: %+v %v", e, err)
		}
	}
	for _, body := range []string{`{}`, `{"primary":{"usedPercent":4,"windowDurationMins":300}}`, `{"primary":{"windowDurationMins":10080}}`, `{"primary":{"usedPercent":101,"windowDurationMins":10080}}`} {
		var limits codexUsageLimits
		_ = json.Unmarshal([]byte(body), &limits)
		if _, err := codexUsageEntry(limits, "fake"); err == nil {
			t.Error("invalid weekly window accepted")
		}
	}
}

// A copied test executable is the fake app-server on every platform. It
// validates the protocol before answering; no shell or owner login is used.
func init() {
	if os.Getenv("WS_TEST_USAGE_CODEX") != "1" || len(os.Args) != 2 || os.Args[1] != "app-server" {
		return
	}
	dec := json.NewDecoder(os.Stdin)
	read := func(method string, id int) {
		var request struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			ID      *int   `json:"id"`
			Params  struct {
				Client struct {
					Name    string `json:"name"`
					Version string `json:"version"`
				} `json:"clientInfo"`
			} `json:"params"`
		}
		if dec.Decode(&request) != nil || request.JSONRPC != "2.0" || request.Method != method ||
			(id == 0 && request.ID != nil) || (id != 0 && (request.ID == nil || *request.ID != id)) {
			os.Exit(23)
		}
		if method == "initialize" && (request.Params.Client.Name != "ws-dashboard" || request.Params.Client.Version != "1") {
			os.Exit(23)
		}
	}
	read("initialize", 1)
	fmt.Println(`{"id":1,"result":{}}`)
	read("initialized", 0)
	read("account/rateLimits/read", 2)
	fmt.Println(os.Getenv("WS_TEST_USAGE_CODEX_REPLY"))
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

func fakeUsageCodex(t *testing.T, s *planUsageStore, reply string) {
	t.Helper()
	t.Setenv("WS_TEST_USAGE_CODEX", "1")
	t.Setenv("WS_TEST_USAGE_CODEX_REPLY", reply)
	original, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "fake codex with spaces")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "codex")
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	if err := os.WriteFile(path, binary, 0700); err != nil {
		t.Fatal(err)
	}
	s.codexBinary = path
	if runtime.GOOS == "windows" {
		shim := filepath.Join(dir, "codex.cmd")
		if err := os.WriteFile(shim, []byte("@echo off\r\n\""+path+"\" %*\r\n"), 0700); err != nil {
			t.Fatal(err)
		}
		s.codexBinary = shim
	}
}

func TestPlanUsageFakeRejectsInvalidProtocol(t *testing.T) {
	for _, input := range []string{
		`{"jsonrpc":"2.0","id":9,"method":"initialize"}`,
		`{"jsonrpc":"2.0","id":1,"method":"account/rateLimits/read"}`,
		"{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"clientInfo\":{\"name\":\"ws-dashboard\",\"version\":\"1\"}}}\n{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"account/rateLimits/read\"}",
	} {
		s := usageTestStore(t)
		fakeUsageCodex(t, s, `{"id":2,"result":{}}`)
		path := s.codexBinary
		if runtime.GOOS == "windows" {
			path = strings.TrimSuffix(path, ".cmd") + ".exe"
		}
		cmd := exec.Command(path, "app-server")
		cmd.Stdin = strings.NewReader(input)
		if err := cmd.Run(); err == nil || cmd.ProcessState.ExitCode() != 23 {
			t.Fatal("fake accepted invalid request sequence")
		}
	}
}

func TestPlanUsageCodexBatchShim(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows batch invocation")
	}
	s := usageTestStore(t)
	fakeUsageCodex(t, s, `{"id":2,"result":{"rateLimits":{"primary":{"usedPercent":7,"windowDurationMins":10080}}}}`)
	batch := strings.TrimSuffix(s.codexBinary, ".cmd") + ".bat"
	if err := os.Rename(s.codexBinary, batch); err != nil {
		t.Fatal(err)
	}
	s.codexBinary = batch
	if _, err := s.queryCodex(context.Background()); err != nil {
		t.Fatal("batch app-server query failed")
	}
}

func setPiUsageAuth(t *testing.T, s *planUsageStore, expires int64) {
	usageWrite(t, filepath.Join(s.home, ".pi", "agent", "auth.json"), fmt.Sprintf(`{"openai-codex":{"type":"oauth","access":"fake-secret-access","accountId":"fake-account","expires":%d}}`, expires))
}

func TestPlanUsageFallbackOrderAndCadence(t *testing.T) {
	for _, mode := range []string{"chatgpt", "apikey", "missing", "cli-fails"} {
		t.Run(mode, func(t *testing.T) {
			s := usageTestStore(t)
			setPiUsageAuth(t, s, time.Now().Add(time.Hour).UnixMilli())
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("Authorization") != "Bearer fake-secret-access" || r.Header.Get("ChatGPT-Account-Id") != "fake-account" || r.Method != "GET" || r.ContentLength > 0 || r.URL.RawQuery != "" {
					t.Error("incorrect usage request")
				}
				_, _ = fmt.Fprint(w, `{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":23,"limit_window_seconds":604800,"reset_at":2100000000}}}`)
			}))
			defer server.Close()
			s.usageURL = server.URL
			if mode != "missing" {
				authMode := mode
				if mode == "cli-fails" {
					authMode = "chatgpt"
				}
				usageWrite(t, filepath.Join(s.home, ".codex", "auth.json"), fmt.Sprintf(`{"auth_mode":%q,"tokens":{}}`, authMode))
			}
			reply := `{"id":2,"result":{"rateLimits":{"planType":"pro","secondary":{"usedPercent":41,"windowDurationMins":10080,"resetsAt":2100000000}}}}`
			if mode == "cli-fails" {
				reply = `{"id":2,"error":{"message":"fake-secret-access"}}`
			}
			fakeUsageCodex(t, s, reply)
			s.query(context.Background(), false)
			entries := s.entries()
			if len(entries) != 1 {
				t.Fatal("GPT not detected")
			}
			e := entries[0]
			if mode == "chatgpt" {
				if calls.Load() != 0 || e.Source != "via Codex CLI" || len(e.Windows) != 1 || e.Windows[0].Percent != 41 {
					t.Fatalf("CLI precedence failed: %+v", e)
				}
			} else if mode == "cli-fails" {
				if calls.Load() != 0 || e.Status != "usage unavailable" {
					t.Fatal("CLI failure fell back or had wrong status")
				}
			} else {
				if calls.Load() != 1 || e.Source != "via Pi" || e.Windows[0].Percent != 23 {
					t.Fatalf("Pi fallback failed: %+v", e)
				}
			}
			var wg sync.WaitGroup
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); s.query(context.Background(), true) }()
			}
			wg.Wait()
			if calls.Load() > 1 {
				t.Fatal("viewers did not share cached request")
			}
			raw, _ := os.ReadFile(filepath.Join(s.dir, planUsageCache))
			if strings.Contains(string(raw), "fake-secret-access") || strings.Contains(string(raw), "fake-account") {
				t.Fatal("credential leaked to cache")
			}
		})
	}
}

func TestPlanUsagePiExpiryFailureRetentionAndNoLeaks(t *testing.T) {
	s := usageTestStore(t)
	setPiUsageAuth(t, s, time.Now().Add(-time.Minute).UnixMilli())
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Error(w, "fake-secret-access", 500) }))
	defer server.Close()
	s.usageURL = server.URL
	s.gpt = planUsageEntry{Harness: "codex", Source: "via Pi", AsOf: 123, Windows: []planUsageWindow{{Kind: "seven_day", Percent: 37, ResetsAt: 2100000000000, AsOf: 123}}}
	s.query(context.Background(), false)
	if calls.Load() != 0 || s.entries()[0].Status != "Pi token expired - open Pi to refresh" {
		t.Fatal("expired Pi made a request or wrong status")
	}
	setPiUsageAuth(t, s, time.Now().Add(time.Hour).UnixMilli())
	s.lastQuery = time.Time{}
	s.query(context.Background(), false)
	e := s.entries()[0]
	if calls.Load() != 1 || e.Status != "usage unavailable" || e.AsOf != 123 || e.Windows[0].Percent != 37 {
		t.Fatal("failed query lost last value")
	}
	_, err := s.queryPi(context.Background())
	if err == nil || strings.Contains(err.Error(), "fake-secret-access") {
		t.Fatal("upstream error leaked")
	}
	raw, _ := json.Marshal(e)
	if strings.Contains(string(raw), "fake-secret-access") {
		t.Fatal("viewer error leaked")
	}
}

func TestPlanUsagePiRedirectDoesNotForwardToken(t *testing.T) {
	s := usageTestStore(t)
	setPiUsageAuth(t, s, time.Now().Add(time.Hour).UnixMilli())
	var calls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer origin.Close()
	s.usageURL = origin.URL
	if _, err := s.queryPi(context.Background()); err == nil || calls.Load() != 0 {
		t.Fatal("usage request followed redirect")
	}
}

func TestPlanUsageCodexMalformedAndTimeout(t *testing.T) {
	for _, reply := range []string{`garbage`, `{"id":2,"result":{}}`, `{"id":2,"error":{"message":"fake-secret-access"}}`} {
		s := usageTestStore(t)
		fakeUsageCodex(t, s, reply)
		_, err := s.queryCodex(context.Background())
		if err == nil || err.Error() != "usage unavailable" {
			t.Fatal("malformed/error reply not sanitized")
		}
	}
	s := usageTestStore(t)
	fakeUsageCodex(t, s, `{"method":"ignored"}`)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := s.queryCodex(ctx)
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatal("codex query was not bounded")
	}
}

func TestPlanUsageCancelledViewerStoresSuccess(t *testing.T) {
	s := usageTestStore(t)
	setPiUsageAuth(t, s, time.Now().Add(time.Hour).UnixMilli())
	finished := make(chan time.Time, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finished <- time.Now()
		_, _ = fmt.Fprint(w, `{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":23,"limit_window_seconds":604800,"reset_at":2100000000}}}`)
	}))
	defer server.Close()
	s.usageURL = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d := &Daemon{planUsage: s}
	w := httptest.NewRecorder()
	d.handlePlanUsage(w, httptest.NewRequest("GET", "/rail/usage", nil).WithContext(ctx))
	if w.Code != http.StatusOK || s.gpt.Status != "" || len(s.gpt.Windows) != 1 || s.gpt.Windows[0].Percent != 23 {
		t.Fatalf("cancelled viewer poisoned usage: %+v, response %s", s.gpt, w.Body.String())
	}
	if s.lastQuery.Before(<-finished) {
		t.Fatal("query cadence starts before the query finishes")
	}
	restored := newPlanUsageStore(s.dir, s.home, s.getenv)
	if restored.gpt.Status != "" || len(restored.gpt.Windows) != 1 || restored.gpt.Windows[0].Percent != 23 {
		t.Fatalf("successful result was not persisted: %+v", restored.gpt)
	}
}

func TestPlanUsageRefreshSameOrigin(t *testing.T) {
	for _, tc := range []struct {
		name, site, origin string
		wantCalls          int32
	}{
		{"foreign", "cross-site", "http://foreign.example", 0},
		{"same-site", "same-site", "http://other.example", 0},
		{"missing", "", "", 0},
		{"opaque", "", "null", 0},
		{"fetch-same-origin", "same-origin", "", 1},
		{"matching-origin", "", "http://dashboard.example", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := usageTestStore(t)
			setPiUsageAuth(t, s, time.Now().Add(time.Hour).UnixMilli())
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = fmt.Fprint(w, `{"rate_limit":{"primary_window":{"used_percent":23,"limit_window_seconds":604800,"reset_at":2100000000}}}`)
			}))
			defer server.Close()
			s.usageURL = server.URL
			s.gpt = planUsageEntry{Harness: "codex", Source: "via Pi", Windows: []planUsageWindow{{Kind: "seven_day", Percent: 37}}}
			// Eligible for a forced refresh, but not an ordinary cached poll.
			s.lastQuery = time.Now().Add(-time.Minute)
			previous := s.lastQuery
			d := &Daemon{planUsage: s}
			r := httptest.NewRequest("GET", "http://dashboard.example/rail/usage?refresh=1", nil)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			d.handlePlanUsage(w, r)
			if w.Code != http.StatusOK || calls.Load() != tc.wantCalls {
				t.Fatalf("refresh calls = %d, want %d; status %d", calls.Load(), tc.wantCalls, w.Code)
			}
			if tc.wantCalls == 0 {
				if !s.lastQuery.Equal(previous) || !strings.Contains(w.Body.String(), `"percent":37`) {
					t.Fatal("untrusted refresh did not serve the cached result")
				}
			} else if !strings.Contains(w.Body.String(), `"percent":23`) {
				t.Fatal("same-origin refresh did not serve the new result")
			}
		})
	}
}

func TestPlanUsageViewerRouteAndMethods(t *testing.T) {
	s := usageTestStore(t)
	d := &Daemon{planUsage: s}
	w := httptest.NewRecorder()
	d.handlePlanUsage(w, httptest.NewRequest("GET", "/rail/usage", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"entries":[]`) || !strings.Contains(w.Body.String(), `"warnPercent":80`) {
		t.Fatalf("route shape: %s", w.Body.String())
	}
	w = httptest.NewRecorder()
	d.handlePlanUsage(w, httptest.NewRequest("POST", "/rail/usage", nil))
	if w.Code != 405 {
		t.Fatal("mutating method accepted")
	}
}
