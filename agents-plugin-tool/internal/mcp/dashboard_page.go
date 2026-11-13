package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// The dashboard daemon's session page, read through DashboardSessionSource:
// the board and its panels rendered with the static page's templates under
// the served-page link rule (a file link is an action link for the page's
// link menu, opening the file view inside the session's project root; a
// board image inside the root is the image route's URL, inlined as a data:
// URL in a topic body's sandboxed frame, and path text otherwise), the mail
// correspondents, and the mailbox history bodies.
// Everything here reaches a browser, so no returned field may carry the
// session key.

var (
	_ wsdashboard.LangPageSource   = (*DashboardSessionSource)(nil)
	_ wsdashboard.ContactSource    = (*DashboardSessionSource)(nil)
	_ wsdashboard.MailPrintSource  = (*DashboardSessionSource)(nil)
	_ wsdashboard.BoardItemsSource = (*DashboardSessionSource)(nil)
)

// ReadPage reads the page parts of the session reg names, in English. It
// reports false exactly when ReadSession does. A session without a board
// still reads, with HasBoard false, since the dashboard lists it for its live
// snapshot.
func (src *DashboardSessionSource) ReadPage(reg wsdashboard.Registration) (wsdashboard.PageContent, bool) {
	return src.ReadPageLang(reg, "")
}

// ReadPageLang is ReadPage with the board's and panels' own text (panel
// titles, counts, tooltips) in lang, a wsi18n code. The text of the board's
// items, the panel rows' own titles, and everything else the session wrote
// stays as written.
func (src *DashboardSessionSource) ReadPageLang(reg wsdashboard.Registration, lang string) (wsdashboard.PageContent, bool) {
	key, _, record, ok := src.lookupRecord(reg)
	if !ok {
		return wsdashboard.PageContent{}, false
	}
	page := wsdashboard.PageContent{
		Contacts:   src.readContacts(reg),
		MailBodies: map[string]string{},
	}
	if history, err := src.history.read(reg.ReplyID, key); err == nil {
		for _, r := range history {
			page.MailBodies[r.Name] = wslive.ScrubSessionKeys(r.Body, key)
		}
	}
	if record.Whiteboard == nil {
		return page, true
	}
	page.HasBoard = true
	page.Seq = record.Whiteboard.Seq
	page.Reasked = record.Whiteboard.ReaskTimes()
	board, ok := src.renderBoard(key, record, reg.ReplyID, lang, page.Reasked)
	if ok {
		page.Board = board.regions
		page.Watch = slices.Clone(board.watch)
		page.WatchPIDs = slices.Clone(board.pids)
	}
	return page, true
}

// boardRender is one render of a session's board and panels, with what the
// stream path reads beside the regions: the files the render named
// (PageContent.Watch) and the processes whose liveness it showed
// (PageContent.WatchPIDs).
type boardRender struct {
	regions wswhiteboard.Regions
	watch   []string
	pids    []int
}

// boardMemo holds each session's latest board render per language, so a
// page re-read that changed nothing the board shows (a live-snapshot change
// re-renders only the live regions) reuses it. An entry is used only while
// its key, taken before the render read anything, still matches
// (boardMemoBase) and every file the render named still stamps the same
// (wsdashboard.StampWatched).
type boardMemo struct {
	mu      sync.Mutex
	entries map[string]boardMemoEntry
	// renders counts the board renders run (a test probe).
	renders int
}

type boardMemoEntry struct {
	base  string
	stamp string
	// stampOK is false when the stamp was taken over another watch list
	// than this render's: the next read renders again.
	stampOK bool
	board   boardRender
}

// forget drops the renders of session replyID, in every language.
func (m *boardMemo) forget(replyID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.entries {
		if strings.HasPrefix(k, replyID+"\x00") {
			delete(m.entries, k)
		}
	}
}

// boardMemoMax bounds the entries; a new key past it starts the memo over.
const boardMemoMax = 64

