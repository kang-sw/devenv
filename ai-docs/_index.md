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
| `260725-bug-dashboard-terminal-platform-macos-unsupported` | ready | Dashboard daemon does not build on macOS: the `#[cfg(unix)]` terminal platform layer is Linux-only (pidfd syscalls + `/proc` start-time) |
| `260725-feat-dashboard-nav-row-two-line-open-state` | ready | Left-nav work-root rows: two-line layout with open-surface counts, plus open-vs-closed de-emphasis |
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
| `260725-refactor-dashboard-agent-gui-physical-module-isolation` | idea | Tier 2 wire-out: physically extract the suspended agent-GUI modules (FE+BE) from the live dashboard build; needs sage design gating before `ready` |
| `260725-research-ws-dashboard-pty-agent-pivot` | idea | Owner-directed pivot: replace the structured agent-GUI with a thin decorative layer over a vendor agent CLI running in the existing PTY/terminal substrate |

## Ticket Focus

- `260725-bug-dashboard-terminal-platform-macos-unsupported` (ready, bug) —
  **priority target (owner: macOS must be feature-complete, 2026-07-25).**
  The daemon does not compile on macOS: `terminal_platform.rs`'s
  `#[cfg(unix)]` module is Linux-only (pidfd syscalls, `/proc` start-time),
  so `260723`'s "both platforms" claim really means Linux + Windows. Kill
  mechanism is pinned (verify → `kill(2)` → best-effort post-kill re-read via
  `proc_pidinfo`/`PROC_PIDTBSDINFO`; kqueue and `task_for_pid` rejected with
  reasons in the ticket). Phase 1 build/identity, Phase 2 native macOS
  lifecycle acceptance. Spec addressing via `## Spec Impact`
  (`ws-web-dashboard/index.md`, Contract-first: no) — includes amending the
  currently-absolute never-kill-a-recycled-pid guarantee into a platform
  tier. Sage combined = passed.
- `260725-feat-dashboard-nav-row-two-line-open-state` (ready, feat) —
  owner UX request: left-nav work-root rows get a second line showing open
  terminal/document counts, plus open-vs-closed de-emphasis by saturation.
  Agent counter deliberately deferred until the PTY pivot settles what an
  agent pane is. The substantial part is data plumbing, not CSS:
  `terminalPanes` is `WorkbenchShell`-local while the nav renders from
  `App()`. Spec addressing via `## Spec Impact`
  (`#260516-ws-web-dashboard-inspectable-navigation-shell`, Contract-first:
  no). Sage combined = passed.
- `260725-feat-dashboard-terminal-steady-state-stream-throughput` (ready, feat)
  — open-state terminal streaming throughput: four independent,
  behavior-preserving per-output-chunk fixes remaining after the `260723`
  batch — Phase 1 xterm WebGL renderer (canvas/DOM fallback), Phase 2 port the
  daemon O(1) `output_after` to the helper ring, Phase 3 batched Output frame
  (array of chunks) across both IPC hops, Phase 4 debounce the per-chunk
  `refocusActiveTerminal`. Deliberately scoped away from a retention/replay
  redesign (would collide with the gapless-contiguous sequence invariant and
  xterm-owned scrollback). Phase 1 shipped live as a hotfix; Phases 2–4 open.
  Spec addressing via `## Spec Impact`
  (`#260516-ws-web-dashboard-terminal-io-transport`, Contract-first: no). Sage
  combined = passed.

