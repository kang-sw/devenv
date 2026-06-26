# Brief: 260625-ws-session-state-machine-p3b

## Intent

Restructure the manual-entry skill surface so post-compaction recovery routes
through the `ws.workflow_manual` tool, and harden that tool so a subagent cannot
obtain lead self-bootstrap guidance. Two coupled deliverables:

1. **Phase 3a security hardening (fold-in).** `ws.workflow_manual` currently
   serves fresh-mode (the self-bootstrap line that teaches `ws.ferrule`, the
   lead privilege-escalation call) to ANY keyless caller, and also to any
   unresolvable key. That re-exposes guidance the old playbook kept behind a
   skill read. Make a valid `session_key` mandatory; gate the tool lead-only;
   serve fresh mode ONLY for a reserved sentinel key taught only in lead skills.
2. **Phase 3b skill restructure.** Remove the `lead-load-workflow-manual`
   launcher, add a `lead-revive` launcher, and repoint the manual self-load line
   in the four lead skills that carry it to call `ws.workflow_manual`.

## Scope Boundary

In scope:
- Go: `internal/mcp/workflow_manual.go` handler rewrite (key required + sentinel
  + fail-loud strips bootstrap); `internal/mcp/server.go` `isLeadOnlyTool` +
  `ws.workflow_manual` tool schema (mark `session_key` required, scrub the
  keyless/sentinel cue from the advertised description).
- Go tests in `internal/mcp/`.
- Skills: remove `agents-plugin/skills/lead-load-workflow-manual/`; add
  `lead-revive` SKILL.md to BOTH `agents-plugin/skills/` and
  `agents-plugin-wsflow/skills/`; repoint the self-load line in
  `lead-proceed`, `lead-discuss`, `lead-sprint`, `lead-salvage` rsrc bodies (+
  wsflow rsrc mirror via regen).
- Regenerate `agents-plugin/rsrc/manifest.json` then the wsflow rsrc mirror.

Deferred / NOT in this slice (later Phase 2 remainder, separate slices):
- `lead-forge-spec` / `lead-forge-mental-model` host-task → `ws.todo` migration.
- `lead-sprint` `Sprint-Edit:` marker-resume rewrite.
- `delegate-orientation.md` agenda/todo/enter contract documentation.
- Spec (`mcp-tools.md`, `plugin-runtime.md`) + mental-model doc updates — handled
  by the lead in the Doc Pre-Pass stage, NOT by the implementer. Do not edit
  `ai-docs/spec/` or `ai-docs/mental-model/`.

## Caller-Visible Contract

`ws.workflow_manual(session_key)` — `session_key` is now REQUIRED. Behavior by key:

| `session_key` | Result |
|---|---|
| absent / empty | Error: a valid `session_key` is required. No manual body, no bootstrap line. |
| the reserved fresh-bootstrap sentinel (see Details) | Fresh mode: full manual incl. the gated self-bootstrap line (`ws.ferrule` guidance). |
| resolves to a **lead**-scoped record | Continue mode: manual (bootstrap line stripped) + `## Session State` (agenda remind + todo summary). |
| resolves to a **delegate/leaf**-scoped record | Rejected by the keyed capability gate ("tool not available in current MCP profile"). Never reaches the handler. |
| syntactically valid but unresolvable | Fail-loud: manual with the self-bootstrap line STRIPPED + an explicit no-restorable-state notice. No key minted. |

The advertised tool schema must NOT reveal the sentinel value, nor invite a
keyless call. The sentinel is taught only in lead skill prose.

Skill surface:
- `lead-load-workflow-manual` skill no longer exists.
- `lead-revive` skill exists (host-surfaced strong-attention post-compaction entry).
- The four repointed lead skills load the manual via `ws.workflow_manual`, not
  `playbook.print(name: "lead-workflow-manual")`.

## Contract Instructions

### Go — `internal/mcp/workflow_manual.go` (`handleWorkflowManual`)

Replace the `switch` branching (currently keyless→fresh, recOK→continue,
default→fail-loud-keeps-bootstrap) with, in this order:

