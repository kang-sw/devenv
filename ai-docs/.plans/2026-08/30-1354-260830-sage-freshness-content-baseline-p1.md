# Plan: 260830-feat-sage-freshness-content-baseline — Phase 1: Record and consume a body-digest freshness baseline

## Relevant Ticket Contract

- Primary path: when `sage_stamp` writes `sage-review-<stage>: completed`, also
  write a sibling `sage-review-<stage>-reviewed: <digest>` where `<digest>` is
  sha256 of the normalized reviewed body, truncated to a fixed 16-hex-char
  prefix. Freshness compares recorded digest vs. current digest for that
  stage; equal → fresh, differ → stale. No git walk on this path.
- Body-only normalization: digest and freshness comparison both hash the
  markdown body **below the frontmatter fence, trimmed** — the whole
  frontmatter is excluded (not just `sage-review*` keys as today). Primary and
  fallback paths must share this normalization.
- Fallback (legacy, no recorded digest): git-history baseline, corrected to
  the **latest** completed-transition (not earliest), body-only compared.
  Legacy tickets self-heal on next stamp (no migration pass).
- New field is a sibling, not an inline posture value; posture stays exactly
  `completed`.
- The new field must never perturb posture parsing or the digest (body-only
  normalization already excludes all frontmatter, including this field).
- Backward compatible by construction: absent digest → fallback; no existing
  ticket needs editing.
