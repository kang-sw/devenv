package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// readCacheFileReads counts the file reads wsstate.ReadFileCached made: the
// key.json reads of read-only session lookups and every config file read.
func readCacheFileReads() int64 {
	var n int64
	for site, count := range wsstate.FileIOCounters() {
		if strings.HasPrefix(site, "file_io_site:wsstate/read_cache.go:") && strings.HasSuffix(site, ":read_file") {
			n += count
		}
	}
	return n
}

// TestKeyedCallReadsKeyAndConfigAtMostOnce: the keyed gate, the handler's
// root resolution, the config session overlay, and the mailbox piggyback all
// resolve key.json, and config.get reads the config files; an unchanged file
// is served by one stat, and a changed one (by any process) is read once and
// picked up by the very next call.
func TestKeyedCallReadsKeyAndConfigAtMostOnce(t *testing.T) {
	server, _, key := manifestServer(t)
	dirs, err := server.sessions.dirs()
	if err != nil {
		t.Fatal(err)
	}
	keyPath := dirs.partPath(key, partKey)
	configPath, err := wsconfig.Path(wsconfig.Options{})
	if err != nil {
		t.Fatal(err)
	}
	// Older than the read cache's racy window, younger than the touch guard,
	// so no call rewrites the activity mark.
	settle := func(path string) {
		t.Helper()
		old := time.Now().Add(-10 * time.Second)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	writeConfig := func(value string) {
		t.Helper()
		tmp := configPath + ".test.tmp"
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(tmp, []byte(`{"overrides":{"acme.limit":"`+value+`"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := wsstate.AtomicReplaceFile(tmp, configPath); err != nil {
			t.Fatal(err)
		}
	}
	id := 961000
	get := func() (any, int64) {
		t.Helper()
		id++
		before := readCacheFileReads()
		resp := keyedCall(t, server, id, key, "config.get", map[string]any{"key": "acme.limit", "format": "json"})
		reads := readCacheFileReads() - before
		if toolIsError(t, resp) {
			t.Fatalf("config.get failed: %s", resp)
		}
		var result configGetResult
		if err := json.Unmarshal([]byte(toolText(t, resp)), &result); err != nil {
			t.Fatal(err)
		}
		return result.Value, reads
	}

	writeConfig("7")
	settle(configPath)
	settle(keyPath)
	get() // warm
	if value, reads := get(); value != float64(7) || reads != 0 {
		t.Fatalf("warm keyed call = %v with %d file reads, want 7 and none", value, reads)
	}

	// Another process replaces the config: the next call sees it.
	writeConfig("9")
	if value, _ := get(); value != float64(9) {
		t.Fatalf("call after a config change = %v, want 9", value)
	}
	settle(configPath)
	if value, reads := get(); value != float64(9) || reads != 1 {
		t.Fatalf("call after a settled config change = %v with %d file reads, want 9 and one", value, reads)
	}

	// key.json gains a session override: the next call sees it, and the
	// gate, root resolution, overlay and piggyback share one read.
	if err := server.sessions.setOverride(key, "acme.limit", "5"); err != nil {
		t.Fatal(err)
	}
	if value, _ := get(); value != float64(5) {
		t.Fatalf("call after a key.json change = %v, want 5", value)
	}
	settle(keyPath)
	if value, reads := get(); value != float64(5) || reads != 1 {
		t.Fatalf("call after a settled key.json change = %v with %d file reads, want 5 and one", value, reads)
	}
	if value, reads := get(); value != float64(5) || reads != 0 {
		t.Fatalf("warm call = %v with %d file reads, want 5 and none", value, reads)
	}
}
