package wsdashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Frozen latest-only wire types and read/write logic from
// 55b50bd534ba5246d64f9597d6296e2308612705:plan_usage.go (type names renamed).
// Do not reuse production types: changing their tags must fail this T0 check.
type pinnedUsageWindow struct {
	Kind     string  `json:"kind"`
	Percent  float64 `json:"percent"`
	ResetsAt int64   `json:"resetsAt,omitempty"`
	AsOf     int64   `json:"asOf"`
}
type pinnedUsageEntry struct {
	Harness string              `json:"harness"`
	Plan    string              `json:"plan,omitempty"`
	Source  string              `json:"source,omitempty"`
	Status  string              `json:"status,omitempty"`
	AsOf    int64               `json:"asOf,omitempty"`
	Windows []pinnedUsageWindow `json:"windows"`
}
type pinnedUsageStore struct {
	Claude pinnedUsageEntry `json:"claude"`
	GPT    pinnedUsageEntry `json:"gpt"`
}

func pinnedUsageReadWrite(t *testing.T, dir string) {
	t.Helper()
	var saved pinnedUsageStore
	// The pinned reader ignores everything beside plan-usage.json.
	if raw, err := os.ReadFile(filepath.Join(dir, "plan-usage.json")); err == nil && json.Unmarshal(raw, &saved) == nil {
		raw, err = json.Marshal(saved)
		if err != nil {
			t.Fatal(err)
		}
		if err = writePrivateFile(dir, "plan-usage.json", raw); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatalf("pinned read failed: %v", err)
	}
}

// Frozen history read/validate/prune/append implementation from
// f3f8a4259076fddfb900127b52638507d758d40f:plan_usage_history.go.
// Names and the dir-only receiver are adapted; validUsage is frozen from that
// commit's plan_usage.go too. No production history types or validation apply.
const (
	pinnedHistoryCache = "plan-usage-history.json"
	pinnedHistoryAge   = 14 * 24 * time.Hour
	pinnedHistoryCap   = 5000
)

type pinnedHistoryStore struct {
	dir string
}
type pinnedHistorySample struct {
	Harness  string  `json:"harness"`
	Kind     string  `json:"kind"`
	Source   string  `json:"source"`
	ResetsAt int64   `json:"resetsAt"`
	AsOf     int64   `json:"asOf"`
	Percent  float64 `json:"percent"`
}
type pinnedHistoryRecord struct {
	Sample pinnedHistorySample
	Fields map[string]json.RawMessage
}
type pinnedHistoryFile struct {
	Fields  map[string]json.RawMessage
	Records []pinnedHistoryRecord
}

func pinnedHistoryValidUsage(percent float64) bool {
	return !math.IsNaN(percent) && !math.IsInf(percent, 0) && percent >= 0 && percent <= 100
}

func pinnedHistoryValidSample(v pinnedHistorySample) bool {
	sourceOK := v.Harness == "claude" && v.Source == "Claude push" || v.Harness == "codex" && (v.Source == "via Codex CLI" || v.Source == "via Pi")
	kindOK := v.Kind == "seven_day" || v.Harness == "claude" && v.Kind == "five_hour"
	return sourceOK && kindOK && v.AsOf > 0 && v.ResetsAt >= 0 && pinnedHistoryValidUsage(v.Percent)
}

// Read again before every write, as in the pinned implementation. The test
// serializes access instead of using the production store's mutex.
func (s *pinnedHistoryStore) pinnedHistoryRead() (pinnedHistoryFile, error) {
	h := pinnedHistoryFile{Fields: map[string]json.RawMessage{}}
	raw, err := os.ReadFile(filepath.Join(s.dir, pinnedHistoryCache))
	if errors.Is(err, os.ErrNotExist) {
		return h, nil
	}
	if err != nil {
		return h, err
	}
	if err = json.Unmarshal(raw, &h.Fields); err != nil {
		return h, err
	}
	var version int
	if json.Unmarshal(h.Fields["schema_version"], &version) != nil || version != 1 {
		return h, errors.New("unsupported usage history")
	}
	var records []json.RawMessage
	if json.Unmarshal(h.Fields["samples"], &records) != nil || records == nil {
		return h, errors.New("invalid usage history")
	}
	for _, raw := range records {
		var v pinnedHistorySample
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &v) != nil || !pinnedHistoryValidSample(v) || json.Unmarshal(raw, &fields) != nil {
			return h, errors.New("invalid usage history sample")
		}
		// All six members are required, including a truthful zero/unknown reset.
		for _, key := range []string{"harness", "kind", "source", "resetsAt", "asOf", "percent"} {
			if value, ok := fields[key]; !ok || string(value) == "null" {
				return h, errors.New("incomplete usage history sample")
			}
		}
		h.Records = append(h.Records, pinnedHistoryRecord{Sample: v, Fields: fields})
	}
	return h, nil
}

