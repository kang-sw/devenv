package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// live_snapshot.go is ws-mcp's side of the dashboard's live layer: the
// adapter-only intake tool, the per-session snapshot writer (debounced, with
// a heartbeat), the conversation ring writes, and the mail entries the
// mailbox send and receive path adds to that ring. The schema, the reducer,
// and the ring files live in internal/wslive, which the daemon reads with.
//
// Both stores sit in the session's dashboard registry directory, beside its
// registration (wsdashboard.SessionDir), so the daemon finds them from the
// reply-id alone and the registry's pruning removes them with the session.
// Only a top-level lead session is registered, so only its stores are
// written; a child session's activity reaches the dashboard as part of its
// lead's subagent tree.
//
// Nothing here may fail or delay an intake call or a mailbox tool: the
// snapshot is written from a timer goroutine, ring and snapshot write errors
// are logged as debug events only, and each write sends the registry's
// fire-and-forget nudge (dashboardNudge). With the session's
// dashboard.enabled off, nothing is written at all.
//
// A server the dashboard daemon hosts (ServerOptions.Live) hands each
// snapshot to the daemon in memory instead (wsdashboard.HostedSlot): the
// debounced fire that writes the file on the direct path publishes the
// snapshot, and the file is written only for recovery, on its own clock: at
// a fire or heartbeat once a heartbeat interval passed since the last file
// write and the state changed since it, at a turn's end, and once more when
// the server closes (liveHub.close), which also releases the daemon's copy.
// A hosted heartbeat with nothing changed writes nothing and sends no
// nudge; its publish only moves the copy's updatedAt, so the session never
// reads as stale. A ring append signals the slot instead of nudging. A
// session whose directory the daemon does not serve (another cache home)
// stays on the direct path.
//
// The snapshot's repo fields (the session root's folder, branch, and
// upstream standing, and working-tree counts) are read here, not sent by the
// adapter: a few git reads, cached per session and refreshed at most once per
// heartbeat interval. The refresh runs on the timer goroutine, outside
// writeMu, so an intake call never waits on git; the snapshot write that
// finds the cache missing or a heartbeat interval old does wait (up to
// repoReadTimeout): a session's first snapshot, and at most one write per
// heartbeat interval after it, change or heartbeat alike, and only when a
// tool batch ended since the last read (wslive.State.BatchesEnded): the
// working tree changes through tool calls, so an idle session runs no git.

const dashboardIntakeTool = "dashboard.intake"

// liveSeams are the writer's timings and file writers (test seams).
// debounceUrgent applies to a change of the lead's phase or streamed reply,
// debounceAgent to a batch whose every change is a subagent's own activity
// (agentOnlyEvent), and debounce to every other change; changes inside one
// window coalesce into one write, and an earlier pending write is never
// pushed back. Timer goroutines read the seams, so they sit behind a lock.
type liveSeams struct {
	debounceUrgent time.Duration
	debounce       time.Duration
	debounceAgent  time.Duration
	heartbeat      time.Duration
	writeSnapshot  func(path string, data []byte) error
	appendRing     func(dir string, entries []wslive.RingEntry) error
	readRepo       func(root string) wslive.Repo
	// now is the clock of the snapshots' updatedAt and the hosted flush
	// clock.
	now func() time.Time
}

var (
	liveSeamMu sync.RWMutex
	liveSeam   = liveSeams{
		debounceUrgent: 100 * time.Millisecond,
		debounce:       250 * time.Millisecond,
		debounceAgent:  time.Second,
		heartbeat:      wslive.HeartbeatInterval,
		writeSnapshot:  writeWhiteboardFile,
		appendRing:     wslive.AppendRing,
		readRepo:       readSessionRepo,
		now:            time.Now,
	}
)

func currentLiveSeams() liveSeams {
	liveSeamMu.RLock()
	defer liveSeamMu.RUnlock()
	return liveSeam
}

// setLiveSeams changes the seams under the lock and returns the previous
// value.
func setLiveSeams(change func(*liveSeams)) liveSeams {
	liveSeamMu.Lock()
	defer liveSeamMu.Unlock()
	previous := liveSeam
	change(&liveSeam)
	return previous
}

