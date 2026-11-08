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
}

// streamProcess is a child speaking line-delimited JSON on stdin and
// stdout, parsed by a lineMapper.
type streamProcess struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	proto   lineProtocol
	events  chan Event
	closing chan struct{}
	exited  chan struct{}
	read    chan struct{}

	writeMu sync.Mutex
	ctlMu   sync.Mutex
	ctlSeq  int
	waiters map[string]chan controlAnswer
	once    sync.Once
}

func startStreamProcess(cmd *exec.Cmd, mapper lineMapper, proto lineProtocol) (*streamProcess, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &tailBuffer{max: 4 << 10}
	cmd.Stderr = stderr
	// A grandchild that inherited stderr (a tool process, a stdio MCP
	// server) would otherwise hold Wait, and the conversation's stop, open
	// after the child itself exited.
	cmd.WaitDelay = streamWaitDelay
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", filepath.Base(cmd.Path), err)
	}
	p := &streamProcess{
		cmd: cmd, stdin: stdin, proto: proto,
		events:  make(chan Event, 256),
		closing: make(chan struct{}),
		exited:  make(chan struct{}),
		read:    make(chan struct{}),
		waiters: map[string]chan controlAnswer{},
	}
	go func() {
		defer close(p.read)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64<<10), 64<<20)
		for sc.Scan() {
			for _, ev := range mapper.mapLine(sc.Bytes(), p) {
				if ev.Fatal != nil {
					// A refused launch never gets to run a tool call.
					_ = cmd.Process.Kill()
				}
				select {
				case p.events <- ev:
				case <-p.closing:
				}
			}
		}
	}()
	go func() {
		<-p.read
		err := cmd.Wait()
		if errors.Is(err, exec.ErrWaitDelay) {
			// The child exited cleanly; only its stderr was still held.
			err = nil
		}
		close(p.exited)
		select {
		case <-p.closing:
		default:
			if err != nil {
				msg := strings.TrimSpace(stderr.String())
				if msg == "" {
					msg = err.Error()
				}
				select {
				case p.events <- Event{Fatal: fmt.Errorf("%s exited: %s", filepath.Base(cmd.Path), msg)}:
				case <-p.closing:
				}
			}
		}
		close(p.events)
	}()
	return p, nil
}

func (p *streamProcess) Events() <-chan Event { return p.events }

func (p *streamProcess) writeLine(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	_, err = p.stdin.Write(append(raw, '\n'))
	return err
}

func (p *streamProcess) Send(text string) error { return p.writeLine(p.proto.userLine(text)) }

func (p *streamProcess) send(v any) { go func() { _ = p.writeLine(v) }() }

// streamWaitDelay bounds how long an exited child's output pipes are read.
const streamWaitDelay = 2 * time.Second

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
		return err
	}
	select {
	case a := <-ch:
		if !a.ok {
			if strings.Contains(strings.ToLower(a.err), "unsupported") {
				return fmt.Errorf("%w: %s", ErrUnsupported, a.err)
			}
			return errors.New(a.err)
		}
		return nil
	case <-p.exited:
		return errors.New("the process exited")
	case <-time.After(controlTimeout):
		return fmt.Errorf("no answer to %s within %s", name, controlTimeout)
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
		stopChild(p.cmd, p.stdin.Close, p.exited)
	})
	for range p.events {
	}
	return nil
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
	Name      string `json:"name"`
	StartedAt int64  `json:"startedAt"`
	EndedAt   int64  `json:"endedAt,omitempty"`
	Failed    bool   `json:"failed,omitempty"`
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
		w.Calls = append(w.Calls, wireCall{Name: c.name, StartedAt: c.startedAt, EndedAt: c.endedAt, Failed: c.failed})
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
