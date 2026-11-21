package wsdashboard

import (
	"errors"
	"github.com/gofrs/flock"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

var _ LiveHost = (*Daemon)(nil)

// ErrLocked reports that another daemon holds the single-instance lock.
var ErrLocked = errors.New("another dashboard daemon holds the lock")

// DefaultLockWait bounds how long a starting daemon waits for the lock. It
// covers a predecessor that is shutting down during a replacement handoff; a
// holder still alive after it is left running.
const DefaultLockWait = 5 * time.Second

const (
	lockRetryDelay   = 50 * time.Millisecond
	shutdownDeadline = 3 * time.Second
	// healthService names the daemon in its /health answer, so a client can
	// tell it from an unrelated server that took the recorded port.
	healthService = "ws-dashboard"
)

// DaemonOptions configures StartDaemon.
type DaemonOptions struct {
	// Dir is the state directory (Dir's result); it is created when absent.
	Dir string
	// Home isolates the daemon's local account-usage reader, including its
	// config search roots. Empty uses os.UserHomeDir and config environment
	// overrides; an explicit home uses only that home's .claude/.codex/.pi.
	Home string
	// ScratchCapture serves synthetic picker roots and empty account usage.
	ScratchCapture bool
	// Port is the preferred port; 0 asks the OS for one. A taken port falls
	// back to the port the last daemon bound (last-port file), then to an
	// OS-assigned one, so a restart keeps its origin while the preferred
	// port stays taken.
	Port int
	// LAN binds every interface instead of loopback only, and admits a
	// request that is not from this machine only with the LAN access token
	// (access.go).
	LAN bool
	// Version is reported by /health and written to the endpoint file.
	Version string
	// BuildTime is reported by /health (ExecutableBuildTime); zero is
	// unknown and is omitted.
	BuildTime int64
	// ContentHash is the daemon executable's sha256 (ExecutableContentHash),
	// reported by /health; empty is unknown and is omitted.
	ContentHash string
	// Executable is this daemon's executable, when supplied by the command
	// layer. Startup retains it and records its identity under the singleton
	// lock so hosted launchers can restart without a plugin installation.
	Executable string
	// Proxy builds the MCP sessions the daemon hosts for the launchers
	// that connect to ProxyPath (proxy.go); nil leaves the route off.
	Proxy ProxyHandler
	// LockWait bounds the wait for the single-instance lock; zero means
	// DefaultLockWait.
	LockWait time.Duration
	// Log receives diagnostics; nil discards them.
	Log io.Writer
	// Sessions reads registered sessions' stores for the session index; nil
	// runs the daemon without one (no polling, and /nudge answers 404).
	Sessions SessionSource
	Config   *ConfigBackend
	// PollInterval is the polling backstop period; zero means
	// DefaultPollInterval.
	PollInterval time.Duration
	// Agents is the agent runner the daemon hosts; nil serves no agent
	// routes.
	Agents AgentHost
	// LeadNudges starts after runner recovery and stops before the daemon
	// releases its cache-root lock. It runs without pages or an index.
	LeadNudges interface {
		Start()
		Close()
	}
	// BeforeUnlock runs in Close after the daemon has stopped serving and
	// before the cache-root lock is released, so work a successor must not
	// overlap (the mailbox export for a previous version) finishes while
	// the lock is still held. Nil runs nothing.
	BeforeUnlock func()
	// Tracker is the tracker host the tracker routes serve (tracker_routes.go);
	// nil serves no tracker route and sends no tracker stream event.
	Tracker TrackerHost
	// Worktrees reads a session's project worktree pool for the
	// pooled-worktree routes (worktrees.go); nil serves none.
	Worktrees WorktreeSource
	// Support serves the tracker support conversations under
	// SupportRoutePrefix (the agent runner's per-store conversations); nil
	// serves none.
	Support http.Handler
	// Holders is the session holder manager (holders.go): the daemon runs
	// its scans and serves the holders routes; nil runs none.
	Holders HolderHost
	// Pprof serves net/http/pprof under /debug/pprof/ to loopback viewers
	// only (pprof_route.go; PprofEnv); false serves no such route.
	Pprof bool
	// WorkspacesFile is the stored workspace list's file (workspaces.go),
	// which belongs in the config home; empty keeps no stored workspace and
	// serves no workspace route (workspace_routes.go).
	WorkspacesFile string
	// MailWake wakes the mailbox waits this process hosts: Wake after a
	// board-answer inbox write the routes made, Nudge on every /nudge
	// before its session is checked, since the nudge's sender may have
	// written mail any waiter watches. Nil wakes none.
	MailWake MailWaker
	// AgyCommand resolves the agy CLI program and leading arguments for
	// user-triggered /usage probes without spawning. Nil skips agy probes.
	AgyCommand func() (string, []string, error)
	// CardBound reports whether replyID is bound to a subordinate whiteboard
	// agent card whose Record.Parent stays unset; nil defaults to false.
	CardBound func(replyID string) bool

	// bindHost overrides the address the daemon binds, so a test runs LAN
	// mode on loopback; empty derives it from LAN.
	bindHost string
	// openSystem replaces the system-open route's OS open command, so a
	// test never starts a browser; nil runs systemOpen.
	openSystem func(target string) error
	// hostFiles replaces the host the file browser's host route acts on
	// (fs_act_route.go), so a test never opens a window; nil is the host
	// the daemon runs on.
	hostFiles *hostFiles
	// browseRoots supplies local filesystem roots. Nil discovers host roots.
	browseRoots func() browseRoots
	// terminalShell replaces the shell a new terminal runs, so a test's
	// shell does not depend on the machine's; nil picks daemonShell.
	terminalShell func() terminalShell
	// gitRead replaces the git handle's read, so a test never runs git; nil
	// runs readGitBadge. noGitBadge turns the handle off: no read and no
	// handle, for the tests that pin which regions a stream sends.
	gitRead    func(root string) GitBadge
	noGitBadge bool
	// refreshInterval replaces sseRefreshInterval, so a test drives the
	// refresh tick; zero is sseRefreshInterval.
	refreshInterval time.Duration
	// foreignInterval replaces foreignSafetyInterval, so a test sees a
	// no-wake change within a few refresh ticks; zero is foreignSafetyInterval.
	foreignInterval time.Duration
	// keepAlive replaces sseKeepAlive, so a test drives the streams' idle
	// frame and the WebSocket read window; zero is sseKeepAlive.
	keepAlive time.Duration
	// ackDelay replaces ackCheckDelay, so a test drives the answer check;
	// zero is ackCheckDelay.
	ackDelay time.Duration
	// hireNow is a fixture clock for proposal expiry.
	hireNow func() time.Time
	// authNow is a fixture clock for login codes and failed exchanges.
	authNow func() time.Time
	// clipboard replaces the native login-link clipboard writer in tests.
	// Nil selects the platform writer; it never disables authentication.
	clipboard clipboardWriter
}

// MailWaker receives the daemon's mailbox wake signals (DaemonOptions.MailWake).
type MailWaker interface {
	// Wake signals a write made in this process.
	Wake()
	// Nudge signals that another process reported a store change.
	Nudge()
}

// nudgeBodyLimit bounds a /nudge request body.
const nudgeBodyLimit = 4 << 10

// NudgeBody is the /nudge request body. Readers ignore unknown fields and
// treat absent ones as empty.
type NudgeBody struct {
	ReplyID string `json:"reply_id"`
	// Project is the session's root path, as its registration records it.
	Project string `json:"project"`
}

// Daemon is a running dashboard daemon: it holds the lock, listens on its
// port (loopback, or every interface in LAN mode), and has written its
// endpoint file.
type Daemon struct {
	dir        string
	lock       *flock.Flock
	server     *http.Server
	listener   net.Listener
	endpoint   Endpoint
	log        io.Writer
	index      *Index
	source     SessionSource
	config     *ConfigBackend
	agents     AgentHost
	leadNudges interface {
		Start()
		Close()
	}
	broker *broker
	// mailWake is DaemonOptions.MailWake, nil without one.
	mailWake MailWaker
	// stats counts the live-update work (stream_stats.go).
	stats *streamStats
	// cache holds the renders open streams share (stream_cache.go);
	// refreshInterval is the streams' refresh tick, keepAlive their idle
	// frame's period.
	cache           *pageCache
	refreshInterval time.Duration
	keepAlive       time.Duration
	// foreign is the refresh stamps' shared look at inputs no wake reports
	// (foreign_schedule.go).
	foreign *foreignSchedule
	// tracker is the tracker host, nil without one; trackerRev is the
	// revision TrackerChanged bumps and the streams send. trackerData is
	// the data revision the cached page reads are keyed by (trackerAnswers,
	// tracker_answers.go): TrackerChanged bumps it too, a change the daemon
	// applies to those answers itself (the seen marks) does not.
	tracker        TrackerHost
	trackerRev     atomic.Int64
	trackerData    atomic.Int64
	trackerAnswers trackerAnswers
	// entries caches rendered conversation entries per session.
	entries *entryCache
	// entryFiles caches the parsed drained entries per session (ring entries
	// stay in wslive's pinned RAM cache for active hosted sessions).
	entryFiles *entryFiles
	// snapshots caches the parsed live snapshots the pages, their refresh
	// stamps, and agent conversation re-reads read.
	snapshots *SnapshotCache
	// live is the hosted sessions' snapshot slots (live_hosted.go).
	live hostedSlots
	// inboxMu serializes the daemon's own inbox writes (sends and cancels,
	// and the answer check's record and nudge).
	inboxMu       sync.Mutex
	answerReplay  sync.Map        // reply-id -> one active replay/retry loop
	answerPending map[string]bool // inboxMu; absent means not yet checked at startup
	// acks runs the answer checks (ack_check.go), nil without a session
	// index.
	acks *ackChecker
	// lan is LAN mode; tokenMu serializes minting the LAN token and the
	// localhost secret (persistedSecret).
	lan     bool
	tokenMu sync.Mutex
	// auth holds bounded, ephemeral login codes and the exchange rate window.
	auth      localhostAuth
	clipboard clipboardWriter
	memoMu    sync.Mutex      // serializes daemon-local memo read/modify/write
	planUsage *planUsageStore // daemon-private account usage, never credentials
	// pprof serves the pprof routes (pprofHandler).
	pprof bool
	// buildTime is the build time /health reports; zero is unknown.
	// contentHash is the executable hash it reports; empty is unknown.
	buildTime   int64
	contentHash string
	// openSystem opens a system-open route's target (systemOpen).
	openSystem func(target string) error
	// hostFiles is the host the file browser's host route acts on
	// (fs_act_route.go).
	hostFiles   *hostFiles
	browseRoots func() browseRoots
	// fsCapOnce draws fsCapSecret, the key of the HTML preview's
	// capabilities (fs_capability.go); fsNow replaces their clock in a test.
	fsCapOnce   sync.Once
	fsCapSecret []byte
	fsNow       func() time.Time
	// git reads the git handle of the roots open session pages show
	// (gitbadge.go); nil when the handle is off.
	git *gitWatcher
	// gitActing holds the roots a git drawer action runs in (gitpush.go),
	// one at a time per root; gitActMu guards it.
	gitActMu  sync.Mutex
	gitActing map[string]bool
	// worktrees reads worktree pools, nil without the routes; sizes
	// measures their worktrees' disk use (worktrees.go).
	worktrees WorktreeSource
	sizes     *sizeCache
	notes     sync.Map // project root -> *notesView, no background timer
	// rail reads the session rail's worktree lists while a page shows the
	// rail (railworktrees.go); railMsgs holds each session's last message
	// for it (railmessages.go).
	rail       *railLister
	railMsgs   *railMessages
	railInputs railInputCache
	// workspaces holds the stored workspaces, nil without them
	// (workspaces.go); wsAdmission holds rejected unregistered workspaces
	// (workspace_admission.go); pathHost is the machine whose rules read a
	// pasted folder path (workspacepath.go).
	workspaces  *workspaceStore
	wsAdmission *workspaceAdmissionStore
	pathHost    pathHost
	// layout holds the rail's shared row order, tree, and stars
	// (rail_layout.go).
	layout *railLayoutStore
	// hireMu serializes proposal decisions, flag changes, and rail nesting.
	hireMu     sync.Mutex
	hireNow    func() time.Time
	hireAfter  func(time.Duration) (<-chan time.Time, func())
	hireRead   func() ([]HireProposal, error)
	hireInLoop bool // guarded by hireMu
	hireWake   chan struct{}
	hireDone   chan struct{}
	// beforeUnlock is DaemonOptions.BeforeUnlock.
	beforeUnlock func()
	// files caches each session root's file list for the files route
	// (files_route.go).
	files *fileListCache
	// support serves SupportRoutePrefix, nil without the routes.
	support http.Handler
	// holders is the session holder manager, nil without the routes
	// (holders.go).
	holders HolderHost
	// terminals are the shells on holder PTYs the session pages show, nil
	// unless the holder manager can host them (terminals.go); shell picks a
	// new one's shell.
	terminals *terminals
	shell     func() terminalShell
	// proxy builds the hosted MCP sessions (proxy.go), nil without the
	// route; proxyConns are the open ones, proxyWG counts their
	// goroutines, and proxyDraining refuses new ones once Close began.
	proxy         ProxyHandler
	proxyMu       sync.Mutex
	proxyConns    map[*proxyConn]struct{}
	proxyWG       sync.WaitGroup
	proxyDraining bool
	// httpSessions are ProxyHTTPPath's hosted sessions by token
	// (proxy_http.go), each also one of proxyConns; httpReplay numbers
	// their replayed initializes.
	httpMu       sync.Mutex
	httpSessions map[string]*httpMCPSession
	httpReplay   atomic.Uint64
	// web is the web card targets and their proxy listeners
	// (webcard_proxy.go).
	web       *webRegistry
	cardBound func(replyID string) bool

	closeOnce sync.Once
	stop      chan struct{}
	done      chan struct{}
}

// HealthInfo is the /health answer.
type HealthInfo struct {
	Service   string `json:"service"`
	Version   string `json:"version"`
	PID       int    `json:"pid"`
	Port      int    `json:"port"`
	StartedAt string `json:"started_at"`
	// LAN reports LAN mode; an older daemon omits it, which reads as off.
	LAN bool `json:"lan"`
	// BuildTime is the daemon's ws-mcp build time in Unix milliseconds
	// (ExecutableBuildTime); an older daemon omits it, which reads as older
	// than any known build.
	BuildTime int64 `json:"build_time,omitempty"`
	// ContentHash is the sha256 of the daemon's executable. At an equal
	// version it decides whether the build time matters at all: the same
	// content is the same build, whatever the file's mtime (each plugin
	// install is its own copy of one binary). An older daemon omits it.
	ContentHash string `json:"content_hash,omitempty"`
}
