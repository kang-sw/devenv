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
func SpawnDetached(exe string, args []string, dir string) error {
	cmd, err := startDetached(exe, args, dir)
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
// prunes the directory to its own executable (PruneStagedExecutables). The
// launcher's stage_daemon_binary does the same.
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

// PruneStagedExecutables removes every entry of dir's staged-binary
// directory except the one exe names, when exe lives there. The daemon
// calls it once it holds the lock: only then is every other copy known
// unused (the previous daemon is gone, and a copy staged for a start that
// lost the lock race is not needed). A copy still running cannot be
// deleted on Windows; that failure is ignored and a later daemon removes it.
func PruneStagedExecutables(dir, exe string) {
	binDir := filepath.Join(dir, stagedBinDir)
	if filepath.Dir(exe) != binDir {
		return
	}
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.Name() != filepath.Base(exe) {
			_ = os.RemoveAll(filepath.Join(binDir, entry.Name()))
		}
	}
}

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
