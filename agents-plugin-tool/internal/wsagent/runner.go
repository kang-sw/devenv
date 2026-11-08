package wsagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// KeyStore mints and keeps the ws session key of each conversation. The
// daemon command supplies ws-mcp's key store, which this package cannot
// import.
type KeyStore interface {
	// Mint creates a new key rooted at root.
	Mint(root string) (string, error)
	// Ensure keeps key usable for root: it refreshes the key's record, or
	// writes it again when the key store pruned it.
	Ensure(root, key string) error
}

// Env names the runner sets on an attached ws-mcp.
const (
	// EnvAllowedTools is ws-mcp's tool allowlist: a comma list of ws tool
	// names; tools/list advertises and tools/call accepts only these.
	EnvAllowedTools = "WS_MCP_ALLOWED_TOOLS"
	// EnvNoDashboard turns ws-mcp's dashboard off (no daemon ensure, no
	// session registration), whatever its config resolves.
	EnvNoDashboard = "WS_MCP_NO_DASHBOARD"
)

// Options configures a Runner.
type Options struct {
	// Dir is the dashboard daemon's state directory; conversations live in
	// its session registry (wsdashboard.SessionDir).
	Dir      string
	Backends map[string]Backend
	Keys     KeyStore
	// Config is the base of each profile resolution; RepoRoot is replaced by
	// the conversation's working root.
	Config wsconfig.Options
	// WSMCP is the stdio ws-mcp command serving root, without the runner's
	// environment; nil refuses profiles that allow ws tools.
	WSMCP func(root string) (MCPCommand, error)
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
	// Log receives diagnostics; nil discards them.
	Log io.Writer
	// heartbeat overrides wslive.HeartbeatInterval for tests.
	heartbeat time.Duration
}

// Runner hosts the agent conversations of one dashboard daemon.
type Runner struct {
	opts   Options
	notify func(replyID, project string)

	mu     sync.Mutex
	convs  map[string]*conv
	closed map[string]Record
	quit   chan struct{}
	wg     sync.WaitGroup
	once   sync.Once
}

// NewRunner returns a runner; Start loads its conversations.
func NewRunner(opts Options) *Runner {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Log == nil {
		opts.Log = io.Discard
	}
	if opts.heartbeat <= 0 {
		opts.heartbeat = wslive.HeartbeatInterval
	}
	return &Runner{opts: opts, convs: map[string]*conv{}, closed: map[string]Record{}, quit: make(chan struct{}), notify: func(string, string) {}}
}

// Start loads every conversation record from the session registry, with no
// process (lazy resume), and starts serving them. notify is called after
// each write to a conversation's stores, so the daemon re-reads it. The
// daemon calls Start once it holds its lock.
func (r *Runner) Start(notify func(replyID, project string)) {
	if notify != nil {
		r.notify = notify
	}
	entries, _ := os.ReadDir(filepath.Join(r.opts.Dir, "sessions"))
	for _, e := range entries {
		if !e.IsDir() || !wsdashboard.ValidReplyID(e.Name()) {
			continue
		}
		dir := wsdashboard.SessionDir(r.opts.Dir, e.Name())
		rec, err := readRecord(dir)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(r.opts.Log, "agent: skip %s: %v\n", e.Name(), err)
			}
			continue
		}
		r.mu.Lock()
		if rec.ClosedAt != "" {
			r.closed[rec.ReplyID] = rec
		} else {
			r.startConv(rec)
		}
		r.mu.Unlock()
	}
}

// Close ends every conversation's process and stops the runner; the
// conversations stay open on disk and resume lazily under the next daemon.
func (r *Runner) Close() {
	r.once.Do(func() { close(r.quit) })
	r.wg.Wait()
}

// InboxChanged tells the runner a conversation's inbox changed from the
// session page (a message, cancel, steer, or interrupt), so it delivers or
// interrupts as for its own routes. An unknown reply-id is ignored.
func (r *Runner) InboxChanged(replyID string) {
	if c := r.conv(replyID); c != nil {
		c.do(func() {
			c.checkInterrupt()
			c.kick()
		})
	}
}

func (r *Runner) conv(replyID string) *conv {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.convs[replyID]
}

