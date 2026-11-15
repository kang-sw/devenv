---
title: Guard mixed-version ws deployments (skew notice, tool-surface monotonicity, newer-schema write refusal, tool-list refresh)
related:
  261018-research-claude-mod-ws-mcp-reconnect: the Claude mod reconnect watchdog this ticket widens to daemon replacement
  261112-bug-ws-launcher-fingerprint-skips-embedded-assets: same discussion; dev-only stale binary (scenario 6), not in scope here
  261021-bug-launcher-local-devenv-minor-bump-breaks-installed-plugin: neighbouring launcher minor-mismatch behavior
---

# Guard mixed-version ws deployments (skew notice, tool-surface monotonicity, newer-schema write refusal, tool-list refresh)

## Background

The dashboard daemon is now the default channel for ws MCP tools, skills, and
whiteboard traffic, so harnesses updated at different times (for example Pi
updated, Claude not) meet one shared daemon and one set of shared stores. A
2026-11-12 code exploration found that versions meet with effectively no
negotiation:

- One daemon per machine; within a minor the newest version wins
  (`ensure.go` keepRunning, `version.go` CompareVersions). Hosted MCP tools
  run in the daemon binary while playbook and skill text comes from the
  connecting plugin's own directory (`WS_RSRC_ROOT`), so the typical in-minor
  skew is old skill text against a new binary.
- On a minor mismatch the launcher silently (one stderr line) bypasses the
  daemon and runs its own binary per session, while the stores stay shared.
- The `/mcp` hello requires `protocol: 1` exactly; hello's plugin version is
  used only for the minor check and is not recorded. runtime.json per-tool
  ranges are never evaluated. No `tools/list_changed` is sent. The comment at
  `version.go:52-54` says the wire surface changes only in a minor; nothing
  checks it.
- Stores mostly read tolerantly, but Go struct round-trips can drop fields an
  older writer does not know (suspected; no per-writer audit yet). Only the
  config manifest and rsrc manifest check `schema_version` on read; sqlite
  has no guard against an older binary opening a newer schema.

The exploration above is historical, not a description of the current
launcher: minor skew now refuses connection rather than falling back to direct
execution (`agents-plugin/bin/ws-mcp-launcher.py:1457-1460`).

The directional T0–T4 taxonomy, scenario sheet, and release rules live in
`ai-docs/manuals/release-compatibility.md`, with the discussion-time rule in
`AGENTS.md` `### Compatibility Tiers`. This ticket builds the enforcement
those rules name.

## Decisions

### Current compatibility assessment - 2026-11-15

The policy landed in `c0ec803cb` supersedes the old tier names and approval
thresholds below, not the confirmed three-phase feature scope. T0 is transparent;
T1 is display-only degradation with no data change; T2 requires actual recovery
on new-version reopen after an old write. T0–T2 permit patches. T3 blocks the old
side's running, connection, read or write and requires a minor bump plus explicit
owner approval. T4 means the new side cannot read old data and must never ship;
migration must lower it to T3 or below.

- **Calls:** the old “T2 breaking” removal/protocol boundary is now **T3**.
  A behavior-preserving alias is normally **T0**, not reconnect-recoverable T1.
  The old caller supplies legacy names; the new dispatcher refuses unknown
  tools (`agents-plugin-tool/internal/mcp/server.go:2315-2316`) and undeclared
  arguments before the handler (`agents-plugin-tool/internal/mcp/tool_arguments.go:65-128`).
  These refusal paths write no handler data; reopening cannot restore a removed
  call. Refresh is not an alias. Preserve the existing alias commitment; removal
  at a minor still requires approval. A tracker event or store change is not
  automatically breaking: grade its actual readers and writers.