1. `key == ""` → return an error response: `ws.workflow_manual: a valid session_key is required`. (Do not name the sentinel or ferrule in the error.)
2. `key == <sentinel const>` → FRESH: `stripModeGatedRegion(body, true)` (keep bootstrap line). Same as today's keyless branch.
3. `recOK` (record resolves) → CONTINUE: `stripModeGatedRegion(body, false)` + `renderSessionState(rec)`. Unchanged.
4. default (unresolvable, non-sentinel key) → FAIL-LOUD: `stripModeGatedRegion(body, false)` — **strip** the bootstrap line (changed from keep) — then append the existing no-restorable-state notice. Never mint.

Define the sentinel as a package const near the top of `workflow_manual.go`
(beside `freshOnlyStart`), e.g. `const freshBootstrapKey = "<sentinel value>"`,
with a comment explaining it is the reserved fresh-mode trigger taught only in
lead skills, mirroring `ws.ferrule`'s "no semantic cue" rationale.

Note: `rec, recOK := s.sessions.readState(key)` may stay computed before the
switch; the sentinel branch precedes the `recOK` branch so a (non-existent)
sentinel record never matters.

### Go — `internal/mcp/server.go`

- `isLeadOnlyTool` (line ~59): add `|| name == "ws.workflow_manual"`. This makes
  the keyed capability gate (line ~340-346) reject delegate/leaf-scoped keys
  before dispatch — same mechanism that protects `ws.ferrule` from privilege
  escalation. Lead keys and keyless/sentinel/unresolvable keys are unaffected by
  the gate (lookup miss or lead scope), so they reach the handler and follow the
  truth table.
