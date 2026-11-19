package wslive

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
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
	info, err = os.Stat(filepath.Join(dir, ringSegmentName(1)))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("ring segment mode = %o, want 600", perm)
	}
}

func ringIOCount(op string) int64 {
	var n int64
	for site, count := range wsstate.FileIOCounters() {
		if strings.HasPrefix(site, "file_io_site:wslive/ring") && strings.HasSuffix(site, ":"+op) {
			n += count
		}
	}
	return n
}

func TestRingSegmentRotationAndWholeFileRetention(t *testing.T) {
	origEntries, origMax := ringSegmentEntries, ringMaxSegments
	ringSegmentEntries, ringMaxSegments = 5, 4
	t.Cleanup(func() { ringSegmentEntries, ringMaxSegments = origEntries, origMax })

	dir := filepath.Join(t.TempDir(), "ring")
	stepRingClock(t)
	// Write 30 entries -> 6 segments of 5; ringMaxSegments=4 deletes oldest 2 whole segments.
	for i := 0; i < 30; i++ {
		appendTexts(t, dir, i, i+1)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	segCount := 0
	for _, de := range entries {
		if _, ok := parseRingSegmentSeq(de.Name()); ok {
			segCount++
		}
		if isRingEntryName(de.Name()) {
			t.Fatalf("unexpected per-entry .json file: %s", de.Name())
		}
	}
	if segCount > ringMaxSegments {
		t.Fatalf("segment count = %d, want <= %d", segCount, ringMaxSegments)
	}
	page, err := ReadRingPage(dir, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) == 0 || page.Records[len(page.Records)-1].Text != "m29" {
		t.Fatalf("newest entry = %+v, want m29", page.Records)
	}
}

func TestRingTornTrailingLineRecovery(t *testing.T) {
	sessions := filepath.Join(t.TempDir(), "sessions")
	dir := filepath.Join(sessions, "torn", "ring")
	stepRingClock(t)
	appendTexts(t, dir, 0, 2)

	segPath := filepath.Join(dir, ringSegmentName(1))
	f, err := os.OpenFile(segPath, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"name":"20261106T090009.000000000Z-aaaa.json","entry":{"kind":"user","text":"torn`); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	closeCache := EnableRingCache(sessions)
	defer closeCache()
	// Read before write must ignore the torn tail.
	got, err := ReadRing(dir)
	if err != nil || len(got) != 2 || got[0].Text != "m0" || got[1].Text != "m1" {
		t.Fatalf("read with torn tail = %+v, %v", got, err)
	}
	// Next append must truncate the torn tail so the new entry parses cleanly.
	appendTexts(t, dir, 2, 3)
	if err := closeCache(); err != nil {
		t.Fatal(err)
	}
	reopened, err := ReadRing(dir)
	if err != nil || len(reopened) != 3 || reopened[2].Text != "m2" {
		t.Fatalf("reopen after torn tail = %+v, %v", reopened, err)
	}
}

func TestRingLegacyAndSegmentMergeCompatibility(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ring")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	// Pre-upgrade legacy entry.
	beforeName := base.Format(ringStampLayout) + "-aaaa.json"
	if err := os.WriteFile(filepath.Join(dir, beforeName), []byte(`{"kind":"user","text":"legacy-before"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// New segment entry between two legacy entries.
	if err := AppendRing(dir, []RingEntry{{ringStamp: base.Add(time.Second), Kind: RingReply, Text: "segment-middle"}}); err != nil {
		t.Fatal(err)
	}
	// Old binary (downgrade) ignores .jsonl segment files and writes another legacy .json file.
	if isRingEntryName(ringSegmentName(1)) {
		t.Fatal("legacy isRingEntryName matched segment file")
	}
	afterName := base.Add(2*time.Second).Format(ringStampLayout) + "-bbbb.json"
	if err := os.WriteFile(filepath.Join(dir, afterName), []byte(`{"kind":"user","text":"legacy-after"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRing(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Text != "legacy-before" || got[1].Text != "segment-middle" || got[2].Text != "legacy-after" {
		t.Fatalf("merged order = %+v", got)
	}
}

func TestRingPersistentCacheZeroDiskReadsAndSingleOpen(t *testing.T) {
	sessions := filepath.Join(t.TempDir(), "sessions")
	dir := filepath.Join(sessions, "cached", "ring")
	stepRingClock(t)
	closeCache := EnableRingCache(sessions)
	defer closeCache()

	// Warm initial segment open with first append.
	appendTexts(t, dir, 0, 1)

	beforeReadDir := ringIOCount("read_dir")
	beforeReadFile := ringIOCount("read_file")
	beforeOpen := ringIOCount("open_file")
	beforeCreateTemp := ringIOCount("create_temp")
	beforeRename := ringIOCount("atomic_replace")

	for i := 1; i < 40; i++ {
		appendTexts(t, dir, i, i+1)
	}
	for range 10 {
		recs, err := ReadRing(dir)
		if err != nil || len(recs) != 40 {
			t.Fatalf("ReadRing = %d, %v", len(recs), err)
		}
		if page, err := ReadRingPage(dir, recs[10].Name, 5); err != nil || len(page.Records) != 5 {
			t.Fatalf("ReadRingPage = %d, %v", len(page.Records), err)
		}
		if rec, ok := ReadRingEntry(dir, recs[20].Name); !ok || rec.Text != "m20" {
			t.Fatalf("ReadRingEntry = %+v, %v", rec, ok)
		}
		if names, err := RingNames(dir); err != nil || len(names) != 40 {
			t.Fatalf("RingNames = %d, %v", len(names), err)
		}
	}
	if got := ringIOCount("read_dir") - beforeReadDir; got != 0 {
		t.Fatalf("steady-state ring appends/reads made %d read_dir calls, want 0", got)
	}
	if got := ringIOCount("read_file") - beforeReadFile; got != 0 {
		t.Fatalf("steady-state ring appends/reads made %d read_file calls, want 0", got)
	}
	if got := ringIOCount("open_file") - beforeOpen; got != 0 {
		t.Fatalf("steady-state ring appends made %d open_file calls, want 0", got)
	}
	if got := ringIOCount("create_temp") - beforeCreateTemp; got != 0 {
		t.Fatalf("steady-state ring appends made %d create_temp calls, want 0", got)
	}
	if got := ringIOCount("atomic_replace") - beforeRename; got != 0 {
		t.Fatalf("steady-state ring appends made %d atomic_replace calls, want 0", got)
	}

	// Graceful close + reopen recovers every entry with zero loss.
	if err := closeCache(); err != nil {
		t.Fatal(err)
	}
	reopened, err := ReadRing(dir)
	if err != nil || len(reopened) != 40 || reopened[0].Text != "m0" || reopened[39].Text != "m39" {
		t.Fatalf("reopened = %d (%v)", len(reopened), err)
	}
}