- **Connection:** an older plugin writes its hello version and the newer daemon
  reads and rejects a different minor before session acceptance
  (`agents-plugin-tool/internal/wsdashboard/proxy.go:232-254`): **T3**, not T2.
  No session/store write occurs on this refusal path; reopening data cannot fix
  connection refusal. The strong notice and quiet badge remain planned features,
  not permission to bypass this boundary. A cross-minor notice alone cannot
  waive a T3 data-safety refusal. Inspection uses existing hello or `/health`
  information without direct execution or daemon startup just to get a version.
- **Stores:** Phase 3 is an audit and guard target, not implemented proof of T0.
  For example, if a newer writer adds a registration field unknown to a
  current-style older binary, struct decode loses it and whole-file replacement
  removes it (`agents-plugin-tool/internal/wsdashboard/registry.go:75-140,236-254,280-316`).
  New reopen reads that replacement; this path retains no raw-field recovery.
  That is **not T2** and is not patch-safe as-is. Acceptance must demonstrate
  preservation, or an effective **T3** refusal before the older writer damages
  data, with the required minor and owner approval. A schema field alone does
  not protect against an already-installed writer that ignores it.
- **Planned additive paths:** optional hello fields, notices/badges and refresh
  target **T0** only when old peers continue normally and writes preserve new
  fields. A display-only unknown/default with no data change can be **T1**.
  Any claimed **T2** must cite the particular new writer/value, intervening old
  write, retained raw/append-only bytes, and the new reader's reopen
  reconstruction. No such T2 trace is established here. No blanket patch grade
  is settled for the complete ticket; the highest affected path controls release.

### Historical decisions (prior taxonomy)

These retain the original board record. The current assessment above supersedes
its old T0/T1-only patch rule, T2 label, unconditional “keeps working” wording,
and direct-execution fallback requirement. It does not select a new connection
architecture or authorize a refusal bypass; any design needed beyond the landed
policy must return to the owner before implementation.

- **Breaking boundary.** A patch release carries only T0/T1 changes; T2
  (removal without an alias, non-additive store change, protocol or tracker
  event change) lands only in a minor bump. Within a minor, tools and
  arguments are never removed and renames keep the old name as an alias for
  one minor. (User, board 341 a.)
  Rejected: allowing tool/argument removal in a patch and relying on
  reconnect plus error messages - old skill text runs on the new binary, so a
  patch-level removal keeps failing in every not-yet-updated package.
- **Minor-skew response.** A session whose plugin minor differs from the
  daemon's gets a strong notice injected for the agent ("plugin
  compatibility is broken; tell the user to update the plugin") plus a
  dashboard badge, and keeps working. A patch difference shows only a quiet
  dashboard badge. (User, board 342 a.)
  Rejected: refusing shared-store write tools for skewed sessions (newer-schema
  write refusal already prevents the actual corruption); a dashboard badge
  alone (the agent and user would still not be told).
- **The daemon judges skew.** It records each session's hello plugin version
  and compares it with its own; a session that fell back to direct execution
  makes the same judgment from the daemon's `/health` version. (Board 343.)
  Rejected: launcher-side judgment - the launcher knows only its own package,
  while the daemon sees every connecting package.
- **Tool-surface monotonicity is a test, not a document rule.** A test
  compares the current tool surface (tool names and argument names) with the
  previous release's surface and fails on a removal within the same minor.
  (Board 344.)
- **A writer refuses a newer `schema_version`.** A store writer that reads a
  `schema_version` newer than its own refuses to write and fails loud,
  instead of silently overwriting. A per-writer audit of read-modify-write
  round-trips comes first. (Board 345.)
  Rejected: silent overwrite with best-effort field preservation - a dropped
  field cannot be detected afterward.
- **Tool-list refresh after a daemon replacement.** Send
  `tools/list_changed` first; where a host ignores it, force a reconnect (the
  Claude mod watchdog widened from "tools disappeared" to daemon version or
  content-hash change; Pi re-fetches `tools/list` on reconnect). Whether
  Claude Code and Codex honor the notification is unverified; the probe
  result may make the forced reconnect the primary path. (Board 346.)
