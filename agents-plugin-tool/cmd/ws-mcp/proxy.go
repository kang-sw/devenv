package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

// hostedCallLinger bounds how long the daemon process stays, after it has
// released its lock and endpoint, for a hosted call that outlived the
// drain budget (wsdashboard.Daemon.WaitProxy): long enough for a git
// merge or a ticket close, short enough that a wedged call never leaves a
// daemon process behind for good.
const hostedCallLinger = 2 * time.Minute

// proxy.go hosts the MCP sessions the dashboard daemon serves for the
// launchers that connect to its MCP route (internal/wsdashboard/proxy.go).
// Each accepted hello becomes one mcp.Server built from the launcher's
// process context — its environment, project root, and PID — never from
// the daemon's own, so two sessions on one daemon are as separate as two
// ws-mcp processes were. A CLI-mode hello runs the shared CLI core in the
// daemon and answers with its output and exit code.

// proxyHost is the daemon's ProxyHandler.
type proxyHost struct {
	dir         string
	port        int
	lan         bool
	buildTime   int64
	contentHash string
	logw        io.Writer
	// configPath is the global config file this daemon's own environment
	// resolves to; a session anchored at another one (its WS_CONFIG_HOME)
	// reads a dashboard.lan_enabled that is not this daemon's to follow.
	configPath string
	// replacing is set once a connection's global dashboard.lan_enabled
	// differs from this daemon's mode and the replacement began; background
	// counts the replacement goroutine, which the daemon process waits for
	// before it exits (it spawns the successor).
	replacing  atomic.Bool
	background sync.WaitGroup
	// live is the daemon, bound once StartDaemon returned (the host is built
	// before it): each hosted server hands its live snapshots to it in
	// memory (mcp.ServerOptions.Live). A connection accepted before the
	// binding writes its snapshot files as a ws-mcp of its own would.
	live atomic.Pointer[wsdashboard.Daemon]
	// agents is the daemon's agent runner, where a hosted session's pi.*
	// tools spawn its Pi subagents (mcp.ServerOptions.Agents); nil leaves
	// them refusing.
	agents mcp.PiAgents
}

// wait blocks until the replacement goroutine, if any, has ended.
func (h *proxyHost) wait() { h.background.Wait() }

// Accept builds the session for hello: an MCP server or a one-shot CLI run.
func (h *proxyHost) Accept(hello wsdashboard.ProxyHello) (wsdashboard.ProxySession, error) {
	hello = callerHello(hello)
	env := mcp.EnvFromMap(hello.Env)
	switch hello.Mode {
	case wsdashboard.ProxyModeMCP:
		if strings.TrimSpace(hello.Env[wsrsrc.EnvRsrcRoot]) == "" {
			return nil, fmt.Errorf("hello carries no %s: a hosted session reads its resources from the launcher's plugin, never from the daemon's", wsrsrc.EnvRsrcRoot)
		}
		root := hello.ProjectRoot
		if root == "" {
			root = defaultRootFrom(".", func(key string) string { return hello.Env[key] })
		}
		if root == "" || root == "." {
			root = hello.Cwd
		}
		opts := mcp.ServerOptions{
			Root:         root,
			Version:      version,
			SourceCommit: sourceCommit,
			Env:          env,
			OwnerPID:     hello.OwnerPID,
			Harness:      hello.Harness,
			Diag:         prefixedWriter{w: h.logw, prefix: fmt.Sprintf("dashboard: session pid %d: ", hello.OwnerPID)},
		}
		if d := h.live.Load(); d != nil {
			opts.Live = d
		}
		if h.agents != nil {
			opts.Agents = h.agents
		}
		server := mcp.NewServerWithOptions(opts)
		h.applyLANChange(server)
		return &proxyMCPSession{server: server}, nil
	case wsdashboard.ProxyModeCLI:
		return &proxyCLISession{hello: hello, host: h}, nil
	}
	return nil, fmt.Errorf("unsupported mode %q", hello.Mode)
}

