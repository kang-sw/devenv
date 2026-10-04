---
title: "Investigate discoverability of Pi code-reviewer report write scopes"
---

# Investigate discoverability of Pi code-reviewer report write scopes

## Background

During the ws release-gate review on 2026-10-04, the lead rendered the
correctness partition and dispatched it using the returned prompt and tier
bindings. Spawn refused because a required report-file write grant had not
been supplied. Capture this dogfood surprise without selecting a repair.

## Outcome Ledger

### Verified Findings

- `ws/playbook.render(name: "code-review-correctness")` returned a prompt
  path, recommended tier `large`, model `gpt-6.1-sol`, and effort `high`.
  Its response did not state the required report write scope or allocate one.
- `ws-agent-spawn` with that prompt and no `write_scopes` returned exactly:
  `ws-pi-agent: code reviewer requires exactly one file write scope`.
- The lead review procedure instructs rendering and spawning partitions but
  does not identify this adapter-specific dispatch requirement in its
  reviewer-spawn step. The generic spawn schema describes `write_scopes`
  as optional.
- The lead subsequently allocated exact report paths through
  `ws/path.generate(kind: "review")` for the retry. Successful retry is not
  yet evidence recorded by this capture.

### Confirmed Decisions

None; this is evidence capture only.

### Proposals

None selected.

### Open Questions

- Where should the required report grant be discoverable before dispatch:
  rendered metadata, the adapter's tool contract, or a generic reviewer hook?
- How can discovery preserve read-only source access while allowing exactly
  the report-file write, without requiring error-driven retries?

### Rejected Alternatives

None.
