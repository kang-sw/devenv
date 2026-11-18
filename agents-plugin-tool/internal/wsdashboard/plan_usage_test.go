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

// usageWriteCounter counts writes per usage file through the store's seam.
func usageWriteCounter(s *planUsageStore) map[string]int {
	writes := map[string]int{}
	s.writeFile = func(dir, name string, data []byte) error {
		writes[name]++
		return writePrivateFile(dir, name, data)
	}
	return writes
}

func TestPlanUsageSaveSkipsUnchangedAndRetriesFailedWrite(t *testing.T) {
	s := usageTestStore(t)
	writes := 0
	fail := false
	s.writeFile = func(dir, name string, data []byte) error {
		writes++
		if fail {
			return os.ErrPermission
		}
		return writePrivateFile(dir, name, data)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveLocked()
	s.saveLocked()
	if writes != 1 {
		t.Fatalf("identical saves: got %d writes, want 1", writes)
	}
	s.gpt.Status = "usage unavailable"
	s.saveLocked()
	if writes != 2 {
		t.Fatalf("changed entry: got %d writes, want 2", writes)
	}
	s.gpt.Status = "Pi token expired - open Pi to refresh"
	fail = true
	s.saveLocked()
	if writes != 3 {
		t.Fatalf("failed save: got %d writes, want 3", writes)
	}
	fail = false
	s.saveLocked()
	if writes != 4 {
		t.Fatalf("retry: got %d writes, want 4", writes)
	}
	s.saveLocked()
	if writes != 4 {
		t.Fatalf("unchanged after retry: got %d writes, want 4", writes)
	}
}

func claudeKinds(s *planUsageStore) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	kinds := []string{}
	for _, w := range s.claude.Windows {
		kinds = append(kinds, w.Kind)
	}
	return kinds
}

func TestPlanUsageClaudeKeepsLiveAbsentWindow(t *testing.T) {
	s := usageTestStore(t)
	now := time.Now().UnixMilli()
	five := wslive.UsageLimit{Kind: wslive.LimitFiveHour, Percent: 40, ResetsAt: now + 100000}
	week := wslive.UsageLimit{Kind: wslive.LimitSevenDay, Percent: 20, ResetsAt: now + 200000}
	push := func(at int64, limits ...wslive.UsageLimit) {
		s.pushClaude(&wslive.Snapshot{Adapter: "claude", UpdatedAt: at, Usage: &wslive.Usage{Limits: limits}})
	}
	push(now, five, week)
	writes := usageWriteCounter(s)
	push(now+1, week) // a report without the five-hour window does not end a live one
	push(now + 2)     // no limits is not a complete report
	push(now+2, wslive.UsageLimit{Kind: wslive.LimitFiveHour, Percent: -1})
	for _, store := range []*planUsageStore{s, newPlanUsageStore(s.dir, s.home, s.getenv)} {
		entries := store.entries()
		if len(entries) != 1 || len(entries[0].Windows) != 2 || entries[0].AsOf != now {
			t.Fatalf("live five-hour window lost or asOf refreshed: %+v", entries)
		}
	}
	if len(writes) != 0 {
		t.Fatalf("absent-window report rewrote usage files: %v", writes)
	}
	// Once its reset passes, a report without it ends the window.
	s.mu.Lock()
	s.claude.Windows[0].ResetsAt = now - 1
	s.mu.Unlock()
	push(now+3, week)
	if writes[planUsageCache] != 1 || writes[planUsageHistoryCache] != 0 {
		t.Fatalf("dropping the expired window: writes %v, want one latest and no history", writes)
	}
	// The production flap: a stale session keeps reporting the expired window
	// while a fresh one omits it. Nothing changes after the single drop.
	gone := wslive.UsageLimit{Kind: wslive.LimitFiveHour, Percent: 40, ResetsAt: now - 1}
	push(now+4, gone, week)
	push(now+5, week)
	push(now+1, five, week) // a snapshot older than the stored asOf is rejected outright
	if got := claudeKinds(s); len(got) != 1 || got[0] != wslive.LimitSevenDay || writes[planUsageCache] != 1 || writes[planUsageHistoryCache] != 0 {
		t.Fatalf("expired absent window kept or flapped: %v, writes %v", got, writes)
	}
	if entries := newPlanUsageStore(s.dir, s.home, s.getenv).entries(); len(entries) != 1 || len(entries[0].Windows) != 1 || entries[0].AsOf != now+3 {
		t.Fatalf("expected only persisted weekly window: %+v", entries)
	}
}

