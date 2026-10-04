---
title: "Pi manual compaction success event can report an earlier identical-summary entry"
related:
  261004-feat-pi-compaction-interactive-ux: discovered during lite review; adapter workaround and SDK regression landed here
---

# Pi manual compaction success event can report an earlier identical-summary entry

## Background

During display-only history implementation, independent review found that Pi's
manual `AgentSession.compact()` selects the success event's stored entry with
`getEntries().find(e => e.type === "compaction" && e.summary === summary)`.
Repeating a summary reports the first matching compaction, not the one just
appended. Consumers that associate state with that ID can lose their new
post-compaction entry during retain-none transcript rebuilding.

Evidence: installed `dist/core/agent-session.js` near `savedCompactionEntry`,
and the real SDK regression in
`agents-plugin-pi/test/compaction-history.test.ts` named
"installed manual compaction with identical summaries". The adapter resolves
the newest matching boundary on the active branch while checking reported
branch membership. No installed-host patch was made.

## Phases

### Phase 1: Confirm and track supported host correction

Investigate upstream/manual versus automatic event identity, report the host
bug, and adopt a supported corrected host version when available. Preserve
compatibility with the existing active-branch workaround and regression; do
not patch the installed host or claim the event ID is authoritative until
verified.
