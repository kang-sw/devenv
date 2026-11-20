package wswhiteboard

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

var archiveNow = time.Date(2026, 11, 18, 15, 30, 0, 0, time.Local)

func archiveApply(commands string) func(*State) error {
	env := Env{Now: time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)}
	return func(s *State) error {
		if _, problems := Apply(s, commands, env); len(problems) != 0 {
			return fmt.Errorf("apply: %v", problems)
		}
		return nil
	}
}

func archiveUpdate(t *testing.T, f FolderStore, fn func(*State) error) {
	t.Helper()
	if err := f.UpdateAndPublish(nil, fn, nil); err != nil {
		t.Fatal(err)
	}
}

// archiveSource is a board with topics, a panel, logs, and items.
func archiveSource(t *testing.T, dir string) FolderStore {
	t.Helper()
	f := FolderStore{Dir: dir}
	archiveUpdate(t, f, archiveApply(`<wb:topic name="handoff" label="Handoff">
<wb:panel recap="Design">Full panel payload</wb:panel>
<wb:log>Ready for takeover</wb:log>
<wb:add class="check"><wb:title>Verify the handoff</wb:title></wb:add>
</wb:topic>
<wb:topic name="context" label="Context"><wb:log>Keep this too</wb:log></wb:topic>`))
	return f
}

// archiveDestination is a board with more history than the source, so the
// load lowers its counters.
func archiveDestination(t *testing.T, dir string) FolderStore {
	t.Helper()
	f := FolderStore{Dir: dir}
	for i := 0; i < 6; i++ {
		archiveUpdate(t, f, archiveApply(fmt.Sprintf(`<wb:topic name="own-%d"><wb:log>own %d</wb:log><wb:log>more</wb:log><wb:add class="check"><wb:title>own item %d</wb:title></wb:add></wb:topic>`, i, i, i)))
	}
	return f
}