// renderBoard renders the board of record (session key key, reply-id
// replyID) in lang, or reuses the memoized render when none of its inputs
// changed; false when the render failed.
//
// The inputs are every input to the board render other than the live
// snapshot (boardMemoBase): an input found later joins the key. With dev
// assets on, the templates themselves change between renders, so nothing
// is memoized.
func (src *DashboardSessionSource) renderBoard(key string, record sessionRecord, replyID, lang string, reasked map[int]int64) (boardRender, bool) {
	if wswhiteboard.DevAssetsOn() {
		return src.renderBoardNow(key, record, replyID, lang)
	}
	memoKey := replyID + "\x00" + lang
	replies, repliesOK := src.loadReplies()
	base := boardMemoBase(key, record, lang, reasked, replyPrint(replies, repliesOK, replyID))
	m := &src.boards
	m.mu.Lock()
	prev, had := m.entries[memoKey]
	m.mu.Unlock()
	var stamp string
	if had {
		stamp = boardWatchStamp(prev.board.watch)
		if prev.base == base && prev.stampOK && stamp == prev.stamp {
			return prev.board, true
		}
	}
	board, ok := src.renderBoardNow(key, record, replyID, lang)
	if !ok {
		return board, false
	}
	// The stamp was taken before this render, over the previous render's
	// watch list, so a file changed during the render still misses next
	// time; it is this render's stamp only when the list did not change.
	// A first render has none, and the next read renders once more.
	entry := boardMemoEntry{base: base, board: board, stamp: stamp, stampOK: had && slices.Equal(prev.board.watch, board.watch)}
	m.mu.Lock()
	if _, replaces := m.entries[memoKey]; m.entries == nil || !replaces && len(m.entries) >= boardMemoMax {
		m.entries = map[string]boardMemoEntry{}
	}
	m.entries[memoKey] = entry
	m.mu.Unlock()
	return board, true
}

