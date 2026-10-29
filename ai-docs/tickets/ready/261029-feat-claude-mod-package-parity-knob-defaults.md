---
title: Claude mod package parity with per-package knob defaults
related:
  261017-feat-claude-mod-lead-compaction-timing: supersedes its ws-only compaction gate
  261018-feat-claude-mod-compaction-threshold-knob: supersedes its D8 (wsflow stays inert, declares no manifest)
  261018-feat-claude-mod-ws-manual-injection: supersedes its ws-only manual injection gate
  261022-feat-claude-mod-tool-call-captions: supersedes its ws-only caption gate
  261028-feat-claude-mod-status-band: supersedes its ws-only band gate and changes the ws band default
  261017-feat-claude-mailbox-active-push-mod: D11 supersedes its "wsflow never raises a permission prompt" guarantee for knob reads
  261029-chore-wsflow-no-agent-env-rename-and-setup-tool-removal: deferred follow-up
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 4970cc0f267c9008
sage-review-completeness-reviewed: 4970cc0f267c9008
---

# Claude mod package parity with per-package knob defaults

## Background

`ws` and `wsflow` ship one byte-identical Claude mod
(`agents-plugin/hooks/claude-mod/`, mirrored into `agents-plugin-wsflow/`),
but every lead feature beyond the mailbox is switched on by package name:
`compactionEnabledFor` (`compaction.ts`, `pluginName === 'ws'`) gates lead
compaction timing, workflow manual injection, tool-call captions, and the
status band, and `drainModeFor` (`register.ts`, `'ws'` -> `direct`, else
`notice`) picks the mailbox drain mode. wsflow's `.mcp.json` and
`.claude-plugin/plugin.json` declare no `WS_MCP_CONFIG_MANIFESTS`, so its mod
reads no `claude.*` key and a wsflow user cannot turn any of these on. Outside
the mod, only `ws` registers the SessionStart(compact) revive nudge hook, and
the CLI `config tune` command is refused under `WS_MCP_NO_AGENT`
(`fatalIfNoAgentCommand` / `filterNoAgentCommands` in
`agents-plugin-tool/cmd/ws-mcp/main.go`).

The MCP surface is already nearly unified: the rsrc trees are byte-identical
with no `ws:full-only` / `ws:wsflow-only` markers, both packages expose the
same tool set, and `WS_MCP_NO_AGENT` otherwise gates only the
`playbook.render` context bridge. The "agentless derivative" identity no
longer matches what wsflow ships (effort agents, a worker-spawning
`lead-run`).

Goal (user decision, 2026-10-29): feature parity between the two packages on
the Claude surface, where the only difference is each knob's per-package
default, and wsflow is redefined as a conservative-default distribution of the
same product.

## Decisions

- **D1. Every package-name gate in the mod becomes a config key.** Remove
  `compactionEnabledFor` and `drainModeFor` and every call site that branches
  on `$.plugin.name` for feature selection; the mod reads keys only and stays
  byte-identical between packages. Rejected: keeping a package-name table of
  defaults in mod code - it reintroduces the package branch the knobs replace.
- **D2. Knobs and per-package defaults.** Each default is that package's
  current behavior, except the ws status band (user change):

  | key | type | ws default | wsflow default |
  |---|---|---|---|
  | `claude.compaction_enabled` | boolean | `true` | `false` |
  | `claude.manual_injection_enabled` | boolean | `true` | `false` |
  | `claude.mailbox_drain` | `direct` \| `notice` | `direct` | `notice` |
  | `claude.display_summary_enabled` | boolean (existing) | `false` | `false` |
  | `claude.status_band_enabled` | boolean (existing) | `true` (was `false`) | `false` |

  Names follow the existing `claude.*_enabled` keys; the drain values reuse the
  mod's `DrainMode` names. Manual injection keeps its existing
  `ai-docs/WORKFLOW.md` project gate in addition to the knob.
- **D3. One manifest per package; only `default` differs.** Each package ships
  its own `config-manifest.json`. Both declare the same key set, and only `default`
  may differ (D9). wsflow's `.mcp.json` and `.claude-plugin/plugin.json` gain
  `WS_MCP_CONFIG_MANIFESTS` pointing at its manifest, as ws's do. A drift
  guard test pins that shape.
