package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// splitKeyPath returns key's key.json path in the split layout.
func splitKeyPath(t *testing.T, s *sessionStore, key string) string {
	t.Helper()
	d, err := s.dirs()
	if err != nil {
		t.Fatalf("dirs: %v", err)
	}
	return d.partPath(key, partKey)
}

// writeRawLegacyRecordForTest writes body as key's legacy one-file record
// (keys/<key>.json) and returns its path.
func writeRawLegacyRecordForTest(t *testing.T, s *sessionStore, key, body string) string {
	t.Helper()
	d, err := s.dirs()
	if err != nil {
		t.Fatalf("dirs: %v", err)
	}
	if err := os.MkdirAll(d.legacy, 0o755); err != nil {
		t.Fatalf("create legacy keys dir: %v", err)
	}
	path := d.legacyPath(key)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write legacy record %q: %v", key, err)
	}
	return path
}

// writeLegacyRecordForTest writes record as key's legacy one-file record and
// returns its path.
func writeLegacyRecordForTest(t *testing.T, s *sessionStore, key string, record sessionRecord) string {
	t.Helper()
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal legacy record: %v", err)
	}
	return writeRawLegacyRecordForTest(t, s, key, string(payload))
}

// fullLegacyRecord is a legacy record carrying every field, for checking that
// serving and migrating it keep each one.
func fullLegacyRecord(root, parent string) sessionRecord {
	return sessionRecord{
		sessionKeyRecord: sessionKeyRecord{
			SchemaVersion:         1,
			Root:                  root,
			Scope:                 string(roleLead),
			Parent:                parent,
			Overrides:             map[string]string{"review_phase": "full"},
			Note:                  "a note",
			ReviewTrackNudgeShown: true,
			MailboxReplyOpened:    true,
		},
		Agenda: map[string]json.RawMessage{"implement": json.RawMessage(`{"stage":"build"}`)},
		Todos:  []todoItem{{Key: "a", Title: "Step A", Status: todoWip}, {Key: "b", Title: "Step B", Status: todoPending}},
		Whiteboard: &wswhiteboard.State{
			Counter: 4,
			Seq:     7,
			Topics:  []wswhiteboard.TopicDoc{{Name: "alpha", Body: "notes"}},
		},
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readFileForTest(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// assertRecordContent compares every persisted field of got with want, the
// schema version aside.
func assertRecordContent(t *testing.T, got, want sessionRecord) {
	t.Helper()
	got.SchemaVersion, want.SchemaVersion = 0, 0
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("record content\n got %s\nwant %s", gotJSON, wantJSON)
	}
}

func TestSplitLayoutMintLookupChildrenAndMtime(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	t.Setenv("WS_CACHE_HOME", cache)
	s := newSessionStore()
	parent, err := s.mint("/work/lead", roleLead, "")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	child, err := s.mint("/work/child", roleDelegate, parent)
	if err != nil {
		t.Fatalf("mint child: %v", err)
	}

	keyPath := splitKeyPath(t, s, parent)
	var stored map[string]any
	if err := json.Unmarshal(readFileForTest(t, keyPath), &stored); err != nil {
		t.Fatalf("key.json: %v", err)
	}
	if stored["schema_version"] != float64(sessionRecordSchemaVersion) || stored["root"] != "/work/lead" || stored["scope"] != string(roleLead) {
		t.Fatalf("key.json = %v", stored)
	}
	entries, err := os.ReadDir(filepath.Dir(keyPath))
	if err != nil || len(entries) != 2 || entries[0].Name() != partKey.fileName() || entries[1].Name() != recordLockName {
		t.Fatalf("a minted key holds key.json and its record lock alone, got %v (%v)", entries, err)
	}
	if exists(filepath.Join(cache, legacyKeysDirName)) {
		t.Fatal("mint must not create the legacy keys/ directory")
	}

	if entry, ok := s.lookup(child); !ok || entry.root != "/work/child" || entry.scope != roleDelegate || entry.parent != parent {
		t.Fatalf("lookup(child) = %+v, %v", entry, ok)
	}
	children, err := s.children(parent, 0)
	if err != nil || len(children) != 1 || children[0].key != child {
		t.Fatalf("children = %+v, %v", children, err)
	}

	old := time.Now().Add(-2 * touchGuardWindow).Truncate(time.Second)
	if err := os.Chtimes(keyPath, old, old); err != nil {
		t.Fatal(err)
	}
	if got, ok := s.recordMtime(parent); !ok || !got.Equal(old) {
		t.Fatalf("recordMtime = %v, %v; want key.json's %v", got, ok, old)
	}
	if _, ok := s.lookup(parent); !ok {
		t.Fatal("lookup(parent) failed")
	}
	if !mtimeOf(t, keyPath).After(old) {
		t.Fatal("lookup must touch key.json")
	}
}

// Reading a legacy record leaves its session layout alone. Opening its board
// converts the board authority, without changing the inline rollback bytes.
func TestLegacyRecordServesWithoutMigrating(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newSessionStore()
	root := t.TempDir()
	writeSessionRecordForTest(t, s, "split-lead-00", root, roleLead, "")
	want := fullLegacyRecord(root, "split-lead-00")
	legacyPath := writeLegacyRecordForTest(t, s, "legacy-child-00", want)
	old := time.Now().Add(-2 * touchGuardWindow).Truncate(time.Second)
	if err := os.Chtimes(legacyPath, old, old); err != nil {
		t.Fatal(err)
	}

	if got, ok := s.recordMtime("legacy-child-00"); !ok || !got.Equal(old) {
		t.Fatalf("recordMtime(legacy) = %v, %v; want %v", got, ok, old)
	}
	entry, ok := s.lookup("legacy-child-00")
	if !ok || entry.root != root || entry.scope != roleLead || entry.parent != "split-lead-00" {
		t.Fatalf("lookup(legacy) = %+v, %v", entry, ok)
	}
	if !mtimeOf(t, legacyPath).After(old) {
		t.Fatal("lookup must touch the legacy record file")
	}
	for _, parts := range []recordPart{partKey, partTodos, partAll} {
		got, ok := s.readState("legacy-child-00", parts)
		if !ok {
			t.Fatalf("readState(legacy, %d) failed", parts)
		}
		assertRecordContent(t, got, want)
	}
	if v, ok := s.getOverride("legacy-child-00", "review_phase"); !ok || v != "full" {
		t.Fatalf("getOverride(legacy) = %q, %v", v, ok)
	}
	children, err := s.children("split-lead-00", 0)
	if err != nil || len(children) != 1 || children[0].key != "legacy-child-00" || children[0].note != "a note" {
		t.Fatalf("children over a legacy child = %+v, %v", children, err)
	}
	d, _ := s.dirs()
	if exists(d.keyDir("legacy-child-00")) {
		t.Fatal("a read created the legacy key's split directory")
	}
	if !exists(legacyPath) {
		t.Fatal("a read removed the legacy record file")
	}

	// A no-op mutation writes nothing, so it does not migrate either. It
	// still takes the record lock, which lives in the key's directory.
	if err := s.deleteOverride("legacy-child-00", "absent.item"); err != nil {
		t.Fatalf("deleteOverride(absent): %v", err)
	}
	if exists(d.partPath("legacy-child-00", partKey)) || !exists(legacyPath) {
		t.Fatal("a no-op mutation migrated the legacy record")
	}
}

// The first metadata write splits a legacy session, but both old board-bearing
// representations remain immutable rollback snapshots.
func TestLegacyRecordMigratesOnFirstWrite(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write func(s *sessionStore, key string) error
		apply func(r *sessionRecord)
	}{
		{
			name: "agenda",
			write: func(s *sessionStore, key string) error {
				return s.setAgenda(key, "scratch", json.RawMessage(`{"x":1}`))
			},
			apply: func(r *sessionRecord) { r.Agenda["scratch"] = json.RawMessage(`{"x":1}`) },
		},
		{
			name: "whiteboard",
			write: func(s *sessionStore, key string) error {
				return s.mutateRecordParts(key, partAll, partWhiteboard, func(r *sessionRecord) error {
					r.Whiteboard.Seq++
					return nil
				})
			},
			apply: func(r *sessionRecord) { r.Whiteboard.Seq++ },
		},
		{
			name:  "key",
			write: func(s *sessionStore, key string) error { return s.setOverride(key, "some.item", "v") },
			apply: func(r *sessionRecord) { r.Overrides["some.item"] = "v" },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			s := newSessionStore()
			const key = "legacy-lead-00"
			want := fullLegacyRecord(t.TempDir(), "")
			legacyPath := writeLegacyRecordForTest(t, s, key, want)
			inlineBefore := readFileForTest(t, legacyPath)
			var original map[string]json.RawMessage
			if err := json.Unmarshal(inlineBefore, &original); err != nil {
				t.Fatal(err)
			}

			if err := tc.write(s, key); err != nil {
				t.Fatalf("write: %v", err)
			}
			tc.apply(&want)

			if got := readFileForTest(t, legacyPath); !bytes.Equal(got, inlineBefore) {
				t.Fatal("the first write changed the inline rollback snapshot")
			}
			d, _ := s.dirs()
			if got := readFileForTest(t, d.partPath(key, partWhiteboard)); !bytes.Equal(got, original["whiteboard"]) {
				t.Fatal("migration must copy the original board JSON, not converted state")
			}
			for _, part := range recordParts {
				if !exists(d.partPath(key, part)) {
					t.Fatalf("%s missing after migration", part.fileName())
				}
			}
			var stored sessionKeyRecord
			if err := json.Unmarshal(readFileForTest(t, d.partPath(key, partKey)), &stored); err != nil {
				t.Fatal(err)
			}
			if stored.SchemaVersion != sessionRecordSchemaVersion {
				t.Fatalf("migrated key.json schema_version = %d, want %d", stored.SchemaVersion, sessionRecordSchemaVersion)
			}
			got, ok := s.readState(key, partAll)
			if !ok {
				t.Fatal("readState after migration failed")
			}
			assertRecordContent(t, got, want)
		})
	}
}