func writeArchive(t *testing.T, path string, data []byte) string {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(files[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func unzip(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		if _, err := b.ReadFrom(rc); err != nil {
			t.Fatal(err)
		}
		rc.Close()
		out[f.Name] = b.Bytes()
	}
	return out
}

// archiveComparable drops the revision receipts, monotonic bookkeeping the
// destination's own receipt floors, and the render sequence and load time a
// load sets; they are not board content or ID counters.
func archiveComparable(s *State) *State {
	c := s.clone()
	c.MetadataRevision, c.StaleNudgeRevision = 0, ""
	c.Seq, c.LoadedAt = 0, ""
	return c
}

func backups(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(dir + ".bak-*")
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func TestArchiveExportThenLoadReplacesTheBoardWithABackup(t *testing.T) {
	root := t.TempDir()
	source := archiveSource(t, filepath.Join(root, "source"))
	dest := archiveDestination(t, filepath.Join(root, "dest"))
	original := folderOpen(t, dest.Dir, nil)
	sourceTree := folderTree(t, source.Dir)

	data, err := source.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	files := unzip(t, data)
	for _, want := range []string{"meta.json", "committed.json"} {
		if _, ok := files[want]; !ok {
			t.Fatalf("archive lacks %s: %v", want, archiveKeys(files))
		}
	}
	for name := range files {
		if name == ".lock" || name == "pending.json" || strings.HasPrefix(filepath.Base(name), ".") {
			t.Fatalf("archive carries %s", name)
		}
	}
	zipPath := writeArchive(t, filepath.Join(root, "board.zip"), data)

	backup, topics, err := dest.LoadArchive(zipPath, archiveNow)
	if err != nil {
		t.Fatal(err)
	}
	if want := dest.Dir + ".bak-261118_1530"; backup != want || topics != 2 {
		t.Fatalf("LoadArchive = %q, %d; want %q, 2", backup, topics, want)
	}
	want := folderOpen(t, source.Dir, nil)
	got := folderOpen(t, dest.Dir, nil)
	if !reflect.DeepEqual(archiveComparable(want), archiveComparable(got)) {
		t.Fatalf("loaded board differs from the source\nwant %s\ngot  %s", folderJSON(archiveComparable(want)), folderJSON(archiveComparable(got)))
	}
	if got.Counter >= original.Counter || got.EntrySeq >= original.EntrySeq {
		t.Fatalf("fixture must lower the counters: loaded %d/%d, replaced %d/%d", got.Counter, got.EntrySeq, original.Counter, original.EntrySeq)
	}
	if prev := folderOpen(t, backup, nil); !reflect.DeepEqual(archiveComparable(original), archiveComparable(prev)) {
		t.Fatalf("backup is not the previous board\nwant %s\ngot  %s", folderJSON(original), folderJSON(prev))
	}
	if !reflect.DeepEqual(sourceTree, folderTree(t, source.Dir)) {
		t.Fatal("export or load changed the source board")
	}

	// The installed topic folders are all the board references. Removing the
	// old ones is best-effort (a reader or antivirus can hold one open on
	// Windows), so a leftover is allowed only while nothing references it,
	// no archive carries it, and no new topic reuses it.
	var meta folderSession
	if _, err := folderDecode(filepath.Join(dest.Dir, "meta.json"), &meta); err != nil {
		t.Fatal(err)
	}
	installed := map[string]bool{}
	for _, ref := range meta.Topics {
		installed[ref.Dir] = true
	}
	entries, err := os.ReadDir(dest.Dir)
	if err != nil {
		t.Fatal(err)
	}
	leftover := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() && folderDirID(e.Name()) && e.Name() != folderRevisionDir && !installed[e.Name()] {
			leftover[e.Name()] = true
		}
	}
	if len(leftover) > 0 {
		exported, err := dest.ExportArchive()
		if err != nil {
			t.Fatal(err)
		}
		for name := range unzip(t, exported) {
			if leftover[strings.SplitN(filepath.ToSlash(name), "/", 2)[0]] {
				t.Fatalf("archive carries the unreferenced leftover %s", name)
			}
		}
	}
	archiveUpdate(t, dest, archiveApply(`<wb:topic name="after-load"><wb:log>new</wb:log><wb:add class="check"><wb:title>next</wb:title></wb:add></wb:topic>`))
	after := folderOpen(t, dest.Dir, nil)
	if after.Items[len(after.Items)-1].ID != want.Counter+1 {
		t.Fatalf("next minted ID %d does not continue the loaded counter %d", after.Items[len(after.Items)-1].ID, want.Counter)
	}
	var next folderSession
	if _, err := folderDecode(filepath.Join(dest.Dir, "meta.json"), &next); err != nil {
		t.Fatal(err)
	}
	for _, ref := range next.Topics {
		if ref.Key == "after-load" && (installed[ref.Dir] || leftover[ref.Dir]) {
			t.Fatalf("new topic reuses an existing folder %s", ref.Dir)
		}
	}

	// A second load in the same minute keeps the first backup.
	second, _, err := dest.LoadArchive(zipPath, archiveNow)
	if err != nil {
		t.Fatal(err)
	}
	if second != dest.Dir+".bak-261118_1530-2" {
		t.Fatalf("second backup %q", second)
	}
	if prev := folderOpen(t, backup, nil); !reflect.DeepEqual(archiveComparable(original), archiveComparable(prev)) {
		t.Fatal("the second load overwrote the first backup")
	}
}

func archiveKeys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func TestArchiveLoadIntoNoBoardMakesNoBackup(t *testing.T) {
	root := t.TempDir()
	source := archiveSource(t, filepath.Join(root, "source"))
	data, err := source.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	dest := FolderStore{Dir: filepath.Join(root, "dest")}
	backup, topics, err := dest.LoadArchive(writeArchive(t, filepath.Join(root, "b.zip"), data), archiveNow)
	if err != nil || backup != "" || topics != 2 {
		t.Fatalf("LoadArchive = %q, %d, %v", backup, topics, err)
	}
	if got := backups(t, dest.Dir); len(got) != 0 {
		t.Fatalf("backups %v", got)
	}
	if got := folderOpen(t, dest.Dir, nil); got == nil || len(got.Topics) != 2 {
		t.Fatalf("loaded board %s", folderJSON(got))
	}
}

func TestArchiveExportWithoutBoardCreatesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "none")
	if _, err := (FolderStore{Dir: dir}).ExportArchive(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ExportArchive = %v, want not exist", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("export created %s", dir)
	}
}

