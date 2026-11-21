---
title: "agy 플랜 사용량 표시 (CLI /usage)"
parent: 261114-epic-ws-common-session-foundation
related:
  261118-research-ws-agy-plan-usage-source: "superseded; this ticket covers the agy CLI /usage source and gutter display"
  261113-feat-ws-dashboard-rail-usage-gutter: "the gutter, /rail/usage, and popover this ticket extends"
  261115-bug-ws-dashboard-fixture-real-home-plan-usage-probe: "fixture-isolation hazard the fake-agy tests must avoid"
sage-review-completeness: completed
sage-review-design: completed
sage-review-design-reviewed: a90f0331abaed07c
sage-review-completeness-reviewed: a90f0331abaed07c
completed: 2026-11-20
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
"10분 정도 비동기 디바운스, 화면 보일 때만"). The "only while visible"
(화면 보일 때만) part is superseded by D14, which allows probing on user action
only:

- **D1 — Usage source (`agy` CLI `/usage`).** Acquire `agy` plan usage via the
  `agy` CLI print-mode `/usage` command (`--output-format json`); ws never reads
  `agy` Keychain entries or OAuth tokens.
  - *Rejected:* Reading Keychain tokens and calling the private
    `cloudcode-pa` `v1internal` HTTP endpoint (private endpoint, no token
    refresh, credential-handling risk).
- **D2 — Async, debounced; never on an interactive path.** A probe runs
  asynchronously with a ~10-minute debounce and never on an interactive path.
  The original "visibility-gated background polling" part is superseded by D14
  ((1413)=d): no probe runs on a timer.
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

Confirmed by the owner on 2026-11-19 (board items (1402)-(1409)):

- **D6 — Two entries, one per `agy` group (1402)=a.** Each `agy` group is
  its own gutter entry, using the shared `agy` badge and a group label. The
  existing per-entry footer column and popover card code renders it.
  - *Rejected:* one `agy` entry with two sub-groups, because it adds a nesting
    level to the wire and the UI. Also rejected: showing only the more-consumed
    group or an aggregate in the footer, because it hides which group a number
    belongs to.
- **D7 — Bucket mapping (1403).**
  - Map `window: "5h"` → `five_hour` and `"weekly"` → `seven_day`.
  - Compute `percent = clamp((1 - remaining_fraction) * 100, 0, 100)`.
  - Map `reset_time` → `resetsAt`.
  - Drop a bucket with an unknown window, a missing or non-numeric fraction, or
    an unparsable reset time. The other buckets are kept.
  - *Rejected:* remaining-percent, because every existing `percent` is the
    consumed share and `pace()` (`usage.js#L18-L23`) compares it against the
    elapsed share. Also rejected: new window kinds, because `pace()`, `label()`,
    the chart geometry and the footer keys switch on exactly these two kinds.
    Also rejected: failing the whole fetch on one malformed bucket.
- **D8 — `disabled` buckets (1404)=c.** The footer omits a bucket with
  `disabled: true`, and the popover shows it dimmed. A group whose buckets are
  all disabled has no footer columns.
  - *Rejected:* omitting it everywhere. Also rejected: showing it dimmed in the
    footer too.
- **D9 — Subprocess bounds (1405).**
  - Run with `--print-timeout 20s`, a Go context timeout of 30s (kill on
    expiry), and a 512 KiB cap on combined stdout and stderr.
  - A timeout or cap overflow records "usage unavailable" and keeps the
    previous cached value.
  - *Rejected:* reusing the 10s Codex REST timeout (`plan_usage_probe.go`
    `query`), because an `agy` print run starts a CLI process and makes a
    backend round trip. The bounds have not been measured on a cold start.
