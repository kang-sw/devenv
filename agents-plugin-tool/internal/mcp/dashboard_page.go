package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
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
// board image inside the root is the image route's URL, relayed through the
// host page into a topic body's sandboxed frame, and path text otherwise),
// the mail
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
// follows ReadSession's found/absent/transient result. A session without a board
// still reads, with HasBoard false, since the dashboard lists it for its live
// snapshot.
func (src *DashboardSessionSource) ReadPage(reg wsdashboard.Registration) (wsdashboard.PageContent, bool, error) {
	return src.ReadPageLang(reg, "")
}

// ReadPageLang is ReadPage with the board's and panels' own text (panel
// titles, counts, tooltips) in lang, a wsi18n code. The text of the board's
// items, the panel rows' own titles, and everything else the session wrote
// stays as written.
func (src *DashboardSessionSource) ReadPageLang(reg wsdashboard.Registration, lang string) (wsdashboard.PageContent, bool, error) {
	key, _, record, ok, err := src.lookupRecord(reg)
	if !ok || err != nil {
		return wsdashboard.PageContent{}, false, err
	}
	page := wsdashboard.PageContent{
		Contacts:   src.readContacts(reg),
		MailBodies: map[string]string{},
	}
	if history, err := readMailboxHistory("", key); err == nil {
		for _, r := range history {
			page.MailBodies[r.Name] = wslive.ScrubSessionKeys(r.Body, key)
		}
	}
	if record.Whiteboard == nil {
		return page, true, nil
	}
	page.HasBoard = true
	page.Seq = record.Whiteboard.Seq
	page.Reasked = record.Whiteboard.ReaskTimes()
	page.BoardTargets = wsdashboard.BoardNavigationTargets(record.Whiteboard)
	board, ok := src.renderBoard(key, record, reg.ReplyID, lang, page.Reasked)
	if !ok {
		return wsdashboard.PageContent{}, false, fmt.Errorf("render session board failed")
	}
	page.Board = board.regions
	page.BoardUnits = board.units
	page.Watch = slices.Clone(board.watch)
	page.WatchPIDs = slices.Clone(board.pids)
	return page, true, nil
}

// boardRender is one render of a session's board and panels, with what the
// stream path reads beside the regions: the files the render named
// (PageContent.Watch) and the processes whose liveness it showed
// (PageContent.WatchPIDs).
type boardRender struct {
	regions       wswhiteboard.Regions
	units         *wsdashboard.BoardUnits
	watch         []string
	externalWatch []string
	imagePaths    []string
	imageRevision uint64
	pids          []int
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
	// topicRenders is an opt-in test probe; production retains no per-ID counters.
	topicRenders map[string]int
}

type boardMemoEntry struct {
	base  string
	stamp string
	// stampOK is false when the stamp was taken over another watch list
	// than this render's: the next read renders again.
	stampOK bool
	board   boardRender
	shared  string
	topics  map[string]boardTopicMemo
}

type boardTopicMemo struct {
	revision string
	unit     wswhiteboard.TopicUnit
	watch    []string
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
	mail := sessionMailPrint(replies, repliesOK, replyID, key)
	base := boardMemoBase(key, record, lang, reasked, mail)
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
	shared := boardMemoSharedBase(key, record, lang, reasked, mail)
	if had {
		shared += boardWatchStamp(prev.board.externalWatch)
	}
	topics := map[string]boardTopicMemo{}
	images := prev.board.imagePaths
	if !had || prev.board.imageRevision != record.boardRevision {
		images = servedImagePaths(record)
	}
	board, ok := src.renderBoardTopics(key, record, replyID, lang, images, func(out *boardRender) wswhiteboard.TopicReuse {
		return func(id, revision string, render func() (wswhiteboard.TopicUnit, error)) (wswhiteboard.TopicUnit, error) {
			if old, found := prev.topics[id]; found && prev.shared == shared && old.revision == revision {
				topics[id] = old
				out.watch = append(out.watch, old.watch...)
				out.externalWatch = append(out.externalWatch, old.watch...)
				return old.unit, nil
			}
			start := len(out.externalWatch)
			src.boards.mu.Lock()
			if src.boards.topicRenders != nil {
				src.boards.topicRenders[id]++
			}
			src.boards.mu.Unlock()
			unit, err := render()
			if err == nil {
				topics[id] = boardTopicMemo{revision: revision, unit: unit, watch: slices.Clone(out.externalWatch[start:])}
			}
			return unit, err
		}
	})
	if !ok {
		return board, false
	}
	// The stamp was taken before this render, over the previous render's
	// watch list, so a file changed during the render still misses next
	// time; it is this render's stamp only when the list did not change.
	// A first render has none, and the next read renders once more.
	entry := boardMemoEntry{base: base, board: board, stamp: stamp, stampOK: had && slices.Equal(prev.board.watch, board.watch), shared: shared, topics: topics}
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
	fmt.Fprint(h, boardMemoSharedBase(key, record, lang, reasked, replies))
	fmt.Fprintf(h, "seq\x00%d\x00", record.Whiteboard.Seq)
	// Folder metadata, logs, and panel versions are authoritative inputs too;
	// equal render sequences must not hide changed on-disk content. The folder
	// cache assigns a fresh identity whenever its authoritative read changes.
	fmt.Fprintf(h, "folder\x00%d\x00", record.boardRevision)
	return hex.EncodeToString(h.Sum(nil))
}

