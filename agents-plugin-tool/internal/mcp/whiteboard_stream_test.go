package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

type whiteboardStreamRegion struct {
	Name string `json:"name"`
	Hash string `json:"hash"`
	HTML string `json:"html"`
}

func openWhiteboardStreamForTest(t *testing.T, daemon *wsdashboard.Daemon, replyID string) <-chan whiteboardStreamRegion {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	url := fmt.Sprintf("http://127.0.0.1:%d/s/%s/events?regions=board,panels", daemon.Endpoint().Port, replyID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("open whiteboard stream: %s", resp.Status)
	}
	regions := make(chan whiteboardStreamRegion, 64)
	go func() {
		defer close(regions)
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 4096), 4*1024*1024)
		event := ""
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: ") {
				event = strings.TrimPrefix(line, "event: ")
			} else if event == "region" && strings.HasPrefix(line, "data: ") {
				var region whiteboardStreamRegion
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &region) == nil {
					select {
					case regions <- region:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return regions
}

func waitWhiteboardRegionForTest(t *testing.T, regions <-chan whiteboardStreamRegion, name, marker string) whiteboardStreamRegion {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case region, ok := <-regions:
			if !ok {
				t.Fatalf("stream closed before %s contained %q", name, marker)
			}
			if region.Name == name && strings.Contains(region.HTML, marker) {
				return region
			}
		case <-timer.C:
			t.Fatalf("no %s stream region containing %q", name, marker)
		}
	}
}

