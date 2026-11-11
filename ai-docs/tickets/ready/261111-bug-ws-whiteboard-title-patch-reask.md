---
title: "A whiteboard patch that rewrites an item's title re-asks it"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 66551c078560dc18
sage-review-completeness-reviewed: 66551c078560dc18
---

# Title patch re-asks a whiteboard item

## Background

User, 2026-11-11: an agent sometimes rewrites a question it already put on
the whiteboard and posts it again. When the user had already sent an answer
for that item from the dashboard, the item keeps its sent mark, so the
rewritten question looks answered and the user does not see that it waits
on them again.

Today `applyPatch` (`agents-plugin-tool/internal/wswhiteboard/store.go`)
stamps `ReaskedAt` only when a patch reopens a settled item or really
changes its class or options; the dashboard's `sentMarks`
(`internal/wsdashboard/pageview_inbox.go`) drops sent marks older than that
stamp. A patch that rewrites only the title is not a re-ask, and the
whiteboard playbook says so.

## Decisions

1. A patch that really changes an item's title is a re-ask, the same as a
   class or options change: it stamps `ReaskedAt`, so the dashboard drops a
   sent mark older than the patch and the item shows as waiting on the user
   (user acknowledgement, board (294)). The title and the options carry the
   question's meaning.
   - Rejected: re-asking on every field change. A patch that edits only
     `wb:why`, `wb:recommend`, or links leaves the question itself as it was,
     so it keeps the sent mark.
   - Resending the current title unchanged is not a re-ask, matching the
     existing rule for class and options.
   - A title change re-asks only an item that is open once the patch
     applies, the gate class and options already have; a title fix on a
     settled item leaves it settled and stamps nothing, and a settled item
     is re-asked by `status="open"`, which already stamps (technical,
     review-raised).
   - This reverses the title-only clause of
     `261109-bug-ws-whiteboard-patch-cannot-change-item-class` (its re-ask
     definition, decision (122) as amended by (127) a: "A patch that edits
     only the title ... is not a re-ask"), by the user's own request
     (2026-11-11: a sent mark should clear when an agent rewrites the
     question).

## Constraints

- An answer sent before the patch and not yet delivered still reaches the
  lead, as today; only the sent mark changes.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`
  (the whiteboard playbook's `Re-ask` paragraph has a wsflow copy at
  `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`), and
  `ai-docs/manuals/ws-mcp.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)

## Prior Decisions

- 261109-bug-ws-whiteboard-patch-cannot-change-item-class (2026-11-09, Decisions): "a re-ask is a `wb:patch` that changes the item's `class` or its options, or that sets a settled item ... back to `status=\"open\"`. A patch that edits only the t[itle ...]" (amended by 127 a) — bearing: reversed by Decision 1 (user request)
- 57ba56f1 (2026-11-09, commit): "`Item.ReaskedAt` (RFC 3339, millisecond precision) is stamped on a real class change, a real options change (`slices.Equal`), or settled/assumed -> open; a later re-ask replaces it." — bearing: supports
- 57ba56f1 (2026-11-09, commit): "sentMarks filters server-side; .sentdata, applySent and pill.js are untouched. A chip with no send time (pre-change records) keeps its mark." — bearing: constrains
- 261109-bug-ws-whiteboard-patch-cannot-change-item-class (2026-11-09, Decisions): "the `whiteboard` playbook (`agents-plugin/rsrc/whiteboard/whiteboard.md`, mirrored in wsflow and pi), all three byte-identical and pinned by drift tests" — bearing: constrains
- 36062a7e (2026-11-11, commit): "board (294) acknowledged the technical call: title and options changes re-ask, why/recommend/link-only edits keep the sent mark." — bearing: supports
- cca49a60 (2026-11-10, commit): "Manifest hashes were set to sha256sum of the new playbook (the manifest format) since remote regen does not return files; the wsrsrc drift tests and the Pi version-check test pass." — bearing: constrains
- d39b84d6 (2026-11-08, commit): "Whiteboard: wb:add and wb:patch fields (title, option, recommend, why, link) decode." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wswhiteboard/store.go, reask_test.go, three rsrc/whiteboard/whiteboard.md copies plus three rsrc/manifest.json hashes |
| scope.surface | public-interface | shipped whiteboard playbook Re-ask paragraph and observable wb:patch re-ask semantics; no exported Go symbol changes |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none; existing Item.ReaskedAt field reused |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/reask_test.go TestReaskStampsOnlyARealClassOrOptionsChangeOrAReopen; internal/wsdashboard/pageview_inbox_test.go; internal/wsrsrc drift tests |
| complexity.reuse_points | confirmed | applyPatch re-ask condition store.go#L948-L950 and sentMarks pageview_inbox.go#L144 read |
| complexity.side_effect_risk | low | one added comparison in the existing re-ask condition; sentMarks unchanged |
| risk.correctness | low | prev.Title vs next.Title comparison mirrors the existing class/options rule |
| risk.fit | moderate | playbook text must stay byte-identical across three packages with manifest hashes regenerated |
| risk.test | low | existing reask_test.go table extends directly |
| risk.security_or_contract | moderate | changes shipped re-ask semantics recorded in 261109-bug-ws-whiteboard-patch-cannot-change-item-class decisions |

## Phases

### Phase 1: title change counts as a re-ask

- `applyPatch` stamps `ReaskedAt` when the patched title differs from the
  stored one on an item open after the patch; update its doc comment and
  the `ReaskedAt` field comment.
- The whiteboard playbook's `Re-ask` paragraph (three byte-identical copies:
  `agents-plugin/rsrc/whiteboard/whiteboard.md`,
  `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`, and
  `agents-plugin-pi/rsrc/whiteboard/whiteboard.md#L139-L146`, each hashed in
  its package's `rsrc/manifest.json#L52`) names the
  title among the re-asking changes (on an open item) and drops it from the
  not-a-re-ask list; the sentence on undelivered answers says such an answer
  refers to the title and options it was sent against. Find the paragraph by
  its `Re-ask:` lead, not by line number, and regenerate all three
  `rsrc/manifest.json` hashes after the edit (whichever of this ticket and
  `261110-feat-ws-whiteboard-item-field-html` lands second carries the
  final hashes).

Verification:

- A store test: a title-only patch on an open item stamps `ReaskedAt`; a
  patch resending the same title, or editing only `wb:why`, `wb:recommend`,
  or links, does not; a title fix on a settled item stamps nothing.
- `TestReaskStampsOnlyARealClassOrOptionsChangeOrAReopen`
  (`reask_test.go`) asserts today that a title patch is not a re-ask: split
  that block so the title change stamps and why/link alone do not, and
  rename the test to match the new rule.
- The sent-mark tests stay green; run the wswhiteboard,
  wsdashboard, and mcp packages plus any playbook or wsflow mirror drift
  tests through `~/.local/bin/ws-rgo`.
