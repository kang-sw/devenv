package wsdashboard

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// liveSnap is a fresh snapshot with an adapter at now.
func liveSnap(now time.Time, lead wslive.Lead, agents ...wslive.Agent) *wslive.Snapshot {
	return &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000, Lead: lead, Agents: agents}
}

func liveSession(id, project string, now time.Time) Session {
	reg := Registration{ReplyID: id, Project: project, Harness: "pi", RegisteredAt: stampFor(now), HeartbeatAt: stampFor(now)}
	return Session{Registration: reg, Live: true, HasBoard: true}
}

func endedSession(id, project string, now time.Time) Session {
	s := liveSession(id, project, now.Add(-time.Hour))
	s.EndedAt = stampFor(now.Add(-time.Hour))
	s.Live = false
	return s
}

// lookupOf serves sessions and snapshots from maps.
func lookupOf(sessions map[string]Session, snaps map[string]*wslive.Snapshot) func(string) (Session, *wslive.Snapshot, bool) {
	return func(id string) (Session, *wslive.Snapshot, bool) {
		s, ok := sessions[id]
		return s, snaps[id], ok
	}
}

func TestAdapterPresenceChoosesTheMode(t *testing.T) {
	now := testNow
	base := pageInputs{Session: liveSession("self", "/p", now), Now: now}
	cases := map[string]struct {
		snap *wslive.Snapshot
		want string
	}{
		"no snapshot":     {nil, modeBoard},
		"no adapter":      {&wslive.Snapshot{UpdatedAt: now.UnixMilli()}, modeBoard},
		"stale adapter":   {&wslive.Snapshot{Adapter: "pi", UpdatedAt: now.Add(-2 * time.Minute).UnixMilli(), HeartbeatMs: 30000}, modeBoard},
		"fresh adapter":   {liveSnap(now, wslive.Lead{}), modeLive},
		"default beat ok": {&wslive.Snapshot{Adapter: "claude", UpdatedAt: now.Add(-60 * time.Second).UnixMilli()}, modeLive},
	}
	for name, c := range cases {
		in := base
		in.Snapshot = c.snap
		if got := buildSessionView(in).Mode(); got != c.want {
			t.Errorf("%s: mode = %s, want %s", name, got, c.want)
		}
	}
}

func TestAgentTreeNestsAndLinksServedDelegates(t *testing.T) {
	now := testNow
	running := wslive.BatchView{Seq: 2, ToolNames: []string{"Bash"}, Running: &wslive.RunningView{ToolName: "Bash", ArgsPreview: "go test"}}
	done := wslive.BatchView{Seq: 1, ToolNames: []string{"Read"}, CallCount: 2, EndedAt: now.UnixMilli()}
	snap := liveSnap(now, wslive.Lead{},
		wslive.Agent{ID: "a1", Label: "worker", Status: wslive.StatusRunning, ReplyID: "delegateone", Activity: wslive.Activity{Batches: []wslive.BatchView{done, running}}},
		wslive.Agent{ID: "a2", ParentID: "a1", Label: "child", Status: wslive.StatusDone},
		wslive.Agent{ID: "a3", ParentID: "gone", Label: "orphan", Status: wslive.StatusFailed, ReplyID: "noboard"},
	)
	sessions := map[string]Session{"delegateone": liveSession("delegateone", "/p", now)}
	noBoard := liveSession("noboard", "/p", now)
	noBoard.HasBoard = false
	sessions["noboard"] = noBoard
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: snap, Now: now, Lookup: lookupOf(sessions, nil)})
	if len(v.Tree.Agents) != 2 || v.Tree.Agents[0].ID != "a1" || v.Tree.Agents[1].ID != "a3" {
		t.Fatalf("roots = %+v", v.Tree.Agents)
	}
	a1 := v.Tree.Agents[0]
	if !a1.Linked || a1.Running == nil || a1.Running.ToolName != "Bash" || len(a1.Finished) != 1 {
		t.Errorf("a1 = %+v", a1)
	}
	if len(a1.Children) != 1 || a1.Children[0].ID != "a2" {
		t.Errorf("a1 children = %+v", a1.Children)
	}
	if v.Tree.Agents[1].Linked {
		t.Errorf("a delegate whose session has no board must not link")
	}
	if v.Tree.Running != 1 || v.Tree.Finished != 1 || v.Tree.Failed != 1 {
		t.Errorf("counts = %d/%d/%d", v.Tree.Running, v.Tree.Finished, v.Tree.Failed)
	}
}

