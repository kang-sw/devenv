---
title: "Make lead-run ticket-only: retire its ad-hoc implementation-contract route, route bounded ticketless code work to lead-delegate, and add a tier-unaware delegate-implementer playbook"
related:
  260909-epic-ws-worker-interpreter-refoundation: context; this refactor tightens the lead surface the refoundation collapsed, removing the one ticketless worker-ceremony route that never had a clean home
  260911-research-golden-fixture-verification-gap: adjacent; the run_ad_hoc golden pin removed here is one of the exact-prose fixtures that research covers
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0858c3ae54efac5f
sage-review-completeness-reviewed: 0858c3ae54efac5f
completed: 2026-09-11
---

# Make lead-run ticket-only and give lead-delegate a disciplined implementer

## Background

`lead-run` today carries two entry shapes: drain the `ready/` queue (or a direct
ticket path), and accept an **ad-hoc implementation contract** — a ticketless
description the lead hands to a worker "whose scope, behavioral impact, or review
needs warrant the full worker workflow" (`agents-plugin/rsrc/lead-run/lead-run.md:28-30`).

That ad-hoc route has no clean home:

- The worker it spawns is `ticket-worker`, whose whole body assumes a ticket:
  its Inputs read the ticket file and pick the earliest phase without a
  `### Result`; Execute step 1 calls `route.resolve_implement(target: {kind:
  "ticket", ticket_path})`; step 5 appends a `### Result` and `tickets.close(stem)`
  (`agents-plugin/rsrc/ticket-worker/ticket-worker.md:22-25,57-58,79-82`). With a
  `Contract:` line and no ticket, every ticket-shaped step degrades and the worker
  improvises. A 2026-09-11 ad-hoc run (the `lead_delegate_contract.json` golden
  fix, commit `0c884ce4`) confirmed the misfit: the worker made the edit but left
  it uncommitted with no impl branch, so the lead had to commit — delegate-shaped
  behavior emerging from a worker-shaped playbook.
- `lead-delegate` already claims exactly this work. Its routing gate sends a task
  to `lead-run` only when a ready ticket owns it, the change touches public
  behavior/API/protocol/canonical/architecture, scope is unclear, **independent
  review is needed**, or an unresolved decision remains; **otherwise** delegate
  handles a "bounded, reversible, self-verifying task, including repository
  mutations such as housekeeping, mechanical updates, and localized internal
  hotfixes" (`agents-plugin/rsrc/lead-delegate/lead-delegate.md:20-31`). The golden
  fix — mechanical, self-verifying, reversible, no independent review — is by that
  gate's own criteria a delegate task, not a lead-run task.

So the ad-hoc lead-run route overlaps delegate's declared territory, and what it
uniquely offered (an independent reviewer + isolated impl branch on a ticketless
task) is a near-contradiction: full worker ceremony *is* the ticket. The gap that
remains is that `lead-delegate` gives its executor no coding discipline — its
`## Assignment` (`lead-delegate.md:39-51`) tells the lead to hand-author an
executor prompt with no floor for commit hygiene, test verification, or
convention-following. This ticket closes the surface by removal on the lead-run
side and by a small, reusable implementer playbook on the delegate side.

## Decisions

- **`lead-run` becomes ticket-only.** Remove the ad-hoc implementation-contract
  route entirely: the Select-skip paragraph (`lead-run.md:28-30`), the two `Ad
  hoc` rows in the Spawn worker-selection table (`:52-53`), the ad-hoc `Contract:`
  task-block substitution (`:80-81`), the ad-hoc judgment clause in tier
  selection (`:46`), and the intro sentence that admits a contract (`:11-15`).
  `lead-run` still supports both queue-drain and a direct ticket path — both
  ticket-shaped — so no ticket-driven behavior is lost. *Rejected:* patching
  `ticket-worker` to add a first-class contract/no-ticket mode — that grows the
  worker to serve a route whose own justification the delegate gate already
  routes away, i.e. it adds surface to keep a redundant one.
