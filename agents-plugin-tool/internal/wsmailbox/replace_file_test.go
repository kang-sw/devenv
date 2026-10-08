package wsmailbox

import (
	"os"
	"path/filepath"
	"testing"
)

// replaceFile must replace an existing destination and consume tmp on every
// platform; the Windows-only test covers the retry under an open reader.
func TestReplaceFileReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mailbox.json")
	tmp := filepath.Join(dir, "mailbox.json-1.tmp")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(tmp, path); err != nil {
		t.Fatalf("replaceFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("file = %q, want %q", got, "new")
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("tmp still present after replace: %v", err)
	}
}
