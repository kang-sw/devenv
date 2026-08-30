# Plan: 260824-feat-review-watermark-ledger — Phase 3

## Relevant Ticket Contract

- Phase 3 scope (dormant for the serial baseline, activates only under
  concurrent maintainers; depends on Phases 1-2, both landed):
  1. Emit the self-documenting banner as part of the Phase-1 ledger format
     (top-of-file, and/or a comment adjacent to the tail append anchor) so a
     git conflict surfaces it verbatim to the resolver.
  2. Document the canary as an intended property, not a new mechanism: the
     Phase-1 tail-append format already produces the conflict; name it,
     specify that resolving it means integrating the other branch's work,
     and (optional hardening) that the resolution append a fresh verdict
     entry over the re-integrated range for a future CI check to validate.
  3. State the no-squash / landing-topology constraint (ff or merge-commit
     on the review-track; entry `<head>` = reviewed commit R, never the
     bookkeeping commit L) as review-track guidance; platform-config
     hardening (disable squash/rebase) is owned by ④, out of scope here.
- Phase 3 verification (ticket text): two branches that both stamp without
  absorbing each other produce a ledger tail conflict (canary fires); the
  serial single-writer path never conflicts (no false positive); the banner
  text appears in the conflicted region; a squash landing on the
  review-track orphans the marker and the next range simply re-covers the
  orphaned span (over-review, never under-review).
- Phase 1 already designed the parser for this: `entryLineRE` skips every
  `#`-comment/banner line and any other non-matching line, "deliberate
  Phase-3 readiness" (`ledger.go:41-49`). No parser change is needed.
- Phase 1 Result explicitly deferred "a ledger-domain mental-model lands
  with the Phase-3 canary" — this phase is the intended landing point for
  that doc, not an incidental addition.
- Write discipline (epic-level decision, restated in ticket Background):
  only the review-track branch writes the ledger; the append-only format is
  what makes two independent tail appends git-conflict. This is the
  mechanism the canary reuses — Phase 3 adds no new write path.

## Out of Scope

- ④ (`260824-feat-review-release-gate-policy`): platform-config hardening
  (disabling squash/rebase), and the release gate that blocks promotion on
  an unresolved routed blocking ledger entry.
- Building an actual CI check that validates a post-conflict-resolution
  fresh verdict entry — the ticket frames this as "(optional hardening)"
  documentation guidance for a *future* check, not a Phase-3 deliverable.
- Reading the `260829` multi-maintainer research ticket beyond what this
  ticket's own "Multi-maintainer constraints" section already inlines —
  that section is treated as complete and authoritative for this phase.
- Any change to Phase 2's `CheckpointNudge`/sweep mechanics
  (`checkpoint.go`) — read-only, unaffected by banner/canary/no-squash
  documentation.

## Codebase Findings

- `agents-plugin-tool/internal/wsreview/ledger.go#L41-L49` — `entryLineRE`
  and its doc comment already anticipate "the eventual top-of-file banner
  and tail-anchor comment"; the skip behavior is pre-built, confirmed by
  `TestParseLatestSkipsBannerLinesBeforeAndAfter` (`ledger_test.go:52-69`).
- `agents-plugin-tool/internal/wsreview/ledger.go#L106-L140` — `Append` is
  the **sole physical file-creation point**: `os.OpenFile(path,
  O_APPEND|O_CREATE|O_WRONLY, ...)`. No banner is currently emitted on
  first creation.
- `agents-plugin-tool/internal/wsreview/ledger.go#L174-L194` — `Bootstrap`
  never creates the file itself; it calls `Append` when no entry is found.
- **Risk signal (reuse/gap):** `agents-plugin-tool/internal/mcp/server.go#L1276-L1288`
  — the `review.stamp` MCP tool calls `wsreview.Append` directly and never
  calls `Bootstrap`. So the ledger file can come into existence via a bare
  `Append` (e.g. a first-ever `review.stamp` call with no prior
  `review.marker(bootstrap: true)`), bypassing `Bootstrap` entirely. Banner
  emission must therefore live in `Append`'s first-creation path, not in
  `Bootstrap` alone, or a stamp-first ledger would never get a banner.
