package wsdashboard

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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
func lookupOf(sessions map[string]Session, snaps map[string]*wslive.Snapshot) sessionLookup {
	return sessionLookup{
		session: func(id string) (Session, bool) {
			s, ok := sessions[id]
			return s, ok
		},
		snapshot: func(id string) *wslive.Snapshot { return snaps[id] },
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

// TestLookupReadsNoSnapshotForPresenceOnly: a view that uses a looked-up
// session only for whether it is served (a mail line's peer link) or has a
// board (a linked agent row) reads no snapshot of it.
func TestLookupReadsNoSnapshotForPresenceOnly(t *testing.T) {
	now := testNow
	snap := liveSnap(now, wslive.Lead{}, wslive.Agent{ID: "a1", Label: "worker", Status: wslive.StatusDone, ReplyID: "linked"})
	sessions := map[string]Session{
		"linked":   liveSession("linked", "/p", now),
		"mailpeer": liveSession("mailpeer", "/p", now),
	}
	var read []string
	lookup := lookupOf(sessions, nil)
	lookup.snapshot = func(id string) *wslive.Snapshot {
		read = append(read, id)
		return nil
	}
	ring := []wslive.RingRecord{{Name: "m1", At: now.Add(-time.Minute), RingEntry: wslive.RingEntry{Kind: wslive.RingMail, Direction: wslive.MailRecv, Peer: "id:mailpeer", Title: "hi"}}}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: snap, Ring: ring, Now: now, Lookup: lookup})
	if len(v.Tree.Agents) != 1 || !v.Tree.Agents[0].Linked {
		t.Fatalf("agents = %+v, want a1 linked", v.Tree.Agents)
	}
	if len(v.Conversation.Entries) != 1 || !v.Conversation.Entries[0].PeerLinked {
		t.Fatalf("entries = %+v, want the mail line linked", v.Conversation.Entries)
	}
	if len(read) != 0 {
		t.Fatalf("a presence-only lookup read snapshots %v", read)
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

// TestSubagentCounts pins the composer badge's N(+M): Direct counts the
// running agents with no known parent (an evicted parent's running child
// included), Deep the running agents below them plus the running agents of
// a running delegate that holds its own served session, recursively, each
// session counted once.
func TestSubagentCounts(t *testing.T) {
	now := testNow
	run := func(id, parent, reply string) wslive.Agent {
		return wslive.Agent{ID: id, ParentID: parent, ReplyID: reply, Status: wslive.StatusRunning}
	}
	done := func(id, parent string) wslive.Agent {
		return wslive.Agent{ID: id, ParentID: parent, Status: wslive.StatusDone}
	}
	stale := *liveSnap(now, wslive.Lead{}, run("s1", "", ""))
	stale.UpdatedAt = now.Add(-10 * time.Minute).UnixMilli()
	sessions := map[string]Session{}
	for _, id := range []string{"self", "dlg", "dlg2", "loop", "stale", "nosnap"} {
		sessions[id] = liveSession(id, "/p", now)
	}
	snaps := map[string]*wslive.Snapshot{
		// dlg runs two agents, one nested, and a delegate of its own (dlg2).
		"dlg":  liveSnap(now, wslive.Lead{}, run("d1", "", "dlg2"), run("d2", "d1", ""), done("d3", "")),
		"dlg2": liveSnap(now, wslive.Lead{}, run("e1", "", "")),
		// loop's agent points back at the viewed session and at loop itself.
		"loop":  liveSnap(now, wslive.Lead{}, run("l1", "", "self"), run("l2", "", "loop")),
		"stale": &stale,
	}
	lookup := lookupOf(sessions, snaps)
	cases := map[string]struct {
		agents []wslive.Agent
		want   SubagentCount
	}{
		"none":                {nil, SubagentCount{}},
		"direct children":     {[]wslive.Agent{run("a", "", ""), run("b", "", "")}, SubagentCount{Direct: 2}},
		"grandchildren":       {[]wslive.Agent{run("a", "", ""), run("b", "a", ""), run("c", "b", "")}, SubagentCount{Direct: 1, Deep: 2}},
		"under finished":      {[]wslive.Agent{done("a", ""), run("b", "a", "")}, SubagentCount{Deep: 1}},
		"evicted parent":      {[]wslive.Agent{run("b", "gone", ""), run("c", "b", "")}, SubagentCount{Direct: 1, Deep: 1}},
		"finished ignored":    {[]wslive.Agent{done("a", ""), done("b", "a"), {ID: "c", Status: wslive.StatusFailed}}, SubagentCount{}},
		"linked delegate":     {[]wslive.Agent{run("a", "", "dlg")}, SubagentCount{Direct: 1, Deep: 3}},
		"finished delegate":   {[]wslive.Agent{{ID: "a", ReplyID: "dlg", Status: wslive.StatusDone}}, SubagentCount{}},
		"delegate twice":      {[]wslive.Agent{run("a", "", "dlg"), run("b", "", "dlg")}, SubagentCount{Direct: 2, Deep: 3}},
		"lookup cycle":        {[]wslive.Agent{run("a", "", "loop")}, SubagentCount{Direct: 1, Deep: 2}},
		"stale delegate":      {[]wslive.Agent{run("a", "", "stale")}, SubagentCount{Direct: 1}},
		"unserved delegate":   {[]wslive.Agent{run("a", "", "unknown"), run("b", "", "nosnap")}, SubagentCount{Direct: 2}},
		"self parent ignored": {[]wslive.Agent{run("a", "a", "")}, SubagentCount{Direct: 1}},
	}
	for name, c := range cases {
		if got := subagentCounts("self", c.agents, lookup, now); got != c.want {
			t.Errorf("%s: counts = %+v, want %+v", name, got, c.want)
		}
	}
}

// TestConvLiveCarriesSubagentCounts: the convlive region carries the counts
// for the composer's subagent badge beside the lead's phase.
func TestConvLiveCarriesSubagentCounts(t *testing.T) {
	snap := liveSnap(testNow, wslive.Lead{},
		wslive.Agent{ID: "a", Status: wslive.StatusRunning},
		wslive.Agent{ID: "b", ParentID: "a", Status: wslive.StatusRunning},
		wslive.Agent{ID: "c", ParentID: "a", Status: wslive.StatusRunning},
		wslive.Agent{ID: "d", Status: wslive.StatusDone})
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", testNow), Snapshot: snap, Now: testNow})
	if v.Conversation.Subagents != (SubagentCount{Direct: 1, Deep: 2}) {
		t.Fatalf("Subagents = %+v", v.Conversation.Subagents)
	}
	set, err := renderSessionRegions(v, newEntryCache())
	if err != nil {
		t.Fatal(err)
	}
	if html := string(set[regionConvLive].HTML); !strings.Contains(html, `<div class="conv clive" data-phase="" data-sub="1" data-subdeep="2">`) {
		t.Errorf("convlive lacks the subagent counts:\n%s", html)
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

// TestPeerRowFolder pins the row label's folder: Path's last element, split
// on either separator, trailing separators ignored.
func TestPeerRowFolder(t *testing.T) {
	for path, want := range map[string]string{
		"/home/u/devenv":      "devenv",
		"/home/u/devenv/":     "devenv",
		`C:\Users\u\proj`:     "proj",
		`C:\Users\u\proj\`:    "proj",
		`D:/mixed\sep/leaf//`: "leaf",
		"plain":               "plain",
		"":                    "",
		"/":                   "",
	} {
		if got := (PeerRow{Path: path}).Folder(); got != want {
			t.Errorf("Folder(%q) = %q, want %q", path, got, want)
		}
	}
}

// TestPeerFolderLabel pins the linked-worktree prefix: "<main>(<folder>)"
// from both paths' last elements under either separator, the plain folder
// when no main root is known or the path is empty.
func TestPeerFolderLabel(t *testing.T) {
	for _, c := range []struct{ path, main, want string }{
		{"/home/u/.wt/devenv/gamma", "/home/u/devenv", "devenv(gamma)"},
		{"/home/u/.wt/devenv/gamma/", "/home/u/devenv/", "devenv(gamma)"},
		{`C:\wt\gamma\`, `C:\src\devenv`, "devenv(gamma)"},
		{"/home/u/devenv", "", "devenv"},
		{"", "/home/u/devenv", ""},
		{"/home/u/x", "/", "x"},
	} {
		if got := (PeerRow{Path: c.path, WorktreeMain: c.main}).Folder(); got != c.want {
			t.Errorf("Folder(%q, %q) = %q, want %q", c.path, c.main, got, c.want)
		}
	}
}

// TestWorktreeMain pins linked-worktree detection from fake .git entries: a
// .git file pointing into <root>/.git/worktrees/<name> yields root (via
// commondir when present, relative gitdir included); a plain repo, a
// submodule, a bare-repo worktree, a missing path, and a Windows path seen
// here yield "". Trailing separators resolve like the clean path.
func TestWorktreeMain(t *testing.T) {
	base := t.TempDir()
	mk := func(parts ...string) string {
		p := filepath.Join(append([]string{base}, parts...)...)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write := func(path, body string) {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	main := mk("devenv")
	mk("devenv", ".git", "modules", "sub")
	wtGit := mk("devenv", ".git", "worktrees", "gamma")
	write(filepath.Join(wtGit, "commondir"), "../..\n")
	linked := mk("wt", "gamma")
	write(filepath.Join(linked, ".git"), "gitdir: "+wtGit+"\n")

	// No commondir, relative gitdir.
	mk("devenv", ".git", "worktrees", "delta")
	rel := mk("wt", "delta")
	write(filepath.Join(rel, ".git"), "gitdir: ../../devenv/.git/worktrees/delta\n")

	sub := mk("devenv", "sub")
	write(filepath.Join(sub, ".git"), "gitdir: ../.git/modules/sub\n")

	bareWt := mk("repo.git", "worktrees", "eps")
	write(filepath.Join(bareWt, "commondir"), "../..\n")
	bare := mk("wt", "eps")
	write(filepath.Join(bare, ".git"), "gitdir: "+bareWt+"\n")

	for path, want := range map[string]string{
		linked:                              main,
		linked + string(filepath.Separator): main,
		rel:                                 main,
		main:                                "",
		sub:                                 "",
		bare:                                "",
		filepath.Join(base, "missing"):      "",
		`C:\Users\u\proj`:                   "",
		"":                                  "",
	} {
		if got := readWorktreeMain(path); path != "" && got != want {
			t.Errorf("readWorktreeMain(%q) = %q, want %q", path, got, want)
		}
		if got := worktreeMain(path); got != want {
			t.Errorf("worktreeMain(%q) = %q, want %q", path, got, want)
		}
	}

	// Peer rows carry the prefix for a linked-worktree cwd.
	now := testNow
	contacts := []Contact{{Address: "id:wt", ReplyID: "wt", LastExchange: now, PresenceLive: true, Cwd: linked}}
	g := peerGroup(contacts, "/p", lookupOf(map[string]Session{}, nil), now)
	if len(g.Live) != 1 || g.Live[0].Folder() != "devenv(gamma)" {
		t.Errorf("peer row folder = %+v, want devenv(gamma)", g.Live)
	}
}

// TestPeerRowPath pins where a peer row's Path comes from: a served peer's
// registration Project over any contact cwd; an unserved id: or named peer's
// Contact.Cwd (the newest non-empty over merged addresses); none when no cwd
// is known. The row renders the folder prefix muted inside .pname, the full
// path as its title, and the bare address when no folder is known.
func TestPeerRowPath(t *testing.T) {
	now := testNow
	at := func(min int) time.Time { return now.Add(-time.Duration(min) * time.Minute) }
	sessions := map[string]Session{"served": liveSession("served", "/srv/work/served-wt", now)}
	contacts := []Contact{
		{Address: "id:served", ReplyID: "served", LastExchange: at(1), Cwd: "/elsewhere/ignored"},
		{Address: "id:unserved", ReplyID: "unserved", LastExchange: at(2), PresenceLive: true, Cwd: "/home/u/idpeer"},
		{Address: "named@machine", ReplyID: "namedpeer", LastExchange: at(3), PresenceLive: true},
		{Address: "id:namedpeer", ReplyID: "namedpeer", LastExchange: at(4), Cwd: "/home/u/namedwt/"},
		{Address: "win@machine", LastExchange: at(5), PresenceLive: true, Cwd: `C:\Users\u\winproj`},
		{Address: "id:nocwd", ReplyID: "nocwd", LastExchange: at(6), PresenceLive: true},
	}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}), Now: now,
		Page: PageContent{Contacts: contacts}, Lookup: lookupOf(sessions, nil)})
	got := map[string][2]string{}
	for _, r := range v.Tree.Peers.Rows() {
		got[r.Address] = [2]string{r.Path, r.Folder()}
	}
	want := map[string][2]string{
		"id:served":     {"/srv/work/served-wt", "served-wt"},
		"id:unserved":   {"/home/u/idpeer", "idpeer"},
		"named@machine": {"/home/u/namedwt/", "namedwt"},
		"win@machine":   {`C:\Users\u\winproj`, "winproj"},
		"id:nocwd":      {"", ""},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("rows (path, folder) = %v, want %v", got, want)
	}
	html := renderTree(t, v)
	for _, w := range []string{
		`<a class="pname" href="/s/served" title="/srv/work/served-wt"><span class="pfolder">served-wt/</span>id:served</a>`,
		`<span class="pname" title="/home/u/idpeer"><span class="pfolder">idpeer/</span>id:unserved</span>`,
		`<span class="pname">id:nocwd</span>`,
	} {
		if !strings.Contains(html, w) {
			t.Errorf("tree lacks %q: %s", w, html)
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
	// The compact row has the phase mark (a pulsing dot) and last mail but no
	// tool, count, or tree.
	i := strings.Index(html, `class="peer live compact"`)
	row := html[i : i+strings.Index(html[i:], "</div>")]
	for _, want := range []string{`href="/s/otherwt"`, `class="pulse"`, `last mail exchange`} {
		if !strings.Contains(row, want) {
			t.Errorf("compact row lacks %q: %s", want, row)
		}
	}
	// Its one running subagent shows as the running label, not the total; an
	// agent mark, so it carries no spinner.
	if !strings.Contains(row, `class="prun"`) || !strings.Contains(row, `title="1 subagent"`) || strings.Contains(row, `spin`) {
		t.Errorf("compact row lacks the running-subagent label: %s", row)
	}
	for _, not := range []string{"ptool", "ptree", "pline"} {
		if strings.Contains(row, not) {
			t.Errorf("compact row carries %q: %s", not, row)
		}
	}
}

func TestPeerRunningSubagentSpinner(t *testing.T) {
	now := testNow
	agents := []wslive.Agent{
		{ID: "a", Status: wslive.StatusRunning},
		{ID: "b", Status: wslive.StatusRunning, ParentID: "a"}, // nested counts too
		{ID: "c", Status: wslive.StatusDone},
		{ID: "d", Status: wslive.StatusFailed},
	}
	build := func(project string, as ...wslive.Agent) PeerRow {
		sessions := map[string]Session{"peer": liveSession("peer", project, now)}
		v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}), Now: now,
			Page:   PageContent{Contacts: []Contact{{Address: "id:peer", ReplyID: "peer", LastExchange: now}}},
			Lookup: lookupOf(sessions, map[string]*wslive.Snapshot{"peer": liveSnap(now, wslive.Lead{}, as...)})})
		return v.Tree.Peers.Live[0]
	}
	render := func(row PeerRow) string {
		v := SessionView{}
		v.Tree.Peers.Live = []PeerRow{row}
		return renderTree(t, v)
	}
	for name, project := range map[string]string{"compact": "/q", "same-worktree": "/p"} {
		row := build(project, agents...)
		if row.Compact != (name == "compact") {
			t.Fatalf("%s: Compact = %v", name, row.Compact)
		}
		if row.RunningAgents != 2 {
			t.Errorf("%s: RunningAgents = %d, want 2 (finished agents excluded)", name, row.RunningAgents)
		}
		if html := render(row); !strings.Contains(html, `class="prun"`) || !strings.Contains(html, `title="2 subagents"`) || !strings.Contains(html, "</svg>2</span>") || strings.Contains(html, `class="spin"`) {
			t.Errorf("%s: no running label: %s", name, html)
		}
		// Zero running: finished agents leave no spinner label.
		row = build(project, agents[2:]...)
		if row.RunningAgents != 0 {
			t.Errorf("%s: RunningAgents = %d, want 0", name, row.RunningAgents)
		}
		if html := render(row); strings.Contains(html, "prun") {
			t.Errorf("%s: spinner label with nothing running: %s", name, html)
		}
	}
	if html := render(build("/p", agents[0])); !strings.Contains(html, `title="1 subagent"`) || strings.Contains(html, "1 subagents") {
		t.Errorf("singular form: %s", html)
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

// renderNamed renders one named template of the page set.
func renderNamed(t *testing.T, name string, data any) string {
	t.Helper()
	a, err := loadDashboardAssets()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := a.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// liveList is the one ol.brows.live in html, with its rows split out.
func liveList(t *testing.T, html string) (list string, rows []string) {
	t.Helper()
	const open = `<ol class="brows live bguide">`
	if n := strings.Count(html, open); n != 1 {
		t.Fatalf("live lists = %d, want 1: %s", n, html)
	}
	i := strings.Index(html, open) + len(open)
	list = html[i : i+strings.Index(html[i:], "</ol>")]
	for _, r := range strings.SplitAfter(list, "</li>") {
		if r != "" {
			rows = append(rows, r)
		}
	}
	return list, rows
}

// TestRunningAgentHasOneLiveSlot pins the fixed live row: a running agent
// always shows exactly one, the latest entry, a pending permission prompt in
// place of it, a placeholder when there is nothing.
func TestRunningAgentHasOneLiveSlot(t *testing.T) {
	now := testNow
	ms := now.UnixMilli()
	fin := func(tool string) wslive.BatchView {
		return wslive.BatchView{ToolNames: []string{tool}, CallCount: 1, EndedAt: ms}
	}
	running := wslive.BatchView{Running: &wslive.RunningView{ToolName: "RunTool", ArgsPreview: "run args", StartedAt: ms}}
	generating := wslive.BatchView{Generating: &wslive.GeneratingView{Kind: wslive.PhaseTool, ToolName: "GenTool", Chars: 12, PhaseStartedAt: ms}}
	thinking := wslive.BatchView{Generating: &wslive.GeneratingView{Kind: wslive.PhaseThinking, PhaseStartedAt: ms}}
	perm := &wslive.Permission{ToolName: "PermTool", StartedAt: ms}
	const ph = "placeholder"
	cases := []struct {
		name    string
		batches []wslive.BatchView
		perm    *wslive.Permission
		want    [1]string // the tool name the row shows, or ph
	}{
		{"no batches", nil, nil, [1]string{ph}},
		{"thinking only", []wslive.BatchView{thinking}, nil, [1]string{ph}},
		{"one finished", []wslive.BatchView{fin("F1")}, nil, [1]string{"F1"}},
		{"three finished", []wslive.BatchView{fin("F1"), fin("F2"), fin("F3")}, nil, [1]string{"F3"}},
		{"finished then thinking", []wslive.BatchView{fin("F1"), fin("F2"), thinking}, nil, [1]string{"F2"}},
		{"running and finished", []wslive.BatchView{fin("F1"), fin("F2"), running}, nil, [1]string{"RunTool"}},
		{"generating and running", []wslive.BatchView{fin("F1"), running, generating}, nil, [1]string{"GenTool"}},
		{"permission, running, generating", []wslive.BatchView{fin("F1"), running, generating}, perm, [1]string{"PermTool"}},
		{"permission, finished", []wslive.BatchView{fin("F1"), fin("F2")}, perm, [1]string{"PermTool"}},
		{"permission alone", nil, perm, [1]string{"PermTool"}},
	}
	for _, c := range cases {
		agent := wslive.Agent{ID: "a1", Status: wslive.StatusRunning, StartedAt: ms,
			Activity: wslive.Activity{Phase: wslive.PhaseTool, Batches: c.batches, Permission: c.perm}}
		v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}, agent), Now: now})
		if got := len(v.Tree.Agents[0].Slots); got != liveSlotCount {
			t.Errorf("%s: Slots = %d, want %d", c.name, got, liveSlotCount)
		}
		_, rows := liveList(t, renderTree(t, v))
		if len(rows) != 1 {
			t.Errorf("%s: live rows = %d, want 1: %v", c.name, len(rows), rows)
			continue
		}
		for i, want := range c.want {
			row := rows[i]
			switch {
			case want == ph:
				if row != `<li class="brow ph"><span class="mk">-</span></li>` {
					t.Errorf("%s: row %d is no placeholder: %s", c.name, i, row)
				}
			case !strings.Contains(row, `title="`+want+`"`):
				t.Errorf("%s: row %d lacks %s: %s", c.name, i, want, row)
			case want == "PermTool" && !strings.HasPrefix(row, `<li class="brow live perm">`),
				want == "RunTool" && !strings.Contains(row, `run args`),
				want == "GenTool" && !strings.Contains(row, `writing arguments`),
				strings.HasPrefix(want, "F") && !strings.HasPrefix(row, `<li class="brow done"><span class="mk">✓</span>`):
				t.Errorf("%s: row %d has the wrong look for %s: %s", c.name, i, want, row)
			}
			if (want == "RunTool" || want == "GenTool") && !strings.Contains(row, `<span class="mk"><span class="spin"></span></span>`) {
				t.Errorf("%s: live tool row %d lacks its spinner: %s", c.name, i, row)
			}
		}
	}
	// A finished or failed agent has no live list.
	for _, status := range []string{wslive.StatusDone, wslive.StatusFailed} {
		agent := wslive.Agent{ID: "a1", Status: status, Activity: wslive.Activity{Batches: []wslive.BatchView{fin("F1")}}}
		v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}, agent), Now: now})
		if v.Tree.Agents[0].Slots != nil {
			t.Errorf("%s agent has slots: %+v", status, v.Tree.Agents[0].Slots)
		}
		if html := renderTree(t, v); strings.Contains(html, "brows live") {
			t.Errorf("%s agent renders a live list: %s", status, html)
		}
	}
}

// TestAgentsPanelSpinsOnlyOnToolRows pins Decision "agent marks are a state
// dot": in the agents panel only live tool rows carry the rotating spinner;
// agent, lead, and peer marks pulse, phase pills and the running-subagent
// label have none. The session index row and the subagent badge keep theirs.
func TestAgentsPanelSpinsOnlyOnToolRows(t *testing.T) {
	now := testNow
	ms := now.UnixMilli()
	run := func(tool string) wslive.BatchView {
		return wslive.BatchView{Running: &wslive.RunningView{ToolName: tool, StartedAt: ms}}
	}
	busy := func(phase string) wslive.Lead {
		return wslive.Lead{Activity: wslive.Activity{Phase: phase, PhaseStartedAt: ms, Batches: []wslive.BatchView{{ToolNames: []string{"Read"}, CallCount: 1, EndedAt: ms}, run("Bash")}}}
	}
	agents := []wslive.Agent{
		{ID: "a", Status: wslive.StatusRunning, Activity: wslive.Activity{Phase: wslive.PhaseTool, Batches: []wslive.BatchView{run("Edit")}}},
		{ID: "b", ParentID: "a", Status: wslive.StatusRunning, Activity: wslive.Activity{Phase: wslive.PhaseThinking}},
	}
	sessions := map[string]Session{"samewt": liveSession("samewt", "/p", now), "otherwt": liveSession("otherwt", "/q", now)}
	snaps := map[string]*wslive.Snapshot{
		"samewt":  liveSnap(now, busy(wslive.PhaseReplying), agents...),
		"otherwt": liveSnap(now, busy(wslive.PhaseCompacting), agents...),
	}
	contacts := []Contact{{Address: "id:samewt", ReplyID: "samewt", LastExchange: now}, {Address: "id:otherwt", ReplyID: "otherwt", LastExchange: now}}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, busy(wslive.PhaseTool), agents...), Now: now,
		Page: PageContent{Contacts: contacts}, Lookup: lookupOf(sessions, snaps)})
	html := renderTree(t, v)
	for _, want := range []string{`class="prun"`, `<span class="phase tool">`, `<span class="phase thinking">`, `<span class="phase replying">`, `<ol class="brows live bguide">`} {
		if !strings.Contains(html, want) {
			t.Fatalf("tree lacks %q: %s", want, html)
		}
	}
	// Lead, two peers (one compact), and every running agent row pulse.
	if n := strings.Count(html, `<span class="pulse"></span>`); n < 6 {
		t.Errorf("pulsing marks = %d, want at least 6: %s", n, html)
	}
	// Remove every live tool row; no spinner may remain.
	rest := html
	spinRows := 0
	for {
		i := strings.Index(rest, `<li class="brow live`)
		if i < 0 {
			break
		}
		j := strings.Index(rest[i:], "</li>")
		if strings.Contains(rest[i:i+j], `class="spin"`) {
			spinRows++
		}
		rest = rest[:i] + rest[i+j+len("</li>"):]
	}
	if spinRows == 0 {
		t.Errorf("no live tool row spins: %s", html)
	}
	if strings.Contains(rest, "spin") {
		t.Errorf("a spinner outside the live tool rows: %s", rest)
	}
	// Outside the agents panel the spinner stays.
	if srow := renderNamed(t, "srow", IndexRow{ReplyID: "s", Live: true, Adapter: true, Phase: wslive.PhaseTool}); !strings.Contains(srow, `<span class="phase tool"><span class="spin"></span>`) {
		t.Errorf("session index row lost its spinner: %s", srow)
	}
	if badge := renderNamed(t, "subbadge", nil); !strings.Contains(badge, `<span class="spin"></span>`) {
		t.Errorf("subagent badge lost its spinner: %s", badge)
	}
	// Reduced motion stops the pulse as it stops the spinner.
	css := mustAsset(t, "dashboard.css")
	if !strings.Contains(css, "@media (prefers-reduced-motion: reduce) { .spin, .caret, .pulse { animation: none; } }") {
		t.Errorf("reduced-motion rule does not name .pulse")
	}
}
