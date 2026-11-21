package wsagent

import (
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
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
	// RestoreBundle renders fresh hosted session state at the point it is
	// delivered after compaction or daemon restart. children is the caller's
	// direct agent.list projection; the daemon owns the renderer so this
	// package does not read the ws session store itself. Nil keeps the Phase 1
	// key-bearing pointer note and disables restart restore.
	RestoreBundle func(backend, dir, key string, cause RestoreCause, children []AgentChild) (string, error)
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
	// startupLoaded closes after Start installed every open record. Restore
	// markers may be present before then, but compactIdle sends none until the
	// complete direct-child projection exists.
	startupMu     sync.RWMutex
	startupLoaded chan struct{}
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
