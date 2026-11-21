package wsagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// runCommand sends a slash command as its own turn: through the process's
// own mapping when it has one (commandRunner), else as the bare typed line,
// which a harness that reads a message starting with a slash as a command
// runs as one.
func (c *conv) runCommand(cmd wsinbox.Command) error {
	p := c.r.opts.Backends[c.rec.Backend].CompactionProtocol()
	isCompact := cmd.Name == "compact" && p.DetectsLifecycle()
	if isCompact && !c.compacting {
		c.compactEvent(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactStart})
		if p.CommandEndConfirms {
			c.apply(wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactStart, Trigger: wslive.TriggerManual})
		}
	}
	err := controlsFor(c.proc).runCommand(cmd)
	if err == nil && isCompact {
		c.armCompactWatchdog()
	}
	return err
}

// startProcess resolves the profile again (it may have changed since the
// conversation opened) and starts the backend process, resuming the
// recorded backend session.
func (c *conv) startProcess() error {
	if c.hidden {
		return conflict("the session is archived; unhide it before starting")
	}
	rec := c.record()
	rp, err := c.r.resolve(rec.Profile, rec.Root)
	if err != nil {
		return err
	}
	if rp.Definition.Backend != rec.Backend {
		return fmt.Errorf("agent profile %q now selects backend %q; this conversation runs on %q", rec.Profile, rp.Definition.Backend, rec.Backend)
	}
	if rp.Definition.Lead != rec.Lead {
		return fmt.Errorf("agent profile %q changed its lead setting; refusing to start", rec.Profile)
	}
	if rec.Lead {
		return c.startLead(rec, rp.Definition, false)
	}
	return c.startWorker(rec, rp.Definition, false)
}

