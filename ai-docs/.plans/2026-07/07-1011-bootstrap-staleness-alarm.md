# Plan: 260703-chore-bootstrap-staleness-alarm — Phase 1: Survey and implement session-bootstrap staleness warning

## Relevant Ticket Contract

- Injection point: session-bootstrap time (`ws/ferrule` / `ws/workflow_manual`
  load path), not every `ws/project_tree` call.
- Config surface: extend the existing layered config-item pattern with a new
  `wsconfig.Item*` (e.g. `ItemBootstrapAlarm`, values `on`/`off`, builtin
  default `on`), mirroring `ItemWorkflowPreferSubagent`, exposed through
  `config.tuning`/`ws:lead-tune`. No standalone `config.set_flag` tool.
- Warning message must point to how to permanently silence it via the new
  config item's setter surface (do not hardcode a tool-call string ahead of
  the actual setter — the setter is built in this same phase, so reference it
  directly).
- New Go-side work required: a reader for the downstream project's root
  `AGENTS.md` `<!-- Template Version: vNNNN -->` tag, and a source of "latest
  known version" per package (ws vs wsflow), package-local (never
  cross-package comparison).
- No-tag case: default to silent (untagged project never opted into ws
  bootstrap), not maximally-stale.
- Verification boundary (from Phase 1 body): a test confirming the warning
  fires when installed tag is behind latest, is suppressed when
  `ItemBootstrapAlarm` is off, is silent when no tag is present, and that
  `config.tuning`/`ws:lead-tune` lists and can set the new item.

## Out of Scope

- Changing `lead-bootstrap`'s own upgrade/migration procedure (ticket-level
  Out of Scope).
- Any per-`project_tree`-call variant of this warning (ticket-level Out of
  Scope).
- Spec authoring: ticket's `## Spec Impact` says spec addressing is left for
  the implementation-survey pass promoting this ticket to `ready/` — the
  ticket is already in `ready/`, so spec impact should be captured as part of
  this implementation, but no separate spec-authoring ticket work is created
  here; addressing spec is part of the same commit per repo commit-rule
  `## Spec` trailer if a spec doc exists for MCP tool contracts
  (`ai-docs/spec/mcp-tools.md`) — see Implementation Plan step 8.
- wsflow-specific manual verification (no wsflow build/test harness surveyed
  here); the reader is package-local by construction (see Codebase Findings)
  so no wsflow-specific code branch is needed, but a wsflow live test run is
  out of scope for this phase.

## Codebase Findings

- `agents-plugin-tool/internal/wsconfig/scope.go#L26-L30,L72` —
  `ItemWorkflowPreferSubagent` is the exact sibling shape to mirror: a
  `RegisterGlobalOnly` item, builtin default resolved through
  `builtinConfigDefaults()`, values `on`/`off`. Add
  `ItemBootstrapAlarm = "bootstrap_alarm"` next to it and
  `RegisterGlobalOnly(ItemBootstrapAlarm)` in the same `init()`.
- `agents-plugin-tool/internal/mcp/server.go#L319-L325` — `builtinConfigDefaults()`
  returns the map consumed everywhere resolvers are built; add
  `wsconfig.ItemBootstrapAlarm: "on"`.
- `agents-plugin-tool/internal/mcp/server.go#L547-L583` — `config.workflow_prefer_subagent`
  handler is the exact writer pattern to clone for a new
  `config.bootstrap_alarm` case: `requireLeadSessionKey`, mutually-exclusive
  `value`/`reset`, `on`/`off` validation, `resolver.Set`/`resolver.Unset`
  against `builtinConfigDefaults()`.
- `agents-plugin-tool/internal/mcp/server.go#L3111-L3120` — tool schema block
  for `config.workflow_prefer_subagent` to clone verbatim (name, description,
  `value`/`reset` properties) for `config.bootstrap_alarm`.
- `agents-plugin-tool/internal/mcp/server.go#L69-L74` — `workflowPreferenceWriterTool`
  switch gates lead-only-tool status via `isLeadOnlyTool` (`#L65-L67`); add
  `"config.bootstrap_alarm"` to this switch so the new writer is lead-only,
  matching its siblings.
