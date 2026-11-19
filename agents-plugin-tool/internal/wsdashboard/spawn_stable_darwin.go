package wsdashboard

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// stableDaemonName is the fixed executable path launched on macOS so TCC
// permissions attach to one path across rebuilds (combined with a stable code
// signing identity). Content-addressed ws-mcp-dashboard-<sum16> copies stay
// alongside it for last-running-executable.json and rollback/old-launcher
// compatibility.
const stableDaemonName = "ws-mcp-dashboard"

// LaunchExecutable ensures exe is retained in <dir>/bin and, on macOS,
// atomically installs a stable launch path (<dir>/bin/ws-mcp-dashboard)
// pointing at the same executable content so TCC sees one stable binary path.
// Renaming over the stable path while an older daemon runs from the previous
// inode is safe on POSIX.
func LaunchExecutable(exe, dir string) (string, error) {
	binDir := filepath.Join(dir, stagedBinDir)
	dst := filepath.Join(binDir, stableDaemonName)
	if dstInfo, err := os.Lstat(dst); err == nil && dstInfo.Mode().IsRegular() {
		if srcInfo, err := os.Stat(exe); err == nil && os.SameFile(srcInfo, dstInfo) {
			return dst, nil
		}
	}
	staged, err := DaemonExecutable(exe, dir)
	if err != nil {
		return "", err
	}
	return installStableExecutable(staged, binDir)
}

// installStableExecutable atomically points <binDir>/ws-mcp-dashboard at src's
// content, preferring a hardlink (which shares the inode and Mach-O code
// signature without duplicating bytes) and falling back to an atomic copy when
// linking is unavailable.
func installStableExecutable(src, binDir string) (string, error) {
	realSrc, err := filepath.EvalSymlinks(src)
	if err != nil {
		return "", err
	}
	srcInfo, err := os.Stat(realSrc)
	if err != nil {
		return "", err
	}
	if !srcInfo.Mode().IsRegular() {
		return "", fmt.Errorf("stable daemon executable source is not a regular file: %s", realSrc)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(binDir, stableDaemonName)
	if dstInfo, err := os.Lstat(dst); err == nil && dstInfo.Mode().IsRegular() && os.SameFile(srcInfo, dstInfo) {
		return dst, nil
	}
	if srcInfo.Mode().Perm()&0o111 != 0 {
		var b [6]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		tmp := filepath.Join(binDir, fmt.Sprintf(".%s.%d.%s.tmp", stableDaemonName, os.Getpid(), hex.EncodeToString(b[:])))
		if err := os.Link(realSrc, tmp); err == nil {
			if err := os.Rename(tmp, dst); err != nil {
				_ = os.Remove(tmp)
				return "", err
			}
			return dst, nil
		}
	}
	if err := copyExecutable(realSrc, dst); err != nil {
		return "", err
	}
	return dst, nil
}
