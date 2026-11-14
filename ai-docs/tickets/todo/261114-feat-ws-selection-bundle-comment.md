---
title: "Selection bundles: keep each push's mentions and memo together"
related:
  261114-feat-ws-dashboard-selection-mention-note: "follow-up to source integrated at ec23205525e240bf3d3da2cf36ea3fd4642b2211; supersedes its loose-note presentation and delivery choice, not its historical acceptance evidence"
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
6. **Queue after EXEC1's current items.** MAIN explicitly requested this follow-up in EXEC1's lane. Capture as accepted `todo/`; no implementation, ready promotion, runtime replacement, or heavy-slot ownership follows from ticket authoring.

7. **Support image paste and drop in the mention memo popup.** MAIN relayed the owner's explicit image-paste request on 2026-11-14, accompanied by a screenshot of the existing memo textarea and Mention button. MAIN explicitly instructed capture of (779)'s concrete default in the 13:53:05 settlement relay: reuse existing composer attachment storage and size limits, show a thumbnail in the popup, and reference the image inside that push's `wb:selection` beside its `wb:comment`. Ownership is per selection, like the memo. No new storage shape is authorized. The screenshot identifies the target control, not an approved replacement layout.
8. **Reuse an existing image reference rather than inventing an image protocol.** Recover the actual existing attachment reference and renumbering behavior during implementation grounding; preserve it through selection grouping, draft restore, message merge and cancel/resend. No exact new image tag or field is approved by this capture. If a per-selection reference cannot remain T0 without another protocol/API/store change, return the concrete gap to MAIN for an owner question before making that change. Authority: MAIN's (779) settlement relay. This is specific authority to capture this decision, not a change to the repository's general decision-acknowledgement rules.

## Constraints

- Preserve source/provenance, document order, per-session attachment and draft isolation, separate send, focus/selection handling, and IME safety from C670. No assertion relaxation or retrospective claim of complete C670 browser acceptance.
- Preserve legacy `wb:mention` parsing and rendering, including ordinary text around it. Do not silently reinterpret existing transcript text or add an unsupported attribute to the strict mention grammar.
- Keep memo ownership recoverable across multiple pushes, cross-entry drags, draft restore, inbox lifecycle, delivered text and history rendering. A flat list of all mentions followed by unrelated memo paragraphs does not satisfy the contract.
- Implementation must establish the actual request, storage and delivery representation from source before changing those surfaces; this ticket does not choose a new field name, store shape, tracker event or protocol version.
- Handle quote text and memo content as untrusted content. Preserve existing output escaping and provenance trust boundaries; markup-looking user content must not create a different selection or comment.
- No approved visual mockup was supplied for this follow-up. Do not invent a mockup approval or couple this work to a shared-card redesign.
- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing `agents-plugin/`, `agents-plugin-wsflow/` or `agents-plugin-tool/`.
- Read `ai-docs/manuals/release-compatibility.md` before editing `agents-plugin-tool/internal/`; assess parser, transport, stored representation and installed-version interaction against T0/T1/T2.
- Read `ai-docs/manuals/ws-mcp.md` before editing `agents-plugin-tool/internal/mcp/`, if needed.
- Read `ai-docs/manuals/skill-authoring.md` before editing skills, playbooks, agent prompts or convention docs, including applicable embedded conventions.
- Read `ai-docs/manuals/wsflow-mirroring.md` before changing shared playbooks, full ws skills, or mirrored plugin surfaces.
- Ticket authoring authorizes no runtime mutation or owner-environment installation. After eventual develop integration, the merging session follows the owner's confirmed (778) merge-equals-immediate-rebuild/swap routine inside the heavy slot, with dashboard smoke and reporting; this ticket adds no separate swap gate. Main push and version bumps still require approval.

## Prior Art and Evidence

