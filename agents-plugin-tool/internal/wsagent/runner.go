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

// LeadMail is the ws mailbox of the sessions the runner owns, hosted leads
// and spawned children, as the runner uses it: the daemon, not the
// session's own mailbox wait, delivers their mail. A child's address
// (AgentAddress) is its key's reply-id queue, so mail to it lives in the
// mailbox store's memory and restore ring, never in a file of the
// runner's.
type LeadMail interface {
	// Watch calls changed whenever the mailbox of key may hold new mail,
	// from any goroutine, until the returned stop is called. While watched,
	// the session reads to its peers as listening.
	Watch(root, key string, changed func()) (stop func())
	// Drain claims every message waiting for key and returns each as the
	// text the session's model reads for it, oldest first. relation, when
	// not nil, names how a sender (its reply-id) relates to the session
	// ("parent", "child", "peer"; "" for none), for the message's head.
	Drain(root, key string, relation func(sender string) string) ([]ClaimedMail, error)
	// Return puts texts, mail a Drain claimed and the session never read,
	// back in key's mailbox for its next claim: the session moved to a
	// terminal harness, whose own mailbox wait delivers it.
	Return(root, key string, texts []string) error
}

// ClaimedMail is one message Drain claimed: the text the session reads,
// and its sender's reply-id, "" for system mail or a sender with none.
type ClaimedMail struct {
	Text   string
	Sender string
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
	// DaemonVersion bounds hosted package registry roots to this daemon's minor.
	DaemonVersion string
	// Dir is the dashboard daemon's state directory; conversations live in
	// its session registry (wsdashboard.SessionDir).
	Dir      string
	Backends map[string]BackendRegistration
	Keys     KeyStore
	// Config is the base of each profile resolution; RepoRoot is replaced by
	// the conversation's working root.
	Config wsconfig.Options
	// AgyModelWaitTimeoutSeconds is the selected hosted package's manifest
	// default for agy.model_wait_timeout_seconds. Nil uses the ws default.
	AgyModelWaitTimeoutSeconds *int
	// AgyModelWaitRetries is the selected hosted package's manifest default
	// for agy.model_wait_retries. Nil uses the ws default.
	AgyModelWaitRetries *int
	// AgyModelWaitBackoff returns the delay before automatic continue attempt
	// n (one-based). Nil uses bounded exponential jitter.
	AgyModelWaitBackoff func(attempt int) time.Duration
	// WSMCP is the stdio ws-mcp command serving root, without the runner's
	// environment; nil refuses profiles that allow ws tools.
	WSMCP func(root string) (MCPCommand, error)
	// LeadManual renders the workflow manual of a hosted lead for its lead
	// key, as the session's own ws-mcp would serve it from the selected
	// package: backend is the lead's harness, dir its working directory.
	// The lead's launch appends it to the harness's system prompt
	// (composeLeadInstructions), so the session reads it from its first
	// turn. internal/mcp renders the manual and imports this package, so the
	// daemon supplies the function; nil, or an error, leaves the prompt
	// with the key sentence alone.
	LeadManual func(backend, dir, key string) (string, error)
	// PlaybookCommands lists the lead playbooks a hosted lead of a backend
	// with BackendRegistration.PlaybookCommands offers as dashboard
	// commands. The runner reads it once and keeps the list for its
	// lifetime; nil, or an error, offers none.
	PlaybookCommands func() ([]wslive.SlashCommand, error)
	// ExpandCommand renders the body of playbook command name for the
	// hosted lead whose key is key (backend its harness, dir its working
	// directory): the body the session's own playbook.read returns, which
	// the turn running the command carries, so the session needs no read of
	// its own. internal/mcp renders it, so the daemon supplies the function;
	// nil, or an error, sends a pointer to that read instead.
	ExpandCommand func(backend, dir, key, name string) (string, error)
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
	// Timer starts the lever hold's cap timer (leverHoldCap): a channel
	// that fires once d passed, and a stop that ends it early; nil uses
	// time.NewTimer.
	Timer func(d time.Duration) (<-chan time.Time, func())
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
	// CardBound reports whether conversation id is bound to a subordinate
	// whiteboard agent card (whose Record.Parent stays unset); nil defaults
	// to false. Used by IsChild for both HTTP settings refusal and SessionView.
	CardBound func(id string) bool
	// TrackerChanged, when not nil, is called after a conversation's
	// successful tracker write (an Event with TrackerWrite set), so the
	// daemon re-reads its tracker stores. It may be called from any
	// goroutine.
	TrackerChanged func()
	// AgyActivity, when not nil, is called after a live agy conversation's
	// turn settles. It may be called from any conversation loop and must not
	// block on the runner.
	AgyActivity func()
	// Report, when not nil, receives each settled turn of a conversation
	// with a parent (Runner.Spawn): a turn's end, a failed start, a process
	// lost mid-turn, and, when the next daemon loads the record, a turn
	// this daemon's exit cut off. Closing the conversation reports nothing.
	// It runs on the conversation's loop and must not block on the runner.
	Report func(TurnReport)
	// LiveHost is the daemon's in-memory snapshot hand-off. The daemon sets
	// it before Start; nil keeps the legacy file-only projection.
	LiveHost wsdashboard.LiveHost
	// ChildCheck, when not nil, is asked before each call of the token
	// route (children.go): an error refuses the call with its text (no
	// usable Pi, say).
	ChildCheck func() error
	// heartbeat overrides wslive.HeartbeatInterval for tests.
	heartbeat time.Duration
	// WakeRefused, when not nil, tells the sender of a message Deliver
	// queued for a sleeping conversation whose start admission refused
	// now (from: the sender's reply-id, id: the conversation, reason).
	WakeRefused func(from, id, reason string)
	// ChildWaitingDays overrides agent.child_waiting_days; 0 reads it.
	ChildWaitingDays int
	// spawnListed observes global spawn-projection listings in tests.
	spawnListed func(string)
	// stateInboxRead observes state-held inbox scans in tests.
	stateInboxRead func(string)
	// closedSnapshotRead observes closed/handed-off snapshot reads in tests.
	closedSnapshotRead func(string)
	// sendAdmitting observes the revival admission fence in tests.
	sendAdmitting func(string)
}

