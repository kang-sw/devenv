package wsdashboard

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