func pinnedHistoryPrune(records []pinnedHistoryRecord, now int64) []pinnedHistoryRecord {
	kept := make([]pinnedHistoryRecord, 0, len(records))
	cutoff := now - pinnedHistoryAge.Milliseconds()
	for _, record := range records {
		if record.Sample.AsOf >= cutoff {
			kept = append(kept, record)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		a, b := kept[i].Sample, kept[j].Sample
		if a.AsOf != b.AsOf {
			return a.AsOf < b.AsOf
		}
		if a.Harness != b.Harness {
			return a.Harness < b.Harness
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return strconv.FormatInt(a.ResetsAt, 10) < strconv.FormatInt(b.ResetsAt, 10)
	})
	counts := map[[2]string]int{}
	newest := make([]pinnedHistoryRecord, 0, len(kept))
	for i := len(kept) - 1; i >= 0; i-- {
		key := [2]string{kept[i].Sample.Harness, kept[i].Sample.Kind}
		if counts[key] < pinnedHistoryCap {
			newest = append(newest, kept[i])
			counts[key]++
		}
	}
	for i, j := 0, len(newest)-1; i < j; i, j = i+1, j-1 {
		newest[i], newest[j] = newest[j], newest[i]
	}
	return newest
}

func (s *pinnedHistoryStore) pinnedHistoryAppend(samples []pinnedHistorySample, now int64) {
	if len(samples) == 0 {
		return
	}
	h, err := s.pinnedHistoryRead()
	if err != nil {
		return
	} // Preserve corrupt/newer files for explicit repair.
	type pinnedHistoryIdentity struct {
		harness, kind, source string
		reset, at             int64
	}
	key := func(v pinnedHistorySample) pinnedHistoryIdentity {
		return pinnedHistoryIdentity{v.Harness, v.Kind, v.Source, v.ResetsAt, v.AsOf}
	}
	indices := map[pinnedHistoryIdentity]int{}
	for i, r := range h.Records {
		indices[key(r.Sample)] = i
	}
	changed := false
	for _, v := range samples {
		if !pinnedHistoryValidSample(v) {
			continue
		}
		if i, ok := indices[key(v)]; ok {
			h.Records[i].Sample = v
		} else {
			indices[key(v)] = len(h.Records)
			h.Records = append(h.Records, pinnedHistoryRecord{Sample: v})
		}
		changed = true
	}
	if !changed {
		return
	}
	h.Records = pinnedHistoryPrune(h.Records, now)
	records := make([]map[string]json.RawMessage, 0, len(h.Records))
	for _, r := range h.Records {
		fields := r.Fields
		if fields == nil {
			fields = map[string]json.RawMessage{}
		}
		raw, _ := json.Marshal(r.Sample)
		var known map[string]json.RawMessage
		_ = json.Unmarshal(raw, &known)
		for k, v := range known {
			fields[k] = v
		}
		records = append(records, fields)
	}
	h.Fields["schema_version"] = json.RawMessage(`1`)
	h.Fields["samples"], err = json.Marshal(records)
	if err != nil {
		return
	}
	raw, err := json.Marshal(h.Fields)
	if err == nil {
		_ = writePrivateFile(s.dir, pinnedHistoryCache, raw)
	}
}

func TestPlanUsageCodexRESTHistoryOldWriterReopen(t *testing.T) {
	s := usageTestStore(t)
	at := time.Now().Add(-time.Minute).UnixMilli()
	s.claude = planUsageEntry{Harness: "claude", Source: "Claude push", AsOf: at, Windows: []planUsageWindow{{Kind: "five_hour", Percent: 33.5, ResetsAt: at + 3600000, AsOf: at}}}
	s.gpt = planUsageEntry{Harness: "codex", Plan: "pro", Source: "via Pi", AsOf: at, Windows: []planUsageWindow{{Kind: "seven_day", Percent: 23, ResetsAt: 2100000000000, AsOf: at}}}
	prior := append(usageSamples(s.claude), usageSamples(s.gpt)...)
	s.appendHistoryLocked(prior, at)
	s.saveLocked()

	// Seed supported root and sample extensions before either writer runs.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(historyBytes(t, s), &fields); err != nil {
		t.Fatal(err)
	}
	var records []map[string]json.RawMessage
	if err := json.Unmarshal(fields["samples"], &records); err != nil {
		t.Fatal(err)
	}
	fields["future_root"] = json.RawMessage(`{"retained":true}`)
	records[0]["future_sample"] = json.RawMessage(`{"retained":true}`)
	var err error
	fields["samples"], err = json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err = writePrivateFile(s.dir, "plan-usage-history.json", raw); err != nil {
		t.Fatal(err)
	}

	setCodexUsageAuth(t, s, `{"auth_mode":"chatgpt","tokens":{"access_token":"fake-access"}}`)
	calls := 0
	s.client.Transport = usageRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(weeklyUsageBody))}, nil
	})
	// query invokes the real queryCodex and persists both latest and history.
	s.query(context.Background(), false)
	if calls != 1 || s.gpt.Source != "via Codex CLI" || s.gpt.Status != "" || s.gpt.AsOf <= at || len(s.gpt.Windows) != 1 {
		t.Fatalf("REST query did not store a successful Codex observation: %+v (requests %d)", s.gpt, calls)
	}
	wantREST := pinnedHistorySample{Harness: "codex", Kind: "seven_day", Source: "via Codex CLI", Percent: 41, ResetsAt: 2100000000000, AsOf: s.gpt.AsOf}
	if w := s.gpt.Windows[0]; w.Kind != wantREST.Kind || w.Percent != wantREST.Percent || w.ResetsAt != wantREST.ResetsAt || w.AsOf != wantREST.AsOf {
		t.Fatalf("unexpected REST window: %+v", w)
	}
	wantClaude, wantGPT := s.claude, s.gpt
	latest, err := os.ReadFile(filepath.Join(s.dir, "plan-usage.json"))
	if err != nil {
		t.Fatal(err)
	}

	old := &pinnedHistoryStore{dir: s.dir}
	h, err := old.pinnedHistoryRead()
	if err != nil {
		t.Fatalf("pre-REST reader rejected new REST history: %v", err)
	}
	if len(h.Records) != 3 || h.Records[2].Sample != wantREST {
		t.Fatalf("pre-REST reader lost or changed the REST sample: %+v", h.Records)
	}
	appended := pinnedHistorySample{Harness: "codex", Kind: "seven_day", Source: "via Pi", Percent: 52.5, ResetsAt: 2100000000000, AsOf: wantREST.AsOf + 1}
	old.pinnedHistoryAppend([]pinnedHistorySample{appended}, appended.AsOf)
	h, err = old.pinnedHistoryRead()
	if err != nil || len(h.Records) != 4 || h.Records[2].Sample != wantREST || h.Records[3].Sample != appended {
		t.Fatalf("pre-REST read/append failed or changed REST data: %+v, %v", h.Records, err)
	}
	if string(h.Fields["future_root"]) != `{"retained":true}` || string(h.Records[0].Fields["future_sample"]) != `{"retained":true}` {
		t.Fatal("new/old history writers lost supported extensions")
	}
	pinnedUsageReadWrite(t, s.dir)
	afterLatest, err := os.ReadFile(filepath.Join(s.dir, "plan-usage.json"))
	if err != nil || !bytes.Equal(latest, afterLatest) {
		t.Fatalf("old writers changed latest cache: %v", err)
	}

	reopened := newPlanUsageStore(s.dir, s.home, s.getenv)
	if !reflect.DeepEqual(reopened.claude, wantClaude) || !reflect.DeepEqual(reopened.gpt, wantGPT) {
		t.Fatalf("new reopen lost latest windows/source/asOf: claude=%+v gpt=%+v", reopened.claude, reopened.gpt)
	}
	want := append(prior,
		planUsageSample{Harness: "codex", Kind: "seven_day", Source: "via Codex CLI", Percent: 41, ResetsAt: 2100000000000, AsOf: wantREST.AsOf},
		planUsageSample{Harness: "codex", Kind: "seven_day", Source: "via Pi", Percent: 52.5, ResetsAt: 2100000000000, AsOf: appended.AsOf})
	if got := reopened.history(appended.AsOf); got.Status != "ok" || !reflect.DeepEqual(got.Samples, want) {
		t.Fatalf("new reopen lost prior/REST/old-appended samples: %+v, want %+v", got, want)
	}
}

