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
	"syscall"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// dashboardCommand runs the machine-wide dashboard daemon in the foreground.
// ws-mcp serve starts it detached; running it by hand is for diagnosis.
func dashboardCommand(args []string) {
	fs := flag.NewFlagSet("dashboard", flag.ExitOnError)
	port := fs.Int("port", wsdashboard.DefaultPort, "preferred loopback port; 0 lets the OS choose, and a taken port falls back to an OS-chosen one")
	lan := fs.Bool("lan", false, "listen on every network interface and admit a request from another device only with the LAN access token; ws-mcp serve passes it when the global dashboard.lan_enabled value is on")
	buildTime := fs.Int64("build-time", 0, "the ws-mcp build time in Unix milliseconds to report in /health; ws-mcp serve passes its own executable's, and 0 reads this executable's")
	lockWait := fs.Duration("lock-wait", wsdashboard.DefaultLockWait, "how long to wait for another daemon to release the single-instance lock")
	_ = fs.Parse(args)

	dir, err := wsdashboard.Dir("")
	if err != nil {
		fatal("dashboard", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fatal("dashboard", err)
	}
	if *buildTime <= 0 {
		if exe, err := os.Executable(); err == nil {
			*buildTime = wsdashboard.ExecutableBuildTime(exe)
		}
	}
	logw := openDashboardLog(dir)
	daemon, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{
		Dir:       dir,
		Port:      *port,
		LAN:       *lan,
		Version:   version,
		BuildTime: *buildTime,
		LockWait:  *lockWait,
		Log:       logw,
		Sessions:  mcp.NewDashboardSessionSource(),
		Agents:    newAgentRunner(dir, logw),
	})
	if errors.Is(err, wsdashboard.ErrLocked) {
		fmt.Fprintln(os.Stderr, "ws-mcp dashboard: another dashboard daemon is running (it holds the lock); exiting")
		os.Exit(3)
	}
	if err != nil {
		fatal("dashboard", err)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case <-signals:
		daemon.Close()
	case <-daemon.Done():
	}
}

// newAgentRunner is the daemon's agent runner: the claude and pi backends,
// ws-mcp's key store for conversation keys, and this executable as the
// ws-mcp it attaches to a conversation whose profile allows ws tools.
func newAgentRunner(dir string, logw io.Writer) *wsagent.Runner {
	return wsagent.NewRunner(wsagent.Options{
		Dir:      dir,
		Backends: map[string]wsagent.Backend{wsagent.BackendClaude: wsagent.ClaudeBackend{}},
		Keys:     mcp.NewAgentKeyStore(),
		WSMCP: func(root string) (wsagent.MCPCommand, error) {
			exe, err := os.Executable()
			if err != nil {
				return wsagent.MCPCommand{}, err
			}
			return wsagent.MCPCommand{Command: exe, Args: []string{"serve", "--stdio", "--root", root}}, nil
		},
		Log: logw,
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
func startDashboard(ctx context.Context, settings mcp.DashboardSettings, dir string, buildTime int64, spawn dashboardSpawner, logw io.Writer) wsdashboard.Outcome {
	if !settings.Enabled {
		return ""
	}
	outcome, err := wsdashboard.Ensure(ctx, wsdashboard.EnsureOptions{
		Dir:       dir,
		Version:   version,
		BuildTime: buildTime,
		Port:      settings.Port,
		LAN:       settings.LAN,
		Spawn:     spawn,
		Log:       logw,
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
	// DaemonExecutable may stage a copy whose mtime is the copy time.
	var buildTime int64
	if exe, err := os.Executable(); err == nil {
		buildTime = wsdashboard.ExecutableBuildTime(exe)
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
	startDashboard(context.Background(), settings, dir, buildTime, spawn, os.Stderr)
}
