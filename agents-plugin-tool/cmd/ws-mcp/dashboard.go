package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

// dashboardCommand runs the machine-wide dashboard daemon in the foreground.
// The launcher's `daemon-start` (and a direct `ws-mcp serve`) starts it
// detached through wsdashboard.Ensure; running it by hand is for diagnosis
// and, with WS_MCP_DASHBOARD_DEV_SOURCE set, for editing the page assets
// live.
func dashboardCommand(args []string) {
	fs := flag.NewFlagSet("dashboard", flag.ExitOnError)
	port := fs.Int("port", wsdashboard.DefaultPort, "preferred loopback port; 0 lets the OS choose, and a taken port falls back to an OS-chosen one")
	lan := fs.Bool("lan", false, "listen on every network interface and admit a request from another device only with the LAN access token; the starter (daemon-start, or ws-mcp serve) passes it when the global dashboard.lan_enabled value is on")
	buildTime := fs.Int64("build-time", 0, "the ws-mcp build time in Unix milliseconds to report in /health; the starter passes the time of the executable it measured, and 0 reads this executable's")
	lockWait := fs.Duration("lock-wait", wsdashboard.DefaultLockWait, "how long to wait for another daemon to release the single-instance lock")
	_ = fs.Parse(args)

	dir, err := wsdashboard.Dir("")
	if err != nil {
		fatal("dashboard", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fatal("dashboard", err)
	}
	exe, _ := os.Executable()
	if *buildTime <= 0 && exe != "" {
		*buildTime = wsdashboard.ExecutableBuildTime(exe)
	}
	// The content hash is this executable's own: the staged copy a launcher
	// made has the plugin binary's content, so the launcher's and the
	// starter's hashes of their binaries compare against it.
	contentHash := ""
	if exe != "" {
		contentHash = wsdashboard.ExecutableContentHash(exe)
	}
	configPath, _ := wsconfig.GlobalPath(wsconfig.Options{})
	logw := openDashboardLog(dir)
	// Dev assets switch on before the daemon serves its first request. The
	// notice waits for StartDaemon: the lazy log truncates on first write, and
	// only the lock holder may do that.
	devRoot, err := wsdashboard.EnableDevAssets(os.Getenv(wsdashboard.DevSourceEnv))
	if err != nil {
		fatal("dashboard", err)
	}
	// One tracker host per daemon, shared by the poll and the tracker
	// routes; a host that cannot be created still goes in, so the page
	// shows why.
	tracker, trackerErr := wstrackerhost.New(wstrackerhost.Options{})
	closeTracker := func() {
		if tracker != nil {
			_ = tracker.Close()
		}
	}
	sessions := mcp.NewDashboardSessionSource()
	// StartDaemon starts the runner before it returns the daemon, so the
	// runner's tracker-changed callback is bound once the daemon exists.
	var trackerChanged lateFunc
	agents := newAgentRunner(dir, logw, trackerChanged.call)
	host := &proxyHost{dir: dir, port: *port, lan: *lan, buildTime: *buildTime, contentHash: contentHash, logw: logw, configPath: filepath.Clean(configPath)}
	// A store's support conversation runs in the store's clone, so an
	// unregister ends it: right after the dashboard's own, and on the store
	// watch's change for one another process ran (tk:unregister).
	support := wsagent.NewSupport(wsagent.SupportOptions{
		Runner:    agents,
		Dir:       dir,
		StoreRoot: trackerStoreRoot(tracker, trackerErr),
	})
	daemon, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{
		Dir:         dir,
		Port:        *port,
		LAN:         *lan,
		Version:     version,
		BuildTime:   *buildTime,
		ContentHash: contentHash,
		LockWait:    *lockWait,
		Log:         logw,
		Proxy:       host,
		Pprof:       wsdashboard.PprofEnabled(os.Getenv(wsdashboard.PprofEnv)),
		// User-curated, so beside the config file rather than in the cache.
		WorkspacesFile: wsdashboard.WorkspacesFilePath(configPath),
		Sessions:       sessions,
		Worktrees:      sessions,
		Agents:         agents,
		Tracker:        &dashboardTracker{h: tracker, err: trackerErr, unregistered: func() { support.CloseUnregistered() }},
		// The holders a predecessor left running are found in the state
		// directory; the interpreter for a new one is the launcher's own.
		Holders: wsholder.New(wsholder.Options{
			Dir:    filepath.Join(dir, wsholder.DirName),
			Python: os.Getenv(wsholder.PythonEnv),
			Log:    logw,
		}),
		Support: support,
	})
	if errors.Is(err, wsdashboard.ErrLocked) {
		closeTracker()
		fmt.Fprintln(os.Stderr, "ws-mcp dashboard: another dashboard daemon is running (it holds the lock); exiting")
		os.Exit(3)
	}
	if err != nil {
		closeTracker()
		fatal("dashboard", err)
	}
	wsdashboard.LogDevAssets(logw, devRoot)
	// Holding the lock, this daemon is the one the staged copies were for;
	// every copy but its own is unused now.
	if exe != "" {
		wsdashboard.PruneStagedExecutables(dir, exe)
	}
	trackerChanged.set(daemon.TrackerChanged)
	stopPoll := startTrackerPoll(tracker, trackerErr, daemon.TrackerChanged, logw)
	stopWatch := startTrackerWatch(tracker, trackerErr, func() {
		daemon.TrackerChanged()
		support.CloseUnregistered()
	})
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case <-signals:
		daemon.Close()
	case <-daemon.Done():
	}
	// The lock and endpoint are released (the successor is starting); a
	// hosted call that outlived the drain budget still runs and ends on its
	// own, and the replacement goroutine, when one runs, spawns the
	// successor. Both finish before this process exits.
	linger, cancel := context.WithTimeout(context.Background(), hostedCallLinger)
	if !daemon.WaitProxy(linger) {
		fmt.Fprintf(logw, "dashboard: a hosted call still runs after %s; exiting\n", hostedCallLinger)
	}
	cancel()
	host.wait()
	stopWatch()
	stopPoll()
	closeTracker()
}

