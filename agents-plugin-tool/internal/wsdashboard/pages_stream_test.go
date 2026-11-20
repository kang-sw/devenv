// Page stream tests cover region updates and rendered state.

package wsdashboard

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// sseEvent is one parsed SSE event.
type sseEvent struct {
	name string
	data string
}

func readEvents(t *testing.T, r *bufio.Reader, out chan<- sseEvent) {
	var ev sseEvent
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			close(out)
			return
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "":
			if ev.name != "" {
				out <- ev
			}
			ev = sseEvent{}
		case strings.HasPrefix(line, "event: "):
			ev.name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			ev.data = strings.TrimPrefix(line, "data: ")
		}
	}
}

func nextRegion(t *testing.T, events <-chan sseEvent, timeout time.Duration) (regionEvent, bool) {
	t.Helper()
	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatal("stream closed")
		}
		// Every stream's first event names the assets identity, which
		// assets_id_test.go pins; region readers skip it.
		if ev.name == "assets" {
			return nextRegion(t, events, timeout)
		}
		if ev.name != "region" {
			return regionEvent{Name: "event:" + ev.name}, true
		}
		var re regionEvent
		if err := json.Unmarshal([]byte(ev.data), &re); err != nil {
			t.Fatal(err)
		}
		return re, true
	case <-time.After(timeout):
		return regionEvent{}, false
	}
}

var regionHashPattern = regexp.MustCompile(`data-region="([a-z]+)" data-h="([0-9a-f]+)"`)

// heldOf lists the region hashes a rendered page holds, as the page script
// sends them.
func heldOf(page string) string {
	var parts []string
	for _, m := range regionHashPattern.FindAllStringSubmatch(page, -1) {
		parts = append(parts, m[1]+"."+m[2])
	}
	return strings.Join(parts, ",")
}

func TestStreamSendsOnlyChangedRegions(t *testing.T) {
	f := startPageFixture(t)
	_, page := get(t, f.base+"/s/boardsession?frame=1")
	modeHash := newRegion(modeBoard).Hash
	client := &http.Client{Transport: localhostTestHTTPTransport()}
	resp, err := client.Get(f.base + "/s/boardsession/events?frame=1&h=" + heldOf(page) + ",mode." + modeHash)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	events := make(chan sseEvent, 16)
	go readEvents(t, bufio.NewReader(resp.Body), events)

	// The page is current: nothing is sent.
	if ev, ok := nextRegion(t, events, 400*time.Millisecond); ok {
		t.Fatalf("a current page received %+v", ev)
	}

	// The board changes; a nudge sends exactly the board region.
	f.src.setPage("boardsession", PageContent{HasBoard: true, Seq: 2, Board: boardRegions(t, "the board changed")})
	f.d.index.Nudge("boardsession", f.project)
	f.d.index.Settle()
	var got []string
	for {
		ev, ok := nextRegion(t, events, 800*time.Millisecond)
		if !ok {
			break
		}
		got = append(got, ev.Name)
		if ev.Name == regionBoard && !strings.Contains(ev.HTML, "the board changed") {
			t.Errorf("board event lacks the new board: %s", ev.HTML)
		}
	}
	// The head carries the board's sequence number, so it changes too.
	if strings.Join(got, ",") != "head,board" {
		t.Fatalf("regions sent = %v, want head,board", got)
	}

	// The session leaves the index: the stream says so and ends.
	if err := os.RemoveAll(SessionDir(f.dir, "boardsession")); err != nil {
		t.Fatal(err)
	}
	f.d.index.Refresh()
	if ev, ok := nextRegion(t, events, 2*time.Second); !ok || ev.Name != "event:gone" {
		t.Fatalf("after removal got %+v (%v)", ev, ok)
	}
}

