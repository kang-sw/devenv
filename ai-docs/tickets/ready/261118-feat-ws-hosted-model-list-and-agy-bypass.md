---
title: "Hosted Codex/agy model lists without opening a session, and a bypass mode for agy leads"
parent: 261114-epic-ws-common-session-foundation
related:
  261115-feat-ws-hosted-codex-adapter-contract: "its Phase 2 fills Codex choices only from a lead's model/list (codexChoices)"
  261118-feat-ws-agy-tool-names-and-worker-role: "(157) base model names plus effort; WorkerAutoApprove is the auto-allow path reused here"
  261117-feat-ws-hosted-antigravity-adapter: "agy choices come from session configOptions (agyChoices)"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: c3407be083e0dfad
sage-review-completeness-reviewed: c3407be083e0dfad
---

# Hosted Codex/agy model lists without opening a session, and a bypass mode for agy leads

## Background

Live dogfood on 2026-11-18, after the shared daemon swap to track/agent-v2
`4eebad0d8`, surfaced two problems in the dashboard's new-session dialog.

**The model select is disabled for Codex and agy.** Both backends' `Choices`
return only what the last opened session reported:

- `codexChoices` (`agents-plugin-tool/internal/wsagent/codex.go:38-50`) is
  filled from a lead's `model/list`.
- `agyChoices` (`agents-plugin-tool/internal/wsagent/agy.go:92-112`) is
  filled from a session's `configOptions` (`agy.go:866-913`).

After a daemon restart both lists are empty until a session of that harness
opens. The original reasons were "opening the dashboard must not start a
process or create a Codex thread" and "agy lists models only inside a
session". Cheaper sources exist (verified 2026-11-18 on macOS):

- **Codex:** `$CODEX_HOME/models_cache.json` (default `~/.codex`) holds
  `models[]`. Each entry has a `slug`, a `display_name` and
  `supported_reasoning_levels[].effort`. Its top level also holds
  `fetched_at`, `etag`, `client_version` and an account-bound `identity`.
- **agy:** `agy models` (agy 1.5.x) prints `Fetching available models...` and
  then one `<id>\t<display name>` line per model, in about 2.8 s, with no
  session created. Ids carry effort suffixes (`gemini-4-argon-high`); a few
  have none (`gemini-pro-orchestrator`).

The dialog's listing path already caches per harness and bounds each listing
(`Support.listChoices`, `agents-plugin-tool/internal/wsagent/support.go#L289-L336`,
`choicesTimeout` 15 s at `support.go#L54-L55`). Pi's
`Choices` already runs a listing process through the `runListing` seam
(`pi.go:38-45`).

**A hosted agy lead keeps raising approval prompts, and it has no bypass.**
`AgyPermissionModes` (`agy.go:53-55`) is agy's native session modes:
`default`, `accept-edits`, `auto` and `plan`. agy workers are auto-allowed:
`runner_events.go:286` answers every worker request through
`answerApproval` when the registration's `WorkerAutoApprove` is set. Lead
records are excluded by `!c.rec.Lead`. Codex already has a bypass:
`danger-full-access` sets approvalPolicy `never` (`catalog.go:401-403`,
`codex.go:218-227`).

## Decisions

- (167)=a, 2026-11-18: fill the Codex and agy model lists without opening a
  session.
  - Codex reads `models_cache.json`.
  - agy runs `agy models` once, on demand, from the dialog's listing path.
  - Both results are cached in memory, and a session's own report refreshes
    them.
  - Compatibility T0: the choices answer keeps its shape.
  - Rejected: Codex only, with agy left session-filled; and leaving both as
    they are.
- (168)=a, 2026-11-18: add a ws-side `bypass` permission mode for hosted agy
  leads. The daemon answers every permission request "allow" through the
  same `answerApproval` path that `WorkerAutoApprove` uses. The default mode
  is unchanged.
  - Rejected: first probing whether `agy acp` accepts
    `--dangerously-skip-permissions`, which would tie the mode to an agy
    version; and leaving agy leads as they are.
- (169), 2026-11-18 (technical, owner ack): `bypass` opens agy's native
  `default` mode. `default` routes tool permission requests to the daemon,
  so the auto-allow answers them deterministically. Rejected: native `auto`,
  whose classifier may resolve some calls natively in ways ws does not
  observe.
- (170)+(172)=a, 2026-11-18 (design review reopened (170)): the bypass
  intent is stored in a separate optional top-level `Record` field, and
  `PermissionMode` stays agy's native `default`. `Record` keeps unknown
  top-level members across read-modify-write (`record.go:175-233`, since
  `184452a15`). So an older 0.50.x daemon keeps the field, runs the lead in
  native `default` with prompts (including holder reattach, which skips the
  set-mode check at `agy.go:168-170`), and changes no data; reopening with the
  new daemon restores bypass. Compatibility T1. The worker cites the read and
  write lines in its Result and stops if the verdict exceeds T2.
  - Rejected: `bypass` as a `PermissionMode` value. An older daemon can
    overwrite it to `default` (`runner_events.go:126-127`) with no recovery,
    which needs a T3 boundary and a minor bump that this track does not take.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)
