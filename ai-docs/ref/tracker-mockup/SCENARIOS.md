# Tracker mockup v3: scenario walk

Board (267) a, (281) a: nine user scenarios walked through the page and the
engine rules after v3 landed. Each scenario ran in headless Chromium on a fresh
page from the `team-alpha` fixtures (clock 2026-11-06 09:00), driven by clicks
and dialog forms, with the outcome read back from the DOM and the view state.
Snags are classified:

- **view**: the page shows or offers the wrong thing; the model is fine. Small
  ones were fixed in the page during the walk.
- **model**: a rule is missing or produces a bad outcome. Recorded with a
  recommended resolution; the lead resolved all nine as board (284)-(292),
  and the follow-up applied them to the engine and page. Scenarios 2, 3, 4,
  5, 7 and 8 were walked again afterwards; each snag below ends with its
  resolution and the re-walk result.

The layout change of board (350), (352)-(360) merged My assignments, Received
requests, Assigned by me, My team, and Items into one Items panel with scope
presets, and moved the card buttons to hover actions on the rows. Scenarios 1,
2, 3, 4, 5, 8 and 10 were walked again through the new panel (2026-11-08) with
the same outcomes; their steps below name the new places. A row action is
reached by hovering the row (or focusing it, or tapping it on touch) and
clicking its icon.

## 1. Morning check

Steps: view as `maya.pc`; read the Today card; accept ivy's pending review
from its row there (Accept); confirm ivy's close of Tokenizer from its row
there (Confirm).

