package wsdashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// stableDaemonName is the fixed executable path launched on macOS so TCC
// permissions attach to one path across rebuilds (combined with a stable code
// signing identity). Content-addressed ws-mcp-dashboard-<sum16> copies stay
// alongside it for last-running-executable.json and rollback/old-launcher
// compatibility.
const stableDaemonName = "ws-mcp-dashboard"

// LaunchExecutable ensures exe is retained in <dir>/bin and, on macOS,
// atomically installs a separate copy at <dir>/bin/ws-mcp-dashboard. A
// single-link inode avoids exposing the retained build name as another path
// to the executable when macOS identifies the daemon for privacy permissions.
func LaunchExecutable(exe, dir string) (string, error) {
	binDir := filepath.Join(dir, stagedBinDir)
	dst := filepath.Join(binDir, stableDaemonName)
	if dstInfo, err := statStableExecutable(dst); err == nil {
		if srcInfo, err := os.Stat(exe); err == nil && os.SameFile(srcInfo, dstInfo) {
			return dst, nil
		}
	}
	staged, sum, err := stageExecutable(exe, binDir)
	if err != nil {
		return "", err
	}
	srcInfo, err := os.Stat(staged)
	if err != nil {
		return "", err
	}
	if !srcInfo.Mode().IsRegular() {
		return "", fmt.Errorf("stable daemon executable source is not a regular file: %s", staged)
	}
	if dstInfo, err := statStableExecutable(dst); err == nil && dstInfo.Size() == srcInfo.Size() {
		if dstSum, err := fileSHA256(dst); err == nil && dstSum == sum {
			return dst, nil
		}
	}
	if err := copyExecutable(staged, dst); err != nil {
		return "", err
	}
	// A rename failure with matching bytes can leave the old hardlink in place.
	if _, err := statStableExecutable(dst); err != nil {
		return "", err
	}
	return dst, nil
}

func statStableExecutable(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 || info.Sys().(*syscall.Stat_t).Nlink != 1 {
		return nil, fmt.Errorf("stable daemon executable must be a regular executable with one link: %s", path)
	}
	return info, nil
}
