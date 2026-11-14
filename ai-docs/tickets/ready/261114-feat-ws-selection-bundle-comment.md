---
title: "Selection bundles: keep each push's mentions and memo together"
related:
  261114-feat-ws-dashboard-selection-mention-note: "follow-up to source integrated at ec23205525e240bf3d3da2cf36ea3fd4642b2211; supersedes its loose-note presentation and delivery choice, not its historical acceptance evidence"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1ba1f8224ef30ee9
sage-review-completeness-reviewed: 1ba1f8224ef30ee9
---

# Selection bundles with one associated comment

## Background

The owner found that the C670 memo was appended to ordinary composer text, visually separate from the selected-source mention capsules. One selection attachment can contain several entry-specific mentions, so a loose memo makes ownership ambiguous when several attachments share a draft or message.

MAIN relayed the owner's confirmed (772)=b on 2026-11-14: each composer push creates one `wb:selection` block containing that push's mention tag(s) and exactly one `wb:comment` memo child. A cross-line drag may yield several mentions inside the same selection block. Tags and their memo must remain visibly associated. This ticket captures that requested follow-up; it does not reopen or claim completion of C670's retained browser gaps.

## Decisions

1. **One push, one selection, one comment.** Preserve ordered, entry-specific source mentions and one memo belonging to the whole push, not a memo copied onto every excerpt. The confirmed logical shape is:

   ```xml
   <wb:selection>
   <wb:mention topic="example" entry="e1">First selected excerpt</wb:mention>
   <wb:mention topic="example" entry="e2">Second selected excerpt</wb:mention>
   <wb:comment>One memo for this complete selection</wb:comment>
   </wb:selection>
   ```

   Each mention retains the existing topic/item/immutable-entry provenance and selected text. The memo is not quoted-source text. The example illustrates the confirmed grouping, not an additional optional-attribute or escaping grammar decision.
2. **Keep the memo attached visually and through delivery.** Composer attachment, draft restoration, send, and history display must retain each push's mention/comment association. The memo no longer becomes loose composer text. Ordinary independently typed user text remains ordinary text. Rejected by (772): preserving the C670 loose-text memo representation merely to avoid redesigning the tags.
3. **Attach first; send separately.** Preserve the existing explicit composer send action and the original C670 selection capture/provenance protections. Creating a selection bundle does not send it or retarget its captured source.
4. **Compatibility target: T0 additive.** Add `wb:selection` and `wb:comment` support while preserving legacy transcripts without `wb:selection`, including their current `wb:mention` and ordinary-text rendering. This is a target, not a claim of proven mixed-version compatibility. No removal of legacy syntax, non-additive store migration, protocol-number change, or concrete T2/minor bump is authorized. Return an incompatible transport/store/parser requirement to MAIN before implementing it.
5. **Explain the new message form in shipped lead guidance.** An installed session obtains playbooks from its own installed plugin, so a daemon-only change is insufficient. Shipped guidance must explain the selection boundary, source mentions, and the associated user memo without treating quoted source as agent instructions. Mirror applicable ws/wsflow surfaces according to the repository manuals.
6. **Hand off accepted backlog to EXEC3.** MAIN's 2026-11-14 14:01:42 rebalance transfers this not-yet-started follow-up from EXEC1 to EXEC3 and requests docs integration so it can be picked up. The initial capture remains separate from implementation and heavy-slot allocation. MAIN's same rebalance assigns ready promotion and implementation to EXEC3; EXEC1 performs no further implementation on this item.

