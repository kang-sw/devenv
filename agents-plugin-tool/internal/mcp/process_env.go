package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/execjob"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsrsrc"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// process_env.go is the per-caller process context of a Server. A stdio
// server built with NewServer serves the process it runs in, so its
// environment, PID, and debug ring are the process's own. A server the
// dashboard daemon builds for one proxy connection serves another process:
// the launcher that holds the harness's stdio. Everything that used to be a
// process-global read (os.Getenv, os.Getpid, the debug ring) goes through
// the accessors here, so two connections hosted by one daemon never see each
// other's environment and never fall back to the daemon's own.

// ServerOptions configures a Server for one caller process.
type ServerOptions struct {
	// Root is the repository root the server resolves tool roots from.
	Root string
	// Version is the ws-mcp version reported to the client.
	Version string
	// SourceCommit is the build's source commit; empty means "dev".
	SourceCommit string
	// Env is the caller process's environment as "KEY=value" entries. nil
	// means this process's own environment, read live on every access (the
	// stdio path); a non-nil slice, even an empty one, is the whole
	// environment the server sees.
	Env []string
	// OwnerPID is the PID that owns the server's mailbox presence and
	// listening markers: the launcher a proxy connection belongs to. Zero
	// means this process.
	OwnerPID int
	// Harness is the harness kind the caller reported, when known; the
	// server still detects it from the initialize handshake.
	Harness string
	// Diag receives the server's diagnostics (a rejected config manifest at
	// construction, a hosted session it could not confirm); nil means stderr.
	Diag io.Writer
	// Live is the dashboard daemon hosting this server: the dashboard live
	// layer hands each session's snapshot to it in memory, and writes the
	// snapshot file only for recovery (live_snapshot.go). nil is the direct
	// path, a ws-mcp in its own process, which writes the file and nudges.
	Live wsdashboard.LiveHost
	// Agents is the dashboard daemon's agent runner, where the pi.* tools
	// spawn the session's Pi subagents (pi_tools.go); nil on the direct
	// path, where they refuse.
	Agents PiAgents
}

// procEnv is a snapshot of one process's environment.
type procEnv struct {
	list  []string
	byKey map[string]string
}

func newProcEnv(list []string) *procEnv {
	e := &procEnv{list: append([]string(nil), list...), byKey: make(map[string]string, len(list))}
	for _, kv := range list {
		key, value, ok := strings.Cut(kv, "=")
		if !ok || key == "" {
			continue
		}
		if runtime.GOOS == "windows" {
			key = strings.ToUpper(key)
		}
		e.byKey[key] = value
	}
	return e
}

func (e *procEnv) get(key string) string {
	if runtime.GOOS == "windows" {
		key = strings.ToUpper(key)
	}
	return e.byKey[key]
}

// EnvFromMap turns an environment map into the "KEY=value" list
// ServerOptions.Env takes, in a stable order.
func EnvFromMap(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	sortStrings(out)
	return out
}

func sortStrings(list []string) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && list[j] < list[j-1]; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

// NewServerWithOptions builds a server for the process opts describes.
func NewServerWithOptions(opts ServerOptions) *Server {
	commit := opts.SourceCommit
	if commit == "" {
		commit = "dev"
	}
	diag := opts.Diag
	if diag == nil {
		diag = os.Stderr
	}
	s := &Server{root: filepath.Clean(opts.Root), version: opts.Version, sourceCommit: commit, ownerPID: opts.OwnerPID, diag: diag}
	s.live.host = opts.Live
	if opts.Env != nil {
		s.env = newProcEnv(opts.Env)
		logPath := strings.TrimSpace(s.env.get(envDebugLog))
		s.debug = &debugRing{logPath: func() string { return logPath }}
		s.dashboard.cacheHome = s.env.get(wsstate.EnvCacheHome)
	} else {
		s.debug = processDebug
	}
	s.inflightDone = make(chan struct{})
	s.drainCtx, s.drainCancel = context.WithCancel(context.Background())
	s.sessions = newSessionStore()
	s.sessions.cacheHome = s.getenv(wsstate.EnvCacheHome)
	s.declared = loadDeclaredConfig(s.getenv(envConfigManifests))
	for _, rejected := range s.declared.rejected {
		fmt.Fprintf(diag, "ws-mcp: config manifest rejected: %s\n", rejected)
		s.debugEvent("config.manifest_rejected", map[string]any{"manifest": rejected})
	}
	if opts.Harness != "" {
		s.observeHarness("hello", opts.Harness)
	}
	s.pi.agents = opts.Agents
	s.startPiFeed()
	return s
}