func TestStreamSendsTheModeWhenTheAdapterAppears(t *testing.T) {
	f := startPageFixture(t)
	_, page := get(t, f.base+"/s/boardsession?frame=1")
	client := &http.Client{Transport: localhostTestHTTPTransport()}
	resp, err := client.Get(f.base + "/s/boardsession/events?frame=1&h=" + heldOf(page) + ",mode." + newRegion(modeBoard).Hash)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := make(chan sseEvent, 16)
	go readEvents(t, bufio.NewReader(resp.Body), events)

	writeSnapshot(t, f.dir, "boardsession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: time.Now().UnixMilli(), HeartbeatMs: 30000})
	f.d.broker.wake("boardsession")
	ev, ok := nextRegion(t, events, 2*time.Second)
	if !ok || ev.Name != regionMode || ev.HTML != modeLive {
		t.Fatalf("first event = %+v (%v), want the live mode", ev, ok)
	}
}

func TestRegionHashIgnoresRenderTime(t *testing.T) {
	// Times render as fixed UTC text; the page script formats them, so a
	// re-render of unchanged state hashes the same.
	view := SessionView{ReplyID: "x", Adapter: true, Lead: LeadView{Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseThinking, PhaseStartedAt: testNow.UnixMilli()}}}}
	a, err := renderSessionRegions(view, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	b, _ := renderSessionRegions(view, nil)
	if a[regionTree].Hash != b[regionTree].Hash {
		t.Fatal("tree hash changed between identical renders")
	}
	if !strings.Contains(string(a[regionTree].HTML), `data-f="since"`) {
		t.Errorf("tree has no client-formatted time: %s", a[regionTree].HTML)
	}
}

func TestActivityElapsedTimers(t *testing.T) {
	now := time.Now()
	phase := now.Add(-18 * time.Second).UnixMilli()
	turn := now.Add(-72 * time.Second).UnixMilli()
	for _, tc := range []struct {
		name, lang  string
		phase, turn int64
		wantTimes   int
		wantTurn    bool
	}{
		{"both English", "en", phase, turn, 2, true},
		{"both Korean", "ko", phase, turn, 2, true},
		{"phase only", "en", phase, 0, 1, false},
		{"turn only", "en", 0, turn, 1, false},
		{"neither", "en", 0, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lead := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseThinking, PhaseStartedAt: tc.phase}, TurnStartedAt: tc.turn}
			view := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, lead), Now: now})
			view.Lang = tc.lang
			regions, err := renderSessionRegions(view, nil)
			if err != nil {
				t.Fatal(err)
			}
			again, err := renderSessionRegions(view, nil)
			if err != nil {
				t.Fatal(err)
			}
			if regions[regionConvLive].Hash != again[regionConvLive].Hash {
				t.Fatal("unchanged activity changed its region hash")
			}
			activity := string(regions[regionConvLive].HTML)
			if got := strings.Count(activity, `data-f="since"`); got != tc.wantTimes {
				t.Fatalf("time count = %d, want %d: %s", got, tc.wantTimes, activity)
			}
			if tc.phase > 0 {
				want := `data-ms="` + strconv.FormatInt(tc.phase, 10) + `" data-f="since">18s</time>`
				if !strings.Contains(activity, want) {
					t.Errorf("phase time lacks initial elapsed text %q: %s", want, activity)
				}
			}
			if tc.turn > 0 {
				want := `data-ms="` + strconv.FormatInt(tc.turn, 10) + `" data-f="since">1m 12s</time>`
				if !strings.Contains(activity, want) {
					t.Errorf("turn time lacks initial elapsed text %q: %s", want, activity)
				}
			}
			turnLabel := "(turn "
			if tc.lang == "ko" {
				turnLabel = "(턴 "
			}
			if got := strings.Contains(activity, turnLabel); got != tc.wantTurn {
				t.Errorf("turn label present = %t, want %t: %s", got, tc.wantTurn, activity)
			}
			if tc.wantTimes == 0 && strings.Contains(activity, " · ") {
				t.Errorf("empty elapsed separator: %s", activity)
			}
		})
	}
}

