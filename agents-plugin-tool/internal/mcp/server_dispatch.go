package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/execjob"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsindex"
	"github.com/kang-sw/devenv/internal/wsrationale"
	"github.com/kang-sw/devenv/internal/wsreview"
	"github.com/kang-sw/devenv/internal/wsstate"
)

func (s *Server) callTool(ctx context.Context, req request) (resp response) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
		Meta      map[string]any `json:"_meta"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "invalid params")
	}
	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}
	// Central mailbox piggyback wrapper (Decision: one interception point):
	// every reply this call produces gets a chance at an unread-mail badge,
	// regardless of which case below built it. See mailbox_tools.go.
	// The adapter-only intake and inbox drain are no model-facing replies, so
	// they carry no badge and cost no mailbox read on their call rate.
	if keyStr, ok := params.Arguments["session_key"].(string); ok && params.Name != dashboardIntakeTool && params.Name != dashboardInboxDrainTool {
		jsonFormat := wantsJSON(params.Arguments)
		defer func() {
			resp = s.applyMailboxPiggyback(resp, keyStr, jsonFormat)
		}()
	}
	s.observeHarness("tools.call.meta", detectHarnessFromMeta(params.Meta))
	if s.noAgentMode() && noAgentHiddenTool(params.Name) {
		return errorResponse(req.ID, -32601, fmt.Sprintf("%s agentless mode disables agent-backed tool: %s", s.runtimeNamespace(), params.Name))
	}
	if !s.toolAllowed(params.Name) {
		return errorResponse(req.ID, -32601, fmt.Sprintf("tool not available in current %s MCP profile: %s", s.runtimeNamespace(), params.Name))
	}
	// Keyed capability gate: when a session_key is present and maps to a known
	// non-lead scope, enforce roleAllowsTool for this call. Unknown session keys
	// are not rejected here; root-aware tools surface the unknown_session error
	// via resolveToolRoot. Tools that do not call resolveToolRoot (e.g.
	// runtime.read) silently ignore an unrecognised session_key.
	//
	// Lead-only tools (see isLeadOnlyTool) are additionally blocked for any
	// non-lead scoped key to prevent self-bootstrap escalation: a delegate or
	// leaf key must not be able to call the bootstrap tool and receive a
	// lead-scoped key, bypassing all capability restrictions. A KEYLESS caller
	// (no session_key) is unaffected — the normal lead bootstrap path remains
	// open.
	//
	// Note mutation sits beside the role gate, not inside roleAllowsTool,
	// because it also denies lead-scoped keys that carry a parent (a ticket
	// worker, a spawned fork): every key but a top-level lead is note
	// read-only, and its denial names the report hand-off instead of the
	// generic profile text. The whiteboard tools sit beside it for the same
	// reason: the board is the top-level lead's channel to its user, so a
	// spawned key can neither write nor read it. A marked sub-lead key alone
	// passes (subleadBoardFor): its call is rewritten onto its top-level
	// lead's key, so the write, the board refresh, and the dashboard nudge
	// all target that board, and handleWhiteboardUpdate admits status only.
	//
	// dashboard.intake alone resolves its key from the intake cache
	// (intakeSession), so its call rate reads no key.json; a removed key
	// still passes here for up to intakeRecheck, for that tool only.
	var sublead *subleadBoard
	if keyStr, ok := params.Arguments["session_key"].(string); ok && strings.TrimSpace(keyStr) != "" {
		lookup := s.lookupSession
		if params.Name == dashboardIntakeTool {
			lookup = s.intakeSession
		}
		if entry, found := lookup(keyStr); found {
			// pi.* on a Pi subagent's server is authorized by the runner's
			// capability token, not the key (pi_forward.go).
			piByToken := isPiTool(params.Name) && s.piForwards()
			if entry.scope != roleLead && !piByToken && (isLeadOnlyTool(params.Name) || !roleAllowsTool(entry.scope, params.Name)) {
				return errorResponse(req.ID, -32601, fmt.Sprintf("tool not available in current %s MCP profile: %s", s.runtimeNamespace(), params.Name))
			}
			if isNoteMutationTool(params.Name) && !entry.isTopLevelLead() {
				return errorResponse(req.ID, -32601, noteReadOnlyDenial(params.Name))
			}
			if params.Name == whiteboardLoadTool && !entry.isTopLevelLead() {
				return errorResponse(req.ID, -32601, whiteboardLeadOnlyDenial(params.Name))
			}
			if isWhiteboardTool(params.Name) && !entry.isTopLevelLead() {
				board, marked := s.subleadBoardFor(keyStr, entry)
				if !marked {
					return errorResponse(req.ID, -32601, whiteboardLeadOnlyDenial(params.Name))
				}
				sublead = &board
				onBoard := make(map[string]any, len(params.Arguments))
				for name, value := range params.Arguments {
					onBoard[name] = value
				}
				onBoard["session_key"] = board.boardKey
				params.Arguments = onBoard
			}
			// A store's support conversation passes like a top-level lead for
			// the tracker and wiki tools its profile names (its writes are
			// pinned to its store in handleTrackerUpdate, and the wiki tools
			// reach its store alone); tracker.show and tracker.ui are its own.
			if (isTrackerTool(params.Name) || isWikiTool(params.Name) || params.Name == trackerShowTool || params.Name == trackerUITool) && !entry.isTopLevelLead() && !s.trackerSupportAdmits(keyStr, entry, params.Name) {
				return errorResponse(req.ID, -32601, trackerLeadOnlyDenial(params.Name))
			}
			if entry.isTopLevelLead() {
				// Registers the session on the dashboard, or keeps it off.
				s.dashboardTouch(keyStr, entry.root, false)
				// A worktree/clone mailbox identity registers at the first
				// call that carries a root: the ferrule login, or, for a
				// server hosted again for a session already logged in, this
				// keyed call.
				s.ensureMailboxRegistered(entry.root)
				if params.Name == "config.tune" {
					defer func() {
						s.dashboardTouch(keyStr, entry.root, true)
						s.live.forgetEnabled(keyStr)
					}()
				}
			}
		}
	}

	if testPanicHook != nil {
		testPanicHook(params.Name)
	}

	// Central argument precheck: refuse a call that misses a required
	// property or carries an undeclared top-level argument before any handler
	// runs, naming every problem and the tool's own schema at once.
	if refusal, ok := s.precheckToolArguments(params.Name, params.Arguments); !ok {
		return toolErrorTextResponse(req.ID, refusal)
	}

	// Once a session's whiteboard exists, a successful write by a tool that
	// dirties one of its panels re-renders its full page (never an embed);
	// see whiteboardProviders in whiteboard_tools.go.
	// Registered before the refresh below, so it runs after it: the nudge
	// follows the rewritten page.
	if dashboardNudgingTool(params.Name) {
		defer func() {
			if key, _ := params.Arguments["session_key"].(string); key != "" && !responseIsError(resp) {
				s.dashboardNudge(key)
			}
		}()
	}
	if whiteboardDirtyingTool(params.Name) || params.Name == whiteboardLoadTool {
		defer func() { s.refreshWhiteboardAfter(params.Arguments, resp) }()
	}

	switch params.Name {
	case "runtime.read":
		info, err := runtimeInfo(s.version, s.sourceCommit)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, info, nil)
		}
		return toolTextResponse(req.ID, formatRuntimeInfo(info), nil)
	case "runtime.debug_events":
		text, err := s.debug.jsonl(intFromArgument(params.Arguments["limit"], 80))
		return toolTextResponse(req.ID, text, err)
	case "session.set_default_root":
		return toolTextResponse(req.ID, "", fmt.Errorf("session default roots were removed; if you are the lead, obtain a session_key per ws:workflow-manual and pass it"))
	case "session.get_default_root":
		return toolTextResponse(req.ID, "", fmt.Errorf("session default roots were removed; if you are the lead, obtain a session_key per ws:workflow-manual and pass it"))
	case "session.children":
		return s.handleSessionChildren(req.ID, params.Arguments)
	case "session.request_hire", "session.roster":
		return s.handleSessionHire(req.ID, params.Name, params.Arguments)
	case "agenda.set":
		return s.handleAgendaSet(req.ID, params.Arguments)
	case "agenda.clear":
		return s.handleAgendaClear(req.ID, params.Arguments)
	case "agenda.list":
		return s.handleAgendaList(req.ID, params.Arguments)
	case "git.resolve_impl_branch":
		return s.handleResolveImplBranch(req.ID, params.Arguments)
	case "tickets.acquire":
		return s.handleTicketsAcquire(req.ID, params.Arguments, params.Meta)
	case "tickets.release":
		return s.handleTicketsRelease(req.ID, params.Arguments, params.Meta)
	case "tickets.index_init":
		return s.handleTicketsIndexInit(req.ID, params.Arguments, params.Meta)
	case "route.resolve_implement":
		return s.handleEnterImplement(req.ID, params.Arguments)
	case "todo.add":
		return s.handleTodoAdd(req.ID, params.Arguments)
	case "todo.check":
		return s.handleTodoCheck(req.ID, params.Arguments)
	case "todo.erase":
		return s.handleTodoErase(req.ID, params.Arguments)
	case "todo.clear":
		return s.handleTodoClear(req.ID, params.Arguments)
	case "todo.list":
		return s.handleTodoList(req.ID, params.Arguments)
	case "todo.read":
		return s.handleTodoRead(req.ID, params.Arguments)
	case "todo.reorder":
		return s.handleTodoReorder(req.ID, params.Arguments)
	case "workflow_manual":
		return s.handleWorkflowManual(req.ID, params.Arguments)
	case "workflow_state":
		return s.handleWorkflowState(req.ID, params.Arguments)
	case bootstrapToolName:
		return s.handleLeadLogin(req.ID, params.Arguments)
	case "api.list":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		domains, err := apiListDomains(root)
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, domains, err)
		}
		return toolTextResponse(req.ID, formatStringLines(domains), err)

	case "exec.spawn":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		cmd, _ := params.Arguments["cmd"].(string)
		workingDir, _ := params.Arguments["working_dir"].(string)
		stdin, _ := params.Arguments["stdin"].(string)
		result, err := s.execStore().Launch(execjob.LaunchOptions{Root: root, WorkingDir: workingDir, Cmd: cmd, Args: stringList(params.Arguments["args"]), Env: stringMapArgument(params.Arguments["env"]), Stdin: stdin, Environ: s.environ()})
		return execResponse(req.ID, result, err, true)
	case "exec.shell":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		command, _ := params.Arguments["command"].(string)
		workingDir, _ := params.Arguments["working_dir"].(string)
		stdin, _ := params.Arguments["stdin"].(string)
		shell, _ := params.Arguments["shell"].(string)
		result, err := s.execStore().Launch(execjob.LaunchOptions{Root: root, WorkingDir: workingDir, Command: command, Shell: shell, Env: stringMapArgument(params.Arguments["env"]), Stdin: stdin, ShellMode: true, Environ: s.environ()})
		return execResponse(req.ID, result, err, true)
	case "exec.status":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		key, _ := params.Arguments["exec_key"].(string)
		result, err := s.execStore().Status(root, key)
		return execResponse(req.ID, result, err, false)
	case "exec.result":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		key, _ := params.Arguments["exec_key"].(string)
		timeout := durationFromSeconds(params.Arguments["timeout_seconds"])
		result, err := s.execStore().ResultWithContext(s.drainContext(), root, key, timeout)
		return execResponse(req.ID, result, err, true)
	case "exec.abort":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		key, _ := params.Arguments["exec_key"].(string)
		result, err := s.execStore().Abort(root, key)
		return execResponse(req.ID, result, err, false)
	case "exec.raw.tail":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		key, _ := params.Arguments["exec_key"].(string)
		stream, _ := params.Arguments["stream"].(string)
		result, err := s.execStore().Tail(root, key, stream, intFromArgument(params.Arguments["lines"], 0))
		return execRawTailResponse(req.ID, result, err)
	case "exec.raw.read":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		key, _ := params.Arguments["exec_key"].(string)
		stream, _ := params.Arguments["stream"].(string)
		result, err := s.execStore().Read(root, key, stream, int64FromArgument(params.Arguments["offset"], 0), int64FromArgument(params.Arguments["limit"], 0))
		return execRawReadResponse(req.ID, result, err)
	case "exec.raw.grep":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		key, _ := params.Arguments["exec_key"].(string)
		stream, _ := params.Arguments["stream"].(string)
		pattern, _ := params.Arguments["pattern"].(string)
		result, err := s.execStore().Grep(root, key, stream, pattern, intFromArgument(params.Arguments["before"], 0), intFromArgument(params.Arguments["after"], 0), intFromArgument(params.Arguments["max_matches"], 0), boolArgument(params.Arguments["regex"]))
		return execRawGrepResponse(req.ID, result, err)
	case "config.list":
		// Read-only combined config surface (260814 Phase 2): subsumes config.show
		// (resolved-scope View) AND config.tuning (per-key tuning knob catalog),
		// preserving BOTH contracts. Lead-only via the config.* prefix gate; a
		// keyless caller passes exactly like config.show did.
		sessionKey, _ := params.Arguments["session_key"].(string)
		sessionKey = strings.TrimSpace(sessionKey)
		rootOverride, _ := params.Arguments["root_override"].(string)
		rsrcRoot, err := s.resolveRsrcRoot(strings.TrimSpace(rootOverride))
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		// Resolve the committed repo scope's anchor from the session's canonical
		// worktree root. Keyless callers (the former config.show contract) and
		// unknown keys leave it empty, so the repo scope simply drops out — never
		// an error. Shared across every scope-sensitive path below so repo
		// resolution stays consistent between the show view and the tuning catalog.
		configOpts := s.sessionConfigOptions(sessionKey)
		// config.show path: enumerate every known override key across all scopes.
		showResolver := s.sessionResolver(sessionKey, builtinConfigDefaults())
		view, err := wsconfig.ScopedShow(&showResolver, configOpts, sessionKey)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		if err := s.resolveBucketedOverrideRows(&showResolver, sessionKey, &view); err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		// config.tuning path: project the per-key writer schema + current values,
		// with the no-agent full-ws-only cut applied per entry.
		catalogResolver := s.sessionResolver(sessionKey, s.declared.withDefaults(builtinConfigAndPromptDefaults()))
		catalog, err := buildTuningCatalog(rsrcRoot, &catalogResolver, sessionKey, s.currentHarness(), s.noAgentMode(), s.configOptions(), s.agentBackends()...)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		catalog.Knobs = append(catalog.Knobs, s.declared.knobs(&catalogResolver, sessionKey)...)
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, configListView{View: view, Knobs: catalog.Knobs, ManifestErrors: s.declared.rejected}, nil)
		}
		return toolTextResponse(req.ID, formatConfigView(view)+formatTuningCatalog(catalog)+formatManifestErrors(s.declared.rejected), nil)

	case "config.tune":
		// Generic per-key config writer (260814 Phase 2): subsumes the eight
		// per-knob writers. The concrete contract for each key is surfaced by
		// config.list; here we route to the correct write path by the resolved
		// registry entry. Lead-only for lead-authority keys (enforced below),
		// otherwise gated by the config.* prefix in roleAllowsTool.
		key, _ := params.Arguments["key"].(string)
		key = strings.TrimSpace(key)
		if key == "" {
			return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: key is required"))
		}
		entry, ok := resolveConfigEntryForKey(key)
		if !ok {
			declared, isDeclared := s.declared.lookup(key)
			if !isDeclared {
				return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: unknown config key %q; call config.list for supported keys", key))
			}
			entry = declared.entry()
		}
		// prompt.* keys fold the point id into the generic key; a bare "prompt."
		// leaves an empty point id. The removed config.prompt.set/unset guarded
		// this with "pointId must be non-empty"; restore that here so config.tune
		// never writes/clears a "prompt..<harness>" override.
		if strings.HasPrefix(entry.Key, "prompt.") && strings.TrimSpace(strings.TrimPrefix(entry.Key, "prompt.")) == "" {
			return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: pointId must be non-empty"))
		}
		// No-agent gate at the KEY level: config.tune itself is never tool-hidden,
		// so a wsflow caller could otherwise reach a full-ws-only knob through it.
		// Mirrors the outer callTool per-tool gate's message shape.
		if s.noAgentMode() && !entry.NoAgentVisible {
			return toolTextResponse(req.ID, "", fmt.Errorf("%s agentless mode disables agent-backed tool: %s", s.runtimeNamespace(), key))
		}
		// Session-key requirement varies per key (no registry field): lead-authority
		// keys need a lead-scoped key; prompt.* keys need a non-empty (any-scope)
		// key; agents.tier needs none. Matches the pre-collapse per-tool behavior.
		sessionKey, _ := params.Arguments["session_key"].(string)
		sessionKey = strings.TrimSpace(sessionKey)
		if entry.RequiresLeadAuthority {
			if _, err := s.requireLeadSessionKey("config.tune", params.Arguments); err != nil {
				return toolTextResponse(req.ID, "", err)
			}
		} else if strings.HasPrefix(entry.Key, "prompt.") {
			if sessionKey == "" {
				return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: session_key is required"))
			}
		}
		// Harness: load-bearing for harness-applicable keys (the harness-bucketed
		// prompt.*, workflow.whiteboard_enabled, workflow.tracker_enabled and workflow.profile, and agents.tier); accepted
		// but ignored (warning-only, Decision 5) for keys that do not vary by
		// harness.
		harness, _ := params.Arguments["harness"].(string)
		// Case-insensitive, matching the wsconfig/CLI paths: aliasTargetKey
		// lowercases via strings.ToLower before matching codex/claude/pi/default,
		// so a caller spelling "Claude", "CODEX", or "Default" must be accepted
		// here too, not just downstream. Lowercasing "*" is a no-op.
		harness = strings.ToLower(strings.TrimSpace(harness))
		if entry.HarnessApplicable {
			if harness == "" {
				harness = s.currentHarness()
			}
			if harnessEnum := fieldEnum(entry.SelectorFields, "harness"); len(harnessEnum) > 0 {
				if harness == "" && enumContains(harnessEnum, "default") {
					harness = "default"
				}
				if !enumContains(harnessEnum, harness) {
					return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: harness must be one of %s; got %q", strings.Join(harnessEnum, ", "), harness))
				}
			}
		}
		// Scope: parse when non-empty. agents.tier is not resolver-backed, so its
		// compound writer validates its own project/global axis; resolver-backed
		// keys lean on Resolver.Set/Unset for scope enforcement.
		scopeArg, _ := params.Arguments["scope"].(string)
		var explicitScope wsconfig.Scope
		if strings.TrimSpace(scopeArg) != "" {
			explicitScope = wsconfig.Scope(strings.TrimSpace(scopeArg))
		}
		// A declared key's default scope lives on its manifest, not in the
		// wsconfig scope registry Resolver.Set/Unset consult, so resolve it here.
		if explicitScope == "" && entry.Declared != nil {
			explicitScope = entry.Declared.DefaultScope
		}
		// A harness-bucketed key stores its value under the harness bucket
		// (harnessBucketKey); every other key stores under its own name.
		storeKey := entry.Key
		if entry.HarnessBucketed() {
			storeKey = harnessBucketKey(entry.Key, harness)
		}
		if entry.Key == "agents.tier" && explicitScope != "" && explicitScope != wsconfig.ScopeProject && explicitScope != wsconfig.ScopeGlobal {
			return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: agents.tier only supports project or global scope; got %q", explicitScope))
		}
		reset, _ := params.Arguments["reset"].(bool)
		if entry.Structured {
			text, err := s.tuneStructured(entry, params.Arguments["value"], sessionKey, explicitScope, reset)
			return toolTextResponse(req.ID, text, err)
		}
		// Writers anchor the committed repo scope at the session's worktree
		// root, so scope "repo" writes that worktree's .ws-workflow/config.json.
		writeOpts := s.sessionConfigOptions(sessionKey)
		if reset {
			if entry.ResetTool == "" {
				return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: %s does not support reset", key))
			}
			if entry.Key == "agents.tier" {
				result, err := tuneAgentsTier(s.configOptions(), params.Arguments["value"], harness, explicitScope, true)
				return toolJSONResponse(req.ID, result, err)
			}
			if rawValue, hasValue := params.Arguments["value"]; hasValue && rawValue != nil {
				v, isString := rawValue.(string)
				if !isString {
					return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: value and reset are mutually exclusive; %s reset takes no value, got %s", key, jsonValueTypeName(rawValue)))
				}
				if strings.TrimSpace(v) != "" {
					return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: value and reset are mutually exclusive"))
				}
			}
			adapter := sessionConfigAdapter{s: s.sessions}
			if strings.HasPrefix(entry.Key, "prompt.") {
				// prompt override unset (subsumes config.prompt.unset).
				pointID := strings.TrimPrefix(entry.Key, "prompt.")
				storedHarness := strings.TrimPrefix(storeKey, entry.Key+".")
				resolver := wsconfig.NewResolver(writeOpts, nil, adapter, adapter)
				if err := resolver.Unset(storeKey, wsconfig.SetOptions{ExplicitScope: explicitScope, SessionKey: sessionKey}); err != nil {
					return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: %w", err))
				}
				resolvedScope := explicitScope
				if resolvedScope == "" {
					resolvedScope = wsconfig.DefaultScope(storeKey)
				}
				return toolTextResponse(req.ID, fmt.Sprintf("prompt override cleared: %s/%s (scope: %s)\n", pointID, storedHarness, resolvedScope)+repoWriteNote(&resolver, resolvedScope), nil)
			}
			// scalar resolver-backed knob reset (subsumes the alarm/preference resets).
			// Reset means "drop the override and fall back to the builtin default" —
			// distinct from explicitly writing the builtin's current value.
			resolver := wsconfig.NewResolver(writeOpts, builtinConfigDefaults(), adapter, adapter)
			if err := resolver.Unset(storeKey, wsconfig.SetOptions{ExplicitScope: explicitScope, SessionKey: sessionKey}); err != nil {
				return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: %w", err))
			}
			// The echo reads through the session-anchored resolver so a reset
			// reports the value that now applies (a committed repo value
			// included). The reset already happened, so an echo load failure is
			// reported beside it rather than as a failed tune.
			note := repoWriteNote(&resolver, resetEchoScope(explicitScope, entry))
			resolved, _, err := s.effectiveConfigValue(entry, sessionKey)
			if err != nil {
				return toolTextResponse(req.ID, fmt.Sprintf("%s: reset [scope:%s]\nwarning: effective value unreadable: %v\n", storeKey, resetEchoScope(explicitScope, entry), err)+note, nil)
			}
			if entry.HarnessBucketed() {
				// The cleared bucket and the value that now applies to this
				// session's harness are different keys, so name both.
				return toolTextResponse(req.ID, fmt.Sprintf("%s: reset [scope:%s]\neffective %s: %s [scope:%s]\n", storeKey, resetEchoScope(explicitScope, entry), entry.Key, resolved.Value, resolved.Scope)+note, nil)
			}
			return toolTextResponse(req.ID, fmt.Sprintf("%s: %s [scope:%s]\n", entry.Key, resolved.Value, resolved.Scope)+note, nil)
		}
		// Non-reset write branch, dispatched by key family. Every key except
		// agents.tier reads a string value; reject any other JSON type here so it
		// is not silently coerced to "".
		if entry.Key != "agents.tier" {
			rawValue, hasValue := params.Arguments["value"]
			if err := tuneStringValueRejection(key, rawValue, hasValue); err != nil {
				return toolTextResponse(req.ID, "", err)
			}
		}
		adapter := sessionConfigAdapter{s: s.sessions}
		switch {
		case entry.Key == "agents.tier":
			result, err := tuneAgentsTier(s.configOptions(), params.Arguments["value"], harness, explicitScope, false)
			return toolJSONResponse(req.ID, result, err)
		case strings.HasPrefix(entry.Key, "prompt."):
			// prompt override set (subsumes config.prompt.set). The text lives in the
			// generic value argument; the harness "*" stores as "all".
			pointID := strings.TrimPrefix(entry.Key, "prompt.")
			promptText, _ := params.Arguments["value"].(string)
			if strings.TrimSpace(promptText) == "" {
				return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: prompt must be non-empty"))
			}
			storedHarness := strings.TrimPrefix(storeKey, entry.Key+".")
			resolver := wsconfig.NewResolver(writeOpts, nil, adapter, adapter)
			if err := resolver.Set(storeKey, promptText, wsconfig.SetOptions{ExplicitScope: explicitScope, SessionKey: sessionKey}); err != nil {
				return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: %w", err))
			}
			resolvedScope := explicitScope
			if resolvedScope == "" {
				resolvedScope = wsconfig.DefaultScope(storeKey)
			}
			return toolTextResponse(req.ID, fmt.Sprintf("prompt override set: %s/%s (scope: %s)\n", pointID, storedHarness, resolvedScope)+repoWriteNote(&resolver, resolvedScope), nil)
		default:
			// scalar resolver-backed knob (subagent / bootstrap_alarm).
			// Resolver.Set enforces global-only + session-key.
			value, hasValue := params.Arguments["value"].(string)
			if entry.Declared != nil {
				if !hasValue {
					return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: %s value is required unless reset is true", key))
				}
				normalized, err := entry.Declared.normalize(value)
				if err != nil {
					return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: %s value %w", key, err))
				}
				value = normalized
			} else if wsconfig.ValidPreferenceKey(entry.Key) {
				if !hasValue {
					return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: %s value is required unless reset is true", key))
				}
				// Preserve text verbatim; an explicit empty string clears the slot.
			} else if len(fieldEnum(entry.ValueFields, "value")) == 0 {
				// Free-form knob (workflow.lang): keep the caller's case. An empty
				// value would store a blank override; reset is the unset path.
				value = strings.TrimSpace(value)
				if value == "" {
					return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: %s value is required unless reset is true", key))
				}
			} else {
				value = strings.ToLower(strings.TrimSpace(value))
				if err := validateEnumValue("config.tune", entry.ValueFields, "value", value); err != nil {
					return toolTextResponse(req.ID, "", err)
				}
			}
			resolver := wsconfig.NewResolver(writeOpts, builtinConfigDefaults(), adapter, adapter)
			if err := resolver.Set(storeKey, value, wsconfig.SetOptions{ExplicitScope: explicitScope, SessionKey: sessionKey}); err != nil {
				return toolTextResponse(req.ID, "", fmt.Errorf("config.tune: %w", err))
			}
			resolvedScope := explicitScope
			if resolvedScope == "" {
				resolvedScope = entry.DefaultScope()
			}
			text := fmt.Sprintf("%s: %s [scope:%s]\n", storeKey, value, resolvedScope) + repoWriteNote(&resolver, resolvedScope)
			// A write below a set scope (for example a global write under a
			// committed repo value), or into another harness's bucket, does not
			// take effect for this session; say which value does.
			effective, answeredKey, err := s.effectiveConfigValue(entry, sessionKey)
			if err != nil {
				text += fmt.Sprintf("warning: effective value unreadable: %v\n", err)
			} else if effective.Scope != resolvedScope || answeredKey != storeKey {
				text += fmt.Sprintf("shadowed: effective %s: %s [scope:%s]\n", answeredKey, effective.Value, effective.Scope)
			}
			if entry.Key == wsconfig.ItemWorkflowProfile && value == "orchestrate" {
				text += orchestrateWhitelistWarning(harness)
			}
			return toolTextResponse(req.ID, text, nil)
		}

	case "config.get":
		// Read-only config reads for adapters: resolve each key through the
		// session-anchored resolver (session parent chain, project, repo,
		// global, builtin) without config.list's playbook scan. Callable by
		// delegate and leaf keys (roleAllowsTool) so a spawned child reads the
		// value its lead tuned at session scope.
		keys, bulk, err := configGetKeys(params.Arguments)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		sessionKey, _ := params.Arguments["session_key"].(string)
		sessionKey = strings.TrimSpace(sessionKey)
		if bulk {
			result, text := s.resolveConfigGetBulk(sessionKey, keys)
			if wantsJSON(params.Arguments) {
				return toolJSONResponse(req.ID, result, nil)
			}
			return toolTextResponse(req.ID, text, nil)
		}
		result, err := s.resolveConfigGet(sessionKey, keys[0])
		if err != nil {
			return toolTextResponse(req.ID, "", fmt.Errorf("config.get: %w", err))
		}
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, nil)
		}
		return toolTextResponse(req.ID, formatConfigGet(result), nil)

	case "config.resolve_agent":
		// Read-only tier resolution for adapters (260905 Phase 3): resolves a
		// fixed tier's {backend, model, effort} under a harness through the
		// same fallback chain playbook.render/agents.tier use, and reports
		// which aliasResolutionKeys bucket answered (resolved_from) so a
		// caller (the Pi adapter) can distinguish a harness-local hit from a
		// cross-harness fallback. No session_key required — callable early in
		// an adapter's own resolution path, before a lead session exists.
		tier, _ := params.Arguments["tier"].(string)
		harness, _ := params.Arguments["harness"].(string)
		harness = strings.ToLower(strings.TrimSpace(harness))
		if harness == "" {
			harness = s.currentHarness()
		}
		backend, model, effort, resolvedFrom, err := wsconfig.ResolveAgentTierForHarness(s.configOptions(), tier, harness)
		if err != nil {
			return toolTextResponse(req.ID, "", fmt.Errorf("config.resolve_agent: %w", err))
		}
		result := resolveAgentTierResult{Backend: backend, Model: model, Effort: effort, ResolvedFrom: resolvedFrom}
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, nil)
		}
		return toolTextResponse(req.ID, fmt.Sprintf("backend: %s\nmodel: %s\neffort: %s\nresolved_from: %s\n", backend, model, formatEffortForText(effort), resolvedFrom), nil)

	case "git.status":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		result, err := s.gitClient().Status(context.Background(), root)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		implTicket, err := activeImplTicket(root, result.Branch.Head, s.environ())
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		status := gitStatusResult{StatusResult: result, ImplTicket: implTicket}
		// The ticket index is read from the local cache only: git.status never
		// pays a network round trip for ownership.
		if view := s.loadOwnershipView(root, true); view != nil && view.overlay != nil {
			if implTicket != nil && implTicket.State == "active" {
				implTicket.Owner = view.ownership(implTicket.Stem, implTicket.Status)
			}
			status.Leases = view.callerLeases()
		}
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, status, nil)
		}
		return toolTextResponse(req.ID, formatGitStatusWithImplTicket(result, implTicket)+formatIndexStatus(implTicket, status.Leases), nil)
	case "git.diff":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		rangeValue, _ := params.Arguments["range"].(string)
		mode, _ := params.Arguments["mode"].(string)
		result, err := s.gitClient().Diff(context.Background(), root, wsgit.DiffOptions{Range: rangeValue, Mode: mode, Paths: stringList(params.Arguments["paths"])})
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, result.Output, err)
	case "git.log":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		rangeValue, _ := params.Arguments["range"].(string)
		includeBody, _ := params.Arguments["include_body"].(bool)
		result, err := s.gitClient().Log(context.Background(), root, wsgit.LogOptions{Range: rangeValue, Limit: intFromArgument(params.Arguments["limit"], 20), IncludeBody: includeBody})
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, formatGitLog(result), err)
	case "git.followups":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		result, err := wsrationale.OpenFollowups(context.Background(), s.gitRunner(), root, wsrationale.FollowupOptions{
			Range:    optString(params.Arguments["range"]),
			Category: optString(params.Arguments["category"]),
			MinLevel: optString(params.Arguments["min_level"]),
		})
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, wsrationale.FormatFollowups(result), err)
	case "rationale.query":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		opts := wsrationale.Options{
			Query:       optString(params.Arguments["query"]),
			Paths:       stringList(params.Arguments["paths"]),
			Site:        optString(params.Arguments["site"]),
			Occurrence:  intFromArgument(params.Arguments["occurrence"], 0),
			Pickaxe:     optString(params.Arguments["pickaxe"]),
			Since:       optString(params.Arguments["since"]),
			Until:       optString(params.Arguments["until"]),
			Stems:       stringList(params.Arguments["stems"]),
			ExcludeStem: optString(params.Arguments["exclude_stem"]),
			Kinds:       stringList(params.Arguments["kinds"]),
			Order:       optString(params.Arguments["order"]),
			Limit:       intFromArgument(params.Arguments["limit"], 0),
		}
		result, err := wsrationale.Query(context.Background(), s.gitRunner(), root, opts)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, wsrationale.JSON(result), nil)
		}
		return toolTextResponse(req.ID, wsrationale.FormatText(result), nil)
	case "git.merge_base":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		base, _ := params.Arguments["base"].(string)
		head, _ := params.Arguments["head"].(string)
		result, err := s.gitClient().MergeBase(context.Background(), root, base, head)
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, formatMergeBase(result), err)
	case "git.merge":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		branch, _ := params.Arguments["branch"].(string)
		target, _ := params.Arguments["target"].(string)
		releaseOverride, _ := params.Arguments["release_target_override"].(bool)
		expectedSource, _ := params.Arguments["expected_source_oid"].(string)
		expectedTarget, _ := params.Arguments["expected_target_oid"].(string)
		title, _ := params.Arguments["title"].(string)
		description, _ := params.Arguments["description"].(string)
		mergeAIContextRaw, mergeAIContextPresent := params.Arguments["ai_context"]
		if err := typedArrayRejection("ai_context", mergeAIContextRaw, mergeAIContextPresent); err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		// Resolve the worktree pool root the same way worktree.acquire does, so a
		// held-target refusal can name a parallel lead's housekeeping checkout as
		// such. A worktree-listing failure passes no pool roots (unknown pool)
		// rather than failing the merge; a config load error (a malformed
		// committed repo file) fails loud like every other repo-anchored reader.
		var mergePoolRoots []string
		mergeSizeGuard := defaultMergeSizeGuard()
		if mergeKey, ok := params.Arguments["session_key"].(string); ok && strings.TrimSpace(mergeKey) != "" {
			resolver := s.sessionResolver(mergeKey, builtinConfigDefaults())
			poolRV, poolErr := resolver.Get(mergeKey, wsconfig.ItemWorktreePool)
			if poolErr != nil {
				return toolTextResponse(req.ID, "", fmt.Errorf("git.merge: %w", poolErr))
			}
			mergeSizeGuard = resolveMergeSizeGuard(&resolver, mergeKey)
			if entries, lerr := listWorktrees(context.Background(), s.gitRunner(), root); lerr == nil && len(entries) > 0 {
				mergePoolRoots = ownedPoolRoots(poolRV.Value, entries[0].Path)
			}
		}
		result, err := mergeImplBranch(context.Background(), root, s.gitRunner(), branch, target, wsgit.CommitOptions{
			Title: title, Description: description, AIContext: stringList(mergeAIContextRaw), UpdatedTickets: stringList(params.Arguments["updated_tickets"]),
		}, implMergeAcknowledgement{ReleaseTargetOverride: releaseOverride, ExpectedSourceOID: expectedSource, ExpectedTargetOID: expectedTarget}, mergePoolRoots, mergeSizeGuard)
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, result.text(), err)
	case "worktree.acquire":
		key, err := s.requireLeadSessionKey("worktree.acquire", params.Arguments)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		entry, _ := s.lookupSession(key) // requireLeadSessionKey verified it exists and is lead
		base, _ := params.Arguments["base"].(string)
		targetBranch, _ := params.Arguments["target_branch"].(string)
		if strings.TrimSpace(base) == "" {
			return toolTextResponse(req.ID, "", fmt.Errorf("worktree.acquire: base is required"))
		}
		if _, legacy := params.Arguments["sparse_paths"]; legacy {
			return toolTextResponse(req.ID, "", fmt.Errorf("worktree.acquire: sparse_paths was removed; pass profile instead (profile %q is the ai-docs checkout sparse_paths: [\"ai-docs\"] gave)", worktreeProfileHousekeeping))
		}
		resolver := s.sessionResolver(key, builtinConfigDefaults())
		profiles, err := resolveWorktreeProfiles(&resolver)
		if err != nil {
			return toolTextResponse(req.ID, "", fmt.Errorf("worktree.acquire: %w; fix or reset that profile (config.list shows the family's value_schema) before acquiring", err))
		}
		profileName, _ := params.Arguments["profile"].(string)
		profileName = strings.TrimSpace(profileName)
		if profileName == "" {
			return toolTextResponse(req.ID, "", fmt.Errorf("worktree.acquire: profile is required; resolved profiles: %s", worktreeProfileNames(profiles)))
		}
		profile, found := findWorktreeProfile(profiles, profileName)
		if !found {
			return toolTextResponse(req.ID, "", fmt.Errorf("worktree.acquire: unknown profile %q; resolved profiles: %s", profileName, worktreeProfileNames(profiles)))
		}
		poolRV, err := resolver.Get(key, wsconfig.ItemWorktreePool)
		if err != nil {
			return toolTextResponse(req.ID, "", fmt.Errorf("worktree.acquire: %w", err))
		}
		result, err := acquireWorktree(ctx, s.gitRunner(), s.environ(), entry.root, base, targetBranch, profile, poolRV.Value, key, func(mintPath string) (string, error) {
			resolved, err := s.resolveSession(sessionAccess{mode: sessionAccessMintIdentity, root: mintPath, scope: roleLead, parent: key})
			return resolved.key, err
		})
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		staleAfter, preservedLimit := worktreeNudgeThresholds(&resolver, key)
		if nudge, err := poolNudge(context.Background(), s.gitRunner(), entry.root, result.Pool, staleAfter, preservedLimit, time.Now()); err != nil {
			result.Warnings = append(result.Warnings, "stale-pool check failed: "+err.Error())
		} else {
			result.Nudge = nudge
		}
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, nil)
		}
		return toolTextResponse(req.ID, result.text(), nil)
	case "worktree.release":
		key, err := s.requireLeadSessionKey("worktree.release", params.Arguments)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		path, _ := params.Arguments["path"].(string)
		workerKey, _ := params.Arguments["key"].(string)
		path = strings.TrimSpace(path)
		workerKey = strings.TrimSpace(workerKey)
		if path == "" && workerKey == "" {
			return toolTextResponse(req.ID, "", fmt.Errorf("worktree.release: path or key is required"))
		}
		if path == "" {
			wentry, ok := s.lookupSession(workerKey)
			if !ok {
				return toolTextResponse(req.ID, "", fmt.Errorf("worktree.release: key %q is not a known session key", workerKey))
			}
			path = wentry.root
		}
		resolver := s.sessionResolver(key, builtinConfigDefaults())
		poolRV, err := resolver.Get(key, wsconfig.ItemWorktreePool)
		if err != nil {
			return toolTextResponse(req.ID, "", fmt.Errorf("worktree.release: %w", err))
		}
		lookup := func(name string) (resolvedWorktreeProfile, bool, error) {
			profiles, err := resolveWorktreeProfiles(&resolver)
			return staticProfileLookup(profiles, err)(name)
		}
		force, _ := params.Arguments["force"].(bool)
		caller := &worktreeReleaseCaller{LeadKey: key, WorkerKey: workerKey, Force: force}
		result, err := releaseWorktreeAs(context.Background(), s.gitRunner(), s.environ(), path, poolRV.Value, lookup, caller)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, nil)
		}
		return toolTextResponse(req.ID, result.text(), nil)
	case "worktree.remove":
		key, err := s.requireLeadSessionKey("worktree.remove", params.Arguments)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		path, _ := params.Arguments["path"].(string)
		workerKey, _ := params.Arguments["key"].(string)
		path = strings.TrimSpace(path)
		workerKey = strings.TrimSpace(workerKey)
		if path == "" && workerKey == "" {
			return toolTextResponse(req.ID, "", fmt.Errorf("worktree.remove: path or key is required"))
		}
		if path == "" {
			wentry, ok := s.lookupSession(workerKey)
			if !ok {
				return toolTextResponse(req.ID, "", fmt.Errorf("worktree.remove: key %q is not a known session key", workerKey))
			}
			path = wentry.root
		}
		resolver := s.sessionResolver(key, builtinConfigDefaults())
		poolRV, err := resolver.Get(key, wsconfig.ItemWorktreePool)
		if err != nil {
			return toolTextResponse(req.ID, "", fmt.Errorf("worktree.remove: %w", err))
		}
		lookup := func(name string) (resolvedWorktreeProfile, bool, error) {
			profiles, err := resolveWorktreeProfiles(&resolver)
			return staticProfileLookup(profiles, err)(name)
		}
		staleAfter, _ := worktreeNudgeThresholds(&resolver, key)
		result, err := removeWorktree(context.Background(), s.gitRunner(), s.environ(), path, poolRV.Value, staleAfter, time.Now(), lookup)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, nil)
		}
		return toolTextResponse(req.ID, result.text(), nil)
	case "worktree.list":
		key, err := s.requireLeadSessionKey("worktree.list", params.Arguments)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		entry, _ := s.lookupSession(key) // requireLeadSessionKey verified it exists and is lead
		resolver := s.sessionResolver(key, builtinConfigDefaults())
		poolRV, err := resolver.Get(key, wsconfig.ItemWorktreePool)
		if err != nil {
			return toolTextResponse(req.ID, "", fmt.Errorf("worktree.list: %w", err))
		}
		result, err := listPoolWorktrees(context.Background(), s.gitRunner(), entry.root, poolRV.Value, s.sessions.recordMtime)
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, result.text(), err)
	case "git.commit":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		title, _ := params.Arguments["title"].(string)
		description, _ := params.Arguments["description"].(string)
		aiContextRawValue, aiContextPresent := params.Arguments["ai_context"]
		aiContextItems, aiContextIsArray := aiContextRawValue.([]any)
		aiContextRawEntryCount := -1
		aiContextRawBytes := 0
		if aiContextIsArray {
			aiContextRawEntryCount = len(aiContextItems)
			for _, item := range aiContextItems {
				if text, ok := item.(string); ok {
					aiContextRawBytes += len(text)
				}
			}
		}
		if err := typedArrayRejection("ai_context", aiContextRawValue, aiContextPresent); err != nil {
			// Emit the debug event on the reject path too — the ring buffer
			// exists specifically to diagnose ai_context arrival shape, and a
			// wrong-type call is exactly the case it needs to catch.
			// type_mismatch distinguishes it from the absent/empty/all-blank
			// events normalizeCommitOptions classifies below.
			s.debugEvent("git.commit.ai_context_received", map[string]any{
				"present":               aiContextPresent,
				"raw_entry_count":       aiContextRawEntryCount,
				"raw_bytes":             aiContextRawBytes,
				"post_trim_entry_count": 0,
				"type_mismatch":         true,
			})
			return toolTextResponse(req.ID, "", err)
		}
		// Use stringListKeepBlank, not stringList: stringList silently drops
		// exact-empty-string entries, which would make an ["" ] payload
		// indistinguishable from an empty array by the time wsgit sees it.
		// wsgit.normalizeCommitOptions is the single source of truth for
		// TrimSpace-based blank classification (shared with the CLI commit
		// path); it needs the untouched, type-checked entries to classify
		// "present but all blank" the same way regardless of whether an
		// entry is "" or whitespace-only.
		aiContext := stringListKeepBlank(aiContextRawValue)
		aiContextPostTrimCount := 0
		for _, entry := range aiContext {
			if strings.TrimSpace(entry) != "" {
				aiContextPostTrimCount++
			}
		}
		s.debugEvent("git.commit.ai_context_received", map[string]any{
			"present":               aiContextPresent,
			"raw_entry_count":       aiContextRawEntryCount,
			"raw_bytes":             aiContextRawBytes,
			"post_trim_entry_count": aiContextPostTrimCount,
		})
		// SparseScopeActive is computed here, not inside wsgit: the
		// {#260720-wsdoc-commit-boundary} rule forbids wsdoc importing wsgit,
		// not the reverse, and internal/mcp already imports both — the
		// established bridge for this shape (mirrors the Verifier injection
		// above). wsdoc.SparseCheckoutActive is the cheap gate (no index
		// enumeration), matching #260810's guardrail that the unscoped path
		// (the common case) pays no extra cost.
		expectedBranch, _ := params.Arguments["expected_branch"].(string)
		// followups is an argument-level gate (the tool is shared with workers
		// and delegates): checked before Commit stages anything, so a refused
		// call mutates nothing. resolves is open to every caller because the
		// fixing commit is usually not the root lead's.
		followups, followupsPresent, err := followupsArgument(params.Arguments)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		if followupsPresent {
			if err := s.requireRootLeadForFollowups(params.Arguments); err != nil {
				return toolTextResponse(req.ID, "", err)
			}
		}
		resolvesRaw, resolvesPresent := params.Arguments["resolves"]
		if err := typedArrayRejection("resolves", resolvesRaw, resolvesPresent); err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		resolves := stringList(resolvesRaw)
		// Resolve warnings read the history before this commit lands; they
		// never block it.
		var resolveWarnings []string
		if len(resolves) > 0 {
			warnings, werr := wsrationale.ResolveWarnings(context.Background(), s.gitRunner(), root, resolves)
			if werr != nil {
				warnings = []string{fmt.Sprintf("resolves: could not check ids against history: %v", werr)}
			}
			resolveWarnings = warnings
		}
		updatedTickets := stringList(params.Arguments["updated_tickets"])
		result, err := wsgit.Client{Runner: s.gitRunner(), Verifier: s.verifyAdapter}.Commit(context.Background(), root, wsgit.CommitOptions{
			Paths:                         stringList(params.Arguments["paths"]),
			Title:                         title,
			Description:                   description,
			AIContext:                     aiContext,
			UpdatedTickets:                updatedTickets,
			SuppressInferredTicketUpdates: updatedTickets != nil && len(updatedTickets) == 0,
			Followups:                     followups,
			Resolves:                      resolves,
			ExpectedBranch:                expectedBranch,
			SparseScopeActive:             wsdoc.SparseCheckoutActiveEnv(root, s.environ()),
		})
		if err == nil {
			result.Warnings = append(result.Warnings, resolveWarnings...)
		}
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		// Re-inject the current session's todo summary after the commit output
		// (text mode only). resolveToolRoot already required session_key, so it is
		// available in params.Arguments.
		commitText := formatGitCommit(result)
		commitKey, _ := params.Arguments["session_key"].(string)
		commitKey = strings.TrimSpace(commitKey)
		if commitKey != "" {
			if rec, ok := s.sessions.readState(commitKey, partTodos); ok && len(rec.Todos) > 0 {
				commitText += fmt.Sprintf("\n## TODO(%s reminder: update this if stale)\n%s\n", s.runtimeNamespace(), renderTodos(rec.Todos, false))
			}
		}
		commitText = appendSessionKeyTip(commitText, commitKey)
		return toolTextResponse(req.ID, commitText, err)
	case "project_tree":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		text, err := wsdoc.ProjectTreeEnv(root, s.environ())
		if err == nil {
			// Appended here rather than threaded into ProjectTree: the
			// parent-nested ticket tree is filesystem-only and project_tree has
			// no JSON mode, so the sparse-checkout-hidden-ticket count stays an
			// out-of-band annotation on the tool case rather than a tree node.
			text += ticketScopeAnnotation(root, []string{"ready", "todo", "idea"})
		}
		return toolTextResponse(req.ID, text, err)
	case "infra.read":
		name, _ := params.Arguments["name"].(string)
		text, err := wsdoc.ReadInfra(name)
		return toolTextResponse(req.ID, text, err)
	case "convention.read":
		name, _ := params.Arguments["name"].(string)
		text, err := wsdoc.ReadConvention(name)
		return toolTextResponse(req.ID, text, err)
	case "note.write":
		return s.handleNoteWrite(req.ID, params.Arguments, params.Meta)
	case "note.erase":
		return s.handleNoteErase(req.ID, params.Arguments, params.Meta)
	case "note.mute":
		return s.handleNoteMute(req.ID, params.Arguments, params.Meta)
	case "note.unmute":
		return s.handleNoteUnmute(req.ID, params.Arguments, params.Meta)
	case "note.query":
		return s.handleNoteSearch(req.ID, params.Arguments, params.Meta)
	case "whiteboard.update":
		return s.handleWhiteboardUpdate(req.ID, params.Arguments, sublead)
	case "whiteboard.query":
		return s.handleWhiteboardQuery(req.ID, params.Arguments)
	case whiteboardLoadTool:
		return s.handleWhiteboardLoad(req.ID, params.Arguments)
	case "tracker.update":
		return s.handleTrackerUpdate(ctx, req.ID, params.Arguments)
	case trackerShowTool:
		return s.handleTrackerShow(ctx, req.ID, params.Arguments)
	case trackerUITool:
		return s.handleTrackerUI(ctx, req.ID, params.Arguments)
	case gitSearchTool:
		return s.handleGitSearch(ctx, req.ID, params.Arguments)
	case "tracker.query":
		return s.handleTrackerQuery(ctx, req.ID, params.Arguments)
	case wikiQueryTool:
		return s.handleWikiQuery(ctx, req.ID, params.Arguments)
	case wikiUpdateTool:
		return s.handleWikiUpdate(ctx, req.ID, params.Arguments)
	case "tickets.query":
		if hasSpecStemArgument(params.Arguments) {
			return toolTextResponse(req.ID, "", fmt.Errorf("tickets tools use ticket_stem, not spec_stem"))
		}
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		sessionKey, _ := params.Arguments["session_key"].(string)
		query, _ := params.Arguments["query"].(string)
		ticketStem, _ := params.Arguments["ticket_stem"].(string)
		mentionsTicketStem, _ := params.Arguments["mentions_ticket_stem"].(string)
		statuses := stringList(params.Arguments["statuses"])
		// The assignee feature reads the committed ticket-assignee-aware flag and,
		// when on, the caller's git identity. Off ⇒ inert: no gate is attached and
		// the assigned_to_me omit-filter does nothing.
		assigneeAware, currentEmail, err := s.assigneeFeature(root, sessionKey)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		// A pure point-resolve call - ticket_stem set, no query text, no
		// mentions_ticket_stem filter, and no statuses override - is exactly
		// the old tickets.status shape: reuse its logic (TicketsStatus +
		// formatTickets over a single-element slice) so the object-shaped
		// JSON and not-found error survive the collapse byte-identically
		// instead of falling through to TicketsFind's array/empty-on-miss
		// discovery shape.
		if strings.TrimSpace(ticketStem) != "" && strings.TrimSpace(query) == "" && strings.TrimSpace(mentionsTicketStem) == "" && len(statuses) == 0 {
			result, err := wsdoc.TicketsStatus(root, wsdoc.TicketStatusOptions{
				Env:            s.environ(),
				TicketStem:     ticketStem,
				IncludeDone:    true,
				IncludeDropped: true,
				// This point-resolve branch is a resolution surface by
				// definition: it answers "where is this stem", so a hidden
				// ticket must be reported as hidden-but-found rather than
				// absent.
				Resolve: true,
			})
			// The single-stem point-resolve is the one chokepoint both the
			// selector path and the directly-named-ticket path pass through, so
			// the dispatch-time hard gate is computed here and only here (the
			// projection field stays absent on every discovery listing). A gate
			// computation failure fails open — it must not turn a plain "where is
			// this stem" resolve into an error — since the promotion closure is
			// the other half of the same gate.
			if err == nil && result != nil {
				if block, blockErr := s.dispatchBlock(root, *result, s.environ()); blockErr == nil {
					result.DispatchBlocked = block
				}
				// The warning token rides both projections whenever the feature is
				// on, so the point-resolve path (a directly-named ticket) warns the
				// same way the discovery listing does.
				if assigneeAware {
					gate := wsdoc.AssigneeGateFor(result.Assignee, currentEmail)
					result.AssigneeGate = &gate
				}
			}
			var ownership *ownershipView
			if err == nil && result != nil {
				ownership = s.loadOwnershipView(root, false)
				if ownership != nil && ownership.state != wsindex.ViewAbsent {
					result.Ownership = ownership.ownership(result.Stem, result.Status)
				}
			}
			if wantsJSON(params.Arguments) {
				return toolJSONResponse(req.ID, result, err)
			}
			if result == nil {
				return toolTextResponse(req.ID, "", err)
			}
			if ownership != nil {
				return toolTextResponse(req.ID, formatTicketsWithOwnership(ownership, []wsdoc.TicketInfo{*result}, false), err)
			}
			return toolTextResponse(req.ID, formatTickets([]wsdoc.TicketInfo{*result}), err)
		}
		// The explicit-stem form is a resolution query and must report a
		// hidden-but-found ticket; the free-text query form stays a discovery
		// surface, filesystem-only plus the aggregate hidden count below.
		offset, limit, err := ticketDiscoveryPagination(params.Arguments)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		resolve := strings.TrimSpace(ticketStem) != ""
		// Selector hard-skip: when the caller asks for assigned_to_me and the
		// feature is on with a known identity, omit others'-assigned tickets from
		// the discovery result entirely (assign-any and self-assigned pass). An
		// unknown identity or an off feature leaves the filter empty, so the query
		// keeps returning the whole board.
		assignedToEmail := ""
		if assigneeAware && boolArgument(params.Arguments["assigned_to_me"]) && strings.TrimSpace(currentEmail) != "" {
			assignedToEmail = currentEmail
		}
		// The ticket index overlay (nil when the project has no index): the
		// ownership filter drops others' leases and origin-closed tickets
		// before pagination, composing with assigned_to_me.
		ownership := s.loadOwnershipView(root, false)
		var excludeOwned func(wsdoc.TicketInfo) bool
		if ownership != nil && ownership.state != wsindex.ViewAbsent && boolArgument(params.Arguments["unleased_or_mine"]) {
			excludeOwned = func(t wsdoc.TicketInfo) bool {
				return !availableToCaller(ownership.ownership(t.Stem, t.Status))
			}
		}
		result, err := wsdoc.TicketsFind(root, wsdoc.TicketFindOptions{
			Env:                s.environ(),
			Statuses:           statuses,
			IncludeDone:        boolArgument(params.Arguments["include_done"]),
			IncludeDropped:     boolArgument(params.Arguments["include_dropped"]),
			Query:              query,
			TicketStem:         ticketStem,
			MentionsTicketStem: mentionsTicketStem,
			Offset:             offset,
			Limit:              limit,
			Resolve:            resolve,
			AssignedToEmail:    assignedToEmail,
			Exclude:            excludeOwned,
		})
		// Attach the warning token to every returned ticket (the omit-filter, when
		// active, has already dropped the others'-assigned ones; plain queries keep
		// them, gated but visible, for design review and other contexts).
		if err == nil && assigneeAware {
			applyAssigneeGate(result, currentEmail)
		}
		if err == nil {
			annotateOwnership(ownership, result)
		}
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		text := formatTickets(result)
		if ownership != nil {
			text = formatTicketsWithOwnership(ownership, result, true)
		}
		if !resolve {
			text += ticketScopeAnnotation(root, effectiveTicketStatuses(params.Arguments))
		}
		return toolTextResponse(req.ID, text, err)
	case "tickets.close":
		if hasSpecStemArgument(params.Arguments) {
			return toolTextResponse(req.ID, "", fmt.Errorf("tickets tools use ticket_stem, not spec_stem"))
		}
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		stem, _ := params.Arguments["stem"].(string)
		status, _ := params.Arguments["status"].(string)
		resolution, _ := params.Arguments["resolution"].(string)
		guard, err := s.guardMoveClose(root, "close", stem, params.Arguments)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		result, err := wsdoc.TicketsClose(root, s.gitRunner(), wsdoc.TicketCloseOptions{
			Env:        s.environ(),
			TicketStem: stem,
			Status:     status,
			Resolution: resolution,
			Today:      time.Now().Format("2006-01-02"),
		})
		if err != nil {
			// The error branch must not drop result: TicketsClose populates
			// PartialMutationNotice when its writes landed before the git move
			// failed, and the error's own widen-then-retry remedy would append a
			// second `## Resolution` section if the caller never learns that.
			// Same shape as tickets.move below.
			if result.PartialMutationNotice != "" {
				return toolErrorTextResponse(req.ID, err.Error()+
					"\npartial-mutation: "+result.PartialMutationNotice+
					" A retry will not find an unchanged file.")
			}
			return toolTextResponse(req.ID, "", err)
		}
		text := formatTicketMutate("closed", result)
		text += guard.text()
		text += s.indexPiggyback(root, wsindex.OpClose, strings.TrimSuffix(filepath.Base(result.NewPath), ".md"), guard.override)
		if nudge := implementCloseMergeReviewNudge(s.gitClient(), root); nudge != "" {
			text += "next_instruction: " + nudge + "\n"
		}
		if reviewNudge := wsreview.CheckpointNudgeWith(context.Background(), s.gitRunner(), root); reviewNudge != "" {
			text += "review-watermark: " + reviewNudge + "\n"
		}
		return toolTextResponse(req.ID, text, nil)
	case "review.marker":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		if boolArgument(params.Arguments["bootstrap"]) {
			_, created, err := wsreview.BootstrapWith(context.Background(), s.gitRunner(), root)
			if err != nil {
				return toolTextResponse(req.ID, "", err)
			}
			// Re-resolve via the frontier — a just-created bootstrap entry is
			// itself a clearing/floor verdict, so this is a no-behavior-change
			// simplification for the reported entry, not a special case: both
			// the created and no-op branches read the same frontier function.
			// The created/no-op wording distinction is preserved, only the
			// entry-lookup path is unified.
			entry, found, err := wsreview.Frontier(root)
			if err != nil {
				return toolTextResponse(req.ID, "", err)
			}
			if wantsJSON(params.Arguments) {
				return toolJSONResponse(req.ID, reviewMarkerJSONValue(entry, found), nil)
			}
			if !found {
				return toolTextResponse(req.ID, "review ledger has no entry yet; call review.marker(bootstrap: true) to establish a baseline\n", nil)
			}
			if created {
				return toolTextResponse(req.ID, fmt.Sprintf("bootstrapped review ledger baseline: %s..%s: %s\n", entry.Base, entry.Head, entry.Verdict), nil)
			}
			return toolTextResponse(req.ID, fmt.Sprintf("review ledger latest entry: %s..%s: %s%s\n", entry.Base, entry.Head, entry.Verdict, reviewRefSuffix(entry.Ref)), nil)
		}
		entry, found, err := wsreview.Frontier(root)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, reviewMarkerJSONValue(entry, found), nil)
		}
		if !found {
			return toolTextResponse(req.ID, "review ledger has no entry yet; call review.marker(bootstrap: true) to establish a baseline\n", nil)
		}
		return toolTextResponse(req.ID, fmt.Sprintf("review ledger latest entry: %s..%s: %s%s\n", entry.Base, entry.Head, entry.Verdict, reviewRefSuffix(entry.Ref)), nil)
	case "review.stamp":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		base, _ := params.Arguments["base"].(string)
		head, _ := params.Arguments["head"].(string)
		verdict, _ := params.Arguments["verdict"].(string)
		ref, _ := params.Arguments["ref"].(string)
		if err := wsreview.Append(root, wsreview.Entry{Base: base, Head: head, Verdict: verdict, Ref: ref}); err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		return toolTextResponse(req.ID, fmt.Sprintf("stamped review ledger: %s..%s: %s%s\n", base, head, verdict, reviewRefSuffix(ref)), nil)
	case "tickets.move":
		if hasSpecStemArgument(params.Arguments) {
			return toolTextResponse(req.ID, "", fmt.Errorf("tickets tools use ticket_stem, not spec_stem"))
		}
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		sessionKey, _ := params.Arguments["session_key"].(string)
		stem, _ := params.Arguments["stem"].(string)
		to, _ := params.Arguments["to"].(string)
		entry, _ := s.lookupSession(strings.TrimSpace(sessionKey)) // resolveToolRoot verified the key.
		checkRoot := func() error {
			// Native workers are parented; daemon-spawned workers instead have
			// parentless delegate keys. resolveToolRoot pins both to entry.root;
			// neither exemption asserts branch ownership or requires a lease.
			if entry.parent != "" || entry.scope == roleDelegate {
				return nil
			}
			return guardOccupiedRoot(context.Background(), s.gitRunner(), root)
		}
		// Run before ownership refresh or posture resolution can write.
		if err := checkRoot(); err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		sageReview, err := s.resolveSageReviewConfig(sessionKey)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		guard, err := s.guardMoveClose(root, "move", stem, params.Arguments)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		// Re-observe HEAD at the writer boundary after the intervening gates.
		// This detects observed changes, not arbitrary external Git races.
		if err := checkRoot(); err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		result, err := wsdoc.TicketsMove(root, s.gitRunner(), wsdoc.TicketMoveOptions{
			Env:        s.environ(),
			TicketStem: stem,
			To:         to,
			SageReview: sageReview,
		})
		if err != nil {
			if result.PartialMutationNotice != "" {
				return toolErrorTextResponse(req.ID, err.Error()+
					"\npartial-mutation: frontmatter was written before this call blocked; "+
					result.PartialMutationNotice+
					" a retry will not find an unchanged file.")
			}
			return toolTextResponse(req.ID, "", err)
		}
		return toolTextResponse(req.ID, formatTicketMutate("moved", result)+guard.text()+s.indexPiggyback(root, wsindex.OpRegister, strings.TrimSuffix(filepath.Base(result.NewPath), ".md"), guard.override), nil)
	case "tickets.create_empty":
		if hasSpecStemArgument(params.Arguments) {
			return toolTextResponse(req.ID, "", fmt.Errorf("tickets tools use ticket_stem, not spec_stem"))
		}
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		stem, _ := params.Arguments["stem"].(string)
		initialState, _ := params.Arguments["initial_state"].(string)
		sessionKey, _ := params.Arguments["session_key"].(string)
		sageReview, err := s.resolveSageReviewConfig(sessionKey)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		// Auto-fill the assignee at creation (the explicit ownership act). Inert
		// unless the ticket-assignee-aware flag is on; set_assignee (default true)
		// then chooses self / none / explicit emails.
		assigneeAware, currentEmail, err := s.assigneeFeature(root, sessionKey)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		rawSetAssignee, setAssigneePresent := params.Arguments["set_assignee"]
		result, err := wsdoc.TicketCreate(root, wsdoc.TicketCreateOptions{
			Env:          s.environ(),
			Stem:         stem,
			InitialState: initialState,
			SageReview:   sageReview,
			Assignee:     resolveSetAssignee(rawSetAssignee, setAssigneePresent, assigneeAware, currentEmail),
		})
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		return toolTextResponse(req.ID, strings.ReplaceAll(formatTicketCreate(result), "ws/", s.runtimeNamespace()+"/")+s.indexPiggyback(root, wsindex.OpRegister, strings.TrimSuffix(filepath.Base(result.Path), ".md"), nil), nil)
	case "tickets.template":
		typeStr, _ := params.Arguments["type"].(string)
		text, err := wsdoc.TicketTemplate(typeStr)
		return toolTextResponse(req.ID, text, err)
	case "tickets.checklist":
		typeStr, _ := params.Arguments["type"].(string)
		phaseStr, _ := params.Arguments["phase"].(string)
		text, err := wsdoc.TicketChecklist(typeStr, phaseStr)
		return toolTextResponse(req.ID, text, err)
	case "tickets.sage_gate":
		if hasSpecStemArgument(params.Arguments) {
			return toolTextResponse(req.ID, "", fmt.Errorf("tickets tools use ticket_stem, not spec_stem"))
		}
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		sessionKey, _ := params.Arguments["session_key"].(string)
		stem, _ := params.Arguments["stem"].(string)
		landing, _ := params.Arguments["landing"].(string)
		answer, _ := params.Arguments["answer"].(string)
		sageReview, err := s.resolveSageReviewConfig(sessionKey)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		result, err := wsdoc.SageGate(root, wsdoc.SageGateOptions{
			Env:        s.environ(),
			TicketStem: stem,
			Landing:    landing,
			Answer:     answer,
		}, sageReview)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		// The ask-decline path (recommended posture + answer=="no") writes and
		// persists the "skipped" posture inside wsdoc.SageGate itself and
		// stops there. This case proposes no commit at all — not an automatic
		// one, and not a suggested one: per 260725's tickets.sage_stamp
		// precedent ({#260720-wsdoc-commit-boundary}), a separate
		// canonically-titled commit for a posture flip swallows the ticket's
		// co-located real edits under a message that describes only the flip.
		// The write rides the caller's own next ordinary ws/git.commit, the
		// same guarded chokepoint every other ticket commit uses.
		return toolTextResponse(req.ID, strings.ReplaceAll(formatSageGate(result), "ws/", s.runtimeNamespace()+"/"), nil)
	case "tickets.sage_stamp":
		if hasSpecStemArgument(params.Arguments) {
			return toolTextResponse(req.ID, "", fmt.Errorf("tickets tools use ticket_stem, not spec_stem"))
		}
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		stem, _ := params.Arguments["stem"].(string)
		stage, _ := params.Arguments["stage"].(string)
		verdicts, err := parseSageVerdicts(params.Arguments["verdicts"])
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		result, err := wsdoc.SageRecord(root, wsdoc.SageRecordOptions{
			TicketStem: stem,
			Stage:      stage,
			Verdicts:   verdicts,
			Today:      time.Now().Format("2006-01-02"),
		})
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		return toolTextResponse(req.ID, strings.ReplaceAll(formatSageRecord(result), "ws/", s.runtimeNamespace()+"/")+s.indexPiggyback(root, wsindex.OpRegister, stem, nil), nil)
	case "tickets.verify":
		if hasSpecStemArgument(params.Arguments) {
			return toolTextResponse(req.ID, "", fmt.Errorf("tickets tools use ticket_stem, not spec_stem"))
		}
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		result, err := wsdoc.TicketVerifyEnv(root, stringList(params.Arguments["paths"]), s.environ())
		if wantsJSON(params.Arguments) {
			return toolJSONResponse(req.ID, result, err)
		}
		return toolTextResponse(req.ID, formatTicketVerify(result), err)
	case "path.generate":
		root, err := s.resolveToolRoot(params.Arguments, params.Meta)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		kind, _ := params.Arguments["kind"].(string)
		generated, err := wsstate.NewManager(s.stateOptions()).GeneratePaths(root, kind, stringList(params.Arguments["stems"]))
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		text := ""
		for _, path := range generated {
			text += path.Path + "\n"
		}
		return toolTextResponse(req.ID, text, nil)
	case "playbook.read":
		// Phase 2: name + context; rsrc root is call-site-overridable seam for M3.
		// Argument parsing is named/extensible (not positional) for forward-compat
		// with M3's session_key prepend.
		name, _ := params.Arguments["name"].(string)
		callerContext := stringMapArgument(params.Arguments["context"])
		// M3 forward-compat: rsrc root resolved here so M3 can pass root_override.
		rsrcRoot, err := s.resolveRsrcRoot("")
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		// Build an override lookup from the session-keyed resolver when a session_key
		// is present.
		keyStr, _ := params.Arguments["session_key"].(string)
		// Provide the render-time {{.MailboxWaitCommand}} value (concrete when the
		// key resolves an owned mailbox identity, generic otherwise) for the one
		// stem whose body substitutes it. Because it is a reserved implicit var,
		// any path that substitutes the body must supply it or substitution fails
		// with ErrUnprovidedVar — so the render dispatch below gates on the same
		// stem to keep playbook.render of this stem from failing closed.
		if name == mailboxWaitPlaybookName {
			callerContext = s.injectMailboxWaitCommand(callerContext, keyStr)
		}
		printOverrideLookup, err := buildOverrideLookup(s, keyStr)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		// Resolve workflow.lang for language-binding injection.
		printLangResolver := s.sessionResolver(keyStr, nil)
		printWorkflowLangRV, err := printLangResolver.Get(keyStr, wsconfig.ItemWorkflowLang)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		printProfile, err := s.callerPlaybookProfile(keyStr)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		body, recommendedTier, err := printPlaybookForCaller(s, strings.TrimSpace(keyStr), printProfile, rsrcRoot, name, callerContext, s.configOptions(), printWorkflowLangRV.Value, printOverrideLookup)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		// The remaining manual/discussion knob variants run after rendering;
		// ticket knob markers resolve in the shared render core.
		knobResolver := s.sessionResolver(keyStr, builtinConfigDefaults())
		whiteboardOn, err := s.whiteboardEnabled(&knobResolver, keyStr)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		trackerOn, err := s.trackerEnabled(&knobResolver, keyStr)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		// The tracker section is never a spawned key's (as workflow_manual
		// renders it); its tools refuse that key, except a store's support
		// conversation, which the tracker gate admits.
		if entry, found := s.lookupSession(keyStr); found && trackerOn && !entry.isTopLevelLead() && !s.trackerSupportReadsTracker(keyStr, entry) {
			trackerOn = false
		}
		body, err = applyKnobVariants(s, rsrcRoot, name, body, whiteboardOn, trackerOn)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		if name == whiteboardPlaybookName || name == trackerPlaybookName {
			if body, err = appendCommandSyntax(rsrcRoot, body); err != nil {
				return toolTextResponse(req.ID, "", err)
			}
		}
		return toolTextResponse(req.ID, withRecommendedTier(body, recommendedTier)+"\n", nil)

	case "playbook.render":
		// Phase 2c: name + context + root_override; child-key mint for lead callers.
		name, _ := params.Arguments["name"].(string)
		callerContext := stringMapArgument(params.Arguments["context"])
		rootOverride, _ := params.Arguments["root_override"].(string)
		rootOverride = strings.TrimSpace(rootOverride)
		tierOverride, _ := params.Arguments["tier_override"].(string)

		// Determine worktree root: root_override (when set) or session-bound root.
		var worktreeRoot string
		if rootOverride != "" {
			var err error
			rootOverride, err = canonicalGitRoot(s.gitRunner(), rootOverride)
			if err != nil {
				return toolTextResponse(req.ID, "", err)
			}
			worktreeRoot = rootOverride
		} else {
			var err error
			worktreeRoot, err = s.resolveToolRoot(params.Arguments, params.Meta)
			if err != nil {
				return toolTextResponse(req.ID, "", err)
			}
		}
		// Delegate worktrees do not contain the installed resource manifest.
		rsrcRoot, err := s.resolveRsrcRoot("")
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}

		// Lead-gate: check caller entry to determine whether to mint a child key.
		// mintRoot is empty when caller is not a lead (no mint).
		var mintRoot string
		var parentKey string
		// Override lookup is built for any present session_key (shared helper with
		// the playbook.read path); it is independent of the lead-gate.
		renderSessionKey, _ := params.Arguments["session_key"].(string)
		// Supply {{.MailboxWaitCommand}} on the render path too, gated on the one
		// stem that substitutes it: without this an off-contract
		// playbook.render of that stem fails closed with ErrUnprovidedVar (the
		// reserved implicit var is declared but unprovided). The stem is not
		// render-eligible, so this never feeds the wsflow free-text bridge.
		if name == mailboxWaitPlaybookName {
			callerContext = s.injectMailboxWaitCommand(callerContext, renderSessionKey)
		}
		renderOverrideLookup, err := buildOverrideLookup(s, renderSessionKey)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		topLevelLead := false
		if keyStr, ok := params.Arguments["session_key"].(string); ok && strings.TrimSpace(keyStr) != "" {
			capturedKey := strings.TrimSpace(keyStr)
			entry, err := s.resolveSession(sessionAccess{mode: sessionAccessLookup, key: capturedKey})
			if err != nil {
				return toolTextResponse(req.ID, "", err)
			}
			if entry.found && entry.scope == roleLead {
				mintRoot = entry.root
				if rootOverride != "" {
					mintRoot = rootOverride
				}
				parentKey = entry.key
				topLevelLead = entry.entry().isTopLevelLead()
			}
		}
		callerContext, planStem := renderPlanStem(name, callerContext, topLevelLead)

		// Resolve workflow.lang for language-binding injection.
		renderLangResolver := s.sessionResolver(renderSessionKey, nil)
		renderWorkflowLangRV, err := renderLangResolver.Get(renderSessionKey, wsconfig.ItemWorkflowLang)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		renderProfile, err := s.callerPlaybookProfile(renderSessionKey)
		if err != nil {
			return toolTextResponse(req.ID, "", err)
		}
		path, planPath, recommendedTier, err := renderPlaybookWithPlan(s, strings.TrimSpace(renderSessionKey), renderProfile, rsrcRoot, worktreeRoot, name, callerContext, s.configOptions(), mintRoot, parentKey, renderWorkflowLangRV.Value, renderOverrideLookup, tierOverride, planStem)
		text := withRecommendedRenderBinding(path, s.currentHarness(), recommendedTier, s.configOptions())
		if planPath != "" {
			text += "\n" + planPathLinePrefix + planPath
		}
		return toolTextResponse(req.ID, text+"\n", err)

	case "pi.spawn", "pi.send", "pi.list", "pi.stop", "pi.transcript", "pi.wait":
		return s.handlePiTool(ctx, req.ID, params.Name, params.Arguments)
	case "mailbox.send":
		return s.handleMailboxSend(req.ID, params.Arguments)
	case "mailbox.recv":
		return s.handleMailboxRecv(req.ID, params.Arguments)
	case "mailbox.lookup_peers":
		return s.handleMailboxLookupPeers(req.ID, params.Arguments)
	case "mailbox.history":
		return s.handleMailboxHistory(req.ID, params.Arguments)
	case dashboardIntakeTool:
		return s.handleDashboardIntake(req.ID, params.Arguments)
	case dashboardInboxDrainTool:
		return s.handleDashboardInboxDrain(req.ID, params.Arguments)

	default:
		return errorResponse(req.ID, -32602, fmt.Sprintf("unknown tool: %s", params.Name))
	}
}
