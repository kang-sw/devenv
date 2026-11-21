// Page readers assemble session and index data for rendering.

package wsdashboard

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// pageReader reads what the pages render from the index and disk. It holds
// no state of its own: every read re-reads, so a replacement daemon renders
// the same pages from the same files. (The streams share their renders in
// memory, and the daemon's caches serve unchanged files, which only skips
// reads; stream_cache.go, entry_files.go, file_cache.go.)
type pageReader struct {
	// lang is the viewer's language, passed to a LangPageSource.
	lang    string
	dir     string
	index   *Index
	source  SessionSource
	version string
	now     func() time.Time
	// entries holds the ring and drained entries already read (nil reads
	// every entry each time).
	entries *entryFiles
	// snapshots holds the snapshots already parsed (nil reads each time).
	snapshots *SnapshotCache
	// Initial document/control reads must observe a preceding write immediately.
	// Stream renders leave this false and share the presentation validation TTL.
	freshSnapshots bool
	// foreign is the daemon's shared look at the inputs refresh stamps cannot
	// learn of from a wake (foreign_schedule.go); nil looks every time.
	foreign *foreignSchedule
	// layout is the rail's stored nesting, read from memory for a head's
	// parent jump when the registration names no owner (nil: none).
	layout *railLayoutStore
	// agents supplies in-memory hosted/child settings metadata when present.
	agents AgentHost
	// cardBound reports subordinate card-bound sessions whose Record.Parent is nil.
	cardBound func(replyID string) bool
}

func (r pageReader) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

// readSnapshot reads a session's live snapshot; nil when it has none. The
// snapshot may be shared with other readers (SnapshotCache): the caller
// only reads it.
func (r pageReader) readSnapshot(replyID string) *wslive.Snapshot {
	if r.freshSnapshots {
		r.snapshots.Invalidate(r.dir, replyID)
	}
	return r.snapshots.Snapshot(r.dir, replyID)
}

// readSnapshotWithoutObserved serves consumers that do not render model history.
func (r pageReader) readSnapshotWithoutObserved(replyID string) *wslive.Snapshot {
	return r.snapshots.snapshotWithoutObserved(r.dir, replyID)
}

// readFreshSnapshot bypasses presentation TTLs for decisions that write to a
// session. Hosted RAM remains authoritative when the daemon owns the session.
func (r pageReader) readFreshSnapshot(replyID string) *wslive.Snapshot {
	return r.snapshots.FreshSnapshot(r.dir, replyID)
}

// Write-route admission keeps its read-through check. Refresh stamps use
// adapterStamp instead, so bounded presentation evidence is not authority.
func (r pageReader) adapterPresent(replyID string, now time.Time) bool {
	return adapterPresent(r.readFreshSnapshot(replyID), now)
}

// adapterStamp ages already-read evidence without probing disk. Include the
// evidence itself, not just its current presence: a render may load a new
// snapshot after the pre-render stamp, and it must not reuse that older stamp
// merely because both the old and new evidence later read as absent.
func (r pageReader) adapterStamp(replyID string, now time.Time) (bool, string) {
	snap := r.snapshots.lastKnown(r.dir, replyID)
	if snap == nil {
		return false, ""
	}
	if adapterPresent(snap, now) {
		return true, "" // a healthy heartbeat alone changes no visible stamp
	}
	return false, fmt.Sprintf("%d/%d/%d/%s", snap.SchemaVersion, snap.UpdatedAt, snap.HeartbeatMs, snap.Adapter)
}

func (r pageReader) readPage(s Session) (PageContent, bool, error) {
	if ls, ok := r.source.(LangPageSource); ok {
		return ls.ReadPageLang(s.Registration, r.lang)
	}
	if ps, ok := r.source.(PageSource); ok {
		return ps.ReadPage(s.Registration)
	}
	return PageContent{HasBoard: s.HasBoard}, true, nil
}

// served reads a session this daemon serves and its page content; false
// when it does not serve that reply-id.
func (r pageReader) served(replyID string) (Session, PageContent, bool) {
	s, page, ok, err := r.servedWithError(replyID)
	return s, page, ok && err == nil
}

func (r pageReader) servedWithError(replyID string) (Session, PageContent, bool, error) {
	if r.index == nil || !ValidReplyID(replyID) {
		return Session{}, PageContent{}, false, nil
	}
	s, ok := r.index.Session(replyID)
	if !ok {
		return Session{}, PageContent{}, false, r.index.readError(replyID)
	}
	page, ok, err := r.readPage(s)
	return s, page, ok, err
}

