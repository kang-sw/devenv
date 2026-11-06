package wslive

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRingKeepsNewestNAndDropsOldest pins the ring bound: past RingCap
// entries the oldest are dropped, and entries read back oldest first with
// their stamp time.
func TestRingKeepsNewestNAndDropsOldest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ring")
	base := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	tick := 0
	orig := ringNow
	ringNow = func() time.Time { tick++; return base.Add(time.Duration(tick) * time.Second) }
	t.Cleanup(func() { ringNow = orig })

	for i := 0; i < RingCap+5; i++ {
		if err := AppendRing(dir, []RingEntry{{Kind: RingUser, Origin: OriginTerminal, Text: fmt.Sprintf("m%d", i)}}); err != nil {
			t.Fatal(err)
		}
	}
	records, err := ReadRing(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != RingCap {
		t.Fatalf("%d entries, want %d", len(records), RingCap)
	}
	if records[0].Text != "m5" || records[RingCap-1].Text != fmt.Sprintf("m%d", RingCap+4) {
		t.Fatalf("kept %q..%q, want m5..m%d", records[0].Text, records[RingCap-1].Text, RingCap+4)
	}
	if !records[1].At.After(records[0].At) || records[0].Name == "" {
		t.Fatalf("records not in time order with names: %+v %+v", records[0], records[1])
	}
}

func TestRingReadSkipsJunkAndToleratesUnknownFields(t *testing.T) {
	dir := t.TempDir()
	if err := AppendRing(dir, []RingEntry{{Kind: RingMail, Direction: MailSent, Peer: "bob@machine", Title: "hi", HistoryKey: "k.json"}}); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "20260101T000000.000000000Z-ffff.json"), []byte(`{"kind":"user","text":"x","future":1}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "20260101T000001.000000000Z-eeee.json"), []byte(`not json`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "nostamp.json"), []byte(`{"kind":"user"}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".tmp-123"), []byte(`{}`), 0o644)
	records, err := ReadRing(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Text != "x" || records[1].HistoryKey != "k.json" {
		t.Fatalf("records = %+v", records)
	}
	if got, err := ReadRing(filepath.Join(dir, "missing")); err != nil || got != nil {
		t.Fatalf("missing ring = %v, %v", got, err)
	}
}