// Crash-shaped session migrations keep split-key precedence and repair stale
// parts while preserving board-bearing inline rollback snapshots.
func TestCrashShapedLayouts(t *testing.T) {
	t.Run("both layouts", func(t *testing.T) {
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		s := newSessionStore()
		const key = "both-layouts-00"
		legacyPath := writeLegacyRecordForTest(t, s, key, fullLegacyRecord(t.TempDir(), ""))
		original := readFileForTest(t, legacyPath)
		root := t.TempDir()
		writeSessionRecordForTest(t, s, key, root, roleLead, "")

		if entry, ok := s.lookup(key); !ok || entry.root != root {
			t.Fatalf("lookup = %+v, %v; want the split record", entry, ok)
		}
		if rec, ok := s.readState(key, partAll); !ok || rec.Root != root || rec.Agenda != nil {
			t.Fatalf("readState = %+v, %v; want the split record alone", rec, ok)
		}
		if !exists(legacyPath) {
			t.Fatal("a read removed the legacy record file")
		}
		if err := s.setAgenda(key, "k", json.RawMessage(`{}`)); err != nil {
			t.Fatal(err)
		}
		if got := readFileForTest(t, legacyPath); !bytes.Equal(got, original) {
			t.Fatal("the next write changed the inline rollback snapshot")
		}
		if rec, ok := s.readState(key, partAll); !ok || rec.Root != root || len(rec.Agenda) != 1 || rec.Todos != nil {
			t.Fatalf("readState after write = %+v, %v", rec, ok)
		}
	})
	t.Run("parts without key.json", func(t *testing.T) {
		t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
		s := newSessionStore()
		const key = "half-moved-00"
		want := fullLegacyRecord(t.TempDir(), "")
		want.Todos = nil
		legacyPath := writeLegacyRecordForTest(t, s, key, want)
		original := readFileForTest(t, legacyPath)
		d, _ := s.dirs()
		if err := os.MkdirAll(d.keyDir(key), 0o755); err != nil {
			t.Fatal(err)
		}
		stale := filepath.Join(d.keyDir(key), partTodos.fileName())
		if err := os.WriteFile(stale, []byte(`[{"key":"stale","title":"Stale","status":"pending"}]`), 0o600); err != nil {
			t.Fatal(err)
		}

		if rec, ok := s.readState(key, partAll); !ok || rec.Todos != nil || rec.Note != "a note" {
			t.Fatalf("readState = %+v, %v; want the legacy record", rec, ok)
		}
		if err := s.setOverride(key, "x", "y"); err != nil {
			t.Fatal(err)
		}
		want.Overrides["x"] = "y"
		if got := readFileForTest(t, legacyPath); !bytes.Equal(got, original) || exists(stale) {
			t.Fatalf("migration must preserve the inline board and remove the stale empty part (%v)", exists(stale))
		}
		got, ok := s.readState(key, partAll)
		if !ok {
			t.Fatal("readState after migration failed")
		}
		assertRecordContent(t, got, want)
	})
}

