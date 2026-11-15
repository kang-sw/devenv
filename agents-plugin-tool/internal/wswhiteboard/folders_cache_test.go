package wswhiteboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func folderIdleBoard(tb testing.TB) FolderStore {
	tb.Helper()
	f := FolderStore{Dir: tb.TempDir()}
	if err := f.Update(nil, func(s *State) error {
		for i := 0; i < 112; i++ {
			s.EntrySeq++
			s.Topics = append(s.Topics, TopicDoc{Name: fmt.Sprintf("topic-%d", i), Body: "unchanged topic body", Entries: []Entry{{ID: fmt.Sprintf("e%d", s.EntrySeq), Kind: EntryAgent, Text: "history", At: "2026-11-15T00:00:00Z"}}})
		}
		return nil
	}); err != nil {
		tb.Fatal(err)
	}
	return f
}

func countFolderReads(tb testing.TB) *int {
	tb.Helper()
	read := folderReadFile
	count := new(int)
	folderReadFile = func(path string) ([]byte, error) {
		*count++
		return read(path)
	}
	tb.Cleanup(func() { folderReadFile = read })
	return count
}

func TestFolderCacheIdleOpenAndOtherWriter(t *testing.T) {
	f := folderIdleBoard(t)
	var cache FolderCache
	want, err := cache.Open(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	reads := countFolderReads(t)
	for i := 0; i < 20; i++ {
		got, err := cache.Open(f, nil)
		if err != nil || !reflect.DeepEqual(want, got) {
			t.Fatalf("idle open %d: %v", i, err)
		}
		if _, problems := Apply(got, `<wb:topic name="topic-0"><wb:body>private update</wb:body></wb:topic>`, Env{}); len(problems) > 0 {
			t.Fatal(problems)
		}
		if want.Topics[0].Body != "unchanged topic body" {
			t.Fatal("Apply changed another cached reader")
		}
	}
	if *reads != 0 {
		t.Fatalf("idle refreshes reopened %d board files", *reads)
	}
	// A separate handle uses the same flock, as an MCP writer in another
	// process does; neither a nudge nor a shared cache object is involved.
	writer := FolderStore{Dir: f.Dir}
	if err := writer.Update(nil, func(s *State) error {
		s.Topics[0].Body = "external write"
		return nil // deliberately keep Seq unchanged
	}); err != nil {
		t.Fatal(err)
	}
	got, err := cache.Open(f, nil)
	if err != nil || got.Topics[0].Body != "external write" {
		t.Fatalf("external update missed: %v", err)
	}
	*reads = 0
	if _, err := cache.Open(f, nil); err != nil || *reads != 0 {
		t.Fatalf("updated board not cached: reads=%d err=%v", *reads, err)
	}
	cache.Forget(f.Dir)
	if _, err := cache.Open(f, nil); err != nil || *reads == 0 {
		t.Fatalf("Forget retained board: reads=%d err=%v", *reads, err)
	}
}

func TestFolderCachePendingReplayAndSchemaGuard(t *testing.T) {
	f := folderIdleBoard(t)
	var cache FolderCache
	if _, err := cache.Open(f, nil); err != nil {
		t.Fatal(err)
	}
	writer := FolderStore{Dir: f.Dir, Fault: func(stage string) error {
		if stage == "intent" {
			return folderCrash
		}
		return nil
	}}
	if err := writer.Update(nil, func(s *State) error {
		s.Topics[0].Body = "replayed write"
		return nil
	}); !errors.Is(err, folderCrash) {
		t.Fatal(err)
	}
	got, err := cache.Open(f, nil)
	if err != nil || got.Topics[0].Body != "replayed write" {
		t.Fatalf("cached Open did not replay: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.Dir, "pending.json")); !os.IsNotExist(err) {
		t.Fatalf("pending not removed: %v", err)
	}
	if _, err := cache.Open(f, nil); err != nil {
		t.Fatal(err)
	}
	// An out-of-band newer topic schema is still refused, even though
	// the session metadata and generation did not change.
	path := filepath.Join(f.Dir, "t1", "meta.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":999}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Open(f, nil); err == nil {
		t.Fatal("cache hid newer nested schema")
	}
}

func TestFolderCacheRecoveryAfterPartialPublication(t *testing.T) {
	for _, stage := range []string{"log-partial", "topic-meta", "session-meta", "cleanup"} {
		t.Run(stage, func(t *testing.T) {
			f := FolderStore{Dir: t.TempDir()}
			var cache FolderCache
			if _, err := cache.Open(f, folderFixture()); err != nil {
				t.Fatal(err)
			}
			if _, err := cache.Open(f, nil); err != nil {
				t.Fatal(err)
			}
			writer := FolderStore{Dir: f.Dir, Fault: func(at string) error {
				if at == stage {
					return folderCrash
				}
				return nil
			}}
			if err := writer.Update(nil, folderAdd); !errors.Is(err, folderCrash) {
				t.Fatalf("did not reach %s: %v", stage, err)
			}
			got, err := cache.Open(f, nil)
			if err != nil {
				t.Fatal(err)
			}
			want := folderOpen(t, f.Dir, nil)
			if !reflect.DeepEqual(got.clone(), want) || got.EntrySeq != 5 {
				t.Fatalf("cached recovery at %s lost or duplicated entries", stage)
			}
		})
	}
}

func TestFolderCacheObservesAnotherProcess(t *testing.T) {
	f := FolderStore{Dir: t.TempDir()}
	var cache FolderCache
	if _, err := cache.Open(f, &State{Topics: []TopicDoc{{Name: "topic", Body: "body"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Open(f, nil); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestFolderProcessWorker$")
	cmd.Env = append(os.Environ(), "WS_WHITEBOARD_FOLDER_TEST="+f.Dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("external writer: %v\n%s", err, out)
	}
	got, err := cache.Open(f, nil)
	if err != nil || len(got.Topics[0].Entries) != 3 || got.EntrySeq != 3 {
		t.Fatalf("missed another process's write: %v", err)
	}
}

func TestFolderCacheDetectsAtomicReplaceWithSameStamp(t *testing.T) {
	f := folderIdleBoard(t)
	var cache FolderCache
	if _, err := cache.Open(f, nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.Dir, "t1", "meta.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	data := folderRead(t, path)
	tmp := path + ".replacement"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(tmp, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	reads := countFolderReads(t)
	if _, err := cache.Open(f, nil); err != nil || *reads == 0 {
		t.Fatalf("same-stamp replacement not re-read: reads=%d err=%v", *reads, err)
	}
}

func TestFolderCacheAbsentAndLegacy(t *testing.T) {
	f := FolderStore{Dir: filepath.Join(t.TempDir(), "absent")}
	var cache FolderCache
	if got, err := cache.Open(f, nil); err != nil || got != nil {
		t.Fatalf("absent: %v %v", got, err)
	}
	if _, err := os.Stat(f.Dir); !os.IsNotExist(err) {
		t.Fatalf("absent open created folder: %v", err)
	}
	if _, err := cache.Open(f, folderFixture()); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Open(f, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.Dir, "meta.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Open(f, folderFixture()); err == nil {
		t.Fatal("missing committed metadata resurrected legacy state")
	}
}

// Dashboard readers keep one frozen value while update/commit and implicit
// render callers mutate their own handles. Run with -race to enforce the seam.
func TestFolderCacheSharedReadersAndMutators(t *testing.T) {
	f := folderIdleBoard(t)
	var cache FolderCache
	frozen, err := cache.Open(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(frozen)
	other, err := cache.Open(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if &other.Topics[0] != &frozen.Topics[0] || other == frozen || !other.shared {
		t.Fatal("cache must share backing data, not mutable State headers")
	}
	var wg sync.WaitGroup
	for n := 0; n < 4; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 8; i++ {
				view := BuildView(frozen, ViewInput{Layout: LayoutFull})
				if _, err := RenderRegions(view); err != nil {
					t.Error(err)
				}
				_, _ = json.Marshal(frozen)
			}
		}()
	}
	for _, render := range []bool{false, true} {
		wg.Add(1)
		go func(render bool) {
			defer wg.Done()
			for i := 0; i < 8; i++ {
				// Equivalent to an Open header; retaining the original backing
				// also tests readers of the old generation during publication.
				local := frozen.sharedRead()
				if render {
					local.BeginRender()
					local.Topics[0].Body = "private render data"
				} else {
					_, problems := Apply(local, `<wb:topic name="topic-0"><wb:body>private update</wb:body><wb:log>new history</wb:log></wb:topic>`, testEnv())
					if len(problems) > 0 {
						t.Error(problems)
					}
				}
				if local.shared {
					t.Error("mutation did not detach")
				}
			}
		}(render)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 4; i++ {
			if err := f.Update(nil, func(s *State) error {
				_, problems := Apply(s, `<wb:topic name="topic-0"><wb:body>committed update</wb:body></wb:topic>`, testEnv())
				if len(problems) > 0 {
					return fmt.Errorf("%v", problems)
				}
				s.BeginRender()
				return nil
			}); err != nil {
				t.Error(err)
			}
			if _, err := cache.Open(f, nil); err != nil {
				t.Error(err)
			}
		}
	}()
	wg.Wait()
	after, _ := json.Marshal(frozen)
	if !bytes.Equal(before, after) {
		t.Fatal("cached generation changed during mutations")
	}
	got, err := cache.Open(f, nil)
	if err != nil || got.Topics[0].Body != "committed update" {
		t.Fatalf("committed update lost: %v", err)
	}
}

func BenchmarkFolderIdleOpen112(b *testing.B) {
	for _, cached := range []bool{false, true} {
		b.Run(fmt.Sprintf("cached=%t", cached), func(b *testing.B) {
			f := folderIdleBoard(b)
			var cache *FolderCache
			if cached {
				cache = new(FolderCache)
			}
			if _, err := cache.Open(f, nil); err != nil {
				b.Fatal(err)
			}
			reads := countFolderReads(b)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := cache.Open(f, nil); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			// One read seam call attempts one open; add the flock file.
			b.ReportMetric(float64(*reads)/float64(b.N)+1, "opens/op")
		})
	}
}