// callerHello resolves filesystem inputs against the caller, including defaults
// when an older launcher omitted cache/config overrides. Passing empty options
// to a store would otherwise fall back to the daemon's WS_* environment.
func callerHello(hello wsdashboard.ProxyHello) wsdashboard.ProxyHello {
	env := make(map[string]string, len(hello.Env)+2)
	for key, value := range hello.Env {
		env[key] = value
	}
	hello.Env = env
	home := env["HOME"]
	if runtime.GOOS == "windows" {
		home = env["USERPROFILE"]
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if env["WS_CACHE_HOME"] == "" {
		env["WS_CACHE_HOME"] = filepath.Join(home, ".cache", "ws@kang-sw-devenv")
	}
	if env["WS_CONFIG_HOME"] == "" {
		env["WS_CONFIG_HOME"] = filepath.Join(home, ".ws")
	}
	absolute := func(path string) string {
		if path != "" && !filepath.IsAbs(path) {
			return filepath.Join(hello.Cwd, path)
		}
		return path
	}
	for _, key := range []string{"WS_CACHE_HOME", "WS_CONFIG_HOME", "WS_RSRC_ROOT", "WS_SKILLS_ROOT", "WS_MCP_PROJECT_ROOT", "WS_WORKTREE_DIR", "WS_MCP_DEBUG_LOG", "WS_MCP_RUNTIME_BINARY", "WS_MCP_RUNTIME_DIR"} {
		if path, ok := env[key]; ok {
			env[key] = absolute(path)
		}
	}
	if manifests := env["WS_MCP_CONFIG_MANIFESTS"]; manifests != "" {
		paths := strings.FieldsFunc(manifests, func(r rune) bool { return r == ';' || r == rune(os.PathListSeparator) })
		for i, path := range paths {
			paths[i] = absolute(path)
		}
		env["WS_MCP_CONFIG_MANIFESTS"] = strings.Join(paths, ";")
	}
	hello.ProjectRoot = absolute(hello.ProjectRoot)
	return hello
}

// applyLANChange re-reads the global dashboard.lan_enabled through the
// connection's server: a value that differs from this daemon's mode
// replaces the daemon, once, through the drain path (Ensure asks this
// daemon to stop and starts a successor; the launchers reconnect to it).
//
// Only a session that takes part in the dashboard has a say: one whose
// manifests declare no dashboard keys, or that sets WS_MCP_NO_DASHBOARD,
// reads the key's default rather than the user's value, and one anchored at
// another WS_CONFIG_HOME reads another user's config file. Each of those
// used to flip the daemon back and forth with every connection.
func (h *proxyHost) applyLANChange(server *mcp.Server) {
	if !server.DashboardApplies() {
		return
	}
	if path, err := server.GlobalConfigPath(); err != nil || filepath.Clean(path) != h.configPath {
		return
	}
	settings := server.DashboardSettings()
	if settings.LAN == h.lan || h.replacing.Swap(true) {
		return
	}
	fmt.Fprintf(h.logw, "dashboard: global dashboard.lan_enabled is now %v; replacing this daemon\n", settings.LAN)
	port := settings.Port
	if port <= 0 {
		port = h.port
	}
	h.background.Add(1)
	go func() {
		defer h.background.Done()
		defer func() { _ = recover() }()
		spawn := func(args []string) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			return wsdashboard.SpawnDetached(exe, args, h.dir)
		}
		outcome, err := wsdashboard.Ensure(context.Background(), wsdashboard.EnsureOptions{
			Dir:         h.dir,
			Version:     version,
			BuildTime:   h.buildTime,
			ContentHash: h.contentHash,
			Port:        port,
			LAN:         settings.LAN,
			Spawn:       spawn,
			Log:         h.logw,
		})
		if err != nil || outcome == wsdashboard.OutcomeUnavailable {
			fmt.Fprintf(h.logw, "dashboard: LAN mode change did not take (%s, %v)\n", outcome, err)
			h.replacing.Store(false)
		}
	}()
}

// prefixedWriter prefixes every write, so a session's diagnostics in the
// daemon log say which session they belong to.
type prefixedWriter struct {
	w      io.Writer
	prefix string
}

func (p prefixedWriter) Write(b []byte) (int, error) {
	if _, err := io.WriteString(p.w, p.prefix+string(b)); err != nil {
		return 0, err
	}
	return len(b), nil
}

// proxyMCPSession serves one mcp.Server over the stream.
type proxyMCPSession struct {
	server *mcp.Server
}

func (s *proxyMCPSession) Serve(ctx context.Context, stream io.ReadWriteCloser) {
	defer func() {
		if r := recover(); r != nil {
			mcp.RecordLifecycleEvent("session.panic", map[string]any{"panic": fmt.Sprint(r)})
		}
	}()
	_ = s.server.ServeStdio(ctx, stream, stream)
	s.server.Close()
}

func (s *proxyMCPSession) Drain(ctx context.Context) bool { return s.server.Drain(ctx) }

// proxyCLISession frames a CLI invocation's output. Disconnect cancels the
// invocation. Drain cancels long waits, but lets mutations finish, just like
// hosted MCP calls, rather than interrupting a commit or ticket move halfway.
type proxyCLISession struct {
	hello    wsdashboard.ProxyHello
	host     *proxyHost
	server   atomic.Pointer[mcp.Server]
	draining atomic.Bool

	cancel func()
	done   chan struct{}
	mu     sync.Mutex
}

