//go:build windows

package wsagent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCacheLauncherCommandSkipsShellShimOnWindows verifies that when a plugin
// cache directory contains both the extensionless POSIX shell shim
// (ws-mcp-launcher) and the Python launcher (ws-mcp-launcher.py), the Windows
// probe does NOT return the shell shim. The shell shim is not executable on
// Windows; returning it would break async/mercenary spawn.
//
// This test creates a fake Codex plugin cache layout, injects a fake exe path
// that points into it, and asserts the returned command path is not the shell
// shim.
func TestCacheLauncherCommandSkipsShellShimOnWindows(t *testing.T) {
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
		if err := os.WriteFile(p, []byte("# placeholder\n"), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", p, err)
		}
	}

	// Fake exe path inside the cache — codexPluginCacheRoot uses this to find
	// the cache root.
	fakeExe := filepath.Join(tmp, ".codex", "plugins", "cache", "kang-sw-devenv", "ws", "1.0.0", "ws-mcp.exe")
	if err := os.WriteFile(fakeExe, []byte{}, 0o644); err != nil {
		t.Fatalf("WriteFile fakeExe: %v", err)
	}

	cmd, ok := cacheLauncherCommand(fakeExe)
	if !ok {
		// No python3/python on this host; the probe may legitimately return
		// false. The important invariant is that if ok is true, it must not
		// be the shell shim.
		t.Logf("cacheLauncherCommand returned ok=false (no python on PATH); shell-shim skip is still the correct code path")
		return
	}
	if cmd.Path == shimPath {
		t.Errorf("cacheLauncherCommand returned shell shim path %q on Windows; must not select extensionless launcher", shimPath)
	}
}
