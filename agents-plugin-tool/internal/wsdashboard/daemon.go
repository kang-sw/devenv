package wsdashboard

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofrs/flock"
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
	// PollInterval is the polling backstop period; zero means
	// DefaultPollInterval.
	PollInterval time.Duration
	// Agents is the agent runner the daemon hosts; nil serves no agent
	// routes.
	Agents AgentHost
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
	// keepAlive replaces sseKeepAlive, so a test drives the streams' idle
	// frame and the WebSocket read window; zero is sseKeepAlive.
	keepAlive time.Duration
	// ackDelay replaces ackCheckDelay, so a test drives the answer check;
	// zero is ackCheckDelay.
	ackDelay time.Duration
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
	dir      string
	lock     *flock.Flock
	server   *http.Server
	listener net.Listener
	endpoint Endpoint
	log      io.Writer
	index    *Index
	source   SessionSource
	agents   AgentHost
	broker   *broker
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
	// tracker is the tracker host, nil without one; trackerRev is the
	// revision TrackerChanged bumps and the streams send.
	tracker    TrackerHost
	trackerRev atomic.Int64
	// entries caches rendered conversation entries per session.
	entries *entryCache
	// entryFiles caches the parsed ring and drained entries per session.
	entryFiles *entryFiles
	// snapshots caches the parsed live snapshots the pages, their refresh
	// stamps, and agent conversation re-reads read.
	snapshots *SnapshotCache
	// live is the hosted sessions' snapshot slots (live_hosted.go).
	live hostedSlots
	// inboxMu serializes the daemon's own inbox writes (sends and cancels,
	// and the answer check's record and nudge).
	inboxMu sync.Mutex
	// acks runs the answer checks (ack_check.go), nil without a session
	// index.
	acks *ackChecker
	// lan is LAN mode; tokenMu serializes reading and minting the LAN token.
	lan       bool
	tokenMu   sync.Mutex
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
	hostFiles *hostFiles
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
	// rail reads the session rail's worktree lists while a page shows the
	// rail (railworktrees.go); railMsgs holds each session's last message
	// for it (railmessages.go).
	rail     *railLister
	railMsgs *railMessages
	// workspaces holds the stored workspaces, nil without them
	// (workspaces.go); pathHost is the machine whose rules read a pasted
	// folder path (workspacepath.go).
	workspaces *workspaceStore
	pathHost   pathHost
	// layout holds the rail's shared row order, tree, and stars
	// (rail_layout.go).
	layout *railLayoutStore
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
	// web is the web card targets and their proxy listeners
	// (webcard_proxy.go).
	web *webRegistry

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

