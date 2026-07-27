<!-- Memory policy: prune aggressively as project advances. Completed
     work belongs in git history, not here. Keep only what an AI session
     needs to orient itself and pick up work. If it's derivable from
     code or git log, delete it from this file. -->

# devenv - Project Index

## Repo

Meta-workflow repository for workflow documents, skills, agents, plugin
packaging, helper commands, MCP tooling, and dev-environment templates. Specs,
tickets, and mental models here describe the workflow system itself; downstream
application material belongs in downstream projects.

Active plugin package: `agents-plugin/` (`ws@0.34.0`).
Agentless derivative package: `agents-plugin-wsflow/` (`wsflow@0.34.0`).
Native MCP/tooling source: `agents-plugin-tool/`.
Dashboard scaffold: `ws-dashboard/` (Rust workspace with core, harness-core,
harness-cli, bind-guarded daemon shell, resource API fixtures, and a React/Vite
inspectable frontend shell).
Retired Claude source material: `ai-docs/ref/claude-home-legacy.md` and git
history.

## Current Branch Rules

- Verify the current branch with `git status`; do not rely on `_index.md` for a
  live branch name.
- No branch-specific spec or mental-model freeze is active.
- Keep `.codex` untracked unless the user explicitly asks to stage it.

## Plugin Topology

- `./install.sh update` handles first-time install and settings patching on a
  new machine.
- Root `CLAUDE.md` is the only live Claude compatibility shim and points at
  `AGENTS.md`.
- `install.sh` snapshots only `agents-plugin/` for Claude-compatible plugin
  installs when Claude Code is available; it intentionally does not install
  wsflow into Claude.
- `agents-plugin/` is registered through `.agents/plugins/marketplace.json`;
  Codex UI install has verified `ws:lead-skill-authoring`,
  `ws:lead-write-ticket`, and `ws:lead-discuss`.
- `agents-plugin-wsflow/` is an agentless derivative package with
  Codex/Claude manifests, package-local no-agent MCP env, shared launcher
  copies, a reduced `runtime.json`, thin wsflow skill shims over shared
  playbooks, and package tests for runtime-contract plus skill-shim drift.
- `.agents/plugins/marketplace.json` exposes both `ws` and `wsflow` as local
  Codex plugin entries; `.claude-plugin/marketplace.json` exposes both packages
  for manual Claude marketplace installation while `install.sh` still installs
  only `ws`.
- Codex local plugin iteration has no known CLI refresh path; use UI
  uninstall/install or a fresh Codex session after editing the registered source.
- `agents-plugin/.codex-plugin/plugin.json` references plugin-local `.mcp.json`
  through `"mcpServers": "./.mcp.json"`.
- Changed plugin-managed Codex MCP config requires user-performed plugin cache
  refresh before installed-cache verification.
- `claude plugin validate agents-plugin` passes; runtime Claude invocation of
  `agents-plugin` remains compatibility behavior, not a separate source tree.
- `ai-docs/.old/` is the Git-tracked project archive for inactive reference
  material that should not appear in default file listings.

## Read Before Editing

| File | Use |
|------|-----|
| `agents-plugin/skills/lead-skill-authoring/SKILL.md` | Skill/agent/prompt/convention authoring rules |
| `ai-docs/ref/wsflow-mirroring.md` | Required before editing full ws skills or plugin surfaces that may need wsflow mirrors |
| `ai-docs/ref/codex-integration.md` | Probed Codex CLI behavior |
| `ai-docs/ref/ws-mcp.md` | MCP operational runbook, launcher environment, release and verification steps |
| `ai-docs/ref/windows-dogfood.md` | Native-Windows source-build dogfood / Phase C cold-load acceptance procedure |
| `ai-docs/ref/ws-agent-runtime.md` | Durable agent runtime contract |
| `ai-docs/ref/dashboard-headless-browser-verification.md` | Headless-Playwright dashboard verification: no-auth-daemon ad hoc live-UI probing, `npm run test:browser` cold-start, driving a real (non-stub) Codex/Claude harness, and verifying past the hidden-transcript landmine |
| `ai-docs/ship/ws.md` | Release process for `ws` |
| `ws/infra.read("impl-playbook")` | Implementation discipline |
| `ws/infra.read("subagent-rules")` | Subagent dispatch rules |
| `ws/infra.read("executor-wrapup")` | Shared post-implementation wrapup |

