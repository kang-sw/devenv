# Plan: 260726-feat-verify-ticket-graph-advisories — Phase 1: Surface verify warnings through the git.commit response

## Relevant Ticket Contract

- Widen the verify-to-commit channel so `ws/git.commit`'s response carries
  advisory text alongside the commit result, without letting advisories affect
  `OK` and without importing `internal/wsdoc` into `wsgit`
  ({#260720-wsdoc-commit-boundary}).
- Advisories must arrive inside `wsgit.CommitResult`, so the `wsgit.Verifier`
  boundary must carry them (plain-error shape is not enough).
- Both `ws/git.commit` entry points (MCP dispatch and ws-cli via
  `mcp.VerifyAdapter`) must stay gated identically — the channel widening
  happens once, at the `wsgit.Verifier` boundary, not on a second path.
- Severity/blocking model is unchanged: `Warnings` never affect `OK`, and this
  phase adds no new check.
- Verification boundary (from the ticket): an existing warning class
  (`unresolved-phases` on close, or `spec-address` on a `ready/` ticket)
  appears in `ws/git.commit`'s response, the commit still lands, and `OK` is
  unchanged.
- `git.commit` text-mode trailer order is fixed: commit output -> todo
  re-injection block -> session-key tip
  ({#260708-git-commit-session-key-tip}); a new trailer is rendered as part of
  the "commit output" stage (inside `formatGitCommit`), which already runs
  before the todo re-injection and the tip call, so no reordering of the
  dispatch code is required.
- JSON-mode answer (must be stated explicitly per the ticket): advisories are
  **text-mode only**, following the todo re-injection precedent
  ({#260626-git-commit-todo-reinjection}). `format: "json"` does **not** carry
  them — enforced with a `json:"-"` tag on the new `CommitResult.Advisories`
  field so both JSON emission sites (`internal/mcp/server.go`'s
  `toolJSONResponse` and `cmd/ws-mcp/main.go`'s `printJSONOrFatal`, both plain
  `encoding/json.Marshal`) omit it automatically with no per-call-site branch.
- Carrier shape (per ticket Prior Art, "do not implement any Phase 2 check"
  but "design the carrier so Phase 2 can use it without reshaping it again"):
  widen `wsgit.Verifier` from `func(root string, paths []string) error` to
  `func(root string, paths []string) ([]string, error)` — advisories as
  pre-formatted, already-multi-line-capable text lines, order-preserving, no
  path/guardrail attribution baked into the type. Add
  `CommitResult.Advisories []string `json:"-"`` as the matching field. This
  channel is generic text lines; Phase 2 can grow what it puts in the slice
  (board-block text, `FIX:`/`CHECK:` integrity lines) without touching the
  `wsgit` boundary again. Phase 1 does **not** add a `wsdoc.VerifyResult.Advisories`
  field — nothing populates it yet in this phase, and `verifyAdapter` (which
  already lives in `internal/mcp`, not `wsdoc`) is the natural place to format
  `VerifyResult.Warnings` into the `[]string` wire shape. Phase 2 is free to
  add a `wsdoc.VerifyResult.Advisories` field later and have `verifyAdapter`
  merge it in — that is an internal `verifyAdapter` change, not a further
  `wsgit`-boundary reshape.
- A graph-load failure degrading to silence (never a commit veto) is a Phase 2
  concern (Phase 2 introduces the whole-board load that can fail on an
  unrelated malformed file); Phase 1 introduces no new failure source, so
  `verifyAdapter`'s existing error-propagation behavior for `TicketVerify`'s Go
  error (caller-input-only today) is unchanged.

## Out of Scope

- Any of Phase 2's checks: ancestor walk, `## Parent Board` block,
  cross-reference integrity checks (`parent:`/`related:` resolution, cycle
  detection), `FIX:`/`CHECK:` message shapes, the amend-recipe sentence.
- Adding a `wsdoc.VerifyResult.Advisories` field — Phase 2's job, once
  something needs to populate it.
- Any spec text change — `{#260723-git-commit-ticket-verify-gate}` already
  states the soft-warning-surfaces-at-commit contract; Phase 1 closes a
  spec/implementation divergence, it does not add new spec surface.
- Staged-rename detection / `260724-bug-ws-git-commit-verify-fails-on-staged-rename`.
- Amend support in `ws/git.commit`.

## Codebase Findings

- `agents-plugin-tool/internal/wsgit/git.go#L36-L45` — `Verifier` type and its
  doc comment explaining the `internal/wsdoc`-free boundary
  ({#260720-wsdoc-commit-boundary}); the comment should gain one clause noting
  the widened signature also carries non-blocking advisory text now.
- `agents-plugin-tool/internal/wsgit/git.go#L61-L66` — `(c Client) verifier()`
  supplies the nil-safe default; its no-op literal `func(string, []string) error { return nil }`
  becomes `func(string, []string) ([]string, error) { return nil, nil }`.
- `agents-plugin-tool/internal/wsgit/git.go#L434-L439` — `CommitResult` struct;
  add `Advisories []string `json:"-"`` after `TicketChanges`.
- `agents-plugin-tool/internal/wsgit/git.go#L488-L505` — `Client.Commit`'s
  verifier invocation (`c.verifier()(root, verifyPaths)`) and final
  `CommitResult{...}` construction; both need updating for the two-return-value
  call and to thread `advisories` into the returned struct. The call currently
  sits inside `if verifyPaths := filterIndexDeleteSidePaths(...); len(verifyPaths) > 0 { ... }` —
  when that block is skipped (outright deletion), `advisories` stays nil,
  which is correct (nothing was verified, so nothing to advise).
- `agents-plugin-tool/internal/mcp/server.go#L2692-L2707` — `verifyAdapter`,
  the sole implementation of the `wsgit.Verifier` contract for the MCP path
  (also reused by the CLI path via `mcp.VerifyAdapter`). Today it returns
  `nil` whenever `result.OK`, discarding `result.Warnings`. Change signature to
  `func verifyAdapter(root string, paths []string) ([]string, error)`; on the
  veto branch return `nil, fmt.Errorf(...)` (unchanged text); on the OK branch,
  format each `result.Warnings` entry into a line and return that slice (nil
  when there are no warnings) plus `nil` error.
- `agents-plugin-tool/internal/mcp/server.go#L2708-L2726` — `formatTicketVerify`
  is the existing rendering precedent for the standalone `tickets.verify` tool:
  `fmt.Fprintf(&b, "  WARN [%s] %s: %s\n", warning.Guardrail, warning.Path, warning.Message)`.
  Reuse this exact per-warning text shape (`WARN [%s] %s: %s`, no amend
  sentence, no `next_instruction`) inside `verifyAdapter` so a warning reads
  identically whether seen via `tickets.verify` or via `git.commit` — matching
  `{#260723-tickets-verify-tool}`'s "same verdict for identical input"
  guarantee (verdict/content identical; only presentation site differs).
- `agents-plugin-tool/internal/mcp/server.go#L2466-L2496` — `formatGitCommit`,
  the argument-free formatter both entry points render through. Add an
  `advisories:` block (rendered only when `len(result.Advisories) > 0`) after
  the existing `ticket_changes:` block, before the function returns. This
  keeps advisories inside "commit output," which is the first trailer stage
  per {#260708-git-commit-session-key-tip}'s fixed order — no change needed at
  the call site (`server.go#L1092` `commitText := formatGitCommit(result)`) or
  to the later `appendSessionKeyTip` call at `server.go#L1100`.
- `agents-plugin-tool/internal/mcp/format.go#L25-L27` and `#L53-L60` —
  `FormatGitCommit` (no change needed; still argument-free, still just calls
  `formatGitCommit`) and `VerifyAdapter` (signature must widen in lockstep
  with `verifyAdapter`: `func VerifyAdapter(root string, paths []string) ([]string, error) { return verifyAdapter(root, paths) }`).
- `agents-plugin-tool/cmd/ws-mcp/main.go#L466-L509` — `gitCommit` CLI handler
  constructs `wsgit.Client{Runner: wsgit.ExecRunner{}, Verifier: mcp.VerifyAdapter}`
  and calls `mcp.FormatGitCommit(result)` / `printJSONOrFatal`. No code change
  needed here — `mcp.VerifyAdapter`'s widened signature satisfies the widened
  `wsgit.Verifier` type automatically, and both the text formatter and the
  JSON marshal path pick up the new field/behavior without a call-site edit.
- `agents-plugin-tool/internal/mcp/server.go#L1036-L1101` — the `case "git.commit":`
  dispatch. No change needed beyond what `formatGitCommit` already does
  internally: `wsgit.Client{Runner: wsgit.ExecRunner{}, Verifier: verifyAdapter}.Commit(...)`
  at `#L1073` builds fine against the widened `Verifier` type without edits
  since `verifyAdapter`'s new signature is a structural match.
- `agents-plugin-tool/internal/wsdoc/tickets_verify.go#L19-L26` — `VerifyResult`
  doc comment already states "`Warnings` never affect `OK`"; this stays
  authoritative and unchanged by Phase 1 (no new field added here, see Out of
  Scope).
- `agents-plugin-tool/internal/wsdoc/tickets_verify.go#L130-L143` — the two
  live warning emitters this phase must surface: `addWarning("unresolved-phases", ...)`
  (status `.done`/`.dropped`) and `addWarning("spec-address", ...)` (status
  `ready`). No change needed in this file; Phase 1 only widens the channel
  these warnings already flow into.
- **Risk signal — test breakage surface.** `agents-plugin-tool/internal/wsgit/git_test.go`
  has 8 inline `Verifier: func(...) error { ... }` closures that construct
  `wsgit.Client` literals directly (lines 737, 821, 864, 905, 955, 994, 1031,
  1065). Every one must be updated to the new two-return-value signature
  (`func(...) ([]string, error) { ...; return nil, <err-or-nil> }`) or the
  package fails to compile. None of them assert on advisories today, so the
  fix is purely mechanical (add `nil,` before each existing return value).
  Confirmed via `grep -n "Verifier: func" internal/wsgit/git_test.go`.
- Confirmed no other call sites need updating: `grep -rln "Verifier:|verifyAdapter|VerifyAdapter|wsgit\.Verifier" $(find agents-plugin-tool -name '*.go')`
  returns exactly `internal/mcp/server.go`, `internal/mcp/format.go`,
  `cmd/ws-mcp/main.go`, `internal/wsgit/git_test.go` — no other file
  constructs a `wsgit.Verifier` or calls `verifyAdapter`/`VerifyAdapter`.
- Confirmed no `reflect.DeepEqual` compares a whole `wsgit.CommitResult` or
  `wsdoc.VerifyResult` struct anywhere in the tree (`grep -n "DeepEqual.*[Rr]esult\b"` /
  `VerifyResult{` both come back empty of full-literal comparisons), so adding
  `CommitResult.Advisories` cannot break an existing struct-equality
  assertion.
- `agents-plugin-tool/internal/wsdoc/tickets_verify_test.go` — the 19
  `t.TempDir()` fixture tests (e.g. `TestTicketVerifySpecAddressIsSoftWarnOnly`
  at line 261, `TestTicketVerifyUnresolvedPhaseIsSoftWarnOnly` at line 342)
  already assert `result.Warnings` shape field-by-field, not via `DeepEqual`
  on the struct. **No change needed in this file** — Phase 1 does not touch
  `wsdoc.TicketVerify` or `VerifyResult` at all.
- `agents-plugin-tool/internal/mcp/server_test.go#L1708-L1826` and
  `agents-plugin-tool/cmd/ws-mcp/main_test.go#L351-L385` — existing `git.commit`
  integration tests use ticket paths in `todo/` status (server_test.go) or no
  ticket path at all (main_test.go), so none of them cross the `ready`/`.done`/
  `.dropped` status gates that produce `spec-address`/`unresolved-phases`
  warnings. These tests are unaffected and need no changes; they also serve as
  the negative-case proof that "no advisories" still renders identical text
  output (no new block appears when `Advisories` is empty).

## Implementation Plan

1. `agents-plugin-tool/internal/wsgit/git.go`:
   - `#L45` widen `type Verifier func(root string, paths []string) error` to
     `type Verifier func(root string, paths []string) ([]string, error)`; extend
     the doc comment (`#L36-L44`) with one sentence noting the second return
     carries non-blocking advisory text, still `internal/wsdoc`-free.
   - `#L61-L66` update the nil-default in `verifier()` to
     `func(string, []string) ([]string, error) { return nil, nil }`.
   - `#L434-L439` add `Advisories []string `json:"-"`` to `CommitResult`
     (after `TicketChanges`), with a short comment: "text-mode only; see
     {#260626-git-commit-todo-reinjection} precedent — never serialized to
     JSON."
   - `#L488-L492` change the verifier call site to capture both return values:
     ```go
     var advisories []string
     if verifyPaths := filterIndexDeleteSidePaths(status, opts.Paths); len(verifyPaths) > 0 {
         var err error
         advisories, err = c.verifier()(root, verifyPaths)
         if err != nil {
             return CommitResult{}, err
         }
     }
     ```
   - `#L505` add `Advisories: advisories` to the final `CommitResult{...}`
     literal.
2. `agents-plugin-tool/internal/mcp/server.go`:
   - `#L2692-L2707` `verifyAdapter`: change signature to
     `func verifyAdapter(root string, paths []string) ([]string, error)`.
     Keep the `err != nil` and `!result.OK` branches returning `nil, err` /
     `nil, fmt.Errorf(...)` (unchanged error text). On the pass branch, build
     `advisories := make([]string, 0, len(result.Warnings))`, append
     `fmt.Sprintf("WARN [%s] %s: %s", w.Guardrail, w.Path, w.Message)` per
     warning, and `return advisories, nil` (nil slice is fine when there are
     no warnings — `make(...,0,0)` vs `nil` both range/len as empty, so either
     is acceptable; prefer returning `nil` when `len(result.Warnings) == 0` to
     keep `CommitResult.Advisories` unambiguously empty rather than an
     allocated-but-empty slice).
   - `#L2466-L2496` `formatGitCommit`: after the `ticket_changes:` block and
     before `return b.String()`, add:
     ```go
     if len(result.Advisories) > 0 {
         b.WriteString("advisories:\n")
         for _, advisory := range result.Advisories {
             fmt.Fprintf(&b, "  %s\n", advisory)
         }
     }
     ```
3. `agents-plugin-tool/internal/mcp/format.go`:
   - `#L58-L60` widen `VerifyAdapter` to
     `func VerifyAdapter(root string, paths []string) ([]string, error) { return verifyAdapter(root, paths) }`.
4. `agents-plugin-tool/cmd/ws-mcp/main.go` — no edits required; `gitCommit`
   (`#L466-L509`) references `mcp.VerifyAdapter` and `mcp.FormatGitCommit` by
   value/call, both of which compile against the widened types without a
   call-site change.
5. `agents-plugin-tool/internal/wsgit/git_test.go` — update all 8 `Verifier:
   func(...)` closures (lines 737, 821, 864, 905, 955, 994, 1031, 1065) to the
   two-return-value form. Concretely: change each closure's declared return
   type from `error` to `([]string, error)`, and change every `return <expr>`
   inside them to `return nil, <expr>` (the veto case at line ~743,
   `return verifyErr`, becomes `return nil, verifyErr`; every `return nil`
   pass-through case becomes `return nil, nil`).
6. Run `gofmt -l` / `go vet` implicitly via the verification plan below to
   catch any missed call site.

## Verification Plan

- `cd agents-plugin-tool && go build ./...` — catches any remaining signature
  mismatch across the four touched files plus `git_test.go`.
- `cd agents-plugin-tool && go test ./internal/wsgit/... -run TestCommit -v`
  — exercises all 8 updated `Verifier` closures plus
  `TestCommitProceedsWhenVerifierNilDefaultsToNoOp` (nil-default path) and
  `TestCommitBlockedByVerifierNeverReachesCommit` (veto-still-vetoes path).
- `cd agents-plugin-tool && go test ./internal/wsdoc/... -run TestTicketVerify -v`
  — confirms the two source warnings (`TestTicketVerifySpecAddressIsSoftWarnOnly`,
  `TestTicketVerifyUnresolvedPhaseIsSoftWarnOnly`) are unaffected (Phase 1
  touches no code in this package).
- **New test to add** (fixture pattern matches
  `internal/wsgit/git_test.go`'s existing `sequenceRunner`/`t.TempDir()` style,
  and separately `internal/mcp/server_test.go`'s JSON-RPC harness style used at
  `#L1708-L1826`): a `wsgit`-level or `internal/mcp`-level test that stages a
  real ticket transitioning into `.done/` with an unresolved (non-`[dropped]`)
  `### Phase N:` heading (the same fixture shape
  `TestTicketVerifyUnresolvedPhaseIsSoftWarnOnly` uses in `wsdoc`), commits it
  through `wsgit.Client{Verifier: verifyAdapter}.Commit(...)` (or through the
  MCP `git.commit` dispatch), and asserts three things: (1) the commit
  succeeds (`err == nil`, `result.Hash != ""`), (2)
  `result.Advisories`/the rendered text contains a `WARN [unresolved-phases] ...`
  line, (3) `format: "json"` output for the same scenario does **not** contain
  `"advisories"` or the warning text (proving the `json:"-"` tag holds). This
  is the concrete instance of the ticket's stated verification boundary and
  should live in `internal/mcp/server_test.go` (it needs the full dispatch +
  ticket-file fixture, closer to the existing `git.commit` integration tests
  at `#L1708` than to `wsgit`'s unit-level `sequenceRunner` tests) or
  alternatively as a new `wsgit`-level test if a lighter fixture suffices —
  executor's call based on how much MCP dispatch scaffolding the assertion
  actually needs.
- `cd agents-plugin-tool && go test ./cmd/ws-mcp/... -run TestGitCommit -v` —
  regression guard for the CLI text-mode path (`TestGitCommitCLIRendersMentalModelNotes`
  and any JSON-mode CLI test), confirming no behavior change for the
  no-advisories case.
- `cd agents-plugin-tool && gofmt -l .` — must return nothing.

## Escalations

- None.
