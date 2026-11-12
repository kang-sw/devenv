package wsagent

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// lineProtocol encodes the input lines of one harness's JSON-lines
// protocol: a user turn and a request awaiting a response.
type lineProtocol interface {
	userLine(text string) any
	requestLine(id string, request map[string]any) any
}

// lineReplies is what a lineMapper may do back to the process while it
// reads: answer a pending request, or send a line of its own (written
// asynchronously, so a mapper never blocks the reader on the child's
// input).
type lineReplies interface {
	answer(id string, a controlAnswer)
	send(v any)
}

// lineMapper turns one output line into events.
type lineMapper interface {
	mapLine(line []byte, replies lineReplies) []Event
}

type controlAnswer struct {
	ok  bool
	err string
	// body is a success answer's response payload, when it has one.
	body json.RawMessage
}

// child is the started process a streamProcess drives: an exec.Cmd of the
// daemon's own (execChild), or a session holder's child (holderChild).
type child interface {
	// name is the command's base name, for messages.
	name() string
	// write writes one input line (the newline included).
	write(line []byte) error
	// kill ends the child at once, without waiting: a refused launch never
	// gets to run a tool call. It may be called from the goroutine that
	// feeds the process.
	kill()
	// stop ends the child in order (input EOF, then a signal) and returns
	// once it has gone; exited is closed when the child's exit was seen.
	stop(exited <-chan struct{})
}

// streamProcess is a child speaking line-delimited JSON on stdin and
// stdout, parsed by a lineMapper.
type streamProcess struct {
	child   child
	proto   lineProtocol
	mapper  lineMapper
	events  chan Event
	closing chan struct{}
	exited  chan struct{}
	// stderr keeps the tail of the child's standard error for its exit
	// message.
	stderr *tailBuffer

	// feedMu serializes feed and finish, so events is never closed under a
	// send; a send blocked on a full channel ends when closing closes.
	feedMu   sync.Mutex
	finished bool

	writeMu sync.Mutex
	ctlMu   sync.Mutex
	ctlSeq  int
	waiters map[string]chan controlAnswer
	once    sync.Once

	// holder is set for a process under a session holder (holder.go).
	holder *holderChild
}

func newStreamProcess(mapper lineMapper, proto lineProtocol) *streamProcess {
	return &streamProcess{
		proto: proto, mapper: mapper,
		events:  make(chan Event, 256),
		closing: make(chan struct{}),
		exited:  make(chan struct{}),
		stderr:  &tailBuffer{max: 4 << 10},
		waiters: map[string]chan controlAnswer{},
	}
}

// startChild starts cmd and maps its output: under a session holder when
// spec asks for one (a lead conversation), else as the daemon's own child.
func startChild(spec StartSpec, cmd *exec.Cmd, mapper lineMapper, proto lineProtocol) (*streamProcess, error) {
	if spec.Lead != nil && spec.Lead.Holder != nil {
		p := newStreamProcess(mapper, proto)
		if err := spec.Lead.Holder.start(p, cmd); err != nil {
			return nil, err
		}
		return p, nil
	}
	return startStreamProcess(cmd, mapper, proto)
}

// startStreamProcess starts cmd as the daemon's own child.
func startStreamProcess(cmd *exec.Cmd, mapper lineMapper, proto lineProtocol) (*streamProcess, error) {
	p := newStreamProcess(mapper, proto)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = p.stderr
	// A grandchild that inherited stderr (a tool process, a stdio MCP
	// server) would otherwise hold Wait, and the conversation's stop, open
	// after the child itself exited.
	cmd.WaitDelay = streamWaitDelay
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", filepath.Base(cmd.Path), err)
	}
	p.child = &execChild{cmd: cmd, stdin: stdin}
	read := make(chan struct{})
	go func() {
		defer close(read)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, min(64<<10, maxLineBytes)), maxLineBytes)
		for sc.Scan() {
			p.feed(sc.Bytes(), 0, false)
		}
		if err := sc.Err(); err != nil {
			// A line over maxLineBytes or a failed read ends the reader; the
			// child, left writing to a pipe nobody reads, would block and hold
			// the turn open, so it is ended here with the error.
			p.child.kill()
			p.emit(Event{Fatal: fmt.Errorf("reading %s output: %w", p.child.name(), err)})
		}
	}()
	go func() {
		<-read
		err := cmd.Wait()
		if errors.Is(err, exec.ErrWaitDelay) {
			// The child exited cleanly; only its stderr was still held.
			err = nil
		}
		p.finish(err)
	}()
	return p, nil
}

