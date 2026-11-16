package wsdashboard

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const weeklyUsageBody = `{"plan_type":"pro","rate_limit":{"secondary_window":{"used_percent":41,"limit_window_seconds":604800,"reset_at":2100000000}}}`

func setCodexUsageAuth(t *testing.T, s *planUsageStore, body string) {
	t.Helper()
	usageWrite(t, filepath.Join(s.configHome("CODEX_HOME", ".codex"), "auth.json"), body)
}

func usageJWT(claims string) string {
	return "e30." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".unsigned"
}

type usageRoundTrip func(*http.Request) (*http.Response, error)

func (f usageRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPlanUsageCodexCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, body, access, account string
		request                     bool
	}{
		{"explicit", `{"auth_mode":"chatgpt","tokens":{"access_token":"fake-codex-access","account_id":"fake-codex-account"}}`, "fake-codex-access", "fake-codex-account", true},
		{"explicit-key-ignored", `{"auth_mode":"chatgpt","OPENAI_API_KEY":"fake-api-key","tokens":{"access_token":"fake-codex-access"}}`, "fake-codex-access", "", true},
		{"legacy", `{"tokens":{"access_token":"fake-legacy-access"}}`, "fake-legacy-access", "", true},
		{"legacy-null-key", `{"OPENAI_API_KEY":null,"tokens":{"access_token":"fake-access"}}`, "fake-access", "", true},
		{"legacy-mixed", `{"OPENAI_API_KEY":"fake-api-key","tokens":{"access_token":"fake-codex-access"}}`, "", "", false},
		{"legacy-empty-key", `{"OPENAI_API_KEY":"","tokens":{"access_token":"fake-codex-access"}}`, "", "", false},
		{"explicit-apikey", `{"auth_mode":"apikey","tokens":{"access_token":"fake-codex-access"}}`, "", "", false},
		{"external", `{"auth_mode":"chatgptAuthTokens","tokens":{"access_token":"fake-access"}}`, "", "", false},
		{"malformed", `{"auth_mode":"chatgpt","tokens":`, "", "", false},
		{"trailing-json", `{"auth_mode":"chatgpt","tokens":{"access_token":"fake-access"}} {}`, "", "", false},
		{"missing", `{"auth_mode":"chatgpt"}`, "", "", false},
		{"empty", `{"auth_mode":"chatgpt","tokens":{"access_token":""}}`, "", "", false},
		{"whitespace", `{"auth_mode":"chatgpt","tokens":{"access_token":" \t"}}`, "", "", false},
		{"id-not-access", `{"auth_mode":"chatgpt","tokens":{"id_token":"fake-id","refresh_token":"fake-refresh"}}`, "", "", false},
		{"wrong-type", `{"auth_mode":"chatgpt","tokens":{"access_token":42}}`, "", "", false},
		{"token-header", `{"auth_mode":"chatgpt","tokens":{"access_token":"fake\r\nInjected: bad"}}`, "", "", false},
		{"account-header", `{"auth_mode":"chatgpt","tokens":{"access_token":"fake-access","account_id":"fake\u0000account"}}`, "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := usageTestStore(t)
			setCodexUsageAuth(t, s, tc.body)
			var calls int
			s.client.Transport = usageRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != "https://chatgpt.com/backend-api/wham/usage" || r.Method != http.MethodGet || r.Body != nil || r.URL.RawQuery != "" || r.Header.Get("Cookie") != "" {
					t.Error("usage request expanded beyond the fixed passive endpoint")
				}
				if r.Header.Get("Authorization") != "Bearer "+tc.access || r.Header.Get("ChatGPT-Account-Id") != tc.account || r.Header.Get("originator") != "codex_cli_rs" {
					t.Error("wrong selected OAuth headers")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(weeklyUsageBody)), Header: make(http.Header)}, nil
			})
			e, err := s.queryCodex(context.Background())
			if tc.request {
				if err != nil || calls != 1 || e.Source != "via Codex CLI" || e.Plan != "pro" || len(e.Windows) != 1 || e.Windows[0].Percent != 41 || e.AsOf <= 0 || e.Windows[0].AsOf != e.AsOf {
					t.Fatal("eligible selected OAuth request failed")
				}
			} else if calls != 0 || err != errPlanUsage {
				t.Fatal("invalid credentials did not fail closed before transport")
			}
			raw, _ := os.ReadFile(filepath.Join(s.home, ".codex", "auth.json"))
			if string(raw) != tc.body {
				t.Fatal("usage acquisition wrote credentials")
			}
		})
	}
}