// getenv reads one variable of the caller process's environment.
func (s *Server) getenv(key string) string {
	if s.env == nil {
		return os.Getenv(key)
	}
	return s.env.get(key)
}

// Getenv is getenv for the command layer that hosts proxy connections.
func (s *Server) Getenv(key string) string { return s.getenv(key) }

// environ is the caller process's whole environment, the one every
// subprocess a tool starts runs under.
func (s *Server) environ() []string {
	if s.env == nil {
		return os.Environ()
	}
	return append([]string(nil), s.env.list...)
}

// Environ is environ for the command layer.
func (s *Server) Environ() []string { return s.environ() }

// pid is the PID that owns this server's presence records and markers.
func (s *Server) pid() int {
	if s.ownerPID > 0 {
		return s.ownerPID
	}
	return os.Getpid()
}

// configOptions anchors wsconfig at the caller's WS_CONFIG_HOME and
// WS_CACHE_HOME rather than this process's.
func (s *Server) configOptions() wsconfig.Options {
	return wsconfig.Options{ConfigHome: s.getenv(wsconfig.EnvConfigHome), CacheHome: s.getenv(wsstate.EnvCacheHome)}
}

// stateOptions anchors wsstate at the caller's WS_CACHE_HOME.
func (s *Server) stateOptions() wsstate.Options {
	return wsstate.Options{CacheHome: s.getenv(wsstate.EnvCacheHome), Env: s.environ()}
}

// execStore is the exec-job store under the caller's cache home and
// environment.
func (s *Server) execStore() execjob.Store {
	return execjob.Store{State: s.stateOptions()}
}

// mailboxPaths anchors the mailbox stores, reply registry, and listening
// markers at the caller's config and cache homes.
func (s *Server) mailboxPaths() wsmailbox.Paths {
	return wsmailbox.Paths{Config: s.configOptions(), State: s.stateOptions()}
}

// gitClient is the git client whose subprocesses run under the caller's
// environment.
func (s *Server) gitClient() wsgit.Client {
	return wsgit.Client{Runner: s.gitRunner()}
}

// gitRunner is the git runner whose subprocesses run under the caller's
// environment, for the call sites that take a bare wsgit.Runner.
func (s *Server) gitRunner() wsgit.ExecRunner {
	return wsgit.ExecRunner{Environ: s.environ}
}

// rsrcRoot is the caller's resource tree. A hosted server takes it from
// the caller's WS_RSRC_ROOT alone: the daemon's own executable lives in its
// state directory, beside no resource tree, so the executable-derived
// fallback would resolve to nothing meaningful.
func (s *Server) rsrcRoot() (string, error) {
	if s.env == nil {
		return wsrsrc.ResolveRoot()
	}
	if root := s.getenv(wsrsrc.EnvRsrcRoot); root != "" {
		return root, nil
	}
	return "", fmt.Errorf("%s not set for this connection", wsrsrc.EnvRsrcRoot)
}

// skillsRoot is the caller's skills tree, resolved as rsrcRoot is.
func (s *Server) skillsRoot() (string, error) {
	if s.env == nil {
		return wsrsrc.ResolveSkillsRoot()
	}
	if root := s.getenv(wsrsrc.EnvSkillsRoot); root != "" {
		return root, nil
	}
	return "", fmt.Errorf("%s not set for this connection", wsrsrc.EnvSkillsRoot)
}

