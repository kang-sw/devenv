package wsagent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wslive"
)

// fakeStream is one holder connection: what it carries (the holder's
// records and its answers to calls) arrives in order, one item after the
// other, as the real client's single reader delivers it. An answer
// therefore waits behind every record sent before it.
type fakeStream struct {
	mu   sync.Mutex
	cond *sync.Cond
	q    []func()
	dead bool
}

func newFakeStream() *fakeStream {
	s := &fakeStream{}
	s.cond = sync.NewCond(&s.mu)
	go func() {
		for {
			s.mu.Lock()
			for len(s.q) == 0 && !s.dead {
				s.cond.Wait()
			}
			if s.dead {
				s.mu.Unlock()
				return
			}
			fn := s.q[0]
			s.q = s.q[1:]
			s.mu.Unlock()
			fn()
		}
	}()
	return s
}

func (s *fakeStream) push(fn func()) {
	s.mu.Lock()
	if !s.dead {
		s.q = append(s.q, fn)
	}
	s.mu.Unlock()
	s.cond.Signal()
}

func (s *fakeStream) kill() {
	s.mu.Lock()
	s.dead, s.q = true, nil
	s.mu.Unlock()
	s.cond.Broadcast()
}

type fakeHolder struct {
	id      string
	pid     int
	session json.RawMessage
	spec    *wsholder.SpawnSpec
	// ring is every record the holder holds; a record's sequence is its
	// index plus one.
	ring     []wsholder.Event
	inputs   []string
	attached bool
	exited   bool
	stream   *fakeStream
}

// fakeHolders is a HolderHost with no process behind it. Holders outlive
// the runner that started them: restart() is the daemon ending, after
// which a new runner's Scan attaches again and each holder replays its
// whole ring.
type fakeHolders struct {
	mu      sync.Mutex
	sinks   []wsholder.Sink
	holders map[string]*fakeHolder
	order   []string
	// ops lists the launches, spawns, and stops asked for, in order
	// ("launch h1", "spawn h1", "stop h1").
	ops  []string
	next int
	// callTimeout bounds a call's wait for its answer; timeouts counts the
	// calls that ran into it.
	callTimeout time.Duration
	timeouts    int
	// spawnErr fails Spawn and stopErr Stop (the holder stays).
	spawnErr, stopErr error
	// refuse answers a control request of a subtype with an error; any
	// other control request written to a child is answered with success,
	// as a harness answers it.
	refuse map[string]string
}

func newFakeHolders() *fakeHolders {
	return &fakeHolders{holders: map[string]*fakeHolder{}, callTimeout: 5 * time.Second, refuse: map[string]string{}}
}

func (f *fakeHolders) AddSink(sink wsholder.Sink) {
	f.mu.Lock()
	f.sinks = append(f.sinks, sink)
	f.mu.Unlock()
}

// deliverLocked queues a record on an attached holder's connection.
func (f *fakeHolders) deliverLocked(h *fakeHolder, ev wsholder.Event) {
	if !h.attached {
		return
	}
	id := h.id
	h.stream.push(func() {
		f.mu.Lock()
		sinks := slices.Clone(f.sinks)
		f.mu.Unlock()
		for _, sink := range sinks {
			sink.HolderEvent(id, ev)
		}
	})
}

func (f *fakeHolders) appendLocked(h *fakeHolder, ev wsholder.Event) {
	ev.Seq = uint64(len(h.ring) + 1)
	h.ring = append(h.ring, ev)
	f.deliverLocked(h, ev)
}

func (f *fakeHolders) Scan() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range f.order {
		h := f.holders[id]
		if h == nil || h.attached {
			continue
		}
		h.attached, h.stream = true, newFakeStream()
		for _, ev := range h.ring {
			f.deliverLocked(h, ev)
		}
	}
}

func (f *fakeHolders) List() wsholder.Listing {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out wsholder.Listing
	for _, id := range f.order {
		h := f.holders[id]
		if h == nil {
			continue
		}
		info := wsholder.Info{ID: h.id, PID: 9000 + h.pid, Attached: h.attached, Session: h.session}
		if h.attached {
			info.Child = &wsholder.ChildStatus{State: "running", PID: h.pid}
			if h.exited {
				info.Child.State = "exited"
			}
		}
		out.Holders = append(out.Holders, info)
	}
	return out
}