func (c *conv) startWorker(rec Record, def Profile, attach bool) error {
	releaseAdmission := func() {}
	if !attach {
		releaseAdmission = c.r.holdParentAdmission(rec.Parent)
	}
	defer releaseAdmission()
	c.mcpStale = false
	c.loadCompaction()
	c.loadModelWait()
	if rec.Backend == BackendPi && !attach {
		if err := c.r.checkChildProfile(rec.Parent, rec.Profile, def); err != nil {
			return err
		}
	}
	level, source := PermissionLevel(""), ""
	if attach {
		var err error
		level, source, err = c.r.workerPermission(rec, def, c.dir)
		if err != nil {
			// A daemon replacement observes an existing held process instead of
			// orphaning it solely because its selection cannot be read.
			fmt.Fprintf(c.r.opts.Log, "agent %s: rejoin worker permission selection (advisory): %v\n", rec.ReplyID, err)
			level, source = "", ""
		}
	} else {
		// Every launch re-admits the worker's current explicit or legacy-derived
		// authority against its parent's current level (agent_tools.go).
		var err error
		if level, source, err = c.r.childAdmission(rec, def); err != nil {
			return err
		}
	}
	c.setStatus(func() {
		c.workerPermissionLevel = level
		c.workerPermissionSource = source
	})
	if !attach {
		// Emit only after reading the stored authority used for admission.
		c.permissionNudge(level, true, "")
	}
	if c.r.opts.Holders != nil && !attach {
		if !c.r.scanDone() {
			return errors.New("holder scan is not complete")
		}
		if err := c.retireHolder(); err != nil {
			return err
		}
	}
	instructions := ""
	if !attach {
		keyRoot := rec.Root
		if rec.Backend == BackendPi {
			keyRoot = rec.workDir()
		}
		if err := c.r.opts.Keys.Ensure(keyRoot, rec.WSKey); err != nil {
			// 0.49.x bound Pi child keys to Root even when Cwd differed.
			// Ensure still checks root and scope: a foreign key fails both.
			if rec.Backend != BackendPi || rec.Parent == nil || keyRoot == rec.Root {
				return fmt.Errorf("ws session key: %w", err)
			}
			if err := c.r.opts.Keys.Ensure(rec.Root, rec.WSKey); err != nil {
				return fmt.Errorf("ws session key: %w", err)
			}
		}
		if rec.SystemPrompt && c.systemPrompt == "" {
			// Read once per daemon: the text stays in memory for every
			// later start (a new segment re-injects it from there).
			raw, err := os.ReadFile(filepath.Join(c.dir, systemPromptFile))
			if err != nil {
				return fmt.Errorf("agent.spawn system prompt: %w", err)
			}
			c.systemPrompt = string(raw)
		}
		var err error
		instructions, err = composeInstructions(c.dir, rec, def, c.systemPrompt, c.r.subagentSteer(rec))
		if err != nil {
			return err
		}
	}
	spec := StartSpec{
		Root:         rec.Root,
		Dir:          c.dir,
		Profile:      def,
		Model:        rec.Model,
		Effort:       rec.Effort,
		SessionID:    rec.SessionID,
		Cwd:          rec.Cwd,
		Instructions: instructions,
		Env:          environ(),
		// The child's authority, which a backend that sandboxes its worker
		// maps itself (codexWorkerMode).
		PermissionLevel: level,
		// An agent.spawn child delegates through agent.spawn only.
		RemoveSubagents: RemovesSubagents(rec),
	}
	if attach {
		for _, steer := range rec.Steered {
			spec.RecoveredSteers = append(spec.RecoveredSteers, steer.Text)
		}
	}
	if attach && len(def.WSTools) > 0 {
		// The mapper needs the original ws tool surface, not a new bridge.
		spec.WSMCP = &MCPCommand{}
	} else if len(def.WSTools) > 0 {
		cmd, err := c.r.opts.WSMCP(rec.Root)
		if err != nil {
			return fmt.Errorf("ws-mcp command: %w", err)
		}
		c.update(func(rec *Record) { rec.PluginWarning = cmd.PluginWarning })
		if cmd.PluginWarning != "" {
			fmt.Fprintf(c.r.opts.Log, "agent %s: %s\n", rec.ReplyID, cmd.PluginWarning)
		}
		env := map[string]string{}
		for k, v := range cmd.Env {
			env[k] = v
		}
		env[EnvAllowedTools] = strings.Join(def.WSTools, ",")
		env[EnvNoDashboard] = "1"
		env[EnvPermissionLevel] = string(level)
		env[EnvAgentSessionKey] = rec.WSKey
		// The daemon may inherit a lead's mailbox identity; the agent's
		// ws-mcp must never claim that inbox's presence. Blank is inert.
		for _, name := range mailboxIdentityEnv {
			env[name] = ""
		}
		// A spawned conversation that may spawn gets this start's
		// capability token (children.go); any other gets both blank, so
		// none is inherited.
		env[EnvPiToken], env[EnvPiEndpoint] = "", ""
		if mayHoldToken(rec, def) {
			if !attach {
				token, endpoint := c.r.issueToken(rec.ReplyID)
				env[EnvPiToken], env[EnvPiEndpoint] = token, endpoint
				if c.r.opts.Holders != nil {
					c.update(func(rec *Record) { rec.Capability = token })
				}
				rec.Capability = token
			}
		}
		cmd.Env = env
		spec.WSMCP = &cmd
		if c.r.opts.Backends[rec.Backend].WorkerHTTPMCP {
			// The same HTTP tuple is persisted with the holder after launch.
			spec.MCPSession = c.r.attachMCP(rec.ReplyID, "", env, rec.workDir(), 0, rec.Backend)
		}
	}
	if c.r.opts.Holders != nil {
		spec.Holder = &holderChild{host: c.r.opts.Holders, table: &c.r.holders,
			session: holderSession{Kind: holderKindAgentWorker, Backend: rec.Backend, ReplyID: rec.ReplyID, Cwd: rec.workDir(), Capability: capabilityHash(rec.Capability)}}
		if attach {
			spec.Holder.id, spec.Holder.skipTo = rec.HolderID, rec.HolderSeq
			spec.Holder.attachReady = make(chan struct{})
		}
	}
	if attach && c.r.opts.Backends[rec.Backend].WorkerHTTPMCP {
		// Let the backend reconstruct its filtered harness environment, but
		// do not register a bearer before checkReattach validates ownership.
		spec.MCPSession = &MCPSession{Started: func(env []string, _ int) { c.workerMCPEnv = env }}
	}
	if rec.Backend == BackendPi && !attach {
		// Persist exactly the definition checked above and passed to Start,
		// after retiring the previous process. Reattachment keeps this copy.
		c.update(func(rec *Record) { rec.ResolvedDefinition = &def })
	}
	proc, err := c.startBackend(spec)
	if err != nil {
		c.r.revokeToken(rec.ReplyID)
		return err
	}
	c.setStatus(func() { c.proc = proc })
	c.events = proc.Events()
	if !attach {
		c.steerSeq, c.steerAnswers = 0, 0
	}
	c.idleAfter = def.IdleTimeout()
	if hp, ok := proc.(holderProcess); ok {
		id, pid, held := hp.HolderInfo()
		if held {
			if attach {
				c.replaying, c.seq = true, rec.HolderSeq
				// Hold runner events until ownership and downgrade reconciliation
				// finish; Scan may already be delivering buffered output.
				c.events = nil
				return nil
			}
			c.seq, c.seqDirty, c.live.ringDirty = 0, false, false
			c.update(func(rec *Record) {
				rec.HolderID, rec.HostPID, rec.HolderSeq = id, pid, 0
				rec.SteerSeq, rec.SteerAnswers, rec.SteerAnswerSeq = 0, 0, 0
				rec.HandedOffAt = c.r.opts.Now().UTC().Format(time.RFC3339)
				rec.forgetMCP()
				if s := spec.MCPSession; s != nil {
					rec.MCPToken, rec.MCPURL, rec.MCPEnv, rec.MCPHolder = s.Token, s.URL, spec.WSMCP.Env, id
				}
			})
		}
	}
	c.dirtyModel, c.dirtyEffort, c.dirtyPermission = false, false, false
	if def.ListInIndex != rec.Listed {
		c.update(func(rec *Record) { rec.Listed = def.ListInIndex })
		c.writeRegistration()
	}
	return nil
}

