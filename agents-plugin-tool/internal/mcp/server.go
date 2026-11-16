package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/execjob"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsindex"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsstate"
)

type Server struct {
	root              string
	version           string
	sourceCommit      string
	whiteboardChanged func(cacheHome, root, key string)
	loginLink         func(string) (string, error)
	rootMu            sync.RWMutex
	sessionHarness    string
	piDaemonAgents    bool // initialize experimental capability, protected by rootMu
	sessions          *sessionStore
	mailbox           mailboxRuntimeState
	// indexOpts configures the ticket ownership index client; zero values
	// take the library defaults. Tests inject clocks and timeouts here.
	indexOpts wsindex.Options
	// dispatchBlock permits a test to inject a failure between the two scans.
	dispatchBlock func(string, wsdoc.TicketInfo, []string) (*wsdoc.DispatchBlock, error)
	// declared holds the adapter-declared config keys loaded from the
	// envConfigManifests manifests at construction.
	declared declaredConfig
	// live holds the dashboard live state of the sessions this process
	// received adapter intake for (live_snapshot.go).
	live liveHub
	// intake tracks canonical operations from connection open; direct/file-only
	// handler callers do not negotiate reasoning_summary_v1.
	intake *intakeOrder
	// dashboard is the dashboard session registry and nudge state
	// (dashboard_sessions.go).
	dashboard dashboardLink
	// tracker is this process's tracker host (tracker_host.go).
	tracker trackerLink
	// pi is the pi.* tools' daemon runner and live feed (pi_tools.go).
	pi piHost
	// env is the caller process's environment; nil reads this process's
	// live (process_env.go). ownerPID is the caller's PID; zero means this
	// process. debug is the caller's debug ring.
	env      *procEnv
	ownerPID int
	debug    *debugRing
	// diag receives this server's own warnings: stderr, or the daemon's log
	// for a proxy connection.
	diag io.Writer
	// hostedKey is the caller's hosted-session resolution, made once at the
	// first hostedSessionKey call.
	hostedOnce sync.Once
	hostedKey  string
	// inflight counts the requests being handled; draining refuses new
	// ones, and inflightDone closes once a draining server goes idle
	// (Drain).
	inflightMu   sync.Mutex
	inflight     int
	draining     bool
	inflightDone chan struct{}
	// drainCtx ends when Drain begins: the blocking waits a tool call holds
	// (exec.result's timeout) select on it, so a drain cancels them at once
	// instead of waiting them out.
	drainCtx    context.Context
	drainCancel context.CancelFunc
}

type toolRole string

const (
	roleLead     toolRole = "lead"
	roleDelegate toolRole = "delegate"
	roleLeaf     toolRole = "leaf"
)

