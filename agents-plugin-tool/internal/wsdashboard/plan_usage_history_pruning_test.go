package wsdashboard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPlanUsageHistoryPersistedPruningAndIdentity(t *testing.T) {
	s := usageTestStore(t)
	now := time.Now().UnixMilli()
	cutoff := now - planUsageHistoryAge.Milliseconds()
	old := historySample(cutoff-1, 9)
	exact := historySample(cutoff, 10)
	exact.Harness = "claude"
	exact.Kind = "five_hour"
	exact.Source = "Claude push"
	samples := []planUsageSample{old, exact}
	for i := 0; i < 5001; i++ {
		v := historySample(now-int64(i), 20)
		if i%2 == 0 {
			v.Source = "via Codex CLI"
		}
		samples = append(samples, v)
	}
	unknown := exact
	unknown.AsOf = now
	unknown.ResetsAt = 0
	unknown.Percent = 30
	samples = append(samples, unknown)
	s.appendHistoryLocked(samples, now)
	// Read the physical file too: projection-only pruning is not sufficient.
	var disk struct {
		Samples []planUsageSample `json:"samples"`
	}
	if err := json.Unmarshal(historyBytes(t, s), &disk); err != nil {
		t.Fatal(err)
	}
	if len(disk.Samples) != 5002 || disk.Samples[0] != exact {
		t.Fatalf("stored bounds: %d", len(disk.Samples))
	}
	if len(s.history(now).Samples) != len(disk.Samples) {
		t.Fatal("disk and projection bounds differ")
	}
	foundUnknown := false
	for _, v := range disk.Samples {
		if v == unknown {
			foundUnknown = true
		}
		if v.AsOf < cutoff {
			t.Fatal("expired sample persisted")
		}
	}
	if !foundUnknown {
		t.Fatal("unknown reset real observation discarded")
	}
	restored := newPlanUsageStore(s.dir, s.home, s.getenv)
	if len(restored.history(now).Samples) != 5002 {
		t.Fatal("bounded history did not survive restart")
	}
}

func TestPlanUsageHistoryInvalidFileAndNilStoreProjection(t *testing.T) {
	for _, body := range []string{
		`{"schema_version":1,"samples":[{"harness":"codex","kind":"seven_day","source":"via Pi","resetsAt":0,"asOf":123,"percent":null}]}`,
		`{"schema_version":1,"samples":[{"harness":"codex","kind":"seven_day","source":"via Pi","asOf":123,"percent":5}]}`,
	} {
		s := usageTestStore(t)
		usageWrite(t, filepath.Join(s.dir, planUsageHistoryCache), body)
		s.appendHistoryLocked([]planUsageSample{historySample(time.Now().UnixMilli(), 20)}, time.Now().UnixMilli())
		if s.history(time.Now().UnixMilli()).Status != "unavailable" || string(historyBytes(t, s)) != body {
			t.Fatal("incomplete history was repaired implicitly")
		}
	}
	var s *planUsageStore
	if p := s.history(time.Now().UnixMilli()); p.Status != "empty" || p.Samples == nil {
		t.Fatal("nil history projection")
	}
	// IO errors also fail closed and never replace the obstructing path.
	s = usageTestStore(t)
	path := filepath.Join(s.dir, planUsageHistoryCache)
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	s.appendHistoryLocked([]planUsageSample{historySample(time.Now().UnixMilli(), 20)}, time.Now().UnixMilli())
	if p := s.history(time.Now().UnixMilli()); p.Status != "unavailable" {
		t.Fatal(fmt.Sprintf("read error: %+v", p))
	}
}
