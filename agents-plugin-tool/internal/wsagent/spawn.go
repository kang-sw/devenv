package wsagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

// spawn.go is the runner's spawned conversations: a non-lead conversation
// another session opens with its first message (Spawn) and steers by its
// id, whose settled turns are reported to that parent (Options.Report),
// waited for (Wait), and watched (Watch).
//
// A turn settles once: when it ends (done, or failed with the harness's
// error), when its process ends under it (failed), when the process it
// needed could not start or take the message (failed), or, for a turn the
// daemon's exit cut off, when the next daemon loads the record
// (interrupted). Closing the conversation settles nothing: the parent that
// closed it needs no report.

// Turn settle statuses (TurnResult.Status). TurnRunning and TurnClosed are
// Wait's alone: the wait ended before a settle, or the conversation is
// closed.
const (
	TurnDone        = "done"
	TurnFailed      = "failed"
	TurnInterrupted = "interrupted"
	TurnRunning     = "running"
	TurnClosed      = "closed"
)

// TurnResult is how a conversation's turn settled.
type TurnResult struct {
	// ID is the conversation's id (its reply-id).
	ID     string `json:"id"`
	Status string `json:"status"`
	// Message is the turn's final assistant message; Error the failure of
	// a failed or interrupted turn.
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	// n counts the conversation's settles so far (0: none yet), and mark
	// is the Runner.Send count the settled turn's messages covered.
	n, mark uint64
}

// TurnReport is one settled turn of a conversation with a parent, for
// Options.Report.
type TurnReport struct {
	Parent ParentRef
	// Child is the conversation's state after the settle.
	Child  State
	Result TurnResult
}

// SpawnSpec is a spawned conversation's launch: Runner.Spawn.
type SpawnSpec struct {
	// Profile is a non-lead agent profile; Root the conversation's working
	// root (absolute), which its ws session key is bound to.
	Profile string
	Root    string
	// Cwd is the directory its process runs in: absolute, an existing
	// directory; empty is Root.
	Cwd string
	// Model and Effort replace the profile's; empty keeps it.
	Model  string
	Effort string
	// Prompt is the first user message; required.
	Prompt string
	// Label is the conversation's one-line name; empty is none.
	Label string
	// Parent is the session the settled turns are reported to; nil
	// reports none.
	Parent *ParentRef
}

// apply sets the spawn's choices on a new conversation's record.
func (spec *SpawnSpec) apply(rec *Record) {
	if spec.Cwd != rec.Root {
		rec.Cwd = spec.Cwd
	}
	if spec.Model != "" {
		rec.Model = spec.Model
	}
	if spec.Effort != "" {
		rec.Effort = spec.Effort
	}
	rec.Label = spec.Label
	if spec.Parent != nil {
		parent := *spec.Parent
		rec.Parent = &parent
	}
}

// labelRunes bounds SpawnSpec.Label.
const labelRunes = 120

// Spawn opens a non-lead conversation of spec.Profile and sends it
// spec.Prompt, in one call: the conversation either exists with its first
// message held or is not opened at all. The process starts as for any
// first send; a launch failure settles the turn as failed.
func (r *Runner) Spawn(spec SpawnSpec) (State, error) {
	if strings.TrimSpace(spec.Prompt) == "" {
		return State{}, badRequest("prompt is empty")
	}
	if spec.Model != "" && !modelPattern.MatchString(spec.Model) {
		return State{}, badRequest("model %q must match %s", spec.Model, modelPattern.String())
	}
	if spec.Effort != "" && !effortPattern.MatchString(spec.Effort) {
		return State{}, badRequest("effort %q must be a lowercase word", spec.Effort)
	}
	if p := spec.Parent; p != nil && (strings.TrimSpace(p.Kind) == "" || !wsdashboard.ValidReplyID(p.ReplyID)) {
		return State{}, badRequest("parent must name a kind and a valid reply-id")
	}
	spec.Root = strings.TrimSpace(spec.Root)
	if !filepath.IsAbs(spec.Root) {
		return State{}, badRequest("root must be an absolute path; got %q", spec.Root)
	}
	spec.Root = filepath.Clean(spec.Root)
	cwd := strings.TrimSpace(spec.Cwd)
	if cwd == "" {
		cwd = spec.Root
	}
	if !filepath.IsAbs(cwd) {
		return State{}, badRequest("cwd must be an absolute path; got %q", cwd)
	}
	cwd = filepath.Clean(cwd)
	if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		return State{}, badRequest("cwd %s is not a directory", cwd)
	}
	spec.Cwd = cwd
	spec.Label = cleanLabel(spec.Label)
	return r.create(spec.Profile, spec.Root, nil, &spec)
}

// cleanLabel is label's first non-blank line, at most labelRunes runes.
func cleanLabel(label string) string {
	for _, line := range strings.Split(label, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			if runes := []rune(line); len(runes) > labelRunes {
				line = string(runes[:labelRunes-1]) + "…"
			}
			return line
		}
	}
	return ""
}

