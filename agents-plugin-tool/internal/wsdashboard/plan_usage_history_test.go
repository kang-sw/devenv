package wsdashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func historySample(at int64, percent float64) planUsageSample {
	return planUsageSample{Harness: "codex", Kind: "seven_day", Source: "via Pi", ResetsAt: at + int64(time.Hour/time.Millisecond), AsOf: at, Percent: percent}
}
func historyRecord(v planUsageSample) planUsageHistoryRecord {
	return planUsageHistoryRecord{Sample: v}
}
func historyBytes(t *testing.T, s *planUsageStore) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(s.dir, planUsageHistoryCache))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestPlanUsageHistoryPruneBoundsAndOrdering(t *testing.T) {
	now := time.Now().UnixMilli()
	cutoff := now - planUsageHistoryAge.Milliseconds()
	exact, old := historySample(cutoff, 1), historySample(cutoff-1, 2)
	records := []planUsageHistoryRecord{historyRecord(exact), historyRecord(old)}
	for i := 0; i < 5002; i++ {
		v := historySample(now-int64(i), 3)
		// Count is per harness/kind, not per reset or source partition.
		if i%2 == 0 {
			v.Source = "via Codex CLI"
		}
		records = append(records, historyRecord(v))
	}
	other := exact
	other.Harness = "claude"
	other.Source = "Claude push"
	records = append(records, historyRecord(other))
	got := prunePlanUsageHistory(records, now)
	if len(got) != 5001 || got[0].Sample != other {
		t.Fatalf("bounds or cross-harness cap: len=%d first=%+v", len(got), got[0])
	}
	if got[1].Sample.AsOf != now-4999 || got[len(got)-1].Sample.AsOf != now {
		t.Fatal("did not keep newest 5000 across partitions")
	}
	got = prunePlanUsageHistory([]planUsageHistoryRecord{historyRecord(old), historyRecord(exact)}, now)
	if len(got) != 1 || got[0].Sample != exact {
		t.Fatal("exact age boundary removed")
	}
	a, b, c := historySample(now, 1), historySample(now, 2), historySample(now, 3)
	a.Source = "via Codex CLI"
	b.ResetsAt = 2
	c.ResetsAt = 10
	got = prunePlanUsageHistory([]planUsageHistoryRecord{historyRecord(b), historyRecord(a), historyRecord(c)}, now)
	if got[0].Sample != a || got[1].Sample != c || got[2].Sample != b {
		t.Fatal("tie order is not lexical harness/kind/source/reset")
	}
}

func TestPlanUsageHistoryValidation(t *testing.T) {
	v := historySample(time.Now().UnixMilli(), 0)
	if !validPlanUsageSample(v) {
		t.Fatal("valid sample rejected")
	}
	for _, mutate := range []func(*planUsageSample){
		func(v *planUsageSample) { v.Harness = "pi" }, func(v *planUsageSample) { v.Kind = "monthly" },
		func(v *planUsageSample) { v.Source = "account-secret" }, func(v *planUsageSample) { v.AsOf = 0 },
		func(v *planUsageSample) { v.ResetsAt = -1 }, func(v *planUsageSample) { v.Percent = 101 },
		func(v *planUsageSample) { v.Percent = math.NaN() }, func(v *planUsageSample) { v.Percent = math.Inf(1) },
		func(v *planUsageSample) { v.Harness = "claude" }, func(v *planUsageSample) { v.Kind = "five_hour" },
	} {
		bad := v
		mutate(&bad)
		if validPlanUsageSample(bad) {
			t.Fatalf("invalid accepted: %+v", bad)
		}
	}
}

func TestPlanUsageHistoryUnknownFieldsUpsertAndPrivateFile(t *testing.T) {
	s := usageTestStore(t)
	at := time.Now().UnixMilli()
	v := historySample(at, 20)
	raw := fmt.Sprintf(`{"schema_version":1,"future":{"keep":true},"samples":[{"harness":"codex","kind":"seven_day","source":"via Pi","resetsAt":%d,"asOf":%d,"percent":10,"private_future":"do-not-project"}]}`, v.ResetsAt, at)
	usageWrite(t, filepath.Join(s.dir, planUsageHistoryCache), raw)
	s.appendHistoryLocked([]planUsageSample{v}, at)
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(historyBytes(t, s), &envelope); err != nil {
		t.Fatal(err)
	}
	if string(envelope["future"]) != `{"keep":true}` {
		t.Fatal("unknown envelope field lost")
	}
	var records []map[string]json.RawMessage
	_ = json.Unmarshal(envelope["samples"], &records)
	if len(records) != 1 || string(records[0]["private_future"]) != `"do-not-project"` || string(records[0]["percent"]) != "20" {
		t.Fatal("upsert lost extension or duplicated identity")
	}
	projection := s.history(at)
	wire, _ := json.Marshal(projection)
	if projection.Status != "ok" || len(projection.Samples) != 1 || bytes.Contains(wire, []byte("future")) {
		t.Fatalf("unsafe projection: %s", wire)
	}
	info, err := os.Stat(filepath.Join(s.dir, planUsageHistoryCache))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("history mode: %v", info.Mode())
	}
	if temps, _ := filepath.Glob(filepath.Join(s.dir, planUsageHistoryCache+".*.tmp")); len(temps) != 0 {
		t.Fatal("atomic write left temporary files")
	}
}

