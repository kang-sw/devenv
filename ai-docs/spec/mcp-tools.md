---
title: MCP Tools
summary: Host-neutral ws MCP tool contracts for project context, workflow state, Git, documentation, and named-agent orchestration.
---

# MCP Tools

The ws MCP server exposes workflow capabilities through named MCP tools rather
than host-specific shell commands or repository-local paths. Tool outputs are
plain MCP text content that callers can use from Codex, Claude, or another
MCP-capable host.

This spec owns stable caller-visible behavior, not a copied tool schema
inventory. The runtime-owned MCP registry and `tools/list` response own input
schemas, and `runtime capabilities` owns the launcher-facing surface inventory.
When those runtime-discoverable fields change without changing caller-visible
behavior, update code and tests rather than copying field lists into this spec.

## MCP Server Protocol Surface {#260505-mcp-server-protocol-surface}

The `ws-mcp serve --stdio` process implements a stdio JSON-RPC MCP server. It
responds to `initialize`, `ping`, `tools/list`, and `tools/call`, advertises MCP
protocol version `2025-03-26`, and declares tool capability. A `ping` request
preserves its JSON-RPC id and returns an empty result object; it is a base-
protocol method and is not advertised as a tool.

Unknown methods and profile-rejected tools return JSON-RPC errors. Tool-level
runtime failures return MCP text content with `isError: true`, preserving a
normal MCP response envelope while still making the failure visible to callers.

Setup calls are request-order fences. When `ws.setup` or the advertised setup
alias appears in the stdio stream, the server completes earlier in-flight
requests, applies setup synchronously, writes that setup response, and only then
accepts later requests from the same stream. This preserves batch-safe
setup-then-call behavior for session and actor state.

