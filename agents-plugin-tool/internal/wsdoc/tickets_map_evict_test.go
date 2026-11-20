package wsdoc

import (
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// writeEvictBoard writes a small board with one open ticket referencing one
// closed ref, so a build fills both ticketCache docs and ticketRefCache.
func writeEvictBoard(t *testing.T, root string) {
	t.Helper()
	writeMapBoard(t, root, map[string]string{
		"todo/260101-feat-alpha-sample": mapTicket("Alpha sample", "related:", "  260101-feat-closed-sample: note"),
		"idea/260102-feat-beta-sample":  mapTicket("Beta sample"),
	})
	writeOldTicket(t, root, TicketsDir+"/.done/260101-feat-closed-sample.md", mapTicket("Closed sample"), 3600e9)
}

func rootDocsCached(root string) (docs, refs int) {
	prefix := filepath.Join(root, filepath.FromSlash(TicketsDir)) + string(filepath.Separator)
	ticketCache.Lock()
	for dir, entries := range ticketCache.dirs {
		if !strings.HasPrefix(dir+string(filepath.Separator), prefix) {
			continue
		}
		for _, entry := range entries {
			if entry.doc != nil {
				docs++
			}
		}
	}
	ticketCache.Unlock()
	ticketRefCache.Lock()
	for path := range ticketRefCache.files {
		if strings.HasPrefix(path, prefix) {
			refs++
		}
	}
	ticketRefCache.Unlock()
	return docs, refs
}

// TestTicketMapRootEvictionDropsLeastRecentRoot builds one more root than the
// cap: the least recently built root loses its memo, its carried documents and
// its closed-ref entries, while its plain parses stay listable.
func TestTicketMapRootEvictionDropsLeastRecentRoot(t *testing.T) {
	resetTicketMapState(t)
	t.Cleanup(func() { resetTicketMapState(t) })
	roots := make([]string, ticketMapRootCap+1)
	for i := range roots {
		roots[i] = t.TempDir()
		writeEvictBoard(t, roots[i])
	}
	for _, root := range roots[:ticketMapRootCap] {
		buildTicketMap(t, root)
	}
	if docs, refs := rootDocsCached(roots[0]); docs != 2 || refs != 1 {
		t.Fatalf("before eviction root 0 docs=%d refs=%d, want 2 and 1", docs, refs)
	}
	buildTicketMap(t, roots[ticketMapRootCap])

	ticketMapState.mu.Lock()
	_, kept := ticketMapState.roots[roots[0]]
	n := len(ticketMapState.roots)
	ticketMapState.mu.Unlock()
	if kept || n != ticketMapRootCap {
		t.Fatalf("root 0 kept=%v, roots=%d; want evicted and %d roots", kept, n, ticketMapRootCap)
	}
	if docs, refs := rootDocsCached(roots[0]); docs != 0 || refs != 0 {
		t.Fatalf("after eviction root 0 docs=%d refs=%d, want 0 and 0", docs, refs)
	}
	if docs, _ := rootDocsCached(roots[1]); docs != 2 {
		t.Fatalf("root 1 docs=%d after evicting root 0, want 2", docs)
	}
	list, err := TicketsList(roots[0], TicketListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stems := []string{}
	for _, info := range list {
		stems = append(stems, info.Stem)
	}
	if !slices.Equal(stems, []string{"260101-feat-alpha-sample", "260102-feat-beta-sample"}) {
		t.Fatalf("plain listing after eviction = %v", stems)
	}
	// Rebuilding the evicted root works and refills its documents.
	buildTicketMap(t, roots[0])
	if docs, refs := rootDocsCached(roots[0]); docs != 2 || refs != 1 {
		t.Fatalf("rebuilt root 0 docs=%d refs=%d, want 2 and 1", docs, refs)
	}
}

// TestTicketMapEvictionRacesPlainListings lists an about-to-be-evicted root
// concurrently with the builds that evict it; under -race this pins that
// eviction never writes a directory map a listing may be reading.
func TestTicketMapEvictionRacesPlainListings(t *testing.T) {
	resetTicketMapState(t)
	t.Cleanup(func() { resetTicketMapState(t) })
	roots := make([]string, ticketMapRootCap+4)
	for i := range roots {
		roots[i] = t.TempDir()
		writeEvictBoard(t, roots[i])
	}
	buildTicketMap(t, roots[0])
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := TicketsList(roots[0], TicketListOptions{}); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	for round := 0; round < 3; round++ {
		for _, root := range roots {
			if _, err := TicketMapJSON(root); err != nil {
				t.Error(err)
			}
		}
	}
	close(stop)
	wg.Wait()
}

// TestTicketMapNodeOrderIsByStemNotFileName pins stem order where a stem is a
// prefix of another: "x-a.md" lists before "x.md" by file name.
func TestTicketMapNodeOrderIsByStemNotFileName(t *testing.T) {
	resetTicketMapState(t)
	t.Cleanup(func() { resetTicketMapState(t) })
	root := t.TempDir()
	writeMapBoard(t, root, map[string]string{
		"todo/260101-feat-sample":       mapTicket("Short"),
		"todo/260101-feat-sample-extra": mapTicket("Long"),
	})
	got := mapStems(buildTicketMap(t, root))
	want := []string{"260101-feat-sample", "260101-feat-sample-extra"}
	if !slices.Equal(got, want) {
		t.Fatalf("node order = %v, want %v", got, want)
	}
}