// add makes a holder as an earlier daemon left it: running, not attached.
func (f *fakeHolders) add(session any) string {
	raw, _ := json.Marshal(session)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.newLocked(raw, false).id
}

func (f *fakeHolders) newLocked(session json.RawMessage, attached bool) *fakeHolder {
	f.next++
	h := &fakeHolder{id: fmt.Sprintf("h%d", f.next), pid: 4000 + f.next, session: session, attached: attached}
	if attached {
		h.stream = newFakeStream()
	}
	f.holders[h.id] = h
	f.order = append(f.order, h.id)
	return h
}

func (f *fakeHolders) Launch(session json.RawMessage) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	h := f.newLocked(slices.Clone(session), true)
	f.ops = append(f.ops, "launch "+h.id)
	return h.id, nil
}

// call waits for the holder's answer, which arrives after every record
// queued before it.
func (f *fakeHolders) call(id, name string) error {
	f.mu.Lock()
	h := f.holders[id]
	if h == nil || !h.attached {
		f.mu.Unlock()
		return wsholder.ErrDetached
	}
	done := make(chan struct{})
	h.stream.push(func() { close(done) })
	wait := f.callTimeout
	f.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-time.After(wait):
		f.mu.Lock()
		f.timeouts++
		f.mu.Unlock()
		return fmt.Errorf("holder: %s was not answered within %s", name, wait)
	}
}

func (f *fakeHolders) Spawn(id string, spec wsholder.SpawnSpec) (wsholder.ChildStatus, error) {
	f.mu.Lock()
	h := f.holders[id]
	if h == nil {
		f.mu.Unlock()
		return wsholder.ChildStatus{}, wsholder.ErrDetached
	}
	f.ops = append(f.ops, "spawn "+id)
	if err := f.spawnErr; err != nil {
		f.mu.Unlock()
		return wsholder.ChildStatus{}, err
	}
	h.spec = &spec
	pid := h.pid
	f.mu.Unlock()
	if err := f.call(id, "spawn"); err != nil {
		return wsholder.ChildStatus{}, err
	}
	return wsholder.ChildStatus{State: "running", PID: pid, Mode: spec.Mode}, nil
}

func (f *fakeHolders) Input(id string, data []byte, eof bool) error {
	f.mu.Lock()
	h := f.holders[id]
	if h == nil || !h.attached {
		f.mu.Unlock()
		return wsholder.ErrDetached
	}
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		h.inputs = append(h.inputs, string(line))
		var d struct {
			Type      string `json:"type"`
			RequestID string `json:"request_id"`
			Request   struct {
				Subtype string `json:"subtype"`
			} `json:"request"`
		}
		if json.Unmarshal(line, &d) == nil && d.Type == "control_request" {
			response := map[string]any{"subtype": "success", "request_id": d.RequestID}
			if msg, ok := f.refuse[d.Request.Subtype]; ok {
				response = map[string]any{"subtype": "error", "request_id": d.RequestID, "error": msg}
			}
			answer, _ := json.Marshal(map[string]any{"type": "control_response", "response": response})
			f.appendLocked(h, wsholder.Event{Event: "output", Stream: "stdout", Data: append(answer, '\n')})
		}
	}
	if eof && !h.exited {
		// A harness ends when its input does.
		h.exited = true
		code := 0
		f.appendLocked(h, wsholder.Event{Event: "exit", ExitCode: &code})
	}
	f.mu.Unlock()
	return f.call(id, "input")
}

