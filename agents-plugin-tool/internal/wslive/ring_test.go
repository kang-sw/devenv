package wslive

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// stepRingClock makes every ring stamp one second after the last.
func stepRingClock(t *testing.T) {
	t.Helper()
	base := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	tick := 0
	orig := ringNow
	ringNow = func() time.Time { tick++; return base.Add(time.Duration(tick) * time.Second) }
	t.Cleanup(func() { ringNow = orig })
}

// appendTexts appends one user entry per text, in one call.
func appendTexts(t *testing.T, dir string, from, to int) {
	t.Helper()
	entries := make([]RingEntry, 0, to-from)
	for i := from; i < to; i++ {
		entries = append(entries, RingEntry{Kind: RingUser, Origin: OriginTerminal, Text: fmt.Sprintf("m%d", i)})
	}
	if err := AppendRing(dir, entries); err != nil {
		t.Fatal(err)
	}
}

// TestRingViewIsTheNewestRingCap pins the live view: ReadRing returns the
// newest RingCap entries, oldest first with their stamp time, while the
// older entries stay on disk.
func TestRingViewIsTheNewestRingCap(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ring")
	stepRingClock(t)
	for i := 0; i < RingCap+5; i++ {
		appendTexts(t, dir, i, i+1)
	}
	records, err := ReadRing(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != RingCap {
		t.Fatalf("%d entries, want %d", len(records), RingCap)
	}
	if records[0].Text != "m5" || records[RingCap-1].Text != fmt.Sprintf("m%d", RingCap+4) {
		t.Fatalf("view %q..%q, want m5..m%d", records[0].Text, records[RingCap-1].Text, RingCap+4)
	}
	if !records[1].At.After(records[0].At) || records[0].Name == "" {
		t.Fatalf("records not in time order with names: %+v %+v", records[0], records[1])
	}
	if names, _ := RingNames(dir); len(names) != RingCap+5 {
		t.Fatalf("%d entries on disk, want every %d", len(names), RingCap+5)
	}
}

// TestRingTrimsAtTheHistoryCap pins the disk bound: past RingHistoryCap
// entries the oldest are dropped.
func TestRingTrimsAtTheHistoryCap(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ring")
	stepRingClock(t)
	appendTexts(t, dir, 0, RingHistoryCap)
	appendTexts(t, dir, RingHistoryCap, RingHistoryCap+3)
	names, err := RingNames(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != RingHistoryCap {
		t.Fatalf("%d entries on disk, want %d", len(names), RingHistoryCap)
	}
	page, err := ReadRingPage(dir, "", 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	if !page.End || page.Records[0].Text != "m3" || page.Records[len(page.Records)-1].Text != fmt.Sprintf("m%d", RingHistoryCap+2) {
		t.Fatalf("kept %q..%q (end %v), want m3..m%d", page.Records[0].Text, page.Records[len(page.Records)-1].Text, page.End, RingHistoryCap+2)
	}
}

// TestRingPagesByCursor pins paging: a page is the newest n entries before
// the exclusive cursor, oldest first; its Before cursor walks to the next
// older page, and End is set on the page that reaches the oldest entry.
func TestRingPagesByCursor(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ring")
	stepRingClock(t)
	appendTexts(t, dir, 0, 7)
	texts := func(p RingPage) string {
		var out []string
		for _, r := range p.Records {
			out = append(out, r.Text)
		}
		return strings.Join(out, ",")
	}
	first, err := ReadRingPage(dir, "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if texts(first) != "m4,m5,m6" || first.End || first.Before != first.Records[0].Name {
		t.Fatalf("newest page = %q end %v before %q", texts(first), first.End, first.Before)
	}
	second, _ := ReadRingPage(dir, first.Before, 3)
	if texts(second) != "m1,m2,m3" || second.End {
		t.Fatalf("second page = %q end %v", texts(second), second.End)
	}
	last, _ := ReadRingPage(dir, second.Before, 3)
	if texts(last) != "m0" || !last.End {
		t.Fatalf("last page = %q end %v", texts(last), last.End)
	}
	past, _ := ReadRingPage(dir, last.Before, 3)
	if len(past.Records) != 0 || !past.End || past.Before != "" {
		t.Fatalf("page past the oldest = %+v", past)
	}
	// A cursor that names no entry still pages by its sort position.
	between := first.Records[1].Name[:len(first.Records[1].Name)-len(".json")] + "0.json"
	if mid, _ := ReadRingPage(dir, between, 2); texts(mid) != "m4,m5" {
		t.Fatalf("page before a non-entry cursor = %q", texts(mid))
	}
	if missing, err := ReadRingPage(filepath.Join(dir, "missing"), "", 3); err != nil || !missing.End || len(missing.Records) != 0 {
		t.Fatalf("missing ring page = %+v, %v", missing, err)
	}
}

func TestValidRingName(t *testing.T) {
	dir := t.TempDir()
	appendTexts(t, dir, 0, 1)
	names, _ := RingNames(dir)
	if len(names) != 1 || !ValidRingName(names[0]) {
		t.Fatalf("a written name is not valid: %v", names)
	}
	for _, bad := range []string{"", "../x.json", "20260101T000000.000000000Z-ab.json/../../etc", "nostamp.json", ".tmp-1", "20260101T000000.000000000Z-AB.json", "20260101T000000.000000000Z-ab.jsonx"} {
		if ValidRingName(bad) {
			t.Errorf("ValidRingName(%q) = true", bad)
		}
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

// TestRingIsOwnerOnly pins the ring's permissions: it holds conversation
// text, so its directory and entry files are owner-only.
func TestRingIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	dir := filepath.Join(t.TempDir(), "ring")
	if err := AppendRing(dir, []RingEntry{{Kind: RingUser, Text: "private words"}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("ring dir mode = %o, want 700", perm)
	}
	names, err := RingNames(dir)
	if err != nil || len(names) != 1 {
		t.Fatalf("ring names = %v, %v", names, err)
	}
	info, err = os.Stat(filepath.Join(dir, names[0]))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("ring entry mode = %o, want 600", perm)
	}
}
