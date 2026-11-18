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

// archiveSource is a board with topics, a panel, logs, items, and render
// bookkeeping a load must scrub.
func archiveSource(t *testing.T, dir string) FolderStore {
	t.Helper()
	f := FolderStore{Dir: dir}
	archiveUpdate(t, f, archiveApply(`<wb:topic name="handoff" label="Handoff">
<wb:panel recap="Design">Full panel payload</wb:panel>
<wb:log>Ready for takeover</wb:log>
<wb:add class="check"><wb:title>Verify the handoff</wb:title></wb:add>
</wb:topic>
<wb:topic name="context" label="Context"><wb:log>Keep this too</wb:log></wb:topic>`))
	archiveUpdate(t, f, func(s *State) error {
		s.EmbedDir, s.Embeds, s.LastEmbedSeq = "D:/host/embeds", []Embed{{Seq: 1, Name: "x.html"}}, 1
		return nil
	})
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
// destination's own receipt floors; they are not board content or ID counters.
func archiveComparable(s *State) *State {
	c := s.clone()
	c.MetadataRevision, c.StaleNudgeRevision = 0, ""
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
	want.EmbedDir, want.Embeds, want.LastEmbedSeq = "", nil, 0
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

	// The installed topic folders are all the board references; the old
	// ones are gone, and a new topic gets a folder of its own.
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
	for _, e := range entries {
		if e.IsDir() && folderDirID(e.Name()) && e.Name() != folderRevisionDir && !installed[e.Name()] {
			t.Errorf("old topic folder %s left behind", e.Name())
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
		if ref.Key == "after-load" && installed[ref.Dir] {
			t.Fatalf("new topic reuses installed folder %s", ref.Dir)
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
	if got := folderOpen(t, dest.Dir, nil); got == nil || len(got.Topics) != 2 || got.EmbedDir != "" {
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