// StartDaemon takes the single-instance lock, binds the port, writes the
// endpoint file, and starts serving. It returns ErrLocked when another
// daemon still holds the lock after the lock wait.
func StartDaemon(opts DaemonOptions) (*Daemon, error) {
	if opts.Dir == "" {
		return nil, errors.New("dashboard: empty state directory")
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, err
	}
	wait := opts.LockWait
	if wait <= 0 {
		wait = DefaultLockWait
	}
	lock := flock.New(filepath.Join(opts.Dir, lockFileName))
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	locked, err := lock.TryLockContext(ctx, lockRetryDelay)
	cancel()
	if !locked {
		_ = lock.Close()
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("dashboard lock: %w", err)
		}
		return nil, ErrLocked
	}

	listener, err := listen(bindHostFor(opts), portCandidates(opts.Dir, opts.Port))
	if err != nil {
		_ = lock.Unlock()
		return nil, err
	}
	logw := opts.Log
	if logw == nil {
		logw = io.Discard
	}
	// Recorded under the lock; losing it only costs the next restart its
	// sticky port.
	if err := writeLastPort(opts.Dir, listener.Addr().(*net.TCPAddr).Port); err != nil {
		fmt.Fprintf(logw, "dashboard: record last port: %v\n", err)
	}
	token, err := newToken()
	if err != nil {
		_ = listener.Close()
		_ = lock.Unlock()
		return nil, err
	}
	if opts.Pprof {
		enableContentionProfiles()
	}
	stats := &streamStats{}
	d := &Daemon{
		dir:             opts.Dir,
		lock:            lock,
		listener:        listener,
		log:             logw,
		agents:          opts.Agents,
		mailWake:        opts.MailWake,
		tracker:         opts.Tracker,
		broker:          newBroker(stats),
		stats:           stats,
		refreshInterval: opts.refreshInterval,
		keepAlive:       opts.keepAlive,
		entries:         newEntryCache(),
		entryFiles:      newEntryFiles(),
		snapshots:       &SnapshotCache{},
		lan:             opts.LAN,
		pprof:           opts.Pprof,
		buildTime:       opts.BuildTime,
		contentHash:     opts.ContentHash,
		proxy:           opts.Proxy,
		openSystem:      opts.openSystem,
		hostFiles:       opts.hostFiles,
		worktrees:       opts.Worktrees,
		sizes:           newSizeCache(),
		files:           newFileListCache(),
		support:         opts.Support,
		holders:         opts.Holders,
		stop:            make(chan struct{}),
		done:            make(chan struct{}),
		endpoint: Endpoint{
			SchemaVersion: endpointSchemaVersion,
			Port:          listener.Addr().(*net.TCPAddr).Port,
			PID:           os.Getpid(),
			Version:       opts.Version,
			Token:         token,
			StartedAt:     time.Now().UTC().Format(time.RFC3339),
		},
	}
	home, _ := os.UserHomeDir()
	d.planUsage = newPlanUsageStore(opts.Dir, home, os.Getenv)
	d.cache = newPageCache(d.broker, stats)
	d.web = newWebRegistry(d.endpoint.Port, d.endpoint.Token, logw)
	if d.refreshInterval <= 0 {
		d.refreshInterval = sseRefreshInterval
	}
	if d.keepAlive <= 0 {
		d.keepAlive = sseKeepAlive
	}
	if d.openSystem == nil {
		d.openSystem = systemOpen
	}
	if d.hostFiles == nil {
		d.hostFiles = defaultHostFiles()
	}
	if host, ok := opts.Holders.(TerminalHost); ok {
		// The terminals take the holders' events before the first scan
		// (Run, below), so they see a predecessor's terminals attach.
		d.terminals = newTerminals(host, logw)
		host.AddSink(d.terminals)
		if d.shell = opts.terminalShell; d.shell == nil {
			d.shell = daemonShell
		}
	}
	if !opts.noGitBadge {
		read := opts.gitRead
		if read == nil {
			read = readGitBadge
		}
		d.git = newGitWatcher(read, func(root string) {
			d.stats.WakeGit.Add(1)
			d.broker.wake(gitDep(root))
		}, d.stop)
	}
	var railRead func(string, []Registration) (RepoWorktrees, error)
	if src, ok := opts.Worktrees.(RepoWorktreeSource); ok {
		railRead = src.ReadRepoWorktrees
	}
	d.railMsgs = newRailMessages()
	d.rail = newRailLister(railRead, func() { d.broker.wake(railListRepoKey) }, d.stop)
	d.workspaces = openWorkspaceStore(opts.WorkspacesFile)
	d.layout = openRailLayoutStore(opts.Dir)
	d.pathHost = daemonPathHost()
	if d.workspaces != nil {
		d.rail.sweep = d.workspaces.sweep
	}
	if d.lan {
		// Mint the token now, so the localhost index shows it before any
		// LAN request; a failure leaves every LAN request refused.
		if _, err := d.lanToken(); err != nil {
			fmt.Fprintf(logw, "dashboard: LAN access token unavailable: %v\n", err)
		}
	}
	if opts.Sessions != nil {
		d.source = agentSessions{inner: opts.Sessions, dir: opts.Dir, now: time.Now, snaps: d.snapshots}
		delay := opts.ackDelay
		if delay <= 0 {
			delay = ackCheckDelay
		}
		d.acks = &ackChecker{delay: delay, run: d.checkAnswers}
		d.index = NewIndex(IndexOptions{Dir: opts.Dir, Source: d.source, Log: logw, OnChange: d.sessionChanged})
	}
	d.server = &http.Server{Handler: d.Handler(), ReadHeaderTimeout: 10 * time.Second}
	// The runner takes its change hook before the first request can reach
	// it.
	if d.agents != nil {
		if eh, ok := d.agents.(AgentEndpointHost); ok {
			eh.SetEndpoint(endpointURL(d.endpoint, ""))
		}
		d.agents.Start(d.agentChanged)
	}
	if err := writeEndpoint(opts.Dir, d.endpoint); err != nil {
		if d.agents != nil {
			d.agents.Close()
		}
		_ = listener.Close()
		_ = lock.Unlock()
		return nil, fmt.Errorf("write endpoint file: %w", err)
	}
	go func() {
		if err := d.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(d.log, "dashboard: serve: %v\n", err)
			d.Close()
		}
	}()
	if d.index != nil {
		go d.poll(opts.PollInterval)
	}
	go d.web.run(webSweepEvery)
	// The holder scans start only now, under the single-instance lock: a
	// predecessor has let go of its holders before this daemon attaches.
	if d.holders != nil {
		go func() {
			if d.terminals != nil {
				d.terminals.firstScan()
			}
			d.holders.Run()
		}()
	}
	fmt.Fprintf(d.log, "dashboard: version %s listening on %s (lan %v, pid %d)\n", d.endpoint.Version, listener.Addr(), d.lan, d.endpoint.PID)
	return d, nil
}

