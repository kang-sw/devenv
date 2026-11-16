package wsdashboard

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"0.48.10", "0.48.10", 0},
		{"0.48.10", "0.48.9", 1},
		{"0.48.10", "0.49.0", -1},
		{"1.0.0", "0.99.99", 1},
		{"v0.48.10", "0.48.10", 0},
		{"0.48.10-dev", "0.48.10", 1},
		{"0.48.10-dev", "0.48.11", -1},
		{"0.48.10-dev", "0.48.9", 1},
		{"0.48.10-dev", "0.48.10-dev", 0},
		{"0.48.10-alpha", "0.48.10-beta", -1},
		{"garbage", "0.0.1", -1},
		{"", "0.0.1", -1},
		{"garbage", "other", 0},
		{"1.2", "1.2.0", -1},
	} {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
		if got := CompareVersions(tc.b, tc.a); got != -tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d (antisymmetry)", tc.b, tc.a, got, -tc.want)
		}
	}
}

// TestStageExecutableCopiesAndPrunes: the daemon's binary copy is
// content-addressed and reused for the same content; staging leaves the
// other copies alone. Singleton ownership and age do not establish process
// exit or accepted/rollback retention, so even old artifacts stay.
func TestStageExecutableCopiesAndPrunes(t *testing.T) {
	src := filepath.Join(t.TempDir(), "ws-mcp.exe")
	if err := os.WriteFile(src, []byte("binary-v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(t.TempDir(), "bin")
	first, err := StageExecutable(src, binDir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(first) != binDir || filepath.Ext(first) != ".exe" {
		t.Fatalf("staged path = %s", first)
	}
	if body, _ := os.ReadFile(first); string(body) != "binary-v1" {
		t.Fatalf("staged content = %q", body)
	}
	again, err := StageExecutable(src, binDir)
	if err != nil || again != first {
		t.Fatalf("restage = %s, %v; want %s", again, err, first)
	}
	if err := os.WriteFile(src, []byte("binary-v2"), 0o755); err != nil {
		t.Fatal(err)
	}
	second, err := StageExecutable(src, binDir)
	if err != nil || second == first {
		t.Fatalf("stage v2 = %s, %v", second, err)
	}
	entries, _ := os.ReadDir(binDir)
	if len(entries) != 2 {
		t.Fatalf("bin dir holds %v, want the v1 copy kept beside v2 (the running daemon may be v1)", entries)
	}
	// Another launcher's staging in flight: a temp file and a fresh copy.
	tmp := filepath.Join(binDir, ".ws-mcp-dashboard-other.123.tmp")
	young := filepath.Join(binDir, "ws-mcp-dashboard-young.exe")
	for _, path := range []string{tmp, young} {
		if err := os.WriteFile(path, []byte("in flight"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Age alone proves neither process-use nor rollback safety.
	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(first, old, old); err != nil {
		t.Fatal(err)
	}
	PruneStagedExecutables(filepath.Dir(binDir), second)
	// Include an unknown metadata file, a directory and a symlink: none is
	// disposable just because it lives beside a launcher-owned executable.
	metadata := first + ".metadata.json"
	foreign := filepath.Join(binDir, "electron-package")
	if err := os.WriteFile(metadata, []byte(`{"accepted":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(foreign, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(binDir, "ws-mcp-dashboard-link.exe")
	linked := os.Symlink(first, link) == nil
	PruneStagedExecutables(filepath.Dir(binDir), second)
	for _, path := range []string{first, second, tmp, young, metadata, foreign} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("the prune removed %s: %v", path, err)
		}
	}
	if linked {
		if _, err := os.Lstat(link); err != nil {
			t.Fatalf("prune removed symlink: %v", err)
		}
	}
	// A daemon that runs from outside the staged directory (POSIX, where
	// the Go side never stages) prunes nothing.
	if err := os.Chtimes(young, old, old); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadDir(binDir)
	PruneStagedExecutables(filepath.Dir(binDir), src)
	if entries, _ = os.ReadDir(binDir); len(entries) != len(before) {
		t.Fatalf("a prune from outside the bin dir changed it: %v", entries)
	}
}

func TestExecutableContentHashIsTheFileSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ws-mcp")
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := ExecutableContentHash(path)
	if want, _ := fileSHA256(path); len(got) != 64 || got != want {
		t.Fatalf("ExecutableContentHash = %q, want %q", got, want)
	}
	if ExecutableContentHash(filepath.Join(t.TempDir(), "absent")) != "" {
		t.Fatal("a missing file must hash to the empty (unknown) string")
	}
}

func TestDaemonExecutableIsUnchangedOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows stages a copy; covered by TestStageExecutableCopiesAndPrunes")
	}
	if got, err := DaemonExecutable("/opt/ws-mcp", t.TempDir()); err != nil || got != "/opt/ws-mcp" {
		t.Fatalf("DaemonExecutable = %s, %v", got, err)
	}
}
