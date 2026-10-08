//go:build windows

package wsmailbox

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A reader holding the registry open (os.Open omits FILE_SHARE_DELETE) must
// delay the replace, not fail it.
func TestReplaceFileWaitsOutOpenReader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mailbox-replyids.json")
	tmp := filepath.Join(dir, "mailbox-replyids.json-1.tmp")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err == nil {
		reader.Close()
		t.Skip("this filesystem replaces an open file; nothing to retry")
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		reader.Close()
	}()
	if err := replaceFile(tmp, path); err != nil {
		t.Fatalf("replace with a briefly open reader: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("file = %q, want %q", got, "new")
	}
}
