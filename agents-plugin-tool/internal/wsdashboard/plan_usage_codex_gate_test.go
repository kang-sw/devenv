package wsdashboard

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func isolatedUsageProbeStore(t *testing.T) *planUsageStore {
	t.Helper()
	s := usageTestStore(t)
	for key, path := range map[string]string{
		"HOME":              s.home,
		"CLAUDE_CONFIG_DIR": filepath.Join(s.home, ".claude"),
		"CODEX_HOME":        filepath.Join(s.home, ".codex"),
		"WS_CACHE_HOME":     filepath.Join(s.home, "cache"),
		"WS_CONFIG_HOME":    filepath.Join(s.home, "config"),
	} {
		t.Setenv(key, path)
	}
	return s
}

func usageProbeCalls(t *testing.T, path string) int {
	t.Helper()
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(raw), "called\n")
}

func TestPlanUsageCodexRegistryGate(t *testing.T) {
	for _, goos := range []string{"windows", "linux"} {
		for _, state := range []string{"fresh", "expired", "ended", "pi-only", "nil", "empty", "mixed"} {
			for _, force := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/force=%t", goos, state, force), func(t *testing.T) {
					s := isolatedUsageProbeStore(t)
					s.goos = goos
					usageWrite(t, filepath.Join(s.home, ".codex", "auth.json"), `{"auth_mode":"chatgpt","tokens":{}}`)
					setPiUsageAuth(t, s, time.Now().Add(time.Hour).UnixMilli())
					var piCalls atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						piCalls.Add(1)
						http.Error(w, "unexpected Pi fallback", http.StatusInternalServerError)
					}))
					defer server.Close()
					s.usageURL = server.URL
					fakeUsageCodex(t, s, `{"id":2,"result":{"rateLimits":{"planType":"pro","primary":{"usedPercent":41,"windowDurationMins":10080,"resetsAt":2100000000}}}}`)
					callsPath := filepath.Join(s.home, "codex-calls")
					t.Setenv("WS_TEST_USAGE_CODEX_CALLS", callsPath)

					now := time.Now()
					index := NewIndex(IndexOptions{Now: func() time.Time { return now }})
					reg := Registration{ReplyID: "codex-probe", Harness: "codex", HeartbeatAt: now.Format(time.RFC3339)}
					switch state {
					case "expired":
						reg.HeartbeatAt = now.Add(-LiveWindow - time.Minute).Format(time.RFC3339)
					case "ended":
						reg.EndedAt = now.Format(time.RFC3339)
					case "pi-only", "mixed":
						reg.Harness = "pi"
					}
					if state == "nil" {
						index = nil
					} else if state != "empty" {
						// Deliberately stale Live: admission must use Sessions' recomputation.
						index.sessions[reg.ReplyID] = &indexEntry{session: Session{Registration: reg, Live: state != "fresh"}}
						if state == "mixed" {
							reg.ReplyID, reg.Harness = "second-codex", "codex"
							index.sessions[reg.ReplyID] = &indexEntry{session: Session{Registration: reg}}
						}
					}
					cached := planUsageEntry{Harness: "codex", Plan: "plus", Source: "via Pi", AsOf: now.Add(-time.Minute).UnixMilli(), Windows: []planUsageWindow{{Kind: "seven_day", Percent: 37, ResetsAt: 2100000000000, AsOf: now.Add(-time.Minute).UnixMilli()}}}
					s.gpt = cached
					s.saveLocked()
					s.appendHistoryLocked(usageSamples(cached), now.UnixMilli())
					historyBefore := historyBytes(t, s)
					previous := now.Add(-6 * time.Minute)
					if force {
						previous = now.Add(-time.Minute) // forced-only cadence
					}
					s.lastQuery = previous
					d := &Daemon{planUsage: s, index: index}
					request := httptest.NewRequest("GET", "/rail/usage?history=1", nil)
					if force {
						request = httptest.NewRequest("GET", "/rail/usage?history=1&refresh=1", nil)
						request.Header.Set("X-WS-Usage-Refresh", "1")
					}
					response := httptest.NewRecorder()
					d.handlePlanUsage(response, request)
					if response.Code != http.StatusOK || piCalls.Load() != 0 || !s.lastQuery.After(previous) {
						t.Fatalf("status=%d Pi calls=%d lastQuery=%v", response.Code, piCalls.Load(), s.lastQuery)
					}
					allowed := goos != "windows" || state == "fresh" || state == "mixed"
					wantCalls := 0
					if allowed {
						wantCalls = 1
						if s.gpt.Source != "via Codex CLI" || s.gpt.Status != "" || len(s.gpt.Windows) != 1 || s.gpt.Windows[0].Percent != 41 {
							t.Fatalf("admitted probe did not succeed: %+v", s.gpt)
						}
					} else {
						cached.Status = "usage unavailable"
						if !reflect.DeepEqual(s.gpt, cached) || !strings.Contains(response.Body.String(), `"percent":37`) {
							t.Fatalf("suppression changed cached values: %+v; %s", s.gpt, response.Body.String())
						}
						if !bytes.Equal(historyBefore, historyBytes(t, s)) {
							t.Fatal("suppressed acquisition rewrote history")
						}
					}
					if calls := usageProbeCalls(t, callsPath); calls != wantCalls {
						t.Fatalf("Codex invocations=%d, want %d", calls, wantCalls)
					}
					restored := newPlanUsageStore(s.dir, s.home, s.getenv)
					if !reflect.DeepEqual(restored.gpt, s.gpt) {
						t.Fatal("latest result changed on reopen")
					}
				})
			}
		}
	}
}

