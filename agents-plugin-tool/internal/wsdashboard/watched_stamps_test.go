package wsdashboard

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatchedStampsShareValidationAcrossConcurrentRenders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watched.json")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	var stats, lists atomic.Int64
	cache := watchedStampCache{
		now: func() time.Time { return now },
		stat: func(path string) (os.FileInfo, error) {
			stats.Add(1)
			return os.Stat(path)
		},
		readDir: func(path string) ([]os.DirEntry, error) {
			lists.Add(1)
			return os.ReadDir(path)
		},
	}
	stamp := func(watch []string) string {
		h := sha256.New()
		cache.writeWatched(h, watch, time.Time{})
		return sumHex(h)
	}
	fileBefore, listBefore := stamp([]string{path}), stamp([]string{dir, path})
	const renders = 100
	// Reproduce the pre-cache render path with the same counted IO seams.
	for range renders {
		rawStatStamp(sha256.New(), path, cache.stat)
		rawListStamp(sha256.New(), dir, cache.readDir, cache.stat)
	}
	beforeStats, beforeLists := stats.Load()-1, lists.Load()-1
	stats.Store(1)
	lists.Store(1)
	var wg sync.WaitGroup
	for range renders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if stamp([]string{path}) != fileBefore || stamp([]string{dir, path}) != listBefore {
				t.Error("unchanged inputs changed the stamp")
			}
		}()
	}
	wg.Wait()
	if stats.Load() != 1 || lists.Load() != 1 {
		t.Fatalf("%d renders: stats=%d listings=%d, want one each", renders, stats.Load(), lists.Load())
	}
	if err := os.WriteFile(path, []byte("after, with a new size"), 0o600); err != nil {
		t.Fatal(err)
	}
	if stamp([]string{path}) != fileBefore || stamp([]string{dir, path}) != listBefore {
		t.Fatal("validation was not shared within its one-second window")
	}
	now = now.Add(watchedStampInterval)
	if stamp([]string{path}) == fileBefore || stamp([]string{dir, path}) == listBefore {
		t.Fatal("external edit not seen after one second")
	}
	if stats.Load() != 2 || lists.Load() != 2 {
		t.Fatalf("next window: stats=%d listings=%d, want two each", stats.Load(), lists.Load())
	}
	t.Logf("MEASURE renders=%d before_stats=%d after_stats=1 before_listings=%d after_listings=1", renders, beforeStats, beforeLists)
}

func TestWatchedStampsMissingPathsAndClockRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent")
	now := time.Now()
	stats := 0
	cache := watchedStampCache{
		now: func() time.Time { return now },
		stat: func(path string) (os.FileInfo, error) {
			stats++
			return os.Stat(path)
		},
	}
	stamp := func() string {
		h := sha256.New()
		cache.writeWatched(h, []string{path}, time.Time{})
		return sumHex(h)
	}
	missing := stamp()
	stamp()
	if stats != 1 {
		t.Fatalf("missing path stat %d times, want one", stats)
	}
	if err := os.WriteFile(path, []byte("created"), 0o600); err != nil {
		t.Fatal(err)
	}
	now = now.Add(-time.Second)
	if stamp() == missing || stats != 2 {
		t.Fatal("clock rollback must force validation")
	}
}
