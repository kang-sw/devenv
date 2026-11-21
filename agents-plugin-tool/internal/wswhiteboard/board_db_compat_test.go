package wswhiteboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The archive source must be a temporary git archive, never the live worktree.
const historicalArchiveDriver = `// Build this driver inside the historical source tree being tested.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: compat-old DIR read|update|export|load [argument]")
	}
	store := wswhiteboard.FolderStore{Dir: os.Args[1]}
	switch os.Args[2] {
	case "read":
	case "update":
		if len(os.Args) != 4 {
			return fmt.Errorf("update requires commands")
		}
		if err := store.Update(nil, func(s *wswhiteboard.State) error {
			_, problems := wswhiteboard.Apply(s, os.Args[3], wswhiteboard.Env{Now: time.Date(2026, 11, 16, 1, 2, 3, 0, time.UTC)})
			if len(problems) != 0 {
				return fmt.Errorf("apply: %v", problems)
			}
			return nil
		}); err != nil {
			return err
		}
	case "export":
		if len(os.Args) != 4 {
			return fmt.Errorf("export requires a path")
		}
		exporter, ok := any(store).(interface{ ExportArchive() ([]byte, error) })
		if !ok {
			return fmt.Errorf("historical version has no archive export")
		}
		data, err := exporter.ExportArchive()
		if err != nil {
			return err
		}
		return os.WriteFile(os.Args[3], data, 0600)
	case "load":
		if len(os.Args) != 4 {
			return fmt.Errorf("load requires a path")
		}
		loader, ok := any(store).(interface {
			LoadArchive(string, time.Time) (string, int, error)
		})
		if !ok {
			return fmt.Errorf("historical version has no archive load")
		}
		if _, _, err := loader.LoadArchive(os.Args[3], time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown command %q", os.Args[2])
	}
	s, err := store.Open(nil)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(s)
}
`

// Reproduce with git archive of 8cd2566ec and 2d528c09b into separate
// temporary directories. Copy testdata/compat-old into the 8cd source's
// internal/wswhiteboard/testdata, build it there, and set WS_COMPAT_OLD_8CD
// to that binary and WS_COMPAT_ZIP_SOURCE to the root of the 2d archive.
func TestBoardDBHistoricalCompatibility(t *testing.T) {
	old, zipSource := os.Getenv("WS_COMPAT_OLD_8CD"), os.Getenv("WS_COMPAT_ZIP_SOURCE")
	if old == "" && zipSource == "" {
		t.Skip("set WS_COMPAT_OLD_8CD and WS_COMPAT_ZIP_SOURCE to the old binary and historical archive source")
	}
	if old == "" || zipSource == "" {
		t.Fatal("both WS_COMPAT_OLD_8CD and WS_COMPAT_ZIP_SOURCE are required")
	}
	driverDir, err := os.MkdirTemp(filepath.Join(zipSource, "agents-plugin-tool", "internal", "wswhiteboard"), "compat-driver-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(driverDir) })
	if err := os.WriteFile(filepath.Join(driverDir, "main.go"), []byte(historicalArchiveDriver), 0600); err != nil {
		t.Fatal(err)
	}
	zipOld := filepath.Join(t.TempDir(), "historical-zip-driver")
	build := exec.Command("go", "build", "-o", zipOld, ".")
	build.Dir = driverDir
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build historical archive driver: %v\n%s", err, output)
	}
	run := func(binary, dir string, args ...string) []byte {
		t.Helper()
		out, err := exec.Command(binary, append([]string{dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", binary, args, err, out)
		}
		return out
	}
	readState := func(data []byte) *State {
		t.Helper()
		var s State
		if err := json.Unmarshal(data, &s); err != nil {
			t.Fatalf("decode historical state: %v\n%s", err, data)
		}
		return &s
	}
	source := filepath.Join(t.TempDir(), "source")
	command := `<wb:topic name="work"><wb:body>body</wb:body><wb:panel recap="status">panel</wb:panel></wb:topic>`
	original := readState(run(zipOld, source, "update", command))
	oldZip := filepath.Join(t.TempDir(), "old.zip")
	run(zipOld, source, "export", oldZip)
	board := openTestBoardDB(t, source, nil, BoardDBOptions{})
	if !reflect.DeepEqual(board.State(), original) {
		t.Fatalf("import differs: old=%s new=%s", folderJSON(original), folderJSON(board.State()))
	}
	archive, err := board.ExportArchive()
	if err != nil {
		t.Fatal(err)
	}
	newZip := filepath.Join(t.TempDir(), "new.zip")
	if err := os.WriteFile(newZip, archive, 0600); err != nil {
		t.Fatal(err)
	}
	want := board.State()
	if err := board.Close(); err != nil {
		t.Fatal(err)
	}
	snapshot := func() map[string][]byte {
		t.Helper()
		files := map[string][]byte{}
		err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			if info.IsDir() {
				files[rel+"/"] = nil
				return nil
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("unexpected file type %s", path)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			files[rel] = data
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return files
	}
	before := snapshot()
	for _, args := range [][]string{{"read"}, {command}} {
		out, err := exec.Command(old, append([]string{source}, args...)...).CombinedOutput()
		if err == nil || !bytes.Contains(out, []byte("newer schema_version 3")) {
			t.Fatalf("old %v: err=%v output=%s", args, err, out)
		}
		if after := snapshot(); !reflect.DeepEqual(before, after) {
			t.Fatalf("old %v changed folder bytes or entries", args)
		}
		t.Logf("8cd operation refused: %s; folder entries and bytes unchanged", strings.TrimSpace(string(out)))
	}
	reopened := openTestBoardDB(t, source, nil, BoardDBOptions{})
	if !reflect.DeepEqual(reopened.State(), want) {
		t.Fatal("new DB reopen changed after old refusals")
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	oldLoaded := readState(run(zipOld, filepath.Join(t.TempDir(), "old-load"), "load", newZip))
	// Loading establishes its own sequence and answer floor; all board content remains equal.
	oldLoaded.Seq, oldLoaded.LoadedAt = want.Seq, want.LoadedAt
	if !reflect.DeepEqual(oldLoaded, want) {
		t.Fatalf("old zip load differs: want=%s got=%s", folderJSON(want), folderJSON(oldLoaded))
	}
	t.Log("historical LoadArchive accepted new export; full state equal after load sequence/floor normalization")
	dest := openTestBoardDB(t, filepath.Join(t.TempDir(), "new-load"), &State{}, BoardDBOptions{})
	if _, _, err := dest.LoadArchive(oldZip, time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	loaded := dest.State()
	loaded.Seq, loaded.LoadedAt = original.Seq, original.LoadedAt
	if !reflect.DeepEqual(loaded, original) {
		t.Fatalf("new zip load differs: want=%s got=%s", folderJSON(original), folderJSON(loaded))
	}
	t.Log("new BoardDB.LoadArchive accepted historical export; full state equal after load sequence/floor normalization")
}
