//go:build !windows

package wsagent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCacheLauncherCommandPrefersShellShimOnNonWindows verifies that on
// non-Windows platforms the extensionless POSIX shell shim (ws-mcp-launcher)
// is returned before ws-mcp-launcher.py when both are present. This is the
// existing probe order that Windows breaks; this test guards the non-Windows
// path does not regress.
//
// The test is hermetic: it populates the shim file so the shim branch returns
// immediately, before the .py+python branch is ever reached (no reliance on
// python presence on PATH).
func TestCacheLauncherCommandPrefersShellShimOnNonWindows(t *testing.T) {
	// Build a fake cache tree:
	//   <tmp>/.codex/plugins/cache/kang-sw-devenv/ws/1.0.0/bin/ws-mcp-launcher
	//   <tmp>/.codex/plugins/cache/kang-sw-devenv/ws/1.0.0/bin/ws-mcp-launcher.py
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, ".codex", "plugins", "cache", "kang-sw-devenv", "ws", "1.0.0", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	shimPath := filepath.Join(binDir, "ws-mcp-launcher")
	pyPath := filepath.Join(binDir, "ws-mcp-launcher.py")
	for _, p := range []string{shimPath, pyPath} {
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatalf("WriteFile %s: %v", p, err)
		}
	}

	// Fake exe inside the cache so codexPluginCacheRoot resolves correctly.
	fakeExe := filepath.Join(tmp, ".codex", "plugins", "cache", "kang-sw-devenv", "ws", "1.0.0", "ws-mcp")
	if err := os.WriteFile(fakeExe, []byte{}, 0o755); err != nil {
		t.Fatalf("WriteFile fakeExe: %v", err)
	}

	cmd, ok := cacheLauncherCommand(fakeExe)
	if !ok {
		t.Fatal("cacheLauncherCommand returned ok=false; expected shim to be found")
	}
	if cmd.Path != shimPath {
		t.Errorf("cacheLauncherCommand returned %q, want shell shim %q", cmd.Path, shimPath)
	}
}
