// runner_state.go holds conversation state and closed-state projection.
package wsagent

import (
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

const (
	StatusIdle    = "idle"
	StatusRunning = "running"
	StatusClosed  = "closed"
	// StatusHandedOff is a lead session handed to a terminal harness.
	StatusHandedOff = "handed_off"
)

// State is one conversation as the REST routes report it. It never carries
// the ws session key.
type State struct {
	// ID is the conversation's reply-id: its session directory's name and
	// its dashboard page (/s/<id>).
	ID      string `json:"id"`
	Profile string `json:"profile"`
	Backend string `json:"backend"`
	Root    string `json:"root"`
	// Status is idle (no turn runs), running (a turn runs), closed, or
	// handed_off (a lead session a terminal harness holds: no process runs
	// here, mail no longer resumes it, and its row is the terminal's).
	Status string `json:"status"`
	// Process reports a live backend process.
	Process bool `json:"process"`
	// Holding reports whether a finished turn is held pending child reports.
	Holding bool `json:"holding,omitempty"`
	// Hidden is dashboard sidecar state, never part of the runner record.
	Hidden    bool                         `json:"hidden,omitempty"`
	Lifecycle wsdashboard.SessionLifecycle `json:"lifecycle,omitempty"`
	Model     string                       `json:"model,omitempty"`
	Effort    string                       `json:"effort,omitempty"`
	// Observed is this conversation's own response evidence, not its settings
	// or a descendant's output. It remains available when the turn ends.
	Observed *wslive.ModelObservation `json:"observed,omitempty"`
	// Observations is an internal reconciliation window, not a REST payload.
	// Parent refresh notifications coalesce; latest-only state would lose changes.
	Observations []wslive.ModelObservation `json:"-"`
	// ContextTokens and OutputTps are the child's own latest usage; the
	// approximate streaming rate is absent when no turn runs.
	ContextTokens *float64           `json:"contextTokens,omitempty"`
	OutputTps     *float64           `json:"outputTps,omitempty"`
	Batches       []wslive.BatchView `json:"batches,omitempty"`
	// Held counts the messages waiting for the next turn.
	Held int `json:"held"`
	// Page is the conversation's dashboard page path.
	Page string `json:"page"`
	// Error combines the last launch, turn, or command error (cleared when a
	// turn starts) with current lead advisories, which clear on recovery.
	Error     string `json:"error,omitempty"`
	CreatedAt string `json:"created_at"`
	ClosedAt  string `json:"closed_at,omitempty"`
	// HandedOffAt is when the session was handed to a terminal harness;
	// set with the handed_off status alone.
	HandedOffAt string `json:"handed_off_at,omitempty"`

	// Lead marks a hosted lead session; the fields below are its own.
	Lead bool `json:"lead,omitempty"`
	// IsChild marks a spawned worker or card-bound subordinate session whose
	// model and effort are fixed at spawn (read-only on HTTP settings writes).
	IsChild bool `json:"is_child,omitempty"`
	// PermissionMode is the mode the next process launches in, and
	// ActiveMode the one the running harness last reported (it may fall
	// back from the one asked for).
	PermissionMode string `json:"permission_mode,omitempty"`
	ActiveMode     string `json:"active_mode,omitempty"`
	// Common projections are computed for both leads and workers; workers keep
	// their fixed capability loadout while permission admission remains separate.
	PermissionLevel       PermissionLevel `json:"permission_level,omitempty"`
	PermissionSource      string          `json:"permission_source,omitempty"`
	PermissionEnforcement string          `json:"permission_enforcement,omitempty"`
	PluginIsolation       string          `json:"plugin_isolation,omitempty"`
	PluginWarning         string          `json:"plugin_warning,omitempty"`
	// Approvals are the tool permission prompts waiting for an answer,
	// oldest first.
	Approvals []ApprovalRequest `json:"approvals,omitempty"`
	// BackgroundTasks counts the harness's running background tasks, as its
	// stream last reported them.
	BackgroundTasks int `json:"background_tasks,omitempty"`
	// Background and Monitors are known shell/monitor counts, absent when
	// the harness only exposes an aggregate.
	Background *int `json:"background,omitempty"`
	Monitors   *int `json:"monitors,omitempty"`
	// ActionNeeded is set while the harness reports that it waits on the
	// user with no prompt in Approvals to answer; it says what to do.
	ActionNeeded string `json:"action_needed,omitempty"`
	// Failure is the typed failure (a login needed, a model unavailable, a
	// usage limit) the harness last reported, until a turn succeeds.
	Failure *wslive.Notice `json:"failure,omitempty"`
	// MCPBroken names the harness's MCP servers whose connection broke and
	// did not come back in-stream; a restart (Restart) is the way back.
	MCPBroken []string `json:"mcp_broken,omitempty"`
	// Capabilities is the catalog profile the process's harness version
	// resolved to, demoted by its init shape; Unverified in it is the
	// dashboard's "unverified version" warning. Absent for a backend
	// without a profile and before a process reported. Never persisted.
	Capabilities *Capabilities `json:"capabilities,omitempty"`
	// CanControl reports whether the asking viewer may use the lead control
	// routes (a browser on this machine); the routes set it per request.
	CanControl bool `json:"can_control,omitempty"`
	// ResumeCommand is the shell command that resumes a handed-off lead
	// session's conversation in a terminal. It names paths of this
	// machine, so the routes answer it to a viewer with CanControl alone.
	ResumeCommand string `json:"resume_command,omitempty"`

	// Parent, Label, and Cwd are a spawned conversation's (Runner.Spawn):
	// the session its turns are reported to, its one-line name, and the
	// directory its process runs in when that is not Root.
	Parent *ParentRef `json:"parent,omitempty"`
	Label  string     `json:"label,omitempty"`
	Cwd    string     `json:"cwd,omitempty"`
	// Tier is a spawned conversation's capability tier, when its spawner
	// chose the model by one.
	Tier string `json:"tier,omitempty"`
	// ActiveMs excludes the current turn; TurnStartedAt is set only while it runs.
	ActiveMs      int64 `json:"active_ms,omitempty"`
	TurnStartedAt int64 `json:"turn_started_at,omitempty"`
}

// memoryClosedState is a presentation-only read under r.mu. Explicit Get can
// still refresh a handed-off terminal's evidence through closedState.
func (r *Runner) memoryClosedState(rec Record) State {
	if st, ok := r.closedRows[rec.ReplyID]; ok {
		st.IsChild = r.isChildRecord(rec)
		return st
	}
	st := closedState(rec)
	st.IsChild = r.isChildRecord(rec)
	return st
}

// closedState is the state of a record no loop serves: closed, or handed
// to a terminal. The caller holds r.mu.
func (r *Runner) closedState(rec Record) State {
	if rec.ClosedAt != "" {
		if st, ok := r.closedRows[rec.ReplyID]; ok {
			st.IsChild = r.isChildRecord(rec)
			return r.permissionState(st, rec, wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID))
		}
	}
	st := closedState(rec)
	st.IsChild = r.isChildRecord(rec)
	if r.opts.closedSnapshotRead != nil {
		r.opts.closedSnapshotRead(rec.ReplyID)
	}
	dir := wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID)
	if snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(dir)); err == nil {
		if records, err := wslive.ReadObserved(dir); err == nil {
			wslive.OverlayObserved(&snap, records)
			for _, rec := range records {
				if rec.AgentID == "" {
					st.Observations = append(st.Observations, rec.ModelObservation)
				}
			}
		}
		if snap.Lead.Observed != nil {
			observed := snap.Lead.Observed.Current
			st.Observed = &observed
		}
		if snap.Usage != nil && snap.Usage.Context != nil && snap.Usage.Context.Tokens != nil {
			tokens := float64(*snap.Usage.Context.Tokens)
			st.ContextTokens = &tokens
		}
	}
	if st.Status == StatusHandedOff {
		st.ResumeCommand = resumeCommand(rec, wsdashboard.SessionDir(r.opts.Dir, rec.ReplyID))
	} else {
		if r.closedRows == nil {
			r.closedRows = map[string]State{}
		}
		r.closedRows[rec.ReplyID] = st
	}
	return r.permissionState(st, rec, dir)
}

func closedState(rec Record) State {
	st := State{ID: rec.ReplyID, Profile: rec.Profile, Backend: rec.Backend, Root: rec.Root, Status: StatusClosed, Model: rec.Model, Effort: rec.Effort, Page: "/s/" + rec.ReplyID, CreatedAt: rec.CreatedAt, ClosedAt: rec.ClosedAt, Lead: rec.Lead, PermissionMode: rec.selectedPermissionMode(), Label: rec.Label, Tier: rec.Tier, ActiveMs: rec.ActiveMs}
	if rec.Lead {
		st.Lifecycle = wsdashboard.SessionPendingDeletion
		st.PluginIsolation = leadPluginIsolation(rec.PluginIsolation)
	}
	st.PluginWarning = rec.PluginWarning
	if rec.Parent != nil {
		parent := *rec.Parent
		st.Parent = &parent
	}
	if !rec.Lead {
		// A lead's Cwd is a taken-over terminal's, not a spawn choice.
		st.Cwd = rec.Cwd
	}
	if rec.ClosedAt == "" && rec.handedOff() {
		st.Status, st.HandedOffAt = StatusHandedOff, rec.HandedOffAt
	}
	return st
}
