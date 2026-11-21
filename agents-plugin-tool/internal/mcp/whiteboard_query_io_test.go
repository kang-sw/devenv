package mcp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsstate"
)

func TestWhiteboardQueryHeldStoreUsesNoFolderIO(t *testing.T) {
	f := newWhiteboardFixture(t)
	defer f.s.Close()
	var commands strings.Builder
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&commands, `<wb:topic name="topic-%d"><wb:body>body</wb:body></wb:topic>`, i)
	}
	f.update(t, commands.String())
	query := func() { f.call(t, "whiteboard.query", map[string]any{"session_key": f.key}) }
	query()
	before := wsstate.FileIOCounters()
	query()
	after := wsstate.FileIOCounters()
	var stats, boardReads int64
	for site, count := range after {
		if strings.HasPrefix(site, "file_io_site:wswhiteboard/") && strings.HasSuffix(site, ":stat") {
			stats += count - before[site]
		}
		if strings.HasPrefix(site, "file_io_site:wsstate/replace_file_") && strings.HasSuffix(site, ":read_file") {
			boardReads += count - before[site]
		}
	}
	if stats != 0 || boardReads != 0 {
		t.Fatalf("held query used %d board stats and %d board reads", stats, boardReads)
	}
	f.update(t, `<wb:topic name="topic-0"><wb:body>fresh body</wb:body></wb:topic>`)
	if got := f.call(t, "whiteboard.query", map[string]any{"session_key": f.key, "payload": map[string]any{"topic": []string{"topic-0"}, "body": true}}); !strings.Contains(got, "fresh body") {
		t.Fatalf("query did not read its write: %s", got)
	}
}
