package wsstate

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// readCacheRacyWindow is how recently a file may have been modified and still
// be read again on every call: wider than any supported filesystem's timestamp
// granule (FAT's 2 s is the coarsest), so a same-size rewrite inside one
// granule cannot hide behind an unchanged stamp.
const readCacheRacyWindow = 2 * time.Second

type readCacheEntry struct {
	info os.FileInfo
	data []byte
}

var readCache struct {
	mu      sync.Mutex
	entries map[string]readCacheEntry
}

// ReadFileCached returns path's bytes, revalidated by one stat per call
// instead of a read. A cached copy is served only while the file stats as it
// did before the read that produced it (mode, size, mtime, and file identity)
// and that read happened at least readCacheRacyWindow after its mtime;
// otherwise the file is read again. The stat comes first, so a write landing
// between stat and read is cached under the older stamp and read again next
// time. A failed stat or read is returned uncached (a missing file stays
// fs.ErrNotExist).
//
// It is for small files that every writer replaces whole (atomic temp and
// rename) or edits by hand: an in-place edit that keeps size and mtime is not
// seen. The returned slice is shared: callers must not modify it.
func ReadFileCached(path string) ([]byte, error) {
	path = filepath.Clean(path)
	info, err := Stat(path)
	if err != nil {
		readCache.mu.Lock()
		delete(readCache.entries, path)
		readCache.mu.Unlock()
		return nil, err
	}
	readCache.mu.Lock()
	cached, ok := readCache.entries[path]
	readCache.mu.Unlock()
	if ok && SameFileMetadata(cached.info, info) {
		return cached.data, nil
	}
	data, err := ReadFileOnce(path)
	if err != nil {
		return nil, err
	}
	readCache.mu.Lock()
	if time.Since(info.ModTime()) >= readCacheRacyWindow {
		if readCache.entries == nil {
			readCache.entries = map[string]readCacheEntry{}
		}
		readCache.entries[path] = readCacheEntry{info: info, data: data}
	} else {
		delete(readCache.entries, path)
	}
	readCache.mu.Unlock()
	return data, nil
}