func TestArchiveLoadRefusalsLeaveTheDestination(t *testing.T) {
	root := t.TempDir()
	source := archiveSource(t, filepath.Join(root, "source"))
	data, err := source.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	good := unzip(t, data)
	with := func(name string, content []byte) map[string][]byte {
		out := map[string][]byte{}
		for k, v := range good {
			out[k] = v
		}
		if content == nil {
			delete(out, name)
		} else {
			out[name] = content
		}
		return out
	}
	newer := bytes.Replace(good["meta.json"], []byte(`"schema_version":2`), []byte(`"schema_version":3`), 1)
	cases := []struct {
		name  string
		files map[string][]byte
		caps  func()
		want  string
	}{
		{"zip slip", with("../escape.json", []byte("{}")), nil, "path"},
		{"absolute", with("/abs.json", []byte("{}")), nil, "path"},
		{"backslash", with(`t1\..\..\x`, []byte("{}")), nil, "path"},
		{"no committed receipt", with("committed.json", nil), nil, "not a board"},
		{"newer schema", with("meta.json", newer), nil, "not a valid board"},
		{"broken log", with("t1/log.jsonl", []byte("garbage\n")), nil, "not a valid board"},
		{"entry cap", good, func() { archiveMaxEntries = 2 }, "entries"},
		{"size cap", good, func() { archiveMaxBytes = 64 }, "limit"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dest := archiveDestination(t, filepath.Join(t.TempDir(), "dest"))
			before := folderTree(t, dest.Dir)
			if c.caps != nil {
				entries, size := archiveMaxEntries, archiveMaxBytes
				t.Cleanup(func() { archiveMaxEntries, archiveMaxBytes = entries, size })
				c.caps()
			}
			zipPath := writeArchive(t, filepath.Join(t.TempDir(), "b.zip"), zipOf(t, c.files))
			if _, _, err := dest.LoadArchive(zipPath, archiveNow); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("LoadArchive error %v, want %q", err, c.want)
			}
			if !reflect.DeepEqual(before, folderTree(t, dest.Dir)) {
				t.Fatal("a refused load changed the destination")
			}
			if got := backups(t, dest.Dir); len(got) != 0 {
				t.Fatalf("a refused load made backups %v", got)
			}
		})
	}
	dest := archiveDestination(t, filepath.Join(root, "dest"))
	if _, _, err := dest.LoadArchive(filepath.Join(root, "missing.zip"), archiveNow); err == nil {
		t.Fatal("a missing zip loaded")
	}
}

// A replace that stops after its commit leaves the replaced board's topic
// folders behind; they are no board's, so the next export and backup leave
// them out.
func TestArchiveLeavesOutUnreferencedTopicFolders(t *testing.T) {
	root := t.TempDir()
	source := archiveSource(t, filepath.Join(root, "source"))
	data, err := source.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	zipPath := writeArchive(t, filepath.Join(root, "b.zip"), data)
	dest := archiveDestination(t, filepath.Join(root, "dest"))
	crashing := dest
	crashing.Fault = func(stage string) error {
		if stage == "complete" {
			return folderCrash
		}
		return nil
	}
	if _, _, err := crashing.LoadArchive(zipPath, archiveNow); !errors.Is(err, folderCrash) {
		t.Fatalf("LoadArchive = %v, want the injected crash", err)
	}
	if _, err := os.Stat(filepath.Join(dest.Dir, "t1")); err != nil {
		t.Fatalf("fixture must leave the old t1 behind: %v", err)
	}
	var meta folderSession
	if _, err := folderDecode(filepath.Join(dest.Dir, "meta.json"), &meta); err != nil {
		t.Fatal(err)
	}
	held := map[string]bool{folderRevisionDir: true}
	for _, ref := range meta.Topics {
		held[ref.Dir] = true
	}
	export, err := dest.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	for name := range unzip(t, export) {
		if top, _, nested := strings.Cut(name, "/"); nested && !held[top] {
			t.Errorf("export carries unreferenced %s", name)
		}
	}
	backup, _, err := dest.LoadArchive(zipPath, archiveNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(backup, "t1")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("backup carries the unreferenced t1: %v", err)
	}
}

