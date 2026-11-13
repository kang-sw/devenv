package wsdashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func usageTestStore(t *testing.T) *planUsageStore {
	t.Helper()
	home := t.TempDir()
	return newPlanUsageStore(home, home, func(string) string { return "" })
}

func TestPlanUsageClaudeStaleGuardAndPersistence(t *testing.T) {
	s := usageTestStore(t)
	now := time.Now().UnixMilli()
	push := func(percent float64, reset, at int64) {
		s.pushClaude(&wslive.Snapshot{Adapter: "claude", UpdatedAt: at, Usage: &wslive.Usage{Limits: []wslive.UsageLimit{{Kind: wslive.LimitFiveHour, Percent: percent, ResetsAt: reset}}}})
	}
	push(40, now+10000, now)
	push(20, now+10000, now+1)
	push(90, now+5000, now+2)
	got := s.entries()[0]
	if got.Windows[0].Percent != 40 || got.AsOf != now {
		t.Fatalf("stale push replaced data: %+v", got)
	}
	push(5, now+20000, now+3)
	got = s.entries()[0]
	if got.Windows[0].Percent != 5 || got.AsOf != now+3 {
		t.Fatalf("new window rejected: %+v", got)
	}
	restored := newPlanUsageStore(s.dir, s.home, s.getenv)
	if v := restored.entries(); len(v) != 1 || v[0].Windows[0].Percent != 5 {
		t.Fatalf("cache not restored: %+v", v)
	}
	raw, err := os.ReadFile(filepath.Join(s.dir, planUsageCache))
	if err != nil || !json.Valid(raw) {
		t.Fatal("cache missing or invalid")
	}
	// A heartbeat must not make an unchanged value look newly observed.
	push(5, now+20000, now+100)
	if s.entries()[0].AsOf != now+3 {
		t.Fatal("heartbeat refreshed asOf")
	}
}

func TestPlanUsageResetNormalization(t *testing.T) {
	want := int64(1800000000000)
	for _, raw := range []string{`1800000000`, `1800000000000`, `"1800000000"`, `"2027-01-15T08:00:00Z"`} {
		if got := usageReset(json.RawMessage(raw)); got != want {
			t.Errorf("reset %s = %d", raw, got)
		}
	}
}

func usageWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPlanUsageDetection(t *testing.T) {
	s := usageTestStore(t)
	claude, codex := filepath.Join(s.home, "custom-claude"), filepath.Join(s.home, "custom-codex")
	s.getenv = func(k string) string {
		if k == "CLAUDE_CONFIG_DIR" {
			return claude
		}
		if k == "CODEX_HOME" {
			return codex
		}
		return ""
	}
	if c, x, p := s.detect(); c != "" || x || p {
		t.Fatal("empty tree detected login")
	}
	usageWrite(t, filepath.Join(claude, ".credentials.json"), `{"claudeAiOauth":{"subscriptionType":"max","accessToken":"FAKE-DO-NOT-REPORT"}}`)
	usageWrite(t, filepath.Join(codex, "auth.json"), `{"auth_mode":"apikey","OPENAI_API_KEY":"FAKE-DO-NOT-REPORT"}`)
	usageWrite(t, filepath.Join(s.home, ".pi", "agent", "auth.json"), `{"openai-codex":{"type":"oauth","access":"FAKE-DO-NOT-REPORT"}}`)
	if c, x, p := s.detect(); c != "max" || x || !p {
		t.Fatal("override/API-key/Pi detection mismatch")
	}
	usageWrite(t, filepath.Join(codex, "auth.json"), `{"auth_mode":"chatgpt","tokens":{}}`)
	if _, x, _ := s.detect(); !x {
		t.Fatal("ChatGPT login absent")
	}
	b, _ := json.Marshal(s.entries())
	if strings.Contains(string(b), "FAKE-DO-NOT-REPORT") {
		t.Fatal("credential leaked")
	}
}

func TestPlanUsageExpiredWindowAndPushedOnlyClaude(t *testing.T) {
	s := usageTestStore(t)
	now := time.Now().UnixMilli()
	s.pushClaude(&wslive.Snapshot{Adapter: "pi", UpdatedAt: now, Usage: &wslive.Usage{Limits: []wslive.UsageLimit{{Kind: wslive.LimitSevenDay, Percent: 99, ResetsAt: now + 10000}}}})
	if len(s.entries()) != 0 {
		t.Fatal("Pi data treated as Claude")
	}
	s.pushClaude(&wslive.Snapshot{Adapter: "claude", UpdatedAt: now, Usage: &wslive.Usage{Limits: []wslive.UsageLimit{{Kind: wslive.LimitSevenDay, Percent: 70, ResetsAt: now + 10000}}}})
	if got := s.entries(); len(got) != 1 || got[0].Plan != "" || got[0].Harness != "claude" {
		t.Fatal("pushed-only Claude missing")
	}
	s.mu.Lock()
	s.claude.Windows[0].ResetsAt = now - 1000
	s.mu.Unlock()
	if len(s.entries()[0].Windows) != 0 {
		t.Fatal("expired window displayed")
	}
}
