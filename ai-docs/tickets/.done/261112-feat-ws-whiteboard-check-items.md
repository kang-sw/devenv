---
title: "Whiteboard `check` items: a confirmation gate rendered as a checklist under the recap, and a status-only recap"
related:
  261030-feat-session-whiteboard-mode: whiteboard store and playbook this extends
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 2e094e752c6e07aa
sage-review-completeness-reviewed: 2e094e752c6e07aa
completed: 2026-11-12
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
   - The topic bar's amber chip is also the policy bulk button
     (`data-bulk="policy"`, "pick every recommendation"; `form.js`
     `bulkAnswer`, `count_chips.go` `bulkCountChip`). Its count includes
     open checks, but a click stages recommendations for policy items only;
     a topic whose open asks are all checks renders the chip as a plain
     count, not a button.
   - Check rows do not reuse the `label.ack` markup, which the all-ok box
     handler and `syncAll` in `form.js` select page-wide. Answer staging
     lives in `form.js`.
3. **Recap is a status snapshot only (user, (569) ok).** The playbook's
   recap bullet drops "waiting on an answer" from the states that get a
   recap and says plainly that anything waiting on the user is an item; the
   plain-confirmation example is changed to name `check` instead of "`announced` or
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
8. **Mirrors (announced, (581)).** Mirror the playbook and skill edits
   to wsflow and Pi rsrc and regenerate manifests as ebf89cb07 did
   (`ai-docs/manuals/wsflow-mirroring.md`).

9. **Dashboard session and rail chips count open checks (design review,
   auto-confirmed).** The dashboard's session rows, rail rows, and rail
   total take their amber chip from `State.OpenPolicyCount()` (`store.go`;
   `mcp/dashboard_sessions.go`, `wsdashboard` `index.go` /
   `pageview_rail.go`, the srow/rrow/r-rail chips in `dashboard.tmpl`),
   whose doc says it matches the full page header. It counts open checks
   too (renamed if the name no longer fits), so a session waiting only on
   checks still shows the amber chip. Rejected: counting checks only on the
   board page - the session list is where a missed ask is noticed.
10. **Answered checks stay in the check list (design review,
    auto-confirmed).** Per the mockup, an answered (`ok` / `no`) or assumed
    check stays in its topic's check list, locked and stamped, and does not
    also appear in the settled list; "answered n / m" counts m = open,
    answered, and assumed checks. A withdrawn or superseded check leaves the
    check list and the count and goes to the settled list like any other
    item. In a closed topic the check list collapses with the topic's
    settled items; the embed layout shows the same list as the full page.
    The new list is handled at every site that reads the policy and ack
    lists: `quiet()`, `Answerable`, the waiting sort, `bumpRank` (a check
    must not rank as an ack) in `view.go`, and the count / `ItemOf` ranges
    in `render.go`.
11. **A check takes `wb:why` (design review, auto-confirmed).** Optional
    context, shown inline under the row as an open technical or announced
    item shows its `wb:why`. Rejected: refusing `wb:why` on a check - a gate
    sometimes needs one line of what to look at.

## Constraints

- Read `ai-docs/manuals/skill-authoring.md`,
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/wsflow-mirroring.md`, and
  `ai-docs/manuals/release-compatibility.md` before editing.
- `impl/develop/whiteboard-watching-output` merged into develop (edd6dfe69):
  it rewrote the playbook's `## Each turn` output rule and
  `whiteboardManualMandate` in
  `agents-plugin-tool/internal/mcp/whiteboard_text.go`. Build on that text and
  leave those lines as they are.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/rsrc/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Decisions