// A load raises the render sequence past the replaced board's, so the change
// stamp an open page polls (derived from it) differs from every stamp
// published before, even when the loaded board's sequence is lower; and it
// stamps the load time, which floors every item's re-ask time so dashboard
// answers sent to the replaced board attach to none of the loaded items.
func TestArchiveLoadStartsANewBoardIncarnation(t *testing.T) {
	clock := archiveLoadNow
	archiveLoadNow = func() time.Time { return archiveNow.Add(-time.Millisecond) }
	t.Cleanup(func() { archiveLoadNow = clock })
	root := t.TempDir()
	source := archiveSource(t, filepath.Join(root, "source"))
	dest := archiveDestination(t, filepath.Join(root, "dest"))
	data, err := source.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	loaded, original := folderOpen(t, source.Dir, nil), folderOpen(t, dest.Dir, nil)
	if loaded.Seq >= original.Seq {
		t.Fatalf("fixture must lower the render sequence: loaded %d, replaced %d", loaded.Seq, original.Seq)
	}
	if _, _, err := dest.LoadArchive(writeArchive(t, filepath.Join(root, "b.zip"), data), archiveNow); err != nil {
		t.Fatal(err)
	}
	got := folderOpen(t, dest.Dir, nil)
	if got.Seq <= original.Seq {
		t.Fatalf("render sequence after the load %d, want past the replaced board's %d", got.Seq, original.Seq)
	}
	if want := archiveNow.UTC().Format(reaskLayout); got.LoadedAt != want {
		t.Fatalf("LoadedAt = %q, want %q", got.LoadedAt, want)
	}
	reasked := got.ReaskTimes()
	if len(got.Items) == 0 || len(reasked) != len(got.Items) {
		t.Fatalf("ReaskTimes = %v for %d items, want every item floored", reasked, len(got.Items))
	}
	for _, item := range got.Items {
		if reasked[item.ID] != archiveNow.UnixMilli() {
			t.Fatalf("item %d re-ask floor %d, want the load time %d", item.ID, reasked[item.ID], archiveNow.UnixMilli())
		}
	}
	// A later re-ask still wins over the load floor.
	later := archiveNow.Add(time.Minute)
	got.Items[0].ReaskedAt = later.UTC().Format(reaskLayout)
	if at := got.ReaskTimes()[got.Items[0].ID]; at != later.UnixMilli() {
		t.Fatalf("re-ask after the load = %d, want %d", at, later.UnixMilli())
	}

	// Loading into a session with no board keeps the archive's sequence.
	empty := FolderStore{Dir: filepath.Join(root, "empty")}
	if _, _, err := empty.LoadArchive(filepath.Join(root, "b.zip"), archiveNow); err != nil {
		t.Fatal(err)
	}
	if got := folderOpen(t, empty.Dir, nil); got.Seq != loaded.Seq {
		t.Fatalf("render sequence into no board %d, want the archive's %d", got.Seq, loaded.Seq)
	}
}

// archiveMemoEntries counts the folder memo entries under load extractions.
func archiveMemoEntries() int {
	folderMemo.mu.Lock()
	defer folderMemo.mu.Unlock()
	n := 0
	for path := range folderMemo.files {
		if strings.Contains(path, "ws-whiteboard-load-") {
			n++
		}
	}
	for path := range folderMemo.logs {
		if strings.Contains(path, "ws-whiteboard-load-") {
			n++
		}
	}
	return n
}

// Validating an archive opens its extraction as a board, which fills the
// process-wide topic memo; every load, accepted or refused, forgets those
// entries, so repeated imports do not pile payload copies up in the daemon.
func TestArchiveLoadForgetsTheExtractionMemo(t *testing.T) {
	root := t.TempDir()
	source := archiveSource(t, filepath.Join(root, "source"))
	data, err := source.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	zipPath := writeArchive(t, filepath.Join(root, "b.zip"), data)
	baseline := archiveMemoEntries()
	dest := archiveDestination(t, filepath.Join(root, "dest"))
	if _, _, err := dest.LoadArchive(zipPath, archiveNow); err != nil {
		t.Fatal(err)
	}
	if got := archiveMemoEntries(); got != baseline {
		t.Fatalf("memo entries after a load %d, want the baseline %d", got, baseline)
	}

	// Refused after the extraction opened as a board: the destination fails.
	refused := archiveDestination(t, filepath.Join(root, "refused"))
	refused.Fault = func(stage string) error {
		if stage == "complete" {
			return folderCrash
		}
		return nil
	}
	if _, _, err := refused.LoadArchive(zipPath, archiveNow); !errors.Is(err, folderCrash) {
		t.Fatalf("LoadArchive = %v, want the injected crash", err)
	}
	if got := archiveMemoEntries(); got != baseline {
		t.Fatalf("memo entries after a refused load %d, want the baseline %d", got, baseline)
	}

	// Refused while opening: the second topic's log is broken after the
	// first decoded.
	files := unzip(t, data)
	files["t2/log.jsonl"] = []byte("garbage\n")
	broken := writeArchive(t, filepath.Join(root, "broken.zip"), zipOf(t, files))
	if _, _, err := dest.LoadArchive(broken, archiveNow); err == nil || !strings.Contains(err.Error(), "not a valid board") {
		t.Fatalf("LoadArchive = %v, want a refusal", err)
	}
	if got := archiveMemoEntries(); got != baseline {
		t.Fatalf("memo entries after a refused open %d, want the baseline %d", got, baseline)
	}
}