func (f *fakeHolders) Status(id string) (wsholder.AttachInfo, error) {
	if err := f.call(id, "status"); err != nil {
		return wsholder.AttachInfo{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	h := f.holders[id]
	if h == nil {
		return wsholder.AttachInfo{}, wsholder.ErrDetached
	}
	return wsholder.AttachInfo{Protocol: 1, Child: wsholder.ChildStatus{State: "running", PID: h.pid}, FirstSeq: 1, NextSeq: uint64(len(h.ring) + 1)}, nil
}

func (f *fakeHolders) Stop(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	h := f.holders[id]
	if h == nil {
		return wsholder.ErrUnknownHolder
	}
	f.ops = append(f.ops, "stop "+id)
	if f.stopErr != nil {
		return f.stopErr
	}
	if h.stream != nil {
		h.stream.kill()
	}
	delete(f.holders, id)
	return nil
}

// out is one line the holder's child wrote to its output.
func (f *fakeHolders) out(id string, lines ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	h := f.holders[id]
	if h == nil {
		panic("no holder " + id)
	}
	for _, line := range lines {
		f.appendLocked(h, wsholder.Event{Event: "output", Stream: "stdout", Data: []byte(line + "\n")})
	}
}

// exit ends the holder's child from its side.
func (f *fakeHolders) exit(id string, code int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	h := f.holders[id]
	h.exited = true
	f.appendLocked(h, wsholder.Event{Event: "exit", ExitCode: &code})
}

// restart is the daemon's end: every connection closes, and the holders
// run on for the next daemon to find.
func (f *fakeHolders) restart() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sinks = nil
	for _, h := range f.holders {
		if h.stream != nil {
			h.stream.kill()
		}
		h.attached, h.stream = false, nil
	}
}

// vanish removes a holder as if it ended while no daemon ran.
func (f *fakeHolders) vanish(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if h := f.holders[id]; h != nil && h.stream != nil {
		h.stream.kill()
	}
	delete(f.holders, id)
}

func (f *fakeHolders) opList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.ops)
}

func (f *fakeHolders) opCount(prefix string) int {
	n := 0
	for _, op := range f.opList() {
		if strings.HasPrefix(op, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeHolders) timedOut() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.timeouts
}

func (f *fakeHolders) set(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

// inputs lists the lines written to the holder's child whose "type" is
// kind (every line for an empty kind), oldest first.
func (f *fakeHolders) inputs(id, kind string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	h := f.holders[id]
	if h == nil {
		return nil
	}
	var out []string
	for _, line := range h.inputs {
		var d struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal([]byte(line), &d)
		if kind == "" || d.Type == kind {
			out = append(out, line)
		}
	}
	return out
}

func (f *fakeHolders) spawned(id string) wsholder.SpawnSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	if h := f.holders[id]; h != nil && h.spec != nil {
		return *h.spec
	}
	return wsholder.SpawnSpec{}
}

func (f *fakeHolders) records(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if h := f.holders[id]; h != nil {
		return len(h.ring)
	}
	return 0
}

type fakeLeadKeys struct {
	mu      sync.Mutex
	n       int
	ensured []string
	rootErr error
}

func (k *fakeLeadKeys) LeadRoot(root string) (string, error) {
	if k.rootErr != nil {
		return "", k.rootErr
	}
	return root, nil
}

func (k *fakeLeadKeys) MintLead(string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.n++
	return fmt.Sprintf("lead-key-%d", k.n), nil
}

func (k *fakeLeadKeys) EnsureLead(_, key string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.ensured = append(k.ensured, key)
	return nil
}

// fakeMail is a LeadMail a test delivers into.
type fakeMail struct {
	mu      sync.Mutex
	watches map[string]func()
	queued  map[string][]string
	stops   int
}

func newFakeMail() *fakeMail {
	return &fakeMail{watches: map[string]func(){}, queued: map[string][]string{}}
}

func (m *fakeMail) Watch(_, key string, changed func()) func() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.watches[key] = changed
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.stops++
		delete(m.watches, key)
	}
}

func (m *fakeMail) Drain(_, key string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.queued[key]
	delete(m.queued, key)
	return out, nil
}

// deliver puts mail in key's mailbox and tells its watcher, if any.
func (m *fakeMail) deliver(key string, texts ...string) {
	m.mu.Lock()
	m.queued[key] = append(m.queued[key], texts...)
	changed := m.watches[key]
	m.mu.Unlock()
	if changed != nil {
		changed()
	}
}

func (m *fakeMail) stopped() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stops
}

// logBuffer is a daemon log several goroutines write.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// leadHarness runs lead conversations through the real backends over a
// fake holder host: nothing is executed, and the test writes the harness's
// output lines itself.
type leadHarness struct {
	t       *testing.T
	dir     string
	root    string
	opts    Options
	holders *fakeHolders
	keys    *fakeLeadKeys
	mail    *fakeMail
	log     *logBuffer
	runner  *Runner
	syncs   int
	// notes counts the runner's page notifications.
	notes atomic.Int64
}

