---
title: "agy 플랜 사용량 표시 (CLI /usage)"
parent: 261114-epic-ws-common-session-foundation
related:
  261118-research-ws-agy-plan-usage-source: "superseded; this ticket covers the agy CLI /usage source and gutter display"
---

# agy plan usage display (CLI /usage)

## Background

The dashboard's plan-usage gutter
(`agents-plugin-tool/internal/wsdashboard/plan_usage.go`, `assets/usage.js`)
tracks Claude and Codex/Pi windows only. For `agy`, ws currently maps only the
per-session context window (`usage_update {used, size}`) and has no plan quota
or rate-limit display (`261118-research-ws-agy-plan-usage-source`).

A live probe and prior-art check on 2026-11-19 established that the `agy` CLI
print-mode `/usage` command exposes both model-group quotas directly in JSON,
superseding the open source-discovery questions in
`261118-research-ws-agy-plan-usage-source`.

## Verified Findings

- **Live `agy` CLI probe (lead, 2026-11-19 on macOS, `agy` 1.5.12):**
  Running `agy -p /usage --output-format json --print-timeout 20s` with env
  `AGY_CLI_DISABLE_AUTO_UPDATE=true` and stdin `</dev/null` returned:
  - `status`: `"SUCCESS"`
  - `num_turns`: `0`
  - `conversation_id`: `""` (empty)
  - `command.name`: `"usage"`
  - `command.data.groups[]`: `{name: "Gemini Models" | "Claude and GPT models", buckets[]: {name, window: "weekly" | "5h", remaining_fraction, reset_time, disabled}}`
- **Existing store & UI constraints (verified in repo):**
  - `agents-plugin-tool/internal/wsdashboard/plan_usage.go:37-43,89-108` holds
    fixed `claude, gpt planUsageEntry` fields and marshals/unmarshals fixed
    struct `{Claude planUsageEntry json:"claude", GPT planUsageEntry json:"gpt"}`
    to `plan-usage.json`.
  - `agents-plugin-tool/internal/wsdashboard/plan_usage_history.go:44-48,143-148`
    validates every sample with `validPlanUsageSample` (accepting only `claude`
    with `"Claude push"` and `codex` with `"via Codex CLI"` / `"via Pi"`, and
    only `"seven_day"` or Claude `"five_hour"`), rejecting the entire
    `plan-usage-history.json` file with `"invalid usage history sample"` if any
    record fails.
  - `agents-plugin-tool/internal/wsdashboard/assets/usage.js:112-146,281-294`
    renders `data.entries` in the footer and popover, currently labeling
    entries as `"Claude"` or `"Codex / GPT"` and iterating `["five_hour", "seven_day"]`.

## Prior Art

- **Orca 1.4.257** (`github.com/stablyai/orca`,
  `src/main/rate-limits/providers/antigravity/antigravity-usage-command.ts`,
  `antigravity-usage-fetcher.ts`):
  - Uses the same `agy -p /usage --output-format json` command; requires
    `agy >= 1.1.11`.
  - Latches polling off for the process lifetime if `num_turns > 0` or
    `conversation_id` is non-empty (indicating `/usage` was billed as a prompt).
  - Holds polling indefinitely when stderr contains
    `"Authentication required. Please visit the URL"` (avoids repeated browser
    sign-in prompts).
  - Uses a 30s timeout and 512 KiB output cap.

## Decisions

Confirmed by the owner on 2026-11-19 (board item `(1323)=a` with note
"10분 정도 비동기 디바운스, 화면 보일 때만"):

- **D1 — Usage source (`agy` CLI `/usage`).** Acquire `agy` plan usage via the
  `agy` CLI print-mode `/usage` command (`--output-format json`); ws never reads
  `agy` Keychain entries or OAuth tokens.
  - *Rejected:* Reading Keychain tokens and calling the private
    `cloudcode-pa` `v1internal` HTTP endpoint (private endpoint, no token
    refresh, credential-handling risk).
- **D2 — Async, debounced, visibility-gated polling.** Polling is asynchronous
  with a ~10-minute debounce and runs only while the dashboard is visible; never
  run on an interactive path.
- **D3 — Orca safety latches.** Implement both safety latches from Orca:
  1. **Billed-turn latch:** latch `agy` polling off for the daemon process
     lifetime if `num_turns > 0` or `conversation_id` is non-empty.
  2. **Auth-required hold:** hold `agy` polling indefinitely when stderr
     contains `"Authentication required. Please visit the URL"`.
- **D4 — Separate cache file (`plan-usage-agy.json`), compatibility tier T0.**
  Store `agy` plan usage in a separate `plan-usage-agy.json` file; leave
  `plan-usage.json` and `plan-usage-history.json` shapes unchanged.
  - *Compatibility verdict (T0):* An old daemon ignores `plan-usage-agy.json`
    (`agents-plugin-tool/internal/wsdashboard/plan_usage.go:87-108` reads and
    writes only `plan-usage.json`; pinned check in
    `plan_usage_compat_test.go:46-47` ignores everything beside
    `plan-usage.json`) and continues reading/writing `plan-usage.json` and
    `plan-usage-history.json` normally.
  - *Rejected:* Adding `agy` to `plan-usage.json` or `plan-usage-history.json`
    (**T3**): old `saveLocked()` (`plan_usage.go:98-108`) marshals the fixed
    `{claude, gpt}` struct and drops unknown keys on write, and old
    `parsePlanUsageHistory()` (`plan_usage_history.go:44-48,143-148`) rejects the
    entire `plan-usage-history.json` file when `validPlanUsageSample` encounters
    any harness/source/kind outside `claude` and `codex`.
