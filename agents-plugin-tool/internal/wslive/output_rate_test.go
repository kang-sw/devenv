package wslive

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOutputRateApplyResetRetentionAndValidation(t *testing.T) {
	st := NewState("test-session")

	// Invalid rates are ignored without changing state or requesting persistence.
	for _, bad := range []*OutputRate{
		{OutputTps: 0, At: 100},
		{OutputTps: -5, At: 100},
		{OutputTps: MaxOutputTps + 1, At: 100},
		{OutputTps: math.NaN(), At: 100},
		{OutputTps: math.Inf(1), At: 100},
		{OutputTps: 42, At: 0},
		{OutputTps: 42, At: -1},
	} {
		res, err := st.Apply(Event{Kind: KindStatus, OutputRate: bad}, 1000)
		if err != nil || res.Changed || res.OutputRate != nil {
			t.Fatalf("bad rate %+v accepted: res=%+v err=%v", bad, res, err)
		}
	}

	// Lead-level KindAgent rejects OutputRate just like Observed.
	if _, err := st.Apply(Event{Kind: KindAgent, OutputRate: &OutputRate{OutputTps: 40, At: 100}}, 1000); err == nil {
		t.Fatal("lead KindAgent accepted OutputRate")
	}

	// Valid rate updates state, leaves Observed nil, and requests persistence.
	in := &OutputRate{OutputTps: 41.6, At: 2000}
	res, err := st.Apply(Event{Kind: KindStatus, OutputRate: in}, 2000)
	if err != nil || !res.Changed || res.OutputRate == nil || res.OutputRate.OutputTps != 41.6 || res.OutputRate.At != 2000 {
		t.Fatalf("valid rate apply: res=%+v err=%v", res, err)
	}
	in.OutputTps = 999 // caller ownership ends at Apply
	snap := st.Snapshot(2100)
	if snap.Lead.Observed != nil || snap.Lead.LastTurnOutput != nil {
		t.Fatalf("OutputRate fabricated model observation: %+v", snap.Lead)
	}
	if snap.Lead.OutputRate == nil || snap.Lead.OutputRate.OutputTps != 41.6 || snap.Lead.OutputRate.At != 2000 {
		t.Fatalf("snapshot outputRate = %+v", snap.Lead.OutputRate)
	}
	snap.Lead.OutputRate.OutputTps = 1 // snapshot must not alias state
	if st.Snapshot(2200).Lead.OutputRate.OutputTps != 41.6 {
		t.Fatal("snapshot aliased mutable outputRate")
	}

	// Duplicate or older rate writes nothing and keeps newer value.
	res, err = st.Apply(Event{Kind: KindStatus, OutputRate: &OutputRate{OutputTps: 41.6, At: 2000}}, 2300)
	if err != nil || res.Changed || res.OutputRate != nil {
		t.Fatalf("duplicate rate changed state: %+v %v", res, err)
	}
	res, err = st.Apply(Event{Kind: KindStatus, OutputRate: &OutputRate{OutputTps: 10, At: 1500}}, 2400)
	if err != nil || res.Changed || res.OutputRate != nil {
		t.Fatalf("older rate overwrote newer: %+v %v", res, err)
	}

	// Retained across KindTurnStart, KindTurnEnd without output, and KindReset.
	_, _ = st.Apply(Event{Kind: KindTurnStart, At: 3000}, 3000)
	if got := st.Snapshot(3100).Lead.OutputRate; got == nil || got.OutputTps != 41.6 {
		t.Fatalf("turn start cleared outputRate: %+v", got)
	}
	_, _ = st.Apply(Event{Kind: KindTurnEnd, At: 3500}, 3500)
	if got := st.Snapshot(3600).Lead.OutputRate; got == nil || got.OutputTps != 41.6 {
		t.Fatalf("ineligible turn end cleared outputRate: %+v", got)
	}
	_, _ = st.Apply(Event{Kind: KindReset, At: 4000}, 4000)
	if got := st.Snapshot(4100).Lead.OutputRate; got == nil || got.OutputTps != 41.6 {
		t.Fatalf("reset cleared outputRate: %+v", got)
	}
}