- **D4. Revive nudge ships in both packages, gated on `ai-docs/WORKFLOW.md`.**
  `compact-revive-nudge.py` is registered under SessionStart(`compact`) in both
  packages' `hooks/hooks.json` and fires only when `ai-docs/WORKFLOW.md` is
  found at the session's repository root, falling back to the nearest ancestor
  of the working directory outside a repository - the same detection manual
  injection uses (`register.ts`, the `$.session.repo()` /
  `$.fs.ancestors({ names: ['ai-docs/WORKFLOW.md'] })` gate). It has no knob:
  revive applies wherever a lead holds a session key, including a wsflow lead
  with manual injection off. The package name is passed as a hook-command
  argument so the nudge text names the package's own plugin and
  `<package>:lead-revive`; the script is byte-identically mirrored into
  `agents-plugin-wsflow/hooks/` under a drift guard like
  `TestWsflowHooksMirrorUpToDate`. Rejected: cwd-only detection - a session
  opened in a subdirectory would miss the nudge; tying it to the manual
  knob - see above.
- **D5. CLI `config tune` works in wsflow.** Remove `fatalIfNoAgentCommand` and
  `filterNoAgentCommands` and add `config.tune` to
  `agents-plugin-wsflow/runtime.json` `commands`. The MCP `config.tune` tool is
  already exposed in wsflow; only the CLI path was refused.
- **D6. wsflow is a conservative-default distribution of the same product.**
  Internal docs say so: `ai-docs/manuals/wsflow-mirroring.md` (Purpose,
  Claude Mod Mirror, Doctrine) and `AGENTS.md` (`## Project Scope` package
  list, `## Project Orientation`) drop "agentless derivative" and "not a
  user-facing ws variant". Distributed wsflow text (both wsflow manifest
  descriptions, wsflow `hooks/hooks.json`) describes conservative defaults and
  never names ws, per the mirroring manual's non-ws-aware rule. ws's
  `hooks/hooks.json` description and the mirroring manual's Claude Mod Mirror
  section are rewritten from "ws only" features to knob-gated features, and
  `261018-feat-claude-mod-compaction-threshold-knob` D8 (wsflow stays inert) is
  recorded there as superseded.
- **D7. New knobs are read keyless once at session start.**
  `claude.compaction_enabled`, `claude.manual_injection_enabled`, and
  `claude.mailbox_drain` are read from the project, global, and built-in
  scopes only, like `claude.compaction_advisory_percent`; a session- or
  repo-scope tune is not read. Rejected: live or session-keyed reads - manual
  injection mints the lead key at session start, so no key exists when the
  knob is read, and toggling key minting or the mailbox binding mid-session is
  unsafe. The two existing keys keep their current read semantics.
- **D8. A failed knob read selects the inert state.** If the `config_get` for a
  new knob fails, the feature is off (`notice` for the drain), matching the
  band's existing "a rejection reads as off". Rejected: falling back to the
  package default - the mod would need per-package code defaults (D1).
- **D9. The manifest drift guard is a Go test.** It sits beside
  `config_manifest_test.go` and asserts both manifests declare the same keys
  with every field of each key identical except `default` (including
  `minimum` and any field added later); key descriptions state no package's
  default. Rejected: hand review - manifest drift is the failure this design
  must make visible; an enumerated field list - a field added later would
  drift silently.
- **D10. Status band without manual injection draws its key-less rows only.**
  The band's lead key comes only from manual injection's `keyChanged`; with
  `claude.manual_injection_enabled` off, the band draws the model, folder,
  git, context, rate-limit, and cost rows and omits the key-bound mailbox,
  todo, and agenda rows. Rejected: sourcing a key from the `mailbox_watch`
  bind - a second key source is new structure.
- **D11. Knob reads stay on `$.mcp.call` config_get; a permission dialog is
  accepted in both packages.** Every mod knob read, new and existing, goes
  through the MCP `config_get` as the current reads do. In a prompting
  permission mode this can raise a dialog at session start in wsflow too; a
  declined or denied read counts as failed (D8). This supersedes the
  `261017-feat-claude-mailbox-active-push-mod` guarantee that wsflow never
  raises a permission prompt, for knob reads (user decision, 2026-10-29: auto
  mode is the usual setting). Rejected: reading through the plugin CLI via
  `$.process` - per the user, it is permission-gated the same way under the
  usual setup, so it buys nothing for its cost; new knobs only on a separate
  path - two read paths for no gain.
- **D12. Knob-gated registrations wait for their read, not session.start.**
  The compaction lever, the mailbox bind tool, and the mailbox watcher are
  created when their knob read settles, outside session.start's awaited
  registration path, so a slow server start never delays the first prompt;
  the features come up once the read settles. Phase 1 first probes that a tool
  registered after session.start is listed to the model by its next turn; if
  it is not, the worker stops and returns before choosing another shape.
  Rejected: a bounded await inside session.start - a timeout reads as failed
  (D8) and would switch ws compaction and direct drain off for the whole
  session, breaking the default-config Constraint.
