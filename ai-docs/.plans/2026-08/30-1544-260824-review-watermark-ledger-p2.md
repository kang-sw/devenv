# Plan: Review watermark marker + dotfile ledger + advisory sweep with lazy checkpoint recompute — Phase 2: Checkpoint nudge (cheap) + separately-invoked sweep

## Relevant Ticket Contract

- Two distinct mechanisms, never conflated: (a) a cheap checkpoint recompute+nudge wired at `tickets.close`, `workflow_manual` session start, and `enter.implement`/`enter.proceed` backstops, which **never spawns a review and never appends** to the ledger; (b) a separately-invoked standalone sweep that runs the already-landed lead-review range scenario (②) over `marker..HEAD` and stamps the ledger as its final step.
- Pre-④ review-track fallback: when no `AGENTS.md` review-track declaration exists (it never does yet — ④ is unimplemented), default the review-track to the git default branch (main/master); if HEAD isn't that branch, recompute against the default branch's tip, not the feature HEAD.
- Marker advances on every **completed sweep**, regardless of verdict (pass/concern/block all stamp); the ledger is verdict history, not a retry gate. A non-passing verdict routes to a ticket stem carried on the entry's `Ref` (required only for `block`, per Phase 1's lead adjudication).
- Nudge scaling: *size* = commit count over `marker..HEAD`, reusing the Deep Review / is-large-diff threshold; *staleness* = commit distance since the marker, via a new `_review.local.md` config knob with a modest default (no existing constant to reuse).
- Ledger-honesty guard (binding verification bullet): "a checkpoint recompute/nudge, which never runs a review, must never append." Phase 1's `Bootstrap` (`agents-plugin-tool/internal/wsreview/ledger.go:174-193`) **does append** when no entry exists — so the cheap checkpoint path must call `wsreview.Read`, never `wsreview.Bootstrap`.
- Merges stay native — no MCP-mediated merge. `tickets.close` must stay atomic/cheap (no delegated review spawn).
- Advisory-only; any defensible default is safe given the nudge never blocks (explicit epic-level tolerance, restated for the pre-④ fallback and reused elsewhere in this plan for the size-threshold unit mismatch below).
- Finding hand-off: the sweep's job ends at surfacing — route non-pass verdicts to a ticket/comm path, don't fix inline; the ledger entry references the routed stem, never a mutable "resolved" flag.

## Out of Scope

- Phase 1's ledger primitives (`LedgerPath`, `Entry`, `ParseLatest`, `Read`, `Append`, `Bootstrap`) — already landed in `agents-plugin-tool/internal/wsreview/ledger.go`; Phase 2 composes them, does not redefine them.
- ④ (`260824-feat-review-release-gate-policy`): the `AGENTS.md` review-track-branch declaration and the release gate itself. Phase 2 only builds the pre-④ fallback.
- Phase 3: multi-maintainer canary, self-documenting ledger banner, no-squash/landing-topology constraint. The ledger format Phase 1 shipped is already banner-tolerant (comment-skipping parser); no format change needed here.
- `260829-research-review-watermark-multi-maintainer-model` and `260829-research-review-checkpoint-relief-valve` — both are unaccepted research/idea tickets. The relief-valve idea (upgrading the nudge into an active "run the review for me" proposal) is explicitly **not** part of this ticket's Phase 2; Phase 2 stays a passive FYI nudge that never spawns a review. Do not implement the proposal/assent flow.
- Actual spec authoring for `ai-docs/spec/mcp-tools.md` / `ai-docs/spec/workflow-skills.md` — new caller-visible MCP behavior here will need spec entries, but that is a later doc step, not part of this implementation plan.
- Any change to `git.commit`, merge behavior, or a new MCP-mediated merge primitive.

## Codebase Findings

