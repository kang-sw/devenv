package wsagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
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

// LeadKeyStore mints and keeps the lead ws session key of each hosted lead
// session (Profile.Lead); ws-mcp's key store implements it.
type LeadKeyStore interface {
	// LeadRoot is root's canonical git root, where a lead key lives; it
	// fails for a folder outside git.
	LeadRoot(root string) (string, error)
	// MintLead creates a parent-less lead key at root's canonical git root.
	MintLead(root string) (string, error)
	// EnsureLead keeps the lead key usable for its canonical root, as
	// KeyStore.Ensure does for a conversation key.
	EnsureLead(canonical, key string) error
	// LeadSession finds the lead key behind a registered session (the
	// parent-less lead key rooted at root whose reply-id is replyID) with
	// the harness conversation its last login reported, so the daemon can
	// take the session over under the same key. ok is false when no such
	// key is left.
	LeadSession(root, replyID string) (session LeadSession, ok bool)
}

// LeadSession is a terminal lead session as its ws key record describes it.
type LeadSession struct {
	// Key is the session's lead key and Root its canonical git root.
	Key, Root string
	// Harness is the harness that last logged in with the key, Session its
	// own name for the conversation (a claude session id, a pi session
	// file path), and Cwd the directory it ran in. All three are empty for
	// a key whose logins reported none.
	Harness, Session, Cwd string
}

// LeadMail is a hosted lead session's ws mailbox as the runner uses it: the
// daemon, not the session's own mailbox wait, delivers its mail.
type LeadMail interface {
	// Watch calls changed whenever the mailbox of key may hold new mail,
	// from any goroutine, until the returned stop is called. While watched,
	// the session reads to its peers as listening.
	Watch(root, key string, changed func()) (stop func())
	// Drain claims every message waiting for key and returns each as the
	// text the session's model reads for it.
	Drain(root, key string) ([]string, error)
	// Return puts texts, mail a Drain claimed and the session never read,
	// back in key's mailbox for its next claim: the session moved to a
	// terminal harness, whose own mailbox wait delivers it.
	Return(root, key string, texts []string) error
}

// Env names the runner sets on a hosted lead session's harness. The flag
// and the key are what the hosted-session mode reads; the row is the
// session's registration file, whose host_pid names the one process the
// flag holds for.
const (
	EnvHostedSession    = "WS_HOSTED_SESSION"
	EnvHostedSessionKey = "WS_HOSTED_SESSION_KEY"
	EnvHostedSessionRow = "WS_HOSTED_SESSION_ROW"
)

// Env names the runner sets on an attached ws-mcp.
const (
	// EnvAllowedTools is ws-mcp's tool allowlist: a comma list of ws tool
	// names; tools/list advertises and tools/call accepts only these.
	EnvAllowedTools = "WS_MCP_ALLOWED_TOOLS"
	// EnvNoDashboard turns ws-mcp's dashboard off (no daemon ensure, no
	// session registration), whatever its config resolves.
	EnvNoDashboard = "WS_MCP_NO_DASHBOARD"
)

// mailboxIdentityEnv are the variables that give a ws-mcp process a mailbox
// identity of its own; the attached ws-mcp gets them blank.
var mailboxIdentityEnv = []string{"WS_MAILBOX", "WS_MAILBOX_AUTO"}

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
	// Holders is the session holder manager lead conversations run under,
	// LeadKeys their key store, and Mail their mailbox; a lead profile is
	// refused without the first two, and gets no mail without the third.
	Holders  HolderHost
	LeadKeys LeadKeyStore
	Mail     LeadMail
	// Alive reports whether a process of this machine runs: what refuses a
	// takeover of a session whose terminal harness has not exited. nil is
	// wsstate.ProcessAlive.
	Alive func(pid int) bool
	// Trusted is the gate of a lead conversation's control writes (spawn,
	// settings, stop, close, approval answers); nil is
	// wsdashboard.TrustedWrite: a browser on this machine that sends the
	// daemon's own Origin.
	Trusted func(req *http.Request) (local, own bool)
	// TrackerChanged, when not nil, is called after a conversation's
	// successful tracker write (an Event with TrackerWrite set), so the
	// daemon re-reads its tracker stores. It may be called from any
	// goroutine.
	TrackerChanged func()
	// Report, when not nil, receives each settled turn of a conversation
	// with a parent (Runner.Spawn): a turn's end, a failed start, a process
	// lost mid-turn, and, when the next daemon loads the record, a turn
	// this daemon's exit cut off. Closing the conversation reports nothing.
	// It runs on the conversation's loop and must not block on the runner.
	Report func(TurnReport)
	// ChildCheck, when not nil, is asked before each call of the token
	// route (children.go): an error refuses the call with its text (no
	// usable Pi, say).
	ChildCheck func() error
	// heartbeat overrides wslive.HeartbeatInterval for tests.
	heartbeat time.Duration
	// spawnListed observes global spawn-projection listings in tests.
	spawnListed func(string)
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
	// scanned is closed once the holder manager's first scan returned, or
	// at Start when there is no manager (scanDone); scanOnce closes it.
	scanned  chan struct{}
	scanOnce sync.Once
	// rejoined gates nested calls until held-process capabilities are validated.
	rejoined   chan struct{}
	rejoinOnce sync.Once

	// holders routes the session holders' events to the lead processes.
	holders holderTable

	// watchers are Watch's callbacks, by registration (watchMu).
	watchMu  sync.Mutex
	watchers map[*func(State)]struct{}

	// The spawn tree (children.go), under mu: endpoint is the daemon's
	// loopback base URL (SetEndpoint), tokens the capability tokens of
	// the running processes by token, and tokenOf by conversation id.
	endpoint string
	tokens   map[string]string
	tokenOf  map[string]string
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
	if opts.Trusted == nil {
		opts.Trusted = wsdashboard.TrustedWrite
	}
	if opts.Alive == nil {
		opts.Alive = wsstate.ProcessAlive
	}
	r := &Runner{opts: opts, convs: map[string]*conv{}, closed: map[string]Record{}, quit: make(chan struct{}), scanned: make(chan struct{}), rejoined: make(chan struct{}), notify: func(string, string) {}}
	if opts.Holders != nil {
		// Before the manager's first scan, so a predecessor's holders are
		// seen attaching.
		opts.Holders.AddSink(&r.holders)
	}
	return r
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
		if rec.ClosedAt != "" || rec.HandedOffAt != "" {
			// A handed-off session is a terminal harness's: nothing here
			// writes its row, watches its mail, or starts a process for it.
			r.mu.Lock()
			r.closed[rec.ReplyID] = rec
			r.mu.Unlock()
		} else {
			// A lead whose process may still run under its holder rejoins
			// it there, before the scan below replays the holder's records.
			// A turn the last daemon's exit cut off settles there too.
			r.startConv(rec, nil)
		}
	}
	// All records are loaded now, including children sorted after their lead.
	for _, st := range r.List() {
		if c := r.conv(st.ID); c != nil {
			c.do(func() {
				c.spawnDirty = true
				c.live.writeSnapshot()
			})
		}
	}
	// A child loaded before its parent found no parent to report to.
	r.deliverPending()
	if r.opts.Holders == nil {
		// No holder to wait for.
		r.markScanned()
		r.rejoinOnce.Do(func() { close(r.rejoined) })
		return
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.opts.Holders.Scan()
		r.markScanned()
		r.holdersScanned()
		r.rejoinOnce.Do(func() { close(r.rejoined) })
	}()
}

// markScanned closes scanned, once however often it is called.
func (r *Runner) markScanned() {
	r.scanOnce.Do(func() { close(r.scanned) })
}

// holdersScanned follows the first holder scan of this daemon: each
// rejoined process learns where its replay ends (or that its holder is
// gone), and a lead holder no conversation runs under is ended, so no
// second process of a session stays behind a crash between a spawn and its
// record.
func (r *Runner) holdersScanned() {
	// Listed before the conversations are taken and asked: a conversation
	// opened after the listing launched no holder in it, and a holder a
	// start launched meanwhile is in its record by the time the
	// conversation's loop answers.
	listed := r.opts.Holders.List().Holders
	r.mu.Lock()
	convs := make([]*conv, 0, len(r.convs))
	for _, c := range r.convs {
		convs = append(convs, c)
	}
	r.mu.Unlock()
	owned := map[string]bool{}
	for _, c := range convs {
		c.do(func() {
			c.checkReattach()
			if c.proc == nil {
				// A start kick put off until the scan (a rejoin that failed).
				c.kick()
			}
			if id := c.rec.HolderID; id != "" {
				owned[id] = true
			}
		})
	}
	select {
	case <-r.quit:
		// The daemon closes: a loop that ended on it let its process go
		// under its holder for the next daemon (letGo) and answered nothing
		// above, so its holder would read as no conversation's. The next
		// daemon's first scan sweeps instead.
		return
	default:
	}
	for _, info := range listed {
		var meta holderSession
		if json.Unmarshal(info.Session, &meta) != nil || (meta.Kind != holderKindLead && meta.Kind != holderKindPiWorker) || owned[info.ID] {
			continue
		}
		fmt.Fprintf(r.opts.Log, "agent: ending conversation holder %s of %s: no open conversation runs under it\n", info.ID, meta.ReplyID)
		if err := r.opts.Holders.Stop(info.ID); err != nil {
			fmt.Fprintf(r.opts.Log, "agent: stop holder %s: %v\n", info.ID, err)
		}
	}
}

// scanDone reports whether the holder manager's first scan of this daemon
// returned. Before it the manager lists only the holders attached so far,
// so a holder a record names that is missing from the listing may still
// run: nothing may take it for gone, or start a second process beside it.
func (r *Runner) scanDone() bool {
	select {
	case <-r.scanned:
		return true
	default:
		return false
	}
}

// Close ends every conversation's process and stops the runner; the
// conversations stay open on disk and resume lazily under the next daemon.
// A lead conversation's process is let go, not ended: it runs on under its
// session holder, and the next daemon rejoins it.
func (r *Runner) Close() {
	r.once.Do(func() { close(r.quit) })
	r.wg.Wait()
}

// InboxChanged tells the runner a conversation's inbox changed from the
// session page (a message, cancel, steer, or interrupt), so it delivers or
// interrupts as for its own routes. It returns at once: the loop does the
// work (inboxChanged). An unknown reply-id is ignored.
func (r *Runner) InboxChanged(replyID string) {
	if c := r.conv(replyID); c != nil {
		c.signalInbox()
	}
}

func (r *Runner) conv(replyID string) *conv {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.convs[replyID]
}

