package wsstate

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

// File I/O counters measure actual attempted operations, including failures and
// retries, not inferred syscall costs. ReadFile includes its internal open/read;
// atomic_replace is a replacement attempt, not an extra open. They do not count
// kernel calls hidden inside os, directory-entry Info, or third-party flock.
var fileIOSites sync.Map // operation + source location -> *atomic.Int64

func countFileIO(operation string) {
	_, file, line, _ := runtime.Caller(2)
	site := fmt.Sprintf("file_io_site:%s/%s:%d:%s", filepath.Base(filepath.Dir(file)), filepath.Base(file), line, operation)
	v, ok := fileIOSites.Load(site)
	if !ok {
		v, _ = fileIOSites.LoadOrStore(site, new(atomic.Int64))
	}
	v.(*atomic.Int64).Add(1)
}

// CountFileIO instruments operations that cannot use an os-compatible wrapper,
// notably platform-specific atomic replacement attempts. Call at the real call,
// inside any retry loop so every attempt is included.
func CountFileIO(operation string) { countFileIO(operation) }

// FileIOCounters returns process-lifetime totals and per-source-site counts.
func FileIOCounters() map[string]int64 {
	out := map[string]int64{"file_io_total": 0}
	fileIOSites.Range(func(k, v any) bool {
		site := k.(string)
		n := v.(*atomic.Int64).Load()
		out[site] = n
		operation := site[strings.LastIndexByte(site, ':')+1:]
		out["file_io_"+operation] += n
		out["file_io_total"] += n
		return true
	})
	return out
}

func Stat(path string) (os.FileInfo, error) {
	countFileIO("stat")
	return os.Stat(path)
}

func Lstat(path string) (os.FileInfo, error) {
	countFileIO("lstat")
	return os.Lstat(path)
}

func ReadDir(path string) ([]os.DirEntry, error) {
	countFileIO("read_dir")
	return os.ReadDir(path)
}

func Open(path string) (*os.File, error) {
	countFileIO("open")
	return os.Open(path)
}

func OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	countFileIO("open_file")
	return os.OpenFile(path, flag, perm)
}

func CreateTemp(dir, pattern string) (*os.File, error) {
	countFileIO("create_temp")
	return os.CreateTemp(dir, pattern)
}

// ReadFileOnce counts one attempt without adding sharing-violation retries.
func ReadFileOnce(path string) ([]byte, error) {
	countFileIO("read_file")
	return os.ReadFile(path)
}