- `agents-plugin-tool/internal/wsreview/ledger.go:26-51` — `Entry{Base,Head,Verdict,Ref}`, verdict consts (`pass`/`concern`/`block`/`routed`/`bootstrap`), and `entryLineRE` (Phase 1, reuse as-is).
- `agents-plugin-tool/internal/wsreview/ledger.go:53-57` (`LedgerPath`), `:62-79` (`ParseLatest`), `:81-95` (`Read` — missing file returns `(Entry{}, false, nil)`, not an error), `:106-152` (`Append` — validates SHA-shaped Base/Head, known verdict, `Ref` non-empty iff `block`) — the composable primitives Phase 2 calls into.
- `agents-plugin-tool/internal/wsreview/ledger.go:174-193` (`Bootstrap`) — **risk signal**: internally calls `Append`. The cheap checkpoint path must never call this (would violate the ledger-honesty guard); only the explicit sweep may bootstrap.
- `agents-plugin-tool/internal/mcp/implement_resolver.go:442-466` (`implementCloseMergeReviewNudge`) — the reserved forward hook named by the ticket ("leaves room for epic 260824's later review-watermark hook to compose without rework"); its doc comment at `:454` is the explicit composition point.
- `agents-plugin-tool/internal/mcp/implement_resolver.go:420-436` (`aheadOfMergeRootCount`) — existing `git rev-list --count <base>..<head>` pattern via `wsgit.ExecRunner{}.RunGit`, fails open to `0` on any git error. Reuse this exact shape for the checkpoint's commit-count computation instead of inventing a new git call pattern.
- `agents-plugin-tool/internal/mcp/server.go:1214-1247` — the `tickets.close` dispatch case; `root` is already resolved at `:1217-1220`, and the existing nudge is appended at `:1245-1246` (`if nudge := implementCloseMergeReviewNudge(root); nudge != "" { text += "next_instruction: " + nudge + "\n" }`). Append the new review-watermark nudge the same way, right after.
- `agents-plugin-tool/internal/mcp/workflow_manual.go:255-291` (FRESH-with-root branch, `canonical` root in scope) and `:297-323` (CONTINUE branch, `rec.Root` in scope) — both already call `injectBootstrapStalenessWarning(body, scopeAnnouncement(root))` / `computeManuals(root)` at `:288-289` / `:320-321`. This is the exact wiring shape to mirror for the new nudge (same helper, same call-site family). The FRESH-no-root branch (`:292-296`) has no root — do not wire the nudge there.
- `agents-plugin-tool/internal/mcp/doc_coverage_alarm.go:16-41` — precedent for a pure `xWarning(root, ...) string` function returning `""` for every silent case, never erroring; `:45-47` (`injectDocCoverageWarning`) just delegates to the shared prepend helper. Model the new `wsreview.CheckpointNudge`-style function on this shape (pure, root-in/string-out, fail-open).
- `agents-plugin-tool/internal/mcp/bootstrap_alarm.go:139-146` (`injectBootstrapStalenessWarning`) — the generic "prepend if non-empty, else passthrough" injector already reused by three different warning kinds; reuse verbatim for the fourth (review nudge), no new injector needed.
- `agents-plugin-tool/internal/mcp/session_state.go:1024-1073` — `handleEnterImplement`'s `hasNewTarget` branch: `record, ok := s.sessions.readState(sessionKey)` at `:1031` already gives `record.Root`; append the nudge to `result.Raw` before the `:1072` return (text mode only, mirroring the `tickets.close`/`git.commit` advisory-is-text-only precedent).
- `agents-plugin-tool/internal/mcp/session_state.go:1074-1100` — the legacy `enter.implement` branch calls `s.handleEnter(...)` at `:1099`, which (`:1001-1022`) never resolves `root`. `handleEnter` has exactly one caller (grep-confirmed), so root resolution + nudge appending can be added inside `handleEnter` itself without touching unrelated tools.
- `agents-plugin-tool/internal/mcp/session_state.go:1135-1159` — `handleEnterProceed` resolves `sessionKey` at `:1137` but never fetches `root` today; add a `s.sessions.readState(sessionKey)` call and append the nudge to `result.Raw` before returning, same shape as the other enter path.
- `agents-plugin-tool/internal/mcp/server.go:4112-4123` — `tickets.close`'s tool schema (name/description/inputSchema/required) is the pattern to model the two new tools on (see plan step 5).
- `agents-plugin-tool/runtime.json` (`agents-plugin/runtime.json:8-69` "tools" section, and the mirrored `agents-plugin-wsflow/runtime.json:11-...`) — both must gain entries for any new tool name, version-gated `">=0.43.7-dev <0.44.0"` like `tickets.close`. wsflow includes `tickets.close` too (grep-confirmed), so the new review tools belong in both manifests (pure git/file operation, not agent-backed).
- `agents-plugin-tool/cmd/ws-mcp/main.go` — CLI mirrors are not universal; `todo.*`/`agenda.*`/`session.*` tools have none (grep-confirmed zero hits). Skip a CLI mirror for the two new review tools; MCP-only is consistent precedent.
- No `symbolic-ref`/`origin/HEAD`/default-branch detection exists anywhere in `agents-plugin-tool/internal/` (grep-confirmed) — this is new code, not a reuse gap.
- No Go-side parser for `ai-docs/_review.local.md` exists anywhere in `agents-plugin-tool/` (grep-confirmed zero hits), and the file itself does not exist in this repo yet. `agents-plugin/rsrc/lead-review/lead-review.md:11-18` (Invariants: config load) confirms the file is always optional — both scenarios degrade to built-in defaults when absent, never error.
- `agents-plugin/rsrc/lead-review/lead-review.md:73,125-126,199-203` — the existing "Deep Review" / `judge: is-large-diff` threshold defaults to **20 files / 500 lines** (a diff-footprint unit), consumed only by the LLM-driven skill text, not by any Go constant. **Risk signal**: the ticket says the cheap Go-side checkpoint should "reuse" this threshold for *size*, but size there is commit count (a different unit) and must stay cheap (no diff stat). Faithful reuse, consistent with the ticket's own advisory-only/defensible-default tolerance (already used for the pre-④ fallback), is to mirror only the numeric magnitude (20) as a commit-count analog, documented in code as an intentional unit reinterpretation — not a shared source-of-truth constant.
- `agents-plugin/rsrc/lead-review/lead-review.md:12-18` — the Invariants section already establishes a **range-scenario-only** gated step (`## Landing Lens` runs for range scenario, never branch scenario) — the precedent shape for gating the new stamp step to range-scenario only.
- `agents-plugin/rsrc/lead-review/lead-review.md:49-58` (`### 4. Review`, step 6 "Aggregate findings → emit verdict") and `:140-176` (`## On: verdict`, LGTM/NEEDS FIX/OPEN) — the sweep's stamp step belongs here: it must fire for **every** completed range-scenario verdict (LGTM/NEEDS FIX/OPEN alike, per "marker advances regardless of verdict"), so it fits better as a range-scenario-only addition right after step 6's aggregation than duplicated into all three `On: verdict` branches.
- `ai-docs/manuals/skill-authoring.md:10-27` (Layer model) — a new MCP tool call from a skill is Layer 1 (name the tool + inputs) plus Layer 2 (the tool's own returned text governs post-call handling — do not restate confirmation/error text in the playbook). The lead-review.md edit must name the `review.marker`/`review.stamp` calls and their inputs only, not restate their output.
- `ai-docs/tickets/idea/260829-research-review-checkpoint-relief-valve.md:1-23,183-216` — unaccepted idea ticket that documents real, still-current code facts (`session_state.go:632-672` merge-is-opt-in on `enter_implement`, `implement_resolver.go:672-682` low-ceremony inline-commit path) useful as background on why the nudge matters, but its "agent proposes/runs the review" upgrade is explicitly out of scope here.
- `agents-plugin-tool/internal/wsconfig/scope.go:77-98` — the layered `wsconfig` Item registration pattern (`ItemDocCoverageAlarm`, etc.) exists but is **not** the mechanism the ticket specifies for the staleness knob — the ticket names `_review.local.md` explicitly as the knob's home, not the session/project/global config store. Do not redirect the knob into `wsconfig` — that would be inventing a different policy than the one the ticket settled.

## Implementation Plan

1. **Review-track resolution** — add a small helper (e.g. `agents-plugin-tool/internal/wsreview/track.go`, same package): resolve the pre-④ review-track branch via `git symbolic-ref --short refs/remotes/origin/HEAD` (strip the `origin/` prefix), falling back to checking local `refs/heads/main` then `refs/heads/master` existence via `git rev-parse --verify --quiet`. Fail open (return `"", err`) on any git error — the caller (step 3) treats a resolution failure as "skip the nudge," never as fatal.
2. **Staleness config knob** — add a best-effort reader (same new file or a sibling `agents-plugin-tool/internal/wsreview/config.go`): read `ai-docs/_review.local.md` if present (mirror `wsreview.LedgerPath`'s join style), regex-extract a staleness commit-count knob under a small dedicated section (e.g. `## Checkpoint Nudge` / `staleness: <N> commits`), default to a modest constant (e.g. 15-30) when the file is absent, the section is absent, or the value doesn't parse. Never error — same fail-open posture as `Read`.
3. **Cheap checkpoint recompute** — add `wsreview.CheckpointNudge(ctx context.Context, root string) string` (pure, root-in/string-out, fail-open — mirror `doc_coverage_alarm.go:16-41`'s shape) composing: (a) resolve review-track branch (step 1); (b) `wsreview.Read(root)` — **never `Bootstrap`**; not-found → return a short "no review ledger yet; run a sweep to establish a baseline" advisory, no append; (c) found → commit count via the `aheadOfMergeRootCount` rev-list `--count` pattern (`implement_resolver.go:420-436`) over `<marker.Head>..<track-tip>`; (d) apply the reused size-threshold magnitude (step: document the 20-commit analog inline) and the staleness knob (step 2) to produce a proportional advisory string, or `""` when the range is small and fresh (quiet).
4. **Wire the nudge at the four checkpoint call sites**, always fail-open (never propagate an error, never block the underlying tool):
   - `agents-plugin-tool/internal/mcp/server.go:1245-1246` — after the existing `implementCloseMergeReviewNudge` append, add `if reviewNudge := wsreview.CheckpointNudge(context.Background(), root); reviewNudge != "" { text += "review-watermark: " + reviewNudge + "\n" }`.
   - `agents-plugin-tool/internal/mcp/workflow_manual.go:288-289` and `:320-321` — add `body = injectBootstrapStalenessWarning(body, wsreview.CheckpointNudge(context.Background(), canonical))` / `(..., rec.Root)` alongside the existing `scopeAnnouncement`/`computeManuals` injections.
   - `agents-plugin-tool/internal/mcp/session_state.go` — in `handleEnterImplement`'s `hasNewTarget` branch (`:1024-1073`), append the nudge to `result.Raw` using `record.Root` before the `:1072` return; inside `handleEnter` (`:1001-1022`, sole caller is the legacy `enter.implement` branch), add a best-effort `s.sessions.readState(sessionKey)` and append the nudge to the text response.
   - `agents-plugin-tool/internal/mcp/session_state.go:1135-1159` (`handleEnterProceed`) — add a best-effort `s.sessions.readState(sessionKey)` after `:1137`, append the nudge to `result.Raw` before returning.
5. **New MCP tools for the sweep to stamp the ledger** — add to `agents-plugin-tool/internal/mcp/server.go`'s `tools()` schema list (model on `tickets.close` at `:4112-4123`) and `callTool` dispatch (model on the `tickets.close` case at `:1214-1247`), both root-aware via `s.resolveToolRoot`:
   - `review.marker` — wraps `wsreview.Read`; optional `bootstrap: bool` input, and when `true` and no entry exists, calls `wsreview.Bootstrap` (the one and only explicit, caller-opted-in bootstrap trigger — never automatic). Returns the latest entry (or the bootstrap surfacing text) as text.
   - `review.stamp` — wraps `wsreview.Append` with `base`, `head`, `verdict`, optional `ref` inputs; surfaces `Append`'s own validation errors (SHA shape, block-requires-ref) as the tool error.
   - Add both to `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json` "tools" sections, version-gated like the neighboring entries. No CLI mirror (consistent with `todo.*`/`agenda.*`/`session.*` precedent).
6. **Extend the range scenario to stamp** — in `agents-plugin/rsrc/lead-review/lead-review.md`, read `ai-docs/manuals/skill-authoring.md` first, then add a range-scenario-only step after `### 4. Review` step 6 (`:57`, "Aggregate findings → emit verdict"), gated the same way `## Landing Lens` is gated to range-only (`:12-18`): call `review.marker(bootstrap: true)` to obtain/establish Base, map the resolved verdict to a `wsreview` verdict token (LGTM → `pass`, NEEDS FIX → `concern` or `block` by severity, OPEN → `concern`), and call `review.stamp(base, head=<range's head arg>, verdict, ref=<routed ticket stem, required when verdict is block>)`. Name only the tool + inputs (Layer 1); do not restate the tool's own returned confirmation/error text (Layer 2).

## Verification Plan

- `go build ./...` and `go vet ./...` from `agents-plugin-tool/` (whole-module compile/vet sanity after adding `wsreview.CheckpointNudge`, the two new tool dispatch cases, and the four call-site edits).
- `go test ./internal/wsreview/...` — new tests for: review-track resolution (origin/HEAD present, local main/master fallback, resolution failure); staleness knob parsing (file absent, section absent, malformed value, valid value); `CheckpointNudge` (no ledger → baseline-missing text and **no file written**; small/fresh range → `""`; large/stale range → scaled advisory) — assert directly that `Append`/`Bootstrap` were never invoked on the no-ledger and quiet paths (the ledger-honesty guard is exactly this: a checkpoint call must never grow the ledger file).
- `go test ./internal/mcp/...` — new integration-style tests mirroring `session_state_test.go:2532-2611`'s `TestServeStdioTicketsCloseMergeReviewNudgeOnUnmergedImplBranch`-family shape: `tickets.close`, `workflow_manual` (FRESH-with-root and CONTINUE), `enter.implement` (both branches), and `enter.proceed` each surface the nudge text when the review-track has an unswept range past the marker, stay silent on a freshly-swept trunk, and never error/block when git resolution fails (e.g. detached HEAD, no `origin` remote) or when `_review.local.md` is malformed.
- New tests for `review.marker` (bootstrap-on-request creates exactly one `bootstrap` entry, idempotent on repeat) and `review.stamp` (round-trips through `wsreview.Read`, rejects a `block` entry without `ref`, matching Phase 1's `Append` test coverage shape in `ledger_test.go`).
- Manual/documented-only check: after the `lead-review.md` range-scenario edit, a dry run of `lead-review range: <marker>..<HEAD>` on this repo confirms the stamp call fires exactly once per verdict and the ledger's latest entry advances to the reviewed head — this is a skill-behavior check, not something `go test` can cover, and belongs to the executor's own doc-edit verification pass per skill-authoring's Fresh-Reader Audit.

## Lead Adjudications (30-1544)

Resolved the survey's three flagged judgment calls (drain posture: reversible,
advisory-only, defensible-default tolerance applies):

1. **Checkpoint never appends** — accepted as planned. The cheap path calls
   `wsreview.Read` only; the sole bootstrap trigger is the explicit,
   caller-opted-in `review.marker(bootstrap: true)` invoked by the sweep. This
   is the ticket's binding ledger-honesty guard.
2. **Size threshold unit reinterpretation** — accepted. Mirror the is-large-diff
   numeric magnitude (**20**) as a commit-count analog for *size*, documented
   inline as an intentional unit reinterpretation, not a shared constant. The
   cheap path must not compute a diff stat.
3. **New MCP surface (`review.marker`/`review.stamp`)** — accepted as
   ticket-scoped: the sweep-stamps-ledger design is sage-settled, and a
   validated tool bridge is the only honest way for a skill to append to the
   ledger. MCP-only (no CLI mirror), version-gated in both runtime manifests.
   Spec entries (`mcp-tools.md`, `workflow-skills.md`) are deferred to the doc
   step, correctly out of implementation scope. New API surface — disclose in
   the drain handoff.

Additional pin (plan left "e.g. 15-30" loose): **staleness knob default = 10
commits**, deliberately below the size magnitude (20) so the two thresholds are
non-degenerate — quiet below 10, gentle FYI 10–19, stronger "large accumulation"
nudge at ≥20. User-overridable via `_review.local.md`.

## Escalations

- None.