// startLead starts a lead conversation's process under a new session
// holder, or with attach rejoins the one the record names (a daemon
// restart). The harness loads the user's own setup; what makes it this
// session is the hosted environment: the flag, the lead key, and the path
// of the registration row whose host_pid names the one process the flag
// holds for. The row says 0 until the process exists, so a harness that
// reads it early waits rather than misreading an older PID.
func (c *conv) startLead(rec Record, def Profile, attach bool) error {
	c.loadCompaction()
	c.loadModelWait()
	level, source, err := c.r.leadPermission(rec, c.dir)
	if err != nil {
		if !attach {
			return err
		}
		// Admission checks still block new launches, but must not orphan an
		// existing held process. Rejoin observes it without applying a selection.
		fmt.Fprintf(c.r.opts.Log, "agent %s: rejoin permission selection (advisory): %v\n", rec.ReplyID, err)
	}
	if attach {
		// Root lead keys remain parentless under the existing key-store contract.
		err := c.r.opts.LeadKeys.EnsureLead(rec.Root, rec.WSKey)
		c.setStatus(func() { c.rejoinKeyPending = err != nil })
		if err != nil {
			diagnostic := "rejoin ws session key (advisory): " + err.Error()
			fmt.Fprintf(c.r.opts.Log, "agent %s: %s\n", rec.ReplyID, diagnostic)
		}
	}
	host := c.r.opts.Holders
	holder := &holderChild{host: host, table: &c.r.holders, session: holderSession{Kind: holderKindLead, ReplyID: rec.ReplyID, Cwd: rec.workDir(), Label: rec.Profile}}
	instructions := ""
	if attach {
		holder.id, holder.skipTo = rec.HolderID, rec.HolderSeq
	} else {
		// One process per session: whatever holder the record still names
		// (a stop that failed, a crash) ends before --resume runs again.
		if err := c.retireHolder(); err != nil {
			return err
		}
		if err := c.r.opts.LeadKeys.EnsureLead(rec.Root, rec.WSKey); err != nil {
			return fmt.Errorf("ws session key: %w", err)
		}
		c.setStatus(func() { c.rejoinKeyPending = false })
		if rec.Backend == BackendClaude || rec.Backend == BackendAgy || rec.Backend == BackendCodex {
			// Written on every launch, --resume included, so each process
			// carries the key and the manual in its prompt. Pi leads get
			// them from their extension, or from its bridge fallback
			// (writePiLeadBridge, LeadSpec.Manual).
			if instructions, err = composeLeadInstructions(c.dir, rec, c.leadManual(rec), c.r.subagentSteer(rec), c.r.opts.Backends[rec.Backend].PlaybookCommands); err != nil {
				return err
			}
		}
		// One step with the check that the session is still the daemon's:
		// the row written here names no process and is not stopped, so a
		// terminal login is refused from now until the process it gets has
		// ended. No row written, no process.
		c.spawning = true
		if !c.writeRegistration() {
			c.spawning = false
			if c.isForeign() {
				return errHandedOff
			}
			return errors.New("the session's registration could not be written, so no process was started")
		}
	}
	// spawned: the backend recorded its process before Start returned (LeadSpec.Spawned).
	var spawned bool
	spec := StartSpec{
		Root:         rec.Root,
		Dir:          c.dir,
		Profile:      def,
		Model:        rec.Model,
		Effort:       rec.Effort,
		SessionID:    rec.SessionID,
		Cwd:          rec.Cwd,
		SessionFile:  rec.SessionFile,
		Instructions: instructions,
		Env:          environ(),
		Holder:       holder,
		// A hosted lead delegates through agent.spawn only. A rejoin builds
		// no command line; the replayed init's shape shows whether the
		// running process was started without the tool.
		RemoveSubagents: RemovesSubagents(rec),
		Lead: &LeadSpec{
			DaemonVersion:   c.r.opts.DaemonVersion,
			PermissionMode:  rec.launchPermissionMode(),
			PluginIsolation: leadPluginIsolation(rec.PluginIsolation),
			PluginWarning: func(warning string) {
				c.update(func(rec *Record) { rec.PluginWarning = warning })
				if warning != "" {
					fmt.Fprintf(c.r.opts.Log, "agent %s: %s\n", rec.ReplyID, warning)
					// A usable package fallback is a persisted warning, not a
					// process/recovery error. Real launch errors use lastErr.
					c.changed(false)
				}
			},
			WSMCP: func() (MCPCommand, error) {
				if c.r.opts.WSMCP == nil {
					return MCPCommand{}, errors.New("bundled ws-mcp launcher unavailable")
				}
				cmd, err := c.r.opts.WSMCP(rec.workDir())
				if err != nil {
					return MCPCommand{}, err
				}
				env := map[string]string{}
				for name, value := range cmd.Env {
					env[name] = value
				}
				env[EnvAllowedTools], env[EnvPiToken], env[EnvPiEndpoint] = "", "", ""
				for _, name := range mailboxIdentityEnv {
					env[name] = ""
				}
				env[EnvHostedSession], env[EnvHostedSessionKey] = "1", rec.WSKey
				env[EnvHostedSessionRow] = wsdashboard.RegistrationPath(c.r.opts.Dir, rec.ReplyID)
				cmd.Env = env
				return cmd, nil
			},
			Env: map[string]string{
				EnvHostedSession:    "1",
				EnvHostedSessionKey: rec.WSKey,
				EnvHostedSessionRow: wsdashboard.RegistrationPath(c.r.opts.Dir, rec.ReplyID),
			},
			Manual: func() string { return c.leadManual(rec) },
			Spawned: func(holderID string, pid int) {
				spawned = true
				c.update(func(rec *Record) { rec.HolderID, rec.HostPID, rec.HolderSeq = holderID, pid, 0 })
				c.writeRegistration()
			},
		},
	}
	c.mcpStale = false
	var mcpEnv map[string]string
	switch {
	case attach && rec.MCPToken != "" && rec.MCPHolder != "" && rec.MCPHolder != rec.HolderID:
		// The process was started again since its token was drawn, by a
		// daemon that keeps the record's members without knowing them: it
		// loaded the bundle's stdio server, and the token is no one's.
		c.update(func(rec *Record) { rec.forgetMCP() })
	case attach && rec.MCPToken != "" && (rec.MCPURL != c.r.mcpURL() || rec.MCPEnv == nil):
		// The process sends its token to a route this daemon does not
		// serve (it listens on another port), or its hello is unknown:
		// its ws-mcp is gone until it restarts, at its first idle point
		// (restartIfDue).
		fmt.Fprintf(c.r.opts.Log, "agent %s: the process's ws-mcp route %s is not this daemon's; it restarts at its first idle point\n", rec.ReplyID, rec.MCPURL)
		c.mcpStale = true
	case attach && rec.MCPToken == "" && (rec.Backend == BackendAgy || rec.Backend == BackendCodex):
		// A start the last daemon did not finish: Spawned named the
		// process before its token was recorded. These HTTP-only backends
		// have no stdio fallback; restart at a known-safe idle point,
		// as for a stale route.
		fmt.Fprintf(c.r.opts.Log, "agent %s: the %s process's start did not finish; it restarts at its first known idle point\n", rec.ReplyID, rec.Backend)
		c.mcpStale = true
	case attach && rec.MCPToken != "":
		// The recorded token and hello, as the process was started with.
		spec.MCPSession = c.r.attachMCP(rec.ReplyID, rec.MCPToken, rec.MCPEnv, rec.workDir(), rec.HostPID, rec.Backend)
	case (rec.Backend == BackendClaude || rec.Backend == BackendAgy || rec.Backend == BackendCodex) && !attach:
		// The lead's ws-mcp goes over the daemon's HTTP route with a new
		// token. Without a route a Claude lead runs the same server over
		// stdio (its --mcp-config entry); an agy lead has no stdio
		// fallback. Neither loads a plugin, so a start without any ws-mcp
		// is refused by its backend.
		cmd, err := spec.Lead.WSMCP()
		if err != nil {
			fmt.Fprintf(c.r.opts.Log, "agent %s: ws-mcp unavailable, the start is refused: %v\n", rec.ReplyID, err)
			spec.Lead.WSMCPErr = err
			break
		}
		if rec.Backend == BackendClaude {
			spec.WSMCP, spec.Lead.PluginNote = &cmd, cmd.PluginWarning
		}
		if spec.MCPSession = c.r.attachMCP(rec.ReplyID, "", cmd.Env, rec.workDir(), 0, rec.Backend); spec.MCPSession != nil {
			mcpEnv = cmd.Env
		}
	}
	if source == "explicit" {
		spec.PermissionLevel = level
		if mode := nativePermissionMode(rec.Backend, level); mode != "" {
			spec.Lead.PermissionMode = mode
		}
	}
	if !attach {
		c.permissionNudge(level, false, "")
	}
	proc, err := c.startBackend(spec)
	if c.spawning {
		c.spawning = false
		// The spawn settled: its row, written below with the process on
		// success, says stopped again when there is none.
		defer func() {
			if c.proc == nil {
				c.writeRegistration()
			}
		}()
	}
	if err != nil {
		c.r.revokeToken(rec.ReplyID)
		if spawned {
			// The process the row named early is gone (the backend closed
			// it); the deferred write says stopped.
			c.update(func(rec *Record) { rec.HolderID, rec.HostPID, rec.HolderSeq = "", 0, 0 })
		}
		return err
	}
	hp, ok := proc.(holderProcess)
	id, pid, held := "", 0, false
	if ok {
		id, pid, held = hp.HolderInfo()
	}
	if !held {
		_ = proc.Close()
		c.r.revokeToken(rec.ReplyID)
		return fmt.Errorf("the %s backend did not start the process under a session holder", rec.Backend)
	}
	// A process-scoped observation cannot carry into a new process or holder
	// attach. Replay/open reports repopulate it; silence stays unknown.
	c.setStatus(func() { c.proc, c.activeMode = proc, "" })
	c.events = proc.Events()
	c.idleAfter = def.IdleTimeout()
	if attach {
		c.replaying, c.seq = true, rec.HolderSeq
		return nil
	}
	c.dirtyModel, c.dirtyEffort, c.dirtyMode, c.dirtyPermission, c.reconcileMode = false, false, false, false, false
	c.seq, c.seqDirty, c.live.ringDirty = 0, false, false
	c.update(func(rec *Record) {
		rec.HolderID, rec.HostPID, rec.HolderSeq = id, pid, 0
		rec.forgetMCP()
		if s := spec.MCPSession; s != nil {
			rec.MCPToken, rec.MCPURL, rec.MCPEnv, rec.MCPHolder = s.Token, s.URL, mcpEnv, id
		}
	})
	c.writeRegistration()
	return nil
}

