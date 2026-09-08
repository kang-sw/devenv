# Plan: 260908-bug-sage-gate-stale-completed-has-no-rerun-path — Phase 1: Let the freshness verdict be answered

## Relevant Ticket Contract

- Decision 1: `answer` must resolve `check_review_required` the same way it
  resolves a `recommended` ask. `answer: yes` returns `run` with `reviewers`
  set to the listed stale stages, `mode` = `combined` when two stages are
  listed / `standalone` when one is, and the non-waivable `advisory`.
  `answer: no` falls through to ordinary posture resolution for the
  remaining stages — a pending `required` or `recommended` stage must never
  be swallowed by the decline.
- Decision 2: declining (`answer: no`) writes nothing — posture stays
  `completed`, the `-reviewed` digest stays stale, `tickets.verify` keeps
  warning until a fresh `tickets.sage_stamp`.
- Constraints: touch `sageGateFreshnessResult` and its call sites in
  `SageGate` (`tickets_sage.go`); `resolveStage` is unchanged for
  `recommended`; the freshness `run` result is built through `stageOutcome`
  so it carries the advisory; `sageGateNextInstruction`'s
  `check_review_required` branch gains the same re-invoke sentence the
  `ask` branch renders; extend `TestSageGateWarnsWhenCompletedReviewIsStale`
  (or sit beside it) with a `yes` case and a `no` case (pending `required`
  completeness stage, asserting `run` and byte-identical frontmatter), plus
  a next-instruction text test; spec `ai-docs/spec/mcp-tools.md`
  `{#260720-sage-gate-record-tools}` gains one sentence per answer in the
  freshness paragraph and lists the freshness `run` in the advisory
  sentence; the `ask` contract sentence is unchanged.
- Out-of-scope items named by the ticket: digest computation, the legacy
  Git-walk baseline, `tickets.verify`'s warning text, playbook text
  (`lead-write-ticket` already follows `next_instruction` verbatim).

## Out of Scope

- `tickets.verify`'s freshness warning (`agents-plugin-tool/internal/wsdoc/tickets_verify.go:173`) — ticket names it explicitly out of scope.
- The legacy Git-walk baseline path (`sageReviewStageBaseline`, `tickets_sage_freshness.go:88-138`) — untouched, only consulted when no `-reviewed` digest exists.
- `resolveStage`'s `recommended` handling (`tickets_sage.go:238-270`) — unchanged; the `ask` contract stays one-answer-per-question.
- `lead-write-ticket` playbook prose — the ticket states it already follows the gate's returned `next_instruction`.
- Related mental model `mcp-runtime` — not named in Constraints' touch list; no edit found necessary for this phase.

## Codebase Findings

