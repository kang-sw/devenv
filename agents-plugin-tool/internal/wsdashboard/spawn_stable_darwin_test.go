package wsdashboard

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchExecutableInstallsAndReplacesAtomically(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, stagedBinDir)
	srcDir := t.TempDir()

	v1 := filepath.Join(srcDir, "ws-mcp")
	if err := os.WriteFile(v1, []byte("daemon-binary-v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	sum1 := ExecutableContentHash(v1)

	stable1, err := LaunchExecutable(v1, dir)
	if err != nil {
		t.Fatalf("LaunchExecutable(v1): %v", err)
	}
	wantStable := filepath.Join(binDir, stableDaemonName)
	if stable1 != wantStable {
		t.Fatalf("stable path = %q, want %q", stable1, wantStable)
	}
	staged1 := filepath.Join(binDir, "ws-mcp-dashboard-"+sum1[:16])
	infoStable1, err := os.Lstat(stable1)
	if err != nil || !infoStable1.Mode().IsRegular() {
		t.Fatalf("stable Lstat = %v, %v; want regular file", infoStable1, err)
	}
	infoStaged1, err := os.Stat(staged1)
	if err != nil {
		t.Fatalf("content-addressed staged copy missing: %v", err)
	}
	if !os.SameFile(infoStable1, infoStaged1) {
		t.Fatal("stable launch path is not hardlinked to staged binary")
	}

	// Idempotent: repeated calls (and self-replace via the stable path itself)
	// keep the same inode and leave no temp artifacts.
	for _, input := range []string{v1, stable1} {
		again, err := LaunchExecutable(input, dir)
		if err != nil || again != wantStable {
			t.Fatalf("LaunchExecutable(%q) = %q, %v", input, again, err)
		}
		infoAgain, err := os.Lstat(again)
		if err != nil || !os.SameFile(infoStable1, infoAgain) {
			t.Fatalf("repeat LaunchExecutable(%q) replaced inode", input)
		}
	}

	// Hold an open descriptor on the v1 inode to model a still-draining older
	// daemon while replacing the stable launch path with v2.
	oldHandle, err := os.Open(stable1)
	if err != nil {
		t.Fatal(err)
	}
	defer oldHandle.Close()

	v2 := filepath.Join(srcDir, "ws-mcp-v2")
	if err := os.WriteFile(v2, []byte("daemon-binary-v2-longer"), 0o755); err != nil {
		t.Fatal(err)
	}
	sum2 := ExecutableContentHash(v2)

	stable2, err := LaunchExecutable(v2, dir)
	if err != nil || stable2 != wantStable {
		t.Fatalf("LaunchExecutable(v2) = %q, %v", stable2, err)
	}
	infoStable2, err := os.Lstat(stable2)
	if err != nil || !infoStable2.Mode().IsRegular() {
		t.Fatalf("stable v2 Lstat = %v, %v", infoStable2, err)
	}
	if os.SameFile(infoStable1, infoStable2) {
		t.Fatal("stable path still points to v1 inode after replacement")
	}
	staged2 := filepath.Join(binDir, "ws-mcp-dashboard-"+sum2[:16])
	infoStaged2, err := os.Stat(staged2)
	if err != nil || !os.SameFile(infoStable2, infoStaged2) {
		t.Fatalf("stable v2 is not hardlinked to staged v2: %v", err)
	}
	if got, err := os.ReadFile(stable2); err != nil || string(got) != "daemon-binary-v2-longer" {
		t.Fatalf("stable v2 content = %q, %v", got, err)
	}
	// The open handle to the old inode and the retained v1 staged copy remain intact.
	if oldBytes, err := io.ReadAll(oldHandle); err != nil || string(oldBytes) != "daemon-binary-v1" {
		t.Fatalf("old running inode content = %q, %v", oldBytes, err)
	}
	if retainedV1, err := os.ReadFile(staged1); err != nil || string(retainedV1) != "daemon-binary-v1" {
		t.Fatalf("retained v1 copy = %q, %v", retainedV1, err)
	}
	entries, err := os.ReadDir(binDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("stray temp file left in binDir: %s", e.Name())
		}
	}
}

func TestLaunchExecutableReplacesSymlinkAndPreservesRunningExecutableRecord(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, stagedBinDir)
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "ws-mcp")
	if err := os.WriteFile(src, []byte("signed-daemon-payload"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-plant a symlink at the stable path; LaunchExecutable must atomically
	// replace it with a regular file rather than treating it as already matching.
	stablePath := filepath.Join(binDir, stableDaemonName)
	if err := os.Symlink(src, stablePath); err != nil {
		t.Fatal(err)
	}
	got, err := LaunchExecutable(src, dir)
	if err != nil || got != stablePath {
		t.Fatalf("LaunchExecutable = %q, %v", got, err)
	}
	info, err := os.Lstat(got)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("stable path after symlink replacement = %v, %v; want regular file", info, err)
	}

	// Recording the running executable from the stable launch path still writes
	// the content-addressed ws-mcp-dashboard-<sum16> path expected by old launchers.
	if err := recordRunningExecutable(dir, got, "0.50.8-dev", 456); err != nil {
		t.Fatalf("recordRunningExecutable: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, RunningExecutableFile))
	if err != nil {
		t.Fatal(err)
	}
	var rec runningExecutable
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	sum := ExecutableContentHash(src)
	wantStaged := filepath.Join(binDir, "ws-mcp-dashboard-"+sum[:16])
	if rec.SchemaVersion != 1 || rec.Executable != wantStaged || rec.SHA256 != sum || rec.Version != "0.50.8-dev" {
		t.Fatalf("last-running-executable.json = %+v, want executable=%q sha256=%q", rec, wantStaged, sum)
	}
	recInfo, err := os.Lstat(rec.Executable)
	if err != nil || !recInfo.Mode().IsRegular() {
		t.Fatalf("recorded executable Lstat = %v, %v; want regular file", recInfo, err)
	}
}

func TestInstallStableExecutableCopyFallbackSetsMode(t *testing.T) {
	binDir := filepath.Join(t.TempDir(), stagedBinDir)
	// Non-executable source forces the copyExecutable fallback path.
	src := filepath.Join(t.TempDir(), "ws-mcp-noexec")
	if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst, err := installStableExecutable(src, binDir)
	if err != nil {
		t.Fatalf("installStableExecutable fallback: %v", err)
	}
	info, err := os.Lstat(dst)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("fallback stable file = %v, %v; want executable regular file", info, err)
	}
	if data, err := os.ReadFile(dst); err != nil || string(data) != "payload" {
		t.Fatalf("fallback content = %q, %v", data, err)
	}
}
