---
title: "Pi adapter: replace the default footer with lead and subagent cost telemetry"
related:
  260906-workset-ws-pi-dogfood-ux: owner-confirmed Pi UX request
  260909-feat-ws-pi-agent-row-model-and-usage: existing per-agent cumulative usage and estimated-cost telemetry to aggregate
spec:
  - pi-adapter-runtime
sage-review-completeness: completed
sage-review-design: completed
sage-review-design-reviewed: 508457798b30f385
sage-review-completeness-reviewed: 508457798b30f385
---

# Pi adapter: replace the default footer with lead and subagent cost telemetry

## Background

Pi 0.85.1 exposes extension status and widget APIs but no hook for styling one field of the built-in footer. `ctx.ui.setFooter(factory)` replaces the footer as a whole; `setFooter(undefined)` restores the built-in component. The owner wants the current lead-session cost colored and the cumulative cost of every subagent associated with that lead session shown separately.

The existing ws agent list is not a footer. It is a `setWidget("ws-agents", ..., { placement: "belowEditor" })` component and remains independent of this replacement.

## Decisions

- Install one ws-owned custom footer through `ctx.ui.setFooter(factory)` in applicable TUI lead sessions. The ws Pi extension is the sole custom-footer owner; composition with another extension calling `setFooter` is not required.
- Reproduce the useful current built-in footer information rather than replacing it with a ws-only status line: working directory, Git branch, model, context usage, current lead token/cost information, session name where applicable, and every extension status from `footerData.getExtensionStatuses()` remain visible subject to width.
- Color only monetary values with Pi's semantic `accent` theme token. Labels and the remaining footer fields retain the built-in footer's dim/warning/error semantics; do not embed fixed ANSI color codes.
- Add a distinct `Subagents` cost segment. Its scope is every descendant associated with the current lead session, including running, idle, dormant, and capacity-evicted children; unrelated lead sessions are excluded. Preserve an eviction roll-up so the displayed cumulative total never decreases merely because a registry record is removed.
- Reuse child-attributable cumulative telemetry rather than charging inherited context to a child. Count each child once across reload, retention, and eviction. Reconstruct the session total from retained ownership/session records and the roll-up without double-counting.
- Render a fully known cumulative estimate as `~$1.42`, a partial estimate as `~$1.42 + ?`, and wholly unknown cost as `—`. A zero known total remains an explicit zero rather than unknown.
- Keep the existing `belowEditor` agent-list widget unchanged. Goal-loop and other `setStatus` callers continue to work because the custom footer renders `footerData.getExtensionStatuses()`.
- Reinstall the footer on each applicable `session_start`; dispose subscriptions and restore the default footer on shutdown, reload, mode change, or controller replacement. Subscribe to Git branch and telemetry changes and request a render when visible state changes.
- Use the host-provided TUI/theme component instances. If runtime Pi TUI utilities are required, resolve them through the existing host-safe loader rather than a static runtime import that can create a second incompatible `pi-tui` copy.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/index.ts, agents-plugin-pi/src/agent-telemetry.ts, agents-plugin-pi/src/agent-widget.ts, agents-plugin-pi/src/pi-tui.ts, footer module, and tests |
| scope.surface | public-interface | owner-visible persistent TUI footer replaces Pi's built-in status surface |
| scope.new_public_symbol | no | internal footer controller only |
| scope.new_type_contract | yes | descendant cost aggregate and eviction roll-up state |
| scope.test_surface | existing | agents-plugin-pi/test/agent-telemetry-lifecycle.test.ts and agents-plugin-pi/test/agent-widget.test.ts cover telemetry lifecycle, themes, and width bounds |
| complexity.reuse_points | confirmed | agents-plugin-pi/src/agent-telemetry.ts, agents-plugin-pi/src/agent-widget.ts, and agents-plugin-pi/src/pi-tui.ts |
| complexity.side_effect_risk | high | setFooter replaces the entire built-in footer and is last-writer-wins across extensions |
| risk.correctness | high | descendant aggregation must avoid lost or double-counted cost across dormancy, eviction, reload, and nested lineage |
| risk.fit | high | replacement must preserve built-in fields and extension statuses beside the separate agent widget |
| risk.test | high | lifecycle, theme, narrow-width, unknown-estimate, and retention/eviction boundaries |
| risk.security_or_contract | moderate | persistent owner-visible accounting must not attribute another lead session's usage |

## Phases

### Phase 1: Install a cost-aware ws custom footer

Implement a theme-aware custom footer for TUI lead sessions using `ctx.ui.setFooter(factory)`. Preserve the current built-in footer fields and extension-status map, color the current lead monetary value with `accent`, and add a separate accented `Subagents` cumulative-cost segment. Keep the current agent-list widget below the editor unchanged.

Aggregate all descendants owned by the current lead session, including retained dormant records and an eviction roll-up, without charging inherited context or double-counting after reload. Apply the confirmed known, partial, unknown, and zero display forms. Reconstruct state on session start, subscribe to branch and telemetry changes, invalidate theme-derived rendering correctly, bound every line to the supplied width, and dispose/restore cleanly.

Verification covers: known/partial/unknown/zero lead and descendant costs; direct, fork, Explore, execute-worker, nested, dormant, and capacity-evicted children; reload and session-switch reconstruction without decreases or duplication; exclusion of unrelated lead sessions; preservation of Git branch, model, context, session name, and extension statuses; coexistence with the existing `belowEditor` agent widget; light/dark theme rerender; 120/80/40-column layouts; last-writer replacement behavior; and default-footer restoration on shutdown. Owner-live acceptance confirms the colored lead cost, separate cumulative `Subagents` value, unchanged agent cards, and retained goal/status messages in a real Pi TUI session.

### Result (e62ae22e) - 2026-09-13

Installed a host-TUI-resolved custom footer for lead and fork TUI sessions. It retains path, branch, session, token/cache, context, model/thinking, and extension-status information while rendering separate accented `Lead` and `Subagents` monetary values with explicit known, partial, unknown, and zero forms. The existing `belowEditor` agent cards remain independently mounted.

Child-attributable telemetry now persists in ownership metadata. Recursive aggregation follows exact ownership/session lineage through worker, execute-worker, fork, and Explore descendants, excludes unrelated leads, and uses owner-scoped identity-keyed roll-ups before capacity or retention deletion so reloads and retries neither lose nor double-count cost. Owner-artifact I/O shares the storage module's canonical containment and symlink-refusal boundary.

Verification:
- Final package suite excluding the independently failing `test/fork-lifecycle.integration.test.ts`: 1,792 passed, 0 failed, 2 skipped. The excluded matrix's six cases already failed in the earlier full `npm test` run because its inherited-tool fixture expected `ws-queue-question`; this change does not modify that tool-registration surface.
- Focused footer, telemetry, storage, retention, and spawner suites passed, including real watcher updates, production lifecycle seams, ANSI-aware 120/80/40-column bounds, real capacity eviction, nested descendants, and durable reload/non-duplication.
- `npm pack --dry-run --ignore-scripts` passed and includes `src/agent-footer.ts`.
- Owner-live TUI observation remains a post-integration acceptance check because this worker runs headlessly.

Independent partitioned review found one Critical and eight Important issues in round one. Commit `270ac98f` fixed known-zero formatting, empty-namespace watcher discovery, legacy eviction, built-in cache-rate fidelity, storage ownership of roll-up I/O, and the initial coverage gaps. Round two correctness and fit were clean; its two remaining Important test-coverage findings were fixed in `c0061e8d` through the exact `index.ts` lifecycle seams and a second real watcher transition. No Critical finding remains.
