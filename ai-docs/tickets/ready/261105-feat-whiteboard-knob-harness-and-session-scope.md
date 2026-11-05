---
title: workflow.whiteboard_enabled - per-harness and session-scoped values
related:
  - 261030-feat-session-whiteboard-mode
  - 261105-feat-lead-ticket-review-auto-ack
  - 261105-feat-whiteboard-explicit-item-ref-syntax
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 04ce7a068fe675a6
sage-review-completeness-reviewed: 04ce7a068fe675a6
---

# workflow.whiteboard_enabled - per-harness and session-scoped values

## Background

Dogfood observation (2026-11-05): the user wanted the whiteboard on for the
Claude Code harness only, as a global preference, and found that
`workflow.whiteboard_enabled` cannot express that. The knob is global-only
(`RegisterGlobalOnly` in `agents-plugin-tool/internal/wsconfig/scope.go`)
and has no harness selector, so a global `on` also turns the whiteboard on
for Codex and Pi leads.

### Why it is global-only today

`261030-feat-session-whiteboard-mode` declared the knob global-only after
the `workflow.prefer_subagent` precedent (technical decision (101)), with
the reason "`handleWorkflowManual` renders the manual with
`wsconfig.Options{}` and no repo root, so a repo-scoped override would be
invisible to the manual gate". `workflow.prefer_subagent` itself became
global-only in f4144db58 so that keyless bootstrap and tool-surface reads
could not diverge from session or project state.

The premise does not hold for the whiteboard knob:

- `handleWorkflowManual` (`internal/mcp/workflow_manual.go`) already resolves
  `workflow.lang` and `workflow.skeptical_posture` through
  `s.sessionResolver(key, nil)`, which reads the session, project, and repo
  scopes from the lead's session key. Only the `printPlaybook` call passes
  `wsconfig.Options{}`, and `applyWhiteboardVariants`
  (`internal/mcp/whiteboard_text.go`) reads the knob through that empty
  `configOpts` via `workflowWhiteboardEnabled`
  (`internal/mcp/whiteboard_tools.go`), which calls `resolver.Get("", ...)`.
- The knob gates only lead-facing text (the workflow manual and the
  `lead-discuss` and `lead-ticket` renders). A CONTINUE manual read carries
  the lead's key; a FRESH bootstrap read carries the bootstrap sentinel, and
  `handleWorkflowManual` renders the body before the FRESH branch mints a
  key, so a sentinel resolver has no repository root (the project scope is
  process-level and still resolves).
  `applyWhiteboardVariants` runs from `printPlaybook` for every render
  (`internal/mcp/playbook_tools.go`), so a render without a session key, if
  one reaches it, falls back to the global and builtin values, which is
  today's behavior; the keyless-versus-session divergence that motivated
  `workflow.prefer_subagent` (tool visibility differing between keyless and
  keyed reads) has no counterpart here.

## Decisions

- **Per-harness values (user decision 2026-11-05).** The knob's value can
  differ per harness, for example on for `claude` and off for `codex` and
  `pi`.
- **Session scope (user decision 2026-11-05).** The knob can be set for a
  single session.

- **One generic harness-bucket path ((134)).** The harness axis reuses the
  `prompt.<pointId>` harness-bucket mechanism, lifted out of its
  `prompt.`-prefix branches into one path keyed on the registry's
  `HarnessApplicable` flag, which `prompt.*` and
  `workflow.whiteboard_enabled` both use. A bucket is stored as
  `<key>.<claude|codex|pi|all>`, `*` is stored as `all`, and a read takes
  the session's harness bucket, then `all`, then the builtin. Today the
  bucket composition, the `*`-to-`all` mapping, the repo key, and the
  harness-then-`all` fallback exist only under
  `strings.HasPrefix(key, "prompt.")` in `internal/mcp/server.go`
  (`config.tune`), `internal/mcp/config_registry.go` (`RepoKey`), and
  `internal/mcp/playbook_tools.go` (`applyOverrideMarkers`; there the
  `prompt.` keys are composed literally as `"prompt."+pointId+"."+harness`
  in `buildOverrideLookup` rather than under a `HasPrefix` branch, and the
  harness-then-`all` fallback is in `applyOverrideMarkers`,
  `agents-plugin-tool/internal/mcp/playbook_tools.go#L688-L697`,
  `agents-plugin-tool/internal/mcp/playbook_tools.go#L781`); the layered
  resolver, the scope argument, and the harness defaulting and validation in
  `config.tune` are already generic. Rejected: a key-specific whiteboard
  branch beside the `prompt.` ones (duplicates the mechanism), and
  per-harness adapter settings such as `claude.whiteboard_enabled` (the
  manual mandate is host-neutral, and Codex and Pi have no adapter-setting
  namespace for it).