func TestPlanUsageCodexSuppressionCredentialPrecedence(t *testing.T) {
	for _, mode := range []string{"chatgpt", "legacy-tokens", "apikey", "missing"} {
		for _, source := range []string{"", "via Codex CLI"} {
			t.Run(mode+"/cached="+source, func(t *testing.T) {
				s := isolatedUsageProbeStore(t)
				s.goos = "windows"
				setPiUsageAuth(t, s, time.Now().Add(time.Hour).UnixMilli())
				switch mode {
				case "chatgpt", "apikey":
					usageWrite(t, filepath.Join(s.home, ".codex", "auth.json"), fmt.Sprintf(`{"auth_mode":%q,"tokens":{}}`, mode))
				case "legacy-tokens":
					usageWrite(t, filepath.Join(s.home, ".codex", "auth.json"), `{"tokens":{}}`)
				}
				fakeUsageCodex(t, s, `{"id":2,"result":{}}`)
				callsPath := filepath.Join(s.home, "codex-calls")
				t.Setenv("WS_TEST_USAGE_CODEX_CALLS", callsPath)
				var piCalls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					piCalls.Add(1)
					if r.Header.Get("Authorization") != "Bearer fake-secret-access" {
						t.Error("wrong Pi credential")
					}
					_, _ = fmt.Fprint(w, `{"rate_limit":{"primary_window":{"used_percent":23,"limit_window_seconds":604800,"reset_at":2100000000}}}`)
				}))
				defer server.Close()
				s.usageURL = server.URL
				s.gpt.Source = source
				d := &Daemon{planUsage: s} // nil registry must not admit Codex
				response := httptest.NewRecorder()
				d.handlePlanUsage(response, httptest.NewRequest("GET", "/rail/usage", nil))
				if response.Code != http.StatusOK || usageProbeCalls(t, callsPath) != 0 {
					t.Fatal("suppressed Codex was invoked or bad response")
				}
				if mode == "chatgpt" || mode == "legacy-tokens" {
					if piCalls.Load() != 0 || s.gpt.Status != "usage unavailable" || s.gpt.Source != "via Codex CLI" || len(s.gpt.Windows) != 0 {
						t.Fatalf("suppression changed credential precedence/status: %+v, Pi calls=%d", s.gpt, piCalls.Load())
					}
				} else if piCalls.Load() != 1 || s.gpt.Source != "via Pi" || s.gpt.Status != "" || len(s.gpt.Windows) != 1 || s.gpt.Windows[0].Percent != 23 {
					t.Fatalf("existing Pi fallback changed: %+v, Pi calls=%d", s.gpt, piCalls.Load())
				}
			})
		}
	}
}
