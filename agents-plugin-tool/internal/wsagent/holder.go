package wsagent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsholder"
)

// A lead conversation's process runs under a session holder
// (internal/wsholder), so it outlives the daemon: the holder owns the child
// and keeps its output in a ring, and each daemon attaches and replays.
// One holder serves one process; a stop, a settings change, or an exit ends
// the holder with it, and the next start launches a new one.

// HolderHost is the session holder manager as the runner uses it
// (ManagerHost over *wsholder.Manager).
type HolderHost interface {
	// AddSink adds a receiver of every attached holder's events; the runner
	// adds itself before the first scan.
	AddSink(sink wsholder.Sink)
	// Scan attaches to every holder not attached yet.
	Scan()
	List() wsholder.Listing
	// LaunchWith starts a holder that records spec.Session in its
	// descriptor and returns its id.
	LaunchWith(spec wsholder.LaunchSpec) (string, error)
	Spawn(id string, spec wsholder.SpawnSpec) (wsholder.ChildStatus, error)
	Input(id string, data []byte, eof bool) error
	Status(id string) (wsholder.AttachInfo, error)
	// Stop ends holder id and its child, and returns once both are gone.
	Stop(id string) error
}

// ManagerHost is *wsholder.Manager as a HolderHost.
type ManagerHost struct{ *wsholder.Manager }

func (h ManagerHost) Spawn(id string, spec wsholder.SpawnSpec) (wsholder.ChildStatus, error) {
	c := h.Client(id)
	if c == nil {
		return wsholder.ChildStatus{}, wsholder.ErrDetached
	}
	return c.Spawn(spec)
}

func (h ManagerHost) Input(id string, data []byte, eof bool) error {
	c := h.Client(id)
	if c == nil {
		return wsholder.ErrDetached
	}
	return c.Input(data, eof)
}

func (h ManagerHost) Status(id string) (wsholder.AttachInfo, error) {
	c := h.Client(id)
	if c == nil {
		return wsholder.AttachInfo{}, wsholder.ErrDetached
	}
	return c.Status()
}

// holderKindLead is the kind a lead conversation's holder records in its
// descriptor.
const holderKindLead = "lead"
const holderKindPiWorker = "pi-worker"

// leadRingBytes is the output ring of a lead conversation's holder. A
// rejoin rebuilds the turn state from the ring, so it holds a long turn's
// output whole: one agentic turn echoing large tool results outgrows the
// holder's 4 MiB default. The holder fills it as output comes. Terminal
// holders keep the manager's default.
const leadRingBytes = 32 << 20

// holderSession is the descriptor a lead conversation's holder records.
type holderSession struct {
	Kind    string `json:"kind"`
	ReplyID string `json:"reply_id"`
	Cwd     string `json:"cwd,omitempty"`
	Label   string `json:"label,omitempty"`
	// Capability binds a persisted worker token to this held process.
	Capability string `json:"capability,omitempty"`
}

// holderTable routes the holders' events to the processes they belong to.
type holderTable struct {
	mu    sync.Mutex
	procs map[string]*holderChild
}

func (t *holderTable) add(id string, h *holderChild) {
	t.mu.Lock()
	if t.procs == nil {
		t.procs = map[string]*holderChild{}
	}
	t.procs[id] = h
	t.mu.Unlock()
}

func (t *holderTable) remove(id string, h *holderChild) {
	t.mu.Lock()
	if t.procs[id] == h {
		delete(t.procs, id)
	}
	t.mu.Unlock()
}

// HolderEvent is the runner's wsholder.Sink: an event of a holder that
// serves a conversation goes to its process; any other holder's is not the
// runner's.
func (t *holderTable) HolderEvent(id string, ev wsholder.Event) {
	t.mu.Lock()
	h := t.procs[id]
	t.mu.Unlock()
	if h != nil {
		h.event(ev)
	}
}

// holderChild is a lead process's child under a session holder: the
// streamProcess's transport and the receiver of the holder's records.
type holderChild struct {
	host    HolderHost
	table   *holderTable
	session holderSession
	// id is the holder. Set before start, the process attaches to that
	// running holder instead of spawning (a daemon restart); start fills it
	// for a spawn.
	id string
	// skipTo is the last record sequence the runner applied under an
	// earlier daemon: records up to it are replay.
	skipTo uint64

	proc *streamProcess
	base string
	pid  int

	// The fields below are the event goroutine's, but for target, which
	// replayUntil sets under mu.
	mu        sync.Mutex
	partial   []byte
	last      uint64
	replaying bool
	target    uint64
	targetSet bool
	// wrapped is set by replayUntil when the holder's ring has dropped
	// records, whether while no daemon was attached or before the earlier
	// daemon let go.
	wrapped bool
	stopErr error
}

// start makes h the child of p: it attaches to the holder h.id names, or
// launches a holder and spawns cmd under it.
func (h *holderChild) start(p *streamProcess, cmd *exec.Cmd) error {
	h.proc, h.base = p, filepath.Base(cmd.Path)
	p.child, p.holder = h, h
	if h.id != "" {
		h.replaying = true
		h.table.add(h.id, h)
		return nil
	}
	raw, err := json.Marshal(h.session)
	if err != nil {
		return err
	}
	id, err := h.host.LaunchWith(wsholder.LaunchSpec{Session: raw, RingBytes: leadRingBytes})
	if err != nil {
		return fmt.Errorf("start a session holder: %w", err)
	}
	h.id = id
	// Listed before the spawn, so no record of the child is missed.
	h.table.add(id, h)
	env := map[string]string{}
	for _, kv := range cmd.Env {
		if name, value, ok := strings.Cut(kv, "="); ok && name != "" {
			env[name] = value
		}
	}
	st, err := h.host.Spawn(id, wsholder.SpawnSpec{Argv: append([]string{cmd.Path}, cmd.Args[1:]...), Cwd: cmd.Dir, Env: env, Mode: "pipe"})
	if err != nil {
		h.table.remove(id, h)
		_ = h.host.Stop(id)
		return fmt.Errorf("start %s: %w", h.base, err)
	}
	h.pid = st.PID
	return nil
}