// boardMemoBase fingerprints the board render's inputs that are not files
// it names: the board's update sequence (every store write bumps it), the
// todo and agenda contents the panels read (they sit outside the board
// state, and Seq covers them only when the board's refresh after their
// write succeeds), the language, the mailbox listening marker's presence,
// process, and that process's liveness (an armed wait shows only while it
// runs), the re-ask times, and replies, the session's reply-registry print
// (replyPrint). The mailbox owner and identity are not in it: the served
// page never sets them (servedWhiteboardPanels).
func boardMemoBase(key string, record sessionRecord, lang string, reasked map[int]int64, replies string) string {
	h := sha256.New()
	fmt.Fprintf(h, "seq\x00%d\x00lang\x00%s\x00replies\x00%s\x00", record.Whiteboard.Seq, lang, replies)
	// Folder metadata, logs, and panel versions are authoritative inputs too;
	// equal render sequences must not hide changed on-disk content.
	board, _ := json.Marshal(record.Whiteboard)
	h.Write(board)
	todos, _ := json.Marshal(record.Todos)
	agenda, _ := json.Marshal(record.Agenda)
	asked, _ := json.Marshal(reasked)
	fmt.Fprintf(h, "todos\x00%s\x00agenda\x00%s\x00reasked\x00%s\x00", todos, agenda, asked)
	if marker, ok, _ := wsmailbox.ReadListeningMarker(key); ok {
		fmt.Fprintf(h, "marker\x00%d\x00%t\x00", marker.PID, markerAlive(marker.PID))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// markerAlive reports whether a listening marker's process runs (a test
// seam).
var markerAlive = wsstate.ProcessAlive

// boardWatchStamp stamps every file a board render named
// (wsdashboard.StampWatched: a ticket directory by its listing, any other
// path by its stat).
func boardWatchStamp(watch []string) string {
	h := sha256.New()
	wsdashboard.StampWatched(h, watch)
	return hex.EncodeToString(h.Sum(nil))
}

// renderBoardNow renders the board and panels of record under the served
// page's link rule.
func (src *DashboardSessionSource) renderBoardNow(key string, record sessionRecord, replyID, lang string) (boardRender, bool) {
	src.boards.mu.Lock()
	src.boards.renders++
	src.boards.mu.Unlock()
	var out boardRender
	// BuildView only reads the state: the served page never begins a render
	// or writes a file, so the stored board and the static page are untouched.
	// The ticket files the panels list, the ticket status directories the
	// topic badges resolve against, and the images a topic body inlines
	// change without a nudge, so the page names them (PageContent.Watch).
	watch := func(path string) { out.watch = append(out.watch, path) }
	folder := whiteboardFolder(record.Root, key).Dir
	watchWhiteboardFiles(folder, watch)
	in := servedViewInput(record, replyID)
	in.Tickets = &whiteboardTickets{root: record.Root, watch: watch}
	inline := in.DocImageSrc
	in.DocImageSrc = func(path string) string {
		watch(docImagePath(record.Root, path))
		return inline(path)
	}
	in.Panels = servedWhiteboardPanels(record, key, replyID, lang, watch, func() ([]mailboxHistoryRecord, error) {
		return src.history.read(replyID, key)
	}, src.loadReplies)
	// An armed wait shows as long as its process runs, and a process can
	// die leaving its marker: no watched file reports that, so the page
	// names the process.
	if marker, ok, _ := wsmailbox.ReadListeningMarker(key); ok {
		out.pids = []int{marker.PID}
	}
	view := wswhiteboard.BuildView(record.Whiteboard, in)
	view.Lang = lang
	regions, err := wswhiteboard.RenderRegions(view)
	if err != nil {
		appendDebugEvent("dashboard.page_render_failed", map[string]any{"error": err.Error()})
		return boardRender{}, false
	}
	out.regions = regions
	return out, true
}

// BoardItems reads the session reg names' board status for the daemon's
// answer check (wsdashboard.BoardItemsSource): the IDs of the open items and
// each re-asked item's latest re-ask time. It reads the stored state and
// renders nothing. A session without a board answers an empty value and
// true; false when the session is gone.
func (src *DashboardSessionSource) BoardItems(reg wsdashboard.Registration) (wsdashboard.BoardItems, bool) {
	_, _, record, ok := src.lookupRecord(reg)
	if !ok {
		return wsdashboard.BoardItems{}, false
	}
	if record.Whiteboard == nil {
		return wsdashboard.BoardItems{}, true
	}
	items := wsdashboard.BoardItems{Open: map[int]bool{}, Reasked: record.Whiteboard.ReaskTimes()}
	for _, item := range record.Whiteboard.Items {
		if item.Status == wswhiteboard.StatusOpen {
			items.Open[item.ID] = true
		}
	}
	return items, true
}

var _ wsdashboard.ClosedTopicsSource = (*DashboardSessionSource)(nil)

// ReadClosedTopics renders n of the closed topics the served board leaves
// past its cap, from index from, under the served page's link rule; false
// when the session is gone or has no board.
func (src *DashboardSessionSource) ReadClosedTopics(reg wsdashboard.Registration, lang string, from, n int) (template.HTML, bool) {
	_, _, record, ok := src.lookupRecord(reg)
	if !ok || record.Whiteboard == nil {
		return "", false
	}
	in := servedViewInput(record, reg.ReplyID)
	in.Closed = &wswhiteboard.ClosedWindow{From: from, N: n}
	view := wswhiteboard.BuildView(record.Whiteboard, in)
	view.Lang = lang
	html, err := wswhiteboard.RenderClosedTopics(view)
	if err != nil {
		appendDebugEvent("dashboard.page_render_failed", map[string]any{"error": err.Error()})
		return "", false
	}
	return html, true
}

var _ wsdashboard.OutsideLinkSource = (*DashboardSessionSource)(nil)

// ReadOutsideLinks lists the files outside the session root that the board
// of the session reg names links (wsdashboard.OutsideLinkSource): it renders
// the board, and the closed topics the board leaves past its cap, under the
// served page's link rule with a recorder on it, so the list holds the paths
// the page's own links carry. It runs on a click, not on a page render, and
// inlines no image. false when the session is gone.
func (src *DashboardSessionSource) ReadOutsideLinks(reg wsdashboard.Registration) ([]string, bool) {
	_, _, record, ok := src.lookupRecord(reg)
	if !ok {
		return nil, false
	}
	if record.Whiteboard == nil {
		return nil, true
	}
	var links []string
	in := servedViewInput(record, reg.ReplyID)
	in.DocImageSrc = nil
	in.FileActions, in.OutsideImage = wsdashboard.RecordOutsideLinks(in.FileActions, in.OutsideImage, func(abs string) { links = append(links, abs) })
	if _, err := wswhiteboard.RenderRegions(wswhiteboard.BuildView(record.Whiteboard, in)); err != nil {
		appendDebugEvent("dashboard.page_render_failed", map[string]any{"error": err.Error()})
	}
	in.Closed = &wswhiteboard.ClosedWindow{From: wswhiteboard.ClosedCap, N: len(record.Whiteboard.ClosedTopics)}
	if _, err := wswhiteboard.RenderClosedTopics(wswhiteboard.BuildView(record.Whiteboard, in)); err != nil {
		appendDebugEvent("dashboard.page_render_failed", map[string]any{"error": err.Error()})
	}
	return links, true
}

// servedViewInput is the served page's view input for a session record,
// without its panels: a full layout under the served page's link rule.
func servedViewInput(record sessionRecord, replyID string) wswhiteboard.ViewInput {
	return wswhiteboard.ViewInput{
		Layout:   wswhiteboard.LayoutFull,
		ReplyID:  replyID,
		FileText: servedFileText(record.Root),
		ImageSrc: func(path string) string {
			return wsdashboard.ImageSrc(record.Root, replyID, path)
		},
		// A topic body renders in a sandboxed frame that sends no cookie, so
		// its images are inlined rather than loaded through the image route.
		DocImageSrc: func(path string) string {
			return wsdashboard.ImageDataURL(record.Root, path)
		},
		FileActions: wsdashboard.FileActions(record.Root, replyID),
		// An item field's absolute image path outside the root loads, for a
		// loopback viewer, through the image route; a topic body's frame asks
		// the page to fetch it there (wswhiteboard.View OutsideImage).
		OutsideImage: wsdashboard.OutsideImage(record.Root, replyID),
		Tickets:      &whiteboardTickets{root: record.Root},
	}
}

// docImagePath is the file a topic body's image path names under root, as
// the inlining resolves it before its containment check: an absolute path
// as is, a relative one against root.
func docImagePath(root, path string) string {
	native := filepath.FromSlash(path)
	if filepath.IsAbs(native) {
		return filepath.Clean(native)
	}
	return filepath.Join(root, native)
}

// servedWhiteboardPanels builds the full page's panels for the served page.
// The daemon process serves no session itself, so it holds no mailbox
// identity: the mailbox panel shows the session's reply-id address and its
// reply-id unread count, never a named inbox the session may own. watch,
// when not nil, is told each file and directory the panels read that a
// writer may change without a nudge (whiteboardPanelInput.watch). history,
// when not nil, reads the mailbox history (whiteboardPanelInput.history),
// and replies the reply registry (whiteboardPanelInput.replies).
func servedWhiteboardPanels(record sessionRecord, key, replyID, lang string, watch func(path string), history func() ([]mailboxHistoryRecord, error), replies func() (wsmailbox.ReplyStore, bool)) []wswhiteboard.Panel {
	return whiteboardPanels(whiteboardPanelInput{
		rec:        record,
		layout:     wswhiteboard.LayoutFull,
		root:       record.Root,
		sessionKey: key,
		replyID:    replyID,
		served:     true,
		lang:       lang,
		watch:      watch,
		history:    history,
		replies:    replies,
	})
}

// ReadContacts re-reads the contacts of the session reg names, as ReadPage
// reads them (wsdashboard.ContactSource).
func (src *DashboardSessionSource) ReadContacts(reg wsdashboard.Registration) ([]wsdashboard.Contact, bool) {
	return src.readContacts(reg), true
}

// servedFileText shows a file path an item's markdown names as text on a
// served page, taking a relative path from the session root the way
// whiteboardItemFileLink does: a relative path is shown cleaned, an absolute
// path inside root relative to root, and any other absolute path as is, all
// with forward slashes.
func servedFileText(root string) func(string) string {
	return func(path string) string {
		native := filepath.FromSlash(path)
		if !filepath.IsAbs(native) {
			return filepath.ToSlash(filepath.Clean(native))
		}
		if rel, err := filepath.Rel(root, native); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(rel)
		}
		return filepath.ToSlash(native)
	}
}

// readContacts resolves the session's mail correspondents, newest exchange
// first, reading the reply registry and each named scope's store once. An
// id: address resolves to its own reply-id and is live while its registry
// entry was seen within mailboxLivenessThreshold (a reply entry carries no
// pid, so only that half of mailboxPresenceLive applies); a named address
// resolves through its presence record's owner and is live per
// mailboxPresenceLive. Its Cwd is the registry entry's or presence record's,
// else the contact record's. An unreadable store leaves the fields it would
// fill empty.
func (src *DashboardSessionSource) readContacts(reg wsdashboard.Registration) []wsdashboard.Contact {
	replies, ok := src.loadReplies()
	if !ok {
		return nil
	}
	return src.contactsFrom(reg, replies)
}

// loadReplies loads the machine reply-id registry through the source's
// cache; false when it cannot be read. The store is shared: the caller only
// reads it.
func (src *DashboardSessionSource) loadReplies() (wsmailbox.ReplyStore, bool) {
	path, err := wsmailbox.ReplyRegistryPath()
	if err != nil {
		return wsmailbox.ReplyStore{}, false
	}
	replies, err := src.replies.Load(path, wsmailbox.LoadReplyStore)
	if err != nil {
		return wsmailbox.ReplyStore{}, false
	}
	return replies, true
}

// replyPrint is what the served board and page show of session replyID's
// reply-registry entry: its contacts and its reply-id unread count, never
// the rest of the entry (the heartbeat-driven LastSeen would move it on
// every refresh) or another session's entry. ok false prints that the
// registry did not read.
func replyPrint(replies wsmailbox.ReplyStore, ok bool, replyID string) string {
	if !ok {
		return "-"
	}
	contacts, _ := json.Marshal(replies.Entries[replyID].Contacts)
	return fmt.Sprintf("%s\x00%d", contacts, len(replies.Queues[replyID]))
}

// MailPrint is the session's reply-registry print (replyPrint), which the
// daemon's refresh stamp folds, since mail to a reply-id sends no nudge
// (wsdashboard.MailPrintSource).
func (src *DashboardSessionSource) MailPrint(reg wsdashboard.Registration) (string, bool) {
	replies, ok := src.loadReplies()
	return replyPrint(replies, ok, reg.ReplyID), true
}

// contactsFrom is readContacts over a reply registry already loaded.
func (src *DashboardSessionSource) contactsFrom(reg wsdashboard.Registration, replies wsmailbox.ReplyStore) []wsdashboard.Contact {
	sorted := wsmailbox.SortedContacts(replies.Entries[reg.ReplyID].Contacts)
	if len(sorted) == 0 {
		return nil
	}
	now := src.clock()
	stores := map[wsmailbox.Scope]*wsmailbox.StoreFile{}
	scopeStore := func(scope wsmailbox.Scope) *wsmailbox.StoreFile {
		if store, seen := stores[scope]; seen {
			return store
		}
		stores[scope] = nil
		if path, err := wsmailbox.PathForScope(scope, reg.Project); err == nil {
			if store, err := src.scopes.Load(path, wsmailbox.Load); err == nil {
				stores[scope] = &store
			}
		}
		return stores[scope]
	}
	out := make([]wsdashboard.Contact, 0, len(sorted))
	for _, rec := range sorted {
		c := wsdashboard.Contact{Address: rec.Address, Cwd: rec.Cwd}
		if at, err := time.Parse(time.RFC3339, rec.LastExchange); err == nil {
			c.LastExchange = at
		}
		if id, isReply := strings.CutPrefix(rec.Address, wsmailbox.ReplyIDPrefix); isReply {
			c.ReplyID = id
			if entry, ok := replies.Entries[id]; ok {
				if seen, err := time.Parse(time.RFC3339, entry.LastSeen); err == nil {
					c.PresenceLive = now.Sub(seen) < mailboxLivenessThreshold
				}
				if entry.Cwd != "" {
					c.Cwd = entry.Cwd
				}
			}
		} else if name, scope, err := wsmailbox.ParseSlugScope(rec.Address); err == nil {
			if store := scopeStore(scope); store != nil {
				if p, ok := store.Presence[name]; ok {
					if p.Owner != "" {
						c.ReplyID = wsmailbox.ReplyID(p.Owner)
					}
					c.PresenceLive = mailboxPresenceLive(p, now)
					if p.Cwd != "" {
						c.Cwd = p.Cwd
					}
				}
			}
		}
		out = append(out, c)
	}
	return out
}
