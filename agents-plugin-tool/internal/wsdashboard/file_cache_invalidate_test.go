package wsdashboard

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileCacheSignalInvalidationRetainsParsedSharing(t *testing.T) {
	dir := t.TempDir()
	path, other := filepath.Join(dir, "one", "part.json"), filepath.Join(dir, "two", "part.json")
	for _, p := range []string{path, other} {
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("before"), 0600); err != nil {
			t.Fatal(err)
		}
		ageFile(t, p, time.Hour)
	}
	now := time.Now()
	stats, reads := 0, 0
	c := FileCache[string]{now: func() time.Time { return now }, stat: func(p string) (os.FileInfo, error) { stats++; return os.Stat(p) }}
	load := func(p string) (string, error) { reads++; raw, err := os.ReadFile(p); return string(raw), err }
	for _, p := range []string{path, other} {
		if _, err := c.Load(p, load); err != nil {
			t.Fatal(err)
		}
	}
	// Invalidate an unchanged stable part: one authoritative stat, no decode.
	c.InvalidateUnder(filepath.Dir(path))
	if _, err := c.Load(path, load); err != nil {
		t.Fatal(err)
	}
	if stats != 3 || reads != 2 {
		t.Fatalf("unchanged invalidation: stats=%d reads=%d", stats, reads)
	}
	if err := os.WriteFile(path, []byte("after single signal"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.Load(path, load); got != "before" {
		t.Fatal("fixture did not warm TTL")
	}
	c.InvalidateUnder(filepath.Dir(path))
	for range 16 {
		if got, err := c.Load(path, load); got != "after single signal" || err != nil {
			t.Fatalf("signal read = %q, %v", got, err)
		}
		if got, err := c.Load(other, load); got != "before" || err != nil {
			t.Fatalf("unrelated cache = %q, %v", got, err)
		}
	}
	if stats != 4 || reads != 3 {
		t.Fatalf("signal + 16 readers: stats=%d reads=%d, want 4/3", stats, reads)
	}
	t.Log("single signal: one authoritative validation/read; 16 subsequent readers and unrelated paths add 0 stats/reads")
}
