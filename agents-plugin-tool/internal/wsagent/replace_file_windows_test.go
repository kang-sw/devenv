//go:build windows

package wsagent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAtomicReplaceFileWindowsReplacesExisting verifies that atomicReplaceFile
// succeeds when the destination already exists and the new contents are visible
// after the call (i.e., the rename was not silently a no-op).
func TestAtomicReplaceFileWindowsReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "target.json")
	tmp := filepath.Join(dir, "target.json.tmp")

	// Pre-populate the destination with "old content".
	if err := os.WriteFile(dst, []byte("old content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile dst: %v", err)
	}
	// Write the new content to the temp file.
	if err := os.WriteFile(tmp, []byte("new content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile tmp: %v", err)
	}

	if err := atomicReplaceFile(tmp, dst); err != nil {
		t.Fatalf("atomicReplaceFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile after replace: %v", err)
	}
	if string(got) != "new content\n" {
		t.Errorf("destination contains %q, want %q", string(got), "new content\n")
	}
	// Temp file must be gone.
	if _, err := os.Stat(tmp); err == nil {
		t.Errorf("temp file %q still exists after atomicReplaceFile", tmp)
	}
}