// feed maps one output line and delivers its events. seq is the line's
// holder record sequence (0 for the daemon's own child); a replayed line
// (one the runner applied under an earlier daemon) rebuilds the mapper's
// and the runner's state and answers nothing. A holder line is followed by
// a LineEnd event, so the runner knows the line is applied whole: also a
// line with no events, which the mapper may have answered on the child's
// input and must not answer again after a daemon restart.
func (p *streamProcess) feed(line []byte, seq uint64, replay bool) {
	p.feedMu.Lock()
	defer p.feedMu.Unlock()
	if p.finished {
		return
	}
	var replies lineReplies = p
	if replay {
		replies = silentReplies{}
	}
	evs := p.mapper.mapLine(line, replies)
	for _, ev := range evs {
		if ev.Fatal != nil && !replay {
			p.child.kill()
		}
		ev.Seq, ev.Replay = seq, replay
		p.emit(ev)
	}
	if seq > 0 && (len(evs) > 0 || !replay) {
		p.emit(Event{Seq: seq, Replay: replay, LineEnd: true})
	}
}

func (p *streamProcess) emit(ev Event) {
	select {
	case p.events <- ev:
	case <-p.closing:
	}
}

// deliver sends an event from outside feed (a holder's replay mark, an
// oversized line), unless the stream has ended.
func (p *streamProcess) deliver(ev Event) {
	p.feedMu.Lock()
	defer p.feedMu.Unlock()
	if !p.finished {
		p.emit(ev)
	}
}

// finish ends the event stream: the child has exited. A failed exit (err)
// the runner did not ask for is reported first, with the child's last
// standard error lines.
func (p *streamProcess) finish(err error) {
	p.feedMu.Lock()
	defer p.feedMu.Unlock()
	if p.finished {
		return
	}
	p.finished = true
	close(p.exited)
	select {
	case <-p.closing:
	default:
		if err != nil {
			msg := strings.TrimSpace(p.stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			p.emit(Event{Fatal: fmt.Errorf("%s exited: %s", p.child.name(), msg)})
		}
	}
	close(p.events)
}

// silentReplies drops what a mapper sends back while a replayed line is
// mapped: the answer was given, or the request made, under an earlier
// daemon.
type silentReplies struct{}

func (silentReplies) answer(string, controlAnswer) {}
func (silentReplies) send(any)                     {}

// execChild is a child the daemon started itself.
type execChild struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
}

func (c *execChild) name() string { return filepath.Base(c.cmd.Path) }

func (c *execChild) write(line []byte) error {
	_, err := c.stdin.Write(line)
	return err
}

func (c *execChild) kill() { _ = c.cmd.Process.Kill() }

func (c *execChild) stop(exited <-chan struct{}) { stopChild(c.cmd, c.stdin.Close, exited) }

func (p *streamProcess) Events() <-chan Event { return p.events }

func (p *streamProcess) writeLine(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return p.child.write(append(raw, '\n'))
}

func (p *streamProcess) Send(text string) error { return p.writeLine(p.proto.userLine(text)) }

func (p *streamProcess) send(v any) { go func() { _ = p.writeLine(v) }() }

// streamWaitDelay bounds how long an exited child's output pipes are read.
const streamWaitDelay = 2 * time.Second

// maxLineBytes bounds one stdout line; a test lowers it.
var maxLineBytes = 64 << 20

// controlTimeout bounds the wait for a request's response.
var controlTimeout = 10 * time.Second

// nextID is a fresh request id.
func (p *streamProcess) nextID() string {
	p.ctlMu.Lock()
	defer p.ctlMu.Unlock()
	p.ctlSeq++
	return "ws-" + strconv.Itoa(p.ctlSeq)
}

// request sends a request named name and waits for its response. An error
// answer that says the request is unsupported wraps ErrUnsupported.
func (p *streamProcess) request(name string, req map[string]any) error {
	_, err := p.requestBody(name, req, controlTimeout)
	return err
}

// requestBody is request that also returns a success answer's response
// payload, waiting at most timeout.
func (p *streamProcess) requestBody(name string, req map[string]any, timeout time.Duration) (json.RawMessage, error) {
	id := p.nextID()
	ch := make(chan controlAnswer, 1)
	p.ctlMu.Lock()
	p.waiters[id] = ch
	p.ctlMu.Unlock()
	defer func() {
		p.ctlMu.Lock()
		delete(p.waiters, id)
		p.ctlMu.Unlock()
	}()
	if err := p.writeLine(p.proto.requestLine(id, req)); err != nil {
		return nil, err
	}
	select {
	case a := <-ch:
		if !a.ok {
			if strings.Contains(strings.ToLower(a.err), "unsupported") {
				return nil, fmt.Errorf("%w: %s", ErrUnsupported, a.err)
			}
			return nil, errors.New(a.err)
		}
		return a.body, nil
	case <-p.exited:
		return nil, errors.New("the process exited")
	case <-time.After(timeout):
		return nil, fmt.Errorf("no answer to %s within %s", name, timeout)
	}
}