// bindHostFor is the address a daemon binds: loopback, or every interface
// (the empty host) in LAN mode.
func bindHostFor(opts DaemonOptions) string {
	switch {
	case opts.bindHost != "":
		return opts.bindHost
	case opts.LAN:
		return ""
	}
	return "127.0.0.1"
}

// portCandidates is the order a daemon tries ports: the preferred port, then
// the recorded last port when it differs, each only when valid. An
// OS-assigned port (0) is always the final fallback and is left to listen. A
// request for an OS-assigned port (preferred 0) skips the record, so it stays
// deterministic.
func portCandidates(dir string, preferred int) []int {
	if preferred <= 0 {
		return nil
	}
	ports := []int{preferred}
	if last, ok := readLastPort(dir); ok && last != preferred {
		ports = append(ports, last)
	}
	return ports
}

// listen binds the first of ports that is free on host, falling back to an
// OS-assigned port when none is.
func listen(host string, ports []int) (net.Listener, error) {
	for _, port := range ports {
		if l, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port))); err == nil {
			return l, nil
		}
	}
	return net.Listen("tcp", net.JoinHostPort(host, "0"))
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// sessionChanged is the index's change hook: it wakes the streams whose
// page shows the session, and drops a session's cached conversation entries,
// entry files, and snapshot, and the source's caches of it
// (SessionForgetter), once it left the index.
func (d *Daemon) sessionChanged(replyID string) {
	if d.index != nil {
		if session, ok := d.index.Session(replyID); ok {
			if session.Harness == "claude" {
				d.planUsage.pushClaude(d.snapshots.Snapshot(d.dir, replyID))
			}
		} else {
			d.entries.drop(replyID)
			d.entryFiles.drop(replyID)
			d.railMsgs.drop(replyID)
			d.live.drop(replyID)
			d.snapshots.Forget(d.dir, replyID)
			if f, ok := d.source.(SessionForgetter); ok {
				f.ForgetSession(replyID)
			}
		}
	}
	d.stats.WakeSession.Add(1)
	d.broker.wake(replyID)
	d.acks.poke(replyID)
}