// startConv starts serving rec. It writes the registration and the
// snapshot, and tells the daemon, before it takes r.mu to list the
// conversation, so no other conversation's Get, List, or act waits on
// those writes; r.mu is not held. seed, when not empty, is the command
// list the first snapshot shows (lastCommands); a process's own list
// replaces it.
func (r *Runner) startConv(rec Record, seed []wslive.SlashCommand) *conv {
	dir := wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID)
	c := &conv{
		r:          r,
		dir:        dir,
		inbox:      wsinbox.Dir(dir),
		rec:        rec,
		live:       newLiveWriter(dir, rec.WSKey, rec.Backend, r.opts.Now, r.opts.Log),
		reqs:       make(chan func()),
		inboxC:     make(chan struct{}, 1),
		spawnC:     make(chan struct{}, 1),
		spawnDirty: true,
		mailC:      make(chan struct{}, 1),
		done:       make(chan struct{}),
		settleC:    make(chan struct{}),
	}
	c.live.beforeSnapshot = c.seedSpawnRows
	if len(seed) > 0 {
		c.live.applyQuiet(wslive.Event{Kind: wslive.KindCommands, Commands: seed})
	}
	c.sends, c.drained, c.turnMark, c.voided = rec.Sends, rec.Drained, rec.TurnMark, rec.Voided
	c.heldRes, c.holding = rec.HeldTurn, rec.HeldTurn != nil
	if c.heldRes != nil {
		c.heldRes.mark = rec.TurnMark
	}
	c.steered = slices.Clone(rec.Steered)
	for i := range c.steered {
		c.steered[i].Mark = rec.SteerMarks[c.steered[i].Seq]
	}
	c.steerSeq, c.steerAnswers = rec.SteerSeq, rec.SteerAnswers
	if last := rec.LastTurn; last != nil {
		// Persisted coverage keeps a wait for queued input from taking a
		// prior settlement. Legacy records retain their zero-count behavior.
		c.settled, c.settledReport = last.Result, last.ID
		c.settled.n, c.settled.mark = 1, last.Mark
		if !rec.TurnOpen && (last.Result.Status == TurnFailed || last.Result.Status == TurnInterrupted) {
			c.lastErr = last.Result.Error
		}
	}
	c.writeRegistration()
	// A row a terminal login took while no daemon ran: the loop lets the
	// session go before it takes anything else (settle), and nothing below
	// starts for it.
	foreign := c.foreignRow
	if !foreign {
		c.live.writeSnapshot()
	}
	if rec.HolderID != "" && !foreign && (rec.Lead || rec.Backend == BackendPi) {
		if rec.Parent != nil && rec.TurnOpen {
			c.turn = true
			c.live.applyQuiet(wslive.Event{Kind: wslive.KindTurnStart})
		}
		// Rejoined before the loop runs: what the loop takes first (waiting
		// mail, a message from the page) would otherwise start a second
		// process, and end this one to do it.
		c.reattach()
	}
	r.mu.Lock()
	r.convs[rec.ReplyID] = c
	r.wg.Add(1)
	go c.loop()
	r.mu.Unlock()
	// Do not expose a previous LastTurn while startup still reconciles the
	// later TurnOpen. The loop reconciles before it accepts this request.
	c.do(func() {})
	if rec.Lead && r.opts.Mail != nil && !foreign {
		// Mail that waited while no daemon ran is found by the first check.
		c.stopMail = r.opts.Mail.Watch(rec.Root, rec.WSKey, c.signalMail)
		c.signalMail()
	}
	if (len(rec.Queued) > 0 || len(rec.Reports) > 0) && !foreign {
		// Drained input an earlier daemon queued: nothing else would send
		// it before the next message. A rejoin's replay end sends it
		// instead (kick waits for it).
		c.signalInbox()
	}
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
var errHandedOff = &requestError{status: http.StatusConflict, msg: "the session was handed to a terminal: it runs there, not in the dashboard"}

// endedError is the refusal of a write to a conversation no loop serves.
func endedError(rec Record) error {
	if rec.ClosedAt == "" && rec.HandedOffAt != "" {
		return errHandedOff
	}
	return errClosed
}

// LeadOptions are a lead conversation's spawn choices; an empty one takes
// the profile's value (the harness default when it has none).
type LeadOptions struct {
	Model          string
	Effort         string
	PermissionMode string
}

// Create opens a conversation of profile bound to root. It resolves and
// checks the profile (backend, instruction file) now, so a bad launch fails
// here; the process itself starts on the first send. A lead profile is
// refused: its conversations open through CreateLead, behind the trusted
// gate.
func (r *Runner) Create(profile, root string) (State, error) {
	return r.create(profile, root, nil, nil)
}

// CreateLead opens a hosted lead session of harness backend on root, with
// the builtin lead profile of that harness (or the user's definition of the
// same name). The caller has passed the trusted gate.
func (r *Runner) CreateLead(backend, root string, opts LeadOptions) (State, error) {
	profile := LeadProfileFor(strings.TrimSpace(backend))
	if profile == "" {
		return State{}, badRequest("harness must be %q or %q; got %q", BackendClaude, BackendPi, backend)
	}
	if opts.Model != "" && !modelPattern.MatchString(opts.Model) {
		return State{}, badRequest("model %q must match %s", opts.Model, modelPattern.String())
	}
	if opts.Effort != "" && !effortPattern.MatchString(opts.Effort) {
		return State{}, badRequest("effort %q must be a lowercase word", opts.Effort)
	}
	if opts.PermissionMode != "" && !ValidLeadPermissionMode(opts.PermissionMode) {
		return State{}, badRequest("permission_mode %q must be one of %s", opts.PermissionMode, strings.Join(LeadPermissionModes, ", "))
	}
	return r.create(profile, root, &opts, nil)
}

