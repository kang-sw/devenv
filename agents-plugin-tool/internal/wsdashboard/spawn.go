package wsdashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// stagedBinDir holds the copies a daemon runs from on Windows.
const stagedBinDir = "bin"

// SpawnDetached starts exe with args as a process detached from the caller:
// its own session (POSIX) or its own process group with its own hidden
// console, outside the caller's job when the job allows it (Windows), stdio
// on the null device, and dir as its working directory. It returns once the
// process has started and reaps it in the background if it exits while the
// caller lives.
//
// On macOS the process is first offered to launchd (see launchdSpawn) so it
// escapes a utility QoS clamp inherited from the caller's tree; any launchd
// failure falls back to the plain spawn.
func SpawnDetached(exe string, args []string, dir string) error {
	if launchdSpawn(exe, args, dir) == nil {
		return nil
	}
	cmd, err := startDetached(exe, args, dir)
	if err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// SpawnDetachedStrict reports OS breakaway refusal for the Windows Electron
// prestart path. Electron may explicitly retry with job lifetime after a numeric
// access-denied signal; the retry keeps the same drain/compatibility ordering.
func SpawnDetachedStrict(exe string, args []string, dir string) error {
	cmd, err := startDetachedStrict(exe, args, dir)
	if err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// DaemonExecutable returns the binary a daemon should run from. On Windows a
// running executable locks its file and directory, which would block removal
// of a versioned plugin cache that holds ws-mcp, so the binary is copied into
// the daemon's state directory and the copy runs instead. Elsewhere exe is
// returned unchanged.
func DaemonExecutable(exe, dir string) (string, error) {
	if runtime.GOOS != "windows" {
		return exe, nil
	}
	return StageExecutable(exe, filepath.Join(dir, stagedBinDir))
}

// StageExecutable copies exe into binDir under a content-addressed name,
// reusing an existing copy. Other entries are left alone: one of them is
// the executable the running daemon was started from (it respawns itself
// from that path on a LAN mode change), and the daemon that holds the lock
// retains settled copies until their process-use and rollback safety can be
// established (PruneStagedExecutables). The launcher stages the same names.
func StageExecutable(exe, binDir string) (string, error) {
	sum, err := fileSHA256(exe)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}
	name := "ws-mcp-dashboard-" + sum[:16] + strings.ToLower(filepath.Ext(exe))
	target := filepath.Join(binDir, name)
	if existing, err := fileSHA256(target); err != nil || existing != sum {
		if err := copyExecutable(exe, target); err != nil {
			return "", err
		}
	}
	return target, nil
}

// PruneStagedExecutables currently retains every settled artifact. Holding
// the singleton does not prove the previous draining process has exited, or
// that an unused executable is not an accepted/rollback target. There is no
// authoritative retention source yet. Never infer it from age, a hash-shaped
// name, or missing metadata; directories, symlinks and unknown metadata are
// not ours to remove either. Each failed copy cleans only its own temp file
// in copyExecutable (and the Python launcher's stage_daemon_binary).
// Keep the entry point for callers, without inventing a retention API.
func PruneStagedExecutables(dir, exe string) {}

// ExecutableContentHash is the sha256 of the file at path, the daemon's
// content identity for /health (HealthInfo.ContentHash); "" when the file
// cannot be read.
func ExecutableContentHash(path string) string {
	sum, err := fileSHA256(path)
	if err != nil {
		return ""
	}
	return sum
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyExecutable writes src to a temp file beside dst and renames it into
// place, so a concurrent spawner never runs a half-written copy.
func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return fmt.Errorf("copy daemon binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o755); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, dst); err != nil {
		_ = os.Remove(name)
		// A concurrent spawner that renamed the same content first wins.
		if sum, sumErr := fileSHA256(dst); sumErr == nil {
			if srcSum, _ := fileSHA256(src); srcSum == sum {
				return nil
			}
		}
		return err
	}
	return nil
}
