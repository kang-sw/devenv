package mcp

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func nudgeCacheFixture(t *testing.T) (*WhiteboardNudges, nudgeTarget, string) {
	t.Helper()
	n := NewWhiteboardNudges(filepath.Join(t.TempDir(), "dashboard"), nil, io.Discard)
	target := nudgeTarget{"cache-nudge-lead", t.TempDir()}
	path := n.dirs().partPath(target.key, partKey)
	nudgeWriteJSON(t, path, sessionKeyRecord{SchemaVersion: 2, Scope: string(roleLead), Root: target.root})
	if err := os.Chtimes(path, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(whiteboardFolder(target.root, target.key).Dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(whiteboardFolder(target.root, target.key).Dir, "meta.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	return n, target, path
}

func TestWhiteboardNudgeCacheMetadataAndAuthority(t *testing.T) {
	n, target, path := nudgeCacheFixture(t)
	var reads, stats int
	n.read = func(p string, value func() any) ([]byte, any, error) { reads++; return readRecordFile(p, value) }
	n.stat = func(p string) (os.FileInfo, error) { stats++; return os.Stat(p) }
	want := map[nudgeTarget]time.Time{target: {}}
	check := func(label string, expected map[nudgeTarget]time.Time) {
		t.Helper()
		if got := n.discover(); !reflect.DeepEqual(got, expected) {
			t.Fatalf("%s: got %v, want %v", label, got, expected)
		}
	}
	check("cold", want)
	if reads != 1 || len(n.identities) != 1 {
		t.Fatalf("cold: reads=%d cache=%d", reads, len(n.identities))
	}
	check("warm", want)
	if reads != 1 || stats < 2 {
		t.Fatalf("warm: reads=%d metadata-stats=%d", reads, stats)
	}
	// Change the same file without changing its length. Directory mtime is not a stamp.
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	oldInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := []byte(strings.Replace(string(original), `"lead"`, `"xxxx"`, 1))
	if len(changed) != len(original) {
		t.Fatal("in-place fixture changed size")
	}
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	// Both stamps are quiet but distinct: the mtime check (not the recent
	// metadata guard) must detect this same-inode, same-size edit.
	if err := os.Chtimes(path, oldInfo.ModTime().Add(time.Second), oldInfo.ModTime().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	beforeReads := reads
	check("stable same-size in-place change", nilMap())
	if reads != beforeReads+1 || n.identities[target.key].key.Scope != "xxxx" {
		t.Fatal("same-size write retained stale identity")
	}
	// Restore lead with an old timestamp, then replace by rename with the same size
	// and mtime. File identity, not just size/mtime, must invalidate the hit.
	nudgeWriteJSON(t, path, sessionKeyRecord{SchemaVersion: 2, Scope: string(roleLead), Root: target.root})
	check("recent restoration", want)
	if len(n.identities) != 0 {
		t.Fatal("recent metadata cached")
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	check("restored", want)
	before, _ := os.Stat(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tmp := path + ".replacement"
	if err := os.WriteFile(tmp, []byte(strings.Replace(string(data), `"lead"`, `"xxxx"`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(tmp, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	beforeReads = reads
	check("equal-size replacement", nilMap())
	if reads != beforeReads+1 || n.identities[target.key].key.Scope != "xxxx" {
		t.Fatal("replacement retained old authority or skipped content reread")
	}
	// Even a cached hit may never validate an enqueue; the delivery identity
	// reader is independent of the injected discovery reader.
	if _, ok := n.identity(target); ok {
		t.Fatal("delivery accepted replaced non-lead")
	}
}

func nilMap() map[nudgeTarget]time.Time { return map[nudgeTarget]time.Time{} }

func TestWhiteboardNudgeCacheFallbackErrorsAndRaces(t *testing.T) {
	n, target, path := nudgeCacheFixture(t)
	want := map[nudgeTarget]time.Time{target: {}}
	check := func(label string, expected map[nudgeTarget]time.Time) {
		t.Helper()
		if got := n.discover(); !reflect.DeepEqual(got, expected) {
			t.Fatalf("%s: got %v, want %v", label, got, expected)
		}
	}
	check("cold", want)
	n.stat = func(p string) (os.FileInfo, error) {
		if p == path {
			return nil, errors.New("transient stat")
		}
		return os.Stat(p)
	}
	check("stat error", nilMap())
	n.stat = nil
	check("stat recovery", want)
	// Force a metadata miss; failed reads must not leave a valid cached entry.
	if err := os.Chtimes(path, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	n.read = func(string, func() any) ([]byte, any, error) { return nil, nil, errors.New("transient read") }
	check("read error", nilMap())
	n.read = nil
	check("read recovery", want)
	if len(n.identities) != 0 {
		t.Fatal("recent identity cached")
	}
	// A replacement during read cannot be stamped with the old metadata.
	nudgeWriteJSON(t, path, sessionKeyRecord{SchemaVersion: 2, Scope: string(roleLead), Root: target.root, Parent: "x"})
	n.read = func(p string, value func() any) ([]byte, any, error) {
		data, v, err := readRecordFile(p, value)
		if err == nil {
			nudgeWriteJSON(t, p, sessionKeyRecord{SchemaVersion: 2, Scope: string(roleLead), Root: target.root})
			future := time.Now().Add(time.Hour)
			if err := os.Chtimes(p, future, future); err != nil {
				t.Fatal(err)
			}
		}
		return data, v, err
	}
	check("during-read write", nilMap())
	n.read = nil
	check("after-race", want)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	legacy := n.dirs().legacyPath(target.key)
	nudgeWriteJSON(t, legacy, sessionRecord{sessionKeyRecord: sessionKeyRecord{Scope: string(roleLead), Root: target.root}, Whiteboard: &wswhiteboard.State{}})
	check("legacy fallback", want)
	if len(n.identities) != 0 {
		t.Fatal("recent legacy board cached")
	}
	if err := os.Chtimes(legacy, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	check("stable legacy", want)
	if len(n.identities) != 1 {
		t.Fatal("stable legacy not cached")
	}
	nudgeWriteJSON(t, path, sessionKeyRecord{SchemaVersion: 999, Scope: string(roleLead), Root: target.root})
	check("unsupported split shadows legacy", nilMap())
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	check("migration back to legacy", want)
	if err := os.WriteFile(legacy, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	check("corrupt legacy", nilMap())
	if err := os.Remove(legacy); err != nil {
		t.Fatal(err)
	}
	check("deleted", nilMap())
	if len(n.identities) != 0 {
		t.Fatal("deleted key retained")
	}
}

func TestWhiteboardNudgeCacheCollisionIntroducedAndRemoved(t *testing.T) {
	// Exercise the association gate with a synthetic colliding association:
	// finding a real four-word hash collision is not a deterministic fixture.
	target := nudgeTarget{"collision-lead", t.TempDir()}
	id := wsmailbox.ReplyID(target.key)
	for _, count := range []int{1, 2, 1} {
		out := map[nudgeTarget]time.Time{target: {}}
		nudgeSuppressCollisions(out, map[string]int{id: count})
		if (len(out) == 1) != (count == 1) {
			t.Fatalf("association count %d: %v", count, out)
		}
	}
}

func TestWhiteboardNudgeCacheScopeRootAndParentChanges(t *testing.T) {
	n, target, path := nudgeCacheFixture(t)
	want := map[nudgeTarget]time.Time{target: {}}
	if got := n.discover(); !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	for _, record := range []sessionKeyRecord{
		{SchemaVersion: 2, Scope: string(roleDelegate), Root: target.root},
		{SchemaVersion: 2, Scope: string(roleLead), Root: target.root, Parent: "other"},
		{SchemaVersion: 2, Scope: string(roleLead), Root: "relative"},
	} {
		nudgeWriteJSON(t, path, record)
		got := n.discover()
		if len(got) != 0 {
			t.Fatalf("changed identity: %v", got)
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got := n.discover(); len(got) != 0 || len(n.identities) != 0 {
		t.Fatal("removed identity", got)
	}
}

// Mirror the pre-cache discover loop in the benchmark, including listings,
// board existence, and association suppression. Only the identity reads differ.
func oldNudgeDiscoveryForBenchmark(b *testing.B, n *WhiteboardNudges) map[nudgeTarget]time.Time {
	b.Helper()
	d := n.dirs()
	keys := map[string]bool{}
	for i, dir := range []string{d.sessions, d.legacy} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			b.Fatal(err)
		}
		for _, e := range entries {
			key := e.Name()
			if i == 0 && !e.IsDir() || i == 1 && e.IsDir() {
				continue
			}
			if i == 1 {
				var ok bool
				key, ok = strings.CutSuffix(key, ".json")
				if !ok {
					continue
				}
			}
			if sessionKeyFilenamePattern.MatchString(key) {
				keys[key] = true
			}
		}
	}
	out := map[nudgeTarget]time.Time{}
	associations := map[string]int{}
	for key := range keys {
		r, _, err := loadRecordWith(d, key, partKey, nil, n.read)
		if err != nil || r.Scope != string(roleLead) || r.Parent != "" || !filepath.IsAbs(r.Root) {
			continue
		}
		associations[wsmailbox.ReplyID(key)]++
		target := nudgeTarget{key, r.Root}
		for _, name := range []string{"meta.json", "pending.json", "committed.json"} {
			if _, err := n.stat(filepath.Join(whiteboardFolder(target.root, target.key).Dir, name)); err == nil {
				out[target] = time.Time{}
				break
			}
		}
	}
	nudgeSuppressCollisions(out, associations)
	return out
}

// These are source-level content/stat counters, not physical disk-I/O or
// process CPU measurements. Stats include identity and board metadata.
func BenchmarkWhiteboardNudgeDiscovery3500(b *testing.B) {
	cache, root := b.TempDir(), b.TempDir()
	n := NewWhiteboardNudges(filepath.Join(cache, "dashboard"), nil, io.Discard)
	d := n.dirs()
	if err := os.MkdirAll(d.sessions, 0700); err != nil {
		b.Fatal(err)
	}
	if err := os.MkdirAll(d.legacy, 0700); err != nil {
		b.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	keys := make([]string, 3500)
	for i := range keys {
		key := fmt.Sprintf("bench-nudge-%04d", i)
		keys[i] = key
		rec := sessionKeyRecord{SchemaVersion: 2, Scope: string(roleDelegate), Root: root}
		if i%9 == 0 {
			rec.Scope = string(roleLead)
		} else if i%3 == 0 {
			rec.Parent = "bench-parent"
			rec.Scope = string(roleLead)
		}
		path := d.legacyPath(key)
		if i >= 2644 {
			path = d.partPath(key, partKey)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			b.Fatal(err)
		}
		data := fmt.Appendf(nil, `{"schema_version":%d,"scope":%q,"root":%q,"parent":%q}`, rec.SchemaVersion, rec.Scope, rec.Root, rec.Parent)
		if err := os.WriteFile(path, data, 0600); err != nil {
			b.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			b.Fatal(err)
		}
	}
	var reads, stats int
	n.read = func(p string, value func() any) ([]byte, any, error) {
		data, v, err := readRecordFile(p, value)
		if err == nil {
			reads++
		}
		return data, v, err
	}
	n.stat = func(p string) (os.FileInfo, error) { stats++; return os.Stat(p) }
	n.discover() // warm; exclude fixture population and first read
	reads, stats = 0, 0
	n.discover()
	if reads > 350 {
		b.Fatalf("warm content reads %d/3500 exceed 10%%", reads)
	}
	b.Run("cached-warm", func(b *testing.B) {
		b.ReportAllocs()
		reads, stats = 0, 0
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			n.discover()
		}
		b.StopTimer()
		b.ReportMetric(float64(reads)/float64(b.N), "content-reads/op")
		b.ReportMetric(float64(stats)/float64(b.N), "metadata-stats/op")
	})
	b.Run("old-full-reread", func(b *testing.B) {
		b.ReportAllocs()
		reads, stats = 0, 0
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			oldNudgeDiscoveryForBenchmark(b, n)
		}
		b.StopTimer()
		b.ReportMetric(float64(reads)/float64(b.N), "content-reads/op")
		b.ReportMetric(float64(stats)/float64(b.N), "metadata-stats/op")
	})
}

func TestWhiteboardNudgeCacheSmallWarmReads(t *testing.T) {
	// The benchmark enforces the representative 3500-key reduction; this
	// smaller fixture pins the per-key hit without OS file-cache assumptions.
	n, target, _ := nudgeCacheFixture(t)
	var reads int
	n.read = func(p string, value func() any) ([]byte, any, error) { reads++; return readRecordFile(p, value) }
	n.discover()
	n.discover()
	if reads != 1 || len(n.identities) != 1 || target.root == "" {
		t.Fatalf("warm content reads=%d", reads)
	}
}

// TestWhiteboardNudgeDiscoveryListedLegacyIdentity: a legacy-only key's
// identity comes from discovery's listing. Before, a warm recovery pass made
// two key stats per legacy-only key (failed split, legacy) and a cold one four
// plus a failed split read; a capture's 5-minute pass over ~4,070 keys made
// 6,670 key stats. Now a warm pass makes none for such keys, and a cold one
// only the after-read check's two and one read.
func TestWhiteboardNudgeDiscoveryListedLegacyIdentity(t *testing.T) {
	n, target, splitPath := nudgeCacheFixture(t)
	d := n.dirs()
	const legacyKeys = 10
	old := time.Now().Add(-time.Hour)
	for i := 0; i < legacyKeys; i++ {
		path := d.legacyPath(fmt.Sprintf("legacy-nudge-%02d", i))
		nudgeWriteJSON(t, path, sessionRecord{sessionKeyRecord: sessionKeyRecord{SchemaVersion: 2, Scope: string(roleDelegate), Root: target.root}})
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	var legacyStats, splitStats, legacyReads, splitReads int
	n.stat = func(p string) (os.FileInfo, error) {
		switch {
		case strings.HasPrefix(filepath.Base(filepath.Dir(p)), "legacy-nudge-") || strings.HasPrefix(filepath.Base(p), "legacy-nudge-"):
			legacyStats++
		case p == splitPath:
			splitStats++
		}
		return os.Stat(p)
	}
	n.read = func(p string, value func() any) ([]byte, any, error) {
		if strings.Contains(p, "legacy-nudge-") {
			legacyReads++
		} else if p == splitPath {
			splitReads++
		}
		return readRecordFile(p, value)
	}
	want := map[nudgeTarget]time.Time{target: {}}
	if got := n.discover(); !reflect.DeepEqual(got, want) {
		t.Fatalf("cold: got %v, want %v", got, want)
	}
	if len(n.known) != legacyKeys+1 || len(n.identities) != legacyKeys+1 {
		t.Fatalf("cold: known=%d cached=%d, want %d", len(n.known), len(n.identities), legacyKeys+1)
	}
	if legacyStats != 2*legacyKeys || legacyReads != legacyKeys {
		t.Fatalf("cold: legacy stats=%d reads=%d, want %d and %d (was %d and %d)", legacyStats, legacyReads, 2*legacyKeys, legacyKeys, 4*legacyKeys, 2*legacyKeys)
	}
	if splitStats != 2 || splitReads != 1 {
		t.Fatalf("cold: split stats=%d reads=%d, want 2 and 1", splitStats, splitReads)
	}
	legacyStats, splitStats, legacyReads, splitReads = 0, 0, 0, 0
	if got := n.discover(); !reflect.DeepEqual(got, want) {
		t.Fatalf("warm: got %v, want %v", got, want)
	}
	if legacyStats != 0 || legacyReads != 0 || splitStats != 1 || splitReads != 0 {
		t.Fatalf("warm: legacy stats=%d reads=%d split stats=%d reads=%d, want 0, 0, 1, 0 (legacy stats were %d)", legacyStats, legacyReads, splitStats, splitReads, 2*legacyKeys)
	}
	// A listed legacy change is still seen: an edit moves its listed stamp.
	path := d.legacyPath("legacy-nudge-00")
	nudgeWriteJSON(t, path, sessionRecord{sessionKeyRecord: sessionKeyRecord{SchemaVersion: 2, Scope: string(roleLead), Root: target.root, Parent: "x"}})
	if err := os.Chtimes(path, old.Add(time.Minute), old.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	n.discover()
	if n.known["legacy-nudge-00"].Parent != "x" || legacyReads != 1 {
		t.Fatalf("listed legacy edit missed: reads=%d identity=%+v", legacyReads, n.known["legacy-nudge-00"])
	}
	// A split file created for a listed legacy key still takes precedence:
	// its directory is listed, so the identity takes the stat path.
	split := d.partPath("legacy-nudge-01", partKey)
	nudgeWriteJSON(t, split, sessionKeyRecord{SchemaVersion: 2, Scope: string(roleLead), Root: target.root, Parent: "split"})
	if err := os.Chtimes(split, old, old); err != nil {
		t.Fatal(err)
	}
	n.discover()
	if n.known["legacy-nudge-01"].Parent != "split" || n.identities["legacy-nudge-01"].path != split {
		t.Fatalf("split did not shadow listed legacy: identity=%+v", n.identities["legacy-nudge-01"])
	}
}