7. **Support image paste and drop in the mention memo popup.** MAIN relayed the owner's explicit image-paste request on 2026-11-14, accompanied by a screenshot of the existing memo textarea and Mention button. MAIN explicitly instructed capture of (779)'s concrete default in the 13:53:05 settlement relay, then relayed the owner's `(779)=ok` at 13:54:14: reuse existing composer attachment storage and size limits, show a thumbnail in the popup, and reference the image inside that push's `wb:selection` beside its `wb:comment`. Ownership is per selection, like the memo. No new storage shape is authorized. The screenshot identifies the target control, not an approved replacement layout.
8. **Reuse existing image markers, not a new image protocol.** Static grounding established `[Image #N]` references and `Image #N: <absolute path>` delivery lines, with existing upload/reference fields and image-number remapping. Preserve selection ownership through merge and cancel/resend without rewriting marker-looking selected source. No new image tag, upload route or storage field is authorized. Return any additional protocol/API/store requirement to MAIN before implementing it. Authority: MAIN's (779) settlement and (791)/(792) follow-up.
9. **Use existing text storage and transport.** MAIN settled (791), acknowledged by the owner on 2026-11-14: server-serialized, escaped selection blocks live inside existing `Parts` text. Add no persisted fields and no grouped request/cancel metadata. Protect marker-looking selected source from ordinary image renumbering. Older writers have no new unknown field to discard; older sessions/leads see the self-describing block as plain text. This is the compatibility fallback, with no new refresh rule beyond MAIN's normal daemon swap. If the existing text paths cannot carry the approved behavior without grouped metadata, stop and send the concrete requirement to MAIN. Rejected: introducing optional grouping fields merely because they look additive, or silently relying on unknown-field preservation.
10. **Apply the confirmed strict group grammar.** MAIN settled (792), acknowledged by the owner: ordered legacy-shaped mentions, then exactly one comment even when empty, then existing image references. Escape source and comment independently. Forbid nested groups and unknown children. Retain each malformed group wholly as ordinary text rather than extracting a misleading partial selection. Keep legacy transcripts and their existing rendering intact.
11. **Preserve the existing unsent-image reload fallback.** Owner (790)=a, relayed by MAIN on 2026-11-14: keep warning and discarding unsent pasted-image bytes on reload. Preserve already-written attachment references and the surviving selection's source/comment association; do not retain dangling image markers. No durable unsent-byte storage or upload-before-send policy is authorized. Rejected: expanding draft durability to retain new pasted bytes.

## Constraints

- Preserve source/provenance, document order, per-session attachment and draft isolation, separate send, focus/selection handling, and IME safety from C670. No assertion relaxation or retrospective claim of complete C670 browser acceptance.
- Preserve legacy `wb:mention` parsing and rendering, including ordinary text around it. Do not silently reinterpret existing transcript text or add an unsupported attribute to the strict mention grammar.
- Keep memo ownership recoverable across multiple pushes, cross-entry drags, draft restore, inbox lifecycle, delivered text and history rendering. A flat list of all mentions followed by unrelated memo paragraphs does not satisfy the contract.
- Implement (791) using existing text/Parts storage and existing attachment fields, with no grouped request/cancel metadata or new persisted shape. Ground any additional requirement before returning it to MAIN; no tracker event, protocol-version change or T2 migration is authorized.
- Handle quote text and memo content as untrusted content. Preserve existing output escaping and provenance trust boundaries; markup-looking user content must not create a different selection or comment.
- No approved visual mockup was supplied for this follow-up. Do not invent a mockup approval or couple this work to a shared-card redesign.
- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing `agents-plugin/`, `agents-plugin-wsflow/` or `agents-plugin-tool/`.
- Read `ai-docs/manuals/release-compatibility.md` before editing `agents-plugin-tool/internal/`; assess parser, transport, stored representation and installed-version interaction against T0/T1/T2.
- Read `ai-docs/manuals/ws-mcp.md` before editing `agents-plugin-tool/internal/mcp/`, if needed.
- Read `ai-docs/manuals/skill-authoring.md` before editing skills, playbooks, agent prompts or convention docs, including applicable embedded conventions.
- Read `ai-docs/manuals/wsflow-mirroring.md` before changing shared playbooks, full ws skills, or mirrored plugin surfaces.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Ticket authoring authorizes no runtime mutation or owner-environment installation. Owner (778), corrected at 13:53:33 on 2026-11-14, reserves daemon rebuild/restart/swap timing and execution to MAIN alone. After eventual develop integration, EXEC mails MAIN the merge hash immediately and does not rebuild, swap, smoke-test a swap or perform swap-related profiling. Main push and version bumps still require approval.

