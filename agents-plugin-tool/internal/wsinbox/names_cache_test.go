package wsinbox

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// settle backdates the directories past namesTrustAge, as an inbox nobody
// wrote for a while; restoring the same time after a write also stands in for
// a filesystem whose directory time did not move.
func settle(t *testing.T, old time.Time, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.Chtimes(d, old, old); err != nil {
			t.Fatal(err)
		}
	}
}

func mustNames(t *testing.T, list func(string) ([]string, error), dir string) []string {
	t.Helper()
	names, err := list(dir)
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func readDirs() int64 { return wsstate.FileIOCounters()["file_io_read_dir"] }

func TestUnchangedInboxIsNotRelisted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "inbox")
	a, _ := send(t, dir, "first")
	old := time.Now().Add(-time.Hour)
	settle(t, old, dir)
	if got := mustNames(t, Names, dir); !slices.Equal(got, []string{a}) {
		t.Fatalf("Names = %v, want [%s]", got, a)
	}
	before := readDirs()
	for range 20 {
		if got := mustNames(t, Names, dir); !slices.Equal(got, []string{a}) {
			t.Fatalf("Names = %v, want [%s]", got, a)
		}
		if held, err := Held(dir); err != nil || len(held) != 1 {
			t.Fatalf("Held = %v, %v", held, err)
		}
	}
	if got := readDirs() - before; got != 0 {
		t.Fatalf("40 reads of an unchanged inbox listed it %d times, want 0", got)
	}
}

func TestRecentlyChangedInboxIsRelisted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "inbox")
	send(t, dir, "first")
	mustNames(t, Names, dir)
	before := readDirs()
	mustNames(t, Names, dir)
	if got := readDirs() - before; got != 1 {
		t.Fatalf("a directory changed within namesTrustAge was listed %d times, want 1", got)
	}
}

func TestInProcessWriteIsSeenAtOnce(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "inbox")
	a, _ := send(t, dir, "first")
	old := time.Now().Add(-time.Hour)
	settle(t, old, dir)
	mustNames(t, Names, dir)
	// Each write is seen even where the directory keeps its old time.
	b, _ := send(t, dir, "second")
	settle(t, old, dir)
	if got := mustNames(t, Names, dir); !slices.Equal(got, []string{a, b}) {
		t.Fatalf("after a send: Names = %v, want [%s %s]", got, a, b)
	}
	steered, err := Steer(dir, b)
	if err != nil {
		t.Fatal(err)
	}
	settle(t, old, dir)
	if got := mustNames(t, Names, dir); !slices.Equal(got, []string{a, steered}) {
		t.Fatalf("after a steer: Names = %v, want [%s %s]", got, a, steered)
	}
	drained := DrainedDir(dir)
	if err := os.MkdirAll(drained, 0o755); err != nil {
		t.Fatal(err)
	}
	settle(t, old, drained)
	if got := mustNames(t, DrainedNames, dir); len(got) != 0 {
		t.Fatalf("DrainedNames = %v before the drain", got)
	}
	if recs, err := Drain(dir); err != nil || len(recs) != 2 {
		t.Fatalf("Drain = %v, %v", recs, err)
	}
	settle(t, old, dir, drained)
	if got := mustNames(t, Names, dir); len(got) != 0 {
		t.Fatalf("after a drain: Names = %v, want none", got)
	}
	if got := mustNames(t, DrainedNames, dir); !slices.Equal(got, []string{a, steered}) {
		t.Fatalf("after a drain: DrainedNames = %v, want [%s %s]", got, a, steered)
	}
}

func TestExternalWriteIsSeenOnTheNextCheck(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "inbox")
	a, _ := send(t, dir, "first")
	old := time.Now().Add(-time.Hour)
	settle(t, old, dir)
	mustNames(t, Names, dir)
	// Another process's write bumps no counter here; only the directory
	// time tells.
	ext := "20261117T000000.000000000Z-0123abcd.json"
	if err := os.WriteFile(filepath.Join(dir, ext), []byte(`{"schemaVersion":1,"parts":["outside"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := mustNames(t, Names, dir); !slices.Equal(got, []string{ext, a}) {
		t.Fatalf("after an external send: Names = %v, want [%s %s]", got, ext, a)
	}
	settle(t, old.Add(time.Minute), dir)
	mustNames(t, Names, dir)
	if err := os.Remove(filepath.Join(dir, a)); err != nil {
		t.Fatal(err)
	}
	if got := mustNames(t, Names, dir); !slices.Equal(got, []string{ext}) {
		t.Fatalf("after an external drain: Names = %v, want [%s]", got, ext)
	}
}
