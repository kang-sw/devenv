package wsnote

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsstate"
)

func TestReadPathsDoesNotEnsureStores(t *testing.T) {
	root, linked := twoWorktreesFixture(t)
	cache, config := filepath.Join(t.TempDir(), "absent-cache"), filepath.Join(t.TempDir(), "absent-config")
	opts := wsstate.Options{CacheHome: cache}
	paths, err := ReadPaths(root, wsconfig.Options{ConfigHome: config}, opts)
	if err != nil {
		t.Fatal(err)
	}
	other, err := ReadPaths(linked, wsconfig.Options{ConfigHome: config}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if paths[LayerClone] != other[LayerClone] || paths[LayerWorktree] == other[LayerWorktree] || paths[LayerClone] == paths[LayerWorktree] {
		t.Fatalf("layer identities: %v %v", paths, other)
	}
	c := NewCache()
	for layer, path := range paths {
		if _, err := c.Read(path, layer == LayerRepo, false); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{cache, config, RepoDir(root)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("read created %s: %v", path, err)
		}
	}
	layout, _, _, err := wsstate.NewManager(opts).Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	if paths[LayerWorktree] != filepath.Join(layout.WorktreeDir, "notes.json") || paths[LayerClone] != filepath.Join(layout.SharedDir, "notes.json") {
		t.Fatalf("read paths differ from writer paths: %v %+v", paths, layout)
	}
}

func TestCachePublishesMutationsWithoutRereads(t *testing.T) {
	for _, repo := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "repo"}[repo], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "notes")
			reads := 0
			c := NewCache()
			c.loadFile = func(path string) (map[string]Record, error) { reads++; return Load(path) }
			c.loadRepo = func(path string) (map[string]Record, error) { reads++; return RepoLoad(path) }
			read := func(force bool) []Record {
				t.Helper()
				rows, err := c.Read(path, repo, force)
				if err != nil {
					t.Fatal(err)
				}
				return rows
			}
			write := func(records []Record, cached bool) {
				t.Helper()
				var cache *Cache
				if cached {
					cache = c
				}
				var err error
				if repo {
					err = RepoWrite(path, records, cache)
				} else {
					err = Write(path, records, cache)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			visible := func(value bool) {
				t.Helper()
				var err error
				if repo {
					err = RepoSetVisible(path, []string{"a"}, value, c)
				} else {
					err = SetVisible(path, []string{"a"}, value, c)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			read(false)
			write([]Record{{Key: "a", Value: "first", Priority: 2, WrittenAt: "2026-11-17T00:00:00Z"}}, true)
			visible(false)
			write([]Record{{Key: "a", Value: "updated", Priority: 3, WrittenAt: "2026-11-17T01:00:00Z"}, {Key: "b", Value: "second", Priority: 0}}, true)
			rows := read(false)
			if len(rows) != 2 || rows[0].Key != "a" || rows[0].Visible || rows[0].Value != "updated" || reads != 1 {
				t.Fatalf("published write/mute = %+v, reads=%d", rows, reads)
			}
			visible(true)
			var err error
			if repo {
				err = RepoErase(path, []string{"b", "absent"}, c)
			} else {
				err = Erase(path, []string{"b", "absent"}, c)
			}
			if err != nil {
				t.Fatal(err)
			}
			rows = read(false)
			if len(rows) != 1 || !rows[0].Visible || reads != 1 {
				t.Fatalf("published unmute/erase = %+v reads=%d", rows, reads)
			}
			// Standalone writers deliberately do not publish to this daemon.
			write([]Record{{Key: "external", Value: "outside daemon", Priority: 9}}, false)
			rows = read(false)
			wantReads, wantLen := 1, 1
			if repo {
				wantReads, wantLen = 2, 2
			}
			if reads != wantReads || len(rows) != wantLen {
				t.Fatalf("external open = %+v reads=%d", rows, reads)
			}
			rows = read(true)
			if reads != wantReads+1 || len(rows) != 2 || rows[0].Key != "external" {
				t.Fatalf("explicit refresh = %+v reads=%d", rows, reads)
			}
			copy := append([]Record(nil), rows...)
			rows[0].Value = "browser copy"
			if !reflect.DeepEqual(read(false), copy) {
				t.Fatal("caller mutated retained cache")
			}
		})
	}
}

func TestCacheRepoFingerprintSeesInPlaceEdits(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "repo-notes")
	if err := RepoWrite(dir, []Record{{Key: "a", Value: "original"}}); err != nil {
		t.Fatal(err)
	}
	c := NewCache()
	if _, err := c.Read(dir, true, false); err != nil {
		t.Fatal(err)
	}
	// Directory names do not change. Size detects this regardless of clock granularity.
	if err := os.WriteFile(filepath.Join(dir, repoKeyFilename("a")), []byte(`{"key":"a","value":"in-place changed contents"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := c.Read(dir, true, false)
	if err != nil || len(rows) != 1 || rows[0].Value != "in-place changed contents" {
		t.Fatalf("in-place = %+v %v", rows, err)
	}
	if err := os.Remove(filepath.Join(dir, repoKeyFilename("a"))); err != nil {
		t.Fatal(err)
	}
	if rows, err := c.Read(dir, true, false); err != nil || len(rows) != 0 {
		t.Fatalf("deletion = %+v %v", rows, err)
	}
}