func TestPlanUsageHistoryMixedVersionNonDestruction(t *testing.T) {
	s := usageTestStore(t)
	now := time.Now().UnixMilli()
	s.claude = planUsageEntry{Harness: "claude", Source: "Claude push", AsOf: now, Windows: []planUsageWindow{{Kind: "five_hour", Percent: 33.5, ResetsAt: now + 100000, AsOf: now}}}
	s.gpt = planUsageEntry{Harness: "codex", Plan: "pro", Source: "via Pi", Status: "usage unavailable", AsOf: now, Windows: []planUsageWindow{{Kind: "seven_day", Percent: 45, AsOf: now}}}
	s.saveLocked()
	baseline, err := os.ReadFile(filepath.Join(s.dir, planUsageCache))
	if err != nil {
		t.Fatal(err)
	}
	s.appendHistoryLocked(usageSamples(s.claude), now)
	s.appendHistoryLocked(usageSamples(s.gpt), now)
	history := historyBytes(t, s)
	pinnedUsageReadWrite(t, s.dir)
	if !bytes.Equal(history, historyBytes(t, s)) {
		t.Fatal("pinned latest-only writer destroyed history")
	}
	latest, _ := os.ReadFile(filepath.Join(s.dir, planUsageCache))
	if !bytes.Equal(latest, baseline) {
		t.Fatal("new latest cache bytes differ from pinned writer")
	}
	restored := newPlanUsageStore(s.dir, s.home, s.getenv)
	restored.saveLocked()
	latest, _ = os.ReadFile(filepath.Join(s.dir, planUsageCache))
	if !bytes.Equal(latest, baseline) || !bytes.Equal(history, historyBytes(t, s)) {
		t.Fatal("new reader/writer changed latest shape or separate history")
	}
	if got := restored.history(now); len(got.Samples) != 2 {
		t.Fatal("new reader lost mixed-version history")
	}
}

