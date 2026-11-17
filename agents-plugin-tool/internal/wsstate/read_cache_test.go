package wsstate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// readCacheReads counts read_file attempts made by ReadFileCached itself.
func readCacheReads() int64 {
	var n int64
	for site, count := range FileIOCounters() {
		if strings.HasPrefix(site, "file_io_site:wsstate/read_cache.go:") && strings.HasSuffix(site, ":read_file") {
			n += count
		}
	}
	return n
}

func backdate(t *testing.T, path string) {
	t.Helper()
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

func TestReadFileCachedServesAnUnchangedFileByStat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	backdate(t, path)
	before := readCacheReads()
	for i := 0; i < 3; i++ {
		got, err := ReadFileCached(path)
		if err != nil || string(got) != `{"a":1}` {
			t.Fatalf("read %d = %q, %v", i, got, err)
		}
	}
	if reads := readCacheReads() - before; reads != 1 {
		t.Fatalf("three reads of an unchanged file read it %d times, want 1", reads)
	}

	// Another process replaces it whole: the next call sees the new bytes.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(`{"a":22}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AtomicReplaceFile(tmp, path); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadFileCached(path); err != nil || string(got) != `{"a":22}` {
		t.Fatalf("read after replace = %q, %v", got, err)
	}

	// A removed file is not served from the cache.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFileCached(path); !os.IsNotExist(err) {
		t.Fatalf("read of a removed file = %v, want not exist", err)
	}
}

func TestReadFileCachedRereadsARecentlyModifiedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key.json")
	if err := os.WriteFile(path, []byte(`{"v":"a"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFileCached(path); err != nil {
		t.Fatal(err)
	}
	// Same size, same mtime: only the racy window catches this rewrite.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"v":"b"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadFileCached(path); err != nil || string(got) != `{"v":"b"}` {
		t.Fatalf("read after a same-stamp rewrite = %q, %v", got, err)
	}
}
