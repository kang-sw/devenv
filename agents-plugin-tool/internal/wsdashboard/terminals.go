package wsdashboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"sync"

	"github.com/kang-sw/devenv/internal/wsholder"
)

// The terminals: shells the dashboard page shows as cards. Each one is the
// PTY child of its own session holder (internal/wsholder), so a shell
// outlives this daemon; the next daemon finds the holder by its descriptor
// and attaches again. The daemon keeps no output of its own: the holder's
// ring is the only buffer, and a viewer that needs earlier output has the
// holder replay it.
//
// A terminal is known by its holder's id. What it is lives in the holder's
// descriptor (terminalSession): the marker that makes the holder a terminal,
// the reply-id of the session page it was opened from, and its working
// folder.
//
// One holder serves one record stream, to the daemon. The terminal fans it
// out to its viewers (one WebSocket each, terminal_routes.go) by sequence:
// a viewer holds the next sequence it needs and takes exactly that record
// from the stream, so a record replayed for another viewer never reaches it
// twice. When a viewer needs a sequence the stream has passed (a late
// joiner, or a page that reconnects), the terminal asks the holder to
// replay from the lowest sequence any viewer still needs.

const (
	// terminalKind marks a holder's session description as a terminal.
	terminalKind = "terminal"
	// terminalBacklogMax bounds the frames queued for one viewer, in bytes.
	// A viewer's backlog is at most a replay of the holder's ring (4 MiB,
	// more once encoded); one that grows past this is not reading, and is
	// dropped to connect again from the sequence it reached.
	terminalBacklogMax = 16 << 20

	terminalDefaultRows, terminalDefaultCols = 14, 80
	terminalMaxRows, terminalMaxCols         = 500, 1000
)

// terminalSession is a terminal holder's session description, the object
// the holder records in its descriptor (wsholder.Descriptor.Session).
type terminalSession struct {
	// Kind is terminalKind.
	Kind string `json:"kind"`
	// ReplyID is the session page the terminal was opened from.
	ReplyID string `json:"reply_id"`
	// Cwd is the folder the shell started in.
	Cwd string `json:"cwd"`
	// Shell is the shell's program, as the daemon picked it.
	Shell string `json:"shell,omitempty"`
	// Label names the holder in the drawer's holder list.
	Label string `json:"label,omitempty"`
}

// TerminalHost is the holder manager as the terminals use it
// (*wsholder.Manager): a HolderHost that also starts a holder, hands out
// its attach connection, and takes another sink for its events.
type TerminalHost interface {
	HolderHost
	Launch(session json.RawMessage) (string, error)
	Client(id string) *wsholder.Client
	AddSink(sink wsholder.Sink)
}

// TerminalInfo is one terminal as the routes answer it.
type TerminalInfo struct {
	ID string `json:"id"`
	// Session is the reply-id of the session page it was opened from.
	Session string `json:"session"`
	Cwd     string `json:"cwd"`
	Shell   string `json:"shell,omitempty"`
	// StartedAt is the holder's start in epoch milliseconds.
	StartedAt int64 `json:"started_at"`
	// State is "running", "exited", or "lost" (the daemon is not attached
	// to the terminal's holder, so the shell cannot be reached).
	State    string `json:"state"`
	ExitCode *int   `json:"exit_code,omitempty"`
	Signal   *int   `json:"signal,omitempty"`
	// RestartSafe is false for a terminal that may end with the daemon's
	// host (wsholder.Descriptor.RestartSafe).
	RestartSafe bool `json:"restart_safe"`
}

// errTerminalLost reports a terminal whose holder the daemon is not
// attached to.
var errTerminalLost = errors.New("the daemon is not attached to the terminal's holder")

// terminals is the daemon's terminal registry and the sink of every
// holder's events.
type terminals struct {
	host TerminalHost
	log  io.Writer

	mu   sync.Mutex
	byID map[string]*terminal
	// plain are the holders known to host something else: their events are
	// dropped without another listing.
	plain map[string]bool
}

