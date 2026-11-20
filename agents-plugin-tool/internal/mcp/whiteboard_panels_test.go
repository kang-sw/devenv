package mcp

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

var (
	panelSectionPattern = regexp.MustCompile(`(?s)<details class="panel" id="p-([a-z]+)"[^>]*>.*?</details>`)
	panelLinkPattern    = regexp.MustCompile(`<a href="([^"]+)">([^<]*)</a>`)
	panelKeyedPattern   = regexp.MustCompile(`<li class="keyed">(?:<code>([^<]*)</code>)?<span>(?:<a href="[^"]*">)?([^<]*)`)
)

// panelHTML returns the rendered panel with the given id, "" when absent.
func panelHTML(page, id string) string {
	for _, m := range panelSectionPattern.FindAllStringSubmatch(page, -1) {
		if m[1] == id {
			return m[0]
		}
	}
	return ""
}

type panelLink struct{ href, text string }

func panelLinks(panel string) []panelLink {
	var out []panelLink
	for _, m := range panelLinkPattern.FindAllStringSubmatch(panel, -1) {
		out = append(out, panelLink{href: m[1], text: m[2]})
	}
	return out
}

// panelRows returns each keyed row as "label | text".
func panelRows(panel string) []string {
	var out []string
	for _, m := range panelKeyedPattern.FindAllStringSubmatch(panel, -1) {
		out = append(out, m[1]+" | "+m[2])
	}
	return out
}

func readPage(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read page %s: %v", path, err)
	}
	return string(data)
}

func (f *whiteboardFixture) fullPage(t *testing.T) string {
	t.Helper()
	return readPage(t, f.pagePath())
}

func writeTicket(t *testing.T, root, status, stem, title string) {
	t.Helper()
	body := "---\n"
	if title != "" {
		body += "title: " + title + "\n"
	}
	mustWrite(t, root, "ai-docs/tickets/"+status+"/"+stem+".md", body+"---\n\n# "+stem+"\n")
}

// assertLinksResolve checks each link names an existing file from pageDir:
// a relative path, or a file: URL when wantFileURL.
func assertLinksResolve(t *testing.T, name, pageDir string, links []panelLink, wantFileURL bool) {
	t.Helper()
	for _, l := range links {
		if strings.Contains(l.href, "ZgotmplZ") {
			t.Fatalf("%s: link %q was rewritten by the template", name, l.href)
		}
		var target string
		if wantFileURL {
			u, err := url.Parse(l.href)
			if err != nil || u.Scheme != "file" {
				t.Fatalf("%s: link %q is not a file: URL", name, l.href)
			}
			path := u.Path
			if len(path) > 2 && path[0] == '/' && path[2] == ':' {
				path = path[1:] // file:///C:/x names the drive path C:/x
			}
			target = filepath.FromSlash(path)
		} else {
			if strings.Contains(l.href, ":") || filepath.IsAbs(l.href) {
				t.Fatalf("%s: link %q is not a relative path", name, l.href)
			}
			target = filepath.Join(pageDir, filepath.FromSlash(l.href))
		}
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("%s: link %q (%s) does not resolve from %s: %v", name, l.text, l.href, pageDir, err)
		}
	}
}

func linkTexts(links []panelLink) []string {
	out := make([]string, len(links))
	for i, l := range links {
		out[i] = l.text
	}
	return out
}