// liveHub holds the live state of every session this process received
// intake for. Its zero value is ready.
type liveHub struct {
	mu       sync.Mutex
	sessions map[string]*liveSession

	// enabledMu guards enabled, each session's last resolved
	// dashboard.enabled (liveDashboardEnabled).
	enabledMu sync.Mutex
	enabled   map[string]liveEnabled
	// resolveEnabled resolves one session's dashboard.enabled; nil uses
	// Server.dashboardSessionResolve (test seam).
	resolveEnabled func(sessionKey string) (bool, error)

	// host is the daemon hosting this server (ServerOptions.Live); nil on
	// the direct path.
	host wsdashboard.LiveHost
}

// liveEnabled is one session's cached dashboard.enabled and when it was
// resolved.
type liveEnabled struct {
	on bool
	at time.Time
}

// forgetEnabled drops a session's cached dashboard.enabled, so its next
// live write resolves the key again (after a config.tune).
func (h *liveHub) forgetEnabled(sessionKey string) {
	h.enabledMu.Lock()
	defer h.enabledMu.Unlock()
	delete(h.enabled, sessionKey)
}

type liveSession struct {
	// dir is the session's registry directory; nudge tells the daemon a
	// store in it changed; active reports whether the session may still be
	// written to dir (its dashboard key on, its directory unchanged).
	dir     string
	root    string
	replyID string
	nudge   func()
	active  func() bool
	// onStop removes the stopped session from its hub.
	onStop func()
	// debug records into the owning server's debug ring.
	debug func(event string, fields map[string]any)
	// slot is the hosting daemon's in-memory hand-off of this session's
	// snapshot; nil on the direct path, which writes the file and nudges at
	// every fire. Set at creation, never changed.
	slot wsdashboard.HostedSlot

	mu    sync.Mutex
	state *wslive.State
	timer *time.Timer
	due   time.Time
	// stopped is set once the session went inactive; its timer never arms
	// again and the hub replaces it on the next intake.
	stopped bool
	// The hosted flush clock: dirty reports a change since the last file
	// write, flushNow a turn end since it, and lastFlush is its time.
	dirty, flushNow bool
	lastFlush       time.Time

	// writeMu serializes snapshot writes, so a slow write is never
	// overtaken by an older marshal.
	writeMu sync.Mutex
	// ringMu is taken before mu is released, so concurrent intake calls
	// write their ring entries in the order their events applied.
	ringMu sync.Mutex

	// repoMu guards the cached repo read; it is taken before writeMu and
	// held across a refresh, so two writes never read git twice.
	repoMu sync.Mutex
	repo   *wslive.Repo
	repoAt time.Time
	// repoEnds is the state's BatchesEnded at the last read.
	repoEnds uint64
}

func (h *liveHub) session(key, dir, root string, nudge func(), active func() bool, debug func(string, map[string]any)) *liveSession {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sessions == nil {
		h.sessions = map[string]*liveSession{}
	}
	ls, ok := h.sessions[key]
	if ok && ls.dir == dir && !ls.isStopped() {
		return ls
	}
	if ok {
		ls.stop()
	}
	state := wslive.NewState(key)
	ls = &liveSession{dir: dir, root: root, replyID: state.ReplyID(), nudge: nudge, active: active, debug: debug, state: state}
	if h.host != nil {
		if slot, ok := h.host.OpenHostedSlot(dir); ok {
			ls.slot = slot
		}
	}
	created := ls
	ls.onStop = func() { h.drop(key, created) }
	h.sessions[key] = ls
	return ls
}

// existing is the session's live state, nil when this process has none.
func (h *liveHub) existing(key string) *liveSession {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessions[key]
}

// drop removes ls from the hub unless a newer session already replaced it.
func (h *liveHub) drop(key string, ls *liveSession) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sessions[key] == ls {
		delete(h.sessions, key)
	}
}

func (ls *liveSession) isStopped() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.stopped
}

