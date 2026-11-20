package mcp

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsdoc"
)

var (
	panelSectionPattern = regexp.MustCompile(`(?s)<details class="panel" id="p-([a-z]+)"[^>]*>.*?</details>`)
	panelLinkPattern    = regexp.MustCompile(`<a[^>]*data-rel="([^"]+)"[^>]*>([^<]*)</a>`)
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

func (f *whiteboardFixture) servedPage(t *testing.T) string {
	t.Helper()
	if f.source == nil {
		f.source = dashboardPageSource(t, nil)
	}
	entry, ok := f.s.sessions.lookup(f.key)
	if !ok {
		t.Fatal("fixture session missing")
	}
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: entry.root}
	// A daemon nudge refreshes authoritative session inputs before ReadPage.
	if _, ok, err := f.source.ReadSession(reg); !ok || err != nil {
		t.Fatalf("session refresh: %v", err)
	}
	page := readPageOK(t, f.source, reg)
	return string(page.Board.Board) + string(page.Board.Panels)
}

func writeTicket(t *testing.T, root, status, stem, title string) {
	t.Helper()
	body := "---\n"
	if title != "" {
		body += "title: " + title + "\n"
	}
	mustWrite(t, root, "ai-docs/tickets/"+status+"/"+stem+".md", body+"---\n\n# "+stem+"\n")
}

func linkTexts(links []panelLink) []string {
	out := make([]string, len(links))
	for i, l := range links {
		out[i] = l.text
	}
	return out
}

// TestWhiteboardTicketPanel pins the ticket panel: the full page lists every
// ready/ and todo/ ticket, never idea/, with served file actions.
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
	full := panelHTML(f.servedPage(t), "tickets")
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

// Empty mailbox history omits its panel; the ticket demand control remains.
func TestWhiteboardPanelsOmittedWhenEmpty(t *testing.T) {
	f := newWhiteboardFixture(t)
	writeTicket(t, f.root, "idea", "261106-feat-idea-x", "Idea X")
	f.update(t, ``)
	for _, id := range []string{"mailbox"} {
		if panelHTML(f.servedPage(t), id) != "" {
			t.Fatalf("empty %s panel rendered", id)
		}
	}
	writeTicket(t, f.root, "todo", "261106-feat-todo-x", "Todo X")
	f.update(t, ``)
	if panelHTML(f.servedPage(t), "tickets") == "" {
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
	full := panelHTML(f.servedPage(t), "todo")
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

// TestWhiteboardAgendaPanelTree pins the served agenda panel: each blob is a tree (root open, nested containers
// folded, a long string folded to its whole value). agenda.clear drops the
// key, and clearing the last key drops the panel.
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
	for name, page := range map[string]string{"served": served()} {
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
	// This isolated source has no daemon signal; allow its validation interval.
	waitForDashboardValidation(t)
	for name, page := range map[string]string{"served": served()} {
		if strings.Contains(page, "round2") || !strings.Contains(page, "agenda/overnight") || !strings.Contains(page, `<span class="chip">1</span>`) {
			t.Errorf("%s agenda panel after clearing round2:\n%s", name, page)
		}
	}
	if resp := f.call(t, "agenda.clear", map[string]any{"all": true}); isToolError(resp) {
		t.Fatalf("agenda.clear all: %s", resp)
	}
	waitForDashboardValidation(t)
	for name, page := range map[string]string{"served": served()} {
		if strings.Contains(page, `id="p-agenda"`) {
			t.Errorf("%s page keeps an agenda panel with no agenda", name)
		}
	}
}

// TestWhiteboardTicketWriteRefreshesServedPanel pins the ticket panel's refresh:
// a tickets.move refreshes the served panel without publishing files.
func TestWhiteboardTicketWriteRefreshesServedPanel(t *testing.T) {
	f := newWhiteboardFixture(t)
	writeTicket(t, f.root, "ready", "261101-feat-ready-a", "Ready A")
	writeTicket(t, f.root, "ready", "261102-feat-ready-b", "Ready B")
	runGit(t, f.root, "add", "-A")
	runGit(t, f.root, "commit", "-q", "-m", "tickets")
	f.update(t, ``)
	boardBefore := whiteboardTreeForTest(t, f.pageDir())
	if !strings.Contains(panelHTML(f.servedPage(t), "tickets"), "Ready B") {
		t.Fatalf("ticket panel lacks Ready B before the move")
	}
	if resp := f.call(t, "tickets.move", map[string]any{"stem": "261102-feat-ready-b", "to": "idea"}); isToolError(resp) {
		t.Fatalf("tickets.move: %s", resp)
	}
	panel := panelHTML(f.servedPage(t), "tickets")
	if strings.Contains(panel, "Ready B") || !strings.Contains(panel, "Ready A") {
		t.Fatalf("ticket panel after the move = %s", panel)
	}
	assertWhiteboardTreeForTest(t, f.pageDir(), boardBefore)
}

func TestWhiteboardServedPageResolvesTopicTickets(t *testing.T) {
	f := newWhiteboardFixture(t)
	writeTicket(t, f.root, "ready", "261101-feat-go", "Go")
	writeTicket(t, f.root, ".dropped", "261101-bug-gone", "Gone")
	f.update(t, `<wb:topic name="t" tickets="261101-feat-go 261101-bug-gone 261101-chore-nowhere"><wb:log>working</wb:log></wb:topic>`)
	page := f.servedPage(t)
	for _, want := range []string{
		`data-topic="t"`, `<span class="tname" title="t">t</span>`, `data-kind="agent"`, "working",
		`<span class="dw wbtk">`, `<span class="bd">3</span>`,
		`<span class="row open" data-ticket="261101-feat-go" data-status="ready">`,
		`<span class="row dropped" data-ticket="261101-bug-gone" data-status="dropped">`,
		`<span class="row unknown" data-ticket="261101-chore-nowhere" data-status="unknown">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the served page lacks %q", want)
		}
	}
	if regexp.MustCompile(`<section class="topic[^"]*\blive\b`).MatchString(page) {
		t.Error("the served page renders a ticket-derived topic glow")
	}
	resp := f.call(t, "whiteboard.query", map[string]any{"payload": map[string]any{"topic": "t", "depth": "topics"}})
	if text := toolText(t, resp); isToolError(resp) || !strings.Contains(text, "tickets: 261101-feat-go 261101-bug-gone 261101-chore-nowhere") {
		t.Fatalf("stored links missing from query:\n%s", text)
	}
}