// bootstrapToolName is the deliberately obscure, function-inert name of the
// lead session-bootstrap tool (mints a session key). Its name<->purpose mapping
// is taught ONLY in ws:workflow-manual (260617 obscurity): a semantically
// meaningless name removes the session-start "pull" that a descriptive name
// (login/attach/bind) creates for every agent, lowering accidental/curious
// invocation by subagents that share the lead's MCP connection. Invariants:
// do not give it a descriptive alias, do not leak its purpose through its
// tools/list description, and do not name it in error-guidance strings.
const bootstrapToolName = "ferrule"

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type cancelledNotificationParams struct {
	RequestID json.RawMessage `json:"requestId"`
	Reason    string          `json:"reason,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const ProtocolVersion = "2025-03-26"

const maxDebugEvents = 256

const leadWorkflowBootstrapMethod = "lead-workflow-bootstrap"

const (
	envNoAgent   = "WS_MCP_NO_AGENT"
	envNamespace = "WS_MCP_NAMESPACE"
	// envNoDashboard turns the dashboard off for this ws-mcp whatever its
	// config resolves: no daemon ensure, no session registration. The
	// dashboard daemon's agent runner sets it on the ws-mcp it attaches to
	// an agent conversation, which the daemon itself hosts.
	envNoDashboard = "WS_MCP_NO_DASHBOARD"
	// envHostedSession marks a session the dashboard daemon hosts,
	// envHostedSessionKey carries the lead key the daemon minted for it, and
	// envHostedSessionRow names the session's registration file, whose
	// host_pid is the one harness process the flag holds for. Every process
	// below that harness inherits all three, so this ws-mcp honours them
	// only when its own harness (envHarnessPID) is the one the row names
	// (hostedSessionKey); it then leaves the session's registration to the
	// daemon and holds the session to that key.
	envHostedSession    = "WS_HOSTED_SESSION"
	envHostedSessionKey = "WS_HOSTED_SESSION_KEY"
	envHostedSessionRow = "WS_HOSTED_SESSION_ROW"
	// envHarnessPID is the PID of the process that started this ws-mcp's
	// launcher: the harness. The launcher sets it on every start, to its
	// own parent, never an inherited value. On Windows that parent may be a
	// cmd.exe wrapper; the runtime reads it through Server.harnessPID, which
	// skips it.
	envHarnessPID = "WS_MCP_HARNESS_PID"
	// envDebugLog names a file every debug event is appended to.
	envDebugLog = "WS_MCP_DEBUG_LOG"
	// envAllowedTools is the comma-separated tool allowlist.
	envAllowedTools = "WS_MCP_ALLOWED_TOOLS"
)

// testPanicHook is always nil in production. Only _test.go files in this
// package set it, to exercise goroutine-level panic recovery (server.go's
// recover-defer in ServeStdio) through a real dispatched tools/call request
// instead of a synthetic panic.
var testPanicHook func(toolName string)

// NewServer builds a server for this process: its environment, read live,
// its PID, and the process debug ring.
func NewServer(root, version string, sourceCommit ...string) *Server {
	commit := ""
	if len(sourceCommit) > 0 {
		commit = sourceCommit[0]
	}
	return NewServerWithOptions(ServerOptions{Root: root, Version: version, SourceCommit: commit})
}

func (s *Server) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	s.intake = &intakeOrder{}
	// Restoring the launcher's dashboard rows scans the registry and the
	// session store; run it beside the serve loop so requests are read (and
	// refused while draining) at once, even when a drain starts right after
	// accept. Handlers and the end path wait for it, so no reply precedes
	// the restore and a late restore never outlives dashboardEnd.
	reconnectDone := make(chan struct{})
	go func() {
		defer close(reconnectDone)
		s.dashboardReconnect()
	}()
	scanner := bufio.NewScanner(in)
	encoder := json.NewEncoder(out)
	var writeMu sync.Mutex
	var wg sync.WaitGroup
	var requests sync.Map

	writeResponse := func(resp response) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return encoder.Encode(resp)
	}

	// The mailbox presence ticker lives exactly as long as this serve loop:
	// every return path below cancels it and waits for its goroutine. At
	// EOF it stops before in-flight handlers drain, so a disconnected
	// client's name is not kept live by a slow handler.
	tickerCtx, stopTicker := context.WithCancel(ctx)
	tickerDone := s.startMailboxPresenceTicker(tickerCtx)
	stopPresenceTicker := func() {
		stopTicker()
		<-tickerDone
	}
	defer stopPresenceTicker()
	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	heartbeatDone := s.startDashboardHeartbeat(heartbeatCtx)
	defer func() {
		stopHeartbeat()
		<-heartbeatDone
		<-reconnectDone
		s.dashboardEnd()
	}()

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		default:
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			s.debugEvent("parse_error", map[string]any{"error": err.Error()})
			if err := writeResponse(errorResponse(nil, -32700, "parse error")); err != nil {
				return err
			}
			continue
		}
		if _, v1 := canonicalIntake(req); v1 && len(line)+1 > wslive.SummaryFrameBytes {
			if err := writeResponse(errorResponse(req.ID, -32602, "reasoning summary JSONL frame exceeds 49152 bytes")); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) == 0 {
			s.handleNotification(req, &requests)
			continue
		}
		s.debugEvent("request.received", map[string]any{"id": rawMessageString(req.ID), "method": req.Method})
		id := rawMessageString(req.ID)
		finish, ok := s.requestStarted()
		if !ok {
			// The daemon hosting this server is draining: the proxy
			// reconnects to its successor and replays, so refuse rather
			// than start work this stream will not live to answer.
			if err := writeResponse(errorResponse(req.ID, -32000, "server draining")); err != nil {
				return err
			}
			continue
		}
		op := s.intake.ingress(req)
		reqCtx, cancel := context.WithCancel(ctx)
		requests.Store(id, cancel)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer op.finish()
			defer finish()
			defer cancel()
			defer requests.Delete(id)
			// This defer is appended after the three above, so on unwind it
			// runs FIRST (Go LIFO defer order): a panic inside s.handle is
			// caught and converted into a visible JSON-RPC error before
			// wg.Done/cancel/requests.Delete fire, so it fails only this
			// request instead of crashing the ws-mcp serve process.
			defer func() {
				if r := recover(); r != nil {
					stack := debug.Stack()
					s.recordPanic(req.Method, id, r, stack)
					resp := errorResponse(req.ID, -32000, fmt.Sprintf("internal error: request handler panicked (%s)", req.Method))
					if err := writeResponse(resp); err != nil {
						s.debugEvent("response.write_error", map[string]any{"id": id, "error": err.Error()})
					}
				}
			}()
			// Answer only once the launcher's rows are restored, so the
			// replayed initialize's reply means the rail sees them live.
			<-reconnectDone
			if err := op.await(s); err != nil {
				_ = writeResponse(toolTextResponse(req.ID, "", err))
				return
			}
			if err := writeResponse(s.handle(reqCtx, req)); err != nil {
				s.debugEvent("response.write_error", map[string]any{"id": id, "error": err.Error()})
			}
		}()
	}
	err := scanner.Err()
	stopPresenceTicker()
	wg.Wait()
	return err
}

func (s *Server) handleNotification(req request, requests *sync.Map) {
	switch req.Method {
	case "notifications/cancelled":
		var params cancelledNotificationParams
		fields := map[string]any{"method": req.Method}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			fields["error"] = err.Error()
		} else {
			requestID := rawMessageString(params.RequestID)
			fields["request_id"] = requestID
			if params.Reason != "" {
				fields["reason"] = params.Reason
			}
			if cancelValue, ok := requests.Load(requestID); ok {
				if cancel, ok := cancelValue.(context.CancelFunc); ok {
					cancel()
					fields["matched"] = true
				}
			}
		}
		s.debugEvent("notification.cancelled", fields)
	default:
		s.debugEvent("notification.ignored", map[string]any{"method": req.Method})
	}
}

func (s *Server) handle(ctx context.Context, req request) response {
	switch req.Method {
	case "initialize":
		s.observePiDaemonAgents(req.Params)
		s.observeHarness("initialize", detectHarnessFromInitializeParams(req.Params))
		// No session/root exists yet at this call: a machine-scope identity
		// registers immediately (needs no root); a worktree/clone identity
		// defers until a session-bound root arrives via the owning ferrule
		// login (rebindMailboxOwnerAtFerrule), per ensureMailboxRegistered's
		// root-deferral contract.
		s.ensureMailboxRegistered("")
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"protocolVersion": ProtocolVersion,
			"serverInfo": map[string]string{
				"name":    "ws-mcp",
				"version": s.version,
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		}}
	case "ping":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": s.filteredTools()}}
	case "tools/call":
		return s.callTool(ctx, req)
	default:
		return errorResponse(req.ID, -32601, "method not found")
	}
}

// appendDebugEvent records an event that belongs to no server: the
// process ring (processDebug). A server's own events go through
// Server.debugEvent, so a hosted server's ring holds only its connection's.
func appendDebugEvent(event string, fields map[string]any) {
	processDebug.append(event, fields)
}

// crashLogPath resolves the always-on, cross-platform panic-trace sink under
// the shared cache root (WS_CACHE_HOME or ~/.cache/ws@...), the same
// root-independent seam sessionStore.dirs uses for the session key store: a
// panicking request may not know which project root it belonged to, so the
// crash file lives in a flat top-level dir under CacheRoot(), not under any
// per-project layout. It creates the directory (0o755) if needed and returns
// the fixed file path; callers append-only (no rotation), matching the
// existing WS_MCP_DEBUG_LOG sink's precedent.
func crashLogPath() (string, error) {
	dir, err := crashDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mcp-panic.log"), nil
}

// crashDir resolves and creates the always-on, cross-platform crash-dir sink
// under the shared cache root (WS_CACHE_HOME or ~/.cache/ws@...). Shared by
// crashLogPath (mcp-panic.log) and RecordLifecycleEvent (mcp-lifecycle.log) so
// both sinks live in the same durable, root-independent location family.
func crashDir() (string, error) {
	cacheRoot, err := wsstate.CacheRoot(wsstate.Options{})
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cacheRoot, "crash")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create crash log dir: %w", err)
	}
	return dir, nil
}

// recordPanic persists a recovered request-goroutine panic to the always-on
// crash file (required sink) and mirrors it into the in-memory debug ring
// (and, if WS_MCP_DEBUG_LOG is set, that opt-in secondary log) via
// appendDebugEvent. Unlike appendDebugEvent's best-effort mirror, failure to
// resolve or write the crash file falls back to stderr so a broken/unwritable
// cache home never fully swallows the trace.
func recordPanic(method, id string, recovered any, stack []byte) {
	processDebug.append("request.panic", recordPanicToCrashLog(method, id, recovered, stack))
}

// recordPanic is the package function's server form: the crash file plus
// this server's ring.
func (s *Server) recordPanic(method, id string, recovered any, stack []byte) {
	s.debugEvent("request.panic", recordPanicToCrashLog(method, id, recovered, stack))
}

func recordPanicToCrashLog(method, id string, recovered any, stack []byte) map[string]any {
	fields := map[string]any{
		"id":     id,
		"method": method,
		"panic":  fmt.Sprint(recovered),
		"stack":  string(stack),
	}
	record := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"event": "request.panic",
	}
	for key, value := range fields {
		record[key] = value
	}
	if raw, err := json.Marshal(record); err == nil {
		if path, err := crashLogPath(); err == nil {
			if file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
				_, writeErr := file.Write(append(raw, '\n'))
				closeErr := file.Close()
				if writeErr != nil || closeErr != nil {
					fmt.Fprintf(os.Stderr, "mcp: failed to append crash log %s: write=%v close=%v\n", path, writeErr, closeErr)
				}
			} else {
				fmt.Fprintf(os.Stderr, "mcp: failed to open crash log %s: %v\n", path, err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "mcp: failed to resolve crash log path: %v\n", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "mcp: failed to marshal crash record: %v\n", err)
	}
	return fields
}

// RecordLifecycleEvent persists a serve-process lifecycle event (e.g. detected
// parent-process death) to the always-on crash-dir sink (<cache-root>/crash/
// mcp-lifecycle.log), the same durable location family as recordPanic, and
// mirrors it into the in-memory debug ring / opt-in WS_MCP_DEBUG_LOG via
// appendDebugEvent. Exported so cmd/ws-mcp's Windows parent-death watcher can
// leave a durable trace before it self-terminates the orphaned process. Failure
// to resolve or write the file falls back to stderr so a broken cache home
// never fully swallows the trace.
func RecordLifecycleEvent(event string, fields map[string]any) {
	record := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"event": event,
	}
	for key, value := range fields {
		record[key] = value
	}
	if raw, err := json.Marshal(record); err == nil {
		if dir, err := crashDir(); err == nil {
			path := filepath.Join(dir, "mcp-lifecycle.log")
			if file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
				_, writeErr := file.Write(append(raw, '\n'))
				closeErr := file.Close()
				if writeErr != nil || closeErr != nil {
					fmt.Fprintf(os.Stderr, "mcp: failed to append lifecycle log %s: write=%v close=%v\n", path, writeErr, closeErr)
				}
			} else {
				fmt.Fprintf(os.Stderr, "mcp: failed to open lifecycle log %s: %v\n", path, err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "mcp: failed to resolve lifecycle log path: %v\n", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "mcp: failed to marshal lifecycle record: %v\n", err)
	}

	appendDebugEvent(event, fields)
}

func recentDebugEvents(limit int) []map[string]any { return processDebug.recent(limit) }

func rawMessageString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return string(raw)
}

// NoDashboardMode reports whether envNoDashboard turns the dashboard off.
func NoDashboardMode() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envNoDashboard))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func NoAgentMode() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(envNoAgent)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func RuntimeNamespace() string {
	value := strings.TrimSpace(os.Getenv(envNamespace))
	if value == "" {
		return "ws"
	}
	return value
}

func runtimeInfo(version, sourceCommit string) (map[string]any, error) {
	return map[string]any{
		"version":       version,
		"source_commit": sourceCommit,
	}, nil
}

func wantsJSON(arguments map[string]any) bool {
	format, _ := arguments["format"].(string)
	return strings.EqualFold(strings.TrimSpace(format), "json")
}

func formatRuntimeInfo(info map[string]any) string {
	var b strings.Builder
	if version, _ := info["version"].(string); version != "" {
		fmt.Fprintf(&b, "version: %s\n", version)
	}
	if commit, _ := info["source_commit"].(string); commit != "" {
		fmt.Fprintf(&b, "source_commit: %s\n", commit)
	}
	return b.String()
}

func canonicalSetupRoot(runner wsgit.Runner, root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "<cwd>" {
		return "", fmt.Errorf("root must be an absolute repository path; the MCP server cannot infer the agent's current directory from %q", root)
	}
	return canonicalGitRoot(runner, root)
}

// handleLeadLogin implements the session-bootstrap tool (bootstrapToolName): canonicalize root, mint an
// ephemeral session key, store the {root, scope} entry in the registry, and
// return the key to the caller.
func (s *Server) handleLeadLogin(id json.RawMessage, arguments map[string]any) response {
	// Best-effort prune of stale key records, throttled to at most once per
	// pruneScanCadence internally. Piggybacks on every bootstrap call rather
	// than a dedicated tool or background goroutine; a prune hiccup must never
	// fail bootstrap, so its result is intentionally discarded.
	s.sessions.maybePrune()

	rootArg, _ := arguments["root"].(string)
	if strings.TrimSpace(rootArg) == "" {
		return toolTextResponse(id, "", fmt.Errorf("session bootstrap: root is required"))
	}
	canonical, err := canonicalSetupRoot(s.gitRunner(), rootArg)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	scope := parseCapabilityScope(arguments["capability"])
	parentKey, _ := arguments["parent_session_key"].(string)
	parentKey = strings.TrimSpace(parentKey)
	if relogin, _ := arguments["relogin_session_key"].(string); strings.TrimSpace(relogin) != "" {
		return s.handleLeadRelogin(id, arguments, strings.TrimSpace(relogin), canonical, scope, parentKey)
	}
	resolved, err := s.resolveSession(sessionAccess{mode: sessionAccessBootstrap, root: canonical, scope: scope, parent: parentKey})
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	key := resolved.key
	if resolved.hosted {
		s.rebindMailboxOwnerAtFerrule(key, "", roleLead, canonical)
		s.recordHarnessSession(key, arguments)
		return s.leadLoginResponse(id, arguments, key, canonical)
	}
	// Ferrule owner binding (mailbox core Decision 3): a parent-less
	// lead-capability mint is a top-lead (re-)login, so it rebinds this
	// process's active mailbox identity's owner pointer to the freshly minted
	// key. A parent-carrying mint, or a parent-less delegate/leaf mint (a
	// child launched without its lead's key), never touches it.
	s.rebindMailboxOwnerAtFerrule(key, parentKey, scope, canonical)
	// The same top-lead mint is the manual's documented session start, so it
	// runs the tracker's session-start check as the FRESH manual path does.
	if parentKey == "" && scope == roleLead {
		s.startTrackerSessionCheck()
		s.recordHarnessSession(key, arguments)
	}
	return s.leadLoginResponse(id, arguments, key, canonical)
}

// handleLeadRelogin is the bootstrap tool's hidden re-login: `relogin_session_key`
// names an existing key, and the call mints nothing. The argument is
// deliberately absent from the advertised tools/list schema: an agent that
// forgets its key mints a new one by convention, and an advertised re-login
// would invite re-logging in with another session's key to take its mailbox.
// The only intended caller is a harness adapter that persisted the lead key
// with its own session file and re-enters the same session (restart, reload,
// or revive with the key the human supplied). It is not named `session_key`
// because an adapter that default-fills `session_key` into every bridged call
// would turn every agent bootstrap into a re-login with the default key.
//
// Only a parent-less lead-capability key bound to this call's canonical root
// is accepted; it then takes the mailbox owner pointer under the ordinary
// rebind rules (rebindMailboxOwnerAtFerrule), including never stealing a name
// another live process holds. That rule is what keeps a copied session file
// (an import keeps the original's session id, so the copy re-logs in with the
// original's key) from becoming a mailbox takeover while the original process
// is alive. Any other key - unknown, parent-carrying, delegate or leaf, or
// bound to another root - is refused without a mint or an owner change, as is
// a re-login that also carries a parent or a non-lead capability, since those
// describe a key the re-login could never return.
func (s *Server) handleLeadRelogin(id json.RawMessage, arguments map[string]any, key, canonical string, scope toolRole, parentKey string) response {
	if _, err := s.resolveSession(sessionAccess{mode: sessionAccessRelogin, key: key, root: canonical, scope: scope, parent: parentKey}); err != nil {
		return toolTextResponse(id, "", err)
	}
	s.rebindMailboxOwnerAtFerrule(key, "", roleLead, canonical)
	s.recordHarnessSession(key, arguments)
	return s.leadLoginResponse(id, arguments, key, canonical)
}

// leadKeyRefusal is why key cannot re-log in as the top-level lead at
// canonical, as the predicate that follows the key in a refusal; empty when
// it can: a known, parent-less, lead-scope key bound to canonical.
func (s *Server) leadKeyRefusal(key, canonical string) string {
	entry, ok := s.lookupSession(key)
	switch {
	case !ok:
		return "is not a known session key"
	case entry.parent != "":
		return "carries a parent; only a parent-less lead key can re-log in"
	case entry.scope != roleLead:
		return fmt.Sprintf("is a %s key; only a lead key can re-log in", entry.scope)
	case entry.root != canonical:
		return "is bound to another root"
	}
	return ""
}

// hostedKeyRefusal is the refusal of key in a session hosted under the
// daemon-minted key hosted: it names the key to use instead.
func hostedKeyRefusal(tool, hosted, key string) error {
	return fmt.Errorf("%s: this session is hosted and its session key is fixed: use session_key %q (%s); %q is refused", tool, hosted, envHostedSessionKey, key)
}

// leadLoginResponse renders the bootstrap tool's result for key bound to
// canonical, shared by a fresh mint and a re-login so both carry the same
// fields and the same bootstrap staleness warning.
func (s *Server) leadLoginResponse(id json.RawMessage, arguments map[string]any, key, canonical string) response {
	result := map[string]any{
		"session_key": key,
		"root":        canonical,
	}
	text := fmt.Sprintf("session_key: %s\nroot: %s\n", key, canonical)
	if skillsRoot, srErr := s.skillsRoot(); srErr == nil {
		adapter := sessionConfigAdapter{s: s.sessions}
		resolver := wsconfig.NewResolver(s.configOptions(), builtinConfigDefaults(), adapter, adapter)
		warning := bootstrapStalenessWarning(canonical, skillsRoot, &resolver, "")
		if warning != "" {
			result["bootstrap_alarm"] = warning
			text += "\n" + warning + "\n"
		}
	}
	if wantsJSON(arguments) {
		return toolJSONResponse(id, result, nil)
	}
	return toolTextResponse(id, text, nil)
}

type sessionChildOutput struct {
	Key    string `json:"key"`
	Scope  string `json:"scope"`
	Parent string `json:"parent"`
	Depth  int    `json:"depth"`
	Live   bool   `json:"live"`
	Root   string `json:"root"`
	Note   string `json:"note,omitempty"`
}

func (s *Server) handleSessionChildren(id json.RawMessage, arguments map[string]any) response {
	sessionKey, _ := arguments["session_key"].(string)
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return toolTextResponse(id, "", fmt.Errorf("session.children: session_key is required"))
	}

	depth := 1
	if raw, ok := arguments["depth"]; ok {
		if f, ok := raw.(float64); ok {
			depth = int(f)
		}
	}
	includeDead, _ := arguments["include_dead"].(bool)

	scope := "any"
	if raw, exists := arguments["scope"]; exists {
		value, ok := raw.(string)
		if !ok || (value != "any" && value != "control" && value != "delegate") {
			return toolTextResponse(id, "", fmt.Errorf("session.children: scope must be control, delegate, or any"))
		}
		scope = value
	}
	unnotedOnly := false
	if raw, exists := arguments["unnoted_only"]; exists {
		value, ok := raw.(bool)
		if !ok {
			return toolTextResponse(id, "", fmt.Errorf("session.children: unnoted_only must be a boolean"))
		}
		unnotedOnly = value
	}

	children, err := s.sessions.children(sessionKey, depth)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	filtered := make([]sessionChild, 0, len(children))
	for _, child := range children {
		if !child.live && !includeDead {
			continue
		}
		if scope != "any" && sessionChildScopeLabel(child.scope) != scope {
			continue
		}
		if unnotedOnly && child.note != "" {
			continue
		}
		filtered = append(filtered, child)
	}

	out := make([]sessionChildOutput, 0, len(filtered))
	for _, child := range filtered {
		out = append(out, sessionChildOutput{
			Key:    child.key,
			Scope:  sessionChildScopeLabel(child.scope),
			Parent: child.parent,
			Depth:  child.depth,
			Live:   child.live,
			Root:   child.root,
			Note:   child.note,
		})
	}

	if wantsJSON(arguments) {
		return toolJSONResponse(id, map[string]any{
			"session_key": sessionKey,
			"depth":       depth,
			"children":    out,
		}, nil)
	}
	return toolTextResponse(id, formatSessionChildren(sessionKey, out, includeDead), nil)
}

func sessionChildScopeLabel(scope toolRole) string {
	switch scope {
	case roleLead:
		return "control"
	case roleDelegate:
		return "delegate"
	case roleLeaf:
		return "leaf"
	default:
		return string(scope)
	}
}

func formatSessionChildren(sessionKey string, children []sessionChildOutput, includeDead bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "session_key: %s\n", sessionKey)
	if len(children) == 0 {
		b.WriteString("children: none\n")
		return b.String()
	}
	b.WriteString("children:\n")
	for _, child := range children {
		indent := strings.Repeat("  ", child.Depth-1)
		fmt.Fprintf(&b, "%s- key: %s scope: %s depth: %d", indent, child.Key, child.Scope, child.Depth)
		if includeDead {
			live := "no"
			if child.Live {
				live = "yes"
			}
			fmt.Fprintf(&b, " live: %s", live)
		}
		fmt.Fprintf(&b, " root: %s", child.Root)
		if child.Note != "" {
			fmt.Fprintf(&b, " note: %s", child.Note)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// parseCapabilityScope maps the optional capability argument to a toolRole.
// An absent, nil, or "lead" capability maps to roleLead (unrestricted).
func parseCapabilityScope(raw any) toolRole {
	s, _ := raw.(string)
	switch strings.TrimSpace(s) {
	case "delegate":
		return roleDelegate
	case "leaf":
		return roleLeaf
	default:
		return roleLead
	}
}

func formatStringLines(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, "\n") + "\n"
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

// capitalizeFirst upper-cases only the first byte, matching the capitalized-
// sentence convention formatSageGate's sibling fields use. It intentionally
// leaves the shared sageReviewNonWaivableAdvisory constant itself lowercase
// (tickets_mutate.go) so its other embedding — mid-sentence inside
// readySagePostureWarning's Tip-channel text — stays grammatically correct;
// only this display site capitalizes.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func writeIndentedLines(b *strings.Builder, prefix string, lines []string) {
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fmt.Fprintf(b, "%s%s\n", prefix, strings.TrimSpace(line))
	}
}

func displayString(value any) string {
	text, _ := value.(string)
	return displayOrDash(text)
}

func displayOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func boolValue(value any) bool {
	got, _ := value.(bool)
	return got
}

func shortHash(value string) string {
	if len(value) > 7 && isHexString(value) {
		return value[:7]
	}
	return value
}

func isHexString(value string) bool {
	for _, ch := range value {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') {
			continue
		}
		return false
	}
	return value != ""
}

func (s *Server) resolveToolRoot(arguments map[string]any, meta map[string]any) (string, error) {
	// Session-key branch: highest priority. When present, the key authoritatively
	// resolves the root, bypassing every fallback in the chain below.
	if key, ok := arguments["session_key"].(string); ok && strings.TrimSpace(key) != "" {
		entry, found := s.lookupSession(key)
		if !found {
			return "", fmt.Errorf("unknown_session: session key not found; " +
				"if you are the lead, re-bootstrap your session per ws:workflow-manual with your known root and retry the call")
		}
		return entry.root, nil
	}

	return "", fmt.Errorf("mandatory_session_key: root-aware ws tools require a session_key; if you are the lead, obtain one per ws:workflow-manual and pass it")
}

// canonicalGitRoot resolves root's worktree top level with runner's git
// (a hosted server's runner carries the connection's environment).
func canonicalGitRoot(runner wsgit.Runner, root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("root is required")
	}
	output, err := runner.RunGit(context.Background(), root, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("root %q is not inside a Git worktree: %v", root, err)
	}
	canonical := strings.TrimSpace(string(output))
	if canonical == "" {
		return "", fmt.Errorf("root %q did not resolve to a Git worktree", root)
	}
	if evaluated, err := filepath.EvalSymlinks(canonical); err == nil {
		canonical = evaluated
	}
	return filepath.Clean(canonical), nil
}

func codexWorkspaceRoots(meta map[string]any) []string {
	turn, ok := meta["x-codex-turn-metadata"].(map[string]any)
	if !ok {
		return nil
	}
	rawWorkspaces, ok := turn["workspaces"].(map[string]any)
	if !ok || len(rawWorkspaces) == 0 {
		return nil
	}
	roots := make([]string, 0, len(rawWorkspaces))
	for root := range rawWorkspaces {
		if strings.TrimSpace(root) != "" {
			roots = append(roots, root)
		}
	}
	return roots
}

func (s *Server) observeHarness(source, harness string) {
	harness = normalizedHarness(harness)
	if harness == "" {
		return
	}
	s.rootMu.Lock()
	defer s.rootMu.Unlock()
	if s.sessionHarness == "" {
		s.sessionHarness = harness
		s.debugEvent("harness.detected", map[string]any{"source": source, "harness": harness})
		return
	}
	if s.sessionHarness != harness {
		s.debugEvent("harness.conflict", map[string]any{
			"source":   source,
			"current":  s.sessionHarness,
			"observed": harness,
		})
	}
}

// observePiDaemonAgents records the client's explicit model-facing opt-in.
// Missing or malformed metadata never implies support.
func (s *Server) observePiDaemonAgents(raw json.RawMessage) {
	var params struct {
		Capabilities struct {
			Experimental struct {
				PiDaemonAgents bool `json:"wsPiDaemonAgents"`
			} `json:"experimental"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		params.Capabilities.Experimental.PiDaemonAgents = false
	}
	s.rootMu.Lock()
	s.piDaemonAgents = params.Capabilities.Experimental.PiDaemonAgents
	s.rootMu.Unlock()
}