- **Notice placement.** The minor-skew notice is a leading line on every ws
  tool result of a skewed session. A tool result reaches the agent on every
  call, so a mid-session daemon replacement is announced at once. (Board 357.)
  Rejected: only the top of `workflow_manual` - it is read once per session
  start or compaction, so a mid-session replacement would go unannounced
  until the next revive.
- **Surface baseline.** The monotonicity test compares against a committed
  snapshot file of the last release's tool surface (tool and argument names),
  regenerated by the release step. (Board 358.)
  Rejected: reading a git tag at test time - not hermetic, and fails in
  shallow clones and worktrees without tags.
- **Ticket shape.** Three phases in this one ticket. (Board 359.)
  Rejected: one phase (too large for one worker cycle); three tickets (would
  duplicate the shared decision set).

## Constraints

- Cross-version surfaces here follow the current assessment in `## Decisions`
  and `ai-docs/manuals/release-compatibility.md`. Prefer T0 additions, but do not
  label newer-schema read/write refusal T0: an effective old-side refusal is
  T3. Settle each writer/reader/reopen trace and approval before release; the
  store audit may expose a boundary that cannot ship in a patch.
- Out of scope: the dev-only stale binary (scenario 6,
  `261112-bug-ws-launcher-fingerprint-skips-embedded-assets`) and
  cross-machine tracker ledger skew (scenario 7), which daemon rules cannot
  reach.
- Heavy Go build/test runs through the project's remote build wrapper, not
  locally (machine note `remote-go-build`).

## Phases

### Phase 1: Daemon skew detection, agent notice, and tool-list refresh

- Record the hello plugin version per hosted session; compare minor and patch
  against the daemon's own version.
- Minor skew: inject the strong agent notice (a leading line on every ws tool result) and show a
  dashboard badge on the session. Patch skew: quiet badge only.
- Use existing hello information or read-only `/health` inspection for version
  evidence; do not restore the historical direct-execution fallback or start a
  daemon solely for inspection. Preserve required T3 refusals rather than treating
  the notice as authorization for unsafe cross-minor operation.
- On daemon replacement, send `tools/list_changed`; probe whether Claude Code,
  Codex, and Pi honor it, and widen the Claude mod reconnect watchdog to a
  daemon version or content-hash change where they do not.
- Verification: unit tests for the skew classification (same, patch, minor)
  and notice injection; a test that version inspection reads existing hello or
  `/health` information without starting a daemon; the probe result recorded
  in the phase Result.

### Phase 2: Tool-surface monotonicity test

- Snapshot the tool surface (tool names, argument names) as a committed snapshot file and add a
  test that fails when the current surface removes a tool or argument present
  in the snapshot while the minor is unchanged. A minor bump resets the
  baseline, but does not substitute for explicit owner approval of T3 removal;
  refresh must never be treated as preserving removed calls.
- Verification: the test passes on the current tree and fails on a synthetic
  removal.

### Phase 3: Store writer audit and newer-schema write refusal

- Audit each shared-store writer (session parts, mailbox, inbox, sqlite,
  tracker, config) for read-modify-write round-trips that drop unknown
  fields; record the audit in the phase Result.
- Give each store that lacks one a `schema_version` and make writers refuse
  to write, failing loud, when the stored version is newer than their own.
  sqlite: refuse to open a newer `user_version` for writing. These are planned
  T3 write boundaries, not T0 merely because the version field is additive.
  Establish how each older writer actually refuses before damaging writes;
  do not infer protection for installed binaries that ignore the field. If the
  audit needs an unchosen migration or guard architecture, stop for owner
  direction rather than widening this plan.
- Verification: per-store tests that an older writer refuses a newer
  schema and that a same-version round-trip preserves unknown fields where the
  audit says it must. Include old-data reads by the new binary (T4 is forbidden),
  and cite actual new reopen recovery before claiming any T2 overwrite.