// stop ends the session's writes: its pending timer is cancelled and never
// re-armed, and a hosted session's daemon copy is released (its file
// renders again).
func (ls *liveSession) stop() {
	ls.halt()
	if ls.slot != nil {
		ls.slot.Close()
	}
}

// halt marks the session stopped and cancels its timer; it reports whether
// the session was running.
func (ls *liveSession) halt() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	was := !ls.stopped
	ls.stopped = true
	ls.due = time.Time{}
	if ls.timer != nil {
		ls.timer.Stop()
	}
	return was
}

// close ends every session of the hub when its server closes
// (Server.Close): a hosted session writes its snapshot file once more, so a
// later reader (a restarted daemon, or the daemon once the copy is gone)
// starts from the last state, and releases the daemon's copy.
func (h *liveHub) close() {
	h.mu.Lock()
	sessions := make([]*liveSession, 0, len(h.sessions))
	for _, ls := range h.sessions {
		sessions = append(sessions, ls)
	}
	h.sessions = nil
	h.mu.Unlock()
	for _, ls := range sessions {
		ls.finish()
	}
}

// finish stops the session; a hosted one flushes its snapshot file first
// and then releases its slot.
func (ls *liveSession) finish() {
	if !ls.halt() || ls.slot == nil {
		return
	}
	ls.repoMu.Lock()
	var repo *wslive.Repo
	if ls.repo != nil {
		r := *ls.repo
		repo = &r
	}
	ls.repoMu.Unlock()
	ls.writeMu.Lock()
	ls.mu.Lock()
	snap := ls.state.Snapshot(currentLiveSeams().now().UnixMilli())
	ls.mu.Unlock()
	snap.Repo = repo
	data, err := json.Marshal(snap)
	if err == nil {
		err = writeLiveSnapshot(ls.dir, data)
	}
	ls.writeMu.Unlock()
	if err != nil {
		ls.debug("dashboard.snapshot_write_error", map[string]any{"error": err.Error()})
	}
	ls.slot.Close()
}

// liveSessionDir returns the registry directory that holds the live stores
// of the session behind sessionKey, or false when nothing may be written for
// it: not a top-level lead session, its dashboard.enabled off, or no daemon
// state directory.
func (s *Server) liveSessionDir(sessionKey string) (string, bool) {
	entry, found := s.sessions.lookup(sessionKey)
	if !found || !entry.isTopLevelLead() || !s.liveDashboardEnabled(sessionKey) {
		return "", false
	}
	dir, err := s.dashboard.stateDir()
	if err != nil {
		return "", false
	}
	return wsdashboard.SessionDir(dir, wsmailbox.ReplyID(sessionKey)), true
}

// liveSessionActive reports whether the live session behind sessionKey may
// still write to dir: liveSessionDir's answer for a timer, which reads the
// session record without touching it (a timer's read is not activity). A
// record that exists but cannot be read right now (Windows: a reader meeting
// the record's replace in flight) keeps the session running: stopping it on
// a transient failure drops the turn state until the next intake, and a mail
// line sent meanwhile finds no tool row to flush.
func (s *Server) liveSessionActive(sessionKey, dir string) bool {
	entry, found, transient := s.sessions.inspect(sessionKey)
	if transient {
		return true
	}
	if !found || !entry.isTopLevelLead() || !s.liveDashboardEnabled(sessionKey) {
		return false
	}
	current, err := s.dashboard.stateDir()
	if err != nil {
		return true
	}
	return wsdashboard.SessionDir(current, wsmailbox.ReplyID(sessionKey)) == dir
}