func (s *Server) clientPiDaemonAgents() bool {
	s.rootMu.RLock()
	defer s.rootMu.RUnlock()
	return s.piDaemonAgents
}

func (s *Server) currentHarness() string {
	s.rootMu.RLock()
	defer s.rootMu.RUnlock()
	return s.sessionHarness
}

// detectHarnessFromInitializeParams parses the "initialize" request's
// clientInfo.name via structured JSON first: an exact match on
// "ws-pi-bridge" identifies the Pi harness. Any other (or absent) clientInfo
// falls through to the substring-based detectHarnessFromRaw, which stays
// byte-identical for Codex/Claude detection.
func detectHarnessFromInitializeParams(raw json.RawMessage) string {
	var params struct {
		ClientInfo struct {
			Name string `json:"name"`
		} `json:"clientInfo"`
	}
	if err := json.Unmarshal(raw, &params); err == nil {
		if strings.TrimSpace(params.ClientInfo.Name) == "ws-pi-bridge" {
			return "pi"
		}
	}
	return detectHarnessFromRaw(raw)
}

func detectHarnessFromRaw(raw json.RawMessage) string {
	text := strings.ToLower(string(raw))
	if text == "" {
		return ""
	}
	if strings.Contains(text, "x-codex-turn-metadata") || strings.Contains(text, "codex") {
		return "codex"
	}
	if strings.Contains(text, "claude") || strings.Contains(text, "anthropic") {
		return "claude"
	}
	return ""
}