func TestPlanUsageClaudeIgnoresExpiredReportedWindow(t *testing.T) {
	s := usageTestStore(t)
	now := time.Now().UnixMilli()
	expired := wslive.UsageLimit{Kind: wslive.LimitFiveHour, Percent: 80, ResetsAt: now - 1000}
	week := wslive.UsageLimit{Kind: wslive.LimitSevenDay, Percent: 20, ResetsAt: now + 200000}
	push := func(at int64, limits ...wslive.UsageLimit) {
		s.pushClaude(&wslive.Snapshot{Adapter: "claude", UpdatedAt: at, Usage: &wslive.Usage{Limits: limits}})
	}
	push(now, expired, week)
	if got := claudeKinds(s); len(got) != 1 || got[0] != wslive.LimitSevenDay {
		t.Fatalf("expired reported window stored: %v", got)
	}
	if p := s.history(now); len(p.Samples) != 1 || p.Samples[0].Kind != wslive.LimitSevenDay {
		t.Fatalf("expired reported window entered history: %+v", p)
	}
	// The flap: a stale session keeps reporting the expired window while a
	// fresh one reports none. Neither changes anything, so nothing is written.
	writes := usageWriteCounter(s)
	for i := int64(1); i <= 5; i++ {
		push(now+2*i-1, expired, week)
		push(now+2*i, week)
	}
	if len(writes) != 0 || len(claudeKinds(s)) != 1 || len(s.history(now).Samples) != 1 {
		t.Fatalf("expired window flapped: writes %v, kinds %v", writes, claudeKinds(s))
	}
}

func TestPlanUsageClaudeNewerWindowReplacesAtBoundary(t *testing.T) {
	s := usageTestStore(t)
	now := time.Now().UnixMilli()
	push := func(at int64, percent float64, reset int64) {
		s.pushClaude(&wslive.Snapshot{Adapter: "claude", UpdatedAt: at, Usage: &wslive.Usage{Limits: []wslive.UsageLimit{{Kind: wslive.LimitFiveHour, Percent: percent, ResetsAt: reset}}}})
	}
	first := now + 100000
	push(now, 90, first)
	// The first window's reset passes; the next report carries the new window.
	s.mu.Lock()
	s.claude.Windows[0].ResetsAt = now - 1
	s.mu.Unlock()
	next := now + 5*3600000
	push(now+1, 3, next)
	s.mu.Lock()
	got := append([]planUsageWindow(nil), s.claude.Windows...)
	s.mu.Unlock()
	if len(got) != 1 || got[0].ResetsAt != next || got[0].Percent != 3 || got[0].AsOf != now+1 {
		t.Fatalf("newer window did not replace the old one: %+v", got)
	}
	if p := s.history(now); len(p.Samples) != 2 || p.Samples[1].ResetsAt != next {
		t.Fatalf("newer window missing from history: %+v", p)
	}
}

func TestPlanUsageUnchangedPushWritesNothing(t *testing.T) {
	s := usageTestStore(t)
	now := time.Now().UnixMilli()
	limits := []wslive.UsageLimit{{Kind: wslive.LimitFiveHour, Percent: 40, ResetsAt: now + 100000}, {Kind: wslive.LimitSevenDay, Percent: 20, ResetsAt: now + 200000}}
	writes := usageWriteCounter(s)
	s.pushClaude(&wslive.Snapshot{Adapter: "claude", UpdatedAt: now, Usage: &wslive.Usage{Limits: limits}})
	if writes[planUsageCache] != 1 || writes[planUsageHistoryCache] != 1 {
		t.Fatalf("first push: %v", writes)
	}
	for i := int64(0); i < 3; i++ {
		s.pushClaude(&wslive.Snapshot{Adapter: "claude", UpdatedAt: now + i, Usage: &wslive.Usage{Limits: limits}})
	}
	if writes[planUsageCache] != 1 || writes[planUsageHistoryCache] != 1 {
		t.Fatalf("unchanged pushes wrote: %v", writes)
	}
	// An accepted sample the prune drops at once leaves the history as read.
	old := historySample(now-planUsageHistoryAge.Milliseconds()-1, 10)
	s.mu.Lock()
	s.appendHistoryLocked([]planUsageSample{old}, now)
	s.mu.Unlock()
	if writes[planUsageHistoryCache] != 1 {
		t.Fatalf("pruned-at-once sample rewrote the history: %v", writes)
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