Before editing tickets/specs/mental models, read the matching convention through
`ws/convention.read`. Before editing skill, agent, prompt, or convention text,
read `agents-plugin/skills/lead-skill-authoring/SKILL.md`. Before editing full
`agents-plugin/skills/lead-*` skills, plugin packaging, runtime contracts,
launcher behavior, prompt guidance, or release validation that may affect
wsflow, read `ai-docs/ref/wsflow-mirroring.md` and run
`python3 -m unittest discover agents-plugin-wsflow/tests` when the derivative
surface may drift.

## Runtime Surfaces

MCP behavior contracts live in `ai-docs/spec/mcp-tools.md`,
`ai-docs/spec/plugin-runtime.md`, and related mental models. Current tool
schemas and inventory are runtime-discoverable through `tools/list` and
`runtime capabilities`; do not copy them into project memory or reference docs.
Shared `agents-plugin` skill text uses MCP names, not repo-local paths. Infra
and convention text are bundled into the runtime and read through
`ws/infra.read` and `ws/convention.read`.

## MCP Runtime Notes

Runtime and launcher contracts are maintained in `ai-docs/spec/plugin-runtime.md`,
`ai-docs/spec/mcp-tools.md`, and the source under `agents-plugin-tool/` and
`agents-plugin/bin/`. `ai-docs/ref/ws-mcp.md` is the operational runbook for
launcher environment, release, and verification steps.

Windows plugin-managed startup uses the same Python launcher. Native Windows
needs a working `python3` command; if the Windows Store alias is present without
Python installed, install Python 3 and refresh/reinstall the plugin before
rechecking `codex mcp list`.

## Prompt And Agent Inventory

Active workflows use the embedded runtime prompt bundle. For prompt inventory
and hash behavior, read `ai-docs/mental-model/prompt-bundle.md`,
`agents-plugin/runtime.json`, and the embedded prompt sources.

## Skill Inventory

Codex-first skills live under `agents-plugin/skills/` and use `lead-*` names.
Use the source tree and plugin manifest/tests for the current inventory.

## Canonical Flows

```text
Full ceremony:  discuss -> proceed -> implement -> review/docs/final gate
Direct:         implement <description>
Auto-route:     proceed <ticket-path>
Sprint:         sprint -> discuss/explore -> sprint-edit episode? -> episode closure or normal handoff
Review:         review [branch] -> verdict -> (discuss -> fix | comment | merge)
Recovery:       salvage -> research report -> recovery epic? -> child tickets
```

User decides next step at each handoff. `proceed` is the explicit opt-in for
auto-chaining through the pipeline.

## Specs

| File | Title | Summary |
|------|-------|---------|
| `ai-docs/spec/plugin-runtime.md` | Plugin Runtime | Codex plugin packaging, runtime metadata, launcher repair, release assets, and runtime CLI |
| `ai-docs/spec/mcp-tools.md` | MCP Tools | Host-neutral ws MCP tool contracts for context, workflow state, Git, docs, and agents |
| `ai-docs/spec/named-agent-runtime.md` | Named Agent Runtime | Durable named-agent sessions, async lifecycle, subquery fan-out, diagnostics, and Codex backend behavior |
| `ai-docs/spec/workflow-skills.md` | Workflow Skills | Codex-facing lead skills, routing, sprint work, reconstruction, utilities, and workflow primitives |
| `ai-docs/spec/documentation-system.md` | Documentation System | Project memory, conventions, specs, tickets, mental models, reference tracing, and doc workflows |
| `ai-docs/spec/api-documentation-cache.md` | API Documentation Cache | Host-neutral API documentation lookup through cached domain docs and manager sessions |
| `ai-docs/spec/claude-compatibility.md` | Claude Compatibility | Root Claude shim, agents-plugin compatibility metadata, installer behavior, and retired legacy boundaries |
| `ai-docs/spec/developer-environment-tools.md` | Developer Environment Tools | Personal bootstrap, shell/terminal/editor config, tmux helpers, statusline, and Claude TUIs |
| `ai-docs/spec/ws-web-dashboard/index.md` | ws Web Dashboard | Personal ws-aware web dashboard daemon, browser UI, and host-control behavior |

## Tickets

Status directories: `ready/`, `todo/`, `idea/`, `.done/`, `.dropped/`.
Reference tickets by stem. This index lists active tickets only; completed or
dropped tickets live in hidden archive dirs and git history.

