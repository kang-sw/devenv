package wsdoc

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdoc/wsdoctest"
)

// mapTouches records every file touch a ticket map build makes under one root
// through the readTicketFile, lstatTicketFile and readTicketDir seams.
type mapTouches struct {
	mu     sync.Mutex
	reads  []string
	lstats []string
	dirs   []string
}

// countMapTouches swaps the three seams for the test's duration. A test using
// it must not run in parallel: the seams are package state.
func countMapTouches(t *testing.T, root string) *mapTouches {
	t.Helper()
	c := &mapTouches{}
	record := func(list *[]string, path string) {
		if strings.HasPrefix(path, root) {
			c.mu.Lock()
			*list = append(*list, path)
			c.mu.Unlock()
		}
	}
	origRead, origLstat, origDir := readTicketFile, lstatTicketFile, readTicketDir
	readTicketFile = func(path string) ([]byte, error) {
		record(&c.reads, path)
		return origRead(path)
	}
	lstatTicketFile = func(path string) (fs.FileInfo, error) {
		record(&c.lstats, path)
		return origLstat(path)
	}
	readTicketDir = func(dir string) ([]os.DirEntry, error) {
		record(&c.dirs, dir)
		return origDir(dir)
	}
	t.Cleanup(func() { readTicketFile, lstatTicketFile, readTicketDir = origRead, origLstat, origDir })
	return c
}

// take returns and clears the recorded touches.
func (c *mapTouches) take() (reads, lstats, dirs []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	reads, lstats, dirs = c.reads, c.lstats, c.dirs
	c.reads, c.lstats, c.dirs = nil, nil, nil
	return reads, lstats, dirs
}

// checkListings asserts exactly the three open status directories were
// listed, and never a closed one.
func checkListings(t *testing.T, step string, dirs []string) {
	t.Helper()
	if len(dirs) != 3 {
		t.Errorf("%s: %d directory listings %v, want 3", step, len(dirs), dirs)
	}
	for _, d := range dirs {
		switch filepath.Base(d) {
		case "ready", "todo", "idea":
		default:
			t.Errorf("%s: listed %s", step, d)
		}
	}
}

// checkLstats asserts the closed-ref lookups stay within 2 per distinct
// referenced closed stem (resolved refs plus dangling) and target only those.
func checkLstats(t *testing.T, step string, lstats []string, layout wsdoctest.TicketMapLayout) {
	t.Helper()
	referenced := map[string]bool{}
	for _, list := range [][]string{layout.RefDone, layout.RefDropped, layout.Dangling} {
		for _, stem := range list {
			referenced[stem] = true
		}
	}
	if len(lstats) > 2*len(referenced) {
		t.Errorf("%s: %d lstats for %d referenced closed stems", step, len(lstats), len(referenced))
	}
	for _, path := range lstats {
		dir := filepath.Base(filepath.Dir(path))
		if (dir != ".done" && dir != ".dropped") || !referenced[strings.TrimSuffix(filepath.Base(path), ticketFileSuffix)] {
			t.Errorf("%s: lstat of %s", step, path)
		}
	}
}

