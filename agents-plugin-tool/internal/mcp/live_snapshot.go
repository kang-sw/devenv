package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// live_snapshot.go is ws-mcp's side of the dashboard's live layer: the
// adapter-only intake tool, the per-session snapshot writer (debounced, with
// a heartbeat), the conversation ring writes, and the mail entries the
// mailbox send and receive path adds to that ring. The schema, the reducer,
// and the ring files live in internal/wslive, which the daemon reads with.
//
// Nothing here may fail or delay an intake call or a mailbox tool: the
// snapshot is written from a timer goroutine, ring and snapshot write errors
// are logged as debug events only, and the nudge runs on its own goroutine.
// With the session's dashboard.enabled off, nothing is written at all.

const dashboardIntakeTool = "dashboard.intake"

// liveSeams are the writer's timings and side effects. debounceUrgent
// applies to a change of the lead's phase or streamed reply, debounce to
// every other change; changes inside one window coalesce into one write.
// nudge tells a running dashboard daemon that a session's stores changed
// (nil sends nothing); the daemon's nudge sender installs itself there. It
// runs on its own goroutine, so it may block or fail without touching the
// caller. Timer goroutines read the seams, so they sit behind a lock.
type liveSeams struct {
	debounceUrgent time.Duration
	debounce       time.Duration
	heartbeat      time.Duration
	writeSnapshot  func(path string, data []byte) error
	appendRing     func(dir string, entries []wslive.RingEntry) error
	nudge          func(root, replyID string)
}

var (
	liveSeamMu sync.RWMutex
	liveSeam   = liveSeams{
		debounceUrgent: 100 * time.Millisecond,
		debounce:       250 * time.Millisecond,
		heartbeat:      wslive.HeartbeatInterval,
		writeSnapshot:  writeWhiteboardFile,
		appendRing:     wslive.AppendRing,
	}
)

func currentLiveSeams() liveSeams {
	liveSeamMu.RLock()
	defer liveSeamMu.RUnlock()
	return liveSeam
}

// setLiveSeams changes the seams under the lock and returns the previous
// value, for a caller (the nudge sender, a test) to install its own.
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
}

type liveSession struct {
	root    string
	replyID string

	mu    sync.Mutex
	state *wslive.State
	timer *time.Timer
	due   time.Time

	// writeMu serializes snapshot writes, so a slow write is never
	// overtaken by an older marshal.
	writeMu sync.Mutex
	// ringMu is taken before mu is released, so concurrent intake calls
	// write their ring entries in the order their events applied.
	ringMu sync.Mutex
}

func (h *liveHub) session(key, root string) *liveSession {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sessions == nil {
		h.sessions = map[string]*liveSession{}
	}
	ls, ok := h.sessions[key]
	if !ok || ls.root != root {
		state := wslive.NewState(key)
		ls = &liveSession{root: root, replyID: state.ReplyID(), state: state}
		h.sessions[key] = ls
	}
	return ls
}

// dashboardOn reports whether the session's dashboard.enabled resolves to
// true. An undeclared key (its manifest not loaded) is off.
func (s *Server) dashboardOn(sessionKey string) bool {
	if _, ok := s.declared.lookup(DashboardEnabledKey); !ok {
		return false
	}
	got, err := s.resolveConfigGet(sessionKey, DashboardEnabledKey)
	if err != nil {
		return false
	}
	on, _ := got.Value.(bool)
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
	if !s.dashboardOn(key) {
		return toolTextResponse(id, "dashboard off: nothing recorded\n", nil)
	}

	ls := s.live.session(key, entry.root)
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
		s.appendLiveRing(ls.root, ls.replyID, ring)
	}
	ls.ringMu.Unlock()

	var b strings.Builder
	fmt.Fprintf(&b, "ok: %d of %d events applied\n", applied, len(rawEvents))
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
// is already pending.
func (ls *liveSession) fire() {
	ls.writeMu.Lock()
	ls.mu.Lock()
	ls.due = time.Time{}
	data, err := json.Marshal(ls.state.Snapshot(time.Now().UnixMilli()))
	ls.mu.Unlock()
	if err == nil {
		err = writeLiveSnapshot(ls.root, ls.replyID, data)
	}
	ls.writeMu.Unlock()
	if err != nil {
		appendDebugEvent("dashboard.snapshot_write_error", map[string]any{"error": err.Error()})
	} else {
		sendDashboardNudge(ls.root, ls.replyID)
	}
	ls.mu.Lock()
	if ls.due.IsZero() {
		ls.scheduleLocked(currentLiveSeams().heartbeat)
	}
	ls.mu.Unlock()
}

func writeLiveSnapshot(root, replyID string, data []byte) error {
	if _, err := ensureWhiteboardDir(root); err != nil {
		return err
	}
	dir := wslive.Dir(root, replyID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create live snapshot dir: %w", err)
	}
	return currentLiveSeams().writeSnapshot(filepath.Join(dir, wslive.SnapshotName), data)
}

// appendLiveRing writes entries to the session's conversation ring and
// nudges; a failed write is logged, never surfaced.
func (s *Server) appendLiveRing(root, replyID string, entries []wslive.RingEntry) {
	err := func() error {
		if _, err := ensureWhiteboardDir(root); err != nil {
			return err
		}
		return currentLiveSeams().appendRing(wslive.RingDir(root, replyID), entries)
	}()
	if err != nil {
		appendDebugEvent("dashboard.ring_write_error", map[string]any{"error": err.Error()})
		return
	}
	sendDashboardNudge(root, replyID)
}

// recordLiveMail adds a mail line to the session's conversation ring for
// each mailbox history entry just written, while the session's dashboard is
// on. keys are those entries' history file names, in order; an entry whose
// history write failed has no key and its line no expansion.
func (s *Server) recordLiveMail(sessionKey, root string, entries []mailboxHistoryEntry, keys []string) {
	if len(entries) == 0 || strings.TrimSpace(root) == "" || !s.dashboardOn(sessionKey) {
		return
	}
	ring := make([]wslive.RingEntry, 0, len(entries))
	for i, e := range entries {
		line := wslive.RingEntry{Kind: wslive.RingMail, Direction: e.Direction, Peer: e.Counterparty, Title: mailboxHistoryTitle(e.Body)}
		if i < len(keys) {
			line.HistoryKey = keys[i]
		}
		ring = append(ring, line)
	}
	s.appendLiveRing(root, wsmailbox.ReplyID(sessionKey), ring)
}

// sendDashboardNudge runs the installed nudge sender on its own goroutine;
// a panic there is contained.
func sendDashboardNudge(root, replyID string) {
	nudge := currentLiveSeams().nudge
	if nudge == nil {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		nudge(root, replyID)
	}()
}