| Stem | Status | Summary |
|------|--------|---------|
| `260726-chore-dashboard-verify-notification-permission-tier-manually` | ready | Tier 2 notification: automate the reachable gate and fix the insecure-context permission guard left undischarged by Phase 8 |
| `260513-feat-runtime-binary-staging-copy` | todo | Stage runtime binaries under deterministic versioned paths |
| `260517-bug-ws-agent-empty-result-after-tool-use` | todo | Investigate ws named-agent empty final result after long Claude backend tool-use runs |
| `260523-bug-implement-merge-target-discovery` | todo | Investigate safer merge-target discovery for nested implement branches |
| `260524-bug-ws-agent-register-stale-dir-result-hang` | todo | Investigate ws agent stale registration reset failure, register/call ordering, and post-test missing result |
| `260524-chore-exec-surface-runtime-contract` | todo | Close runtime capabilities, manifests, CLI mirror policy, and wsflow contract for exec tools |
| `260524-epic-async-exec-job-surface` | todo | Coordinate async exec job tools, bounded output readers, and later model-backed output questions |
| `260524-feat-exec-output-ask` | todo | Add lead-facing model-backed questions over persisted exec job output |
| `260525-bug-implement-review-fix-owner` | todo | Clarify lead-implement review fixes so the implementation owner applies findings |
| `260525-feat-ws-dashboard-document-polishing-backlog` | todo | Track non-critical document viewer/editor polish after the MVP document substrate |
| `260525-feat-ws-dashboard-workroot-polishing-backlog` | todo | Track non-critical WorkRoot lifecycle and Git toolbar polish after the MVP management substrate |
| `260605-epic-ws-playbook-factory-pivot` | todo | Playbook-factory board coordinating the spawn-removal / native-subagent pivot; M0-M4 done, board kept open for residual fill items (see git log for closed-milestone detail) |
| `260611-bug-agent-context-exhaustion-opaque-failure` | todo | ws named agent fails opaquely on context exhaustion |
| `260611-bug-rsrc-manifest-regen-missed-after-shipped-edit` | todo | rsrc manifest regen missed after a shipped-file edit slips past verification |
| `260612-bug-ws-rsrc-dev-server-new-file-staleness` | todo | Dev MCP server reports newly added rsrc files as "manifest-listed file missing" until restart |
| `260616-epic-api-namespace-documentation-memory-tooling` | todo | Rebuild api.* later as pure documentation, corpus, hierarchical memory, and playbook-manual tooling |
| `260620-bug-mercenary-path-visible-when-prefer-off` | todo | Reduce mercenary dispatch guidance salience when prefer_mercenary is off |
| `260620-bug-ws-delegate-playbook-output-language-unbound` | todo | Delegate playbooks do not bind subagent output language to English |
| `260622-research-ws-dashboard-ferrule-session-binding` | todo | Capture the dashboard ferrule/session-key binding model and migration impact |
| `260624-epic-pre-release-cleanup` | todo | Pre-release cleanup epic: merge-gate items before main |
| `260624-feat-ws-dashboard-managed-cli-terminal` | todo | Terminal-first managed Codex/Claude/OpenCode CLI surface with shared PTY I/O; now the pre-written substrate for the `260725` PTY-agent pivot |
| `260626-bug-sage-review-config-setter-missing` | todo | Add a lead-facing setter/tuning catalog knob for `sage_review` so review posture can be changed without manual config JSON edits |
| `260626-feat-session-key-format-and-retention` | todo | Change new session keys to three words plus two digits, refresh key-file mtime on keyed use, and prune stale key records about monthly with daily-bounded scans |
| `260627-bug-playbook-render-uses-stale-plugin-cache-during-source-dogfood` | todo | Investigate branch-local playbook render or cache-refresh guidance for source rsrc dogfood |
| `260630-epic-skill-playbook-diet` | todo | Epic: skill playbook diet — trim playbook bodies, MCP surface, and unnecessary golden rules |
| `260702-research-destructive-dedup-methodology` | todo | Aggressive playbook dedup: audit methodology for destructive-first section merges |
| `260703-bug-claude-plugin-cache-stuck-below-source-version-mcp-refuses-start` | todo | Claude plugin cache stuck two versions behind source; MCP refuses to start |
| `260703-chore-review-delegates-true-classification` | todo | Review `delegates: true` classification across rsrc playbooks |
| `260703-chore-windows-branch-pinned-acceptance` | todo | Branch-pinned Windows acceptance for the playbook-factory epic |
| `260708-feat-lead-revive-hook-replacement` | todo | Delete `lead-revive`, replace post-compaction session-key reload with a plugin-bundled hook; needs a fresh sage design review before its next `ready` promotion |
| `260710-bug-lead-write-ticket-playbook-runtime-schema-drift` | todo | `lead-write-ticket` playbook uses a stale `tickets.create` schema |
| `260710-bug-project-index-ticket-focus-stale-status` | todo | This ticket: project index Tickets table / Ticket Focus drifted from live ticket directories; mechanical reconciliation done, recurrence-prevention mechanism still sage-design-blocked |
| `260710-epic-ws-dashboard-terminal-ux-polishing` | todo | Coordinate dashboard-centric terminal/UX polish backlog split from the retired MVP epic |
| `260711-epic-ws-dashboard-command-surface` | todo | ws dashboard command surface board: quick-open, custom commands, shortcuts |
| `260711-idea-dashboard-command-bus-quick-open-shortcuts` | todo | Unify dashboard quick-open command bar, custom command buttons, and keyboard shortcuts on one command bus |
| `260711-idea-dashboard-git-status-polling-index-lock-contention` | todo | Overlapping git status polling risks `.git/index.lock` contention, especially on Windows |
| `260714-chore-dashboard-windows-gateway-frontend-drift` | todo | Windows dogfood gateway serves a stale frontend build; repoint its static-dir at the WSL build |
| `260716-feat-mental-model-comment-placement-rule` | todo | Anchor-scope placement rule: site-local traps to code comments, cross-cutting invariants to mental-model |
| `260716-feat-mental-model-openup-injection` | todo | Deterministic mental-model pointer-injection at delegation open-up |
| `260716-feat-sage-related-mental-model-curation` | todo | Sage design review curates related-mental-model via prose recommendation + lead-owned frontmatter edit |
| `260716-feat-ws-doc-condition-diagnostics` | todo | Hidden doc-condition diagnostics: verification crawl, consumption counters, workflow health metrics |
| `260722-feat-dashboard-hint-click-fast-jump` | todo | Vimium/flash/leap-style hint-click fast-jump over the full visible viewport, performance-gated |
| `260722-refactor-dashboard-app-tsx-state-decomposition` | todo | Decompose App.tsx: untangle the WorkbenchShell/App() state core (design-gated) |
| `260725-bug-dashboard-e2e-harness-destroys-daemon-diagnostics` | todo | e2e browser-acceptance harness drains and discards the daemon's stdout/stderr instead of preserving diagnostics for failure analysis |
| `260725-bug-dashboard-fitnow-short-viewport-shrink` | todo | macOS short-viewport regression gate fails: `fitNow()` shrinks the terminal to 47 rows instead of holding 120 |
| `260725-bug-dashboard-terminal-create-failure-silent` | todo | Terminal creation failure is invisible in the UI: a failed `create_terminal` call is swallowed with no toast, console error, or state change |
| `260512-research-claude-cli-stream-json` | idea | Capture Claude CLI stream-json contract before changing the Claude named-agent runner |
| `260513-research-dual-mcp-startup-order` | idea | Validate dual stdio doctor and HTTP MCP startup ordering |
| `260513-research-streamable-http-mcp-transport` | idea | Research Streamable HTTP transport and reconnect boundaries |
| `260514-research-ws-web-dashboard-direction` | idea | Research dashboard resource model, document UX, harness-library direction, and absorbed child backlog |
| `260523-bug-worktree-local-index-missing` | idea | Explore dashboard-managed propagation of ignored local workflow context across worktrees |
| `260523-research-ws-dashboard-persistable-ui-state-map` | idea | Map persistable ws dashboard UI state |
| `260524-research-ws-dashboard-react-aria-ui-primitives` | idea | Research broader React Aria primitive adoption for dashboard UI |
| `260524-research-ws-dashboard-visual-design-system-refresh` | idea | Research a coherent visual design system refresh for ws dashboard surfaces |
| `260525-bug-ws-dashboard-agent-tab-close-confirmation-sticky` | idea | Investigate sticky agent tab close confirmation in browser acceptance |
| `260605-research-ws-native-subagent-pivot` | idea | ws native-subagent pivot: spawn removal and playbook-factory direction (migration anchor research) |
| `260610-chore-wsflow-explore-playbook-mirroring` | idea | wsflow parity for the explore playbook + native-Explore delegation |
| `260611-research-ws-per-role-delegation-tuning-config` | idea | Config surface for per-role / per-partition delegation tuning (tier + prompt) at the ws level |
| `260620-feat-ws-dashboard-agent-client-activity-sources` | idea | Normalize Codex app-server/OpenCode ACP activity through a dashboard agent-client provider contract; demoted 2026-07-22, now suspended 2026-07-25 under the agent-GUI suspension |
| `260622-epic-ws-dashboard-session-key-realignment` | idea | Coordinate dashboard migration onto ferrule-backed top-level harness sessions |
| `260624-bug-ws-installed-cache-missing-rsrc-manifest` | idea | ws installed cache missing rsrc manifest |
| `260624-feat-ws-dashboard-managed-cli-recent-sessions` | idea | ws dashboard managed CLI recent sessions |
| `260625-research-fork-posture-leak-system-guarantee` | idea | Fork posture-leak: prefer-subagent forks echo deferral instead of executing; system-level guarantee needed |
| `260626-research-playbook-print-lead-surface-leak` | idea | `playbook.print` of lead skill/manual bodies leaks gated bootstrap surface to subagents |
| `260626-research-ws-todo-stack-nesting-model` | idea | Research enter/exit stack-based todo-list nesting model |
| `260627-research-lead-proceed-route-matrix-authoring` | idea | Research whether Route Facts and Route Matrix tables would make `lead-proceed` routing clearer without semantic drift |
| `260629-research-fork-worker-persona-bleed` | idea | Fork worker persona-bleed (model-conditioned, notably Opus 4.8) |
| `260707-chore-dashboard-linked-server-tunnel-dogfood-plan` | idea | Dogfood the SSH-tunnel and localhost-forwarding linked-server paths across a real WSL/Windows boundary; demoted 2026-07-10, still parked on an unresolved SSH-probe escalation |
| `260708-research-lead-revive-low-salience` | idea | `lead-revive`'s post-compaction trigger has low model-attention salience; prerequisite for `260708-feat-lead-revive-hook-replacement` |
| `260710-bug-release-downstream-plugin-layout-untested` | idea | Release shipping does not verify downstream plugin installation layout |
| `260710-idea-dashboard-open-work-root-full-registry-redundant-rediscovery` | idea | Avoid re-discovering every opened work root on every open-work-root call |
| `260710-idea-dogfood-credential-redaction-regex-miss` | idea | Session tooling's credential-redaction regex missed a linked-server passphrase fragment during a dogfood run |
| `260711-idea-dashboard-agent-facing-mcp-control-surface` | idea | Dashboard-owned MCP surface for agent-driven dashboard control (open-file, execution approval, worktree management) |
| `260711-idea-dashboard-workroot-scoped-artifact-consolidation` | idea | Consolidate dashboard-managed workroot files under `.ws-dashboard/`, shared across worktrees |
| `260713-bug-dashboard-acceptance-codex-tile-transcript-hidden` | idea | `dashboard-acceptance.spec.ts` e2e fails: transcript stays hidden after Codex tile click |
| `260713-feat-ws-dashboard-activity-session-fork-cursor` | idea | `ActivitySessionForkRequest` needs a cursor/turn-cut-point field |
| `260713-feat-ws-dashboard-agent-chat-real-adapter-wiring` | idea | Wire agent chat UI to real Codex/Claude adapters (MVP-complete) |
| `260713-idea-dashboard-agent-chat-bubble-visual-design` | idea | Agent chat bubble visual design is undifferentiated (all roles share the same box style) |
| `260714-idea-dashboard-linked-server-localhost-ipv6-hang` | idea | Linked-server endpoint using bare `localhost` can wedge the daemon's outbound client on WSL2 (IPv6 `::1` not forwarded) |
| `260714-idea-dashboard-workbench-active-root-derivation-fragility` | idea | Dashboard workbench "active work root" derivation is structurally fragile — three regressions in one locus in one session |
| `260714-research-dashboard-workroot-watch-push-channel` | idea | Backend "watched work-root" subscription/push channel to replace fixed-interval polling |
| `260720-research-remote-serverroute-masking-audit` | idea | Audit remote-only latent bugs masked by the server-local fallback pattern |
| `260721-bug-lead-write-ticket-sage-ready-ordering` | idea | `lead-write-ticket` attempts a ready move before the sage gate |
| `260721-idea-dashboard-worktree-label-alias-split` | idea | Dashboard workspace label is basename-derived, so differently-named path aliases to the same physical directory still split into separate buckets |
| `260722-bug-dashboard-daemon-git-ops-block-api` | idea | ws-dashboard daemon: long-running git ops (clone/refresh) block the whole async API surface |
| `260722-bug-dashboard-terminal-bottom-row-clip-scrollbar` | idea | ws-dashboard terminal bottom-row clipping and spurious scrollbar at fractional heights |
| `260722-bug-ws-sage-record-index-lock-race-on-concurrent-calls` | idea | `ws sage_record` (and other git-writing ws tools) race on `.git/index.lock` when invoked concurrently |
| `260722-idea-dashboard-git-status-diff-inspector` | idea | Dedicated right-sidebar surface to inspect changed files' contents and diffs, focused via `<leader> g s` |
| `260723-bug-dashboard-terminal-detached-helper-leaks-in-tests` | idea | Dashboard terminal: detached helper processes leak indefinitely when a caller never explicitly closes the terminal |
| `260724-bug-dashboard-daemon-long-session-transcript-oN-degradation` | idea | ws-dashboard daemon: long-uptime responsiveness degrades from O(conversation) live-transcript rebuild + unbounded projector growth |
| `260724-idea-dashboard-daemon-side-git-poll-response-timeout` | idea | Daemon has no bounded timeout around git `Command` invocations, so a wedged git child can hang a request unboundedly server-side |
| `260724-idea-dashboard-daemon-terminal-lifetime-test-interactive-shell-timing` | idea | Daemon `terminal_lifetime` tests are not robust to interactive-shell startup timing and fail on interactive-zsh hosts |
| `260724-idea-dashboard-hotkey-leader-dispatch-gap` | idea | Global leader-key `executeCommand` call has no registered handler for `terminal.create` and most other default leaf commandIds, so leader-sub dispatch silently no-ops for them |
| `260724-idea-dashboard-hotkey-rebind-editor-settings-section` | idea | Dashboard hotkey-rebind editor settings section, split from `260722-feat-dashboard-settings-panel` Phase 2 |
| `260725-bug-agent-synthetic-load-cleanup-guard` | idea | Agent-generated synthetic CPU load must record PIDs at spawn and self-limit; a review subagent's load experiment leaked 70 orphaned busy loops |
| `260725-bug-dashboard-routes-test-terminal-helper-leak-no-reaper` | idea | Integration tests in `tests/routes.rs` leak detached helper processes with no reaper (platform-independent, found during macOS Phase 1 port) |
| `260725-bug-dashboard-terminal-lifetime-load-fragility` | idea | Two pre-existing `terminal_lifetime` tests fail reproducibly under CPU-saturation load (found during macOS Phase 2 acceptance) |
| `260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers` | idea | Terminal registry entries have no schema versioning, so adding a field would orphan live helpers permanently |
| `260725-bug-dashboard-terminal-socket-path-length-unguarded` | idea | Production terminal helper socket path has no guard against the macOS 104-byte `sockaddr_un` ceiling |
| `260725-bug-dashboard-workroot-id-unstable-when-path-canonicalize-fails` | idea | `discovery.rs::canonical_or_normalized` hashes a resolved vs. unresolved path depending on whether the workRoot exists, so `WorkRootId` flips across directory removal/recreation when a path segment is a symlink |
| `260725-idea-ws-git-commit-rename-and-payload-rejections` | idea | `ws/git.commit` cannot commit a staged ticket rename and rejects large `ai_context` payloads with a misleading error |
| `260725-refactor-dashboard-agent-gui-physical-module-isolation` | idea | Tier 2 wire-out: physically extract the suspended agent-GUI modules (FE+BE) from the live dashboard build; needs sage design gating before `ready` |
| `260725-research-ws-dashboard-pty-agent-pivot` | idea | Owner-directed pivot: replace the structured agent-GUI with a thin decorative layer over a vendor agent CLI running in the existing PTY/terminal substrate |
| `260727-bug-dashboard-tab-strip-scroll-swallows-close-click` | idea | Deferred sibling of `260726`: tab-strip scroll on activation may swallow the close click on an overflowing workbench tab strip; verified in dockview source, never reproduced in a browser |
| `260727-chore-dashboard-clippy-never-loop-error-blocks-lint-gate` | idea | A vestigial loop in the attention SSE stream keeps `cargo clippy -p ws-dashboard-daemon --all-targets` permanently red, so clippy cannot gate any phase |
| `260727-chore-dashboard-e2e-helper-modules-never-type-checked` | idea | `tsconfig.e2e-tests.json` includes only `daemonHarness.*`, so shared e2e helper modules and every `*.spec.ts` are type-checked by no script in `package.json` |

