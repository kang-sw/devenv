---
title: "Whiteboard: panel-first ordered stream with immutable patch versions"
---

# Whiteboard: panel-first ordered stream with immutable patch versions

## Background

Owner approval (1137)=ok, relayed by MAIN on 2026-11-15, authorizes this todo capture only. Implementation is post-v0.50.0. The owner identified two connected problems: topic content easily becomes stale when agents only append logs, while panels are underused even though dated snapshots communicate their age. Make panels the durable surface and retain logs, panels and questions in one readable history.

English rendering of the owner's request (2026-11-15 20:52-21:05 KST): remove topic content rendering, make panels primary, interleave logs/panels/questions, and support smaller panel updates instead of full overwrite only. The owner preferred recoverable older versions under (1136)=a, suggested stem-based panel identity and a wrapper around old/new pairs, and approved the revised `wb:change` syntax under (1137)=ok. This ticket records the final ten agreed decisions rather than treating every preliminary command spelling as a separate API request.

## Decisions

1. **Panel-first presentation.** Remove the separate topic-content rendering surface, not its information. Panels become the primary durable surface: each is a dated snapshot rather than an undated claim to current state. Rejected: continuing to present mutable content as current while agents only maintain logs; hiding or dropping existing content during the transition.
2. **One ordered topic stream.** Inside a topic, `wb:log`, `wb:panel` and `wb:add` question commands interleave in write order. Existing question transition events keep their history positions. There is no new `wb:message` or `wb:ask` spelling in this approved contract; the settled stream uses the current log/add names. Rejected: separate content/log/panel regions that obscure the order of updates and questions.
3. **Stable identity plus optional stem.** Server-minted `pN` remains the panel identity for links and history position. Add optional `name="..."`, unique within a topic, analogous to topic names. Reposting the same name creates a new version of that panel. Existing `id="pN"` updates continue working, including unnamed legacy panels. Reusing a name when a new panel was intended updates that existing panel, but its previous version remains recoverable. Rejected: replacing server-minted ids with agent-assigned ids or losing old versions on name reuse.
4. **Wrapped exact-match patches.** A panel patch contains one or more changes in this literal form:

   ```xml
   <wb:panel name="implementation" recap="Updated status">
   <wb:change><wb:old>Pending</wb:old><wb:new>Complete</wb:new></wb:change>
   <wb:change><wb:old>Anchor</wb:old><wb:new>Anchor plus inserted text</wb:new></wb:change>
   </wb:panel>
   ```

   Each `wb:change` contains exactly one `wb:old` and one `wb:new`. Empty new text deletes the matched text; insertion retains an old-text anchor and includes the anchor plus new text in `wb:new`. Refuse text outside the change wrappers, missing operands, zero matches, multiple matches, or mixed full payload and changes. Full replacement remains a full payload posted with the same name/id. The new changes require one unique old-text match, not legacy `replace_all` behavior. Rejected: a flat old/new array, which makes pairing error-prone; HTML `li` wrappers, because panel HTML or old text can itself contain `li`, unlike the reserved `wb:` wrapper.
5. **Immutable dated versions.** Every successful patch creates a new dated immutable `eN` version. Patching saves command tokens; it is not in-place mutation of a historical panel. Rejected: editing the earlier entry and making an old snapshot silently claim a newer state.
6. **Patch recap inheritance.** Omitting `recap` on a patch inherits the previous version's recap. This is a patch-specific omission rule, not permission to remove the existing full-payload recap requirement.
7. **Recoverable earlier versions.** Under (1136)=a, append the new version at the bottom of history. Keep prior versions at their original positions, folded but expandable, with a newer-version reference, version number and time. A panel link continues to resolve to the latest version; an `eN` link resolves to that exact version. The optional old/new diff viewer mentioned during discussion was a proposal, not an approved requirement, and is not part of this capture. Rejected: deleting, hiding permanently, or moving the old entry to the newest position.
8. **Legacy calls and existing content survive.** Legacy `wb:body` posts a new version of one fixed per-topic panel labeled "Current content" (localized UI label permitted); legacy `wb:edit` patches that same panel. Existing topic content migrates into history as that panel at its last-changed time and remains visible/recoverable. Old playbooks keep working. Compatibility approval is the **T0 transparent target**, including behavior-preserving aliases; a concrete store verdict remains subject to the directional evidence below. No T3 boundary is approved. Rejected: removing body/edit calls, silently losing legacy content, or asserting that new syntax alone proves store compatibility.
9. **Header summary remains the latest agent log.** The topic header continues to use the latest agent log line, not a panel payload, question or system event. Rejected: changing the summary source as an incidental consequence of the panel-first layout.
10. **Literal code is inert.** Fix the dogfood parser failure in this same implementation: literal `wb:` examples inside fenced code blocks and inline backticks in panel/log content are text, not commands or operand delimiters. Preserve unrelated payload and one-line-log validation. Rejected: requiring callers to use entity-escaped, blank-line-free HTML `pre` as the only way to show command syntax.