// lateFunc is a callback bound after the code that calls it is built; a
// call before set does nothing. Safe from any goroutine.
type lateFunc struct{ f atomic.Pointer[func()] }

func (l *lateFunc) set(f func()) { l.f.Store(&f) }

func (l *lateFunc) call() {
	if f := l.f.Load(); f != nil {
		(*f)()
	}
}

// startTrackerWatch runs the tracker's store watch in the daemon on h, so a
// write by another process (a lead session's or a support agent's ws-mcp)
// calls onChange within wstrackerhost.DefaultWatchInterval; onChange is
// also what a page read that served the last state calls when the store's
// reload lands. newErr is h's creation error: the daemon runs without the
// watch. The caller closes h.
func startTrackerWatch(h *wstrackerhost.Host, newErr error, onChange func()) (stop func()) {
	if newErr != nil {
		return func() {}
	}
	return h.StartWatch(wstrackerhost.DefaultWatchInterval, onChange)
}

// startTrackerPoll runs the tracker's background poll in the daemon on h:
// every wstrackerhost.DefaultPollInterval, a fetch of every registered
// store, whatever workflow.tracker_enabled says (the knob gates only manual
// text), and onRound after the first round and each later one that changed
// something (wstrackerhost.Host.PollOnce). While it runs, a ws-mcp's pre-read
// check leaves the network to it. newErr is h's creation error: it is
// logged once, and the daemon runs without the poll. The caller closes h.
func startTrackerPoll(h *wstrackerhost.Host, newErr error, onRound func(), logw io.Writer) (stop func()) {
	if newErr != nil {
		fmt.Fprintf(logw, "ws-mcp dashboard: tracker poll not started: %v\n", wstrackerhost.RedactCredentials(newErr.Error()))
		return func() {}
	}
	return h.StartPoll(wstrackerhost.DefaultPollInterval, onRound)
}

// trackerStoreRoot returns a tracker store's working root for the support
// conversations: the registry entry's directory on the daemon's tracker
// host h, the same root the tracker gate pins a support conversation's
// writes to. newErr is h's creation error; every store is unknown then.
func trackerStoreRoot(h *wstrackerhost.Host, newErr error) func(id string) (string, error) {
	return func(id string) (string, error) {
		if newErr != nil {
			return "", redacted(newErr)
		}
		e, err := h.Entry(id)
		if errors.Is(err, wstrackerhost.ErrNotRegistered) {
			return "", fmt.Errorf("%w: %v", wsagent.ErrStoreUnregistered, redacted(err))
		}
		if err != nil {
			return "", redacted(err)
		}
		return e.Dir, nil
	}
}

