---
title: "대시보드 입력창 하단 컨트롤(모델·effort·권한) 및 ws compaction 기준 컨텍스트 미터기 개편"
related:
  261115-feat-ws-permission-profile: common permission_level and /agents/<id>/settings/permission route
  261117-feat-ws-hosted-compaction-lifecycle: hosted compaction thresholds, advisory/hard nudges, CompactEnd reset
  261115-feat-ws-whiteboard-agent-card: shared subordinate conversation/composer shell read-only consumer
  261114-feat-ws-dashboard-usage-footer-meter: top-bar plan usage popover replacing footer quota chips
---

# Dashboard Composer Footer Controls and ws-Compaction Context Meter

## Background

The live session composer (`dashboard.tmpl` `#composer`) currently has two UX and behavioral gaps:

1. **Asymmetric, noisy `.cfoot` chrome without inline controls.** `r-sline` (`pageview_band.go`, `dashboard-conversation.css`) renders workspace folder, 5h/7d quota chips, and session cost (`~$USD`), producing cluttered footer noise on Claude and uneven empty/folder-only rows on AGY, Codex, and Pi. Meanwhile, switching **Model**, **Effort**, and **Permission** requires opening the top-bar session menu (`dashboard-leads.js` `leadRows`). Removing `.cfoot` quota chips also risks hiding Codex's 5-hour window (`300m`), because `plan_usage_probe.go` currently keeps only `10080m` (`604800s`) weekly windows.
2. **Native-window context meter disconnected from ws compaction.** `pageview_band.go` `contextBar` scales `r-ctx` to raw `UsageContext.Window` (e.g., `1.0M`), draws hardcoded `.t80`/`.t95` CSS ticks, and displays trailing `.sb-flag` rectangle badges (`high`, `near limit`), even though ws compaction triggers at resolved `compaction.{lead,worker}_{advisory,hard}` thresholds (`wsconfig/compaction.go`). Hosted sessions in `wsagent/compaction.go` also lack the 1/3 and 2/3 intermediate nudges already present in `claude-mod/timing.ts` and `agents-plugin-pi/src/goal-loop.ts`.

Approved text mockup: [composer-meter-v2.html](ai-docs/assets/mockups/261120/composer-footer-context-meter/composer-meter-v2.html) (with sibling `composer-meter-v2.css` and `composer-meter-v2.js`).

## Decisions

### 1. Composer Footer (`.cfoot`) Redesign
- **Uniform stripped `.cfoot` across all 4 harnesses (`claude`, `codex`, `agy`, `pi`).** Remove workspace folder, 5h/7d usage chips, and session cost (`~$USD`) from `.cfoot` across all harnesses (`BandView` / `r-sline` no longer emits folder, limits, or cost in the composer footer; top-bar `/rail/usage` owns plan quota visibility).
- **Codex 5h window promotion (`plan_usage_probe.go`, `plan_usage_history.go`).** Accept the 5-hour window (`Seconds == 18000` / `Minutes == 300`) as `wslive.LimitFiveHour` (`"five_hour"`) alongside `seven_day` in `queryCodex` and `codexUsageEntry`, and allow `(harness == "codex", kind == "five_hour")` in `validPlanUsageSample` so removing `.cfoot` usage loses zero Codex quota telemetry.
- **22px slim borderless ghost tonal strip outside SSE-swapped `r-sline`.** Place the 3-button control strip (`.fpills` / `.fpill`, `min-height: 22px`, no resting border, subtle `--wb-wash` hover) inside `.cfoot` as a sibling *outside* `{{region . "sline"}}` so open popovers and keyboard focus survive SSE region swaps. Each trigger shows a `12px` monochrome Lucide SVG icon + concise value + tiny chevron (`▾`), with no uppercase `MODEL`/`EFFORT`/`PERMISSION` prefixes.
- **Common `wsagent` control wiring.** Reuse `dashboard-leads.js` (`leadChoices`, `leadModels`, `leadEfforts`, `leadModes`) against `GET /support/choices`, `POST /agents/<id>/settings`, and `POST /agents/<id>/settings/permission`, supporting common `permission_level` where enabled or native `permission_mode` per `/support/choices`.
- **Pi parity.** Keep the Permission slot visible in the exact same position on `pi`, rendered `disabled` with `shield-off` + `—`, muted opacity, no chevron, and a localized tooltip explaining Pi has no native permission mode.
- **Child-agent preview & read-only enforcement.** Expose read-only settings metadata on `SessionView` (`IsChild` / `Reg.Agent`, immutable spawn `Model`, `Effort`, `PermissionMode`/`PermissionLevel`, and `CanEditSettings`) and enforce `local && own` (`CanControl`) + non-lead settings guard on `POST /agents/<id>/settings` in `wsagent/http.go` (currently `leadControl` gates `settings` only when `r.IsLead(id)`). Live child pages (`/s/<child-id>`) and any subordinate shared composer shell (`261115-feat-ws-whiteboard-agent-card`) render all 3 controls `disabled` with a `10px` `lock` icon in place of chevrons.
- *Rejected alternatives:* keeping Claude-only usage chips; placing triggers inside SSE-swapped `r-sline`; thick bordered capsule pills with uppercase prefixes; hiding the Permission slot on Pi.