// create opens a conversation; lead is a lead's spawn choices, spawn a
// spawned conversation's (Runner.Spawn), at most one of them set.
func (r *Runner) create(profile, root string, lead *LeadOptions, spawn *SpawnSpec) (State, error) {
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
	if rp.Definition.Lead != (lead != nil) {
		if lead == nil {
			return State{}, &requestError{status: http.StatusForbidden, msg: fmt.Sprintf("agent profile %q is a lead profile: a lead session is spawned from the dashboard on this machine", rp.Name)}
		}
		return State{}, badRequest("agent profile %q is not a lead profile (lead: true)", rp.Name)
	}
	var key string
	if lead != nil {
		// A lead key lives at the canonical git root, which is then the
		// session's root: its registration and session record must agree.
		if root, err = r.opts.LeadKeys.LeadRoot(root); err != nil {
			return State{}, badRequest("a lead session needs a git worktree: %v", err)
		}
		if key, err = r.opts.LeadKeys.MintLead(root); err != nil {
			return State{}, fmt.Errorf("mint the lead session key: %w", err)
		}
	} else if key, err = r.opts.Keys.Mint(root); err != nil {
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
	if lead != nil {
		rec.Lead = true
		rec.PermissionMode = rp.Definition.PermissionMode
		if lead.Model != "" {
			rec.Model = lead.Model
		}
		if lead.Effort != "" {
			rec.Effort = lead.Effort
		}
		if lead.PermissionMode != "" {
			rec.PermissionMode = lead.PermissionMode
		}
		if rec.Backend != BackendClaude {
			rec.PermissionMode = ""
		}
	}
	if spawn != nil {
		spawn.apply(&rec)
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
	var seed []wslive.SlashCommand
	if rec.Lead {
		seed = r.lastCommands(rec)
	}
	c := r.startConv(rec, seed)
	if spawn != nil {
		return r.spawned(c, spawn.Prompt)
	}
	return c.state(), nil
}

// lastCommands is the command list in the persisted snapshot of the
// newest other hosted lead session of rec's root and profile that has
// one; nil when none has. A new lead's process starts only at its first
// send, and the composer offers commands only from the snapshot's list,
// so without this an empty session would offer none until then.
func (r *Runner) lastCommands(rec Record) []wslive.SlashCommand {
	r.mu.Lock()
	recs := make([]Record, 0, len(r.convs)+len(r.closed))
	convs := make([]*conv, 0, len(r.convs))
	for _, c := range r.convs {
		convs = append(convs, c)
	}
	for _, prev := range r.closed {
		recs = append(recs, prev)
	}
	r.mu.Unlock()
	for _, c := range convs {
		recs = append(recs, c.record())
	}
	recs = slices.DeleteFunc(recs, func(prev Record) bool {
		return !prev.Lead || prev.ReplyID == rec.ReplyID || prev.Root != rec.Root || prev.Profile != rec.Profile || prev.Backend != rec.Backend
	})
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].CreatedAt != recs[j].CreatedAt {
			return recs[i].CreatedAt > recs[j].CreatedAt
		}
		return recs[i].ReplyID > recs[j].ReplyID
	})
	for _, prev := range recs {
		snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(wsdashboard.SessionDir(r.opts.Dir, prev.ReplyID)))
		if err == nil && len(snap.Commands) > 0 {
			return snap.Commands
		}
	}
	return nil
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
	if rp.Definition.Lead && (r.opts.Holders == nil || r.opts.LeadKeys == nil) {
		return ResolvedProfile{}, badRequest("agent profile %q is a lead profile, but this daemon runs no session holders", rp.Name)
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
		return r.closedState(rec), nil
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
		out = append(out, r.closedState(rec))
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
// now when no turn runs. A spawned Pi worker takes it as a native steer
// during a running turn; other conversations hold it until that turn ends.
// It reports whether the message is still held.
//
// Send returns once the message is in the conversation's inbox: it neither
// waits for the loop nor starts the process, which the loop does on its
// signal (inboxChanged), so a busy loop or a slow start never holds the
// caller. A start failure reaches the conversation's State.Error, not this
// answer. The inbox write is serialized against close under c.mu, so a send
// racing CloseConversation either lands before the close (which drops it
// as it drops every held message) or is refused, never written into a
// closed conversation's inbox that nothing reads.
func (r *Runner) Send(id, text string) (State, error) {
	if strings.TrimSpace(text) == "" {
		return State{}, badRequest("text is empty")
	}
	c, err := r.open(id)
	if err != nil {
		return State{}, err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	c.mu.Lock()
	if c.closed {
		err := endedError(c.rec)
		c.mu.Unlock()
		return State{}, err
	}
	steer := c.rec.Parent != nil && c.rec.Backend == BackendPi && !c.rec.Lead
	_, _, err = wsinbox.Send(c.inbox, wsinbox.Entry{Parts: []string{text}, Steer: steer}, r.opts.Now())
	if err == nil {
		// After the write, under the lock kick drains with: a turn whose
		// drain counted this send holds its message (Wait).
		c.sends++
		c.rec.Sends = c.sends
	}
	rec := c.rec
	c.mu.Unlock()
	if err == nil {
		err = writeRecord(c.dir, rec)
	}
	if err != nil {
		return State{}, err
	}
	c.signalInbox()
	return c.state(), nil
}

// Interrupt stops the running turn, if any, and takes back the held
// messages, returning their text, as the session page's interrupt does.
// What a drain already took and queued behind a slash command (Record.Queued)
// is not held: the inbox shows it received, so it stays and is sent after
// the interrupted turn ends, as the page's interrupt leaves it too.
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

// Restart ends a lead conversation's process and starts a new one on the
// same backend session at once: the way back from an MCP connection that
// did not recover in-stream. It ends the session's background work with
// the process, so the dashboard asks first.
func (r *Runner) Restart(id string) (State, error) {
	return r.act(id, func(c *conv) error {
		if !c.rec.Lead {
			return badRequest("restart applies to a lead session only")
		}
		return c.restartProcess()
	})
}

// IsLead reports whether id is a lead conversation, open or closed: one
// whose control writes take the trusted gate.
func (r *Runner) IsLead(id string) bool {
	r.mu.Lock()
	c, open := r.convs[id]
	rec, closed := r.closed[id]
	r.mu.Unlock()
	if open {
		return c.record().Lead
	}
	return closed && rec.Lead
}

// AnswerApproval answers the waiting tool permission prompt requestID of a
// lead conversation: allow runs the tool call as asked, deny refuses it
// with message for the model. A prompt is answered once: an id that is not
// waiting (answered from another tab, withdrawn, or of an ended process) is
// a conflict.
func (r *Runner) AnswerApproval(id, requestID string, allow bool, message string) (State, error) {
	return r.act(id, func(c *conv) error {
		return c.answerApproval(requestID, allow, message)
	})
}

// SetPermissionMode changes a claude lead conversation's permission mode.
// Like a model or effort change it applies at the next turn boundary,
// in-stream when the harness takes it; the mode the harness then reports is
// State.ActiveMode.
func (r *Runner) SetPermissionMode(id, mode string) (State, error) {
	if !ValidLeadPermissionMode(mode) {
		return State{}, badRequest("permission_mode %q must be one of %s", mode, strings.Join(LeadPermissionModes, ", "))
	}
	return r.act(id, func(c *conv) error {
		if !c.rec.Lead || c.rec.Backend != BackendClaude {
			return badRequest("the permission mode applies to a claude lead session only")
		}
		if mode == c.rec.PermissionMode {
			return nil
		}
		c.update(func(rec *Record) { rec.PermissionMode = mode })
		c.dirtyMode = true
		if !c.turn {
			c.applySettings()
		}
		return nil
	})
}

// CloseConversation ends the conversation for good: its process stops,
// held messages are dropped, later sends are refused, and its session
// directory stays readable. Its open descendants through conversation
// parents (children.go) close first, deepest first. A conversation parent
// whose done turn waited for this child's report is released.
func (r *Runner) CloseConversation(id string) (State, error) {
	if _, err := r.open(id); err != nil {
		return State{}, err
	}
	for _, d := range r.descendants(id) {
		_, _ = r.closeOne(d)
	}
	st, err := r.closeOne(id)
	if err != nil {
		return st, err
	}
	if rec, ok := r.recordOf(id); ok && rec.Parent != nil && rec.Parent.Kind == ParentConversation {
		if parent := r.conv(rec.Parent.ReplyID); parent != nil {
			parent.do(parent.releaseHeld)
		}
	}
	return st, nil
}

// closeOne closes conversation id alone.
func (r *Runner) closeOne(id string) (State, error) {
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

// open returns the open conversation id: errClosed for a closed one,
// errUnknown for one the runner never had.
func (r *Runner) open(id string) (*conv, error) {
	r.mu.Lock()
	c, open := r.convs[id]
	rec, closed := r.closed[id]
	r.mu.Unlock()
	switch {
	case open:
		return c, nil
	case closed:
		return nil, endedError(rec)
	}
	return nil, errUnknown
}

// act runs fn in the conversation's loop and returns its state after.
func (r *Runner) act(id string, fn func(c *conv) error) (State, error) {
	c, err := r.open(id)
	if err != nil {
		return State{}, err
	}
	if !c.do(func() {
		if c.closed {
			err = endedError(c.rec)
			return
		}
		err = fn(c)
	}) {
		return State{}, endedError(c.record())
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
	// StatusHandedOff is a lead session handed to a terminal harness.
	StatusHandedOff = "handed_off"
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
	// Status is idle (no turn runs), running (a turn runs), closed, or
	// handed_off (a lead session a terminal harness holds: no process runs
	// here, mail no longer resumes it, and its row is the terminal's).
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
	// HandedOffAt is when the session was handed to a terminal harness;
	// set with the handed_off status alone.
	HandedOffAt string `json:"handed_off_at,omitempty"`

	// Lead marks a hosted lead session; the fields below are its own.
	Lead bool `json:"lead,omitempty"`
	// PermissionMode is the mode the next process launches in, and
	// ActiveMode the one the running harness last reported (it may fall
	// back from the one asked for).
	PermissionMode string `json:"permission_mode,omitempty"`
	ActiveMode     string `json:"active_mode,omitempty"`
	// Approvals are the tool permission prompts waiting for an answer,
	// oldest first.
	Approvals []ApprovalRequest `json:"approvals,omitempty"`
	// BackgroundTasks counts the harness's running background tasks, as its
	// stream last reported them.
	BackgroundTasks int `json:"background_tasks,omitempty"`
	// ActionNeeded is set while the harness reports that it waits on the
	// user with no prompt in Approvals to answer; it says what to do.
	ActionNeeded string `json:"action_needed,omitempty"`
	// Failure is the typed failure (a login needed, a model unavailable, a
	// usage limit) the harness last reported, until a turn succeeds.
	Failure *wslive.Notice `json:"failure,omitempty"`
	// MCPBroken names the harness's MCP servers whose connection broke and
	// did not come back in-stream; a restart (Restart) is the way back.
	MCPBroken []string `json:"mcp_broken,omitempty"`
	// CanControl reports whether the asking viewer may use the lead control
	// routes (a browser on this machine); the routes set it per request.
	CanControl bool `json:"can_control,omitempty"`
	// ResumeCommand is the shell command that resumes a handed-off lead
	// session's conversation in a terminal. It names paths of this
	// machine, so the routes answer it to a viewer with CanControl alone.
	ResumeCommand string `json:"resume_command,omitempty"`

	// Parent, Label, and Cwd are a spawned conversation's (Runner.Spawn):
	// the session its turns are reported to, its one-line name, and the
	// directory its process runs in when that is not Root.
	Parent *ParentRef `json:"parent,omitempty"`
	Label  string     `json:"label,omitempty"`
	Cwd    string     `json:"cwd,omitempty"`
	// Tier is a spawned conversation's capability tier, when its spawner
	// chose the model by one.
	Tier string `json:"tier,omitempty"`
}

// closedState is the state of a record no loop serves: closed, or handed
// to a terminal.
func (r *Runner) closedState(rec Record) State {
	st := closedState(rec)
	if st.Status == StatusHandedOff {
		st.ResumeCommand = resumeCommand(rec, wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID))
	}
	return st
}

func closedState(rec Record) State {
	st := State{ID: rec.ReplyID, Profile: rec.Profile, Backend: rec.Backend, Root: rec.Root, Status: StatusClosed, Model: rec.Model, Effort: rec.Effort, Page: "/s/" + rec.ReplyID, CreatedAt: rec.CreatedAt, ClosedAt: rec.ClosedAt, Lead: rec.Lead, PermissionMode: rec.PermissionMode, Label: rec.Label, Tier: rec.Tier}
	if rec.Parent != nil {
		parent := *rec.Parent
		st.Parent = &parent
	}
	if !rec.Lead {
		// A lead's Cwd is a taken-over terminal's, not a spawn choice.
		st.Cwd = rec.Cwd
	}
	if rec.ClosedAt == "" && rec.HandedOffAt != "" {
		st.Status, st.HandedOffAt = StatusHandedOff, rec.HandedOffAt
	}
	return st
}

// conv is one open conversation. Its loop goroutine owns the process and
// every write but Send's inbox write; other goroutines read through state(),
// act through do(), and tell the loop of an inbox change through
// signalInbox().
type conv struct {
	r     *Runner
	dir   string
	inbox string
	live  *liveWriter
	reqs  chan func()
	// inboxC (one slot) tells the loop the inbox changed (signalInbox).
	inboxC chan struct{}
	done   chan struct{}

	// mu guards what state() reads: rec and the status fields below. Send
	// holds it across its closed check and inbox write; close sets closed
	// under it before it drops the held messages.
	mu      sync.Mutex
	rec     Record
	proc    Process
	turn    bool
	lastErr string
	closed  bool
	// approvals are a lead's waiting tool permission prompts, oldest first;
	// activeMode and bgTasks are what its harness last reported.
	approvals  []ApprovalRequest
	activeMode string
	bgTasks    int
	// actionNeeded is State.ActionNeeded.
	actionNeeded string
	// failure and mcpBroken are State.Failure and State.MCPBroken.
	failure   *wslive.Notice
	mcpBroken []string

	// loop-owned.
	events      <-chan Event
	idle        *time.Timer
	idleAfter   time.Duration
	dirtyModel  bool
	dirtyEffort bool
	dirtyMode   bool
	lastReg     time.Time

	// A lead's own, loop-owned. mailC (one slot) tells the loop the
	// mailbox may hold mail (signalMail); stopMail ends the watch.
	mailC    chan struct{}
	stopMail func()
	// spawnC coalesces daemon-owned descendant changes for this writer.
	spawnC     chan struct{}
	spawnDirty bool
	// spawning is set from the row write that precedes a spawn until the
	// spawn settled: the row then names no process and is not stopped, so
	// a terminal login is refused meanwhile (Registration.HostMayRun).
	// foreignRow is set once the session's row was found to be a terminal
	// harness's (lockRow, checkRow); the loop then lets the session go
	// (settle).
	spawning   bool
	foreignRow bool
	// replaying is set from a rejoin of the holder's process (reattach)
	// until its replay ends: no turn is submitted meanwhile, since the
	// replay is what tells whether one runs.
	replaying       bool
	replayAsked     bool
	replayTurnError string
	// replayGap is set when records left the holder's ring during the
	// replay, and replayBoundary when the replay showed a turn begin or
	// end after the last such loss. wrapHold is set from a rejoin that lost
	// records with no turn boundary after them (replayDone) until the
	// harness's next turn end: the turn is taken as running, since its
	// start may be among the lost records, and nothing is submitted. A
	// harness that can tell is asked whether a turn runs (probeWrapHold);
	// wrapTurnSeen marks a turn start seen since the ask.
	replayGap      bool
	replayBoundary bool
	wrapHold       bool
	wrapTurnSeen   bool
	// seq is the last holder record sequence applied whole; seqDirty that
	// the record does not hold it yet, and ringDirty that a ring entry was
	// written since, which makes the next line end persist it at once.
	seq       uint64
	seqDirty  bool
	ringDirty bool
	// lineOpen is the holder record sequence of a line some of whose events
	// are applied and whose LineEnd is not; 0 between lines.
	lineOpen uint64
	// harnessState is the state the harness last reported; actionTimer runs
	// while it waits on the user with no prompt to show (checkAction).
	harnessState string
	actionTimer  *time.Timer
	// The process's MCP recovery (runner_recovery.go). mcpSeen are the
	// servers it reported connected; mcpBroken (above) are those that broke
	// and did not come back in-stream, and a restart is due at the next
	// idle point (mcpRestartDue). mcpRestarted outlives the process: the
	// servers an automatic
	// restart was for that have not connected since. mcpChecking runs
	// while a check is off the loop, and mcpAgain asks for one more after
	// it, for mcpAgainNames.
	mcpSeen       map[string]bool
	mcpRestarted  map[string]bool
	mcpChecking   bool
	mcpAgain      bool
	mcpAgainNames []string
	// retry is the API request retry under way; alerted that the running
	// turn brought a typed failure, so its end clears none.
	retry   *wslive.Retry
	alerted bool
	// sendRetried is set when a turn's failed send armed its one automatic
	// retry (kick) and cleared by the next send that succeeds; a send that
	// fails again meanwhile waits for the next inbox or mail signal.
	sendRetried bool

	// Turn settles (spawn.go). sends counts Runner.Send's messages and
	// settled is the last settled turn, settleC closed and replaced at each
	// settle and at close; voided is the sends no turn will take, since
	// the page cancelled them before a drain (voidCancelled); all four
	// under mu. drained is the sends the last drain covered, turnMark the
	// sends the running turn's messages covered, and reply its last reply
	// text; all three loop-owned. quitting is set at the daemon's
	// shutdown, whose cut-off turn the next daemon reports instead.
	sends    uint64
	settled  TurnResult
	settleC  chan struct{}
	voided   uint64
	drained  uint64
	turnMark uint64
	reply    string
	quitting bool

	// wmu orders the record writes of update, which other conversations'
	// goroutines also make (children.go takes a delivered report).
	wmu sync.Mutex

	// The spawn tree (children.go). settledReport is the pending report id
	// of the last settled turn, and parentWaits counts the conversation
	// parent's waits by target, and holding mirrors heldRes != nil; all
	// three under mu. Loop-owned: heldRes is a
	// done turn held for its children's reports; steered are the
	// reports and user messages steered into the running turn and not yet taken
	// by it, steerSeq the steer commands the process was written and
	// steerAnswers how many it answered.
	settledReport string
	parentWaits   map[uint64]int
	holding       bool
	heldRes       *TurnResult
	steered       []steeredReport
	steerSeq      int
	steerAnswers  int
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

// signalInbox tells the loop the inbox changed, without waiting; signals
// sent before the loop takes one fold into it.
func (c *conv) signalInbox() {
	select {
	case c.inboxC <- struct{}{}:
	default:
	}
}

// signalMail tells the loop the session's mailbox may hold mail, without
// waiting.
func (c *conv) signalMail() {
	select {
	case c.mailC <- struct{}{}:
	default:
	}
}

// mailChanged claims the session's waiting mail into the record, so a
// message taken from the mailbox survives until it is delivered, and
// submits it with the held messages (kick). It runs in the loop.
func (c *conv) mailChanged() {
	if c.closed || c.r.opts.Mail == nil {
		return
	}
	// The claim and the check that the session is still the daemon's are
	// one step: a terminal login that takes the row waits for the lock,
	// and what was claimed before it goes back to the mailbox (release).
	unlock, ok := c.lockRow()
	if !ok {
		return
	}
	texts, err := c.r.opts.Mail.Drain(c.rec.Root, c.rec.WSKey)
	unlock()
	if err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: mailbox: %v\n", c.rec.ReplyID, err)
	}
	if len(texts) > 0 {
		c.update(func(rec *Record) { rec.Mail = append(rec.Mail, texts...) })
		// The claim changed the session's mail history and held count. A
		// turn that runs submits nothing now, so nothing else would tell
		// the page.
		c.r.notify(c.rec.ReplyID, c.rec.Root)
	}
	// Mail claimed earlier and not yet delivered (a start that failed, a
	// daemon that ended) goes with it: nothing else would submit it.
	if len(c.rec.Mail) > 0 {
		c.kick()
	}
}

// inboxChanged takes the inbox's interrupt requests and submits the held
// messages, starting the process when needed (kick). It runs in the loop.
func (c *conv) inboxChanged() {
	c.checkInterrupt()
	c.steer()
	c.kick()
}

// steer writes steered messages to the process while a turn runs:
// the user marked them to reach the harness now, not at the turn's end.
// They join the running turn, so no turn starts. Messages not marked stay
// held, with no turn running a marked one is an ordinary send (kick), and
// a backend that takes no message mid-turn, or a rejoin that cannot tell
// whether the turn runs (wrapHold), keeps it held.
func (c *conv) steer() {
	if !c.turn || c.proc == nil || c.replaying || c.wrapHold || c.closed {
		return
	}
	if !c.rec.Lead {
		c.steerWorker()
		return
	}
	st, ok := c.proc.(steerer)
	if !ok {
		return
	}
	recs, _ := wsinbox.DrainSteer(c.inbox)
	var parts []string
	for _, rec := range recs {
		if text := strings.TrimSpace(rec.Entry.Text()); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return
	}
	text := strings.Join(parts, "\n\n")
	c.apply(wslive.Event{Kind: wslive.KindUser, Origin: wslive.OriginWhiteboard, Text: text})
	if err := st.Steer(text); err != nil {
		c.setStatus(func() { c.lastErr = "send the steered message: " + err.Error() })
		c.stopProcess("")
	}
}

func (c *conv) loop() {
	defer c.r.wg.Done()
	defer close(c.done)
	beat := time.NewTicker(c.r.opts.heartbeat)
	defer beat.Stop()
	// Settle the predecessor before inbox signals (including nested reports)
	// can start a genuinely new turn.
	if c.rec.Parent != nil && c.rec.HolderID == "" {
		c.interrupted()
		if c.requeueSteered() {
			c.signalInbox()
		}
	}
	c.reportLast()
	if c.settle() {
		return
	}
	for {
		var idleC, actionC <-chan time.Time
		if c.idle != nil {
			idleC = c.idle.C
		}
		if c.actionTimer != nil {
			actionC = c.actionTimer.C
		}
		select {
		case fn := <-c.reqs:
			// An inbox change signaled before the request is handled
			// first, so a request sees what a send that returned before
			// it was made did, as when sends ran in the loop.
			select {
			case <-c.inboxC:
				c.inboxChanged()
			default:
			}
			fn()
		case <-c.inboxC:
			c.inboxChanged()
		case <-c.mailC:
			c.mailChanged()
		case <-c.spawnC:
			c.spawnDirty = true
			c.live.writeSnapshot()
			c.changed(false)
		case ev, ok := <-c.events:
			if !ok {
				c.processExited()
			} else {
				c.handle(ev)
			}
		case <-idleC:
			c.idle = nil
			if c.idleNow() {
				c.stopProcess("")
			}
		case <-actionC:
			c.actionTimer = nil
			c.actionDue()
		case <-beat.C:
			c.live.writeSnapshot()
			c.persistSeq()
			if c.replaying {
				// A holder that attached late (a rescan) is found here.
				c.checkReattach()
			}
			if c.r.opts.Now().Sub(c.lastReg) >= wsdashboard.HeartbeatInterval {
				c.writeRegistration()
			} else {
				c.checkRow()
			}
		case <-c.r.quit:
			c.letGo()
			// The watch's listening marker names this process: left behind,
			// it would name a dead one until the next daemon's watch.
			if c.stopMail != nil {
				c.stopMail()
			}
			return
		}
		if c.settle() {
			return
		}
	}
}

// settle follows each step of the loop: a session whose row a terminal
// harness took is let go (release), and it reports whether the loop ends
// (the conversation closed, or was let go), having stopped the mail watch.
func (c *conv) settle() bool {
	if c.foreignRow && !c.closed {
		c.release()
	}
	if !c.closed {
		return false
	}
	if c.stopMail != nil {
		c.stopMail()
	}
	return true
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
		if c.rec.ClosedAt == "" && c.rec.HandedOffAt != "" {
			st.Status, st.HandedOffAt = StatusHandedOff, c.rec.HandedOffAt
			st.ResumeCommand = resumeCommand(c.rec, c.dir)
		}
	}
	st.Process = c.proc != nil
	st.Held = len(held) + len(c.rec.Mail) + len(c.rec.Queued) + len(c.rec.Reports)
	st.Error = c.lastErr
	if c.rec.Lead {
		st.ActiveMode, st.BackgroundTasks, st.ActionNeeded = c.activeMode, c.bgTasks, c.actionNeeded
		st.Approvals = append([]ApprovalRequest(nil), c.approvals...)
		if c.failure != nil {
			f := *c.failure
			st.Failure = &f
		}
		st.MCPBroken = slices.Clone(c.mcpBroken)
	}
	return st
}

func (c *conv) record() Record {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rec
}

// update changes the record under mu and persists it; wmu keeps the
// writes in the order of the changes.
func (c *conv) update(fn func(rec *Record)) {
	c.wmu.Lock()
	defer c.wmu.Unlock()
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

// writeRegistration writes the conversation's row and reports whether it
// did. A lead's row may have been taken by a terminal login (the session
// then is that harness's), so its write is one step with the check that
// the row is still the daemon's (lockRow): a row found taken is left as it
// is.
func (c *conv) writeRegistration() bool {
	rec := c.record()
	now := c.r.opts.Now()
	reg := registration(rec, now)
	if rec.Lead {
		unlock, ok := c.lockRow()
		if !ok {
			return false
		}
		defer unlock()
		reg.HostStopped = rec.HostPID == 0 && !c.spawning
	}
	err := wsdashboard.WriteRegistration(c.r.opts.Dir, reg)
	if err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: registration write: %v\n", rec.ReplyID, err)
	}
	c.lastReg = now
	c.r.notify(rec.ReplyID, rec.Root)
	return err == nil
}

// rowForeign reports whether a session's row, as read, is no longer the
// daemon's: it exists and is neither a hosted nor an agent row. A terminal
// harness that logged in with the session's key while no process of the
// daemon's ran rewrote it (a hand to the terminal). A missing or unreadable
// row is the daemon's to write.
func rowForeign(dir, replyID string) bool {
	row, err := wsdashboard.ReadRegistration(dir, replyID)
	// An agent row is no terminal's either, since a terminal login never
	// writes Agent: a daemon of a release that knew no hosted lead served
	// the session as an agent conversation while it ran, and its row is
	// the daemon's to take back.
	return err == nil && !row.Hosted && !row.Agent
}

// lockRow takes a lead session's row lock and checks the row is still the
// daemon's: the step every row write, mail claim, and spawn is guarded by.
// It returns the lock's release and true to go on. False means nothing may
// be done now: the row is a terminal harness's (foreignRow is set, and the
// loop lets the session go), or the lock could not be taken (logged; the
// caller's next try asks again).
func (c *conv) lockRow() (unlock func(), ok bool) {
	if c.foreignRow {
		return nil, false
	}
	unlock, err := wsdashboard.LockRegistration(c.r.opts.Dir, c.rec.ReplyID)
	if err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: %v\n", c.rec.ReplyID, err)
		return nil, false
	}
	if rowForeign(c.r.opts.Dir, c.rec.ReplyID) {
		unlock()
		c.foreignRow = true
		return nil, false
	}
	return unlock, true
}

// checkRow reads a lead session's row between its own writes, without the
// lock, so a hand to the terminal shows within a heartbeat. Nothing is
// decided on this read alone: every guarded step checks again under the
// lock, and the row leaves the daemon only through a terminal login, which
// never gives it back.
func (c *conv) checkRow() {
	if c.rec.Lead && !c.foreignRow && rowForeign(c.r.opts.Dir, c.rec.ReplyID) {
		c.foreignRow = true
	}
}

// release lets a lead session go to the terminal harness that took its
// row: the process and its holder end (the login was accepted only with no
// live process), mail claimed and not delivered goes back to the session's
// mailbox, held dashboard messages are dropped as a close drops them, and
// the record becomes a handed-off one, which no daemon start serves again.
// The row is the terminal's: nothing here writes it. The loop ends after
// (settle), which stops the mail watch.
func (c *conv) release() {
	c.stopProcess("")
	if err := c.retireHolder(); err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: %v\n", c.rec.ReplyID, err)
	}
	mail := c.rec.Mail
	if len(mail) > 0 && c.r.opts.Mail != nil {
		if err := c.r.opts.Mail.Return(c.rec.Root, c.rec.WSKey, mail); err != nil {
			// Kept in the record: a later takeover delivers it.
			fmt.Fprintf(c.r.opts.Log, "agent %s: return claimed mail: %v\n", c.rec.ReplyID, err)
		} else {
			mail = nil
		}
	}
	c.update(func(rec *Record) {
		rec.HandedOffAt = c.r.opts.Now().UTC().Format(time.RFC3339)
		rec.Mail = mail
		c.closed = true
	})
	c.dropHeld()
	c.live.setAdapter("")
	rec := c.record()
	c.r.mu.Lock()
	if c.r.convs[rec.ReplyID] == c {
		delete(c.r.convs, rec.ReplyID)
	}
	c.r.closed[rec.ReplyID] = rec
	c.r.mu.Unlock()
	fmt.Fprintf(c.r.opts.Log, "agent %s: handed to a terminal session\n", rec.ReplyID)
	c.r.notify(rec.ReplyID, rec.Root)
}