// liveDashboardEnabled is the session's dashboard.enabled for the live
// layer, which asks on every intake call (about ten a second while a reply
// streams) and every snapshot write. A resolution is trusted for
// dashboardRecheck, and config.tune drops it (forgetEnabled). A failed
// resolve keeps the last known value: reading it as off would stop the
// session, and the next intake would rebuild an empty wslive.State, losing
// the turn start, the subagent tree, and the usage figures. A session never
// resolved is off until a resolve succeeds.
func (s *Server) liveDashboardEnabled(sessionKey string) bool {
	h := &s.live
	h.enabledMu.Lock()
	cached, known := h.enabled[sessionKey]
	resolve := h.resolveEnabled
	h.enabledMu.Unlock()
	now := time.Now()
	if known && now.Sub(cached.at) < dashboardRecheck {
		return cached.on
	}
	if resolve == nil {
		resolve = s.dashboardSessionResolve
	}
	on, err := resolve(sessionKey)
	if err != nil {
		s.debugEvent("dashboard.enabled_resolve_error", map[string]any{"error": err.Error()})
		if !known {
			return false
		}
		on = cached.on
	}
	h.enabledMu.Lock()
	if h.enabled == nil {
		h.enabled = map[string]liveEnabled{}
	}
	h.enabled[sessionKey] = liveEnabled{on: on, at: now}
	h.enabledMu.Unlock()
	return on
}

func (s *Server) handleDashboardIntake(id json.RawMessage, args map[string]any) response {
	const tool = dashboardIntakeTool
	key, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	entry, found := s.sessions.lookup(key)
	if !found {
		return toolTextResponse(id, "", fmt.Errorf("%s: unknown_session: session key not found", tool))
	}
	adapter, _ := args["adapter"].(string)
	if strings.TrimSpace(adapter) == "" {
		return toolTextResponse(id, "", fmt.Errorf("%s: adapter is required", tool))
	}
	rawEvents, ok := args["events"].([]any)
	if !ok {
		return toolTextResponse(id, "", fmt.Errorf("%s: events must be an array of event objects", tool))
	}
	dir, ok := s.liveSessionDir(key)
	if !ok {
		return toolTextResponse(id, "dashboard off: nothing recorded\n", nil)
	}

	ls := s.live.session(key, dir, entry.root, func() { s.dashboardNudge(key) }, func() bool { return s.liveSessionActive(key, dir) }, s.debugEvent)
	now := currentLiveSeams().now().UnixMilli()
	var (
		ring     []wslive.RingEntry
		problems []string
		applied  int
		changed  bool
		urgent   bool
		// agentOnly stays true while every change is a subagent's own.
		agentOnly bool
	)
	ls.mu.Lock()
	changed = ls.state.SetAdapter(adapter)
	agentOnly = !changed
	for i, raw := range rawEvents {
		ev, derr := decodeLiveEvent(raw)
		if derr == nil {
			var res wslive.Result
			res, derr = ls.state.Apply(ev, now)
			if derr == nil {
				applied++
				changed = changed || res.Changed
				urgent = urgent || res.Urgent
				if res.Changed || len(res.Ring) > 0 {
					agentOnly = agentOnly && agentOnlyEvent(ev)
				}
				if ev.Kind == wslive.KindTurnEnd {
					// A flush point of the hosted file (liveSession.fire).
					ls.flushNow = true
				}
				ring = append(ring, res.Ring...)
				continue
			}
		}
		problems = append(problems, fmt.Sprintf("event %d: %v", i, derr))
	}
	if changed {
		seams := currentLiveSeams()
		delay := seams.debounce
		switch {
		case urgent:
			delay = seams.debounceUrgent
		case agentOnly:
			delay = seams.debounceAgent
		}
		ls.dirty = true
		ls.scheduleLocked(delay)
	}
	ls.ringMu.Lock()
	ls.mu.Unlock()
	if len(ring) > 0 {
		s.appendLiveRing(key, ls.dir, ls.slot, ring)
	}
	ls.ringMu.Unlock()

	var b strings.Builder
	fmt.Fprintf(&b, "ok: %d of %d events applied\n", applied, len(rawEvents))
	b.WriteString("accepts: " + strings.Join(wslive.OptionalKinds, ", ") + "\n")
	for _, p := range problems {
		b.WriteString("skipped " + p + "\n")
	}
	return toolTextResponse(id, b.String(), nil)
}