- **Ordinary layered scopes ((135), user decision 2026-11-05).** The knob
  resolves through session, project, repo, and global scopes; dropping
  `RegisterGlobalOnly` makes it repo-scoped through `RepoScoped` in
  `internal/wsconfig/scope.go`, so a project may commit it team-wide. A
  committed repo value overrides a member's global value in the same harness
  bucket ((142)); the member overrides it back at project or session scope. Rejected: session, project,
  and global only (needs a new per-key allowed-scope list).
- **Bucket-major precedence ((142), user decision 2026-11-05).** A read
  resolves the session's harness bucket through every stored scope
  (session, project, repo, global) first, then the `all` bucket through
  every stored scope, then the legacy bare key ((147)), then the builtin, exactly as `prompt.<pointId>` resolves today (`buildOverrideLookup`
  calls `resolver.Get` on the literal bucket key, and `applyOverrideMarkers`
  tries `all` only when that walk is empty). So a member's global `claude`
  value beats a repo-committed `all` value; a team that wants to override a
  harness bucket commits that bucket. Rejected: scope-major (at each scope
  try the harness bucket then `all`), which needs a new resolution path and
  either splits the rule from `prompt.*` or reverses the
  `260620-bug-ws-prompt-override-no-unset-path` resolution model.
- **Write defaults ((136)).** A write that names no scope still lands in
  global scope (a viewing habit is cross-project,
  `261030-feat-session-whiteboard-mode`); a write that names no harness
  lands in the current session's harness bucket, the `prompt.<pointId>`
  selector's rule.
- **Every reader uses the session resolver ((137)).** The workflow manual
  gate and the `lead-discuss` and `lead-ticket` knob-on variants
  (`internal/mcp/whiteboard_text.go`, `workflowWhiteboardEnabled` in
  `internal/mcp/whiteboard_tools.go`) resolve the knob through the lead's
  session resolver with the session's harness, so the manual and the
  playbooks agree.
- **Builtin default unchanged ((138)).** The builtin stays `off` in ws and
  wsflow, held only on the bare key in `builtinConfigDefaults`, never seeded
  per bucket key (a bucket builtin would end the harness-bucket walk and the
  `all` bucket would never apply); the user wanted a personal setting, not a shipped default
  (2026-11-05).
- **Legacy bare key reads as `all` ((139), (147)).** A value already stored
  under the bare `workflow.whiteboard_enabled` key is read in the `all`
  position, so an existing global `on` keeps working with no upgrade step.
  It is read at any scope it is found, after the `.all` key of the same
  position, so the keys tried are the harness bucket, `.all`, the bare key,
  then the builtin, each through every scope ((142)). Rejected: requiring a
  migration, or dropping the stored value; reading the bare key at global
  scope only (two rules, and no write path creates a new bare value).
- **FRESH bootstrap render reads the canonical root ((143)).** The FRESH
  bootstrap manual render resolves the knob with the canonical root, as
  `worktreeProfilesForManual(wsconfig.Options{RepoRoot: canonical})`
  already does on that path (`internal/mcp/workflow_manual.go`), by
  threading a root-anchored resolver into `printPlaybook` or rendering after
  canonicalization. FRESH without a root falls back to project, global, and
  builtin. Rejected: keeping the empty-options render (a repo-committed `on`
  would never show in the bootstrap manual while the keyed `lead-discuss`
  and `lead-ticket` renders apply it, breaking (137)).
- **Bucket path condition ((144)).** The generic bucket path applies to
  entries with `HarnessApplicable && ResolverBacked`. Rejected:
  `HarnessApplicable` alone (`agents.tier` is harness-applicable but
  `ResolverBacked: false` with structured values, so it must not gain bucket
  repo keys or bucket composition).
- **Bucket keys default to global scope ((145)).** The four
  `workflow.whiteboard_enabled.<bucket>` keys register `ScopeGlobal` through
  `RegisterDefaultScope`, keeping the `RegisteredItemKeys` namespace check in
  `config_manifest.go` passing; an unregistered key's `wsconfig.DefaultScope`
  is `ScopeProject`, which would break (136). The bare
  `workflow.whiteboard_enabled` key keeps
  `RegisterDefaultScope(..., ScopeGlobal)`, which `RegisterGlobalOnly`
  registers today (`internal/wsconfig/scope.go`), or the generic bucket path
  reports its scope from the bucket key as the `prompt.` path does, so the
  `config.tune` write and reset echoes (`resetEchoScope` in
  `internal/mcp/server.go`) never label a scope-less write `project`.