## Ticket Focus

- `260725-feat-dashboard-pty-agent-attention-notification` — CLOSED
  (`.done/`, Phase 8 `bd32b10c`). All eight phases landed: the vendor
  turn-boundary hook injected at spawn, the token-authed callback endpoint, the
  server-scoped SSE stream, and the three surfaces it feeds — tab label, nav
  row, and browser chrome. Detail lives in the ticket's per-phase Results and in
  the `ws-web-dashboard` spec/mental-model entries; do not re-derive it here.
  One thread stays open and is NOT discharged by the close: the OS notification
  permission tier is manual-by-design and has never been driven by a human
  (`260726-chore-dashboard-verify-notification-permission-tier-manually`).
  Priced debt left behind: the daemon-side `attention.forget` leak. The
  rebuild-before-Playwright hazard is discharged
  (`260726-chore-e2e-playwright-serves-stale-frontend-dist`, `.done/`,
  `951b0f27`): Playwright `globalSetup` now builds the frontend unconditionally
  on every invocation path, skipping only under `WS_DASHBOARD_STATIC_DIR` or
  external daemon mode and announcing the skip. The daemon-binary half of the
  same hazard is deliberately still open: `cargo build -p ws-dashboard-daemon`
  lives only in `test:browser`.

- `260726-bug-dashboard-restored-tab-close-inert-until-activated` — CLOSED
  (`.done/`, 2026-07-27). Fixed one member of a lost-`click` family on
  terminal tab close buttons: on a reload-restored, never-activated tab, the
  attention badge's presence in the tab's layout flow slid the close button
  under the pointer between press and release, so the `click` landed on the
  tab body instead and was silently swallowed. Taking the badge out of
  layout flow fixed it (measured shift `-11.0px` -> `0.0px`). Deliberately
  NOT discharged by this closure: a second, distinct trigger for the same
  lost-click symptom — tab-strip scroll on activation shifting an
  overflowing tab strip under the pointer — is verified in dockview source
  but has never been reproduced in a browser. That is tracked separately at
  `260727-bug-dashboard-tab-strip-scroll-swallows-close-click` (`idea/`; not
  promoted to `ready` because it lacks a reproduction).