func detectHarnessFromMeta(meta map[string]any) string {
	if len(codexWorkspaceRoots(meta)) > 0 {
		return "codex"
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return ""
	}
	return detectHarnessFromRaw(raw)
}

func normalizedHarness(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "codex":
		return "codex"
	case "claude":
		return "claude"
	case "pi":
		return "pi"
	default:
		return ""
	}
}

func execResponse(id json.RawMessage, r execjob.Response, err error, includeStreams bool) response {
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	return toolTextResponse(id, formatExecResponse(r, includeStreams), nil)
}

func formatExecResponse(r execjob.Response, includeStreams bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "exec_key: %s\n", r.ExecKey)
	fmt.Fprintf(&b, "status: %s\n", r.Status)
	fmt.Fprintf(&b, "result_ready: %t\n", r.ResultReady)
	if r.PID != 0 {
		fmt.Fprintf(&b, "pid: %d\n", r.PID)
	}
	if r.ExitCode != 0 || r.ResultReady {
		fmt.Fprintf(&b, "exit_code: %d\n", r.ExitCode)
	}
	if r.StartedAt != "" {
		fmt.Fprintf(&b, "started_at: %s\n", r.StartedAt)
	}
	if r.UpdatedAt != "" {
		fmt.Fprintf(&b, "updated_at: %s\n", r.UpdatedAt)
	}
	if r.CompletedAt != "" {
		fmt.Fprintf(&b, "completed_at: %s\n", r.CompletedAt)
	}
	fmt.Fprintf(&b, "stdout_bytes: %d\n", r.StdoutBytes)
	fmt.Fprintf(&b, "stderr_bytes: %d\n", r.StderrBytes)
	fmt.Fprintf(&b, "combined_bytes: %d\n", r.CombinedBytes)
	if r.Error != "" {
		fmt.Fprintf(&b, "error: %s\n", r.Error)
	}
	if r.Guidance != "" {
		fmt.Fprintf(&b, "guidance: %s\n", r.Guidance)
	}
	if includeStreams && (r.Stdout != "" || r.Stderr != "") {
		b.WriteString("========== stdout ==========" + "\n")
		b.WriteString(r.Stdout)
		if r.Stdout != "" && !strings.HasSuffix(r.Stdout, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("========== stderr ==========" + "\n")
		b.WriteString(r.Stderr)
		if r.Stderr != "" && !strings.HasSuffix(r.Stderr, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func execRawTailResponse(id json.RawMessage, r execjob.RawTailResponse, err error) response {
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "exec_key: %s\n", r.ExecKey)
	fmt.Fprintf(&b, "stream: %s\n", r.Stream)
	b.WriteString("========== text ==========" + "\n")
	b.WriteString(r.Text)
	if r.Text != "" && !strings.HasSuffix(r.Text, "\n") {
		b.WriteString("\n")
	}
	return toolTextResponse(id, b.String(), nil)
}

func execRawReadResponse(id json.RawMessage, r execjob.RawReadResponse, err error) response {
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "exec_key: %s\n", r.ExecKey)
	fmt.Fprintf(&b, "stream: %s\n", r.Stream)
	fmt.Fprintf(&b, "offset: %d\n", r.Offset)
	fmt.Fprintf(&b, "next_offset: %d\n", r.NextOffset)
	fmt.Fprintf(&b, "limit: %d\n", r.Limit)
	fmt.Fprintf(&b, "size: %d\n", r.Size)
	fmt.Fprintf(&b, "eof: %t\n", r.EOF)
	b.WriteString("========== text ==========" + "\n")
	b.WriteString(r.Text)
	if r.Text != "" && !strings.HasSuffix(r.Text, "\n") {
		b.WriteString("\n")
	}
	return toolTextResponse(id, b.String(), nil)
}

func execRawGrepResponse(id json.RawMessage, r execjob.RawGrepResponse, err error) response {
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "exec_key: %s\n", r.ExecKey)
	fmt.Fprintf(&b, "stream: %s\n", r.Stream)
	fmt.Fprintf(&b, "matches: %d\n", len(r.Matches))
	fmt.Fprintf(&b, "truncated: %t\n", r.Truncated)
	b.WriteString("========== matches ==========" + "\n")
	for i, m := range r.Matches {
		if i > 0 {
			b.WriteString("--\n")
		}
		for _, line := range m.Before {
			fmt.Fprintf(&b, "%d- %s\n", m.Line, line)
		}
		fmt.Fprintf(&b, "%d: %s\n", m.Line, m.Text)
		for _, line := range m.After {
			fmt.Fprintf(&b, "%d+ %s\n", m.Line, line)
		}
	}
	return toolTextResponse(id, b.String(), nil)
}