// retireHolder ends the holder the record names, if it still exists, and
// forgets it; an error means its process may still run, and no new one
// starts.
func (c *conv) retireHolder() error {
	id := c.rec.HolderID
	if id == "" {
		return nil
	}
	for _, info := range c.r.opts.Holders.List().Holders {
		if info.ID != id {
			continue
		}
		if err := c.r.opts.Holders.Stop(id); err != nil && !errors.Is(err, wsholder.ErrUnknownHolder) {
			return fmt.Errorf("the session's previous process could not be ended (holder %s): %w", id, err)
		}
	}
	c.update(func(rec *Record) {
		rec.HolderID, rec.HostPID, rec.HolderSeq = "", 0, 0
		if !rec.Lead {
			rec.HandedOffAt = ""
		}
	})
	c.writeRegistration()
	return nil
}

// reattach rejoins the process that kept running under the record's holder
// while no daemon ran. It runs before the holder manager's first scan, so
// the holder's whole replay reaches the process; checkReattach follows the
// scan.
func (c *conv) reattach() {
	rec := c.record()
	def := Profile{Backend: rec.Backend, Lead: rec.Lead}
	if rec.Backend == BackendPi && rec.ResolvedDefinition != nil {
		def = *rec.ResolvedDefinition
	} else if rp, err := c.r.resolve(rec.Profile, rec.Root); err == nil && rp.Definition.Backend == rec.Backend {
		// Best-effort mapping for a legacy held process is not evidence of
		// what it launched with. Keep its child ceiling unavailable until
		// an actual startWorker launch records the checked definition.
		def = rp.Definition
	}
	var err error
	if rec.Lead {
		err = c.startLead(rec, def, true)
	} else {
		c.replaying = true
		err = c.startWorker(rec, def, true)
	}
	if err != nil {
		// The holder stays in the record: the next start ends it first.
		fmt.Fprintf(c.r.opts.Log, "agent %s: rejoin holder %s: %v\n", rec.ReplyID, rec.HolderID, err)
	}
}

