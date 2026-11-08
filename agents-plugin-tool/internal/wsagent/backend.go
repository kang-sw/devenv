package wsagent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// Backend starts one harness's headless process. A backend owns the
// harness's flags, its output parsing, and its init check; the runner owns
// the conversation around it (turns, held sends, the session directory).
type Backend interface {
	// Start launches a sustained process for one conversation. A non-empty
	// spec.SessionID resumes that backend session; empty starts a new one,
	// whose id arrives as an Event.SessionID.
	Start(ctx context.Context, spec StartSpec) (Process, error)
}

// StartSpec is what a backend launches a process from.
type StartSpec struct {
	// Root is the conversation's working root: the child's working
	// directory, and the root relative profile paths resolve against.
	Root string
	// Dir is the conversation's session directory, where a backend may keep
	// files it launches with (the composed instruction file is already
	// there).
	Dir     string
	Profile Profile
	// Model and Effort are the conversation's current values; empty leaves
	// the harness default.
	Model  string
	Effort string
	// SessionID is the backend session to resume; empty starts a new one.
	SessionID string
	// Instructions is the composed instruction file to append to the
	// harness's own system prompt; empty appends none.
	Instructions string
	// WSMCP is the ws-mcp server to attach, carrying the profile's ws tool
	// allowlist; nil when the profile allows no ws tool.
	WSMCP *MCPCommand
	// Env is the environment the child starts from; the backend removes the
	// harness variables an inherited parent session leaves behind.
	Env []string
}

// MCPCommand is a stdio MCP server's launch command.
type MCPCommand struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Process is one running backend process. Its methods are safe to call from
// one goroutine at a time (the runner's conversation loop).
type Process interface {
	// Events delivers the process's output in order; it is closed after the
	// process exits.
	Events() <-chan Event
	// Send submits one user turn.
	Send(text string) error
	// Interrupt asks the running turn to stop; the turn still ends with a
	// TurnEnd event.
	Interrupt() error
	// SetModel and SetEffort change the value for the next turn in-stream;
	// ErrUnsupported makes the runner restart the process on the same
	// backend session instead.
	SetModel(model string) error
	SetEffort(effort string) error
	// Close ends the process: it closes the child's input, waits a grace
	// period, then terminates and finally kills it. It returns once the
	// child has exited; Events is closed by then.
	Close() error
}

// ErrUnsupported is a Process's answer to a control it has no in-stream
// form of.
var ErrUnsupported = errors.New("not supported in-stream")

// Event is one item of a process's output. Exactly one of its parts is set,
// except that a TurnEnd may carry TurnError.
type Event struct {
	// Live is a content event for the conversation's wslive state: a reply,
	// the lead's tool batches, or usage figures. The runner applies turn
	// boundaries and user messages itself; a backend never sends
	// turn_start, turn_end, user, or reset.
	Live *wslive.Event
	// SessionID is the backend session id, once known; the runner persists
	// it for resume.
	SessionID string
	// TurnEnd ends the running turn; TurnError describes a turn that ended
	// in an error (a refusal, an interrupt).
	TurnEnd   bool
	TurnError string
	// Fatal ends the process with an error the conversation reports, such as
	// a failed init check; the process exits after it.
	Fatal error
}

// harnessEnv returns env without the variables a parent harness session
// leaves behind for its own children (session ids, sockets, plugin
// directories), keeping the user's configuration and credentials: a name
// with one of dropPrefixes is dropped unless keep lists it.
func harnessEnv(env []string, dropPrefixes []string, keep map[string]bool, set map[string]string) []string {
	out := make([]string, 0, len(env)+len(set))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if _, override := set[name]; override {
			continue
		}
		drop := false
		for _, prefix := range dropPrefixes {
			if strings.HasPrefix(name, prefix) && !keep[name] {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	for name, value := range set {
		out = append(out, name+"="+value)
	}
	return out
}

// closeGrace is how long Close waits for the child to exit on its own after
// its input closes, and then after SIGTERM, before it kills it.
var closeGrace = 3 * time.Second

// stopChild ends cmd, whose input stdin is: close stdin (a headless harness
// exits at input EOF), then SIGTERM, then SIGKILL, waiting closeGrace after
// each step. exited is closed once cmd.Wait returned.
func stopChild(cmd *exec.Cmd, closeInput func() error, exited <-chan struct{}) {
	_ = closeInput()
	select {
	case <-exited:
		return
	case <-time.After(closeGrace):
	}
	if cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
	select {
	case <-exited:
		return
	case <-time.After(closeGrace):
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	<-exited
}

// lookPath resolves a harness command; a test replaces it.
var lookPath = exec.LookPath

// environ is the environment a child starts from; a test replaces it.
var environ = os.Environ