func (h *holderChild) name() string { return h.base }

func (h *holderChild) write(line []byte) error { return h.host.Input(h.id, line, false) }

// kill may run on the holder's reader goroutine, which a stop's answer
// arrives on, so it does not wait.
func (h *holderChild) kill() { go func() { _ = h.host.Stop(h.id) }() }

// stop closes the child's input, waits closeGrace for it to exit on its
// own, then ends the holder, which terminates and finally kills the child.
func (h *holderChild) stop(exited <-chan struct{}) {
	select {
	case <-exited:
	default:
		_ = h.host.Input(h.id, nil, true)
		select {
		case <-exited:
		case <-time.After(closeGrace):
		}
	}
	if err := h.host.Stop(h.id); err != nil && !errors.Is(err, wsholder.ErrUnknownHolder) {
		h.mu.Lock()
		h.stopErr = err
		h.mu.Unlock()
	}
}

func (h *holderChild) release() { h.table.remove(h.id, h) }

// event takes one holder event, on the holder's reader goroutine.
func (h *holderChild) event(ev wsholder.Event) {
	switch ev.Event {
	case "output":
		h.seen(ev.Seq)
		switch ev.Stream {
		case "stderr":
			_, _ = h.proc.stderr.Write(ev.Data)
		case "stdout":
			h.partial = append(h.partial, ev.Data...)
			if len(h.partial) > maxLineBytes {
				h.partial = nil
				h.kill()
				h.proc.deliver(Event{Fatal: fmt.Errorf("reading %s output: a line exceeds %d bytes", h.base, maxLineBytes)})
			} else if !ev.Partial {
				line := bytes.TrimRight(h.partial, "\r\n")
				h.partial = nil
				h.proc.feed(line, ev.Seq, ev.Seq <= h.skipTo)
			}
		}
	case "gap":
		// The records From..To-1 left the ring before they reached this
		// daemon: a line in progress lost its middle and is no line.
		h.partial = nil
		if ev.To > 0 {
			h.seen(ev.To - 1)
		}
		h.proc.deliver(Event{ReplayGap: true})
	case "exit":
		h.seen(ev.Seq)
		var err error
		switch {
		case ev.Signal != nil && *ev.Signal != 0:
			err = fmt.Errorf("signal %d", *ev.Signal)
		case ev.ExitCode != nil && *ev.ExitCode != 0:
			err = fmt.Errorf("exit status %d", *ev.ExitCode)
		}
		h.proc.finish(err)
		return
	}
	h.checkReplayed()
}

func (h *holderChild) seen(seq uint64) {
	h.mu.Lock()
	if seq > h.last {
		h.last = seq
	}
	h.mu.Unlock()
}

// replayUntil tells an attached process the first and the last record
// sequence the holder held when the daemon attached: once the last has been
// delivered the process sends ReplayDone, marked ReplayLost when the ring
// has dropped records (its first is past 1). That covers a ring that
// wrapped while the earlier daemon was still attached, whose first is not
// past the record after skipTo: the replay rebuilds the turn state from the
// ring alone, so a turn start that left it is lost all the same.
func (h *holderChild) replayUntil(first, target uint64) {
	h.mu.Lock()
	h.target, h.targetSet = target, true
	h.wrapped = first > 1
	h.mu.Unlock()
	h.checkReplayed()
}

func (h *holderChild) checkReplayed() {
	h.mu.Lock()
	done := h.replaying && h.targetSet && h.last >= h.target
	if done {
		h.replaying = false
	}
	wrapped := h.wrapped
	h.mu.Unlock()
	if done {
		h.proc.deliver(Event{ReplayDone: true, ReplayLost: wrapped})
	}
}

// HolderInfo is the holder a process runs under and its child's PID (0 for
// an attached process, whose PID the record already holds); ok is false for
// the daemon's own child.
func (p *streamProcess) HolderInfo() (id string, pid int, ok bool) {
	if p.holder == nil {
		return "", 0, false
	}
	return p.holder.id, p.holder.pid, true
}

// ReplayUntil is holderChild.replayUntil for a process under a holder.
func (p *streamProcess) ReplayUntil(first, target uint64) {
	if p.holder != nil {
		p.holder.replayUntil(first, target)
	}
}

// StopError is why the holder could not be ended by Close, nil when it
// ended (or the process is the daemon's own child).
func (p *streamProcess) StopError() error {
	if p.holder == nil {
		return nil
	}
	p.holder.mu.Lock()
	defer p.holder.mu.Unlock()
	return p.holder.stopErr
}

// holderProcess is what the runner reads from a process under a holder;
// claudeProcess and piProcess have it through their streamProcess.
type holderProcess interface {
	HolderInfo() (id string, pid int, ok bool)
	ReplayUntil(first, target uint64)
	StopError() error
	Detach() bool
}