Read-only tools whose primary consumer is an LLM prefer compact readable text
defaults over JSON serialized into text content. Tools that need stable machine
parsing, launcher compatibility, or structured protocol metadata preserve an
explicit JSON or full-detail escape hatch. {#260512-mcp-llm-readable-output-defaults}

The MCP server detects the host harness from observable MCP payloads before
relying on environment variables. It inspects `initialize.params` and request
metadata for high-confidence Codex or Claude markers, treats
`tools/call.params._meta.x-codex-turn-metadata` as a Codex signal, and records
conflicting signals in diagnostics instead of silently changing the session
harness. The detected harness is exposed through session inspection output.
{#260508-mcp-payload-harness-detection}

## Runtime And Debug Metadata Tools {#260505-runtime-debug-metadata-tools}

`runtime.info` returns runtime compatibility metadata: the runtime version and
source commit. (Prompt bundle metadata was removed when the embedded prompt
bundle was retired in favor of the rsrc tree.) Launchers and workflow checks use
this output to detect stale or incompatible runtime binaries.
The default response is compact labeled text; callers that need stable fields
can request structured JSON.

`runtime.debug_events` returns recent in-process MCP debug events as JSONL. The
tool is bounded by an optional limit parameter and is intended for diagnosing MCP
server behavior without reading process-local files directly.

## MCP Session Root Defaults {#260505-mcp-session-default-root}

Root-aware MCP tools resolve their repository root exclusively from a mandatory
`session_key` argument; root resolution is the ephemeral session-auth model
(`#260610-ephemeral-session-auth-model`). There is no fallback chain. A root-aware
call without a `session_key` is rejected with `mandatory_session_key` guidance
that routes the lead to `ws:workflow-manual` (the recovery message names no tool,
per the bootstrap-name obscurity scrub); a call whose key has no record in the
session store is
rejected with the `unknown_session` recovery contract. Public schemas for
root-aware tools advertise `session_key` and do not advertise `root`;
`ferrule(root)` is the sole bootstrap verb and the only tool that accepts
a `root` argument.

The former resolution sources are removed: the explicit per-tool `root` argument,
the volatile session default root, host-workspace metadata, the explicit server
startup root, `WS_MCP_PROJECT_ROOT` as a resolution source, the `ws.setup` public
setup surface (both the bare root-session form and the
`lead-workflow-bootstrap` actor form), and the persistent actor / authority /
child-actor bootstrap. With root carried by a per-call key rather than a
process-global default field, concurrent distinct worktree roots resolve without
clobber and without the former request-order setup fence.

## Ephemeral Session-Auth Model {#260610-ephemeral-session-auth-model}

The former persistent actor / authority / child-actor model has been replaced by
an ephemeral, in-memory session-auth model. This is the caller-visible
authentication contract for ws tool calls.

A lead-centric bootstrap verb mints a session:
`ferrule(root) -> session_key`. The returned key is an LLM-friendly
word-chain string (for example `amber-tide-fox`), not a UUID. Only the lead logs
in; subagents and mercenaries never call login — they receive a render-minted key
(`#260610-mercenary-delegation-surface`).

Every ws tool call carries a session key (REST-bearer style). There is no keyless
fallback to a foreign root: a call without a valid key does not silently operate
on a server-default or lead root. This closes the wrong-tree footgun in which a
worktree delegate doing root-omitted calls silently mutated the lead's main
repository.

The server resolves `{session_key -> root context}` from a flat, filesystem-backed
store: one JSON record per key at `<cache-root>/keys/<session_key>.json`. It
replaces the process-global default-root field and the request-order setup fence,
so parallel requests each resolve their own root with no serialization and no
shared-field clobber. The file is the source of truth, not the process: keys are
minted with an `O_EXCL` create (atomic cross-process uniqueness) and updated with
temp-write + rename (no partial reads), and per-key sharding removes write
contention without a single shared file or SQLite. A fresh MCP server instance —
a subagent that did not inherit the lead's process, or a lead that restarted
mid-delegation — resolves a key by reading its file, so session continuity does
not depend on a shared in-memory registry.

`login` is a bootstrap verb only: there is no logout and no eviction (rows are a
tiny `(word-chain key, root path)` bounded by the number of distinct roots a
fleet touches).

Every keyed call honors an `unknown_session` recovery contract: when a key has no
record file (a genuinely unknown or path-unsafe key, or state cleared by deleting
the cache), the call is rejected with an `unknown_session` signal and the caller
re-logins with its own known root and retries. Because the caller-visible contract
(`login(root) -> key`; `<tool>(key, …)`; re-login-on-reject) hides the backend,
the move from the original in-memory map to this filesystem-backed store was a
pure implementation swap with no contract migration.

Key issuance accepts an optional capability/role-scope parameter
(`#260505-tool-profile-gating`), so the lead can mint capability-scoped keys for
delegates; the keyed `tools/call` handler enforces that scope (see Tool Profile
Gating).

> [!note] Constraints
> - The session key is mandatory on every ws call; there is no keyless lead
>   default. A delegate that drops its key gets `unknown_session`, not a silent
>   foreign-root operation.
> - The bootstrap tool (`ferrule`) is lead-only. It lives outside the
>   `lead.*` namespace, so the keyed-call handler blocks non-lead keys from it
>   by explicit name in addition to the `lead.*` prefix block
>   (`#260610-mercenary-delegation-surface`); a delegate cannot self-bootstrap or
>   escalate from a contained context. Re-bootstrap for recovery uses the caller's
>   own already-known root.
> - The bootstrap tool name is deliberately obscure (260617 obscurity, soft
>   guard): semantically disconnected from "session start" and taught only in
>   `ws:workflow-manual`. The three subagent-reachable surfaces must not leak it —
>   the `tools/list` description is inert, error-guidance strings name no tool and
>   route the lead to the manual, and the rendered delegate prompt carries a
>   capability-level instruction with no tool name. This lowers accidental/curious
>   invocation by subagents that share the lead's MCP connection; it is not a hard
>   barrier (a name-aware caller can still keyless-bootstrap).
> - The store is filesystem-backed (one record file per key under a flat `keys/`
>   directory) and survives a server restart. There is still no logout and no
>   automatic eviction, though deleting a key file is now a physically possible
>   removal path (deferred).

### Session-Key Lineage And Child Enumeration {#260619-session-key-lineage-children}

Session keys form a parent→child lineage so a lead can re-discover the keys it
minted after they fall out of its own (compacted or restarted) context.

- Each session record carries an optional `parent` — the session key that minted
  it. It is absent for a lead's first bootstrap key. Recording a parent is
  metadata only and never widens the child's capability scope.
- `ferrule` accepts an optional `parent_session_key`. A lead coordinating
  several repository roots in one conversation (for example multiple git
  worktrees, each a distinct root) records each additional control key's parent
  as its primary control key. Because `ferrule` is lead-only, control-key
  lineage stays within one lead — it does not create a tree of independent
  control agents.
- A render-minted delegate child key (`playbook.render`, including a
  worktree-bound leaf produced via `root_override`) records the dispatching lead
  key as its `parent`.
- `session.children(session_key, depth?, format?, include_dead?)` returns,
  read-only, the subtree of keys whose `parent` chain roots at the presented
  key. Each entry is labeled by its capability scope (control coordination key
  vs delegate leaf) and includes the child key string so the lead can re-thread
  it. A caller only ever sees the subtree under a key it presents. It lives in
  the `session.*` tool family, so the existing keyed-gate `session.` prefix block
  already restricts it to lead-scoped keys (a delegate/leaf key is rejected;
  those scopes mint no children anyway).
  - `depth`: integer, default `1` (immediate children); a higher value returns
    that many levels; `0` returns the full subtree.
  - Liveness is whether the child's bound `root` path still exists. Dead keys
    (such as a removed worktree's key) are filtered by default;
    `include_dead: true` returns them flagged `live: no`, preserving a
    prune/debug path.
  - Output defaults to compact labeled text per
    `#260512-mcp-llm-readable-output-defaults`; `format: "json"` is the
    structured escape hatch.

## Session State Tools {#260625-session-state-tools}

The session state machine persists routing and implementation context across
context compaction. After compaction the session key is the only stable anchor
that survives, so the state is stored as additive fields on the existing
per-session record file (`<cache-root>/keys/<session-key>.json`, the same file
`ferrule` mints) rather than in a separate store. Writes reuse the record's
atomic temp-write+rename path, so a concurrent reader never observes a partial
record; fields are omitted when empty and unknown fields are ignored, preserving
backward and forward compatibility.

Two namespaces share the record:

- **agenda** — named blobs recording session-level mode context ("what are we
  doing and why"). Reminded at workflow-manual load only, not at intermediate
  checkpoints.
- **todos** — an ordered step-level checklist. Injected at every restoration
  point (workflow-manual load and major checkpoints).

All session-state tools require a `session_key` and are reachable by any role
that holds one — they carry no `session.`/`config.`/`lead.` prefix, so the
keyed capability gate does not restrict them to the lead. This lets a lead
populate state and hand its key to a delegate that reads or extends it; a child
that mints its own key via `ferrule` has independent state.

**Agenda (freeform).** `agenda.set(key, value)` upserts an arbitrary JSON
object blob; `agenda.clear(key)` removes one (a missing key is a no-op), or
`agenda.clear(all: true)` removes every agenda blob for the session in one
call (`key` is ignored when `all` is set). `agenda.list(session_key)`
enumerates the session's current agenda keys, each with a short one-line
summary (an object blob's top-level keys, or a truncated raw preview for
non-object JSON), sorted alphabetically; an empty agenda reports `no agenda
blobs` rather than an empty list. These are the fallback primitives for modes
not covered by a typed enter tool, and let a caller discover or clear
orphaned blobs without guessing key names from tool descriptions.

**Enter (typed mode switches).** `enter.implement`, `enter.proceed`,
`enter.sprint`, and `enter.salvage` each perform one atomic write that both
stores the typed payload as an agenda blob (keyed by the mode name) and
**replaces** the entire todo list with items derived from the mode. Because the
list is replaced, calling any enter tool is always a mode switch; a prior mode's
derived list is discarded. Derivation logic lives in Go, so no skill-side
`todo.append` loop is needed for a covered mode:

- `implement`: `enter.implement` is the public mode-switch call for the
  implementation-facts-complete boundary. It accepts `session_key`, a required
  `target` object, optional grouped `facts.scope` / `facts.complexity` /
  `facts.risk` objects, a small `policy` object, and optional `format:
  text|json`. MCP observes Git branch state from the session root, including the
  current branch, HEAD/start commit, target branch existence, and
  upstream/tracking ambiguity; callers provide only policy that cannot be
  observed mechanically, such as a merge target while already on an
  implementation branch (`impl/*`, or legacy `implement/*`) and whether safe
  branch rename is allowed; branch rename defaults to allowed unless the
  caller explicitly withholds consent (`policy.branch.allow_rename: no`); and
  whether the caller's own merge-approval ask may be skipped for this merge;
  merge confirmation defaults to asking unless the caller explicitly passes
  `policy.branch.merge_confirm: skip`. The
  resolver derives
  `delegation`, `branch_plan`, `plan_depth`, `review_alloc`, `need_review`, and
  `doc_mode`, stores the implement agenda, and replaces the todo list with the
  derived lead-implement checklist. `plan_depth` is `none` for direct edit and
  `survey` for reachable delegated preparation; delegated preparation creates a
  plan path, renders `plan-populator-survey` to write the light implementation
  plan, and renders `plan-populator-research` on the same authority and plan
  path only when
  the survey returns `[escalate-to-research]` for low confidence or strategic
  uncertainty. Planner instructions identify ticket or inline authority and
  pass every declared render variable, using explicit empty values for the
  inactive authority. The derived todos carry focused `instruction` prose from
  those
  resolved verdict labels, so branch, prep, edit, review, doc, final-gate, and
  merge todos describe only the path reachable under the current verdict;
  branch-stop todos describe the blocker instead of telling the caller to
  continue source edits. Non-stop prep instructions carry required
  runbook-loading guardrails, including mental-model lookup, ancestor reads,
  conditional migration-anchor loading, and implementation-runbook loading
  before edits or delegate dispatch. Text output is the canonical raw verdict
  beginning `Implementation Verdict`, with `Mode`, `Branch Action`, `Plan Depth`,
  `Review Allocation`, `Doc Mode`, and a concrete `Next:` instruction; JSON
  output returns the structured result plus `next_instruction` and the identical
  `raw` string. A `Branch Action: stop` verdict is a safety blocker and must say
  what policy or branch condition needs correction before source edits continue
  without naming unreachable planner or implementer actions. When a caller
  supplies `policy.branch.merge_target` outside its applicability window (the
  observed current branch is not already an implementation branch, i.e. not
  prefixed `impl/` or legacy `implement/`, so the branch action is `create`),
  the verdict adds a one-line warning naming the supplied value and the branch
  it was derived from instead, e.g. `policy.branch.merge_target "master"
  ignored (not on an implementation branch: impl/*, or legacy implement/*);
  derived from current branch "test/wsflow-smoke"`, so a caller unfamiliar with
  the applicability rule sees that the field was read and deliberately not
  applied. Fresh implementation branches are created under the `impl/<stem>`
  convention, with `<stem>` <=15 characters recommended (trailing `-`
  trimmed); legacy `implement/<scope-slug>` branches already in progress are
  still recognized as implementation branches for continue/rename purposes.
  Automatic review allocation derives independent correctness, fit, and test
  partitions from material risk, contracts/public symbols, cross-module/reuse
  uncertainty, and new or unknown test surfaces. Public-interface surface and
  existing-test surface alone add no partition. Zero or one automatic partition
  resolves to `single`; `single` dispatches the delegate-grade generic
  `reviewer` over the shared full-scope `code-reviewer` contract, while two or
  more resolve to `partitioned`. Explicit review overrides remain authoritative.
  Final-action todo guidance may reuse
  passing full-suite evidence only while code, tests, dependencies, build
  configuration, and generated inputs remain unchanged; documentation-only
  commits run only affected checks. Documentation pre-pass guidance dispatches
  mental-model work only for new non-obvious invariants, reusable domain rules,
  or modification guidelines not already covered by the authoritative spec.
  `policy.low_ceremony_if_safe` accepts `yes|no|unknown` and defaults to
  `unknown`. It is a preference-only input: `yes` is necessary but not
  sufficient for `Branch Action: current`, while missing, `no`, and `unknown`
  retain the standard branch result. `current` is the no-merge result for an
  inline target on a named non-implementation branch only when the policy is
  `yes`, raw unoverridden facts satisfy the automatic direct-edit and automatic
  lead-only predicates, review override is `auto`, and documentation is skipped
  with a non-empty reason. Explicit direct-edit or lead-only overrides, unknown
  or failed predicates, detached HEAD, or a missing/`(initial)` start commit,
  ticket targets,
  and existing implementation branches cannot authorize the result. When a
  supplied `yes` is inapplicable, the resolver emits a concise warning and
  preserves independently derived delegation, review, documentation, standard
  branch, final-action, and merge behavior. A successful `current` result
  carries no merge target, renders merge confirmation as `n/a`, and installs
  route, prep, edit, lead-only-review, and completion todos. Completion requires
  focused verification, one logical explicit-path commit with `## AI Context`,
  retained branch and commit-range reporting, and no push; final-action and
  merge todos are absent only for this result.
- `proceed`: `enter.proceed` is the public mode-switch call for the
  routing-facts-complete boundary. It accepts `session_key`, a required
  `target` object, optional grouped `facts.ticket` / `facts.gates` /
  `facts.work` objects, and optional `format: text|json`. It normalizes the
  current proceed route vocabulary (`target-kind`, `ticket-missing`,
  `has-ticket`, `status`, `migration-anchor`, `actionable`,
  `discussion-needed`, `needs-ticket`, `freshness`, `category`, `slice`, and
  `scope-blocked`), resolves one deterministic route, emits non-blocking
  warnings for contradictory or inapplicable facts, stores the selected route
  agenda, and replaces the todo list with Build route context and Resolve MCP
  verdict. Text output is the canonical raw verdict
  beginning `Proceed Verdict`, `Route: ...`, `NEXT: ...`, and `Next: ...`; JSON
  output returns the structured result plus `next_instruction` and the identical
  `raw` string.
- `sprint` (`enter.sprint`): Edit, Verify, Commit, Post-edit decision, Wrap episode.
- `salvage` (`enter.salvage`): Containment, Survey fanout, Premise interview, Classification,
  Capture.

**Todo.** Item identity is a caller-provided `key`, unique within the active
list after normalization; keys are lowercased, may contain only lowercase
letters, digits, `.`, `_`, and `-`, and are rejected when they include leading or
trailing whitespace. Todo items persist `key`, `title`, `status`, and an
optional nullable `instruction` field for full execution prose. Existing session
records without `instruction` remain valid and read as `null`. A duplicate key is
rejected after normalization, and an erased key is reusable. Creation mutations
(`todo.append`, `todo.insert_before`, and `todo.insert_after`) accept optional
nullable `instruction` and reject non-string non-null values. Status and order
mutations do not rewrite untouched item payloads, so existing `instruction`
values are preserved through status and order changes. `todo.check` returns a
compact confirmation followed by a checkpoint todo rendering. Other status/order
mutations (`erase`, `clear`, `reorder`) return a compact confirmation.
`todo.read(key)` returns one item's full JSON payload, including
`instruction`. `todo.list` returns rendered text. `clear(done_only=false)`
removes all items; `done_only=true` removes only `done` items.
`reorder(span:{from_key,to_key}, position:{before|after: ref_key})` moves a
contiguous span as a block; the ref_key must lie outside the span.

Rendering lines include the visible key after the marker: `- [ ] {key} Title`,
`- [~] {key} Title`, `- [x] {key} Title`, or `- [>] {key} Title`. Summary mode
(the default list mode) shows every pending/wip item plus one adjacent context
item on each side of each contiguous active block, collapsing every other run to
a single `...` line with no synthetic key or checkbox marker; `defer` collapses
the same as `done`. When an item has a non-empty instruction, summary rendering
adds an indented second line containing at most the first 60 runes of that
instruction; absent, null, or empty instructions render no second line. Full
mode shows every item in order and renders each non-empty instruction in full on
the indented second line. Workflow manual restoration uses the same summary
rendering, so restored todos show the same 60-rune instruction previews.
Checkpoint rendering from `todo.check` shows the full ordered list without
ellipsis collapse after the status update. It renders full instruction lines only
for the checked item's immediate previous and next items when those items are
actionable (`pending` or `wip`) and have non-empty instructions; the checked item,
non-adjacent items, `done` items, `defer` items, and instruction-less items stay
compact. Compact checkpoint rows with a non-empty instruction that is not rendered
add an indented `...+` marker line to distinguish hidden instruction payloads from
instruction-less rows. `ws.commit` does not auto-mark todos; status transitions
are always explicit via `todo.check`.

### Workflow Manual Entry And Restoration {#260626-workflow-manual-restoration-entry}

`workflow_manual(session_key)` is the canonical workflow-manual entry tool. A
valid `session_key` is **required**, and the tool is **lead-only**
(`isLeadOnlyTool`): a `session_key` resolving to a delegate/leaf scope is rejected
at the keyed capability gate before the handler runs (mirroring `ferrule`). It
renders the `lead-workflow-manual` playbook through the same variable substitution
as `playbook.print` — the rsrc playbook stays the single prompt source of truth —
and branches on `session_key`:

- **fresh** (`session_key` equals the reserved fresh-bootstrap sentinel — a
  deliberately non-descriptive token taught only in lead skill prose such as
  `lead-revive`): two sub-cases based on whether the optional `root` parameter
  is supplied:
  - **fresh with root** (`root` is a non-empty absolute Git worktree path): the
    handler validates and canonicalizes the path via `canonicalSetupRoot` (same
    as `ferrule`), mints a new lead session key, strips the fresh-only
    self-bootstrap block from the manual body, and appends a `## Session Key`
    section containing the minted key followed by an empty `## Session State`
    section. A separate `ferrule` call is not needed; the caller can proceed
    directly to `project_tree`, `git.status`, and other keyed tools using the
    returned key.
  - **fresh without root** (sentinel, no `root`): the primitives reference plus
    the always-shown per-root rule (call `ferrule` once per working root and
    thread its key) and the self-bootstrap line ("you have no key yet; mint one
    with `ferrule`").
- **continue** (`session_key` present and its record resolves to a lead scope): the
  primitives plus the per-root rule, with the self-bootstrap line omitted, followed
  by a restored **Session State** section — agenda blobs as a remind list and the
  todo list in summary mode, rendered server-side from the session record.
- **keyless** (`session_key` omitted or empty): a hard error requiring a valid
  `session_key`. The error names neither the sentinel nor `ferrule`, so a
  keyless caller gets no bootstrap hint.
- **fail-loud** (`session_key` present, non-sentinel, but no record resolves): a
  minimal "no restorable state for this key" notice pointing to the `lead-revive`
  skill for recovery. **No manual body is rendered** — the always-shown per-root
  rule names `ferrule`, and any unregistered key bypasses the lead-only gate via
  lookup-miss, so rendering it would leak the lead self-bootstrap call to a non-lead
  caller. The tool never mints a key in this mode.

The fresh-only self-bootstrap line is delimited in the rsrc by a dedicated
mode-gating marker that only this tool's handler consumes; it is independent of the
prompt override-marker engine and the product-mode markers. The handler owns mode
branching and the Session State scaffolding only; all manual prose lives in the
rsrc.

> Known residual: `playbook.print(name: "lead-workflow-manual")` — and printing the
> repointed lead skills — is not role-gated and re-exposes the gated bootstrap line
> and the fresh-bootstrap sentinel to any caller that knows the stem; the defense
> there is obscurity. Tracked in idea ticket
> `260626-research-playbook-print-lead-surface-leak`.

### Session-State-Only View {#260702-workflow-state-tool}

`workflow_state(session_key)` is a cheaper sibling of `workflow_manual`: it
returns **only** the `## Session State` section (agenda blobs and the todo
summary) for the caller's session, with no manual reference/primitives text.
It exists so a lead that only needs "what's my key and current state" —
notably right after compaction or during `lead-revive`, when context budget is
tightest — does not have to re-dump the full ~150-line manual body.
`workflow_manual` itself is unchanged: same always-full-dump behavior, same
schema.

- **Lead-only, same gating as `workflow_manual`** (`isLeadOnlyTool`): a
  `session_key` resolving to a delegate/leaf scope is rejected at the keyed
  capability gate before the handler runs. This tool is a cheaper view of the
  same lead-bootstrap/recovery surface `workflow_manual` serves, not a general
  `todo.*`/`agenda.*` accessor, so it stays in the same tool family and gating
  as its sibling even though the underlying todo/agenda data is itself
  scope-open to non-lead callers via the dedicated `todo.*`/`agenda.*` tools.
- **Key validation is reused verbatim from `workflow_manual`**, not a separate
  state machine:
  - **keyless** (`session_key` omitted or empty): the same hard
    required-`session_key` error shape as `workflow_manual`.
  - **resolved** (`session_key` present and its record resolves): renders only
    `renderSessionState` for that record — identical content to the
    `## Session State` suffix `workflow_manual` would render for the same
    session at the same point in time. An empty session (no agenda, no todos)
    renders an empty-but-valid Session State payload (`(no todos)`), not an
    error.
  - **fail-loud** (`session_key` present but unresolvable, including the
    fresh-bootstrap sentinel, which is never a stored record): the identical
    "no restorable state for this key" notice `workflow_manual` renders in its
    own fail-loud path, pointing to `lead-revive` for recovery. The tool never
    mints a key. Unlike `workflow_manual`, `workflow_state` has no FRESH mode —
    the sentinel simply falls through to this same fail-loud path.

### Bootstrap Staleness Warning {#260703-bootstrap-staleness-warning}

`ferrule` and `workflow_manual` (FRESH-with-root and CONTINUE branches only)
each surface a one-line staleness banner when the downstream project's root
`AGENTS.md` carries a `<!-- Template Version: vNNNN -->` tag behind the
version shipped in the running package's own `lead-bootstrap` skill template
(`agents-plugin/skills/lead-bootstrap/AGENTS.template.md` for ws,
`agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md` for wsflow).
The comparison is package-local: whichever package's MCP binary is running
resolves its own shipped template via `wsrsrc.ResolveSkillsRoot()`, so there is
no cross-package (ws vs wsflow) comparison and no separate version manifest to
hand-maintain. The `workflow_manual` FRESH-without-root branch (no established
root yet) never checks or warns.

The check is silent by design in three cases: the `bootstrap_alarm` config
item (see Config Tools) resolves to `off`; the downstream root has no
`AGENTS.md` or the file has no Template Version tag at all (an untagged
project never opted into the ws bootstrap contract, so absence is not treated
as maximal staleness); or the shipped template's own tag is unreadable or
malformed (fail-safe — the tool never warns off of an unreadable "latest").
When the warning does fire, its text names both the installed and latest
version numbers and instructs the caller to run
`config.bootstrap_alarm(value: "off")` to silence it permanently. The warning
fires on every `ferrule`/`workflow_manual` call while stale — there is no
once-per-session suppression state, matching the existing precedent of
per-call injection (e.g. the mercenary agentId tip) rather than the
per-`project_tree`-call anti-pattern this repo's Decisions section warns
against.

Changing `lead-bootstrap`'s own upgrade/migration procedure is out of scope for
this warning; it only detects and reports staleness.

### Doc Coverage Warning {#260707-doc-coverage-warning}

`ferrule` and `workflow_manual` (FRESH-with-root and CONTINUE branches only)
each surface a one-line warning when the project's `ai-docs/spec/` or
`ai-docs/mental-model/` directory has no `.md` file carrying a parsed YAML
frontmatter block. The check is live and stateless: it re-scans both
directories on every call rather than reading a stored coverage flag. The
`workflow_manual` FRESH-without-root branch (no established root yet) never
checks or warns, matching the bootstrap-staleness precedent
(`#260703-bootstrap-staleness-warning`).

The check is silent by design in two cases: the `doc_coverage_alarm` config
item (see Config Tools) resolves to `off`; or both `ai-docs/spec/` and
`ai-docs/mental-model/` already contain at least one frontmatter-bearing `.md`
file. A missing directory counts as uncovered, not an error — fresh projects
legitimately lack these directories before `lead-forge-spec`/
`lead-forge-mental-model` has run. When the warning does fire, its text names
which area(s) are missing coverage and instructs the caller to run
`config.doc_coverage_alarm(value: "off")` to silence it permanently. The
warning fires on every `ferrule`/`workflow_manual` call while uncovered —
there is no once-per-session suppression state, mirroring
`#260703-bootstrap-staleness-warning`.

Whether a project's spec/mental-model authoring is otherwise complete is out
of scope for this warning; it only detects the presence-of-any-frontmatter-file
floor.

## Config Tools {#260505-config-tools}

`config.show` returns the resolved ws user-local configuration path and current
configuration without modifying it. The default response is compact labeled
text, and structured JSON remains available for callers that need stable fields.

`config.agents_tier` is the surface for updating the backend/model/effort mapping
for a capability tier. Callers provide `tier` as the capability tier name
(`small`/`medium`/`large`/`xlarge`); the `light`/`core`/`deep` aliases and
`haiku`/`sonnet`/`opus` provider names are accepted as read-compat synonyms on
input. A caller may also provide a backend, a concrete model, a portable effort, a
harness selector, or any combination of those fields. When backend is omitted,
ws infers it from the model family where possible. Empty effort, omitted effort,
and `none` store the no-override state; supported non-empty effort values are
visible through configuration output. The update applies to the explicit harness
when provided, otherwise the detected MCP session harness when available, and
otherwise the default tier mapping. This makes `backend` mean the execution
backend rather than the tier-table key. {#260513-harness-local-agent-tier-config}

`config.workflow_prefer_subagent(session_key, value: "on"|"off")` sets the
global `"workflow.prefer_subagent"` item, whose builtin default is `off`.
`config.workflow_prefer_mercenary(session_key, value: "on"|"off"|"hide")` sets
the global `"workflow.prefer_mercenary"` item, whose builtin default is `hide`.
Both writer tools require a lead session key for authority but always write the
global config scope. The former unprefixed `"prefer_mercenary"` entry is not
migrated; it remains orphaned local state unless a later ticket introduces
migration. `prompt.DelegationSection.*` prompt override keys are likewise not
migrated.

`config.workflow_prefer_subagent` additionally accepts `reset: true` as an
alternative to `value`; the two are mutually exclusive. `reset: true` removes
the global override entirely (rather than writing an explicit value, even the
builtin's current value) so resolution falls back to `global > builtin` and
tracks any future change to the builtin default. This mirrors the general
unset-vs-set distinction in `#260702-unset-means-reset-to-builtin`.
{#260702-config-unset-reset-to-builtin}

`config.bootstrap_alarm(session_key, value: "on"|"off")` sets the global
`"bootstrap_alarm"` item, whose builtin default is `on`; it gates the
bootstrap staleness warning (`#260703-bootstrap-staleness-warning`). It
requires a lead session key for authority, always writes the global config
scope (global-only, mirroring `config.workflow_prefer_subagent`), and accepts
the same mutually-exclusive `reset: true` alternative to `value` with
identical unset-to-builtin semantics.

`config.doc_coverage_alarm(session_key, value: "on"|"off")` sets the global
`"doc_coverage_alarm"` item, whose builtin default is `on`; it gates the doc
coverage warning (`#260707-doc-coverage-warning`). It requires a lead session
key for authority, always writes the global config scope (global-only,
mirroring `config.bootstrap_alarm`), and accepts the same mutually-exclusive
`reset: true` alternative to `value` with identical unset-to-builtin
semantics.

## Tuning Catalog {#260625-tuning-catalog}

`config.tuning` is a read-only discovery surface for workflow-tuning knobs used
by `ws:lead-tune`. It returns a compact catalog whose entries describe
user-facing knobs, their current resolved values when available, the selector
fields a caller must choose, the value fields a caller may set, and the writer
tool that performs the actual mutation.

The catalog is a projection, not a second setter schema. Each entry names a
small semantic knob id and derives field names, enum values, required fields, and
descriptions from the existing MCP writer tool schema where possible. Prompt
override entries derive their point ids from the same shipped override-marker
scan used by `config.prompt`; model-tier entries derive their fields from
`config.agents_tier`; workflow-preference entries derive their values from
`config.workflow_prefer_subagent`, `config.workflow_prefer_mercenary`,
`config.bootstrap_alarm`, and `config.doc_coverage_alarm`.
The shipped `DelegationSection` override marker is removed, so
`prompt.DelegationSection` is absent from `config.tuning` and `config.prompt`
discovery; orphaned stored prompt keys remain ignored.

Catalog output defaults to LLM-readable text. `format: "json"` returns a stable
structured shape for callers that need to build a proposal or compare runtime
support: `knobs[]` entries carry `id`, `kind`, `description`, `writer`,
optional `reset`, `selector_fields`, `value_fields`, and `current`.
Compatibility-only writer arguments may be omitted from the catalog even when
they remain accepted by the writer tool; the catalog exposes the canonical
tuning syntax, not every legacy call shape. Product mode is honored:
full-ws-only knobs are absent when the runtime is in wsflow/no-agent mode.

> [!note] Constraints
> - `config.tuning` does not mutate config and does not replace
>   `config.prompt.set`, `config.workflow_prefer_mercenary`, or
>   `config.agents_tier`.
> - Adding a new lead-tune knob requires registering its semantic id and writer
>   tool, but must not copy enum/property schema by hand when that schema already
>   belongs to the writer tool.
> - Prompt override discovery remains marker-driven; the tuning catalog must not
>   invent or expose prompt `pointId` values absent from the shipped rsrc tree.

Configuration exposes harness-aware capability-tier mappings. The capability
tiers `small`/`medium`/`large`/`xlarge` map to backend/model defaults per harness;
the historical `light`/`core`/`deep` aliases (this entry's original keys) and the
`haiku`/`sonnet`/`opus` provider names are folded to capability tiers as
read-compat synonyms, so existing tier-shaped and alias-shaped config still
resolves without migration. {#260508-model-alias-config-tools}

The delegation tier abstraction is the capability vocabulary
`small`/`medium`/`large`/`xlarge`, which names task-intrinsic reasoning depth
independent of host or subscription plan, and is the single tier vocabulary across
every surface. `light`/`core`/`deep` (and the `haiku`/`sonnet`/`opus` provider
names) are accepted as read-compat synonyms on input, folded to the capability
tiers `light↦small`, `core↦medium`, `deep↦large`; `xlarge` (fable-class) has no
legacy alias and is independently configurable. Playbook frontmatter declares
`role:` and `tier:` in the capability vocabulary; a tier resolves directly to a
concrete backend/model through `config.agents_tier`
(`#260513-harness-local-agent-tier-config`), which is keyed by the capability tier
(the earlier "remains keyed by `light`/`core`/`deep`" framing is superseded by
`#260620-tier-vocabulary-collapse-direct-model-map`).
{#260612-first-class-tier-vocabulary}

The two-vocabulary split is collapsed to a single tier vocabulary. The capability
vocabulary `small`/`medium`/`large`/`xlarge` is the only tier vocabulary across
every surface — playbook frontmatter, `playbook.render`, `mercenary.register`,
and the model-config tool (the `config.agents_tier` surface, re-homed by this
change rather than by the previously pending `config.model_alias` rename, which
this supersedes). Config is keyed directly by the capability tier: a tier resolves
to its per-harness `(backend, model, effort)` with no intervening
`light`/`core`/`deep` alias step, and the `firstClassTierToAlias` bridge is
retired. `xlarge` has an independently configurable mapping instead of folding
onto `deep`. The `light`/`core`/`deep` aliases and the `haiku`/`sonnet`/`opus`
provider names remain accepted as read-compatibility synonyms on input — and for
existing on-disk config and persisted agent records — so no stored configuration
breaks and no schema migration is required. Per-harness mapping, portable effort,
and backend-affinity resolution are unchanged; only the key vocabulary changed.
Delegate playbooks declare a single tier-derived model hint variable
(`{{.RoleModel}}`) resolved from the playbook's own `tier:`, replacing the
alias-named `{{.LightModel}}`/`{{.CoreModel}}`/`{{.DeepModel}}` set; the rendered
native model hint is preserved. Beyond that per-role hint, any playbook body may
reference the four generic fixed-tier vars
(`{{.SmallTierModel}}`/`{{.MediumTierModel}}`/`{{.LargeTierModel}}`/`{{.XLargeTierModel}}`)
to name a specific tier's model in prose; these resolve through the same
per-harness config seam and are injected as reserved implicit variables (see
`#260609-playbook-harness-rendering`), not as terminology-table entries.
{#260620-tier-vocabulary-collapse-direct-model-map}

### Layered Config Scope Model {#260619-layered-config-scope-model}

Most config items resolve across four ordered scopes, highest precedence first:
`session > project > global > builtin`. `builtin` is the code default (for
example the `wsconfig` tier/alias defaults and
`"workflow.prefer_mercenary"="hide"`). A read returns the value from the
highest-precedence scope that holds one.

Some config items are **global-only** because callers may need them before a
session key, root, or project scope exists. `"workflow.prefer_subagent"` and
`"workflow.prefer_mercenary"` skip session and project overlays, resolve only
from `global > builtin`, and reject non-global writes. The old unprefixed
`"prefer_mercenary"` key remains orphaned local state unless a later ticket
introduces migration.

Each config item declares a natural **default write scope** in code; items that
declare nothing fall back to `project`. A write without an explicit scope lands
in the item's declared default scope. An explicit `scope:` argument on a set
always wins over the declared default. `get`/`show` report *which scope* a value
resolved from, so a caller can see whether a value is session-, project-,
global-, or builtin-sourced.

Scope storage map:

- `session` — the per-key session store (`keys/<key>.json`,
  `#260617-stateless-subagent-context`); tied to the session key's lifetime.
- `project` — the existing project-scoped `config.json` under `${WS_CACHE_HOME}`
  (`~/.ws@<project-id>/`).
- `global` — a project-agnostic `~/.ws/config.json`, with a `WS_CONFIG_HOME`
  environment override mirroring the `WS_CACHE_HOME → ~/.ws@<id>/` convention.

Adding the `global` layer is non-breaking: because `project` outranks `global`,
existing project-stored values keep winning, so no data migration is required;
only the *write default* for future sets follows each item's declared scope.
Read-modify-write on the project and global files is serialized so concurrent
writers cannot corrupt the file (atomic replace under a file lock). The scope
resolution rule and the `scope` argument shape are a single shared contract that
every scope-aware config tool consumes, rather than per-tool re-implementations.

> [!note] Constraints
> - Scope-awareness is opt-in per config item; this contract does not retrofit
>   the existing `config.agents_tier` surface, which is re-homed under the same
>   model by the capability-tier collapse
>   (`#260620-tier-vocabulary-collapse-direct-model-map`) rather than here.
> - Item-level write gating still applies: a scope-aware setter honors an item's
>   existing role/capability restrictions (not every item is freely settable at
>   every scope).
> - The substrate (resolver, default-scope registry, file-lock RMW, global store,
>   shared `scope` schema fragment) and scope-reporting on `config.show` are the
>   caller-visible surface today. Per-item scope-aware *set* surfaces arrive as
>   individual items adopt the model (`"workflow.prefer_mercenary"`
>   (`#260619-prefer-mercenary-session-scope-item`), prompt overrides); the set
>   capability otherwise lives at the internal `wsconfig` API.

### Prompt Override Tuning Tools {#260620-config-prompt-override-tuning-tools}

The prompt-override surface (`#260619-prompt-override-marker-engine`) is tunable
from inside the MCP through a dedicated `config.prompt.*` namespace, distinct from
`config.agents_tier`/`config.show`.

`config.prompt.set(point id, harness, prompt, scope?)` stores a prompt override
keyed by `(point id, harness)`, where `harness` is `claude`, `codex`, or `*`
(the cross-harness `all` bucket; `*` is stored under the `all` key). The value is
written through the layered config scope model (`#260619-layered-config-scope-model`)
under the key `prompt.<point id>.<harness>`: with no `scope`, the write lands in
the item's declared default scope (`project` for these unregistered `prompt.*`
keys); an explicit `scope:` argument wins. A `session`-scope write requires the
caller's `session_key`. The setter is lead-only — delegate and leaf keys are
blocked by the `config.*` capability-gate prefix — and is visible in both full-ws
and agentless wsflow modes, since prompt overrides are a mode-neutral rendering
concern. Once stored, the override is honored at render time by the marker engine
for the matching `(point id, harness)` and resolved scope.

`config.prompt.unset(point id, harness, scope?)` resets a stored prompt
override back to whatever the next-broader scope (or the inline seed default)
resolves to; it never writes an empty-string value in place of the removed
override — an explicit empty override is a distinct intent covered by
`config.prompt.set` with an empty `prompt` value. `scope` accepts `session`,
`project`, or `global` (the same enum as `config.prompt.set`); a `session`-scope
unset requires the caller's `session_key`, matching the setter's session-scope
write requirement. With no `scope`, the item's declared default scope is used
(`project` for unregistered `prompt.*` keys). {#260702-unset-means-reset-to-builtin}

No-argument `config.prompt()` returns a **data listing**, not a manual: a scan of
the shipped playbook resource tree for declared override markers (the marker
grammar from `#260619-prompt-override-marker-engine`) reporting each
override-point's id and short `desc` together with any current override values per
harness bucket and the scope each resolved from, ending with a one-line pointer to
the `ws:lead-tune` workflow-tuning skill (which owns the how-to manual and the
proactive-proposal trigger). The tuning manual itself is deliberately not rendered
there, so `config.prompt()` stays a lean data surface. Like `config.show`, it takes
an optional `session_key` — session-scope overrides are listed and annotated only
when it is supplied — and is lead-only via the same `config.*` prefix gate (a
keyless caller passes; delegate and leaf keys are blocked). The listing is keyed on
the declared markers (orphan `prompt.*` values without a marker are not surfaced),
and each value's scope is resolved through the layered config scope model.

> [!note] Constraints
> - The setter does not introduce its own storage; it writes through the layered
>   config primitive and inline into the single config file, so the override
>   surface inherits that file's lock/atomicity story.

## Project Context And Convention Tools {#260505-project-context-convention-tools}

`project_tree` renders the project document map, spec inventory, and active
ticket inventory for the current repository. The document map omits entries
ignored by the repository's Git ignore rules so generated or vendored
directories do not dominate the readable project context.

`infra.read` reads ws infra documents shipped in the rsrc tree by bare stem or
filename (path-escaping names are rejected). The backing source is the rsrc
loader; the embedded prompt bundle that previously served these documents was
retired.
`convention.read` reads bundled convention documents shipped with the runtime,
such as ticket, spec, or mental-model conventions. Shared workflow skills use
these tools instead of hard-coded repository-local convention paths.

## Spec Discovery Tools {#260505-spec-discovery-tools}

`spec_stem.generate` returns a collision-free spec anchor stem for a descriptive
slug.

`spec_index.verify` checks the spec corpus for anchor-index health problems such
as duplicate stems.

`specs.list`, `specs.find`, and `specs.status` provide read-only spec discovery.
They expose spec file metadata, anchors, ticket references, marker context, query
matches, and exact-stem status without requiring callers to scan the spec tree
manually.

Spec, ticket, and mental-model discovery tools default to compact line-oriented
summaries. Broad list/find calls avoid expanding every nested anchor, phase,
related map, snippet, source, or spec-reference array unless callers request
JSON output. {#260512-documentation-discovery-readable-output-defaults}

Documentation lookup tools treat broad human `query` inputs as tolerant
candidate discovery while preserving exact structured selectors such as
`spec_stem`, `ticket_stem`, and `domain`. Default text output for broad
documentation queries groups evidence by document, renders document metadata as
`<path>\tscore=<score>\thits=<count>`, and lists selected line-number snippets
under each document. JSON output keeps document-centered metadata and adds
line-level match evidence. Convention lookup accepts common aliases such as
`spec`, `ticket`, and `mental-model`.
{#260519-tolerant-documentation-lookup-query-evidence}

## Ticket Discovery Tools {#260505-ticket-discovery-tools}

`tickets.list` returns ticket paths and structured status metadata across ticket
status directories. Active discovery includes `ready/`, `todo/`, and `idea/` by
default; archived `.done/` and `.dropped/` tickets are omitted unless explicitly
requested. `ready/` identifies spec-addressed implementation work, while
`todo/` remains accepted backlog.

`tickets.find` locates tickets by text query, exact ticket stem, mentioned
ticket stem, and optional status filters. `tickets.status` returns structured
metadata for a single ticket stem and can optionally include archived done or
dropped tickets.

`tickets.close` moves a ticket to `.done/` (status=done) or `.dropped/`
(status=dropped), writing the appropriate `completed:` or `dropped:` date into
frontmatter and optionally appending a `## Resolution (YYYY-MM-DD)` body section.
The operation is atomic: the frontmatter write, `git add`, and `git mv` happen as
one staged change set, and the tool never commits. {#260620-ticket-close-tool}

`tickets.move` moves a ticket along the `idea ↔ todo ↔ ready` axis. Downward
moves from `ready/` return a tip to clear spec frontmatter before re-promoting.
Upward moves stamp or validate the ticket's per-stage sage-review posture from
the resolved `sage_review` config: `skipped` for `off`, empty, or unset;
`recommended` for `ask`; and `required` for `auto` (see the Sage Review Gate
section below for the two-field, per-category contract). A move
into `todo/` may leave `recommended` or `required` as the visible unresolved
posture on the fields the ticket's category requires. A move into `ready/`
requires each required field to hold a resolved terminal posture (`completed`
or `skipped`); `recommended`, `required`, and `blocked` on any required field
stop with an action-oriented message naming that field. When both stages apply,
`sage-review-design` is checked before `sage-review-completeness`, so a ticket
that reaches `ready/` without a terminal design posture is always blocked on
the design field first, regardless of entry path. A move into `ready/` for a
non-`epic`/`research`/`workset` ticket with no detected spec addressing (no
confirmed `spec:`/`spec-remove:` frontmatter entry and no `## Spec Impact`
section) additionally returns a soft, non-blocking tip noting that the ready
gate is normally enforced by `lead-write-ticket`; the move still succeeds. The
move stages atomically and never commits.

The blocking sage-review validation above runs after a self-healing
frontmatter write: a legacy single `sage-review:` field, or an unresolved
posture on a required field, is migrated/stamped to the resolved two-field
form before the block is evaluated. When that write happens on a call that
then blocks or errors, the tool response is not a bare error — it appends an
explicit `partial-mutation:` notice line stating that frontmatter was written
before the call blocked and that a retry will not find an unchanged file, so
a retrying caller cannot mistake a blocked move for a fully unresolved,
unchanged ticket.
{#260620-ticket-move-tool}

`tickets.create` creates a dated ticket stub at a caller-specified initial state
(`idea`, `todo`, or `ready`). It auto-prefixes today's date to form the full
ticket stem, writes a minimal frontmatter stub (`title: ""` placeholder;
resolved `sage-review-design:` posture for `todo/+` states when the ticket's
category requires design review), and returns the created path and a
caller-facing tip that names the posture. It does not stamp
`sage-review-completeness` at creation time, even for `state: "ready"` —
completeness is evaluated only at ready-promotion time via `tickets.move`,
which has a "from" state to validate against. Creating directly at `ready/`
for a category requiring design review still enforces the never-skippable
design invariant: if the freshly resolved design posture is not terminal
(`completed` or `skipped`), the call is rejected with an action-oriented error
instead of silently stamping a non-terminal posture and succeeding. Terminal
states (`done`, `dropped`) and an empty stem are rejected with errors. The tool
is not idempotent: a duplicate path returns an error. The `idea/` tip directs
the caller to promote through `todo/` so the resolved posture can be stamped.
{#260622-create-ticket-tool}

`tickets.template` returns the typed body skeleton for a given ticket type.
`type` is required; accepted values are `feat`, `bug`, `refactor`, `chore`,
`research`, `workset`, and `epic`. `feat`/`bug`/`refactor`/`chore` share a
single actionable skeleton (phases-driven structure); `research`, `workset`, and
`epic` each return a distinct skeleton reflecting their section shapes. The
returned markdown is a ready-to-fill stub — section headers and inline fill-in
prompts with no surrounding convention prose, so callers can paste it directly
into a new ticket body. An unknown or empty `type` is rejected with an error
listing valid types. Capability range: `>=0.30.6-dev <0.31.0`.
{#260624-tickets-template-tool}

`tickets.checklist` returns the verification checklist for one `lead-write-ticket`
phase as data, so the playbook can install it as a single todo instead of
carrying the item list as static prose. `type` and `phase` are both required:
`type` accepts the same set as `tickets.template`
(`feat`/`bug`/`refactor`/`chore`/`research`/`workset`/`epic`), and `phase` is
`content` (the ticket-content capture checklist) or `intent` (the intent-review
checklist). The returned markdown is the full multi-item text for that phase,
numbered and ready to paste verbatim into one todo `instruction`; the `intent`
phase emits one extra category-scoped item for `epic` and `workset` and
renumbers the trailing items accordingly. Like `tickets.template` it is a pure
lookup — no `session_key`/root, no gate. An unknown or empty `type`, or a `phase`
other than `content`/`intent`, is rejected with an error listing valid values.
{#260720-tickets-checklist-tool}

The Sage Review Gate is split into two sequential, non-looping stage gates
keyed to ticket lifecycle, both running after `lead-write-ticket` commits a
ticket: a design-sketch review at `todo/` landing (tolerant of missing detail;
catches wrong direction) and a completeness review at `ready/` promotion
(checks implementation-readiness, undecided user-policy points, and capture
gaps). Frontmatter carries two independent stage-scoped fields —
`sage-review-design:` and `sage-review-completeness:` — each using the same
five-value vocabulary as before: `skipped`, `recommended`, `required`,
`completed`, `blocked`. Both fields resolve independently from the same
`sage_review` config value via the same posture-resolution rule (`skipped` for
`off`/empty/unset, `recommended` for `ask`, `required` for `auto`); the config
axis itself is not split, only which stage(s) a given ticket category stamps.

Category exemptions (mirroring the existing spec-address-gate category
detection): `feat`/`bug`/`refactor`/`chore` (default/actionable categories)
require both stages; `epic` requires only `sage-review-design` (epics never
reach `lead-implement`, so completeness never applies); `research` and
`workset` are exempt from both stages, matching their existing blanket
spec-address-gate exemption.

Hard invariant: design review is never skippable regardless of entry path. A
ticket that reaches `ready/` without ever passing `todo/` design review must
still pass design review before or as part of completeness review. Concretely:
`tickets.move`'s promotion validation checks `sage-review-design` before
`sage-review-completeness` and blocks on the design field first if it is not
terminal; the `lead-write-ticket` playbook, when landing at `ready/`, checks
`sage-review-design` before dispatching the completeness reviewer and runs
`ticket-reviewer-design` inline first if the design field is not yet terminal.
This covers `idea/` → `ready/` direct promotion and tickets authored directly
at `ready/`, since both layers check the same field rather than traversal
history.

At `todo/` landing (category requires design), the gate dispatches only
`ticket-reviewer-design` (tier: large) and writes the result to
`sage-review-design:` only. At `ready/` landing (category requires
completeness), after the design-invariant check above passes, the gate
dispatches only `ticket-reviewer-completeness` (tier: medium) and writes the
result to `sage-review-completeness:` only. Each reviewer emits a structured
verdict (`pass`, `concern`, or `block`) with an issues list; since each landing
dispatches at most one reviewer, that reviewer's own verdict directly becomes
the stage's result (no cross-reviewer aggregation), except the
inline-design-then-completeness ready-promotion case, which applies the
existing pairwise aggregation across the two sequential results. A `block`
result appends a `## Blocked (YYYY-MM-DD)` summary section to the ticket body
and writes `blocked` to the corresponding stage field. `idea/` tickets and
`research`/`workset` tickets bypass the gate at every landing; `epic` tickets
bypass only the completeness stage.

The completeness reviewer's checklist includes a scope-boundary check that
distinguishes a genuine completeness/readiness gap (`resolution: autonomous`,
fill it) from a design-shaped gap in disguise — a new public interface,
cross-module interaction change, or architecture reshaping — which must be
raised as `resolution: missing` and left unfilled rather than patched under
cover of a completeness fix.

Legacy migration: a ticket carrying only the old single `sage-review:` field
(no new fields) is read lazily at the first `tickets.move` or
`lead-write-ticket` gate touch. A legacy `completed` migrates to both new
fields as `completed`; legacy `skipped` migrates to both as `skipped`; legacy
`blocked` migrates to both as `blocked` (still must be addressed); any other
legacy value (`recommended`, `required`, missing, or `pending`) is treated as
absent for both new fields and each is resolved fresh, the same as new-ticket
stamping. The migration write persists both new fields on that first touch
(self-healing, no bulk-rewrite script) and leaves the old `sage-review:` field
in place.
{#260624-sage-review-gate}

`tickets.sage_gate` and `tickets.sage_record` are the two root-aware tools the
`lead-write-ticket` playbook calls to run the gate above; both require
`session_key`. `tickets.sage_gate(stem, landing[, answer])` resolves the gate
decision for a ticket and returns `{ action, ask_prompt?, reviewers?, mode? }`
where `action` is one of `skip`, `stop_blocked`, `ask`, or `run`. It owns
posture resolution, the legacy single-field `sage-review:` migration, the
`sage_review` config fallback, the category×stage matrix, and
standalone-versus-combined mode selection. For `ask` it returns the exact
question to relay; the caller re-invokes with `answer` (`yes`/`no`), and each
still-pending `recommended` stage is asked separately (design first) so one
answer never resolves another stage. A declined `ask` and a config-fallback
resolution each persist the resolved posture and commit. The tool never spawns
reviewers — for `run` it names the reviewer(s) to dispatch and leaves spawning
to the lead. `tickets.sage_record(stem, stage, verdicts)` aggregates the
supplied stage verdicts into the final posture, writes the frontmatter field(s),
renders any `## Blocked` section from a Go-owned template whose output is
byte-identical to the prior playbook templates, commits with the canonical
title, and returns the applied posture plus the commit reference. A `stage`
whose expected reviewer verdict is absent from `verdicts` is rejected with an
error rather than recording a passing posture for a review that did not run.
Capability range: `>=0.33.15-dev <0.34.0`. {#260720-sage-gate-record-tools}

## Mental-Model Discovery Tools {#260505-mental-model-discovery-tools}

`mental_models.list` returns available mental-model documents with domain,
description, and source metadata.

`mental_models.find` locates mental-model paths by text query, domain, or spec
stem reference. `mental_models.status` returns path-first metadata for documents
selected by domain or path.

## Reference Trace Tool {#260505-reference-trace-tool}

`references.trace` returns the reference graph reachable from exactly one ticket
stem or spec stem. The result connects tickets, specs, and mental-model
documents so callers can inspect traceability without manually searching each
document system.

Small metadata and trace tools such as `api.list`, `ws.setup`, selected
runtime/config inspection views, and `references.trace` default to compact
labeled text where no caller needs stable structured fields.
Launcher-facing compatibility data remains available where required.
{#260512-metadata-trace-readable-output-defaults}

Interactive workflow command surfaces default to compact readable text when the
caller has not explicitly requested structured JSON. Write-capable workflow
tools summarize the completed action, affected paths or entities, and detected
workflow annotations without forcing callers to parse JSON. CLI mirrors follow
the same default where they are workflow-oriented wrappers, while Git command
mirrors preserve the original Git command output shape rather than reserializing
it into a ws-specific JSON envelope. Explicit JSON modes remain available for
structured consumers. {#260519-workflow-command-readable-output-defaults}

## Git Workflow Tools {#260505-git-workflow-tools}

`git.status` returns the current branch and worktree status.

`git.diff` returns read-only diff output. It defaults to stat mode for context
control and supports explicit `mode: "full"` for patch content or
`mode: "name_only"` for path listings. Range-less diffs include untracked files
where applicable.

`git.log` returns a bounded commit log with an optional body flag. `git.merge_base`
returns the merge base for two revisions.

Git read tools default to direct, LLM-readable text: `git.status` as a
branch/worktree summary with changed-file codes, `git.diff` as the selected
diff text, `git.log` as bounded commit blocks without JSON-escaped bodies, and
`git.merge_base` as a labeled hash line. JSON output remains available when a
caller explicitly asks for structured compatibility output.
{#260512-git-readable-output-defaults}

`git.commit` creates a workflow-aware commit from explicit paths and structured
message fields. It stages only the requested paths and formats commit messages
with required AI Context and optional ticket, spec, or mental-model update
sections. Ticket update detection recognizes added `### Result` headings and
added `#### Edition` headings so commit summaries can report first completion
records and later append-only tweak records. The default response is a compact
readable commit summary; callers can request structured JSON explicitly.
`git.commit` also accepts structured Mental Model Notes input and renders it as
a `### Mental Model Notes` sub-section under `## AI Context`, while preserving
the existing `ai_context` bullet path and deterministic commit body shape.
{#260519-git-commit-mental-model-notes}
When an explicit commit path names an old root from a rename or a deleted root,
`git.commit` stages the concrete removed paths reported by Git status rather
than passing the missing root to `git add`; requested roots with live changes
still stage through the explicit add path.
{#260513-git-commit-result-edition-detection}
`git.commit` ticket-change summaries conservatively reconstruct an unambiguous
same-stem ticket status move even when native Git reports the staged change as
separate add/delete records instead of a rename. Ambiguous add/delete sets remain
non-move ticket changes rather than inventing a destination status.
{#260519-git-commit-add-delete-ticket-move-summary}
After a successful commit, `git.commit` re-injects the calling session's todo list
as a summary-mode block appended to the text-mode commit response, so a checkpoint
commit doubles as a todo restoration point. The injection is text-mode only and is
skipped when the session holds no todos or when structured JSON output is requested;
it does not change todo status — `git.commit` never auto-marks items done.
{#260626-git-commit-todo-reinjection}
After the todo re-injection (if any), `git.commit`'s text-mode response appends a
trailer line — `tip: preserve this session key: <key> during compaction` — naming
the calling session's key. The trailer repeats the reminder `workflow_manual`
already places near the top of a manual reload, on a high-frequency, lead-scoped
call that tends to land near the end of a working turn, so the key stays recent in
the transcript at the point context compaction is likely to trigger. The trailer
is omitted only when no session key is present; structured JSON output is
unaffected.
{#260708-git-commit-session-key-tip}

## Workflow State And Delegation Tools {#260505-workflow-state-delegation-tools}

`path.generate` allocates writable workflow artifact paths so workflow agents can
exchange file paths without inventing cache locations. `kind: "review"` and
`kind: "prompt"` allocate worktree-scoped cache artifacts. `kind: "plan"`
allocates repo-local implementation plan files under
`ai-docs/.plans/YYYY-MM/DD-hhmm-<stem>.md`; collisions append a numeric suffix
while preserving the sanitized logical stem.

## wsflow Agentless Runtime Mode {#260513-wsflow-agentless-runtime-mode}

The MCP server supports an environment-selected agentless product mode for the
internal `wsflow` distribution. With `WS_MCP_NO_AGENT=1`, advertised tools
omit named-agent and model-alias configuration surfaces:
`mercenary.*` and `config.agents_tier`. `api.list` remains available as
read-only cache discovery; the agent-backed API documentation ask tools are
removed from the full ws surface rather than hidden only in wsflow mode.

Explicit calls to hidden agent-backed tools fail with a clear disabled error and
do not start named-agent workers. Runtime capability output and CLI command
surfaces match the selected mode, so no-agent mode omits the hidden MCP tools
and matching CLI groups such as `agents` and
`config agents-tier`.

`WS_MCP_NAMESPACE=wsflow` changes ordinary user-facing namespace text to
`wsflow` without renaming generic MCP tool names. If `WS_MCP_NAMESPACE` is
unset or empty, the server keeps the default `ws` namespace and existing full
plugin behavior. `WS_MCP_SETUP_TOOL=setup` advertises `setup` instead of
`ws.setup`; when unset or empty, the canonical setup name remains `ws.setup`.
`ws.setup` may remain available only as hidden compatibility dispatch when a
different setup name is advertised.

The playbook surface also follows product mode. In no-agent mode,
`playbook.print` and `playbook.render` serve the shared rsrc playbook bodies
through product-aware selection: `<!-- ws:full-only:start/end -->` regions are
omitted, `<!-- ws:wsflow-only:start/end -->` regions are included, marker
comments are never emitted, and the remaining user-facing namespace notation is
rendered through reserved namespace variables such as `McpNamespace` and
`SkillNamespace`. In wsflow these variables render as `wsflow`; in full ws they
render as `ws`. The variables do not rename literal generic MCP tool
identifiers such as `ferrule`.

## Playbook Tools {#260609-playbook-tools}

The playbook tools are the ws-distribution surface for serving workflow procedure
text and subagent-injection prompts from a plain-text resource tree, with content
selected for the detected host harness. `prompt.render` was the retired
wsflow-only predecessor for delegate prompt materialization; it is no longer
advertised or callable in either product mode. Legacy wsflow delegate context
materialization is preserved through `playbook.render`.

`playbook.print(name, context?)` returns the named playbook's procedure text
inline in the tool result, with `context` values substituted and declared
includes resolved. It is the lead-facing successor of internal workflow-skill
bodies.

`playbook.render(session_key, name, context?, root_override?)` materializes the
named playbook as a context-injected, harness-rendered prompt, writes it to a
worktree-scoped temporary file, and returns that file path together with a
`recommended-tier` line carrying the playbook's first-class frontmatter tier (when
declared). When the tier resolves through the shared per-harness `(backend,
model, effort)` config seam (`#260609-playbook-harness-rendering`), the payload
also carries an additive `recommended-model` line, and — only when the resolved
effort is non-empty — a `recommended-reasoning-effort` line; a resolver error
omits both additive lines but never the `recommended-tier` line, and neither
additive line is ever emitted with an empty value. The caller hands the path to
a host-native subagent or a mercenary and routes these bindings to whichever
path it picks: for a native subagent, `recommended-model`/
`recommended-reasoning-effort` are the host model-selection binding (Codex
binds them to the native `spawn_agent.model` / `spawn_agent.reasoning_effort`
spawn parameters — never `effort`; if the native surface cannot accept the
exact binding, the caller reports the binding as unavailable rather than
silently dropping it, since the mercenary path is the explicit exact-binding
fallback); for a mercenary, `recommended-tier` remains the value passed to
`mercenary.register`'s pass-through `tier` (`recommended-model`/
`recommended-reasoning-effort` are not consumed by mercenary registration,
which takes no model/effort fields). `playbook.print` surfaces the same
`recommended-tier`/`recommended-model`/`recommended-reasoning-effort` lines. The
tool carries no routing or strategy decision — the caller selects `name`, and
the tool only materializes a rendered copy. `root_override`, when set, rebinds both the
auto-include resolution root and the child-key binding root for a delegate
running in a different worktree. When the calling `session_key` is lead-scoped
and the playbook frontmatter declares a delegate-eligible role, the render mints
a fresh child session key and splices it into the rendered prompt, so both native
and mercenary delegates receive a prompt with their key already embedded
(`#260610-mercenary-delegation-surface`).

In no-agent/wsflow mode only, `playbook.render` has a compatibility bridge for
the five legacy render-eligible stems. When `name` is one of those stems,
declared caller `context` keys are rendered as normal template variables and any
remaining undeclared keys are appended as prompt data in a `## Render Context`
block after normal playbook rendering. The bridge does not apply to
`implementer`, to arbitrary playbooks, or to full ws mode.

A playbook is selected by `name`; the tool does not decide which playbook to use.
A load or render failure for a requested `name` is a loud error, not a silent
empty result.

Harness-aware content selection uses the harness the MCP session has already
detected (`#260508-mcp-payload-harness-detection`). Harness differences are
served as data, not as separate code paths: a shared playbook body plus a
per-harness terminology table (exploration agent name, spawn idiom, continuation
idiom, model aliases), with structural divergence expressed only through
per-harness overlay files. The supported harness set is Claude and Codex; an
unrecognized harness renders host-neutral text rather than failing. Concrete
per-provider model names are resolved from configuration
(`#260513-harness-local-agent-tier-config`), never baked into the resource tree
or the binary, so model-name churn is a config update rather than a
redistribution. {#260609-playbook-harness-rendering}

Product-mode content selection is separate from harness selection. Shared rsrc
playbooks may mark full-ws-only or wsflow-only sections with the product markers
documented in `#260513-wsflow-agentless-runtime-mode`; `playbook.print` and
`playbook.render` select those sections after harness rendering and before
returning text or writing a prompt file. User-facing namespace notation in
shared playbooks is authored with reserved implicit variables (`McpNamespace`
for `ws/<tool>` notation and `SkillNamespace` for `ws:<skill>` notation). The
same reserved-implicit-variable mechanism also carries the four generic
tier-model vars (`SmallTierModel`, `MediumTierModel`, `LargeTierModel`,
`XLargeTierModel`), which resolve at render time from config through the same
per-harness `(backend, model, effort)` seam as `RoleModel` and fall back to a
stable `the <tier>-tier model` label when resolution fails, so they never render
empty mid-sentence. These vars are injected by the playbook tool layer, are
available without frontmatter declarations, and override caller-supplied
`context` keys. Literal MCP tool identifiers remain literal unless a dedicated
semantic variable is introduced.

A playbook may declare text dependencies in its frontmatter; the renderer
auto-includes that text at print/render time, so a single `playbook.print(name)`
call returns the procedure together with its required conventions. The include
set is fixed at authoring time, not chosen by the caller per call. This does not
replace `convention.read` / `infra.read`, which remain standalone discovery tools
for raw access.

Code-side pragmatic playbook concatenation is renderer-owned behavior, not a
source-template syntax. When global `"workflow.prefer_subagent"` resolves to
`on`, `playbook.print(name: "lead-workflow-manual")` renders
`lead-prefer-subagent` through the same harness-aware renderer, prompt override
resolver, and product-mode pass, then appends it to the manual. The appended
body is wrapped as `<playbook name="lead-prefer-subagent" title="Prefer Subagent">...</playbook>`.
Standalone `playbook.print` and `playbook.render` output remains Markdown, and
no duplicate-insertion guard is applied.

A playbook marked as delegating carries a compact continuation tip in its
rendered output, reminding the caller to reuse the host-returned subagent agent
id for continuation instead of respawning. The tip is the only continuity
mechanism: the playbook surface keeps no agent registry and mandates no
continuity-recording file.
Delegate-eligible `role:` metadata is independent of the `delegates` tip flag.
A rendered playbook can mint a role-scoped child key and expose its recommended
tier while setting `delegates: false` to suppress the generic continuation tip
when the prompt is meant for direct execution, such as the initial implementer
prompt or review-fix relay implementer prompt.

> [!note] Constraints
> - Gemini is out of scope; only Claude and Codex have terminology tables. Any
>   other harness, including none detected, gets host-neutral text.
> - The continuation tip is advisory text, not an enforced or tracked binding.

### Resource Tree Distribution {#260609-rsrc-playbook-distribution}

Playbook and prompt text ships as a plain-text resource tree distributed with the
plugin and loaded at call time, rather than compiled into the binary. Text-only
changes to playbooks are therefore deployable without a binary version change.

The tree carries a manifest recording per-file integrity data and a playbook
schema version. The runtime gates loading on **schema-version compatibility**,
not on exact content-hash equality, so compatible text edits load without a
binary bump while an incompatible schema version is refused.

A manifest mismatch or load failure is a loud, partial failure of the playbook
surface: the playbook tools report the failure and do not serve playbook content,
and there is no embedded fallback copy. A session whose playbook surface has
failed still serves the discovery, Git, and other tools that do not depend on the
resource tree.

When a caller requests a playbook stem that is absent from both the resource
manifest and the resource tree, the playbook surface reports a no-such-playbook
diagnostic. Manifest integrity diagnostics are reserved for corrupted or stale
resource trees, such as a manifest-listed file missing from disk or a listed
file whose hash no longer matches.

`WS_RSRC_ROOT` overrides the resource-tree load root. When set, the runtime loads
the tree from that path instead of the distributed plugin copy, so a development
checkout can edit playbook text and see it live without waiting on plugin cache
refresh.

> [!note] Constraints
> - Compatibility is defined by schema version, not file-hash equality; the
>   manifest's hashes are integrity data, not a load gate.
> - There is no embedded fallback text. When the resource tree is unavailable or
>   incompatible, the playbook surface fails loudly rather than degrading to a
>   stale built-in copy.

### Prompt Override Marker Engine {#260619-prompt-override-marker-engine}

A playbook body may declare named **override-points** with block markers so a
user can replace or extend a named section of the rendered text without editing
the shipped resource tree. An override-point is a pair of single-line markers,
each on its own line, wrapping inline seed text:

```
<!-- ws:override:ExampleSection desc="human-readable summary" -->
<seed default text — the shipped wording for this point>
<!-- ws:/override:ExampleSection -->
```

The identifier after `ws:override:` is the **point id**. The text between the
open and close markers is the **inline seed default** — there is no separate
default field, so the shipped wording stays in the `.md` body and is readable in
review. `desc` is a short human-readable summary of the point.

The override pass runs during both `playbook.print` and `playbook.render`,
alongside product-mode marker selection (`#260513-wsflow-agentless-runtime-mode`)
and after harness rendering. For each override-point it resolves a value along
two orthogonal axes:

- **What** is selected by `(point id, harness)`: a stored override whose harness
  matches the rendered harness wins; otherwise an override stored for the
  cross-harness `all` bucket applies; otherwise the inline seed default is used.
  The harness axis values are `claude`, `codex`, and `all` (the `all` bucket is
  the cross-harness / `*` setting).
- **Where** the override is stored is selected by scope through the layered
  config scope model (`#260619-layered-config-scope-model`); resolution reads the
  highest-precedence scope that holds a value, including code-owned builtin
  defaults when a shipped harness binding must stay out of the shared seed text.

The resolved text replaces the block body and the marker lines themselves are
stripped, so the rendered output contains only resolved content and never the
marker syntax. An **empty seed body** is a pure extension slot: it renders the
stored override if one exists, or nothing when none is set. `UserPreferenceSection`
uses this empty-slot shape for standing preferences.

Override values resolve through the layered config scope model under the key
`prompt.<point id>.<harness>`, so a write at any scope through the config layer
is honored by the resolver's precedence; the point id is the user-facing handle
even though the body carries it as a marker rather than a template variable. The
dedicated `config.prompt.set(point id, harness, prompt, scope?)` setter makes the
override surface tunable from inside the MCP without external docs; a
self-documenting `config.prompt()` listing makes that surface discoverable from
inside the MCP as well (`#260620-config-prompt-override-tuning-tools`).

Shipped lead workflow-manual override-points include `UserPreferenceSection`, an
empty extension slot for standing communication, terminology, and workflow
preferences. A user adds standing preferences by storing an override under
`prompt.UserPreferenceSection.<harness>` without editing the shipped resource
tree. Delegation posture is controlled by `"workflow.prefer_subagent"` rather
than a freeform prompt override.

Shipped lead-prefer-subagent uses the generic empty extension point
`PreferSubagentInvocationGuidance` for harness invocation details. Codex receives
a code-owned builtin default under `prompt.PreferSubagentInvocationGuidance.codex`
for its `spawn_agent(fork_context:true, ...)` binding, while Claude uses the
empty shared seed unless configured otherwise.
{#260619-delegation-section-override-point}

> [!note] Constraints
> - The marker grammar is a ws-private schema (ws is the sole reader); it is not
>   an external comment-processing standard and carries no meaning outside the
>   playbook surface.
> - Override-points do not use the `{{.Var}}` template-variable mechanism. The
>   point id lives only as the marker id and the `config.prompt.set` key; the seed
>   default is the inline block body, not a frontmatter default.
> - Critical-path render mechanics are intentionally NOT exposed as
>   override-points: the harness-aware continuation tip, the delegate child-key
>   credential splice, and the `prefer_mercenary` guidance block
>   (`#260619-prefer-mercenary-session-scope-item`) stay fixed because they are
>   correctness, security, and continuity machinery, not user-tunable style.

## Named-Agent MCP Tools {#260505-named-agent-mcp-tools}

The `mercenary.*` tool family exposes durable named-agent orchestration.

The `mercenary.*` family is the reshaped scoped **mercenary** delegation surface
(`#260610-mercenary-delegation-surface`): codex and claude runners retained,
scoped to implementer/reviewer roles, invoked with a single self-contained prompt
from `playbook.render`.

`mercenary.register` registers a mercenary agent with an optional `backend` (codex
or claude) and a self-contained `system_prompt_text` produced by
`playbook.render`. The former `prompts: [stems]`/`prompt_refs` and `model`
registration fields are removed. The `tier` field is a *pass-through* of the
first-class recommended tier that `playbook.render` returns — its origin is the
playbook frontmatter, not a caller-chosen workload tier: `mercenary.register` maps it
to the alias layer and resolves the per-mercenary backend/model from harness config
(`#260513-harness-local-agent-tier-config`), so a mercenary's model follows its
playbook frontmatter `tier:` rather than defaulting to core. `mercenary.call` starts an asynchronous
call, returns immediately, and yields a native-shaped continuation handle
(`agentId=<name>`) so the lead reuses one continuation idiom across the native
and mercenary paths. Named-agent calls resolve their root from the mandatory
`session_key` like every other root-aware tool
(`#260610-ephemeral-session-auth-model`): no `mercenary.*` schema advertises a
`root` argument, and there is no actor scope, hidden explicit-root dispatch, or
persistent child-actor credential injection. The named-agent registry namespaces
role pointers by the resolved worktree root, so the same public agent name stays
distinct across distinct worktree roots without an actor dimension.

`mercenary.wait` waits for one or more agents to become ready and returns readiness
metadata, not final output. `mercenary.result` is the result-consumption surface and
may optionally wait for completion; successful ephemeral agents are erased after
their result is consumed.

`mercenary.status`, `mercenary.tail`, and `mercenary.cancel` inspect or control current
agent work. Cancelled status text points callers toward retrying `mercenary.call`
on the same registered agent when no result is available, so timeout-driven
cancellation does not look like a final erase-only state.
{#260512-agent-cancel-resume-guidance}

`mercenary.recall` is hidden from the advertised MCP tool surface and workflow
guidance. The implementation may remain as a manual or compatibility path, but
ordinary model-visible recovery uses `mercenary.call` on the same registered agent.
{#260512-agent-recall-hidden-surface}

Normal `mercenary.tail` is context-bounded. Raw diagnostic inspection is available
through `mercenary.debug.tail`, `mercenary.debug.stdout`, `mercenary.debug.stderr`,
`mercenary.debug.runtime_log`, and `mercenary.debug.events`.

`mercenary.interrupt` queues a redirect message for a running agent. `mercenary.print`
remains a deprecated compatibility reader over the resolved current instance.
`mercenary.erase` removes or hides the resolved role pointer for the current
worktree and actor scope; historical instance payloads are removed later by the
named-agent retention cleanup policy rather than synchronously during erase.

## Mercenary Delegation Surface {#260610-mercenary-delegation-surface}

The reshaped delegation surface. A **mercenary** is a ws-spawned external
subprocess agent — a deliberately distinct term from a harness-native
**subagent**, so callers never confuse the two delegation paths. This section is
the caller-visible contract for the reshaped `mercenary.*` family.

**Default is hidden; native is the ordinary delegation path.** The global
`"workflow.prefer_mercenary"` item controls whether the public mercenary surface
is visible and whether implementer/reviewer playbook renders prefer mercenary
guidance. Its builtin value is `hide`, which suppresses `mercenary.*` from
tool discovery, runtime capabilities, and explicit calls. `off` exposes the
mercenary surface but keeps host-native subagents as the default guidance. `on`
exposes the surface and makes implementer/reviewer renders prefer the
mercenary-call path. The lead writes this item through
`config.workflow_prefer_mercenary(session_key, value)`; the writer requires a
lead session key for authority but writes the global config item because
keyless tool visibility cannot read session or project state.

`ws.lead.prefer_mercenary` is removed with no alias. The old unprefixed
`"prefer_mercenary"` key remains orphaned local state and is not migrated.
{#260619-prefer-mercenary-session-scope-item}

**Scope: implementer and reviewer roles only.** Mercenaries cover implementer and
reviewer delegation. Exploration, survey (reference-discovery, plan-populator),
and mental-model update route to host-native subagents
(`#260609-playbook-harness-rendering`), not mercenaries.

**Live, pluggable backends.** The codex and claude runner backends are retained
and live. The runner-backend interface is harness-neutral and pluggable: the
gemini backend implementation is unshipped (model-compat cost), but the plug
point is preserved so gemini, antigravity, or a custom harness can re-attach as a
deferred plug, not a structural exclusion.

**Single self-contained prompt; native-shaped handle.** A mercenary is invoked
with one self-contained prompt produced by `playbook.render`
(`#260609-playbook-tools`); there is no `register(prompts: [stems])` step. The
playbook's first-class frontmatter `tier:` is surfaced by `playbook.render` as a
recommended tier and passed through to `mercenary.register`'s `tier` arg, which
selects the mercenary's model via config — the caller never hand-picks a workload
tier. A
mercenary call returns a continuation handle of the same shape as a native
subagent id, so the lead reuses one continuation idiom across both paths.

**Render-minted child keys.** `playbook.render(session_key, name, context?,
root_override?)` is the mint-and-inject point for both native and mercenary
delegates: when `session_key.role == lead` it mints a fresh child key (role taken
from the playbook frontmatter) and splices it into the rendered prompt, so the
delegate receives a prompt with its key already embedded. `root_override` rebinds
both the auto-include resolution root and the child-key binding root when the
child runs in a different worktree; render does not infer worktree shape — the
caller passes the path.

**Containment is server-side on the keyed call handler.** The keyed `tools/call`
handler rejects `lead.*` calls from non-lead keys. A child key (native or
mercenary) is therefore unable to login or spawn, so spawn depth is strictly 1
(lead → mercenary leaf); no recursion-depth counter is needed. Schema and
`tools/list` filtering remain a harness-owned soft-guard for LLM-confusion
reduction only — they are not the enforcement boundary.

> [!note] Constraints
> - `config.workflow_prefer_mercenary` controls both public mercenary surface
>   visibility and default render guidance. The on-request path is reachable
>   only when the value is `off` or `on`; `hide` suppresses the public surface.
> - Mercenary scope is implementer/reviewer only. Exploration and mental-model
>   work are native-subagent only and never mint a mercenary.
> - Gemini is a preserved plug point, not a shipped backend.
> - Containment is the server-side keyed-handler role check, not schema hiding.

## API Documentation MCP Tools {#260505-api-documentation-mcp-tools}

`api.list` returns sorted API documentation cache domain names under
`ai-docs/.deps`. The default response is one domain per line, with structured
JSON available on request.

The retired agent-backed API documentation tools are not exposed by full ws:
`api.ask`, `api.ask_async`, `api.status`, `api.result`, and `api.cancel` are
unknown tools and absent from runtime capability metadata.
{#260508-api-documentation-async-mcp-tools}

The remaining `api.list` behavior is limited to deterministic read-only local
cache discovery. Workflow guidance routes external dependency/API documentation
questions through scoped native exploration or direct official documentation
lookup until a future pure-tooling `api.*` namespace is designed.

## Exec Job MCP Tools {#260524-exec-job-mcp-tools}

The `exec.*` tool family exposes durable command execution jobs for trusted lead
workflows. `exec.spawn` runs structured argv commands with `cmd`, optional
`args`, optional `working_dir`, optional environment overlays, and optional
textual stdin. `exec.shell` runs an explicit shell command string with optional
`working_dir`, environment overlays, textual stdin, and shell selection. Omitted
`working_dir` resolves to the current ws worktree root. Relative values resolve
beneath that root rather than the plugin cache process cwd, and resolved working
directories must stay inside the worktree root.

Launch tools create an `exec_key`, start the process, persist stdout and stderr
under job-owned files, and wait up to a fixed short foreground window before
returning. That foreground wait is not a caller-configurable timeout. When a job
finishes during the window and combined stdout plus stderr is within the fixed
4096-byte inline budget, the launch response may include the output, exit
status, `exec_key`, and metadata. Running jobs or larger outputs return compact
metadata, stream sizes, and follow-up guidance without inline raw output.

The `exec.*` MCP tools return MCP text content formatted for direct model
reading; they do not expose a public `format: json` response mode. Lifecycle
responses use compact labeled metadata such as `exec_key`, `status`,
`result_ready`, timestamps, exit state, and stream byte counts. When inline
stdout or stderr is present, metadata appears first and raw stream text appears
below obvious separator lines such as `========== stdout ==========` and
`========== stderr ==========`. JSON-shaped command output remains raw text in
that output area rather than being escaped inside a serialized JSON response.
If output exceeds the fixed 4096-byte inline budget, lifecycle responses keep
the raw body out of the result and include guidance to use the raw fallback
readers.

Exec lifecycle metadata is SQLite-backed while stream payload bytes remain in
job-owned files. SQLite stores job identity, command and working-directory
metadata, lifecycle state, process or lost-worker state, timestamps, exit
status, stream paths, stream byte counts, and retention/prune metadata. Existing
file-backed exec state is imported when possible; corrupt or unimportable legacy
state returns bounded recovery metadata rather than silently disappearing.

`exec.status` reports job lifecycle state and stream metadata. `exec.result`
returns job metadata and at most the fixed 4096-byte inline output budget for a
terminal job. When `timeout_seconds` is omitted or zero, `exec.result` is
non-blocking; a running job returns readable running metadata and guidance
without an MCP error. When `timeout_seconds` is positive, `exec.result` waits up
to that many seconds for the job to become terminal, then returns either the
terminal result or the same readable running guidance if the timeout expires.
Larger results guide callers to the future `exec.ask` path first and the raw
fallback readers second. `exec.abort` best-effort terminates a running job while
preserving partial output and terminal state metadata.

If a process-local worker is lost while a persisted job still appears running,
later status/result calls reconcile the record from process liveness and mark a
missing worker terminal rather than leaving the job indefinitely running.

Raw fallback readers are named under `exec.raw.*`. `exec.raw.tail` returns a
bounded tail from a selected stream. `exec.raw.read` reads by byte offset and
returns `next_offset`. `exec.raw.grep` searches selected streams, defaults to
literal matching, and uses regular expressions only when the caller explicitly
sets `regex: true`. If a stored stream path is missing, raw readers report a
recoverable file-backed payload consistency state instead of treating the stream
as empty.

Raw-reader MCP responses are also readable text rather than JSON payload text.
They identify the selected `exec_key` and `stream` with labels. Tail and read
responses place returned bytes below a `========== text ==========` separator;
read responses additionally expose `offset`, `next_offset`, `limit`, `size`,
and `eof` metadata above the separator. Grep responses expose match count and
truncation metadata above `========== matches ==========` and render each match
as readable line blocks with any requested context.

## Runtime Metadata Migration Gate {#260525-runtime-metadata-migration-gate}

The ws runtime has a SQLite metadata migration gate for moving named-agent and
exec runtime metadata into SQLite authority. The gate keeps public `mercenary.*`
and `exec.*` MCP APIs stable while separating lifecycle metadata from
file-backed payload bodies. Named-agent registry metadata and exec job metadata
are SQLite-backed. SQLite metadata may track identities, lifecycle state,
session binding, path indexes, byte counts, retention visibility, leases,
tombstones, and prune bookkeeping. Prompts, streams, runtime logs, event JSONL,
transcripts, backend raw output, and final output bodies remain file-backed.

SQLite state-store configure, migration, and short write paths use bounded
retry for `SQLITE_BUSY` and `SQLITE_LOCKED` conditions while retaining
process-local write serialization. Runtime migrations must keep transactions
short and must not hold a transaction across subprocess or model execution.

## Tool Profile Gating {#260505-tool-profile-gating}

The MCP server defaults to the `lead` tool surface, and `tools/list` advertises
the full lead surface regardless of any caller environment. Schema visibility is
advisory, not an authority boundary, because plugin-managed hosts can start the
server from cache directories and can fail to propagate environment variables
consistently.

Tool-permission enforcement is the server-side capability check in the keyed
`tools/call` handler. A session key carries `{root + capability scope}` —
`lead`, `delegate`, or `leaf` — minted by `ferrule(capability)` or as a
render-minted child key. When a call presents a known non-lead key, the handler
rejects any tool that scope disallows (`delegate` cannot call `mercenary.*`,
`config.*`, or `session.*`; `leaf` additionally cannot call `git.commit`) and
rejects any lead-only tool from any non-lead key — the `lead.*` prefix plus
the bootstrap tool `ferrule` matched by explicit name (self-bootstrap
escalation block). The retained `api.list` cache-domain discovery tool is read-only and
leaf-callable; the retired agent-backed API ask tools are absent from the
surface rather than blocked by scope. Keyless callers and lead keys are not
restricted by this gate,
so the keyless `ferrule` bootstrap stays open; a delegate can therefore
keyless-re-`login` to re-escalate. The scope is a soft defense-in-depth guard
layered on the host's own subagent tool restriction, not a hard sandbox.

`WS_MCP_TOOL_PROFILE` no longer gates the served tool surface and is not
propagated to spawned mercenary subprocesses; the env-profile role mechanism is
retired in favor of the keyed capability gate, having been verified
non-functional for containment (it was lost whenever the host failed to propagate
the env var). Delegate tool scope now travels in-band through the render-minted
child key rather than the environment. `WS_MCP_ALLOWED_TOOLS` is retained as an
optional visibility allowlist for tests and debugging, independent of capability
scope: it can narrow the visible surface but cannot expand access beyond what the
keyed gate permits.

## CLI Mirror Coverage {#260505-cli-mirror-coverage}

The `ws-mcp` binary mirrors selected MCP behavior as CLI commands for smoke
tests, compatibility probes, and fallback usage.

CLI mirrors exist for runtime info, single-process smoke checks, config, path
generation, named agents, Git, tickets, specs, selected mental-model
discovery, and reference tracing. Not every MCP tool has a CLI mirror; the MCP
surface is the canonical host-neutral interface, and CLI coverage is limited to
the surfaces needed for runtime checks and workflow fallback use.
