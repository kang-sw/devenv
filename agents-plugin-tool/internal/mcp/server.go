package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"slices"
	"strings"
	"sync"

	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsindex"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsnote"
)

type Server struct {
	root              string
	version           string
	sourceCommit      string
	whiteboardChanged func(cacheHome, root, key string)
	notes             *wsnote.Cache
	noteBindings      sync.Map // session keys already bound on this connection
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
	// Key-record ancestry is immutable. Cache successes only; downstream role
	// and root lookups still reject a deleted leaf, so no invalidation is needed.
	hostedDescendants sync.Map
	// descriptor is the connection's backend descriptor (tool_aliases.go),
	// resolved once under descriptorMu.
	descriptorMu       sync.Mutex
	descriptorResolved bool
	descriptor         connectionDescriptor
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
		descriptor := s.connectionDescriptor()
		listed := s.filteredTools()
		if descriptor.directTools != nil {
			listed = slices.DeleteFunc(listed, func(tool map[string]any) bool {
				name := descriptor.aliases.canonical(tool["name"].(string))
				return name != "call" && !slices.Contains(descriptor.directTools, name)
			})
		}
		return response{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": listed}}
	case "tools/call":
		return s.callTool(ctx, req)
	default:
		return errorResponse(req.ID, -32601, "method not found")
	}
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