- `agents-plugin-tool/internal/mcp/server.go#L3707-L3724` — `toolSchemaRequiresSessionKey`
  switch; add `"config.bootstrap_alarm"` alongside
  `"config.workflow_prefer_subagent", "config.workflow_prefer_mercenary"` so
  the schema auto-requires `session_key`.
- `agents-plugin-tool/internal/mcp/server.go#L1698-L1766` — `buildTuningCatalog`
  is where every tuning knob is appended (`workflow.prefer_subagent` block at
  `#L1725-L1736` is the template: `ID`, `Kind`, `Description`, `Writer`,
  `Reset`, `ValueFields`, `Current` via `currentWorkflowPreference`). Append a
  `bootstrap_alarm` knob the same way, using `currentWorkflowPreference(resolver,
  wsconfig.ItemBootstrapAlarm)` for `Current` (works unmodified since it is a
  plain on/off value, not the mercenary tri-state).
- `agents-plugin-tool/internal/mcp/server.go#L1414-L1443` — `handleLeadLogin`
  (the `ferrule` handler, dispatched at `#L434`) has `canonical` (the
  resolved downstream project root) in scope right before building `result`.
  This is the first session-bootstrap injection point: read/compare AGENTS.md
  here and add a field to `result` (JSON path) plus a line to the text-format
  return (`#L1442`).
- `agents-plugin-tool/internal/mcp/workflow_manual.go#L188-L276` —
  `handleWorkflowManual`: three branches all end by building `body` then
  `return toolTextResponse(id, body+"\n", nil)`. `canonical` is in scope in
  the FRESH-with-root branch (`#L237-L258`); `rec.Root` is available in the
  CONTINUE branch (`#L264-L273`) since `sessionRecord.Root` is persisted
  (`session_auth.go#L44-L60`). The FRESH-without-root branch (`#L259-L263`,
  sentinel only, no established root) has no root to check — skip it, mirror
  how `injectSkepticalPosture` is conditionally applied per branch.
- `agents-plugin-tool/internal/mcp/workflow_manual.go#L118-L134` —
  `skepticalPostureBlock` + `injectSkepticalPosture` is the exact structural
  precedent for a config-gated banner prepended to `body`: a package-level
  const string plus a small injector function, called conditionally per
  branch based on a resolved config value. Model the new warning injector the
  same way (`injectBootstrapStalenessWarning(body, msg string) string`).
- `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md#L291-L306`
  (`## Decision: agentId continuity — tip-only`) — resolves the apparent
  tension in the ticket's Decisions section: "inject at the point of action,
  not on every subsequent call" is a contrast against unrelated tools
  (`project_tree`), not against repeat calls of the *same* bootstrap tool.
  The agentId tip fires on every `playbook.render`/`print` call (the action
  itself), not once per session. So firing the staleness check on every
  `ferrule` call and every `workflow_manual` call (FRESH-with-root and
  CONTINUE) is consistent with this precedent — no extra "already warned this
  session" suppression state is required.
- `agents-plugin-tool/internal/wsrsrc/loader.go#L32-L58` — `ResolveRoot()` /
  `ResolveSkillsRoot()` resolve to the *currently running plugin's* rsrc/skills
  tree (via `WS_RSRC_ROOT`/`WS_SKILLS_ROOT` env override or the executable's
  sibling directory). This means "latest known version for this package" can
  be read directly from
  `filepath.Join(wsrsrc.ResolveSkillsRoot(), "lead-bootstrap", "AGENTS.template.md")`
  with **no ws-vs-wsflow branching in Go**: whichever package's MCP binary is
  running already resolves to that package's own template file. This
  confirms the ticket's "without hand duplication" question — reuse
  `ResolveSkillsRoot`, do not build a separate manifest.
- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md#L191` (and
  `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md#L163`) — both
  templates end with their own `<!-- Template Version: vNNNN -->` tag (ws:
  `v0043`; wsflow: `v0004`), which is already the max of that file's numbered
  checklist. Parsing this one tag (same regex as used for the downstream
  root `AGENTS.md`) is simpler and sufficient — no need to scan/parse the
  numbered `- vNNNN:` checklist list at all.
