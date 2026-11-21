// Conversation and index view tests cover rendered state.

package wsdashboard

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

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

func TestSavedConversationCarriesNoLiveState(t *testing.T) {
	now := testNow
	stale := liveSnap(now.Add(-time.Hour), wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, Reply: &wslive.Reply{Text: "stale reply"}})
	for _, snap := range []*wslive.Snapshot{nil, stale} {
		v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: snap, Now: now,
			Ring: []wslive.RingRecord{{Name: "n", At: now, RingEntry: wslive.RingEntry{Kind: wslive.RingUser, Text: "x"}}},
			Page: PageContent{HasBoard: true, Contacts: []Contact{{Address: "id:peer", ReplyID: "peer", LastExchange: now}}}})
		if v.Adapter || v.Mode() != modeHistory || len(v.Tree.Peers.Rows()) != 0 || len(v.Tree.Agents) != 0 || v.Conversation.Reply != nil || v.Conversation.Phase != "" || v.Conversation.Permission != nil || v.Conversation.ToolGap != nil || v.Host != nil || len(v.Commands) != 0 {
			t.Fatalf("saved history carries live state: %+v", v)
		}
		if !v.Live || len(v.Conversation.Entries) != 1 || v.Conversation.Entries[0].Text != "x" {
			t.Fatalf("saved history changed registration liveness or lost persisted text: %+v", v)
		}
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

// TestHostedRecoveryAndFailures pins what a hosted session's page shows of
// its runner's host state and typed failures. The head: an error ring on
// the harness chip and a chip naming the failure's kind (the harness text
// on hover) while a failure is set, and an MCP-broken chip naming the
// servers on hover. The agents panel head: the background task count. The
// live region: the count for the composer's badge (data-bg) and a retry
// row, "attempt N/M" or "attempt N" when the maximum is unknown, with the
// error after it. The conversation: a notice line per typed failure in its
// own words, a usage limit with its reset time when known, the harness text
// after it. A session with no host state shows none of these.
func TestHostedRecoveryAndFailures(t *testing.T) {
	now := testNow
	reset := now.Add(3 * time.Hour).UnixMilli()
	s := liveSession("hosted", "/a", now)
	s.Hosted = true
	ring := []wslive.RingRecord{
		{RingEntry: wslive.RingEntry{Kind: wslive.RingNotice, Notice: &wslive.Notice{Kind: wslive.NoticeLogin, Text: "Please run /login"}}, Name: "20261107T000000.000000001Z-n1.json", At: now},
		{RingEntry: wslive.RingEntry{Kind: wslive.RingNotice, Notice: &wslive.Notice{Kind: wslive.NoticeModel}}, Name: "20261107T000000.000000002Z-n2.json", At: now},
		{RingEntry: wslive.RingEntry{Kind: wslive.RingNotice, Notice: &wslive.Notice{Kind: wslive.NoticeUsageLimit, Text: "Limit reached", ResetsAt: reset}}, Name: "20261107T000000.000000003Z-n3.json", At: now},
		{RingEntry: wslive.RingEntry{Kind: wslive.RingNotice, Notice: &wslive.Notice{Kind: wslive.NoticeUsageLimit}}, Name: "20261107T000000.000000004Z-n4.json", At: now},
	}
	render := func(host *wslive.Host) regionSet {
		t.Helper()
		snap := liveSnap(now, wslive.Lead{})
		snap.Host = host
		v := buildSessionView(pageInputs{Session: s, Snapshot: snap, Ring: ring, Now: now, Lookup: lookupOf(nil, nil)})
		regions, err := renderSessionRegions(v, nil)
		if err != nil {
			t.Fatal(err)
		}
		return regions
	}
	host := &wslive.Host{
		BackgroundTasks: 2,
		Failure:         &wslive.Notice{Kind: wslive.NoticeUsageLimit, Text: "Limit reached", ResetsAt: reset},
		MCPBroken:       []string{"github", "plugin:ws:ws"},
		Retry:           &wslive.Retry{Attempt: 2, MaxRetries: 10, Error: "overloaded", Status: 529},
	}
	regions := render(host)
	head, tree, live, entries := string(regions[regionHead].HTML), string(regions[regionTree].HTML), string(regions[regionConvLive].HTML), string(regions[regionEntries].HTML)
	for _, want := range []string{
		`<span class="host sse stopped err" title=`,
		`<span class="chip fail" title="Limit reached">usage limit reached</span>`,
		`<span class="chip mcpbad" title="MCP servers that did not reconnect: github, plugin:ws:ws">MCP broken</span>`,
	} {
		if !strings.Contains(head, want) {
			t.Errorf("head lacks %s:\n%s", want, head)
		}
	}
	if want := `<span class="chip bgt" title="background tasks the harness runs: shells, monitors, and background subagents">Background 2</span>`; !strings.Contains(tree, want) {
		t.Errorf("agents panel head lacks %s:\n%s", want, tree)
	}
	for _, want := range []string{
		` data-bg="2" data-monitors="0">`,
		`<div class="sysline retry running"><span class="mk"><span class="spin"></span></span><span class="nt">retrying, attempt 2/10: overloaded</span></div>`,
	} {
		if !strings.Contains(live, want) {
			t.Errorf("live region lacks %s:\n%s", want, live)
		}
	}
	for _, want := range []string{
		`<div class="sysline notice fail" data-entry="20261107T000000.000000001Z-n1.json" data-notice="login"><span class="mk">✗</span><span class="nt"><b>login needed</b>: Please run /login</span><time`,
		`<div class="sysline notice fail" data-entry="20261107T000000.000000002Z-n2.json" data-notice="model"><span class="mk">✗</span><span class="nt"><b>model unavailable</b></span><time`,
		`<div class="sysline notice fail" data-entry="20261107T000000.000000003Z-n3.json" data-notice="usage_limit"><span class="mk">✗</span><span class="nt"><b>usage limit reached</b> · resets <time class="t" data-ms="` + fmt.Sprint(reset) + `" data-f="dt">` + clockText(reset) + `</time>: Limit reached</span><time`,
		`<div class="sysline notice fail" data-entry="20261107T000000.000000004Z-n4.json" data-notice="usage_limit"><span class="mk">✗</span><span class="nt"><b>usage limit reached</b></span><time`,
	} {
		if !strings.Contains(entries, want) {
			t.Errorf("entries lack %s:\n%s", want, entries)
		}
	}

	host.Failure, host.MCPBroken = nil, nil
	host.BackgroundTasks = 0
	host.Retry = &wslive.Retry{Attempt: 3}
	regions = render(host)
	if live := string(regions[regionConvLive].HTML); !strings.Contains(live, `<span class="nt">retrying, attempt 3</span>`) || !strings.Contains(live, ` data-bg="0" data-monitors="0">`) {
		t.Errorf("a retry with no maximum or error, and no background task:\n%s", live)
	}

	regions = render(nil)
	head, tree, live = string(regions[regionHead].HTML), string(regions[regionTree].HTML), string(regions[regionConvLive].HTML)
	for name, html := range map[string]string{"head": head, "tree": tree, "live": live} {
		for _, not := range []string{"sse err", "chip fail", "mcpbad", "chip bgt", "sysline retry"} {
			if strings.Contains(html, not) {
				t.Errorf("%s without host state shows %q:\n%s", name, not, html)
			}
		}
	}
}