func toolJSONResponse(id json.RawMessage, value any, err error) response {
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	return toolTextResponse(id, string(raw)+"\n", nil)
}

func toolTextResponse(id json.RawMessage, text string, err error) response {
	if err != nil {
		return toolErrorTextResponse(id, err.Error())
	}
	return response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []map[string]string{{
			"type": "text",
			"text": text,
		}},
	}}
}

func toolErrorTextResponse(id json.RawMessage, text string) response {
	return response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"isError": true,
		"content": []map[string]string{{
			"type": "text",
			"text": text,
		}},
	}}
}

func errorResponse(id json.RawMessage, code int, message string) response {
	return response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

func (s *Server) requireLeadSessionKey(toolName string, arguments map[string]any) (string, error) {
	key, _ := arguments["session_key"].(string)
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("%s: session_key is required", toolName)
	}
	entry, found := s.lookupSession(key)
	if !found {
		return "", fmt.Errorf("%s: unknown_session: session key not found; if you are the lead, re-bootstrap your session per ws:workflow-manual with your known root and retry the call", toolName)
	}
	if entry.scope != roleLead {
		return "", fmt.Errorf("%s: session_key must belong to a lead session", toolName)
	}
	return key, nil
}

// wsflowRenderEligibleStems is the exact set of prompt stems that are
// eligible for the wsflow playbook.render legacy context bridge.
// Add entries here as the spec expands the set.
var wsflowRenderEligibleStems = map[string]bool{
	"reference-discovery": true,
	"code-reviewer":       true,
}

