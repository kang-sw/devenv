---
title: Ticket close-checker flags phase result written at #### instead of ### Result
---

# Ticket close-checker flags phase result written at #### instead of ### Result

## Observation (dogfood, 2026-09-17)

Closing `260917-bug-exec-mcp-windows-test-timing-margin-flake` (ready -> .done via
`git.commit`) emitted:

```
WARN [unresolved-phases] ...: closed with unresolved phase heading(s)
(no ### Result before close): ### Phase 1: Convert exec tests to poll-until-terminal
```

The phase was in fact resolved: the ticket-worker recorded its outcome as a
`#### Result (1da0789d) - 2026-09-17` sub-heading nested under
`### Phase 1: ...`. The close-checker's `unresolved-phases` rule looks for a
`### Result` (h3) and does not recognize the `#### Result` (h4) the worker
actually wrote, so it warns on a phase that has a result.

## Why it matters

A convention drift between two shipped surfaces:

- the ticket-worker playbook emits the per-phase result as `#### Result`
  (h4, nested under the h3 phase), and
- the `git.commit` close-checker's `unresolved-phases` rule scans for
  `### Result` (h3).

Both cannot be right. Every phased ticket a worker closes normally will trip
this WARN, training readers to ignore a gate that is supposed to catch a
genuinely unfinished phase. AGENTS.md's own prose is ambiguous too: it says
"append a `#### Edition`" under a result but refers to the result section as
"`### Result`".

## Open questions / directions

- Decide the canonical heading level for a per-phase result (likely `#### Result`
  nested under the phase, matching worker output and the Edition level), then
  align the close-checker to accept it — or, if `### Result` is canonical, fix
  the worker playbook and AGENTS.md wording instead.
- Whichever wins, the checker, the worker playbook, and AGENTS.md must agree;
  this is a shipped-surface consistency fix, so mirror obligations apply to any
  playbook text touched.

## Evidence

- Close commit `f1b898ae` advisory (this repo, 2026-09-17).
- Result heading in `ai-docs/tickets/.done/260917-bug-exec-mcp-windows-test-timing-margin-flake.md`.