- `260726-refactor-ws-dashboard-git-fs-watch-invalidation` (ready, refactor) —
  owner-directed 2026-07-26: replace interval-driven git polling with
  FS-watch-driven epoch invalidation behind cheap cached endpoints, keeping
  polling as a hard TTL ceiling (**120 s armed** / 2 s degraded) so a missed event
  self-heals with no resync protocol — the armed ceiling is only the
  missed-event safety net, since real changes land within one tick via the epoch
  bump. Measured driver: 9.6 git spawns/s (~830k/day) at 12.0% of a core, cut to
  2.1/s and 5.7% by the already-landed probe-memo hotfix (`18037cc3`) — which is
  only amortizing the fan-out behind a 30 s TTL, not removing it. **Four phases
  in scope:** 1 git-exec seam (timeout, concurrent pipe drain, explicit spawn
  stats, warn only on unexpected failure) which also closes
  `260724-idea-dashboard-daemon-side-git-poll-response-timeout`; 2 per-root
  `resolve_git_context` reusing the existing `resolve_online_available_work_root`
  gate, plus the `git_identity` memo that is the real measurable win; 3 result
  cache with `EpochSource` stubbed; 4 the `notify` watcher, which **arms on every
  platform** — that is what it absorbed from the former Phase 5. Governing
  invariant (owner 2026-07-26): *never register a watch we did not count.*
  Registration is therefore platform-split, and deliberately so: **recursive on
  Windows/macOS** (count is 1 per target by construction — one kernel handle / one
  FSEvents stream, no walk, no cap; the ignore set only filters events), and
  **gitignore-aware walk → per-directory `NonRecursive` → counted against a cap
  (default 1024) → arm all or degrade wholly on Linux**, because there is no kernel
  subtree primitive and `notify`'s emulation registers descriptors we cannot count.
  Recursive-on-Linux is a Non-Goal. The Phase 5 heading is retained only as
  history — split off, dropped, inverted to uniform-per-directory, then corrected
  back to the split; the ticket records all four states. Two verified facts:
  pruned directory counts are **~200/repo, not the ~2,400 previously assumed**
  (which is what makes the Linux cap a safety valve rather than a live limit), and
  **`git status -uno --ignored=matching` returns zero `!!` entries** on git 2.43.0
  — the ticket's original command would have silently produced an empty ignore set
  on every repo; use `-unormal`. Decided against git SSE push (the problem is CPU,
  not latency, and FS events are lossy hints). Plan at
  `ai-docs/.plans/2026-07/26-1315-git-fs-watch-invalidation.md`. Spec addressing
  via `## Spec Impact` (`#260524-ws-dashboard-git-aware-workroot-toolbar`,
  Contract-first: no). Sage combined = passed (design concern, all findings
  applied). **Phase 1 landed 2026-07-26 (`0c48065a`, impl plan
  `ai-docs/.plans/2026-07/26-1511-git-exec-seam.md`)** — the git-exec seam ships
  with `GET /api/dashboard/diag/git` and `WS_DASHBOARD_GIT_TIMEOUT_MS` (default
  10 000; `0` = unbounded), spec'd at
  `#260726-dashboard-git-invocation-budget-and-spawn-diagnostics`. Two facts to
  carry into Phase 2: the phase's own acceptance number (spawns/s from two diag
  reads 60 s apart on the Windows dogfood host) was **never measured** — the
  sandbox is Linux, so whichever phase needs it must take it; and the `#[cfg(windows)]`
  test counterparts were written but never executed on Windows. The seam's bound
  means "bounded except a child wedged in uninterruptible I/O"; that residue plus
  the per-timeout detached reader threads are forwarded to
  `260726-refactor-ws-dashboard-long-uptime-leak-hardening` Phase 2, whose
  bounded-timeout half is now delivered. `git_worktree.rs`'s 8 direct git spawns
  stay outside the seam and counters by design →
  `260726-refactor-dashboard-worktree-git-spawns-through-exec-seam` (todo; the
  budget, not the counting, is the open question, since `worktree add` can
  legitimately outrun any poll-path budget). **Phase 2 landed 2026-07-26
  (`3b66441d`, impl plan `ai-docs/.plans/2026-07/26-1717-per-root-git-context.md`,
  dispositions D1-D7)** with a simpler mechanism than the phase specified: no
  separate identity cache, no `WS_DASHBOARD_GIT_IDENTITY_NEGATIVE_TTL_MS`, and no
  `WatchKey` (deferred to Phase 3, its first real consumer). `GitDiscovery::probe`
  already answers `--show-toplevel`/`--git-common-dir` in **one** memoized spawn,
  so the Activity identity is derived from the shared `GitProbeCache` instead of
  re-asking in two; `None` rides the existing 30 s probe TTL, which makes the
  Activity pane agree with the sidebar's git/plain label rather than
  self-correcting 27 s earlier. Spec'd at
  `#260726-dashboard-shared-git-probe-memo-and-per-root-git-context`. Three things
  to carry forward: the accepted "unavailable ⇒ 409" delta is **only true for one
  poll interval** — `live_dashboard_resources_with_sync` unregisters the root and
  the id goes back to 404, filed as
  `260726-idea-dashboard-resources-poll-eagerly-prunes-unavailable-work-roots`;
  **Phase 4 must not assume `GitProbeCache::evict` works**, because its key
  (`discovered.path`) diverges from the warm memo key (`canonical_or_normalized`,
  `\\?\`-prefixed on Windows) so the evict silently misses on the reappear
  transition the reconcile exists to handle (pre-existing `18037cc3`, noted inline
  in Phase 4); and a git probe that fails to answer is now memoized as "not a
  repository" for the full TTL, emptying the Activity pane for up to 30 s after one
  timeout. The diag delta acceptance number is **still unmeasured** — two phases
  in a row have closed without it, so Phase 3 or 4 should take it rather than
  inherit it again. **Phase 3 landed 2026-07-26 (`b8e4f89b`, impl plan
  `ai-docs/.plans/2026-07/26-1830-git-state-cache.md`, dispositions D1-D8)**:
  `GitStateCache` with two independently-revalidated slots per `WatchKey`
  (`worktree`, `refs`) and `MutationEpochSource` (real per-key counters,
  correcting the ticket's `StaticZero` label — a literal always-zero source
  cannot be told apart from "never bumped"). Measured cold-cache payoff for
  one concurrent `/git/status` + `/git/branches` pair: 7 → 6 spawns
  (no-upstream) / 10 → 8 (upstream-tracked); no TTL-driven win is claimed,
  since every steady-state 5 s poll tick still misses the 2 s TTL by design —
  the win is de-duplicating the union refs fill plus single-flight burst
  coalescing on a concurrent miss. One review cycle (3 partitions, 8
  Important findings total, all fixed): switch/create branch was TTL-delaying
  the *worktree* axis (a `.gitignore` difference between branches can flip
  tracked/untracked status even on a tree-neutral switch); a failed
  `pull --ff-only` was not invalidating refs despite its embedded fetch
  having already mutated `refs/remotes/*` before the ff-only merge aborted;
  `git worktree add`/`remove` cleared `GitProbeCache` but not the new
  `GitStateCache`, so a dashboard-driven worktree change could leave
  `/git/branches` stale; the TTL env var was read per-request instead of once
  per process; and three tests (the D2 single-flight payoff, the D7
  epoch-sample-before-probe pin, and the `current_branch_counts` reuse path)
  passed under implementations that violated the property each claimed to
  pin. Carry-forward to Phase 4: `WatchKey` is keyed per worktree path but
  `refs/heads`/`worktree list` are repository-wide, so a mutation in one
  linked worktree can leave a sibling's cached refs stale for up to the TTL
  (bounded, self-healing) — Phase 4 already widens `DiscoveredWorkRoot` with
  `git_dir`/`common_dir`, which is where the refs axis should be re-keyed by
  common dir instead. The diag delta acceptance number is **still unmeasured
  after three phases**; Phase 4 should take it.

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
is the pre-written PTY-agent substrate design for this pivot.

- `260710-bug-project-index-ticket-focus-stale-status` (todo, bug) — this
  ticket. The mechanical reconciliation half is done as of this pass; the
  recurrence-prevention-mechanism half remains sage-design-blocked (pick
  automated guard vs. documented manual-regen procedure vs. some other
  shape) before it can promote to `ready`.