### 2. Context Meter (`r-ctx`) & Hosted 1/3·2/3 Nudge Parity
- **Hosted 1/3 & 2/3 milestones (`wsagent/compaction.go`).** Port the intermediate milestones from `claude-mod/timing.ts` (`thresholdsFor`, `levelAt`, `observe`) and `agents-plugin-pi/src/goal-loop.ts` into `conv`: `m1 = advisory + (hard - advisory) / 3`, `m2 = advisory + 2 * (hard - advisory) / 3` whenever `c.compactAdvisoryEnabled` is true and `hard > advisory` (both finite). Latch each milestone per compaction epoch in memory on `conv`, re-arm on usage drop below the milestone (or live `refreshCompaction`), and reset on `wslive.CompactEnd`.
- **Compaction metadata on `wslive.UsageContext`.** Add optional additive `Compaction *UsageCompaction` (`json:"compaction,omitempty"`) to `wslive.UsageContext` (`Soft`, `Hard`, `Milestones []int64`, `AdvisoryEnabled bool`, or resolved token points), populated by `wsagent` from lead vs worker `compaction.{lead,worker}_{advisory,hard}` thresholds and preserved through `wslive.normalizeUsage`. `pageview_band.go` `contextBar` consumes it server-side.
- **Bar scaling & text format.** Scale bar width and percentage to `hard` (falling back to native `window` when `hard` is unset/infinite): `x% [ progress bar ] <current> / <hard> (<native>)` (e.g., `28% [...] 51,293 / 180K (1.0M)`). Omit `(<native>)` when `hard == native` or in narrow (`<= 360px` container / `.is-compact` rail-hover) views. Overflow (`current >= hard`) shows unclamped `x%`, `100%` clamped bar fill, and inverted `crit` badge.
- **Dynamic ticks & floating boundary icon (no trailing status boxes).** Remove hardcoded `.t80`/`.t95` ticks and trailing `.sb-flag` rectangle badges (`high`, `near limit`). Render dynamic percentage tick markers (`m-soft`, `m-tick` for active 1/3 and 2/3) inside `.sb-bar` and float one `12px` crisp SVG icon directly above **only the most-recently-crossed boundary**: none below Soft -> amber `info` over Soft -> amber `bell` over 1/3 -> warm-orange `bell-ring` over 2/3 -> red `zap` right-aligned over Hard (`100%`). Rich hover/focus tooltip (`.ctx-tip`) displays Current / Soft / 1/3 / 2/3 / Hard / Native.
- *Rejected alternatives:* scaling against native 1M window; hardcoded 80%/95% ticks; ghost 1/3·2/3 ticks without hosted `wsagent` nudge parity; trailing `SOFT CROSSED` / `OVER HARD` rectangle badges.

