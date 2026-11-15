---
title: Topic card x button attaches a close-topic request to the composer
related:
  261115-feat-ws-dashboard-user-message-envelope: prerequisite; close requests are delivered inside its ws:user envelope
  261115-feat-ws-whiteboard-topic-state-badge: coordinate; both edit the topic header action row (page.tmpl, page.css, cards.js decorate order)
  261114-refactor-ws-dashboard-whiteboard-asset-split: prerequisite; consume its landed move map before touching dashboard assets
  261115-feat-ws-dashboard-conversation-rendering-consolidation: coordinate; the delivered close-request capsule renders in the user bubble it rewrites
---

# Topic card x button attaches a close-topic request to the composer

## Background

Owner request (relayed by MAIN, 2026-11-15): an "x" button at the top right
of each whiteboard topic card. Pressing it attaches a "close topic" request
to the chat composer the way a note or mention attaches, with no duplicates
per send; several topics can be added before sending. The agent reads it as
"user requested to close this topic: ...". Pressing it opens the composer so
the user can add an optional reason.

Today (develop `883762d19`, paths under `agents-plugin-tool/internal/`):
- Topic header: `wswhiteboard/assets/page.tmpl` ~46 renders
  `section.topic[data-topic]` > `.tophead` > `.tbar` (name, age, freshness,
  answer/ack chips, settled controls, `.tnote-toggle`); a closed topic has the
  `.closed` class. The legacy document disclosure already sits in the
  absolute top-right corner with reserved header padding
  (`wswhiteboard/assets/page.css` ~580-586).
- `wsdashboard/assets/cards.js` `decorate()` (~523-543) inserts the pin,
  fold, raise and maximize/restore buttons before the note pen and re-applies
  them after board replacement; `ibtn()` (~85-92) sets title and aria-label.
  Topics deliberately have no generic close operation (~577-578).
- Composer attachments (`wsdashboard/assets/dashboard.js`): item chips in an
  id-keyed map, board mentions in an ordered list (~1530-1539), capsules with
  x removal (`renderChips()` ~1602-1638, removal ~2914-2917), mentions
  appended without dedupe (`addMention()` ~2456-2469, which also focuses
  `ctext`). `sendMessage()` posts structured `{text, chips, topics, mentions}`
  and keeps unsent content on failure (~2518-2559). The composer stays
  present on the board tab (`dashboard.css` ~934-943).
- Serialization is server side in `wsinbox.Entry.Text()` (`wsinbox/inbox.go`
  ~331-362), e.g. `<wb:mention topic="ship-gate" id="03" entry="e47">...</wb:mention>`
  (`wsinbox/mentions.go` ~43-63, attribute and body escaping). Unknown JSON
  request fields are ignored (`wsdashboard/inbox_routes.go` ~82-87).
- The shipped whiteboard playbook defines delivered tags in `## Answers`
  (`agents-plugin/rsrc/whiteboard/whiteboard.md` ~400-450; board-watching
  detection ~389); the wsflow copy is byte-identical and generated
  (`ai-docs/manuals/wsflow-mirroring.md`), and the Pi package carries a copy.
- i18n tables `wsi18n/tables/*.json`, enforced by
  `TestEveryUsedKeyIsInTheEnglishTable` and
  `TestOtherTablesHoldEveryEnglishKey` (`wsi18n/keys_test.go`).

## Decisions

- **Owner request, as relayed:** x at the top right of each topic card;
  attaches a close request to the composer like a note or mention; no
  duplicates per send; several topics per send; opens the composer for an
  optional reason.
- **MAIN technical calls (announced, accepted):** delivered as
  `<wb:close-request topic="<stem>">reason</wb:close-request>` inside the
  `<ws:user>` envelope; the whiteboard playbook's Answers section defines it
  (shipped playbook edit under skill-authoring and wsflow mirroring); the x is
  hidden on closed topics; a request is removable before send; no client-side
  close.

