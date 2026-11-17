package wslive

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