// TestPartWritesAreIndependent pins the split: a write rewrites only its own
// part files.
func TestPartWritesAreIndependent(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newSessionStore()
	root := t.TempDir()
	key, err := s.mint(root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.setAgenda(key, "a", json.RawMessage(`{"v":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.mutateTodos(key, func([]todoItem) ([]todoItem, error) {
		return []todoItem{{Key: "t", Title: "T", Status: todoPending}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.mutateRecordParts(key, partAll, partWhiteboard, func(r *sessionRecord) error {
		r.Whiteboard = &wswhiteboard.State{Seq: 1}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	d, _ := s.dirs()
	partPath := func(part recordPart) string {
		if part == partWhiteboard {
			return filepath.Join(whiteboardFolder(root, key).Dir, "meta.json")
		}
		return d.partPath(key, part)
	}
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	type snapshot struct {
		data  []byte
		mtime time.Time
	}
	snap := func() map[recordPart]snapshot {
		out := map[recordPart]snapshot{}
		for _, part := range recordParts {
			path := partPath(part)
			out[part] = snapshot{readFileForTest(t, path), mtimeOf(t, path)}
		}
		return out
	}
	backdate := func() {
		for _, part := range recordParts {
			if err := os.Chtimes(partPath(part), old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, tc := range []struct {
		name    string
		write   func() error
		changed recordPart
	}{
		{"agenda", func() error { return s.setAgenda(key, "a", json.RawMessage(`{"v":2}`)) }, partAgenda},
		{"todos", func() error {
			return s.mutateTodos(key, func(list []todoItem) ([]todoItem, error) {
				return append(list, todoItem{Key: "u", Title: "U", Status: todoPending}), nil
			})
		}, partTodos},
		{"whiteboard", func() error {
			return s.mutateRecordParts(key, partAll, partWhiteboard, func(r *sessionRecord) error {
				r.Whiteboard.Seq++
				return nil
			})
		}, partWhiteboard},
		{"key", func() error { return s.setOverride(key, "item", "v") }, partKey},
	} {
		backdate()
		before := snap()
		if err := tc.write(); err != nil {
			t.Fatalf("%s write: %v", tc.name, err)
		}
		after := snap()
		for _, part := range recordParts {
			same := bytes.Equal(before[part].data, after[part].data) && before[part].mtime.Equal(after[part].mtime)
			if part == tc.changed && same {
				t.Fatalf("%s write did not rewrite %s", tc.name, part.fileName())
			}
			if part != tc.changed && !same {
				t.Fatalf("%s write rewrote %s", tc.name, part.fileName())
			}
		}
	}

	if exists(d.partPath(key, partWhiteboard)) {
		t.Fatal("new board writes must never create a legacy whiteboard.json")
	}

	// Emptying a part removes its file; the record still reads.
	if err := s.clearAllAgenda(key); err != nil {
		t.Fatal(err)
	}
	if exists(d.partPath(key, partAgenda)) {
		t.Fatal("an emptied agenda must remove agenda.json")
	}
	if rec, ok := s.readState(key, partAll); !ok || rec.Agenda != nil || len(rec.Todos) != 2 || rec.Whiteboard == nil {
		t.Fatalf("readState after clearing the agenda = %+v, %v", rec, ok)
	}
}

func TestMaybePruneCoversBothLayouts(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newSessionStore()
	d, _ := s.dirs()
	stale := keyRetentionAge + time.Hour

	staleLegacy := writeLegacyRecordForTest(t, s, "stale-legacy-00", fullLegacyRecord("/w", ""))
	backdateMtime(t, staleLegacy, stale)
	freshLegacy := writeLegacyRecordForTest(t, s, "fresh-legacy-00", fullLegacyRecord("/w", ""))

	staleSplit, err := s.mint("/w", roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.setAgenda(staleSplit, "a", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	backdateMtime(t, d.partPath(staleSplit, partKey), stale)
	freshSplit, err := s.mint("/w", roleLead, "")
	if err != nil {
		t.Fatal(err)
	}

	malformed := "malformed-key-00"
	if err := os.MkdirAll(d.keyDir(malformed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.partPath(malformed, partKey), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	backdateMtime(t, d.partPath(malformed, partKey), stale)

	orphanOld, orphanNew := "orphan-old-00", "orphan-new-00"
	for _, k := range []string{orphanOld, orphanNew} {
		if err := os.MkdirAll(d.keyDir(k), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	backdateMtime(t, d.keyDir(orphanOld), stale)

	s.maybePrune()

	for path, want := range map[string]bool{
		staleLegacy:            false,
		freshLegacy:            true,
		d.keyDir(staleSplit):   false,
		d.keyDir(freshSplit):   true,
		d.keyDir(malformed):    false,
		d.keyDir(orphanOld):    false,
		d.keyDir(orphanNew):    true,
		d.keyDir("absent-key"): false,
	} {
		if got := exists(path); got != want {
			t.Errorf("after prune, exists(%s) = %v, want %v", path, got, want)
		}
	}
}

// TestClaimKeyRefusesTakenCandidates pins mint's uniqueness claim: a legacy
// record file, a split record, and a bare directory all hold the name, and of
// concurrent claims of one free name exactly one wins.
func TestClaimKeyRefusesTakenCandidates(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newSessionStore()
	d, _ := s.dirs()
	payload := []byte(`{"schema_version":2,"root":"/w","scope":"lead"}`)

	writeLegacyRecordForTest(t, s, "legacy-taken-00", fullLegacyRecord("/w", ""))
	writeSessionRecordForTest(t, s, "split-taken-00", "/w", roleLead, "")
	if err := os.MkdirAll(d.keyDir("bare-dir-00"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"legacy-taken-00", "split-taken-00", "bare-dir-00"} {
		claimed, err := s.claimKey(d, key, payload)
		if err != nil || claimed {
			t.Fatalf("claimKey(%s) = %v, %v; want taken", key, claimed, err)
		}
	}
	if exists(d.keyDir("legacy-taken-00")) {
		t.Fatal("a refused claim must not leave a directory behind")
	}

	const racers = 16
	var wins int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := newSessionStore().claimKey(d, "raced-key-00", payload)
			if err != nil {
				t.Errorf("claimKey: %v", err)
			}
			if claimed {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("%d concurrent claims of one name won, want exactly 1", wins)
	}
	if entry, ok := s.lookup("raced-key-00"); !ok || entry.root != "/w" {
		t.Fatalf("the winning claim must be a readable record: %+v, %v", entry, ok)
	}
}

// TestDashboardSourceSeesBothLayouts pins the daemon's session reads over a
// split record and a legacy one, and that its reads never migrate.
func TestDashboardSourceSeesBothLayouts(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	project := t.TempDir()
	src := &DashboardSessionSource{store: newSessionStore(), stateDir: func() (string, error) { return t.TempDir(), nil }}
	s := src.store

	splitKey, err := s.mint(project, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.mutateRecordParts(splitKey, partAll, partWhiteboard, func(r *sessionRecord) error {
		r.Whiteboard = &wswhiteboard.State{Seq: 2}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	const legacyKey = "legacy-lead-00"
	legacy := fullLegacyRecord(project, "")
	legacyPath := writeLegacyRecordForTest(t, s, legacyKey, legacy)

	for _, tc := range []struct {
		key string
		seq int
	}{{splitKey, 2}, {legacyKey, legacy.Whiteboard.Seq}} {
		reg := wsdashboard.Registration{ReplyID: wsmailbox.ReplyID(tc.key), Project: project}
		data, ok := src.ReadSession(reg)
		if !ok || !data.HasBoard || data.Fingerprint == "" {
			t.Fatalf("ReadSession(%s) = %+v, %v", tc.key, data, ok)
		}
		page, ok := src.ReadPage(reg)
		if !ok || !page.HasBoard || page.Seq != tc.seq {
			t.Fatalf("ReadPage(%s) = board %v seq %d, %v; want seq %d", tc.key, page.HasBoard, page.Seq, ok, tc.seq)
		}
	}

	// A whiteboard write changes the split session's fingerprint.
	reg := wsdashboard.Registration{ReplyID: wsmailbox.ReplyID(splitKey), Project: project}
	before, _ := src.ReadSession(reg)
	if err := s.mutateRecordParts(splitKey, partAll, partWhiteboard, func(r *sessionRecord) error {
		r.Whiteboard.Seq++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if after, _ := src.ReadSession(reg); after.Fingerprint == before.Fingerprint {
		t.Fatal("a whiteboard write must change the session fingerprint")
	}

	d, _ := s.dirs()
	if exists(d.keyDir(legacyKey)) || !exists(legacyPath) {
		t.Fatal("dashboard reads must not migrate a legacy record")
	}
}

// TestRecordLockSerializesWritersAcrossStores pins the cross-process record
// lock: separate stores share no s.mu, as separate processes would not, and
// their concurrent writes to different agenda keys and to the todo list of one
// key all survive.
func TestRecordLockSerializesWritersAcrossStores(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	key, err := newSessionStore().mint("/work", roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	const stores, writes = 6, 15
	var wg sync.WaitGroup
	for i := 0; i < stores; i++ {
		// A generous timeout: this pins lost updates, not lock latency.
		s := &sessionStore{lockTimeout: time.Minute}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < writes; j++ {
				name := fmt.Sprintf("s%d-w%d", i, j)
				if err := s.setAgenda(key, name, json.RawMessage(`{"v":1}`)); err != nil {
					t.Errorf("setAgenda(%s): %v", name, err)
				}
				if err := s.mutateTodos(key, func(list []todoItem) ([]todoItem, error) {
					return append(list, todoItem{Key: name, Title: name, Status: todoPending}), nil
				}); err != nil {
					t.Errorf("mutateTodos(%s): %v", name, err)
				}
			}
		}()
	}
	wg.Wait()
	rec, ok := newSessionStore().readState(key, partAll)
	if !ok {
		t.Fatal("record unreadable after concurrent writes")
	}
	if len(rec.Agenda) != stores*writes || len(rec.Todos) != stores*writes {
		t.Fatalf("lost updates: %d agenda keys and %d todos, want %d each", len(rec.Agenda), len(rec.Todos), stores*writes)
	}
}

// TestFailedReplaceKeepsPreviousRecord pins that a write whose replace fails
// returns the error and leaves the previous part files readable, unchanged,
// and free of temp files.
func TestFailedReplaceKeepsPreviousRecord(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newSessionStore()
	key, err := s.mint("/work", roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.enterMode(key, "implement", json.RawMessage(`{"v":1}`), []todoItem{{Key: "a", Title: "A", Status: todoPending}}); err != nil {
		t.Fatal(err)
	}
	d, _ := s.dirs()
	before := map[recordPart][]byte{}
	for _, part := range recordParts {
		if exists(d.partPath(key, part)) {
			before[part] = readFileForTest(t, d.partPath(key, part))
		}
	}
	want, _ := s.readState(key, partAll)

	replaceErr := errors.New("replace refused")
	failing := &sessionStore{replace: func(string, string) error { return replaceErr }}
	if err := failing.enterMode(key, "implement", json.RawMessage(`{"v":2}`), []todoItem{{Key: "b", Title: "B", Status: todoPending}}); !errors.Is(err, replaceErr) {
		t.Fatalf("enterMode with a failing replace = %v, want %v", err, replaceErr)
	}
	if err := failing.setOverride(key, "item", "v"); !errors.Is(err, replaceErr) {
		t.Fatalf("setOverride with a failing replace = %v, want %v", err, replaceErr)
	}

	for part, data := range before {
		if got := readFileForTest(t, d.partPath(key, part)); !bytes.Equal(got, data) {
			t.Fatalf("%s changed by a failed write: %s, want %s", part.fileName(), got, data)
		}
	}
	got, ok := s.readState(key, partAll)
	if !ok {
		t.Fatal("record unreadable after a failed write")
	}
	assertRecordContent(t, got, want)
	entries, err := os.ReadDir(d.keyDir(key))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("a failed write left temp file %s", entry.Name())
		}
	}
	// The lock was released: a working store writes next.
	if err := s.setAgenda(key, "after", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("write after a failed write: %v", err)
	}
}

// TestRecordLockTimeoutIsAnError pins that a record lock held past the
// timeout fails the write, from the store and from a tool call, and writes
// nothing.
func TestRecordLockTimeoutIsAnError(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 905000, root, nil))
	d, _ := server.sessions.dirs()

	holder := flock.New(filepath.Join(d.keyDir(key), recordLockName))
	if locked, err := holder.TryLock(); err != nil || !locked {
		t.Fatalf("hold the record lock: %v, %v", locked, err)
	}
	defer holder.Unlock() //nolint:errcheck

	server.sessions.lockTimeout = 100 * time.Millisecond
	err := server.sessions.setAgenda(key, "notes", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("setAgenda under a held lock = %v, want a timeout error", err)
	}
	if got := callToolWithKey(t, server, 5001, key, "agenda.set", map[string]any{
		"key": "notes", "value": map[string]any{"v": 1},
	}); !strings.Contains(got, "waiting for session record lock") {
		t.Fatalf("agenda.set under a held lock: %s", got)
	}
	if rec, ok := server.sessions.readState(key, partAll); !ok || rec.Agenda != nil {
		t.Fatalf("a timed-out write changed the record: %+v, %v", rec, ok)
	}
}

// TestWriteToUnknownKeyCreatesNothing pins that the record lock is taken only
// for a key that has a record: a write to an unknown key fails and leaves no
// key directory, while a legacy key gets its directory, lock, and migration.
func TestWriteToUnknownKeyCreatesNothing(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newSessionStore()
	d, _ := s.dirs()
	if err := s.setAgenda("unknown-key-00", "a", json.RawMessage(`{}`)); err == nil || !strings.Contains(err.Error(), "session key not found") {
		t.Fatalf("setAgenda(unknown) = %v, want session key not found", err)
	}
	if exists(d.keyDir("unknown-key-00")) {
		t.Fatal("a write to an unknown key created its directory")
	}

	writeLegacyRecordForTest(t, s, "legacy-lock-00", fullLegacyRecord("/w", ""))
	if err := s.setAgenda("legacy-lock-00", "a", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("setAgenda(legacy): %v", err)
	}
	if !exists(filepath.Join(d.keyDir("legacy-lock-00"), recordLockName)) || !exists(d.partPath("legacy-lock-00", partKey)) {
		t.Fatal("a legacy write must lock in, and migrate to, the key's directory")
	}
}
