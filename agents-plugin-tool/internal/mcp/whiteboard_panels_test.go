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
	return readPage(t, filepath.Join(f.dir(), f.replyID()+".html"))
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
			target = filepath.FromSlash(u.Path)
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
// ready/ ticket and the latest ten todo/ tickets, never idea/; the embed lists
// ready/ only; and every link resolves from where its page is written (the
// whiteboard folder, an embed_dir inside the repository, or one outside it,
// where it is a file: URL).
func TestWhiteboardTicketPanel(t *testing.T) {
	f := newWhiteboardFixture(t)
	writeTicket(t, f.root, "ready", "261101-feat-ready-a", "Ready A")
	writeTicket(t, f.root, "ready", "261020-bug-ready-b", "")
	writeTicket(t, f.root, "idea", "261106-feat-idea-x", "Idea X")
	for day := 1; day <= 11; day++ {
		writeTicket(t, f.root, "todo", fmt.Sprintf("2610%02d-feat-todo", day), fmt.Sprintf("Todo %02d", day))
	}
	writeTicket(t, f.root, "todo", "261011-bug-todo", "Todo 11 bug")

	f.update(t, `{}`)
	full := panelHTML(f.fullPage(t), "tickets")
	if full == "" {
		t.Fatalf("full page has no ticket panel")
	}
	links := panelLinks(full)
	want := []string{"261020-bug-ready-b", "Ready A", "Todo 11", "Todo 11 bug", "Todo 10", "Todo 09", "Todo 08", "Todo 07", "Todo 06", "Todo 05", "Todo 04", "Todo 03"}
	if got := linkTexts(links); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("full ticket rows = %q, want %q", got, want)
	}
	if !strings.Contains(full, `<span class="chip">2 ready, 12 todo</span>`) {
		t.Fatalf("ticket chip must count the whole backlog: %s", full)
	}
	if strings.Contains(full, "Idea X") {
		t.Fatalf("idea tickets must not be listed")
	}
	assertLinksResolve(t, "full", f.dir(), links, false)

	embedPage := func(embed string) string {
		t.Helper()
		if filepath.IsAbs(embed) {
			return readPage(t, embed)
		}
		return readPage(t, filepath.Join(f.root, filepath.FromSlash(embed)))
	}
	wantEmbed := []string{"261020-bug-ready-b", "Ready A"}

	embed := panelHTML(embedPage(resultLine(t, f.update(t, `{}`), "embed: ")), "tickets")
	if got := linkTexts(panelLinks(embed)); strings.Join(got, "|") != strings.Join(wantEmbed, "|") {
		t.Fatalf("embed ticket rows = %q, want ready only %q", got, wantEmbed)
	}
	assertLinksResolve(t, "embed in the whiteboard folder", f.dir(), panelLinks(embed), false)

	inner := filepath.Join(f.root, "host", "visuals")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	name := resultLine(t, f.update(t, fmt.Sprintf(`{"embed_dir":%q}`, inner)), "embed: ")
	embed = panelHTML(embedPage(filepath.Join(inner, name)), "tickets")
	assertLinksResolve(t, "embed_dir inside the repository", inner, panelLinks(embed), false)

	outside := t.TempDir()
	name = resultLine(t, f.update(t, fmt.Sprintf(`{"embed_dir":%q}`, outside)), "embed: ")
	embed = panelHTML(embedPage(filepath.Join(outside, name)), "tickets")
	if got := linkTexts(panelLinks(embed)); strings.Join(got, "|") != strings.Join(wantEmbed, "|") {
		t.Fatalf("outside embed ticket rows = %q, want %q", got, wantEmbed)
	}
	assertLinksResolve(t, "embed_dir outside the repository", outside, panelLinks(embed), true)
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
// todo tickets and no mailbox address or history renders neither panel, and
// that an embed with todo tickets but no ready ones omits the ticket panel.
func TestWhiteboardPanelsOmittedWhenEmpty(t *testing.T) {
	f := newWhiteboardFixture(t)
	writeTicket(t, f.root, "idea", "261106-feat-idea-x", "Idea X")
	embed := resultLine(t, f.update(t, `{}`), "embed: ")
	for _, page := range []string{f.fullPage(t), readPage(t, filepath.Join(f.root, embed))} {
		for _, id := range []string{"tickets", "mailbox"} {
			if panelHTML(page, id) != "" {
				t.Fatalf("empty %s panel rendered", id)
			}
		}
	}
	writeTicket(t, f.root, "todo", "261106-feat-todo-x", "Todo X")
	embed = resultLine(t, f.update(t, `{}`), "embed: ")
	if panelHTML(f.fullPage(t), "tickets") == "" {
		t.Fatalf("full page must list the todo ticket")
	}
	if panelHTML(readPage(t, filepath.Join(f.root, embed)), "tickets") != "" {
		t.Fatalf("an embed with no ready tickets must omit the ticket panel")
	}
}

// TestWhiteboardEmbedTodoPanelShowsUnfinished pins the embed todo panel: the
// count of every todo plus the titles of the unfinished ones; the full page
// lists them all.
func TestWhiteboardEmbedTodoPanelShowsUnfinished(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.call(t, "todo.add", map[string]any{"key": "a", "title": "finished work"})
	f.call(t, "todo.add", map[string]any{"key": "b", "title": "open work"})
	f.call(t, "todo.add", map[string]any{"key": "c", "title": "deferred work"})
	f.call(t, "todo.check", map[string]any{"key": "a", "status": "done"})
	f.call(t, "todo.check", map[string]any{"key": "c", "status": "defer"})
	embed := panelHTML(readPage(t, filepath.Join(f.root, resultLine(t, f.update(t, `{}`), "embed: "))), "todo")
	if !strings.Contains(embed, `<span class="chip">1/3</span>`) || !strings.Contains(embed, "open work") || !strings.Contains(embed, "deferred work") || strings.Contains(embed, "finished work") {
		t.Fatalf("embed todo panel = %s", embed)
	}
	full := panelHTML(f.fullPage(t), "todo")
	if !strings.Contains(full, "finished work") || !strings.Contains(full, "open work") {
		t.Fatalf("full todo panel must list every todo: %s", full)
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
	f.update(t, `{}`)
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
// page carries the owned address, the unread count over the named inbox and
// the reply-id queue, the armed-wait state, the correspondents, and the
// history ring with the ring's own titles; the embed carries one line. A
// mailbox.send and a mailbox.recv each re-render the full page; mail
// arriving from another session shows on the next render.
func TestWhiteboardMailboxPanel(t *testing.T) {
	t0 := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	hf := newHistoryFixture(t, t0)
	f := &whiteboardFixture{s: hf.serverA, key: hf.keyA, root: hf.root, id: 100}
	replyA := wsmailbox.ReplyIDPrefix + wsmailbox.ReplyID(hf.keyA)
	replyB := wsmailbox.ReplyIDPrefix + wsmailbox.ReplyID(hf.keyB)

	f.update(t, `{}`)
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
	embed := panelHTML(readPage(t, filepath.Join(f.root, resultLine(t, f.update(t, `{}`), "embed: "))), "mailbox")
	if got := panelRows(embed); strings.Join(got, "\n") != "alice@worktree | 2 unread" {
		t.Fatalf("embed mailbox line = %q", got)
	}
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