- `260726-bug-dashboard-terminal-notify-silent-failure-no-expiry` — CLOSED
  (`.done/`, 2026-07-27). Single phase, `91b7a6ba` plus remediation `02a6cd2e`.
  `terminal-notify` now writes a per-terminal `notify-failures.json` record on
  every failed delivery and clears it on success; the existing
  `sweep_agent_profiles` pass reads it and emits one `tracing::warn!` per
  terminal for a failure that survived a grace window and was not superseded by
  a callback-target rewrite. The `terminal_notify.rs` stdio-silence CONTRACT is
  intact and guarded by a CLI test. Deliberately NOT fixed, and stated in the
  ticket: the stranded `working` badge on a live session stays stranded — the
  only new signal is operator-facing, in `daemon.log`. Two things the closure
  does not discharge: the end-to-end manual reproduction (a ~10 minute
  two-sweep-period wait with a live agent and a browser) was never run, so the
  escalation rule is proven as pure logic and at the CLI writer level only; and
  `cargo clippy -p ws-dashboard-daemon --all-targets` is still red for an
  unrelated pre-existing reason, tracked at
  `260727-chore-dashboard-clippy-never-loop-error-blocks-lint-gate` (`idea/`).
  Also learned here and worth not re-deriving: `cargo test -p
  ws-dashboard-daemon` aborts at the failing `routes` target and never reaches
  the later integration targets, so `--no-fail-fast` is required for the daemon
  suite to carry any signal.

