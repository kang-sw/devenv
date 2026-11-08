//go:build !windows

package wsinbox

import "os"

// POSIX renames, removes, and opens never fail because another handle holds
// the file, so the inbox file operations need no retry.

func rename(from, to string) error { return os.Rename(from, to) }

func remove(path string) error { return os.Remove(path) }

func readFile(path string) ([]byte, error) { return os.ReadFile(path) }