Outcome: Today lists my open work by priority then due ("Prepare Friday demo"
긴급, due today; "Ship search v2" 긴급, 11-20; "Security training" 알림, 11-30),
the pending assignment waiting for my answer (ivy's "Review the emoji
tokenizer fix", assigner confirms), and the close waiting for my confirm
(ivy on Tokenizer). Accept made the review a live assignment that keeps ivy
as assigner (she confirms), and it moved into my open work; Confirm moved
Tokenizer to done. Both actions dispatch from the Today rows.

Snags:

- V1 (view, fixed): an item due today looked like any future due date. Dues
  equal to the clock's day now show "due today".

## 2. A lead finds stuck work

Steps: view as `maya.pc`, `raj.pc`, then `ada.pc`; read the Items panel's
*my team* scope.

Outcome: each follower's group header counts open, blocked, stale, and
unaccepted work.
maya sees lena's unaccepted request from noa, omar's blocked "Shard writer
docs" (omar's late commit, the display winner over her reject), and ivy's
stale "Security training" (doing since 11-02). raj sees sven, tia and noa
stale and pia blocked on "Cache layer". ada, the root, sees all of it down to
L2.

Snags:

- V2 (view, fixed): a follower's own pending assignments to others that
  nobody accepted were invisible to their lead (noa's request to lena did not
  show on raj's view). My team now shows "N waiting on others" and the rows
  (today in the follower's group of the *my team* scope).
- M1 (model): "stale" was never defined on the board. The mockup uses "live,
  doing or blocked, no status write for 3 days"; a comment by the assignee
  does not reset it, and todo work never goes stale (otherwise every
  "everyone within L2" training floods the lead view). Recommendation: settle
  this definition and keep the threshold a client-side constant (a view
  concern, not shared data).
  **Resolved (284)** as recommended: `STALE_DAYS` in the engine, overridable
  per view with `deriveViewState(input, {staleDays})`. Re-walk: same counts.

## 3. Accepted, returned by the accepter's lead, sent again

Steps: as `lena.pc`, accept noa's pending "Explain the tokenizer API" from its
row in the *received requests* scope; as `maya.pc` (lena's lead), read the
reference notice on the Notifications card, then Return with a note from the
item's row in lena's group of the *my team* scope; as `noa.pc`, open the item
and assign it to lena again.

Outcome: on accept, maya's Notifications card shows "Lena Weiss accepted an
assignment from Noa Fisher (you are their lead)", the row in lena's group
offers Return, and maya's handle badge counts it. The return ends lena's assignment as `returned` with maya's note;
the item status is `returned` and it leaves every roll-up. noa's new
`item.assign` is valid (nothing live or pending to lena remains) and creates a
fresh pending assignment; the returned one stays in history.

Snags:

- V3 (view, fixed): the "sent by me" heading on Received requests said
  "pending until accepted" above returned and declined rows.
- M2 (model): the sender is never notified that their pending assignment was
  declined or their accepted one returned. noa's badge and notices stay
  empty; she learns only by opening the card. Notices exist only for the
  accepter's lead (on accept) and the original assigner (on an accepted
  handoff). Recommendation: add a derived notice to the sender on decline and
  on return (no new event type).
  **Resolved (285)**: derived `declined` / `returned` notices (keys `dec:` /
  `ret:`). Re-walk: after maya's return, noa's badge holds
  `ret:noa.pc#4>lena` and her *received requests* scope opens with an
  "answers to what I sent" row whose details read "Maya Ortiz (lead of Lena
  Weiss) returned Lena Weiss's accepted assignment" and the note. The fixture shows the decline side: tia has `dec:tia.pc#4>uma`.

## 4. A member leaves

Steps: as `ada.pc` (admin), try to take over omar's open work (maya's "Shard
writer docs", ada's "Security training"), then remove omar with
`roles.remove_member`, then look at his assignments again.

Outcome (first walk, before (286), (287)): before removal ada has no way to move omar's work: unassign belongs
to the assigner (maya), and (208) lets an admin unassign only after the
assignee or assigner was removed; a handoff belongs to omar. Removal is valid
(omar leads nobody and is not an admin). After it, omar's two live
assignments are still live: "Shard writer docs" keeps omar's status in its
item status and its parent's roll-up, and "Security training" can never be
all done. ada can now unassign them (admin, member removed); reassigning
"Shard writer docs" is maya's, its creator.

Snags:

- M3 (model): an admin cannot take over a member's open work before removing
  them, so the scenario's order "take over, then remove" is impossible; only
  "remove, then take over" works. Recommendation: allow an admin
  `item.handoff` on any member's live assignment (a pending assignment to the
  new person; on accept the member's assignment ends), so takeover happens
  while the member is still around and the new holder accepts explicitly.
  The alternative, settling the order as remove-then-unassign, needs no new
  rule but leaves work unowned in between.
  **Resolved (286)**: `item.handoff` takes an `assignee`, allowed for an
  admin on any member's live assignment. Re-walk: ada opens "Shard writer
  docs", clicks *Hand off (admin)* on omar's row, hands it to jon; jon's
  *received requests* row shows "admin handoff of Omar Haddad's assignment by
  Ada Park (assigner Maya Ortiz)" in its details; jon accepts from the row and
  starts at omar's `blocked`
  (290); omar's assignment is `handed`; then ada removes omar. The scenario's
  order now works.
- M4 (model): removing a member leaves their live and pending assignments in
  place, still counted in item status and roll-ups. Recommendation: in the
  fold, `roles.remove_member` ends the member's live assignments as
  `unassigned` and pending ones as `withdrawn`, attributed to the removal
  event; history stays.
  **Resolved (287)** as recommended. Re-walk: after removal omar's remaining
  "Security training" assignment is `unassigned` by `ada.pc-16` (the sheet
  says "member removed"), and no live assignment of omar is left counted.
  Without the takeover, "Shard writer docs" becomes `unassigned` and maya
  (its creator) reassigns it.
- V4 (view, fixed): the assignees table offered no Unassign to an admin on a
  removed member's assignment, although the engine allows it (208).

## 5. An agent auto-updates status; the human sees and corrects it

Steps: as `lena.pc`, set "Index builder" to blocked through the status dialog
(the row's Status action in the *mine* scope) with origin `agent-auto`; see it
in the row's details ("set by Lena Weiss's agent") and in the thread; correct
it from the thread composer (doing, with a note).

Outcome: the agent's write is valid (agent-auto on the author's own
assignment) and shows `agent` / `auto` chips in the thread; the correction is
a later human write on the same register and wins; both stay in the thread.
The parent shows "blocked below" while the agent's status stands.

Snags:

- V5 (view, fixed): cards showed an agent-written status like a human one
  (only the note author). Assignment rows now carry a "by agent" chip, from
  the view state's `statusOrigin` (today "set by ...'s agent" in the row's
  details).
- M5 (model): when the agent writes from one clone and the human corrects
  from another clone of the same member without having synced, (271)
  applies: the later ts wins with no marker. Checked in the engine: a human
  `doing` on `maya.lap` and a later agent-auto `blocked` on `maya.pc` end as
  `blocked`, silently. Recommendation: keep (271) for writes of the same
  origin, but when concurrent same-member writes differ in origin (human vs
  agent), mark them like a cross-member conflict (or let the human write win).
  **Resolved (288)**: the human write wins; same origin keeps (271). A new
  fixture case exercises it: maya's human `doing` on "Ship search v2" from
  `maya.pc` at 17:00 beats her agent-auto `blocked` from `maya.lap` at 18:00.
  Re-walk: maya's assignment shows `doing` (human), and `maya.lap-4` carries
  "superseded by maya.pc-21 (a human write from another clone)" (289).

## 6. Offline for days on one clone, then sync

Steps: as `maya.lap`, Go offline; reject ivy's close of Tokenizer; advance the
clock a day and comment; advance another day; as `ivy.pc` (online), set
Tokenizer to blocked (which withdraws her close) and sync; back on
`maya.lap`, Come back online.

Outcome: while offline, lap's writes sit in its open batch (shown in the store
header's unpushed list, each event with its own write time two days apart)
and are invisible to ivy. On reconnect the batch closes into one commit, is
pushed, and lap fetches ivy's write. maya's reject and ivy's blocked are
concurrent writes to ivy's status by two members: both get a "concurrent
change" box, and ivy's later ts is the display winner (`blocked`).

Snags: none. The reject losing to ivy's later status is the settled (203)
rule, and a status write while awaiting already withdraws the close (202).

## 7. One member edits the same body from two clones

Steps: as `maya.lap`, Go offline and edit the body of "Shard writer docs"; an
hour later, as `maya.pc`, edit the same body and sync; back on `maya.lap`,
come online; then sync `maya.pc` again.

Outcome: both clones converge on the desktop edit (the later ts). Both edits
stay in the thread, and neither has a marker, as (271) settled.

Snags:

- M6 (model): the laptop edit loses silently. The thread shows it, but nothing
  says it is not the shown text, so a person who edited on the laptop sees
  their change "vanish" after sync. A clone with a fast clock would also
  always win (inherent to ts winners, accepted by (203)). Recommendation:
  keep no conflict box, but show a quiet "superseded by maya.pc-21 (later
  edit from another clone)" note on the losing edit line.
  **Resolved (289)** as recommended: events carry `superseded[{field, by,
  why}]`, and the thread shows the note. Re-walk: the laptop edit line reads
  "superseded by maya.pc-22 (a later write from another clone): body of
  maya.pc-7"; the fixture's `maya.lap-1` has the same note.

