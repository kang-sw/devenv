package wsdashboard

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
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
// content-addressed, reused for the same content, and every other copy in the
// directory is deleted (on Windows, a running copy survives until later).
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
	if len(entries) != 1 || filepath.Join(binDir, entries[0].Name()) != second {
		t.Fatalf("bin dir holds %v, want only %s", entries, second)
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