- **Lead authority kept ((146)).** The tuning entry sets
  `RequiresLeadAuthority: true` explicitly once `RegisterGlobalOnly` is
  dropped (today it derives from `wsconfig.GlobalOnly`,
  `internal/mcp/config_registry.go`), so a write still needs a lead
  `session_key` at every scope, as `261030-feat-session-whiteboard-mode`
  made it a lead-only knob.

## Constraints

- Manuals: `ai-docs/manuals/ws-mcp.md` (config key and render path),
  `ai-docs/manuals/shipped-surface-boundary.md` (the tuning catalog text
  ships), `ai-docs/manuals/skill-authoring.md` if any rsrc text changes,
  `ai-docs/manuals/wsflow-mirroring.md` for mirrored rsrc.
- `261105-feat-lead-ticket-review-auto-ack` (132) cites this ticket for the
  global-only premise not holding.
- `261105-feat-lead-ticket-review-auto-ack` (ready) also moves a knob read
  under `applyWhiteboardVariants`/`printPlaybook` from the
  `wsconfig.Options{}` helper to `s.sessionResolver`; whichever ticket lands
  second reuses the session-key threading the first adds rather than adding
  a second parameter.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 261030-feat-session-whiteboard-mode (2026-11-04, Decisions): "Declared as a built-in `workflow.*` key (technical decision (101)), like `workflow.prefer_subagent` ... registered global-only (`RegisterGlobalOnly`) ... Global-only because `handleWorkflowManual` renders the manual with `wsconfig.Options{}`" — bearing: superseded by (135) (its premise fails; see Background)
- f98d8e83 (2026-11-04, commit): "Knob-on text is applied code-side in printPlaybook (the prefer_subagent precedent) with replace-or-append fallback, so rsrc mirrors stay byte-identical" — bearing: constrains
- 261105-feat-lead-ticket-review-auto-ack (2026-11-05, Decisions): "Ordinary layered scopes ((132)) ... Rejected: a global-only key like `workflow.whiteboard_enabled`; that knob's global-only premise (the manual renders without a repository root) does not hold" — bearing: supports
- 260620-bug-ws-prompt-override-no-unset-path (2026-06-30, 71d082eb): "Conservative blast radius: omitting harness targets only the current host bucket, never wildcards all buckets." — bearing: supports
- 260620-bug-ws-prompt-override-no-unset-path (2026-06-20, Resolution model): "host-specific bucket (`claude`/`codex`) > `*` (all-hosts) bucket > shipped seed" — bearing: supports
- 260814-refactor-config-collapse-tuning-knobs-to-list-tune (2026-08-14, Decisions): "`harness` is load-bearing for the keys that use it; a warning only where it genuinely does not apply. Two key families are harness-scoped today ... prompt-override keys ... and `agents.tier`" — bearing: constrains
- 260905-feat-ws-pi-harness-config-layer (2026-09-06, Result 0bd5d65e): "`config.tune` lowercases the harness before the enum check, normalizes an empty harness to `default` only when the key's enum contains `default` (so `prompt.*` still rejects an unresolved empty harness)" — bearing: constrains
- 260921-feat-config-tune-agents-tier-global-scope (2026-09-21, 81780547): "Kept the chosen builtin < global < project overlay at per-tier/harness leaf granularity rather than replacing whole tier maps" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsconfig/scope.go, internal/mcp/server.go, config_registry.go, playbook_tools.go, workflow_manual.go, whiteboard_text.go, whiteboard_tools.go |
| scope.surface | public-interface | config.tune and config.list caller-visible behavior for workflow.whiteboard_enabled gains harness and scope selectors |
| scope.new_public_symbol | no | reuses existing HarnessApplicable field and config.tune harness/scope arguments |
| scope.new_type_contract | no | stored key shape <key>.<claude/codex/pi/all> mirrors the existing prompt.* bucket contract |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/whiteboard_tools_test.go, manuals_workflow_manual_test.go, prompt_override_test.go |
| complexity.reuse_points | confirmed | prompt.* bucket path in buildOverrideLookup and applyOverrideMarkers, config_registry.go RepoKey, HarnessApplicable at config_registry.go#L55-L56 |
| complexity.side_effect_risk | moderate | lifting prompt.-prefix branches into a generic path touches prompt override resolution and repo-key composition shared by all prompt.* knobs |
| risk.correctness | moderate | legacy bare-key-as-all fallback and harness-then-all precedence across layered scopes must agree between manual and playbook renders |
| risk.fit | moderate | generalizes a prompt-only mechanism across server.go, config_registry.go, and playbook_tools.go and reverses a recorded global-only decision |
| risk.test | moderate | verification needs per-harness and per-session render assertions plus a legacy stored-value case |
| risk.security_or_contract | moderate | changes the config.tune and config.list contract and committed repo-scope key shape for a shipped knob |