- **D13. The mailbox's "manual injection owns the key" gate includes the
  knob.** The gate that suppresses `mailbox_watch` and auto-binds the mailbox
  (today `wsGate`, the WORKFLOW.md check) becomes
  `claude.manual_injection_enabled` AND `ai-docs/WORKFLOW.md`; with manual
  injection off, `mailbox_watch` registers even in a WORKFLOW.md project.
  Otherwise a wsflow session in such a project would mint no key and register
  no bind tool, leaving mailbox push dead.
- **D14. Every multi-feature hook gates each feature on its own knob.** At
  `session.compact`, `session.measure`, `turn.step`, the lever-landed path,
  `tool.call`, `ui.render`, and `session.end`, each feature's branch checks its
  own knob. Rejected: one gate per hook - it would switch features on and off
  together, which the per-feature knobs exist to prevent.
- **D15. The nudge finds the repository root with git.** It runs
  `git rev-parse --show-toplevel` from the SessionStart payload's cwd and walks
  ancestors outside a repository, mirroring the mod's `$.session.repo()` root
  plus ancestor fallback. Rejected: cwd only (D4).

## Constraints

- Out of scope: the Codex Stop hook, the wsflow-only skill shims
  (`lead-check-blockers`, `lead-workflow-manual`, `lead-proceed`), the
  `playbook.render` context bridge, and the Pi package.
- Deferred to `261029-chore-wsflow-no-agent-env-rename-and-setup-tool-removal`:
  renaming `WS_MCP_NO_AGENT` (after D5 it gates only the render bridge) and
  removing the `WS_MCP_SETUP_TOOL` env, which no Go source reads.
- With default config, every wsflow behavior is unchanged except the new
  revive nudge in projects with `ai-docs/WORKFLOW.md`, the CLI `config
  tune`, and a possible session-start permission dialog for knob reads (D11); every ws behavior is unchanged except the status band now drawing by
  default and the revive nudge no longer firing outside such projects.
- Installing ws and wsflow together stays a user error (261016 decision); no
  double-binding guard is added.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (all three packages),
  `ai-docs/manuals/wsflow-mirroring.md` (`agents-plugin/hooks/`,
  `agents-plugin-wsflow/`), `ai-docs/manuals/ws-mcp.md`
  (`agents-plugin-tool/internal/mcp/`). Resync the mod mirror with
  `rsync -a --delete agents-plugin/hooks/claude-mod/ agents-plugin-wsflow/hooks/claude-mod/`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 0acdd4a5 / 261017-feat-claude-mailbox-active-push-mod (2026-10-17, commit): "Round 1 design block: the knob read through $.mcp.call config_get is itself permission-gated ... User chose to drop the knob and split by package character: ws opinionated (always drain...), wsflow conservative (notice only, never prompts)." — bearing: contradiction-candidate