- `agents-plugin-tool/internal/wsreview/ledger_test.go#L233-L270` —
  `TestRoutedCorrectiveEntryAppendsWithoutEditingEarlierBlock` asserts
  **exactly 2 raw lines** after two direct `Append` calls with no prior
  file. Adding a first-creation banner will change this file's raw line
  count and needs a test update (the *parsed* assertions, which go through
  `Read`/`ParseLatest`, are unaffected since the parser skips `#` lines).
- `ai-docs/spec/mcp-tools.md#L1880-1922` (`#260830-review-watermark-ledger-tools`)
  — current spec text describes the entry line format only; does not
  mention a banner/comment line. Small addition candidate, not a new tool
  contract (the banner is invisible to `review.marker`/`review.stamp`
  callers — it's a file-format detail, not tool output).
- `mental_models.find(query="review ledger watermark canary")` returned no
  dedicated hit (closest is the general `ai-docs/mental-model/workflow-skills.md`,
  score 66, not ledger-specific) — confirms no ledger-domain mental-model
  exists yet, consistent with Phase 1 Result's deferral note.
- `agents-plugin-tool/internal/wsreview/checkpoint.go` (whole file) — reads
  the ledger only (`Read`, never `Bootstrap`/`Append`); nothing here needs
  to change for banner/canary/no-squash documentation.

## Implementation Plan

1. In `ledger.go`, add a banner constant near the `Verdict*` consts (e.g.
   `ledgerBanner`), adapted from the ticket's example text, naming the
   canary explicitly and stating: (a) a conflict here means two branches
   reviewed independently — do not blindly `checkout --theirs`; integrate
   the other branch's reviewed work; (b) optionally, append a fresh verdict
   entry over the re-integrated range once resolved. Format each line
   `#`-prefixed so `entryLineRE` skips it (already guaranteed by existing
   parser design/tests).
2. In `Append` (`ledger.go:106-140`), check once (before `OpenFile`)
   whether `LedgerPath(root)` does not yet exist; if so, write the banner
   block ahead of the formatted entry line in the same write (still a
   single `O_CREATE|O_WRONLY` open — no read-modify-write introduced).
   This guarantees the banner lands on **every** code path that creates the
   file (`Bootstrap`, and a bare `review.stamp`-triggered `Append`), closing
   the gap in the risk signal above. Recommended default: a single
   top-of-file placement, written once at first physical creation — this
   matches the render-input phrasing "on bootstrap/first-create" and, for
   the concurrent-first-landing scenario the canary targets (two branches
   racing to create the ledger from nothing), top-of-file and tail-anchor
   coincide, so the banner text lands inside the actual conflict markers.
   **Flagged for lead confirmation** — see Escalations note below; the
   ticket's "and/or" leaves room for an additional per-entry tail comment,
   which this plan does not add.
3. Update `ledger_test.go`:
   - Fix `TestRoutedCorrectiveEntryAppendsWithoutEditingEarlierBlock`'s raw
     line-count/content assertions to account for the new banner lines
     (banner once, at the top; the two entry lines unchanged below it).
   - Add a test asserting a fresh `Append`-created ledger's raw file starts
     with the banner text (and that `ParseLatest`/`Read` still resolve to
     the real entry, not the banner).
   - Add a test reproducing the actual canary: using the
     `reviewTestInitGitWithCommit`/`reviewTestRunGit` real-git-repo fixture
     idiom already in this file, create two branches from one committed
     ledger state, run `Append` independently on each, merge one into the
     other, and assert (a) a real git conflict occurs, (b) the banner text
     appears within the conflicted region, (c) a serial single-writer
     sequence (one `Append` then a normal fast-forward/merge with no
     competing tail write) produces zero conflict.
4. Author a new mental-model file, e.g.
   `ai-docs/mental-model/review-watermark-ledger.md` (read mental-model
   conventions via `ws/convention.read` first, per AGENTS.md), covering:
   the ledger format and marker (already spec'd, cross-reference
   `#260830-review-watermark-ledger-tools`), the canary as an intended
   property of the append-only tail format (not a new mechanism — name it,
   state that resolving the conflict means integrating the other branch's
   reviewed work, note the optional fresh-verdict-entry convention), and
   the no-squash/landing-topology constraint (ff or merge-commit only on
   the review-track; ledger `<head>` = reviewed commit R, never bookkeeping
   commit L; squash/rebase forbidden by convention on the review-track,
   platform-config hardening owned by ④).
5. Optionally extend `ai-docs/spec/mcp-tools.md` around
   `#260830-review-watermark-ledger-tools` (`L1886-1889`) with one short
   sentence noting the ledger's first line may carry a `#`-prefixed banner
   comment the parser skips — a file-format clarification, not a new tool
   contract or caller-visible output change.

## Verification Plan

- `go test ./internal/wsreview/...` — existing suite stays green plus the
  new/updated tests in step 3.
- `go build ./...` && `go vet ./...`.
- The new canary-repro test in step 3 directly exercises the ticket's own
  verification bullets (conflict fires on concurrent stamps, banner text in
  the conflicted region, serial path stays conflict-free).
- Squash-orphans-marker behavior (ticket's fourth verification bullet) is a
  restatement of already-existing, already-tested behavior (marker resolves
  from ledger content, never graph-walked) — no new test is required unless
  the executor judges a direct regression test adds value; otherwise cover
  it as a documented invariant in the new mental-model file.
- `spec_index_verify` if step 5's spec addition lands.

## Lead Adjudications (30-1644)

Resolved the survey's one flagged judgment call plus a runbook scope split:

1. **Banner placement — top-of-file, once, in `Append`'s O_CREATE path.**
   Confirmed the survey's recommendation. The banner block is written exactly
   once, at first physical file creation, inside `Append`'s single
   `O_CREATE|O_WRONLY` open (banner + first entry in one write — no
   read-modify-write, append-only invariant preserved). This guarantees the
   banner on *every* creation path, including a bare `review.stamp`→`Append`
   that never calls `Bootstrap` (the risk signal). Rationale for top-of-file
   over a per-entry comment: a merge-conflict resolver opens the full file to
   edit the conflict markers and therefore sees the top banner; append-only
   stays simplest; the ticket permits top-only ("and/or") and frames the banner
   as a soft mitigation over a physically irreducible gap. The richer per-entry
   tail-comment design (banner text stays literally inside the conflict markers
   on a mature ledger) is a real but **optional** hardening — **deferred**, and
   must be recorded as a known limitation in the new mental-model doc, not
   silently dropped.
2. **Scope split — implementer does code+tests only.** The `{edit}` implementer
   dispatch covers **steps 1-3 only** (banner constant, `Append` create-path
   emission, and the test updates including the real-git canary-repro test).
   **Step 4 (new mental-model authoring) and step 5 (spec sentence) move to the
   `{doc-pre-pass}` step**, per the runbook's doc-pipeline ownership: the
   ledger-domain mental-model (the Phase-1-deferred doc) is dispatched to
   `mental-model-updater`, and the spec clarification is part of the spec
   update. Keeps the implementer focused on the append-only-sensitive code and
   routes convention-bound doc authoring through the doc pipeline.

## Escalations

- None.

Risk signal surfaced for lead judgment (not blocking a light plan, but
flagged per the render instructions): banner placement is implemented here
as a single top-of-file block written once at first physical file creation
inside `Append` (covering both `Bootstrap` and a bare `review.stamp`
first-call). The ticket's "and/or a comment adjacent to the tail append
anchor" leaves open a second design — a short comment re-emitted
immediately before *every* entry — which would keep banner text inside the
conflict markers for conflicts that happen after the ledger has
accumulated many entries (a top-of-file-only banner stops being part of the
diverging insertion once entries pile up below it, though it remains
visible to anyone opening the full file to resolve the conflict). This plan
recommends the simpler one-time placement, consistent with the render
input's own "on bootstrap/first-create" framing, and treats the richer
per-entry design as optional future hardening rather than required for this
phase.