// dropHeld drops the dashboard messages held for a conversation that takes
// no more turns, the ones drained and still queued, and its pending
// interrupt requests.
func (c *conv) dropHeld() {
	if len(c.rec.Queued) > 0 {
		c.update(func(rec *Record) { rec.Queued = nil })
	}
	if held, err := wsinbox.Held(c.inbox); err == nil {
		for _, rec := range held {
			_, _ = wsinbox.Cancel(c.inbox, rec.Name)
		}
	}
	_, _ = wsinbox.DrainInterrupt(c.inbox)
}

// apply applies a wslive event and tells the daemon when it wrote. quiet
// (a replayed holder record) applies it to the state alone.
func (c *conv) apply(ev wslive.Event, quiet ...bool) {
	if ev.Kind == wslive.KindReset {
		c.spawnDirty = true
	}
	if len(quiet) > 0 && quiet[0] {
		c.live.applyQuiet(ev)
		return
	}
	if c.live.apply(ev) {
		c.ringDirty = true
		c.r.notify(c.rec.ReplyID, c.rec.Root)
	}
}

// kick submits the next turn when no turn runs, starting the process first
// when there is none. A failed start leaves everything held.
//
// The turn is taken from the drained inbox, in order, behind what earlier
// drains queued (Record.Queued): the messages up to the first slash command
// are joined into one turn with a lead's claimed mail, and a command at the
// front runs alone, as its bare typed line, so the harness runs it as a
// command. What follows the turn's part stays queued for the turns after
// it, which each turn end submits; mail waits for the next message turn, or
// goes alone once nothing is queued.
func (c *conv) kick() {
	if c.closed || c.turn || c.replaying || len(c.steered) > 0 || (c.rec.Backend == BackendPi && c.r.opts.Holders != nil && !c.r.scanDone()) {
		return
	}
	held, err := wsinbox.Held(c.inbox)
	mail := c.rec.Mail
	// A child's reports (children.go) go with the next message turn, as
	// mail does.
	reports := c.rec.Reports
	if (err != nil || len(held) == 0) && len(mail) == 0 && len(c.rec.Queued) == 0 && len(reports) == 0 {
		c.voidCancelled()
		return
	}
	if c.proc == nil {
		if c.rec.Lead && c.rec.HolderID != "" && !c.r.scanDone() {
			// The holder the record names may still run unseen: the start
			// waits for the first scan (holdersScanned kicks again), since
			// retireHolder would not find that holder to end it first.
			return
		}
		if err := c.startProcess(); err != nil {
			c.setStatus(func() { c.lastErr = err.Error() })
			fmt.Fprintf(c.r.opts.Log, "agent %s: start: %v\n", c.rec.ReplyID, err)
			c.live.writeSnapshot()
			c.r.notify(c.rec.ReplyID, c.rec.Root)
			c.settleTurn(TurnFailed, err.Error(), c.sendCount())
			return
		}
	}
	// Under mu, as Send writes: every send counted here is in the drain
	// (Wait's turn marks).
	c.mu.Lock()
	recs, _ := wsinbox.Drain(c.inbox)
	covered := c.sends
	c.mu.Unlock()
	from := c.drained
	c.drained = covered
	queue := slices.Clone(c.rec.Queued)
	for _, rec := range recs {
		if cmd := rec.Entry.Command; cmd != nil {
			cmd := *cmd
			queue = append(queue, QueuedInput{Command: &cmd, After: from})
		} else if text := strings.TrimSpace(rec.Entry.Text()); text != "" {
			queue = append(queue, QueuedInput{Text: text, After: from})
		}
	}
	if len(queue) == 0 && len(mail) == 0 && len(reports) == 0 {
		// A cancel took the message after the start: the process waits for
		// input like after a turn.
		c.voidCancelled()
		c.armIdle()
		return
	}
	if len(recs) > 0 {
		// Drained entries are no longer in the inbox: until a turn sent
		// them, the record holds them.
		c.update(func(rec *Record) { rec.Queued = queue })
	}
	cmd, parts, rest := nextTurn(queue)
	if cmd != nil {
		mail, reports = nil, nil
	}
	// The turn covers the sends its messages hold: all the drain took
	// when nothing waits behind it, else only those before the drain that
	// took the first input left waiting (a leading slash command's turn
	// leaves the text drained with it for the next turn).
	mark := covered
	if len(rest) > 0 {
		mark = rest[0].After
	}
	c.stopIdle()
	startedAt := c.r.opts.Now().UnixMilli()
	c.setStatus(func() { c.turn, c.lastErr = true, "" })
	// The turn reaches the page only once its send went through: a failed
	// send leaves no user entry or turn line of its own (its error shows),
	// so its retry shows the message once. The process's events for the
	// turn are handled by this loop after kick returns, so they still
	// follow the turn's start.
	if cmd != nil {
		err = c.runCommand(*cmd)
	} else {
		err = c.proc.Send(strings.Join(append(append(slices.Clone(parts), reports...), mail...), "\n\n"))
	}
	if err != nil {
		// The turn never began: its process ends with no turn to close.
		c.setStatus(func() { c.turn, c.lastErr = false, "send turn: "+err.Error() })
		c.stopProcess("")
		// The turn stays queued (its mail stays claimed), and nothing else
		// would submit it before an unrelated signal: it is retried once on
		// a new process, through the loop's inbox signal rather than on
		// this stack. A send that fails again leaves it for the next inbox
		// or mail signal, so a send that always fails does not spin.
		if (len(c.rec.Queued) > 0 || len(c.rec.Mail) > 0 || len(c.rec.Reports) > 0) && !c.sendRetried {
			c.sendRetried = true
			c.signalInbox()
		} else {
			c.settleTurn(TurnFailed, c.lastError(), mark)
		}
		return
	}
	c.sendRetried = false
	c.turnMark, c.reply = mark, ""
	// This turn's settle supersedes a done turn held for the children.
	c.hold(nil)
	c.update(func(rec *Record) {
		rec.Queued = rest
		rec.Drained, rec.TurnMark, rec.Voided = c.drained, mark, c.voided
		rec.TurnStartedAt = startedAt
		rec.TurnStartSeq = max(c.seq, c.lineOpen)
		if len(mail) > 0 {
			rec.Mail = nil
		}
		if len(reports) > 0 {
			rec.Reports = nil
		}
		if rec.Parent != nil {
			rec.TurnOpen, rec.HeldTurn = true, nil
		}
	})
	// Mail shows on the page through the session's mail history, so only
	// the dashboard's own text is a user entry; a turn of mail alone is
	// marked as mail's.
	origin := wslive.OriginMail
	text := strings.Join(append(slices.Clone(parts), reports...), "\n\n")
	if cmd != nil {
		text = cmd.Line()
	}
	if text != "" {
		origin = wslive.OriginWhiteboard
		c.apply(wslive.Event{Kind: wslive.KindUser, Origin: origin, Text: text})
	}
	c.apply(wslive.Event{Kind: wslive.KindTurnStart, Origin: origin})
	c.watched()
}