## Constraints

- This is an owner-approved command/protocol semantic change, not a documentation-only implementation. It is deliberately outside the v0.50.0 ship cut. This capture changes only this todo ticket; it does not promote to ready or authorize implementation now.
- Preserve panel `pN` and immutable entry `eN` references, legacy body/edit calls, question transition history and the latest-agent-log summary while removing duplicate content presentation.
- The visual phase is Claude-owned and mockup-first under the owner's rule: a GPT-created UI must be replaced, so begin with an approved mockup rather than implementing visuals speculatively. Store the approved reference under `ai-docs/tickets/assets/261115-feat-ws-whiteboard-panel-first-stream/`, link it here before ready promotion, and require side-by-side comparison of result captures against that reference. No visual reference has been approved for this ticket yet.
- Compatibility is constrained to the approved T0 target; do not introduce a schema/read/write or connection boundary and call it transparent. Any need to relax that target returns to the owner; T3 in particular requires separate explicit approval and a minor release. No such relaxation is authorized here.
- Implementation manuals: `ai-docs/manuals/shipped-surface-boundary.md` for shipped tooling/playbook text; `ai-docs/manuals/release-compatibility.md` for internal command/store changes; `ai-docs/manuals/ws-mcp.md` if MCP routing or emitted results change; `ai-docs/manuals/skill-authoring.md` and `ai-docs/manuals/wsflow-mirroring.md` before updating the shared whiteboard playbook. Shared text must remain downstream-resolvable and mirrors must be generated rather than hand-edited.

## Current Source Evidence and Compatibility Boundary

The following describes the captured baseline, not completed implementation:

- `agents-plugin-tool/internal/wswhiteboard/commands.go:22-42` defines one topic group with body/edit/panel/log/add siblings. Panel is currently a leaf accepting id/recap; edit has old/new fields. Ordered parsing is in `agents-plugin-tool/internal/wsxmlcmd/wsxmlcmd.go:280-315,327-373,380-470`. The current table has no `wb:message` command.
- `agents-plugin-tool/internal/wswhiteboard/entries.go:26-49,75-81,103-158` provides server-minted panel ids, immutable entries, recap requirements and full reposts. There is no panel-name or panel-patch implementation yet. The latest agent-log summary remains in `entries.go:52-73` and `view.go:180-188`. `history_items.go:8-53` preserves recorded item transitions at their stream positions; do not reorder legacy undated items by inventing timestamps.
- Mutable body writes and exact-match edits are in `agents-plugin-tool/internal/wswhiteboard/store.go:1161-1233`. The approved new patch contract is stricter than legacy edit's optional multi-match replacement.
- `agents-plugin-tool/internal/wsxmlcmd/wsxmlcmd.go:599-650` scans raw bodies through exact closing tags/CDATA without Markdown code-span/fence state; `commands.go:23-24` also enables operand-field-opening checks. The owner reported literal-code failures; this capture does not claim an executed reproduction or that every unrelated opening tag is interpreted as a command. Include enclosing closing tags and operand tags in regressions, not only an innocent opening tag.
- Current folder schema is fixed at 2 (`agents-plugin-tool/internal/wswhiteboard/folders.go:27`). Fixed document structs and schema checks are at `folders.go:49-67,226-250`; load reconstructs documents and immutable entries and validates matching panel/log metadata at `folders.go:384-484`. Ordinary saves regenerate fixed topic/session metadata while accepting only new immutable entries (`folders.go:595-704`). Replay retains identical existing panel files, appends exact log bytes and replaces metadata (`folders.go:747-819`). Entry codecs preserve unknown members in memory (`agents-plugin-tool/internal/wswhiteboard/entry_json.go:5-44`); the committed log is not reserialized.
- **Directional verdict before ready/release:** T0 aliases are the approved API direction, but storing authoritative new names or migration markers only in fixed topic metadata is not proven T0: an older ordinary writer can decode fixed structs and regenerate that metadata. An increased folder schema would make that baseline reader refuse, which is T3 rather than T0. No concrete persistence/reconstruction mechanism is selected by this todo capture. Prove old-write/new-reopen preservation for the actual implementation before settling its store tier; do not infer it from an additive field or append-only log alone.
- Use the actual shipped v0.50.0 baseline once cut, and trace any other supported older reader/writer separately. In particular, v0.49.1 lacks both `folders.go` and `entry_json.go`; current folder/unknown-member preservation cannot be attributed to that tag. The implementation proof must cover the new writer, each relevant old reader/writer, retained bytes after old writes and the new reopen reconstruction path, with source lines for every concrete tier verdict.