func TestRenderedPermissionWaitAndEndedSessions(t *testing.T) {
	now := testNow
	lead := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhasePermission,
		Permission: &wslive.Permission{ToolName: "Bash", ArgsPreview: "rm -rf <build>", StartedAt: now.UnixMilli()}}}
	view := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, lead), Now: now})
	set, err := renderSessionRegions(view, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The conversation says it in words; the tree's one-line lead row shows
	// the approval phase with the tool and its arguments. No lead region
	// exists to say it a third time.
	conv := string(set[regionConvLive].HTML)
	if !strings.Contains(conv, "waiting for approval in the terminal") || !strings.Contains(conv, "rm -rf &lt;build&gt;") {
		t.Errorf("conversation region lacks the escaped permission wait: %s", conv)
	}
	tree := string(set[regionTree].HTML)
	for _, want := range []string{`class="phase permission"`, `<span class="tn" title="Bash">Bash</span>`, "rm -rf &lt;build&gt;"} {
		if !strings.Contains(tree, want) {
			t.Errorf("tree region lacks %q in the permission phase: %s", want, tree)
		}
	}
	if _, ok := set["lead"]; ok {
		t.Error("a lead region is still rendered")
	}

	old := liveSession("oldend", "/a", now.Add(-48*time.Hour))
	old.EndedAt = stampFor(now.Add(-48 * time.Hour))
	idx, err := renderIndexRegions(buildIndexView([]indexInput{
		{Session: liveSession("livenow", "/a", now)},
		{Session: endedSession("recentend", "/a", now)},
		{Session: old},
	}, now, "1.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(idx[regionSessions].HTML)
	for _, want := range []string{`href="/s/livenow"`, `<div class="endedhead">ended</div>`, `class="sess ended" href="/s/recentend"`, "1 sessions ended more than a day ago"} {
		if !strings.Contains(html, want) {
			t.Errorf("index region lacks %q: %s", want, html)
		}
	}
	if strings.Contains(html, "oldend") {
		t.Errorf("a session ended two days ago is listed")
	}
}

// TestRenderedCompaction pins the compaction rows: while the lead compacts,
// the conversation's live region shows one running compaction line in place
// of the generic activity line and the tree's lead row shows the phase; a
// landed or failed compaction is a ring entry line naming its trigger, and a
// failed one shows the harness's error when the ring entry carries it.
func TestRenderedCompaction(t *testing.T) {
	now := testNow
	started := now.Add(-3 * time.Second).UnixMilli()
	lead := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseCompacting, PhaseStartedAt: started}, TurnStartedAt: started}
	ring := []wslive.RingRecord{
		{RingEntry: wslive.RingEntry{Kind: wslive.RingCompact, Trigger: wslive.TriggerAuto}, Name: "20261107T000000.000000001Z-a1.json", At: now},
		{RingEntry: wslive.RingEntry{Kind: wslive.RingCompact, Trigger: wslive.TriggerManual}, Name: "20261107T000000.000000002Z-a2.json", At: now},
		{RingEntry: wslive.RingEntry{Kind: wslive.RingCompact, CompactFailed: true}, Name: "20261107T000000.000000003Z-a3.json", At: now},
		{RingEntry: wslive.RingEntry{Kind: wslive.RingCompact, CompactFailed: true, Text: "Conversation too long"}, Name: "20261107T000000.000000004Z-a4.json", At: now},
	}
	view := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, lead), Ring: ring, Now: now})
	set, err := renderSessionRegions(view, nil)
	if err != nil {
		t.Fatal(err)
	}
	conv := string(set[regionConvLive].HTML)
	if !strings.Contains(conv, `<div class="sysline compaction running">`) || !strings.Contains(conv, "compacting context…") || !strings.Contains(conv, `data-ms="`+strconv.FormatInt(started, 10)+`"`) {
		t.Errorf("live region lacks the running compaction line: %s", conv)
	}
	if strings.Contains(conv, `class="activity"`) {
		t.Errorf("live region also shows the generic activity line: %s", conv)
	}
	if tree := string(set[regionTree].HTML); !strings.Contains(tree, `<span class="phase compacting">compacting</span>`) {
		t.Errorf("tree's lead row lacks the compacting phase: %s", tree)
	}
	entries := string(set[regionEntries].HTML)
	for _, want := range []string{
		`<div class="sysline compaction" data-entry="20261107T000000.000000001Z-a1.json"><span class="mk">✓</span>context compacted (auto)`,
		`<div class="sysline compaction" data-entry="20261107T000000.000000002Z-a2.json"><span class="mk">✓</span>context compacted (manual)`,
		`<div class="sysline compaction fail" data-entry="20261107T000000.000000003Z-a3.json"><span class="mk">✗</span><span class="nt">context compaction failed</span><time`,
		`<div class="sysline compaction fail" data-entry="20261107T000000.000000004Z-a4.json"><span class="mk">✗</span><span class="nt">context compaction failed: Conversation too long</span><time`,
	} {
		if !strings.Contains(entries, want) {
			t.Errorf("entries region lacks %q: %s", want, entries)
		}
	}
}

