package wslive

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestSnapshotForDiskExcludesOnlyLiveReasoningSummary(t *testing.T) {
	st := NewState("disk-fixture")
	_, _ = st.Apply(Event{Kind: KindTurnStart, At: 1000}, 1000)
	if _, err := st.Apply(Event{Kind: KindAgent, ReasoningSummary: summaryFixture()}, 1001); err != nil {
		t.Fatal(err)
	}
	live := st.Snapshot(1002)
	disk := live.ForDisk()
	if live.Lead.ReasoningSummary == nil || disk.Lead.ReasoningSummary != nil {
		t.Fatal("disk filtering mutated the RAM projection or retained live data")
	}
	raw, err := json.Marshal(disk)
	if err != nil || bytes.Contains(raw, []byte(`"reasoningSummary"`)) {
		t.Fatalf("mutable snapshot contains live summary: %s, %v", raw, err)
	}
	live.Lead.ReasoningSummary = nil
	legacy, err := json.Marshal(live)
	if err != nil || !bytes.Equal(raw, legacy) {
		t.Fatal("filter changed legacy snapshot fields", err)
	}
	var reopened Snapshot
	if err := json.Unmarshal(raw, &reopened); err != nil || reopened.Lead.ReasoningSummary != nil || reopened.SchemaVersion != SchemaVersion || reopened.Lead.TurnStartedAt != 1000 {
		t.Fatalf("reopen invented live recovery or changed legacy schema: %+v %v", reopened, err)
	}
}