// startConv starts serving rec; r.mu is held.
func (r *Runner) startConv(rec Record) *conv {
	dir := wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID)
	c := &conv{
		r:     r,
		dir:   dir,
		inbox: wsinbox.Dir(dir),
		rec:   rec,
		live:  newLiveWriter(dir, rec.WSKey, rec.Backend, r.opts.Now, r.opts.Log),
		reqs:  make(chan func()),
		done:  make(chan struct{}),
	}
	r.convs[rec.ReplyID] = c
	c.writeRegistration()
	c.live.writeSnapshot()
	r.wg.Add(1)
	go c.loop()
	return c
}

// requestError is an error a route answers with its status.
type requestError struct {
	status int
	msg    string
}

func (e *requestError) Error() string { return e.msg }

func badRequest(format string, args ...any) error {
	return &requestError{status: http.StatusBadRequest, msg: fmt.Sprintf(format, args...)}
}

var errUnknown = &requestError{status: http.StatusNotFound, msg: "unknown agent conversation"}
var errClosed = &requestError{status: http.StatusConflict, msg: "the conversation is closed"}

// Create opens a conversation of profile bound to root. It resolves and
// checks the profile (backend, instruction file) now, so a bad launch fails
// here; the process itself starts on the first send.
func (r *Runner) Create(profile, root string) (State, error) {
	root = strings.TrimSpace(root)
	if !filepath.IsAbs(root) {
		return State{}, badRequest("root must be an absolute path; got %q", root)
	}
	root = filepath.Clean(root)
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return State{}, badRequest("root %s is not a directory", root)
	}
	rp, err := r.resolve(profile, root)
	if err != nil {
		return State{}, err
	}
	key, err := r.opts.Keys.Mint(root)
	if err != nil {
		return State{}, fmt.Errorf("mint ws session key: %w", err)
	}
	now := r.opts.Now()
	rec := Record{
		ReplyID:   wsmailbox.ReplyID(key),
		Profile:   rp.Name,
		Backend:   rp.Definition.Backend,
		Root:      root,
		Model:     rp.Definition.Model,
		Effort:    rp.Definition.Effort,
		WSKey:     key,
		Listed:    rp.Definition.ListInIndex,
		CreatedAt: now.UTC().Format(time.RFC3339),
	}
	if !wsdashboard.ValidReplyID(rec.ReplyID) {
		return State{}, fmt.Errorf("minted key gives an invalid reply-id %q", rec.ReplyID)
	}
	dir := wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return State{}, err
	}
	if err := writeRecord(dir, rec); err != nil {
		return State{}, err
	}
	r.mu.Lock()
	c := r.startConv(rec)
	r.mu.Unlock()
	return c.state(), nil
}

// resolve resolves profile through root and checks everything a launch
// needs: a backend this daemon has, an existing instruction file, and a
// ws-mcp command when the profile allows ws tools.
func (r *Runner) resolve(profile, root string) (ResolvedProfile, error) {
	rp, err := ResolveProfile(r.opts.Config, root, strings.TrimSpace(profile))
	if err != nil {
		return ResolvedProfile{}, badRequest("%v", err)
	}
	if r.opts.Backends[rp.Definition.Backend] == nil {
		return ResolvedProfile{}, badRequest("agent profile %q: backend %q is not available in this daemon", rp.Name, rp.Definition.Backend)
	}
	if path := rp.Definition.InstructionsPath(root); path != "" {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return ResolvedProfile{}, badRequest("agent profile %q: instruction file %s does not exist", rp.Name, path)
		}
	}
	if len(rp.Definition.WSTools) > 0 && r.opts.WSMCP == nil {
		return ResolvedProfile{}, badRequest("agent profile %q allows ws tools, but this daemon cannot attach ws-mcp", rp.Name)
	}
	return rp, nil
}

// Get reads one conversation's state.
func (r *Runner) Get(id string) (State, error) {
	r.mu.Lock()
	c, open := r.convs[id]
	rec, closed := r.closed[id]
	r.mu.Unlock()
	switch {
	case open:
		return c.state(), nil
	case closed:
		return closedState(rec), nil
	}
	return State{}, errUnknown
}