- **Bounded ticketless code work is lead-delegate's, unchanged in principle.**
  delegate already owns "mechanical updates, localized internal hotfixes" by its
  routing gate; no new route is created, the boundary is just made real by giving
  delegate the missing discipline (below). *Rejected:* a third top-level skill for
  ticketless code — the two-pole model (ticket-run vs delegate) is the point.
- **The eliminated middle tier is deliberate.** Anything that genuinely needs an
  independent reviewer or an isolated branch becomes a ticket; that is what the
  ceremony is for. delegate's handoff line — today "returns an implementation
  contract for lead-run" (`lead-delegate.md:33-36`) — re-points to
  `lead-ticket`: on discovering broader scope, the delegate stops and the lead
  authors a ticket, rather than hand a contract to a now-ticket-only lead-run.
- **New `delegate-implementer` playbook: a tier-unaware minimum floor.** A
  renderable rsrc playbook (not inline delegate text, so it is reusable and keeps
  delegate lean) carrying the least contract an executor must satisfy *whatever*
  core assignment the lead provides: commit discipline sufficient to reconstruct
  the change's intent afterward — the commit message records rationale and
  rejected alternatives so the mental model is recoverable post-hoc, stated as a
  generic commit-discipline floor and **not** tied to this repository's own
  `## AI Context` commit-section heading (that heading is a devenv AGENTS.md
  convention; a shipped playbook must not impose it downstream — see
  `shipped-surface-boundary.md`) — minimal test verification (run the relevant
  build/test and read full output before claiming pass), and coding conventions
  (surgical changes, follow existing style, resolve warnings the change introduces). It is **tier-unaware** — it names no model or
  tier; delegate still resolves the executor's tier separately via
  `config.resolve_agent(tier)`. It carries **no independent reviewer** — that is
  the deliberate line between delegate (self-verify) and worker (independent
  review). The lead's core contract sits on top; this playbook is only the bottom.
  *Rejected:* folding the discipline into `lead-delegate`'s Assignment prose —
  that bloats the lead playbook and cannot be rendered as an executor system
  prompt; and resurrecting the retired `lead-implement` skill wholesale — its
  weight and review machinery are exactly what "lightweight" excludes.
- **delegate renders `delegate-implementer` whenever the assignment writes code.**
  When the bounded assignment includes a repository mutation, delegate
  `playbook.render`s `delegate-implementer` and passes its path as (part of) the
  executor's system prompt; pure evidence-gathering/investigation assignments do
  not get it. *Rejected:* lead discretion per task — that leaves the "undisciplined
  ad-hoc code" gap this ticket exists to close.

## Constraints

- Convention: ai-docs/manuals/skill-authoring.md (lead-run, lead-delegate, and the
  new delegate-implementer are shipped playbooks/skills under
  `agents-plugin/rsrc/` and `agents-plugin/skills/`; apply its invariant checklist
  to every changed Invariants/Constraints line).
- Convention: ai-docs/manuals/wsflow-mirroring.md (every edit under
  `agents-plugin/rsrc/` and `agents-plugin/skills/` mirrors to
  `agents-plugin-wsflow/`; the new playbook and both playbook edits ship in wsflow
  too).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (all three of lead-run,
  lead-delegate, and delegate-implementer are downstream-shipped text; the
  implementer's discipline must be generic, not depend on anything this repo alone
  has — no repo-only fact, no binding-anchor leak).
- AGENTS.md `## Project Orientation` canonical-flow block lists `Direct: run
  <ticket-path or implementation contract>`; the "or implementation contract"
  clause is removed there as part of this change. This is a canonical-flow edit,
  already user-approved for this ticket.
- The change must land as one coherent unit: removing the `run_ad_hoc` golden pin,
  the lead-run body edit, the delegate rewire, and the new playbook are mutually
  dependent (delegate renders a playbook that must exist; the golden must stop
  pinning prose that is being removed), so no intermediate red state is acceptable.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin/rsrc/lead-delegate/lead-delegate.md, agents-plugin/rsrc/delegate-implementer/delegate-implementer.md (new), AGENTS.md, agents-plugin/tests/fixtures/lead_delegate_contract.json, agents-plugin/tests/test_skill_dispatch_contracts.py, plus their agents-plugin-wsflow mirrors |
