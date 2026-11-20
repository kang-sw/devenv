package wsagent

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// mcp_session.go is the hosted path's ws-mcp attachment over HTTP: a
// harness the daemon starts reaches the daemon's ws MCP route
// (wsdashboard.ProxyHTTPPath) directly, with no launcher between them.
//
//   - Token. Each process start draws a per-session token; a new start
//     voids the one the conversation's last process had, and the token is
//     void once the process ends or the conversation closes (revokeToken).
//     A held process's token is persisted in its record (Record.MCPToken),
//     with the hello's ws-mcp environment and the holder it was drawn for,
//     and restored when a later daemon rejoins the process. Worker restoration follows holder ownership validation.
//   - Context. The token resolves to the hello the stdio launcher would
//     have sent from the same harness (mcpHello): the harness's own
//     environment, the ws-mcp entry's environment (the tool allowlist, the
//     hosted package policy), the harness PID and the working directory.
//   - Credentials. The harness reads the URL and the token from its
//     environment (EnvMCPSessionURL, EnvMCPSessionToken), or takes them in
//     a protocol request (agy's session request), never its argv, so
//     neither shows in a process listing or a config file.

// Env names the backend sets on a harness attached over HTTP.
const (
	// EnvMCPSessionURL is the daemon's ws MCP route for the session.
	EnvMCPSessionURL = "WS_MCP_SESSION_URL"
	// EnvMCPSessionToken is the session's bearer token on that route.
	EnvMCPSessionToken = "WS_MCP_SESSION_TOKEN"
	// EnvAgentSessionKey identifies a worker's connection for descriptor
	// lookup only. It grants no hosted-lead authority or key substitution.
	EnvAgentSessionKey = "WS_MCP_AGENT_SESSION_KEY"
)

// mcpStartWait bounds how long a token's first resolution waits for its
// process to start: the harness connects as soon as it runs, so only a
// start that never reports (a failed launch) reaches it.
var mcpStartWait = 10 * time.Second

// MCPSession attaches a harness's ws-mcp over the daemon's HTTP route
// instead of the stdio launcher.
type MCPSession struct {
	URL, Token string
	// Started is called by the backend once the harness runs, with the
	// whole environment it was started with and its PID (0 when unknown):
	// the context the stdio launcher would have sent in its hello. A rejoin
	// calls it too.
	Started func(env []string, pid int)
	// Connected is closed once the token first resolves on the route: the
	// harness reached ws-mcp with it. A harness that drops a refused
	// server silently is checked against it.
	Connected <-chan struct{}
}

// mcpToken is one live token's state, under Runner.mu.
type mcpToken struct {
	id string
	// env is the ws-mcp entry's environment, laid over the harness's.
	env map[string]string
	cwd string
	// hello is set once ready is closed; void is closed when the token is;
	// connected is closed at the token's first resolution.
	hello     wsdashboard.ProxyHello
	ready     chan struct{}
	void      chan struct{}
	connected chan struct{}
}

// newMCPToken draws a token, or "" when no random source answers.
func newMCPToken() string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	return hex.EncodeToString(raw)
}

// attachMCP binds token (a new one when empty) to conversation id for its
// next process, voiding the one it had, and returns the session to start
// it with; nil when a new token has no endpoint to go with or none could
// be drawn. A given token is a rejoined process's, which already holds
// its URL. env is the ws-mcp entry's environment and cwd the harness's
// working directory; knownPID stands in for a PID the backend cannot report
// (a rejoined process, whose PID the record holds).
func (r *Runner) attachMCP(id, token string, env map[string]string, cwd string, knownPID int) *MCPSession {
	restore := token != ""
	if !restore {
		token = newMCPToken()
	}
	if token == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.endpoint == "" && !restore {
		return nil
	}
	r.voidMCPLocked(id)
	t := &mcpToken{id: id, env: env, cwd: cwd, ready: make(chan struct{}), void: make(chan struct{}), connected: make(chan struct{})}
	if r.mcpTokens == nil {
		r.mcpTokens, r.mcpTokenOf = map[string]*mcpToken{}, map[string]string{}
	}
	r.mcpTokens[token], r.mcpTokenOf[id] = t, token
	version := r.opts.DaemonVersion
	return &MCPSession{
		URL:       r.endpoint + wsdashboard.ProxyHTTPPath,
		Token:     token,
		Connected: t.connected,
		Started: func(harnessEnv []string, pid int) {
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.mcpTokens[token] != t {
				return
			}
			select {
			case <-t.ready:
				return
			default:
			}
			if pid <= 0 {
				pid = knownPID
			}
			t.hello = mcpHello(harnessEnv, t.env, t.cwd, pid, version)
			close(t.ready)
		},
	}
}