func newTerminals(host TerminalHost, log io.Writer) *terminals {
	return &terminals{host: host, log: log, byID: map[string]*terminal{}, plain: map[string]bool{}}
}

// HolderEvent receives every attached holder's events (wsholder.Sink), on
// that holder's reader goroutine; it never blocks on a viewer or calls the
// holder.
func (ts *terminals) HolderEvent(id string, ev wsholder.Event) {
	ts.mu.Lock()
	t, plain := ts.byID[id], ts.plain[id]
	ts.mu.Unlock()
	if t == nil && !plain {
		// A holder this registry has not seen: one a predecessor left
		// running, which the manager attached to on its scan.
		ts.sync()
		ts.mu.Lock()
		t = ts.byID[id]
		ts.mu.Unlock()
	}
	if t != nil {
		t.event(ev)
	}
}

// sync brings the registry in line with the holders the manager knows: a
// terminal holder gains its entry, and an entry whose holder is gone ends,
// which closes its viewers.
func (ts *terminals) sync() {
	listing := ts.host.List()
	var ended []*terminal
	ts.mu.Lock()
	seen := map[string]bool{}
	for _, h := range listing.Holders {
		seen[h.ID] = true
		t := ts.byID[h.ID]
		if t == nil {
			var meta terminalSession
			if json.Unmarshal(h.Session, &meta) != nil || meta.Kind != terminalKind {
				ts.plain[h.ID] = true
				continue
			}
			t = &terminal{id: h.ID, meta: meta, started: h.StartedAt, host: ts.host, viewers: map[*termViewer]struct{}{}}
			ts.byID[h.ID] = t
		}
		t.mu.Lock()
		t.attached, t.safe = h.Attached, h.RestartSafe
		if h.Child != nil && h.Child.State == "exited" && !t.exited {
			t.exited, t.exitCode, t.signal = true, h.Child.ExitCode, h.Child.Signal
		}
		t.mu.Unlock()
	}
	for id, t := range ts.byID {
		if !seen[id] {
			delete(ts.byID, id)
			ended = append(ended, t)
		}
	}
	for id := range ts.plain {
		if !seen[id] {
			delete(ts.plain, id)
		}
	}
	ts.mu.Unlock()
	for _, t := range ended {
		t.end()
	}
}

// get returns the terminal id names, nil when no holder hosts one.
func (ts *terminals) get(id string) *terminal {
	ts.sync()
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.byID[id]
}

