---
title: Factor lead-use-mailbox "arm the wait" into a harness-aware include
related:
  260911-feat-ws-pi-harness-idiom-playbook-overlays: predecessor
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: ce50a4c1cfe3609e
sage-review-design-reviewed: ce50a4c1cfe3609e
completed: 2026-09-19
---

# Factor lead-use-mailbox "arm the wait" into a harness-aware include

## Background

The shared `lead-use-mailbox` playbook body is a single monolithic file
(`agents-plugin/rsrc/lead-use-mailbox/lead-use-mailbox.md`) mirrored
byte-identically into `agents-plugin-pi/rsrc/` and `agents-plugin-wsflow/rsrc/`.
Its `## On: arm the wait` section carries a general imperative ("whenever you
sit idle waiting on a peer, launch the wait command as a background process")
with harness carve-outs that name only Claude (background-task-completion wake)
and Codex (Stop-hook wake once a registered address exists).

On pi this guidance is redundant and misleading. The pi extension owns a
session-bound background mailbox waiter that the adapter arms automatically for
the lead/owner role and that active-pushes arriving mail into the live
conversation. A pi lead therefore never needs to arm anything, yet the mirrored
base tells it to. This is the "demonstrated shared-base delta" that the prior
overlay-policy ticket required before adding a pi harness overlay.

The fix: keep the shared body host-neutral and single-source, and factor only
the divergent `## On: arm the wait` section into a harness-aware include so each
harness's arm behavior is composed in per harness without duplicating the
unaffected sections.

## Decisions

- **Include-based split (chosen), not a top-level harness overlay.** The loader
  resolves `<name>.<harness>.md` as a *whole-file replacement* of the base
  (`resolvePlaybookPath` returns the overlay path and never reads the base),
  which would force every harness copy to duplicate all four unaffected
  sections. The compositional path is includes: `Load` appends
  `resolveIncludes` output to the body, and each include name resolves its own
  `<playbook>/<include>.<harness>.md` overlay with a base-file fallback
  (`resolveIncludePath`). That gives a host-neutral skeleton plus a per-harness
  arm section with no section duplication — the layering the user asked for.
  - Rejected: a top-level `lead-use-mailbox.pi.md` — whole-file replacement,
    duplicates the shared sections, and the mirror drift guard only checks
    pi==agents-plugin, not overlay==base, so later base edits would silently
    fail to reach the overlay.
  - Rejected: terminology substitution (`playbookTerminologyTable["pi"]`) — it
    only swaps wording (`{{.Var}}`), and this is a procedural/semantic delta it
    cannot express.

- **Host-neutral base include.** The base `arm-the-wait.md` states the default
  as a capability condition ("arm the wait only if your harness does not itself
  deliver arriving mail as a turn-starting message"), not a bare imperative, so
  any current or future push-delivery harness is covered without being named
  (Architecture Rule 3, host-neutral first). Harness names live in overlays or
  as examples, not in the default rule.

- **pi overlay is authoritative for pi.** `arm-the-wait.pi.md` is a short
  overlay stating the adapter already arms the waiter and pushes mail, so the
  lead launches nothing.

## Constraints

- Author upstream in `agents-plugin/rsrc/lead-use-mailbox/` only. The pi and
  wsflow `rsrc/` trees are byte-identical mirrors enforced by
  `TestPiMirrorUpToDate` and the wsflow mirror regen/drift tests. Never
  hand-edit the mirrored copies; regenerate them. For a non-release single
  ticket, resync with a manual rsync/cp of `agents-plugin/rsrc/` into the two
  mirror trees, not `bump-ws-version.sh` — that script is release-scoped and
  also bumps the plugin version (commit 0fa9d218). The drift guards must pass
  after the resync.
- Every new include target file must be registered in
  `agents-plugin/rsrc/manifest.json` and pass hash verification (`loadAndVerify`
  checks each loaded file against its manifest hash).
- Shipped-surface + observable-workflow change: read
  `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/skill-authoring.md` before editing (Implementation
  Conventions table + Code Standards rule 5), and
  `ai-docs/manuals/wsflow-mirroring.md` for the mirror regen path.
- The declared binding-anchor topics are `lead surface, worker interpreter,
  document-layer retirement, stop conditions` (AGENTS.md `## Workflow` ->
  `### Binding Anchor`); neither `mailbox` nor `harness wake` appears verbatim
  in that list or anywhere in the anchor ticket's body (grep of
  `ai-docs/tickets/idea/260909-research-ws-refoundation-evidence-audit.md` for
  "mailbox"/"wake" returns nothing). Read the declared anchor before
  implementing regardless, since this is lead-surface playbook content.
- Behavior parity: Claude and Codex arm-the-wait behavior must be preserved.
  Codex's nuance (auto-wake fires only once a registered named inbox exists;
  no address still requires arming) must survive the refactor; pi's wake works
  off the session key alone (named inbox additive).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)