func startWhiteboardDaemonForTest(t *testing.T, dir string, src *DashboardSessionSource) *wsdashboard.Daemon {
	t.Helper()
	daemon, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{
		Dir: dir, Version: "test", Sessions: src, PollInterval: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(daemon.Close)
	daemon.Index().Refresh()
	daemon.Index().Settle()
	return daemon
}

func TestWhiteboardStreamRefreshesPanelLogItemAndRestart(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="stream"><wb:add class="technical"><wb:title>initial item</wb:title></wb:add><wb:body>current content</wb:body></wb:topic>`)
	dir := t.TempDir()
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root, HeartbeatAt: time.Now().UTC().Format(time.RFC3339)}
	if err := wsdashboard.WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	newSource := func() *DashboardSessionSource {
		return &DashboardSessionSource{store: newSessionStore(), stateDir: func() (string, error) { return dir, nil }}
	}
	src := newSource()
	daemon := startWhiteboardDaemonForTest(t, dir, src)
	stream := openWhiteboardStreamForTest(t, daemon, f.replyID())
	last := waitWhiteboardRegionForTest(t, stream, "board", "initial item")
	fingerprint := readFingerprint(t, src, reg)
	for _, step := range []struct {
		name, commands, marker string
	}{
		{"agent log", `<wb:topic name="stream"><wb:log>agent-line-marker</wb:log></wb:topic>`, "agent-line-marker"},
		{"panel", `<wb:topic name="stream"><wb:panel recap="panel-recap-marker">panel-payload-marker</wb:panel></wb:topic>`, "panel-recap-marker"},
		{"item patch", `<wb:patch id="1"><wb:title>patched-item-marker</wb:title></wb:patch>`, "patched-item-marker"},
	} {
		f.update(t, step.commands)
		if !daemon.Index().Nudge(f.replyID(), f.root) {
			t.Fatalf("%s nudge refused", step.name)
		}
		daemon.Index().Settle()
		next := waitWhiteboardRegionForTest(t, stream, "board", step.marker)
		if next.Hash == last.Hash {
			t.Fatalf("%s kept the board region hash", step.name)
		}
		last = next
		if nextFingerprint := readFingerprint(t, src, reg); nextFingerprint == fingerprint {
			t.Fatalf("%s kept the session fingerprint", step.name)
		} else {
			fingerprint = nextFingerprint
		}
		readPageOK(t, src, reg)
		readPageOK(t, src, reg)
		renders := boardRenders(src)
		readPageOK(t, src, reg)
		if boardRenders(src) != renders {
			t.Fatalf("%s unchanged re-read missed the board memo", step.name)
		}
	}
	before, err := newSessionStore().readWhiteboard(f.key)
	if err != nil {
		t.Fatal(err)
	}
	entriesBefore, _ := json.Marshal(before.Whiteboard.Topics[0].Entries)
	embedsBefore := f.embeds(t, f.dir())
	// An auxiliary write refreshes derived output and panels but must not
	// append a system event or duplicate the stream's agent/panel entries.
	if resp := f.call(t, "todo.add", map[string]any{"key": "aux", "title": "aux-refresh-marker"}); isToolError(resp) {
		t.Fatal(resp)
	}
	if !daemon.Index().Nudge(f.replyID(), f.root) {
		t.Fatal("auxiliary nudge refused")
	}
	daemon.Index().Settle()
	waitWhiteboardRegionForTest(t, stream, "panels", "aux-refresh-marker")
	after, err := newSessionStore().readWhiteboard(f.key)
	if err != nil {
		t.Fatal(err)
	}
	entriesAfter, _ := json.Marshal(after.Whiteboard.Topics[0].Entries)
	if string(entriesAfter) != string(entriesBefore) || strings.Join(f.embeds(t, f.dir()), ",") != strings.Join(embedsBefore, ",") {
		t.Fatal("implicit auxiliary refresh changed entries or wrote a new embed")
	}
	if !strings.Contains(f.fullPage(t), "aux-refresh-marker") || !strings.Contains(f.stamp(t), fmt.Sprintf("\"%d\"", after.Whiteboard.Seq)) {
		t.Fatal("implicit refresh did not publish the committed index page and stamp")
	}
	daemon.Close()
	restarted := startWhiteboardDaemonForTest(t, dir, newSource())
	reconnected := openWhiteboardStreamForTest(t, restarted, f.replyID())
	region := waitWhiteboardRegionForTest(t, reconnected, "board", "patched-item-marker")
	for _, marker := range []string{"agent-line-marker", "panel-recap-marker", `data-panel="p1"`} {
		if !strings.Contains(region.HTML, marker) {
			t.Fatalf("restart lost committed stream marker %q", marker)
		}
	}
	loaded, err := newSessionStore().readWhiteboard(f.key)
	if err != nil {
		t.Fatal(err)
	}
	entriesRestarted, _ := json.Marshal(loaded.Whiteboard.Topics[0].Entries)
	if string(entriesRestarted) != string(entriesBefore) {
		t.Fatal("restart or stream re-open appended history")
	}
}

func TestWhiteboardStreamMetadataSharesFileAndFolderAuthority(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="versions"><wb:body>content-marker</wb:body><wb:add class="technical"><wb:title>item-marker</wb:title></wb:add><wb:log>agent-marker</wb:log><wb:panel recap="old-recap">old-payload</wb:panel><wb:panel id="p1" recap="latest-recap">latest-payload</wb:panel></wb:topic>`)
	rec, err := newSessionStore().readWhiteboard(f.key)
	if err != nil {
		t.Fatal(err)
	}
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	served := string(readPageOK(t, dashboardPageSource(t, nil), reg).Board.Board)
	file := f.fullPage(t)
	entryTags := regexp.MustCompile(`<[^>]*\bdata-entry="e[0-9]+"[^>]*>`)
	for name, html := range map[string]string{"file index": file, "dashboard": served} {
		for _, entry := range rec.Whiteboard.Topics[0].Entries {
			found := false
			for _, tag := range entryTags.FindAllString(html, -1) {
				if strings.Contains(tag, `data-entry="`+entry.ID+`"`) && strings.Contains(tag, `data-kind="`+entry.Kind+`"`) {
					found = true
					if entry.ItemID != 0 && !strings.Contains(tag, fmt.Sprintf(`data-entry-item="%d"`, entry.ItemID)) {
						t.Errorf("%s omitted item attribution for %s", name, entry.ID)
					}
				}
			}
			if !found {
				t.Errorf("%s omitted durable %s entry locator %s", name, entry.Kind, entry.ID)
			}
			if entry.Kind == wswhiteboard.EntryPanel {
				if !strings.Contains(html, `id="`+entry.ID+`"`) {
					t.Errorf("%s omitted immutable panel-version anchor %s", name, entry.ID)
				}
				frame := `data-doc="entry:` + entry.ID + `" data-doc-key="entry:` + entry.ID + `" data-topic="versions" data-entry="` + entry.ID + `"`
				if name == "dashboard" && !strings.Contains(html, frame) {
					t.Errorf("dashboard omitted distinct panel frame identity %s", frame)
				}
			}
		}
		if strings.Count(html, `id="p1"`) != 1 || !strings.Contains(html, "latest-recap") || !strings.Contains(html, "old-recap") {
			t.Errorf("%s lost latest panel target or immutable versions", name)
		}
	}
	if !strings.Contains(file, `data-stamp="index.stamp.js"`) || filepath.Base(f.pagePath()) != "index.html" {
		t.Fatal("full output no longer names the committed session index")
	}
}