// TestWhiteboardTicketPanel pins the ticket panel: the full page lists every
// ready/ and todo/ ticket, never idea/, and every link resolves from the
// whiteboard page directory.
func TestWhiteboardTicketPanel(t *testing.T) {
	f := newWhiteboardFixture(t)
	writeTicket(t, f.root, "ready", "261101-feat-ready-a", "Ready A")
	writeTicket(t, f.root, "ready", "261020-bug-ready-b", "")
	writeTicket(t, f.root, "idea", "261106-feat-idea-x", "Idea X")
	for day := 1; day <= 11; day++ {
		writeTicket(t, f.root, "todo", fmt.Sprintf("2610%02d-feat-todo", day), fmt.Sprintf("Todo %02d", day))
	}
	writeTicket(t, f.root, "todo", "261011-bug-todo", "Todo 11 bug")

	f.update(t, ``)
	full := panelHTML(f.fullPage(t), "tickets")
	if full == "" {
		t.Fatalf("full page has no ticket panel")
	}
	links := panelLinks(full)
	want := []string{"261020-bug-ready-b", "Ready A", "Todo 11", "Todo 11 bug", "Todo 10", "Todo 09", "Todo 08", "Todo 07", "Todo 06", "Todo 05", "Todo 04", "Todo 03", "Todo 02", "Todo 01"}
	if got := linkTexts(links); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("full ticket rows = %q, want %q", got, want)
	}
	if !strings.Contains(full, `<span class="chip">2 ready, 12 todo</span>`) {
		t.Fatalf("ticket chip must count the whole backlog: %s", full)
	}
	for _, want := range []string{
		`<span class="lname">ready</span><span class="chip">2</span>`,
		`<span class="lname">todo</span><span class="chip">12</span>`,
		`<span class="chip cat bug">bug</span>`,
		`<span class="csub" title="261101-feat-ready-a">261101-feat-ready-a</span>`,
	} {
		if !strings.Contains(full, want) {
			t.Fatalf("ticket lanes lack %q: %s", want, full)
		}
	}
	if strings.Contains(full, `class="fpath"`) {
		t.Fatalf("a card must not show its path as body text")
	}
	if strings.Contains(full, "Idea X") {
		t.Fatalf("idea tickets must not be listed")
	}
	assertLinksResolve(t, "full", f.pageDir(), links, false)

}

// TestWhiteboardLatestTodoTicketsFollowsStemDate pins the latest-ten selection: by the
// stem's date prefix, newest first, ties by stem name descending, never by
// listing order.
func TestWhiteboardLatestTodoTicketsFollowsStemDate(t *testing.T) {
	var in []wsdoc.TicketInfo
	for _, stem := range []string{"261003-feat-c", "260101-feat-zzz", "261105-bug-a", "261105-feat-b", "261002-feat-z"} {
		in = append(in, wsdoc.TicketInfo{Stem: stem})
	}
	got := latestTodoTickets(in, 3)
	var stems []string
	for _, t := range got {
		stems = append(stems, t.Stem)
	}
	if want := "261105-feat-b|261105-bug-a|261003-feat-c"; strings.Join(stems, "|") != want {
		t.Fatalf("latest = %v, want %s", stems, want)
	}
	if in[0].Stem != "261003-feat-c" {
		t.Fatalf("selection reordered its input")
	}
}

// TestWhiteboardPanelsOmittedWhenEmpty pins that a session with no ready or
// todo tickets and no mailbox address or history renders neither panel.
func TestWhiteboardPanelsOmittedWhenEmpty(t *testing.T) {
	f := newWhiteboardFixture(t)
	writeTicket(t, f.root, "idea", "261106-feat-idea-x", "Idea X")
	f.update(t, ``)
	for _, id := range []string{"tickets", "mailbox"} {
		if panelHTML(f.fullPage(t), id) != "" {
			t.Fatalf("empty %s panel rendered", id)
		}
	}
	writeTicket(t, f.root, "todo", "261106-feat-todo-x", "Todo X")
	f.update(t, ``)
	if panelHTML(f.fullPage(t), "tickets") == "" {
		t.Fatalf("full page must list the todo ticket")
	}
}