// Shared inputs deliberately exclude the board sequence and folder identities:
// a commit can rewrite every topic's metadata without changing its view.
func boardMemoSharedBase(key string, record sessionRecord, lang string, reasked map[int]int64, replies string) string {
	h := sha256.New()
	fmt.Fprintf(h, "lang\x00%s\x00replies\x00%s\x00", lang, replies)
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
	return src.renderBoardTopics(key, record, replyID, lang, servedImagePaths(record), nil)
}

func (src *DashboardSessionSource) renderBoardTopics(key string, record sessionRecord, replyID, lang string, images []string, reuse func(*boardRender) wswhiteboard.TopicReuse) (boardRender, bool) {
	src.boards.mu.Lock()
	src.boards.renders++
	src.boards.mu.Unlock()
	out := boardRender{imagePaths: images, imageRevision: record.boardRevision}
	// BuildView only reads the state: the served page never begins a render
	// or writes a file, so the stored board and the static page are untouched.
	// The ticket files the panels list, the ticket status directories the
	// topic badges resolve against, and the file images a topic body shows
	// change without a nudge, so the page names them (PageContent.Watch).
	watch := func(path string) {
		out.watch = append(out.watch, path)
		out.externalWatch = append(out.externalWatch, path)
	}
	for _, path := range images {
		watch(path)
	}
	folder := whiteboardFolder(record.Root, key).Dir
	watchWhiteboardFiles(folder, func(path string) { out.watch = append(out.watch, path) })
	in := servedViewInput(record, replyID)
	in.Tickets = &whiteboardTickets{root: record.Root, watch: watch}
	docImageSrc := in.DocImageSrc
	in.DocImageSrc = func(path string) string {
		watch(docImagePath(record.Root, path))
		return docImageSrc(path)
	}
	in.Panels = servedWhiteboardPanels(record, key, replyID, lang, watch, func() ([]mailboxHistoryRecord, error) {
		return readMailboxHistory("", key)
	}, src.loadReplies)
	// An armed wait shows as long as its process runs, and a process can
	// die leaving its marker: no watched file reports that, so the page
	// names the process.
	if marker, ok, _ := wsmailbox.ReadListeningMarker(key); ok {
		out.pids = []int{marker.PID}
	}
	view := wswhiteboard.BuildView(record.Whiteboard, in)
	view.Lang = lang
	var topicReuse wswhiteboard.TopicReuse
	if reuse != nil {
		topicReuse = reuse(&out)
	}
	regions, units, err := wswhiteboard.RenderRegionsWithTopics(view, topicReuse)
	if err != nil {
		appendDebugEvent("dashboard.page_render_failed", map[string]any{"error": err.Error()})
		return boardRender{}, false
	}
	// Lazy history has no image URLs in the initial board to change. The
	// client uses this deterministic stamp to refresh retained image frames,
	// including after reconnecting to a replacement daemon.
	marker := template.HTML(fmt.Sprintf(`<template data-history-images="%s"></template>`, boardWatchStamp(images)))
	regions.Board += marker
	if units != nil {
		units.Board = regions.Board
		units.Shell += marker
	}
	out.regions = regions
	out.units = wsdashboard.NewBoardUnits(units)
	return out, true
}