- 0ac1e9e31 (2026-11-12, commit): "User on whiteboard topic recap-ask: leads write 'waiting for feedback' into recaps instead of adding items; (569) ok, (570) a, (572) a chose a new `check` class rendered as a checklist under the recap" — bearing: supports
- 9180b2ff (2026-11-12, commit): "`check` is a gate for 'look at / confirm this', kept apart from announced/technical acks." (r6 mockup; staging uses the existing reply bar, no new send flow) — bearing: supports
- 716e6f6b (2026-11-12, commit): "Class mapping follows the workflow manual's `### Asking the user`: announced or technical for a plain confirmation, policy for a choice." — bearing: supports
- ee5baba9 (2026-11-12, commit): "asked that the whiteboard playbook state a recap is not a place to ask for confirmation (dogfood: the lead had put a confirm-and-proceed request in a recap/body instead of an item)" — bearing: constrains
- 261109-bug-ws-whiteboard-patch-cannot-change-item-class (2026-11-09, ticket Decisions): "`class` only on open items: `wb:patch` accepts `class` only on an item that is open, or that the same patch sets `status=\"open\"`; anywhere else it is refused." — bearing: constrains
- 57ba56f1 (2026-11-09, ticket Result): "The update `info:` line warns ('Options on a non-policy item: (nn) [class] ...') when an add or patch leaves an item holding options under a class other than policy." — bearing: constrains
- 261111-feat-ws-whiteboard-topic-recap (2026-11-11, ticket Decisions): "(296) a confirmed by the user's approval of the board recap mockup: recap renders by the item block-field rule with no hard length cap (Decision 6)." — bearing: supports
- f98d8e83 (2026-11-04, commit): "The inner payload schema has one source: refusals quote the rendered whiteboard playbook's ## Payload section instead of a Go copy, so schema text cannot drift from the playbook." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wswhiteboard store.go, commands/view/query/render, assets/page.tmpl, page.css, form.js; wsdashboard chips; lead-discuss; rsrc whiteboard.md and lead-workflow-manual.md in agents-plugin, agents-plugin-wsflow, agents-plugin-pi; manifests |
| scope.surface | cross-module | wswhiteboard store, the shared page assets (also embedded by the dashboard), mcp playbook text, and three rsrc trees |
| scope.new_public_symbol | yes | ClassCheck beside ClassPolicy, ClassTechnical, ClassAnnounced (store.go#L28-L30) |
| scope.new_type_contract | no | no new type or signature; an additive value in classValues (store.go#L60) for the wb:add and wb:patch class attribute |
| scope.test_surface | existing | wswhiteboard store_test.go, query_test.go, view_test.go, answers_script_test.go, render_test.go exist; the headless Chromium check may add a file |
| complexity.reuse_points | confirmed | ack row, ack list, answer staging in form.js, and reply bar read; ClassPolicy branches at view.go#L423 and view.go#L552 are the split points |
| complexity.side_effect_risk | moderate | waiting and ack counts (view.go#L552-L558) and bulk-ack controls must exclude checks without changing policy or ack behaviour |
| risk.correctness | moderate | count and bulk-ack exclusion logic across view, page template, and JS staging |
| risk.fit | moderate | byte-identical rsrc mirrors and regenerated manifests across three packages; the playbook is also edited by an in-flight branch |
| risk.test | moderate | rendering and staging need a headless Chromium check beyond Go unit tests |
| risk.security_or_contract | moderate | T0 per Decision 5, but a stored check read by an older binary degrades to an ack row, and the class enum is a tool-visible contract |

## Phases

### Phase 1: `check` class, rendering, and playbook text

- Store and commands: add `ClassCheck`; `wb:add` / `wb:patch` accept it;
  answers are `ok`, `no`, or short text, with an optional note, like the
  ack classes; options on a `check` get the same handling as on the other
  non-policy classes (kept, with the update result's `info:` warning,
  `noteNonPolicyOptions` in `store.go`).
- View and page: render per Decision 2 in the shared page template, CSS,
  and answer staging JS; waiting counts and bulk-ack exclusion per
  Decision 2.
- Query per Decision 7.
- Playbook (`whiteboard.md`): `class` bullet lists `check` with its meaning;
  recap bullet per Decision 3. Workflow manual `### Asking the user` per
  Decision 4 (its source is `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`, `### Asking the user` at L47, mirrored in `agents-plugin-wsflow/rsrc/lead-workflow-manual/` and `agents-plugin-pi/rsrc/lead-workflow-manual/`; no copy exists under `agents-plugin-tool/internal/`, whose only reference is a test, `mcp/playbook_tools_test.go#L2241`).
- Other shipped text that lists the classes: the playbook's `wb:answer`
  bullet (answers for a check are `ok`, `no`, or short text) and `wb:why`
  bullets (Decision 11), and `lead-discuss` (its discussion-ask class list
  gains `check`, per Decision 4), with their mirrors.
- Dashboard chips per Decision 9; answered-check routing per Decision 10.
- Mirrors and manifests per Decision 8.

Verification:
- Go tests for store validation (class accepted, answers `ok`/`no`/note,
  options on a check draw the non-policy options warning), query output, view counts (open checks in the
  answer-needed count, excluded from ack counts and bulk ack), the
  dashboard session/rail count including open checks, answered checks
  staying in the check list and out of the settled list, and a
  checks-only topic's amber chip rendering as a plain count.
- Headless Chromium over a rendered board matching the mockup's four row
  states, staging `ok`/`no`/note into the reply bar, and the bulk ack
  control leaving checks unticked; no page errors.
- Existing whiteboard, mcp, and wsrsrc test suites.

### Result (f8a44350f) - 2026-11-12

Landed in 71d882a02 (shipped text), 003958695 (implementation), and
f8a44350f (review fixes).

- Store: `ClassCheck` is accepted by `wb:add` / `wb:patch` with the ack-class
  answers and the non-policy options warning. `OpenPolicyCount` was renamed
  `OpenAskCount` and counts open checks; the dashboard session, rail, and
  index chips read it. The dashboard's `CountPolicy` fields keep their names
  and only their docs changed.
- View and page: each topic has a `Checks` list (open, answered, and assumed
  checks, in add order). It renders under the recap with a "check" header
  and an "answered n / m" count. Answered rows are locked and stamped: `ok`,
  or `no` with a red x and the user's words. Assumed rows keep a pen.
  Withdrawn and superseded checks settle. A closed topic folds the list into
  its settled details. `recentSettled` skips check-list checks. Open checks
  rank with policy asks (`bumpRank`) and join the amber count in the topic
  bar, page header, and `Regions.CountPolicy`. The ack chip, the all-ok box,
  and the bulk ack button skip checks, and a checks-only topic's amber chip
  is a plain count.
- Markup: check rows are `div.crow` / `label.ck`, not `.arow` / `label.ack`.
  Open and assumed rows carry `data-id`, and assumed rows also carry
  `data-assumed`.
- Scripts: form.js stages `ok` (box), `no` (x toggle), and notes. The
  dashboard's answer carry-over, composer chips, and tab-label count
  (`TL_ASK`) read check rows, and so does pill.js. Three i18n keys were added
  in every table.
- Stamp: `ok` or `no` stamps only when the word stands alone or is followed
  by punctuation. Any other answer shows as text on a ticked row.
- Query: a check lists like any item. No change was needed.
- Shipped text: the playbook's `class`, `wb:answer`, `wb:why`, and recap
  bullets; the workflow manual's `### Asking the user` Check bullet; and
  lead-discuss. All are mirrored to wsflow and Pi, and the manifests were
  regenerated.
- Decision: the manual scopes Check to "asks outside a ticket's Open
  Decision Queue" rather than naming the whiteboard, since
  `TestWhiteboardWorkflowTextFollowsKnob` forbids whiteboard text in the
  manual when the knob is off.
- Decision: the embed size budget test moves from 40% to 42% of 512 KiB
  (the stress embed measures 215 KB).

Verification:
- `go test ./... -count=1` passes, except
  `cmd/ws-mcp TestHostedConnectionSeeingALANChangeReplacesTheDaemon`. That
  test expects "master" in git status and gets "main" on this machine, which
  is unrelated to this change.
- New tests: `check_test.go` (store, query, view, counts, chips, row states,
  closed fold, rank, stamp rule), `check_form_test.go` (node:
  answers.js+form.js staging over the rendered board), and
  `wsdashboard/check_answers_test.go` (node: board-swap carry-over).
- `python3 -m unittest discover agents-plugin-wsflow/tests` is OK.
- Headless Chromium (cached Playwright) ran over a four-state board, full
  page and embed. Staging produced `03=ok`, `03=no^note`, the all-ok box
  left the check unticked, and the assumed note produced `06=^note`. There
  were no page errors.
- Lite review: one Important finding (no client-side test) and two Minor
  findings (`no problem` read as no; no free-text render test). All three
  were fixed in f8a44350f.