// TestWhiteboardTodoPanelShowsAllRows pins the todo count, order, and folded
// completed rows on the full page.
func TestWhiteboardTodoPanelShowsAllRows(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.call(t, "todo.add", map[string]any{"key": "a", "title": "finished work"})
	f.call(t, "todo.add", map[string]any{"key": "b", "title": "open work"})
	f.call(t, "todo.add", map[string]any{"key": "c", "title": "deferred work"})
	f.call(t, "todo.check", map[string]any{"key": "a", "status": "done"})
	f.call(t, "todo.check", map[string]any{"key": "c", "status": "defer"})
	f.update(t, ``)
	full := panelHTML(f.fullPage(t), "todo")
	if !strings.Contains(full, "finished work") || !strings.Contains(full, "open work") {
		t.Fatalf("full todo panel must list every todo: %s", full)
	}
	if !strings.Contains(full, `<span class="chip">1/3</span>`) || !strings.Contains(full, `done 1</summary>`) {
		t.Fatalf("full todo panel keeps the count and folds done rows: %s", full)
	}
	fold := strings.Index(full, `class="folded"`)
	if fold < 0 || strings.Index(full, "open work") > strings.Index(full, "deferred work") || strings.Index(full, "deferred work") > fold || fold > strings.Index(full, "finished work") {
		t.Fatalf("full todo panel order must be unfinished rows, fold, done rows: %s", full)
	}
}

// agendaPanelHTML returns the agenda panel of a page up to the next panel or
// the panel column's end: its tree nests details, which panelHTML's
// first-</details> match would cut short. "" when absent.
func agendaPanelHTML(page string) string {
	start := strings.Index(page, `<details class="panel" id="p-agenda"`)
	if start < 0 {
		return ""
	}
	rest := page[start+1:]
	end := len(rest)
	for _, stop := range []string{`<details class="panel"`, `</aside>`, `</div></body>`} {
		if i := strings.Index(rest, stop); i >= 0 && i < end {
			end = i
		}
	}
	return page[start : start+1+end]
}

// TestWhiteboardAgendaPanelTree pins the agenda panel: the full page, on disk
// and served, lays each blob out as a tree (root open, nested containers
// folded, a long string folded to its whole value). agenda.clear drops the
// key from both views, and clearing the last key drops the panel.
func TestWhiteboardAgendaPanelTree(t *testing.T) {
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	served := func() string {
		page := readPageOK(t, src, reg)
		return string(page.Board.Panels)
	}
	long := strings.Repeat("long text ", 10)
	f.call(t, "agenda.set", map[string]any{"key": "round2", "value": map[string]any{"notes": "masonry memo"}})
	f.call(t, "agenda.set", map[string]any{"key": "overnight", "value": map[string]any{
		"run":   map[string]any{"branch": "delegate/x", "phases": []any{map[string]any{"n": 1, "done": true}}},
		"story": long,
		"flaky": []any{},
	}})
	f.update(t, ``)
	for name, page := range map[string]string{"disk": agendaPanelHTML(f.fullPage(t)), "served": served()} {
		for _, want := range []string{
			`<details class="tnode" data-key="agenda/overnight" open>`,
			`<details class="tnode" data-key="agenda/overnight/run"><summary><span class="tkey">run</span><span class="tsum">{branch, phases}</span>`,
			`<details class="tnode" data-key="agenda/overnight/run/phases">`,
			`<details class="tnode tlong" data-key="agenda/overnight/story">`,
			`<span class="tkey">flaky</span><span class="tv arr">[]</span>`,
			`<details class="tnode" data-key="agenda/round2" open>`,
			`<span class="tkey">notes</span><span class="tv str">masonry memo</span>`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("%s agenda panel lacks %s\n%s", name, want, page)
			}
		}
		if strings.Index(page, "agenda/overnight") > strings.Index(page, "agenda/round2") {
			t.Errorf("%s agenda panel must list keys in order", name)
		}
	}

	if resp := f.call(t, "agenda.clear", map[string]any{"key": "round2"}); isToolError(resp) {
		t.Fatalf("agenda.clear: %s", resp)
	}
	// Disk output is synchronous. This standalone presentation source gets
	// no daemon signal and may retain its parsed agenda for one second.
	disk := agendaPanelHTML(f.fullPage(t))
	waitForDashboardValidation(t)
	for name, page := range map[string]string{"disk": disk, "served": served()} {
		if strings.Contains(page, "round2") || !strings.Contains(page, "agenda/overnight") || !strings.Contains(page, `<span class="chip">1</span>`) {
			t.Errorf("%s agenda panel after clearing round2:\n%s", name, page)
		}
	}
	if resp := f.call(t, "agenda.clear", map[string]any{"all": true}); isToolError(resp) {
		t.Fatalf("agenda.clear all: %s", resp)
	}
	disk = f.fullPage(t) // preserve the immediate disk-output assertion
	waitForDashboardValidation(t)
	for name, page := range map[string]string{"disk": disk, "served": served()} {
		if strings.Contains(page, `id="p-agenda"`) {
			t.Errorf("%s page keeps an agenda panel with no agenda", name)
		}
	}
}