// voidCancelled counts the sends no drain took as voided when kick finds
// nothing to send with no turn running: every send a drain took went in a
// turn, so one the inbox no longer holds was cancelled from the page, and
// no turn will ever cover it. A conversation parent whose done turn waited
// for this child (openChildOf) is released. It runs in the loop.
func (c *conv) voidCancelled() {
	c.mu.Lock()
	held, err := wsinbox.Held(c.inbox)
	grew := err == nil && len(held) == 0 && c.sends > max(c.voided, c.settled.mark)
	if grew {
		c.voided = c.sends
	}
	parent := c.rec.Parent
	c.mu.Unlock()
	if grew && parent != nil && parent.Kind == ParentConversation {
		c.r.releaseLater(parent.ReplyID)
	}
}

// nextTurn splits queue at its first slash command: a command at the front
// is the turn (cmd), else the texts before the first command are (parts).
// rest is what waits for the turns after it; nil when nothing does.
func nextTurn(queue []QueuedInput) (cmd *wsinbox.Command, parts []string, rest []QueuedInput) {
	n := 0
	if len(queue) > 0 && queue[0].Command != nil {
		cmd, n = queue[0].Command, 1
	} else {
		for n < len(queue) && queue[n].Command == nil {
			parts = append(parts, queue[n].Text)
			n++
		}
	}
	if n < len(queue) {
		rest = slices.Clone(queue[n:])
	}
	return cmd, parts, rest
}

// runCommand sends a slash command as its own turn: through the process's
// own mapping when it has one (commandRunner), else as the bare typed line,
// which a harness that reads a message starting with a slash as a command
// runs as one.
func (c *conv) runCommand(cmd wsinbox.Command) error {
	if cr, ok := c.proc.(commandRunner); ok {
		return cr.RunCommand(cmd)
	}
	return c.proc.Send(cmd.Line())
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
	if rec.Lead {
		return c.startLead(rec, rp.Definition, false)
	}
	return c.startWorker(rec, rp.Definition, false)
}

func (c *conv) startWorker(rec Record, def Profile, attach bool) error {
	if rec.Backend == BackendPi && c.r.opts.Holders != nil && !attach {
		if !c.r.scanDone() {
			return errors.New("holder scan is not complete")
		}
		if err := c.retireHolder(); err != nil {
			return err
		}
	}
	instructions := ""
	if !attach {
		if err := c.r.opts.Keys.Ensure(rec.Root, rec.WSKey); err != nil {
			return fmt.Errorf("ws session key: %w", err)
		}
		var err error
		instructions, err = composeInstructions(c.dir, rec, def)
		if err != nil {
			return err
		}
	}
	spec := StartSpec{
		Root:         rec.Root,
		Dir:          c.dir,
		Profile:      def,
		Model:        rec.Model,
		Effort:       rec.Effort,
		SessionID:    rec.SessionID,
		Cwd:          rec.Cwd,
		Instructions: instructions,
		Env:          environ(),
	}
	if attach && len(def.WSTools) > 0 {
		// The mapper needs the original ws tool surface, not a new bridge.
		spec.WSMCP = &MCPCommand{}
	} else if len(def.WSTools) > 0 {
		cmd, err := c.r.opts.WSMCP(rec.Root)
		if err != nil {
			return fmt.Errorf("ws-mcp command: %w", err)
		}
		env := map[string]string{}
		for k, v := range cmd.Env {
			env[k] = v
		}
		env[EnvAllowedTools] = strings.Join(def.WSTools, ",")
		env[EnvNoDashboard] = "1"
		// The daemon may inherit a lead's mailbox identity; the agent's
		// ws-mcp must never claim that inbox's presence. Blank is inert.
		for _, name := range mailboxIdentityEnv {
			env[name] = ""
		}
		// A spawned conversation that may spawn gets this start's
		// capability token (children.go); any other gets both blank, so
		// none is inherited.
		env[EnvPiToken], env[EnvPiEndpoint] = "", ""
		if mayHoldToken(rec, def) {
			if !attach {
				token, endpoint := c.r.issueToken(rec.ReplyID)
				env[EnvPiToken], env[EnvPiEndpoint] = token, endpoint
				if rec.Backend == BackendPi && c.r.opts.Holders != nil {
					c.update(func(rec *Record) { rec.Capability = token })
				}
				rec.Capability = token
			}
		}
		cmd.Env = env
		spec.WSMCP = &cmd
	}
	if rec.Backend == BackendPi && c.r.opts.Holders != nil {
		spec.Holder = &holderChild{host: c.r.opts.Holders, table: &c.r.holders,
			session: holderSession{Kind: holderKindPiWorker, ReplyID: rec.ReplyID, Cwd: rec.workDir(), Capability: capabilityHash(rec.Capability)}}
		if attach {
			spec.Holder.id, spec.Holder.skipTo = rec.HolderID, rec.HolderSeq
		}
	}
	proc, err := c.r.opts.Backends[rec.Backend].Start(context.Background(), spec)
	if err != nil {
		c.r.revokeToken(rec.ReplyID)
		return err
	}
	c.setStatus(func() { c.proc = proc })
	c.events = proc.Events()
	if !attach {
		c.steerSeq, c.steerAnswers = 0, 0
	}
	c.idleAfter = def.IdleTimeout()
	if hp, ok := proc.(holderProcess); ok {
		id, pid, held := hp.HolderInfo()
		if held {
			if attach {
				c.replaying, c.seq = true, rec.HolderSeq
				return nil
			}
			c.seq, c.seqDirty, c.ringDirty = 0, false, false
			c.update(func(rec *Record) {
				rec.HolderID, rec.HostPID, rec.HolderSeq = id, pid, 0
				rec.SteerSeq, rec.SteerAnswers, rec.SteerAnswerSeq = 0, 0, 0
			})
		}
	}
	c.dirtyModel, c.dirtyEffort = false, false
	if def.ListInIndex != rec.Listed {
		c.update(func(rec *Record) { rec.Listed = def.ListInIndex })
		c.writeRegistration()
	}
	return nil
}

