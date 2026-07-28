# Plan: 260727-chore-merge-ws-dashboard-dev-into-goal-branch — Phase 5: route the inherited spec debt to its owning ticket

## Relevant Ticket Contract

- Deliverable (Phase 5 text): append a spec-impact note to
  `ai-docs/tickets/idea/260725-feat-dashboard-graceful-shutdown-from-settings.md`
  recording that its shipped behavior — the Advanced settings section,
  `build-info`, `shutdown`, `kill-all` — has no spec anchor, and that
  `{#260722-ws-dashboard-settings-panel}` still documents only two registered
  sections.
- Instrument escape hatch (Phase 5 text, verbatim): "If its status or shape
  makes an appended note the wrong instrument, open an `idea/` ticket instead
  and say why in the Result."
- Hard non-goal (Phase 5 text): "Do not author the spec text; that is that
  ticket's own work, against behavior its authors know and this one does not."
- Verification boundary (Phase 5 text): "the note exists on a ticket that
  resolves on this branch, names all four undocumented surfaces, and does not
  invent spec text."
- **Deviation this plan makes from the phase text, and why (must be flagged
  in the Result):** the phase text also says to record "a callback token
  stops being accepted once its terminal is closed by any path, including
  kill-all." This is false. Verified directly against
  `ws-dashboard/crates/daemon/src/terminal.rs`: three removal paths discharge
  BOTH the attention obligation (`attention.forget`) and the callback-token
  obligation (`forget_token`) — `remove` (lines 667–680, explicit close),
  `remove_for_work_roots` (lines 690–716, owning workRoot/workspace removal),
  and `drain_all` (lines 747–757, kill-all sweep, fixed in this ticket's own
  Phase 3). A fourth path, `insert`'s eviction-on-cap `sessions.retain` (lines
  569–611), discharges only `attention.forget` (line 595 and 610) and never
  calls `forget_token` — confirmed by reading the full `insert` body, no
  `forget_token` call appears anywhere in it. That gap is deliberately
  deferred, tracked by
  `ai-docs/tickets/idea/260728-bug-dashboard-terminal-eviction-leaks-callback-token.md`,
  and explicitly out of scope for this ticket (its own Constraints: "Do not
  fold a fix into `260727-...`"). The note must therefore state the
  **three-path** scope, not "any path". This also matches what the spec
  itself already says — see the verified spec text below, which enumerates
  exactly three closes, not "any path".

## Out of Scope

- Authoring `260725-feat-dashboard-graceful-shutdown-from-settings`'s own
  `## Spec Impact`-driven spec text (its future work, not this phase's).
- Fixing `insert`'s eviction-path token leak (owned by
  `260728-bug-dashboard-terminal-eviction-leaks-callback-token`, explicitly
  deferred there).
- Any edit to `ai-docs/spec/ws-web-dashboard/index.md` (already correctly
  states the three-path token-revocation rule; nothing to change there).
- Any edit to ticket status/directory (target ticket stays in `idea/`).
- Phase 1–4 material and any future phase — this plan covers Phase 5 only.

## Codebase Findings

- `ai-docs/tickets/idea/260725-feat-dashboard-graceful-shutdown-from-settings.md`
  — target ticket. Sections present: `## Background`, `## Decisions`,
  `## Constraints`, `## Open questions`, `## Implementation notes (dogfood
  v1, 2026-07-25)`, `## Prior Art`. No `## Spec Impact` or equivalent section
  exists yet. It is the ticket `d1d6bb31` (the dev-side Advanced-panel commit)
  actually landed against — confirmed by its own "Implementation notes"
  section describing exactly `build-info`, `shutdown`, and
  `terminals/kill-all`. It is in `idea/`, which per this repo's ticket
  convention uses "freeform topic sections and no phases" — appending a new
  freeform section is consistent with that convention; there is no phase
  structure to conflict with and no existing Spec Impact section to collide
  with.
- `ws-dashboard/crates/daemon/src/router.rs:157` — `GET
  /api/dashboard/build-info` → `dashboard_build_info`.
- `ws-dashboard/crates/daemon/src/router.rs:159` — `POST
  /api/dashboard/shutdown` → `dashboard_shutdown`.
- `ws-dashboard/crates/daemon/src/router.rs:440-443` — `POST
  /api/dashboard/terminals/kill-all` → `crate::terminal::close_all_terminals`.
- `ws-dashboard/frontend/src/settingsSections.tsx:403-410` —
  `SETTINGS_SECTIONS` registry; the third entry is `{ id: "advanced", title:
  "Advanced", Component: AdvancedSection }`. Confirmed section name is
  "Advanced" (`id: "advanced"`).
- `ai-docs/spec/ws-web-dashboard/index.md:1053-1169`
  (`{#260722-ws-dashboard-settings-panel}`) — verified content: the panel
  documents exactly two registered sections, "Terminal style" (first,
  line 1093) and "Notifications" (second, line 1134), plus one `[!note]
  Planned 🚧` hotkey-rebind section that is explicitly "not specified
  further here" (lines 1163-1168) — i.e. not a third registered section.
  **The phase text's claim is accurate, not stale**: the anchor still
  documents only two registered sections; no mention anywhere of Advanced,
  build-info, shutdown, or kill-all under this anchor.
- `ai-docs/spec/ws-web-dashboard/index.md:36-59`
  (`{#260516-ws-web-dashboard-token-free-pairing-landing}`) — verified
  content, lines 54-58: "That per-terminal callback token is revoked
  whenever its terminal closes through explicit close, owning
  workRoot/workspace removal, or a kill-all sweep that tears down every
  terminal at once. A hook process that still POSTs against the token
  afterward is rejected rather than authorized, because the token no longer
  resolves to any terminal." This is the **already-landed** Phase 3
  spec sentence (per that phase's own Result: "The spec gained a
  callback-token revocation sentence... scoped to the same three close
  paths"). It already states three enumerated paths, matching the
  code-verified scope above — nothing here needs correction, but it is the
  wording model the new note in Phase 5 should mirror rather than "any
  path".
- `ai-docs/tickets/idea/260728-bug-dashboard-terminal-eviction-leaks-callback-token.md`
  — confirms the fourth-path (eviction) token-leak gap, its provenance, and
  that it is deliberately out of scope for `260727-...`.

## Implementation Plan

1. Open
   `ai-docs/tickets/idea/260725-feat-dashboard-graceful-shutdown-from-settings.md`
   and append one new top-level section at the end of the file, after
   `## Prior Art`:

   ```markdown
   ## Spec Impact

   This ticket's shipped v1 behavior (`## Implementation notes (dogfood v1,
   2026-07-25)`) has no spec anchor. Routed here from
   `260727-chore-merge-ws-dashboard-dev-into-goal-branch` Phase 5, which
   carried this behavior onto the goal branch by merge and deliberately did
   not author spec text on its behalf.

   Undocumented surfaces (all four, verified against the merged source at
   merge time):

   - The **Advanced** settings section (`id: "advanced"`,
     `ws-dashboard/frontend/src/settingsSections.tsx`).
   - `GET /api/dashboard/build-info`.
   - `POST /api/dashboard/shutdown`.
   - `POST /api/dashboard/terminals/kill-all`.

   `{#260722-ws-dashboard-settings-panel}` in
   `ai-docs/spec/ws-web-dashboard/index.md` documents exactly two registered
   sections (Terminal style, Notifications) plus one planned, not-yet-registered
   hotkey-rebind section; it does not mention Advanced, build-info, shutdown,
   or kill-all.

   Related, and already resolved elsewhere — not part of this gap: the
   `kill-all` endpoint's callback-token contract is now spec'd. A callback
   token stops being accepted once its terminal is closed via **any of three
   paths**: `remove` (explicit close), `remove_for_work_roots` (owning
   workRoot/workspace removal), or `drain_all` (kill-all) — see
   `{#260516-ws-web-dashboard-token-free-pairing-landing}` in
   `ai-docs/spec/ws-web-dashboard/index.md`. A fourth path (`insert`'s
   eviction-on-cap retain) does not revoke the token; that is a known,
   separately tracked gap
   (`260728-bug-dashboard-terminal-eviction-leaks-callback-token`), not part
   of this ticket's undocumented-surface count.

   Writing the spec text for the four surfaces above is this ticket's own
   future work, against behavior its authors know and this one does not —
   not authored here.
   ```

2. Do not touch any other section of the target ticket, and do not create or
   move any ticket file. Do not edit
   `ai-docs/spec/ws-web-dashboard/index.md` or any source file.

## Verification Plan

- Manual read-back of the appended section against the three checkable
  conditions the phase's own verification boundary states:
  - The note exists on `260725-feat-dashboard-graceful-shutdown-from-settings.md`,
    a ticket that resolves on this branch (it is present in `idea/` on this
    branch's tree — confirmed by the `find` used during survey).
  - It names all four undocumented surfaces: Advanced section, `build-info`,
    `shutdown`, `kill-all` — all four appear verbatim, endpoint paths and
    section id/title matching the verified source (router.rs:157,159,442;
    settingsSections.tsx:403-410).
  - It does not invent spec text: the note only states that a gap exists,
    lists what is in it, and points at the `{#260722-...}` anchor and the
    already-landed `{#260516-...}` sentence; it contains no proposed spec
    prose for the four undocumented surfaces themselves.
  - It states the token-revocation scope as three explicit paths (`remove`,
    `remove_for_work_roots`, `drain_all`), not "any path", and cross-references
    the known fourth-path exception ticket.
- No automated test suite applies to a markdown-only ticket edit; this is a
  manual-only verification.

## Escalations

- None.