- **D10 — No version probe; hosted-launch binary resolution (1406).**
  Reopened after design review and reconfirmed on 2026-11-19.
  - The probe resolves the `agy` binary the way hosted `agy` launches do:
    wsagent `harnessCommand(command, "agy")`
    (`agents-plugin-tool/internal/wsagent/backend.go#L498`). This honours a
    configured command override and unwraps Windows `.cmd`/`.bat` shims.
  - Resolution spawns nothing. The probe is skipped when the binary is absent.
  - Output without `command.name == "usage"` or `command.data.groups` is
    treated as unsupported and records "usage unavailable".
  - *Rejected:*
    - a bare `exec.LookPath("agy")`, which can pick a different binary than
      hosted sessions do, or launch a Windows shim unsafely;
    - spawning `agy --version` to gate on `>= 1.1.11`, which adds a second
      subprocess per probe and hard-codes an outside version floor. An old
      `agy` that bills `/usage` as a prompt is caught after one run by the D3
      billed-turn latch.
- **D11 — Cold start never probes inline (1407).** When the `agy` store is
  empty, `/rail/usage` returns at once without `agy` entries. Under D14 no
  probe runs until the first usage-popover open or refresh. That probe runs in
  the background, and a later refresh shows the values (D2,
  `ai-docs/manuals/performance.md` Rules 5 and 6).
  - *Rejected:* an inline first probe like the Codex one
    (`plan_usage_probe.go#L35-L39`), because a subprocess spawn of up to 30s on
    `/rail/usage` contradicts D2.
- **D12 — Separate history file `plan-usage-agy-history.json`, T0 (1408)=b.**
  - `agy` observations go into `plan-usage-agy-history.json`, with the same
    sample shape, 14-day age limit, 5000-sample cap and pruning as
    `plan-usage-history.json` (`plan_usage_history.go#L14-L17`).
  - The popover chart for an `agy` entry reads this history.
  - `plan-usage-history.json` stays untouched (D4). An old daemon ignores the
    new file (T0, same reasoning as D4).
  - *Rejected:* no `agy` history, with cards showing only the current value and
    pace. The owner chose history against the handler's recommendation.
- **D13 — The auth-required hold clears only on daemon restart (1409)=a.**
  While the hold is on, the popover shows a neutral "sign-in required" status.
  The billed-turn latch (D3.1) stays for the process lifetime.
  - *Rejected:* clearing the hold on a popover open (forced refresh), because
    popover opens are frequent and could trigger a sign-in prompt each time.
    Also rejected: expiring the hold after a fixed period, because it would
    prompt periodically with no user intent.
- **D14 — `agy -p /usage` runs only on user action (1413)=d.** Owner,
  2026-11-19.
  - The probe runs only when the usage popover is opened or refreshed (a
    `/rail/usage?refresh=1` request), within the ~10-minute debounce (D2).
  - Ordinary page polls and visibility changes read the cache and never spawn
    `agy`.
  - The gutter shows the last cached value and its age.
  - This supersedes the visibility-gated background-polling part of D2 and the
    owner's board item `(1323)`. The debounce and the asynchronous,
    off-interactive-path execution stay.
  - Why: a launch on user action, like a hosted `agy` session, does not
    conflict with `.done/261116-bug-ws-exe-launch-elimination`'s rule against
    timer-driven subprocess usage acquisition. It works on every OS, and
    Google's developer forum advises against timer polling.
  - Research before the choice found no harmless live-ACP-session path. There
    is no advertised non-prompt ACP quota channel. A `/usage` prompt would
    create ws-visible turns, replies and parent reports, with unverified
    billing. `--process-per-session` makes any extra session a new process.
  - *Rejected:*
    - a timer on every OS, with console suppression on Windows (risks the
      AhnLab V3 alerts that 261116 removed);
    - a timer on macOS/Linux only;
    - an opt-in setting with the timer off by default.

- **D15 — One extra popover follow-up after the probe bound (1414).** While
  the popover is still shown, it schedules one more cache-only refetch
  (`refresh` absent) about 32s after opening, after the probe's 30s outer
  bound (D9). This is in addition to the existing 2s and 11s `followUp()`
  refetches (`usage.js#L341-L345`). The timer is cleared on close like the
  existing ones, and it never spawns `agy`.
  - *Rejected:* leaving the result to the next ordinary 5-minute poll
    (`usage.js#L372`), which usually misses the open popover now that a
    popover open is the only probe trigger (D14). Also rejected: a longer
    polling loop.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/performance.md (declared for agents-plugin-tool/)
