---
title: 대화창 상단 에이전트 팝업 가로 아이콘 조직도 HUD 개편
---

# 대화창 상단 에이전트 팝업 가로 아이콘 조직도 HUD 개편

## Background

The session conversation view currently renders `#agcompact` inside `#agisland` (`wsdashboard/assets/dashboard.tmpl:25`, `wsdashboard/assets/dashboard-composer.js:1849-1890`) as a fixed `320px` vertical list of text rows (harness mark, agent name, phase pill, elapsed time) projected from `.ag-source .compact-agent`. When a lead spawns multi-level subagent trees (`MaxSpawnDepth = 4` in `wsagent/children.go:67`), that vertical list hides parent-child hierarchy, obscures waiting/holding parents (`c.holding`), cannot distinguish alive-idle workers (`Process = true`) from closed ones, and steals vertical conversation viewport space with fixed `320px` width and multi-line rows.

This ticket replaces `#agcompact` with a compact horizontal icon-only org-chart HUD matching the approved v2 mockups at [roster-hud-v2.html](file:///Users/kang-sw/.ws-worktrees/devenv/bountiful-dress-energy/ai-docs/assets/mockups/261120/chat-agent-roster-hud/roster-hud-v2.html), [roster-hud-v2.css](file:///Users/kang-sw/.ws-worktrees/devenv/bountiful-dress-energy/ai-docs/assets/mockups/261120/chat-agent-roster-hud/roster-hud-v2.css), and [roster-hud-v2.js](file:///Users/kang-sw/.ws-worktrees/devenv/bountiful-dress-energy/ai-docs/assets/mockups/261120/chat-agent-roster-hud/roster-hud-v2.js). Each subagent appears as a `24×24px` node with an `18×18px` harness emblem (`Cl`/`Co`/`Ag`/`Pi`), `1.5px` animated/colored status ring, `1px` orthogonal SVG parent-child connectors, clamped hover/focus tooltip, and a slim summary footer (`완료 N · 피어 M` + expand trigger) opening the full `#agpop` drawer.

## Decisions

All owner decisions are settled; there is no `## Open Decision Queue`.

### Settled Product & Visual Decisions

- **(41)=a — `#agcompact` replacement scope & node click navigation:** Replace **only** `#agcompact` (the always-visible top-right floating card inside `#agisland`) with the compact horizontal icon org-chart HUD. Keep `#agpop` (the `500px` full tree/batches/peers popover) intact and reachable from the HUD summary footer (`완료 N · 피어 M` + expand button), `#sbadge`, and `#agidle`. Clicking a HUD node whose `replyId` is linked (`AgentNode.Linked == true`) navigates to `/s/<ReplyID>`; clicking an unlinked node opens `#agpop` and focuses/scrolls to its `<details data-key="agent-<ID>">` row (instead of triggering `#agcompact`'s legacy single-target expand click).
- **(42)=c — Subtree visibility & folding:** Show active subtrees (every node in `permission`, `failed`, `tool`, `thinking`, or `holding` state, plus all ancestor paths needed to connect them to roots) **and** alive-idle nodes (`Process == true`, waiting for mail) rendered dimmed (`opacity: .42`, `.8` on hover/focus). Fold asleep/closed nodes (`Process == false` and not holding) and finished native subagents (`Status == "done"` without active descendants) into the `완료 N` count in the HUD footer (`#agpop` still lists all retained ended rows).
- **(44)=ok — Approved v2 compact visual spec:** Follow `ai-docs/assets/mockups/261120/chat-agent-roster-hud/roster-hud-v2.{html,css,js}`:
  - Node dimensions: `24×24px` button (`border-radius: 7px`), `18×18px` inner harness emblem (`border-radius: 4.5px`, `Cl`/`Co`/`Ag`/`Pi` using existing `.hb` palette; inherit parent/lead harness when empty instead of showing `pi` or `?`).
  - Ring & badges: `1.5px` outer status ring (`1.5px` gap around the `18×18px` emblem); `11×11px` top-right micro-badges (`!` for `permission`, `×` for `failed`); `12px`-high bottom-right `+N` overflow chip on Row 4 nodes.
  - Layout & spacing: `10px` vertical connector gap between parent and `.tree-kids` (`9px` on narrow `<= 480px` / `420px` conversation width), `6px` sibling gap (`5px` narrow), `10px` root gap (`7px` narrow), `6px 8px 4px` scroll padding, `10.5px` slim footer, crisp `1px` (`stroke-width: 1.15`, `0.5px` pixel-aligned) orthogonal SVG connectors (`M...V...H...V...`).
  - Depth cap & horizontal overflow: Maximum `4` rows deep in the org chart (matching `MaxSpawnDepth = 4` in `wsagent/children.go:67`). Any visible/active descendants at depth `> 4` are folded into a `+N` overflow chip on their Row 4 ancestor. Wide fan-out scrolls horizontally inside `#agisland`'s `max-width: calc(100% - 16px)` without forcing a fixed `320px` width.

### Compatibility Tier: T0 transparent

- **Affected cross-version surface:** `wsagent.State` (`wsagent/runner.go:1512-1611`), `wslive.Event` (`wslive/state.go:78-152`), and `wslive.Agent` (`wslive/snapshot.go:309-342`), serialized into `snapshot.json` (`wsagent/record.go:604-659`, `wslive/snapshot_disk.go:6-9`).
- **Additive fields only:**
  - `wsagent.State` adds `Holding bool` (`json:"holding,omitempty"`) populated from `c.holding` (`wsagent/runner_conv.go:272,711-776`; `wsagent/spawn.go:301`; `wsagent/runner.go:702`). `wsagent.State.Process` (`wsagent/runner.go:1524`, `wsagent/runner_conv.go:729`) already exists in-memory/JSON.
  - `wslive.Event` adds `Holding bool` (`json:"holding,omitempty"`) and `Alive bool` (`json:"alive,omitempty"`), populated by `SpawnAgentEvent` (`wsagent/spawn_live.go:14-51`) while keeping legacy `Status` (`StatusRunning` / `StatusDone` / `StatusFailed`) unchanged (`wsagent/spawn_live_test.go:319-348`).
  - `wslive.Agent` adds `Holding bool` (`json:"holding,omitempty"`) and `Alive bool` (`json:"alive,omitempty"`), preserved in `wslive.State.Snapshot` (`wslive/state.go:1064-1086`) and `mergeSpawnAgents` (`wslive/spawn_agents.go:44-82`).
- **Directional old/new evidence (T0):**
  1. *New writer -> old reader:* `wslive.ReadSnapshot` (`wslive/snapshot.go:429-441`) decodes `snapshot.json` with Go `encoding/json.Unmarshal`, which ignores unknown fields (`holding`, `alive`) and leaves legacy `status`, `phase`, `harness`, `batches` intact (`wslive/snapshot.go:19-20`).
  2. *Old writer -> new reader:* Older `ws-mcp` or older daemon writes omit `holding` and `alive`, which decode as `false`; `wsdashboard` falls back cleanly (`Holding=false`, `Alive=false` -> `StatusRunning` stays visible, ended rows fold as `완료`).
  3. *Old writer rewrite + reopen by new binary:* `wslive.State.SeedFinished` (`wslive/state.go:991-1035`) only seeds `StatusDone`/`StatusFailed` rows from `prev.Agents` (`state.go:996-998`), which are neither holding nor alive (`Holding=false, Alive=false`). Live/holding/alive-idle Pi workers are always republished from `wsagent` RAM on daemon startup/rejoin via `seedSpawnRows` -> `SpawnAgentEvent` (`wsagent/spawn_live.go:133-176`), so an intervening old-daemon snapshot write loses zero state upon reopen with the new binary.

## Constraints

- **Implementation Conventions manuals:**
  - `ai-docs/manuals/shipped-surface-boundary.md` (applies to `agents-plugin-tool/`): keep any user-facing strings, i18n keys, and tests free of repository-specific names/stems/paths.
  - `ai-docs/manuals/release-compatibility.md` (applies to `agents-plugin-tool/internal/`): maintain T0 transparency above; never break legacy `Status` or `SubagentCount` contracts.
  - `ai-docs/manuals/performance.md` (applies to `agents-plugin-tool/`): **zero additional file I/O** on page render, SSE swap, or snapshot hot paths. Telemetry (`Holding`, `Alive`) flows strictly through in-memory `conv` -> `SpawnAgentEvent` -> `wslive.State` -> existing `snapshot.json` flush path (`wsagent/record.go:604-659`), and HUD projection is built in-memory during `buildSessionView`.
- **Size guard (`git_merge_size_soft_lines = 1000` in `mcp/git_merge_size.go:25-26`):** `wsdashboard/assets/dashboard-composer.js` is already `2054` lines. Extract new HUD DOM rendering, SVG orthogonal connector geometry, `ResizeObserver`, tooltip clamping, and click handling into a dedicated asset fragment (e.g., `wsdashboard/assets/dashboard-hud.js`, registered in `wsdashboard/pages.go:49` `readDashboardSource` and `wsdashboard/asset_source_test.go:20`), leaving only minimal hook integration in `dashboard-composer.js`.
- **Region & permission invariants:**
  - Preserve `splitTreeRows` (`wsdashboard/tree_rows.go:24-121`): any HUD data carrier inside `#r-tree` (`dashboard.tmpl:227`) MUST live inside `.ag-source` without `.agent` or `.peer` class names or `<!--ws-tree-row-` markers so `splitTreeRows` continues isolating per-row `#agpop` patches cleanly.
  - Preserve zero double-counting of approvals in `tlPerms()` (`wsdashboard/assets/dashboard-stream.js:67-73`) and `drawerWait()` (`wsdashboard/assets/dashboard-drawer.js:346-352`), both of which exclude descendants of `.ag-source`.
  - Preserve `agent-model-hover` (`template.shp-source` inside `.agent` in `dashboard.tmpl:233,486`), `data-sub` / `data-subdeep` on `.ag-source` and `.clive`, and `CompactAgents` / `Subagents` behavior for existing consumers (`subBadge`, `toast_test.go`, `frontend-drawer.test.js`).
- **Animation pause & reduced motion:** Every new `@keyframes` animation on `.hud-node` (`thinking`, `tool`, `holding`, `permission`) MUST pause under `:root[data-animation-paused]` (`dashboard.css:263`, verified by `frontend-idle.test.js:80`) and disable under `@media (prefers-reduced-motion: reduce)`.

## Phases

### Phase 1: Backend telemetry (`wsagent` / `wslive`) & hierarchical HUD tree projection (`wsdashboard`)

**Goal:** Expose live `Holding` and `Alive` state from `wsagent` into `wslive.Agent`, protect parents with active descendants or `Holding == true` from premature `FinishedAgentCap` eviction, and build the structured hierarchical HUD projection inside `wsdashboard/pageview.go` and `#r-tree` without breaking `splitTreeRows`, `tlPerms()`, `drawerWait()`, or `agent-model-hover`.

**Changes:**

1. **`wsagent` telemetry exposure (`wsagent/runner.go`, `wsagent/runner_conv.go`, `wsagent/spawn.go`, `wsagent/spawn_live.go`):**
   - Add `Holding bool \`json:"holding,omitempty"\`` to `wsagent.State` (`wsagent/runner.go:1512-1611`).
   - Populate `st.Holding = c.holding && !c.closed` in `conv.stateWithKeyCheck` (`wsagent/runner_conv.go:711-776`, alongside `st.Process = c.proc != nil` at line 729). Notice `c.holding` already tracks `c.heldRes != nil` (`wsagent/spawn.go:301`, `wsagent/runner.go:702`). When `conv.hold` (`wsagent/spawn.go:291-303`) or `publish` / `releaseHeld` toggles `c.holding`, trigger parent spawn-row refresh (`c.r.wakeSpawnParents(c.memoryState())`) if not already emitted.
   - Update `SpawnAgentEvent` (`wsagent/spawn_live.go:14-51`): keep `Status` mapped identically for legacy consumers (`StatusDone` / `StatusRunning` / `StatusFailed`), and set `ev.Holding = st.Status != StatusClosed && st.Holding` and `ev.Alive = st.Status != StatusClosed && st.Process`.

2. **`wslive` schema, reducer, merge, & eviction protection (`wslive/snapshot.go`, `wslive/state.go`, `wslive/spawn_agents.go`):**
   - Add `Holding bool \`json:"holding,omitempty"\`` and `Alive bool \`json:"alive,omitempty"\`` to `wslive.Event` (`wslive/state.go:78-152`) and `wslive.Agent` (`wslive/snapshot.go:309-342`), plus `holding, alive bool` on internal `agentState` (`wslive/state.go:188-198`).
   - In `State.applyAgent` (`wslive/state.go:619-694`): when `ev.AgentKind == AgentKindPi`, assign `a.holding = ev.Holding` and `a.alive = ev.Alive` (and clear both when `ev.Status == StatusClosed` or on non-Pi `StatusDone`/`StatusFailed`); in `State.Snapshot` (`wslive/state.go:1064-1086`), copy `Holding` and `Alive` onto `wslive.Agent` (deep copy is automatic for `bool`; ensure any snapshot/state helpers preserve both).
   - In `mergeSpawnAgents` (`wslive/spawn_agents.go:44-82`), preserve the canonical daemon row's `d.Holding` and `d.Alive` when merging native activity (`d.Activity = a.Activity` at lines 50-52).
   - **Eviction protection in `State.retain` (`wslive/state.go:913-984`):** Currently `retain()` classifies `a.status == StatusRunning || a.bgTasks > 0` as `running` (line 941) and evicts everything else past `FinishedAgentCap = 5` (lines 967-972). Protect any agent group where `a.holding` is true OR where the agent has any retained `running`/protected descendant (walking `ParentID` edges among `st.agents`, cycle-guarded via a `visited` set), keeping them out of the `FinishedAgentCap` drop slice so the org chart never orphans running grandchildren or hides a holding parent. Preserve `FinishedAgentCap` enforcement for leaf finished/closed agents.

3. **`wsdashboard` hierarchical HUD tree projection (`wsdashboard/pageview.go`, `wsdashboard/assets/dashboard.tmpl`):**
   - Define `HUDNode` (or `TreeView.HUD` struct) in `wsdashboard/pageview.go` alongside `TreeView` (`pageview.go:204-211`), preserving existing `TreeView.CompactAgents` and `TreeView.Subagents` (`subagentProjection` at `pageview.go:456-498`) untouched.
   - **HUD state precedence & visibility (`(42)=c`):**
     - Derive each node's HUD state with strict precedence: `permission` (`Permission != nil`) > `failed` (`Status == StatusFailed`) > `tool` (`Status == StatusRunning` and `Phase == PhaseTool` or `Phase == PhaseReplying`) > `thinking` (`Status == StatusRunning` and `Phase == PhaseThinking` / `PhaseIdle` / other active phase) > `holding` (`Holding == true`) > `idle` (`Alive == true`, i.e., `Process == true` waiting on mail; or asleep/closed when `Alive == false`).
     - A node is visible in the HUD if its state is active (`permission`, `failed`, `tool`, `thinking`, `holding`), OR `Alive == true` (`idle`, dimmed), OR any descendant is visible (making the ancestor path visible to keep the tree connected).
     - Asleep/closed nodes (`Alive == false`, not `holding`, and no visible descendants) and finished native subagents (`Status == StatusDone` without active descendants) are excluded from the visible HUD forest and counted into `CompletedFolded` (`완료 N` = `Tree.Finished + folded` or exact count matching `Tree.Finished` + closed/idle-folded count — define `HUDProjection.CompletedCount` explicitly from `Tree.Finished` minus any visible `Alive`/`Holding` `StatusDone` nodes, or count all non-visible `StatusDone` across root+linked snapshots so `완료 N` never double-counts visible `holding`/`alive-idle` nodes).
   - **Harness fallback inheritance & metadata:**
     - Normalize harness (`claude`, `codex`, `agy`, `pi`). When an agent's `Harness` is empty, inherit its parent's resolved harness, falling back to the viewed session's `SessionView.Harness` (lead harness), and `"pi"` only when `AgentKind == wslive.AgentKindPi` and neither parent nor lead specifies one.
     - Include per-node fields needed by the HUD & tooltip: `ID`, `ReplyID`, `Linked` (`AgentNode.Linked`), `Label` (fallback `shortID(ID)`), `Harness`, `State` (`thinking`|`tool`|`holding`|`idle`|`permission`|`failed`), `Model` (`DisplayModel()`), `Effort`, `ElapsedMs` (`ActiveMs` + current turn delta / `ActiveSince`), `Chain` (ancestor labels for breadcrumb), `Task` (`Permission.ToolName + ArgsPreview`, `Running.ToolName + ArgsPreview`, `Caption`, or `Error`), `Overflow` (count of visible/active descendants below depth 4), and `Children []HUDNode`.
     - **Linked delegate expansion & 4-row depth cap (`(44)=ok`):** Include live linked delegate subtrees (cycle-guarded by `visited[ReplyID]`, matching `subagentProjection` at `pageview.go:459-480`) under the linked parent node. Cap rendered tree depth at `4` (`MaxSpawnDepth = 4`), folding any visible/active descendants at depth `> 4` into `Overflow` (`+N`) on their Row 4 ancestor.
   - **Zero-double-count `#r-tree` data carrier:**
     - Inside `#r-tree`'s `<div class="ag-source" hidden data-sub="..." data-subdeep="...">` (`dashboard.tmpl:227`), embed the serialized `Tree.HUD` JSON payload in `<script type="application/json" class="ag-hud-data">` (using Go `json.Marshal` via template helper, HTML-safe inside `<script type="application/json">` or escaped `html/template`). Do **not** add `.agent`, `.peer`, `.phase.permission`, or `.brow.perm` elements inside `.ag-source`, guaranteeing `splitTreeRows` (`tree_rows.go:24-121`), `tlPerms()` (`dashboard-stream.js:70`), `drawerWait()` (`dashboard-drawer.js:348`), and `agent-model-hover` (`.shp-source`) remain unaffected.

**Deferred scope:** Visual DOM/SVG/CSS/tooltip rendering in `#agcompact` (`dashboard.tmpl:25`, `dashboard-hud.js`, `dashboard-conversation.css`, i18n, and Playwright visual verification) is deferred to Phase 2.

**Verification boundary (Phase 1):**
- Unit tests in `wsagent/spawn_live_test.go` covering `State.Holding`, `SpawnAgentEvent` `Holding`/`Alive` projection, and legacy `Status` contract preservation.
- Unit tests in `wslive/state_test.go` and `wslive/spawn_agents_test.go` covering `Holding`/`Alive` JSON T0 roundtrip, `mergeSpawnAgents` preservation, `SeedFinished` behavior, and `retain()` protection of holding/ancestor-of-running agents under `FinishedAgentCap` pressure (including cycle safety).
- Unit tests in `wsdashboard/pageview_test.go` and `wsdashboard/tree_rows_test.go` covering HUD visibility (`(42)=c`), state precedence, harness fallback inheritance, 4-row depth cap + `+N` overflow aggregation, linked delegate subtree attachment, `완료` count, `splitTreeRows` compatibility, and zero regression on `SubagentCount` / `CompactAgents`.

### Phase 2: Compact Horizontal Icon Org-Chart HUD (`#agcompact`) UI, SVG Connectors, Tooltip, i18n, & Playwright Visual Verification

**Goal:** Replace `#agcompact`'s legacy vertical list UI with the approved v2 horizontal icon org-chart HUD (`roster-hud-v2.{html,css,js}`), preserving `#agpop`, `#sbadge`, `#agidle`, dark/light theming, animation pause guards, i18n across all 11 locales, and passing Playwright visual/functional verification. **Depends on Phase 1** (`Tree.HUD` JSON payload in `#r-tree .ag-source`).

**Changes:**

1. **Template markup (`wsdashboard/assets/dashboard.tmpl`):**
   - Replace `#agcompact`'s empty `<div id="agcompact" ... role="button">` wrapper (`dashboard.tmpl:25`) with a non-button container (`<div id="agcompact" class="agview agcompact" aria-label="{{t "tree.agents"}}">`) housing:
     - `.hud-tree-scroll` -> `.hud-canvas` -> `<svg class="hud-lines" aria-hidden="true"></svg>` + `<div class="hud-roots"></div>`.
     - `.hud-footer` containing a real `<button type="button" class="hud-expand-btn" aria-controls="agpop" aria-expanded="false" title="{{t "hud.expand"}}">` with summary text (`완료 N · 피어 M` / localized equivalent) and expand SVG icon. (Keeping interactive nodes and the expand trigger as proper `<button>` elements avoids nested interactive controls.)
   - Mount `<div id="hud-tooltip" class="hud-tooltip" role="tooltip" hidden></div>` inside the conversation grid layer (`#grid`, positioned relative to `#agisland` / conversation cell so `overflow: hidden` on `#agisland` never clips tooltips).

2. **Stylesheets (`wsdashboard/assets/dashboard-conversation.css`, `wsdashboard/assets/dashboard.css`):**
   - Port the v2 tokens (`--ring-think`, `--ring-tool`, `--ring-hold`, `--ring-perm`, `--ring-fail`, `--hud-glass`, `--hud-line`, `--hud-conn`) for both light `:root` and dark (`@media (prefers-color-scheme: dark)` + `:root[data-theme="dark"]`) in `dashboard.css`, matching existing theme rules.
   - Replace `.agcompact` / `.agrow` / `.agsummary` rules (`dashboard-conversation.css:257,263-270`) with the v2 HUD rules from `roster-hud-v2.css:164-325`:
     - Remove fixed `width: 320px` on `.agisland[data-mode="compact"]` and `width: var(--ag-cw, 320px)` on `.agcompact`; allow `#agisland` to size dynamically to content up to `max-width: calc(100% - 16px)`, with `.hud-tree-scroll` scrolling horizontally on wide fan-out.
     - Implement `24×24px` `.hud-node`, `18×18px` `.hud-emblem[data-h="claude|codex|agy|pi"]`, `1.5px` `.hud-ring` (`thinking` / `tool` `@property` conic spin, `holding` breathing, `idle` `opacity: .42`, `permission` pulsing, `failed` solid), `11×11px` `.hud-badge.{perm,fail}` (`!`/`×`), `12px` `.hud-overflow` (`+N`), `.hud-footer`, `.hud-tooltip`, and narrow spacing (`@media (max-width: 480px)` / narrow conversation width: `gap: 5px`, `padding-top: 9px`, `7px` root gap).
     - Add all new `@keyframes` (`hud-spin-think`, `hud-spin-tool`, `hud-hold-breathe`, `hud-perm-pulse`) and `.hud-node[data-state] .hud-ring` to `:root[data-animation-paused]` (`dashboard.css:263`) and `@media (prefers-reduced-motion: reduce)` (`dashboard-conversation.css:273,310`, `dashboard.css:264`).

3. **Client JS module & size-guard compliance (`wsdashboard/assets/dashboard-hud.js`, `wsdashboard/assets/dashboard-composer.js`, `wsdashboard/pages.go`, `wsdashboard/asset_source_test.go`):**
   - Create `wsdashboard/assets/dashboard-hud.js` (keeping `dashboard-composer.js` under the `1000`-line soft delta guard) and register it in `dashboard.js` assembly in `wsdashboard/pages.go:49` and `wsdashboard/asset_source_test.go:20`.
   - Implement `window.wsHud` (or shared helper called by `agProject()` / `agSwapped()` in `dashboard-composer.js:1849-1896`):
     - Parse `#r-tree .ag-source .ag-hud-data` JSON; compute `agSeen` (when HUD roots or `.tgroup .agent` exist) and active count for mode switching (`idle` vs `compact` vs `full`).
     - Render `.tree-node` / `.node-slot` / `button.hud-node` / `.hud-emblem` / `.hud-badge` / `.hud-overflow` / `.tree-kids`, plus localized summary footer (`완료 N · 피어 M`).
     - Update `agSize()` (`dashboard-composer.js:1813-1833`) so `mode === "compact"` measures `agcompact` at natural width (`width: max-content; max-width: calc(100% - 16px)` instead of hardcoded `"320px"`) and transitions pixel `width`/`height` cleanly.
     - Draw `1px` orthogonal SVG connectors (`M...V...H...V...` with `0.5px` pixel snapping) on `agProject()`, `agSwapped()`, and `ResizeObserver` on `.hud-canvas` / `.hud-tree-scroll` scroll/resize.
     - Hover & keyboard `focusin`/`focusout` tooltip (`#hud-tooltip`) clamped inside the conversation grid cell (`#convo` / `#grid` bounds): show harness badge, `label`, localized state pill, `model` + `effort`, live-ticking or formatted `elapsed` + `+N deeper`, parent breadcrumb `chain`, `task` caption/tool, and localized click hint (`navigate /s/<ReplyID>` when linked, `open tree details` when unlinked).
     - Click behavior (`(41)=a`): clicking a linked `.hud-node` navigates to `/s/<ReplyID>` (using shell/navigation integration when framed); clicking an unlinked `.hud-node` calls `agOpen(true)`, opens `<details data-key="agent-<ID>">` and all ancestor `<details>` inside `#agpop`, and scrolls/focuses its `<summary>`. Clicking `.hud-expand-btn` calls `agOpen(true)`.

4. **i18n across all 11 locales (`agents-plugin-tool/internal/wsi18n/tables/*.json`):**
   - Add HUD keys (`hud.state.thinking`, `hud.state.tool`, `hud.state.holding`, `hud.state.idle`, `hud.state.permission`, `hud.state.failed`, `hud.summary`, `hud.expand`, `hud.deeper`, `hud.hint.link`, `hud.hint.details`, `hud.lead`) to all 11 locale files (`en.json`, `ko.json`, `ja.json`, `de.json`, `es.json`, `fr.json`, `it.json`, `pt-BR.json`, `ru.json`, `zh-Hans.json`, `zh-Hant.json`) with zero duplicate JSON keys and passing `wsi18n` consistency tests.

5. **Tests & Playwright visual verification:**
   - Update `wsdashboard/pageview_test.go`, `wsdashboard/pages_test.go`, `wsdashboard/testdata/agents-island.test.cjs`, and `wsdashboard/testdata/agent-popover.browser.cjs` for the new `#agcompact` HUD markup, `agSize()` dynamic width, summary footer expand trigger, and node click behavior while keeping all `#agpop` / `tlPerms` / `drawerWait` assertions green.
   - Add Playwright browser tests (`wsdashboard/testdata/agent-popover.browser.cjs` or dedicated `hud.browser.cjs` + Go harness) verifying live dashboard renders against `roster-hud-v2.html` across: dark & light themes, desktop & narrow (`420px`) widths, 12-agent 3-level tree, max 4 rows + `+2` overflow chip, all 6 ring states + 4 harnesses, wide fan-out horizontal scroll, clamped tooltip positioning, linked `/s/<ReplyID>` vs unlinked `#agpop` focus clicks, and `#agpop` open/close/pin/Esc.

## Verification

Run after Phase 1:

```bash
cd agents-plugin-tool && go test ./internal/wsagent/... ./internal/wslive/... ./internal/wsdashboard/...
```

Run after Phase 2:

```bash
cd agents-plugin-tool && go test ./internal/wsi18n/... ./internal/wsdashboard/... ./internal/wsagent/... ./internal/wslive/...
node agents-plugin-tool/internal/wsdashboard/testdata/agents-island.test.cjs
node --test agents-plugin-tool/internal/wsdashboard/frontend-idle.test.js agents-plugin-tool/internal/wsdashboard/frontend-drawer.test.js
WS_PLAYWRIGHT_MODULE="$(node -e "console.log(require.resolve('playwright'))")" go test ./internal/wsdashboard -run TestAgentBadgePopoverBrowser -v
```