## Prior Art

- Include-with-harness-overlay is a live pattern:
  `agents-plugin/rsrc/lead-workflow-manual/native-spawn-binding.codex.md`
  (overlays the `native-spawn-binding` include declared in
  `lead-workflow-manual.md#L3-L4`) and
  `agents-plugin/rsrc/lead-ticket/task-list.codex.md` (overlays the
  `task-list` include declared in `lead-ticket.md#L3-L4`).
  `agents-plugin/rsrc/sample-playbook/sample-playbook.codex.md` is not an
  include overlay: `sample-playbook.md` declares `includes: - sample-conventions`
  (not itself), so its `.codex.md` sibling is instead a whole-playbook
  top-level overlay — the rejected pattern above — kept as a test fixture for
  that behavior (`agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go`,
  `agents-plugin-tool/internal/mcp/playbook_tools_test.go`).
- Overlay policy origin: `.done/260911-feat-ws-pi-harness-idiom-playbook-overlays`
  ("no overlay without a demonstrated delta"); this ticket supplies that delta.
- pi auto-push mechanism: `agents-plugin-pi/src/mailbox-waiter.ts`
  (`startMailboxWaiter`, `shouldArmMailboxWaiter`) and its wiring in
  `agents-plugin-pi/src/index.ts`.

## Prior Decisions

- 260917-feat-mailbox-wait-rearm-nudge-and-concrete-command (2026-09-18, Result): "MailboxWaitCommand is a reserved implicit render variable filling {{.MailboxWaitCommand}} in lead-use-mailbox.md, resolved by mailboxWaitCommandVar" — bearing: constrains
- 260917-feat-ws-pi-mailbox-waiter-slug-wake (2026-09-18, Result): "added resolveMailboxSelfSlug (calls mailbox.lookup_peers with session_key, scope: worktree, format: json through the existing MailboxToolCall seam, returns self.address or undefined" — bearing: supports
- 0fa9d218 (2026-09-18, commit): "pi resynced by rsync copy (not bump-ws-version.sh, which would also bump the plugin version); drift guards TestPiMirrorUpToDate / TestWsflowRsrcMirrorUpToDate" — bearing: constrains
- 260913-feat-cross-session-mailbox-wake (2026-09-13, Decisions): "Harness adapters are best-effort, never the contract: Codex/Claude hook wiring and pi native push are adapter/fallback surfaces; the host-neutral contract is the CLI + listening marker" — bearing: supports
- 260913-feat-cross-session-mailbox-core (2026-09-13, Decisions): "Deferred: the blocking CLI wait --timeout + model-driven background-task wake + listening marker; the Codex/Claude/pi hook adapters; and the hook-layer arm-reminder gate. All probe-gated" — bearing: supports
- 02f98366 (2026-06-15, commit): "The ticket records the confirmed harness-aware local include fragment direction for later implementation while deferring inline conditional DSL." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-use-mailbox/lead-use-mailbox.md, a new arm-the-wait.md base include, a new arm-the-wait.pi.md overlay, agents-plugin/rsrc/manifest.json, plus the resynced agents-plugin-pi/rsrc/ and agents-plugin-wsflow/rsrc/ mirrors |
| scope.surface | internal | no exported Go/TS symbol changes; only playbook markdown content and manifest.json hash entries (agents-plugin-tool/internal/wsrsrc/loader.go unchanged) |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go already exercises resolveIncludes/resolveIncludePath harness resolution; agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go and wsflow_mirror_test.go already guard mirror byte-identity |
| complexity.reuse_points | confirmed | resolveIncludes/resolveIncludePath in agents-plugin-tool/internal/wsrsrc/loader.go and the live include-overlay precedent at agents-plugin/rsrc/lead-workflow-manual/native-spawn-binding.codex.md and agents-plugin/rsrc/lead-ticket/task-list.codex.md |
| complexity.side_effect_risk | moderate | a wrong manifest hash or mis-split include silently breaks playbook loading (loadAndVerify ErrHashMismatch) or trips the byte-identity mirror drift guards across three packages |
| risk.correctness | moderate | three harness renders (default/Claude, pi, and possibly Codex) must each compose the right arm-the-wait content while preserving Codex's registered-address-conditional nuance and the {{.MailboxWaitCommand}} substitution |
| risk.fit | low | reuses an already-established loader mechanism and prior-art overlay pattern; the split direction was already settled via lead-discuss per the commit record recorded in this ticket's own creation |
| risk.test | moderate | existing loader/mirror tests are generic (not lead-use-mailbox-specific); Phase 1's verification of pi vs default composition is described as a manual render check, not a named new automated test |
| risk.security_or_contract | low | text-only playbook content and manifest hash bookkeeping; no auth, session, or wire-contract code touched |

## Phases