## Phases

### Phase 1: Harness- and session-scoped whiteboard knob

Make `workflow.whiteboard_enabled` resolve per harness and through the
layered scopes, update its `config.tune` writer and `config.list` catalog
entry (harness and scope selectors, description), and route every reader
through the lead's session resolver with the session's harness.

Verification:

- a global `claude` value of `on` renders the whiteboard section in a Claude
  session's workflow manual and not in a Codex session's;
- a session-scope `on` renders the section for that session only, checked
  through a CONTINUE (keyed) manual read, since no session value exists
  before the FRESH mint;
- a repo-scope `on` renders the section in a FRESH-with-root bootstrap
  manual;
- a value in one scope's harness bucket competing with another scope's
  `all` bucket resolves by the precedence decided in `## Decisions`;
- removing a harness-bucket value, and a session-scope value, through the
  `config.tune` reset path falls back to the next value in order, and the
  `prompt.<pointId>` reset regression tests still pass;
- `agents.tier` gains no bucket repo key and its `config.tune` writes are
  unchanged;
- a write without a lead `session_key` is refused at every scope;
- a `config.tune` write naming no scope lands in global scope and one naming
  no harness lands in the session's harness bucket, and the write and reset
  echoes report `global`;
- a repo-scope `.all` value of `off` beats a global bare-key `on`
  ((147) order);
- the `all` bucket applies when the harness bucket is unset;
- `lead-discuss` and `lead-ticket` knob-on variants agree with the manual
  for the same session;
- `config.list` shows the harness and scope selectors with the builtin
  `off`;
- a value stored under the bare `workflow.whiteboard_enabled` key before
  this change resolves as the `all` bucket.

### Result (9ca6f0039) - 2026-11-05

Landed: `workflow.whiteboard_enabled` is harness-bucketed and resolves
through session, project, repo, and global scopes. A generic bucket path
(`configKeyEntry.HarnessBucketed` = `HarnessApplicable && ResolverBacked`,
plus `harnessBucketKey`, `harnessBucketReadOrder`,
`resolveHarnessBucketed`, and a generic `RepoKey`, all in
`internal/mcp/config_registry.go`) replaces the `prompt.`-only branches
in `config.tune`, `RepoKey`, `buildOverrideLookup`, and
`applyOverrideMarkers`. Reads go bucket-major: the session's harness
bucket, then `.all`, then the legacy bare key, each through every scope,
then the builtin `off` on the bare key. `RegisterGlobalOnly` is dropped.
The bare key and the four bucket keys register `ScopeGlobal`, and
`RequiresLeadAuthority` is set explicitly. `config.list` lists harness
and scope selectors, the bucket repo key, and the effective value for the
session's harness. `config.get` reads the bucket that applies.

`applyWhiteboardVariants` moved out of `printPlaybook` into its two
callers, the `playbook.read` handler and `handleWorkflowManual`. Each
resolves the knob with its own resolver, following the sibling
auto-ack's pattern, and `printPlaybook` gains no new parameter.
FRESH-with-root now canonicalizes before rendering and reads the knob
with `Options{RepoRoot: canonical}`. The `lead-tune` scalar handler now
passes a harness the user names. That change is mirrored to wsflow and
Pi, and the manifest is regenerated.

Verification:
- `go test ./... -count=1` in `agents-plugin-tool`: all packages ok.
- The wsflow package unittest: 20 tests OK.
- New tests in `internal/mcp/whiteboard_tools_test.go` cover the ticket's
  verification list: per harness; session scope through a CONTINUE read;
  repo scope in a FRESH-with-root manual; bucket-major precedence; reset
  fallback for a bucket and for a session value; `agents.tier` taking no
  bucket path; lead key required at every scope; write and reset echo
  defaults (global, the session's bucket); repo `.all` off beating a
  global bare on; the `all` fallback; manual and playbook variants
  agreeing; the catalog selectors with builtin `off`; and a legacy bare
  value read as `all`. The existing `prompt.*` reset regression tests
  pass.

Decisions:
- `config.get` was made bucket-aware, which is beyond the ticket's list
  of readers; without it, `config.get` would report only the legacy or
  builtin value. This also applies to `prompt.<pointId>`.
- A bucketed write echo names the bucket key (`...whiteboard_enabled.claude:
  on [scope:global]`). A bucketed reset echoes the cleared bucket and the
  effective value.
- An undetected harness makes a harness-less write fail the enum check,
  which is the `prompt.*` rule.