- `agents-plugin-tool/internal/mcp/*.go` (via
  `grep -rn "AGENTS.md\""`, no non-test hits) — confirms the ticket's claim:
  no existing reader to reuse; this is genuinely new code. Add it as a new
  file `agents-plugin-tool/internal/mcp/bootstrap_alarm.go` (mirrors the
  one-feature-per-file convention of `workflow_manual.go`,
  `session_config_adapter.go`).
- Risk signal / dogfood note: this repo's own root `AGENTS.md` currently
  carries `<!-- Template Version: v0041 -->` (`AGENTS.md#L224`) while
  `agents-plugin/skills/lead-bootstrap/AGENTS.template.md` is at `v0043`
  (`#L191`). Once implemented, this repo's own sessions will immediately
  start seeing the new warning at bootstrap (a live, real staleness case —
  useful for manual verification, not a bug in the new code).

## Implementation Plan

1. `agents-plugin-tool/internal/wsconfig/scope.go` — add
   `ItemBootstrapAlarm = "bootstrap_alarm"` constant (doc comment: builtin
   default "on", values on/off, global-only, gates the session-bootstrap
   staleness warning) and `RegisterGlobalOnly(ItemBootstrapAlarm)` in
   `init()`, next to `ItemWorkflowPreferSubagent`.
2. `agents-plugin-tool/internal/mcp/server.go#L319-L325` — add
   `wsconfig.ItemBootstrapAlarm: "on"` to `builtinConfigDefaults()`.
3. New file `agents-plugin-tool/internal/mcp/bootstrap_alarm.go`:
   - `parseTemplateVersionTag(content string) (int, bool)` — regex
     `<!--\s*Template Version:\s*v(\d+)\s*-->` (reuse the exact tag text from
     the ticket/template), returns `(0, false)` when absent/malformed.
   - `readTemplateVersion(path string) (int, bool)` — reads the file (missing
     file → `(0, false)`, same as no-tag: silent), delegates to
     `parseTemplateVersionTag`.
   - `latestKnownTemplateVersion(skillsRoot string) (int, bool)` — reads
     `filepath.Join(skillsRoot, "lead-bootstrap", "AGENTS.template.md")` via
     `readTemplateVersion`.
   - `bootstrapStalenessWarning(root, skillsRoot string, resolver
     *wsconfig.Resolver, sessionKey string) string` — orchestrator: resolves
     `ItemBootstrapAlarm` (off → return ""), reads downstream
     `filepath.Join(root, "AGENTS.md")` version (absent/malformed → return "",
     the no-tag-silent rule), reads latest via `latestKnownTemplateVersion`
     (absent/malformed → return "", fail-safe silent — do not warn off of an
     unreadable "latest"), compares; returns "" when not stale (installed >=
     latest). When stale, returns a one-line/blockquote warning naming the
     installed and latest versions and instructing:
     `Run config.bootstrap_alarm(value: "off") to silence this permanently.`
     (mirrors the `config.workflow_prefer_subagent` writer name/shape from
     step 4, satisfying the "point to the actual setter" decision).
   - `injectBootstrapStalenessWarning(body, warning string) string` —
     no-op passthrough when `warning == ""`, else prepend
     (same shape as `injectSkepticalPosture` in `workflow_manual.go#L131-L134`).
4. `agents-plugin-tool/internal/mcp/server.go` — add `config.bootstrap_alarm`
   writer case (clone `config.workflow_prefer_subagent`,
   `#L547-L583`): `requireLeadSessionKey`, mutually-exclusive
   `value`(`on`/`off`)/`reset`, `resolver.Set`/`Unset` against
   `wsconfig.ItemBootstrapAlarm`. Add its tool schema entry near
   `#L3111-L3120` (clone shape, adjust name/description to reference the
   staleness warning). Add `"config.bootstrap_alarm"` to
   `workflowPreferenceWriterTool` (`#L69-L74`) and to
   `toolSchemaRequiresSessionKey` (`#L3707-L3724`).
5. `agents-plugin-tool/internal/mcp/server.go#L1698-L1766` —
   `buildTuningCatalog`: append a `bootstrap_alarm` `tuningKnob` (ID
   `"bootstrap_alarm"`, Kind `"workflow_preference"`, Writer/Reset pointing
   at `config.bootstrap_alarm`, `Current` via
   `currentWorkflowPreference(resolver, wsconfig.ItemBootstrapAlarm)`),
   placed after the `workflow.prefer_subagent` block (`#L1725-L1736`), before
   the `noAgentMode` early-return (`#L1738-L1740`) since this knob is
   agent-mode-independent like `workflow.prefer_subagent`.