// agentChanged is the agent runner's write hook, called from the
// conversation's loop for every write: it schedules a re-read of the one
// conversation, as a ws-mcp's nudge does for a lead session, and returns
// without waiting for it (Index.Reread). The page wakes at once, since its
// stream reads the conversation's own live files; the re-read's change, if
// any, wakes it again when it lands.
func (d *Daemon) agentChanged(replyID, _ string) {
	if d.index != nil {
		d.index.Reread(replyID)
	}
	d.stats.WakeAgent.Add(1)
	d.broker.wake(replyID)
	d.acks.poke(replyID)
}

// poll fills the session index at once and then re-reads every registered
// session each interval until the daemon stops.
func (d *Daemon) poll(interval time.Duration) {
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	d.index.Refresh()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			d.index.Refresh()
		}
	}
}

// Index returns the daemon's session index; nil when it runs without a
// session source.
func (d *Daemon) Index() *Index { return d.index }

// Endpoint returns what the daemon wrote to its endpoint file.
func (d *Daemon) Endpoint() Endpoint { return d.endpoint }

// Done is closed once the daemon has stopped serving, removed its endpoint
// file, and released the lock.
func (d *Daemon) Done() <-chan struct{} { return d.done }

// Close stops the daemon. The endpoint file is removed before the lock is
// released, so a successor that takes the lock never has its own file
// deleted. Close is idempotent. A hosted call that outlives the drain
// budget is still running when Close returns (drainProxy); WaitProxy waits
// for it.
func (d *Daemon) Close() {
	d.closeOnce.Do(func() {
		close(d.stop)
		d.acks.close()
		// Shutdown closes the listener at once, so no new session or page
		// connects, and the hosted sessions drain meanwhile (proxy.go):
		// the whole stop stays under the successor's lock wait.
		ctx, cancel := context.WithTimeout(context.Background(), shutdownDeadline)
		shutdownDone := make(chan struct{})
		go func() {
			_ = d.server.Shutdown(ctx)
			close(shutdownDone)
		}()
		d.drainProxy()
		<-shutdownDone
		cancel()
		// Shutdown closes only the listeners Serve has registered, and Serve
		// runs on its own goroutine: a Close before that goroutine is
		// scheduled would leave the port bound past Close, so a restart could
		// not rebind it. Closing it here makes Close release the port.
		_ = d.listener.Close()
		// The web card listeners are servers of their own; closed here,
		// before the lock goes, a successor never races a leftover port.
		d.web.close()
		if d.agents != nil {
			d.agents.Close()
		}
		// Detaches only: the holders and their children outlive the daemon.
		if d.holders != nil {
			d.holders.Close()
		}
		removeEndpointIfOwned(d.dir, d.endpoint.Token)
		_ = d.lock.Unlock()
		fmt.Fprintf(d.log, "dashboard: stopped (pid %d)\n", d.endpoint.PID)
		close(d.done)
	})
}

