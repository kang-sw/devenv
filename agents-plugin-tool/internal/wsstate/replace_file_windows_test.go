//go:build windows

package wsstate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAtomicReplaceFileWindowsReplacesExisting verifies that AtomicReplaceFile
// succeeds when the destination already exists and the new contents are visible
// after the call.
func TestAtomicReplaceFileWindowsReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "state.json")
	tmp := filepath.Join(dir, "state.json.tmp")

	if err := os.WriteFile(dst, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("WriteFile dst: %v", err)
	}
	if err := os.WriteFile(tmp, []byte("new\n"), 0o644); err != nil {
		t.Fatalf("WriteFile tmp: %v", err)
	}

	if err := AtomicReplaceFile(tmp, dst); err != nil {
		t.Fatalf("AtomicReplaceFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "new\n" {
		t.Errorf("destination = %q, want %q", string(got), "new\n")
	}
	if _, err := os.Stat(tmp); err == nil {
		t.Errorf("temp file %q still exists after replace", tmp)
	}
}

// TestAtomicReplaceFileWindowsWaitsOutAReader: a reader holding the
// destination open (a plain Go open, no FILE_SHARE_DELETE) makes MoveFileEx
// fail with ERROR_ACCESS_DENIED; the replace retries until the reader closes.
func TestAtomicReplaceFileWindowsWaitsOutAReader(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "state.json")
	tmp := filepath.Join(dir, "state.json.tmp")
	if err := os.WriteFile(dst, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(150 * time.Millisecond)
		reader.Close()
		close(released)
	}()
	if err := AtomicReplaceFile(tmp, dst); err != nil {
		t.Fatalf("AtomicReplaceFile with a brief reader: %v", err)
	}
	<-released
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Errorf("destination = %q, want %q", got, "new\n")
	}
}

// TestRemoveFileWindowsWaitsOutAReader: a plain Go reader holding the file
// fails os.Remove on Windows; RemoveFile retries until the reader closes.
func TestRemoveFileWindowsWaitsOutAReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.json")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		reader.Close()
	}()
	if err := RemoveFile(path); err != nil {
		t.Fatalf("RemoveFile with a brief reader: %v", err)
	}
	if _, err := ReadFile(path); !os.IsNotExist(err) {
		t.Fatalf("ReadFile after removal = %v, want not-exist", err)
	}
}