## 8. An assignee hands off to a peer; the peer accepts

Steps: as `lena.pc`, Hand off "Index builder" to ivy with a note (the row's
Hand off action); as `ivy.pc`, accept it from its row in the *received
requests* scope; as `maya.pc`, read the Notifications card and ivy's group in
the *my team* scope.

Outcome: until ivy accepts, lena still holds her assignment (doing) and ivy's
is pending, shown in ivy's *received requests* scope and badge as a handoff
from lena with maya as assigner. On accept lena's assignment ends as `handed` and
ivy's is live with maya still the assigner (confirm and unassign stay with
her). maya, ivy's lead and the original assigner, gets one notice ("Ivy Chen
accepted a handoff from Lena Weiss") on the Notifications card, and the row in
ivy's group of the *my team* scope offers Return.

Snags:

- M7 (model): the handed-on assignment starts at `todo` with no note: the
  item's status drops from doing to todo, and lena's progress note stays only
  on the ended record. Recommendation: the new assignment starts with the
  original's status and note (or the handoff note).
  **Resolved (290)**: status and note carry over. Re-walk: ivy's accepted
  assignment is `doing` with lena's note, and the item stays `doing`.
- M8 (model): if the accepter's lead returns an accepted handoff, nobody holds
  the work. Checked in the engine: lena `handed`, ivy `returned`, item status
  `returned` with no live assignee. Recommendation: returning an accepted
  handoff restores the original assignment (`handed` back to `live`), or at
  least notifies the original assigner (ties to M2).
  **Resolved (291)**: both. Re-walk: maya returns ivy's accepted handoff from
  her My team notice (today: the row in ivy's group of the *my team* scope);
  lena's assignment is live again at `doing` (restored), the item stays
  `doing`, and lena's Received card (today: the *received requests* scope)
  says "Lena Weiss's assignment is restored". Checked in the engine: when the original
  assigner is not the returning lead (raj's handoff to sven, returned by
  raj), the assigner lena gets the `ret:` notice and raj's assignment is
  restored. No restore happens when the original holder was removed or
  holds another assignment on the item by then.

## 9. A typo in a team member's item

Steps: as `maya.pc`, open raj's "Cache layer" and ivy's "Review the emoji
tokenizer fix"; comment on ivy's with the correction; as `ivy.pc`, edit the
body.

Outcome: maya gets no Edit action on items she did not create (only Accept /
Decline on ivy's, where she is the pending target); her comment lands in the
thread; ivy's edit applies. The fixture's `maya.pc-10` shows the rule from
the other side: maya's edit of ada's item is flagged "only the creator (ada)
edits".

Snags:

- M9 (model): if the creator was removed, nobody can fix the item's text.
  (208) lets an admin assign and unassign on a removed member's work, but
  `item.edit` stays creator-only (checked: an admin's edit after removing the
  creator is flagged). Recommendation: extend (208) to `item.edit` and
  `item.hide` by an admin when the creator was removed.
  **Resolved (292)** as recommended. Checked in the engine: after removing
  noa, ada's `item.edit` of `noa.pc-3` applies (a non-admin's is still
  flagged); `item.hide` still needs the open assignments ended first. The
  sheet offers *Edit (admin: creator removed)* and *Hide item (admin)*.

## 10. Notifications, a mention, and an attachment

Added with the notification card ((331)-(340)), walked the same way.

Steps: view as `maya.pc`; read the Notifications card; click the mention
entry for "Retro notes: search v2 sprint"; *Mark all read*; open "Index
builder", load lena's attachment; post a comment that picks `@ivy` and
attaches a 20000000-byte file, then one with a 2048-byte file; view as `ivy`.

Outcome: maya's card lists ten entries, all unread, newest first: due today,
two mentions (bo's topic body, lena's comment), a confirm, a pending
assignment, a reply, completion and blocked of work she assigned, a
reference notice, and a forced assignment. The click opened the thread sheet
and cleared that entry only; *Mark all read* cleared the rest, and with them
the unread dots on the Items panel's rows. In the thread
`lena.pc-6` shows `@Maya Ortiz` / `@Omar Haddad` chips and an
`index-layout-v2.png` chip that toggled from "not loaded" to "loaded";
`lena.pc-7` shows its 50 MiB attachment struck through with the over-limit
reason. The first post was flagged with `attachment "huge.zip" is 20000000
bytes, above the store limit of 10485760 bytes (attachments.max_bytes)`; the
second applied, and ivy's card then shows it as a mention.

Snags: none.

## Summary

| # | scenario | outcome (after the resolutions) | view snags (fixed) | model snags (resolved) |
|---|---|---|---|---|
| 1 | morning check | works | V1 due today | - |
| 2 | lead finds stuck work | works | V2 follower's unaccepted outgoing work | M1 |
| 3 | accept, return, send again | works; the sender is notified | V3 "sent by me" heading | M2 |
| 4 | member leaves | works in the scenario's order: admin handoff, then removal | V4 admin Unassign | M3, M4 |
| 5 | agent update, human correction | works, also across clones | V5 "by agent" chip | M5 |
| 6 | offline days, then sync | works | - | - |
| 7 | same body from two clones | works; the losing edit is marked superseded | - | M6 |
| 8 | handoff to a peer | works; status carries over, a returned handoff restores the original | - | M7, M8 |
| 9 | typo in a member's item | works | - | M9 |
| 10 | notifications, mention, attachment | works | - | - |

| model snag | recommended resolution | board resolution |
|---|---|---|
| M1 stale is undefined | Settle "live, doing or blocked, no status write for 3 days"; threshold a client constant. | (284) as recommended |
| M2 sender not notified of decline or return | Derived notice to the sender on decline and on return; no new event. | (285) as recommended |
| M3 admin cannot take over before removal | Allow an admin `item.handoff` on any member's live assignment (pending until the new holder accepts). | (286) as recommended |
| M4 removal leaves assignments live and counted | `roles.remove_member` ends the member's live (`unassigned`) and pending (`withdrawn`) assignments in the fold. | (287) as recommended |
| M5 same-member agent write silently beats a human correction | Keep (271) for same-origin writes; mark (or prefer the human) when origins differ. | (288) the human write wins |
| M6 a member's losing clone edit vanishes silently | Keep no conflict box; add a quiet "superseded by ..." note on the losing line. | (289) as recommended |
| M7 handoff resets status to todo | Start the handed-on assignment with the original's status and note. | (290) as recommended |
| M8 returning an accepted handoff orphans the work | Restore the original assignment on return, or notify the original assigner. | (291) both |
| M9 nobody can edit a removed creator's item | Extend (208) to `item.edit` / `item.hide` by an admin when the creator was removed. | (292) as recommended |

The six interpretations v3 made where the plan was silent are accepted as
(293); see the README Settled table.
