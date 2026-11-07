# Tracker mockup v3: scenario walk

Board (267) a, (281) a: nine user scenarios walked through the page and the
engine rules after v3 landed. Each scenario ran in headless Chromium on a fresh
page from the `team-alpha` fixtures (clock 2026-11-06 09:00), driven by clicks
and dialog forms, with the outcome read back from the DOM and the view state.
Snags are classified:

- **view**: the page shows or offers the wrong thing; the model is fine. Small
  ones were fixed in the page during the walk.
- **model**: a rule is missing or produces a bad outcome. Not changed here;
  each comes with a recommended resolution for the lead to decide.

## 1. Morning check

Steps: view as `maya.pc`; read the Today card; accept ivy's pending review
from it; confirm ivy's close of Tokenizer from it.

Outcome: Today lists my open work by priority then due ("Prepare Friday demo"
긴급, due today; "Ship search v2" 긴급, 11-20; "Security training" 알림, 11-30),
the pending assignment waiting for my answer (ivy's "Review the emoji
tokenizer fix", assigner confirms), and the close waiting for my confirm
(ivy on Tokenizer). Accept made the review a live assignment that keeps ivy
as assigner (she confirms), and it moved into my open work; Confirm moved
Tokenizer to done. Both buttons dispatch from the card.

Snags:

- V1 (view, fixed): an item due today looked like any future due date. Dues
  equal to the clock's day now show "due today".

## 2. A lead finds stuck work

Steps: view as `maya.pc`, `raj.pc`, then `ada.pc`; read My team.

Outcome: each follower row counts open, blocked, stale, and unaccepted work.
maya sees lena's unaccepted request from noa, omar's blocked "Shard writer
docs" (omar's late commit, the display winner over her reject), and ivy's
stale "Security training" (doing since 11-02). raj sees sven, tia and noa
stale and pia blocked on "Cache layer". ada, the root, sees all of it down to
L2.

Snags:

- V2 (view, fixed): a follower's own pending assignments to others that
  nobody accepted were invisible to their lead (noa's request to lena did not
  show on raj's view). My team now shows "N waiting on others" and the rows.
- M1 (model): "stale" was never defined on the board. The mockup uses "live,
  doing or blocked, no status write for 3 days"; a comment by the assignee
  does not reset it, and todo work never goes stale (otherwise every
  "everyone within L2" training floods the lead view). Recommendation: settle
  this definition and keep the threshold a client-side constant (a view
  concern, not shared data).

## 3. Accepted, returned by the accepter's lead, sent again

Steps: as `lena.pc`, accept noa's pending "Explain the tokenizer API" from
Received requests; as `maya.pc` (lena's lead), open the reference notice in My
team and Return with a note; as `noa.pc`, open the item and assign it to lena
again.

Outcome: on accept, maya's My team shows "Lena Weiss accepted an assignment
from Noa Fisher (you are their lead)" with Return, and maya's handle badge
counts it. The return ends lena's assignment as `returned` with maya's note;
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

## 4. A member leaves

Steps: as `ada.pc` (admin), try to take over omar's open work (maya's "Shard
writer docs", ada's "Security training"), then remove omar with
`roles.remove_member`, then look at his assignments again.

Outcome: before removal ada has no way to move omar's work: unassign belongs
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
- M4 (model): removing a member leaves their live and pending assignments in
  place, still counted in item status and roll-ups. Recommendation: in the
  fold, `roles.remove_member` ends the member's live assignments as
  `unassigned` and pending ones as `withdrawn`, attributed to the removal
  event; history stays.
- V4 (view, fixed): the assignees table offered no Unassign to an admin on a
  removed member's assignment, although the engine allows it (208).

## 5. An agent auto-updates status; the human sees and corrects it

Steps: as `lena.pc`, set "Index builder" to blocked through the status dialog
with origin `agent-auto`; see it on My assignments and in the thread; correct
it from the thread composer (doing, with a note).

Outcome: the agent's write is valid (agent-auto on the author's own
assignment) and shows `agent` / `auto` chips in the thread; the correction is
a later human write on the same register and wins; both stay in the thread.
The parent shows "blocked below" while the agent's status stands.

Snags:

- V5 (view, fixed): cards showed an agent-written status like a human one
  (only the note author). Assignment rows now carry a "by agent" chip, from
  the view state's `statusOrigin`.
- M5 (model): when the agent writes from one clone and the human corrects
  from another clone of the same member without having synced, (271)
  applies: the later ts wins with no marker. Checked in the engine: a human
  `doing` on `maya.lap` and a later agent-auto `blocked` on `maya.pc` end as
  `blocked`, silently. Recommendation: keep (271) for writes of the same
  origin, but when concurrent same-member writes differ in origin (human vs
  agent), mark them like a cross-member conflict (or let the human write win).

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

## 8. An assignee hands off to a peer; the peer accepts

Steps: as `lena.pc`, Hand off "Index builder" to ivy with a note; as `ivy.pc`,
accept it from Received requests; as `maya.pc`, read My team.

Outcome: until ivy accepts, lena still holds her assignment (doing) and ivy's
is pending, shown on ivy's Received card and badge as a handoff from lena
with maya as assigner. On accept lena's assignment ends as `handed` and
ivy's is live with maya still the assigner (confirm and unassign stay with
her). maya, ivy's lead and the original assigner, gets one notice ("Ivy Chen
accepted a handoff from Lena Weiss") with Return.

Snags:

- M7 (model): the handed-on assignment starts at `todo` with no note: the
  item's status drops from doing to todo, and lena's progress note stays only
  on the ended record. Recommendation: the new assignment starts with the
  original's status and note (or the handoff note).
- M8 (model): if the accepter's lead returns an accepted handoff, nobody holds
  the work. Checked in the engine: lena `handed`, ivy `returned`, item status
  `returned` with no live assignee. Recommendation: returning an accepted
  handoff restores the original assignment (`handed` back to `live`), or at
  least notifies the original assigner (ties to M2).

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

## Summary

| # | scenario | outcome | view snags (fixed) | model snags |
|---|---|---|---|---|
| 1 | morning check | works | V1 due today | - |
| 2 | lead finds stuck work | works | V2 follower's unaccepted outgoing work | M1 stale definition |
| 3 | accept, return, send again | works | V3 "sent by me" heading | M2 sender not notified |
| 4 | member leaves | order impossible; removal leaves work counted | V4 admin Unassign | M3 takeover before removal, M4 removal ends assignments |
| 5 | agent update, human correction | works | V5 "by agent" chip | M5 same-member agent vs human |
| 6 | offline days, then sync | works | - | - |
| 7 | same body from two clones | works as settled | - | M6 silent loss |
| 8 | handoff to a peer | works | - | M7 status reset, M8 returned handoff orphans work |
| 9 | typo in a member's item | works | - | M9 removed creator's text |

| model snag | recommended resolution |
|---|---|
| M1 stale is undefined | Settle "live, doing or blocked, no status write for 3 days"; threshold a client constant. |
| M2 sender not notified of decline or return | Derived notice to the sender on decline and on return; no new event. |
| M3 admin cannot take over before removal | Allow an admin `item.handoff` on any member's live assignment (pending until the new holder accepts). |
| M4 removal leaves assignments live and counted | `roles.remove_member` ends the member's live (`unassigned`) and pending (`withdrawn`) assignments in the fold. |
| M5 same-member agent write silently beats a human correction | Keep (271) for same-origin writes; mark (or prefer the human) when origins differ. |
| M6 a member's losing clone edit vanishes silently | Keep no conflict box; add a quiet "superseded by ..." note on the losing line. |
| M7 handoff resets status to todo | Start the handed-on assignment with the original's status and note. |
| M8 returning an accepted handoff orphans the work | Restore the original assignment on return, or notify the original assigner. |
| M9 nobody can edit a removed creator's item | Extend (208) to `item.edit` / `item.hide` by an admin when the creator was removed. |
