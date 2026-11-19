package mcp

import (
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wslive"
)

// The shared descriptions of the session_key and format input properties.
const (
	sessionKeyPropertyDescription = "Your ws session key."
	formatPropertyDescription     = `"json" for structured output.`
)

func tools() []map[string]any {
	toolList := []map[string]any{
		{
			"name":        "runtime.read",
			"description": "Return ws-mcp runtime metadata for compatibility checks.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"format": stringProperty(formatPropertyDescription),
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
					"format":       stringProperty(formatPropertyDescription),
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
					"format":             stringProperty(formatPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
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
					"format": stringProperty(formatPropertyDescription),
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
					"format":        stringProperty(formatPropertyDescription),
					"session_key":   stringProperty("Optional lead session key. When supplied, resolved override scopes and session-scope prompt overrides are included in the output."),
					"root_override": stringProperty("Optional rsrc root override (test/advanced use); when omitted the shipped rsrc tree is scanned."),
				},
			},
		},
		{
			"name":        "config.tune",
			"description": "Write one ws config knob by key. Call config.list first: it gives each key's value domain, scopes, and whether it varies by harness. value is a string for scalar knobs, {tier, backend, model, effort} for agents.tier, and a profile object for worktree.profile.<name>. scope repo writes the committed .ws-workflow/config.json unstaged: add the reported path to your next git.commit. Lead-only.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key":         stringProperty("Knob key, e.g. workflow.prefer_subagent, agents.tier, prompt.<pointId>, worktree.profile.<name>."),
					"value":       stringOrObjectProperty("New value (see the description)."),
					"scope":       enumStringProperty("Storage scope; defaults to the key's declared scope.", wsconfig.ScopeSchemaEnum()),
					"harness":     stringProperty("For keys that vary by harness: claude, codex, pi, or *. Defaults to this session's harness."),
					"reset":       boolProperty("Drop the override back to the default; agents.tier also takes value {tier}, other keys no value."),
					"session_key": stringProperty("Your ws session key; required for lead-authority and prompt.* keys and for scope session."),
				},
				"required": []string{"key"},
			},
		},
		{
			"name":        "config.get",
			"description": "Resolve config keys through session (including parents), project, repo, global and builtin scopes. Supply key or nonempty unique keys. Bulk JSON returns ordered per-key results; errors do not fail siblings. Manifest keys return typed values, falling back to defaults with a warning on invalid values; other keys return strings or unset. Text by default; format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key":         stringProperty("Legacy single config key to read; mutually exclusive with keys."),
					"keys":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1, "uniqueItems": true, "description": "Config keys to resolve independently in request order; mutually exclusive with key. Entries must be nonblank and unique."},
					"session_key": stringProperty("Optional ws session key. When supplied, session-scope values (including those inherited from the session's parent chain) and the session's committed repo scope take part in resolution."),
					"format":      stringProperty(formatPropertyDescription),
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
					"format":  stringProperty(formatPropertyDescription),
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
					"format": stringProperty(formatPropertyDescription),
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
					"format": stringProperty(formatPropertyDescription),
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
					"format":       stringProperty(formatPropertyDescription),
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
					"format":    stringProperty(formatPropertyDescription),
				},
			},
		},
		{
			"name":        "rationale.query",
			"description": "Search commit rationale and ticket decisions by text, paths, site or pickaxe. Results group quotes by ticket thread.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":        stringProperty("BM25 query; combines with addressing."),
					"paths":        stringArrayProperty("Globs matching commit, ticket or thread paths."),
					"site":         stringProperty("git log -L site: path:/regex/,+N (default +10) or path#Lstart-Lend. Exclusive with paths and pickaxe."),
					"occurrence":   integerProperty("Site regex match; default 1."),
					"pickaxe":      stringProperty("git log -S string. Exclusive with paths and site."),
					"since":        stringProperty("YYYY-MM-DD, inclusive."),
					"until":        stringProperty("YYYY-MM-DD, inclusive."),
					"stems":        stringArrayProperty("Limit to these ticket threads."),
					"exclude_stem": stringProperty("Exclude ticket records and commits naming only this stem."),
					"kinds":        stringArrayProperty("commit, ticket. Default both."),
					"order":        enumStringProperty("Default relevance with query, else time.", []string{"relevance", "time"}),
					"limit":        integerProperty("Default 30, max 100."),
					"format":       stringProperty(formatPropertyDescription),
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
					"format": stringProperty(formatPropertyDescription),
				},
				"required": []string{"base", "head"},
			},
		},
		{
			"name":        "git.merge",
			"description": "Lead-only. Merge a local branch into a target with --no-ff; needs a clean index, leaves the target checked out. Deletes merged impl/* and goal/* sources. A main/master target needs release_target_override plus the OIDs a refusal reports. Conflicts stay for lead-delegate. Relay size advisories; never split files.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"branch":                  stringProperty("Source; defaults to current branch. impl/* must be impl/root/stem."),
					"target":                  stringProperty("Target; defaults to source impl/root/stem's root, else required."),
					"release_target_override": map[string]any{"type": "boolean", "default": false, "description": "Acknowledge main/master policy; requires both OIDs, never waives must-resolve findings."},
					"expected_source_oid":     stringProperty("The refusal's source_oid."),
					"expected_target_oid":     stringProperty("The refusal's target_oid."),
					"title":                   stringProperty("Merge commit title."),
					"description":             stringProperty("Merge commit description."),
					"ai_context":              stringArrayProperty("AI Context bullets for the merge commit."),
					"updated_tickets":         stringArrayProperty("Ticket update summaries."),
					"format":                  stringProperty(formatPropertyDescription),
				},
				"required": []string{"title", "ai_context"},
			},
		},
		{
			"name":        "git.commit",
			"description": "Workflow-aware commit of explicit paths.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"paths":           stringArrayProperty("Only these paths are staged."),
					"title":           stringProperty("Commit title."),
					"description":     stringProperty("Body before AI Context."),
					"ai_context":      stringArrayProperty("AI Context bullets."),
					"updated_tickets": stringArrayProperty("Omit to detect staged ticket moves and Result/Edition headings."),
					"followups": map[string]any{
						"type":        "array",
						"description": "Root lead only: ownerless future work; IDs return in order.",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"level":    enumStringProperty("Ascending severity.", wsgit.FollowupLevels),
								"category": enumStringProperty("Closed category.", wsgit.FollowupCategories),
								"content":  stringProperty("One-line summary and location."),
							},
							"required": []string{"level", "category", "content"},
						},
					},
					"resolves":        stringArrayProperty("Follow-up IDs to resolve; unknown IDs warn."),
					"expected_branch": stringProperty("Branch you expect, from memory, not HEAD. A mismatch or detached HEAD stages nothing."),
					"format":          stringProperty(formatPropertyDescription),
				},
				"required": []string{"paths", "title", "ai_context", "expected_branch"},
			},
		},
		{
			"name":        "worktree.acquire",
			"description": "Lead-only. Provision a pooled Git worktree of a named profile for a parallel worker: reuse an idle one of that profile or create one, create target_branch on base when absent and check it out, reset the tree, run the profile's hooks, and mint a worker session key bound to it. Refused when the worktree would hide the ticket board its commit tracks; a failing hook quarantines the worktree and fails the call. A result nudge lists stale pooled worktrees: relay it to the user, who decides on worktree.remove. Profiles: builtins full and housekeeping (ai-docs only), plus worktree.profile.<name> (config.list). Text by default.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"base":          stringProperty("Commit-ish the branch is created on."),
					"target_branch": stringProperty("Branch to create (when absent) and check out in the worktree, e.g. impl/<parent>/<slug>. Omit to check out base itself; base must then be a local branch not checked out in another worktree."),
					"profile":       stringProperty("Worktree profile name, e.g. full or housekeeping, or a project profile config.list lists. A pooled worktree is reused only by an acquire naming the same profile."),
					"format":        stringProperty(formatPropertyDescription),
				},
				"required": []string{"base", "profile"},
			},
		},
		{
			"name":        "worktree.release",
			"description": "Lead-only. Return a worktree to the pool by path or worker key: run pre_release hooks, reset --hard and clean -ffdx (discarding uncommitted and unpreserved ignored files), detach HEAD, record preserved size and remove its lease. Hook failure leaves it leased. Does not delete the worktree or retire the key. Refuses another holder's lease or a dirty unleased worktree unless forced. Text by default; format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   stringProperty("Absolute worktree path to release. Provide path or key."),
					"key":    stringProperty("A worker session key bound to the worktree to release. Provide path or key."),
					"force":  boolProperty("Release another holder's lease or a dirty unleased worktree. Confirm the owner is gone or agreed first; uncommitted work is discarded."),
					"format": stringProperty(formatPropertyDescription),
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
					"format": stringProperty(formatPropertyDescription),
				},
			},
		},
		{
			"name":        "worktree.list",
			"description": "Lead-only, read-only. List the pooled worktrees worktree.release accepts (the primary worktree and foreign worktrees are excluded; records whose directory is gone are skipped) with facts per worktree: path; branch, or detached (JSON branch null); dirty or clean, counting untracked files; HEAD's commit time; the newest mtime over the paths git status reports (dirty only); and the worktree lease written by worktree.acquire — worker_key, parent_key, acquired_at, and each key record's mtime, or record missing (JSON null) when the record was pruned — or no lease record (JSON worktree_lease null) when the worktree was released or acquired before worktree leases existed. Key records are read without refreshing their mtime. Facts only: it gives no stale or alive verdict; judging whether a holder is done is the reader's call. A clean entry carries a release nudge; a dirty entry carries none. Defaults to text; use format=json for structured output.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"format": stringProperty(formatPropertyDescription),
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
			"description": "Write notes to worktree (ephemeral), clone (project, untracked), machine, or repo (git-tracked) layer. Clone and machine require a reason. Notes persist until erased; session state belongs in agenda.set. Each key is overwritten including priority; higher priority surfaces first.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty(sessionKeyPropertyDescription),
					"layer":       enumStringProperty(`Layer: worktree by default; clone or machine only for facts needed across worktrees or projects; repo is git-tracked under ai-docs/ws-notes/.`, []string{"worktree", "clone", "machine", "repo"}),
					"reason":      stringProperty(`Required when layer is "clone" or "machine", rejected as missing when empty: why a session in another worktree (clone) or another project (machine) would fail without this note. Checked only; not stored.`),
					"notes":       objectArrayProperty(`Notes to write, each {"key": string, "value": string, "priority": integer}. priority is an integer defaulting to 0; a larger number is surfaced earlier (5 before 1).`),
					"format":      stringProperty(formatPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
					"layer":       enumStringProperty(`Which layer to erase from: "machine", "worktree", "clone", or "repo".`, []string{"machine", "worktree", "clone", "repo"}),
					"keys":        stringArrayProperty("Note keys to erase. A missing key is a no-op."),
					"format":      stringProperty(formatPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
					"layer":       enumStringProperty(`Which layer to mute on: "machine", "worktree", "clone", or "repo".`, []string{"machine", "worktree", "clone", "repo"}),
					"keys":        stringArrayProperty("Note keys to mute. A missing key is a no-op."),
					"format":      stringProperty(formatPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
					"layer":       enumStringProperty(`Which layer to unmute on: "machine", "worktree", "clone", or "repo".`, []string{"machine", "worktree", "clone", "repo"}),
					"keys":        stringArrayProperty("Note keys to unmute. A missing key is a no-op."),
					"format":      stringProperty(formatPropertyDescription),
				},
				"required": []string{"session_key", "layer", "keys"},
			},
		},
		{
			"name":        "note.query",
			"description": "Query notes by key glob and written_at range, including bodies hidden by workflow_manual. A single layer returns untagged records; an array or omitted layer returns layer-tagged records. Ordered by descending priority, newest written_at, then key.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty(sessionKeyPropertyDescription),
					"layer":       enumStringOrArrayProperty(`Layer name or array; omit to search all. A single string returns untagged records; array or omission tags each layer.`, noteLayerEnumValues),
					"glob":        stringProperty(`Optional key glob (path.Match syntax, e.g. "ticket.*"). Omit or "*" to match every key.`),
					"from":        stringProperty("Optional inclusive lower bound on written_at (RFC3339 or a date prefix such as \"2026-08-01\")."),
					"then":        stringProperty("Optional inclusive upper bound on written_at (RFC3339 or a date prefix)."),
					"format":      stringProperty(formatPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
					"payload":     objectProperty("Defined by the tracker playbook."),
					"format":      stringProperty("\"json\" for the store's whole view state; text when omitted."),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        wikiQueryTool,
			"description": "Read tracker wikis (Markdown or .html) and history; top-level lead or store support only. Pass read's commit as base to wiki.update. Link as [text](wiki:<name>/<path>). Text by default.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty(sessionKeyPropertyDescription),
					"store":       stringProperty("Tracker store ID (tracker.query lists them)."),
					"op":          enumStringProperty("list wikis; pages in wiki; read page and commit; search lines (case-insensitive literal); history commits; diff path in to; recovery replaced tips.", wikiQueryOps),
					"wiki":        stringProperty("Wiki name; required by every op but list and recovery."),
					"path":        stringProperty("Page path, folders joined by / (an .html suffix names an HTML page). Required by read and diff."),
					"commit":      stringProperty("Read at this commit instead of the tip."),
					"query":       stringProperty("search: the text to find."),
					"from":        stringProperty("diff: the commit to diff from; defaults to the parent of to."),
					"to":          stringProperty("diff: the commit whose change is shown."),
					"limit":       integerProperty("history (default 30) or search: at most this many results."),
					"format":      stringProperty(formatPropertyDescription),
				},
				"required": []string{"session_key", "store", "op"},
			},
		},
		{
			"name":        wikiUpdateTool,
			"description": "Save one signed wiki edit (top-level lead or store support). Pass wiki.query read's commit as base. Overlapping later saves conflict, returning both texts and the tip; others merge.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty(sessionKeyPropertyDescription),
					"store":       stringProperty("Tracker store ID."),
					"wiki":        stringProperty("Wiki name: [a-z0-9][a-z0-9-]*, max 64."),
					"op":          enumStringProperty("Operation on the page.", wikiUpdateOps),
					"base":        stringProperty("Commit from read or last save; required unless new_wiki."),
					"new_wiki":    boolProperty("Create the wiki; no base."),
					"path":        stringProperty("Page path; .html makes a sandboxed HTML page."),
					"to":          stringProperty("rename: new path; suffix sets kind."),
					"content":     stringProperty("write: page content."),
					"commit":      stringProperty("restore: source commit."),
					"message":     stringProperty("Commit message."),
					"images": map[string]any{
						"type":        "array",
						"description": "write: PNG/JPEG/GIF/WebP; link as ![alt](attachment:sha256).",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"data": stringProperty("Base64."),
								"name": stringProperty("Label."),
							},
							"required": []string{"data"},
						},
					},
					"format": stringProperty(formatPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
					"to":          stringProperty(`Recipient address: "<slug>@<scope>" or "id:<reply-id>".`),
					"content":     stringProperty("Message body."),
					"format":      stringProperty(formatPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
					"format":      stringProperty(formatPropertyDescription),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        "mailbox.lookup_peers",
			"description": `List registered peers in one scope, this session's correspondents across scopes, and self. Self is the active WS_MAILBOX identity or the session reply-id (opening its return channel). Peers are up to 20 named inboxes by last_seen, including offline ones; offline auto-minted names are omitted and simultaneous name conflicts marked. Correspondents are up to 20 by last_exchange, with harness/cwd when known; their history survives compaction.`, 
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key": stringProperty(sessionKeyPropertyDescription),
					"scope":       enumStringProperty(`Which scope to list: "machine", "worktree", or "clone".`, []string{"machine", "worktree", "clone"}),
					"format":      stringProperty(formatPropertyDescription),
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
					"session_key": stringProperty(sessionKeyPropertyDescription),
					"limit":       integerProperty("Optional maximum number of entries. Defaults to 10; a value below 1 uses the default."),
					"body":        boolProperty("Optional. true includes each entry's full body; defaults to false (title only)."),
					"direction":   enumStringProperty(`Optional filter: "recv", "sent", or "all" (default).`, []string{"recv", "sent", "all"}),
					"format":      stringProperty(formatPropertyDescription),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        dashboardIntakeTool,
			"description": `Adapter-only: agents must not call. Applies ordered events to the session's dashboard state; each has a kind and optional at (epoch ms). Kinds: reset (clears live state; send at adapter start); turn_start {origin?}; turn_end {reason?}; agent {agentId?, parentId?, label?, sessionKey?, status?, caption?, batches?, model?, effort?, contextTokens?, outputTps?} updates the lead or one subagent; permission {agentId?, tool, args} or {agentId?, clear: true}; reply {text, final?} replaces the reply so far, final moves it to the conversation; user {text, origin?}; status {context?, limits?, cost?}; compact {phase: start|end|failed, trigger?}; commands {commands: [{name, description?, source?, layer?}]}; command {command, text?}. Send replies at most 10 times a second. With the dashboard off nothing is recorded. Send status, compact, commands or command only while the latest result's accepts line names it.`,
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
			"description": `Adapter-only: agents must not claim messages. Atomically claim held messages, steering first then oldest; claimed messages cannot be cancelled or steered. Inject each ws:user envelope as a user message. Run command entries natively, splitting the batch in order. steer_only claims steering at a running turn's tool boundary; interrupt_only claims text-free interrupts: stop a turn started before sent_at, discard older interrupts. Full drain drops interrupts. Works with dashboard off. Text by default; format=json for structured messages.`, 
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key":    stringProperty("The adapter's session key: the ws session key of the session it feeds."),
					"steer_only":     boolProperty("When true, claim only the messages promoted to steering; every other message stays held."),
					"interrupt_only": boolProperty("When true, claim only the interrupt entries (the user's requests to stop the running turn); every message stays held. Cannot be combined with steer_only."),
					"format":         stringProperty(formatPropertyDescription),
				},
				"required": []string{"session_key"},
			},
		},
		{
			"name":        "tickets.query",
			"description": "Find tickets. ticket_stem alone resolves one in any status or errors; other arguments run discovery, paginated by offset and limit. Flags unmet blocked-by (single-ticket only), advisory ## Blocked, and enabled assignee and lease.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"statuses":             stringArrayProperty("Statuses to scan: ready, todo, idea, done, dropped."),
					"include_done":         boolProperty("Discovery also scans .done/."),
					"include_dropped":      boolProperty("Discovery also scans .dropped/."),
					"query":                stringProperty("Case-insensitive text query."),
					"ticket_stem":          stringProperty("Exact stem; given alone, resolves that ticket."),
					"mentions_ticket_stem": stringProperty("Ticket stem results must mention."),
					"assigned_to_me":       boolProperty("Drop tickets assigned to someone else (assignee flag on)."),
					"unleased_or_mine":     boolProperty("Keep tickets leased by you or by no one; drop ones closed on origin (ownership index on)."),
					"offset": map[string]any{
						"type":        "integer",
						"description": "Discovery page offset.",
						"default":     0,
						"minimum":     0,
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Discovery page size.",
						"default":     50,
						"minimum":     1,
						"maximum":     200,
					},
					"format": stringProperty(formatPropertyDescription),
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
					"format":    stringProperty(formatPropertyDescription),
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
			"description": "Move a ticket along idea <-> todo <-> ready; stages atomically, does not commit. Ready rejects epic, research and workset tickets (board artifacts, never execution targets) and tickets with an open ## Open Decision Queue. Ready promotion and epic todo settlement resolve sage-review posture from config; actionable todo is ungated. With the ownership index, a lease held by another person refuses unless dangerously_override_lease_status is true with a reason; a lease held by another clone or track warns; the lease never moves.",
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
			"description": "Create a dated empty ticket stub with title and applicable sage-review posture; populate via tickets.template. With assignee-aware flag, set_assignee defaults to caller's git email; false leaves unassigned, array assigns emails. Warns if outside sparse-checkout scope (git drops it after commit). Returns path and promotion tip; does not stage or commit.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"stem":          stringProperty("Semantic ticket stem without date prefix (e.g. feat-foo-bar). Authoring categories: feat, bug, refactor, chore, research, epic."),
					"initial_state": stringProperty("Ticket status: idea, todo, or ready."),
					"set_assignee": map[string]any{
						"description": "When assignee-aware: true (default) stamps caller's git email if set; false leaves unassigned; array assigns these emails.",
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
			"description": "Resolve the sage-review gate for a ticket landing; does not spawn reviewers. The epic todo landing runs design-only review on lead judgment when cross-child design has drifted, not at a status boundary; actionable todo calls fail; research and legacy worksets skip. Ready promotion runs actionable design and completeness review. Returns an action: skip, stop_blocked, stop_missing_route_facts, stop_open_decision_queue, ask or run (with reviewers and mode). Ready needs a ## Route Facts section; ready and epic todo are refused, before any write, while an open decision queue exists outside fenced code.",
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
			"description": "Lease a ticket to your track in the origin ticket ownership index; the track is the current branch's merge root (a rootless impl branch needs track; a detached HEAD is refused). Refused for a ticket already in .done/ or .dropped/ on origin's review track (pull first), and for a lease held by another email or another track of this clone, unless the user explicitly asked for dangerously_override_lease_status. A same-email lease on another clone moves to you with a warning. Offline, the cached index decides and the success is pending. Without the index this is a plain ok. Text by default.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ticket_stem":                       stringProperty("Ticket stem (YYMMDD-category-name). The ticket file must exist in this checkout."),
					"track":                             stringProperty("Optional explicit track (work line) to record, for branches with no merge root."),
					"dangerously_override_lease_status": boolProperty("Take over a lease the ownership rules refuse. Set only on the user's explicit instruction; requires reason."),
					"reason":                            stringProperty("Why the lease is overridden; recorded in the index history. Required with dangerously_override_lease_status."),
					"format":                            stringProperty(formatPropertyDescription),
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
					"format":      stringProperty(formatPropertyDescription),
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
					"format": stringProperty(formatPropertyDescription),
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
					"format": stringProperty(formatPropertyDescription),
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
			"description": "Render a playbook to a worktree-scoped tmp file and return its path, its recommended tier when the playbook or tier_override names one, and the config-resolved model and effort for it. A lead caller's render splices a minted child session key into the body. A top-level lead render of delegate-implementer also reserves an empty plan file under ai-docs/.plans/ and returns its path as a final plan-path: line.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"session_key":   stringProperty(sessionKeyPropertyDescription),
					"name":          stringProperty("Playbook name (bare stem)."),
					"context":       map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}, "description": `Values for the playbook's frontmatter variables. For delegate-implementer, plan_stem names the reserved plan (default "delegate").`},
					"root_override": stringProperty("Delegate worktree path: the tmp file and child key bind to its Git worktree root."),
					"tier_override": stringProperty("small, medium, large, or xlarge: replaces the playbook's tier for this render. Unknown tiers are rejected."),
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
		properties["session_key"] = stringProperty(sessionKeyPropertyDescription)
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
