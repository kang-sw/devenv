---
title: Factor lead-use-mailbox "arm the wait" into a harness-aware include
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
  `TestPiMirrorUpToDate` and the wsflow mirror regen/drift tests, and resynced
  by `bump-ws-version.sh`. Never hand-edit the mirrored copies; regenerate them.
- Every new include target file must be registered in
  `agents-plugin/rsrc/manifest.json` and pass hash verification (`loadAndVerify`
  checks each loaded file against its manifest hash).
- Shipped-surface + observable-workflow change: read
  `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/skill-authoring.md` before editing (Implementation
  Conventions table + Code Standards rule 5), and
  `ai-docs/manuals/wsflow-mirroring.md` for the mirror regen path.
- Binding-anchor topics (mailbox / harness wake) are touched: read the declared
  anchor before implementing.
- Behavior parity: Claude and Codex arm-the-wait behavior must be preserved.
  Codex's nuance (auto-wake fires only once a registered named inbox exists;
  no address still requires arming) must survive the refactor; pi's wake works
  off the session key alone (named inbox additive).

## Prior Art

- Include-with-harness-overlay is a live pattern:
  `agents-plugin/rsrc/lead-workflow-manual/native-spawn-binding.codex.md` and
  the `.codex.md` include overlays under `lead-ticket/` and `sample-playbook/`.
- Overlay policy origin: `.done/260911-feat-ws-pi-harness-idiom-playbook-overlays`
  ("no overlay without a demonstrated delta"); this ticket supplies that delta.
- pi auto-push mechanism: `agents-plugin-pi/src/mailbox-waiter.ts`
  (`startMailboxWaiter`, `shouldArmMailboxWaiter`) and its wiring in
  `agents-plugin-pi/src/index.ts`.

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
include-resolution tests pass. Verify a pi-harness render of `lead-use-mailbox`
composes the pi arm section and a default/Claude render composes the
host-neutral one.
