---
title: Claude mod package parity with per-package knob defaults
related:
  261017-feat-claude-mod-lead-compaction-timing: supersedes its ws-only compaction gate
  261018-feat-claude-mod-compaction-threshold-knob: supersedes its D8 (wsflow stays inert, declares no manifest)
  261018-feat-claude-mod-ws-manual-injection: supersedes its ws-only manual injection gate
  261022-feat-claude-mod-tool-call-captions: supersedes its ws-only caption gate
  261028-feat-claude-mod-status-band: supersedes its ws-only band gate and changes the ws band default
  261029-chore-wsflow-no-agent-env-rename-and-setup-tool-removal: deferred follow-up
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
  its own `config-manifest.json`. Both declare the same key set with identical
  `type`, `pattern`, `description`, and `default_scope`; only `default` may
  differ. wsflow's `.mcp.json` and `.claude-plugin/plugin.json` gain
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
  with identical `type`, `pattern`, `description`, and `default_scope`, only
  `default` differing; key descriptions state no package's default. Rejected:
  hand review - manifest drift is the failure this design must make visible.
- **D10. Status band without manual injection draws its key-less rows only.**
  The band's lead key comes only from manual injection's `keyChanged`; with
  `claude.manual_injection_enabled` off, the band draws the model, folder,
  git, context, rate-limit, and cost rows and omits the key-bound mailbox,
  todo, and agenda rows. Rejected: sourcing a key from the `mailbox_watch`
  bind - a second key source is new structure.

## Constraints

- Out of scope: the Codex Stop hook, the wsflow-only skill shims
  (`lead-check-blockers`, `lead-workflow-manual`, `lead-proceed`), the
  `playbook.render` context bridge, and the Pi package.
- Deferred to `261029-chore-wsflow-no-agent-env-rename-and-setup-tool-removal`:
  renaming `WS_MCP_NO_AGENT` (after D5 it gates only the render bridge) and
  removing the `WS_MCP_SETUP_TOOL` env, which no Go source reads.
- With default config, every wsflow behavior is unchanged except the new
  revive nudge in projects with `ai-docs/WORKFLOW.md` and the CLI `config
  tune`; every ws behavior is unchanged except the status band now drawing by
  default and the revive nudge no longer firing outside such projects.
- Installing ws and wsflow together stays a user error (261016 decision); no
  double-binding guard is added.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (all three packages),
  `ai-docs/manuals/wsflow-mirroring.md` (`agents-plugin/hooks/`,
  `agents-plugin-wsflow/`), `ai-docs/manuals/ws-mcp.md`
  (`agents-plugin-tool/internal/mcp/`). Resync the mod mirror with
  `rsync -a --delete agents-plugin/hooks/claude-mod/ agents-plugin-wsflow/hooks/claude-mod/`.

## Phases

### Phase 1: Knob-gated parity

Implement D1-D10 as one change; one phase, since no slice is useful alone
and the doc changes describe the landed behavior.

Verification:

- Mod engine tests no longer branch on the engine-reported package name for
  feature selection; each feature is tested on and off through its knob, the
  failed-read case (D8), and the band's key-less rows (D10).
- Go: the manifest drift guard (D9); existing `config_manifest_test.go`;
  CLI `config tune` succeeds under `WS_MCP_NO_AGENT=1`.
- Nudge: prints in a repository with `ai-docs/WORKFLOW.md` at its root, from a
  subdirectory, and in a non-repository directory under an ancestor holding
  it; prints nothing without it; text names the passed package; mirror guard
  green.
- `go test ./...` in `agents-plugin-tool`, `TestWsflowHooksMirrorUpToDate`,
  `python3 -m unittest discover agents-plugin/tests`,
  `python3 -m unittest discover agents-plugin-wsflow/tests`, and
  `claude plugin validate` plus `claude plugin test` on both packages.
