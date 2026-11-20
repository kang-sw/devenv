package mcp

import (
	"errors"
	"fmt"
	"os"
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

// ticketPanel lists ready/ and, on the full page, all todo/ tickets. Each
// card links to its file, or on a served page carries its repo-relative path.
// idea/ is read only by the dashboard's on-demand route, never this live panel.
func ticketPanel(in whiteboardPanelInput) (wswhiteboard.Panel, bool) {
	statuses := []string{"ready", "todo"}
	if in.watch != nil {
		// Named before the read: a status directory that appears later
		// changes the panel too.
		for _, status := range statuses {
			in.watch(filepath.Join(in.root, "ai-docs", "tickets", status))
		}
	}
	tickets, err := whiteboardTicketsList(in.root, wsdoc.TicketListOptions{Statuses: statuses})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return wswhiteboard.Panel{}, false
	}
	if in.watch != nil {
		for _, t := range tickets {
			in.watch(filepath.Join(in.root, filepath.FromSlash(t.Path)))
		}
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
	card := func(t wsdoc.TicketInfo) wswhiteboard.PanelRow {
		title, sub := t.Title, t.Stem
		if title == "" {
			title, sub = t.Stem, "" // the stem is already the title
		}
		row := wswhiteboard.PanelRow{Label: ticketCategory(t.Stem), Text: title, Sub: sub}
		row.Path = filepath.ToSlash(t.Path)
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
	count := wsi18n.T(in.lang, "panel.tickets.count.full", len(ready), len(todo))
	lanes = append(lanes, lane(wsi18n.T(in.lang, "panel.lane.todo"), latestTodoTickets(todo, len(todo)), ""))
	// Keep empty lanes so idea-only boards retain their demand control.
	return wswhiteboard.Panel{ID: "tickets", Title: wsi18n.T(in.lang, "panel.tickets"), Icon: "board", Count: count, Lanes: lanes}, true
}

// whiteboardTickets resolves the ticket stems a topic links (wb:topic
// tickets=) against the session root's ticket board, for the topic bar's
// badges (wswhiteboard.TicketResolver). watch, set on a served page, is
// handed every status directory the first time a stem is resolved: a ticket
// moving between them changes a listing, which invalidates the page's
// memoized render (boardWatchStamp stamps a directory by its listing).
type whiteboardTickets struct {
	root    string
	watch   func(string)
	watched bool
}

func (w *whiteboardTickets) TicketStatus(stem string) (string, bool) {
	if w.watch != nil && !w.watched {
		w.watched = true
		for _, dir := range wsdoc.TicketStatusDirs {
			w.watch(filepath.Join(w.root, filepath.FromSlash(wsdoc.TicketsDir), dir))
		}
	}
	return wsdoc.TicketStatus(w.root, stem)
}

// whiteboardTicketsList lists the tickets a ticket panel shows (a test
// seam: it reads every listed ticket in full).
var whiteboardTicketsList = wsdoc.TicketsList

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

// mailboxPanel shows the session's own address, unread count, whether a wait
// is armed, correspondents, and history ring. A session with no history
// has no panel.
func mailboxPanel(in whiteboardPanelInput) (wswhiteboard.Panel, bool) {
	// The mailbox state is in the memory store, not a watched file: a
	// served page learns of its changes from the mail print
	// (DashboardSessionSource.MailPrint) and the store's queue hints.
	readHistory := in.history
	if readHistory == nil {
		readHistory = func() ([]mailboxHistoryRecord, error) { return readMailboxHistory("", in.sessionKey) }
	}
	history, _ := readHistory()
	if len(history) == 0 {
		return wswhiteboard.Panel{}, false
	}
	address := wsmailbox.ReplyIDPrefix + in.replyID
	unread := 0
	// A served board's memo keys on the part of the reply registry the
	// panel shows (replyPrint), so a change for another session or a
	// heartbeat rebuilds no board.
	var entry wsmailbox.ReplyEntry
	readReplies := in.replies
	if readReplies == nil {
		readReplies = func() (wsmailbox.ReplyStore, bool) {
			store, err := (wsmailbox.Paths{}).Store().LoadReply()
			return store, err == nil
		}
	}
	if store, ok := readReplies(); ok {
		unread += len(store.Queues[in.replyID])
		entry = store.Entries[in.replyID]
	}
	panel := wswhiteboard.Panel{ID: "mailbox", Title: wsi18n.T(in.lang, "panel.mailbox"), Icon: "mail", Count: wsi18n.T(in.lang, "panel.mail.unread", unread)}
	status := wsi18n.T(in.lang, "panel.mail.unread", unread)
	if whiteboardMailboxListening(wsmailbox.Paths{}, in.sessionKey) {
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
func whiteboardMailboxListening(paths wsmailbox.Paths, sessionKey string) bool {
	marker, ok, err := paths.ReadListeningMarker(sessionKey)
	if err != nil || !ok {
		return false
	}
	return wsstate.ProcessAlive(marker.PID)
}
