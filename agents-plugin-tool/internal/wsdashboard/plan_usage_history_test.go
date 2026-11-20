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
	if got := s.history(now); len(got.Samples) != 4 || got.Samples[3].AsOf != now+3 || got.Samples[3].Percent != 50 {
		t.Fatalf("latest-wins history omitted an accepted snapshot or kept the older one: %+v", got)
	}
	push(now+3, 55, reset-1) // accepted equal-time value replaces the same real observation
	push(now+4, 5, reset+1)
	before := historyBytes(t, s)
	restored := newPlanUsageStore(s.dir, s.home, s.getenv)
	for i := 0; i < 3; i++ {
		_ = restored.entries()
		_ = restored.history(now)
	}
	if !bytes.Equal(before, historyBytes(t, s)) || len(restored.history(now).Samples) != 5 {
		t.Fatal("cache reread synthesized observations")
	}
	if restored.history(now).Samples[3].Percent != 55 {
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
	s.query(context.Background(), false)
	if len(s.history(time.Now().UnixMilli()).Samples) != 2 {
		t.Fatal("fresh unchanged successful GPT response lost")
	}
	for time.Now().UnixMilli() <= s.gpt.AsOf {
		runtime.Gosched()
	}
	percent = 12
	s.lastQuery = time.Time{}
	s.query(context.Background(), false)
	p := s.history(time.Now().UnixMilli())
	if len(p.Samples) != 3 || p.Samples[2].Percent != 12 || s.gpt.Windows[0].Percent != 12 {
		t.Fatal("real GPT decrease clamped or discarded")
	}
	fail = true
	s.lastQuery = time.Time{}
	s.query(context.Background(), false)
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

func TestPlanUsageHistoryMemoStatValidated(t *testing.T) {
	s := usageTestStore(t)
	var stats, reads int
	s.statFile = func(path string) (os.FileInfo, error) { stats++; return os.Stat(path) }
	s.readFile = func(path string) ([]byte, error) { reads++; return os.ReadFile(path) }
	now := time.Now().UnixMilli()
	first := historySample(now-2, 10)
	s.appendHistoryLocked([]planUsageSample{first}, now)
	// The write itself becomes the memo: an unchanged file costs one stat and
	// no read per request.
	stats, reads = 0, 0
	for i := 0; i < 3; i++ {
		if got := s.history(now); len(got.Samples) != 1 {
			t.Fatalf("history: %+v", got)
		}
	}
	if stats != 3 || reads != 0 {
		t.Fatalf("unchanged file: stats=%d reads=%d, want 3 and 0", stats, reads)
	}
	// Another writer replaces the file. The next append reloads it and keeps
	// that writer's sample instead of overwriting it from the memo.
	external := historySample(now-1, 20)
	external.Source = "via Codex CLI"
	raw := fmt.Sprintf(`{"schema_version":1,"samples":[{"harness":"codex","kind":"seven_day","source":"via Pi","resetsAt":%d,"asOf":%d,"percent":10},{"harness":"codex","kind":"seven_day","source":"via Codex CLI","resetsAt":%d,"asOf":%d,"percent":20}]}`, first.ResetsAt, first.AsOf, external.ResetsAt, external.AsOf)
	path := filepath.Join(s.dir, planUsageHistoryCache)
	usageWrite(t, path, raw)
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	reads = 0
	s.appendHistoryLocked([]planUsageSample{historySample(now, 30)}, now)
	if reads != 1 {
		t.Fatalf("changed stat: reads=%d, want 1", reads)
	}
	reads = 0
	got := s.history(now)
	if reads != 0 || len(got.Samples) != 3 || got.Samples[1] != external {
		t.Fatalf("write after a changed stat lost the other writer's sample: %+v (reads %d)", got, reads)
	}
	if fresh := newPlanUsageStore(s.dir, s.home, s.getenv).history(now); !reflect.DeepEqual(fresh, got) {
		t.Fatalf("memo differs from disk: %+v vs %+v", got, fresh)
	}
	// A transient read failure is not memoized as "unavailable".
	if err := os.Chtimes(path, later.Add(time.Hour), later.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	s.readFile = func(string) ([]byte, error) { return nil, os.ErrPermission }
	if p := s.history(now); p.Status != "unavailable" {
		t.Fatalf("read failure: %+v", p)
	}
	s.readFile = nil
	if p := s.history(now); p.Status != "ok" || len(p.Samples) != 3 {
		t.Fatalf("read failure stuck: %+v", p)
	}
}

func TestPlanUsageHistoryIdenticalSampleWritesNothing(t *testing.T) {
	s := usageTestStore(t)
	now := time.Now().UnixMilli()
	v := historySample(now, 10)
	s.appendHistoryLocked([]planUsageSample{v}, now)
	path := filepath.Join(s.dir, planUsageHistoryCache)
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	s.appendHistoryLocked([]planUsageSample{v}, now)
	if info, err := os.Stat(path); err != nil || !info.ModTime().Equal(old) {
		t.Fatal("identical observation rewrote the history file")
	}
	v.Percent = 11
	s.appendHistoryLocked([]planUsageSample{v}, now)
	if got := s.history(now); len(got.Samples) != 1 || got.Samples[0].Percent != 11 {
		t.Fatalf("changed value at the same identity not upserted: %+v", got)
	}
}

func TestPlanUsageHistoryViewKeepsDrawableSamples(t *testing.T) {
	s := usageTestStore(t)
	now := time.Now().UnixMilli()
	reset, stale := now+3600000, now-60000
	claude := func(at, r int64, p float64) planUsageSample {
		return planUsageSample{Harness: "claude", Kind: "five_hour", Source: "Claude push", ResetsAt: r, AsOf: at, Percent: p}
	}
	base := now - 600000
	var (
		staleOld   = claude(base, stale, 90)   // not drawn, not a neighbor: dropped
		stalePrev  = claude(base+1, stale, 91) // neighbor of the first current sample
		cur1       = claude(base+2, reset, 10) // drawn
		staleMid   = claude(base+3, stale, 92) // separates cur1 and cur2
		cur2       = claude(base+4, reset, 11) // drawn
		cur3       = claude(base+5, reset, 12) // drawn
		staleTail  = claude(base+6, stale, 93) // neighbor of the entry observation
		weeklyOld  = planUsageSample{Harness: "claude", Kind: "seven_day", Source: "Claude push", ResetsAt: now + 86400000, AsOf: base, Percent: 5}
		codexOther = historySample(base, 50) // no codex entry window: not drawn
	)
	all := []planUsageSample{staleOld, stalePrev, cur1, staleMid, cur2, cur3, staleTail, weeklyOld, codexOther}
	s.appendHistoryLocked(all, now)
	// The entry observation is newer than every stored sample.
	entries := []planUsageEntry{{Harness: "claude", Source: "Claude push", AsOf: base + 7, Windows: []planUsageWindow{{Kind: "five_hour", Percent: 13, ResetsAt: reset, AsOf: base + 7}}}}
	raw := s.historyView(entries, now)
	var got planUsageHistoryProjection
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	want := []planUsageSample{stalePrev, cur1, staleMid, cur2, cur3, staleTail}
	if got.SchemaVersion != 1 || got.Status != "ok" || !reflect.DeepEqual(got.Samples, want) {
		t.Fatalf("projection: %+v\nwant %+v", got, want)
	}
	// Memoized by revision and entries; a new sample or new entries recompute.
	if again := s.historyView(entries, now); &again[0] != &raw[0] {
		t.Fatal("unchanged history and entries re-encoded the projection")
	}
	cur4 := claude(base+8, reset, 14)
	s.appendHistoryLocked([]planUsageSample{cur4}, now)
	entries[0].AsOf, entries[0].Windows[0].AsOf, entries[0].Windows[0].Percent = base+8, base+8, 14
	_ = json.Unmarshal(s.historyView(entries, now), &got)
	if want = append(want, cur4); !reflect.DeepEqual(got.Samples, want) {
		t.Fatalf("projection after append: %+v", got.Samples)
	}
	// No entry window: no samples, but the status still reflects the file.
	_ = json.Unmarshal(s.historyView(nil, now), &got)
	if got.Status != "ok" || len(got.Samples) != 0 {
		t.Fatalf("projection without windows: %+v", got)
	}
	// The handler serves the projection under the unchanged member name.
	d := &Daemon{planUsage: s}
	w := httptest.NewRecorder()
	d.handlePlanUsage(w, httptest.NewRequest("GET", "/rail/usage?history=1", nil))
	var body struct {
		History planUsageHistoryProjection `json:"history"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.History.SchemaVersion != 1 || body.History.Status != "ok" {
		t.Fatalf("handler history member: %s", w.Body.String())
	}
}

func TestPlanUsageHistoryViewTwoWindowsTiesAndLateSamples(t *testing.T) {
	t0 := time.Now().UnixMilli() - 600000
	ra, rb, rx := t0+7*86400000, t0+8*86400000, t0-1
	v := func(at, reset int64, source string) planUsageSample {
		return planUsageSample{Harness: "codex", Kind: "seven_day", Source: source, ResetsAt: reset, AsOf: at, Percent: 20}
	}
	cli, pi := codexUsageSource, "via Pi"
	var (
		d    = v(t0, rx, pi)    // no drawn successor: dropped
		o    = v(t0+1, rx, pi)  // neighbor of a1
		a1   = v(t0+2, ra, cli) // window A
		b1   = v(t0+3, rb, cli) // window B
		a2   = v(t0+4, ra, cli) // sorts before y: same asOf, "via Codex CLI" < "via Pi"
		y    = v(t0+4, rx, pi)  // after a2 in trend()'s order: dropped
		z    = v(t0+5, rx, pi)  // dropped
		w    = v(t0+6, rx, pi)  // neighbor of both entry observations and of late
		late = v(t0+8, ra, cli) // newer than the entry observation: trend() filters it
	)
	var records []planUsageHistoryRecord
	for _, s := range []planUsageSample{late, y, d, w, a2, o, z, b1, a1} {
		records = append(records, historyRecord(s))
	}
	records = prunePlanUsageHistory(records, t0+9)
	entries := []planUsageEntry{{Harness: "codex", Source: cli, AsOf: t0 + 7, Windows: []planUsageWindow{
		{Kind: "seven_day", Percent: 30, ResetsAt: ra, AsOf: t0 + 7}, {Kind: "seven_day", Percent: 40, ResetsAt: rb, AsOf: t0 + 7},
	}}}
	got := projectPlanUsageHistory(records, entries)
	want := []planUsageSample{o, a1, b1, a2, w, late}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projection: %+v\nwant %+v", got, want)
	}
}
