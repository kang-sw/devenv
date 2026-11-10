package mcp

import (
	"net/http"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// TestDashboardSourceRescanBlocksNoKnownLookup: a reply-id miss rescans the
// key store without holding the source's lock, so a lookup of a known
// reply-id answers while the rescan is held, and concurrent misses share
// one rescan.
func TestDashboardSourceRescanBlocksNoKnownLookup(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	src := NewDashboardSessionSource()
	d, err := src.store.dirs()
	if err != nil {
		t.Fatal(err)
	}
	// A map scanned before the key directories last changed: a miss
	// rescans.
	src.byReply = map[string][]string{"knownreply": {"known-key"}}
	src.scanned = time.Now()
	var calls atomic.Int32
	entered, release := make(chan struct{}, 4), make(chan struct{})
	src.listKeys = func(sessionDirs) ([]string, error) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		return []string{"known-key"}, nil
	}

	var misses sync.WaitGroup
	miss := func() {
		misses.Add(1)
		go func() {
			defer misses.Done()
			if keys := src.keysFor(d, "missingreply", false); len(keys) != 0 {
				t.Errorf("a missing reply-id found %v", keys)
			}
		}()
	}
	miss()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the miss did not rescan")
	}
	miss()
	known := make(chan []string, 1)
	go func() { known <- src.keysFor(d, "knownreply", false) }()
	select {
	case keys := <-known:
		if !reflect.DeepEqual(keys, []string{"known-key"}) {
			t.Fatalf("known reply-id = %v", keys)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a known reply-id's lookup waited on the rescan")
	}
	close(release)
	misses.Wait()
	if n := calls.Load(); n != 1 {
		t.Fatalf("two concurrent misses listed the key store %d times, want once", n)
	}
}

// TestDashboardSourceReadsSessionsConcurrently runs the daemon index's
// re-reads of two sessions through the real source at once (the index
// re-reads sessions in parallel), for the race detector.
func TestDashboardSourceReadsSessionsConcurrently(t *testing.T) {
	dir := dashboardSessionEnv(t)
	startFakeDaemon(t, dir, http.StatusNoContent, false)
	var projects []string
	var ids []string
	for i := 0; i < 2; i++ {
		s, key, loginRoot := dashboardSessionServer(t, dashboardGitRoot(t), true)
		if resp := dashCall(t, s, "todo.add", map[string]any{"session_key": key, "key": "step", "title": "a step"}); isToolError(resp) {
			t.Fatalf("todo.add: %s", resp)
		}
		projects = append(projects, loginRoot)
		ids = append(ids, wsmailbox.ReplyID(key))
	}
	idx := wsdashboard.NewIndex(wsdashboard.IndexOptions{Dir: dir, Source: NewDashboardSessionSource()})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			idx.Refresh()
		}()
		for j := range ids {
			idx.Nudge(ids[j], projects[j])
		}
	}
	wg.Wait()
	idx.Settle()
	if got := idx.Sessions(); len(got) != 2 {
		t.Fatalf("index = %+v, want both sessions", got)
	}
}