func TestPlanUsageAgyT0CompatibilityLeavesExistingFilesUnchanged(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	reset5h := now.Add(3 * time.Hour).Format(time.RFC3339)
	resetWeekly := now.Add(4 * 24 * time.Hour).Format(time.RFC3339)
	s, _, _ := fakeAgyEnv(t, "cat <<'EOF'\n"+validAgyFixtureJSON(reset5h, resetWeekly)+"\nEOF")
	s.now = func() time.Time { return now }

	at := now.UnixMilli()
	s.claude = planUsageEntry{Harness: "claude", Source: "Claude push", AsOf: at, Windows: []planUsageWindow{{Kind: "five_hour", Percent: 33.5, ResetsAt: at + 3600000, AsOf: at}}}
	s.gpt = planUsageEntry{Harness: "codex", Plan: "pro", Source: "via Pi", AsOf: at, Windows: []planUsageWindow{{Kind: "seven_day", Percent: 45, ResetsAt: at + 86400000, AsOf: at}}}
	s.saveLocked()
	s.appendHistoryLocked(append(usageSamples(s.claude), usageSamples(s.gpt)...), at)

	baselineLatest, err := os.ReadFile(filepath.Join(s.dir, planUsageCache))
	if err != nil {
		t.Fatal(err)
	}
	baselineHistory := historyBytes(t, s)

	// Run agy probe, creating plan-usage-agy.json and plan-usage-agy-history.json.
	s.refresh(context.Background(), true)
	s.probes.Wait()
	if _, err := os.Stat(filepath.Join(s.dir, planUsageAgyCache)); err != nil {
		t.Fatalf("missing %s: %v", planUsageAgyCache, err)
	}
	if _, err := os.Stat(filepath.Join(s.dir, planUsageAgyHistoryCache)); err != nil {
		t.Fatalf("missing %s: %v", planUsageAgyHistoryCache, err)
	}

	// plan-usage.json and plan-usage-history.json must stay byte-for-byte identical.
	afterLatest, _ := os.ReadFile(filepath.Join(s.dir, planUsageCache))
	if !bytes.Equal(baselineLatest, afterLatest) {
		t.Fatalf("agy probe changed %s:\nbefore: %s\nafter:  %s", planUsageCache, baselineLatest, afterLatest)
	}
	if !bytes.Equal(baselineHistory, historyBytes(t, s)) {
		t.Fatalf("agy probe changed %s", planUsageHistoryCache)
	}

	// Old pinned readers and writers still read/write plan-usage.json and
	// plan-usage-history.json cleanly while ignoring both new agy files.
	pinnedUsageReadWrite(t, s.dir)
	oldHist := &pinnedHistoryStore{dir: s.dir}
	h, err := oldHist.pinnedHistoryRead()
	if err != nil || len(h.Records) != 2 {
		t.Fatalf("pinned history reader failed with agy files present: len=%d err=%v", len(h.Records), err)
	}
	appended := pinnedHistorySample{Harness: "codex", Kind: "seven_day", Source: "via Pi", Percent: 50, ResetsAt: at + 86400000, AsOf: at + 1000}
	oldHist.pinnedHistoryAppend([]pinnedHistorySample{appended}, appended.AsOf)

	// New store reopen restores Claude, Codex, and both agy files intact.
	reopened := newPlanUsageStore(s.dir, s.home, s.getenv)
	reopened.now = func() time.Time { return now }
	if len(reopened.agyGroups) != 2 {
		t.Fatalf("reopened store lost agy cache after old writer ran: %+v", reopened.agyGroups)
	}
	if got := reopened.agyHistory(at); got.Status != "ok" || len(got.Samples) != 4 {
		t.Fatalf("reopened store lost agy history after old writer ran: %+v", got)
	}
	if got := reopened.history(appended.AsOf); got.Status != "ok" || len(got.Samples) != 3 {
		t.Fatalf("reopened store lost existing history: %+v", got)
	}
}

