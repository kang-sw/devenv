package wsdashboard

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// Synthetic capability only: even an accidental request cannot start a shell.
type stateFilterTerminalHost struct{ *fakeHolders }

func (*stateFilterTerminalHost) Scan()                          {}
func (*stateFilterTerminalHost) AddSink(wsholder.Sink)          {}
func (*stateFilterTerminalHost) Client(string) *wsholder.Client { return nil }
func (*stateFilterTerminalHost) Launch(json.RawMessage) (string, error) {
	return "", fmt.Errorf("synthetic capability only: shell launch forbidden")
}

// Explicitly opt in with cached Playwright/Chromium. The only synthetic
// boundary is owned store content; rendering, streams and closed paging are real.
func TestBoardStateFilterBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for isolated Chromium acceptance")
	}
	// Unset keeps the run disposable; set, it must name an absolute retained
	// directory. The disposable default is not t.TempDir: the first TempDir
	// call registers the removal of every test temp dir, and registering it
	// before the census cleanup below would run it after the census, which
	// then reports the fixture dirs as remaining.
	artifacts := os.Getenv("WS_BOARD_STATE_FILTER_ARTIFACTS")
	if artifacts == "" {
		dir, err := os.MkdirTemp("", "ws-board-state-filter-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		artifacts = dir
	} else if !filepath.IsAbs(artifacts) {
		t.Fatal("WS_BOARD_STATE_FILTER_ARTIFACTS must be absolute")
	}
	if err := os.MkdirAll(artifacts, 0o700); err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	var dirs, listeners []string
	receipt := func(name string, value any) {
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Error(err)
			return
		}
		if err := os.WriteFile(filepath.Join(artifacts, name+".json"), raw, 0o600); err != nil {
			t.Error(err)
		}
	}
	census := func() map[string]any {
		paths := map[string]bool{}
		ports := map[string]bool{}
		for _, dir := range dirs {
			_, err := os.Stat(dir)
			paths[dir] = err == nil
		}
		for _, address := range listeners {
			c, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
			ports[address] = err == nil
			if c != nil {
				_ = c.Close()
			}
		}
		return map[string]any{"go_pid": os.Getpid(), "fixture_dirs_exist": paths, "owned_listeners_accept": ports}
	}
	// Registered before TempDir and daemon cleanup: the final census observes
	// their completed cleanup, including the failure path.
	t.Cleanup(func() {
		after := census()
		receipt("owned-after", after)
		for _, dir := range dirs {
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Errorf("fixture directory remains: %s (%v)", dir, err)
			}
		}
		for _, address := range listeners {
			c, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
			if err == nil {
				_ = c.Close()
				t.Errorf("owned listener remains: %s", address)
			}
		}
	})
	temp := func() string { dir := t.TempDir(); dirs = append(dirs, dir); return dir }
	t.Setenv("WS_CACHE_HOME", temp())
	t.Setenv("WS_CONFIG_HOME", temp())
	// The production usage store consults HOME; an empty owned home prevents
	// browser /rail/usage requests from probing installed provider credentials.
	t.Setenv("HOME", temp())
	t.Setenv("WS_MCP_DASHBOARD_DEV_SOURCE", "")
	f := &pageFixture{dir: temp(), project: temp(), src: newPageFake()}
	if err := os.Mkdir(filepath.Join(f.project, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	ids := []string{"filterordinary", "filtersaved", "filterhundred", "filterlive"}
	states := map[string]*wswhiteboard.State{}
	apply := func(st *wswhiteboard.State, text string) error {
		_, problems := wswhiteboard.Apply(st, text, wswhiteboard.Env{Now: time.Now()})
		if len(problems) != 0 {
			return fmt.Errorf("fixture Apply: %v", problems)
		}
		return nil
	}
	for _, id := range ids {
		register(t, f.dir, id, f.project, time.Now())
		st := &wswhiteboard.State{}
		n := 35
		if id == "filterhundred" {
			n = 135
		}
		for i := 0; i < n; i++ {
			if err := apply(st, fmt.Sprintf(`<wb:topic name="archive%03d"><wb:body>Historical stateless body %d.</wb:body><wb:close result="Archived"/></wb:topic>`, i, i)); err != nil {
				t.Fatal(err)
			}
		}
		for _, state := range []string{"awaiting-confirmation", "discussing", "implementing", "waiting"} {
			for i := 0; i < 2; i++ {
				attrs := ""
				if state == "implementing" {
					attrs = ` owner="fixture-executor" eta_minutes="30"`
				}
				text := fmt.Sprintf(`<wb:topic name="%s%d" label="%s %d"><wb:body># Owned board body

State filter frame identity sentinel.</wb:body><wb:panel recap="Panel history">Panel payload.</wb:panel><wb:log>First history entry.</wb:log><wb:state value="%s"%s/></wb:topic>`, state, i, state, i, state, attrs)
				if err := apply(st, text); err != nil {
					t.Fatal(err)
				}
			}
		}
		// Two checks on one topic count once; assumed and answered checks do not.
		// An open technical item is an acknowledgement.
		asks := `<wb:topic name="discussing0"><wb:add class="check"><wb:title>Open check</wb:title></wb:add><wb:add class="check"><wb:title>Another check</wb:title></wb:add></wb:topic><wb:topic name="discussing1"><wb:add class="policy"><wb:title>Open policy</wb:title><wb:option>A</wb:option><wb:option>B</wb:option></wb:add></wb:topic><wb:topic name="waiting0"><wb:add class="check"><wb:title>Needs ack</wb:title></wb:add></wb:topic><wb:topic name="implementing0"><wb:add class="check"><wb:title>Answered check</wb:title></wb:add></wb:topic><wb:answer id="4" value="ok" assumed="true">Assumed</wb:answer><wb:answer id="5" value="ok">Answered</wb:answer><wb:topic name="waiting1"><wb:add class="technical"><wb:title>Open acknowledgement</wb:title></wb:add></wb:topic>`
		if err := apply(st, asks); err != nil {
			t.Fatal(err)
		}
		// 73ef56d52 intentionally defaults newly created topics to discussing;
		// its AI Context excludes legacy fallback. Seed pre-default data directly
		// so this fixture still verifies a genuinely metadata-free legacy topic.
		st.Topics = append(st.Topics, wswhiteboard.TopicDoc{Name: "legacy", Body: "Legacy topic without invented state."})
		states[id] = st
	}
	initial := map[string][]byte{}
	for id, st := range states {
		initial[id], err = json.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
	}
	seq := 1
	publish := func(id string) error {
		st := states[id]
		view := wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, ReplyID: id})
		view.Lang = "ko"
		board, err := wswhiteboard.RenderRegions(view)
		if err != nil {
			return err
		}
		// ReadClosedTopics may run concurrently: install an immutable snapshot.
		raw, err := json.Marshal(st)
		if err != nil {
			return err
		}
		snapshot := &wswhiteboard.State{}
		if err := json.Unmarshal(raw, snapshot); err != nil {
			return err
		}
		f.src.mu.Lock()
		f.src.boards[id] = snapshot
		f.src.mu.Unlock()
		seq++
		f.src.setPage(id, PageContent{HasBoard: true, Seq: seq, Board: board})
		return nil
	}
	for _, id := range ids {
		if err := publish(id); err != nil {
			t.Fatal(err)
		}
	}
	entries := []wslive.RingEntry{}
	for i := 0; i < 70; i++ {
		entries = append(entries, wslive.RingEntry{Kind: wslive.RingReply, Text: fmt.Sprintf("Saved history entry %d: owned filter verification conversation.", i)})
	}
	if err := wslive.AppendRing(wslive.RingDir(SessionDir(f.dir, "filtersaved")), entries); err != nil {
		t.Fatal(err)
	}
	writeSnapshot(t, f.dir, "filterlive", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 3600000, Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}}})
	f.d = startIndexedDaemon(t, f.dir, f.src, time.Hour, func(o *DaemonOptions) {
		o.noGitBadge = true
		o.Tracker = &fakeTracker{}
		o.Holders = &stateFilterTerminalHost{newFakeHolders()}
	})
	f.base = endpointURL(f.d.Endpoint(), "")
	waitFor(t, "filter sessions indexed", func() bool { _, ok := f.d.Index().Session("filterhundred"); return ok })
	var mu sync.Mutex
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		id := r.URL.Query().Get("id")
		st := states[id]
		if st == nil {
			http.NotFound(w, r)
			return
		}
		var text string
		switch r.URL.Query().Get("action") {
		case "reset":
			st = &wswhiteboard.State{}
			if err := json.Unmarshal(initial[id], st); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			states[id] = st
		case "answer-asks":
			text = `<wb:answer id="1" value="ok">Confirmed</wb:answer><wb:answer id="2" value="ok">Confirmed</wb:answer><wb:answer id="3" value="a">Selected</wb:answer>`
		case "transition":
			text = `<wb:topic name="waiting0"><wb:state value="discussing"/></wb:topic><wb:topic name="waiting1"><wb:state value="discussing"/></wb:topic>`
		case "close":
			text = `<wb:topic name="discussing0"><wb:close result="Verified"/></wb:topic>`
		default:
			http.Error(w, "unknown action", 400)
			return
		}
		if err := apply(st, text); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if err := publish(id); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		f.d.Index().Refresh()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(control.Close)
	for _, address := range []string{f.base, control.URL} {
		u, err := url.Parse(address)
		if err != nil || u.Port() == "29417" {
			t.Fatal("fixture must use port 0, never shared 29417")
		}
		listeners = append(listeners, u.Host)
	}
	receipt("owned-before", census())
	reference, err := filepath.Abs("../../../ai-docs/tickets/assets/261115-feat-ws-whiteboard-toolbar-state-filter/834b-state-filter-toolbar.html")
	if err != nil {
		t.Fatal(err)
	}
	// Go's -timeout is the sole deadline; never hide a child exit behind an
	// outer controller timeout or terminate the installed runtime.
	cmd := exec.Command(node, "testdata/board-state-filter-browser.cjs", f.base, control.URL, artifacts, reference)
	out, runErr := cmd.CombinedOutput()
	t.Log(string(out))
	if err := os.WriteFile(filepath.Join(artifacts, "browser-process.log"), out, 0o600); err != nil {
		t.Error(err)
	}
	exit := 0
	if runErr != nil {
		exit = -1
		if e, ok := runErr.(*exec.ExitError); ok {
			exit = e.ExitCode()
		}
	}
	receipt("browser-exit", map[string]any{"argv": cmd.Args, "exit_code": exit})
	if runErr != nil {
		t.Fatalf("isolated board state filter browser: %v", runErr)
	}
}