// agentOnlyEvent reports an event that moves only a subagent's own row (and
// the subagent counts the page derives from the rows): an agent event that
// names an agent. Its snapshot write may wait debounceAgent. A permission
// prompt, a lead-level agent event (the lead's batches), a status event, and
// every other kind go out at the usual latency.
func agentOnlyEvent(ev wslive.Event) bool {
	return ev.Kind == wslive.KindAgent && ev.AgentID != ""
}

func decodeLiveEvent(raw any) (wslive.Event, error) {
	var ev wslive.Event
	if _, ok := raw.(map[string]any); !ok {
		return ev, errors.New("not an object")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return ev, err
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		return ev, fmt.Errorf("malformed event: %v", err)
	}
	return ev, nil
}

// scheduleLocked arranges a snapshot write within delay, keeping an earlier
// pending deadline. ls.mu is held.
func (ls *liveSession) scheduleLocked(delay time.Duration) {
	if ls.stopped {
		return
	}
	due := time.Now().Add(delay)
	if !ls.due.IsZero() && !due.Before(ls.due) {
		return
	}
	ls.due = due
	if ls.timer == nil {
		ls.timer = time.AfterFunc(delay, ls.fire)
		return
	}
	ls.timer.Reset(delay)
}

// fire hands the snapshot over, and arms the heartbeat unless a change is
// already pending. On the direct path it writes the snapshot file and
// nudges, every time; a hosted session publishes it to its slot and writes
// the file only at a flush point (the flush clock in the file comment). A
// session that went inactive (its dashboard key turned off, or its
// directory moved) is stopped instead, so the heartbeat neither writes for a
// key-off session nor keeps a session that no intake drives reading live
// forever: the next intake call starts it afresh.
func (ls *liveSession) fire() {
	if !ls.active() {
		ls.stop()
		ls.onStop()
		return
	}
	seams := currentLiveSeams()
	now := seams.now()
	ls.mu.Lock()
	ends := ls.state.BatchesEnded()
	ls.mu.Unlock()
	repo, repoChanged := ls.currentRepo(now, ends)
	ls.writeMu.Lock()
	ls.mu.Lock()
	ls.due = time.Time{}
	if ls.stopped {
		// Closed meanwhile (finish wrote the last state).
		ls.mu.Unlock()
		ls.writeMu.Unlock()
		return
	}
	snap := ls.state.Snapshot(now.UnixMilli())
	write := true
	if ls.slot != nil {
		ls.dirty = ls.dirty || repoChanged
		write = ls.flushNow || (ls.dirty && now.Sub(ls.lastFlush) >= seams.heartbeat)
		if write {
			ls.dirty, ls.flushNow, ls.lastFlush = false, false, now
		}
	}
	ls.mu.Unlock()
	snap.Repo = repo
	if ls.slot != nil {
		ls.slot.Publish(snap)
	}
	var err error
	if write {
		var data []byte
		data, err = json.Marshal(snap)
		if err == nil {
			err = writeLiveSnapshot(ls.dir, data)
		}
	}
	ls.writeMu.Unlock()
	switch {
	case err != nil:
		ls.debug("dashboard.snapshot_write_error", map[string]any{"error": err.Error()})
		if ls.slot != nil {
			// Not on disk yet: the next fire or heartbeat tries again.
			ls.mu.Lock()
			ls.dirty = true
			ls.mu.Unlock()
		}
	case write && ls.slot == nil:
		ls.nudge()
	}
	ls.mu.Lock()
	if ls.due.IsZero() {
		ls.scheduleLocked(seams.heartbeat)
	}
	ls.mu.Unlock()
}

// currentRepo is the session root's cached repo read at now, and whether
// this call read it afresh with a different result. It reads when it never
// read, or when its read is a heartbeat interval old and a tool batch ended
// since it (ends, the state's BatchesEnded, moved).
func (ls *liveSession) currentRepo(now time.Time, ends uint64) (*wslive.Repo, bool) {
	ls.repoMu.Lock()
	defer ls.repoMu.Unlock()
	seams := currentLiveSeams()
	changed := false
	if ls.repo == nil || (now.Sub(ls.repoAt) >= seams.heartbeat && ends != ls.repoEnds) {
		repo := wslive.CleanRepo(seams.readRepo(ls.root))
		changed = ls.repo == nil || *ls.repo != repo
		ls.repo, ls.repoAt, ls.repoEnds = &repo, now, ends
	}
	repo := *ls.repo
	return &repo, changed
}