// Runner hosts the agent conversations of one dashboard daemon.
type Runner struct {
	opts   Options
	notify func(replyID, project string)
	// fixedProfile belongs only to a private one-shot answer runner. Freeze
	// the checked ceiling so config edits cannot widen it between create
	// and process start. Ordinary conversations still resolve on every start.
	fixedProfile *ResolvedProfile

	mu            sync.Mutex
	convs         map[string]*conv
	closed        map[string]Record
	childrenIndex map[ParentRef]map[string]bool // mu; parent identity is immutable
	pendingSpawns map[ParentRef]int             // mu; in-flight width-checked spawns until registered
	// expiry is childWaitingExpiry's value, read once (expiryOnce).
	expiryOnce sync.Once
	expiry     time.Duration
	quit       chan struct{}
	wg         sync.WaitGroup
	once       sync.Once
	// summaryMu only guards the two maps below, briefly; it is never held
	// across another lock or I/O (summaryLock, setSummaryOwner,
	// summaryOwner). summaryLocks holds one mutex per reply-id, created
	// lazily and never deleted, so every conversation generation of a
	// session shares it: a writer holds its own session's mutex through
	// summary mutation/ring I/O (lockSummaryOwner) without blocking another
	// session's snapshot publish, and summaryOwners[id] changes only under
	// that mutex.
	summaryMu     sync.Mutex
	summaryOwners map[string]*conv
	summaryLocks  map[string]*sync.Mutex
	// closedRows caches immutable closed states under mu, until reopened.
	// Handed-off snapshots still belong to a terminal and are not cached.
	closedRows map[string]State
	// resumeMu serializes Pi child revival with close cascades without
	// holding mu over store writes or conversation startup.
	resumeMu sync.Mutex
	// hireMu serializes idempotent owner-approved creation retries.
	hireMu sync.Mutex
	// scanned is closed once the holder manager's first scan returned, or
	// at Start when there is no manager (scanDone); scanOnce closes it.
	scanned  chan struct{}
	scanOnce sync.Once
	// rejoined gates nested calls until held-process capabilities are validated.
	rejoined   chan struct{}
	rejoinOnce sync.Once
	// playbookCmds is Options.PlaybookCommands' list, read once
	// (playbookCommands) under playbookMu.
	playbookMu     sync.Mutex
	playbookLoaded bool
	playbookCmds   []wslive.SlashCommand

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
	// mcpTokens are the hosted ws-mcp session tokens (mcp_session.go) by
	// token, and mcpTokenOf by conversation id.
	mcpTokens  map[string]*mcpToken
	mcpTokenOf map[string]string
}

// NewRunner returns a runner; Start loads its conversations.
func NewRunner(opts Options) *Runner {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Timer == nil {
		opts.Timer = func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTimer(d)
			return t.C, func() { t.Stop() }
		}
	}
	if opts.AgyModelWaitBackoff == nil {
		opts.AgyModelWaitBackoff = defaultAgyModelWaitBackoff
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

// SetLiveHost binds the daemon's RAM projection before Start.
func (r *Runner) SetLiveHost(host wsdashboard.LiveHost) { r.opts.LiveHost = host }

// Start loads every conversation record from the session registry, with no
// process (lazy resume), and starts serving them. notify is called after
// each write to a conversation's stores, so the daemon re-reads it. The
// daemon calls Start once it holds its lock.
func (r *Runner) Start(notify func(replyID, project string)) {
	if notify != nil {
		r.notify = notify
	}
	entries, _ := wsstate.ReadDir(filepath.Join(r.opts.Dir, "sessions"))
	var open []Record
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
		if rec.ClosedAt != "" || rec.handedOff() {
			// A handed-off session is a terminal harness's: nothing here
			// writes its row, watches its mail, or starts a process for it.
			r.loadClosed(rec)
		} else {
			open = append(open, rec)
		}
	}
	gone := map[string]bool{}
	for _, id := range r.expiredAtStart(open) {
		// Deepest first: a child waiting past its expiry is deleted with
		// its descendants, as the heartbeat would (expire).
		sessionDir := wsdashboard.SessionDir(r.opts.Dir, id)
		wslive.DropRing(wslive.RingDir(sessionDir))
		if err := os.RemoveAll(sessionDir); err != nil {
			fmt.Fprintf(r.opts.Log, "agent %s: delete: %v\n", id, err)
			continue
		}
		gone[id] = true
	}
	open = slices.DeleteFunc(open, func(rec Record) bool { return gone[rec.ReplyID] })
	orphans := orphansAtStart(open)
	for _, rec := range open {
		if orphans[rec.ReplyID] && r.closeAtStart(rec) {
			continue
		}
		// A lead whose process may still run under its holder rejoins
		// it there, before the scan below replays the holder's records.
		// A turn the last daemon's exit cut off settles there too.
		r.startConv(rec, nil)
	}
	// All records are loaded now, including children sorted after their lead.
	for _, st := range r.List() {
		if c := r.conv(st.ID); c != nil {
			c.do(func() {
				// Release a persisted HeldTurn whose background work ended
				// with the previous daemon (open children still hold it).
				if c.rec.Parent != nil && c.rec.HolderID == "" {
					c.releaseHeld()
				}
				c.spawnDirty = true
				c.live.writeSnapshot()
			})
		}
	}
	// A child loaded before its parent found no parent to report to.
	r.deliverPending()
	r.startHeartbeatSweeper()
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

// loadClosed lists closed (or handed-off) record rec at Start.
func (r *Runner) loadClosed(rec Record) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed[rec.ReplyID] = rec
	r.indexChildLocked(rec)
	st := r.closedState(rec) // prime immutable presentation once at startup
	if r.closedRows == nil {
		r.closedRows = map[string]State{}
	}
	r.closedRows[rec.ReplyID] = st
}