## Open Decision Queue
Last ID: none minted (items below await MAIN's board IDs)

(c1) [open] [technical] Close requests travel as a structured optional request field (`closeRequests: [{topic, reason}]`) serialized to `<wb:close-request>` by `wsinbox.Entry.Text()` after mentions, with the same attribute/body escaping as `wb:mention`, over the browser writing the tag into the free text - matches how mentions and topic notes travel, keeps one serializer, and lets held/draft state and the conversation view treat it as an attachment.
(c2) [open] [technical] Dedupe is keyed by topic stem: pressing x on a topic already attached focuses its capsule (and its reason input, if any) instead of adding a second, over silently ignoring the press or replacing it - the owner asked for no duplicates, and focusing keeps the press visibly answered.
(c3) [open] [technical] The x is added by `cards.js` `decorate()` as the rightmost header action, beside the document disclosure without overlapping it, with a localized title/aria-label, shown wherever the other header actions show (pinned, folded, fullscreen; not inside Overview one-line rows), dashboard only (the standalone whiteboard page has no composer), over an absolutely positioned corner button - reuses the action row's reconciliation after board replacement and stays coherent with the badge lane's header ordering.
(c4) [open] [technical] Pressing x focuses the composer text box without switching tabs, over switching to the conversation tab - the composer is present on the board tab, so a tab switch would move the user away from the board they are closing topics on.
(c5) [open] [technical] A capsule whose topic becomes closed or disappears before send stays attached and is delivered; the playbook tells the agent to treat an already-closed or unknown topic as a no-op it reports briefly, over silently dropping it in the browser - nothing the user attached vanishes, and the agent already sees the board state.
(c6) [open] [technical] The playbook defines `<wb:close-request>` as the user's request to close that topic: the agent closes it through its normal whiteboard update path unless the topic still has open work it must not abandon, in which case it replies in that topic with the reason it did not close; the board-watching detection line (~389) lists the new tag, over a request the agent must confirm back first - the user already decided by pressing x, and the reply covers the exception.
(c7) [open] [technical] The delivered request renders in the user bubble as a capsule (topic name, reason when present) through the conversation view's existing capsule template, over showing the raw tag - consistent with mention capsules; the renderer work lands wherever the rendering-consolidation ticket has left the bubble template.
(c8) [open] [technical] Compatibility tier T0: an optional additive request/inbox field and a new delivered tag; dashboard assets ship with the daemon; an older agent with an older playbook still sees self-describing text, over T2 - no existing field, argument or event changes. The worker audits that inbox rewrites preserve the new field.
(c9) [open] [announced] Header edits coordinate with `261115-feat-ws-whiteboard-topic-state-badge` (EXEC1's lane, ready): whichever lands second rebases onto the other's header markup and keeps both the badges and the x - both tickets edit the same `.tbar` region and `decorate()` order, which the badge ticket requires to keep existing header actions.
(892) [open] [policy] Where the optional close reason is written.
- Owner item minted by MAIN, 2026-11-15.
- a: each close-request capsule has its own small reason field; the tag body carries that capsule's reason.
- b: the capsule carries only the topic; the composer's normal text body is the shared reason for every request in that send, and tags are sent empty.
> MAIN's recommendation: a.

## Constraints

- No client-side topic close; the agent performs any close.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/release-compatibility.md`.
- Owner run rules: offline env `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`;
  never the full `./internal/wsdashboard` package, only `-run`; never the
  shared dashboard daemon or port 29417 (isolated daemons with their own
  `WS_CACHE_HOME` on free ports); end with an owned process scan.

## Phases

### Phase 1: Header x, close-request capsule, delivery and playbook

Pending the queue above and the envelope ticket's Phase 1. Expected scope:
`decorate()` button and i18n keys; capsule state, dedupe, removal, draft
persistence and send field in `dashboard.js`; optional inbox field and
`Entry.Text()` serialization; bubble capsule; playbook Answers definition in
the ws, wsflow and Pi copies.

Verification (to be finalized at promotion):
- `wsinbox` tests: field normalization, escaping, serialization order inside
  the envelope, rewrite preservation.
- Dashboard JS tests: x present on open topics and absent on `.closed`;
  pressing twice yields one capsule and focuses it; several topics attach;
  removal before send; unsent requests survive a failed send and a reload
  draft.
- Isolated Chromium check: the x sits at the header's right end without
  overlapping the document disclosure or the badge row; screenshot kept.
- i18n completeness tests; playbook mirror tests.