// TestWhiteboardTicketWriteRerendersFullPage pins the ticket panel's refresh:
// a tickets.move re-renders the full page (stamp bumped, no embed) with the
// panel updated.
func TestWhiteboardTicketWriteRerendersFullPage(t *testing.T) {
	f := newWhiteboardFixture(t)
	writeTicket(t, f.root, "ready", "261101-feat-ready-a", "Ready A")
	writeTicket(t, f.root, "ready", "261102-feat-ready-b", "Ready B")
	runGit(t, f.root, "add", "-A")
	runGit(t, f.root, "commit", "-q", "-m", "tickets")
	f.update(t, ``)
	embedsBefore := len(f.embeds(t, f.dir()))
	if !strings.Contains(panelHTML(f.fullPage(t), "tickets"), "Ready B") {
		t.Fatalf("ticket panel lacks Ready B before the move")
	}
	if resp := f.call(t, "tickets.move", map[string]any{"stem": "261102-feat-ready-b", "to": "idea"}); isToolError(resp) {
		t.Fatalf("tickets.move: %s", resp)
	}
	if got := f.stamp(t); got != "wbStamp(\"2\");\n" {
		t.Fatalf("stamp after tickets.move = %q, want a re-render", got)
	}
	panel := panelHTML(f.fullPage(t), "tickets")
	if strings.Contains(panel, "Ready B") || !strings.Contains(panel, "Ready A") {
		t.Fatalf("ticket panel after the move = %s", panel)
	}
	if got := len(f.embeds(t, f.dir())); got != embedsBefore {
		t.Fatalf("a ticket write must write no embed: %d -> %d", embedsBefore, got)
	}
}