// Handler is the daemon's HTTP surface. Every route first passes the access
// guard (the loopback Host guard, or in LAN mode the guard that also admits
// the LAN token; access.go); the daemon sends no CORS headers. The pages (/,
// /s/<reply-id>, served as the iframe shell or, with ?frame=1, as the page
// it frames; their SSE or WebSocket streams, the shell's rail stream
// /rail/events, and a session's image, file view, and
// history routes, and the app manifest and its icons, pwa.go) are read-only; a session's message, cancel, steer, and interrupt
// routes (inbox_routes.go), with a tracker host the tracker POST routes
// (tracker_routes.go), and with a support handler the support POST routes
// (a tracker store's support conversation: send, new, select) are the only
// browser writes besides the git drawer's push, pull, and fetch routes
// (gitpush.go) and, with stored workspaces, the rail's workspace routes
// (workspace_routes.go: the stored list, and git worktree add), its layout
// route (rail_layout_route.go: the rows' shared order, tree, and stars), and
// its system-open route (sysopen_route.go) the only route
// that runs a host command beside git and the agent runner's harnesses (with
// a holder manager, the holder stop route, holders.go, ends a holder, and
// the terminal routes, terminal_routes.go, run shells for a browser on this
// machine): a
// session stream keeps its root's git handle read, the git route
// (gitbadge.go) asks for a read at once without running git itself, and the
// git status route (gitdrawer.go) reads the root's changed files. With
// Pprof on, /debug/pprof/ (pprof_route.go) is served behind the guard to
// local viewers only.
func (d *Daemon) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", d.handleHealth)
	mux.HandleFunc("/shutdown", d.handleShutdown)
	if d.proxy != nil {
		mux.HandleFunc(ProxyPath, d.handleProxy)
	}
	mux.HandleFunc("/nudge", d.handleNudge)
	mux.HandleFunc("/events", d.handleIndexEvents)
	mux.HandleFunc(pwaPrefix, handlePWA)
	mux.HandleFunc("/s/", d.handleSessionRoutes)
	mux.HandleFunc(railRoute, d.handleRailRefresh)
	mux.HandleFunc(railLayoutRoute, d.handleRailLayout)
	mux.HandleFunc(railEventsRoute, d.handleRailEvents)
	mux.HandleFunc("/rail/memos", d.handleMemos)
	mux.HandleFunc("/rail/usage", d.handlePlanUsage)
	if d.workspaces != nil {
		d.workspaceRoutes(mux)
	}
	if d.tracker != nil {
		mux.HandleFunc("/tracker/", d.handleTracker)
	}
	if d.agents != nil {
		agents := d.agents.Handler()
		mux.Handle(AgentRoutePrefix, agents)
		mux.Handle(AgentRoutePrefix+"/", agents)
	}
	if d.support != nil {
		mux.Handle(SupportRoutePrefix, d.support)
		mux.Handle(SupportRoutePrefix+"/", d.support)
	}
	if d.holders != nil {
		mux.HandleFunc("/holders", d.handleHolders)
		mux.HandleFunc("/holders/", d.handleHolders)
	}
	if d.pprof {
		mux.Handle(pprofPrefix, pprofHandler(d.serveStreams))
	}
	mux.HandleFunc("/", d.handleIndex)
	if d.lan {
		return d.lanGuard(mux)
	}
	return hostGuard(mux)
}

// hostGuard refuses a request whose Host header is not a loopback name. A
// DNS-rebinding page reaches 127.0.0.1 under its own host name, which this
// refuses.
func hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loopbackHost reports whether a Host header names the loopback interface:
// localhost, 127.0.0.1, or ::1, with or without a port.
func loopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"))
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func (d *Daemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, HealthInfo{
		Service:     healthService,
		Version:     d.endpoint.Version,
		PID:         d.endpoint.PID,
		Port:        d.endpoint.Port,
		StartedAt:   d.endpoint.StartedAt,
		LAN:         d.lan,
		BuildTime:   d.buildTime,
		ContentHash: d.contentHash,
	})
}

// handleShutdown stops the daemon when the request carries the endpoint
// file's token as a bearer credential. Only a process that can read the
// endpoint file (the same OS user) holds that token.
func (d *Daemon) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	got, bearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !bearer || got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(d.endpoint.Token)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"shutting_down": true})
	fmt.Fprintf(d.log, "dashboard: shutdown requested\n")
	go d.Close()
}

// handleNudge schedules a re-read of the one session the body names
// (Index.Nudge) and answers without waiting for it. It needs no credential:
// all it can cause is a re-read of disk. A malformed body is 400, an unknown
// session 404, and a scheduled re-read 204; the sender ignores the answer.
func (d *Daemon) handleNudge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d.stats.Nudges.Add(1)
	var body NudgeBody
	if err := json.NewDecoder(io.LimitReader(r.Body, nudgeBodyLimit)).Decode(&body); err != nil {
		http.Error(w, "malformed nudge", http.StatusBadRequest)
		return
	}
	// Before the registration check: a mail sender need not be a
	// registered session, and each woken wait rechecks its own target.
	if d.mailWake != nil {
		d.mailWake.Nudge()
	}
	if d.index == nil || !d.index.Nudge(body.ReplyID, body.Project) {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
