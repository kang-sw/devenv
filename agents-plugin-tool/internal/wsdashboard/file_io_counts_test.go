package wsdashboard

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsstate"
)

func TestFileIOCountersMeasureRealAttempts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing")
	before := wsstate.FileIOCounters()
	_, _ = wsstate.Stat(path)
	_, _ = wsstate.Lstat(path)
	_, _ = wsstate.ReadDir(path)
	_, _ = wsstate.Open(path)
	_, _ = wsstate.OpenFile(path, os.O_RDONLY, 0)
	_, _ = wsstate.ReadFileOnce(path)
	after := wsstate.FileIOCounters()
	var attempts int64
	for site, count := range after {
		if strings.HasPrefix(site, "file_io_site:wsdashboard/file_io_counts_test.go:") {
			attempts += count - before[site]
		}
	}
	if attempts != 6 {
		t.Fatalf("failed real operations counted %d times, want 6", attempts)
	}
	out := httptest.NewRecorder()
	(&Daemon{stats: new(streamStats)}).serveStreams(out, httptest.NewRequest("GET", "/debug/pprof/streams", nil))
	var counts map[string]int64
	if err := json.Unmarshal(out.Body.Bytes(), &counts); err != nil {
		t.Fatal(err)
	}
	if counts["file_io_total"] < 6 || counts["file_io_stat"] == 0 {
		t.Fatalf("streams omitted file operations: %s", out.Body.String())
	}
}

func TestEmptyAnswerJournalEventsDoNotRescan(t *testing.T) {
	d := &Daemon{dir: t.TempDir()}
	d.source = blockedAnswerRecorder{record: func() error { t.Fatal("empty journal replayed"); return nil }}
	reg := Registration{ReplyID: "livesession"}
	d.scheduleAnswerReplay(reg) // one startup probe
	before := wsstate.FileIOCounters()["file_io_read_dir"]
	for i := 0; i < 200; i++ {
		d.scheduleAnswerReplay(reg)
	}
	if got := wsstate.FileIOCounters()["file_io_read_dir"] - before; got != 0 {
		t.Fatalf("empty-journal events made %d directory reads, want 0", got)
	}
}