| scope.surface | public-interface | shipped playbook prose pinned by an exact-prose golden (agents-plugin/tests/test_skill_dispatch_contracts.py:56-73) and the AGENTS.md canonical-flow line |
| scope.new_public_symbol | yes | new playbook name delegate-implementer (agents-plugin/rsrc/delegate-implementer/delegate-implementer.md) |
| scope.new_type_contract | no | new playbook is prose only, no new schema or function signature |
| scope.test_surface | existing | agents-plugin/tests/test_skill_dispatch_contracts.py:68 and fixtures/lead_delegate_contract.json:10 are edited, no new test files planned |
| complexity.reuse_points | confirmed | ticket-worker's render-only, no-SKILL.md-shim pattern (agents-plugin/rsrc/ticket-worker/ticket-worker.md:2 kind: render; no agents-plugin/skills/ticket-worker dir) and the existing config.resolve_agent call (agents-plugin/rsrc/lead-delegate/lead-delegate.md:47) |
| complexity.side_effect_risk | moderate | removes a routing path shared by two lead skills, mirrored into a second package, guarded by exact-prose goldens |
| risk.correctness | moderate | six coordinated edits (new playbook, two lead bodies, AGENTS.md, two test artifacts) plus a full wsflow mirror must land as one unit or the goldens go red |
| risk.fit | low | the Decisions section resolves the delegate/run boundary with named rejected alternatives; no open architecture question remains |
| risk.test | low | the exact test/golden locations named in the ticket exist and match the described change (test_skill_dispatch_contracts.py:68, fixtures/lead_delegate_contract.json:10) |
| risk.security_or_contract | moderate | edits a canonical-flow line in AGENTS.md and two lead skills' routing contract; the ticket states this is already user-approved |

## Phases

### Phase 1: Retire the ad-hoc route, add delegate-implementer, rewire delegate

Land the whole surface change as one coherent unit, keeping the flagship
dispatch-contract suite and the wsflow package tests green throughout.

1. **New playbook** `agents-plugin/rsrc/delegate-implementer/delegate-implementer.md`:
   a tier-unaware, reviewer-free implementer floor (commit discipline with
   recoverable `## AI Context`, minimal build/test verification with full-output
   reading before a pass claim, surgical convention-following edits). It states
   that the lead's assignment is the governing contract and this is the minimum
   beneath it. Follow `skill-authoring.md`. Decide during execution whether it
   needs a `SKILL.md` shim (it is delegate-rendered, not user-invoked, so likely a
   pure rsrc render body like `ticket-worker`, with no lead-skill entry) — do not
   add it to `EXPECTED_LEAD_SKILLS`.
2. **lead-delegate** (`agents-plugin/rsrc/lead-delegate/lead-delegate.md`): in
   `## Assignment`, when the assignment writes code, render `delegate-implementer`
   and give its path to the executor as system prompt; leave tier resolution via
   `config.resolve_agent` intact and the playbook tier-unaware. Re-point the
   handoff in `## Routing` (`:33-36`) from "implementation contract for lead-run"
   to authoring a ticket via `lead-ticket`.
3. **lead-run** (`agents-plugin/rsrc/lead-run/lead-run.md`): remove every ad-hoc
   element listed in Decisions, leaving queue-drain and direct-ticket-path intact.
   Keep the terminal-line contract and one-worker-per-invocation rule unchanged.
4. **AGENTS.md**: drop "or implementation contract" from the `Direct:` canonical-flow
   line so the doc matches the collapsed surface.