// TestHostedSessionMarks pins what a hosted session's page and index row
// carry. The page: the hosted mark in the head and on <html> (which the
// page script reads), the box its approval cards go in, and a permission
// line that does not send the user to a terminal. The index row: the
// hosted mark, and while the lead waits on an approval the attributes the
// page's notification reads. A session the daemon does not host has none
// of these and keeps its terminal wording. The index page marks a browser
// on the daemon's machine, as a session page does.
func TestHostedSessionMarks(t *testing.T) {
	now := testNow
	wait := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhasePermission, Permission: &wslive.Permission{ToolName: "Bash", ArgsPreview: "git push", StartedAt: now.UnixMilli()}}}
	hosted := liveSession("hosted", "/a", now)
	hosted.Hosted = true
	cli := liveSession("cli", "/a", now)
	page := func(s Session) (head, live, html string) {
		t.Helper()
		v := buildSessionView(pageInputs{Session: s, Snapshot: liveSnap(now, wait), Now: now, Lookup: lookupOf(nil, nil)})
		if v.Hosted != s.Hosted {
			t.Errorf("%s: view hosted = %v", s.ReplyID, v.Hosted)
		}
		regions, err := renderSessionRegions(v, nil)
		if err != nil {
			t.Fatal(err)
		}
		if html, err = renderSessionPage(v, regions, true, ""); err != nil {
			t.Fatal(err)
		}
		return string(regions[regionHead].HTML), string(regions[regionConvLive].HTML), html
	}
	head, live, html := page(hosted)
	if !strings.Contains(head, `<span class="hosted" title="session the dashboard runs">`) {
		t.Errorf("hosted head lacks the hosted mark: %s", head)
	}
	if !strings.Contains(live, `<div class="permwait apslot">`) || !strings.Contains(live, `<b>waiting for approval</b>`) || strings.Contains(live, "terminal") {
		t.Errorf("hosted permission line: %s", live)
	}
	if !strings.Contains(html, ` data-hosted="1"`) || !strings.Contains(html, `<div class="apbox" id="apbox" hidden></div>`) {
		t.Error("hosted page lacks its mark or the approval box")
	}
	head, live, html = page(cli)
	if strings.Contains(head, `class="hosted"`) || !strings.Contains(live, `<div class="permwait">`) || !strings.Contains(live, `<b>waiting for approval in the terminal</b>`) {
		t.Errorf("a session the daemon does not host: head %s, permission line %s", head, live)
	}
	if strings.Contains(html, ` data-hosted="1"`) || strings.Contains(html, `id="apbox"`) {
		t.Error("a session the daemon does not host carries the hosted mark or the approval box")
	}

	view := buildIndexView([]indexInput{
		{Session: hosted, Snapshot: liveSnap(now, wait)},
		{Session: cli, Snapshot: liveSnap(now, wait)},
	}, now, "1.0.0")
	for _, r := range view.Projects[0].Sessions {
		if r.Hosted != (r.ReplyID == "hosted") || (r.Approval != nil) != r.Hosted || (r.Approval != nil && *r.Approval != (ApprovalWait{Tool: "Bash", Args: "git push"})) {
			t.Errorf("index row %s = %+v (approval %+v)", r.ReplyID, r, r.Approval)
		}
	}
	regions, err := renderIndexRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	rows := string(regions[regionSessions].HTML)
	if !strings.Contains(rows, `href="/s/hosted" data-rid="hosted" data-ap="1" data-ap-tool="Bash" data-ap-args="git push">`) || !strings.Contains(rows, `href="/s/cli" data-rid="cli">`) {
		t.Errorf("index rows: %s", rows)
	}
	if n := strings.Count(rows, `<span class="hosted" title="session the dashboard runs">`); n != 1 {
		t.Errorf("hosted marks on the index = %d, want 1: %s", n, rows)
	}
	for local, want := range map[bool]bool{true: true, false: false} {
		index, err := renderIndexPage(view, regions, nil, local, true)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(index, `data-sysopen="1"`); got != want {
			t.Errorf("index page for a local viewer %v: local mark = %v", local, got)
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