// lookup resolves served sessions, reading a session's snapshot only when
// a caller asks for it, and once per lookup.
func (r pageReader) lookup() sessionLookup {
	snaps := map[string]*wslive.Snapshot{}
	observed := map[string]*wslive.Snapshot{}
	return sessionLookup{
		session: func(id string) (Session, bool) {
			if r.index == nil {
				return Session{}, false
			}
			return r.index.Session(id)
		},
		snapshot: func(id string) *wslive.Snapshot {
			snap, read := snaps[id]
			if !read {
				snap = r.readSnapshotWithoutObserved(id)
				snaps[id] = snap
			}
			return snap
		},
		piDeleted: func(id string) bool { return piChildDeleted(r.dir, id) },
		observed: func(id string) *wslive.Snapshot {
			snap, read := observed[id]
			if !read {
				snap = r.readSnapshot(id)
				observed[id] = snap
			}
			return snap
		},
	}
}

// parentSource reads session's owners for a head: lookup serves the index
// (recording what the page names), the rail layout's stored nesting stands in
// for a session whose registration names no owner. The layout is read from
// memory (foreign look rules, as the rail's own stamp), and a parent counts
// only while the index serves it.
func (r pageReader) parentSource(lookup func(string) (Session, bool)) parentSource {
	return parentSource{session: lookup, layoutParent: func(id string) string {
		var since time.Time
		if r.foreign != nil {
			since = r.foreign.filesSince()
		}
		return layoutParentOf(r.layout.docSince(since), id, func(p string) bool {
			_, ok := lookup(p)
			return ok
		})
	}}
}

// session builds one session page's view; false when the daemon does not
// serve that reply-id.
func (r pageReader) session(replyID string) (SessionView, bool) {
	view, _, ok := r.readSession(replyID)
	return view, ok
}

// readSession is session with the page's refresh inputs: every session the
// view looked up (replyID first among them, sorted), the files its page
// source named and the attachments directory its conversation reads, and
// whether it shows contacts (live or saved conversation history).
func (r pageReader) readSession(replyID string) (SessionView, refreshInputs, bool) {
	view, inputs, _, ok, err := r.readSessionWith(replyID, nil, true, true)
	return view, inputs, ok && err == nil
}

// sessionReads are the store reads one session page read made, which a
// live render reuses (liveRender): the page source's content (the board
// panels, the contacts), the saved ring, and (only with an adapter) the
// board-answer inbox's held and drained entries.
type sessionReads struct {
	page          PageContent
	conversation  bool
	ring          []wslive.RingRecord
	held, drained []wsinbox.Record
}

