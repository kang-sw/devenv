---
title: "ws/git.commit: cannot commit a staged ticket rename, and rejects large ai_context payloads with a misleading error"
related-mental-model:
  - mcp-runtime
  - plugin-runtime
---

# ws/git.commit dogfood findings

Two independent surprises hit during a single 2026-07-25 ticketing session.
Both forced a fallback to native `git commit`, which loses the tool's
structured message assembly and its ticket/spec verification.

## Finding 1: cannot commit a staged ticket rename

Promoting a ticket (`git mv ai-docs/tickets/todo/<stem>.md
ai-docs/tickets/ready/<stem>.md`) and then committing through `ws/git.commit`
fails its own verification:

```text
ticket verify failed:
- [file-exists] ai-docs/tickets/todo/<stem>.md: no such file or directory
```

The tool resolves the PRE-rename path of a staged move and then applies a
file-exists check to it. Passing both the old and the new path in `paths` does
not help.

Impact: every `ready/` promotion — the exact operation the ticket system is
built around — cannot be committed through the workflow tool. Observed while
promoting `260725-bug-dashboard-terminal-platform-macos-unsupported` and
`260725-feat-dashboard-nav-row-two-line-open-state`; committed natively as
`95b067e1`.

Note the tool works fine for content-only ticket edits (`0a811fbb`,
`44fe6fba`, `c79feaee`), so the defect is specific to staged renames.

Re-reproduced the same day through the opposite entry point: `ws/tickets.close(stem, status: "done")` performs frontmatter write -> `git add` -> `git mv`, leaving a staged rename `ready/<stem>.md -> .done/<stem>.md`. The following `ws/git.commit` failed with the identical signature (`[file-exists] ai-docs/tickets/ready/<stem>.md: cannot read ticket file: ... no such file or directory`), whether the `ready/` path was included in `paths` or omitted entirely — with `paths: ["ai-docs/_index.md", "ai-docs/tickets/.done/<stem>.md"]` (no `ready/` path anywhere in the call) the error still named the `ready/` path. `ws/tickets.verify(paths: ["ai-docs/tickets/.done/<stem>.md"])` returned PASS on the same tree at the same moment. This confirms the embedded ticket-verify step derives its path set from the staged diff (the rename's OLD side) rather than from the caller's `paths` argument — sharpening the fix direction below from "the tool mishandles a caller-supplied old path" to "the tool ignores `paths` for this check and reconstructs paths from the index itself." Workaround: native `git commit -F <msgfile>` (`eef7c968` on `impl/nav-row-two-line-open-state-phase1`).

## Finding 2: large `ai_context` rejected as if it were empty

A call with a non-empty `ai_context` array of eight long entries was rejected
three times with:

```text
ai_context requires at least one entry
```

The array was not empty. Bisecting confirmed the trigger is payload size, not
emptiness: the identical call with a single short entry succeeded immediately
(`90f35827`). Shortening the eight entries to roughly a third of their length
also succeeded (`c79feaee`).

Impact is worse than the wasted retries. The error text names a condition that
is demonstrably false, so the natural debugging response is to inspect the
array contents rather than its size — and the successful workaround (write
less rationale) is the opposite of what the commit convention asks for, since
`## AI Context` is where decision rationale is supposed to live.

Consequence in practice: the commit whose message had been truncated to a probe
string had to be repaired with `git commit --amend -F <file>` afterwards
(`c1f938af`).

## Suggested direction (not designed)

- Finding 1: resolve staged renames through the index (`git diff --cached
  --name-status -M`) before applying file-exists checks, or skip the check for
  paths the index reports as renamed.
- Finding 2: report the real constraint. If there is a size limit, say what it
  is and which field exceeded it; if there is no intended limit, the rejection
  is a serialization bug sitting upstream of the emptiness check.