// checkReattach tells a rejoined process where the holder's replay ends,
// once the holder is attached; a holder that is gone ended with its
// process while no daemon ran. Until one of the two is known the
// conversation submits nothing (replaying).
func (c *conv) checkReattach() {
	// Before the first scan an unlisted holder may be one not attached yet;
	// holdersScanned asks again after it.
	if !c.replaying || c.replayAsked || c.proc == nil || !c.r.scanDone() {
		return
	}
	hp, ok := c.proc.(holderProcess)
	if !ok {
		return
	}
	if c.workerRetiring {
		c.retireDowngradedWorker(hp)
		return
	}
	id, _, _ := hp.HolderInfo()
	listed, attached := false, false
	for _, info := range c.r.opts.Holders.List().Holders {
		if info.ID == id {
			listed, attached = true, info.Attached
			if !attached {
				continue
			}
			if !c.rec.Lead {
				var meta holderSession
				if json.Unmarshal(info.Session, &meta) != nil || !workerHolderMatches(c.rec, meta, info.Child) {
					// Never stop a holder that failed the ownership check.
					c.r.holders.reject(id)
					hp.Detach()
					c.setStatus(func() { c.proc = nil })
					c.events = nil
					c.update(func(rec *Record) {
						rec.HolderID, rec.HostPID, rec.HolderSeq = "", 0, 0
						if !rec.Lead {
							rec.HandedOffAt = ""
						}
					})
					c.recoveryFailed("held process association does not match the worker record")
					return
				}
				if c.retireDowngradedWorker(hp) {
					return
				}
				rec := c.record()
				rp, resolveErr := c.r.resolve(rec.Profile, rec.Root)
				if resolveErr != nil {
					fmt.Fprintf(c.r.opts.Log, "agent %s: rejoin permission admission (advisory; applies on next launch): %v\n", c.rec.ReplyID, resolveErr)
				} else if _, _, err := c.r.childAdmission(rec, rp.Definition); err != nil {
					fmt.Fprintf(c.r.opts.Log, "agent %s: rejoin permission admission (advisory; applies on next launch): %v\n", c.rec.ReplyID, err)
				}
				c.r.restoreToken(c.record())
				c.restoreWorkerMCP()
				c.events = c.proc.Events()
				if gated, ok := c.proc.(interface{ allowHolderReplay() }); ok {
					gated.allowHolderReplay()
				}
			}
		}
	}
	if !listed {
		c.processGone()
		return
	}
	if !attached {
		return
	}
	// The holder answers on the connection its replayed records arrive on,
	// one after the other, so the answer may wait behind records this loop
	// has to take first: the question is asked off the loop.
	c.replayAsked = true
	go func() {
		st, err := c.r.opts.Holders.Status(id)
		if err != nil {
			// The next beat asks again.
			c.do(func() { c.replayAsked = false })
			return
		}
		hp.ReplayUntil(st.FirstSeq, st.NextSeq-1)
	}()
}