func TestPeerGroupMergesResolvesAndRetains(t *testing.T) {
	now := testNow
	at := func(min int) time.Time { return now.Add(-time.Duration(min) * time.Minute) }
	peerSnap := liveSnap(now, wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseTool}},
		wslive.Agent{ID: "p1", Status: wslive.StatusRunning})
	sessions := map[string]Session{
		"servedpeer": liveSession("servedpeer", "/p", now),
		"endedpeer":  endedSession("endedpeer", "/q", now),
	}
	snaps := map[string]*wslive.Snapshot{"servedpeer": peerSnap}
	contacts := []Contact{
		// One session reached by its id: address and by a named address:
		// one row, labelled by the name, with the latest exchange.
		{Address: "id:servedpeer", ReplyID: "servedpeer", LastExchange: at(30)},
		{Address: "lead@machine", ReplyID: "servedpeer", LastExchange: at(2)},
		// The session itself never lists.
		{Address: "id:self", ReplyID: "self", LastExchange: at(1)},
		// A served peer whose registration ended reads ended whatever its
		// presence says.
		{Address: "id:endedpeer", ReplyID: "endedpeer", LastExchange: at(3), PresenceLive: true},
		// Unserved peers follow their presence.
		{Address: "other@machine", ReplyID: "unservedlive", LastExchange: at(4), PresenceLive: true},
		{Address: "id:unservedgone", ReplyID: "unservedgone", LastExchange: at(5)},
		{Address: "noowner@machine", LastExchange: at(6)},
	}
	for i := 0; i < 6; i++ {
		contacts = append(contacts, Contact{Address: fmt.Sprintf("id:old%d", i), ReplyID: fmt.Sprintf("old%d", i), LastExchange: at(100 + i)})
	}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}), Now: now,
		Page: PageContent{Contacts: contacts}, Lookup: lookupOf(sessions, snaps)})
	g := v.Tree.Peers
	if len(g.Live) != 2 {
		t.Fatalf("live peers = %+v", g.Live)
	}
	first := g.Live[0]
	if first.Address != "lead@machine" || first.ReplyID != "servedpeer" || !first.Served || !first.Adapter ||
		first.LastExchange != at(2).UnixMilli() || first.AgentCount != 1 || len(first.Agents) != 1 || first.Lead.Phase != wslive.PhaseTool {
		t.Errorf("merged served peer = %+v", first)
	}
	if g.Live[1].ReplyID != "unservedlive" || g.Live[1].Served {
		t.Errorf("second live peer = %+v", g.Live[1])
	}
	// Ended: endedpeer (3), unservedgone (5), noowner (6), old0, old1 - then
	// four more hidden.
	if len(g.Ended) != endedPeerCap || g.HiddenEnded != 4 {
		t.Fatalf("ended = %d (hidden %d): %+v", len(g.Ended), g.HiddenEnded, g.Ended)
	}
	if g.Ended[0].ReplyID != "endedpeer" || g.Ended[1].ReplyID != "unservedgone" || g.Ended[2].Address != "noowner@machine" || g.Ended[3].ReplyID != "old0" {
		t.Errorf("ended order = %+v", g.Ended)
	}
	for _, row := range g.Rows() {
		if row.ReplyID == "self" {
			t.Errorf("the session lists itself as a peer")
		}
	}
}

func TestPeerTreeIsOneLevelDeep(t *testing.T) {
	now := testNow
	// The peer's snapshot carries agents; its own contacts are never read,
	// so its peer group cannot appear inside this page.
	peerSnap := liveSnap(now, wslive.Lead{}, wslive.Agent{ID: "x", Status: wslive.StatusRunning})
	sessions := map[string]Session{"peer": liveSession("peer", "/p", now)}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}), Now: now,
		Page:   PageContent{Contacts: []Contact{{Address: "id:peer", ReplyID: "peer", LastExchange: now}}},
		Lookup: lookupOf(sessions, map[string]*wslive.Snapshot{"peer": peerSnap})})
	row := v.Tree.Peers.Live[0]
	if len(row.Agents) != 1 || len(row.Agents[0].Children) != 0 {
		t.Fatalf("peer tree = %+v", row.Agents)
	}
	// A served peer without a live adapter shows no phase.
	stale := *peerSnap
	stale.UpdatedAt = now.Add(-time.Hour).UnixMilli()
	v = buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}), Now: now,
		Page:   PageContent{Contacts: []Contact{{Address: "id:peer", ReplyID: "peer", LastExchange: now}}},
		Lookup: lookupOf(sessions, map[string]*wslive.Snapshot{"peer": &stale})})
	if row := v.Tree.Peers.Live[0]; row.Adapter || len(row.Agents) != 0 {
		t.Fatalf("a peer without an adapter carries live state: %+v", row)
	}
}