// startLead starts a lead conversation's process under a new session
// holder, or with attach rejoins the one the record names (a daemon
// restart). The harness loads the user's own setup; what makes it this
// session is the hosted environment: the flag, the lead key, and the path
// of the registration row whose host_pid names the one process the flag
// holds for. The row says 0 until the process exists, so a harness that
// reads it early waits rather than misreading an older PID.
func (c *conv) startLead(rec Record, def Profile, attach bool) error {
	host := c.r.opts.Holders
	holder := &holderChild{host: host, table: &c.r.holders, session: holderSession{Kind: holderKindLead, ReplyID: rec.ReplyID, Cwd: rec.workDir(), Label: rec.Profile}}
	if attach {
		holder.id, holder.skipTo = rec.HolderID, rec.HolderSeq
	} else {
		// One process per session: whatever holder the record still names
		// (a stop that failed, a crash) ends before --resume runs again.
		if err := c.retireHolder(); err != nil {
			return err
		}
		if err := c.r.opts.LeadKeys.EnsureLead(rec.Root, rec.WSKey); err != nil {
			return fmt.Errorf("ws session key: %w", err)
		}
		// One step with the check that the session is still the daemon's:
		// the row written here names no process and is not stopped, so a
		// terminal login is refused from now until the process it gets has
		// ended. No row written, no process.
		c.spawning = true
		if !c.writeRegistration() {
			c.spawning = false
			if c.foreignRow {
				return errHandedOff
			}
			return errors.New("the session's registration could not be written, so no process was started")
		}
	}
	spec := StartSpec{
		Root:        rec.Root,
		Dir:         c.dir,
		Profile:     def,
		Model:       rec.Model,
		Effort:      rec.Effort,
		SessionID:   rec.SessionID,
		Cwd:         rec.Cwd,
		SessionFile: rec.SessionFile,
		Env:         environ(),
		Holder:      holder,
		Lead: &LeadSpec{
			PermissionMode: rec.PermissionMode,
			Env: map[string]string{
				EnvHostedSession:    "1",
				EnvHostedSessionKey: rec.WSKey,
				EnvHostedSessionRow: wsdashboard.RegistrationPath(c.r.opts.Dir, rec.ReplyID),
			},
		},
	}
	proc, err := c.r.opts.Backends[rec.Backend].Start(context.Background(), spec)
	if c.spawning {
		c.spawning = false
		// The spawn settled: its row, written below with the process on
		// success, says stopped again when there is none.
		defer func() {
			if c.proc == nil {
				c.writeRegistration()
			}
		}()
	}
	if err != nil {
		return err
	}
	hp, ok := proc.(holderProcess)
	id, pid, held := "", 0, false
	if ok {
		id, pid, held = hp.HolderInfo()
	}
	if !held {
		_ = proc.Close()
		return fmt.Errorf("the %s backend did not start the process under a session holder", rec.Backend)
	}
	c.setStatus(func() { c.proc = proc })
	c.events = proc.Events()
	c.idleAfter = def.IdleTimeout()
	if attach {
		c.replaying, c.seq = true, rec.HolderSeq
		return nil
	}
	c.dirtyModel, c.dirtyEffort, c.dirtyMode = false, false, false
	c.seq, c.seqDirty, c.ringDirty = 0, false, false
	c.update(func(rec *Record) { rec.HolderID, rec.HostPID, rec.HolderSeq = id, pid, 0 })
	c.writeRegistration()
	return nil
}

// retireHolder ends the holder the record names, if it still exists, and
// forgets it; an error means its process may still run, and no new one
// starts.
func (c *conv) retireHolder() error {
	id := c.rec.HolderID
	if id == "" {
		return nil
	}
	for _, info := range c.r.opts.Holders.List().Holders {
		if info.ID != id {
			continue
		}
		if err := c.r.opts.Holders.Stop(id); err != nil && !errors.Is(err, wsholder.ErrUnknownHolder) {
			return fmt.Errorf("the session's previous process could not be ended (holder %s): %w", id, err)
		}
	}
	c.update(func(rec *Record) { rec.HolderID, rec.HostPID, rec.HolderSeq = "", 0, 0 })
	c.writeRegistration()
	return nil
}

// reattach rejoins the process that kept running under the record's holder
// while no daemon ran. It runs before the holder manager's first scan, so
// the holder's whole replay reaches the process; checkReattach follows the
// scan.
func (c *conv) reattach() {
	rec := c.record()
	def := Profile{Backend: rec.Backend, Lead: rec.Lead}
	if rp, err := c.r.resolve(rec.Profile, rec.Root); err == nil && rp.Definition.Backend == rec.Backend {
		def = rp.Definition
	}
	var err error
	if rec.Lead {
		err = c.startLead(rec, def, true)
	} else {
		c.replaying = true
		err = c.startWorker(rec, def, true)
	}
	if err != nil {
		// The holder stays in the record: the next start ends it first.
		fmt.Fprintf(c.r.opts.Log, "agent %s: rejoin holder %s: %v\n", rec.ReplyID, rec.HolderID, err)
	}
}

// checkReattach tells a rejoined process where the holder's replay ends,
// once the holder is attached; a holder that is gone ended with its
// process while no daemon ran. Until one of the two is known the
// conversation submits nothing (replaying).
func (c *conv) checkReattach() {
	// Before the first scan an unlisted holder may be one not attached yet;
	// holdersScanned asks again after it.
	if !c.replaying || c.replayAsked || c.proc == nil || !c.r.scanDone() {
		return
	}
	hp, ok := c.proc.(holderProcess)
	if !ok {
		return
	}
	id, _, _ := hp.HolderInfo()
	listed, attached := false, false
	for _, info := range c.r.opts.Holders.List().Holders {
		if info.ID == id {
			listed, attached = true, info.Attached
			if !c.rec.Lead {
				var meta holderSession
				if json.Unmarshal(info.Session, &meta) != nil || meta.Kind != holderKindPiWorker || meta.ReplyID != c.rec.ReplyID || meta.Capability != capabilityHash(c.rec.Capability) || (info.Child != nil && c.rec.HostPID != 0 && info.Child.PID != c.rec.HostPID) {
					// Never stop a holder that failed the ownership check.
					hp.Detach()
					c.setStatus(func() { c.proc = nil })
					c.events = nil
					c.update(func(rec *Record) { rec.HolderID, rec.HostPID, rec.HolderSeq = "", 0, 0 })
					c.recoveryFailed("held process association does not match the worker record")
					return
				}
				c.r.restoreToken(c.rec)
			}
		}
	}
	if !listed {
		c.processGone()
		return
	}
	if !attached {
		return
	}
	// The holder answers on the connection its replayed records arrive on,
	// one after the other, so the answer may wait behind records this loop
	// has to take first: the question is asked off the loop.
	c.replayAsked = true
	go func() {
		st, err := c.r.opts.Holders.Status(id)
		if err != nil {
			// The next beat asks again.
			c.do(func() { c.replayAsked = false })
			return
		}
		hp.ReplayUntil(st.FirstSeq, st.NextSeq-1)
	}()
}

// replayDone ends a rejoin: the state the holder's records rebuilt is
// written, and the conversation takes input again. A replay that lost
// records (lost: the ring dropped records, even ones an earlier daemon
// applied; or a gap during it) and showed no turn boundary after them
// cannot tell whether a turn runs: the turn is taken as running, and
// nothing is submitted until the harness ends one (wrapHold), since a
// message sent into a running turn may be refused.
func (c *conv) replayDone(lost bool) {
	lost = lost || c.replayGap
	boundary := c.replayBoundary
	c.replaying, c.replayAsked, c.replayGap, c.replayBoundary = false, false, false, false
	if !c.rec.Lead && c.rec.Parent != nil {
		if lost && c.rec.TurnOpen {
			c.wrapHold = true
			c.setStatus(func() { c.turn = true })
			c.probeWorkerRecovery()
			return
		}
		if c.rec.TurnOpen && !c.turn {
			status := TurnDone
			if c.replayTurnError != "" {
				status = TurnFailed
			}
			c.settleTurn(status, c.replayTurnError, c.turnMark)
		}
		c.unsteer()
		c.releaseHeld()
	}
	if lost {
		if boundary {
			fmt.Fprintf(c.r.opts.Log, "agent %s: records of holder %s left its ring before this daemon replayed them; the turn state is the one the records after them show\n", c.rec.ReplyID, c.rec.HolderID)
		} else {
			fmt.Fprintf(c.r.opts.Log, "agent %s: records of holder %s left its ring before this daemon replayed them, with no turn boundary after them; the turn is taken as running and nothing is submitted until the harness ends one\n", c.rec.ReplyID, c.rec.HolderID)
			c.wrapHold = true
			c.beginTurn(false)
			c.probeWrapHold()
		}
	}
	c.checkAction(false)
	c.live.writeSnapshot()
	c.r.notify(c.rec.ReplyID, c.rec.Root)
	// A message steered during the rejoin goes to the turn that still runs.
	c.steer()
	if !c.turn {
		c.applySettings()
		c.kick()
		if !c.turn {
			c.armIdle()
		}
	}
}

// recoveryFailed settles once, ends the unrecoverable transport, and never
// launches another process on the strength of a failed probe.
func (c *conv) recoveryFailed(reason string) {
	text := "Pi worker recovery failed: " + reason
	c.setStatus(func() { c.turn = false; c.lastErr = text })
	if c.rec.TurnOpen {
		c.settleTurn(TurnFailed, text, c.turnMark)
	}
	c.stopProcess("")
	c.r.revokeToken(c.rec.ReplyID)
	if c.rec.Capability != "" {
		c.update(func(rec *Record) { rec.Capability = "" })
	}
	c.replaying, c.wrapHold = false, false
}