// newAgentRunner is the daemon's agent runner: the claude and pi backends,
// ws-mcp's key store for conversation keys, and this executable as the
// ws-mcp it attaches to a conversation whose profile allows ws tools.
// trackerChanged runs after a conversation's successful tracker write.
func newAgentRunner(dir string, logw io.Writer, trackerChanged func()) *wsagent.Runner {
	return wsagent.NewRunner(wsagent.Options{
		Dir:      dir,
		Backends: map[string]wsagent.Backend{wsagent.BackendClaude: wsagent.ClaudeBackend{}, wsagent.BackendPi: wsagent.PiBackend{}},
		Keys:     mcp.NewAgentKeyStore(),
		WSMCP: func(root string) (wsagent.MCPCommand, error) {
			exe, err := os.Executable()
			if err != nil {
				return wsagent.MCPCommand{}, err
			}
			return wsagent.MCPCommand{Command: exe, Args: []string{"serve", "--stdio", "--root", root}}, nil
		},
		Log:            logw,
		TrackerChanged: trackerChanged,
	})
}

// openDashboardLog returns the daemon's log writer. The file is truncated and
// opened on the first write, and StartDaemon writes only once it holds the
// lock, so a daemon that loses the lock never truncates the winner's log.
func openDashboardLog(dir string) io.Writer {
	return &lazyLog{path: filepath.Join(dir, "dashboard.log")}
}

type lazyLog struct {
	mu   sync.Mutex
	path string
	file *os.File
	err  error
}

func (l *lazyLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil && l.err == nil {
		l.file, l.err = os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	}
	if l.err != nil {
		return len(p), nil
	}
	return l.file.Write(p)
}

// dashboardSpawner starts a detached daemon process with the given ws-mcp
// arguments.
type dashboardSpawner func(args []string) error

// startDashboard is the serve-startup hook: with the dashboard key on, it
// makes sure a daemon at least as new as this ws-mcp runs, in the LAN mode
// the global dashboard.lan_enabled value asks for (a same-version daemon in the
// other mode, or from an older build time, is replaced). It never fails or
// delays serve: the caller runs it on its own goroutine, and every error is
// a stderr line.
func startDashboard(ctx context.Context, settings mcp.DashboardSettings, dir string, buildTime int64, contentHash string, spawn dashboardSpawner, logw io.Writer) wsdashboard.Outcome {
	if !settings.Enabled {
		return ""
	}
	outcome, err := wsdashboard.Ensure(ctx, wsdashboard.EnsureOptions{
		Dir:         dir,
		Version:     version,
		BuildTime:   buildTime,
		ContentHash: contentHash,
		Port:        settings.Port,
		LAN:         settings.LAN,
		Spawn:       spawn,
		Log:         logw,
	})
	if err != nil {
		fmt.Fprintf(logw, "ws-mcp: dashboard unavailable: %v\n", err)
	} else if outcome == wsdashboard.OutcomeUnavailable {
		fmt.Fprintln(logw, "ws-mcp: dashboard unavailable: no daemon answered after starting one (another may hold the lock without answering)")
	}
	return outcome
}

// serveDashboard runs startDashboard for a stdio serve with the real daemon
// directory and a detached spawner of this binary.
func serveDashboard(server *mcp.Server) {
	defer func() { _ = recover() }()
	settings := server.DashboardSettings()
	if !settings.Enabled {
		return
	}
	dir, err := wsdashboard.Dir("")
	if err == nil {
		err = os.MkdirAll(dir, 0o755)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ws-mcp: dashboard unavailable: %v\n", err)
		return
	}
	// The build time comes from the original executable, before
	// DaemonExecutable may stage a copy whose mtime is the copy time; the
	// content hash is the same for both.
	var buildTime int64
	var contentHash string
	if exe, err := os.Executable(); err == nil {
		buildTime = wsdashboard.ExecutableBuildTime(exe)
		contentHash = wsdashboard.ExecutableContentHash(exe)
	}
	spawn := func(args []string) error {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		exe, err = wsdashboard.DaemonExecutable(exe, dir)
		if err != nil {
			return err
		}
		return wsdashboard.SpawnDetached(exe, args, dir)
	}
	startDashboard(context.Background(), settings, dir, buildTime, contentHash, spawn, os.Stderr)
}