### 3. Compatibility Tier: T0 Transparent
This change is **T0 transparent** under `ai-docs/manuals/release-compatibility.md`:
- `wslive.UsageContext` adds optional `omitempty` JSON fields; `wslive.ReadSnapshot` (`snapshot.go:429-441`) and `normalizeUsage` (`state.go:1335-1359`) ignore unknown fields and default absent ones without touching persisted schema version.
- `planUsageWindow` / `planUsageSample` (`plan_usage.go:24-30`, `plan_usage_history.go:22-30`) use existing `"five_hour"` kind for Codex; older readers retain `plan-usage-history.json` entries without schema break (`validPlanUsageSample` only drops unrecognized records in older readers' derived views without rewriting raw records, or new builds recognize Codex `five_hour`).
- `wsagent/http.go` preserves the existing `/agents/<id>/settings` vs `/agents/<id>/settings/permission` route split (`http.go:290-325`) while adding the `local && own` guard on non-lead settings writes.
- `SessionView` is server-rendered only (`pageview.go:45-120`), never persisted to disk.

## Constraints

- **Implementation Conventions (`AGENTS.md`):**
  - `agents-plugin-tool/`: follow `ai-docs/manuals/shipped-surface-boundary.md` (zero devenv-only ticket stems, hashes, or repo paths in shipped text/i18n/UI strings) and `ai-docs/manuals/performance.md` (compaction milestone latches and `UsageCompaction` derivation stay strictly in memory on `conv`; zero extra disk reads/writes on SSE, page load, or interactive send).
  - `agents-plugin-tool/internal/`: follow `ai-docs/manuals/release-compatibility.md` (T0 transparent additive fields; preserve unknown fields and existing HTTP route split).
- **CSS / Test Contracts:** Update existing assertions in `terminal_page_test.go` (`.cfoot:has(> .r-sline:empty)`) and `pageview_band_test.go` (`sb-flag`, `.t80`/`.t95`, `r-sline` limits/cost) where replaced by the new `.cfoot` strip and floating boundary icon meter, while keeping `.shp .sb-ctx` rail-hover compact compatibility (`dashboard-rail.css`).
- **Icons & Localization:** Vendor missing Lucide SVGs (`cpu`, `gauge`, `shield`, `shield-off`, `info`, `bell`, `bell-ring`, `zap`, `lock`) into `agents-plugin-tool/internal/wsdashboard/assets/lucide/icons/` so `lucideSprite()` (`lucide.go`) embeds them cleanly; update all 11 locale JSON tables under `agents-plugin-tool/internal/wsi18n/tables/` and keep `keys_test.go` green.

## Phases

### Phase 1: Backend & Runtime Contract

Implement the hosted compaction milestones, snapshot metadata, Codex 5h probe promotion, and child read-only settings contract:

1. **Hosted 1/3 & 2/3 compaction milestones (`wsagent/compaction.go`, `wsagent/runner_conv.go`).**
   - Compute `m1 = advisory + (hard - advisory)/3` and `m2 = advisory + 2*(hard - advisory)/3` when `compactAdvisoryEnabled && !math.IsInf(advisory, 0) && !math.IsInf(hard, 0) && hard > advisory`.
   - Latch `compactM1Issued` and `compactM2Issued` on `conv` in memory; emit concise milestone reminders in `compactIdle`, re-arm on drop below threshold in `observeCompaction` / `refreshCompaction`, and reset on `CompactStart`/`CompactEnd` (`compactEvent`).
2. **`wslive.UsageContext` compaction metadata (`wslive/snapshot.go`, `wslive/state.go`, `wsagent/runner_conv.go`).**
   - Add additive `Compaction *UsageCompaction` (`Soft`, `Hard`, `Milestones []int64`, `AdvisoryEnabled bool`) to `wslive.UsageContext`, carry through `normalizeUsage` and `cloneUsageContext`, and populate from `conv`'s resolved lead/worker compaction thresholds on status emission.
3. **Codex 5h window promotion (`wsdashboard/plan_usage_probe.go`, `plan_usage_history.go`).**
   - Map `Seconds == 18000` (`Minutes == 300`) to `LimitFiveHour` (`"five_hour"`) in `queryCodex` and `codexUsageEntry` (preserving `five_hour` before `seven_day` or deterministic ordering), and allow `v.Harness == "codex" && v.Kind == "five_hour"` in `validPlanUsageSample`.
4. **Child read-only settings metadata & guard (`wsagent/http.go`, `wsdashboard/pageview.go`).**
   - Gate non-lead `POST /agents/<id>/settings` (and verify `settings/permission`) on `local && own` and refuse settings mutations for child/subordinate conversations when immutable at spawn.
   - Populate `SessionView` with child/read-only composer control metadata (read-only flag + resolved model/effort/permission display values) for `/s/<child-id>` and shared composer consumers.
5. **Verification:** Add/update Go unit tests in `wsagent/compaction_test.go`, `wsagent/lead_http_test.go`, `wslive/state_test.go`, `wsdashboard/plan_usage_codex_test.go`, and `wsdashboard/plan_usage_history_test.go`; run `go test ./internal/wsagent/... ./internal/wslive/... ./internal/wsdashboard/...`.

### Phase 2: Dashboard UI & Visual Verification

*(Depends on Phase 1 landing `UsageContext.Compaction`, `SessionView` child settings metadata, `wsagent/http.go` guard, and Codex 5h `/rail/usage` window.)*

Implement the context meter, slim `.cfoot` control strip, 11-locale strings, and visual verification against the v2 mockup:

1. **Context meter server & template (`wsdashboard/pageview_band.go`, `assets/dashboard.tmpl`, `assets/dashboard-conversation.css`, `assets/dashboard-rail.css`).**
   - Scale `ContextBar` percent/fill to `Compaction.Hard` (fallback to native `Window`); format `<current> / <hard>` with optional `<native>` parenthetical (omitted when `hard == native` or `<= 360px` / `.is-compact` rail-hover).
   - Replace `.t80`/`.t95` and `.sb-flag` with dynamic percentage ticks (`m-soft`, `m-tick`), single `12px` floating icon (`info` / `bell` / `bell-ring` / `zap`) above the highest crossed boundary, overflow `crit` badge, and `.ctx-tip` breakdown tooltip (Current / Soft / 1/3 / 2/3 / Hard / Native).
2. **22px slim tonal `.cfoot` strip (`assets/dashboard.tmpl`, `assets/dashboard-leads.js`, `assets/dashboard-conversation.css`, `assets/lucide/icons/`).**
   - Remove folder, limit chips, and cost from `r-sline` / `.cfoot` across all 4 harnesses (`claude`, `codex`, `agy`, `pi`).
   - Mount the 3-trigger slim strip (`Model`, `Effort`, `Permission`) as a sibling outside `{{region . "sline"}}` inside `.cfoot`, wired to `/support/choices` and `/agents/<id>/settings` / `/agents/<id>/settings/permission` with portal/popover menu surviving SSE updates.
   - Render `pi` Permission slot disabled (`shield-off` + `—` + localized tooltip), and render all 3 slots locked (`lock` icon, `disabled`, no chevron) for child/read-only sessions (`SessionView` metadata / `can_control == false`).
3. **Localization & tests (`wsi18n`, `wsdashboard` Go + JS + Playwright tests).**
   - Add all new UI/tooltip/status keys across all 11 locale tables in `internal/wsi18n/tables/`.
   - Update `pageview_band_test.go` and `terminal_page_test.go`; add Playwright browser tests covering dark/light themes, 560px/320px/rail-hover widths, 7 meter states, all 4 harnesses (`pi` disabled perm, child read-only locks), and SSE survival while a popover is open.
   - **Side-by-side visual verification:** Capture screenshots from the Playwright suite and visually compare side-by-side against [composer-meter-v2.html](ai-docs/assets/mockups/261120/composer-footer-context-meter/composer-meter-v2.html) across theme, width, harness, role, and meter states.