- Performance (`ai-docs/manuals/performance.md`): no listing work at page
  render.
  - A listing runs only from the existing `Support.listChoices` path, under
    its per-harness cache and `choicesTimeout`.
  - Codex reads the cache file at most once per cache window.
  - `agy models` is spawned at most once per cache window, never per render.
- Privacy: read only the model `slug`/id, display name and effort lists.
  - Never surface, log or store `identity`, `etag` or any account field.
  - Test fixtures are synthetic: no real account, plan, path or hostname.
- No credential, login, logout or account change for Codex or agy.
- A session's own report (`model/list`, `configOptions`) wins over the
  session-less listing once it exists.
- A listing failure stays that harness's `Error` alone. The dialog stays
  usable for the other harnesses.

## Prior Decisions

Local ticket/history fallback on 2026-11-18: `rationale.query` is unavailable
in this session; these pointers are not an exhaustive rationale-index result.

- 261118-feat-ws-agy-tool-names-and-worker-role (2026-11-18, Decisions (157)): "agy models are exposed without the level suffix, and effort is composed separately." — bearing: constrains
- 261118-feat-ws-agy-tool-names-and-worker-role (2026-11-18, Phase 2 Result): "auto-allows permission requests through the ordinary traced AnswerApproval path." — bearing: supports
- 261117-feat-ws-hosted-antigravity-adapter (2026-11-18, Phase 2 Result): "A lead with no permission_mode starts in agy's own default." — bearing: constrains
- 261115-feat-ws-hosted-codex-adapter-contract (2026-11-18, Phase 2 Result): "Model/list supplies actual model names and effort choices without starting a process on a dashboard read." — bearing: constrains
- 261115-feat-ws-permission-profile (2026-11-15, Decisions 5): "retain native permission_mode and observed active_mode. Keep permission and capability dimensions independent." — bearing: constrains
- 261118-refactor-ws-split-agy-adapter (2026-11-18, Decisions (163)): "split later through this ticket; no ws-size-guard override for agy.go." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/codex.go, agy.go, runner_events.go; existing catalog.go and Support choices feed the dashboard |
| scope.surface | public-interface | hosted-lead permission_modes gains bypass in the existing choices/settings interface; Choices model/effort response shape is retained |
| scope.new_public_symbol | no | no new exported Go symbol is required or named; bypass is a new value of existing AgyPermissionModes |
| scope.new_type_contract | yes | bypass becomes a ws-side permission_mode distinct from native agy modes; existing ModelChoice and ChoiceLister signatures remain unchanged (support.go#L57-L72) |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/codex_test.go#L449-L479, agy_worker_test.go#L142-L211, lead_runner_test.go#L359-L405, support_test.go#L496-L520; new test filenames are not specified |
| complexity.reuse_points | confirmed | read Support.listChoices and choicesTTL/choicesFailTTL/choicesTimeout, pi.go runListing, agyBaseModel, agyEnv, addApproval/answerApproval and codexModels |
| complexity.side_effect_risk | high | reads an account-associated Codex cache and spawns agy models; bypass auto-allows a lead's permission requests |
| risk.correctness | high | native mode reports can overwrite Record.PermissionMode (runner_events.go#L113-L129); bypass intent must survive reports, resume and holder reattach without duplicate answers |
| risk.fit | high | ws-side bypass must coexist with native SetPermissionMode, descriptor-driven WorkerAutoApprove and separate common permission intent; session reports must take precedence over cached fallback choices |
| risk.test | high | synthetic tests can cover parsing and auto-allow, but fresh-daemon model lists and real shell approval behavior require the separately planned live check; no providers were run during fact population |
| risk.security_or_contract | high | unconditional lead auto-allow is an approval boundary, cache metadata is account-associated, and bypass downgrade compatibility remains an open lead decision |

## Phases

### Phase 1: Session-less model lists and the agy lead bypass mode

- **Codex `Choices`:** when `codexChoices` is empty, parse
  `models_cache.json` from `$CODEX_HOME` (else `~/.codex`) into
  `ModelChoice{ID: slug}`. Efforts are the union of each entry's
  `supported_reasoning_levels`, lowest first. A missing or malformed file is
  an empty list, not an error.
- **agy `Choices`:** when `agyChoices` is empty, run `agy models` through
  the `runListing` seam.
  - Skip the progress line and fold ids with `agyBaseModel` into
    deduplicated base names, as (157) requires.
  - Efforts stay the session-reported list, else `agyEfforts`.
  - Keep `Executable()` and the profile's environment the same as a session
    start.
- **agy `bypass` mode:**
  - Add `bypass` to the modes the dashboard offers a hosted agy lead.
  - At session start and resume, map it to the native mode per (169).
  - Store the bypass intent per (170)+(172). Native mode reports keep
    updating `PermissionMode` as today (`runner_events.go:113-129`); they do
    not touch the bypass field, so bypass survives mode reports, resume and
    holder reattach.
  - Extend the auto-allow at `runner_events.go:286` to a lead record whose
    bypass field is set, keeping at-most-once answer tracking and
    tracing.
  - Other modes and other backends keep today's behavior.
- **Verification:**
  - Unit tests with synthetic fixtures for the Codex cache parse (including
    a file with an `identity` block that never leaks into choices) and for
    the agy listing parse (suffix folding, progress line, a failure
    surfacing as `Error`).
  - Tests that a `bypass` agy lead's request is auto-allowed, and that a
    `default` lead's request is not; that a native mode report leaves the
    bypass field set; and that a Record written with the field round-trips
    through a decoder that does not know it (the older-daemon case).
  - `cd agents-plugin-tool && go build ./... && go vet ./internal/wsagent
    ./cmd/ws-mcp && go test -timeout 30m ./internal/wsagent ./cmd/ws-mcp`.
  - Live check after the lead's daemon swap: the dialog lists Codex and agy
    models right after a restart, and a `bypass` agy lead runs a shell
    command with no prompt.

#### Edition (261119-feat-ws-codex-hosted-worker) - 2026-11-19

`261119-feat-ws-codex-hosted-worker` (173) and (175) supersede two parts of
this phase's Codex `Choices` plan, from the owner's live report that the
dashboard kept the old Codex model list after a CLI update (owner answer
2026-11-19 12:27, "175=a"):

- The "missing or malformed file is an empty list, not an error" contract
  becomes: a file that does not exist is an empty list with no error; an
  existing file that is empty or unparseable is an error, so Support's 30 s
  fail TTL applies instead of a 5 min success.
- The non-expiring "native session reports immediately override cached
  fallbacks" precedence becomes: the session-reported `codexChoices` expires
  on the shared choices TTL, and a newer session report replaces it.

The Constraints line "A session's own report (model/list, configOptions)
wins over the session-less listing once it exists" is superseded the same way.

The implementation of both lands under that ticket's Phase 1 item 5.

## Implementation Handoff - 2026-11-18

Implementation is committed at `795074e21`; Phase 1 is not closed pending the
lead's independent review and post-swap live check.

- Added session-less Codex cache parsing and bounded `agy models` listing.
  Support retains the existing success/failure TTLs. Native session reports
  immediately override cached fallbacks, including cached errors. Synthetic
  tests cover cache expiry, parsing, privacy, environment/command parity,
  per-harness error isolation and a session report arriving during a listing.
- Added top-level `Record.agy_bypass` intent, native `default` launch/control
  projection and `bypass` settings projection. Auto-allow uses the normal
  tracked answer path, including already-waiting requests when enabled and
  unanswered requests recovered by holder replay. Tests cover default lead
  prompts, enable/disable, native reports, stop/resume, holder reattach,
  duplicate-answer refusal and a typed older-decoder rewrite.
- Compatibility remains **T1** for bypass and **T0** for choices. At base
  `07b96092f`, `record.go:182-214` reads known fields and retains unknown
  top-level raw members, `record.go:219-236` overlays those members on write,
  and `record.go:304-327` uses that decoder/encoder for record persistence.
  Current `record.go:185-237` preserves the same mechanism while recognizing
  the optional field. An older daemon still uses native `PermissionMode`
  (`runner_process.go` at base, line 294), preserves the unknown bypass flag,
  and prompts; a new daemon's `agy_bypass.go:33-38` and
  `runner_process.go:294` restore native default plus ws auto-allow.
- Local Mac verification passed:
  - `cd agents-plugin-tool && go build ./...`
  - `go vet ./internal/wsagent ./cmd/ws-mcp`
  - `go test -timeout 30m ./internal/wsagent ./cmd/ws-mcp`
    (`118.173s`, `175.313s`).
  - Focused new tests passed, then passed with `-race -count=10` (`3.409s`).
  - Changed-file `gofmt -l` and `git diff --check` were clean.
  - Initial focused failures were test-harness errors: a Claude-only stream
    barrier and an assertion before asynchronous native mode reporting.
    Both were corrected without changing the required behavior.
- Performance evidence: the listing seam tests count one agy process per
  cache window and zero additional processes on cached requests. Codex tests
  remove the synthetic file after listing and retain choices until expiry.
  Source-level incremental budget is one file read or one listing spawn per
  uncached fallback, zero on cached/session-reported choices, no added render
  I/O. No Windows syscall or latency measurement is claimed.
- Tooling blocker: this worker session exposes no `workflow_manual`,
  `route.resolve_implement`, `tickets.acquire`, delegation/render/review or
  `tickets.close` tools. Bundled workflow/ticket guidance was read, but no
  route allocation or independent review can be claimed. The lead must run
  the missing workflow/review steps and record the final Result/closure.
- Deliberately omitted: shared daemon operations and the post-swap live check
  (lead-owned); account/login/logout commands; reading any actual Codex cache;
  dashboard assets; push and merge. No approved visual reference is linked.