// list returns every terminal, ordered by start.
func (ts *terminals) list() []TerminalInfo {
	ts.sync()
	ts.mu.Lock()
	out := make([]TerminalInfo, 0, len(ts.byID))
	for _, t := range ts.byID {
		out = append(out, t.info())
	}
	ts.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt != out[j].StartedAt {
			return out[i].StartedAt < out[j].StartedAt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// create starts a holder for a terminal the session replyID opens and
// spawns shell in cwd on a PTY of rows by cols. A holder whose shell could
// not be spawned is stopped again.
func (ts *terminals) create(replyID, cwd string, shell terminalShell, rows, cols int) (TerminalInfo, error) {
	rows, cols = clampTerminalSize(rows, cols, terminalDefaultRows, terminalDefaultCols)
	session, err := json.Marshal(terminalSession{
		Kind:    terminalKind,
		ReplyID: replyID,
		Cwd:     cwd,
		Shell:   shell.Argv[0],
		Label:   "terminal: " + filepath.Base(cwd),
	})
	if err != nil {
		return TerminalInfo{}, err
	}
	id, err := ts.host.Launch(session)
	if err != nil {
		return TerminalInfo{}, err
	}
	t := ts.get(id)
	conn := ts.host.Client(id)
	if t == nil || conn == nil {
		_ = ts.host.Stop(id)
		ts.sync()
		return TerminalInfo{}, errTerminalLost
	}
	t.mu.Lock()
	t.rows, t.cols = rows, cols
	t.mu.Unlock()
	if _, err := conn.Spawn(wsholder.SpawnSpec{Argv: shell.Argv, Cwd: cwd, Env: shell.Env, Mode: "pty", Rows: rows, Cols: cols}); err != nil {
		_ = ts.host.Stop(id)
		ts.sync()
		return TerminalInfo{}, fmt.Errorf("start %s: %w", shell.Argv[0], err)
	}
	fmt.Fprintf(ts.log, "dashboard: terminal %s opened from %s in %s (%s)\n", id, replyID, cwd, shell.Argv[0])
	return t.info(), nil
}

// close ends the terminal id and its shell through the holder manager. It
// reports false for an id that names no terminal.
func (ts *terminals) close(id string) (bool, error) {
	if ts.get(id) == nil {
		return false, nil
	}
	err := ts.host.Stop(id)
	if errors.Is(err, wsholder.ErrUnknownHolder) {
		err = nil
	}
	ts.sync()
	if err == nil {
		fmt.Fprintf(ts.log, "dashboard: terminal %s closed\n", id)
	}
	return true, err
}

func clampTerminalSize(rows, cols, defRows, defCols int) (int, int) {
	if rows <= 0 {
		rows = defRows
	}
	if cols <= 0 {
		cols = defCols
	}
	return min(max(rows, 2), terminalMaxRows), min(max(cols, 2), terminalMaxCols)
}

// terminal is one terminal: its holder's record stream and the viewers it
// fans out to.
type terminal struct {
	id      string
	meta    terminalSession
	started int64
	host    TerminalHost

	// opMu orders the operations sent to the holder (input and resize), so
	// two viewers' keystrokes and sizes reach the shell in one order.
	opMu sync.Mutex

	mu       sync.Mutex
	attached bool
	safe     bool
	viewers  map[*termViewer]struct{}
	// cursor is the sequence the holder's stream delivers next; head is one
	// past the highest sequence it has delivered. They differ while the
	// stream replays earlier records for a viewer.
	cursor, head uint64
	// replaying is set while a replay request is on its way to the holder;
	// again asks for another once it has answered, for a viewer that joined
	// meanwhile.
	replaying, again bool
	exited           bool
	exitCode, signal *int
	// rows and cols are the PTY's size as the daemon last set it. active is
	// the viewer that last typed into or focused the terminal: its size is
	// the PTY's, as with tmux's latest client.
	rows, cols int
	active     *termViewer
	ended      bool
}

func (t *terminal) info() TerminalInfo {
	t.mu.Lock()
	defer t.mu.Unlock()
	info := TerminalInfo{ID: t.id, Session: t.meta.ReplyID, Cwd: t.meta.Cwd, Shell: t.meta.Shell, StartedAt: t.started, State: "running", RestartSafe: t.safe}
	switch {
	case t.exited:
		info.State, info.ExitCode, info.Signal = "exited", t.exitCode, t.signal
	case !t.attached:
		info.State = "lost"
	}
	return info
}

// termFrame is one frame of the terminal WebSocket (terminal_routes.go
// documents the protocol).
type termFrame struct {
	T        string `json:"t"`
	Seq      uint64 `json:"seq,omitempty"`
	Data     []byte `json:"data,omitempty"`
	From     uint64 `json:"from,omitempty"`
	To       uint64 `json:"to,omitempty"`
	State    string `json:"state,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`
	Signal   *int   `json:"signal,omitempty"`
	Next     uint64 `json:"next,omitempty"`
	Rows     int    `json:"rows,omitempty"`
	Cols     int    `json:"cols,omitempty"`
}

func (f termFrame) encode() []byte {
	raw, _ := json.Marshal(f)
	return raw
}

// event applies one event of the holder's stream: a record goes to the
// viewers whose next sequence it is, and a gap moves the viewers it passes
// over.
func (t *terminal) event(ev wsholder.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch ev.Event {
	case "output", "exit":
		frame := termFrame{T: "out", Seq: ev.Seq, Data: ev.Data}
		if ev.Event == "exit" {
			frame = termFrame{T: "exit", Seq: ev.Seq, ExitCode: ev.ExitCode, Signal: ev.Signal}
			t.exited, t.exitCode, t.signal = true, ev.ExitCode, ev.Signal
		}
		var raw []byte
		lag := false
		for v := range t.viewers {
			switch {
			case ev.Seq == v.next:
				if raw == nil {
					raw = frame.encode()
				}
				v.next++
				v.fresh = false
				v.push(raw)
			case ev.Seq > v.next:
				lag = true
			}
		}
		t.cursor = ev.Seq + 1
		if t.cursor > t.head {
			t.head = t.cursor
		}
		if lag && !t.replaying {
			// The stream is past what a viewer needs and no replay is on
			// its way: the manager attached again after the sequence it
			// had seen. While a replay is on its way, the records the
			// holder sent before it are expected to run ahead.
			t.replayLocked()
		}
	case "gap":
		lag := false
		for v := range t.viewers {
			switch {
			case v.next < ev.From:
				lag = true
			case v.next < ev.To:
				// A viewer that asked for the whole ring is not told that
				// the ring begins after its first record.
				if !v.fresh {
					v.push(termFrame{T: "gap", From: v.next, To: ev.To}.encode())
				}
				v.next = ev.To
				v.fresh = false
			}
		}
		t.cursor = ev.To
		if t.cursor > t.head {
			t.head = t.cursor
		}
		if lag && !t.replaying {
			t.replayLocked()
		}
	}
}

// replayLocked has the holder replay for the viewers behind its stream.
// The request runs on its own goroutine: the caller may be the stream's own
// reader, which a request's answer has to pass.
func (t *terminal) replayLocked() {
	if t.replaying {
		t.again = true
		return
	}
	t.replaying = true
	go t.replay()
}

func (t *terminal) replay() {
	for {
		t.mu.Lock()
		t.again = false
		low, ok := t.lowLocked()
		if !ok || low >= t.cursor {
			t.replaying = false
			t.mu.Unlock()
			return
		}
		t.mu.Unlock()
		// By the time the holder's answer is here, the reader has passed
		// on every record the holder sent before it: from the answer on,
		// the stream is the replayed one.
		err := errTerminalLost
		if conn := t.host.Client(t.id); conn != nil {
			_, _, err = conn.Replay(low)
		}
		t.mu.Lock()
		if err != nil {
			// The viewers the replay was for connect again and ask anew.
			for v := range t.viewers {
				if v.next < t.cursor {
					v.drop()
				}
			}
			t.replaying = false
			t.mu.Unlock()
			return
		}
		if !t.again {
			t.replaying = false
			t.mu.Unlock()
			return
		}
		t.mu.Unlock()
	}
}

// lowLocked is the lowest sequence any viewer still needs.
func (t *terminal) lowLocked() (uint64, bool) {
	low, ok := uint64(0), false
	for v := range t.viewers {
		if !ok || v.next < low {
			low, ok = v.next, true
		}
	}
	return low, ok
}

// join adds a viewer that needs the records from sequence from on; zero
// asks for everything the holder's ring still holds. The viewer's first
// frame is the terminal's state. It returns nil for a terminal that has
// ended.
func (t *terminal) join(from uint64) *termViewer {
	v := &termViewer{next: from, wake: make(chan struct{}, 1), gone: make(chan struct{})}
	if from == 0 {
		v.next, v.fresh = 1, true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended {
		return nil
	}
	hello := termFrame{T: "hello", State: "running", Next: t.head, Rows: t.rows, Cols: t.cols}
	if t.exited {
		hello.State, hello.ExitCode, hello.Signal = "exited", t.exitCode, t.signal
	}
	v.push(hello.encode())
	t.viewers[v] = struct{}{}
	if v.next < t.cursor || (t.replaying && v.next < t.head) {
		t.replayLocked()
	}
	return v
}

func (t *terminal) leave(v *termViewer) {
	t.mu.Lock()
	delete(t.viewers, v)
	if t.active == v {
		t.active = nil
	}
	t.mu.Unlock()
	v.drop()
}

// end closes every viewer of a terminal whose holder is gone.
func (t *terminal) end() {
	t.mu.Lock()
	t.ended = true
	closed := termFrame{T: "closed"}.encode()
	for v := range t.viewers {
		v.push(closed)
		v.finish()
	}
	t.mu.Unlock()
}

// input writes a viewer's keystrokes to the shell and makes it the viewer
// whose size the PTY takes.
func (t *terminal) input(v *termViewer, data []byte) error {
	t.opMu.Lock()
	defer t.opMu.Unlock()
	t.activateLocked(v)
	if len(data) == 0 {
		return nil
	}
	conn := t.host.Client(t.id)
	if conn == nil {
		return errTerminalLost
	}
	return conn.Input(data, false)
}

// focus makes v the viewer whose size the PTY takes.
func (t *terminal) focus(v *termViewer) {
	t.opMu.Lock()
	defer t.opMu.Unlock()
	t.activateLocked(v)
}

// resize records the size v's card fits and sets the PTY to it when v is
// the active viewer, or when no viewer has typed or focused yet.
func (t *terminal) resize(v *termViewer, rows, cols int) {
	rows, cols = clampTerminalSize(rows, cols, 0, 0)
	t.opMu.Lock()
	defer t.opMu.Unlock()
	t.mu.Lock()
	v.rows, v.cols = rows, cols
	apply := t.active == nil || t.active == v
	t.mu.Unlock()
	if apply {
		t.applySizeLocked(rows, cols)
	}
}

// activateLocked makes v the active viewer and gives the PTY its size.
// The caller holds opMu.
func (t *terminal) activateLocked(v *termViewer) {
	t.mu.Lock()
	t.active = v
	rows, cols := v.rows, v.cols
	t.mu.Unlock()
	if rows > 0 && cols > 0 {
		t.applySizeLocked(rows, cols)
	}
}

// applySizeLocked sets the PTY's size unless it already has it. The caller
// holds opMu.
func (t *terminal) applySizeLocked(rows, cols int) {
	t.mu.Lock()
	same := (rows == t.rows && cols == t.cols) || t.exited
	t.mu.Unlock()
	if same {
		return
	}
	conn := t.host.Client(t.id)
	if conn == nil || conn.Resize(rows, cols) != nil {
		return
	}
	t.mu.Lock()
	t.rows, t.cols = rows, cols
	t.mu.Unlock()
}

// termViewer is one WebSocket's place in a terminal's stream. The terminal
// queues frames on it and the connection's writer takes them.
type termViewer struct {
	// next is the sequence the viewer needs next; fresh marks one that
	// asked for the whole ring and has taken nothing yet. rows and cols are
	// the size its card last fitted. All four are guarded by the terminal's
	// lock.
	next       uint64
	fresh      bool
	rows, cols int

	mu    sync.Mutex
	queue [][]byte
	bytes int
	// last is set once no frame follows the queued ones.
	last bool
	wake chan struct{}
	// gone is closed when the viewer is dropped: its connection ends at
	// once, and the page connects again.
	gone     chan struct{}
	goneOnce sync.Once
}

// push queues one frame; a viewer whose backlog is over terminalBacklogMax
// is dropped instead.
func (v *termViewer) push(frame []byte) {
	v.mu.Lock()
	if v.bytes+len(frame) > terminalBacklogMax {
		v.mu.Unlock()
		v.drop()
		return
	}
	v.queue = append(v.queue, frame)
	v.bytes += len(frame)
	v.mu.Unlock()
	v.signal()
}

// take returns the queued frames and whether they are the last.
func (v *termViewer) take() ([][]byte, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	frames := v.queue
	v.queue, v.bytes = nil, 0
	return frames, v.last
}

// finish ends the viewer once its queued frames are written.
func (v *termViewer) finish() {
	v.mu.Lock()
	v.last = true
	v.mu.Unlock()
	v.signal()
}

func (v *termViewer) signal() {
	select {
	case v.wake <- struct{}{}:
	default:
	}
}

func (v *termViewer) drop() { v.goneOnce.Do(func() { close(v.gone) }) }