- `260726-chore-dashboard-terminal-hop1-env-clear-guard-fragile` — CLOSED
  (`.done/`, 2026-07-27). Single phase, one commit `28aaf8b6`, no remediation
  Edition: both review partitions came back with no Critical and no Important
  findings. The hop-1 default-spawn env guard no longer reads a rendering of
  the decision — `build_helper_command` now computes a `HelperEnvPlan` through a
  pure `helper_env_plan`, applies it at exactly one `env_clear()` site, and the
  test asserts the plan value itself, so the primary guard runs identically on
  Windows. Mutation evidence M1-M4 is in the ticket's Result and was
  independently reproduced by the test reviewer; do not re-derive it. Two things
  worth not re-deriving elsewhere: the ticket's own recorded verification
  baseline was already stale when the phase ran (true branch-point baseline at
  `b7f524f7` is lib 236 / routes 176, same two known failure sites), and the
  ticket's ~17 source citations had drifted (+42 lines in
  `build_helper_command`, +97..+101 in the test block, two citations outright
  wrong) — the Result records the corrections.

- `260726-chore-dashboard-verify-notification-permission-tier-manually`
  (`ready/`) — still the ONLY ticket in `ready/`, and still the next goal-run
  target. PHASE 1 IS LANDED (`87259c93`, review fix `4acbdc98`): the Tier 2
  browser gate now exists as a sibling `e2e/agent-attention-notification.spec.ts`
  under `channel: "chromium"`, 4 tests, all 7 assertions proven non-vacuous by
  per-assertion mutation runs recorded in the ticket's Phase 1 Result — do not
  re-derive them. PHASE 2 IS THE NEXT AUTONOMOUS TARGET: reorder
  `currentNotificationAvailability()` to consult `isSecureContext` first, extract
  it as a pure function for unit coverage, settle whether the checkbox is offered
  at all on an insecure origin, and amend both spec anchors that carry the false
  "the `Notification` global is absent on a non-secure context" claim. What will
  finally block closure is neither phase but the `## Human verification residue`
  steps 1-6: a person must watch a native permission prompt and an OS banner. No
  agent may mark them observed or infer them from a green Playwright run, so this
  ticket stays open in `ready/` after Phase 2 lands. Also recorded in the Phase 1
  Result and worth not re-deriving: this ticket's own Constraints text claiming
  only `npm run test:browser` chains the frontend build is FALSE (Playwright
  `globalSetup` builds unconditionally on every invocation path); the
  `ws-web-dashboard` mental-model entry on the same subject was checked and is
  correct, so the staleness is the ticket's alone.