// TestWhiteboardMailboxPanel pins the mailbox panel and its refresh. The full
// page carries the owned address, unread count, armed-wait state,
// correspondents, and history ring. mailbox.send and mailbox.recv each
// re-render it; mail arriving from another session shows on the next render.
func TestWhiteboardMailboxPanel(t *testing.T) {
	t0 := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	hf := newHistoryFixture(t, t0)
	f := &whiteboardFixture{s: hf.serverA, key: hf.keyA, root: hf.root, id: 100}
	replyA := wsmailbox.ReplyIDPrefix + wsmailbox.ReplyID(hf.keyA)
	replyB := wsmailbox.ReplyIDPrefix + wsmailbox.ReplyID(hf.keyB)

	f.update(t, ``)
	if got := panelRows(panelHTML(f.fullPage(t), "mailbox")); strings.Join(got, "\n") != "alice@worktree | 0 unread" {
		t.Fatalf("mailbox panel of an owned address with no history = %q", got)
	}

	f.call(t, "mailbox.send", map[string]any{"to": replyB, "content": "hello b\nsecond line"})
	if got := f.stamp(t); got != "wbStamp(\"2\");\n" {
		t.Fatalf("stamp after mailbox.send = %q, want a re-render", got)
	}
	stamp := t0.Format(time.RFC3339)
	wantSent := []string{
		"alice@worktree | 0 unread",
		replyB + " | last exchange " + stamp,
		stamp + " sent to " + replyB + " | hello b",
	}
	if got := panelRows(panelHTML(f.fullPage(t), "mailbox")); strings.Join(got, "\n") != strings.Join(wantSent, "\n") {
		t.Fatalf("mailbox panel after send =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(wantSent, "\n"))
	}

	// Mail from another session: not refreshed until A's next render.
	mustSend(t, hf.serverB, 50, hf.keyB, "alice@worktree", "to the inbox")
	mustSend(t, hf.serverB, 51, hf.keyB, replyA, "to the reply id")
	if err := wsmailbox.WriteListeningMarker(wsmailbox.ListeningMarker{SessionKey: hf.keyA, PID: os.Getpid(), StartedAt: stamp}); err != nil {
		t.Fatal(err)
	}
	f.update(t, ``)
	if got := panelRows(panelHTML(f.fullPage(t), "mailbox")); len(got) == 0 || got[0] != "alice@worktree | 2 unread, waiting for mail" {
		t.Fatalf("full mailbox status row = %q", got)
	}
	if err := wsmailbox.ClearListeningMarker(hf.keyA, os.Getpid()); err != nil {
		t.Fatal(err)
	}

	t1 := t0.Add(time.Minute)
	setMailboxClock(t, t1)
	f.call(t, "mailbox.recv", map[string]any{})
	got := panelRows(panelHTML(f.fullPage(t), "mailbox"))
	if len(got) != 5 || got[0] != "alice@worktree | 0 unread" || got[1] != replyB+" | last exchange "+t1.Format(time.RFC3339) {
		t.Fatalf("mailbox panel after recv = %q", got)
	}
	// The ring's titles skip the peer-lead note line each fixture body
	// carries; the panel derives none of its own.
	titles := map[string]bool{}
	for _, row := range got[2:4] {
		if !strings.HasPrefix(row, t1.Format(time.RFC3339)+" recv from "+replyB+" | ") {
			t.Fatalf("recv row = %q", row)
		}
		titles[strings.SplitN(row, " | ", 2)[1]] = true
	}
	if !titles["to the inbox"] || !titles["to the reply id"] {
		t.Fatalf("recv rows do not show the ring's titles: %q", got)
	}
}

// TestWhiteboardFilePageResolvesTopicTickets pins the file page's ticket
// icon: each linked stem's popover row carries the status of the directory
// holding it at render time, unknown for a stem no directory holds, and the
// topic gets no ticket-derived glow.
func TestWhiteboardFilePageResolvesTopicTickets(t *testing.T) {
	f := newWhiteboardFixture(t)
	writeTicket(t, f.root, "ready", "261101-feat-go", "Go")
	writeTicket(t, f.root, ".dropped", "261101-bug-gone", "Gone")
	f.update(t, `<wb:topic name="t" tickets="261101-feat-go 261101-bug-gone 261101-chore-nowhere"><wb:log>working</wb:log></wb:topic>`)
	page := f.fullPage(t)
	for _, want := range []string{
		`data-topic="t"`, `<span class="tname" title="t">t</span>`, `data-kind="agent"`, "working",
		`<span class="dw wbtk">`, `<span class="bd">3</span>`,
		`<span class="row open" data-ticket="261101-feat-go" data-status="ready">`,
		`<span class="row dropped" data-ticket="261101-bug-gone" data-status="dropped">`,
		`<span class="row unknown" data-ticket="261101-chore-nowhere" data-status="unknown">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the file page lacks %q", want)
		}
	}
	if regexp.MustCompile(`<section class="topic[^"]*\blive\b`).MatchString(page) {
		t.Error("the file page renders a ticket-derived topic glow")
	}
	resp := f.call(t, "whiteboard.query", map[string]any{"payload": map[string]any{"topic": "t", "depth": "topics"}})
	if text := toolText(t, resp); isToolError(resp) || !strings.Contains(text, "tickets: 261101-feat-go 261101-bug-gone 261101-chore-nowhere") {
		t.Fatalf("stored links missing from query:\n%s", text)
	}
}
