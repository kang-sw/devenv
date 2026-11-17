package wsledger

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRefsStamp: the refs stamp moves with every change to a covered ref
// (a rewrite of the same size included), ignores refs outside the covered
// directories, and cannot vouch for a file younger than racyWindow or for a
// reftable repository.
func TestRefsStamp(t *testing.T) {
	git := t.TempDir()
	now := time.Now()
	old := now.Add(-time.Hour)
	write := func(rel, content string, at time.Time) {
		t.Helper()
		p := filepath.Join(git, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
	oidA, oidB := "1111111111111111111111111111111111111111\n", "2222222222222222222222222222222222222222\n"
	write("refs/heads/tracker/genesis", oidA, old)
	write("refs/heads/tracker/m/ada/c1", oidA, old)

	base := refsStamp(git, now)
	if base == "" {
		t.Fatal("settled refs must have a stamp")
	}
	if again := refsStamp(git, now); again != base {
		t.Fatal("the stamp of unchanged refs must not move")
	}

	write("refs/heads/main", oidB, old)
	write("refs/ledger/remote/tracker/m/ada/c1", oidB, old)
	if s := refsStamp(git, now); s != base {
		t.Fatal("a ref outside the covered directories must not move the stamp")
	}

	steps := []struct {
		name string
		do   func()
	}{
		{"a ref rewritten at the same size", func() { write("refs/heads/tracker/m/ada/c1", oidB, old.Add(time.Second)) }},
		{"a new ledger ref", func() { write("refs/heads/tracker/m/bo/c2", oidA, old) }},
		{"a new attachment ref", func() { write("refs/heads/attach/ada/c1", oidA, old) }},
		{"a known ref", func() { write("refs/ledger/known/tracker/m/ada/c1", oidA, old) }},
		{"packed refs", func() { write("packed-refs", oidA, old) }},
		{"a removed ref", func() {
			if err := os.Remove(filepath.Join(git, "refs", "heads", "tracker", "m", "bo", "c2")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	prev := base
	for _, s := range steps {
		s.do()
		cur := refsStamp(git, now)
		if cur == "" || cur == prev {
			t.Fatalf("%s: stamp %q, was %q; want a new one", s.name, cur, prev)
		}
		prev = cur
	}

	write("refs/heads/tracker/m/ada/c1", oidA, now.Add(-racyWindow/2))
	if s := refsStamp(git, now); s != "" {
		t.Fatal("a ref written within racyWindow must leave the stamp unable to vouch")
	}
	write("refs/heads/tracker/m/ada/c1", oidA, old)
	if err := os.Mkdir(filepath.Join(git, "reftable"), 0o755); err != nil {
		t.Fatal(err)
	}
	if s := refsStamp(git, now); s != "" {
		t.Fatal("a reftable repository must leave the stamp unable to vouch")
	}
}

// TestFileStamp: a file's stamp moves with its size or time, a missing
// file has one of its own, and a young file has none.
func TestFileStamp(t *testing.T) {
	p := filepath.Join(t.TempDir(), "journal.jsonl")
	now := time.Now()
	old := now.Add(-time.Hour)
	missing := fileStamp(p, now)
	if missing == "" {
		t.Fatal("a missing file must have a stamp")
	}
	if err := os.WriteFile(p, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := fileStamp(p, now); s != "" {
		t.Fatal("a file written just now must have no stamp")
	}
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	first := fileStamp(p, now)
	if first == "" || first == missing {
		t.Fatalf("settled file stamp %q", first)
	}
	if err := os.WriteFile(p, []byte("{}\n{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if s := fileStamp(p, now); s == "" || s == first {
		t.Fatal("an appended file's stamp must move")
	}
}

// TestStampedPrefixes: the listings refresh and Reload memoize lie under
// the stamped directories, and the remote snapshot does not.
func TestStampedPrefixes(t *testing.T) {
	for _, p := range []string{refPrefix, attachPrefix, knownRef(memberRef("ada", "c1"))} {
		if !stamped(p) {
			t.Errorf("%s must be stamped", p)
		}
	}
	for _, p := range []string{snapPrefix, "refs/heads/", wikiPrefix, "refs/heads/trackers/"} {
		if stamped(p) {
			t.Errorf("%s must not be stamped", p)
		}
	}
}