func (s *proxyCLISession) Serve(ctx context.Context, stream io.ReadWriteCloser) {
	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.done = make(chan struct{})
	s.mu.Unlock()
	defer close(s.done)
	defer cancel()
	// The launcher never writes after the hello; a read that ends means it
	// is gone.
	go func() {
		_, _ = io.Copy(io.Discard, stream)
		cancel()
	}()
	var writeMu sync.Mutex
	writeFrame := func(frame wsdashboard.ProxyCLIFrame) {
		raw, err := json.Marshal(frame)
		if err != nil {
			return
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		_, _ = stream.Write(append(raw, '\n'))
	}
	var stdout bytes.Buffer
	stderr := &frameWriter{emit: func(data string) { writeFrame(wsdashboard.ProxyCLIFrame{Stream: "stderr", Data: data}) }}
	command := hostedCLI{hello: s.hello, host: s.host, ctx: ctx, stdout: &stdout, stderr: stderr, env: mcp.EnvFromMap(s.hello.Env), serverReady: func(server *mcp.Server) {
		s.server.Store(server)
		if s.draining.Load() {
			server.Drain(context.Background())
		}
	}}
	code := command.run()
	writeFrame(wsdashboard.ProxyCLIFrame{Final: true, ExitCode: code, Data: stdout.String()})
}

func (s *proxyCLISession) Drain(ctx context.Context) bool {
	s.draining.Store(true)
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel == nil {
		return true
	}
	if len(s.hello.Argv) >= 2 && s.hello.Argv[0] == "mailbox" && s.hello.Argv[1] == "wait" {
		cancel()
	}
	if server := s.server.Load(); server != nil {
		server.Drain(ctx)
	}
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

// frameWriter turns each write into one frame.
type frameWriter struct {
	emit func(string)
}

func (f *frameWriter) Write(b []byte) (int, error) {
	f.emit(string(b))
	return len(b), nil
}

// daemonStartLAN is the LAN mode daemon-start asks Ensure for. A starter
// the dashboard applies to (a dashboard manifest declared, WS_MCP_NO_DASHBOARD
// unset) resolved settingsLAN from its configuration as serve does, and
// decides. Any other starter does not decide LAN, the rule applyLANChange
// applies to a hosted session: a daemon already running keeps its mode
// (running reports it when one answers), and with none the global
// dashboard.lan_enabled value under the starter's WS_CONFIG_HOME decides
// (global), since the daemon it starts inherits that config home. The
// alternative, settings.LAN read as false, would replace a LAN daemon on
// every start by a session the dashboard does not apply to.
func daemonStartLAN(applies, settingsLAN bool, running func() (lan, ok bool), global func() bool) bool {
	if applies {
		return settingsLAN
	}
	if lan, ok := running(); ok {
		return lan
	}
	return global()
}

// daemonStart is the entry the launcher runs from the staged daemon binary
// when no daemon answers, or the running one is older: it resolves the
// dashboard settings from the launcher's environment and configuration as
// serve does, and runs the Ensure handoff (a daemon as new or newer stays;
// an older one, or one in the other LAN mode, is replaced). The dashboard
// key's own value does not gate it: the daemon always hosts the MCP
// sessions, and only each session's listing follows the key. A starter the
// dashboard does not apply to does not decide the LAN mode
// (daemonStartLAN). It prints the
// outcome as one JSON line and exits non-zero only when no daemon came up.
func daemonStart(args []string) {
	fs := flag.NewFlagSet("daemon-start", flag.ExitOnError)
	buildTime := fs.Int64("build-time", 0, "the build time of the ws-mcp this daemon stands for, in Unix milliseconds; 0 reads this executable's modification time")
	startWait := fs.Duration("start-wait", 0, "how long to wait for a started daemon; 0 means the default")
	_ = fs.Parse(args)

	server := mcp.NewServer(defaultRoot("."), version, sourceCommit)
	settings := server.DashboardSettings()
	dir, err := wsdashboard.Dir("")
	if err == nil {
		err = os.MkdirAll(dir, 0o755)
	}
	if err != nil {
		fatal("daemon-start", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*wsdashboard.DefaultStartWait+*startWait)
	defer cancel()
	lan := daemonStartLAN(server.DashboardApplies(), settings.LAN,
		func() (bool, bool) {
			ep, err := wsdashboard.ReadEndpoint(dir)
			if err != nil {
				return false, false
			}
			info, err := wsdashboard.Health(ctx, &http.Client{Timeout: wsdashboard.DefaultHealthTimeout, Transport: &http.Transport{Proxy: nil}}, ep)
			return info.LAN, err == nil
		},
		func() bool {
			value, _ := wsconfig.GlobalOverride(wsconfig.Options{}, mcp.DashboardLANKey)
			return value == "true"
		})
	exe, err := os.Executable()
	if err != nil {
		fatal("daemon-start", err)
	}
	if *buildTime <= 0 {
		*buildTime = wsdashboard.ExecutableBuildTime(exe)
	}
	spawn := func(args []string) error { return wsdashboard.SpawnDetached(exe, args, dir) }
	outcome, err := wsdashboard.Ensure(ctx, wsdashboard.EnsureOptions{
		Dir:         dir,
		Version:     version,
		BuildTime:   *buildTime,
		ContentHash: wsdashboard.ExecutableContentHash(exe),
		Port:        settings.Port,
		LAN:         lan,
		Spawn:       spawn,
		StartWait:   *startWait,
		Log:         os.Stderr,
	})
	if err != nil {
		fatal("daemon-start", err)
	}
	ep, _ := wsdashboard.ReadEndpoint(dir)
	printJSONOrFatal("daemon-start", map[string]any{"outcome": outcome, "port": ep.Port, "pid": ep.PID, "version": ep.Version}, nil)
	if outcome == wsdashboard.OutcomeUnavailable {
		os.Exit(1)
	}
}