5. **Tests/golden** (`agents-plugin/tests/`): remove the `run_ad_hoc` key from
   `fixtures/lead_delegate_contract.json` and its `assertIn` in
   `test_skill_dispatch_contracts.py:68`; adjust `test_run_dispatches_through_playbook_read`
   and any lead-run assertion that names the ad-hoc prose so the suite pins the
   ticket-only body. The lead-delegate edit (step 2) also breaks
   `test_delegate_and_sibling_exact_prose`'s `assertEqual(delegate,
   contract["delegate"])` (`test_skill_dispatch_contracts.py:62-63`, fixture
   `lead_delegate_contract.json:7`): regenerate the `delegate` fixture value to
   match the new lead-delegate.md body byte-for-byte. The lead-run intro edit
   breaks the `run_opening` assertion (`:67`); update that fixture value too. If
   `delegate-implementer` warrants a contract assertion, add it. Run the full
   `agents-plugin/tests` suite and confirm green by reading full output.
6. **Mirror** all of the above to `agents-plugin-wsflow/` per `wsflow-mirroring.md`,
   including the new playbook and the wsflow package's own drift/contract tests;
   run the wsflow suite and confirm green.

Verification: `agents-plugin/tests` full suite green, `agents-plugin-wsflow` suite
green, and a grep confirming no `run_ad_hoc` / ad-hoc-contract prose remains on
either shipped surface.

### Result (8db4527) - 2026-09-11

Landed the whole surface change as one commit (`8db4527`):

- **lead-run** reduced to ticket-only: removed the intro contract clause, the
  Select-skip paragraph, the tier-selection ad-hoc clause, the two `Ad hoc`
  worker rows, and the `Contract:` task-block substitution. Queue-drain,
  direct-ticket-path, the terminal-line contract, and one-worker-per-invocation
  are intact.
- **delegate-implementer** added at `agents-plugin/rsrc/delegate-implementer/`:
  a tier-unaware (no `tier:` field), reviewer-free implementer floor stating
  verify-before-pass, diagnose-before-fix, surgical convention-following edits,
  and commit-on-branch with recoverable rationale — expressed as a generic
  commit-discipline floor, not devenv's `## AI Context` heading, per
  `shipped-surface-boundary.md`. `role: implementer` so a lead render mints a
  pre-keyed delegate child.
- **lead-delegate** renders `delegate-implementer` when the assignment writes
  code and passes its path as part of the executor system prompt; the
  broader-scope handoff re-points from an implementation contract for lead-run
  to authoring a ticket via `lead-ticket`.
- **AGENTS.md** `Direct:` canonical-flow line dropped `or implementation
  contract`.
- **Tests/golden**: removed `run_ad_hoc` from the fixture and its assertion,
  regenerated the `delegate` body and `run_opening` fixture values byte-for-byte,
  added a `delegate-implementer` floor contract test. Regenerated the rsrc
  manifest and the byte-identical `agents-plugin-wsflow/` mirror.

Verification (all read to completion):
- `python3 -m unittest discover agents-plugin/tests`: 57 tests OK.
- `python3 -m unittest discover agents-plugin-wsflow/tests`: 11 tests OK.
- `go test ./...` (agents-plugin-tool): all packages ok (incl. wsrsrc drift
  guards, `TestLeadDelegateReadContract`, `TestPlaybookPrintLeadRunWorkerTierPolicy`).
- grep: no `run_ad_hoc` / retired ad-hoc lead-run route prose on either shipped
  surface; no devenv-only nouns in the three changed shipped files.
- Independent review (fresh delegate, single allocation): clean — no Critical,
  Important, or Minor findings.

Decisions taken during execution (recorded, not escalated):
- Beyond the ticket's Phase 5 test list (python golden only), two Go prose-pin
  tests also pinned the edited body and had to move with it:
  `playbook_tools_test.go` (dropped the removed `Ad hoc` rows) and
  `lead_delegate_test.go` (the new handoff prose + the floor render). The ticket
  under-named the test surface; the edits are mechanical drift-follows.
- Left the runtime `route.resolve_implement` ad-hoc-target path and
  `worker-stop-protocol.md`'s generic "a ticket, or an ad-hoc contract" worker
  identity untouched. They remain a coherent, still-reachable worker capability
  that lead-run simply no longer triggers — consistent with the ticket's
  explicit rejection of worker-layer changes. This is why the verification grep
  is scoped to the retired lead-run route prose, not every occurrence of the
  words "ad hoc".