func TestOutputRateDurableSidecarT1SurvivesLegacySnapshotWriterAndPrune(t *testing.T) {
	dir := t.TempDir()
	st := NewState("agy-session")
	res, err := st.Apply(Event{Kind: KindStatus, OutputRate: &OutputRate{OutputTps: 53.4, At: 5000}}, 5000)
	if err != nil || res.OutputRate == nil {
		t.Fatalf("apply: res=%+v err=%v", res, err)
	}
	if err := WriteOutputRate(dir, res.OutputRate); err != nil {
		t.Fatal(err)
	}

	// Verify sidecar carries only rate + timestamp, never model or source.
	rawSidecar, err := os.ReadFile(OutputRatePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(rawSidecar, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || fields["outputTps"] != 53.4 || fields["at"] != float64(5000) {
		t.Fatalf("sidecar fields = %v", fields)
	}

	// New snapshot contains lead.outputRate; an old daemon reading it into a
	// legacy Snapshot struct ignores the unknown field and sees no Observed/LastTurnOutput.
	newSnapRaw, err := json.Marshal(st.Snapshot(5100))
	if err != nil {
		t.Fatal(err)
	}
	var legacyRead struct {
		SchemaVersion int    `json:"schemaVersion"`
		ReplyID       string `json:"replyId"`
		Lead          struct {
			Phase          string            `json:"phase"`
			Observed       *ObservedModel    `json:"observed,omitempty"`
			LastTurnOutput *ModelObservation `json:"lastTurnOutput,omitempty"`
		} `json:"lead"`
	}
	if err := json.Unmarshal(newSnapRaw, &legacyRead); err != nil {
		t.Fatalf("legacy reader rejected new snapshot: %v", err)
	}
	if legacyRead.Lead.Observed != nil || legacyRead.Lead.LastTurnOutput != nil {
		t.Fatalf("legacy reader saw model observation: %+v", legacyRead.Lead)
	}

	// Old daemon rewrites snapshot.json, dropping lead.outputRate.
	legacyWrite, _ := json.Marshal(legacyRead)
	if err := os.WriteFile(SnapshotPath(dir), legacyWrite, 0600); err != nil {
		t.Fatal(err)
	}
	// Old model-evidence prune touches only model-evidence/, never output-rate.json.
	pruneObserved(filepath.Join(dir, ObservedDirName))

	// New daemon reopens: reads stripped snapshot + intact output-rate.json.
	snap, err := ReadSnapshot(SnapshotPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if snap.Lead.OutputRate != nil {
		t.Fatalf("legacy rewrite unexpectedly retained lead.outputRate: %+v", snap.Lead.OutputRate)
	}
	rate, err := ReadOutputRate(dir)
	if err != nil || rate == nil || rate.OutputTps != 53.4 || rate.At != 5000 {
		t.Fatalf("ReadOutputRate after legacy rewrite = %+v, %v", rate, err)
	}
	OverlayOutputRate(&snap, rate)
	if snap.Lead.OutputRate == nil || snap.Lead.OutputRate.OutputTps != 53.4 || snap.Lead.Observed != nil {
		t.Fatalf("overlay after reopen = %+v", snap.Lead)
	}
	reopened := NewState("agy-session")
	reopened.SeedOutputRate(rate)
	if got := reopened.Snapshot(6000).Lead.OutputRate; got == nil || got.OutputTps != 53.4 || got.At != 5000 {
		t.Fatalf("reopened state outputRate = %+v", got)
	}
}

func TestOutputRateCacheValidationAndNegativeCache(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1000, 0)
	stats := 0
	cache := OutputRateCache{
		now: func() time.Time { return now },
		stat: func(path string) (os.FileInfo, error) {
			stats++
			return os.Stat(path)
		},
	}

	// Absent file is negatively cached for 1s.
	if got := cache.Load(dir); got != nil {
		t.Fatalf("absent load = %+v", got)
	}
	if got := cache.Load(dir); got != nil {
		t.Fatalf("cached absent load = %+v", got)
	}
	if stats != 1 {
		t.Fatalf("absent stats within 1s = %d, want 1", stats)
	}

	// Write sidecar and invalidate -> reloads immediately.
	if err := WriteOutputRate(dir, &OutputRate{OutputTps: 25, At: 1000}); err != nil {
		t.Fatal(err)
	}
	cache.Invalidate(dir)
	if got := cache.Load(dir); got == nil || got.OutputTps != 25 || got.At != 1000 {
		t.Fatalf("reloaded after invalidate = %+v", got)
	}

	// Within 1s, cached hit does not re-stat.
	before := stats
	if got := cache.Load(dir); got == nil || got.OutputTps != 25 {
		t.Fatalf("cached hit = %+v", got)
	}
	if stats != before {
		t.Fatalf("re-statted within 1s: %d -> %d", before, stats)
	}

	// Advance clock past 1s after updating the file.
	now = now.Add(2 * time.Second)
	if err := WriteOutputRate(dir, &OutputRate{OutputTps: 88.5, At: 2000}); err != nil {
		t.Fatal(err)
	}
	if got := cache.Load(dir); got == nil || got.OutputTps != 88.5 || got.At != 2000 {
		t.Fatalf("updated load = %+v", got)
	}
	cache.Forget(dir)
	if got := cache.Load(dir); got == nil || got.OutputTps != 88.5 {
		t.Fatalf("load after forget = %+v", got)
	}
}