func TestPlanUsageHistoryMissingCorruptNewerAndRepair(t *testing.T) {
	s := usageTestStore(t)
	now := time.Now().UnixMilli()
	if p := s.history(now); p.Status != "empty" || p.Samples == nil {
		t.Fatalf("missing: %+v", p)
	}
	for _, raw := range []string{`{`, `null`, `{"schema_version":2,"samples":[],"future":true}`, `{"schema_version":1,"samples":null}`, `{"schema_version":1,"samples":[{"percent":8}]}`} {
		usageWrite(t, filepath.Join(s.dir, planUsageHistoryCache), raw)
		s.appendHistoryLocked([]planUsageSample{historySample(now, 20)}, now)
		if p := s.history(now); p.Status != "unavailable" || len(p.Samples) != 0 {
			t.Fatalf("invalid history: %+v", p)
		}
		if string(historyBytes(t, s)) != raw {
			t.Fatal("corrupt/newer history destructively rewritten")
		}
		s.pushClaude(&wslive.Snapshot{Adapter: "claude", UpdatedAt: now, Usage: &wslive.Usage{Limits: []wslive.UsageLimit{{Kind: "five_hour", Percent: 30, ResetsAt: now + 100000}}}})
		if len(s.entries()) != 1 || s.entries()[0].Windows[0].Percent != 30 {
			t.Fatal("bad history broke latest usage")
		}
	}
	if err := os.Remove(filepath.Join(s.dir, planUsageHistoryCache)); err != nil {
		t.Fatal(err)
	}
	s.appendHistoryLocked([]planUsageSample{historySample(now, 20)}, now)
	if s.history(now).Status != "ok" {
		t.Fatal("explicit removal did not allow recovery")
	}
}

func TestPlanUsageHistoryClaudeAcceptanceAndCacheRereads(t *testing.T) {
	s := usageTestStore(t)
	now := time.Now().UnixMilli()
	reset := now + 100000
	push := func(at int64, p float64, r int64) {
		s.pushClaude(&wslive.Snapshot{Adapter: "claude", UpdatedAt: at, Usage: &wslive.Usage{Limits: []wslive.UsageLimit{{Kind: "five_hour", Percent: p, ResetsAt: r}}}})
	}
	push(now, 20, reset)
	push(now+1, 20, reset)
	push(now+2, 10, reset)
	push(now-1, 30, reset)
	push(now+3, 50, reset-1)
	if got := s.history(now); len(got.Samples) != 1 || got.Samples[0].AsOf != now {
		t.Fatalf("rejected snapshot entered history: %+v", got)
	}
	push(now, 25, reset) // accepted equal-time value replaces the same real observation
	push(now+4, 5, reset+1)
	before := historyBytes(t, s)
	restored := newPlanUsageStore(s.dir, s.home, s.getenv)
	for i := 0; i < 3; i++ {
		_ = restored.entries()
		_ = restored.history(now)
	}
	if !bytes.Equal(before, historyBytes(t, s)) || len(restored.history(now).Samples) != 2 {
		t.Fatal("cache reread synthesized observations")
	}
	if restored.history(now).Samples[0].Percent != 25 {
		t.Fatal("equal-time upsert did not retain last accepted value")
	}
}

func TestPlanUsageHistoryGPTAcquisitionAndOptionalAPI(t *testing.T) {
	s := usageTestStore(t)
	now := time.Now().UnixMilli()
	setPiUsageAuth(t, s, now+3600000)
	calls, percent, fail := 0, 40, false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if fail {
			http.Error(w, "fake-private-error", 500)
			return
		}
		fmt.Fprintf(w, `{"rate_limit":{"primary_window":{"used_percent":%d,"limit_window_seconds":604800,"reset_at":2100000000}}}`, percent)
	}))
	defer server.Close()
	s.usageURL = server.URL
	d := &Daemon{planUsage: s}
	get := func(url string) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		d.handlePlanUsage(w, httptest.NewRequest("GET", url, nil))
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		return w.Body.Bytes()
	}
	base := get("/rail/usage")
	if bytes.Contains(base, []byte(`"history"`)) || calls != 1 {
		t.Fatalf("default API changed: %s calls=%d", base, calls)
	}
	with := get("/rail/usage?history=1&refresh=1")
	if calls != 1 || !bytes.Contains(with, []byte(`"schema_version":1`)) {
		t.Fatal("history added a probe or absent projection")
	}
	var ordinary, optional map[string]json.RawMessage
	_ = json.Unmarshal(base, &ordinary)
	_ = json.Unmarshal(with, &optional)
	delete(optional, "history")
	if !reflect.DeepEqual(ordinary, optional) {
		t.Fatal("optional API altered existing members")
	}
	first := s.history(now).Samples[0]
	// Wait only for a distinct real response timestamp, not the polling interval.
	for time.Now().UnixMilli() <= first.AsOf {
		runtime.Gosched()
	}
	s.lastQuery = time.Time{}
	s.query(context.Background(), false, nil)
	if len(s.history(time.Now().UnixMilli()).Samples) != 2 {
		t.Fatal("fresh unchanged successful GPT response lost")
	}
	for time.Now().UnixMilli() <= s.gpt.AsOf {
		runtime.Gosched()
	}
	percent = 12
	s.lastQuery = time.Time{}
	s.query(context.Background(), false, nil)
	p := s.history(time.Now().UnixMilli())
	if len(p.Samples) != 3 || p.Samples[2].Percent != 12 || s.gpt.Windows[0].Percent != 12 {
		t.Fatal("real GPT decrease clamped or discarded")
	}
	fail = true
	s.lastQuery = time.Time{}
	s.query(context.Background(), false, nil)
	if len(s.history(time.Now().UnixMilli()).Samples) != 3 || s.gpt.Status != "usage unavailable" {
		t.Fatal("failed acquisition created history or changed latest failure state")
	}
	wire := get("/rail/usage?history=1")
	for _, secret := range []string{"fake-secret-access", "fake-account", "fake-private-error"} {
		if bytes.Contains(wire, []byte(secret)) || bytes.Contains(historyBytes(t, s), []byte(secret)) {
			t.Fatal("private acquisition data leaked")
		}
	}
}