func (c *conv) probeWorkerRecovery() {
	p, ok := c.proc.(*piProcess)
	if !ok {
		c.recoveryFailed("the held process cannot reconcile RPC messages")
		return
	}
	proc, startedAt := c.proc, c.rec.TurnStartedAt
	c.wrapTurnSeen = false
	go func() {
		running, err := p.TurnRunning()
		message, turnErr := "", ""
		var taken []string
		if err == nil && !running {
			message, turnErr, taken, err = p.recoveryReply(startedAt)
		}
		c.do(func() {
			if c.proc != proc || !c.wrapHold || c.wrapTurnSeen {
				return
			}
			if err != nil {
				c.recoveryFailed(err.Error())
				return
			}
			if running {
				return
			}
			for _, text := range taken {
				c.steerTaken(text)
			}
			// The idle RPC state and saved messages replace acknowledgements
			// lost from the bounded ring; unsteer clears any untaken queue.
			c.steerAnswers = c.steerSeq
			c.update(func(rec *Record) { rec.SteerAnswers = c.steerAnswers })
			c.reply = message
			c.wrapHold = false
			c.endTurn(turnErr, false)
		})
	}()
}

// probeWrapHold asks a harness that can tell (turnProber) whether a turn
// runs, so a rejoin's hold does not wait for a turn end an idle harness
// never sends. The ask runs off the loop, whose events it may wait behind;
// its answer comes back to the loop (wrapProbed).
func (c *conv) probeWrapHold() {
	tp, ok := c.proc.(turnProber)
	if !ok {
		return
	}
	proc := c.proc
	c.wrapTurnSeen = false
	go func() {
		running, err := tp.TurnRunning()
		c.do(func() { c.wrapProbed(proc, running, err) })
	}()
}

// wrapProbed takes the harness's answer to probeWrapHold. An idle answer
// ends the held turn as the harness's own turn end would, submitting what
// waited; unless the hold ended already, the process is another, or a turn
// started since the ask (its start may follow the answer on the stream,
// and its end releases the hold). A running answer, or an ask that failed,
// keeps the hold until the harness ends a turn.
func (c *conv) wrapProbed(proc Process, running bool, err error) {
	if c.closed || proc != c.proc || !c.wrapHold {
		return
	}
	switch {
	case err != nil:
		fmt.Fprintf(c.r.opts.Log, "agent %s: ask the harness whether a turn runs: %v; the turn stays taken as running until the harness ends one\n", c.rec.ReplyID, err)
	case running:
		fmt.Fprintf(c.r.opts.Log, "agent %s: the harness reports a turn running; input waits for its end\n", c.rec.ReplyID)
	case c.wrapTurnSeen:
		fmt.Fprintf(c.r.opts.Log, "agent %s: the harness reports no turn running, but one started since; input waits for its end\n", c.rec.ReplyID)
	default:
		fmt.Fprintf(c.r.opts.Log, "agent %s: the harness reports no turn running; the held turn ends\n", c.rec.ReplyID)
		c.endTurn("", false)
	}
}

// lineEnd records that every event of holder record seq is applied. The
// sequence reaches the record at once when the line wrote a conversation
// entry, so a replay after a daemon restart does not write it again.
func (c *conv) lineEnd(seq uint64) {
	if seq <= c.seq {
		return
	}
	c.seq, c.seqDirty = seq, true
	if c.ringDirty {
		c.persistSeq()
	}
}

func (c *conv) persistSeq() {
	if !c.seqDirty {
		return
	}
	c.seqDirty, c.ringDirty = false, false
	c.update(func(rec *Record) { rec.HolderSeq = c.seq })
}