// closeAtStart closes open record rec at Start, with no loop started, and
// reports whether it did.
func (r *Runner) closeAtStart(rec Record) bool {
	now := r.opts.Now()
	rec.ClosedAt = now.UTC().Format(time.RFC3339)
	// An orphan's cut-off turn ends here, as close ends a running one.
	finishActiveTurn(&rec, now.UnixMilli())
	rec.TurnOpen = false
	if err := writeRecord(wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID), rec); err != nil {
		fmt.Fprintf(r.opts.Log, "agent %s: record write: %v\n", rec.ReplyID, err)
		return false
	}
	if err := wsdashboard.WriteRegistration(r.opts.Dir, registration(rec, now)); err != nil {
		fmt.Fprintf(r.opts.Log, "agent %s: registration write: %v\n", rec.ReplyID, err)
	}
	r.notify(rec.ReplyID, rec.Root)
	r.loadClosed(rec)
	return true
}

// orphansAtStart picks the open spawned children a daemon start closes
// instead of serving (closeAtStart): one whose conversation parent is
// closed or gone, so no report of it reaches one, or that descends from
// one. A finished child is not closed: it sleeps (agent_tools.go).
func orphansAtStart(open []Record) map[string]bool {
	isOpen := map[string]bool{}
	for _, rec := range open {
		isOpen[rec.ReplyID] = true
	}
	done := map[string]bool{}
	stays := func(id string) bool { return isOpen[id] && !done[id] }
	for changed := true; changed; {
		changed = false
		for _, rec := range open {
			id := rec.ReplyID
			// Legacy direct non-Pi children keep their existing next-launch
			// admission refusal when a parent disappears. Held workers of
			// every backend use the former Pi orphan cleanup.
			if done[id] || rec.Lead || rec.Parent == nil || (rec.Backend != BackendPi && rec.HolderID == "") {
				continue
			}
			if rec.Parent.Kind == ParentConversation && !stays(rec.Parent.ReplyID) {
				done[id], changed = true, true
			}
		}
	}
	return done
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
			if c.hidden {
				c.stopProcess("")
				if err := c.retireHolder(); err != nil {
					c.setStatus(func() { c.lastErr = "archive process: " + err.Error() })
				}
			}
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
		if json.Unmarshal(info.Session, &meta) != nil || (meta.Kind != holderKindLead && meta.Kind != holderKindPiWorker && meta.Kind != holderKindAgentWorker) || owned[info.ID] || r.holders.isRejected(info.ID) {
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

// summaryLock returns replyID's summary writer mutex, the same one for
// every conversation generation of the session. Only the map guard is
// taken, and it is released before the caller locks the mutex.
func (r *Runner) summaryLock(replyID string) *sync.Mutex {
	r.summaryMu.Lock()
	defer r.summaryMu.Unlock()
	if r.summaryLocks == nil {
		r.summaryLocks = map[string]*sync.Mutex{}
	}
	l := r.summaryLocks[replyID]
	if l == nil {
		l = &sync.Mutex{}
		r.summaryLocks[replyID] = l
	}
	return l
}

// setSummaryOwner makes c replyID's summary writer generation. It waits
// for a writer of the session that holds the summary lock (a ring or
// snapshot write, or a heartbeat sweep), so the owner never changes under
// one.
func (r *Runner) setSummaryOwner(replyID string, c *conv) {
	l := r.summaryLock(replyID)
	l.Lock()
	defer l.Unlock()
	r.summaryMu.Lock()
	defer r.summaryMu.Unlock()
	if r.summaryOwners == nil {
		r.summaryOwners = map[string]*conv{}
	}
	r.summaryOwners[replyID] = c
}

// summaryOwner reads replyID's summary writer generation, unpinned: it can
// change once this returns. Use lockSummaryOwner to decide a write on it.
func (r *Runner) summaryOwner(replyID string) *conv {
	r.summaryMu.Lock()
	defer r.summaryMu.Unlock()
	return r.summaryOwners[replyID]
}

// startConv starts serving rec. It writes the registration and the
// snapshot, and tells the daemon, before it takes r.mu to list the
// conversation, so no other conversation's Get, List, or act waits on
// those writes; r.mu is not held. seed, when not empty, is the command
// list the first snapshot shows (lastCommands); a process's own list
// replaces it.
func (r *Runner) startConv(rec Record, seed []wslive.SlashCommand) *conv {
	return r.startConvWith(rec, seed, false, nil)
}

func (r *Runner) startConvWith(rec Record, seed []wslive.SlashCommand, spawnPending bool, claimWidth func()) *conv {
	dir := wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID)
	c := &conv{
		r:            r,
		dir:          dir,
		inbox:        wsinbox.Dir(dir),
		rec:          rec,
		live:         newLiveWriter(dir, rec.WSKey, rec.Backend, r.opts.Now, r.opts.Log),
		reqs:         make(chan func()),
		inboxC:       make(chan struct{}, 1),
		spawnC:       make(chan struct{}, 1),
		spawnDirty:   true,
		spawnPending: spawnPending,
		mailC:        make(chan struct{}, 1),
		done:         make(chan struct{}),
		settleC:      make(chan struct{}),
	}
	if !rec.Lead {
		if level, source, err := r.currentWorkerPermission(rec); err == nil {
			c.workerPermissionLevel = level
			c.workerPermissionSource = source
		} else {
			c.lastErr = appendDiagnostic(c.lastErr, "worker permission selection: "+err.Error())
		}
	}
	c.refreshHeld()
	if rec.Lead {
		_, _, _ = refreshPermissionSelection(dir)
		r.setSummaryOwner(rec.ReplyID, c)
		c.live.owner = c.lockSummaryWriter
	}
	if rec.Lead {
		hidden, err := wsdashboard.ReadSessionHidden(r.opts.Dir, rec.ReplyID)
		if err != nil {
			// Fail closed before any startup inbox/mail/replay signal can kick.
			hidden = true
			c.lastErr = "read session visibility: " + err.Error()
			fmt.Fprintf(r.opts.Log, "agent %s: %s\n", rec.ReplyID, c.lastErr)
		}
		c.hidden = hidden
	}
	c.live.wake = func() {
		if slot := c.live.currentSlot(); slot != nil {
			slot.RingAppended()
		} else {
			r.notify(rec.ReplyID, rec.Root)
		}
	}
	c.live.beforeSnapshot = c.seedSpawnRows
	c.observations = c.live.st.ObservedResponses("")
	own := c.live.st.Snapshot(r.opts.Now().UnixMilli())
	if own.Lead.Observed != nil {
		observed := own.Lead.Observed.Current
		c.observed = &observed
	}
	if own.Lead.OutputRate != nil {
		rate := own.Lead.OutputRate.OutputTps
		c.outputTps = &rate
	}
	if usage := own.Usage; usage != nil && usage.Context != nil && usage.Context.Tokens != nil {
		tokens := float64(*usage.Context.Tokens)
		c.contextTokens = &tokens
	}
	if seed == nil && rec.Lead {
		// A rejoined lead's list is its snapshot's, which an earlier daemon
		// may have written without this runner's playbook commands.
		if merged := r.withPlaybookCommands(rec, own.Commands); !slices.Equal(merged, own.Commands) {
			seed = merged
		}
	}
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
	foreign := c.isForeign()
	if !foreign {
		if r.opts.LiveHost != nil {
			if slot, ok := r.opts.LiveHost.OpenHostedSlot(dir); ok {
				c.live.setSlot(slot)
				if folder, ok := slot.(wsdashboard.ObservedFoldSlot); ok && c.live.observedSeeded {
					folder.FoldsObserved()
				}
			}
		}
		if rec.Lead {
			// Bind after the slot is fixed: summary retries can wake off-loop.
			c.live.summaryStore().SetWake(c.live.wake)
		}
		c.live.writeSnapshot()
	}
	if rec.HolderID != "" && !foreign {
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
	r.indexChildLocked(rec)
	delete(r.closed, rec.ReplyID)
	delete(r.closedRows, rec.ReplyID)
	if claimWidth != nil {
		claimWidth()
	}
	r.wg.Add(1)
	go c.loop()
	r.mu.Unlock()
	// Do not expose a previous LastTurn while startup still reconciles the
	// later TurnOpen. The loop reconciles before it accepts this request.
	c.do(func() {})
	if (rec.Lead || rec.Parent != nil) && r.opts.Mail != nil && !foreign {
		// Mail that waited while no daemon ran is found by the first check.
		// A spawned child is watched asleep too: mail to its address wakes
		// it.
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
	if rec.ClosedAt == "" && rec.handedOff() {
		return errHandedOff
	}
	return errClosed
}

// LeadOptions are a lead conversation's spawn choices; an empty one takes
// the profile's value (the harness default when it has none).
type LeadOptions struct {
	// HireRequestID and HireLabel are set only by CreateHireLead, never HTTP.
	HireRequestID   string
	HireLabel       string
	Model           string
	Effort          string
	PermissionMode  string
	PluginIsolation string
	// Common intent is transient; selection is stored separately from Record.
	PermissionLevel   PermissionLevel
	PermissionFullAck bool
	permissionUser    bool // set by the trusted control owner, never request JSON
}

// leadPluginIsolation supplies the additive field's legacy default.
func leadPluginIsolation(value string) string {
	if value == "" {
		return "default"
	}
	return value
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
	profile := LeadProfileFor(strings.TrimSpace(backend), r.opts.Backends)
	if profile == "" {
		return State{}, categorized(ErrInvalidProfile, badRequest("harness must be one of %s; got %q", strings.Join(r.leadHarnesses(), ", "), backend))
	}
	if opts.Model != "" && !modelPattern.MatchString(opts.Model) {
		return State{}, categorized(ErrInvalidProfile, badRequest("model %q must match %s", opts.Model, modelPattern.String()))
	}
	if opts.Effort != "" && !effortPattern.MatchString(opts.Effort) {
		return State{}, categorized(ErrInvalidProfile, badRequest("effort %q must be a lowercase word", opts.Effort))
	}
	modes := permissionModes(strings.TrimSpace(backend), []map[string]BackendRegistration{r.opts.Backends})
	if opts.PermissionMode != "" && !slices.Contains(modes, opts.PermissionMode) {
		return State{}, categorized(ErrInvalidProfile, badRequest("permission_mode %q must be one of %s", opts.PermissionMode, strings.Join(modes, ", ")))
	}
	opts.PluginIsolation = leadPluginIsolation(opts.PluginIsolation)
	if opts.PluginIsolation != "default" && opts.PluginIsolation != "isolated" {
		return State{}, categorized(ErrInvalidProfile, badRequest("plugin_isolation must be default or isolated; got %q", opts.PluginIsolation))
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
	if info, err := wsstate.Stat(root); err != nil || !info.IsDir() {
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
	if spawn != nil {
		if spawn.Tier != "" && rp.Definition.Backend != BackendPi && !isAgentWorkerProfile(rp.Name) {
			return State{}, badRequest("pi.spawn: profile %q must be a non-lead Pi profile", rp.Name)
		}
		if err := r.checkChildProfile(spawn.Parent, rp.Name, rp.Definition); err != nil {
			return State{}, err
		}
		level, _, err := resolveWorkerPermission(rp.Definition, spawn.PermissionLevel, spawn.PermissionLevel != "", nil)
		if err != nil {
			return State{}, err
		}
		if err := r.admitWorkerLevel(spawn.Parent, level); err != nil {
			return State{}, err
		}
	}
	if lead != nil && lead.PermissionLevel != "" {
		base := Record{Lead: true, Backend: rp.Definition.Backend, PermissionMode: rp.Definition.PermissionMode}
		if lead.PermissionMode != "" {
			base.PermissionMode = lead.PermissionMode
		}
		if !r.supportsLeadPermission(base.Backend, lead.PermissionLevel) {
			return State{}, badRequest("permission_level %q is not supported on %s", lead.PermissionLevel, base.Backend)
		}
		if err := authorizePermission(legacyLeadPermission(base), lead.PermissionLevel, lead.permissionUser, lead.PermissionFullAck); err != nil {
			return State{}, err
		}
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
	} else {
		keyRoot := root
		if spawn != nil && rp.Definition.Backend == BackendPi && spawn.Cwd != "" {
			keyRoot = spawn.Cwd
		}
		if key, err = r.opts.Keys.Mint(keyRoot); err != nil {
			return State{}, fmt.Errorf("mint ws session key: %w", err)
		}
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
		rec.PluginIsolation = leadPluginIsolation(lead.PluginIsolation)
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
		if len(r.opts.Backends[rec.Backend].PermissionModes) == 0 {
			rec.PermissionMode = ""
		}
		rec.selectPermissionMode(rec.PermissionMode)
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
	if lead != nil && lead.PermissionLevel != "" {
		if err := writePermissionSelection(dir, lead.PermissionLevel); err != nil {
			return State{}, err
		}
		if mode := nativePermissionMode(rec.Backend, lead.PermissionLevel); mode != "" {
			rec.PermissionMode = mode
		}
	}
	if spawn != nil && spawn.PermissionLevel != "" {
		if err := writePermissionSelection(dir, spawn.PermissionLevel); err != nil {
			return State{}, err
		}
	}
	if spawn != nil && spawn.SystemPrompt != "" {
		if err := writeFileAtomic(filepath.Join(dir, systemPromptFile), []byte(spawn.SystemPrompt), 0o600); err != nil {
			return State{}, err
		}
	}
	if lead != nil && lead.HireRequestID != "" {
		if err := wsdashboard.MarkHired(r.opts.Dir, rec.ReplyID, lead.HireRequestID, lead.HireLabel); err != nil {
			return State{}, err
		}
	}
	if err := writeRecord(dir, rec); err != nil {
		return State{}, err
	}
	var seed []wslive.SlashCommand
	if rec.Lead {
		// A new lead offers its playbook commands before its process
		// first reports the harness's own.
		seed = r.withPlaybookCommands(rec, r.lastCommands(rec))
	}
	var claimWidth func()
	if spawn != nil {
		claimWidth = spawn.claimWidth
	}
	c := r.startConvWith(rec, seed, claimWidth != nil, claimWidth)
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
	var rp ResolvedProfile
	var err error
	if r.fixedProfile != nil {
		if profile != r.fixedProfile.Name {
			return ResolvedProfile{}, badRequest("private answer runner profile mismatch")
		}
		rp = *r.fixedProfile
	} else {
		rp, err = ResolveProfile(r.opts.Config, root, strings.TrimSpace(profile), r.opts.Backends)
	}
	if err != nil {
		return ResolvedProfile{}, categorized(ErrInvalidProfile, badRequest("%v", err))
	}
	if _, err := r.backend(rp.Name, rp.Definition.Backend); err != nil {
		return ResolvedProfile{}, err
	}
	if path := rp.Definition.InstructionsPath(root); path != "" {
		info, err := wsstate.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return ResolvedProfile{}, categorized(ErrInvalidProfile, badRequest("agent profile %q: instruction file %s does not exist", rp.Name, path))
		}
	}
	if len(rp.Definition.WSTools) > 0 && r.opts.WSMCP == nil {
		return ResolvedProfile{}, categorized(ErrInvalidProfile, badRequest("agent profile %q allows ws tools, but this daemon cannot attach ws-mcp", rp.Name))
	}
	if rp.Definition.Lead && (r.opts.Holders == nil || r.opts.LeadKeys == nil) {
		return ResolvedProfile{}, categorized(ErrInvalidProfile, badRequest("agent profile %q is a lead profile, but this daemon runs no session holders", rp.Name))
	}
	return rp, nil
}

// backend is shared by profile validation and both launch paths. Keep
// unavailable backends explicit, including when rejoining a held process.
func (r *Runner) backend(profile, name string) (Backend, error) {
	backend := r.opts.Backends[name].Backend
	if backend == nil {
		return nil, categorized(ErrBackendUnavailable, badRequest("agent profile %q: backend %q is not available in this daemon", profile, name))
	}
	return backend, nil
}

func (c *conv) startBackend(spec StartSpec) (Process, error) {
	backend, err := c.r.backend(c.rec.Profile, c.rec.Backend)
	if err != nil {
		return nil, err
	}
	proc, err := backend.Start(context.Background(), spec)
	return proc, categorized(ErrLaunchFailed, err)
}

// Get reads one conversation's state.
func (r *Runner) Get(id string) (State, error) {
	r.mu.Lock()
	c, open := r.convs[id]
	rec, closed := r.closed[id]
	var st State
	if closed && !open {
		st = r.closedState(rec)
	}
	r.mu.Unlock()
	switch {
	case open:
		c.refreshHeld() // explicit API reads retain authoritative held counts
		if c.record().Lead {
			revalidatePermissionSelection(c.dir)
		}
		return c.state(), nil
	case closed:
		return st, nil
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
		out = append(out, r.memoryClosedState(rec))
	}
	r.mu.Unlock()
	for _, c := range convs {
		out = append(out, c.memoryState())
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
// A closed spawned Pi child is reopened on its existing session file;
// other closed conversations still refuse sends.
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
	st, err := r.send(id, text)
	if err != errClosed {
		return st, err
	}
	if rec, ok := r.recordOf(id); ok && !rec.Lead && rec.Parent != nil && rec.Backend == BackendPi {
		// A close may land between the open check and the inbox write.
		// Once that close is done (resumeMu), a second try reopens it as
		// any closed Pi child.
		r.resumeMu.Lock()
		r.resumeMu.Unlock()
		st, err = r.send(id, text)
	}
	return st, err
}

func (r *Runner) send(id, text string) (State, error) {
	c, err := r.open(id)
	if err != nil {
		// Keep revival and admission in one fence against explicit close.
		// Open sends do not take resumeMu, so a slow manual close still
		// refuses them promptly rather than blocking behind its cascade.
		r.resumeMu.Lock()
		defer r.resumeMu.Unlock()
		c, err = r.resumePiChild(id)
		if err != nil {
			return State{}, err
		}
	}
	if r.opts.sendAdmitting != nil {
		r.opts.sendAdmitting(id)
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
		c.held++
		c.rec.Sends = c.sends
		c.cancelModelWaitBackoffForInput()
		if c.rec.Parent != nil {
			// Any message resets a spawned child's waiting expiry.
			c.rec.LastActiveAt = r.opts.Now().UTC().Format(time.RFC3339)
		}
	}
	c.mu.Unlock()
	if err != nil {
		return State{}, err
	}
	// The inbox already owns the text durably. Coverage and the child's
	// activity stamp ride the next immediate record write or shutdown flush.
	c.recordDirty = true
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
		c.mu.Lock()
		held, _, err := wsinbox.Interrupt(c.inbox, r.opts.Now())
		if err == nil {
			c.held = 0
		}
		c.mu.Unlock()
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

// IdleLead is a lead session SleepIdle may stop: one JSON entry of the
// idle list.
type IdleLead struct {
	ID      string `json:"id"`
	Label   string `json:"label,omitempty"`
	Backend string `json:"backend"`
}

// sleepable reports a lead conversation whose process may be stopped for
// power saving: it runs, and no turn, replay, background task, active child
// or approval prompt keeps it busy (the idle timer's own test, idleNow).
// Run in the conversation's loop.
func (c *conv) sleepable() bool {
	return c.rec.Lead && !c.closed && c.proc != nil && len(c.approvals) == 0 && c.idleNow()
}

// IdleLeads lists the lead sessions with a running process that SleepIdle
// would stop, and counts the other running ones (busy or waiting for an
// approval), which it keeps.
func (r *Runner) IdleLeads() (idle []IdleLead, kept int) {
	idle = []IdleLead{}
	r.mu.Lock()
	convs := make([]*conv, 0, len(r.convs))
	for _, c := range r.convs {
		convs = append(convs, c)
	}
	r.mu.Unlock()
	created := map[string]string{}
	for _, c := range convs {
		c.do(func() {
			if !c.rec.Lead || c.closed || c.proc == nil {
				return
			}
			if !c.sleepable() {
				kept++
				return
			}
			idle = append(idle, IdleLead{ID: c.rec.ReplyID, Label: c.rec.Label, Backend: c.rec.Backend})
			created[c.rec.ReplyID] = c.rec.CreatedAt
		})
	}
	sort.Slice(idle, func(i, j int) bool {
		if a, b := created[idle[i].ID], created[idle[j].ID]; a != b {
			return a < b
		}
		return idle[i].ID < idle[j].ID
	})
	return idle, kept
}

// SleepIdle stops each named lead session as Stop does, but only when it is
// still idle in its own loop; one that became busy (or is no lead process
// any more) is skipped. Held messages stay held and the next send resumes
// a stopped session.
func (r *Runner) SleepIdle(ids []string) (slept, skipped []string) {
	slept, skipped = []string{}, []string{}
	for _, id := range ids {
		stopped := false
		_, err := r.act(id, func(c *conv) error {
			if c.sleepable() {
				c.stopProcess("")
				stopped = true
			}
			return nil
		})
		if err == nil && stopped {
			slept = append(slept, id)
		} else {
			skipped = append(skipped, id)
		}
	}
	return slept, skipped
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

func (r *Runner) isChildRecord(rec Record) bool {
	if rec.Parent != nil {
		return true
	}
	return r != nil && r.opts.CardBound != nil && r.opts.CardBound(rec.ReplyID)
}

// IsChild reports whether id is a child conversation whose settings are fixed
// at spawn: either a spawned worker (Record.Parent != nil) or a card-bound
// subordinate lead reported by Options.CardBound.
func (r *Runner) IsChild(id string) bool {
	r.mu.Lock()
	c, open := r.convs[id]
	rec, closed := r.closed[id]
	r.mu.Unlock()
	if open {
		return r.isChildRecord(c.record())
	}
	if closed {
		return r.isChildRecord(rec)
	}
	return r.opts.CardBound != nil && r.opts.CardBound(id)
}

// SessionMeta returns in-memory composer control metadata for replyID without
// touching disk (wsdashboard.AgentSessionMetaHost).
func (r *Runner) SessionMeta(replyID string) (wsdashboard.AgentSessionMeta, bool) {
	r.mu.Lock()
	c, open := r.convs[replyID]
	rec, closed := r.closed[replyID]
	r.mu.Unlock()
	if open {
		rec = c.record()
	} else if !closed {
		return wsdashboard.AgentSessionMeta{}, false
	}
	return wsdashboard.AgentSessionMeta{
		Hosted:  rec.Lead,
		IsChild: r.isChildRecord(rec),
		Model:   rec.Model,
		Effort:  rec.Effort,
	}, true
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

// SetPermissionMode changes an admitted lead conversation's permission mode.
// Like a model or effort change it applies at the next turn boundary,
// in-stream when the harness takes it; the mode the harness then reports is
// State.ActiveMode.
func (r *Runner) SetPermissionMode(id, mode string) (State, error) {
	return r.setPermissionMode(id, mode, false)
}

func (r *Runner) setPermissionMode(id, mode string, user bool) (State, error) {
	modes := LeadPermissionModes
	if rec, ok := r.recordOf(id); ok {
		modes = permissionModes(rec.Backend, []map[string]BackendRegistration{r.opts.Backends})
	}
	if !slices.Contains(modes, mode) {
		return State{}, categorized(ErrInvalidProfile, badRequest("permission_mode %q must be one of %s", mode, strings.Join(modes, ", ")))
	}
	return r.act(id, func(c *conv) error {
		return c.setNativePermissionMode(mode, user)
	})
}

// CloseConversation ends the conversation: its process stops, held
// messages are dropped, and its session directory stays readable. A spawned
// Pi child can be reopened by Send; other closed conversations refuse it.
// Its open descendants through conversation parents, and a lead's Pi
// children (children.go), close first, deepest first. A conversation parent
// whose done turn waited for this child's report is released.
func (r *Runner) CloseConversation(id string) (State, error) {
	r.resumeMu.Lock()
	defer r.resumeMu.Unlock()
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
	r.releaseParent(id)
	return st, nil
}

// releaseParent releases the done turn conversation id's conversation
// parent held for it (releaseHeld), once id closed.
func (r *Runner) releaseParent(id string) {
	if rec, ok := r.recordOf(id); ok && rec.Parent != nil && rec.Parent.Kind == ParentConversation {
		if parent := r.conv(rec.Parent.ReplyID); parent != nil {
			parent.do(parent.releaseHeld)
		}
	}
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
		if r.closedRows == nil {
			r.closedRows = map[string]State{}
		}
		r.closedRows[id] = st
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
	Process bool `json:"process"`
	// Hidden is dashboard sidecar state, never part of the runner record.
	Hidden    bool                         `json:"hidden,omitempty"`
	Lifecycle wsdashboard.SessionLifecycle `json:"lifecycle,omitempty"`
	Model     string                       `json:"model,omitempty"`
	Effort    string                       `json:"effort,omitempty"`
	// Observed is this conversation's own response evidence, not its settings
	// or a descendant's output. It remains available when the turn ends.
	Observed *wslive.ModelObservation `json:"observed,omitempty"`
	// Observations is an internal reconciliation window, not a REST payload.
	// Parent refresh notifications coalesce; latest-only state would lose changes.
	Observations []wslive.ModelObservation `json:"-"`
	// ContextTokens and OutputTps are the child's own latest usage; the
	// approximate streaming rate is absent when no turn runs.
	ContextTokens *float64           `json:"contextTokens,omitempty"`
	OutputTps     *float64           `json:"outputTps,omitempty"`
	Batches       []wslive.BatchView `json:"batches,omitempty"`
	// Held counts the messages waiting for the next turn.
	Held int `json:"held"`
	// Page is the conversation's dashboard page path.
	Page string `json:"page"`
	// Error combines the last launch, turn, or command error (cleared when a
	// turn starts) with current lead advisories, which clear on recovery.
	Error     string `json:"error,omitempty"`
	CreatedAt string `json:"created_at"`
	ClosedAt  string `json:"closed_at,omitempty"`
	// HandedOffAt is when the session was handed to a terminal harness;
	// set with the handed_off status alone.
	HandedOffAt string `json:"handed_off_at,omitempty"`

	// Lead marks a hosted lead session; the fields below are its own.
	Lead bool `json:"lead,omitempty"`
	// IsChild marks a spawned worker or card-bound subordinate session whose
	// model and effort are fixed at spawn (read-only on HTTP settings writes).
	IsChild bool `json:"is_child,omitempty"`
	// PermissionMode is the mode the next process launches in, and
	// ActiveMode the one the running harness last reported (it may fall
	// back from the one asked for).
	PermissionMode string `json:"permission_mode,omitempty"`
	ActiveMode     string `json:"active_mode,omitempty"`
	// Common projections are lead-only. Worker common posture is not applied.
	PermissionLevel       PermissionLevel `json:"permission_level,omitempty"`
	PermissionSource      string          `json:"permission_source,omitempty"`
	PermissionEnforcement string          `json:"permission_enforcement,omitempty"`
	PluginIsolation       string          `json:"plugin_isolation,omitempty"`
	PluginWarning         string          `json:"plugin_warning,omitempty"`
	// Approvals are the tool permission prompts waiting for an answer,
	// oldest first.
	Approvals []ApprovalRequest `json:"approvals,omitempty"`
	// BackgroundTasks counts the harness's running background tasks, as its
	// stream last reported them.
	BackgroundTasks int `json:"background_tasks,omitempty"`
	// Background and Monitors are known shell/monitor counts, absent when
	// the harness only exposes an aggregate.
	Background *int `json:"background,omitempty"`
	Monitors   *int `json:"monitors,omitempty"`
	// ActionNeeded is set while the harness reports that it waits on the
	// user with no prompt in Approvals to answer; it says what to do.
	ActionNeeded string `json:"action_needed,omitempty"`
	// Failure is the typed failure (a login needed, a model unavailable, a
	// usage limit) the harness last reported, until a turn succeeds.
	Failure *wslive.Notice `json:"failure,omitempty"`
	// MCPBroken names the harness's MCP servers whose connection broke and
	// did not come back in-stream; a restart (Restart) is the way back.
	MCPBroken []string `json:"mcp_broken,omitempty"`
	// Capabilities is the catalog profile the process's harness version
	// resolved to, demoted by its init shape; Unverified in it is the
	// dashboard's "unverified version" warning. Absent for a backend
	// without a profile and before a process reported. Never persisted.
	Capabilities *Capabilities `json:"capabilities,omitempty"`
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
	// ActiveMs excludes the current turn; TurnStartedAt is set only while it runs.
	ActiveMs      int64 `json:"active_ms,omitempty"`
	TurnStartedAt int64 `json:"turn_started_at,omitempty"`
}

// memoryClosedState is a presentation-only read under r.mu. Explicit Get can
// still refresh a handed-off terminal's evidence through closedState.
func (r *Runner) memoryClosedState(rec Record) State {
	if st, ok := r.closedRows[rec.ReplyID]; ok {
		st.IsChild = r.isChildRecord(rec)
		return st
	}
	st := closedState(rec)
	st.IsChild = r.isChildRecord(rec)
	return st
}

// closedState is the state of a record no loop serves: closed, or handed
// to a terminal. The caller holds r.mu.
func (r *Runner) closedState(rec Record) State {
	if rec.ClosedAt != "" {
		if st, ok := r.closedRows[rec.ReplyID]; ok {
			st.IsChild = r.isChildRecord(rec)
			return r.permissionState(st, rec, wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID))
		}
	}
	st := closedState(rec)
	st.IsChild = r.isChildRecord(rec)
	if r.opts.closedSnapshotRead != nil {
		r.opts.closedSnapshotRead(rec.ReplyID)
	}
	dir := wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID)
	if snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(dir)); err == nil {
		if records, err := wslive.ReadObserved(dir); err == nil {
			wslive.OverlayObserved(&snap, records)
			for _, rec := range records {
				if rec.AgentID == "" {
					st.Observations = append(st.Observations, rec.ModelObservation)
				}
			}
		}
		if snap.Lead.Observed != nil {
			observed := snap.Lead.Observed.Current
			st.Observed = &observed
		}
		if snap.Usage != nil && snap.Usage.Context != nil && snap.Usage.Context.Tokens != nil {
			tokens := float64(*snap.Usage.Context.Tokens)
			st.ContextTokens = &tokens
		}
	}
	if st.Status == StatusHandedOff {
		st.ResumeCommand = resumeCommand(rec, wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID))
	} else {
		if r.closedRows == nil {
			r.closedRows = map[string]State{}
		}
		r.closedRows[rec.ReplyID] = st
	}
	return r.permissionState(st, rec, dir)
}

func closedState(rec Record) State {
	st := State{ID: rec.ReplyID, Profile: rec.Profile, Backend: rec.Backend, Root: rec.Root, Status: StatusClosed, Model: rec.Model, Effort: rec.Effort, Page: "/s/" + rec.ReplyID, CreatedAt: rec.CreatedAt, ClosedAt: rec.ClosedAt, Lead: rec.Lead, PermissionMode: rec.selectedPermissionMode(), Label: rec.Label, Tier: rec.Tier, ActiveMs: rec.ActiveMs}
	if rec.Lead {
		st.Lifecycle = wsdashboard.SessionPendingDeletion
		st.PluginIsolation = leadPluginIsolation(rec.PluginIsolation)
	}
	st.PluginWarning = rec.PluginWarning
	if rec.Parent != nil {
		parent := *rec.Parent
		st.Parent = &parent
	}
	if !rec.Lead {
		// A lead's Cwd is a taken-over terminal's, not a spawn choice.
		st.Cwd = rec.Cwd
	}
	if rec.ClosedAt == "" && rec.handedOff() {
		st.Status, st.HandedOffAt = StatusHandedOff, rec.HandedOffAt
	}
	return st
}