func TestPlanUsageCodexFiveHourT0CompatibilityAndSeparateFile(t *testing.T) {
	s := usageTestStore(t)
	setCodexUsageAuth(t, s, `{"auth_mode":"chatgpt","tokens":{"access_token":"fake-access"}}`)
	bodyBoth := `{"plan_type":"pro","rate_limit":{"primary_window":{"used_percent":22,"limit_window_seconds":18000,"reset_at":2099000000},"secondary_window":{"used_percent":48,"limit_window_seconds":604800,"reset_at":2100000000}}}`
	s.client.Transport = usageRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(bodyBoth))}, nil
	})
	s.query(context.Background(), false)

	// Shared plan-usage-history.json must stay readable by the pinned old validator
	// (no Codex five_hour sample may ever land there).
	oldHist := &pinnedHistoryStore{dir: s.dir}
	h, err := oldHist.pinnedHistoryRead()
	if err != nil || len(h.Records) != 1 || h.Records[0].Sample.Harness != "codex" || h.Records[0].Sample.Kind != "seven_day" {
		t.Fatalf("pinned history reader failed or saw codex 5h: len=%d err=%v recs=%+v", len(h.Records), err, h.Records)
	}
	// Codex five_hour sample lands exclusively in plan-usage-codex-history.json.
	now := time.Now().UnixMilli()
	codexHist := s.codexFiveHourHistory(now)
	if codexHist.Status != "ok" || len(codexHist.Samples) != 1 || codexHist.Samples[0].Kind != "five_hour" || codexHist.Samples[0].Percent != 22 {
		t.Fatalf("codex 5h history = %+v", codexHist)
	}
	if _, err := os.Stat(filepath.Join(s.dir, planUsageCodexHistoryCache)); err != nil {
		t.Fatalf("missing %s: %v", planUsageCodexHistoryCache, err)
	}

	// /rail/usage?history=1 emits codexFiveHourHistory when samples exist, and omits
	// it when no Codex five_hour samples exist.
	d := &Daemon{dir: s.dir, planUsage: s}
	rec := httptest.NewRecorder()
	d.handlePlanUsage(rec, httptest.NewRequest(http.MethodGet, "/rail/usage?history=1", nil))
	var with5h map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &with5h); err != nil {
		t.Fatal(err)
	}
	if _, ok := with5h["codexFiveHourHistory"]; !ok {
		t.Fatalf("expected codexFiveHourHistory when samples exist: %s", rec.Body.String())
	}

	sWeeklyOnly := usageTestStore(t)
	setCodexUsageAuth(t, sWeeklyOnly, `{"auth_mode":"chatgpt","tokens":{"access_token":"fake-access"}}`)
	sWeeklyOnly.client.Transport = usageRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(weeklyUsageBody))}, nil
	})
	sWeeklyOnly.query(context.Background(), false)
	dWeekly := &Daemon{dir: sWeeklyOnly.dir, planUsage: sWeeklyOnly}
	rec2 := httptest.NewRecorder()
	dWeekly.handlePlanUsage(rec2, httptest.NewRequest(http.MethodGet, "/rail/usage?history=1", nil))
	var without5h map[string]json.RawMessage
	if err := json.Unmarshal(rec2.Body.Bytes(), &without5h); err != nil {
		t.Fatal(err)
	}
	if _, ok := without5h["codexFiveHourHistory"]; ok {
		t.Fatalf("unexpected codexFiveHourHistory without 5h samples: %s", rec2.Body.String())
	}
}