// List reads every conversation's state, oldest first.
func (r *Runner) List() []State {
	r.mu.Lock()
	out := make([]State, 0, len(r.convs)+len(r.closed))
	convs := make([]*conv, 0, len(r.convs))
	for _, c := range r.convs {
		convs = append(convs, c)
	}
	for _, rec := range r.closed {
		out = append(out, closedState(rec))
	}
	r.mu.Unlock()
	for _, c := range convs {
		out = append(out, c.state())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt != out[j].CreatedAt {
			return out[i].CreatedAt < out[j].CreatedAt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Send holds text as the conversation's next user message. It is submitted
// now when no turn runs, else when the running turn ends; held sends merge
// into one message. It reports whether the message is still held.
func (r *Runner) Send(id, text string) (State, error) {
	if strings.TrimSpace(text) == "" {
		return State{}, badRequest("text is empty")
	}
	return r.act(id, func(c *conv) error {
		if _, _, err := wsinbox.Send(c.inbox, wsinbox.Entry{Parts: []string{text}}, r.opts.Now()); err != nil {
			return err
		}
		c.kick()
		return nil
	})
}

// Interrupt stops the running turn, if any, and takes back the held
// messages, returning their text, as the session page's interrupt does.
func (r *Runner) Interrupt(id string) (State, []string, error) {
	var returned []string
	st, err := r.act(id, func(c *conv) error {
		held, _, err := wsinbox.Interrupt(c.inbox, r.opts.Now())
		if err != nil {
			return err
		}
		for _, rec := range held {
			returned = append(returned, rec.Entry.Text())
		}
		c.checkInterrupt()
		return nil
	})
	return st, returned, err
}

// Stop ends the conversation's process; held messages stay held, and the
// next send resumes the backend session.
func (r *Runner) Stop(id string) (State, error) {
	return r.act(id, func(c *conv) error {
		c.stopProcess("")
		return nil
	})
}

// CloseConversation ends the conversation for good: its process stops,
// held messages are dropped, later sends are refused, and its session
// directory stays readable.
func (r *Runner) CloseConversation(id string) (State, error) {
	st, err := r.act(id, func(c *conv) error {
		c.close()
		return nil
	})
	if err != nil {
		return st, err
	}
	r.mu.Lock()
	if c, ok := r.convs[id]; ok {
		delete(r.convs, id)
		r.closed[id] = c.record()
	}
	r.mu.Unlock()
	return st, nil
}

// SetSettings changes the conversation's model or effort (nil leaves one
// unchanged, "" returns it to the harness default). The change applies at
// the next turn boundary: in-stream when the backend has a control for it,
// else by restarting the process on the same backend session.
func (r *Runner) SetSettings(id string, model, effort *string) (State, error) {
	if model != nil && *model != "" && !modelPattern.MatchString(*model) {
		return State{}, badRequest("model %q must match %s", *model, modelPattern.String())
	}
	if effort != nil && *effort != "" && !effortPattern.MatchString(*effort) {
		return State{}, badRequest("effort %q must be a lowercase word", *effort)
	}
	return r.act(id, func(c *conv) error {
		return c.setSettings(model, effort)
	})
}

// act runs fn in the conversation's loop and returns its state after.
func (r *Runner) act(id string, fn func(c *conv) error) (State, error) {
	r.mu.Lock()
	c, open := r.convs[id]
	_, closed := r.closed[id]
	r.mu.Unlock()
	if !open {
		if closed {
			return State{}, errClosed
		}
		return State{}, errUnknown
	}
	var err error
	if !c.do(func() {
		if c.closed {
			err = errClosed
			return
		}
		err = fn(c)
	}) {
		return State{}, errClosed
	}
	if err != nil {
		return State{}, err
	}
	return c.state(), nil
}

// Conversation statuses a State reports.
const (
	StatusIdle    = "idle"
	StatusRunning = "running"
	StatusClosed  = "closed"
)

// State is one conversation as the REST routes report it. It never carries
// the ws session key.
type State struct {
	// ID is the conversation's reply-id: its session directory's name and
	// its dashboard page (/s/<id>).
	ID      string `json:"id"`
	Profile string `json:"profile"`
	Backend string `json:"backend"`
	Root    string `json:"root"`
	// Status is idle (no turn runs), running (a turn runs), or closed.
	Status string `json:"status"`
	// Process reports a live backend process.
	Process bool   `json:"process"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
	// Held counts the messages waiting for the next turn.
	Held int `json:"held"`
	// Page is the conversation's dashboard page path.
	Page string `json:"page"`
	// Error is the last launch or turn failure; cleared when a turn starts.
	Error     string `json:"error,omitempty"`
	CreatedAt string `json:"created_at"`
	ClosedAt  string `json:"closed_at,omitempty"`
}

func closedState(rec Record) State {
	return State{ID: rec.ReplyID, Profile: rec.Profile, Backend: rec.Backend, Root: rec.Root, Status: StatusClosed, Model: rec.Model, Effort: rec.Effort, Page: "/s/" + rec.ReplyID, CreatedAt: rec.CreatedAt, ClosedAt: rec.ClosedAt}
}

// conv is one open conversation. Its loop goroutine owns the process and
// every write; other goroutines read through state() and act through do().
type conv struct {
	r     *Runner
	dir   string
	inbox string
	live  *liveWriter
	reqs  chan func()
	done  chan struct{}

	// mu guards what state() reads: rec and the status fields below.
	mu      sync.Mutex
	rec     Record
	proc    Process
	turn    bool
	lastErr string
	closed  bool

	// loop-owned.
	events      <-chan Event
	idle        *time.Timer
	idleAfter   time.Duration
	dirtyModel  bool
	dirtyEffort bool
	lastReg     time.Time
}

// do runs fn in the loop and waits for it; false when the loop has ended.
func (c *conv) do(fn func()) bool {
	ran := make(chan struct{})
	select {
	case c.reqs <- func() { fn(); close(ran) }:
		<-ran
		return true
	case <-c.done:
		return false
	}
}

func (c *conv) loop() {
	defer c.r.wg.Done()
	defer close(c.done)
	beat := time.NewTicker(c.r.opts.heartbeat)
	defer beat.Stop()
	for {
		var idleC <-chan time.Time
		if c.idle != nil {
			idleC = c.idle.C
		}
		select {
		case fn := <-c.reqs:
			fn()
		case ev, ok := <-c.events:
			if !ok {
				c.processExited()
			} else {
				c.handle(ev)
			}
		case <-idleC:
			c.idle = nil
			if !c.turn {
				c.stopProcess("")
			}
		case <-beat.C:
			c.live.writeSnapshot()
			if c.r.opts.Now().Sub(c.lastReg) >= wsdashboard.HeartbeatInterval {
				c.writeRegistration()
			}
		case <-c.r.quit:
			c.stopProcess("")
			return
		}
		if c.closed {
			return
		}
	}
}

func (c *conv) state() State {
	held, _ := wsinbox.Held(c.inbox)
	c.mu.Lock()
	defer c.mu.Unlock()
	st := closedState(c.rec)
	st.Status, st.ClosedAt = StatusIdle, ""
	if c.turn {
		st.Status = StatusRunning
	}
	if c.closed {
		st.Status, st.ClosedAt = StatusClosed, c.rec.ClosedAt
	}
	st.Process = c.proc != nil
	st.Held = len(held)
	st.Error = c.lastErr
	return st
}

func (c *conv) record() Record {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rec
}

// update changes the record under mu and persists it.
func (c *conv) update(fn func(rec *Record)) {
	c.mu.Lock()
	fn(&c.rec)
	rec := c.rec
	c.mu.Unlock()
	if err := writeRecord(c.dir, rec); err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: record write: %v\n", rec.ReplyID, err)
	}
}

func (c *conv) setStatus(fn func()) {
	c.mu.Lock()
	fn()
	c.mu.Unlock()
}

func (c *conv) writeRegistration() {
	rec := c.record()
	now := c.r.opts.Now()
	if err := wsdashboard.WriteRegistration(c.r.opts.Dir, registration(rec, now)); err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: registration write: %v\n", rec.ReplyID, err)
	}
	c.lastReg = now
	c.r.notify(rec.ReplyID, rec.Root)
}

// apply applies a wslive event and tells the daemon when it wrote.
func (c *conv) apply(ev wslive.Event) {
	if c.live.apply(ev) {
		c.r.notify(c.rec.ReplyID, c.rec.Root)
	}
}

// kick submits the held messages as the next turn when no turn runs,
// starting the process first when there is none. A failed start leaves the
// messages held.
func (c *conv) kick() {
	if c.closed || c.turn {
		return
	}
	held, err := wsinbox.Held(c.inbox)
	if err != nil || len(held) == 0 {
		return
	}
	if c.proc == nil {
		if err := c.startProcess(); err != nil {
			c.setStatus(func() { c.lastErr = err.Error() })
			fmt.Fprintf(c.r.opts.Log, "agent %s: start: %v\n", c.rec.ReplyID, err)
			c.live.writeSnapshot()
			c.r.notify(c.rec.ReplyID, c.rec.Root)
			return
		}
	}
	recs, err := wsinbox.Drain(c.inbox)
	var parts []string
	for _, rec := range recs {
		if text := strings.TrimSpace(rec.Entry.Text()); text != "" {
			parts = append(parts, text)
		}
	}
	if err != nil || len(parts) == 0 {
		// A cancel took the message after the start: the process waits for
		// input like after a turn.
		c.armIdle()
		return
	}
	text := strings.Join(parts, "\n\n")
	c.stopIdle()
	c.setStatus(func() { c.turn, c.lastErr = true, "" })
	c.apply(wslive.Event{Kind: wslive.KindUser, Origin: wslive.OriginWhiteboard, Text: text})
	c.apply(wslive.Event{Kind: wslive.KindTurnStart, Origin: wslive.OriginWhiteboard})
	if err := c.proc.Send(text); err != nil {
		c.setStatus(func() { c.lastErr = "send turn: " + err.Error() })
		c.stopProcess("")
	}
}

// startProcess resolves the profile again (it may have changed since the
// conversation opened) and starts the backend process, resuming the
// recorded backend session.
func (c *conv) startProcess() error {
	rec := c.record()
	rp, err := c.r.resolve(rec.Profile, rec.Root)
	if err != nil {
		return err
	}
	if rp.Definition.Backend != rec.Backend {
		return fmt.Errorf("agent profile %q now selects backend %q; this conversation runs on %q", rec.Profile, rp.Definition.Backend, rec.Backend)
	}
	if err := c.r.opts.Keys.Ensure(rec.Root, rec.WSKey); err != nil {
		return fmt.Errorf("ws session key: %w", err)
	}
	instructions, err := composeInstructions(c.dir, rec, rp.Definition)
	if err != nil {
		return err
	}
	spec := StartSpec{
		Root:         rec.Root,
		Dir:          c.dir,
		Profile:      rp.Definition,
		Model:        rec.Model,
		Effort:       rec.Effort,
		SessionID:    rec.SessionID,
		Instructions: instructions,
		Env:          environ(),
	}
	if len(rp.Definition.WSTools) > 0 {
		cmd, err := c.r.opts.WSMCP(rec.Root)
		if err != nil {
			return fmt.Errorf("ws-mcp command: %w", err)
		}
		env := map[string]string{}
		for k, v := range cmd.Env {
			env[k] = v
		}
		env[EnvAllowedTools] = strings.Join(rp.Definition.WSTools, ",")
		env[EnvNoDashboard] = "1"
		cmd.Env = env
		spec.WSMCP = &cmd
	}
	proc, err := c.r.opts.Backends[rec.Backend].Start(context.Background(), spec)
	if err != nil {
		return err
	}
	c.setStatus(func() { c.proc = proc })
	c.events = proc.Events()
	c.idleAfter = rp.Definition.IdleTimeout()
	c.dirtyModel, c.dirtyEffort = false, false
	if rp.Definition.ListInIndex != rec.Listed {
		c.update(func(rec *Record) { rec.Listed = rp.Definition.ListInIndex })
		c.writeRegistration()
	}
	return nil
}

// composeInstructions writes the instruction file a process appends to the
// harness's own system prompt: the profile's instruction file and, when the
// profile allows ws tools, the conversation's ws session key. It returns ""
// when there is nothing to append.
func composeInstructions(dir string, rec Record, p Profile) (string, error) {
	var parts []string
	if path := p.InstructionsPath(rec.Root); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("agent profile %q: read instruction file: %w", rec.Profile, err)
		}
		parts = append(parts, strings.TrimSpace(string(raw)))
	}
	if len(p.WSTools) > 0 {
		parts = append(parts, fmt.Sprintf("Your ws session key is `%s`. Pass it as the `session_key` argument of every ws tool call.", rec.WSKey))
	}
	if len(parts) == 0 {
		return "", nil
	}
	path := filepath.Join(dir, "instructions.md")
	if err := writeFileAtomic(path, []byte(strings.Join(parts, "\n\n")+"\n"), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (c *conv) handle(ev Event) {
	switch {
	case ev.SessionID != "":
		if ev.SessionID != c.rec.SessionID {
			c.update(func(rec *Record) { rec.SessionID = ev.SessionID })
		}
	case ev.Live != nil:
		c.apply(*ev.Live)
	case ev.Fatal != nil:
		c.setStatus(func() { c.lastErr = ev.Fatal.Error() })
		fmt.Fprintf(c.r.opts.Log, "agent %s: %v\n", c.rec.ReplyID, ev.Fatal)
		c.stopProcess("")
	case ev.TurnEnd:
		c.endTurn(ev.TurnError)
	}
}

// endTurn closes the running turn, applies settings changed during it, and
// submits the messages held meanwhile.
func (c *conv) endTurn(turnErr string) {
	if !c.turn {
		return
	}
	c.setStatus(func() {
		c.turn = false
		if turnErr != "" {
			c.lastErr = turnErr
		}
	})
	c.apply(wslive.Event{Kind: wslive.KindTurnEnd})
	c.applySettings()
	c.kick()
	if !c.turn {
		c.armIdle()
	}
}

// processExited handles a process whose output ended without the runner
// stopping it.
func (c *conv) processExited() {
	c.events = nil
	if c.proc != nil {
		_ = c.proc.Close()
	}
	c.setStatus(func() {
		c.proc = nil
		if c.lastErr == "" {
			c.lastErr = "the backend process exited"
		}
	})
	c.stopIdle()
	if c.turn {
		c.setStatus(func() { c.turn = false })
		c.apply(wslive.Event{Kind: wslive.KindTurnEnd})
	}
	c.live.writeSnapshot()
	c.r.notify(c.rec.ReplyID, c.rec.Root)
}

// stopProcess ends the process, if any; a running turn ends with it.
func (c *conv) stopProcess(string) {
	c.stopIdle()
	if c.proc == nil {
		return
	}
	proc := c.proc
	c.events = nil
	_ = proc.Close()
	c.setStatus(func() { c.proc = nil })
	if c.turn {
		c.setStatus(func() { c.turn = false })
		c.apply(wslive.Event{Kind: wslive.KindTurnEnd})
	}
	c.live.writeSnapshot()
	c.r.notify(c.rec.ReplyID, c.rec.Root)
}

// checkInterrupt takes the inbox's interrupt requests and interrupts the
// running turn when there is one; with no turn they are dropped.
func (c *conv) checkInterrupt() {
	recs, err := wsinbox.DrainInterrupt(c.inbox)
	if err != nil || len(recs) == 0 || !c.turn || c.proc == nil {
		return
	}
	if err := c.proc.Interrupt(); err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: interrupt: %v\n", c.rec.ReplyID, err)
		c.stopProcess("")
	}
}

func (c *conv) setSettings(model, effort *string) error {
	c.update(func(rec *Record) {
		if model != nil && *model != rec.Model {
			rec.Model = *model
			c.dirtyModel = true
		}
		if effort != nil && *effort != rec.Effort {
			rec.Effort = *effort
			c.dirtyEffort = true
		}
	})
	if !c.turn {
		c.applySettings()
	}
	return nil
}

// applySettings carries changed settings to the process at a turn
// boundary: in-stream when the backend can, else by stopping the process so
// the next turn restarts it with the new values on the same backend
// session.
func (c *conv) applySettings() {
	model, effort := c.dirtyModel, c.dirtyEffort
	c.dirtyModel, c.dirtyEffort = false, false
	if c.proc == nil || (!model && !effort) {
		return
	}
	rec := c.record()
	restart := false
	if model && c.proc.SetModel(rec.Model) != nil {
		restart = true
	}
	if effort && !restart && c.proc.SetEffort(rec.Effort) != nil {
		restart = true
	}
	if restart {
		c.stopProcess("")
	}
}

func (c *conv) armIdle() {
	c.stopIdle()
	if c.proc != nil && c.idleAfter > 0 {
		c.idle = time.NewTimer(c.idleAfter)
	}
}

func (c *conv) stopIdle() {
	if c.idle != nil {
		c.idle.Stop()
		c.idle = nil
	}
}

// close ends the conversation for good.
func (c *conv) close() {
	c.stopProcess("")
	if held, err := wsinbox.Held(c.inbox); err == nil {
		for _, rec := range held {
			_, _ = wsinbox.Cancel(c.inbox, rec.Name)
		}
	}
	_, _ = wsinbox.DrainInterrupt(c.inbox)
	c.update(func(rec *Record) { rec.ClosedAt = c.r.opts.Now().UTC().Format(time.RFC3339) })
	c.setStatus(func() { c.closed = true })
	c.live.setAdapter("")
	c.writeRegistration()
}