// replayDone ends a rejoin: the state the holder's records rebuilt is
// written, and the conversation takes input again. A replay that lost
// records (lost: the ring dropped records, even ones an earlier daemon
// applied; or a gap during it) and showed no turn boundary after them
// cannot tell whether a turn runs: the turn is taken as running, and
// nothing is submitted until the harness ends one (wrapHold), since a
// message sent into a running turn may be refused.
func (c *conv) replayDone(lost bool) {
	lost = lost || c.replayGap
	boundary := c.replayBoundary
	c.replaying, c.replayAsked, c.replayGap, c.replayBoundary = false, false, false, false
	if !c.rec.Lead && c.rec.Parent != nil {
		if lost && c.rec.TurnOpen {
			c.wrapHold = true
			c.setStatus(func() { c.turn = true })
			c.probeWorkerRecovery()
			return
		}
		if c.rec.TurnOpen && !c.turn {
			status := TurnDone
			if c.replayTurnError != "" {
				status = TurnFailed
			}
			c.settleTurn(status, c.replayTurnError, c.turnMark)
		}
		c.unsteer()
		c.releaseHeld()
	}
	if lost {
		if boundary {
			fmt.Fprintf(c.r.opts.Log, "agent %s: records of holder %s left its ring before this daemon replayed them; the turn state is the one the records after them show\n", c.rec.ReplyID, c.rec.HolderID)
		} else {
			fmt.Fprintf(c.r.opts.Log, "agent %s: records of holder %s left its ring before this daemon replayed them, with no turn boundary after them; the turn is taken as running and nothing is submitted until the harness ends one\n", c.rec.ReplyID, c.rec.HolderID)
			c.wrapHold = true
			c.beginTurn(false)
			c.probeWrapHold()
		}
	}
	c.reconcileLeadPermission()
	c.allowAgyBypassPending()
	c.checkAction(false)
	c.live.writeSnapshot()
	c.r.notify(c.rec.ReplyID, c.rec.Root)
	// A message steered during the rejoin goes to the turn that still runs.
	c.steer()
	// A process whose ws-mcp route this daemon does not serve restarts
	// before it takes input; any other restart waits for its usual idle
	// point, as before a rejoin. A failed restart leaves no process: what
	// is held then starts one, as with no process at all.
	if c.mcpStale && c.restartIfDue() {
		return
	}
	if !c.turn {
		c.applySettings()
		c.kick()
		if !c.turn {
			c.armIdle()
		}
	}
}