func TestToolNameAndShortID(t *testing.T) {
	for in, want := range map[string]string{
		"mcp__plugin_ws_ws__tickets_create_empty": "tickets_create_empty",
		"mcp__srv__do": "do",
		"mcp__broken":  "mcp__broken",
		"Bash":         "Bash",
	} {
		if got := toolName(in); got != want {
			t.Errorf("toolName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := shortID("0123456789abcdef0"); got != "012345" {
		t.Errorf("shortID = %q", got)
	}
	if got := shortID("abc"); got != "abc" {
		t.Errorf("shortID short = %q", got)
	}
}

// TestTreeRowsAreDense pins the compact tree: the lead's tool shows with its
// MCP prefix stripped (full name in the title) on the one summary row, a
// subagent's caption sits on its own row, a thinking agent with no calls yet
// shows its two placeholder slots (no live row repeating its pill), and a
// lone call reads "1 call".
func TestTreeRowsAreDense(t *testing.T) {
	now := testNow
	lead := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseTool, Batches: []wslive.BatchView{
		{Running: &wslive.RunningView{ToolName: "mcp__plugin_ws_ws__tickets_create_empty", ArgsPreview: "title x", StartedAt: now.UnixMilli()}},
		{ToolNames: []string{"Read"}, CallCount: 1, EndedAt: now.UnixMilli()},
	}}}
	agent := wslive.Agent{ID: "0123456789abcdef0", Status: wslive.StatusRunning, StartedAt: now.UnixMilli(),
		Activity: wslive.Activity{Phase: wslive.PhaseThinking, PhaseStartedAt: now.UnixMilli(), Caption: "ws:effort-high"}}
	view := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, lead, agent), Now: now})
	set, err := renderSessionRegions(view, nil)
	if err != nil {
		t.Fatal(err)
	}
	tree := string(set[regionTree].HTML)
	for _, want := range []string{
		`<span class="tn" title="mcp__plugin_ws_ws__tickets_create_empty">tickets_create_empty</span>`,
		`<span class="acap" title="ws:effort-high">ws:effort-high</span>`,
		`>agent 012345</span>`, "1 call ",
	} {
		if !strings.Contains(tree, want) {
			t.Errorf("tree lacks %q: %s", want, tree)
		}
	}
	if strings.Contains(tree, "1 calls") || strings.Contains(tree, "writing") {
		t.Errorf("tree carries a plural or a live row repeating the phase: %s", tree)
	}
	// The thinking agent's live list holds only its one placeholder.
	if ph := `<li class="brow ph"><span class="mk">-</span></li>`; !strings.Contains(tree, `<ol class="brows live bguide">`+ph+`</ol>`) {
		t.Errorf("thinking agent has a live row: %s", tree)
	}
}

// TestPageMarksASessionHandedToATerminal: the page of an ended terminal
// session carries the handed mark once its directory holds a lead record
// that was handed off, and not for an agent conversation's record.
func TestPageMarksASessionHandedToATerminal(t *testing.T) {
	f := startPageFixture(t)
	now := time.Now()
	reg := Registration{ReplyID: "handedsession", Project: f.project, Harness: "claude", RegisteredAt: stampFor(now), HeartbeatAt: stampFor(now), EndedAt: stampFor(now)}
	if err := WriteRegistration(f.dir, reg); err != nil {
		t.Fatal(err)
	}
	f.src.setPage("handedsession", PageContent{})
	f.d.Index().Refresh()
	marked := func() bool {
		t.Helper()
		code, page := get(t, f.base+"/s/handedsession?frame=1")
		if code != http.StatusOK {
			t.Fatalf("GET the session page = %d", code)
		}
		return strings.Contains(page, ` data-handed="1"`)
	}
	record := func(raw string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(SessionDir(f.dir, "handedsession"), AgentRecordFileName), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if marked() {
		t.Error("a session with no agent record carries the handed mark")
	}
	record(`{"handed_off_at":"2026-11-01T00:00:00Z"}`)
	if marked() {
		t.Error("an agent conversation's record marks the page")
	}
	record(`{"lead":true,"handed_off_at":"2026-11-01T00:00:00Z"}`)
	if !marked() {
		t.Error("a handed-off lead session's page lacks the handed mark")
	}
}

// mainPanel returns the <main id="grid"> element of a rendered page, or "".
func mainPanel(body string) string {
	i := strings.Index(body, `<main id="grid"`)
	if i < 0 {
		return ""
	}
	j := strings.Index(body[i:], `</main>`)
	if j < 0 {
		return ""
	}
	return body[i : i+j]
}