func appendRenderContext(body string, context map[string]string) string {
	if len(context) == 0 {
		return body
	}
	keys := make([]string, 0, len(context))
	for k := range context {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString("\n\n## Render Context\n")
	for _, k := range keys {
		sb.WriteString("- ")
		sb.WriteString(k)
		sb.WriteString(": ")
		sb.WriteString(context[k])
		sb.WriteString("\n")
	}
	return body + sb.String()
}

func stringMapArgument(value any) map[string]string {
	items, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(items))
	for key, value := range items {
		if text, ok := value.(string); ok {
			out[key] = text
		}
	}
	return out
}

func int64FromArgument(value any, fallback int64) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func optString(value any) string {
	text, _ := value.(string)
	return text
}

// typedArrayRejection returns a type-accurate error when a caller supplied a
// present, non-null value for an array-typed argument that is not a JSON
// array — for example a dash-bulleted prose string, the shape a commit
// message's `## AI Context` block takes everywhere else in the workflow,
// passed where ai_context expects one bullet per array element. It returns
// nil when the value is absent, explicit null, or already an array; those
// cases fall through unchanged to stringList/stringListKeepBlank's existing
// empty-vs-absent classification. Applied per call site (git.commit's and
// git.merge's ai_context) rather than inside stringList/stringListKeepBlank
// themselves: those helpers are also shared by other array-typed args
// (updated_tickets, stems, note/todo arrays) whose mismatch shape this fix's
// evidence does not cover, so a helper-level reject would widen the blast
// radius beyond what was verified.
func typedArrayRejection(argName string, value any, present bool) error {
	if !present || value == nil {
		return nil
	}
	if _, ok := value.([]any); ok {
		return nil
	}
	return fmt.Errorf("%s must be an array of strings, one bullet per element; got %s", argName, jsonValueTypeName(value))
}

