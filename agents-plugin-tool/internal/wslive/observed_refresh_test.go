package wslive

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestObservedSweepDoesNotBlockInvalidationOrWarmReads(t *testing.T) {
	dir := t.TempDir()
	if err := AppendObserved(dir, []ObservedRecord{observedCacheRecord("one", "model")}); err != nil {
		t.Fatal(err)
	}
	clock := time.Now().Add(time.Hour)
	entered, release := make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	var block atomic.Bool
	var stats atomic.Int64
	cache := ObservedCache{now: func() time.Time { return clock }, stat: func(path string) (os.FileInfo, error) {
		stats.Add(1)
		if filepath.Base(path) != ObservedDirName && block.CompareAndSwap(true, false) {
			close(entered)
			<-release // stands for AV blocking a record stat for tens of seconds
		}
		return os.Stat(path)
	}}
	first := cache.Load(dir)
	clock = clock.Add(observedResweep)
	block.Store(true)
	done := make(chan struct{})
	go func() { cache.Load(dir); close(done) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("sweep did not reach the record stat")
	}
	invalidated := make(chan struct{})
	go func() { cache.Invalidate(dir); close(invalidated) }()
	select {
	case <-invalidated:
	case <-time.After(time.Second):
		t.Fatal("slow sweep blocked invalidation")
	}
	read := make(chan *ObservedEvidence, 1)
	go func() { read <- cache.Load(dir) }()
	select {
	case evidence := <-read:
		if evidence != first {
			t.Fatal("warm reader lost prior immutable evidence")
		}
	case <-time.After(time.Second):
		t.Fatal("warm reader queued behind slow sweep")
	}
	unblock.Do(func() { close(release) })
	<-done
	before := stats.Load()
	cache.Load(dir)
	if stats.Load() == before {
		t.Fatal("publication lost an invalidation that arrived during the sweep")
	}
}