6. `agents-plugin-tool/internal/mcp/server.go#L1414-L1443` (`handleLeadLogin`)
   — after `canonical` is computed and before building `result`, call
   `bootstrapStalenessWarning(canonical, skillsRoot, resolver, "")` (build a
   `sessionConfigAdapter`-backed resolver exactly as
   `config.workflow_prefer_subagent` does at `#L553-L554`; resolve
   `skillsRoot` via `wsrsrc.ResolveSkillsRoot()`, propagating its error the
   same way other handlers propagate `resolveRsrcRoot` errors). When
   non-empty, add `result["bootstrap_alarm"] = warning` for the JSON path and
   append `"\n" + warning + "\n"` to the text-format return (`#L1442`).
7. `agents-plugin-tool/internal/mcp/workflow_manual.go` — in
   `handleWorkflowManual`:
   - FRESH-with-root branch (`#L237-L258`): after `canonical` is resolved,
     compute the warning the same way as step 6 and call
     `injectBootstrapStalenessWarning(body, warning)` alongside the existing
     `injectSkepticalPosture` call.
   - CONTINUE branch (`#L264-L273`): compute the warning using `rec.Root`
     and call `injectBootstrapStalenessWarning` the same way.
   - FRESH-without-root branch (`#L259-L263`): no change (no root available).
8. `ai-docs/spec/mcp-tools.md` — add/update the entry for `ferrule` /
   `workflow_manual` describing the new staleness-warning behavior and the
   new `config.bootstrap_alarm` tool + `bootstrap_alarm` tuning knob, per the
   repo's commit-rule requirement to include a `## Spec` trailer when a spec
   doc's contract changes. Read the current `ferrule`/`workflow_manual`
   entries in that file first to match its existing structure before editing.

## Verification Plan

- `cd agents-plugin-tool && go build ./...` and
  `go test ./internal/wsconfig/... ./internal/mcp/...`.
- New unit tests (new file `bootstrap_alarm_test.go` in
  `agents-plugin-tool/internal/mcp/`, following the `useLeadProfile(t)` /
  `t.TempDir()` / `initGit(t, root)` setup pattern from
  `session_state_test.go#L2513-L2526`):
  1. Stale case: write a temp downstream root `AGENTS.md` with an old
     `<!-- Template Version: v0001 -->` tag and a temp skills-root
     `lead-bootstrap/AGENTS.template.md` with a higher tag (set
     `WS_SKILLS_ROOT`/`WS_RSRC_ROOT` env per the `wsrsrc.ResolveRoot`
     override, `loader.go#L32-L58`); call `ferrule` and assert the warning
     text (and installed/latest version numbers) appears in the response.
     Repeat for `workflow_manual` FRESH-with-root and CONTINUE.
  2. Suppressed case: same fixture, `config.bootstrap_alarm` set to `off`
     (global scope, since the item is global-only) before calling
     `ferrule`/`workflow_manual`; assert no warning text appears.
  3. No-tag case: downstream root `AGENTS.md` with no Template Version tag
     at all (or file absent); assert silent (no warning), even though
     "latest" is presumably higher than "0" — confirms the ticket's explicit
     no-tag-silent rule.
  4. `config.tuning`/`config.bootstrap_alarm` tests mirroring
     `TestWorkflowPreferSubagentWriterProductionPath` /
     `TestWorkflowPreferSubagentResetRestoresBuiltin`
     (`prefer_mercenary_phase2_test.go#L193-L280`): set/reset via
     `config.bootstrap_alarm`, and assert `config.tuning` output lists the
     `bootstrap_alarm` knob with the resolved current value/scope.
- Manual/dogfood check (optional, cheap): after implementing, calling
  `ferrule` against this repo's own root should surface the warning, since
  this repo's own `AGENTS.md` is at `v0041` vs the shipped template's
  `v0043` (see Codebase Findings dogfood note) — a natural end-to-end smoke
  check with zero fixture setup.

## Escalations

- None.