// readSessionWith is readSession for a render that may reuse prev, the
// reads of the page's previous render (nil reads everything): the index
// entry and the snapshots are read again, every other store is taken from
// prev, the ring included unless ringChanged, and held/drained unless
// inboxChanged. An inbox prev did not read (no adapter showed then) is read
// when an adapter arrives. It returns the reads it used.
func (r pageReader) readSessionWith(replyID string, prev *sessionReads, ringChanged, inboxChanged bool) (SessionView, refreshInputs, *sessionReads, bool, error) {
	var s Session
	var page PageContent
	var err error
	ok := false
	if prev != nil && r.index != nil && ValidReplyID(replyID) {
		s, ok = r.index.Session(replyID)
		page = prev.page
	} else {
		s, page, ok, err = r.servedWithError(replyID)
	}
	if !ok || err != nil {
		return SessionView{}, refreshInputs{}, nil, false, err
	}
	reads := &sessionReads{page: page}
	in := pageInputs{Session: s, Page: page, Snapshot: r.readSnapshot(replyID), Now: r.clock(), Version: r.version}
	if r.cardBound != nil && r.cardBound(replyID) {
		in.IsChild = true
	}
	if metaHost, ok := r.agents.(AgentSessionMetaHost); ok {
		if meta, found := metaHost.SessionMeta(replyID); found {
			if meta.IsChild {
				in.IsChild = true
			}
			in.ChildCloseKnown, in.ChildClosed, in.ChildActive = meta.IsChild, meta.Closed, meta.Active
			in.Model, in.Effort = meta.Model, meta.Effort
		}
	} else if s.Registration.Agent {
		// Fallback when no live in-memory runner is attached (e.g. unit tests).
		record, hosted := readAgentRecordEnd(SessionDir(r.dir, replyID))
		if hosted {
			if record.Parent != nil {
				in.IsChild = true
			}
			in.Model, in.Effort = record.Model, record.Effort
		}
	}
	if !s.Registration.Hosted && !s.Live {
		record, _ := readAgentRecordEnd(SessionDir(r.dir, replyID))
		in.HandedOff = record.Lead && record.HandedOffAt != ""
		// An ended child's row may predate ParentReplyID; this record is
		// already open, so naming its owner from it costs no read.
		if in.Session.ParentReplyID == "" && record.Parent != nil {
			in.Session.ParentReplyID = record.Parent.ReplyID
		}
		if record.Parent != nil {
			in.IsChild = true
		}
		if in.Model == "" {
			in.Model = record.Model
		}
		if in.Effort == "" {
			in.Effort = record.Effort
		}
	}
	looked := map[string]bool{replyID: true}
	in.Parents = r.parentSource(func(id string) (Session, bool) {
		looked[id] = true
		if r.index == nil {
			return Session{}, false
		}
		return r.index.Session(id)
	})
	inputs := refreshInputs{watch: slices.Clone(page.Watch), pids: slices.Clone(page.WatchPIDs)}
	if s.Registration.Hosted && s.Registration.HostPID > 0 {
		inputs.pids = append(inputs.pids, s.Registration.HostPID)
	}
	present := adapterPresent(in.Snapshot, in.Now)
	if present || !s.Agent {
		inbox := sessionInbox(r.dir, replyID)
		if prev != nil && !ringChanged && (prev.conversation || !s.Agent) {
			reads.ring = prev.ring
		} else {
			reads.ring, _ = r.entries.readRing(replyID, wslive.RingDir(SessionDir(r.dir, replyID)))
		}
		if present {
			reads.conversation = true
			if prev != nil && prev.conversation && !inboxChanged {
				reads.held, reads.drained = prev.held, prev.drained
			} else {
				reads.held, _ = r.entries.readHeld(inbox)
				reads.drained, _ = r.entries.readDrained(replyID, inbox)
			}
		}
		in.Ring, in.Held, in.Drained = reads.ring, reads.held, reads.drained
		in.AttachDir = wsinbox.AttachmentsDir(inbox)
		lookup := r.lookup()
		in.Lookup = sessionLookup{
			session: func(id string) (Session, bool) {
				looked[id] = true
				return lookup.get(id)
			},
			snapshot: lookup.snap,
			observed: lookup.observedSnap,
			piDeleted: func(id string) bool {
				if !ValidReplyID(id) {
					return false
				}
				// Refresh the owner's row when a closed child loses its
				// revival file, even without another runner event.
				dir := SessionDir(r.dir, id)
				inputs.watch = append(inputs.watch, filepath.Join(dir, AgentRecordFileName), filepath.Join(dir, "pi", "session.jsonl"))
				return lookup.piDeleted(id)
			},
		}
		inputs.watch = append(inputs.watch, in.AttachDir)
		inputs.contacts = len(page.Contacts) > 0
	}
	view := buildSessionView(in)
	for id := range looked {
		if id != replyID {
			inputs.ids = append(inputs.ids, id)
		}
	}
	slices.Sort(inputs.ids)
	inputs.ids = append([]string{replyID}, inputs.ids...)
	slices.Sort(inputs.watch)
	inputs.watch = slices.Compact(inputs.watch)
	slices.Sort(inputs.pids)
	inputs.pids = slices.Compact(inputs.pids)
	return view, inputs, reads, true, nil
}

// sessionStamp is the refresh stamp of replyID's session page over the
// inputs of its last read (stream_cache.go). Each call folds, from memory:
// for each session it looked up, whether the index holds it, its revision,
// its liveness, and whether its snapshot names a present adapter, all at
// call time; the liveness of each watched process; and each contact's
// liveness at call time (Contact.liveAt) over the contacts last read. The
// foreign inputs come from the shared foreign look (foreign_schedule.go):
// the state of the watched paths (StampWatched) and the session's mailbox
// stamp (mailStamp). ok is false when the page shows contacts the source
// cannot re-read.
func (r pageReader) sessionStamp(replyID string, in refreshInputs) (string, bool) {
	if r.index == nil {
		return "", false
	}
	now := r.clock()
	h := newStampHash()
	for _, id := range in.ids {
		s, ok := r.index.Session(id)
		present, evidence := r.adapterStamp(id, now)
		fmt.Fprintf(h, "%s\x00%t\x00%d\x00%t\x00%t\x00%s\x01", id, ok, s.Rev, ok && s.Registration.Live(now), ok && present, evidence)
	}
	for _, pid := range in.pids {
		fmt.Fprintf(h, "pid\x00%d\x00%t\x01", pid, processAlive(pid))
	}
	stampWatchedSince(h, in.watch, r.foreign.filesSince())
	key := replyID
	if in.contacts {
		key += "\x00contacts"
	}
	mail, contacts, ok := r.foreign.mailStamp(key, func() (string, []Contact, bool) { return r.mailStamp(replyID, in.contacts) })
	if !ok {
		return "", false
	}
	io.WriteString(h, mail)
	// A contact's lapse follows the clock and its process, so it is folded
	// every tick; a recovery needs a store read, at the next mailbox look.
	for _, c := range contacts {
		fmt.Fprintf(h, "live\x00%t\x01", c.liveAt(now))
	}
	return sumHex(h), true
}