func TestPlanUsageHeaderValueBytes(t *testing.T) {
	for b := 0; b < 256; b++ {
		want := b == '\t' || b >= 32 && b != 127
		if validUsageHeaderValue(string([]byte{byte(b)})) != want {
			t.Fatalf("HTTP field-value byte boundary mismatch at %d", b)
		}
	}
	for _, value := range []string{"opaque\taccess", "opaque access", "opaque-é"} {
		s := usageTestStore(t)
		raw, err := json.Marshal(map[string]any{"auth_mode": "chatgpt", "tokens": map[string]string{"access_token": value, "account_id": value}})
		if err != nil {
			t.Fatal(err)
		}
		setCodexUsageAuth(t, s, string(raw))
		calls := 0
		s.client.Transport = usageRoundTrip(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Header.Get("Authorization") != "Bearer "+value || r.Header.Get("ChatGPT-Account-Id") != value {
				t.Error("valid opaque header value was changed")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(weeklyUsageBody))}, nil
		})
		if _, err := s.queryCodex(context.Background()); err != nil || calls != 1 {
			t.Fatal("valid opaque headers were rejected")
		}
	}
}

func TestPlanUsageCodexOptionalExpiry(t *testing.T) {
	for _, tc := range []struct {
		name, access string
		request      bool
	}{
		{"expired", usageJWT(`{"exp":1}`), false},
		{"zero", usageJWT(`{"exp":0}`), false},
		{"negative", usageJWT(`{"exp":-1}`), false},
		{"future", usageJWT(fmt.Sprintf(`{"exp":%d}`, time.Now().Add(time.Hour).Unix())), true},
		{"missing", usageJWT(`{}`), true},
		{"unknown-type", usageJWT(`{"exp":"unknown"}`), true},
		{"invalid-json", usageJWT(`not-json`), true},
		{"undecodable", "opaque.not-base64!.token", true},
		{"opaque", "fake-opaque-access", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := usageTestStore(t)
			setCodexUsageAuth(t, s, fmt.Sprintf(`{"auth_mode":"chatgpt","tokens":{"access_token":%q}}`, tc.access))
			calls := 0
			s.client.Transport = usageRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				// Even an unsigned future exp is not authentication: the server refuses it.
				return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("fake-private-body"))}, nil
			})
			if _, err := s.queryCodex(context.Background()); err != errPlanUsage || (calls == 1) != tc.request || calls > 1 {
				t.Fatal("expiry admission or server-authoritative refusal failed")
			}
		})
	}
}

func TestPlanUsageCodexHomeSelection(t *testing.T) {
	s := usageTestStore(t)
	selected := filepath.Join(s.home, "selected")
	usageWrite(t, filepath.Join(s.home, ".codex", "auth.json"), `{"auth_mode":"chatgpt","tokens":{"access_token":"wrong-home-token"}}`)
	s.getenv = func(k string) string {
		if k == "CODEX_HOME" {
			return selected
		}
		return ""
	}
	setCodexUsageAuth(t, s, `{"auth_mode":"chatgpt","tokens":{"access_token":"selected-token"}}`)
	s.client.Transport = usageRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer selected-token" {
			t.Error("ignored selected CODEX_HOME")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(weeklyUsageBody))}, nil
	})
	if _, err := s.queryCodex(context.Background()); err != nil {
		t.Fatal("selected home failed")
	}
}

func TestPlanUsageCodexRESTWindows(t *testing.T) {
	for _, seconds := range []int{604799, 604800, 604801, 604859, 604860, 300, 0, -604800} {
		for _, slot := range []string{"primary_window", "secondary_window"} {
			t.Run(fmt.Sprintf("%s-%d", slot, seconds), func(t *testing.T) {
				s := usageTestStore(t)
				setCodexUsageAuth(t, s, `{"auth_mode":"chatgpt","tokens":{"access_token":"fake-access"}}`)
				s.client.Transport = usageRoundTrip(func(r *http.Request) (*http.Response, error) {
					body := fmt.Sprintf(`{"rate_limit":{%q:{"used_percent":31,"limit_window_seconds":%d,"reset_at":2100000000}}}`, slot, seconds)
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
				})
				e, err := s.queryCodex(context.Background())
				if seconds == 604800 {
					if err != nil || len(e.Windows) != 1 || e.Windows[0].Kind != "seven_day" || e.Windows[0].ResetsAt != 2100000000000 {
						t.Fatal("weekly window not mapped")
					}
				} else if err != errPlanUsage {
					t.Fatal("non-exact duration mapped as weekly")
				}
			})
		}
	}
	// The existing Pi path deliberately truncates seconds to minutes.
	s := usageTestStore(t)
	setPiUsageAuth(t, s, time.Now().Add(time.Hour).UnixMilli())
	s.client.Transport = usageRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(strings.ReplaceAll(weeklyUsageBody, "604800", "604859")))}, nil
	})
	if e, err := s.queryPi(context.Background()); err != nil || len(e.Windows) != 1 {
		t.Fatal("Pi duration semantics changed")
	}
}