// leadDaemonEnv is the daemon's own environment in a lead test: a harness
// session's, with a mailbox identity and hosted-session variables of its
// own that no spawned session may inherit.
var leadDaemonEnv = []string{
	"PATH=/bin", "HOME=/h", "CLAUDECODE=1", "CLAUDE_CONFIG_DIR=/claude",
	"WS_MAILBOX=lead@elsewhere", "WS_MAILBOX_AUTO=1", "WS_MCP_ROOT=/elsewhere", "WS_RSRC=/rsrc",
	"WS_HOSTED_SESSION=1", "WS_HOSTED_SESSION_KEY=the-daemons-own", "WS_HOSTED_SESSION_ROW=/elsewhere/row.json",
	"WS_PI_ROLE=parent",
}

func newLeadHarness(t *testing.T) *leadHarness {
	t.Helper()
	h := &leadHarness{t: t, dir: t.TempDir(), root: t.TempDir(), holders: newFakeHolders(), keys: &fakeLeadKeys{}, mail: newFakeMail(), log: &logBuffer{}}
	restore := environ
	environ = func() []string { return slices.Clone(leadDaemonEnv) }
	t.Cleanup(func() { environ = restore })
	h.opts = Options{
		Dir:       h.dir,
		Backends:  map[string]Backend{BackendClaude: ClaudeBackend{Command: "/opt/harness/claude"}, BackendPi: PiBackend{Command: "/opt/harness/pi"}},
		Keys:      &fakeKeys{},
		Config:    testConfig(t),
		Log:       h.log,
		Holders:   h.holders,
		LeadKeys:  h.keys,
		Mail:      h.mail,
		heartbeat: 50 * time.Millisecond,
	}
	return h
}

// start starts a runner over the harness's directory and holders, as a
// daemon start does.
func (h *leadHarness) start() *leadHarness {
	h.runner = NewRunner(h.opts)
	h.runner.Start(func(string, string) { h.notes.Add(1) })
	h.t.Cleanup(h.runner.Close)
	return h
}

// restart ends the daemon (its conversations' processes run on under
// their holders) and starts the next one.
func (h *leadHarness) restart() {
	h.runner.Close()
	h.holders.restart()
	h.start()
}

func (h *leadHarness) lead(backend string, opts LeadOptions) State {
	h.t.Helper()
	st, err := h.runner.CreateLead(backend, h.root, opts)
	if err != nil {
		h.t.Fatalf("CreateLead(%s): %v", backend, err)
	}
	return st
}

func (h *leadHarness) state(id string) State {
	h.t.Helper()
	st, err := h.runner.Get(id)
	if err != nil {
		h.t.Fatalf("Get: %v", err)
	}
	return st
}

func (h *leadHarness) send(id, text string) {
	h.t.Helper()
	if _, err := h.runner.Send(id, text); err != nil {
		h.t.Fatalf("Send: %v", err)
	}
}

func (h *leadHarness) record(id string) Record {
	h.t.Helper()
	rec, err := readRecord(wsdashboard.SessionDir(h.dir, id))
	if err != nil {
		h.t.Fatalf("read the record: %v", err)
	}
	return rec
}

// holder waits for the conversation to run under a holder and returns its
// id.
func (h *leadHarness) holder(id string) string {
	h.t.Helper()
	var holder string
	eventually(h.t, "a process under a holder", func() bool {
		rec, err := readRecord(wsdashboard.SessionDir(h.dir, id))
		holder = rec.HolderID
		return err == nil && holder != "" && rec.HostPID != 0
	})
	return holder
}

// ring is the conversation's ring file, oldest entry first.
func (h *leadHarness) ring(id string) []wslive.RingEntry {
	h.t.Helper()
	recs, err := wslive.ReadRing(wslive.RingDir(wsdashboard.SessionDir(h.dir, id)))
	if err != nil {
		h.t.Fatalf("read the ring: %v", err)
	}
	var out []wslive.RingEntry
	for _, rec := range recs {
		out = append(out, rec.RingEntry)
	}
	return out
}

func (h *leadHarness) snapshot(id string) wslive.Snapshot {
	h.t.Helper()
	snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(wsdashboard.SessionDir(h.dir, id)))
	if err != nil {
		h.t.Fatalf("read the snapshot: %v", err)
	}
	return snap
}