- `agents-plugin-tool/internal/wsdoc/tickets_sage.go#L126-L142` — `SageGate` already parses and validates `opts.Answer` into a local `answer` variable (`""`/`"yes"`/`"no"`) before any freshness call; no new plumbing needed to get the answer to the call sites, only to pass it through.
- `agents-plugin-tool/internal/wsdoc/tickets_sage.go#L167,#L186,#L208` — the three `sageGateFreshnessResult(root, ticketRel, stages)` call sites (not four, despite the ticket's Constraints count — see Escalations note). Each follows the same shape: `if result, err := sageGateFreshnessResult(...); err != nil { return err } else if result.Action != "" { return result, nil }`, then falls through to `sageGateStandalone`/a direct `skip`. Only the third site (`#L196-219`, both stages required, design already terminal) has a downstream fallthrough onto a *different* stage (`completeness`) than the one freshness just checked — this is the site where an unreset `answer` could wrongly decline/accept a still-pending `completeness` stage using an answer that was meant for the freshness question.
- `agents-plugin-tool/internal/wsdoc/tickets_sage.go#L235-270` (`resolveStage`) — the `recommended` case reads `answer` (`yes`/`no`/default) to ask/accept/decline; the `required` case ignores `answer` entirely and always runs. This confirms the swallow risk in Decision 1 is real only when the downstream stage posture is `recommended` (a `required` downstream stage is safe either way since it ignores `answer`), but the fix should reset unconditionally for symmetry/simplicity rather than special-casing by posture.
- `agents-plugin-tool/internal/wsdoc/tickets_sage.go#L294-373` (`sageGateCombined`) — existing precedent for exactly this "answer consumed by one stage, reset to `""` before resolving the next stage" pattern, at `#L338` (`answer = "" // consumed by design; completeness asks separately`). The freshness fix should mirror this pattern rather than invent a new one.
- `agents-plugin-tool/internal/wsdoc/tickets_sage.go#L281-292` (`gateResultFromStage`) — existing `stageOutcome -> SageGateResult` mapper, but it hard-codes `Reviewers: []string{reviewer}` (one reviewer). The freshness `run` result needs `Reviewers` set to the (possibly two-element) stale-stage list, so this helper needs a variant/generalization (e.g. accept `[]string` reviewers) rather than reuse as-is — a real but small reuse gap.
- `agents-plugin-tool/internal/wsdoc/tickets_sage_freshness.go#L20-31` (`sageGateFreshnessResult`) — currently has no `answer` parameter and only ever returns `check_review_required` or a zero-value (`Action: ""`) sentinel meaning "not stale." Adding `answer` here requires distinguishing three post-check states for the caller: not stale (proceed with original answer), stale+declined (proceed but reset answer to `""`), stale+accepted or stale+unanswered (return the result immediately). A single `SageGateResult` return value cannot carry this distinction cleanly — recommend widening the return signature (e.g. an added `consumed bool`, true whenever `len(freshness.Stages) > 0`) so all three call sites can share one small caller-side pattern: `if consumed { if result.Action != "" { return result, nil }; answer = "" }`.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L474-481` — `sageReviewNonWaivableAdvisory` constant, the exact advisory string the freshness `run` result must carry (matches Decision 1's "carries the non-waivable advisory like every other run").
- `agents-plugin-tool/internal/mcp/server.go#L2839-2844` (`sageGateNextInstruction`) — `ask` branch: `"next_instruction: Relay ask_prompt to the user, then call tickets.sage_gate again with the same stem/landing plus answer=yes|no."`; `check_review_required` branch today: `"next_instruction: Inspect the ticket diff against review_baseline and decide whether to rerun the listed sage review stage(s); do not treat the prior completed review as current until that decision is made."` — needs the re-invoke clause added while preserving the existing substring `"decide whether to rerun the listed sage review stage(s)"` (an existing test asserts on it, see next finding).
- `agents-plugin-tool/internal/mcp/tickets_sage_test.go#L111-127` (`TestFormatSageGateRoundTrip`) — asserts `checkOut` contains `"decide whether to rerun the listed sage review stage(s)"`; this substring must survive the `sageGateNextInstruction` edit or this test breaks. The ticket's "extend ... the next-instruction text" test item lives here (not in `internal/wsdoc/`), so `go test ./internal/mcp/ -count=1` should also be run even though Phase 1's stated Verification only names `./internal/wsdoc/`.
- `agents-plugin-tool/internal/wsdoc/tickets_sage_test.go#L394-424` (`TestSageGateWarnsWhenCompletedReviewIsStale`) and `#L468-488` (`TestSageGateFreshnessIsStageSpecific`) — reusable fixtures. `TestSageGateFreshnessIsStageSpecific`'s exact setup (design `completed`+stale, completeness `required`) is the scenario Constraints names for the `no`-case test ("the completeness `run` result and byte-identical frontmatter").
- `agents-plugin-tool/internal/wsdoc/tickets_sage_test.go#L77-88` (`writeSageTicket`) and `#L369-388` (`initSageFreshnessRepo`/`commitSageFreshnessRepo`/`runGitOutputSage`) — existing helpers for building the staleness fixture (write, commit, edit, commit-or-leave-uncommitted); reuse directly for the two new test cases, no new helper needed.
- `ai-docs/spec/mcp-tools.md#L1613-1639` — the freshness paragraph named in Constraints ("differing returns `check_review_required` ..." is at `#L1619`); add one sentence describing `answer: yes` -> `run` (stale stages, mode by count, advisory) and one sentence describing `answer: no` -> no write, remaining stages resolve normally, `tickets.verify` keeps warning.
- `ai-docs/spec/mcp-tools.md#L1653-1664` — the advisory paragraph; the sentence "It rides every `run` result — `required`'s direct run and a `recommended` stage's accepted run alike —" (`#L1658-1659`) needs the freshness `run` added to that list.
- `ai-docs/spec/mcp-tools.md#L1606` — "the caller re-invokes with `answer` (`yes`/`no`)" sentence for `ask`; Constraints say this stays unchanged — confirmed no edit needed here.

## Implementation Plan

1. `agents-plugin-tool/internal/wsdoc/tickets_sage_freshness.go#L20-31` — change `sageGateFreshnessResult` to accept `answer string` and return an extra signal that the caller can use to know the freshness question was consumed (e.g. `(SageGateResult, bool, error)` with the bool true whenever `len(freshness.Stages) > 0`). Inside, when stages are stale:
   - `answer == "yes"`: build a `stageOutcome{action: "run", advisory: sageReviewNonWaivableAdvisory}` and map it to a `SageGateResult{Action: "run", Reviewers: freshness.Stages, Mode: <"combined" if len==2 else "standalone">, Advisory: ...}` (generalize `gateResultFromStage`, `tickets_sage.go#L281-292`, to accept a reviewer slice, or add a small sibling that does).
   - `answer == "no"`: write nothing; return a zero-action result (so the caller does not return early) with `consumed = true`.
   - `answer == ""`: unchanged — return the existing `check_review_required` result with `consumed = true`.
2. `agents-plugin-tool/internal/wsdoc/tickets_sage.go#L167,#186,#208` — update all three call sites to the new signature: `if result, consumed, err := sageGateFreshnessResult(root, ticketRel, stages, answer); err != nil { return SageGateResult{}, err } else if consumed { if result.Action != "" { return result, nil }; answer = "" }`. The `answer = ""` reset matters concretely at the third call site (`#L196-219`), where the fallthrough resolves a *different* stage (`completeness`); it is a no-op at the first two sites (single-stage, terminal posture ignores `answer`), but applying it uniformly avoids special-casing and matches the existing `sageGateCombined` precedent (`#L338`).
3. `agents-plugin-tool/internal/mcp/server.go#L2843-2844` — edit the `check_review_required` case of `sageGateNextInstruction` to add the re-invoke sentence, keeping the existing substring `"decide whether to rerun the listed sage review stage(s)"` intact, e.g.: `"next_instruction: Inspect the ticket diff against review_baseline and decide whether to rerun the listed sage review stage(s), then call tickets.sage_gate again with the same stem/landing plus answer=yes|no; do not treat the prior completed review as current until that decision is made." + sageGatePostureUncommittedNote`.
4. `agents-plugin-tool/internal/wsdoc/tickets_sage_test.go` — beside `TestSageGateWarnsWhenCompletedReviewIsStale` (`#L394-424`), add:
   - a `yes` case reusing the same stale-both-stages fixture, calling `SageGate(..., SageGateOptions{..., Answer: "yes"}, ...)`, asserting `Action == "run"`, `Reviewers == []string{"design","completeness"}`, `Mode == "combined"`, and `Advisory == sageReviewNonWaivableAdvisory`.
   - a `no` case reusing `TestSageGateFreshnessIsStageSpecific`'s fixture (`#L468-488`: design `completed`+stale, completeness `required`), calling with `Answer: "no"`, asserting `Action == "run"`, `Reviewers == []string{"completeness"}`, `Mode == "standalone"`, and that the ticket file bytes are byte-identical before and after the call (read the file, snapshot, call, re-read, compare).
5. `agents-plugin-tool/internal/mcp/tickets_sage_test.go#L111-127` (`TestFormatSageGateRoundTrip`) — add an assertion that the `check_review_required` rendered text also contains the new re-invoke sentence (`"call tickets.sage_gate again with the same stem/landing plus answer=yes|no"`), alongside the existing `"decide whether to rerun the listed sage review stage(s)"` assertion.
6. `ai-docs/spec/mcp-tools.md` — in the freshness paragraph (`#L1613-1639`, near `#L1619`), add one sentence for `answer: yes` (returns `run`, `reviewers` = the listed stale stages, `mode` by stage count, carries the advisory) and one sentence for `answer: no` (writes nothing, posture and digest stay as they are, `tickets.verify` keeps warning, remaining pending stages still resolve normally). In the advisory paragraph (`#L1653-1664`), extend the "It rides every `run` result — ... —" sentence (`#L1658-1659`) to list the freshness `run` alongside `required`'s direct run and a `recommended` stage's accepted run.

## Verification Plan

- `go test ./internal/wsdoc/ -count=1` in `agents-plugin-tool/` (ticket-specified).
- `go test ./internal/mcp/ -count=1` in `agents-plugin-tool/` — not named in the ticket's Verification bullet, but `TestFormatSageGateRoundTrip` (`internal/mcp/tickets_sage_test.go#L111-127`) asserts on the exact `sageGateNextInstruction` text this phase edits, so it must be run to confirm no regression and to cover the extended assertion.
- Dogfood run (ticket-specified): on a `ready/` ticket edited after its stamp, confirm `tickets.sage_gate(answer: yes)` returns `run` for exactly the stale stages and `tickets.sage_gate(answer: no)` returns `skip`/`run`-for-remaining with `tickets.verify` still warning.

## Escalations

- None. The count-of-call-sites discrepancy is noted for the executor's awareness, not an escalation: the ticket's Constraints line says "and its four call sites in `SageGate`," but only three call sites of `sageGateFreshnessResult` exist in `tickets_sage.go` (confirmed by direct read and grep). This does not change the required behavior or implementation approach — all three existing sites need the same treatment — so it is reported as a minor factual correction rather than a contract conflict.