**Ordering (owner, 2026-07-25):** macOS first. Discharged: both phases of
`260725-bug-dashboard-terminal-platform-macos-unsupported` are done and the
ticket closed (`ai-docs/tickets/.done/`) — the daemon builds and
unit/integration-tests natively on macOS (Phase 1), and all four terminal
lifecycle legs (spawn, daemon-restart re-adopt, dead-shell detection,
identity-verified close) pass native runtime acceptance with non-vacuity
proofs (Phase 2). The browser-facing UI/WebSocket gate remains an explicit,
separately tracked gap (see that ticket's Phase 2 Result). The other two ready
tickets and dashboard dogfooding are no longer build-blocked. The one
exception worth taking early — now moot as a special case, but recorded for
history — was the turn-start hook spike inside the attention ticket's Phase 3,
which touches no daemon code and decides whether the `working` state and the
nav spinner exist at all. That spike has since RUN and answered POSITIVE
(2026-07-25): `UserPromptSubmit` fires at human turn submission for the vendor
CLI under an interactive PTY, verified with `Stop` as a same-run positive
control, so the first slice keeps the three-state `working`/`ready`/`idle`
vocabulary and the nav spinner is not deferred. Evidence is inlined in that
ticket's Phase 3 step-1 record. Phase 3 itself is still open — its steps 2-3,
the `0600` hook-config materialization and the daemon-to-helper delivery seam,
remain unverified.

**Live direction (owner-directed, 2026-07-25):** pivot the dashboard's agent
surface away from the structured provider-adapter chat GUI and back to a thin
decorative layer over a vendor agent CLI running in the existing
terminal/PTY substrate — reusing the terminal registry, NDJSON IPC, output
ring, and xterm frontend wholesale (load-bearing invariant: no parallel PTY
subsystem). See `260725-research-ws-dashboard-pty-agent-pivot` for the
direction and `260725-refactor-dashboard-agent-gui-physical-module-isolation`
for the paired Tier 2 wire-out (physically extract the suspended agent-GUI
FE/BE modules; needs sage design gating before promotion to `ready`). This
follows the already-landed Tier 1 suspension (`AGENT_GUI_SUSPENDED` flag,
`c3f5b42b`) and turns away from the structured-adapter track
(`260620-feat-ws-dashboard-agent-client-activity-sources` and related
agent-GUI tickets, now suspended). `260624-feat-ws-dashboard-managed-cli-terminal`
is the pre-written PTY-agent substrate design for this pivot; its 2026-07-11
priority supersession was formally REVERSED 2026-07-25 (owner framed the pivot
as "the revival of the PTY agent itself"), recorded by editing that ticket's
Background rather than superseding it.

- `260710-bug-project-index-ticket-focus-stale-status` (todo, bug) — this
  ticket. The mechanical reconciliation half is done as of this pass; the
  recurrence-prevention-mechanism half remains sage-design-blocked (pick
  automated guard vs. documented manual-regen procedure vs. some other
  shape) before it can promote to `ready`.
