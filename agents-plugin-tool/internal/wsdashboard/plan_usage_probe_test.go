package wsdashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func setPiUsageAuth(t *testing.T, s *planUsageStore, expires int64) {
	t.Helper()
	usageWrite(t, filepath.Join(s.home, ".pi", "agent", "auth.json"), fmt.Sprintf(`{"openai-codex":{"type":"oauth","access":"fake-secret-access","accountId":"fake-account","expires":%d}}`, expires))
}

func TestPlanUsageFallbackOrderAndCadence(t *testing.T) {
	for _, mode := range []string{"chatgpt", "apikey", "missing", "codex-fails", "codex-empty", "legacy-mixed"} {
		t.Run(mode, func(t *testing.T) {
			s := usageTestStore(t)
			setPiUsageAuth(t, s, time.Now().Add(time.Hour).UnixMilli())
			if mode != "missing" {
				authMode := mode
				if strings.HasPrefix(mode, "codex-") {
					authMode = "chatgpt"
				}
				body := fmt.Sprintf(`{"auth_mode":%q,"tokens":{"access_token":"fake-codex-access"}}`, authMode)
				if mode == "codex-empty" {
					body = `{"auth_mode":"chatgpt","tokens":{}}`
				}
				if mode == "legacy-mixed" {
					body = `{"OPENAI_API_KEY":"fake-key","tokens":{"access_token":"fake-access"}}`
				}
				setCodexUsageAuth(t, s, body)
			}
			var calls atomic.Int32
			s.client.Transport = usageRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				if mode == "codex-fails" {
					if r.Header.Get("Authorization") != "Bearer fake-codex-access" {
						t.Error("Codex selection fell through to Pi")
					}
					return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("fake-secret-access"))}, nil
				}
				body := weeklyUsageBody
				if mode == "apikey" || mode == "missing" {
					if r.Header.Get("Authorization") != "Bearer fake-secret-access" || r.Header.Get("ChatGPT-Account-Id") != "fake-account" {
						t.Error("wrong Pi credential")
					}
					body = strings.ReplaceAll(weeklyUsageBody, "41", "23")
				} else if r.Header.Get("Authorization") != "Bearer fake-codex-access" {
					t.Error("wrong Codex credential")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			s.query(context.Background(), false)
			entries := s.entries()
			if len(entries) != 1 {
				t.Fatal("GPT not detected")
			}
			e := entries[0]
			wantCalls := int32(1)
			if mode == "codex-empty" || mode == "legacy-mixed" {
				wantCalls = 0
			}
			if calls.Load() != wantCalls {
				t.Fatal("wrong request count")
			}
			switch mode {
			case "chatgpt":
				if e.Source != "via Codex CLI" || len(e.Windows) != 1 || e.Windows[0].Percent != 41 {
					t.Fatal("Codex precedence failed")
				}
			case "apikey", "missing":
				if e.Source != "via Pi" || len(e.Windows) != 1 || e.Windows[0].Percent != 23 {
					t.Fatal("Pi fallback failed")
				}
			default:
				if e.Source != "via Codex CLI" || e.Status != "usage unavailable" || len(e.Windows) != 0 {
					t.Fatal("selected Codex failure fell back")
				}
			}
			var wg sync.WaitGroup
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); s.query(context.Background(), true) }()
			}
			wg.Wait()
			if calls.Load() != wantCalls {
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
	s := usageTestStore(t)
	setCodexUsageAuth(t, s, `{"auth_mode":"chatgpt","tokens":{"access_token":"fake-access"}}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	s.usageURL = server.URL
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := s.queryCodex(ctx); err != errPlanUsage || time.Since(start) > 3*time.Second {
		t.Fatal("codex HTTP query was not bounded")
	}
}

func TestPlanUsageCancelledViewerStoresSuccess(t *testing.T) {
	for _, codex := range []bool{false, true} {
		t.Run(fmt.Sprint(codex), func(t *testing.T) {
			s := usageTestStore(t)
			if codex {
				setCodexUsageAuth(t, s, `{"auth_mode":"chatgpt","tokens":{"access_token":"fake-access"}}`)
			} else {
				setPiUsageAuth(t, s, time.Now().Add(time.Hour).UnixMilli())
			}
			finished := make(chan time.Time, 1)
			s.client.Transport = usageRoundTrip(func(r *http.Request) (*http.Response, error) {
				deadline, ok := r.Context().Deadline()
				if r.Context().Err() != nil || !ok || time.Until(deadline) > 10*time.Second || time.Until(deadline) < 9*time.Second {
					t.Error("viewer query did not retain detached ten-second budget")
				}
				finished <- time.Now()
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(weeklyUsageBody))}, nil
			})
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			d := &Daemon{planUsage: s}
			w := httptest.NewRecorder()
			d.handlePlanUsage(w, httptest.NewRequest("GET", "/rail/usage", nil).WithContext(ctx))
			if w.Code != http.StatusOK || s.gpt.Status != "" || len(s.gpt.Windows) != 1 || s.gpt.Windows[0].Percent != 41 {
				t.Fatal("cancelled viewer poisoned usage")
			}
			if s.lastQuery.Before(<-finished) {
				t.Fatal("query cadence starts before completion")
			}
			restored := newPlanUsageStore(s.dir, s.home, s.getenv)
			if restored.gpt.Status != "" || len(restored.gpt.Windows) != 1 || restored.gpt.Windows[0].Percent != 41 {
				t.Fatal("successful result not persisted")
			}
		})
	}
}

func TestPlanUsageCodexCompletionCadence(t *testing.T) {
	s := usageTestStore(t)
	setCodexUsageAuth(t, s, `{"auth_mode":"chatgpt","tokens":{"access_token":"fake-access"}}`)
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	s.client.Transport = usageRoundTrip(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(weeklyUsageBody))}, nil
	})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); s.query(context.Background(), false) }()
	<-started
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.query(context.Background(), true) }()
	}
	completedAfter := time.Now()
	close(release)
	wg.Wait()
	if calls.Load() != 1 || s.lastQuery.Before(completedAfter) {
		t.Fatal("concurrent viewers were not completion-coalesced")
	}
	for _, tc := range []struct {
		age   time.Duration
		force bool
		want  int32
	}{
		{4 * time.Second, true, 1}, {6 * time.Second, false, 1}, {6 * time.Second, true, 2},
		{4 * time.Minute, false, 2}, {6 * time.Minute, false, 3},
	} {
		s.lastQuery = time.Now().Add(-tc.age)
		s.query(context.Background(), tc.force)
		if calls.Load() != tc.want {
			t.Fatal("ordinary/forced cadence changed")
		}
	}
}

func TestPlanUsageRefreshSameOrigin(t *testing.T) {
	for _, tc := range []struct {
		name, site, origin, refreshHeader string
		wantCalls                         int32
	}{
		{"foreign", "cross-site", "http://foreign.example", "", 0},
		{"same-site", "same-site", "http://other.example", "", 0},
		{"missing", "", "", "", 0}, {"opaque", "", "null", "", 0},
		{"fetch-same-origin", "same-origin", "", "", 1},
		{"matching-origin", "", "http://dashboard.example", "", 1},
		{"refresh-header", "", "", "1", 1},
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
			s.lastQuery = time.Now().Add(-time.Minute)
			previous := s.lastQuery
			d := &Daemon{planUsage: s}
			r := httptest.NewRequest("GET", "http://dashboard.example/rail/usage?refresh=1", nil)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("X-WS-Usage-Refresh", tc.refreshHeader)
			w := httptest.NewRecorder()
			d.handlePlanUsage(w, r)
			if w.Code != http.StatusOK || calls.Load() != tc.wantCalls {
				t.Fatalf("refresh calls = %d, want %d; status %d", calls.Load(), tc.wantCalls, w.Code)
			}
			if tc.wantCalls == 0 {
				if !s.lastQuery.Equal(previous) || !strings.Contains(w.Body.String(), `"percent":37`) {
					t.Fatal("untrusted refresh did not serve cached result")
				}
			} else if !strings.Contains(w.Body.String(), `"percent":23`) {
				t.Fatal("same-origin refresh did not serve new result")
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