// BoardItems reads the session reg names' board status for the daemon's
// answer check (wsdashboard.BoardItemsSource): the IDs of the open items and
// each re-asked item's latest re-ask time. It reads the stored state and
// renders nothing. A session without a board answers an empty value and
// true; false when the session is gone.
func (src *DashboardSessionSource) BoardItems(reg wsdashboard.Registration) (wsdashboard.BoardItems, bool) {
	_, _, record, ok, err := src.lookupRecord(reg)
	if !ok || err != nil {
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
	_, _, record, ok, err := src.lookupRecord(reg)
	if !ok || err != nil || record.Whiteboard == nil {
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

var _ wsdashboard.TopicHistorySource = (*DashboardSessionSource)(nil)

// ReadTopicHistory renders the named topics, or the one topic holding
// target, with their complete history (wswhiteboard.RenderTopicFragments)
// under the served page's link rule: what the served board's lazy history
// leaves out, fetched when the viewer expands it. false when the session is
// gone or has no board.
func (src *DashboardSessionSource) ReadTopicHistory(reg wsdashboard.Registration, lang string, names []string, target string) ([]wswhiteboard.TopicFragment, bool) {
	_, _, record, ok, err := src.lookupRecord(reg)
	if !ok || err != nil || record.Whiteboard == nil {
		return nil, false
	}
	if target != "" {
		name, found := record.Whiteboard.TargetTopic(target)
		if !found {
			return nil, true
		}
		names = []string{name}
	}
	view := wswhiteboard.BuildView(record.Whiteboard, servedViewInput(record, reg.ReplyID))
	view.Lang = lang
	topics, err := wswhiteboard.RenderTopicFragments(view, names)
	if err != nil {
		appendDebugEvent("dashboard.page_render_failed", map[string]any{"error": err.Error()})
		return nil, false
	}
	return topics, true
}

var _ wsdashboard.OutsideLinkSource = (*DashboardSessionSource)(nil)

// ReadOutsideLinks lists the files outside the session root that the board
// of the session reg names links (wsdashboard.OutsideLinkSource): it renders
// the board, and the closed topics the board leaves past its cap, under the
// served page's link rule with a recorder on it, so the list holds the paths
// the page's own links carry. It runs on a click, not on a page render, and
// inlines no image. false when the session is gone.
func (src *DashboardSessionSource) ReadOutsideLinks(reg wsdashboard.Registration) ([]string, bool) {
	_, _, record, ok, err := src.lookupRecord(reg)
	if !ok || err != nil {
		return nil, false
	}
	if record.Whiteboard == nil {
		return nil, true
	}
	var links []string
	in := servedViewInput(record, reg.ReplyID)
	in.DocImageSrc = nil
	// Every link counts, including those in history the page fetches later.
	in.LazyHistory = false
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
		// The image's version (&v=, which the route ignores) changes the
		// rendered page when the file changes, so an edited image reaches a
		// page that already shows it: a frame remounts and relays it again.
		ImageSrc: func(path string) string {
			src := wsdashboard.ImageSrc(record.Root, replyID, path)
			if src == "" {
				return ""
			}
			if info, err := os.Stat(docImagePath(record.Root, path)); err == nil {
				src += "&v=" + wsdashboard.ImageVersion(info)
			}
			return src
		},
		// A topic body renders in a sandboxed frame that sends no cookie, so
		// its file images are never inlined: an empty src relays ImageSrc's
		// cookie-gated image URL through the same-origin host page
		// (wswhiteboard fileTarget, framedDoc's data-relay allowlist).
		DocImageSrc: func(string) string { return "" },
		FileActions: wsdashboard.FileActions(record.Root, replyID),
		// An item field's absolute image path outside the root loads, for a
		// loopback viewer, through the image route; a topic body's frame asks
		// the page to fetch it there (wswhiteboard.View OutsideImage).
		OutsideImage: wsdashboard.OutsideImage(record.Root, replyID),
		Tickets:      &whiteboardTickets{root: record.Root},
		// The served board fetches history on demand (ReadTopicHistory).
		LazyHistory: true,
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

// loadReplies reads the machine reply-id registry from the mailbox store;
// false when it cannot be read.
func (src *DashboardSessionSource) loadReplies() (wsmailbox.ReplyStore, bool) {
	replies, err := wsmailbox.Paths{}.Store().LoadReply()
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

// sessionMailPrint is what the served board and page show of session key's
// mail state (reply-id replyID): its reply-registry print (replyPrint), its
// history window's entry names, and its armed-wait marker. All of it lives
// in the memory-first mailbox store, outside any file a page watches.
func sessionMailPrint(replies wsmailbox.ReplyStore, ok bool, replyID, key string) string {
	var b strings.Builder
	b.WriteString(replyPrint(replies, ok, replyID))
	if key == "" {
		return b.String()
	}
	if history, err := readMailboxHistory("", key); err == nil {
		b.WriteString("\x00history")
		for _, r := range history {
			b.WriteString("\x00" + r.Name)
		}
	}
	if marker, ok, _ := wsmailbox.ReadListeningMarker(key); ok {
		fmt.Fprintf(&b, "\x00marker\x00%d", marker.PID)
	}
	return b.String()
}

// MailPrint is the session's mail print (sessionMailPrint, over every
// session key the reply-id maps to), which the daemon's refresh stamp
// folds: the store's queue hints renew it, and no file a page watches
// changes with mail (wsdashboard.MailPrintSource).
func (src *DashboardSessionSource) MailPrint(reg wsdashboard.Registration) (string, bool) {
	replies, ok := src.loadReplies()
	src.mu.Lock()
	keys := slices.Clone(src.byReply[reg.ReplyID])
	src.mu.Unlock()
	if len(keys) == 0 {
		return sessionMailPrint(replies, ok, reg.ReplyID, ""), true
	}
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(sessionMailPrint(replies, ok, reg.ReplyID, key))
		b.WriteString("\x01")
	}
	return b.String(), true
}

// contactsFrom is readContacts over a reply registry already loaded. The
// local stores map reads each scope's named store once.
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
			if store, err := (wsmailbox.Paths{}).Store().Load(path); err == nil {
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
					c.PresenceUntil = seen.Add(mailboxLivenessThreshold)
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
					c.PresencePID = p.PID
					if seen, err := time.Parse(time.RFC3339, p.LastSeen); err == nil {
						c.PresenceUntil = seen.Add(mailboxLivenessThreshold)
					}
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