## Prior Art and Evidence

- `261114-feat-ws-dashboard-selection-mention-note`: original capture/attach/separate-send contract, prior loose-text technical choice, source/provenance constraints and explicitly retained browser gaps. Its historical phase and acceptance records are not rewritten by this follow-up.
- MAIN's owner-(772) relay: confirmed one-push grouping and compatibility target. EXEC1 read the full supplied report in the project's shared artifact directory as `shared/docs/memo-mention-spec-report.md`, a supporting artifact rather than a downstream dependency. Its source snapshot is `e188b27955fce8e0e73ed84efb21cc0003f33fdc`; the recoverable findings needed by implementation are recorded here.
- The report traces JSON `{text, mentions}` from dashboard POST through inbox storage/normalization, `Entry.Text()` delivery, and history reconstruction by `ParseMentions`. The HTTP route does not parse tag strings. Ring history stores delivered text. Updating only browser JavaScript cannot add grouped fields to the old Go request/model.
- The report distinguishes compatibility directions: new readers accepting old transcripts is not proof that old installed playbooks understand new output or that an older writer preserves new stored data. Pi/Claude inspected delivery adapters transport strings, not an independently versioned `ParseMentions` implementation. Keep this distinction in T0 verification; do not call every new tag inherently T2 or automatically T0.
- The report's earlier suggested optional nested comment and presentation-only alternatives are analysis, not this ticket's authority. Owner (772) instead confirms the outer selection containing legacy-shaped mentions plus exactly one separate comment child; do not import the report's optional-comment emission recommendation as a conflicting contract.
- VERIFIED by static grounding at `34b7ecb57`: `ParseMentions` consumes a leading contiguous strict legacy mention sequence, rejects raw `<` in its body, and preserves failed material as ordinary text (`agents-plugin-tool/internal/wsinbox/mentions.go#L70-L93`, `mentions_test.go#L197-L223`). It does not recognize an outer `wb:selection`. `Entry.Text()` emits answers, mentions, picks, consoles, ordinary parts, then image lines (`agents-plugin-tool/internal/wsinbox/inbox.go#L326-L362`). These are inspected source facts, not runtime verification.
- MAIN reports installed-plugin playbook resolution in `agents-plugin-tool/cmd/ws-mcp/proxy.go:73-97`. Follow the actual shared playbook/adapter loading path when implementing Phase 3 rather than assuming a daemon asset update changes existing sessions.
- Existing composer and selection paths are in `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`; inbox request handling and history rendering are in `inbox_routes.go` and `pageview_inbox.go`. These identify preservation surfaces, not a prescribed new representation.

## Static Grounding

Source inspection at `34b7ecb57`; no runtime checks were run.