### Phase 1: Split arm-the-wait into a host-neutral include with a pi overlay

Refactor `lead-use-mailbox` so `## On: arm the wait` is an include: rewrite the
base include host-neutrally (capability-conditional default), add the pi
overlay, and reduce the main body to a host-neutral skeleton plus the include
reference. Preserve register/find/send-recv/remote-control verbatim in intent.

Design choice left to the worker within this contract: whether Codex's
registered-address-conditional nuance stays inline in the base include or splits
into its own `arm-the-wait.codex.md`. Minimum viable structure is base include +
pi overlay; add the codex overlay only if the host-neutral base cannot carry the
Codex condition cleanly.

Then register the new include target(s) in `manifest.json`, regenerate the pi
and wsflow mirrors, and verify the mirror drift guards and the loader
include-resolution tests pass. Verify all three harness renders of
`lead-use-mailbox`: a pi-harness render composes the pi arm section, a
default/Claude render composes the host-neutral one, and a codex-harness render
still carries Codex's registered-address-conditional nuance intact (whether that
nuance lives inline in the base include or in its own `arm-the-wait.codex.md`).
Also confirm `{{.MailboxWaitCommand}}` substitution still resolves in the
refactored arm section.

### Result (f17a9b6) - 2026-09-19

Landed on `impl/develop/fade-snort-grief` (base `develop` @ `b55784d3`) across
two commits: `8ed4ad6` (the split) and `f17a9b6` (round-1 review fixes).

- Split `## On: arm the wait` out of
  `agents-plugin/rsrc/lead-use-mailbox/lead-use-mailbox.md` into a new
  `arm-the-wait` include (`agents-plugin/rsrc/lead-use-mailbox/arm-the-wait.md`),
  stated host-neutrally as a capability condition ("arm the wait only if your
  harness does not already deliver arriving mail to you as a turn-starting
  message"), with Claude's and Codex's existing behavior kept as in-base
  examples/nuance (minimum-viable structure: no separate `arm-the-wait.codex.md`
  was needed). Added `arm-the-wait.pi.md` stating pi's adapter already arms the
  waiter and pushes mail for the session's lead/owner role.
- Registered both new files in `manifest.json` and resynced the
  `agents-plugin-pi` and `agents-plugin-wsflow` rsrc mirrors via manual
  `rsync` (not `bump-ws-version.sh`, which is release-scoped and also bumps
  the plugin version, per commit `0fa9d218`).
- Round-1 review (partitioned: correctness + test) raised one Important each:
  a leftover host-neutral-skeleton sentence in `On: remote-control another
  session` restated Codex's registered-address-conditional nuance as a
  blanket rule, contradicting the new pi overlay for a pi target; and the
  harness-differentiated composition had zero automated regression coverage.
  Both fixed in `f17a9b6`: the remote-control step now defers to whatever
  condition the reader's own `On: arm the wait` render states, and
  `agents-plugin-tool/internal/wsrsrc/lead_use_mailbox_arm_wait_test.go` was
  added, table-driven over harness `""`/`claude`/`codex`/`pi`, asserting the
  pi overlay composes with no base leakage and the non-pi renders keep the
  Codex nuance, the Claude example, and the `{{.MailboxWaitCommand}}`
  placeholder. Also fixed two Minor findings from the same round (restored the
  general arm occasion-trigger to the shared launch step; scoped the pi
  overlay's auto-arm claim to the lead/owner role, matching
  `shouldArmMailboxWaiter`'s `role === undefined` gate in
  `agents-plugin-pi/src/mailbox-waiter.ts`). Round 2 (both partitions)
  verified the fixes with no new blocking findings.
- Decisions taken within contract: kept the Codex nuance inline in the base
  include rather than adding a separate `arm-the-wait.codex.md` — the
  host-neutral base carries the condition cleanly, satisfying the ticket's
  "minimum viable structure" option.
- Verification: `go test ./... -count=1` in `agents-plugin-tool` (all
  packages pass, including `internal/wsrsrc`'s
  `TestPiMirrorUpToDate`/`TestWsflowRsrcMirrorUpToDate`/`TestValidateRealTree`
  and the new harness-composition test); `python3 -m unittest discover
  agents-plugin-wsflow/tests` (12 tests, OK); `npm test` in
  `agents-plugin-pi` (1589 pass, 0 fail). All three harness renders
  (default/Claude, Codex, pi) of `lead-use-mailbox` were checked directly via
  `wsrsrc.Load`, now pinned by the committed regression test rather than a
  one-off manual check.
- Unresolved (observation only, not a stop): a cross-harness remote-control
  scenario (a Codex lead mailing a pi target) still reads the *reader's own*
  harness's arm-the-wait condition rather than the target's; this is
  strictly better than the pre-fix self-contradiction and was not required by
  this ticket's contract, but is left as a possible future refinement.
