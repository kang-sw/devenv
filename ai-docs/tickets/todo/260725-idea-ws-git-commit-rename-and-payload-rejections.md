---
title: "ws/git.commit: cannot commit a staged ticket rename, and rejects large ai_context payloads with a misleading error"
related-mental-model:
  - mcp-runtime
  - plugin-runtime
sage-review-completeness: recommended
sage-review-design: completed
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

The third row shows there is no escape by omitting the ticket path either, so in
practice no `paths` value commits the promotion. Committed natively as
`89f11d4d`.

**Diagnosis correction.** An earlier reading of this matrix concluded that two
guards were jointly unsatisfiable. Source says otherwise, and the distinction
changes the fix. `Commit` reassigns `opts.Paths =
expandCommitPathsForTicketMoves(preStatus, opts.Paths)`
(`internal/wsgit/git.go:462`), which matches by ticket stem and appends the
pre-rename path; that expanded list then feeds **both**
`validateCommitStatus` (`:473`) and `c.verifier()` (`:480`). So:

- Rows 1 and 2 fail at the **verifier only**. The unrelated-staged-path guard is
  already satisfied by a destination-only `paths` value, precisely because the
  stem expansion supplied the old path for it (pinned by
  `TestCommitExpandsTodoToReadyTicketMovePathsByStem`).
- Row 3 is a different scenario, not a third symptom: with no ticket path passed,
  no stem expansion runs, so the staged rename genuinely is unrelated to the
  caller's paths. **That refusal is correct and must stay.**

The single necessary change is therefore what the *verifier* receives, since
`Verifier` (`git.go:45`) takes only `(root, paths)` and carries no index status
with which to make the distinction itself.

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

- Finding 1: stop handing index-delete-side paths to the verifier. `Commit`
  already holds the needed data — `ParseStatus` populates `FileStatus.OldPath`
  and `IndexStatus` (`git.go:82-88`), and both `preStatus` (`:461`) and the
  post-staging `status` (`:472`) are in scope — so a third `git diff --cached
  --name-status -M` invocation is one option, not the required shape.
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
`tickets.close`. Per the diagnosis correction above, this is a **single-guard**
fix: `validateCommitStatus` already works, and only the verifier must stop
receiving index-delete-side paths. Do not add rename-pair collapsing to
`validateCommitStatus` (it would be dead code, since stem expansion already
supplies the pair), and do not make the `[_index.md]`-only form succeed (that
refusal is correct).

Because `Verifier` (`git.go:45`) is `(root, paths) error` with no status
argument, the filtering has to happen at the call site in `Commit` — either by
passing the unexpanded caller paths, or by filtering the expanded list of entries
the index reports as delete-side. Choose deliberately: the two differ for a
caller who passes extra ticket paths explicitly.

Cover the staged **deletion** case in the same change. The same file-exists
guardrail (`wsdoc/tickets_verify.go:102`) fires for an outright staged ticket
deletion, where there is no destination path to verify instead; "skip
verification for index-delete-side paths" is the natural rule and covers both
rename and deletion, but the choice must be explicit rather than incidental.

Verification: promoting a ticket `todo/` -> `ready/` via `tickets.move` and
closing one to `.dropped/` via `tickets.close` both commit through
`ws/git.commit` with the destination path alone; an outright staged ticket
deletion commits; a genuinely unrelated staged ticket path is still refused and
the `[_index.md]`-only form still fails; content-only ticket edits keep working
unchanged. Regression coverage must include the close and delete paths, not just
promotion — this session confirmed close is affected.

### Phase 2: Report the real `ai_context` constraint

**Expect to find no in-repo size limit.** A design-review source audit found
none: no `maxLength` on `ai_context` in the `git.commit` schema
(`server.go:3802`), no length check in `normalizeCommitOptions`
(`git.go:498-526`), and `stringList` (`server.go:4739`) only drops non-string and
empty-string items. The one size mechanism in range — `bufio.NewScanner`'s
default 64KiB token cap (`server.go:143`, already logged as Technical Debt in
`mental-model/mcp-runtime.md`) — fails with `ErrTooLong`, which ends the read
loop and the connection, so it cannot produce a *successful* JSON-RPC response
carrying `ai_context requires at least one entry`; eight prose bullets are also
far below 64KiB. Corroborating: a later session sent eight long entries through
the same tool with no rejection (`cc67569d`).

The likelier explanation is that the field never arrived — host- or model-side
tool-input truncation, or the CLI mirror path where `cmd/ws-mcp/main.go:481`
does `_ = fs.Parse(args)` and silently discards parse errors, leaving
`aiContext` empty. The originating report captured no evidence that the server
received the call at all, so an implementer must not assume a fixable server-side
limit exists.

The deliverable is therefore **diagnostic first**: record received argument sizes
(e.g. through `runtime.debug_events`) so absent-field, empty-array, and
all-blank-entries become distinguishable, then make the error name which of those
actually occurred instead of collapsing them into an emptiness claim. If the
audit does surface a real limit, report it with the limit and the offending
field. Either way, do not resolve this by asking callers to write less:
`## AI Context` is the project's decision-rationale tier, so a ceiling that
silently pushes callers toward shorter rationale is a workflow regression, and
any limit that survives must be documented rather than discovered by bisection.

Independent of Phase 1 — no ordering dependency.

Verification: pinned in behavioral terms, because the originating payload was
never captured and cannot be reproduced byte-for-byte — a large valid array
commits; an absent field, an empty array, and an all-blank-entry array each
report their own distinct condition; a regression test fixes whichever behavior
is chosen so the emptiness message cannot silently start covering a second
condition again.