- Existing uploads use `images[{name,data,n}]`; written attachments use `refs` and `refNums`; cancel returns references plus `nums`. Existing limits are eight images per held entry, including merged sends, and 10 MiB decoded per image, with sniffed PNG/JPEG/GIF/WebP and session-directory confinement (`agents-plugin-tool/internal/wsdashboard/inbox_routes.go#L64-L132`, `#L338-L425`, `#L478-L503`; `agents-plugin-tool/internal/wsinbox/inbox.go#L57-L70`, `#L275-L312`).
- Merge deduplicates image paths and remaps later markers in ordinary parts, chip notes and topic notes, not `Mention.Text` (`agents-plugin-tool/internal/wsinbox/inbox.go#L585-L640`). Grouped text must distinguish literal marker-looking source from actual selection-owned references; remapping the former would corrupt the excerpt.
- Marker discovery, deletion, synchronization and thumbnail ownership exclude the mention popup and future selection objects. Merely calling `addFiles(files, mnote)` is insufficient (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L1801-L1844`, `#L1876-L2005`, `#L2030-L2040`).
- Drafts retain written references but record unsent images as lost numbers, then strip their markers and warn on restore (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L8707-L8732`, `#L8759-L8815`). Decision (790)=a preserves this behavior.
- Existing request/model/normalize/merge/cancel paths have no bundle association. Known-field reconstruction would drop unknown grouping fields (`agents-plugin-tool/internal/wsdashboard/inbox_routes.go#L82-L112`, `#L414-L425`, `#L478-L503`; `agents-plugin-tool/internal/wsinbox/inbox.go#L173-L199`, `#L383-L410`, `#L593-L640`, `#L696-L741`). Decision (791) therefore uses existing text instead of adding those fields.
- History parses delivered text and image lines; attachment existence controls thumbnail versus path fallback and participates in the render cache key. Cover held messages as well as delivered history (`agents-plugin-tool/internal/wsdashboard/pageview_inbox.go#L69-L145`, `entry_cache.go#L41-L57`).
- Canonical message guidance is the Answers section of `agents-plugin/rsrc/whiteboard/whiteboard.md#L350-L368`. The connecting plugin supplies its own resource root (`agents-plugin-tool/cmd/ws-mcp/proxy.go#L73-L97`, `internal/mcp/whiteboard_tools.go#L341-L353`). Update wsflow/Pi resource mirrors and manifests; Pi's byte-identity guard is `agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go#L20-L80`. Adapter string-only delivery in the supporting report was not independently re-audited in this grounding.

The bounded surfaces are the existing inbox parser/model/lifecycle and tests; dashboard request/cancel, held/history rendering and cache; composer/popup/drafts in `assets/dashboard.js`, `dashboard.css` and `rail.js`; existing mention/image/browser tests; canonical whiteboard guidance and its wsflow/Pi mirrors/manifests. These identify review surfaces, not a requirement to edit every file. No new upload route, adapter protocol, tracker event or daemon lifecycle mechanism is part of this work.

## Prior Decisions

- `261114-feat-ws-dashboard-selection-mention-note`: preserve source/provenance, one memo for a complete multi-entry selection, capture locking and separate send; its loose composer-text delivery choice is superseded by (772), not retained as a constraint.
- `9538354ca` (2026-11-14): input-held bundles retain copied source across later live-selection/DOM changes; only the existing composer send action sends.
- `ec2320552` (2026-11-14): integrated C670 source while explicitly retaining capsule-count/browser/IME gaps; do not rewrite that historical acceptance.
- `3ca6d9a6` (2026-11-13): chip/topic notes are remapped after the image-number map exists, without changing the entry shape (T0).
- `261113-feat-ws-dashboard-board-note-image-paste`: unsent page-data images are lost on draft reload; (790)=a explicitly preserves that precedent.
- `29c81ad6` (2026-11-07): missing attachments render as path text, not broken images.

The predecessor is related, not a hard prerequisite: its source is integrated but its ready ticket remains open with accepted browser gaps. Root's ticket/rationale queries supplied this context; ownership evidence was cached and is not a fresh lease confirmation.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Parser/model/lifecycle, request/cancel, composer/drafts, held/history rendering and shipped resource mirrors. |
| scope.surface | public-interface | Decisions (772), (791), (792) add delivered selection/comment syntax using existing text storage and transport. |
| scope.new_public_symbol | yes | New `wb:selection` and `wb:comment` message tokens; no new persisted fields or grouped request/cancel metadata. |
| scope.new_type_contract | yes | Ordered mentions, exactly one comment and selection-owned image markers survive delivery and supported lifecycle paths. |
| scope.test_surface | existing | wsinbox mention/attachment/lifecycle tests; wsdashboard inbox/history/mention/image/railscript and selection-note browser tests; resource mirror guards. |
| complexity.reuse_points | confirmed | ImageMarker/ImageLine, Merge, addFiles, syncMarkers, restoreHeld, readDraft/restoreDraft, ParseMentions, heldAnswer and history splitting were inspected. |
| complexity.side_effect_risk | high | Shared attachment state, marker renumbering and stored text rewrites can corrupt source or selection/image ownership. |
| risk.correctness | high | Preserve exact source, memo ownership, asynchronous drafts, multiple pushes and merge/cancel/resend behavior. |
| risk.fit | moderate | Grouping and popup thumbnails must fit existing controls; there is no approved replacement layout. |
| risk.test | high | Existing tests pin loose-text behavior; C670 retains capsule-count/native-IME/browser gaps. |
| risk.security_or_contract | high | Untrusted grammar/escaping, attachment confinement and old-reader/writer preservation determine the T0 boundary. |

## Phases

### Phase 1: Add grouped parser and transport support

Implement additive support for the confirmed selection/comment shape through the actual parser, inbox serialization, transport and delivery paths. Include the image association contract confirmed for capture in (779), reusing the existing attachment path without inventing a new image wire shape. Retain legacy syntax and ordinary text. Preserve selection boundaries, ordered source provenance and exactly one associated memo per push; avoid flattening away that relationship in delivery. Record actual cross-version and on-disk effects and stop for MAIN if the T0 target cannot be met.

Verification: focused parser/round-trip and inbox lifecycle tests for a single mention, ordered cross-entry mentions, multiple selection blocks, surrounding ordinary text, escaped markup-looking content and legacy transcripts. Cover any settled image references and storage/limit behavior without losing their owning selection. Demonstrate no source/memo confusion or duplication and classify mixed-version behavior from evidence, not additive-tag naming alone. Pin the confirmed (791)/(792) text representation and strict whole-group fallback, including empty comments and literal image-marker-looking source. Stop for MAIN if grouped metadata or another storage/API change proves necessary.

### Phase 2: Preserve bundles in composer and history

Depends on Phase 1's grouped representation. Replace loose memo attachment with per-push grouping in the composer, per-session draft persistence, send and history display. Keep mentions and their memo visibly together; a multi-entry drag shares one memo. Add image paste/drop and the popup thumbnail using the existing attachment storage/limits settled in (779). Preserve image-to-selection ownership through draft, send and history, subject to (790)=a: written references survive reload, while unsent pasted bytes and their dangling markers are discarded with the existing warning. Preserve independent ordinary text and the separate send action. Reuse original capture/focus/provenance protections rather than retargeting the live selection after input focus.

Verification: one push, a cross-entry drag, several pushes with different memos, image paste into the memo popup, edits, session switching, draft restore, send and history reload. Include thumbnail/drop/storage/size-limit cases and multiple pushes with distinct images so no image moves to another selection. Cover pointer and keyboard selection, narrow/wide layouts and IME safeguards. Verify the same grouping in delivered content and history; old transcripts render as before. Retain actual selected-range/provenance assertions and report unavailable native-IME coverage explicitly.

### Phase 3: Document the grouped message form in shipped playbooks

Depends on the implemented Phase 1 shape and Phase 2 behavior. Update the shipped lead-facing protocol guidance to explain `wb:selection`, its ordered `wb:mention` source children, and its single `wb:comment` memo child. Distinguish selected-source content from the user's associated memo, explain the settled image association, and preserve the legacy-message explanation. Apply applicable wsflow mirrors and adapter boundaries.

Verification: authored-text and packaging/mirror checks, plus a rendered-playbook check from the installed-plugin loading path. State which version/session must refresh to obtain the guidance; do not equate edited source with guidance already loaded by a running session.

### Phase 4: Verify the complete grouped lifecycle and compatibility boundary

Depends on Phases 1–3. Complete focused regression and browser acceptance across parser/transport, composer/drafts, delivered content, history and shipped guidance. Preserve original C670 evidence separately and record only tests actually run against this ticket's pinned candidate.

Verification: round-trip matrix for single/cross-entry/multiple pushes, selection-owned images and legacy transcripts; no memo emitted as loose composer text; correct user/source distinction; session/draft/send/merge lifecycle association; focused Go and JavaScript checks; real-browser narrow/wide selection/composer/history cases; independent implementation review. Record exact source/served pins, commands, output, limitations and process quiescence. Verification fixtures are isolated. After eventual develop integration, mail MAIN the merge hash immediately; MAIN alone owns subsequent daemon replacement, swap smoke and profiling under the corrected (778) directive.
