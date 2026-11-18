package mcp

import (
	"fmt"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wslive"
)

func tools() []map[string]any {
	toolList := []map[string]any{
		{
			"name":        "runtime.read",
			"description": "Return ws-mcp runtime metadata for compatibility checks.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"format": stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "runtime.debug_events",
			"description": "Return recent in-process MCP debug events as JSONL.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit": integerProperty("Maximum number of events to return. Defaults to 80 and is capped."),
				},
			},
		},
		{
			"name":        "session.children",
			"description": "Return the descendant session-key subtree under a lead session key. Defaults to immediate live children; use depth 0 for the full subtree.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key":  stringProperty("Caller's lead session key whose descendants should be enumerated."),
					"depth":        integerProperty("Maximum descendant depth to return. Defaults to 1; 0 returns the full subtree."),
					"include_dead": boolProperty("Include keys whose bound root path no longer exists. Defaults to false."),
					"scope":        enumStringProperty("Filter descendants by scope. Defaults to any (all scopes).", []string{"control", "delegate", "any"}),
					"unnoted_only": boolProperty("Return only descendants carrying no note. Defaults to false."),
					"format":       stringProperty(`Optional output format. Use "json" for structured output.`),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        bootstrapToolName,
			"description": "Reserved workflow primitive. See ws:workflow-manual before use.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":               stringProperty("Absolute Git worktree root to bind to the session key."),
					"capability":         enumStringProperty(`Optional capability scope. Omit or pass "lead" for unrestricted access. Pass "delegate" or "leaf" to restrict the key to that role's allowed tools.`, []string{"lead", "delegate", "leaf"}),
					"parent_session_key": stringProperty("Optional parent session key to record coordination lineage. See ws:workflow-manual."),
					"format":             stringProperty(`Optional output format. Use "json" for structured output.`),
				},
				"required": []string{"root"},
			},
		},
		{
			"name":        "agenda.set",
			"description": "Upsert a session-level agenda blob under a key. Agenda blobs hold mode context ('what are we doing and why') and are reminded at workflow-manual load. Session-scoped: clear a blob when its work ends; a fact a future session needs belongs in note.write. Freeform fallback for cases not covered by a typed enter.* tool. Pass merge: true to patch the stored blob (RFC 7396 JSON Merge Patch) instead of replacing it, so a small change does not re-send the whole blob.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"key":         stringProperty("Agenda blob name to upsert."),
					"value":       objectProperty("Arbitrary JSON object stored under key, or the patch when merge is true."),
					"merge":       boolProperty("When true, apply value to the stored blob as an RFC 7396 JSON Merge Patch: a field set to null is removed, an object field merges recursively, and any other value (array, string, number, boolean) replaces the field whole. A missing key or a non-object stored blob is patched as {}. value must be an object. A field cannot be set to a literal null this way; use agenda.clear to remove the whole key. Absent or false replaces the blob."),
				},
				"required": []string{"session_key", "key", "value"},
			},
		},
		{
			"name":        "agenda.clear",
			"description": "Remove the session-level agenda blob stored under a key. A missing key is a no-op. Pass all: true instead of key to clear every agenda blob for the session in one call.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"key":         stringProperty("Agenda blob name to remove. Omit when all: true."),
					"all":         boolProperty("When true, clear every agenda blob for the session. key is ignored."),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        "agenda.list",
			"description": "Enumerate current agenda blob keys for a session, each with a short one-line summary, without needing to guess key names from tool descriptions.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        "git.resolve_impl_branch",
			"description": "Resolve a ticket's canonical implementation branch before worktree provisioning. Pure name resolution: base is the exact run base/current branch (including goal/* or impl/*); no branch is created. Uses the same merge-root and ticket identity authority as route.resolve_implement.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key."),
					"ticket_stem": stringProperty("Ticket identity to derive."),
					"base":        stringProperty("Exact base/current branch for the run; must not be detached."),
					"format":      stringProperty("Use json for structured output; default compact text."),
				},
				"required": []string{"session_key", "ticket_stem", "base"},
			},
		},
		{
			"name":        "route.resolve_implement",
			"description": "Inner step of the ticket-worker playbook; not a direct entry point — params are constructed by that playbook. Resolves normalized implementation facts and observed Git branch state into one deterministic implementation verdict, stores the 'implement' agenda blob, and replaces the todo list with the derived implement checklist.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"params": map[string]any{
						"type":        "object",
						"description": "Opaque implementation-routing input, constructed by the ticket-worker playbook. Inner step of that playbook; not a direct entry point — see its route step for the full field set.",
					},
				},
				// Open top level: the unwrapped typed call shape (target,
				// policy, facts, format as bare arguments) is a live caller
				// form, so names outside properties are the handler's to judge.
				"additionalProperties": true,
				"required":             []string{"session_key"},
			},
		},
		{
			"name":        "todo.add",
			"description": "Add a new pending todo item with a caller-provided key (unique within the active list) and title. position defaults to \"end\" (append); \"before\"/\"after\" insert relative to ref_key, which is required for those positions and rejected for \"end\". Erased keys are reusable.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"key":         stringProperty("Caller-provided item key. Normalized to lowercase; accepts letters, digits, '.', '_', and '-'; leading or trailing whitespace is rejected; unique within the active list after normalization."),
					"title":       stringProperty("Human-facing item title."),
					"instruction": nullableStringProperty("Optional full instruction prose for this item. Null or omit to leave unset."),
					"position":    enumStringProperty(`Where to add the item. Defaults to "end" (append). "before"/"after" require ref_key; "end" must not carry one.`, []string{"end", "before", "after"}),
					"ref_key":     stringProperty("Existing item key to insert before/after. Required when position is before or after; must be omitted when position is end."),
				},
				"required": []string{"session_key", "key", "title"},
			},
		},
		{
			"name":        "todo.check",
			"description": "Set the status of an existing todo item. Status is one of pending, wip, done, defer. Successful raw/text output includes a full ordered checkpoint todo rendering with instructions only for adjacent actionable items; compact rows with hidden instructions get an indented ...+ marker.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"key":         stringProperty("Item key to update."),
					"status":      enumStringProperty("New status.", []string{"pending", "wip", "done", "defer"}),
				},
				"required": []string{"session_key", "key", "status"},
			},
		},
		{
			"name":        "todo.erase",
			"description": "Remove a todo item by key. The key becomes reusable afterwards.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"key":         stringProperty("Item key to remove."),
				},
				"required": []string{"session_key", "key"},
			},
		},
		{
			"name":        "todo.clear",
			"description": "Remove all todo items, or only done items when done_only is true (leaving pending, wip, defer).",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"done_only":   boolProperty("When true, remove only done items. Defaults to false (remove all)."),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        "todo.list",
			"description": "Render the todo list with visible {key} tokens. Summary mode (default) shows all pending/wip items plus one adjacent context item on each side of each active block, collapsing the rest to '...'. Full mode shows every item in order.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"mode":        enumStringProperty("Rendering mode. Defaults to summary.", []string{"summary", "full"}),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        "todo.read",
			"description": "Return one todo item's full JSON payload, including nullable instruction.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"key":         stringProperty("Item key to read."),
				},
				"required": []string{"session_key", "key"},
			},
		},
		{
			"name":        "todo.reorder",
			"description": "Move the contiguous span [from_key … to_key] as a block to before or after ref_key. ref_key must lie outside the span.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"span": map[string]any{
						"type":        "object",
						"description": "Contiguous span to move, by key.",
						"properties": map[string]any{
							"from_key": stringProperty("First item key of the span."),
							"to_key":   stringProperty("Last item key of the span."),
						},
						"required": []string{"from_key", "to_key"},
					},
					"position": map[string]any{
						"type":        "object",
						"description": "Destination relative to a ref_key. Set exactly one of before or after.",
						"properties": map[string]any{
							"before": stringProperty("Move the span before this ref_key."),
							"after":  stringProperty("Move the span after this ref_key."),
						},
					},
				},
				"required": []string{"session_key", "span", "position"},
			},
		},
		{
			"name":        "api.list",
			"description": "Return sorted local API documentation cache domain names under ai-docs/.deps.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"format": stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "exec.spawn",
			"description": "Start a durable structured command job using argv, with bounded inline output when it completes quickly.",
			"inputSchema": execLaunchSchema(false),
		},
		{
			"name":        "exec.shell",
			"description": "Start a durable explicit shell command job, with bounded inline output when it completes quickly.",
			"inputSchema": execLaunchSchema(true),
		},
		{
			"name":        "exec.status",
			"description": "Return lifecycle status and stream sizes for a durable exec job.",
			"inputSchema": execKeySchema(),
		},
		{
			"name":        "exec.result",
			"description": "Return terminal exec job metadata and at most the fixed 4096-byte inline output budget; positive timeout_seconds waits for completion.",
			"inputSchema": execResultSchema(),
		},
		{
			"name":        "exec.abort",
			"description": "Best-effort terminate a running exec job while preserving partial output.",
			"inputSchema": execKeySchema(),
		},
		{
			"name":        "exec.raw.tail",
			"description": "Read bounded tail text from a persisted exec stdout, stderr, or combined stream.",
			"inputSchema": execRawTailSchema(),
		},
		{
			"name":        "exec.raw.read",
			"description": "Read bytes from a persisted exec stream with offset continuation.",
			"inputSchema": execRawReadSchema(),
		},
		{
			"name":        "exec.raw.grep",
			"description": "Search persisted exec streams with literal matching by default and opt-in regex mode.",
			"inputSchema": execRawGrepSchema(),
		},
		{
			"name":        "config.list",
			"description": "Return the current ws configuration in one read-only view: the resolved config path and per-scope override resolution (session, project, global, builtin) plus the full tuning-knob catalog (each knob's writer, schema-derived field options, allowed/default scope, harness applicability, and current value). Use this to discover every supported config.tune key and its exact write contract. When session_key is supplied, session-scope overrides are included and annotated.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"format":        stringProperty(`Optional output format. Use "json" for structured output.`),
					"session_key":   stringProperty("Optional lead session key. When supplied, resolved override scopes and session-scope prompt overrides are included in the output."),
					"root_override": stringProperty("Optional rsrc root override (test/advanced use); when omitted the shipped rsrc tree is scanned."),
				},
			},
		},
		{
			"name":        "config.tune",
			"description": "Write one ws config knob, selected by its key (e.g. workflow.prefer_subagent, bootstrap_alarm, agents.tier, prompt.<pointId>, worktree.profile.<name>). Call config.list first for each key's exact value domain, scope rules, and harness applicability. value is a string for scalar knobs, an object ({tier, backend, model, effort}) for agents.tier, and a whole profile object (config.list value_schema) for worktree.profile.<name>; agents.tier reset takes value: {tier} only. scope is optional and backstops to the key's declared default; scope repo writes the committed .ws-workflow/config.json in the session's worktree without staging it, so include the reported path in your next git.commit paths; harness is load-bearing for harness-applicable keys (config.list lists a harness selector for them: prompt.*, workflow.whiteboard_enabled, workflow.tracker_enabled, workflow.profile, agents.tier) and warning-only (ignored) for keys that do not vary by harness. reset: true drops a knob's override back to its builtin/inherited default (only for keys that support reset). session_key is required at dispatch for lead-authority keys and prompt.* keys. Lead-only: delegate and leaf keys are blocked by the config.* prefix gate.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key":         stringProperty("Config knob key to write, e.g. workflow.prefer_subagent, bootstrap_alarm, agents.tier, prompt.<pointId>, worktree.profile.<name>, or an adapter-declared key. See config.list for the supported set."),
					"value":       stringOrObjectProperty("New value. A string for scalar knobs (e.g. on/off), an object {tier, backend, model, effort} for agents.tier, or a profile object for worktree.profile.<name>. For agents.tier reset pass only {tier}; omit for other resets."),
					"scope":       enumStringProperty("Optional storage scope. When omitted the write lands in the key's declared default scope. repo writes the committed .ws-workflow/config.json in the session's worktree (session_key required) and leaves staging to your next git.commit. Global-only keys reject non-global scopes; agents.tier supports project and global scopes; worktree.profile.<name> supports project, repo, and global.", wsconfig.ScopeSchemaEnum()),
					"harness":     stringProperty("Optional harness selector. Load-bearing for prompt.*, workflow.whiteboard_enabled, workflow.tracker_enabled and workflow.profile (claude, codex, pi, or * for all) and agents.tier (alias key); ignored for keys that do not vary by harness. When omitted for a harness-applicable key, defaults to the current session's detected harness."),
					"reset":       boolProperty("When true, drop the key's override and fall back to its builtin/inherited default instead of writing an explicit value. Mutually exclusive with value except agents.tier, which requires value: {tier}; only valid for keys that support reset."),
					"session_key": stringProperty("Caller's lead ws session key. Required at dispatch for lead-authority keys (global-only workflow preferences and alarms, workflow.whiteboard_enabled, workflow.tracker_enabled and workflow.profile) and for prompt.* keys; also the target session for a session-scope write."),
				},
				"required": []string{"key"},
			},
		},
		{
			"name":        "config.get",
			"description": "Read-only: resolve config keys and report the scope that supplied each (session, including the session's parent chain, then project, repo, global, builtin). Supply exactly one of key or nonempty keys (unique, nonblank strings). Bulk JSON returns mode=resolved-v1 and ordered results with ok and either value/scope/warnings or error.message; per-key errors do not fail siblings. A key declared by an adapter key manifest returns a value of its declared type (number, boolean, or string), and a stored value that fails that type returns the declared default with a warning; every other key returns its stored string, or scope unset when nothing holds or declares it. Defaults to compact text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key":         stringProperty("Legacy single config key to read; mutually exclusive with keys."),
					"keys":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1, "uniqueItems": true, "description": "Config keys to resolve independently in request order; mutually exclusive with key. Entries must be nonblank and unique."},
					"session_key": stringProperty("Optional ws session key. When supplied, session-scope values (including those inherited from the session's parent chain) and the session's committed repo scope take part in resolution."),
					"format":      stringProperty(`Optional output format. Use "json" for structured output.`),
				},
				"oneOf": []any{
					map[string]any{"required": []string{"key"}, "not": map[string]any{"required": []string{"keys"}}},
					map[string]any{"required": []string{"keys"}, "not": map[string]any{"required": []string{"key"}}},
				},
			},
		},
		{
			"name":        "config.resolve_agent",
			"description": "Read-only: resolve a fixed agent tier (small, medium, large, or xlarge) to its {backend, model, effort} under a harness, applying the same fallback chain (harness bucket, then default, then codex) agents.tier/playbook.render use. Reports resolved_from — the bucket that actually answered — so a caller can tell a harness-local hit from a cross-harness fallback. harness defaults to the current session's detected harness (or default when none is detected) when omitted. No session_key required.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"tier":    stringProperty("Fixed tier to resolve: small, medium, large, or xlarge."),
					"harness": stringProperty("Optional harness selector (e.g. codex, claude, pi). When omitted, defaults to the current session's detected harness, or default when none is known."),
					"format":  stringProperty(`Optional output format. Use "json" for structured output.`),
				},
				"required": []string{"tier"},
			},
		},
		{
			"name":        "git.status",
			"description": "Return read-only Git branch and worktree status. Defaults to compact text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"format": stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "git.diff",
			"description": "Return read-only Git diff output. Defaults to stat text; use mode=full for patch content or format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"range":  stringProperty("Optional revision range."),
					"paths":  stringArrayProperty("Optional path filters appended after --."),
					"mode":   enumStringProperty("Diff mode. Defaults to stat.", []string{"full", "stat", "name_only"}),
					"format": stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "git.log",
			"description": "Return a bounded read-only Git commit log. Defaults to text blocks; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"range":        stringProperty("Optional revision range."),
					"limit":        integerProperty("Maximum commits to return. Defaults to 20 and is capped at 100."),
					"include_body": boolProperty("Include commit body text."),
					"format":       stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "git.followups",
			"description": "Read-only. List open (unresolved) commit-anchored follow-ups: ## Follow-ups entries no commit in the same walk resolves under ## Resolves. Walks commit bodies over range, or the full history from HEAD when range is omitted; openness is evaluated as of the range's end. Each row carries id, level, category, content, and the carrying commit's SHA and date. Defaults to compact text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"range":     stringProperty("Optional revision range (e.g. <start>..HEAD). Omitted: full history from HEAD."),
					"category":  enumStringProperty("Optional category filter. Omitted: every category.", wsgit.FollowupCategories),
					"min_level": enumStringProperty("Optional lowest level to include (minor < important < critical). Omitted: every level.", wsgit.FollowupLevels),
					"format":    stringProperty(`Optional output format. Use "json" for structured output.`),
				},
			},
		},
		{
			"name":        "rationale.query",
			"description": "Search recorded rationale: commit ## AI Context and ## Ticket Updates bullets and ticket decision sections. Address by path globs, a code site (git log -L), a pickaxe string (git log -S), or a text query. Returns pointers with quoted records grouped by ticket thread, newest first. Defaults to compact text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":        stringProperty("BM25 text query over record text. Combines with any addressing mode."),
					"paths":        stringArrayProperty("File globs. A commit record matches when any touched path matches; a ticket record matches when the ticket names a matching path or a commit in its thread touched one."),
					"site":         stringProperty("A code site as <path>:/<regex>/,+N or <path>#L<start>-L<end>, resolved through git log -L. Exclusive with paths and pickaxe. +N defaults to +10."),
					"occurrence":   integerProperty("Which regex match in the file `site` uses when it matches more than once. Default 1."),
					"pickaxe":      stringProperty("A git log -S<string> pickaxe search. Exclusive with paths and site."),
					"since":        stringProperty("YYYY-MM-DD inclusive lower bound on record date."),
					"until":        stringProperty("YYYY-MM-DD inclusive upper bound on record date."),
					"stems":        stringArrayProperty("Keep only records in these ticket threads."),
					"exclude_stem": stringProperty("Drop the ticket's own records and commits that name only this stem."),
					"kinds":        stringArrayProperty("Subset of commit, ticket. Default both."),
					"order":        enumStringProperty("relevance or time. Default relevance when query is given, else time.", []string{"relevance", "time"}),
					"limit":        integerProperty("Maximum records returned. Default 30, cap 100."),
					"format":       stringProperty(`Optional output format. Use "json" for structured output.`),
				},
			},
		},
		{
			"name":        "git.merge_base",
			"description": "Return the read-only Git merge-base hash for two revisions. Defaults to text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"base":   stringProperty("Base revision."),
					"head":   stringProperty("Head revision."),
					"format": stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
				"required": []string{"base", "head"},
			},
		},
		{
			"name":        "git.merge",
			"description": "Lead-only. Merge an exact local source branch into an exact local target using --no-ff. Switches to and leaves the caller on the target branch (no switch-back), so any tolerated worktree change travels to the target checkout and is reported as a non-blocking advisory. Tolerates a dirty worktree but requires a clean index: staged changes and unmerged paths are refused, while unstaged tracked modifications and untracked files are allowed (git's native switch/merge still refuses one that overlaps a merged path). Delete merged impl/* and goal/* sources; preserve other sources. Main/master return policy_blocked diagnostics by default; explicit release-target acknowledgement bound to inspected source_oid and target_oid permits retry. Must-resolve safety findings cannot be waived. Conflicts remain on the target for lead-delegate to resolve. A landed impl/* or goal/* merge may carry advisory size diagnostics for source files it pushed past git.merge_size_soft_lines or edited at git.merge_size_hard_lines or more; relay them to the user rather than splitting the file yourself. Defaults to text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"branch":                  stringProperty("Optional exact local source branch; defaults to the current branch. May be supplied while checked out on another branch. impl/* sources must have impl/<root>/<stem> shape."),
					"target":                  stringProperty("Exact local target branch. Required for non-impl sources; for impl/<root>/<stem>, defaults to the encoded root and must match it when supplied."),
					"release_target_override": map[string]any{"type": "boolean", "default": false, "description": "Explicit acknowledgement of the main/master release-target policy only. Never waives must_resolve diagnostics; requires both expected OIDs."},
					"expected_source_oid":     stringProperty("Full inspected source_oid from the refusal; required when release_target_override is true."),
					"expected_target_oid":     stringProperty("Full inspected target_oid from the refusal; required when release_target_override is true."),
					"title":                   stringProperty("Single-line merge commit title."),
					"description":             stringProperty("Optional merge commit description."),
					"ai_context":              stringArrayProperty("Required AI Context bullets for the merge record."),
					"updated_tickets":         stringArrayProperty("Optional ticket update summaries."),
					"format":                  stringProperty("Use json for structured output."),
				},
				"required": []string{"title", "ai_context"},
			},
		},
		{
			"name":        "git.commit",
			"description": "Create a workflow-aware Git commit from explicit paths and structured message fields. Defaults to compact text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"paths":           stringArrayProperty("Explicit paths to stage and commit. Only these paths are staged."),
					"title":           stringProperty("Single-line commit title."),
					"description":     stringProperty("Optional commit message body before AI Context."),
					"ai_context":      stringArrayProperty("Required AI Context bullets for the commit message."),
					"updated_tickets": stringArrayProperty("Optional ticket update summaries. If omitted, staged ticket moves and Result/Edition headings are detected."),
					"followups": map[string]any{
						"type":        "array",
						"description": "Optional. Root lead session only (any other caller, or no session key, rejects the commit). Forward-looking, ownerless work to track on this commit, written under ## Follow-ups; each gets a minted id returned in the result, in input order. Work a ticket owns belongs in that ticket instead, and past-facing rationale in ai_context.",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"level":    enumStringProperty("Ordered level: minor < important < critical.", wsgit.FollowupLevels),
								"category": enumStringProperty("Closed category.", wsgit.FollowupCategories),
								"content":  stringProperty("Single line, e.g. <path>:<line> and a one-line summary."),
							},
							"required": []string{"level", "category", "content"},
						},
					},
					"resolves":        stringArrayProperty("Optional follow-up ids this commit resolves, written under ## Resolves. Any caller may resolve. An unknown or already-resolved id is a warning; the commit still lands."),
					"expected_branch": stringProperty("The branch you currently remember being on — the one you believe this commit should land on. Fill it from what you already know, not by reading HEAD now: if you cannot recall it, stop and confirm whether you should be committing here at all rather than reflexively resolving the current branch to satisfy the field. This is a safety guard: the commit is refused, with nothing staged or committed, when your believed branch differs from the actual checkout (for example a parallel session switched this shared worktree) or when HEAD is detached."),
					"format":          stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
				"required": []string{"paths", "title", "ai_context", "expected_branch"},
			},
		},
		{
			"name":        "worktree.acquire",
			"description": fmt.Sprintf("Lead-only. Provision an isolated Git worktree of a named profile from a recycled pool for a parallel worker: reuse an idle pooled worktree of that profile or create one, put it into the profile when it is fresh or the profile's provisioning inputs changed (declared sparse-checkout, then post_provision hooks), create target_branch on base if it does not exist and check it out, hygiene-reset the tree keeping the profile's preserve patterns, sync submodules, run the profile's post_acquire hooks, mint a worktree-bound worker session key, and record a worktree lease (worker_key, parent_key, acquired_at) in the worktree's Git admin directory for worktree.list. Fails when the acquired worktree hides the ticket board its commit tracks; a failing post_provision or post_acquire hook quarantines the worktree and fails the call. Returns the worktree path, worker_key, profile, and each hook run (argv, scope, exit code). At most once a day per pool, the result's nudge lists pooled worktrees with a branch checked out and no activity for worktree.nudge_stale_days, or detached with worktree.nudge_preserved_gb or more of preserved artifacts; relay it to the user, who decides whether worktree.remove deletes them. The pool location is the worktree_pool config knob (default %s); profiles are the worktree.profile.<name> config family (config.list), with builtins full (plain checkout) and housekeeping (the ai-docs checkout a lead uses while a worker occupies the root). Defaults to text; use format=json for structured output.", defaultWorktreePoolTemplate),
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"base":          stringProperty("Base commit-ish the worktree branch is created on, e.g. the goal branch."),
					"target_branch": stringProperty("Branch to create (when absent) and check out in the worktree, e.g. impl/<parent>/<slug>. Omit to check out base itself; base must then be a local branch not checked out in another worktree."),
					"profile":       stringProperty("Worktree profile name, e.g. full or housekeeping, or a project profile config.list lists. A pooled worktree is reused only by an acquire naming the same profile."),
					"format":        stringProperty(`Optional output format. Use "json" for structured output.`),
				},
				"required": []string{"base", "profile"},
			},
		},
		{
			"name":        "worktree.release",
			"description": "Lead-only. Return a worktree to the pool: run its profile's pre_release hooks (a failure leaves it unchanged and leased), clean it (git reset --hard + git clean -ffdx, discarding uncommitted changes and every ignored path its profile does not preserve), detach HEAD so it becomes reuse-eligible, record the size of the preserved artifacts it kept for acquire's nudge, and remove its worktree lease. Never deletes the worktree (worktree.remove does) and does not retire the worker key. Identify it by path or by a worker session key bound to it. Refuses, changing nothing, a worktree leased to another holder (the caller's session key is neither the lease's parent_key nor its worker_key, and the key argument is not its worker_key) and a dirty worktree with no lease, unless force is true. Cleaning up a worktree whose owner is gone: pass force: true. Defaults to text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   stringProperty("Absolute worktree path to release. Provide path or key."),
					"key":    stringProperty("A worker session key bound to the worktree to release. Provide path or key."),
					"force":  boolProperty("For whoever cleans up stale worktrees (a lead or the user): release a worktree leased to another holder, or a dirty one with no lease. Cleaning up a worktree whose owner is gone: pass force: true, only when you have confirmed the lease owner is gone or agreed; the release discards its uncommitted work."),
					"format": stringProperty(`Optional output format. Use "json" for structured output.`),
				},
			},
		},
		{
			"name":        "worktree.remove",
			"description": "Lead-only. Delete a pooled worktree: refuse a dirty one, one whose checked-out branch holds commits no other branch reaches, one whose detached HEAD holds commits no ref reaches, the primary worktree, a path outside the owned pool, and a leased one active more recently than worktree.nudge_stale_days; then run its profile's pre_remove hooks (a failure leaves it in place; skipped with a note when the profile no longer resolves) and remove it with its lease and profile marker, clearing any quarantine. Keeps the checked-out branch. Identify it by path or by a worker session key bound to it. Returns the path, profile, kept branch, and each hook run (argv, scope, exit code). Defaults to text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   stringProperty("Absolute worktree path to remove. Provide path or key."),
					"key":    stringProperty("A worker session key bound to the worktree to remove. Provide path or key."),
					"format": stringProperty(`Optional output format. Use "json" for structured output.`),
				},
			},
		},
		{
			"name":        "worktree.list",
			"description": "Lead-only, read-only. List the pooled worktrees worktree.release accepts (the primary worktree and foreign worktrees are excluded; records whose directory is gone are skipped) with facts per worktree: path; branch, or detached (JSON branch null); dirty or clean, counting untracked files; HEAD's commit time; the newest mtime over the paths git status reports (dirty only); and the worktree lease written by worktree.acquire — worker_key, parent_key, acquired_at, and each key record's mtime, or record missing (JSON null) when the record was pruned — or no lease record (JSON worktree_lease null) when the worktree was released or acquired before worktree leases existed. Key records are read without refreshing their mtime. Facts only: it gives no stale or alive verdict; judging whether a holder is done is the reader's call. A clean entry carries a release nudge; a dirty entry carries none. Defaults to text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"format": stringProperty(`Optional output format. Use "json" for structured output.`),
				},
			},
		},
		{
			"name":        "project_tree",
			"description": "Render the ws project document map and active ticket inventory.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "infra.read",
			"description": "Read a bundled ws infra document by bare stem or filename.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]string{
						"type":        "string",
						"description": "Infra document stem or filename, for example impl-playbook.",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "convention.read",
			"description": "Read a bundled ws convention document by bare stem or filename.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]string{
						"type":        "string",
						"description": "Convention document stem or filename, for example ticket-conventions.",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "note.write",
			"description": "Write one or more notes to the worktree (worktree-local, ephemeral; the default), clone (project-scoped, worktree-agnostic, untracked), machine (PC-global), or repo (git-tracked, one file per key under ai-docs/ws-notes/) note layer. A clone or machine write requires a reason. A note outlives the session until erased; in-session state (a pending question, a next step) belongs in agenda.set. Full-overwrite per key, including priority. A larger integer priority is surfaced first in the workflow_manual ambient Notes block.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"layer":       enumStringProperty(`Which layer to write: "worktree" (this worktree only, ephemeral; write here by default), "clone" (this project's checkouts, worktree-agnostic, untracked; only when a session in another worktree would fail without the fact), "machine" (PC-global, project-agnostic; only when a session in another project would fail without the fact), or "repo" (git-tracked, one file per key under ai-docs/ws-notes/; staging/commit rides the caller's normal git.commit).`, []string{"worktree", "clone", "machine", "repo"}),
					"reason":      stringProperty(`Required when layer is "clone" or "machine", rejected as missing when empty: why a session in another worktree (clone) or another project (machine) would fail without this note. Checked only; not stored.`),
					"notes":       objectArrayProperty(`Notes to write, each {"key": string, "value": string, "priority": integer}. priority is an integer defaulting to 0; a larger number is surfaced earlier (5 before 1).`),
					"format":      stringProperty(`Optional output format. Use "json" for structured output.`),
				},
				"required": []string{"session_key", "layer", "notes"},
			},
		},
		{
			"name":        "note.erase",
			"description": "Erase notes by key from the machine, worktree, clone, or repo note layer.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"layer":       enumStringProperty(`Which layer to erase from: "machine", "worktree", "clone", or "repo".`, []string{"machine", "worktree", "clone", "repo"}),
					"keys":        stringArrayProperty("Note keys to erase. A missing key is a no-op."),
					"format":      stringProperty(`Optional output format. Use "json" for structured output.`),
				},
				"required": []string{"session_key", "layer", "keys"},
			},
		},
		{
			"name":        "note.mute",
			"description": "Mute notes by key on the machine, worktree, clone, or repo note layer: sets visible=false so they are excluded from the workflow_manual ambient Notes block and its cap budget (a muted note frees a slot for a previously elided visible note), but note.query still returns them unchanged. Idempotent (muting an already-muted key is a no-op) and never restamps written_at.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"layer":       enumStringProperty(`Which layer to mute on: "machine", "worktree", "clone", or "repo".`, []string{"machine", "worktree", "clone", "repo"}),
					"keys":        stringArrayProperty("Note keys to mute. A missing key is a no-op."),
					"format":      stringProperty(`Optional output format. Use "json" for structured output.`),
				},
				"required": []string{"session_key", "layer", "keys"},
			},
		},
		{
			"name":        "note.unmute",
			"description": "Unmute notes by key on the machine, worktree, clone, or repo note layer: sets visible=true, restoring them to the workflow_manual ambient Notes block. Idempotent (unmuting an already-visible key is a no-op) and never restamps written_at.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"layer":       enumStringProperty(`Which layer to unmute on: "machine", "worktree", "clone", or "repo".`, []string{"machine", "worktree", "clone", "repo"}),
					"keys":        stringArrayProperty("Note keys to unmute. A missing key is a no-op."),
					"format":      stringProperty(`Optional output format. Use "json" for structured output.`),
				},
				"required": []string{"session_key", "layer", "keys"},
			},
		},
		{
			"name":        "note.query",
			"description": "Query notes by key glob and optional written_at date range. \"layer\" is optional: a single layer name (\"worktree\", \"clone\", \"machine\", or \"repo\") queries just that layer and returns a plain untagged record array (today's shape); an array of layer names, or omitting \"layer\" entirely, queries multiple/all layers and returns each record tagged with its originating layer. Results are ordered by priority descending (larger number first), then newest written_at, then key. Retrieves the bodies the workflow_manual ambient Notes block lists key-only or hides.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"layer":       enumStringOrArrayProperty(`Optional. Which layer(s) to search: a single "worktree"/"clone"/"machine"/"repo" string, or an array of those. Omit to search all four layers. A single-string call returns a plain, untagged record array; an array or omitted call returns each record tagged with its "layer". "worktree" is this worktree only; "clone" holds facts a session in another worktree needs; "machine" holds facts a session in another project needs.`, noteLayerEnumValues),
					"glob":        stringProperty(`Optional key glob (path.Match syntax, e.g. "ticket.*"). Omit or "*" to match every key.`),
					"from":        stringProperty("Optional inclusive lower bound on written_at (RFC3339 or a date prefix such as \"2026-08-01\")."),
					"then":        stringProperty("Optional inclusive upper bound on written_at (RFC3339 or a date prefix)."),
					"format":      stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
				"required": []string{"session_key"},
			},
		},
		{
			// The whiteboard tools advertise only a session key and one input:
			// the input's grammar lives in the whiteboard playbook, so a host
			// that loads every schema up front pays almost nothing for a
			// feature its user never turned on, and the description sends a
			// curious model to the instructions instead of inviting a call.
			// whiteboard.update takes a string, not an object: a model writes
			// non-ASCII text inside an object argument as \u escapes, which
			// multiplies the output tokens of a board written in Korean.
			"name":        "whiteboard.update",
			"description": "Lead-only. Read the `whiteboard` playbook (ws/playbook.read) before first use; it defines the commands.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"commands":    stringProperty("Defined by the whiteboard playbook."),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        "whiteboard.query",
			"description": "Lead-only, read-only. Read the `whiteboard` playbook (ws/playbook.read) before first use; it defines the payload.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"payload":     objectProperty("Defined by the whiteboard playbook."),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        "whiteboard.load",
			"description": "Lead-only. Replace your whiteboard with a board zip downloaded from the dashboard board toolbar; the previous board is kept as a backup folder beside it.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"path":        stringProperty("The board zip's path on the host running ws-mcp; a relative path resolves against the session root."),
				},
				"required": []string{"session_key", "path"},
			},
		},
		{
			// The tracker tools follow the whiteboard's shape: always listed
			// with a pointer to their playbook, and a string command batch
			// for the same \u-escape reason.
			"name":        "tracker.update",
			"description": "For a top-level lead, or a store's support conversation, whose writes go to its own store only. Read the `tracker` playbook (ws/playbook.read) before first use; it defines the commands.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"commands":    stringProperty("Defined by the tracker playbook."),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        "tracker.query",
			"description": "Read-only; for a top-level lead, or a store's support conversation. Read the `tracker` playbook (ws/playbook.read) before first use; it defines the payload.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"payload":     objectProperty("Defined by the tracker playbook."),
					"format":      stringProperty("\"json\" for the store's whole view state; text when omitted."),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        wikiQueryTool,
			"description": "Read-only; read a tracker store's wikis: pages kept on git branches of the store, with full history. A page is Markdown, or an HTML page when its path ends in .html: a whole HTML document the dashboard shows in a sandboxed frame, where its scripts run but it cannot show the wiki's images or resolve attachment: references. For a top-level lead, or a store's support conversation on its own store. op list lists the wikis, pages a wiki's pages, read one page with the commit it was read at (the base a wiki.update save of that page must name), search finds text in the pages, history lists commits with their signature verdicts, diff shows a page's change, and recovery lists tips a rewind or deletion replaced. A link wiki:<name>/<path> in a store's item, comment, or wiki page names the page <path> of the wiki <name> in that store. Text by default; format \"json\" for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"store":       stringProperty("The ID of a registered tracker store, as tracker.query lists them."),
					"op":          enumStringProperty("list: the store's wikis and their tips. pages: the pages of wiki (at commit when given). read: page path of wiki (at commit when given), its kind (markdown or html), and the commit read, the base for a later save. search: lines of wiki's pages containing query (case-insensitive) at the tip, an HTML page's raw source included. history: wiki's commits, or those touching path, newest first. diff: path's change in commit to, against from or the parent of to. recovery: the replaced tips kept on this machine, of wiki or of every wiki.", wikiQueryOps),
					"wiki":        stringProperty("The wiki name: [a-z0-9][a-z0-9-]*, at most 64 characters. Required by every op but list and recovery."),
					"path":        stringProperty("A page path: folder segments and the page name joined by /. A Markdown page is named without a suffix, such as guides/setup (guides/setup.md names the same page); a path ending in .html, such as guides/setup.html, names an HTML page. Required by read and diff; narrows history."),
					"commit":      stringProperty("A full commit id to read pages or a page at instead of the tip (from history or recovery)."),
					"query":       stringProperty("The text search finds (a fixed string, not a pattern)."),
					"from":        stringProperty("diff: the full commit id to diff from; omitted, the parent of to."),
					"to":          stringProperty("diff: the full commit id whose change is shown."),
					"limit":       integerProperty("history: at most this many commits (30 when omitted). search: at most this many hits."),
					"format":      stringProperty("\"json\" for structured output; text when omitted."),
				},
				"required": []string{"session_key", "store", "op"},
			},
		},
		{
			"name":        wikiUpdateTool,
			"description": "Save one edit to a tracker store's wiki as a signed commit. For a top-level lead, or a store's support conversation on its own store. Read the page with wiki.query op=read first and pass the commit it returned as base: a save never falls back to the current tip. When others saved since your base, a non-overlapping edit merges automatically; an overlapping one, or a base that is not an ancestor of the tip, is refused as a conflict carrying both texts and the tip to save on next. new_wiki: true creates a wiki that does not exist yet, with no base. The result names the new commit, the base for your next save. A path ending in .html makes an HTML page: its content is a whole HTML document, scripts and styles included, which the dashboard shows in a sandboxed frame; such a page cannot show pasted images or resolve attachment: references, so a write of one carries no images. Item bodies, comments, and pages link a page of the same store as [text](wiki:<name>/<path>), the page <path> of the wiki <name>.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"store":       stringProperty("The ID of a registered tracker store, as tracker.query lists them."),
					"wiki":        stringProperty("The wiki name: [a-z0-9][a-z0-9-]*, at most 64 characters."),
					"op":          enumStringProperty("write: put content at path (creating or replacing the page). rename: move path to to. delete: remove path. restore: put path back as commit held it, with the images it references.", wikiUpdateOps),
					"base":        stringProperty("The full commit id the edit started from: the commit wiki.query op=read returned, or the commit of your previous save. Required unless new_wiki. A stale base is refused as a conflict."),
					"new_wiki":    boolProperty("true creates the wiki with this save; refused when the wiki already exists. Pass no base with it."),
					"path":        stringProperty("The page path: folder segments and the page name joined by /. A Markdown page is named without a suffix, such as guides/setup (guides/setup.md names the same page); a path ending in .html, such as guides/setup.html, names an HTML page. No folder, and no other page name, ends in .md or .html."),
					"to":          stringProperty("rename: the new page path; refused when a page is already there. Its suffix sets the kind, so a rename may turn a Markdown page into an HTML page or back."),
					"content":     stringProperty("write: the whole page text: Markdown, or for an HTML page a whole HTML document."),
					"commit":      stringProperty("restore: the full commit id to restore the page from (from history or recovery)."),
					"message":     stringProperty("Optional commit message body under the generated subject."),
					"images": map[string]any{
						"type":        "array",
						"description": "write: images saved beside a Markdown page, each a PNG, JPEG, GIF, or WebP; refused for an HTML page, which cannot show them. A Markdown page shows one as ![alt](attachment:<sha256>), where <sha256> is the lowercase hex sha256 of the image bytes, so the same write's content can already reference it; the result lists each image's sha256.",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"data": stringProperty("The image bytes, base64-encoded."),
								"name": stringProperty("Optional label echoed beside the image's sha256 in the result."),
							},
							"required": []string{"data"},
						},
					},
					"format": stringProperty("\"json\" for structured output, a conflict included; text when omitted."),
				},
				"required": []string{"session_key", "store", "wiki", "op", "path"},
			},
		},
		{
			"name":        trackerShowTool,
			"description": "Show one item of your store as a card in your store's chat; the user can open it. For a store's support conversation only. An item of another store comes back as text, with no card.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key."),
					"item":        stringProperty("The item's ID as tracker.query prints it, such as maya.pc-5."),
				},
				"required": []string{"session_key", "item"},
			},
		},
		{
			"name":        trackerUITool,
			"description": "Move the user's tracker view of your store, as the user could: search types a query into the Items search box, which takes the search grammar of tracker.query's q (the tracker playbook spells it out) and lists the current scope's matching items, so find the items with tracker.query q first; focus opens an item's window, at one of its thread's comments when comment is given, scope switches the Items scope, and view switches the Items view mode. For a store's support conversation only. The view moves only while the user has your chat open; on a narrow screen a search, scope, or view switch hides the chat and any later command is lost, so send UI commands as the last step of your reply, with at most one search, scope, or view among them. It never writes and never leaves your store.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key."),
					"op":          enumStringProperty("The operation.", wslive.UIOps),
					"query":       stringProperty("search only, and required for it: the search box text, one line, in tracker.query's q grammar."),
					"item":        stringProperty("focus only, and required for it: an item of your store, its ID as tracker.query prints it, such as maya.pc-5."),
					"comment":     stringProperty("focus only, optional: a comment on that item's thread, its ID as tracker.query prints it among the item's comments."),
					"scope":       enumStringProperty("scope only, and required for it. given is the set tracker.query's scope calls assigned.", wslive.UIScopes),
					"mode":        enumStringProperty("view only, and required for it. The all scope's tree is list; received and related offer list alone.", wslive.UIViewModes),
				},
				"required": []string{"session_key", "op"},
			},
		},
		{
			"name":        gitSearchTool,
			"description": "Read-only git search over the user's local repositories: log, show, grep, blame, or ls-files. Call it without repo to list the roots it may read (the project roots of the sessions the dashboard saw on this machine in the last 7 days, plus the agent.git_search_roots config knob), then pass one of them, or a directory inside one, as repo. options takes only flags from the command's allowlist, each value attached (--name=value, -n5); a refused flag's answer lists the accepted ones. Read a file's contents with show and a revision such as HEAD:path/to/file.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key."),
					"repo":        stringProperty("Absolute path of a readable root, or of a directory inside one. Omit it to list the readable roots; the call then does nothing else."),
					"command":     enumStringProperty("Required with repo.", gitSearchCommands),
					"options":     stringArrayProperty("Flags for the command, each value attached (--max-count=5, -n5, -S<text>, -L10,20). No git global options (-c, -C <dir>, --git-dir, --work-tree)."),
					"pattern":     stringProperty("grep only, and required for it: the pattern, passed as -e <pattern>."),
					"revisions":   stringArrayProperty("Revisions such as HEAD~3, main..topic, or HEAD:path/to/file for show. None may start with -. ls-files takes none; blame at most one."),
					"paths":       stringArrayProperty("Paths, relative to repo, that limit the command (passed after --). blame takes exactly one."),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        "mailbox.send",
			"description": `Send a message to another harness session by stable address, without blocking. "to" is either "<slug>@<scope>" (scope one of "machine", "worktree", "clone") to reach a named inbox, or "id:<reply-id>" to reply to whatever mailbox.recv/mailbox.lookup_peers most recently surfaced as your_reply_id/reply_id for that peer. Works even when the caller has no mailbox identity of its own (WS_MAILBOX/WS_MAILBOX_AUTO unset): the first send from any session opens that session's own reply-id return channel, so a reply sent back to "id:<your reply-id>" is always retrievable via mailbox.recv. A top-level lead's sends are prefixed with a "[<product> note: from a peer lead session ...]" line, <product> being this server's namespace, telling the receiver the sender is a peer lead, not its worker.`,
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"to":          stringProperty(`Recipient address: "<slug>@<scope>" or "id:<reply-id>".`),
					"content":     stringProperty("Message body."),
					"format":      stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
				"required": []string{"session_key", "to", "content"},
			},
		},
		{
			"name":        "mailbox.recv",
			"description": "Drain and return every unread message addressed to the caller: both this session's own reply-id queue (always, once opened by a prior mailbox.send or self-lookup) and, when this session currently holds the owner binding for the process's active WS_MAILBOX/WS_MAILBOX_AUTO named inbox, that inbox's queue too. Each returned message carries the sender's return handle (its own slug when reachable, otherwise an id:<reply-id>) to reply to with mailbox.send. Non-blocking: returns immediately, empty when there is nothing unread.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"format":      stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        "mailbox.lookup_peers",
			"description": `List the peers self-registered in one mailbox scope ("machine", "worktree", or "clone"), the caller's own correspondents, and a "self" entry: the caller process's own address when it holds an active WS_MAILBOX/WS_MAILBOX_AUTO identity, otherwise the caller session's own reply-id (opening that session's return channel as a side effect, the same as a first mailbox.send). Peers are the scope's most recent named inboxes whether or not their session is running now, newest last_seen first, at most 20; mail sent to an inbox whose session is gone queues until a session with that WS_MAILBOX name returns. An auto-minted (WS_MAILBOX_AUTO) name whose session is gone is omitted, because a restart mints a new name and never drains the old one. A name held by two running processes at once is surfaced with a conflict marker rather than silently picking one. Correspondents are the addresses this session (its session key) itself sent mail to or received mail from, in any scope, whatever "scope" is queried: newest first, at most 20, each with last_exchange (the time of this session's own send or recv, not the message's sent_at) and the counterparty's harness/cwd when known. No other session's history is visible, and it survives compaction under the same session key.`,
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"scope":       enumStringProperty(`Which scope to list: "machine", "worktree", or "clone".`, []string{"machine", "worktree", "clone"}),
					"format":      stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
				"required": []string{"session_key", "scope"},
			},
		},
		{
			"name":        "mailbox.history",
			"description": `Read-only: list the last 10 messages this session (its session key) sent or received through mailbox.send and mailbox.recv, newest first, without consuming or changing anything. Use it to recover mail the conversation lost after mailbox.recv already drained it, for example after a rewind. Each entry has its direction (recv or sent), the counterparty address, the time this session sent or drained it, and a title: the first line of the body, skipping a leading peer-lead note line, cut at 120 characters. body: true adds the full body as the session saw it. No other session's history is visible, and it survives compaction under the same session key.`,
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Caller's ws session key (see ws:workflow-manual)."),
					"limit":       integerProperty("Optional maximum number of entries. Defaults to 10; a value below 1 uses the default."),
					"body":        boolProperty("Optional. true includes each entry's full body; defaults to false (title only)."),
					"direction":   enumStringProperty(`Optional filter: "recv", "sent", or "all" (default).`, []string{"recv", "sent", "all"}),
					"format":      stringProperty(`Optional output format. Use "json" for structured output.`),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        dashboardIntakeTool,
			"description": `Adapter-only: a host adapter (the extension or mod that runs inside the agent host) calls this to hand its session's live activity to the dashboard; an agent never calls it. Each call applies an ordered list of events to the session's live state, which ws-mcp writes to the session's live snapshot (debounced) and, for finished conversation messages, to its conversation ring. Event kinds: reset (clear the lead and the subagent tree; send it when the adapter starts); turn_start {origin?: terminal|whiteboard|mail} and turn_end {reason?} (turn_end records the turn summary: tool calls done and failed, and duration; reason is the host's end reason, kept on the turn's conversation entry, and aborted marks a turn the user stopped); agent {agentId?, parentId?, label?, sessionKey?, status?: running|done|failed, caption?, batches?, model?, effort?, contextTokens?, outputTps?} updates the lead (no agentId) or one subagent keyed by the host's native agent id, where batches is the agent's newest batch views (one assistant message and its tool calls each, oldest first; at most 5 kept, 5 tool names each, a 200-character argument preview on the running call, and optionally on each entry of a finished batch's calls list; ws-mcp masks well-known credential shapes in every preview, best-effort), sessionKey, a delegate's ws session key, is shown only as its reply-id, and model, effort, contextTokens (the subagent's current context size: its latest model request's whole input), and outputTps (output tokens per second) are a subagent's telemetry, each kept until an event carries it again, the rate shown only while the subagent runs; permission {agentId?, tool, args} or {agentId?, clear: true} marks a tool permission wait; reply {text, final?} is the lead's reply text so far, replacing the previous text, and final: true moves it to the conversation; user {text, origin?: terminal|whiteboard} is a finished user message; status {context?: {tokens (null while unknown), window, percent?, cacheHit?}, limits?: [{kind: five_hour|seven_day, percent, resetsAt? (epoch milliseconds)}], cost?: {usd, estimate?, incomplete?}} replaces the session's usage figures for its status band, where a part left out shows as unknown and reset clears them (the band's folder and branch state are read by ws-mcp itself); compact {phase: start|end|failed, trigger?: auto|manual} marks a compaction of the main conversation (never a subagent's): start shows the lead compacting, end (it landed) or failed (it failed or was vetoed) ends that and records one compaction line in the conversation, and the next turn_start or reset ends a compaction whose end never came, recording nothing; commands {commands: [{name, description?, source?, layer?: user|agent}]} replaces the slash commands the adapter runs from the dashboard page, which the page's command picker offers (a name without its leading slash; an empty list clears them, and so does reset, so the adapter sends its list again after a reset); command {command, text?} records a slash command the adapter ran from a dashboard command entry as one conversation line, where command is its typed line (/name or /name args) and text the output it showed as text, if any. Every event takes an optional at (epoch milliseconds). Coalesce streamed reply text on the adapter side: send the text so far at most about 10 times a second, never one call per token. Session keys never reach the snapshot. With the session's dashboard.enabled off, nothing is recorded. Returns one compact line, then an "accepts: <kinds>" line naming the optional event kinds this ws-mcp accepts (status, compact, commands, command), then one line per skipped event; send an optional kind only while the latest result names it, since an older ws-mcp skips it, and a "dashboard off" result names none.`,
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("The adapter's session key: the ws session key of the session it feeds."),
					"adapter":     stringProperty(`The adapter's name, for example "pi" or "claude".`),
					"events": map[string]any{
						"type":        "array",
						"description": "Ordered events; each is an object with a kind field.",
						"items":       map[string]any{"type": "object"},
					},
				},
				"required": []string{"session_key", "adapter", "events"},
			},
		},
		{
			"name":        dashboardInboxDrainTool,
			"description": `Adapter-only: a host adapter (the extension or mod that runs inside the agent host) calls this at the end of a turn to collect the messages the user sent from the dashboard page; an agent never calls it, because a model call would claim a held message that is then never injected into the conversation. It atomically claims every held whiteboard message of the session (the user's messages sent from the dashboard page, one per send), in queue order: the messages promoted to steering first, in the order they were promoted, then the rest, oldest first; and returns each nonempty message's text inside its own <ws:user at="YYYY-MM-DDThh:mm+09:00">...</ws:user> envelope, with the send's instant at minute precision and the sending browser's UTC offset (the daemon's local zone for older entries). The envelope contains the answer block of any board answers and topic notes sent along (a <wb:answers> block, one line per entry), then the board text the user pointed at (one <wb:mention topic="..." id="...">text</wb:mention> element per line, id only when the selection lay in one item card), then the page elements the user picked in a web card (one <wb:pick url="..." selector="...">text</wb:pick> element per line) and the console errors attached from one (<wb:console url="...">one error per line</wb:console>), then the typed text. A command entry is a slash command the user picked on the page: its text is the typed line (/name or /name args), and the adapter runs it natively, not as a message; a drained batch splits at a command, so the messages before it reach the session first, then it runs, then the messages after it. A claimed message can no longer be cancelled or steered from the page, and no call returns it twice. The adapter injects each returned message as the user's own message. steer_only: true claims only the messages the user promoted to steering, leaving every other one held; the adapter calls it that way at a tool boundary of a running turn and injects what it returns into that turn. interrupt_only: true claims only the interrupt entries, each the user's request from the dashboard page to stop the running turn, and leaves every message held; an interrupt entry carries no text, only its creation time as sent_at, and the adapter stops a turn that started before that time and drops an older entry. steer_only and interrupt_only cannot be combined. A full drain (neither flag) drops every interrupt entry and never returns one. Works whether or not the session's dashboard is on; an empty inbox reports so. Default output is compact text: "inbox empty", or "drained N" and then per message a "message <id> <time>" line (a command entry's headed "command <id> <time>" instead), its text, and a blank line. format: "json" returns {messages: [{id, sent_at (epoch milliseconds), text, parts, chips: [{id, answer, note}], topics: [{topic, note}], mentions: [{topic, id?, text}], picks: [{url, selector, text}], consoles: [{url, lines}], interrupt (true on an interrupt entry, absent otherwise), command ({name, args?} on a command entry, absent otherwise)}]}.`,
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key":    stringProperty("The adapter's session key: the ws session key of the session it feeds."),
					"steer_only":     boolProperty("When true, claim only the messages promoted to steering; every other message stays held."),
					"interrupt_only": boolProperty("When true, claim only the interrupt entries (the user's requests to stop the running turn); every message stays held. Cannot be combined with steer_only."),
					"format":         stringProperty(`Optional output format. Use "json" for structured output.`),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        "tickets.query",
			"description": "Query ticket paths by text query, ticket stem, or mentions of another ticket stem. A ticket_stem given alone (no query, no mentions_ticket_stem, no statuses) point-resolves that ticket across all statuses, including done and dropped, and returns its status metadata, erroring if the stem is not found; this exact form remains unpaginated. Otherwise this is a discovery search, paginated after all filters in deterministic status-rank and ticket-stem order: offset defaults to 0; limit defaults to 50 and accepts 1 through 200; request the next page with offset + limit. The point-resolve projection also carries a dispatch_blocked {blocking_stem, reason} field, computed live, when the ticket declares a blocked-by: prerequisite that has not landed (the producer is not yet in .done/, or its named phase carries no ### Result); it is absent otherwise and never on a discovery listing. Every projection (discovery and point-resolve) also carries blocked_headings: the verbatim body heading lines beginning with '## Blocked', rendered as blocked_marker lines in compact text. This is advisory evidence only, distinct from the typed dispatch_blocked gate — it creates no hard block and its suffix is returned uninterpreted, so a caller reads the referenced section to judge whether the blocker is current. When the ticket-assignee-aware project flag is on, every projection also carries assignee (the ticket's assignee emails) and a computed assignee_gate {assign_any, assigned_to_current, warning}; a ticket assigned to someone else renders 'assignee: <email>   # NOT ASSIGNED TO YOU' in compact text. Setting assigned_to_me=true additionally hard-omits others'-assigned tickets from a discovery result (assign-any and self-assigned tickets remain). Both are inert while the flag is off. When the project uses the origin ticket ownership index, every projection also carries ownership {level: self|local|remote|unowned|unknown, email, track, worktree, phase, touched_at, impl_branch, provisional, origin_closed, index_state, cache_age_seconds}; compact discovery text lists the caller's own and unowned tickets in full and collapses tickets held elsewhere or closed on origin to one line each, and trailing ticket-index lines report a stale cache or pending offline entries. Defaults to compact text; use format=json for structured metadata.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"statuses":             stringArrayProperty("Optional ticket statuses to scan: ready, todo, idea, done, dropped."),
					"include_done":         boolProperty("Include ai-docs/tickets/.done in discovery when true; point-resolve always includes it."),
					"include_dropped":      boolProperty("Include ai-docs/tickets/.dropped in discovery when true; point-resolve always includes it."),
					"query":                stringProperty("Optional case-insensitive text query."),
					"ticket_stem":          stringProperty("Optional exact ticket stem. Given alone, point-resolves that ticket."),
					"mentions_ticket_stem": stringProperty("Optional ticket stem that result tickets must mention."),
					"assigned_to_me":       boolProperty("When true and the ticket-assignee-aware flag is on, hard-omits others'-assigned tickets from a discovery result (assign-any and self-assigned tickets remain). Inert while the flag is off or the caller's git identity is unknown. The selector sets this to steer to the caller's own tickets."),
					"unleased_or_mine":     boolProperty("When true and the project uses the origin ticket ownership index, keeps only tickets whose lease is the caller's own (email, clone, track) or absent, plus tickets whose ownership is unknown, and drops tickets closed on origin; composes with assigned_to_me and applies before pagination. Inert in a project without the index. Queue selectors set this."),
					"offset": map[string]any{
						"type":        "integer",
						"description": "Discovery result offset after filtering and deterministic ordering. Defaults to 0.",
						"default":     0,
						"minimum":     0,
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum discovery results to return. Defaults to 50; accepts 1 through 200.",
						"default":     50,
						"minimum":     1,
						"maximum":     200,
					},
					"format": stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "tickets.close",
			"description": "Close a ticket to .done/ (status=done) or .dropped/ (status=dropped), writing the dated frontmatter field and optionally appending a ## Resolution section. Stages the change set atomically (frontmatter write -> git add -> git mv); does not commit. With the origin ticket ownership index, a lease held by another person (email) refuses the call unless dangerously_override_lease_status is true with a reason; a lease held by another clone or track proceeds with a warning, and the lease never moves.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"stem":                              stringProperty("Ticket stem (YYMMDD-category-name)."),
					"status":                            stringProperty("Close target: done or dropped."),
					"resolution":                        stringProperty("Optional resolution text appended as a ## Resolution (today) section."),
					"dangerously_override_lease_status": boolProperty("With the origin ticket ownership index, proceed although another person (email) holds the ticket's lease; the lease stays with its holder and the index records the actor and reason. Set only on the user's explicit instruction; requires reason. Ignored in a project without the index."),
					"reason":                            stringProperty("Why the lease guard is overridden. Required with dangerously_override_lease_status."),
				},
				"required": []string{"stem", "status"},
			},
		},
		{
			"name":        "review.marker",
			"description": "Read the review-watermark ledger's frontier entry — the last clearing (pass/concern/bootstrap) entry; a trailing block/routed tail does not advance it — as base..head: verdict[ -> ref]. Optional bootstrap=true creates a baseline entry at current HEAD when no entry exists yet (idempotent on repeat) — the only caller-opted-in bootstrap trigger; a bare marker read never bootstraps. Defaults to compact text; use format=json for structured base/head/verdict/ref/found output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"bootstrap": boolProperty("When true and no ledger entry exists, create a baseline bootstrap entry at current HEAD. No-op (returns the existing entry) when one already exists."),
					"format":    stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
			},
		},
		{
			"name":        "review.stamp",
			"description": "Append a verdict entry to the review-watermark ledger, recording a completed sweep over base..head. Verdict is one of pass, concern, block, routed, bootstrap; ref (a routed ticket stem) is required when verdict is block.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"base":    stringProperty("Reviewed range's base commit (SHA-shaped)."),
					"head":    stringProperty("Reviewed range's head commit (SHA-shaped)."),
					"verdict": stringProperty("One of: pass, concern, block, routed, bootstrap."),
					"ref":     stringProperty("Routed ticket stem. Required when verdict is block; optional otherwise."),
				},
				"required": []string{"base", "head", "verdict"},
			},
		},
		{
			"name":        "tickets.move",
			"description": "Move a ticket along the idea <-> todo <-> ready axis. Non-implementation categories (epic, research, workset) are board artifacts, never execution targets, and are rejected at the ready/ landing, as is a ticket that still carries a ## Open Decision Queue section outside fenced code. Ready promotion and epic todo settlement resolve sage-review posture from config; actionable todo moves are ungated. Stages atomically; does not commit. With the origin ticket ownership index, a lease held by another person (email) refuses the call unless dangerously_override_lease_status is true with a reason; a lease held by another clone or track proceeds with a warning, and the lease never moves.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"stem":                              stringProperty("Ticket stem (YYMMDD-category-name)."),
					"to":                                stringProperty("Target status: idea, todo, or ready."),
					"dangerously_override_lease_status": boolProperty("With the origin ticket ownership index, proceed although another person (email) holds the ticket's lease; the lease stays with its holder and the index records the actor and reason. Set only on the user's explicit instruction; requires reason. Ignored in a project without the index."),
					"reason":                            stringProperty("Why the lease guard is overridden. Required with dangerously_override_lease_status."),
				},
				"required": []string{"stem", "to"},
			},
		},
		{
			"name":        "tickets.create_empty",
			"description": "Create a dated ticket stub at ai-docs/tickets/<status>/<YYMMDD>-<stem>.md with minimal frontmatter (title plus resolved sage-review posture for ready or epic todo). Yields only a valid empty skeleton + applicable initial posture, not a full mutation orchestrator — populate the body via tickets.template. When the ticket-assignee-aware project flag is on, set_assignee (default true) stamps the assignee: true = the caller's current git user.email (nothing when unset), false = leave unassigned, or an array of emails = assign to those. Inert while the flag is off. Returns the path and a promotion tip, plus a warning when the path lies outside this worktree's active sparse-checkout scope (git drops such a file from the worktree once it is committed); does not stage or commit.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"stem":          stringProperty("Semantic ticket stem without date prefix (e.g. feat-foo-bar). Authoring categories: feat, bug, refactor, chore, research, epic."),
					"initial_state": stringProperty("Ticket status: idea, todo, or ready."),
					"set_assignee": map[string]any{
						"description": "Assignee to stamp at creation, honored only when the ticket-assignee-aware flag is on. true (default) = the caller's current git user.email; false = leave unassigned (assign-any); an array of email strings = assign to exactly those. Empty current email (CI/bot) yields no stamp.",
						"oneOf": []any{
							map[string]any{"type": "boolean"},
							map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
					},
				},
				"required": []string{"stem", "initial_state"},
			},
		},
		{
			"name":        "tickets.template",
			"description": "Return the fill-in body skeleton for a given ticket type. Use at creation time instead of loading the full ticket-conventions document.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type": stringProperty("Ticket category: feat, bug, refactor, chore, research, or epic."),
				},
				"required": []string{"type"},
			},
		},
		{
			"name":        "tickets.checklist",
			"description": "Return a ticket-authoring phase's checklist item list as data, for installing into a single todo.add instruction. Use instead of following the phase's static prose section directly.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":  stringProperty("Ticket category: feat, bug, refactor, chore, research, or epic."),
					"phase": enumStringProperty("Ticket-authoring phase.", []string{"content", "intent"}),
				},
				"required": []string{"type", "phase"},
			},
		},
		{
			"name":        "tickets.sage_gate",
			"description": "Resolve the sage-review gate for a ticket landing. The todo landing runs epic design-only review (completeness never applies to an epic), invoked on lead judgment when the epic's cross-child design has drifted materially — not a status boundary; actionable todo calls fail, and research and legacy worksets skip. Actionable design and completeness review runs at ready promotion after fact population. Owns posture resolution (legacy sage-review: migration, config.list fallback), the category×stage matrix, and standalone/combined mode selection. Returns an action (skip | stop_blocked | stop_missing_route_facts | stop_open_decision_queue | ask | run); for run, the reviewer(s) to spawn and the mode. A ready/ landing is refused with stop_missing_route_facts when the ticket carries no ## Route Facts section. The ready/ landing and the epic todo landing are refused with stop_open_decision_queue, before any write, while the ticket carries a ## Open Decision Queue section outside fenced code. Does not spawn reviewers.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"stem":    stringProperty("Ticket stem (YYMMDD-category-name)."),
					"landing": enumStringProperty("Landing: todo runs epic design-only review (lead-judgment-invoked, not a boundary), ready runs actionable design and completeness, idea skips.", []string{"idea", "todo", "ready"}),
					"answer":  enumStringProperty("Optional follow-up answer to a prior ask action.", []string{"yes", "no"}),
				},
				"required": []string{"stem", "landing"},
			},
		},
		{
			"name":        "tickets.sage_stamp",
			"description": "Lead-only. Record sage-review verdicts after reviewers ran: aggregates design/completeness verdicts (incl. resolution: missing escalation), writes the resolved frontmatter posture, renders any Blocked section from the Go-owned template, and returns the applied posture. Does not stage or commit; the caller commits the posture change (and any other uncommitted edits it holds) via its own ws/git.commit. This is the thin, lead-only replacement for the former tickets.sage_record write; reviewers never call this tool.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"stem":     stringProperty("Ticket stem (YYMMDD-category-name)."),
					"stage":    enumStringProperty("Which stage's verdicts are being recorded.", []string{"design", "completeness", "combined"}),
					"verdicts": objectArrayProperty("Reviewer verdicts: [{reviewer, verdict, issues:[{title, severity, resolution}]}]. verdict is pass|concern|block."),
				},
				"required": []string{"stem", "stage", "verdicts"},
			},
		},
		{
			"name":        "tickets.acquire",
			"description": "Lease a ticket to the caller's track in the origin ticket ownership index. The owner is (git user.email, this clone's id, track); the track is the current branch's merge root (an impl/<root>/<stem> branch records its root, and when that root's owner already holds the lease only the impl branch is recorded). A rootless impl/<stem> or implement/<stem> branch needs an explicit track; a detached HEAD is refused. A ticket already under .done/ or .dropped/ on the origin review-track is refused (pull first). A lease held by another email, or by another track of this clone, is refused unless dangerously_override_lease_status is true with a non-empty reason, which is set only on the user's explicit instruction; a lease held by the same email on another clone moves to the caller with a warning. When origin is unreachable on a clone that has used the index, the cached index decides, and a success is recorded as pending with an offline warning. In a project without the index this returns a plain ok and validates nothing. Defaults to compact text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ticket_stem":                       stringProperty("Ticket stem (YYMMDD-category-name). The ticket file must exist in this checkout."),
					"track":                             stringProperty("Optional explicit track (work line) to record, for branches with no merge root."),
					"dangerously_override_lease_status": boolProperty("Take over a lease the ownership rules refuse. Set only on the user's explicit instruction; requires reason."),
					"reason":                            stringProperty("Why the lease is overridden; recorded in the index history. Required with dangerously_override_lease_status."),
					"format":                            stringProperty(`Optional output format. Use "json" for structured output.`),
				},
				"required": []string{"ticket_stem"},
			},
		},
		{
			"name":        "tickets.release",
			"description": "Remove the caller's own lease on a ticket from the origin ticket ownership index. The owner is (git user.email, this clone's id, track); the track is the current branch's merge root unless track is given, which releases this email and clone's lease on that track from any checkout. Releasing a lease held by another owner (email, clone, or track) is refused; taking it over is an acquire. Offline on a clone that has used the index, the release is recorded as pending. In a project without the index this returns a plain ok and validates nothing. Defaults to compact text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ticket_stem": stringProperty("Ticket stem (YYMMDD-category-name)."),
					"track":       stringProperty("Optional track (work line) the caller's own lease is recorded on, when it differs from the current branch's merge root."),
					"format":      stringProperty(`Optional output format. Use "json" for structured output.`),
				},
				"required": []string{"ticket_stem"},
			},
		},
		{
			"name":        "tickets.index_init",
			"description": "Lead-only. Set up the origin ticket ownership index: pushes one index ref to origin with the user's git credentials and registers every open ticket (idea/, todo/, ready/) on origin's review-track. If another clone created it first, the existing index is adopted. With check=true it pushes nothing and reports the live state: initialized, uninitialized (origin reachable, no index), no-origin, or unreachable. Ask the user before running init. Defaults to compact text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"check":  boolProperty("When true, only report the index state; nothing is pushed or created."),
					"format": stringProperty(`Optional output format. Use "json" for structured output.`),
				},
			},
		},
		{
			"name":        "tickets.verify",
			"description": "Run the ticket-write guardrails (stem/status-dir, frontmatter fence integrity, ready-landing sage-review posture, phase/Result heading well-formedness, close date-field presence) against ticket-shaped paths without staging or committing. These are the same hard guardrails git.commit enforces before it will commit a ticket-touching change; soft warnings are reported but never block. Non-ticket paths are silently skipped. Use standalone for mid-edit red/green feedback before staging.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"paths":  stringArrayProperty("Ticket file paths to verify (ai-docs/tickets/<status>/<stem>.md)."),
					"format": stringProperty(`Optional output format. Use "json" for structured compatibility output.`),
				},
				"required": []string{"paths"},
			},
		},
		{
			"name":        "path.generate",
			"description": "Generate worktree-scoped writable paths for workflow artifacts.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind":  enumStringProperty(`Generated path kind. "review" and "prompt" allocate cache artifacts; "plan" allocates a repo-local implementation plan under ai-docs/.plans/; "clone" allocates a worktree-agnostic shared doc under the clone's SharedDir (readable "<stem>-<suffix>.md" filename), for the note.write clone-layer oversize nudge.`, []string{"review", "prompt", "plan", "clone"}),
					"stems": stringArrayProperty("Logical file stems to allocate in stable order."),
				},
				"required": []string{"kind", "stems"},
			},
		},
		{
			"name":        "workflow_manual",
			"description": "Render and restore the ws workflow primitives manual and session state for a lead session_key. An unresolvable key returns a fail-loud notice and never mints a new key. Lead-only.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Required. Your lead session key."),
					"root":        stringProperty("Optional absolute Git worktree root. When provided alongside the fresh-bootstrap sentinel, the handler mints a lead session key and returns it inline, eliminating the separate ferrule call."),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        "workflow_state",
			"description": "Return only the Session State section (agenda/todos) for a lead session_key, with no manual reference text. A cheap alternative to workflow_manual for a caller that only needs 'what's my key and current state', costly right after compaction or during lead-revive. An unresolvable key returns the same fail-loud notice as workflow_manual and never mints a new key. Lead-only; workflow_manual itself is unchanged.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty("Required. Your lead session key."),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        "playbook.read",
			"description": "Return a playbook's rendered procedure text inline (harness-aware, includes resolved, declared variables substituted). Available in both full and agentless product modes.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":        stringProperty("Playbook name (bare stem resolvable by the rsrc loader)."),
					"context":     map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Optional caller-supplied substitution values for variables declared in the playbook's frontmatter."},
					"session_key": stringProperty("Optional caller's ws session key. When provided, prompt override-points resolve against the session's layered config; omit to render seed defaults."),
				},
				"required": []string{"name"},
			},
		},
		{
			"name":        "playbook.render",
			"description": "Render a playbook to a worktree-scoped tmp file and return its path plus stable recommended-tier metadata when the playbook declares a tier (or tier_override supplies one), with optional config-resolved recommended-model/recommended-reasoning-effort bindings. Lead callers receive a render-minted child session key spliced into the rendered body. A top-level lead render of delegate-implementer also reserves an empty write-once plan file under the worktree's ai-docs/.plans/, named from context plan_stem (default \"delegate\"), ends the rendered body with its path, and returns it as a final plan-path: line. Available in both full and agentless product modes.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key":   stringProperty("Caller's ws session key (required for root resolution; lead callers trigger child-key minting)."),
					"name":          stringProperty("Playbook name (bare stem resolvable by the rsrc loader)."),
					"context":       map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": "Optional caller-supplied substitution values for variables declared in the playbook's frontmatter. In wsflow no-agent mode, legacy render-eligible stems append context as a ## Render Context block instead. For delegate-implementer, the plan_stem key names the reserved plan and is not a template variable."},
					"root_override": stringProperty("Optional delegate worktree path for prompt artifact allocation and child-key binding. The path resolves to its Git worktree root with symlinks resolved, and the child key binds to that resolved path. Playbooks, manifests, and includes use the plugin resource root (WS_RSRC_ROOT or the executable-derived bundle)."),
					"tier_override": stringProperty("Optional capability tier (small/medium/large/xlarge) that replaces the playbook's own frontmatter tier for this render only, driving both the in-body RoleModel substitution and the recommended-tier/recommended-model/recommended-reasoning-effort payload lines. Absent, render uses the frontmatter tier unchanged. An unknown tier is rejected, not coerced."),
				},
				"required": []string{"name"},
			},
		},
	}
	toolList = append(toolList, sessionHireTools()...)
	toolList = append(toolList, piToolDefinitions()...)
	return withSessionKeyToolSchemas(append(toolList, agentToolDefinitions()...))
}

func withSessionKeyToolSchemas(toolList []map[string]any) []map[string]any {
	for _, tool := range toolList {
		name, _ := tool["name"].(string)
		if !toolSchemaRequiresSessionKey(name) {
			continue
		}
		schema, _ := tool["inputSchema"].(map[string]any)
		if schema == nil {
			schema = map[string]any{"type": "object"}
			tool["inputSchema"] = schema
		}
		properties, _ := schema["properties"].(map[string]any)
		if properties == nil {
			properties = map[string]any{}
			schema["properties"] = properties
		}
		properties["session_key"] = stringProperty("Caller's ws session key (see ws:workflow-manual).")
		if name != "playbook.render" {
			schema["required"] = appendRequiredString(schema["required"], "session_key")
		}
	}
	return toolList
}

func toolSchemaRequiresSessionKey(name string) bool {
	switch name {
	case "api.list",
		"exec.spawn", "exec.shell", "exec.status", "exec.result", "exec.abort", "exec.raw.tail", "exec.raw.read", "exec.raw.grep",
		"git.status", "git.diff", "git.log", "git.merge_base", "git.commit", "git.merge",
		"rationale.query", "git.followups",
		"project_tree",
		"review.marker", "review.stamp",
		"worktree.acquire", "worktree.release", "worktree.remove", "worktree.list",
		"tickets.query", "tickets.close", "tickets.move", "tickets.create_empty", "tickets.sage_gate", "tickets.sage_stamp", "tickets.verify",
		"tickets.acquire", "tickets.release", "tickets.index_init", "path.generate", "playbook.render":
		return true
	default:
		if entry, ok := configKeyEntryForTool(name); ok {
			return entry.RequiresLeadAuthority
		}
		return false
	}
}

func appendRequiredString(raw any, value string) []string {
	required, _ := raw.([]string)
	for _, existing := range required {
		if existing == value {
			return required
		}
	}
	return append(required, value)
}

func execLaunchSchema(shell bool) map[string]any {
	props := map[string]any{
		"working_dir": stringProperty("Command working directory. Defaults to the resolved ws worktree root; relative paths resolve beneath that root."),
		"env":         map[string]any{"type": "object", "additionalProperties": map[string]string{"type": "string"}, "description": "Environment variable overlay."},
		"stdin":       stringProperty("Text stdin for the process."),
	}
	required := []string{"cmd"}
	if shell {
		props["command"] = stringProperty("Shell command string to execute.")
		props["shell"] = stringProperty("Optional explicit shell executable. Defaults to the platform shell.")
		required = []string{"command"}
	} else {
		props["cmd"] = stringProperty("Executable argv0 to run; not a shell command line.")
		props["args"] = stringArrayProperty("Optional argv arguments.")
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func execKeySchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"exec_key": stringProperty("Durable exec job key.")}, "required": []string{"exec_key"}}
}

func execResultSchema() map[string]any {
	s := execKeySchema()
	props := s["properties"].(map[string]any)
	props["timeout_seconds"] = numberProperty("Maximum seconds to wait for a running job to become terminal. Omit or set 0 for non-blocking behavior.")
	return s
}

func execRawTailSchema() map[string]any {
	s := execKeySchema()
	props := s["properties"].(map[string]any)
	props["stream"] = enumStringProperty("Stream to read. Defaults to stdout.", []string{"stdout", "stderr", "combined"})
	props["lines"] = integerProperty("Number of tail lines. Defaults to 40 and is capped.")
	return s
}
func execRawReadSchema() map[string]any {
	s := execKeySchema()
	props := s["properties"].(map[string]any)
	props["stream"] = enumStringProperty("Stream to read. Defaults to stdout.", []string{"stdout", "stderr", "combined"})
	props["offset"] = integerProperty("Byte offset. Defaults to 0.")
	props["limit"] = integerProperty("Maximum bytes to read. Defaults to 4096 and is capped.")
	return s
}
func execRawGrepSchema() map[string]any {
	s := execKeySchema()
	props := s["properties"].(map[string]any)
	props["stream"] = enumStringProperty("Stream to search. Defaults to stdout.", []string{"stdout", "stderr", "combined"})
	props["pattern"] = stringProperty("Search pattern. Literal unless regex is true.")
	props["before"] = integerProperty("Context lines before each match.")
	props["after"] = integerProperty("Context lines after each match.")
	props["max_matches"] = integerProperty("Maximum matches to return.")
	props["regex"] = boolProperty("Treat pattern as a regular expression when true.")
	s["required"] = []string{"exec_key", "pattern"}
	return s
}

func stringProperty(description string) map[string]string {
	return map[string]string{
		"type":        "string",
		"description": description,
	}
}

func nullableStringProperty(description string) map[string]any {
	return map[string]any{
		"type":        []string{"string", "null"},
		"description": description,
	}
}

func stringArrayProperty(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]string{
			"type": "string",
		},
	}
}

func enumStringProperty(description string, values []string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
		"enum":        values,
	}
}

// enumStringOrArrayProperty returns an inputSchema property accepting either
// a single enum string or an array of that same enum, via "anyOf" — the
// shape note.query's "layer" argument needs (optional, single-or-array)
// and no existing helper here covers, since every other enum property in
// this file is single-shape only.
func enumStringOrArrayProperty(description string, values []string) map[string]any {
	return map[string]any{
		"description": description,
		"anyOf": []any{
			enumStringProperty("", values),
			map[string]any{
				"type":  "array",
				"items": enumStringProperty("", values),
			},
		},
	}
}

func nullableEnumStringProperty(description string, values []string) map[string]any {
	enumValues := make([]any, 0, len(values)+1)
	for _, value := range values {
		enumValues = append(enumValues, value)
	}
	enumValues = append(enumValues, nil)
	return map[string]any{
		"type":        []string{"string", "null"},
		"description": description,
		"enum":        enumValues,
	}
}

// scopeSchemaProperty returns the shared MCP inputSchema fragment for the
// scope argument consumed by every scope-aware config tool. Callers embed this
// into their tool's properties map under the "scope" key. The fragment uses the
// canonical scope enum from wsconfig so the definition stays in one place.
func scopeSchemaProperty() map[string]any {
	return enumStringProperty(
		`Optional config scope to target. Omit to use the item's declared default scope (usually "project"). `+
			`"session" stores in the current session only; "project" stores in the per-project config; `+
			`"global" stores in the cross-project ~/.ws/config.json.`,
		wsconfig.ScopeSchemaEnum(),
	)
}

func numberProperty(description string) map[string]string {
	return map[string]string{
		"type":        "number",
		"description": description,
	}
}

func integerProperty(description string) map[string]string {
	return map[string]string{
		"type":        "integer",
		"description": description,
	}
}

func boolProperty(description string) map[string]string {
	return map[string]string{
		"type":        "boolean",
		"description": description,
	}
}

// stringOrObjectProperty returns an inputSchema property accepting either a
// string or a free-form object via "anyOf" — config.tune's polymorphic value,
// a string for scalar knobs and prompt text and an object for agents.tier.
// The type is spelled out rather than omitted: model-server tool-call parsers
// fall back to emitting a JSON-encoded string for a parameter with no declared
// "type", so an untyped value delivered agents.tier's object as a string. The
// "anyOf" form is used over a type array because some provider schema
// converters accept only the former. The object branch stays bare; field shape
// is validated server-side.
func stringOrObjectProperty(description string) map[string]any {
	return map[string]any{
		"description": description,
		"anyOf": []any{
			map[string]any{"type": "string"},
			map[string]any{"type": "object"},
		},
	}
}

// objectProperty describes a free-form JSON object parameter (no fixed property
// schema), used for arbitrary payloads such as ws.agenda.set value blobs.
func objectProperty(description string) map[string]any {
	return map[string]any{
		"type":        "object",
		"description": description,
	}
}

// objectArrayProperty describes an array-of-objects parameter without pinning a
// per-item schema, used for typed enter-payload lists such as active_agents.
func objectArrayProperty(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       map[string]any{"type": "object"},
	}
}
