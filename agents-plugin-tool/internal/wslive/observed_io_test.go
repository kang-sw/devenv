package wslive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

func TestObservedCacheMinuteDoesNotRestatRecords(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		r := observedCacheRecord(time.Unix(int64(i), 0).String(), "model")
		if err := AppendObserved(dir, []ObservedRecord{r}); err != nil {
			t.Fatal(err)
		}
	}
	clock := time.Now().Add(time.Hour)
	recordStats, stats := 0, 0
	cache := ObservedCache{now: func() time.Time { return clock }, stat: func(path string) (os.FileInfo, error) {
		stats++
		if filepath.Base(path) != ObservedDirName {
			recordStats++
		}
		return os.Stat(path)
	}}
	cache.Load(dir)
	cold := recordStats
	beforeStats, beforeOpens := stats, cache.Opens()
	for i := 0; i < 60; i++ {
		clock = clock.Add(time.Second)
		cache.Load(dir)
	}
	if recordStats != cold {
		t.Fatalf("minute re-statted %d immutable records, want 0", recordStats-cold)
	}
	if delta := stats - beforeStats; delta != 60 {
		t.Fatalf("minute made %d stats, want 60 directory stats", delta)
	}
	if delta := cache.Opens() - beforeOpens; delta != 0 {
		t.Fatalf("minute opened %d records, want 0", delta)
	}
}

// observedCacheStats sums the counted stats observed_cache.go made: the
// evidence directory's own and any record's.
func observedCacheStats() int64 {
	var n int64
	for site, count := range wsstate.FileIOCounters() {
		if strings.HasPrefix(site, "file_io_site:wslive/observed_cache.go:") && strings.HasSuffix(site, ":stat") {
			n += count
		}
	}
	return n
}

// TestObservedCacheSweepUsesListedMetadata: cold intake and the 5-minute
// forced re-sweep take a regular record's metadata from the listing. Before,
// each made one path stat per record (the dominant 5-minute wave in a
// session-switch capture: 6,722 stats in 60 s); now each load makes only the
// directory stat. A symlinked record is still stat'ed, following it.
func TestObservedCacheSweepUsesListedMetadata(t *testing.T) {
	dir := t.TempDir()
	const records = 20
	for i := 0; i < records; i++ {
		r := observedCacheRecord(time.Unix(int64(i), 0).String(), "model")
		r.At = int64(1000 + i)
		if err := AppendObserved(dir, []ObservedRecord{r}); err != nil {
			t.Fatal(err)
		}
	}
	clock := time.Now().Add(time.Hour)
	cache := ObservedCache{now: func() time.Time { return clock }}
	before := observedCacheStats()
	cache.Load(dir)
	if got := observedCacheStats() - before; got != 1 {
		t.Fatalf("cold load made %d stats, want 1 directory stat (was %d)", got, records+1)
	}
	if cache.Opens() != records {
		t.Fatalf("cold load opened %d records, want %d", cache.Opens(), records)
	}
	before = observedCacheStats()
	clock = clock.Add(observedResweep)
	cache.Load(dir)
	if got := observedCacheStats() - before; got != 1 {
		t.Fatalf("forced re-sweep made %d stats, want 1 directory stat (was %d)", got, records+1)
	}
	if !cache.sessions[dir].read.sweptAt.Equal(clock) || cache.Opens() != records {
		t.Fatalf("forced re-sweep did not run or reread unchanged records: opens=%d", cache.Opens())
	}

	linked := observedCacheRecord("linked", "linked-model")
	linked.At = 5000
	raw, err := json.Marshal(linked)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target.json")
	if err := os.WriteFile(target, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, ObservedDirName, "linked.json")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	before = observedCacheStats()
	clock = clock.Add(observedResweep)
	got := cache.Load(dir)
	if n := observedCacheStats() - before; n != 2 {
		t.Fatalf("sweep with a symlinked record made %d stats, want directory + symlink", n)
	}
	if got.models[""].Current.Model != "linked-model" {
		t.Fatal("symlinked record not read")
	}
}