// renderTree renders the Agents region of a view through the page template.
func renderTree(t *testing.T, v SessionView) string {
	t.Helper()
	a, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := a.tmpl.ExecuteTemplate(&buf, "r-tree", v); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestPeerWorktreeCompactAndOrder(t *testing.T) {
	now := testNow
	at := func(min int) time.Time { return now.Add(-time.Duration(min) * time.Minute) }
	snap := func() *wslive.Snapshot {
		return liveSnap(now, wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseTool, Batches: []wslive.BatchView{{Seq: 1, EndedAt: 1}, {Seq: 2, Running: &wslive.RunningView{ToolName: "Bash"}}}}},
			wslive.Agent{ID: "a", Status: wslive.StatusRunning})
	}
	sessions := map[string]Session{
		"samewt":   liveSession("samewt", "/p/", now), // Clean makes it equal to /p
		"otherwt":  liveSession("otherwt", "/q", now),
		"otherwt2": liveSession("otherwt2", "/q", now),
		"gone":     endedSession("gone", "/q", now),
	}
	snaps := map[string]*wslive.Snapshot{"samewt": snap(), "otherwt": snap(), "otherwt2": snap(), "gone": snap()}
	contacts := []Contact{
		{Address: "id:otherwt", ReplyID: "otherwt", LastExchange: at(1)},
		{Address: "id:samewt", ReplyID: "samewt", LastExchange: at(5)},
		{Address: "id:otherwt2", ReplyID: "otherwt2", LastExchange: at(2)},
		{Address: "id:unserved", ReplyID: "unserved", LastExchange: at(3), PresenceLive: true},
		{Address: "id:gone", ReplyID: "gone", LastExchange: at(4)},
	}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}), Now: now,
		Page: PageContent{Contacts: contacts}, Lookup: lookupOf(sessions, snaps)})
	g := v.Tree.Peers
	var order []string
	for _, r := range g.Live {
		order = append(order, r.ReplyID)
	}
	if want := []string{"samewt", "otherwt", "otherwt2", "unserved"}; fmt.Sprint(order) != fmt.Sprint(want) {
		t.Fatalf("live order = %v, want %v", order, want)
	}
	same, other := g.Live[0], g.Live[1]
	if !same.SameWorktree || same.Compact || !same.Adapter || len(same.Agents) != 1 || same.AgentCount != 1 || same.Lead.Phase != wslive.PhaseTool || len(same.Lead.Finished) != 1 || same.Lead.Running == nil {
		t.Errorf("same-worktree peer = %+v", same)
	}
	if other.SameWorktree || !other.Compact || !other.Adapter || other.Lead.Phase != wslive.PhaseTool ||
		len(other.Agents) != 0 || other.AgentCount != 0 || len(other.Lead.Finished) != 0 || other.Lead.Running != nil {
		t.Errorf("other-worktree peer = %+v", other)
	}
	if len(g.Ended) != 1 || g.Ended[0].Compact || len(g.Ended[0].Agents) != 1 {
		t.Errorf("ended peer = %+v", g.Ended)
	}
	html := renderTree(t, v)
	if got := strings.Count(html, `<details class="peer live"`); got != 1 {
		t.Errorf("expanding live peers = %d: %s", got, html)
	}
	if got := strings.Count(html, `class="peer live compact"`); got != 2 {
		t.Errorf("compact peers = %d: %s", got, html)
	}
	// The header chip counts every live peer: 2 compact + 1 same + 1 unserved.
	if !strings.Contains(html, `<span class="chip">`+"\u003c"[:0]) && !strings.Contains(html, `4</span></span></div>`) {
		t.Errorf("mail chip does not count every live peer: %s", html)
	}
	// The compact row has the phase mark and last mail but no tool, count, or tree.
	i := strings.Index(html, `class="peer live compact"`)
	row := html[i : i+strings.Index(html[i:], "</div>")]
	for _, want := range []string{`href="/s/otherwt"`, `class="spin`, `last mail exchange`} {
		if !strings.Contains(row, want) {
			t.Errorf("compact row lacks %q: %s", want, row)
		}
	}
	for _, not := range []string{"ptool", `title="subagents"`, "ptree", "pline"} {
		if strings.Contains(row, not) {
			t.Errorf("compact row carries %q: %s", not, row)
		}
	}
}

