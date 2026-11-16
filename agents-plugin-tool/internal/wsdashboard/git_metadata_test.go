package wsdashboard

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

func TestGitMetadataLinkedPointersRefsIndexAndWorktrees(t *testing.T) {
	base := t.TempDir()
	main := filepath.Join(base, "main")
	common := filepath.Join(main, ".git")
	linked := filepath.Join(base, "linked")
	admin := filepath.Join(common, "worktrees", "linked")
	var revision int
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		revision++
		at := time.Unix(1700000000+int64(revision), 0)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(common, "HEAD"), "ref: refs/heads/main\n")
	write(filepath.Join(common, "refs", "heads", "main"), "one\n")
	write(filepath.Join(admin, "commondir"), "../..\n")
	write(filepath.Join(admin, "HEAD"), "ref: refs/heads/topic\n")
	write(filepath.Join(admin, "index"), "index\n")
	write(filepath.Join(linked, ".git"), "gitdir: ../main/.git/worktrees/linked\n")
	stamp := func() string {
		t.Helper()
		got, err := gitMetadataStamp(linked)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	previous := stamp()
	for range 10 {
		if stamp() != previous {
			t.Fatal("unchanged metadata is nondeterministic")
		}
	}
	for _, path := range []string{
		filepath.Join(admin, "HEAD"), filepath.Join(admin, "index"), filepath.Join(common, "config"),
		filepath.Join(common, "refs", "heads", "nested", "topic"), filepath.Join(common, "refs", "remotes", "origin", "main"),
		filepath.Join(common, "packed-refs"), filepath.Join(common, "worktrees", "other", "HEAD"),
		filepath.Join(common, "worktrees", "other", "locked"),
	} {
		write(path, "updated\n")
		current := stamp()
		if current == previous {
			t.Fatalf("metadata change missed: %s", path)
		}
		previous = current
	}
	replacement := filepath.Join(admin, "HEAD-replacement")
	write(replacement, "detached replacement\n")
	if err := wsstate.AtomicReplaceFile(replacement, filepath.Join(admin, "HEAD")); err != nil {
		t.Fatal(err)
	}
	if current := stamp(); current == previous {
		t.Fatal("atomic HEAD replacement missed")
	} else {
		previous = current
	}
	if err := os.Remove(filepath.Join(common, "refs", "heads", "nested", "topic")); err != nil {
		t.Fatal(err)
	}
	if current := stamp(); current == previous {
		t.Fatal("loose ref deletion missed")
	} else {
		previous = current
	}
	write(filepath.Join(linked, "tracked"), "in-place worktree content")
	write(filepath.Join(linked, "untracked"), "untracked content")
	if stamp() != previous {
		t.Fatal("metadata probe traversed worktree contents")
	}
	// Replacing a pointer or its relative common-directory target changes
	// the hint even when the names of the worktree files stay unchanged.
	other := filepath.Join(base, "other", ".git")
	write(filepath.Join(other, "HEAD"), "detached\n")
	write(filepath.Join(admin, "commondir"), other+"\n")
	if stamp() == previous {
		t.Fatal("commondir relocation missed")
	}
	previous = stamp()
	write(filepath.Join(linked, ".git"), "gitdir: "+other+"\n")
	if stamp() == previous {
		t.Fatal("git pointer relocation missed")
	}
	if err := os.RemoveAll(other); err != nil {
		t.Fatal(err)
	}
	if _, err := gitMetadataStamp(linked); err == nil {
		t.Fatal("missing admin directory cached as success")
	}
	write(filepath.Join(other, "HEAD"), "recovered\n")
	previous = stamp()
	if err := os.Remove(filepath.Join(other, "HEAD")); err != nil {
		t.Fatal(err)
	}
	if stamp() == previous {
		t.Fatal("metadata deletion missed")
	}
}

type gitMetadataInfo struct {
	fs.FileInfo
	size int64
}

func (i gitMetadataInfo) Size() int64 { return i.size }

func TestGitMetadataIgnoresUndefinedDirectorySize(t *testing.T) {
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stamp := func(size int64) string {
		var b bytes.Buffer
		writeGitMetadata(&b, "refs", gitMetadataInfo{FileInfo: info, size: size})
		return b.String()
	}
	// Windows directory enumeration exposes EndOfFile, which is undefined
	// for directories. Inject differing values without changing the entry.
	if stamp(0) != stamp(0x123456789) {
		t.Fatal("undefined directory size changed the metadata hint")
	}
	file := filepath.Join(t.TempDir(), "HEAD")
	if err := os.WriteFile(file, []byte("ref: refs/heads/main"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if stamp(1) == stamp(2) {
		t.Fatal("regular file size no longer changes the hint")
	}
}

func TestGitReadGateSafetyExplicitFailureAndClock(t *testing.T) {
	now := time.Unix(1700000000, 0)
	var gate gitReadGate
	if !gate.needed("a", now, false) {
		t.Fatal("startup suppressed")
	}
	gate.complete("a", now, true)
	if gate.needed("a", now.Add(3*time.Minute), false) {
		t.Fatal("unchanged metadata ran commands")
	}
	if gate.delay(now.Add(3*time.Minute), gitReadInterval) != 2*time.Minute {
		t.Fatal("timer exceeded safety deadline")
	}
	if !gate.needed("a", now.Add(5*time.Minute), false) || !gate.needed("a", now, true) || !gate.needed("b", now, false) || !gate.needed("", now, false) || !gate.needed("a", now.Add(-time.Second), false) {
		t.Fatal("forced/change/error/clock recovery suppressed")
	}
	gate.complete("a", now, false)
	if !gate.needed("a", now, false) {
		t.Fatal("failed read cached")
	}
}