func (p *streamProcess) answer(id string, a controlAnswer) {
	p.ctlMu.Lock()
	ch := p.waiters[id]
	p.ctlMu.Unlock()
	if ch != nil {
		ch <- a
	}
}

func (p *streamProcess) Close() error {
	p.once.Do(func() {
		close(p.closing)
		p.child.stop(p.exited)
		if p.holder != nil {
			// The holder is gone; an exit record it could not send ends
			// the stream here.
			p.holder.release()
			p.finish(nil)
		}
	})
	for range p.events {
	}
	return nil
}

// Detach lets go of a process under a session holder without ending it:
// the child keeps running for the next daemon. It reports false for the
// daemon's own child, which the caller closes instead.
func (p *streamProcess) Detach() bool {
	if p.holder == nil {
		return false
	}
	p.once.Do(func() {
		close(p.closing)
		p.holder.release()
		p.finish(nil)
	})
	for range p.events {
	}
	return true
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (t *tailBuffer) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(b), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// batchTracker builds the lead's tool batch list from a harness's tool
// calls: one batch per assistant message (group), running while a call is
// unfinished and ended once every call ended. Each event carries the whole
// current batch under a per-process epoch, so a re-sent batch replaces its
// earlier state and a finished one is counted once.
type batchTracker struct {
	epoch string
	seq   int64
	batch *toolBatch
}

type toolBatch struct {
	group     string
	seq       int64
	startedAt int64
	calls     []*toolCall
}

type toolCall struct {
	id        string
	name      string
	args      string
	startedAt int64
	endedAt   int64
	failed    bool
}

func newBatchTracker(prefix string) batchTracker {
	return batchTracker{epoch: prefix + strconv.FormatInt(time.Now().UnixMilli(), 36)}
}

// addCall records a call of the assistant message group, ending the
// current batch first when it belongs to another message.
func (t *batchTracker) addCall(group, id, name, args string, now int64) []Event {
	var out []Event
	if t.batch != nil && t.batch.group != group {
		if ev := t.finish(now, true); ev != nil {
			out = append(out, *ev)
		}
	}
	if t.batch == nil {
		t.seq++
		t.batch = &toolBatch{group: group, seq: t.seq, startedAt: now}
	}
	t.batch.calls = append(t.batch.calls, &toolCall{id: id, name: name, args: args, startedAt: now})
	return append(out, t.event())
}

// endCall ends the call id; the batch finishes when it was the last open
// call.
func (t *batchTracker) endCall(id string, failed bool, now int64) []Event {
	if t.batch == nil {
		return nil
	}
	changed := false
	for _, c := range t.batch.calls {
		if c.id == id && c.endedAt == 0 {
			c.endedAt, c.failed, changed = now, failed, true
		}
	}
	if !changed {
		return nil
	}
	for _, c := range t.batch.calls {
		if c.endedAt == 0 {
			return []Event{t.event()}
		}
	}
	if ev := t.finish(now, false); ev != nil {
		return []Event{*ev}
	}
	return nil
}

// finish ends the current batch (unfinished calls fail when cutShort) and
// returns its final event.
func (t *batchTracker) finish(now int64, cutShort bool) *Event {
	if t.batch == nil {
		return nil
	}
	for _, c := range t.batch.calls {
		if c.endedAt == 0 {
			c.endedAt, c.failed = now, cutShort
		}
	}
	ev := t.event()
	t.batch = nil
	return &ev
}

type wireBatch struct {
	Epoch     string              `json:"epoch"`
	Seq       int64               `json:"seq"`
	ToolNames []string            `json:"toolNames"`
	CallCount int                 `json:"callCount"`
	Failed    int                 `json:"failed"`
	StartedAt int64               `json:"startedAt"`
	EndedAt   *int64              `json:"endedAt,omitempty"`
	Running   *wslive.RunningView `json:"running,omitempty"`
	Calls     []wireCall          `json:"calls,omitempty"`
}

type wireCall struct {
	Name        string `json:"name"`
	StartedAt   int64  `json:"startedAt"`
	EndedAt     int64  `json:"endedAt,omitempty"`
	Failed      bool   `json:"failed,omitempty"`
	ArgsPreview string `json:"argsPreview,omitempty"`
}

// event is the lead's batch list holding the current batch: running while
// a call is unfinished, else ended.
func (t *batchTracker) event() Event {
	b := t.batch
	w := wireBatch{Epoch: t.epoch, Seq: b.seq, ToolNames: []string{}, CallCount: len(b.calls), StartedAt: b.startedAt}
	var running *toolCall
	done, open := 0, 0
	var ended int64
	for _, c := range b.calls {
		if !contains(w.ToolNames, c.name) {
			w.ToolNames = append(w.ToolNames, c.name)
		}
		w.Calls = append(w.Calls, wireCall{Name: c.name, StartedAt: c.startedAt, EndedAt: c.endedAt, Failed: c.failed, ArgsPreview: c.args})
		switch {
		case c.endedAt == 0:
			open++
			if running == nil {
				running = c
			}
		default:
			done++
			if c.failed {
				w.Failed++
			}
			ended = max(ended, c.endedAt)
		}
	}
	if running != nil {
		w.Calls = nil
		w.Running = &wslive.RunningView{ToolName: running.name, ArgsPreview: running.args, StartedAt: running.startedAt, Done: done, OthersRunning: open - 1}
	} else {
		w.EndedAt = &ended
	}
	raw, _ := json.Marshal(w)
	return Event{Live: &wslive.Event{Kind: wslive.KindAgent, Batches: []json.RawMessage{raw}}}
}

// callName is the current batch's call id's tool name; "" when the batch
// holds no such call.
func (t *batchTracker) callName(id string) string {
	if t.batch == nil {
		return ""
	}
	for _, c := range t.batch.calls {
		if c.id == id {
			return c.name
		}
	}
	return ""
}

// CardTool is the ws tool whose result a support chat draws as a card.
const CardTool = "tracker.show"

// UITool is the ws tool whose result a support chat reads as a dashboard
// UI command.
const UITool = "tracker.ui"

// TrackerWriteTool is the ws tool that writes tracker stores; a successful
// call of it raises an Event with TrackerWrite set.
const TrackerWriteTool = "tracker.update"

// cardEvent reads a CardTool result: a first line "card <store> <alias>",
// then the item line "<alias> <kind> <quoted title>", whose title it keeps
// when it parses. A result without the card line (an item of another
// store, answered as text) shows no card: nil.
func cardEvent(result string) *Event {
	lines := strings.SplitN(result, "\n", 3)
	f := strings.Fields(lines[0])
	if len(f) != 3 || f[0] != "card" {
		return nil
	}
	ref := wslive.CardRef{Store: f[1], Alias: f[2]}
	if len(lines) > 1 {
		if alias, rest, ok := strings.Cut(strings.TrimSpace(lines[1]), " "); ok && alias == ref.Alias {
			if _, quoted, ok := strings.Cut(rest, " "); ok {
				if title, err := strconv.Unquote(quoted); err == nil {
					ref.Title = title
				}
			}
		}
	}
	return &Event{Card: &ref}
}

// uiEvent reads a UITool result: a first line "ui <op> <arg>", where a
// search's arg is the rest of the line and a focus may add a comment alias
// ("ui focus <alias> [<comment>]"). The ring write validates the command;
// a result without the line raises nothing: nil.
func uiEvent(result string) *Event {
	line, _, _ := strings.Cut(result, "\n")
	rest, ok := strings.CutPrefix(line, "ui ")
	if !ok {
		return nil
	}
	op, arg, _ := strings.Cut(strings.TrimSpace(rest), " ")
	cmd := wslive.UICommand{Op: op, Arg: strings.TrimSpace(arg)}
	if op == wslive.UIFocus {
		if alias, comment, ok := strings.Cut(cmd.Arg, " "); ok {
			cmd.Arg, cmd.Comment = alias, strings.TrimSpace(comment)
		}
	}
	return &Event{UI: &cmd}
}

// resultTools are the ws tools whose successful results raise events
// (resultEvents).
var resultTools = []string{CardTool, UITool, TrackerWriteTool}

// resultTool is the result-reading ws tool a backend's tool name names
// under that backend's MCP naming (mcpName) on the attached ws-mcp, or ""
// for any other tool, another server's of the same name included.
func resultTool(name string, mcpName func(server, tool string) string) string {
	for _, tool := range resultTools {
		if name == mcpName(ReservedMCPServer, tool) {
			return tool
		}
	}
	return ""
}

// resultEvents are the events a successful call of the attached ws-mcp's
// tool name raises from its result text: a card line, a UI command, or a
// tracker write.
func resultEvents(name, text string) []Event {
	var out []Event
	switch name {
	case CardTool:
		if ev := cardEvent(text); ev != nil {
			out = append(out, *ev)
		}
	case UITool:
		if ev := uiEvent(text); ev != nil {
			out = append(out, *ev)
		}
	case TrackerWriteTool:
		out = append(out, Event{TrackerWrite: true})
	}
	return out
}

// argsPreview is a tool call's arguments as one line: the first string
// value of a small object, else the compact JSON.
func argsPreview(raw json.RawMessage) string {
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil {
		for _, key := range []string{"command", "file_path", "path", "pattern", "url", "query", "description"} {
			if s, ok := obj[key].(string); ok && s != "" {
				return s
			}
		}
	}
	return string(raw)
}

func contains(list []string, s string) bool {
	for _, have := range list {
		if have == s {
			return true
		}
	}
	return false
}