func TestConversationMailLinesAndLiveState(t *testing.T) {
	now := testNow
	ring := []wslive.RingRecord{
		{Name: "n1", At: now.Add(-5 * time.Minute), RingEntry: wslive.RingEntry{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard, Text: "hello"}},
		{Name: "n2", At: now.Add(-4 * time.Minute), RingEntry: wslive.RingEntry{Kind: wslive.RingMail, Direction: wslive.MailRecv, Peer: "lead@machine", Title: "first line", HistoryKey: "h1"}},
		{Name: "n3", At: now.Add(-3 * time.Minute), RingEntry: wslive.RingEntry{Kind: wslive.RingMail, Direction: wslive.MailSent, Peer: "id:unserved", Title: "old mail", HistoryKey: "h-gone"}},
		{Name: "n4", At: now.Add(-2 * time.Minute), RingEntry: wslive.RingEntry{Kind: wslive.RingTurn, Origin: wslive.OriginMail, Done: 3, Failed: 1, DurationMs: 61000}},
	}
	lead := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhasePermission, Permission: &wslive.Permission{ToolName: "Bash", ArgsPreview: "rm -rf build"}},
		Reply: &wslive.Reply{Text: "partial reply"}}
	sessions := map[string]Session{"peer": liveSession("peer", "/q", now)}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, lead), Ring: ring, Now: now,
		Page:   PageContent{Contacts: []Contact{{Address: "lead@machine", ReplyID: "peer", LastExchange: now}}, MailBodies: map[string]string{"h1": "first line\nsecond line"}},
		Lookup: lookupOf(sessions, nil)})
	c := v.Conversation
	if c.Users != 1 || c.Mails != 2 || len(c.Entries) != 4 {
		t.Fatalf("conversation = %+v", c)
	}
	if e := c.Entries[1]; !e.HasBody || e.Body != "first line\nsecond line" || !e.PeerLinked || e.PeerReplyID != "peer" {
		t.Errorf("mail with history = %+v", e)
	}
	if e := c.Entries[2]; e.HasBody || e.PeerLinked {
		t.Errorf("mail that left the history = %+v", e)
	}
	if c.Reply == nil || c.Reply.Text != "partial reply" || c.Permission == nil || c.Permission.ToolName != "Bash" {
		t.Errorf("live state = %+v", c)
	}
}

func TestBoardFirstPageCarriesNoLiveState(t *testing.T) {
	now := testNow
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Now: now,
		Ring: []wslive.RingRecord{{Name: "n", At: now, RingEntry: wslive.RingEntry{Kind: wslive.RingUser, Text: "x"}}},
		Page: PageContent{HasBoard: true, Contacts: []Contact{{Address: "id:peer", ReplyID: "peer", LastExchange: now}}}})
	if v.Adapter || len(v.Conversation.Entries) != 0 || len(v.Tree.Peers.Rows()) != 0 {
		t.Fatalf("board-first view carries live state: %+v", v)
	}
}

func TestIndexViewListsLiveAndRecentlyEndedSessions(t *testing.T) {
	now := testNow
	recent := endedSession("recentend", "/a", now)
	old := liveSession("oldend", "/a", now.Add(-48*time.Hour))
	old.EndedAt = stampFor(now.Add(-48 * time.Hour))
	onlyOld := liveSession("onlyold", "/b", now.Add(-72*time.Hour))
	lapsed := liveSession("lapsed", "/a", now.Add(-10*time.Minute)) // heartbeat stopped: ended
	view := buildIndexView([]indexInput{
		{Session: liveSession("busy", "/a", now), Snapshot: liveSnap(now, wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseTool, Batches: []wslive.BatchView{{Running: &wslive.RunningView{ToolName: "Edit"}}}}})},
		{Session: liveSession("codex", "/a", now)},
		{Session: recent},
		{Session: old},
		{Session: lapsed},
		{Session: onlyOld},
	}, now, "1.2.3")
	if len(view.Projects) != 1 || view.Projects[0].Path != "/a" {
		t.Fatalf("projects = %+v", view.Projects)
	}
	p := view.Projects[0]
	if len(p.Sessions) != 2 || len(p.Ended) != 2 || p.HiddenEnded != 1 {
		t.Fatalf("project rows: live %d ended %d hidden %d", len(p.Sessions), len(p.Ended), p.HiddenEnded)
	}
	var busy IndexRow
	for _, r := range p.Sessions {
		if r.ReplyID == "busy" {
			busy = r
		}
		if r.ReplyID == "codex" && (r.Adapter || r.Phase != "") {
			t.Errorf("a session without an adapter shows a phase: %+v", r)
		}
	}
	if busy.Phase != wslive.PhaseTool || busy.Tool != "Edit" {
		t.Errorf("busy row = %+v", busy)
	}
	if view.Live != 2 || view.Busy != 1 {
		t.Errorf("totals live %d busy %d", view.Live, view.Busy)
	}
	for _, r := range p.Ended {
		if !strings.Contains("recentend lapsed", r.ReplyID) {
			t.Errorf("unexpected ended row %s", r.ReplyID)
		}
	}
}