// repoReadTimeout bounds one session's git reads.
const repoReadTimeout = 5 * time.Second

// readSessionRepo reads root's folder, branch standing, and working-tree
// counts; a root that is not a repository has the folder alone. A failed
// working-tree read leaves the counts zero, so the page shows no change chip
// while the branch and sync still show.
func readSessionRepo(root string) wslive.Repo {
	repo := wslive.Repo{Folder: filepath.Base(root)}
	if root == "" {
		repo.Folder = ""
		return repo
	}
	ctx, cancel := context.WithTimeout(context.Background(), repoReadTimeout)
	defer cancel()
	st, ok := wsgit.BranchSync(ctx, nil, root)
	if !ok {
		return repo
	}
	repo.Branch, repo.Upstream, repo.Ahead, repo.Behind = st.Head, st.Upstream, st.Ahead, st.Behind
	if st.Head == "" {
		repo.Branch, repo.Detached = st.OID, true
	}
	if c, ok := wsgit.WorkingTreeCounts(ctx, nil, root); ok {
		repo.Modified, repo.Staged, repo.Untracked = c.Modified, c.Staged, c.Untracked
	}
	return repo
}

func writeLiveSnapshot(dir string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create live snapshot dir: %w", err)
	}
	return currentLiveSeams().writeSnapshot(wslive.SnapshotPath(dir), data)
}

// appendLiveRing writes entries to the conversation ring in the session
// directory dir and tells the daemon: through slot, the session's hosted
// hand-off, or with a nudge when it is nil. A failed write is logged, never
// surfaced.
func (s *Server) appendLiveRing(sessionKey, dir string, slot wsdashboard.HostedSlot, entries []wslive.RingEntry) {
	if err := currentLiveSeams().appendRing(wslive.RingDir(dir), entries); err != nil {
		s.debugEvent("dashboard.ring_write_error", map[string]any{"error": err.Error()})
		return
	}
	if slot != nil {
		slot.RingAppended()
		return
	}
	s.dashboardNudge(sessionKey)
}

// recordLiveMail adds a mail line to the session's conversation ring for
// each mailbox history entry just written, while the session's dashboard is
// on and it is a top-level lead. keys are those entries' history file names,
// in order; an entry whose history write failed has no key and its line no
// expansion.
func (s *Server) recordLiveMail(sessionKey string, entries []mailboxHistoryEntry, keys []string) {
	if len(entries) == 0 {
		return
	}
	dir, ok := s.liveSessionDir(sessionKey)
	if !ok {
		return
	}
	// The lead's tool calls finished since its last conversation entry come
	// first. The state is taken under mu and the ring written under ringMu,
	// as intake does, so the entries land in the order they were made. A
	// session with no live state has no tool calls to flush.
	ls := s.live.existing(sessionKey)
	var ring []wslive.RingEntry
	var slot wsdashboard.HostedSlot
	if ls != nil {
		slot = ls.slot
		ls.mu.Lock()
		ring = ls.state.FlushTools()
		if len(ring) > 0 {
			// The snapshot's open gap just emptied.
			ls.dirty = true
			ls.scheduleLocked(currentLiveSeams().debounce)
		}
		ls.ringMu.Lock()
		ls.mu.Unlock()
		defer ls.ringMu.Unlock()
	}
	for i, e := range entries {
		// The ring reaches the browser: scrub the body before the title cap.
		title := mailboxHistoryTitle(wslive.ScrubSessionKeys(e.Body, sessionKey))
		line := wslive.RingEntry{Kind: wslive.RingMail, Direction: e.Direction, Peer: e.Counterparty, Title: title}
		if i < len(keys) {
			line.HistoryKey = keys[i]
		}
		ring = append(ring, line)
	}
	s.appendLiveRing(sessionKey, dir, slot, ring)
}