// sync waits until the runner applied every line written to the holder so
// far: it writes a background task count no other line carries, waits for
// the state to show it, and takes it back. A holder's replay does not
// satisfy it, since each call's count is its own.
func (h *leadHarness) sync(id, holder string) {
	h.t.Helper()
	h.syncs++
	n := 100 + h.syncs
	h.holders.out(holder, tasksLine(n))
	eventually(h.t, "the stream to be applied", func() bool { return h.state(id).BackgroundTasks == n })
	h.holders.out(holder, tasksLine(0))
	eventually(h.t, "the stream to be applied", func() bool { return h.state(id).BackgroundTasks == 0 })
}

// userInputs are the texts of the user turns written to the holder's child.
func (h *leadHarness) userInputs(holder string) []string {
	var out []string
	for _, line := range h.holders.inputs(holder, "user") {
		var d struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}
		_ = json.Unmarshal([]byte(line), &d)
		out = append(out, d.Message.Content)
	}
	return out
}

func jsonLine(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

// Lines a claude lead's harness writes, in the shapes the recordings show.
func stateLine(state string) string {
	return jsonLine(map[string]any{"type": "system", "subtype": "session_state_changed", "state": state})
}

func initLine(session, mode string) string {
	return jsonLine(map[string]any{"type": "system", "subtype": "init", "session_id": session, "permissionMode": mode})
}

func statusModeLine(mode string) string {
	return jsonLine(map[string]any{"type": "system", "subtype": "status", "status": nil, "permissionMode": mode})
}

func replyLine(uuid, text string) string {
	return jsonLine(map[string]any{"type": "assistant", "uuid": uuid, "message": map[string]any{"id": "msg-" + uuid, "content": []map[string]any{{"type": "text", "text": text}}}})
}

func toolUseLine(uuid, toolUse string) string {
	return jsonLine(map[string]any{"type": "assistant", "uuid": uuid, "message": map[string]any{"id": "msg-" + uuid, "content": []map[string]any{{"type": "tool_use", "id": toolUse, "name": "Bash", "input": map[string]any{"command": "echo hi"}}}}})
}

func canUseToolLine(request, toolUse string) string {
	return jsonLine(map[string]any{"type": "control_request", "request_id": request, "request": map[string]any{"subtype": "can_use_tool", "tool_name": "Bash", "input": map[string]any{"command": "echo hi"}, "description": "Print hi", "tool_use_id": toolUse}})
}

func toolResultLine(toolUse string) string {
	return jsonLine(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": []map[string]any{{"type": "tool_result", "tool_use_id": toolUse, "content": "hi"}}}})
}

func resultLine() string {
	return jsonLine(map[string]any{"type": "result", "subtype": "success", "is_error": false, "num_turns": 1})
}

func tasksLine(n int) string {
	tasks := make([]map[string]any, n)
	for i := range tasks {
		tasks[i] = map[string]any{"id": fmt.Sprintf("task-%d", i)}
	}
	return jsonLine(map[string]any{"type": "system", "subtype": "background_tasks_changed", "tasks": tasks})
}

// turn is a whole turn's output on a harness that emits session states.
func turnLines(session, uuid, text string) []string {
	return []string{stateLine(HarnessRunning), initLine(session, "auto"), replyLine(uuid, text), resultLine(), stateLine(HarnessIdle)}
}

// runLead creates a claude lead, sends its first message, and returns the
// conversation and the holder its process runs under.
func (h *leadHarness) runLead(text string) (id, holder string) {
	h.t.Helper()
	id = h.lead(BackendClaude, LeadOptions{}).ID
	h.send(id, text)
	return id, h.holder(id)
}

// idle waits for the conversation to run no turn.
func (h *leadHarness) idle(id string) {
	h.t.Helper()
	eventually(h.t, "the turn to end", func() bool { return h.state(id).Status == StatusIdle })
}

// globalProfile writes a profile into the user's (global) config.
func (h *leadHarness) globalProfile(name, raw string) {
	h.t.Helper()
	writeStructured(h.t, filepath.Join(h.opts.Config.ConfigHome, "config.json"), map[string]string{ProfilePrefix + name: raw})
}

func isConflict(err error) bool {
	var re *requestError
	return errors.As(err, &re) && re.status == 409
}
