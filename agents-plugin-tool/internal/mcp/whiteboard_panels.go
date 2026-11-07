package mcp

import (
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsi18n"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// The ticket and mailbox panels of the whiteboard. The server reads the data
// at render time and writes it into the page; the page itself reads no files.
// A change made outside a dirtying tool call (mail from another session, a
// ticket file edited by hand) shows on the next render.

// whiteboardLatestTodo is how many todo/ tickets the full page lists.
const whiteboardLatestTodo = 10

// whiteboardMailbox resolves, once per render, whether sessionKey holds this
// process's named inbox. A store that cannot be read reads as not the owner:
// the panel is descriptive and never fails a render.
func (s *Server) whiteboardMailbox(sessionKey, root string) whiteboardMailbox {
	owner, identity, err := s.mailboxOwnerCheck(sessionKey, root)
	return whiteboardMailbox{owner: owner && err == nil, identity: identity}
}

// ticketPanel shows the tickets as kanban lanes: every ready/ ticket and, on
// the full page only, the latest whiteboardLatestTodo todo/ tickets. Each
// card links to its file, or on a served page carries its repo-relative path
// (which the page makes the file action link, titled with that path). idea/ tickets are never listed.
func ticketPanel(in whiteboardPanelInput) (wswhiteboard.Panel, bool) {
	tickets, err := wsdoc.TicketsList(in.root, wsdoc.TicketListOptions{Statuses: []string{"ready", "todo"}})
	if err != nil {
		return wswhiteboard.Panel{}, false
	}
	var ready, todo []wsdoc.TicketInfo
	for _, t := range tickets {
		switch t.Status {
		case "ready":
			ready = append(ready, t)
		case "todo":
			todo = append(todo, t)
		}
	}
	pageDir := whiteboardPageDir(in)
	card := func(t wsdoc.TicketInfo) wswhiteboard.PanelRow {
		title, sub := t.Title, t.Stem
		if title == "" {
			title, sub = t.Stem, "" // the stem is already the title
		}
		row := wswhiteboard.PanelRow{Label: ticketCategory(t.Stem), Text: title, Sub: sub}
		if in.served {
			row.Path = filepath.ToSlash(t.Path)
		} else {
			target := filepath.Join(in.root, filepath.FromSlash(t.Path))
			row.Link = whiteboardFileLink(in.root, pageDir, target)
		}
		return row
	}
	lane := func(name string, tickets []wsdoc.TicketInfo, note string) wswhiteboard.PanelLane {
		l := wswhiteboard.PanelLane{Name: name, Count: fmt.Sprint(len(tickets)), Note: note}
		for _, t := range tickets {
			l.Rows = append(l.Rows, card(t))
		}
		return l
	}
	lanes := []wswhiteboard.PanelLane{lane(wsi18n.T(in.lang, "panel.lane.ready"), ready, "")}
	count := wsi18n.T(in.lang, "panel.tickets.count", len(ready))
	if in.layout == wswhiteboard.LayoutFull {
		// The chip counts the whole todo/ backlog, not the cards listed.
		count = wsi18n.T(in.lang, "panel.tickets.count.full", len(ready), len(todo))
		if len(todo) > 0 {
			latest := latestTodoTickets(todo, whiteboardLatestTodo)
			note := ""
			if len(latest) < len(todo) {
				note = wsi18n.T(in.lang, "panel.lane.latest", len(latest), len(todo))
			}
			lanes = append(lanes, lane(wsi18n.T(in.lang, "panel.lane.todo"), latest, note))
		}
	}
	if len(ready) == 0 && (in.layout != wswhiteboard.LayoutFull || len(todo) == 0) {
		return wswhiteboard.Panel{}, false
	}
	return wswhiteboard.Panel{ID: "tickets", Title: wsi18n.T(in.lang, "panel.tickets"), Icon: "board", Count: count, Lanes: lanes}, true
}

// ticketCategory is the category word of a ticket stem (<date>-<category>-
// <slug>: feat, bug, epic, research, ...), empty when the stem has none.
func ticketCategory(stem string) string {
	parts := strings.SplitN(stem, "-", 3)
	if len(parts) < 3 || parts[1] == "" {
		return ""
	}
	return parts[1]
}

// latestTodoTickets keeps the newest n tickets by creation date, newest
// first. The stem's fixed-width date prefix is the creation date, so a
// descending stem sort orders by date and, within one date, by stem name
// descending. File mtime is not used: it differs per checkout.
func latestTodoTickets(tickets []wsdoc.TicketInfo, n int) []wsdoc.TicketInfo {
	out := append([]wsdoc.TicketInfo(nil), tickets...)
	sort.Slice(out, func(i, j int) bool { return out[i].Stem > out[j].Stem })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// whiteboardPageDir is the folder the page being rendered is written to:
// the embed's embed_dir when one is set, else the repository's whiteboard
// folder.
func whiteboardPageDir(in whiteboardPanelInput) string {
	if in.layout == wswhiteboard.LayoutEmbed && in.rec.Whiteboard != nil && in.rec.Whiteboard.EmbedDir != "" {
		return in.rec.Whiteboard.EmbedDir
	}
	return filepath.Join(in.root, filepath.FromSlash(whiteboardDirRel))
}

// whiteboardFileLink links target from a page in pageDir: a relative path
// when pageDir lies inside root, so the link survives moving the checkout,
// and an absolute file: URL otherwise (an embed_dir outside the repository,
// or another volume).
func whiteboardFileLink(root, pageDir, target string) string {
	if inside, err := filepath.Rel(root, pageDir); err == nil && inside != ".." && !strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		if rel, err := filepath.Rel(pageDir, target); err == nil {
			return filepath.ToSlash(rel)
		}
	}
	p := filepath.ToSlash(target)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // a Windows drive path: file:///C:/...
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// whiteboardItemFileLink resolves a file path an item's markdown link or
// image names, for a page written to pageDir: a relative path is taken from
// the session root, and a bare absolute path is used as
// is; either then follows the panel ticket-row rule of whiteboardFileLink.
func whiteboardItemFileLink(root, pageDir string) func(string) string {
	return func(path string) string {
		path = filepath.FromSlash(path)
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		return whiteboardFileLink(root, pageDir, path)
	}
}

// mailboxPanel shows the session's mailbox. The full page carries the own
// address, the unread count, whether a wait is armed, the correspondents,
// and the history ring; the embed carries one line, the address and the
// unread count. A session with no named address and no history has no panel.
func mailboxPanel(in whiteboardPanelInput) (wswhiteboard.Panel, bool) {
	named := ""
	if in.mailbox.owner {
		named = in.mailbox.identity.address()
	}
	history, _ := readMailboxHistory(in.sessionKey)
	if named == "" && len(history) == 0 {
		return wswhiteboard.Panel{}, false
	}
	address := named
	if address == "" {
		address = wsmailbox.ReplyIDPrefix + in.replyID
	}

	unread := 0
	if named != "" {
		if path, err := wsmailbox.PathForScope(in.mailbox.identity.Scope, in.root); err == nil {
			if store, err := wsmailbox.Load(path); err == nil {
				unread += len(store.Queues[in.mailbox.identity.Name])
			}
		}
	}
	var entry wsmailbox.ReplyEntry
	if path, err := wsmailbox.ReplyRegistryPath(); err == nil {
		if store, err := wsmailbox.LoadReplyStore(path); err == nil {
			unread += len(store.Queues[in.replyID])
			entry = store.Entries[in.replyID]
		}
	}
	panel := wswhiteboard.Panel{ID: "mailbox", Title: wsi18n.T(in.lang, "panel.mailbox"), Icon: "mail", Count: wsi18n.T(in.lang, "panel.mail.unread", unread)}
	status := wsi18n.T(in.lang, "panel.mail.unread", unread)
	if in.layout == wswhiteboard.LayoutEmbed {
		panel.Rows = []wswhiteboard.PanelRow{{Label: address, Text: status}}
		return panel, true
	}

	if whiteboardMailboxListening(in.sessionKey) {
		status = wsi18n.T(in.lang, "panel.mail.unread.wait", unread)
	}
	panel.Rows = append(panel.Rows, wswhiteboard.PanelRow{Label: address, Text: status})
	for _, c := range wsmailbox.SortedContacts(entry.Contacts) {
		panel.Rows = append(panel.Rows, wswhiteboard.PanelRow{Label: c.Address, Text: wsi18n.T(in.lang, "panel.mail.exchange", c.LastExchange)})
	}
	for _, r := range history {
		label := wsi18n.T(in.lang, "panel.mail.recv", r.Time, r.Counterparty)
		if r.Direction == mailboxHistorySent {
			label = wsi18n.T(in.lang, "panel.mail.sent", r.Time, r.Counterparty)
		}
		panel.Rows = append(panel.Rows, wswhiteboard.PanelRow{Label: label, Text: r.Title})
	}
	return panel, true
}

// whiteboardMailboxListening reports an armed mailbox wait: a listening
// marker whose process still runs. A wait that crashed leaves its marker
// behind, and that is not waiting.
func whiteboardMailboxListening(sessionKey string) bool {
	marker, ok, err := wsmailbox.ReadListeningMarker(sessionKey)
	if err != nil || !ok {
		return false
	}
	return wsstate.ProcessAlive(marker.PID)
}