// voidMCPLocked voids conversation id's token, if any; r.mu is held.
func (r *Runner) voidMCPLocked(id string) {
	token, ok := r.mcpTokenOf[id]
	if !ok {
		return
	}
	if t := r.mcpTokens[token]; t != nil {
		close(t.void)
	}
	delete(r.mcpTokens, token)
	delete(r.mcpTokenOf, id)
}

// ResolveMCPToken resolves a live session token to its process's hello and
// a channel closed once the token is void (wsdashboard.AgentMCPTokenHost).
// A token whose process has not reported its start yet waits for it, up
// to mcpStartWait.
func (r *Runner) ResolveMCPToken(token string) (wsdashboard.ProxyHello, <-chan struct{}, bool) {
	if token == "" {
		return wsdashboard.ProxyHello{}, nil, false
	}
	r.mu.Lock()
	t := r.mcpTokens[token]
	r.mu.Unlock()
	if t == nil {
		return wsdashboard.ProxyHello{}, nil, false
	}
	timer := time.NewTimer(mcpStartWait)
	defer timer.Stop()
	select {
	case <-t.ready:
	case <-t.void:
		return wsdashboard.ProxyHello{}, nil, false
	case <-timer.C:
		return wsdashboard.ProxyHello{}, nil, false
	}
	select {
	case <-t.void:
		return wsdashboard.ProxyHello{}, nil, false
	default:
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case <-t.connected:
	default:
		close(t.connected)
	}
	return t.hello, t.void, true
}

// mcpHello is the hello the stdio launcher would have sent for a harness
// started with harnessEnv whose ws-mcp entry carries env: the launcher
// inherits the harness's environment, the entry's environment overrides
// it, and the launcher adds the harness PID and its runtime binary.
func mcpHello(harnessEnv []string, env map[string]string, cwd string, pid int, version string) wsdashboard.ProxyHello {
	out := make(map[string]string, len(harnessEnv)+len(env)+2)
	for _, kv := range harnessEnv {
		if name, value, ok := strings.Cut(kv, "="); ok {
			out[name] = value
		}
	}
	// The session's credentials are the harness's, not its server's.
	delete(out, EnvMCPSessionURL)
	delete(out, EnvMCPSessionToken)
	for name, value := range env {
		out[name] = value
	}
	out["WS_MCP_HARNESS_PID"] = strconv.Itoa(pid)
	if exe, err := os.Executable(); err == nil {
		out["WS_MCP_RUNTIME_BINARY"] = exe
	}
	return wsdashboard.ProxyHello{
		Protocol:    wsdashboard.ProxyProtocol,
		Mode:        wsdashboard.ProxyModeMCP,
		Cwd:         cwd,
		ProjectRoot: out["WS_MCP_PROJECT_ROOT"],
		Env:         out,
		Harness:     out["WS_MCP_HARNESS"],
		OwnerPID:    pid,
		Version:     version,
	}
}

// mcpSessionEnv is env with session's credentials, and without any a
// parent session left behind when session is nil.
func mcpSessionEnv(env []string, session *MCPSession) []string {
	set := map[string]string{EnvMCPSessionURL: "", EnvMCPSessionToken: ""}
	if session != nil {
		set[EnvMCPSessionURL], set[EnvMCPSessionToken] = session.URL, session.Token
	}
	return harnessEnv(env, nil, nil, set)
}

// forgetMCP clears the record's ws-mcp session (Record.MCPToken).
func (rec *Record) forgetMCP() {
	rec.MCPToken, rec.MCPURL, rec.MCPEnv, rec.MCPHolder = "", "", nil, ""
}

// mcpURL is the HTTP route a process started now is given, or "" when the
// runner has no endpoint.
func (r *Runner) mcpURL() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.endpoint == "" {
		return ""
	}
	return r.endpoint + wsdashboard.ProxyHTTPPath
}
