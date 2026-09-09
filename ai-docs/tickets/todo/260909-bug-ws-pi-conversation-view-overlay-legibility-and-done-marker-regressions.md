---
title: "Pi adapter: conversation-view / `/answer` overlay legibility defects plus `/done` summary-injection and `working…` marker regressions surfaced by owner-live acceptance"
related:
  260908-feat-ws-pi-conversation-view-component: "origin — its Phase 2 migration of the ask overlay onto the shared ConversationViewComponent surfaced these defects; its own owner-live `.done/` gate depends on this fix landing and being re-verified"
  260904-feat-ws-pi-side-thread-fork-question-surface: "owns the `/done` summary-injection behavior that F1 regresses (already in .done/)"
  260905-feat-ws-pi-overlay-activity-indicator-and-esc-hint: "owns the `working…` activity marker and its states that F2 regresses (already in .done/)"
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-design-reviewed: 94835e2f47c51a37
---

# Pi adapter: conversation-view / `/answer` overlay legibility defects plus `/done` summary-injection and `working…` marker regressions surfaced by owner-live acceptance

## Background

The owner-live acceptance run of `260908-feat-ws-pi-conversation-view-component`
(recorded 2026-09-09, commit `a7fb3d7`, that ticket's
`### Owner-live acceptance attempt (2026-09-09)` section) failed. The run
exercised the shared conversation-view component through the `/answer` overlay
and exposed two classes of defect in that component and its overlay chrome. The
owner decided to capture the fixes here as a separate, immediately-actionable
bug (not folded back into `260908` as Editions) so they can run through a tight
human-in-loop fix/verify loop while `260908` stays blocked in `ready/` pending
this fix and a re-run of its runbook.

The functional regressions (F1, F2) are of behavior that `260904` and `260905`
verified passing before Phase 2 of `260908` migrated the ask overlay onto the
shared component — i.e. migration regressions, not new features.

## Decisions

Confirmed defects the owner observed live:

Legibility / overlay chrome:

- **V1.** The overlay/modal renders with no padding and no border, so it does not
  separate from the lead's background. Render a border around the overlay.
- **V2.** The left and right columns need roughly one character of horizontal
  margin.
- **V3.** The assistant (model) dialogue is not visually distinguished at all;
  give it a distinguishing treatment so model turns read apart from the rest.
- **V4.** The text shown first when the `/answer` window opens looks
  unfinished / placeholder-quality; clean up the initial-view text presentation.
- Overall: legibility is poor and the surface reads as under-polished; the
  fixes above are the concrete, owner-named items, not an open-ended redesign.

Behavioral regressions:

- **F1.** `/done` on a **`lead-ask` (owner-opened discussion) thread** no longer
  injects its summary into the lead transcript. In the live run `/done` closed
  the overlay but the lead received no summary injection. This is the `260904`
  lead-ask `/done` behavior (summary turn + `ws-thread-summary` injection).
  Scope guard: the **`fork-raised` `/done`** route deliberately performs no
  injection and no stop — restore injection only on the lead-ask route and leave
  fork-raised behavior unchanged.
- **F2.** The `working…` activity marker renders in the wrong location — not at
  the end of the agent dialogue, so it is easy to miss — and it is not shown
  immediately before tool output. This is the `260905` working-marker behavior.
  In the same run the **idle-awaiting-owner** and **settled** state renderings
  were left unverified; they must be confirmed once F2 is addressed.

Rejected alternative: folding these into `260908` as `#### Edition` entries on
its frozen phases. The owner chose a standalone bug ticket for a faster,
independently reviewable loop.

## Constraints

- Pi-extension code only (`agents-plugin-pi/src`, `conversation-view.ts` /
  `ask.ts` and the overlay chrome). Authored on this Pi track
  (`track/pi-agent`); this is not ws-mcp / `agents-plugin-tool/` code, so there
  is no cross-track `develop` authoring obligation.
- Honor repo gotcha `gotcha.pi-tui-dual-package`: resolve `pi-tui` through the
  host at runtime (guarded dynamic import / `loadHostPiTui`), static import only
  for types and tests — otherwise the host-`tui` `instanceof` binding breaks.
- Human-only surfaces only, per `260908` constraints: overlay content and any
  chrome never route through `pi.sendMessage`.

## Spec Impact

Target spec area: `pi-adapter-runtime`, the conversation-view / overlay-chat
surface (the component anchor and the `{#260905-…}` overlay-chat `/done`
entry).

- F1 and F2 restore behavior the spec already describes (lead-ask `/done`
  `ws-thread-summary` injection; the `working…` marker and its
  idle-awaiting-owner / settled states) — no spec change intended.
- V1–V3 add overlay chrome the current component anchor does not describe
  (overlay border, ~1-char left/right column margin, assistant-turn visual
  distinction); update that anchor's rendering description on contact when this
  lands. V4 is presentation polish with no contract change.

## Phases

### Phase 1: Fix overlay legibility and restore `/done` / `working…` behavior

Address V1–V4 and F1–F2 on the shared conversation-view component and the
`/answer` overlay in one slice; independent increments on one surface with no
sequential dependency between them.

Verification, two tiers:

- **Structural, unit-locked** in `test/conversation-view.test.ts` (and
  `test/ask.test.ts` where the overlay binding lives), via the existing
  `render(width)` test style: overlay border present; ~1-char left/right column
  margin; assistant rows carry the distinguishing treatment; the `working…`
  marker sits at the end of the agent dialogue and appears immediately before
  tool output; and the `/done` path invokes the summary injection into the lead
  (F1 is functionally testable, not eyeball-only).
- **Owner eyeball, live TUI** for the aesthetic "feel" and for the live
  `idle-awaiting-owner` / `settled` / marker rendering that no render test
  fully captures — run as the tight loop (agent edits + unit tests →
  build + `/reload` → owner observes → feedback), not the full `260908`
  owner-live runbook.

Exit: once the structural tests are green and the owner confirms the live feel,
re-run `260908`'s owner-live runbook items (its item 2 `/done` summary
injection, item 3 activity-indicator states, plus items 1 and 4) to clear that
ticket's `.done/` gate.