## Phases

### Phase 1: Panel identity, wrapped patches and inert literal syntax

Implement the approved optional topic-local panel name and wrapped `wb:change` syntax while retaining server-minted panel ids, existing id-based full reposts and immutable `eN` versions. Preserve full-payload behavior and add patch recap inheritance. Fix command/operand recognition inside inline code and fenced code without relaxing unrelated validation. The syntax is the settled contract above; do not substitute a flat old/new array or HTML list wrapper.

Verification: focused parser/store tests cover multiple changes, exact single matches, missing/duplicate operands, zero/ambiguous matches, deletion, anchored insertion, refusal of stray text and mixed payloads, full replacement, named repost versus unnamed id repost, recap inheritance, immutable prior payloads and stable panel/version links. Literal-code fixtures include inline backticks, fenced blocks, enclosing closing tags and old/new/change spellings in panel/log payloads; retain separate one-line-log checks. This phase does not authorize visual changes.

### Phase 2: Persisted ordered history and mockup-approved fold/expand UI

Depends on Phase 1. Implement the persistence/reconstruction needed for the approved names and immutable version stream, satisfying the T0 evidence boundary rather than selecting a schema bump by default. Append each new version in write order, leave earlier versions at their original positions folded but expandable, and keep the latest-panel/exact-version link distinction. Logs, panels and question transitions form one topic history; the header remains the latest agent log.

The visual implementer is a Claude agent working from an owner-approved mockup stored and linked as required above. Verification includes old/new reader-writer-reopen fixtures for panel identity/version preservation and focused browser behavior for ordering, fold/expand, earlier payload recovery and links. Compare result captures side by side with the approved reference. Do not implement the unapproved optional diff viewer.

### Phase 3: Legacy aliases, content migration and shared playbook

Depends on Phases 1 and 2. Route legacy body/edit calls through the fixed per-topic Current content panel. Migrate preexisting content at its last-changed time without hiding it or deleting prior information. Preserve old playbook calls through repeated use after migration, not just a one-time conversion. Update canonical `agents-plugin/rsrc/whiteboard/whiteboard.md` to teach the panel-first stream and wrapped patch syntax, retain the aliases, and regenerate its wsflow resource mirror according to the declared manuals.

Verification: existing-content migration fixtures preserve payload, recorded time and historical references; legacy body followed by edit and a second legacy update operate on the same panel without losing intervening versions or duplicating migration history. Exercise the intervening old-write/new-reopen path and record its concrete tier with source citations. Run focused command/history regressions, the shared-resource manifest/mirror drift checks and wsflow rendering/package checks required by the mirroring manual. Re-read changed shipped text for downstream dependencies and apply the skill-authoring invariant checklist. No implementation or verification pass is claimed by this todo capture.

## Owner Decisions (2026-11-16)

- (1181)=ok: the panel-first history mockup is approved. One line in
  write order, older versions fold in place, v1/v2 labels, and distinct
  links for "latest" and "this version".
- Phase 1 parser state at handoff: branch `exec4/panel-first-literal-code`
  at `4c875057b` (base EXEC1 `1b7e7cd65`). Round-4 independent review was in
  flight at handoff; see the repo note `handoff`.
