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

The tier taxonomy (T0 transparent, T1 reconnect-recoverable, T2 breaking),
the scenario sheet, and the release rules live in
`ai-docs/manuals/release-compatibility.md`, with the discussion-time rule in
`AGENTS.md` `### Compatibility Tiers`. This ticket builds the enforcement
those rules name.

## Decisions

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

## Open Decision Queue

Last ID: (359)

(357) [open] [technical] Inject the skew notice as a leading line on every ws tool result of a skewed session, over only the top of `workflow_manual`
- A tool result reaches the agent on every call, while the manual is read once per session start or compaction, so a mid-session daemon replacement would go unannounced until the next revive.

(358) [open] [technical] Compare the tool surface against a committed snapshot file of the last release's surface (tool and argument names), regenerated at release, over reading a git tag at test time
- A committed snapshot keeps the test hermetic and works in shallow clones and worktrees without tags; the release step already rewrites runtime.json.

(359) [open] [technical] Three phases in one ticket (skew detection and tool-list refresh; monotonicity test; store audit and newer-schema refusal), over three tickets or one phase
- The phases share the decisions and the manual above, but each is separately reviewable; one phase would be too large for one worker cycle, and three tickets would duplicate the decision set.

## Constraints

- Cross-version surfaces touched here (hello, store writers, tool surface)
  are themselves subject to `AGENTS.md` `### Compatibility Tiers`: every
  addition in this ticket must be T0 (optional hello fields, optional store
  fields, additive notices), so it ships in a patch and an older peer ignores
  it.
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
- Minor skew: inject the strong agent notice (placement per (357)) and show a
  dashboard badge on the session. Patch skew: quiet badge only.
- Direct-execution fallback sessions query the daemon's `/health` version and
  apply the same judgment.
- On daemon replacement, send `tools/list_changed`; probe whether Claude Code,
  Codex, and Pi honor it, and widen the Claude mod reconnect watchdog to a
  daemon version or content-hash change where they do not.
- Verification: unit tests for the skew classification (same, patch, minor)
  and notice injection; a test that a fallback session reads `/health`; the
  probe result recorded in the phase Result.

### Phase 2: Tool-surface monotonicity test

- Snapshot the tool surface (tool names, argument names) per (358) and add a
  test that fails when the current surface removes a tool or argument present
  in the snapshot while the minor is unchanged. A minor bump resets the
  baseline.
- Verification: the test passes on the current tree and fails on a synthetic
  removal.

### Phase 3: Store writer audit and newer-schema write refusal

- Audit each shared-store writer (session parts, mailbox, inbox, sqlite,
  tracker, config) for read-modify-write round-trips that drop unknown
  fields; record the audit in the phase Result.
- Give each store that lacks one a `schema_version` (additively), and make
  writers refuse to write, failing loud, when the stored version is newer
  than their own. sqlite: refuse to open a newer `user_version` for writing.
- Verification: per-store tests that an older writer refuses a newer
  schema and that a same-version round-trip preserves unknown fields where the
  audit says it must.