func TestTicketMapTouchBudget(t *testing.T) {
	root := t.TempDir()
	layout := writeTicketMapFixture(t, root, wsdoctest.TicketMapDefaultOpen)
	resetTicketMapState(t)
	touches := countMapTouches(t, root)
	included := layout.Open + len(layout.RefDone) + len(layout.RefDropped)

	cold := buildTicketMap(t, root)
	reads, lstats, dirs := touches.take()
	if len(reads) != included {
		t.Errorf("cold: %d opens, want %d included tickets", len(reads), included)
	}
	seen := map[string]bool{}
	for _, path := range reads {
		if seen[path] {
			t.Errorf("cold: %s opened twice", path)
		}
		seen[path] = true
	}
	checkListings(t, "cold", dirs)
	checkLstats(t, "cold", lstats, layout)

	warm := buildTicketMap(t, root)
	reads, lstats, dirs = touches.take()
	if len(reads) != 0 || warm != cold {
		t.Errorf("warm memo: %d opens, same result %v", len(reads), warm == cold)
	}
	checkListings(t, "warm", dirs)
	checkLstats(t, "warm", lstats, layout)

	// Memo dropped, documents kept: still no open.
	ticketMapState.mu.Lock()
	ticketMapState.roots = map[string]*ticketMapRoot{}
	ticketMapState.mu.Unlock()
	rebuilt := buildTicketMap(t, root)
	reads, lstats, dirs = touches.take()
	if len(reads) != 0 || rebuilt.Meta.Fingerprint != cold.Meta.Fingerprint {
		t.Errorf("warm caches: %d opens, fingerprint %s vs %s", len(reads), rebuilt.Meta.Fingerprint, cold.Meta.Fingerprint)
	}
	checkListings(t, "warm caches", dirs)
	checkLstats(t, "warm caches", lstats, layout)

	// Touch one open ticket: new content, a new backdated mtime.
	stem := layout.Topics[1].Stems[5]
	open := filepath.Join(root, TicketsDir, layout.Dirs[stem], stem+ticketFileSuffix)
	raw, err := os.ReadFile(open)
	if err != nil {
		t.Fatal(err)
	}
	when := wsdoctest.TicketMapModTime.Add(time.Minute)
	if err := os.WriteFile(open, append(raw, "\nOne more voucher line.\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(open, when, when); err != nil {
		t.Fatal(err)
	}
	touched := buildTicketMap(t, root)
	reads, lstats, dirs = touches.take()
	if len(reads) != 1 || reads[0] != open {
		t.Errorf("touched open ticket: opens %v, want only %s", reads, open)
	}
	if touched.Meta.Fingerprint == cold.Meta.Fingerprint {
		t.Errorf("touched open ticket: fingerprint unchanged")
	}
	checkListings(t, "touched open", dirs)
	checkLstats(t, "touched open", lstats, layout)

	// Touch one unreferenced closed ticket: nothing changes.
	closed := filepath.Join(root, TicketsDir, ".done", layout.UnrefDone[0]+ticketFileSuffix)
	if err := os.WriteFile(closed, []byte("---\ntitle: Rewritten\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(closed, when, when); err != nil {
		t.Fatal(err)
	}
	after := buildTicketMap(t, root)
	reads, lstats, dirs = touches.take()
	if len(reads) != 0 || after != touched || after.Meta.Fingerprint != touched.Meta.Fingerprint {
		t.Errorf("touched unreferenced closed ticket: %d opens, memo reused %v", len(reads), after == touched)
	}
	for _, path := range lstats {
		if path == closed {
			t.Errorf("unreferenced closed ticket was looked up")
		}
	}
	checkListings(t, "touched closed", dirs)
	checkLstats(t, "touched closed", lstats, layout)
}

func TestTicketMapCacheInterop(t *testing.T) {
	root := t.TempDir()
	layout := writeTicketMapFixture(t, root, wsdoctest.TicketMapDefaultOpen)
	all := TicketListOptions{IncludeDone: true, IncludeDropped: true}
	openOnly := TicketListOptions{Statuses: []string{"ready", "todo", "idea"}}

	// A plain listing after a map build sees the same TicketInfo.
	resetTicketMapState(t)
	before, err := TicketsList(root, all)
	if err != nil {
		t.Fatal(err)
	}
	resetTicketMapState(t)
	buildTicketMap(t, root)
	afterMap, err := TicketsList(root, all)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, afterMap) {
		t.Fatal("TicketsList after a map build differs from a cold listing")
	}

	// A map build after a plain listing reads each cached open ticket once to
	// fill its document, then never again.
	resetTicketMapState(t)
	if _, err := TicketsList(root, openOnly); err != nil {
		t.Fatal(err)
	}
	touches := countMapTouches(t, root)
	buildTicketMap(t, root)
	reads, _, _ := touches.take()
	perFile := map[string]int{}
	openReads := 0
	for _, path := range reads {
		perFile[path]++
		if dir := filepath.Base(filepath.Dir(path)); dir == "ready" || dir == "todo" || dir == "idea" {
			openReads++
		}
	}
	if openReads != layout.Open || len(perFile) != len(reads) {
		t.Errorf("map after listing: %d open reads (want %d), %d reads of %d files", openReads, layout.Open, len(reads), len(perFile))
	}
	ticketMapState.mu.Lock()
	ticketMapState.roots = map[string]*ticketMapRoot{}
	ticketMapState.mu.Unlock()
	buildTicketMap(t, root)
	if reads, _, _ := touches.take(); len(reads) != 0 {
		t.Errorf("second map build after listing: %d opens", len(reads))
	}
	if _, err := TicketsList(root, openOnly); err != nil {
		t.Fatal(err)
	}
	if reads, _, _ := touches.take(); len(reads) != 0 {
		t.Errorf("plain listing after the map build: %d opens", len(reads))
	}
}

func BenchmarkTicketMapBuild(b *testing.B) {
	root := b.TempDir()
	wsdoctest.WriteTicketMapFixture(b, root, 300)
	b.Run("cold", func(b *testing.B) {
		for range b.N {
			b.StopTimer()
			resetTicketMapState(b)
			b.StartTimer()
			if _, err := TicketMapJSON(root); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("warm-memo", func(b *testing.B) {
		resetTicketMapState(b)
		if _, err := TicketMapJSON(root); err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for range b.N {
			if _, err := TicketMapJSON(root); err != nil {
				b.Fatal(err)
			}
		}
	})
}