func TestPlanUsageCodexHTTPFailuresRetainHistoryAndNoLeaks(t *testing.T) {
	for _, tc := range []struct {
		name, body     string
		status         int
		transportError bool
	}{
		{"401", "fake-private-body", 401, false}, {"403", "fake-private-body", 403, false},
		{"429", "fake-private-body", 429, false}, {"500", "fake-private-body", 500, false},
		{"503", "fake-private-body", 503, false}, {"malformed", "fake-private-body", 200, false},
		{"null", "null", 200, false}, {"missing", `{}`, 200, false},
		{"null-window", `{"rate_limit":{"primary_window":null}}`, 200, false},
		{"missing-percent", `{"rate_limit":{"primary_window":{"limit_window_seconds":604800}}}`, 200, false},
		{"bad-percent", `{"rate_limit":{"primary_window":{"used_percent":101,"limit_window_seconds":604800}}}`, 200, false},
		{"trailing", weeklyUsageBody + `{}`, 200, false},
		{"oversized", weeklyUsageBody + strings.Repeat(" ", 1<<20), 200, false},
		{"url-error", "", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := usageTestStore(t)
			setCodexUsageAuth(t, s, `{"auth_mode":"chatgpt","tokens":{"access_token":"fake-codex-secret","account_id":"fake-account-secret","refresh_token":"fake-refresh-secret"}}`)
			setPiUsageAuth(t, s, time.Now().Add(time.Hour).UnixMilli())
			at := time.Now().Add(-time.Minute).UnixMilli()
			s.gpt = planUsageEntry{Harness: "codex", Plan: "old-plan", Source: "via Pi", AsOf: at, Windows: []planUsageWindow{{Kind: "seven_day", Percent: 37, ResetsAt: 2100000000000, AsOf: at}}}
			s.appendHistoryLocked(usageSamples(s.gpt), at)
			before := historyBytes(t, s)
			calls := 0
			s.client.Transport = usageRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Header.Get("Authorization") != "Bearer fake-codex-secret" {
					t.Error("selected Codex fell back to Pi")
				}
				if tc.transportError {
					return nil, &url.Error{Op: "GET", URL: "https://fake-codex-secret.example", Err: errors.New("fake-private-body")}
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			s.query(context.Background(), false)
			if calls != 1 || s.gpt.Status != "usage unavailable" || s.gpt.Source != "via Pi" || s.gpt.Plan != "old-plan" || s.gpt.AsOf != at || len(s.gpt.Windows) != 1 || s.gpt.Windows[0].Percent != 37 || !bytes.Equal(before, historyBytes(t, s)) {
				t.Fatal("failure changed cached values/history or fell through to Pi")
			}
			if _, err := s.queryCodex(context.Background()); err != errPlanUsage {
				t.Fatal("error was not sanitized")
			}
			view, _ := json.Marshal(s.entries())
			latest, _ := os.ReadFile(filepath.Join(s.dir, planUsageCache))
			for _, raw := range [][]byte{view, latest, historyBytes(t, s)} {
				for _, secret := range []string{"fake-codex-secret", "fake-account-secret", "fake-refresh-secret", "fake-private-body"} {
					if bytes.Contains(raw, []byte(secret)) {
						t.Fatal("private request data leaked")
					}
				}
			}
		})
	}
}

func TestPlanUsageCodexRedirectDoesNotForwardToken(t *testing.T) {
	s := usageTestStore(t)
	setCodexUsageAuth(t, s, `{"auth_mode":"chatgpt","tokens":{"access_token":"fake-access"}}`)
	var calls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	s.usageURL = origin.URL
	if _, err := s.queryCodex(context.Background()); err != errPlanUsage || calls.Load() != 0 {
		t.Fatal("redirect forwarded credentials")
	}
}