// recoveryFailed settles once, ends the unrecoverable transport, and never
// launches another process on the strength of a failed probe.
func (c *conv) recoveryFailed(reason string) {
	text := "Worker recovery failed: " + reason
	c.setStatus(func() { c.turn = false; c.lastErr = text })
	if c.rec.TurnOpen {
		c.settleTurn(TurnFailed, text, c.turnMark)
	}
	c.stopProcess("")
	c.r.revokeToken(c.rec.ReplyID)
	if c.rec.Capability != "" || c.rec.MCPToken != "" {
		c.update(func(rec *Record) { rec.Capability = ""; rec.forgetMCP() })
	}
	c.replaying, c.wrapHold = false, false
}

func (c *conv) probeWorkerRecovery() {
	p := controlsFor(c.proc).recovery
	if p == nil {
		c.recoveryFailed("the held process cannot reconcile RPC messages")
		return
	}
	proc, startedAt := c.proc, c.rec.TurnStartedAt
	c.wrapTurnSeen = false
	go func() {
		running, err := p.TurnRunning()
		message, turnErr := "", ""
		var taken []string
		if err == nil && !running {
			message, turnErr, taken, err = p.recoveryReply(startedAt)
		}
		c.do(func() {
			if c.proc != proc || !c.wrapHold || c.wrapTurnSeen {
				return
			}
			if err != nil {
				c.recoveryFailed(err.Error())
				return
			}
			if running {
				return
			}
			for _, text := range taken {
				c.steerTaken(text)
			}
			// The idle RPC state and saved messages replace acknowledgements
			// lost from the bounded ring; unsteer clears any untaken queue.
			c.steerAnswers = c.steerSeq
			c.update(func(rec *Record) { rec.SteerAnswers = c.steerAnswers })
			c.reply = message
			c.wrapHold = false
			c.endTurn(turnErr, false)
		})
	}()
}

// probeWrapHold asks a harness that can tell (turnProber) whether a turn
// runs, so a rejoin's hold does not wait for a turn end an idle harness
// never sends. The ask runs off the loop, whose events it may wait behind;
// its answer comes back to the loop (wrapProbed).
func (c *conv) probeWrapHold() {
	tp := controlsFor(c.proc).turns
	if tp == nil {
		return
	}
	proc := c.proc
	c.wrapTurnSeen = false
	go func() {
		running, err := tp.TurnRunning()
		c.do(func() { c.wrapProbed(proc, running, err) })
	}()
}

// wrapProbed takes the harness's answer to probeWrapHold. An idle answer
// ends the held turn as the harness's own turn end would, submitting what
// waited; unless the hold ended already, the process is another, or a turn
// started since the ask (its start may follow the answer on the stream,
// and its end releases the hold). A running answer, or an ask that failed,
// keeps the hold until the harness ends a turn.
func (c *conv) wrapProbed(proc Process, running bool, err error) {
	if c.closed || proc != c.proc || !c.wrapHold {
		return
	}
	switch {
	case err != nil:
		fmt.Fprintf(c.r.opts.Log, "agent %s: ask the harness whether a turn runs: %v; the turn stays taken as running until the harness ends one\n", c.rec.ReplyID, err)
	case running:
		fmt.Fprintf(c.r.opts.Log, "agent %s: the harness reports a turn running; input waits for its end\n", c.rec.ReplyID)
	case c.wrapTurnSeen:
		fmt.Fprintf(c.r.opts.Log, "agent %s: the harness reports no turn running, but one started since; input waits for its end\n", c.rec.ReplyID)
	default:
		fmt.Fprintf(c.r.opts.Log, "agent %s: the harness reports no turn running; the held turn ends\n", c.rec.ReplyID)
		c.endTurn("", false)
	}
}

// lineEnd records that every event of holder record seq is applied. The
// sequence reaches the record at once when the line wrote a conversation
// entry, so a replay after a daemon restart does not write it again.
func (c *conv) lineEnd(seq uint64) {
	if seq <= c.seq {
		return
	}
	c.seq, c.seqDirty = seq, true
	if c.live.ringDirty {
		c.persistSeq()
	}
}