- **`ai-docs/manuals/performance.md`:** Interactive paths (page load, chat
  send/delivery) must not block on file I/O or subprocess spawns (Rules 5 and 6).
  Spawning `agy -p /usage` is strictly off the interactive path. It runs
  asynchronously, only on popover open or refresh, under a ~10-minute debounce
  (D2, D14). There is no in-process alternative that avoids Keychain/OAuth
  reading (D1). Skip
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

## Prior Decisions

- 261113-feat-ws-dashboard-rail-usage-gutter (2026-11-13, Decisions 5): "the Codex/GPT query runs when a viewer opens the popover and otherwise at most every 5 minutes while at least one dashboard page is connected ... Rejected: a resident app-server process; polling with no viewer connected." — bearing: constrains
- b9c3611a3 (2026-11-17, commit): "refresh() answers from the cached GPT entry. A due probe ... runs in one background goroutine at a time. Only a store with no GPT value at all ... probes inline, so a first page still gets numbers." — bearing: constrains
- da771b684 (2026-11-17, commit): "Cache the last successfully written plan-usage.json bytes under the existing store mutex; skip byte-identical saves and retain the previous bytes after write failures" — bearing: supports
- 261114-feat-ws-dashboard-usage-history-trend (2026-11-14, Route Facts): "history must remain credential/account-free and preserve latest cache/API semantics across old/new readers and writers" — bearing: constrains
- 4c50ad056 (2026-11-16, commit): "fix(wsdashboard): acquire Codex usage through file-selected OAuth REST" — bearing: constrains
- .done/261116-bug-ws-exe-launch-elimination (2026-11-16, Decisions): "replace Codex usage acquisition with read-only HTTPS, no subprocess on any OS" (AhnLab V3 flags repeated executable launches on the owner's Windows PC) — bearing: constrains; resolved by D14 ((1413)=d: user-action launches only, no timer)
- 261115-bug-ws-dashboard-fixture-real-home-plan-usage-probe (idea, 2026-11-15): "Dashboard fixture inherits real HOME and launches a real Codex child during plan-usage handling" — bearing: constrains
- 261118-research-ws-agy-plan-usage-source (2026-11-19, Outcome): "Superseded by 261119-feat-ws-agy-plan-usage-display (todo/), which captures the live agy -p /usage --output-format json probe, Orca prior art, and owner-confirmed decisions (D1-D5)" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/plan_usage.go, plan_usage_probe.go, assets/usage.js, assets/usage.css, new agy fetcher and store files, tests |
| scope.surface | cross-module | Go daemon store and /rail/usage wire (daemon.go#L826, plan_usage.go#L333) plus embedded dashboard asset usage.js; no exported Go symbol |
| scope.new_public_symbol | no | none; wsdashboard is under internal/, new fetcher and store types stay unexported |
| scope.new_type_contract | yes | new plan-usage-agy.json and plan-usage-agy-history.json file shapes, and one /rail/usage entry per agy group (D6) with a disabled-bucket marker (D8) |
| scope.test_surface | new-files | existing plan_usage_probe_test.go, plan_usage_compat_test.go, usage.test.js cover Claude/Codex only; fake-agy fetcher tests and agy UI capture are new files |
| complexity.reuse_points | confirmed | planUsageStore.refresh background-probe pattern (plan_usage_probe.go#L33-L50), writePrivateFile with byte-identical skip (plan_usage.go#L98-L108), usage.js drawFooter and drawPopover (assets/usage.js#L125-L150, #L291-L308) |
| complexity.side_effect_risk | high | spawns the external agy CLI that can bill a prompt or trigger a browser sign-in; an existing idea ticket records a fixture launching a real child with the real HOME |
| risk.correctness | moderate | process-lifetime latch and hold state (D3, D13), the D7 bucket mapping, and per-group history identity (D6, D12) |
| risk.fit | high | one-entry-per-harness assumption in usage.js (entry.harness name switch at #L134 and #L298) must change for two per-group agy entries (D6) |
| risk.test | moderate | needs a fake agy binary, fixtures isolated from real HOME, and a UI capture; no existing agy usage tests |
| risk.security_or_contract | high | credential and privacy boundary (no Keychain or token read, no raw subprocess output cached) and T0 compatibility of plan-usage.json and plan-usage-history.json alongside two new files |

## Phases

### Phase 1: agy CLI /usage fetcher, store, UI gutter, and tests

Implement asynchronous `agy` plan-usage acquisition, persistence, UI rendering,
and verification:

- **Fetcher (`wsdashboard`):**
  - Resolve `agy` via `harnessCommand(command, "agy")` without spawning.
    Skip the probe if it is absent (D10).
    - `harnessCommand` is unexported, and wsagent already imports wsdashboard
      (`agent_tools.go`), so wsdashboard cannot call it directly. Inject the
      resolver where both packages are built (`cmd/ws-mcp/dashboard.go`), or
      move `harnessCommand` and its shim helpers to a lower shared package.
      Planning picks which.
    - Pass the same `Command` the hosted `AgyBackend` uses. Today that is
      empty (`cmd/ws-mcp/dashboard.go#L306`), so there is no config override
      yet.
  - Invoke `agy -p /usage --output-format json --print-timeout 20s` with
    `AGY_CLI_DISABLE_AUTO_UPDATE=true` and null stdin, under a 30s context
    timeout and a 512 KiB output cap (D9).
  - Parse `command.name == "usage"` and `command.data.groups[]`, mapping the
    buckets per D7. Treat any other shape as unsupported (D10).
  - Enforce the billed-turn latch (`num_turns > 0` or a non-empty
    `conversation_id`) for the process lifetime.
  - Enforce the auth-required hold (stderr contains `"Authentication required.
    Please visit the URL"`) until daemon restart (D3, D13).
  - Raw stdout and stderr are never cached or returned to viewers.
- **Store & endpoint (`wsdashboard`):**
  - Persist `agy` usage in `plan-usage-agy.json`, skipping byte-identical
    rewrites.
  - Append `agy` observations to `plan-usage-agy-history.json` with the
    existing history's sample shape, age limit, cap and pruning (D12).
  - `plan-usage.json` and `plan-usage-history.json` stay unchanged (D4, T0).
  - Probe only from a popover open or refresh (`/rail/usage?refresh=1`),
    asynchronously, at most once per ~10 minutes (D2, D14).
    - Ordinary polls (`refresh` absent) and visibility changes only read the
      cache. They never spawn `agy`, including on an empty store, so there is
      no probe before the first popover open (D11).
    - Unlike Codex's 5s forced interval, a forced refresh does not bypass the
      ~10-minute debounce.
    - The debounce window starts when a probe is spawned, whatever its
      outcome: success, failure, timeout, oversize, hold or latch. A failed
      probe is not retried inside the window.
  - An `agy` run can take up to 30s, longer than the existing `followUp()`
    refetches wait (2s and 11s). D15 adds one cache-only refetch at about
    32s while the popover is shown.
  - Return one `/rail/usage` entry per `agy` group (D6). Each group keeps an
    identity that the popover chart can match, so one group's history samples
    never plot on the other group's chart.
  - Group identity is a planning choice under these constraints:
    - Samples are filtered by `{harness, kind}` (`usage.js#L40`,
      `projectPlanUsageHistory`). The badge is keyed on `entry.harness`
      (`usage.js#L102`).
    - Either the two entries share harness `agy` and the projection and
      `trend()` gain a group key, or each group gets its own harness value and
      the badge mapping changes.
    - Any group key stored in a sample is a fixed ASCII identifier mapped from
      the known group names, never raw `agy` output. `usageTrendBefore`
      assumes fixed ASCII sources.
    - `agy` history reaches the viewer either merged into the single
      `history` member or as a separate member with its own `status`.
  - Once a probe has run but no group is cached (first-ever probe hit the
    hold, or failed), `/rail/usage` returns one status-only `agy` entry with
    the shared badge, no group label and no windows. It carries "sign-in
    required" or "usage unavailable", following the existing detected-but-empty
    Codex entry (`entries()` in `plan_usage.go`, `usage.js#L305`). There is no
    `agy` entry until the first probe completes (D11, D14).
  - Disabled buckets carry a marker the UI can dim (D8).
  - The latch and hold states surface only as neutral status text: "usage
    unavailable" or "sign-in required".
- **UI (`assets/usage.js`, dashboard styles, i18n):**
  - Render each `agy` group as its own entry with the shared `agy` badge and a
    group label, in the gutter footer and popover next to Claude and Codex
    (D6). Replace the two-way harness label switch (`#L134`, `#L298`).
  - Omit disabled buckets in the footer and show them dimmed in the popover
    (D8).
  - Draw the popover chart of an `agy` entry from its `agy` history (D12).
  - The footer shows each `agy` group's last cached value with its observation
    age visible, because the value refreshes only on popover use (D14). The
    form of the age is a planning choice; the existing `age()` text and the
    30-minute `stale()` clock (`usage.js#L14-L17`) can be reused.
  - Show the "sign-in required" status while the hold is on (D13).
  - Show "usage unavailable" (failure, unsupported output, billed-turn latch)
    through the existing per-entry popover status line (`usage.js#L305`). The
    last cached windows stay visible (D9).
  - All labels and statuses stay downstream-neutral.
- **Verification:**
  - Go unit tests drive a fake `agy` binary on `PATH` with fixture JSON. Every
    test runs with a temporary `HOME` and cache directory, so no test inherits
    the real `HOME` or launches a real `agy` (see
    `261115-bug-ws-dashboard-fixture-real-home-plan-usage-probe`). They cover:
    - parsing both groups' `5h` and `weekly` buckets, including the D7 clamp
      and malformed-bucket drop;
    - `disabled` buckets carried with their marker;
    - the billed-turn latch, triggered by `num_turns > 0` and separately by a
      non-empty `conversation_id`;
    - the auth-required hold persisting across forced refreshes;
    - ordinary `/rail/usage` polls never spawning `agy`, including on an
      empty store;
    - a forced refresh (`refresh=1`) spawning one background probe;
    - a forced refresh inside the ~10-minute debounce not spawning `agy`,
      including after a failed probe;
    - a first-ever probe hitting the hold, or failing, yielding the status-only
      `agy` entry;
    - timeout and oversize output recording "usage unavailable" while keeping
      the previous value;
    - a missing binary skipping the spawn;
    - an empty store answering a forced `/rail/usage` without waiting on the
      probe;
    - samples appended to and pruned in `plan-usage-agy-history.json`;
    - fixtures and cache files holding no raw subprocess output.
  - T0 compatibility: the `plan-usage.json` and `plan-usage-history.json`
    bytes and shapes stay unchanged after `agy` probes. An old-reader check
    (`plan_usage_compat_test.go`) still ignores both new files.
  - `usage.test.js` coverage:
    - two `agy` entries labeled per group with the shared badge;
    - disabled buckets omitted in the footer and dimmed in the popover;
    - the per-group chart drawing only its own group's samples;
    - the "sign-in required" status;
    - the footer showing an `agy` group's value together with its age;
    - the D15 ~32s cache-only refetch, scheduled while the popover is shown and
      cleared on close.
  - A UI capture of the plan-usage gutter and popover showing both `agy` groups
    alongside Claude and Codex.

### Result (ef6b384) - 2026-11-20

- **Fetcher, store, and T0 persistence (`c7c231c`, `ef6b384`):**
  - Added `(AgyBackend).ResolveCommand()` in `agents-plugin-tool/internal/wsagent/agy.go` (delegating to `harnessCommand(b.Command, "agy")`) and injected it via `DaemonOptions.AgyCommand` from `agents-plugin-tool/cmd/ws-mcp/dashboard.go` so `wsdashboard` avoids a cyclic import and daemon test fixtures stay isolated from any host `agy` binary.
  - Added `plan_usage_agy.go`, `plan_usage_agy_unix.go`, and `plan_usage_agy_windows.go` implementing `agy -p /usage --output-format json --print-timeout 20s` with `AGY_CLI_DISABLE_AUTO_UPDATE=true`, null stdin, 30s context timeout + process-tree termination (`execjob.KillTree`), Windows `CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP | HideWindow`, and a mutex-guarded 512 KiB combined stdout+stderr cap (`cappedOutput`).
  - Implemented D7 bucket mapping (`5h` -> `five_hour`, `weekly` -> `seven_day`, clamped `percent` rounded to remove float noise, malformed-bucket drop, `Disabled` flag), D3/D13 Orca safety latches (process-lifetime billed-turn latch on `num_turns > 0` or non-empty `conversation_id`, daemon-lifetime auth-required hold on stderr match), and D2/D11/D14 ~10-minute spawn-start debounce triggered only on `/rail/usage?refresh=1`.
  - Stored `agy` cache in `plan-usage-agy.json` (byte-identical skip) and observations in `plan-usage-agy-history.json` (14-day / 5000-sample pruning, fixed ASCII group keys `gemini` and `claude_gpt`). Left `plan-usage.json` and `plan-usage-history.json` unchanged (T0). Emitted `/rail/usage?history=1` `agyHistory` only when `agy` entries are present so non-`agy` responses keep exact backward shape parity.
  - Added fake-`agy` unit tests (`plan_usage_agy_test.go`, temp `HOME` + `PATH`), T0 non-destruction compatibility coverage (`plan_usage_compat_test.go`), and `ResolveCommand` tests (`cmdshim_test.go`).
- **UI gutter, popover, i18n, and browser capture (`7aa7fe5`):**
  - Updated `assets/usage.js` and `assets/usage.css` to render per-group `agy` entries (`Gemini Models` / footer `Gemini`, `Claude and GPT models` / footer `Claude/GPT`) with the shared `agy` badge, compact two-line `.usage-group` with visible observation age inside the 34px single-row handle, disabled buckets omitted from footer and dimmed in popover (`.usage-disabled` / `.usage-disabled-tag`), per-group history isolation via `(harness, group, kind)`, localized `"sign-in required"` status, and D15 32s cache-only follow-up timer cleared on close.
  - Added `js.usage.signinrequired` and `js.usage.disabled` across all 11 locale tables under `internal/wsi18n/tables/`.
  - Added Node tests in `usage.test.js` and `usage-trend.test.js`, plus a Chromium UI capture harness in `testdata/usage-agy.browser.cjs` (dark/light gutter + popover verified).
- **Review & verification:**
  - Independent `code-review-lite` over `d1aa3aa..7aa7fe5` found 0 Critical, 0 Important, and 1 Minor (`cappedOutput` unlocked read after `cmd.Run()`), fixed in `ef6b384`.
  - Verified green after `ef6b384`: host and `GOOS=windows` `go vet` (`./internal/wsdashboard ./internal/wsagent ./cmd/ws-mcp`), `go test -race -count=1 -run 'TestPlanUsageAgy' ./internal/wsdashboard`, `go test -count=1 -run 'TestPlanUsage|TestCSS|TestOrbitCSS|TestAsset' ./internal/wsdashboard`, `go test -count=1 -run 'TestHarnessCommand' ./internal/wsagent`, `go test -count=1 ./internal/wsi18n`, `node --test` on `usage-axis.test.js`, `usage-trend.test.js`, and 17/17 targeted tests in `usage.test.js` (6 pre-existing unrelated legacy SVG chart tests on `develop` untouched), and Playwright Chromium `usage-agy.browser.cjs` capture.