// mailStamp reads the mailbox side of replyID's refresh stamp: the session's
// mail print (MailPrintSource), since mail to a reply-id sends no wake, and,
// when the page shows contacts, the contacts as re-read now, which it also
// returns. ok is false when the page shows contacts the source cannot
// re-read.
func (r pageReader) mailStamp(replyID string, contacts bool) (string, []Contact, bool) {
	var b strings.Builder
	var list []Contact
	if src, ok := r.source.(MailPrintSource); ok {
		if s, ok := r.index.Session(replyID); ok {
			if print, ok := src.MailPrint(s.Registration); ok {
				fmt.Fprintf(&b, "mail\x00%s\x01", print)
			}
		}
	}
	if contacts {
		src, ok := r.source.(ContactSource)
		if !ok {
			return "", nil, false
		}
		s, ok := r.index.Session(replyID)
		if !ok {
			return "", nil, false
		}
		if list, ok = src.ReadContacts(s.Registration); !ok {
			return "", nil, false
		}
		for _, c := range list {
			fmt.Fprintf(&b, "%s\x00%s\x00%t\x00%s\x00%d\x01", c.Address, c.ReplyID, c.PresenceLive, c.Cwd, c.LastExchange.UnixNano())
		}
	}
	return b.String(), list, true
}

// indexStamp is the refresh stamp of the sessions index: for each listed
// session, its revision, its liveness, whether its snapshot names a present
// adapter, and whether it is inside the ended-session window, all at call
// time. Everything else the index shows comes from the session records,
// which the revision covers.
func (r pageReader) indexStamp() (string, bool) {
	if r.index == nil {
		return "", true
	}
	now := r.clock()
	h := newStampHash()
	for _, s := range r.index.Sessions() {
		if s.Unlisted {
			continue
		}
		reg := s.Registration
		present, evidence := r.adapterStamp(reg.ReplyID, now)
		fmt.Fprintf(h, "%s\x00%d\x00%t\x00%t\x00%t\x00%s\x01", reg.ReplyID, s.Rev, reg.Live(now),
			present, now.Sub(reg.LastSeen()) <= endedSessionWindow, evidence)
	}
	return sumHex(h), true
}

// history reads one page of a served session's older conversation (see
// wslive.ReadRingPage) and builds its entries as the live view builds them.
// It reads whenever the session page itself is served, adapter present or
// not; false when the daemon does not serve that reply-id.
func (r pageReader) history(replyID, before string, n int) (ConversationView, wslive.RingPage, bool, error) {
	s, page, ok := r.served(replyID)
	if !ok {
		return ConversationView{}, wslive.RingPage{}, false, nil
	}
	ring, err := wslive.ReadRingPage(wslive.RingDir(SessionDir(r.dir, replyID)), before, n)
	if err != nil {
		return ConversationView{}, wslive.RingPage{}, true, err
	}
	reg := s.Registration
	contacts := resolvedContacts(page.Contacts, reg.ReplyID)
	cv := conversation(wslive.Lead{}, ring.Records, page.MailBodies, contacts, r.lookup(), replyMarkdown(reg.Project, reg.ReplyID), reg.ReplyID)
	splitUserEntries(cv.Entries, wsinbox.AttachmentsDir(sessionInbox(r.dir, replyID)), reg.ReplyID)
	attributeBoardNavigation(&cv, page.BoardTargets)
	return cv, ring, true, nil
}

// indexView builds the / view. It reads no session page: what it shows of
// a board (whether there is one, its open policy and check items) comes with the
// session from the index (SessionData.CountPolicy).
func (r pageReader) indexView() IndexView {
	var inputs []indexInput
	if r.index != nil {
		for _, s := range r.index.Sessions() {
			if s.Unlisted {
				continue
			}
			inputs = append(inputs, indexInput{Session: s, Snapshot: r.readSnapshotWithoutObserved(s.ReplyID)})
		}
	}
	return buildIndexView(inputs, r.clock(), r.version)
}

func (d *Daemon) pages() pageReader {
	return pageReader{dir: d.dir, index: d.index, source: d.source, version: d.endpoint.Version, entries: d.entryFiles, snapshots: d.snapshots, foreign: d.foreign, layout: d.layout, agents: d.agents, cardBound: d.cardBound}
}

// pagesIn is pages reading in the viewer's language.
func (d *Daemon) pagesIn(lang string) pageReader {
	r := d.pages()
	r.lang = lang
	return r
}