func (c *conv) persistSeq() {
	if !c.seqDirty {
		return
	}
	c.seqDirty, c.live.ringDirty = false, false
	c.update(func(rec *Record) { rec.HolderSeq = c.seq })
}

// composeInstructions writes the instruction file a process appends to the
// harness's own system prompt: agent.spawn's system prompt, the profile's
// instruction file or builtin text and, when the profile allows ws tools,
// the conversation's ws session key and direct parent's reply address,
// followed by steer, the native-subagent line (Runner.subagentSteer; "" for
// none). It returns "" when there is nothing to append.
func composeInstructions(dir string, rec Record, p Profile, system, steer string) (string, error) {
	var parts []string
	if text := strings.TrimSpace(system); text != "" {
		parts = append(parts, text)
	}
	if text, ok := p.BuiltinInstructions(); ok {
		parts = append(parts, strings.TrimSpace(text))
	} else if path := p.InstructionsPath(rec.Root); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("agent profile %q: read instruction file: %w", rec.Profile, err)
		}
		parts = append(parts, strings.TrimSpace(string(raw)))
	}
	if len(p.WSTools) > 0 {
		key := fmt.Sprintf("Your ws session key is `%s`. Pass it as the `session_key` argument of every ws tool call. This key is bound to worktree `%s`; if work moves to another worktree, use a key minted for that destination (from a direct spawn there or a legitimate acquisition that returns one), because this key is never rebound.", rec.WSKey, rec.Root)
		if rec.Parent != nil {
			key += fmt.Sprintf(" Your direct parent's mailbox reply address is `id:%s`; send it reports or questions with `mailbox.send`.", rec.Parent.ReplyID)
		}
		if steer != "" {
			key += " " + steer
		}
		parts = append(parts, key)
	}
	if len(parts) == 0 {
		return "", nil
	}
	path := filepath.Join(dir, "instructions.md")
	if err := writeFileAtomic(path, []byte(strings.Join(parts, "\n\n")+"\n"), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// composeLeadInstructions writes the instruction file a hosted lead (Claude,
// agy, Codex) appends to its system prompt: the session key the daemon minted for
// it and, when one rendered, its workflow manual. This prompt teaches both
// from the first turn without depending on a plugin or a tool call. The
// manual is a snapshot of this launch. steer is the native-subagent line
// (Runner.subagentSteer; "" for none). commands marks a lead whose backend
// has BackendRegistration.PlaybookCommands: it has no ws skills, so the
// prompt says what a `/lead-*` reference is, and the manual it carries is
// the only copy it is told to read.
func composeLeadInstructions(dir string, rec Record, manual, steer string, commands bool) (string, error) {
	text := fmt.Sprintf("Your ws session key is `%s`. Pass it as the `session_key` argument of every ws tool call. "+
		"This hosted session already has a key; do not mint a replacement (no ferrule call, no fresh-bootstrap key). ", rec.WSKey)
	if steer != "" {
		text += steer + " "
	}
	if commands {
		text += "This session has no ws skills: a `/<name>` reference such as `/lead-run` names a workflow playbook. " +
			"The user runs one as that dashboard command, which sends you its playbook; you run one by reading it with the ws `playbook.read` tool and following it. "
	}
	if manual = strings.TrimSpace(manual); manual != "" && commands {
		text += "The workflow manual below is binding; its session state is as of this session's start. " +
			"It stays in this prompt, compactions included, so do not read it again with the workflow_manual tool: " +
			"for fresh session state, and where the manual says to invoke `/lead-revive`, call the ws `workflow_state` tool with this key.\n\n" + manual
	} else if manual != "" {
		text += "The workflow manual below is binding; its session state is as of this session's start. " +
			"After any compaction, or when you need fresh session state, call the workflow_state tool with this key; the manual stays in this prompt.\n\n" + manual
	} else {
		text += "At session start, call the workflow_manual tool with this key and follow it; after any compaction, call workflow_state for fresh session state."
	}
	path := filepath.Join(dir, "instructions.md")
	if err := writeFileAtomic(path, []byte(text+"\n"), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// leadManual is the workflow manual rec's lead reads from its first turn
// (Options.LeadManual), or "" when the daemon supplies no renderer or the
// render failed, which is logged: the lead then starts with its key alone.
func (c *conv) leadManual(rec Record) string {
	if c.r.opts.LeadManual == nil {
		return ""
	}
	text, err := c.r.opts.LeadManual(rec.Backend, rec.workDir(), rec.WSKey)
	if err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: workflow manual not rendered, the lead's prompt carries its key alone: %v\n", rec.ReplyID, err)
		return ""
	}
	return text
}
