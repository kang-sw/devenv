package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

func TestWhiteboardRetirementClosedSet(t *testing.T) {
	dir := t.TempDir()
	removed := []string{
		"liveboard/index.html", "liveboard/index.stamp.js",
		"deadboard/index.html", "deadboard/nested/index.stamp.js",
		"wb-mermaid-123456abcdef.js", "deadboard/nested/wb-mermaid-fedcba654321.js",
		"deadboard-0123abcd.html",
	}
	kept := []string{
		"liveboard-0123abcd.html", "notes.html", "deadboard/unrelated.html",
		"deadboard/meta.json", "deadboard/pending.json", "deadboard/committed.json",
		"deadboard/.lock", "deadboard/topics/t/log.jsonl", "deadboard/topics/t/panel.html",
		".gc-marker", "wb-mermaid-custom.js", "deadboard.stamp.js",
	}
	for _, name := range append(append([]string{}, removed...), kept...) {
		mustWrite(t, dir, name, "original "+name)
	}
	host := t.TempDir()
	mustWrite(t, host, "index.html", "host-owned")
	// WalkDir never follows a symlink into a host-owned embed directory.
	if err := os.Symlink(host, filepath.Join(dir, "external")); err != nil {
		t.Logf("symlink fixture unavailable: %v", err)
	}
	live := func() (map[string]bool, bool) { return map[string]bool{"liveboard": true}, true }
	sweepRetiredWhiteboardPages(dir, live)
	sweepRetiredWhiteboardPages(dir, live)
	for _, name := range removed {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("retired artifact %s remains: %v", name, err)
		}
	}
	for _, name := range kept {
		if got := string(readFileForTest(t, filepath.Join(dir, name))); got != "original "+name {
			t.Errorf("changed retained file %s: %q", name, got)
		}
	}
	if got := string(readFileForTest(t, filepath.Join(host, "index.html"))); got != "host-owned" {
		t.Fatal("changed host-owned embed")
	}
	if info, err := os.Stat(filepath.Join(dir, "deadboard", "nested")); err != nil || !info.IsDir() {
		t.Fatal("removed a board directory")
	}
}

func TestWhiteboardRetirementUnknownLivenessKeepsEmbeds(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "unknown-abcdef.html", "embed")
	mustWrite(t, dir, "board/index.html", "page")
	sweepRetiredWhiteboardPages(dir, func() (map[string]bool, bool) { return nil, false })
	if got := string(readFileForTest(t, filepath.Join(dir, "unknown-abcdef.html"))); got != "embed" {
		t.Fatal("unknown liveness deleted an embed")
	}
	if _, err := os.Stat(filepath.Join(dir, ".gc-marker")); !os.IsNotExist(err) {
		t.Fatal("sweep wrote a marker")
	}
}

func TestWhiteboardRetirementFirstOpenIsAsyncAndShared(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="board"><wb:log>retained</wb:log></wb:topic>`)
	entered, release, finished := make(chan struct{}, 4), make(chan struct{}), make(chan struct{}, 4)
	var scans atomic.Int32
	defer func() {
		close(release)
		if scans.Load() > 0 {
			<-finished
		}
	}()
	r := &whiteboardRetirement{sweep: func(dir string, live func() (map[string]bool, bool)) {
		scans.Add(1)
		entered <- struct{}{}
		<-release
		finished <- struct{}{}
	}}
	f.s.sessions.boardRetirement = r
	// Raw diagnostic calls do not own the cleanup.
	if _, err := f.s.sessions.readWhiteboard(f.key); err != nil {
		t.Fatal(err)
	}
	if scans.Load() != 0 {
		t.Fatal("raw process scheduled cleanup")
	}
	f.s.sessions.boardOwner = true
	readDone := make(chan error, 1)
	go func() { _, err := f.s.sessions.readWhiteboard(f.key); readDone <- err }()
	select {
	case err := <-readDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first board open waited for cleanup")
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first board open did not schedule cleanup")
	}
	src := dashboardPageSource(t, nil)
	// The daemon reader inherits the environment while a hosted caller supplies
	// that same cache explicitly; both must share one cleanup guard.
	f.s.sessions.cacheHome = os.Getenv("WS_CACHE_HOME")
	src.store.boardRetirement = r
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	for i := 0; i < 3; i++ {
		if _, err := f.s.sessions.readWhiteboard(f.key); err != nil {
			t.Fatal(err)
		}
		readPageOK(t, src, reg)
		f.call(t, "todo.add", map[string]any{"key": string(rune('a' + i)), "title": "fresh"})
	}
	if scans.Load() != 1 {
		t.Fatalf("repeated board opens scanned %d times", scans.Load())
	}
	if _, err := os.Stat(filepath.Join(f.dir(), ".gc-marker")); !os.IsNotExist(err) {
		t.Fatal("first open wrote a marker")
	}
}

func TestWhiteboardFullResultUsesConstructionOwnership(t *testing.T) {
	for _, tc := range []struct {
		name   string
		owner  bool
		served bool
		want   string
	}{
		{"raw", false, false, "page unavailable"},
		{"daemon-before-registration", true, false, "page not yet served"},
		{"daemon-served", true, true, "http://127.0.0.1:12345/s/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newWhiteboardFixture(t)
			s := NewServerWithOptions(ServerOptions{Root: f.root, Version: "test", BoardOwner: tc.owner})
			defer s.Close()
			s.sessionHarness = "claude"
			f.s = s
			if s.live.host != nil {
				t.Fatal("fixture must cover dashboard not bound")
			}
			if tc.served {
				s.dashboard.sessions = map[string]*dashboardSession{f.key: {replyID: f.replyID(), hosted: true, servedPort: 12345}}
			}
			text := f.update(t, `<wb:topic name="result"><wb:log>committed once</wb:log></wb:topic>`)
			want := tc.want
			if tc.served {
				want += f.replyID()
			}
			if resultLine(t, text, "full: ") != want || strings.Count(text, "full: ") != 1 {
				t.Fatalf("wrong full result: %s", text)
			}
			if strings.Contains(text, "start") || strings.Contains(text, "index.html") {
				t.Fatal(text)
			}
			rec, err := s.sessions.readWhiteboard(f.key)
			if err != nil || rec.Whiteboard == nil || rec.Whiteboard.Seq != 1 || rec.Whiteboard.EntrySeq != 2 {
				t.Fatalf("update must commit one sequence containing the log and default topic-state event: %+v, %v", rec.Whiteboard, err)
			}
			for _, name := range []string{"index.html", "index.stamp.js"} {
				if _, err := os.Stat(filepath.Join(f.pageDir(), name)); !os.IsNotExist(err) {
					t.Fatal("created page output", name)
				}
			}
		})
	}
}