- 261018-feat-claude-mod-compaction-threshold-knob (2026-10-18, Decisions): "D8. wsflow stays inert. The mod edit is mirrored byte-identically; wsflow's .mcp.json declares no manifest and its mod reads no key, since compaction timing does not run there." — bearing: constrains
- 261028-feat-claude-mod-status-band (2026-10-28, Decisions): "The band runs only in the ws package (compactionEnabledFor) ... wsflow draws nothing and ships no config manifest." / "Reads fail closed: a failed or denied knob read counts as off" — bearing: constrains
- 42ab0938 (2026-10-18, commit): "engine-backed compaction tests branch on the engine-reported package name so the mirrored file runs ws behavior in ws and inert behavior in wsflow" — bearing: constrains
- 99f22d9d (2026-10-17, commit): "a $.state ref must spell its owning plugin as a literal, which a source shared byte-identically by ws and wsflow cannot (validate refused the dual-literal form)." — bearing: constrains
- 6499533d / 260513-hbsflow-agentless-runtime-mode (2026-05-13, commit): "Kept the default full ws contract unchanged when new env vars are unset, while WS_MCP_NO_AGENT filters agent-backed MCP tools and CLI commands together." — bearing: supports
- 4fe73074 / 260730-bug-wsflow-tier-config-access-missing (2026-07-30, commit): "Model tier was grouped with the named-agent surfaces that WS_MCP_NO_AGENT=1 hides, but it is not a named-agent concept ... left wsflow users without an access point" — bearing: supports
- 261016-research-claude-mod-mailbox-active-push (2026-10-16, Confirmed Decisions): "Installing both ws and wsflow in one Claude Code is a user error and out of scope; there is no double-binding guard." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/register.ts, compaction.ts, band.tsx and engine tests, agents-plugin/hooks/hooks.json, compact-revive-nudge.py, agents-plugin/config-manifest.json, agents-plugin-wsflow/.mcp.json, .claude-plugin/plugin.json, runtime.json, hooks/, new config-manifest.json, agents-plugin-tool/cmd/ws-mcp/main.go, main_test.go, internal/mcp drift test, ai-docs/manuals/wsflow-mirroring.md, AGENTS.md |
| scope.surface | cross-module | Claude mod TS, Go CLI, package manifests, hooks.json, docs; removes exported compactionEnabledFor and drainModeFor |
| scope.new_public_symbol | yes | config keys claude.compaction_enabled, claude.manual_injection_enabled, claude.mailbox_drain; wsflow config-manifest.json; wsflow CLI config.tune |
| scope.new_type_contract | yes | claude.mailbox_drain direct or notice enum; new manifest key declarations; compact-revive-nudge.py package argument |
| scope.test_surface | new-files | new manifest drift guard beside agents-plugin-tool/internal/mcp/config_manifest_test.go; existing engine tests, agents-plugin-tool/cmd/ws-mcp/main_test.go#L244 config tune refusal case, wsflow_hooks_mirror_test.go, agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py, test_wsflow_claude_hooks_manifest.py |
| complexity.reuse_points | confirmed | keyless config_get read register.ts#L572-L576, WORKFLOW.md gate register.ts#L377-L386, TestWsflowHooksMirrorUpToDate in internal/wsrsrc/wsflow_hooks_mirror_test.go |
| complexity.side_effect_risk | high | changes ws defaults band on and nudge scope and makes wsflow read config at session start, about 30 compactionEnabledFor call sites in register.ts |
| risk.correctness | high | about 30 package-gate call sites split across three knobs with keyless-once, live band, and fail-inert read semantics |
| risk.fit | moderate | reintroduces a drain knob that 0acdd4a5 rejected over config_get permission gating; D11 accepts the dialog by user decision |
| risk.test | moderate | engine tests currently branch on package name and must be rewritten per knob; permission-prompt behavior is live-only |
| risk.security_or_contract | moderate | wsflow runtime.json commands and CLI no-agent contract change; new public config keys and per-package manifests |

## Phases

### Phase 1: Knob-gated parity

Implement D1-D15 as one change; one phase, since no slice is useful alone
and the doc changes describe the landed behavior.

Verification:

- Mod engine tests no longer branch on the engine-reported package name for
  feature selection; each feature is tested on and off through its knob, the
  failed-read case (D8), and the band's key-less rows (D10).
- Probe first (D12): a tool registered after session.start is listed to the
  model by its next turn.
- A slow server at session start never delays the first prompt and brings
  knob-gated features up once the read settles; an unresolved server leaves
  them inert (D8, D12), which for the mailbox means `mailbox_watch` registered
  and the watcher running in `notice` mode, not an absent watcher.
- Manual injection off with `ai-docs/WORKFLOW.md` present registers
  `mailbox_watch` (D13).
- A session-scope tune of a new knob is ignored (D7).
- Go: the manifest drift guard (D9); existing `config_manifest_test.go`;
  CLI `config tune` succeeds under `WS_MCP_NO_AGENT=1`, with the existing
  wsflow refusal case in `cmd/ws-mcp/main_test.go` inverted (D5).
- wsflow package tests (`test_wsflow_runtime_contract.py`,
  `test_wsflow_claude_hooks_manifest.py`) cover the `config.tune` command, the
  manifest env, and the SessionStart(`compact`) hook (D3, D4, D5).
- Docs (D6): no "agentless derivative" or "not a user-facing ws variant" left
  in the named `wsflow-mirroring.md` and `AGENTS.md` sections, and no ws
  mention in wsflow's manifest descriptions or `hooks/hooks.json`.
- Nudge: prints in a repository with `ai-docs/WORKFLOW.md` at its root, from a
  subdirectory, and in a non-repository directory under an ancestor holding
  it; prints nothing without it; text names the passed package; mirror guard
  green.
- `go test ./...` in `agents-plugin-tool`, `TestWsflowHooksMirrorUpToDate`,
  `python3 -m unittest discover agents-plugin/tests`,
  `python3 -m unittest discover agents-plugin-wsflow/tests`, and
  `claude plugin validate` plus `claude plugin test` on both packages.
