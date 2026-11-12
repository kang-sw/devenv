---
title: "Whiteboard `check` items: a confirmation gate rendered as a checklist under the recap, and a status-only recap"
related:
  261030-feat-session-whiteboard-mode: whiteboard store and playbook this extends
---

# Whiteboard `check` items

## Background

User, 2026-11-12 (whiteboard topic `recap-ask`): leads sometimes write
"waiting for the user's feedback" into a topic recap instead of adding an
item, and the user misses it. Two causes were found in
`agents-plugin/rsrc/whiteboard/whiteboard.md` (topic recap bullet):

- The recap bullet lists "waiting on an answer" among the states that get a
  recap, which invites a recap that waits on the user, contradicting the
  "never asks the user anything" sentence after it (added 716e6f6ba).
- A plain confirmation ("OK this draft and I'll proceed", "verify on your
  machine") has no natural class: the bullet files it under `announced` or
  `technical`, but those classes mean "the lead's call, with a citation or a
  technical reason", and a confirmation is the user's judgment. Filed there,
  it is acknowledged in bulk with the defaults (and, in a ticket queue, a
  review-raised announced item is auto-confirmed), so the gate loses its
  weight.

Mockup: `ai-docs/ref/whiteboard-mockup/r6-check-items/` (`index.html`,
`index.png`, `staged.png`; merged 1aa6c97ff). Its notes panel records the
placement, staging, count, and answer-grammar rules below.

## Decisions

1. **New item class `check` (user, (570) a; (572) a).** A `check` is a gate,
   not a decision: a request that the user look at, verify, or approve
   something. It sits beside `policy`, `technical`, and `announced` in the
   whiteboard store and the `wb:add` / `wb:patch` `class` attribute. It is
   for whiteboard and discussion asks only; a ticket's Open Decision Queue
   keeps its three classes. Rejected: widening `announced` to cover
   confirmations ((570) b: bulk acknowledgement and auto-confirmation would
   sweep gates up); filing confirmations as `policy` with proceed / revise
   options ((570) c: heavy for a light check).
2. **Rendering follows the mockup (user, (572) a).**
   - Placement: recap, then the topic's check list, then the topic body,
     then the policy and announced/technical items.
   - The list uses the ack list's recessed surface with a small "check"
     header and an "answered n / m" progress count; rows carry no class tag.
   - Row: checkbox (answers `ok`), a small ✕ button (answers `no`), the
     existing note pen, the `(nn)` ID, the one-line title, link chips.
     An answered `ok` row is locked, ticked, and stamped `ok`; an answered
     `no` row shows a red ✕, a `no` stamp, and the user's note quoted below.
   - Answers stage into the existing reply bar in the existing answer-block
     grammar (`571=ok`, `572=no^note`, a note alone `=^note`); nothing new
     is sent except through the existing send/copy flow.
   - Open checks count in the existing amber "answer needed" chip of the
     topic bar and page header; answered checks leave the count. The ack
     chip and every bulk acknowledge control (topic bar bulk button, reply
     bar all-ok box) skip checks, so each gate is answered one at a time.
     Rejected: a new chip colour; counting checks under the ack chip.
3. **Recap is a status snapshot only (user, (569) ok).** The playbook's
   recap bullet drops "waiting on an answer" from the states that get a
   recap and says plainly that anything waiting on the user is an item; the
   plain-confirmation example now names `check` instead of "`announced` or
   `technical`".
4. **The workflow manual's `### Asking the user` gains `check`** as a fourth
   kind of ask, described as a gate rather than a decision class, scoped to
   whiteboard and discussion asks (user, (572) a option text).

5. **Compatibility tier T0 (technical, (578)).** The class is an additive
   attribute value validated only at write (`store.go` `classValues` via
   `wsxmlcmd.EnumAttr`); an older binary reading a stored `check` falls into
   its non-policy branch and renders it as an ack row, degraded but without
   error, so no `schema_version` bump. Rejected: T2 with a schema bump.
6. **Render in the shared assets (announced, (579)).** `check` is rendered in
   the shared whiteboard page template and `page.css`
   (`agents-plugin-tool/internal/wswhiteboard/assets/`), so the file page,
   the embed snapshot, and the dashboard (which embeds those assets) all show
   it; the mockup found the dashboard only embeds the wswhiteboard assets.
7. **Query lists checks like any item (technical, (580)).**
   `whiteboard.query` prints a `check` item like any other item (ID, status,
   class `check`, title; `summary` shows its answer). Rejected: a separate
   section - one listing rule keeps query output and filters unchanged.
8. **Mirrors (announced, (581)).** Mirror the playbook and convention edits
   to wsflow and Pi rsrc and regenerate manifests as ebf89cb07 did
   (`ai-docs/manuals/wsflow-mirroring.md`).

## Constraints

- Read `ai-docs/manuals/skill-authoring.md`,
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/wsflow-mirroring.md`, and
  `ai-docs/manuals/release-compatibility.md` before editing.
- `impl/develop/whiteboard-watching-output` (in flight, 2026-11-12) edits
  the same playbook's `## Each turn` and `whiteboardManualMandate` in
  `agents-plugin-tool/internal/mcp/whiteboard_text.go`; rebase onto develop
  after it merges rather than editing those lines here.

## Phases

### Phase 1: `check` class, rendering, and playbook text

- Store and commands: add `ClassCheck`; `wb:add` / `wb:patch` accept it;
  answers are `ok`, `no`, or short text, with an optional note, like the
  ack classes; options on a `check` are refused as on other non-policy
  classes.
- View and page: render per Decision 2 in the shared page template, CSS,
  and answer staging JS; waiting counts and bulk-ack exclusion per
  Decision 2.
- Query per Decision 7.
- Playbook (`whiteboard.md`): `class` bullet lists `check` with its meaning;
  recap bullet per Decision 3. Workflow manual `### Asking the user` per
  Decision 4 (find its source under `agents-plugin-tool/internal/`).
- Mirrors and manifests per Decision 8.

Verification:
- Go tests for store validation (class accepted, options refused, answers
  `ok`/`no`/note), query output, view counts (open checks in the
  answer-needed count, excluded from ack counts and bulk ack).
- Headless Chromium over a rendered board matching the mockup's four row
  states, staging `ok`/`no`/note into the reply bar, and the bulk ack
  control leaving checks unticked; no page errors.
- Existing whiteboard, mcp, and wsrsrc test suites.