- `ws.workflow_manual` tool definition (line ~2961): rewrite `description` to
  remove the keyless/bootstrap-line/sentinel cue. New description states: renders
  and restores the manual + session state for a lead `session_key`; an
  unresolvable key returns a fail-loud notice and never mints; lead-only. Change
  `session_key` property description to "Required. Your lead session key." and
  add `"required": ["session_key"]` to the inputSchema (remove the "intentionally
  optional" comment).

### Skills — removal

- `git rm -r agents-plugin/skills/lead-load-workflow-manual/`. No rsrc body, no
  manifest entry. Not present in `agents-plugin-wsflow/skills/` → no wsflow
  removal. (The `_index.md` reference is updated by the lead in Doc Pre-Pass.)

### Skills — add `lead-revive` (skills-only launcher, no rsrc body)

Mirror the structure of the removed `lead-load-workflow-manual/SKILL.md`
(SKILL.md only, inline body). Two files, per-tree namespace literal:

`agents-plugin/skills/lead-revive/SKILL.md` (use `ws/` literal):
```markdown
---
name: lead-revive
description: Post-compaction recovery. If this session was compacted or continued, invoke this BEFORE any other ws lead skill, passing the session_key preserved in the compaction summary, to restore agenda/todo state and reload the workflow primitives.
---

# Revive

Recover your ws `session_key` from the compaction summary, then call
`ws/workflow_manual(session_key: <recovered key>)` and execute the returned
reference inline. If no key is recoverable (genuinely fresh start), call
`ws/workflow_manual(session_key: "<sentinel value>")` to bootstrap.
```

`agents-plugin-wsflow/skills/lead-revive/SKILL.md`: identical except `wsflow/`
literal in both tool calls, and (matching the wsflow SKILL.md house style) append
"If the tool cannot be loaded, stop and report that blocker." to the body.

### Skills — repoint the 4 self-load lines

Each currently reads (modulo list prefix `-` vs `1.`):
`Call \`{{.McpNamespace}}/playbook.print(name: "lead-workflow-manual")\` and execute the returned reference inline.` (lead-proceed and lead-discuss additionally append a reload clause).

Replace the call+reload text on each with this unified instruction (PRESERVE the
existing list prefix and surrounding step numbering):

```
Call `{{.McpNamespace}}/workflow_manual(session_key: <your lead key>)` and execute the returned reference inline; reload after session compaction (a duplicate load is safe). After compaction, recover your key via `{{.SkillNamespace}}:lead-revive` first. No lead key yet (fresh start)? Call `{{.McpNamespace}}/workflow_manual(session_key: "<sentinel value>")` to bootstrap.
```

Exact locations (verify by content, line numbers may drift):
- `agents-plugin/rsrc/lead-proceed/lead-proceed.md` — Invariants bullet (`- Call ...`), currently ends "Always reload after session compaction; a duplicate load is safe."
- `agents-plugin/rsrc/lead-discuss/lead-discuss.md` — `## On: invoke` step `1.`, currently ends "Reload after session compaction; a duplicate load is safe."
- `agents-plugin/rsrc/lead-sprint/lead-sprint.md` — `## On: invoke` step `1.` (no reload clause today; the unified text adds one — intended consistency normalization).
- `agents-plugin/rsrc/lead-salvage/lead-salvage.md` — `## On: invoke` step `1.` (same as sprint).

Do NOT touch other `lead-workflow-manual` references in these files (reference-
discovery / exploration-worker dispatch prose at lead-discuss lines ~22/56/58):
those point at the manual playbook content, which still exists and is served by
`ws.workflow_manual`. Do NOT touch `lead-tune` or `lead-skill-authoring` — they
have NO self-load line (only prose references to the surviving manual playbook).

### Regen (order matters)

From repo root, after all rsrc edits:
1. `cd agents-plugin-tool && WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`
2. `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
3. Stage BOTH regenerated files (`agents-plugin/rsrc/manifest.json` and the wsflow
   mirror under `agents-plugin-wsflow/rsrc/`). The wsflow manifest mirror is a
   common staging miss — verify `git status` shows it staged.

## Integration Test Instructions

File: `agents-plugin-tool/internal/mcp/session_state_test.go` (+ a skill render
test in `playbook_tools_test.go`). Run:
`cd agents-plugin-tool && go test ./internal/mcp ./internal/wsrsrc -count=1`

- **Edit `TestWorkflowManualFreshMode`**: call with the sentinel key (via
  `callToolWithKey`, not `callToolNoKey`); keep the existing fresh-mode assertions
  ("mint your lead key" present, "once per working root" present, no "Session
  State").
- **New `TestWorkflowManualKeylessRejected`**: `callToolNoKey(... "ws.workflow_manual", nil)`
  → response/error mentions a required `session_key`; asserts "mint your lead key"
  is ABSENT.
- **Edit `TestWorkflowManualUnknownKey`**: keep the no-restore-notice + no-mint
  assertions; ADD an assertion that "mint your lead key" is ABSENT (fail-loud now
  strips the bootstrap line).
- **New `TestWorkflowManualDelegateKeyBlocked`**: mint a delegate key
  (`server.sessions.mint(root, roleDelegate, "")`, pattern in
  `session_auth_test.go:459`), call `ws.workflow_manual` with it through
  `callToolWithKey` → response is the lead-only profile rejection ("tool not
  available in current" / profile error), NOT a manual body.
- **Skill render/presence test** (`playbook_tools_test.go`, near
  `TestSkillsCallEnterTools`): assert the four repointed skills' rendered bodies
  contain `workflow_manual` and `lead-revive` and NO longer contain
  `playbook.print(name: "lead-workflow-manual")` on the self-load line; assert
  `lead-revive` SKILL.md exists and `lead-load-workflow-manual` does not (both
  trees as applicable).
- Drift guards `TestShippedManifestUpToDate` + `TestWsflowRsrcMirrorUpToDate`
  must pass after regen.

Pass criteria: `go test ./internal/mcp ./internal/wsrsrc -count=1` green except
the THREE known pre-existing `internal/mcp` failures unrelated to this work
(`TestShippedDelegationSectionSeedAndOverride`,
`TestShippedUserPreferenceSectionEmptySlotAndOverride`,
`TestConfigPromptSetEndToEnd` — confirm they fail identically on the parent
commit before attributing).

## Implementation Strategy Decisions

- Sentinel-gated fresh mode, NOT keyless fresh mode. The leak is that keyless (or
  any-key) access reveals the `ws.ferrule` escalation guidance; requiring a
  lead-skill-taught sentinel restores the playbook-era "behind a skill read"
  property and matches `ws.ferrule`'s own "no semantic cue" design.
- Lead-only gate via `isLeadOnlyTool` (reuse existing mechanism), not a new
  per-role advertise filter. Mirrors `ws.ferrule`.
- `lead-revive` is a skills-only inline launcher (no rsrc, no `playbook.print`
  self-reference), mirroring the removed `lead-load-workflow-manual`.
- `lead-revive` is added to wsflow/skills too, because the repointed wsflow rsrc
  bodies reference `{{.SkillNamespace}}:lead-revive`; omitting it would dangle in
  product mode.
- Unified repoint text applied to all four skills (incl. sprint/salvage that
  lacked a reload clause) for one canonical manual-load instruction.

## Rejected Alternatives

- Keep keyless fresh mode + only add the lead-only gate: rejected — keyless calls
  carry no role, so the gate can't catch them, and the ferrule guidance stays
  exposed. (User decision.)
- `permanentlyHiddenTool` (like `exec.*`): rejected — the lead must still reach
  the tool via ToolSearch; full hiding breaks legitimate use.
- Repoint all six skills named in the ticket: rejected — `lead-tune` and
  `lead-skill-authoring` have no self-load line; their references are to the
  surviving manual playbook content, not the removed launcher.

## Approach

- Land Go handler + gate + schema + tests first; verify the truth table.
- Then skill removal/add/repoint; regen; verify drift guards + render tests.
- Commit logical checkpoints on the current branch
  (`implement/260625-ws-session-state-machine`).

## Constraints

- Skill/agent prose: apply `lead-skill-authoring` invariant checklist; one-line
  command-shaped directives; refer to other skills only as
  `{{.SkillNamespace}}:<skill>` invocation targets (never bare names).
- Namespace rule: skill prose uses `{{.McpNamespace}}/workflow_manual` and
  `{{.SkillNamespace}}:lead-revive` — NEVER hardcoded `ws.`/`ws:` (the wsflow
  product-mode render test fails on bare `ws[/:]` leakage). SKILL.md launchers are
  the exception: they use the literal per-tree namespace (`ws/` in agents-plugin,
  `wsflow/` in agents-plugin-wsflow), as existing launchers already do.
- The sentinel value must NOT appear in the advertised tool description, the
  `session_key` schema text, or any error message — only in lead skill prose and
  the Go const.
- Canonical and wsflow rsrc mirrors must stay byte-identical (drift guards);
  regen WS_REGEN_MANIFEST first, then WS_REGEN_WSFLOW_RSRC.
- Do not bump the plugin version (dev-merge concern, not this slice).
- Do not edit `ai-docs/spec/`, `ai-docs/mental-model/`, `_index.md`, or the
  ticket — the lead owns those in later stages.

## Out of scope

See Scope Boundary "Deferred". Also: do not modify `lead-bootstrap` (it has no
manual self-load step), `enter.*`/`todo.*`/`agenda.*` role access (intentionally
delegate-reachable per D3 scoping).

## Details

- **Sentinel value**: a deliberately non-descriptive token with no privilege or
  bootstrap cue (per the user's "비서술 토큰" decision and the ferrule philosophy).
  Use `obsidian-latch` unless the reviewer/lead supplies another. Go const name:
  `freshBootstrapKey`. It must read as an opaque handshake token, not as
  "root/admin/key/bootstrap".
- Existing helpers: `callToolNoKey`, `callToolWithKey`, `callLogin` /
  `parseLoginResponse` (mints a lead key), `server.sessions.mint(root, scope, parent)`.
- `bootstrapToolName = "ws.ferrule"` (server.go:52) is the analog to follow.

## Verification Contract

- `cd agents-plugin-tool && go build ./...` clean.
- `go test ./internal/mcp ./internal/wsrsrc -count=1` green except the three known
  pre-existing failures listed above.
- New/edited tests: FreshMode-via-sentinel, KeylessRejected, UnknownKey-strips-
  bootstrap, DelegateKeyBlocked, skill render/presence all pass.
- Drift guards green; `git status` shows both regenerated manifests staged.
- Report the exact test command outputs read in full (no "should pass").

## References
<!-- [Must] read before starting. [Maybe] consult if uncertain. -->
- `ai-docs/mental-model/mcp-runtime.md` — [Must] workflow_manual contract, session
  scoping, lead-only gate rationale.
- `ai-docs/mental-model/workflow-skills.md` — [Must] manual-load invariant, skill
  authoring conventions, Common Mistakes.
- `agents-plugin/skills/lead-skill-authoring/SKILL.md` and its rsrc body — [Must]
  invariant/constraint checklist for skill prose.
- `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md` — [Maybe] the
  fresh-only marker region (lines ~54-61) and ferrule "no semantic cue" rationale.
