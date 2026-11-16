//go:build !windows

package wsstate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSameFileMetadataIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	a, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(path)
	if err != nil || !SameFileMetadata(a, b) {
		t.Fatal("same file did not match", err)
	}
	other := filepath.Join(dir, "other")
	if err := os.WriteFile(other, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(other, old, old); err != nil {
		t.Fatal(err)
	}
	c, err := os.Stat(other)
	if err != nil || SameFileMetadata(a, c) {
		t.Fatal("distinct files matched", err)
	}
	d, err := os.Stat(dir)
	if err != nil || SameFileMetadata(d, d) || SameFileMetadata(nil, a) {
		t.Fatal("directory or nil matched", err)
	}
}
