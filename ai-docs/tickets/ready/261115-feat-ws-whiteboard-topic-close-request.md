---
title: Topic card x button attaches a close-topic request to the composer
related:
  261115-feat-ws-dashboard-user-message-envelope: prerequisite; close requests are delivered inside its ws:user envelope (pending 261115-feat-ws-dashboard-user-message-envelope)
  261115-feat-ws-whiteboard-topic-state-badge: coordinate; both edit the topic header action row (page.tmpl, page.css, cards.js decorate order)
  261114-refactor-ws-dashboard-whiteboard-asset-split: prerequisite; consume its landed move map before touching dashboard assets (pending 261114-refactor-ws-dashboard-whiteboard-asset-split)
  261115-feat-ws-dashboard-conversation-rendering-consolidation: coordinate; the delivered close-request capsule renders in the user bubble it rewrites (pending 261115-feat-ws-dashboard-conversation-rendering-consolidation)
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 74261bee3882af99
sage-review-design-reviewed: 74261bee3882af99
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
  `.closed` class. The legacy document disclosure sits in the absolute
  top-right corner with reserved header padding only on non-framed pages;
  the dashboard's DocFramed renderer uses a plain `.tdoc` div and omits both
  the disclosure and `.has-doc` padding marker
  (`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L40-L46`,
  `agents-plugin-tool/internal/wswhiteboard/assets/page.css#L584-L586`,
  `agents-plugin-tool/internal/wswhiteboard/render.go#L717-L733`).
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

- **(892)=a, owner:** each close-request capsule carries its own optional
  reason field, like a topic note; the tag body is that capsule's reason, and
  an empty reason sends an empty body. Rejected: b (topic-only capsule, the
  composer body as the shared reason).
- **Structured `closeRequests` field, server-side serialization**
  (technical, acked by MAIN): the request carries an optional
  `closeRequests: [{topic, reason}]`; `wsinbox.Entry.Text()` serializes each as
  `<wb:close-request topic="...">reason</wb:close-request>` after mentions,
  with `wb:mention` attribute and body escaping, inside the envelope. A send
  holding only close requests (empty composer text) is valid content.
  Rejected: the browser writing the tag into the free text (precedent: commit
  `e0964b73`, mentions are structured).
- **Dedupe by topic stem** (technical, acked): pressing x on a topic already
  attached focuses that capsule's reason field instead of adding a second.
  Rejected: ignoring the press; replacing the capsule.
- **Header placement** (technical, acked; premise corrected by fact
  population): `cards.js` `decorate()` adds the x as the rightmost `.tbar`
  action, after the note pen, with a localized title and aria-label, shown
  wherever the other header actions show (pinned, folded, fullscreen; not in
  Overview one-line rows), on the dashboard only (commit `01088661`:
  `decorate` runs only there). The dashboard's framed renderer has no legacy
  document disclosure, so no corner collision applies. Rejected: an
  absolutely positioned corner button.
- **Focus without a tab switch** (technical, acked; focus target refined by
  fact population after (892)=a): pressing x attaches the capsule and focuses
  its reason field in the composer, without switching tabs, since the
  composer is present on the board tab. Rejected: switching to the
  conversation tab; focusing the composer's main text box (the reason lives
  in the capsule).
- **Stale requests are delivered** (technical, acked): a capsule whose
  topic closes or disappears before send stays attached and is delivered;
  the playbook tells the agent to treat an already-closed or unknown topic as
  a no-op it reports briefly. Rejected: silently dropping it in the browser.
- **Agent semantics** (technical, acked): the playbook's Answers section
  defines `<wb:close-request>` as the user's request to close that topic. The
  agent closes it through its normal whiteboard update path unless the topic
  still has open work it must not abandon, in which case it replies in that
  topic with why it did not close. The board-watching detection line lists
  the new tag (commit `a6b955cc` defines that signal). Rejected: asking the
  user to confirm first.
- **Delivered capsule rendering** (technical, acked): the request renders in
  the user bubble as a capsule (topic name, reason when present) through the
  existing capsule template. Rejected: showing the raw tag.
- **Compatibility tier T0** (technical, acked): an optional additive
  request and inbox field plus a new delivered tag; `Normalize` and every
  inbox rewrite copy the field, with the accepted minor-mismatch caveat of
  commit `e0964b73`. Rejected: T2.
- **Header coordination with the badge lane** (announced, acked):
  `261115-feat-ws-whiteboard-topic-state-badge` edits the same `.tbar` region
  and `decorate()` order; whichever lands second rebases onto the other and
  keeps both the badges and the x.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- No client-side topic close; the agent performs any close.
- Owner run rules: offline env `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`;
  never the full `./internal/wsdashboard` package, only `-run`; never the
  shared dashboard daemon or port 29417 (isolated daemons with their own
  `WS_CACHE_HOME` on free ports); end with an owned process scan.

