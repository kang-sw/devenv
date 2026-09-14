---
title: "Re-sync the Pi track onto settled epic/refound and land Pi as a 1-tier ws-mcp harness peer"
related:
  260905-feat-ws-pi-harness-config-layer: harness-peer infra (harness enum, ws-pi-bridge detection, agents.tier, prompt overrides, .pi.md variant selection) already landed on develop; this epic consumes it rather than re-building it
  260906-bug-ws-pi-rsrc-mirror-drift: owns the byte-identical agents-plugin-pi/rsrc mirror plus the ".pi.md authored upstream under agents-plugin/rsrc, mirrored" architecture the overlay child consumes; ready/blocked on a separate owner dogfood diagnosis and NOT reopened here
  260907-research-ws-pi-extreme-delegation-lead-profile: re-scoped by this epic to its residual after refound subsumes its core
  260908-bug-ws-pi-delegated-tool-surface-unavailable: possible shared root cause with 260906's blocked dogfood error; flagged, not owned
  260910-bug-ws-execute-worker-missing-exec-tool: the lead-delegate to ws-execute delegated-child tool-surface seam this epic names as a planned follow-up
  260802-research-ws-pi-native-framework: Pi-native direction anchor
---

# Re-sync the Pi track onto settled epic/refound and land Pi as a 1-tier ws-mcp harness peer

## Scope

The Pi extension currently pins its ws-mcp build to a frozen compat worktree
(`ws-mcp-compat-a937b8dc`, an `epic/refound` mid-point) so the Pi track did not
rebuild against the churning root worktree while `epic/refound` moved. The owner
reports (2026-09-11) that `epic/refound` has settled and is bound for `develop`
to ship. This epic re-synchronizes the Pi track onto the settled ws-mcp and
completes Pi's promotion from a bespoke framework extension to a **first-tier
harness peer** that consumes ws-mcp's already-landed harness-keyed surfaces
(`260905`). Two axes:

1. **Mechanical sync** of the Pi track's runtime binding, tool contract, and
   mirrored shared rsrc onto the settled ws-mcp
   (`260911-chore-ws-pi-track-sync-to-refound`).
2. **Behavior-text tune-up** that makes shared playbooks render Pi-correct: the
   ws-mcp terminology `pi` entry plus selected `.pi.md` overlays
   (`260911-feat-ws-pi-harness-idiom-playbook-overlays`).

Consume, do not fork: `epic/refound` made "worker as workflow interpreter, lead
as escalation handler" the shared default (`lead-discuss` / `lead-delegate` /
`lead-run` / `ticket-worker`), which subsumes the core of the Pi extreme-
delegation profile. The Pi adapter consumes those primitives; it does not
maintain a parallel orchestration layer.

## Non-Scope

- Executing the `epic/refound` -> `develop` merge itself (a develop-side release
  action, not Pi-track work).
- ws-mcp tool-contract changes beyond the already-landed `260905` surface;
  refound's 7-tool removal is consumed, not authored here.
- Re-opening `260906`'s blocked dogfood diagnosis or fixing `260908` / `260910`
  root causes; related, not owned.
- The extra orchestrator level (lead -> orchestrator -> worker); a deferred open
  question living in `260907`'s residual, undecided.
- Mechanism surfaces (spawn / `ws-execute` / `ws-worker-exec` / approval / fork /
  TUI / claude-delegate) stay adapter-owned; not moved into ws-mcp.

## Child Tickets

- `260911-chore-ws-pi-track-sync-to-refound` - re-point the local-devenv marker,
  byte-sync `runtime.json`, absorb refound shared rsrc, retire the compat
  worktree. Pi-track-local authorship.
- `260911-feat-ws-pi-harness-idiom-playbook-overlays` - add the ws-mcp
  terminology `pi` entry and author selected `.pi.md` overlays. Develop-authored,
  reaches Pi by cherry-pick + the `260906` mirror.
- Re-scoped: `260907-research-ws-pi-extreme-delegation-lead-profile` - residual
  (curated lead registration allowlist + lineage TUI + adapter-owned mechanism
  guides) after refound subsumes its core.
- Planned (not yet ticketed): bind the `lead-delegate` to `ws-execute`
  delegated-child tool-surface contract; scope once `260908` / `260910`
  diagnosis lands.

## Cross-Child Decisions

1. **Ready-promotion gate (owner, 2026-09-11).** No actionable child is promoted
   to `ready/` until `epic/refound` has merged into `develop`. The develop root
   worktree is the sync source of record; before the merge the tracked-file sync
   (runtime.json, mirrored rsrc, overlays) has no settled source. The marker
   re-point is path-based and may proceed independently, but is not itself a
   ready-gated deliverable.
2. **Authorship boundary (AGENTS.md clause 1).** ws-mcp Go source and shared
   rsrc/overlay text (the terminology `pi` entry, the `.pi.md` overlays) are
   authored on `develop` and reach the Pi track only by cherry-pick. Only
   Pi-track-local artifacts (the `.local-devenv-runtime` marker,
   `agents-plugin-pi/runtime.json`, the rsrc mirror sync, adapter guides, the
   `260907` residual) are authored on the Pi track. Each child's Constraints
   states which side authors it.
3. **Consume, not fork.** The Pi adapter consumes refound's shared delegation
   primitives and ws-mcp's harness-keyed surfaces (`260905`). It adds no parallel
   orchestration and no competing tool vocabulary.
4. **Text-home split.** Pi-specific behavior text with a shared base playbook and
   a small delta -> `.pi.md` overlay authored under `agents-plugin/rsrc/`
   (mirrored per `260906`). Pure-Pi-mechanism text with no shared base
   (execute-worker / orchestrator guides describing `ws-execute` /
   `ws-worker-exec` / approval, `ws-report-to-lead`) -> adapter-owned
   `systemPromptPath` guides. Reserved terminology vars (`SpawnIdiom` /
   `ExploreAgent` / `ContinueIdiom`) are NOT caller-overridable — the
   tool-injected terminology layer deterministically overwrites caller context
   in `buildPlaybookVars` — so their Pi values must live in ws-mcp's Go
   terminology table, never in a Pi-side render-context override.

Flag (not owned): `260906` is `ready/`-blocked on an unresolved owner dogfood
error; `260908` (delegated tool-surface unavailable) may share its root cause.
Diagnose there, not here.

## Completion Criteria

- Done: both actionable children in `.done/`; the Pi track builds ws-mcp from the
  develop root worktree (compat worktree retired); shared playbooks render
  Pi-correct idioms; `260907` re-scoped.
- Dropped: if the `epic/refound` -> `develop` merge is abandoned (the sync target
  disappears).
- Deferred: the extra orchestrator level (`260907` open question); the
  `lead-delegate` to `ws-execute` contract-binding child (pending `260908` /
  `260910` diagnosis).