- `261114-feat-ws-dashboard-selection-mention-note`: original capture/attach/separate-send contract, prior loose-text technical choice, source/provenance constraints and explicitly retained browser gaps. Its historical phase and acceptance records are not rewritten by this follow-up.
- MAIN's owner-(772) relay: confirmed one-push grouping and compatibility target. EXEC1 read the full supplied report in the project's shared artifact directory as `shared/docs/memo-mention-spec-report.md`, a supporting artifact rather than a downstream dependency. Its source snapshot is `e188b27955fce8e0e73ed84efb21cc0003f33fdc`; the recoverable findings needed by implementation are recorded here.
- The report traces JSON `{text, mentions}` from dashboard POST through inbox storage/normalization, `Entry.Text()` delivery, and history reconstruction by `ParseMentions`. The HTTP route does not parse tag strings. Ring history stores delivered text. Updating only browser JavaScript cannot add grouped fields to the old Go request/model.
- The report distinguishes compatibility directions: new readers accepting old transcripts is not proof that old installed playbooks understand new output or that an older writer preserves new stored data. Pi/Claude inspected delivery adapters transport strings, not an independently versioned `ParseMentions` implementation. Keep this distinction in T0 verification; do not call every new tag inherently T2 or automatically T0.
- The report's earlier suggested optional nested comment and presentation-only alternatives are analysis, not this ticket's authority. Owner (772) instead confirms the outer selection containing legacy-shaped mentions plus exactly one separate comment child; do not import the report's optional-comment emission recommendation as a conflicting contract.
- MAIN reports `ParseMentions` in `agents-plugin-tool/internal/wsinbox/mentions.go:78-84` stops at raw `<`; strict attribute rejection is covered at `mentions_test.go:197-220`; `Entry.Text` ordering is at `inbox.go:332-353`. These are relayed source findings, not fresh EXEC1 investigation or runtime verification.
- MAIN reports installed-plugin playbook resolution in `agents-plugin-tool/cmd/ws-mcp/proxy.go:73-97`. Follow the actual shared playbook/adapter loading path when implementing Phase 3 rather than assuming a daemon asset update changes existing sessions.
- Existing composer and selection paths are in `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`; inbox request handling and history rendering are in `inbox_routes.go` and `pageview_inbox.go`. These identify preservation surfaces, not a prescribed new representation.

## Phases

### Phase 1: Add grouped parser and transport support

Implement additive support for the confirmed selection/comment shape through the actual parser, inbox serialization, transport and delivery paths. Include the image association contract confirmed for capture in (779), reusing the existing attachment path without inventing a new image wire shape. Retain legacy syntax and ordinary text. Preserve selection boundaries, ordered source provenance and exactly one associated memo per push; avoid flattening away that relationship in delivery. Record actual cross-version and on-disk effects and stop for MAIN if the T0 target cannot be met.

Verification: focused parser/round-trip and inbox lifecycle tests for a single mention, ordered cross-entry mentions, multiple selection blocks, surrounding ordinary text, escaped markup-looking content and legacy transcripts. Cover any settled image references and storage/limit behavior without losing their owning selection. Demonstrate no source/memo confusion or duplication and classify mixed-version behavior from evidence, not additive-tag naming alone. Unsettled grammar/error-handling choices go to MAIN before ready promotion.

### Phase 2: Preserve bundles in composer and history

Depends on Phase 1's grouped representation. Replace loose memo attachment with per-push grouping in the composer, per-session draft persistence, send and history display. Keep mentions and their memo visibly together; a multi-entry drag shares one memo. Add image paste/drop and the popup thumbnail using the existing attachment storage/limits settled in (779). Preserve image-to-selection ownership through draft, send and history. Preserve independent ordinary text and the separate send action. Reuse original capture/focus/provenance protections rather than retargeting the live selection after input focus.

Verification: one push, a cross-entry drag, several pushes with different memos, image paste into the memo popup, edits, session switching, draft restore, send and history reload. Include thumbnail/drop/storage/size-limit cases and multiple pushes with distinct images so no image moves to another selection. Cover pointer and keyboard selection, narrow/wide layouts and IME safeguards. Verify the same grouping in delivered content and history; old transcripts render as before. Retain actual selected-range/provenance assertions and report unavailable native-IME coverage explicitly.

### Phase 3: Document the grouped message form in shipped playbooks

Depends on the implemented Phase 1 shape and Phase 2 behavior. Update the shipped lead-facing protocol guidance to explain `wb:selection`, its ordered `wb:mention` source children, and its single `wb:comment` memo child. Distinguish selected-source content from the user's associated memo, explain the settled image association, and preserve the legacy-message explanation. Apply applicable wsflow mirrors and adapter boundaries.

Verification: authored-text and packaging/mirror checks, plus a rendered-playbook check from the installed-plugin loading path. State which version/session must refresh to obtain the guidance; do not equate edited source with guidance already loaded by a running session.

### Phase 4: Verify the complete grouped lifecycle and compatibility boundary

Depends on Phases 1–3. Complete focused regression and browser acceptance across parser/transport, composer/drafts, delivered content, history and shipped guidance. Preserve original C670 evidence separately and record only tests actually run against this ticket's pinned candidate.

Verification: round-trip matrix for single/cross-entry/multiple pushes, selection-owned images and legacy transcripts; no memo emitted as loose composer text; correct user/source distinction; session/draft/send/merge lifecycle association; focused Go and JavaScript checks; real-browser narrow/wide selection/composer/history cases; independent implementation review. Record exact source/served pins, commands, output, limitations and process quiescence. Verification fixtures are isolated; eventual develop integration follows the existing owner-approved (778) merge/swap/smoke routine rather than adding a separate acceptance or approval round.
