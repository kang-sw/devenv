//go:build windows

package wsstate

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAtomicReplaceFileWindowsReplacesExisting verifies that atomicReplaceFile
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

	if err := atomicReplaceFile(tmp, dst); err != nil {
		t.Fatalf("atomicReplaceFile: %v", err)
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