- **D5 — Plan-usage gutter UI (`assets/usage.js`).** Display both groups'
  (`"Gemini Models"` and `"Claude and GPT models"`) `5h` and `weekly` buckets
  in the existing plan-usage gutter (`assets/usage.js`) next to Claude/Codex.

## Constraints

- **`ai-docs/manuals/performance.md`:** Interactive paths (page load, chat
  send/delivery) must not block on file I/O or subprocess spawns (Rules 5 and 6).
  Spawning `agy -p /usage` is strictly off the interactive path via
  ~10-minute debounced asynchronous background polling while visible (D2), with
  no in-process alternative that avoids Keychain/OAuth reading (D1). Skip
  rewriting `plan-usage-agy.json` when serialized bytes are unchanged (Rule 2,
  matching `plan_usage.go:103-107`).
- **`ai-docs/manuals/release-compatibility.md`:** Preserve T0 compatibility
  (D4): do not modify `plan-usage.json` or `plan-usage-history.json` schemas or
  break existing `/rail/usage` fields for `claude` and `codex`.
- **`ai-docs/manuals/shipped-surface-boundary.md`:** Any dashboard/UI labels,
  i18n strings, or status messages emitted by `agents-plugin-tool/` must stay
  downstream-neutral (no devenv-only ticket stems, commit hashes, or repo paths).
- **Credential & privacy boundary:** Never persist or expose auth documents,
  account identifiers, real usage values, or raw subprocess output/errors in
  caches, viewer responses, or tracked fixtures (matching
  `plan_usage.go:21-22` and `261118-research-ws-agy-plan-usage-source`).

## Open questions

The items below are not settled and must be resolved before `ready/` promotion:

- **Gutter & popover UI grouping for two `agy` groups:** D5 requires showing
  both `"Gemini Models"` and `"Claude and GPT models"` (`5h` and `weekly`
  buckets) next to Claude/Codex, while `assets/usage.js:117-145,281-294`
  currently assumes one entry per harness with one `five_hour` and one
  `seven_day` window. Decide exact entry shape, group labels, badge icon(s), and
  footer/popover layout.
- **Fraction-to-percent conversion & `disabled` buckets:** Confirm mapping
  `remaining_fraction` to consumed `percent` (`(1 - remaining_fraction) * 100`
  and clamping/validation) and how `disabled: true` buckets render or are
  omitted.
- **Timeout, output cap, and version check:** Confirm Go subprocess timeout and
  `--print-timeout` value (Orca uses 30s timeout and 512 KiB output cap; live
  probe used `--print-timeout 20s`), and whether/how to gate on `agy >= 1.1.11`
  or `agy` binary presence without spawning on interactive paths.
- **Cold-start behavior on first dashboard load:** `plan_usage_probe.go:35-39`
  probes Codex inline when `s.gpt` is empty, whereas D2 prohibits running on an
  interactive path. Confirm that an empty `agy` store also triggers an async
  probe rather than blocking `/rail/usage`, relying on follow-up refresh to
  populate the UI.
- **History tracking:** Whether `agy` observations omit trend history, or use a
  separate `plan-usage-agy-history.json` (keeping `plan-usage-history.json`
  untouched per D4).
- **Latch/hold reset semantics:** Whether manual refresh or only daemon restart
  can clear an auth-required hold (and confirm billed-turn latch stays for the
  process lifetime per D3).

## Phases

### Phase 1: agy CLI /usage fetcher, store, UI gutter, and tests

Implement asynchronous `agy` plan-usage acquisition, persistence, UI rendering,
and verification:

- **Fetcher (`wsdashboard`):** Invoke `agy -p /usage --output-format json --print-timeout <timeout>`
  with `AGY_CLI_DISABLE_AUTO_UPDATE=true` and null stdin under a bounded output
  cap; parse `command.name == "usage"` and `command.data.groups[]`; enforce the
  billed-turn latch (`num_turns > 0` or non-empty `conversation_id`) and
  auth-required hold (stderr `"Authentication required. Please visit the URL"`).
- **Store & endpoint (`wsdashboard`):** Persist `agy` usage in
  `plan-usage-agy.json` (unchanged `plan-usage.json` / `plan-usage-history.json`,
  T0), debounce probes at ~10 minutes, trigger only from visible dashboard
  activity asynchronously (never blocking interactive requests), and include the
  resulting entries in `/rail/usage`.
- **UI (`assets/usage.js` and dashboard styles/i18n):** Render both `agy`
  groups' `5h` and `weekly` buckets in the plan-usage gutter and popover next to
  Claude and Codex.
- **Verification:**
  - Unit tests using a fake `agy` binary and fixture JSON covering: successful
    parse of both groups (`5h` and `weekly`), billed-turn latch (`num_turns > 0`
    and non-empty `conversation_id`), auth-required hold, and timeout/oversize
    handling; plus T0 compatibility verification that `plan-usage.json` and
    `plan-usage-history.json` remain unchanged.
  - UI capture of the plan-usage gutter showing both `agy` groups alongside
    Claude/Codex.
