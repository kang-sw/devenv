package wsdevassets

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, sub := range AssetDirs {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestOpenRequiresAssetDirs(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Error("empty root accepted")
	}
	if _, err := Open(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("missing root accepted")
	}
	if _, err := Open(t.TempDir()); err == nil {
		t.Error("root without asset directories accepted")
	}
}

func TestReadFileServesFilesAndRereads(t *testing.T) {
	root := writeTree(t)
	rel := filepath.Join(AssetDirs[0], "a.css")
	path := filepath.Join(root, rel)
	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.ReadFile(rel)
	if err != nil || string(got) != "one" {
		t.Fatalf("ReadFile = %q, %v", got, err)
	}
	if err := os.WriteFile(path, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.ReadFile(rel); string(got) != "two" {
		t.Errorf("edit not seen: %q", got)
	}
}

func TestReadFileRefusesEscape(t *testing.T) {
	root := writeTree(t)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"../secret.txt", outside, filepath.Join(AssetDirs[0], "..", "..", "..", "..", "secret.txt")} {
		if _, err := r.ReadFile(rel); err == nil {
			t.Errorf("ReadFile(%q) accepted", rel)
		}
	}
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on windows")
	}
	link := filepath.Join(AssetDirs[0], "link.txt")
	if err := os.Symlink(outside, filepath.Join(root, link)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadFile(link); err == nil {
		t.Error("symlink to a file outside the root served")
	}
	// A symlinked file that stays inside the root is fine.
	inside := filepath.Join(AssetDirs[0], "real.txt")
	if err := os.WriteFile(filepath.Join(root, inside), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok := filepath.Join(AssetDirs[0], "alias.txt")
	if err := os.Symlink("real.txt", filepath.Join(root, ok)); err != nil {
		t.Fatal(err)
	}
	if got, err := r.ReadFile(ok); err != nil || string(got) != "ok" {
		t.Errorf("inside symlink = %q, %v", got, err)
	}
}