// composeInstructions writes the instruction file a process appends to the
// harness's own system prompt: the profile's instruction file or builtin
// text and, when the profile allows ws tools, the conversation's ws session
// key. It returns "" when there is nothing to append.
func composeInstructions(dir string, rec Record, p Profile) (string, error) {
	var parts []string
	if text, ok := p.BuiltinInstructions(); ok {
		parts = append(parts, strings.TrimSpace(text))
	} else if path := p.InstructionsPath(rec.Root); path != "" {
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
	quiet := ev.Replay
	if ev.Seq > 0 && !ev.Replay {
		c.lineOpen = ev.Seq
		if ev.LineEnd {
			c.lineOpen = 0
		}
	}
	switch {
	case ev.LineEnd:
		c.lineEnd(ev.Seq)
	case ev.ReplayDone:
		c.replayDone(ev.ReplayLost)
	case ev.ReplayGap:
		if c.replaying {
			c.replayGap, c.replayBoundary = true, false
		}
	case ev.SessionID != "":
		if ev.SessionID != c.rec.SessionID {
			c.update(func(rec *Record) { rec.SessionID = ev.SessionID })
		}
	case ev.Live != nil:
		if ev.Live.Kind == wslive.KindReply && ev.Live.Final && !(quiet && c.rec.Parent != nil && ev.Seq <= c.rec.TurnStartSeq) {
			c.reply = ev.Live.Text
		}
		c.apply(*ev.Live, quiet)
	case ev.Card != nil:
		if !quiet && c.live.card(*ev.Card) {
			c.ringDirty = true
			c.r.notify(c.rec.ReplyID, c.rec.Root)
		}
	case ev.UI != nil:
		if !quiet && c.live.ui(*ev.UI) {
			c.ringDirty = true
			c.r.notify(c.rec.ReplyID, c.rec.Root)
		}
	case ev.TrackerWrite:
		if f := c.r.opts.TrackerChanged; f != nil && !quiet {
			f()
		}
	case ev.Fatal != nil:
		if quiet {
			return
		}
		if c.replaying && c.rec.Parent != nil {
			c.processExited()
			return
		}
		c.setStatus(func() { c.lastErr = ev.Fatal.Error() })
		fmt.Fprintf(c.r.opts.Log, "agent %s: %v\n", c.rec.ReplyID, ev.Fatal)
		c.stopProcess("")
	case ev.TurnStart:
		if quiet && c.rec.Parent != nil && ev.Seq <= c.rec.TurnStartSeq {
			return
		}
		c.replayBoundary = c.replayBoundary || c.replaying
		c.wrapTurnSeen = c.wrapTurnSeen || (c.wrapHold && !quiet)
		c.beginTurn(quiet)
	case ev.TurnEnd:
		if quiet && c.rec.Parent != nil && ev.Seq <= c.rec.TurnStartSeq {
			return
		}
		c.replayBoundary = c.replayBoundary || c.replaying
		c.endTurn(ev.TurnError, quiet)
	case ev.Approval != nil:
		c.addApproval(*ev.Approval, quiet)
	case ev.ApprovalDone != "":
		c.dropApproval(ev.ApprovalDone, quiet)
		c.forgetAnswered(ev.ApprovalDone)
	case ev.PermissionMode != "":
		if ev.PermissionMode != c.activeMode {
			c.setStatus(func() { c.activeMode = ev.PermissionMode })
			c.changed(quiet)
		}
		// The mode the harness runs in is the session's own: its next
		// launch resumes in it. A change of the user's that waits to be
		// applied stands until it is.
		if c.rec.Lead && !quiet && !c.dirtyMode && ev.PermissionMode != c.rec.PermissionMode && ValidLeadPermissionMode(ev.PermissionMode) {
			c.update(func(rec *Record) { rec.PermissionMode = ev.PermissionMode })
			c.changed(quiet)
		}
	case ev.HarnessState != "":
		c.harnessState = ev.HarnessState
		c.checkAction(quiet)
	case ev.SteerAnswer != nil:
		if ev.Seq > 0 && ev.Seq <= c.rec.SteerAnswerSeq {
			return
		}
		c.steerAnswered(ev.SteerAnswer.Refusal)
		c.update(func(rec *Record) { rec.SteerAnswers, rec.SteerAnswerSeq = c.steerAnswers, ev.Seq })
	case ev.UserTaken != "":
		c.steerTaken(ev.UserTaken)
	case ev.Notice != "":
		if !quiet {
			fmt.Fprintf(c.r.opts.Log, "agent %s: %s\n", c.rec.ReplyID, ev.Notice)
		}
	case ev.BackgroundTasks != nil:
		if n := *ev.BackgroundTasks; n != c.bgTasks {
			c.setStatus(func() { c.bgTasks = n })
			c.syncHost(quiet)
			c.changed(quiet)
			if n == 0 && !c.turn && !quiet {
				c.armIdle()
				c.restartIfDue()
			}
		}
	case ev.Alert != nil:
		c.alert(*ev.Alert, quiet)
	case ev.Retry != nil || ev.RetryEnd:
		c.retry = ev.Retry
		c.syncHost(quiet)
	case ev.MCPServers != nil:
		c.mcpReported(ev.MCPServers, quiet)
	case ev.MCPCheck:
		if !quiet {
			c.checkMCP(nil)
		}
	}
}

// changed tells the daemon a lead's state outside its live snapshot moved
// (pending approvals, the reported mode, the background task count).
func (c *conv) changed(quiet bool) {
	if !quiet {
		c.r.notify(c.rec.ReplyID, c.rec.Root)
	}
}

// beginTurn opens a turn the harness began with no send of the runner's: a
// background task's completion, a compaction, a pushed message. A turn the
// runner's own send opened is already running.
func (c *conv) beginTurn(quiet bool) {
	if c.turn {
		return
	}
	c.stopIdle()
	c.setStatus(func() {
		c.turn = true
		if !quiet {
			c.lastErr = ""
		}
	})
	c.apply(wslive.Event{Kind: wslive.KindTurnStart}, quiet)
	c.reply, c.replayTurnError = "", ""
	if !quiet {
		// Not the runner's send: the turn holds no new message, so it
		// keeps the last turn's mark.
		c.reply = ""
		c.hold(nil)
		if c.rec.Parent != nil {
			c.update(func(rec *Record) {
				rec.TurnOpen, rec.HeldTurn = true, nil
				rec.TurnStartSeq = c.seq
			})
		}
		c.watched()
	}
}

// endTurn closes the running turn, applies settings changed during it, and
// submits the messages held meanwhile.
func (c *conv) endTurn(turnErr string, quiet bool) {
	if c.wrapHold && !c.rec.Lead && c.rec.Parent != nil && !quiet {
		// A gap while streaming is reconciled at agent_settled, not left
		// dependent on acknowledgements that may have left the ring.
		c.probeWorkerRecovery()
		return
	}
	if !c.turn && !c.rec.Lead {
		return
	}
	ran := c.turn
	if c.turn {
		c.setStatus(func() { c.turn = false })
		c.apply(wslive.Event{Kind: wslive.KindTurnEnd}, quiet)
	}
	c.turnSettled(turnErr, quiet)
	if c.replaying && c.rec.Parent != nil {
		c.replayTurnError = turnErr
	}
	if quiet {
		return
	}
	// Observers must see the terminal diagnostic in the same emission as
	// the settled lifecycle, including mapped Pi agent_settled failures.
	if turnErr != "" {
		c.setStatus(func() { c.lastErr = turnErr })
	}
	if ran && !c.replaying {
		status := TurnDone
		if turnErr != "" {
			status = TurnFailed
		}
		c.settleTurn(status, turnErr, c.turnMark)
	}
	if c.replaying {
		return
	}
	c.wrapHold = false
	// A child's report steered into the turn that the turn did not take
	// goes as the next one (children.go).
	c.unsteer()
	c.applySettings()
	c.restartIfDue()
	c.kick()
	if !c.turn {
		c.armIdle()
	}
}

// addApproval records a waiting tool permission prompt; the session shows
// as awaiting approval while any waits.
func (c *conv) addApproval(a ApprovalRequest, quiet bool) {
	for _, have := range c.approvals {
		if have.ID == a.ID {
			return
		}
	}
	if slices.Contains(c.rec.Answered, a.ID) {
		// A replay of a prompt an earlier daemon already answered.
		return
	}
	c.setStatus(func() { c.approvals = append(c.approvals, a) })
	c.checkAction(quiet)
	c.showApprovals(quiet)
}

// actionGrace is how long the harness may report that it waits on the user
// with no prompt for the runner to show before the session says so. The
// state line arrives just before its prompt's own line, and stays for a
// moment after a prompt's answer.
var actionGrace = 2 * time.Second

// actionNeededText is State.ActionNeeded for a wait nobody can answer here.
const actionNeededText = "The harness waits on an answer this page has no prompt for. Interrupt the turn or stop the session to go on."

// actionNeededTool names that wait in the live state's permission wait.
const actionNeededTool = "prompt"

// checkAction follows the harness's wait on the user: while it waits with
// no prompt of the runner's to answer (one this process inherited, or a
// request the runner has no form for), the session must not look like it
// works. The wait shows after actionGrace, and ends with the state, with a
// prompt's arrival, or with the process.
func (c *conv) checkAction(quiet bool) {
	if c.harnessState == HarnessWaiting && len(c.approvals) == 0 && c.proc != nil {
		if c.actionNeeded == "" && c.actionTimer == nil && !quiet {
			c.actionTimer = time.NewTimer(actionGrace)
		}
		return
	}
	if c.actionTimer != nil {
		c.actionTimer.Stop()
		c.actionTimer = nil
	}
	if c.actionNeeded != "" {
		c.setStatus(func() { c.actionNeeded = "" })
		c.showApprovals(quiet)
	}
}

// actionDue shows the wait once its grace has passed.
func (c *conv) actionDue() {
	if c.harnessState != HarnessWaiting || len(c.approvals) > 0 || c.proc == nil || c.actionNeeded != "" {
		return
	}
	fmt.Fprintf(c.r.opts.Log, "agent %s: the harness waits on the user (requires_action) with no permission prompt to answer\n", c.rec.ReplyID)
	c.setStatus(func() { c.actionNeeded = actionNeededText })
	c.showApprovals(false)
}

// dropApproval forgets a prompt that needs no answer any more, and reports
// whether it was waiting.
func (c *conv) dropApproval(id string, quiet bool) bool {
	for i, have := range c.approvals {
		if have.ID == id {
			c.setStatus(func() { c.approvals = append(c.approvals[:i:i], c.approvals[i+1:]...) })
			c.checkAction(quiet)
			c.showApprovals(quiet)
			return true
		}
	}
	return false
}

// forgetAnswered drops a settled prompt from the record's answered list
// ("" drops them all: their process is gone).
func (c *conv) forgetAnswered(id string) {
	if len(c.rec.Answered) == 0 || (id != "" && !slices.Contains(c.rec.Answered, id)) {
		return
	}
	c.update(func(rec *Record) {
		if id == "" {
			rec.Answered = nil
			return
		}
		rec.Answered = slices.DeleteFunc(slices.Clone(rec.Answered), func(have string) bool { return have == id })
	})
}

// clearApprovals settles every waiting prompt with no answer: the process
// they came from is gone.
func (c *conv) clearApprovals() {
	c.forgetAnswered("")
	if len(c.approvals) == 0 {
		return
	}
	c.setStatus(func() { c.approvals = nil })
	c.showApprovals(false)
}

// showApprovals carries the oldest waiting prompt to the live state as its
// permission wait, or a wait with no prompt to answer (actionNeeded), or
// ends the wait when neither is left.
func (c *conv) showApprovals(quiet bool) {
	ev := wslive.Event{Kind: wslive.KindPermission, Clear: true}
	if c.actionNeeded != "" {
		ev = wslive.Event{Kind: wslive.KindPermission, Tool: actionNeededTool}
	}
	if len(c.approvals) > 0 {
		a := c.approvals[0]
		var args any
		if json.Unmarshal(a.Input, &args) != nil {
			args = nil
		}
		ev = wslive.Event{Kind: wslive.KindPermission, Tool: a.Tool, Args: args}
	}
	c.apply(ev, quiet)
	c.changed(quiet)
}

// answerApproval writes the answer to the waiting prompt id. The prompt is
// forgotten before the write, so it is answered at most once.
func (c *conv) answerApproval(id string, allow bool, message string) error {
	conflict := func(msg string) error { return &requestError{status: http.StatusConflict, msg: msg} }
	if c.replaying {
		return conflict("the session is rejoining its process; answer again in a moment")
	}
	var req *ApprovalRequest
	for i := range c.approvals {
		if c.approvals[i].ID == id {
			a := c.approvals[i]
			req = &a
		}
	}
	answerer, ok := c.proc.(approvalAnswerer)
	if req == nil || c.proc == nil || !ok {
		return conflict("that permission prompt is not waiting: it was answered, withdrawn, or its process ended")
	}
	c.dropApproval(id, false)
	c.update(func(rec *Record) { rec.Answered = append(rec.Answered, id) })
	if err := answerer.AnswerApproval(*req, allow, message); err != nil {
		c.setStatus(func() { c.lastErr = "answer the permission prompt: " + err.Error() })
		c.stopProcess("")
		return fmt.Errorf("answer the permission prompt: %w", err)
	}
	return nil
}

// processExited handles a process whose output ended without the runner
// stopping it.
func (c *conv) processExited() {
	rejoining, proc := c.replaying, c.proc
	c.events = nil
	if c.replaying && c.rec.Parent != nil {
		// Apply any completed replayed turn before handling an exited child.
		if c.rec.TurnOpen && !c.turn && c.replayBoundary {
			status := TurnDone
			if c.replayTurnError != "" {
				status = TurnFailed
			}
			c.settleTurn(status, c.replayTurnError, c.turnMark)
		} else {
			c.interrupted()
		}
	}
	if proc != nil {
		_ = proc.Close()
		c.releaseHolder(proc)
	}
	c.setStatus(func() {
		c.proc = nil
		if c.lastErr == "" {
			c.lastErr = "the backend process exited"
		}
	})
	c.processEnded()
	if rejoining {
		// The process ended while no daemon ran: what waited goes to a new
		// one.
		c.replaying, c.replayAsked = false, false
		c.kick()
	}
}

// processGone handles a rejoin whose holder no longer exists: the process
// ended with it while no daemon ran.
func (c *conv) processGone() {
	proc := c.proc
	c.events = nil
	if hp, ok := proc.(holderProcess); ok {
		hp.Detach()
	}
	c.setStatus(func() { c.proc = nil })
	c.update(func(rec *Record) { rec.HolderID, rec.HostPID, rec.HolderSeq = "", 0, 0 })
	c.writeRegistration()
	c.replaying, c.replayAsked = false, false
	c.interrupted()
	c.processEnded()
	c.kick()
}

// processEnded settles what ends with a process: its turn, its waiting
// prompts, and what it last reported.
func (c *conv) processEnded() {
	c.stopIdle()
	c.wrapHold = false
	if c.turn {
		c.setStatus(func() { c.turn = false })
		c.apply(wslive.Event{Kind: wslive.KindTurnEnd})
		// A turn the process took with it. A close settles none (its
		// parent ended it), and the daemon's exit leaves it to the next
		// daemon (interrupted).
		if !c.closed && !c.quitting {
			c.settleTurn(TurnFailed, c.lastError(), c.turnMark)
		}
	}
	c.r.revokeToken(c.rec.ReplyID)
	if c.rec.Capability != "" {
		c.update(func(rec *Record) { rec.Capability = "" })
	}
	if c.requeueSteered() && !c.closed && !c.quitting {
		// The steered reports the process took with it go as the next
		// turn; nothing else would send them.
		c.signalInbox()
	}
	c.clearApprovals()
	c.harnessState = ""
	c.checkAction(false)
	c.setStatus(func() { c.activeMode, c.bgTasks, c.mcpBroken = "", 0, nil })
	c.mcpSeen, c.mcpChecking, c.mcpAgain, c.mcpAgainNames = nil, false, false, nil
	c.retry, c.alerted = nil, false
	c.syncHost(true)
	c.live.endAgents()
	c.live.writeSnapshot()
	c.r.notify(c.rec.ReplyID, c.rec.Root)
}

// releaseHolder forgets the holder of an ended lead process, unless it
// could not be ended: then the record keeps it, and the next start ends it
// before anything runs (retireHolder).
func (c *conv) releaseHolder(proc Process) {
	hp, ok := proc.(holderProcess)
	if !ok || c.rec.HolderID == "" {
		return
	}
	if err := hp.StopError(); err != nil {
		c.setStatus(func() { c.lastErr = "end the session's process: " + err.Error() })
		fmt.Fprintf(c.r.opts.Log, "agent %s: stop holder %s: %v\n", c.rec.ReplyID, c.rec.HolderID, err)
		return
	}
	c.seqDirty, c.ringDirty = false, false
	c.update(func(rec *Record) { rec.HolderID, rec.HostPID, rec.HolderSeq = "", 0, 0 })
	c.writeRegistration()
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
	c.releaseHolder(proc)
	c.replaying, c.replayAsked = false, false
	c.processEnded()
}

// letGo is the daemon's shutdown: a lead's process runs on under its
// holder, with the sequence applied so far in the record for the next
// daemon's rejoin; any other process ends.
func (c *conv) letGo() {
	c.quitting = true
	c.finishLine()
	if hp, ok := c.proc.(holderProcess); ok {
		c.persistSeq()
		if hp.Detach() {
			c.live.writeSnapshot()
			return
		}
	}
	c.stopProcess("")
}

// finishLine applies the rest of a holder line the loop stands in the
// middle of, so the sequence the record keeps covers every entry the line
// wrote and the next daemon's replay writes none of them again. The line's
// events are already on their way; a stream that stalls is not waited for.
func (c *conv) finishLine() {
	wait := time.After(time.Second)
	for c.lineOpen != 0 && c.events != nil {
		select {
		case ev, ok := <-c.events:
			if !ok {
				return
			}
			c.handle(ev)
		case <-wait:
			return
		}
	}
}

// idleNow reports whether the idle timer may stop the process: no turn
// runs and the harness reports no background task, whose work a stop would
// end.
func (c *conv) idleNow() bool {
	return !c.turn && !c.replaying && c.bgTasks == 0
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
// boundary: in-stream when the backend can, and only otherwise by stopping
// the process so the next turn restarts it with the new values on the same
// backend session. A restart ends the session's background work with the
// process, so it is the last resort.
func (c *conv) applySettings() {
	model, effort, mode := c.dirtyModel, c.dirtyEffort, c.dirtyMode
	if c.replaying {
		return
	}
	c.dirtyModel, c.dirtyEffort, c.dirtyMode = false, false, false
	if c.proc == nil || (!model && !effort && !mode) {
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
	if mode && !restart {
		if ms, ok := c.proc.(modeSetter); !ok || ms.SetPermissionMode(rec.PermissionMode) != nil {
			restart = true
		}
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

// close ends the conversation for good. It marks the conversation closed
// first, under mu, so a Send either wrote before (and its message is
// dropped below with the other held ones) or is refused.
func (c *conv) close() {
	c.update(func(rec *Record) {
		rec.ClosedAt = c.r.opts.Now().UTC().Format(time.RFC3339)
		c.closed = true
	})
	c.stopProcess("")
	if len(c.rec.Mail) > 0 || len(c.rec.Reports) > 0 {
		c.update(func(rec *Record) { rec.Mail, rec.Reports = nil, nil })
	}
	c.r.revokeToken(c.rec.ReplyID)
	c.hold(nil)
	c.dropHeld()
	c.live.setAdapter("")
	c.writeRegistration()
	if c.rec.TurnOpen {
		c.update(func(rec *Record) { rec.TurnOpen = false })
	}
	c.signalSettled()
	c.watched()
}