## Prior Decisions

- 261114-feat-ws-dashboard-shared-card-actions (2026-11-14, commit 20e647c3): "Adopt the server-rendered topic header/body/badges in place: shared fold and raise operate on its view record, never invoke topic workflow close, and leave inner disclosure owners unchanged." — bearing: constrains
- 01088661 (2026-11-14, commit): "topic buttons are added by cards.js decorate, which only runs on the dashboard." — bearing: supports
- 562bfeb9 (2026-11-12, commit): "Message route accepts `topics`; cancel/interrupt answers return `topics` (never null)" — bearing: constrains
- e0964b73 (2026-11-12, commit): "Mentions are a structured wsinbox.Entry.Mentions field (topic, id, text) rendered into the message by Entry.Text(), not markup the browser writes into the typed text" — bearing: supports
- a6b955cc (2026-11-12, commit): "Watching signal is an OR: current message starts with the whiteboard label `[user message from the {{.McpNamespace}} whiteboard]`, or carries a `<wb:answers>` block / `<wb:mention>` elements" — bearing: constrains
- 3ddae0e1 (2026-11-10, commit): "User whiteboard (598) ok: topic notes get a composer chip with x-clear and click-to-focus; send format unchanged." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Phase 1 spans agents-plugin-tool/internal/wsdashboard/assets/cards.js and dashboard.js, wsinbox/inbox.go, conversation rendering, wsi18n/tables/*.json and agents-plugin/rsrc/whiteboard/whiteboard.md plus wsflow/Pi mirrors. Consume the pending asset-split move map before editing. |
| scope.surface | public-interface | Proposed optional closeRequests HTTP/inbox field and delivered wb:close-request tag cross the browser, daemon, conversation view and agent/playbook boundary; current request is agents-plugin-tool/internal/wsdashboard/inbox_routes.go#L82-L104. |
| scope.new_public_symbol | unknown | An optional exported Entry field is expected, but the Go field/type name is the implementer's. |
| scope.new_type_contract | yes | Proposed closeRequests: [{topic, reason}] and wb:close-request tag body carrying each capsule's reason; owner (892)=a and the settled Decisions fix the shape. |
| scope.test_surface | existing | agents-plugin-tool/internal/wsinbox/mentions_test.go, wsdashboard/cards_test.go, topicnote_chip_test.go, inbox_routes_test.go, pageview_inbox_test.go, wsi18n/keys_test.go and wsrsrc/{wsflow_mirror,pi_mirror}_test.go provide adjacent tests; no new test file is mandated. Static inspection only; no tests run. |
| complexity.reuse_points | confirmed | Read decorate/ibtn (agents-plugin-tool/internal/wsdashboard/assets/cards.js#L85-L92, #L523-L548), renderChips/sendMessage and draft read/restore (assets/dashboard.js#L1581-L1640, #L2518-L2559, #L8826-L8900), Entry.Text/MentionBlock and achips/splitUserEntries. Both whiteboard playbook mirrors compare byte-identical. |
| complexity.side_effect_risk | moderate | Header reconciliation, focus, async send completion and persisted drafts share existing state owners; serialization and steering rewrite persistent inbox entries (agents-plugin-tool/internal/wsinbox/inbox.go#L380-L382, #L692-L749). No browser-side workflow close is authorized. |
| risk.correctness | high | Empty-reason attachment-only sends must count as content, dedupe must survive draft/send/refresh races, and all normalize/cancel/steer/drain paths must preserve requests; existing Normalize constructs a fresh Entry (agents-plugin-tool/internal/wsinbox/inbox.go#L378-L382). Envelope Phase 1 is unlanded. |
| risk.fit | moderate | Reuse renderer-owned topic nodes, dashboard-only decorate and the shared server serializer; c3 assumes a neighboring document disclosure absent from the dashboard (agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L40-L46). Header-badge and asset-split coordination remains outstanding. |
| risk.test | high | Adjacent Node/Go fixtures do not prove the new attachment lifecycle, capsule reason focus or responsive header geometry; Phase 1 leaves exact focused commands and browser fixtures to promotion, and the envelope/split/conversation consumers are still pending. |
| risk.security_or_contract | high | New user-request markup can prompt workflow closure: preserve topic/reason escaping, session-bound attachments, no-op/open-work semantics and inbox rewrite fields; T0 is settled in Decisions. Existing escaping is agents-plugin-tool/internal/wsinbox/mentions.go#L43-L63. |

## Phases

### Phase 1: Header x, close-request capsule, delivery and playbook

Implement the Decisions above after the envelope ticket's Phase 1 lands. Scope:
`decorate()` button and i18n keys; capsule state with its own reason
field ((892)=a), dedupe, removal, draft
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