func envFlagOn(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// noDashboardMode reports whether the caller's envNoDashboard turns the
// dashboard off.
func (s *Server) noDashboardMode() bool { return envFlagOn(s.getenv(envNoDashboard)) }

// hostedRowPoll and hostedRowWait pace the wait for a hosted session's
// registration to name its harness process (resolveHostedSession).
var (
	hostedRowPoll = 100 * time.Millisecond
	hostedRowWait = 3 * time.Second
)

// hostedSessionKey is the daemon-minted lead key of the session this server
// serves, when the dashboard daemon hosts that session; empty for every
// other caller. It is resolved at the first call and never again: a server
// that resolved as an ordinary one stays one, whatever the registration
// says later.
func (s *Server) hostedSessionKey() string {
	s.hostedOnce.Do(func() { s.hostedKey = s.resolveHostedSession() })
	return s.hostedKey
}

// resolveHostedSession reads the caller's hosted-session variables. The
// daemon sets them on the harness it spawns, and every process below that
// harness inherits them: a second harness the session starts, a CLI call
// from its shell. So the key holds only for a server whose harness is the
// process the session's registration names as host_pid:
//
//   - envHostedSession off, or no key, row path, or harness PID: not hosted.
//     The harness PID comes from the launcher, so a ws-mcp started without
//     it is never hosted.
//   - host_pid equal to the harness PID: hosted.
//   - host_pid another process: not hosted, silently. The variables were
//     inherited.
//   - host_pid zero or absent, or the row unreadable: the daemon writes the
//     PID right after it starts the harness, so this waits hostedRowWait
//     for it, then gives up with one warning.
func (s *Server) resolveHostedSession() string {
	if !envFlagOn(s.getenv(envHostedSession)) {
		return ""
	}
	key := strings.TrimSpace(s.getenv(envHostedSessionKey))
	row := strings.TrimSpace(s.getenv(envHostedSessionRow))
	harness, _ := strconv.Atoi(strings.TrimSpace(s.getenv(envHarnessPID)))
	if key == "" || row == "" || harness <= 0 {
		return ""
	}
	deadline := time.Now().Add(hostedRowWait)
	for {
		switch pid := hostedRowPID(row); {
		case pid == harness:
			return key
		case pid > 0:
			return ""
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(hostedRowPoll)
	}
	diag := s.diag
	if diag == nil {
		diag = os.Stderr
	}
	fmt.Fprintf(diag, "ws-mcp: %s is set, but %s named no harness process within %s; serving an ordinary session\n", envHostedSession, row, hostedRowWait)
	s.debugEvent("hosted_session.unconfirmed", map[string]any{"row": row, "harness_pid": harness})
	return ""
}

// hostedRowPID is the host_pid of the registration file at path; zero when
// the file is unreadable, does not parse, or names none.
func hostedRowPID(path string) int {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var row wsdashboard.Registration
	if err := json.Unmarshal(raw, &row); err != nil {
		return 0
	}
	return row.HostPID
}

// DropHarnessSessionEnv removes from this process's environment the
// variables that tie a ws-mcp to one harness session: the hosted-session
// variables and the harness PID. The dashboard daemon calls it at start. Its
// environment is that of whichever session's launcher started it, and the
// conversations it spawns, with the ws-mcp each attaches without a
// launcher, would otherwise read that session's as their own.
func DropHarnessSessionEnv() {
	for _, name := range []string{envHostedSession, envHostedSessionKey, envHostedSessionRow, envHarnessPID} {
		_ = os.Unsetenv(name)
	}
}

// DashboardApplies reports whether this server's caller takes part in the
// dashboard at all: a loaded manifest declares the dashboard keys and the
// caller does not set WS_MCP_NO_DASHBOARD. A caller outside it has no say
// over the daemon (its global dashboard.lan_enabled reads as the manifest
// default, not as a decision).
func (s *Server) DashboardApplies() bool { return s.dashboardDeclared() && !s.noDashboardMode() }

// GlobalConfigPath is the global config file this server's caller reads,
// anchored at its WS_CONFIG_HOME.
func (s *Server) GlobalConfigPath() (string, error) {
	return wsconfig.GlobalPath(s.configOptions())
}

// noAgentMode reports whether the caller runs the agentless profile.
func (s *Server) noAgentMode() bool { return envFlagOn(s.getenv(envNoAgent)) }

// runtimeNamespace is the caller's tool namespace, "ws" by default.
func (s *Server) runtimeNamespace() string {
	if value := strings.TrimSpace(s.getenv(envNamespace)); value != "" {
		return value
	}
	return "ws"
}

// cliCommandName is CLICommandName for the caller process.
func (s *Server) cliCommandName() string { return CLICommandNameFrom(s.getenv) }

// explicitAllowedTools parses the caller's WS_MCP_ALLOWED_TOOLS allowlist;
// nil means no allowlist.
func (s *Server) explicitAllowedTools() map[string]bool {
	return parseAllowedTools(s.getenv(envAllowedTools))
}

// requestStarted counts one request in flight; the returned func ends it.
// A draining server refuses it, so a request that arrives after the daemon
// began shutting down never starts work the proxy will have to abandon.
func (s *Server) requestStarted() (done func(), ok bool) {
	s.inflightMu.Lock()
	defer s.inflightMu.Unlock()
	if s.draining {
		return nil, false
	}
	s.inflight++
	return func() {
		s.inflightMu.Lock()
		defer s.inflightMu.Unlock()
		s.inflight--
		if s.inflight == 0 && s.draining {
			s.signalIdle()
		}
	}, true
}

func (s *Server) signalIdle() {
	select {
	case <-s.inflightDone:
	default:
		close(s.inflightDone)
	}
}

// drainContext ends when Drain begins. A tool call that blocks for a
// deadline of its own (exec.result with timeout_seconds) waits on it, so
// the drain cancels the wait at once and the call answers with the state
// it found, rather than holding the daemon for the rest of its deadline.
func (s *Server) drainContext() context.Context {
	if s.drainCtx == nil {
		return context.Background()
	}
	return s.drainCtx
}

// Drain stops the server taking new requests, cancels the long waits in
// flight (drainContext), and waits for the requests in flight to finish,
// or for ctx to end. It reports whether the server went idle. The caller
// closes the stream afterwards; ServeStdio returns then.
func (s *Server) Drain(ctx context.Context) bool {
	if s.drainCancel != nil {
		s.drainCancel()
	}
	s.inflightMu.Lock()
	s.draining = true
	if s.inflight == 0 {
		s.signalIdle()
	}
	s.inflightMu.Unlock()
	select {
	case <-s.inflightDone:
		return true
	case <-ctx.Done():
		return false
	}
}

// Close releases what a server holds beyond its serve loop: the dashboard
// live sessions (a hosted one flushes its snapshot file and releases the
// daemon's copy), the dashboard nudge sender's goroutine, and the tracker
// host. A stdio server's process exit frees these; a daemon-hosted server
// must release them itself, or every ended session keeps its Server alive
// for the daemon's lifetime. Call it after ServeStdio has returned.
func (s *Server) Close() {
	if stop := s.pi.stopWatch; stop != nil {
		stop()
	}
	s.live.close()
	d := &s.dashboard
	d.mu.Lock()
	if d.nudges != nil && !d.nudgesClosed {
		close(d.nudges)
	}
	d.nudgesClosed = true
	d.mu.Unlock()
	if host := s.tracker.host; host != nil {
		_ = host.Close()
	}
}

// debugRing is one server's recent debug events, mirrored to a log file
// when one is configured.
type debugRing struct {
	mu      sync.Mutex
	events  []map[string]any
	logPath func() string
}

// processDebug is the ring of the stdio server and of the process-level
// lifecycle records (RecordLifecycleEvent, recordPanic), which have no
// server to belong to.
var processDebug = &debugRing{logPath: func() string { return os.Getenv(envDebugLog) }}

func (r *debugRing) append(event string, fields map[string]any) {
	record := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"event": event,
	}
	for key, value := range fields {
		record[key] = value
	}
	r.mu.Lock()
	r.events = append(r.events, record)
	if len(r.events) > maxDebugEvents {
		r.events = append([]map[string]any(nil), r.events[len(r.events)-maxDebugEvents:]...)
	}
	r.mu.Unlock()

	path := strings.TrimSpace(r.logPath())
	if path == "" {
		return
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(raw, '\n'))
}

func (r *debugRing) recent(limit int) []map[string]any {
	if limit <= 0 || limit > maxDebugEvents {
		limit = maxDebugEvents
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	start := len(r.events) - limit
	if start < 0 {
		start = 0
	}
	out := make([]map[string]any, len(r.events[start:]))
	copy(out, r.events[start:])
	return out
}

func (r *debugRing) jsonl(limit int) (string, error) {
	var b strings.Builder
	for _, event := range r.recent(limit) {
		raw, err := json.Marshal(event)
		if err != nil {
			return "", err
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// debugEvent records an event in this server's ring.
func (s *Server) debugEvent(event string, fields map[string]any) {
	s.debug.append(event, fields)
}