// jsonValueTypeName names a decoded JSON-RPC argument's dynamic type for a
// type-accurate rejection message. Go's encoding/json decodes into exactly
// these dynamic types for a map[string]any argument value.
func jsonValueTypeName(value any) string {
	switch value.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func stringList(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if ok && text != "" {
			out = append(out, text)
		}
	}
	return out
}

// stringListKeepBlank mirrors stringList's nil-vs-empty-array and
// type-checking behavior but keeps blank ("" and whitespace-only) string
// entries instead of dropping them. Callers that need to classify a present
// array by blankness themselves (e.g. wsgit.normalizeCommitOptions's
// TrimSpace-based emptiness check) must receive the untouched entries, or an
// exact-empty-string entry becomes indistinguishable from an entry that was
// never present at all.
func stringListKeepBlank(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func boolArgument(value any) bool {
	result, _ := value.(bool)
	return result
}

func exactIntegerArgument(arguments map[string]any, name string, fallback int) (int, error) {
	raw, ok := arguments[name]
	if !ok {
		return fallback, nil
	}
	var value float64
	switch typed := raw.(type) {
	case float64:
		value = typed
	case int:
		value = float64(typed)
	default:
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	maxInt := int(^uint(0) >> 1)
	if value >= float64(maxInt) {
		return maxInt, nil
	}
	return int(value), nil
}

func hasSpecStemArgument(arguments map[string]any) bool {
	_, ok := arguments["spec_stem"]
	return ok
}

func durationFromSeconds(value any) time.Duration {
	switch v := value.(type) {
	case float64:
		if v <= 0 {
			return 0
		}
		return time.Duration(v * float64(time.Second))
	case string:
		if v == "" {
			return 0
		}
		duration, err := time.ParseDuration(v)
		if err == nil {
			return duration
		}
		seconds, err := strconv.ParseFloat(v, 64)
		if err != nil || seconds <= 0 {
			return 0
		}
		return time.Duration(seconds * float64(time.Second))
	default:
		return 0
	}
}

func intFromArgument(value any, fallback int) int {
	switch v := value.(type) {
	case float64:
		if v <= 0 {
			return fallback
		}
		return int(v)
	case string:
		if v == "" {
			return fallback
		}
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			return fallback
		}
		return parsed
	default:
		return fallback
	}
}
