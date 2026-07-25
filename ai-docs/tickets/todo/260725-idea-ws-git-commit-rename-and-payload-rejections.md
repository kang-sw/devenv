---
title: "ws/git.commit: cannot commit a staged ticket rename, and rejects large ai_context payloads with a misleading error"
related-mental-model:
  - mcp-runtime
  - plugin-runtime
sage-review-completeness: recommended
sage-review-design: recommended
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

### Independent reproduction: it is a deadlock, not a bad argument shape

Hit again the same day on the `main` worktree while promoting
`260725-feat-ws-cli-mcp-fallback-surface`, this time after `ws/tickets.move`
staged the rename rather than a manual `git mv` — so the failure is not specific
to how the rename got staged. Full argument matrix tried:

| `paths` passed | Result |
|---|---|
| `[ready/<stem>.md, _index.md]` | `[file-exists] ai-docs/tickets/todo/<stem>.md: cannot read ticket file` |
| `[todo/<stem>.md, ready/<stem>.md, _index.md]` | identical failure |
| `[_index.md]` | `refusing to commit unrelated staged path "ai-docs/tickets/ready/<stem>.md"` |

The third row is the important addition: omitting the ticket path — the only
remaining escape — is refused by a *different* guard. So the verify gate and the
unrelated-staged-path guard are individually reasonable and jointly
unsatisfiable, and no `paths` value exists that commits the promotion. Committed
natively as `89f11d4d`.

This also means the deadlock pushes callers off the gate that
`260723-feat-ticket-write-verify-commit-gate` deliberately made non-bypassable:
the only way through is the native-git fallback that gate exists to prevent.

Scope confirmed to be every staging path, not just promotion: `ws/tickets.close`
(dropping `260725-bug-git-commit-deadlocks-after-tickets-move-rename` to
`.dropped/`) reproduces the identical `[file-exists]` failure on the pre-move
`idea/` path. So promotion, triage, and closure — every status transition the
ticket system defines — are all uncommittable through `ws/git.commit`.

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
  paths the index reports as renamed. Whichever fix lands must also satisfy the
  unrelated-staged-path guard for the same call, since the reproduction above
  shows relaxing only one of the two guards leaves the deadlock intact.
- Finding 2: report the real constraint. If there is a size limit, say what it
  is and which field exceeded it; if there is no intended limit, the rejection
  is a serialization bug sitting upstream of the emptiness check.

## Spec Impact

- Target spec area: `mcp-tools.md`, for the `git.commit` argument contract — which
  `paths` values are accepted when the index holds a rename, and what the
  `ai_context` field actually constrains.
- Expected caller-visible change: a staged ticket rename becomes committable
  through `ws/git.commit` (today no `paths` value succeeds), and an `ai_context`
  rejection names the condition that actually failed instead of reporting
  emptiness for a non-empty array.
- Contract-first spec: no. Both are defects against the already-documented
  contract rather than new caller-facing behavior, so the spec is corrected at
  closeout to match what the fixed tool accepts.

## Phases

### Phase 1: Commit a staged ticket rename

Make `ws/git.commit` accept a status transition staged by `tickets.move` or
`tickets.close`. Both guards must be satisfied by the same call: the ticket
verify gate must stop applying a file-exists check to the delete side of a
rename (resolving staged renames through the index, e.g.
`git diff --cached --name-status -M`), **and** the unrelated-staged-path guard
must treat a renamed pair as one path so that passing only the destination is
not rejected. Relaxing either alone leaves the deadlock intact — that is what
the three-row argument matrix above demonstrates.

Decide and document which `paths` value is canonical for a rename (destination
only is the natural choice, since that is the file that exists), and make the
other accepted forms either work or fail with a message that names the canonical
form.

Verification: promoting a ticket `todo/` -> `ready/` via `tickets.move` and
closing one to `.dropped/` via `tickets.close` both commit through
`ws/git.commit` with the destination path alone; a genuinely unrelated staged
ticket path is still refused; content-only ticket edits keep working
unchanged. Regression coverage must include the close path, not just promotion —
this session confirmed both are affected.

### Phase 2: Report the real `ai_context` constraint

Establish first whether a size limit is intended. If it is, reject with a message
naming the limit and the field that exceeded it; if it is not, fix the
serialization defect sitting upstream of the emptiness check so a large valid
array is accepted. Do not resolve this by lowering what callers may write:
`## AI Context` is the project's decision-rationale tier, so a size ceiling that
silently pushes callers toward shorter rationale is a workflow regression, and
any limit that survives must be documented rather than discovered by bisection.

Independent of Phase 1 — no ordering dependency.

Verification: the eight-long-entry payload that failed three times in the
originating session is accepted, or rejected with a message that states the real
constraint; a genuinely empty array still reports emptiness; a regression test
pins whichever behavior is chosen so the emptiness message cannot silently start
covering a second condition again.
