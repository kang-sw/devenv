// daemon_start.go holds daemon startup and listener selection.
package wsdashboard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/gofrs/flock"
	"github.com/kang-sw/devenv/internal/wslive"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

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
	if opts.Executable != "" {
		if err := recordRunningExecutable(opts.Dir, opts.Executable, opts.Version, opts.BuildTime); err != nil {
			_ = listener.Close()
			_ = lock.Unlock()
			return nil, fmt.Errorf("record running executable: %w", err)
		}
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
		cardBound:       opts.CardBound,
		leadNudges:      opts.LeadNudges,
		beforeUnlock:    opts.BeforeUnlock,
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
		browseRoots:     opts.browseRoots,
		worktrees:       opts.Worktrees,
		sizes:           newSizeCache(),
		files:           newFileListCache(),
		support:         opts.Support,
		holders:         opts.Holders,
		hireNow:         opts.hireNow,
		auth:            localhostAuth{now: opts.authNow},
		clipboard:       opts.clipboard,
		hireWake:        make(chan struct{}, 1),
		hireDone:        make(chan struct{}),
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
	if d.clipboard == nil {
		d.clipboard = newNativeClipboardWriter()
	}
	if opts.ScratchCapture {
		if d.browseRoots == nil {
			d.browseRoots = func() browseRoots {
				return browseRoots{Host: "scratch", Home: d.pathHost.Home, Volumes: []string{}}
			}
		}
	} else {
		home, getenv := opts.Home, os.Getenv
		if home == "" {
			home, _ = os.UserHomeDir()
		} else {
			// Explicit embedding/fixture homes must not escape through inherited
			// CLAUDE_CONFIG_DIR or CODEX_HOME overrides.
			getenv = func(string) string { return "" }
		}
		d.planUsage = newPlanUsageStore(opts.Dir, home, getenv)
		d.planUsage.resolveAgy = opts.AgyCommand
	}
	d.cache = newPageCache(d.broker, stats)
	d.foreign = newForeignSchedule(opts.foreignInterval, opts.MailWake)
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
	d.wsAdmission = openWorkspaceAdmissionStore(WorkspaceAdmissionFilePath(opts.WorkspacesFile))
	d.layout = openRailLayoutStore(opts.Dir)
	d.rail.admission = newRailAdmissionTracker(d.layout)
	d.pathHost = daemonPathHost()
	if d.workspaces != nil {
		d.rail.sweep = func() bool {
			c1 := d.workspaces.sweep()
			c2 := d.wsAdmission.sweep()
			return c1 || c2
		}
	}
	if d.lan {
		// Mint the token now, so the localhost index shows it before any
		// LAN request; a failure leaves every LAN request refused.
		if _, err := d.lanToken(); err != nil {
			fmt.Fprintf(logw, "dashboard: LAN access token unavailable: %v\n", err)
		}
	}
	if opts.Sessions != nil {
		d.source = agentSessions{inner: opts.Sessions, dir: opts.Dir, now: time.Now, snaps: d.snapshots, hosted: d.live.hasOwner}
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
		if lh, ok := d.agents.(AgentLiveHost); ok {
			lh.SetLiveHost(d)
		}
		if eh, ok := d.agents.(AgentEndpointHost); ok {
			eh.SetEndpoint(endpointURL(d.endpoint, ""))
		}
		d.agents.Start(d.agentChanged)
	}
	if err := writeEndpoint(opts.Dir, d.endpoint); err != nil {
		// A detached daemon's returned error has no reader; the log does.
		fmt.Fprintf(logw, "dashboard: write endpoint file: %v\n", err)
		if d.planUsage != nil {
			d.planUsage.close(context.Background())
		}
		if d.agents != nil {
			d.agents.Close()
		}
		_ = wslive.UnpinRingsUnder(filepath.Join(d.dir, sessionsDirName))
		_ = listener.Close()
		_ = lock.Unlock()
		return nil, fmt.Errorf("write endpoint file: %w", err)
	}
	if d.leadNudges != nil {
		d.leadNudges.Start()
	}
	go func() {
		if err := d.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(d.log, "dashboard: serve: %v\n", err)
			d.Close()
		}
	}()
	go d.hireLoop()
	if d.index != nil {
		go d.poll(opts.PollInterval)
	} else {
		d.pokeHires()
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