// spawned sends a new spawned conversation its first message. A failed send
// closes the conversation again, so a spawn that fails leaves nothing
// open.
func (r *Runner) spawned(c *conv, prompt string) (State, error) {
	st, err := r.Send(c.rec.ReplyID, prompt)
	if err != nil {
		_, _ = r.CloseConversation(c.rec.ReplyID)
		return State{}, err
	}
	c.watched()
	return st, nil
}

// sendCount is Runner.Send's count so far.
func (c *conv) sendCount() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sends
}

// lastError is State.Error.
func (c *conv) lastError() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastErr
}

// settleTurn records a settled turn whose messages covered mark sends, wakes
// the waits, and reports it to the parent. The loop calls it.
func (c *conv) settleTurn(status, errText string, mark uint64) {
	res := TurnResult{ID: c.rec.ReplyID, Status: status, Error: errText, mark: mark}
	if status == TurnDone || status == TurnFailed {
		res.Message = c.reply
	}
	c.reply = ""
	c.mu.Lock()
	res.n = c.settled.n + 1
	c.settled = res
	c.mu.Unlock()
	c.signalSettled()
	if c.rec.TurnOpen {
		c.update(func(rec *Record) { rec.TurnOpen = false })
	}
	parent := c.rec.Parent
	if parent == nil {
		return
	}
	st := c.state()
	if report := c.r.opts.Report; report != nil {
		report(TurnReport{Parent: *parent, Child: st, Result: res})
	}
	c.r.emit(st)
}

// interrupted settles the turn a daemon's exit cut off, which the record
// still marks open; the loop calls it once, when the record is loaded.
func (c *conv) interrupted() {
	if c.closed || !c.rec.TurnOpen {
		return
	}
	c.settleTurn(TurnInterrupted, "the dashboard daemon exited while the turn ran", 0)
}

// signalSettled wakes every Wait.
func (c *conv) signalSettled() {
	c.mu.Lock()
	close(c.settleC)
	c.settleC = make(chan struct{})
	c.mu.Unlock()
}

// watched tells the watchers a conversation with a parent changed.
func (c *conv) watched() {
	if c.record().Parent != nil {
		c.r.emit(c.state())
	}
}

// Watch calls fn with a spawned conversation's state (one with a parent)
// whenever it opens, starts or settles a turn, or closes, until stop is
// called. fn runs on the conversation's loop or on the caller of Spawn or
// CloseConversation: it must return promptly and must not call the runner.
func (r *Runner) Watch(fn func(State)) (stop func()) {
	key := &fn
	r.watchMu.Lock()
	if r.watchers == nil {
		r.watchers = map[*func(State)]struct{}{}
	}
	r.watchers[key] = struct{}{}
	r.watchMu.Unlock()
	return func() {
		r.watchMu.Lock()
		delete(r.watchers, key)
		r.watchMu.Unlock()
	}
}

func (r *Runner) emit(st State) {
	r.watchMu.Lock()
	fns := make([]func(State), 0, len(r.watchers))
	for fn := range r.watchers {
		fns = append(fns, *fn)
	}
	r.watchMu.Unlock()
	for _, fn := range fns {
		fn(st)
	}
}

// Children lists the conversations, open or closed, that parent spawned,
// oldest first.
func (r *Runner) Children(parent ParentRef) []State {
	var out []State
	for _, st := range r.List() {
		if st.Parent != nil && *st.Parent == parent {
			out = append(out, st)
		}
	}
	return out
}

// Wait blocks until a turn of conversation id that took every message
// Runner.Send had sent it when Wait was called settles, and returns how it
// settled; a turn that settled so before the call is returned at once. It
// returns TurnRunning when timeout passes or ctx ends first, and TurnClosed
// for a closed conversation. A turn the conversation's harness began on its
// own holds no new message, so it never answers a wait for a sent one.
func (r *Runner) Wait(ctx context.Context, id string, timeout time.Duration) (TurnResult, error) {
	r.mu.Lock()
	c, open := r.convs[id]
	_, closed := r.closed[id]
	r.mu.Unlock()
	switch {
	case closed:
		return TurnResult{ID: id, Status: TurnClosed}, nil
	case !open:
		return TurnResult{}, errUnknown
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	target := c.sendCount()
	for {
		c.mu.Lock()
		res, ch, ended := c.settled, c.settleC, c.closed
		c.mu.Unlock()
		if res.n > 0 && res.mark >= target {
			return res, nil
		}
		if ended {
			return TurnResult{ID: id, Status: TurnClosed}, nil
		}
		select {
		case <-ch:
		case <-c.done:
			return TurnResult{ID: id, Status: TurnClosed}, nil
		case <-timer.C:
			return TurnResult{ID: id, Status: TurnRunning}, nil
		case <-ctx.Done():
			return TurnResult{ID: id, Status: TurnRunning}, nil
		}
	}
}

// Transcript reads conversation id's conversation ring, oldest first.
func (r *Runner) Transcript(id string) ([]wslive.RingRecord, error) {
	r.mu.Lock()
	_, open := r.convs[id]
	_, closed := r.closed[id]
	r.mu.Unlock()
	if !open && !closed {
		return nil, errUnknown
	}
	return wslive.ReadRing(wslive.RingDir(wsdashboard.SessionDir(r.opts.Dir, id)))
}
