package wslive

import (
	"errors"
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
	unpin := PinRing(dir)
	defer unpin()
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
	if err := unpin(); err != nil {
		t.Fatal(err)
	}
	// Unpinned legacy entry files must also be 0600.
	if err := AppendRing(dir, []RingEntry{{Kind: RingUser, Text: "unpinned private"}}); err != nil {
		t.Fatal(err)
	}
	names, err = RingNames(dir)
	if err != nil || len(names) != 2 {
		t.Fatalf("ring names after unpinned = %v, %v", names, err)
	}
	info, err = os.Stat(filepath.Join(dir, names[1]))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("ring legacy entry mode = %o, want 600", perm)
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
	unpin := PinRing(dir)
	defer unpin()
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
	dir := filepath.Join(t.TempDir(), "sessions", "torn", "ring")
	stepRingClock(t)
	unpin := PinRing(dir)
	appendTexts(t, dir, 0, 2)
	if err := unpin(); err != nil {
		t.Fatal(err)
	}

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

	unpin = PinRing(dir)
	defer unpin()
	// Read before write must ignore the torn tail.
	got, err := ReadRing(dir)
	if err != nil || len(got) != 2 || got[0].Text != "m0" || got[1].Text != "m1" {
		t.Fatalf("read with torn tail = %+v, %v", got, err)
	}
	// Next append must truncate the torn tail so the new entry parses cleanly.
	appendTexts(t, dir, 2, 3)
	if err := unpin(); err != nil {
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
	// New pinned segment entry between two legacy entries.
	unpin := PinRing(dir)
	if err := AppendRing(dir, []RingEntry{{ringStamp: base.Add(time.Second), Kind: RingReply, Text: "segment-middle"}}); err != nil {
		t.Fatal(err)
	}
	if err := unpin(); err != nil {
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
	unpin := PinRing(dir)
	defer unpin()

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
		if fp, err := RingFingerprint(dir); err != nil || len(fp) == 0 {
			t.Fatalf("RingFingerprint = %v, %v", fp, err)
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

	// Graceful unpin + reopen recovers every entry with zero loss.
	if err := UnpinRingsUnder(sessions); err != nil {
		t.Fatal(err)
	}
	if RingPinned(dir) {
		t.Fatal("UnpinRingsUnder left ring pinned")
	}
	reopened, err := ReadRing(dir)
	if err != nil || len(reopened) != 40 || reopened[0].Text != "m0" || reopened[39].Text != "m39" {
		t.Fatalf("reopened = %d (%v)", len(reopened), err)
	}
}

// TestRingExternalAppendAndUnpinnedNoHeldFD covers R1 Finding 4: external
// writers write atomic legacy .json files without touching seg-*.jsonl,
// SyncExternalRing merges them into a pinned RAM ring without closing the
// held segment writer, and unpinned appends never leak open handles.
func TestRingExternalAppendAndUnpinnedNoHeldFD(t *testing.T) {
	base := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	dir := filepath.Join(t.TempDir(), "sessions", "hosted", "ring")

	// Pinned hosted writer appends to seg-1.jsonl.
	unpin := PinRing(dir)
	if err := AppendRing(dir, []RingEntry{{ringStamp: base, Kind: RingUser, Text: "hosted-1"}}); err != nil {
		t.Fatal(err)
	}
	segPath := filepath.Join(dir, ringSegmentName(1))
	segBefore, err := os.ReadFile(segPath)
	if err != nil {
		t.Fatal(err)
	}

	// External process appends a legacy .json entry without touching seg-1.jsonl.
	extName := base.Add(time.Second).Format(ringStampLayout) + "-eeee.json"
	if err := writeLegacyRingRaw(dir, extName, []byte(`{"kind":"mail","text":"external-mail"}`)); err != nil {
		t.Fatal(err)
	}
	segAfterExt, err := os.ReadFile(segPath)
	if err != nil || string(segAfterExt) != string(segBefore) {
		t.Fatalf("external append disturbed segment file: %v", err)
	}

	// SyncExternalRing merges external entry into pinned RAM without closing fd.
	SyncExternalRing(dir)
	if err := AppendRing(dir, []RingEntry{{ringStamp: base.Add(2 * time.Second), Kind: RingReply, Text: "hosted-2"}}); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRing(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Text != "hosted-1" || got[1].Text != "external-mail" || got[2].Text != "hosted-2" {
		t.Fatalf("merged pinned + external = %+v", got)
	}
	if err := unpin(); err != nil {
		t.Fatal(err)
	}

	// Unpinned caller appends legacy .json, holds no fd, pins nothing, and RemoveAll succeeds.
	unpinnedDir := filepath.Join(t.TempDir(), "sessions", "unpinned", "ring")
	if err := AppendRing(unpinnedDir, []RingEntry{{Kind: RingMail, Text: "non-slot"}}); err != nil {
		t.Fatal(err)
	}
	if RingPinned(unpinnedDir) {
		t.Fatal("unpinned AppendRing pinned ring in RAM")
	}
	if err := os.RemoveAll(filepath.Dir(unpinnedDir)); err != nil {
		t.Fatalf("RemoveAll after unpinned AppendRing failed (leaked fd?): %v", err)
	}
}

// TestRingTransientNewestSegmentReadErrorNoFallbackToSeg1 covers R1 Finding 5:
// a transient read error on the newest segment must fail load rather than
// leaving segSeq=0 and reopening seg-1.jsonl.
func TestRingTransientNewestSegmentReadErrorNoFallbackToSeg1(t *testing.T) {
	origEntries := ringSegmentEntries
	ringSegmentEntries = 2
	t.Cleanup(func() { ringSegmentEntries = origEntries })

	dir := filepath.Join(t.TempDir(), "sessions", "errseg", "ring")
	stepRingClock(t)
	unpin := PinRing(dir)
	appendTexts(t, dir, 0, 3) // seg-1 (2 entries) + seg-2 (1 entry)
	if err := unpin(); err != nil {
		t.Fatal(err)
	}
	seg1Before, err := os.ReadFile(filepath.Join(dir, ringSegmentName(1)))
	if err != nil {
		t.Fatal(err)
	}

	// Inject transient read failure on seg-2.
	origRead := ringReadFile
	failSeg2 := true
	ringReadFile = func(path string) ([]byte, error) {
		if failSeg2 && filepath.Base(path) == ringSegmentName(2) {
			return nil, errors.New("transient AV lock")
		}
		return origRead(path)
	}
	t.Cleanup(func() { ringReadFile = origRead })

	unpin = PinRing(dir)
	defer unpin()
	if err := AppendRing(dir, []RingEntry{{Kind: RingUser, Text: "during-err"}}); err == nil {
		t.Fatal("AppendRing succeeded despite unreadable newest segment")
	}
	seg1After, err := os.ReadFile(filepath.Join(dir, ringSegmentName(1)))
	if err != nil || string(seg1After) != string(seg1Before) {
		t.Fatalf("transient newest-segment error fell back to rewriting seg-1: %v", err)
	}

	// Once transient error clears, load recovers cleanly and appends to seg-2.
	failSeg2 = false
	if err := AppendRing(dir, []RingEntry{{Kind: RingUser, Text: "after-recovery"}}); err != nil {
		t.Fatalf("AppendRing after recovery: %v", err)
	}
	got, err := ReadRing(dir)
	if err != nil || len(got) != 4 || got[3].Text != "after-recovery" {
		t.Fatalf("ring after recovery = %+v, %v", got, err)
	}
}

// TestRingTruncateErrorRotatesInsteadOfGluingTornTail covers R1 Finding 5:
// if Truncate fails on a torn trailing line, the writer rotates to a fresh
// segment rather than gluing the next record onto the torn tail.
func TestRingTruncateErrorRotatesInsteadOfGluingTornTail(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions", "truncer", "ring")
	stepRingClock(t)
	unpin := PinRing(dir)
	appendTexts(t, dir, 0, 2)
	if err := unpin(); err != nil {
		t.Fatal(err)
	}
	seg1 := filepath.Join(dir, ringSegmentName(1))
	f, err := os.OpenFile(seg1, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"name":"20261106T090009.000000000Z-aaaa.json","entry":{"kind":"user","text":"torn`); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	origTrunc := ringTruncate
	ringTruncate = func(string, int64) error { return errors.New("truncate denied") }
	t.Cleanup(func() { ringTruncate = origTrunc })

	unpin = PinRing(dir)
	appendTexts(t, dir, 2, 3)
	if err := unpin(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ringSegmentName(2))); err != nil {
		t.Fatalf("expected rotation to seg-2 when Truncate failed: %v", err)
	}
	got, err := ReadRing(dir)
	if err != nil || len(got) != 3 || got[0].Text != "m0" || got[1].Text != "m1" || got[2].Text != "m2" {
		t.Fatalf("reopened after truncate-denied rotation = %+v, %v", got, err)
	}
}

// TestRingUnpinnedClosedSessionDoesNotPinOrReadBodies covers R1 Finding 6:
// RingFingerprint and RingNames on unpinned/closed dirs never pin RAM entries
// and RingFingerprint never opens or reads file bodies while still detecting
// segment growth and legacy entry additions.
func TestRingUnpinnedClosedSessionDoesNotPinOrReadBodies(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions", "closed", "ring")
	stepRingClock(t)
	unpin := PinRing(dir)
	appendTexts(t, dir, 0, 3)
	if err := unpin(); err != nil {
		t.Fatal(err)
	}
	beforePinned := pinnedRingCount()
	beforeReadFile := ringIOCount("read_file")

	fp1, err := RingFingerprint(dir)
	if err != nil || len(fp1) == 0 {
		t.Fatalf("RingFingerprint 1 = %v, %v", fp1, err)
	}
	if got := ringIOCount("read_file") - beforeReadFile; got != 0 {
		t.Fatalf("RingFingerprint read %d files, want 0", got)
	}
	if pinnedRingCount() != beforePinned {
		t.Fatalf("RingFingerprint pinned closed ring in RAM")
	}

	// External legacy append changes fingerprint with 0 body reads.
	appendTexts(t, dir, 3, 4)
	fp2, err := RingFingerprint(dir)
	if err != nil || strings.Join(fp1, "|") == strings.Join(fp2, "|") {
		t.Fatalf("RingFingerprint did not change after append: %v vs %v (%v)", fp1, fp2, err)
	}
	if got := ringIOCount("read_file") - beforeReadFile; got != 0 {
		t.Fatalf("RingFingerprint after append read %d files, want 0", got)
	}

	// RingNames on unpinned dir never pins RAM entries.
	names, err := RingNames(dir)
	if err != nil || len(names) != 4 {
		t.Fatalf("RingNames = %d, %v", len(names), err)
	}
	if pinnedRingCount() != beforePinned {
		t.Fatalf("RingNames pinned closed ring in RAM")
	}

	// Legacy-only unpinned dir: RingNames reads 0 file bodies.
	legacyDir := filepath.Join(t.TempDir(), "sessions", "legacy-closed", "ring")
	appendTexts(t, legacyDir, 0, 5)
	beforeLegacyRead := ringIOCount("read_file")
	legacyNames, err := RingNames(legacyDir)
	if err != nil || len(legacyNames) != 5 {
		t.Fatalf("legacy RingNames = %d, %v", len(legacyNames), err)
	}
	if got := ringIOCount("read_file") - beforeLegacyRead; got != 0 {
		t.Fatalf("legacy RingNames read %d file bodies, want 0", got)
	}
}

// TestRingUnpinnedReadPageWindowBoundedReads covers R2 Finding 1: unpinned
// ReadRingPage reads only the legacy .json files (< before, newest n) or
// newest-first segments needed for its window, and unpinned ReadRingEntry
// stops on the first matching segment without loading the whole ring.
func TestRingUnpinnedReadPageWindowBoundedReads(t *testing.T) {
	t.Run("legacy-only-window", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "ring")
		stepRingClock(t)
		appendTexts(t, dir, 0, 200) // unpinned -> 200 legacy .json files

		beforeRead := ringIOCount("read_file")
		p1, err := ReadRingPage(dir, "", 10)
		if err != nil || len(p1.Records) != 10 || p1.End {
			t.Fatalf("p1 = %d end=%v err=%v", len(p1.Records), p1.End, err)
		}
		if p1.Records[0].Text != "m190" || p1.Records[9].Text != "m199" {
			t.Fatalf("p1 range = %q..%q", p1.Records[0].Text, p1.Records[9].Text)
		}
		if got := ringIOCount("read_file") - beforeRead; got > 10 {
			t.Fatalf("unpinned ReadRingPage(10) on 200 legacy files made %d read_file calls, want <= 10", got)
		}

		beforeRead2 := ringIOCount("read_file")
		p2, err := ReadRingPage(dir, p1.Before, 10)
		if err != nil || len(p2.Records) != 10 || p2.End {
			t.Fatalf("p2 = %d end=%v err=%v", len(p2.Records), p2.End, err)
		}
		if p2.Records[0].Text != "m180" || p2.Records[9].Text != "m189" {
			t.Fatalf("p2 range = %q..%q", p2.Records[0].Text, p2.Records[9].Text)
		}
		if got := ringIOCount("read_file") - beforeRead2; got > 10 {
			t.Fatalf("unpinned older ReadRingPage(10) made %d read_file calls, want <= 10", got)
		}
	})

	t.Run("segments-newest-first-and-legacy-skip", func(t *testing.T) {
		origEntries, origMax := ringSegmentEntries, ringMaxSegments
		ringSegmentEntries, ringMaxSegments = 5, 16
		t.Cleanup(func() { ringSegmentEntries, ringMaxSegments = origEntries, origMax })

		dir := filepath.Join(t.TempDir(), "ring")
		stepRingClock(t)
		// 50 older legacy .json files, then 30 pinned segment entries across 6 segments.
		appendTexts(t, dir, 0, 50)
		unpin := PinRing(dir)
		appendTexts(t, dir, 50, 80)
		if err := unpin(); err != nil {
			t.Fatal(err)
		}

		// Requesting newest 4 entries must read only the newest 1 segment and 0 legacy files.
		beforeRead := ringIOCount("read_file")
		page, err := ReadRingPage(dir, "", 4)
		if err != nil || len(page.Records) != 4 || page.End {
			t.Fatalf("page = %d end=%v err=%v", len(page.Records), page.End, err)
		}
		if page.Records[0].Text != "m76" || page.Records[3].Text != "m79" {
			t.Fatalf("page range = %q..%q", page.Records[0].Text, page.Records[3].Text)
		}
		if got := ringIOCount("read_file") - beforeRead; got != 1 {
			t.Fatalf("unpinned ReadRingPage(4) on 6 segments + 50 legacy made %d read_file calls, want 1", got)
		}

		// Unpinned ReadRingEntry for the newest entry must read only 1 segment (0 legacy files).
		beforeEntry := ringIOCount("read_file")
		rec, ok := ReadRingEntry(dir, page.Records[3].Name)
		if !ok || rec.Text != "m79" {
			t.Fatalf("ReadRingEntry = %+v, %v", rec, ok)
		}
		// readLegacyRingEntry tries the .json path once (ErrNotExist -> counted by ReadFileOnce),
		// then scans newest segment once -> at most 2 read_file ops total, never 6+50.
		if got := ringIOCount("read_file") - beforeEntry; got > 2 {
			t.Fatalf("unpinned ReadRingEntry made %d read_file calls, want <= 2", got)
		}
	})
}

// TestRingFileIOCountersAreLive covers R2 Finding 2: open_file, create_temp,
// and atomic_replace wsstate counters are wired to real ring writes so
// zero-op steady-state assertions are non-vacuous.
func TestRingFileIOCountersAreLive(t *testing.T) {
	// Pinned first append must increment open_file by 1; steady-state adds 0.
	pinnedDir := filepath.Join(t.TempDir(), "pinned", "ring")
	stepRingClock(t)
	unpin := PinRing(pinnedDir)
	defer unpin()

	openBefore := ringIOCount("open_file")
	appendTexts(t, pinnedDir, 0, 1)
	if got := ringIOCount("open_file") - openBefore; got != 1 {
		t.Fatalf("first pinned AppendRing recorded open_file=%d, want 1 (counter bypassed?)", got)
	}
	openWarm := ringIOCount("open_file")
	tempWarm := ringIOCount("create_temp")
	renameWarm := ringIOCount("atomic_replace")
	for i := 1; i < 20; i++ {
		appendTexts(t, pinnedDir, i, i+1)
	}
	if got := ringIOCount("open_file") - openWarm; got != 0 {
		t.Fatalf("steady-state pinned appends recorded open_file=%d, want 0", got)
	}
	if got := ringIOCount("create_temp") - tempWarm; got != 0 {
		t.Fatalf("steady-state pinned appends recorded create_temp=%d, want 0", got)
	}
	if got := ringIOCount("atomic_replace") - renameWarm; got != 0 {
		t.Fatalf("steady-state pinned appends recorded atomic_replace=%d, want 0", got)
	}

	// Unpinned append must increment create_temp and atomic_replace by 1 each.
	unpinnedDir := filepath.Join(t.TempDir(), "unpinned", "ring")
	tempBefore := ringIOCount("create_temp")
	renameBefore := ringIOCount("atomic_replace")
	appendTexts(t, unpinnedDir, 0, 1)
	if got := ringIOCount("create_temp") - tempBefore; got != 1 {
		t.Fatalf("unpinned AppendRing recorded create_temp=%d, want 1 (counter bypassed?)", got)
	}
	if got := ringIOCount("atomic_replace") - renameBefore; got != 1 {
		t.Fatalf("unpinned AppendRing recorded atomic_replace=%d, want 1 (counter bypassed?)", got)
	}
}