- Verification boundary (ticket's own bullets, all in scope for this phase):
  1. digest re-stamp (`completed → completed`, no `pending` dip) clears
     freshness; a later body edit without re-stamp goes stale again.
  2. legacy ticket, no recorded digest, committed reset→re-stamp reads fresh
     via latest-transition fallback; a later edit after the last stamp reads
     stale.
  3. a frontmatter-only edit (`related:`/`title:`) after a stamp does not
     trigger staleness.
  4. existing freshness tests (single-transition stale/uncommitted/staged/
     status-move) stay green under body-only normalization.
- Spec Impact explicitly names `ai-docs/spec/mcp-tools.md` (the sage-review
  freshness contract) as part of this ticket's scope.

## Out of Scope

- No `sage_gate` "dismiss without re-review" affordance (Non-Scope).
- No change to `sage_gate` stage ordering (Non-Scope).
- No wall-clock timestamp key (Non-Scope, rejected explicitly).
- No migration pass over existing tickets (self-heal by design).
- `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md`: grep
  confirms this bundled doc carries **no** `sage-review*` field documentation
  today (it only covers path/naming/status-flow/phases/stems/content
  generically). The ticket's "ticket-conventions frontmatter contract" Spec
  Impact target therefore resolves to `ai-docs/spec/mcp-tools.md` only for
  this phase; no edit to the bundled convention doc is needed unless a future
  ticket adds sage-review documentation there.

## Codebase Findings

- `agents-plugin-tool/internal/wsdoc/tickets_sage_freshness.go#L31-69` —
  `sageReviewFreshnessCheck`: per-stage loop currently always calls
  `sageReviewStageBaseline` (git walk) and compares against
  `normalizeTicketForSageFreshness`. Needs a primary digest branch inserted
  before the fallback call.
- `agents-plugin-tool/internal/wsdoc/tickets_sage_freshness.go#L71-109` —
  `sageReviewStageBaseline`: two-loop structure. First loop (oldest→newest,
  `i := len(history)-1; i >= 0; i--`) returns the **earliest** transition
  (confirmed by hand-tracing `TestSageGateWarnsWhenCompletedReviewIsStale`).
  Second loop (newest→oldest linear scan) is effectively dead — the first
  loop's `previous != "completed"` sentinel (initialized `""`) already
  catches the "born completed" case, so the first loop never returns
  `ok=false` when any commit ever had the field completed. Needs replacing
  with a newest→oldest scan that finds the **latest** transition edge.
- `agents-plugin-tool/internal/wsdoc/tickets_sage_freshness.go#L116-171` —
  `sageReviewTicketHistory` returns entries **newest-first**
  (`history[0]` = newest, confirmed via existing loop bounds and git log's
  default order, no `--reverse`). Any rewrite of the baseline scan must
  respect this ordering.
- `agents-plugin-tool/internal/wsdoc/tickets_sage_freshness.go#L183-203` —
  `normalizeTicketForSageFreshness`: today strips only
  `sage-review`/`sage-review-design`/`sage-review-completeness` keys inside
  the frontmatter block and rejoins the whole text (frontmatter + body).
  Must become: return the raw text below the closing `---` fence, trimmed
  (or the whole trimmed text if no fence pair is found — preserves current
  graceful-degradation behavior for malformed input rather than collapsing
  to `""`, which would falsely equate all malformed tickets as identical).
- No `crypto/sha256` / `encoding/hex` import currently exists in this file or
  in `tickets_sage.go` — both need to be added (freshness file is the natural
  home for the shared digest helper).
- `agents-plugin-tool/internal/wsdoc/tickets_sage.go#L1-8` — imports only
  `fmt, os, path/filepath, strings`; no digest helper needed here directly if
  the digest read+compute lives in the freshness file and is called from here.
- `agents-plugin-tool/internal/wsdoc/tickets_sage.go#L441-446` —
  `sageRecordSingle`'s pass/concern-resolved-to-pass branch: sole
  single-stage write site for `completed`. Needs the sibling digest field
  added to the same `writeFrontmatterField` call (one extra key, same map).
- `agents-plugin-tool/internal/wsdoc/tickets_sage.go#L493-497` —
  `sageRecordCombined`'s non-block branch: writes both
  `sage-review-design`/`sage-review-completeness` as `completed` in one call.
  Needs both sibling digest fields added (`sage-review-design-reviewed`,
  `sage-review-completeness-reviewed`) — same digest value for both, since
  it's the same ticket body.
- Confirmed no other write site ever sets a stage posture to `"completed"`:
  `resolveConcretePosture`/`resolveStage`/`sageGateCombined`
  (`tickets_sage.go#L224-373`) only ever fall back to
  `ResolvedSageReviewPosture` (`tickets_mutate.go#L365-374`), whose only
  outputs are `recommended`/`required`/`skipped` — never `completed`. So the
  two write sites above are exhaustive for this phase.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L416-436` —
  `effectiveSageReviewPostures` reads only the exact keys
  `sage-review-design`, `sage-review-completeness`, `sage-review` via type
  assertion (`fm["sage-review-design"].(string)`), never a prefix scan.
  Grep across the package (`HasPrefix.*sage-review`) found zero matches
  anywhere. **The new `sage-review-<stage>-reviewed` field cannot perturb
  posture parsing by construction** — no code change needed there beyond the
  body-only normalization already excluding it from the digest/freshness
  comparison (frontmatter is fully excluded). Risk signal noted and closed:
  the ticket's caution about "ensure it's stripped everywhere" is satisfied
  structurally, not by adding new stripping logic.
- `agents-plugin-tool/internal/wsdoc/frontmatter.go#L19-73` —
  `frontmatterFromText` generic parser handles a new scalar key like
  `sage-review-design-reviewed: <16-hex>` with no special casing needed
  (plain `key: value` scalar path).
- `agents-plugin-tool/internal/mcp/server.go#L2710-2718` — `ReviewBaseline`/
  `ReviewInstruction` are rendered as plain strings
  (`review_baseline: %s`) with no assumption of commit-hash shape or length.
  Safe to populate with a non-commit sentinel string on the digest-primary
  stale path (no git walk happens there, so there is no commit to report).
- `ai-docs/spec/mcp-tools.md#L1376-1381` — the current freshness prose says
  "stale against the Git commit that first recorded that completed posture"
  — describes today's earliest-transition behavior and is now wrong; needs a
  rewrite describing digest-primary + latest-transition-fallback.
- `agents-plugin-tool/internal/wsdoc/tickets_sage_test.go#L77-88` —
  `writeSageTicket(t, root, stem, fields map[string]string)` writes arbitrary
  frontmatter fields directly (bypassing `SageRecord`) — the right helper for
  constructing legacy (no-digest) fixtures. For digest-primary fixtures,
  prefer calling `SageRecord` itself (the real write path) rather than
  hand-computing the expected digest, since the test file is `package wsdoc`
  (internal test package) and can call unexported functions/the real
  `SageRecord` API directly.
- Existing freshness tests
  (`TestSageGateWarnsWhenCompletedReviewIsStale`,
  `TestSageGateWarnsOnUncommittedPostStampEdit`,
  `TestSageGateWarnsOnStagedOnlyPostStampEdit`,
  `TestSageGateFreshnessIsStageSpecific`,
  `TestSageGateFreshnessIgnoresSageOnlyAndStatusOnlyChanges`,
  `TestSageGateFreshnessFollowsStatusMoveThenContentEdit`,
  `tickets_sage_test.go#L394-571`) construct tickets via `writeSageTicket`
  directly (never via `SageRecord`), so none of them ever produce a recorded
  digest — all exercise the fallback path today and will continue to after
  the change. Each has exactly one completed-transition in its commit
  history, so "latest transition" and "earliest transition" coincide for all
  of them (hand-traced for `TestSageGateWarnsWhenCompletedReviewIsStale`:
  `previous` sentinel `""` causes the old first loop to return the sole
  transition immediately; the newest→oldest rewrite finds the same single
  edge). These should stay green unmodified.

## Implementation Plan

1. **`tickets_sage_freshness.go` — normalization (L183-203).** Rewrite
   `normalizeTicketForSageFreshness(text string) string` to: if the first
   line isn't `---`, return `strings.TrimSpace(text)`; else scan for the
   closing `---` line and return `strings.TrimSpace` of everything after it;
   if no closing fence is found, return `strings.TrimSpace(text)` (graceful
   fallback, matches today's degrade-safe behavior). Delete the
   sage-key-stripping loop entirely — whole-frontmatter exclusion supersedes
   it.

2. **`tickets_sage_freshness.go` — digest helper (new, near step 1).** Add
   `import "crypto/sha256"` and `"encoding/hex"`. Add:
   ```go
   func sageReviewBodyDigest(text string) string {
       sum := sha256.Sum256([]byte(normalizeTicketForSageFreshness(text)))
       return hex.EncodeToString(sum[:])[:16]
   }
   func sageReviewCurrentBodyDigest(path string) (string, error) {
       raw, err := os.ReadFile(path)
       if err != nil {
           return "", err
       }
       return sageReviewBodyDigest(string(raw)), nil
   }
   ```
   `sageReviewCurrentBodyDigest` is the entry point `tickets_sage.go`'s write
   path calls (same package, no new import needed there).

3. **`tickets_sage_freshness.go` — primary digest check in
   `sageReviewFreshnessCheck` (L31-69).** After computing `currentClean` and
   before the per-stage fallback call, parse
   `currentFM := frontmatterFromText(string(currentRaw))` and compute
   `currentDigest := sageReviewBodyDigest(string(currentRaw))` once. Inside
   the stage loop, before calling `sageReviewStageBaseline`: look up
   `recorded, ok := currentFM["sage-review-"+stage+"-reviewed"].(string)`.
   - If present and non-blank: compare `strings.TrimSpace(recorded) ==
     currentDigest`. Equal → `continue` (fresh, skip fallback entirely — no
     git walk). Differ → append to `affected`/`baselines`; use a
     non-commit sentinel string (e.g. `"recorded digest"`) for the baseline
     entry, since no commit was inspected on this path — `shortCommit` is a
     git concept and should not be called here.
   - If absent: fall through to the existing
     `sageReviewStageBaseline`/body-only-compare fallback logic, unchanged
     except for step 4 below.

4. **`tickets_sage_freshness.go` — latest-transition fallback in
   `sageReviewStageBaseline` (L71-109).** Replace both loops with a single
   newest→oldest pass over `history` (`history[0]` = newest) that
   precomputes each entry's stage posture once, then finds the first index
   `i` (scanning newest→oldest) where `postures[i] == "completed"` and the
   next-older entry (`postures[i+1]`, or `""` sentinel if `i` is the last/
   oldest index) is `!= "completed"`. Return that entry's commit/raw. This
   generalizes the existing "born completed" sentinel trick to the
   newest-first direction, so single-transition tickets resolve to the same
   commit as before (only one edge exists either way) while multi-transition
   tickets (reset→re-stamp) resolve to the latest edge instead of the
   earliest. Delete the now-fully-superseded old two-loop body.

5. **`tickets_sage.go` — write the digest on the single-stage completed
   path (L441-446).** In `sageRecordSingle`'s pass branch, before/alongside
   the existing `writeFrontmatterField` call: compute
   `digest, err := sageReviewCurrentBodyDigest(ticketAbs)` (propagate `err`),
   then extend the write map to
   `map[string]string{field: "completed", field + "-reviewed": digest}` in
   one `writeFrontmatterField` call (keeps the write atomic/single-pass, no
   behavior change to existing fields).

6. **`tickets_sage.go` — write the digest on the combined completed path
   (L493-497).** Same pattern in `sageRecordCombined`'s non-block branch:
   compute `digest` once via `sageReviewCurrentBodyDigest(ticketAbs)`, extend
   the write map to include `"sage-review-design-reviewed": digest` and
   `"sage-review-completeness-reviewed": digest` alongside the two existing
   `completed` keys.

7. **No change needed** to `effectiveSageReviewPostures`, `resolveStage`, or
   any other posture/frontmatter reader — confirmed by the exact-key-match
   audit in Codebase Findings. Do not add new stripping logic; the body-only
   normalization (step 1) already excludes the new field from every
   freshness/digest computation, and posture readers never touch it. Treat
   this as a verification step in review, not a code step.

8. **`ai-docs/spec/mcp-tools.md#L1376-1381`** — rewrite the freshness
   sentence to describe: (a) the primary digest comparison against
   `sage-review-<stage>-reviewed` with no git walk, (b) the fallback git
   baseline now corrected to the latest completed-transition, and (c)
   body-only (whole-frontmatter-excluded) comparison shared by both paths.
   Keep the surrounding "soft `sage-review-freshness` warning" framing
   (`tickets.verify`/`git.commit` behavior) unchanged — this ticket does not
   touch the warning's hard/soft classification, only what it diffs against.

## Verification Plan

- `cd agents-plugin-tool && go build ./...`
- `cd agents-plugin-tool && go test ./internal/wsdoc/... -run Sage -v` —
  must keep all existing `TestSageGate*`/`TestSageRecord*` freshness and
  non-freshness tests green.
- Add new test coverage in `tickets_sage_test.go` (package `wsdoc`, can call
  unexported functions/the real `SageRecord`/`SageGate` APIs directly),
  covering the ticket's four verification bullets:
  1. **Digest re-stamp clears freshness.** Stamp via `SageRecord` (writes
     digest), commit; edit body, re-stamp via `SageRecord` again (writes a
     new digest, no `pending` dip), commit; assert `SageGate` freshness for
     that stage is fresh (`Action != "check_review_required"` for that
     stage). Then edit body again without re-stamping, commit; assert now
     stale (`check_review_required`, stage listed).
  2. **Legacy fallback advances on committed reset→re-stamp.** Build via
     `writeSageTicket` only (no digest field): commit an initial
     non-completed state, commit a first `completed` stamp (transition A),
     commit a reset to a non-terminal posture with a body change, commit a
     second `completed` stamp on the new body (transition B, latest).
     Assert `SageGate` freshness reads fresh (current body matches
     transition B's recorded content, not transition A's) — this is the
     regression check for the earliest→latest fallback fix. Then edit body
     again post-transition-B without any further stamp; assert stale.
  3. **Frontmatter-only edit stays fresh.** Stamp (either path), commit;
     edit only a non-sage frontmatter key (`title:` or `related:`) with no
     body change, commit; assert `SageGate` freshness stays fresh — this is
     the direct regression test for the Decisions section's called-out bug
     (today's normalization only strips `sage-review*` keys, so `title`/
     `related` edits currently false-positive as stale).
  4. Run the existing freshness suite unmodified (already covered by the
     `go test -run Sage` command above) to confirm bullet 4 (no separate new
     test needed — it's a regression guard on current tests).
- No manual-only verification needed; all four cases are expressible as Go
  unit tests using the existing `t.TempDir()` + git-init test harness
  pattern already in `tickets_sage_test.go` (`initSageFreshnessRepo`,
  `commitSageFreshnessRepo`, `writeSageTicket`).

## Escalations

- None.
