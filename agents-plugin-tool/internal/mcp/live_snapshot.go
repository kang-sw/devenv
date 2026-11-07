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
// The snapshot's repo fields (the session root's folder, branch, and
// upstream standing) are read here, not sent by the adapter: a few git ref
// reads, cached per session and refreshed at most once per heartbeat
// interval from the timer goroutine, outside writeMu, so neither a change
// snapshot nor an intake call ever waits on git.

const dashboardIntakeTool = "dashboard.intake"

// liveSeams are the writer's timings and file writers (test seams).
// debounceUrgent applies to a change of the lead's phase or streamed reply,
// debounce to every other change; changes inside one window coalesce into one
// write. Timer goroutines read the seams, so they sit behind a lock.
type liveSeams struct {
	debounceUrgent time.Duration
	debounce       time.Duration
	heartbeat      time.Duration
	writeSnapshot  func(path string, data []byte) error
	appendRing     func(dir string, entries []wslive.RingEntry) error
	readRepo       func(root string) wslive.Repo
}

var (
	liveSeamMu sync.RWMutex
	liveSeam   = liveSeams{
		debounceUrgent: 100 * time.Millisecond,
		debounce:       250 * time.Millisecond,
		heartbeat:      wslive.HeartbeatInterval,
		writeSnapshot:  writeWhiteboardFile,
		appendRing:     wslive.AppendRing,
		readRepo:       readSessionRepo,
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

	mu    sync.Mutex
	state *wslive.State
	timer *time.Timer
	due   time.Time
	// stopped is set once the session went inactive; its timer never arms
	// again and the hub replaces it on the next intake.
	stopped bool

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
}

func (h *liveHub) session(key, dir, root string, nudge func(), active func() bool) *liveSession {
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
	ls = &liveSession{dir: dir, root: root, replyID: state.ReplyID(), nudge: nudge, active: active, state: state}
	created := ls
	ls.onStop = func() { h.drop(key, created) }
	h.sessions[key] = ls
	return ls
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
// re-armed.
func (ls *liveSession) stop() {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.stopped = true
	ls.due = time.Time{}
	if ls.timer != nil {
		ls.timer.Stop()
	}
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
		appendDebugEvent("dashboard.enabled_resolve_error", map[string]any{"error": err.Error()})
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

	ls := s.live.session(key, dir, entry.root, func() { s.dashboardNudge(key) }, func() bool {
		current, on := s.liveSessionDir(key)
		return on && current == dir
	})
	now := time.Now().UnixMilli()
	var (
		ring     []wslive.RingEntry
		problems []string
		applied  int
		changed  bool
		urgent   bool
	)
	ls.mu.Lock()
	changed = ls.state.SetAdapter(adapter)
	for i, raw := range rawEvents {
		ev, derr := decodeLiveEvent(raw)
		if derr == nil {
			var res wslive.Result
			res, derr = ls.state.Apply(ev, now)
			if derr == nil {
				applied++
				changed = changed || res.Changed
				urgent = urgent || res.Urgent
				ring = append(ring, res.Ring...)
				continue
			}
		}
		problems = append(problems, fmt.Sprintf("event %d: %v", i, derr))
	}
	if changed {
		seams := currentLiveSeams()
		delay := seams.debounce
		if urgent {
			delay = seams.debounceUrgent
		}
		ls.scheduleLocked(delay)
	}
	ls.ringMu.Lock()
	ls.mu.Unlock()
	if len(ring) > 0 {
		s.appendLiveRing(key, ls.dir, ring)
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

// fire writes the snapshot, nudges, and arms the heartbeat unless a change
// is already pending. A session that went inactive (its dashboard key turned
// off, or its directory moved) is stopped instead, so the heartbeat neither
// writes for a key-off session nor keeps a session that no intake drives
// reading live forever: the next intake call starts it afresh.
func (ls *liveSession) fire() {
	if !ls.active() {
		ls.stop()
		ls.onStop()
		return
	}
	repo := ls.currentRepo()
	ls.writeMu.Lock()
	ls.mu.Lock()
	ls.due = time.Time{}
	snap := ls.state.Snapshot(time.Now().UnixMilli())
	ls.mu.Unlock()
	snap.Repo = repo
	data, err := json.Marshal(snap)
	if err == nil {
		err = writeLiveSnapshot(ls.dir, data)
	}
	ls.writeMu.Unlock()
	if err != nil {
		appendDebugEvent("dashboard.snapshot_write_error", map[string]any{"error": err.Error()})
	} else {
		ls.nudge()
	}
	ls.mu.Lock()
	if ls.due.IsZero() {
		ls.scheduleLocked(currentLiveSeams().heartbeat)
	}
	ls.mu.Unlock()
}

// currentRepo is the session root's cached repo read, refreshed when it is
// a heartbeat interval old or was never read.
func (ls *liveSession) currentRepo() *wslive.Repo {
	ls.repoMu.Lock()
	defer ls.repoMu.Unlock()
	seams := currentLiveSeams()
	if ls.repo == nil || time.Since(ls.repoAt) >= seams.heartbeat {
		repo := wslive.CleanRepo(seams.readRepo(ls.root))
		ls.repo, ls.repoAt = &repo, time.Now()
	}
	repo := *ls.repo
	return &repo
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
// directory dir and nudges; a failed write is logged, never surfaced.
func (s *Server) appendLiveRing(sessionKey, dir string, entries []wslive.RingEntry) {
	if err := currentLiveSeams().appendRing(wslive.RingDir(dir), entries); err != nil {
		appendDebugEvent("dashboard.ring_write_error", map[string]any{"error": err.Error()})
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
	ring := make([]wslive.RingEntry, 0, len(entries))
	for i, e := range entries {
		// The ring reaches the browser: scrub the body before the title cap.
		title := mailboxHistoryTitle(wslive.ScrubSessionKeys(e.Body, sessionKey))
		line := wslive.RingEntry{Kind: wslive.RingMail, Direction: e.Direction, Peer: e.Counterparty, Title: title}
		if i < len(keys) {
			line.HistoryKey = keys[i]
		}
		ring = append(ring, line)
	}
	s.appendLiveRing(sessionKey, dir, ring)
}
