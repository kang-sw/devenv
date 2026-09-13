---
title: agents-plugin/.codex-plugin/plugin.json ships uncovered migration vocabulary
related:
  260913-feat-cross-session-mailbox-wake: prior-art — Phase 2's round-1 review fix relocated the Codex Stop-hook manifest into .codex-plugin/, exposing that the neutrality scanner never covered that directory
---

# agents-plugin/.codex-plugin/plugin.json ships uncovered migration vocabulary

## Background

While closing round-1 review findings on
`260913-feat-cross-session-mailbox-wake` Phase 2, the Codex `hooks.json` was
relocated from `agents-plugin/hooks/` to `agents-plugin/.codex-plugin/`
(fixing a Claude auto-discovery leak). Extending
`agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py`'s
`TEXT_TREES` to cover the new `hooks.json` location surfaced that
`agents-plugin/.codex-plugin/` — the plugin manifest directory itself,
containing `plugin.json` — had never been in `TEXT_TREES` at all, at any
point before this ticket.

Once briefly pointed at the whole `.codex-plugin` directory (rather than the
one `hooks.json` file this ticket's fix actually needed to cover),
`plugin.json` failed the scan on three pre-existing lines using the phrase
"codex-first" (in `description`, `interface.longDescription`, twice), which
`test_shipped_surfaces_downstream_neutral.py`'s rule 4 (migration
vocabulary) flags — because it names this repo's Claude-to-Codex migration
status ("a Codex-first candidate before converging with the existing Claude
plugin"), not a fact a downstream installer of this plugin needs or can
verify.

This ticket's fix narrowed the new `TEXT_TREES` entry to the exact file
`agents-plugin/.codex-plugin/hooks.json` (the test script's matching loop
was extended to support an exact-file entry, not just a directory prefix)
specifically to avoid re-litigating this pre-existing gap under an unrelated
ticket. The gap itself — `plugin.json` shipping migration-status language
never checked by the downstream-neutrality scanner — is real and open.

## Investigation

Needed: a decision on whether `plugin.json`'s self-description should drop
"codex-first"/migration-status framing (Architecture Rule 4 reads as
absolute: "Shipped surfaces are downstream-first, non-negotiably"), and
whether `TEXT_TREES` should then be widened back to the whole
`.codex-plugin` directory once that text is fixed, so future manifest edits
get scanned too.

## Outcome Ledger

### Verified Findings

- `agents-plugin/.codex-plugin/plugin.json` lines 4, 20, and 21 (as of
  260913) each contain the phrase "codex-first", which
  `test_shipped_surfaces_downstream_neutral.py` rule 4 (migration
  vocabulary) flags when that directory is scanned.
- `TEXT_TREES` in that test file did not cover `agents-plugin/.codex-plugin`
  before this discovery; the gap predates the mailbox-wake ticket and is not
  something that ticket introduced.

### Confirmed Decisions

- **Judged by the no-downstream-leak principle (user, 2026-09-13):** a shipped
  surface must not leak this repository's own migration status downstream, so
  `plugin.json` drops the "codex-first"/migration-status framing and describes
  current capability instead. This resolves the Open Question below: Rule 4's
  "non-negotiably" does foreclose migration-status text in shipped manifests —
  "Codex-first candidate" is not acceptable shipped wording.
- Consequently, after the reword, widen `TEXT_TREES` back to the full
  `agents-plugin/.codex-plugin` directory so the manifest is scanned going
  forward (the mailbox-wake ticket narrowed it to the single `hooks.json` file
  only to avoid re-litigating this gap under an unrelated ticket).

### Proposals

- Reword `plugin.json`'s `description`/`interface.longDescription` to drop
  "codex-first" framing (describe current capability rather than migration
  status), then widen the narrowed `TEXT_TREES` entry back to the full
  `agents-plugin/.codex-plugin` directory and re-run the neutrality scanner.
  (Now backed by the Confirmed Decision above — this is the actionable shape.)

### Open Questions

- (Resolved 2026-09-13 — see Confirmed Decisions.) "Codex-first ws workflow
  plugin candidate" is not acceptable shipped text: Rule 4's "non-negotiably"
  forecloses naming this repo's migration status in a downstream-shipped
  manifest.

### Rejected Alternatives

- Fixing `plugin.json`'s wording as part of
  `260913-feat-cross-session-mailbox-wake` Phase 2 — rejected as out of
  scope for a hook-adapter ticket; the narrower exact-file `TEXT_TREES`
  entry lets that ticket's own fix land without deciding this separately
  owned question.
