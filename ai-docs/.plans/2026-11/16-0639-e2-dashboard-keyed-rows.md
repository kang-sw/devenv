# E2 dashboard keyed-row consolidation and first-full correctness

## Authority and scope

MAIN assigned this one task at 2026-11-15T21:39:07Z under the owner's (1170)=a grant: "가급적 알아서 고칠 수 있는 것들은 알아서 고쳐가면서, 자잘한 정책까지도 알아서 결정하면서 끝까지 밀어줬으면 합니다." Read section E2 first in /Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/c639c0e9-01-range-e314-findings.md. Supporting reports: /tmp/range-review-e314/{fit,correctness,test}.md.

Exact base: 619237f2331c3dcd95a76d34e5da102dc8612ba5. Fresh branch fix/e2-dashboard-keyed-rows, pooled full worktree /Users/kang-sw/.ws-worktrees/devenv/cosigner-endowment-gown, acquire key cruelty-postnasal-panama. Acquire reported a newly created target branch and zero ahead/behind. Implementation belongs only in agents-plugin-tool/internal/wsdashboard and this immutable plan. No archived-ticket edits. Do not touch whiteboard.md. Read applicable AGENTS.md instructions, shipped-surface-boundary and release-compatibility manuals before editing. Tier: xlarge for compounded correctness, fit, test and wire-contract risk. Compatibility: directional T0; wire bytes and existing opt-in/legacy/first-full/failed-write semantics remain unchanged except fixing stale first-full display.

## Settled changes

1. Consolidate rail/tree/entry/panel row transport representations into one internal generic keyed-rows type. Replace the sendTreeRegions -> sendEntryRegions -> sendPanelRegions wrapper chain and its 13-position argument surface with one send loop over a slice of row states. Remove test-only wrappers and move tests onto the real send path. Preserve every existing assertion and exact bytes/order/JSON shapes on the wire. Share the JS same-check and input patch/HTML logic rather than entrySame/treeSame and entry/panel copies. Keep region-specific parsing, DOM ownership and existing behavior explicit; do not introduce exported APIs or a new external contract.
2. Fix first-full stale same-key entries: when entryRendered is null (initial load or after unparseable full), a full input with a differing region hash must treat all existing keys as changed. Add a substantive regression that fails on untouched base and passes after the fix, including the missing-baseline condition and real production merge/swap behavior. Preserve equal-hash/no-remount behavior and reader lifecycle ownership.
3. Add duplicate-attribute malformed panel input `<details class="panel" id="p-a" id="p-b">`. Replace 700 ms silence-based collection in the new entry/panel stream helpers with first-event/deadline-based deterministic waiting. Replace the single setTimeout(0) after shown(false) in panel-rows.browser.cjs and entry-rows.browser.cjs with deterministic acknowledgement/state observation. Older tree/board helper changes are not assigned.

## Before-edit evidence gate

Before production edits, capture BEFORE from exact assigned base using the SAME entries and panels fixtures/methods as /tmp/entries-phase4-evidence and /tmp/panels-phase5-evidence (read-only references, do not overwrite). Allocate fresh evidence under /tmp/e2-dashboard-keyed-rows-evidence. For BOTH entries and panels, save fixed-coordinate meaningful crops from page.screenshot at a 1440 viewport in dark and light, and exact ordered page.evaluate structures containing key/kind/outerHTML/open state. No element screenshots, no scrolling or capture interactions, no masking/normalization. A fixed image-only crop of a viewport screenshot is allowed. Preserve existing reader/disclosure fixture states. Save capture scripts and coordinates.

Save unchanged-base entries and panels SSE measurement outputs using identical fixture source/workload/isolated daemon configuration for AFTER. Show exact initial and per-change event-byte equality across the refactor, not just rounded percentages. Use disposable homes/config/cache/session directories, explicit daemon Home and ephemeral loopback only. No shared daemon/board/runtime mutation. Preserve scripts/logs/raw data.

Before applying the first-full fix, run the new regression against old production code and record its expected substantive failure. Test-only baseline probes are allowed after uncontaminated BEFORE capture; record provenance precisely. Do not alter production to make a red test.

## Acceptance and review

- AFTER dark/light fixed viewport crops and exact structures must be byte-identical to BEFORE on both unchanged fixtures. Inspect and compare original bytes; report paths. UI creation/design/templates/CSS/visible-markup source changes are out of scope; stop and report if genuinely required.
- Entries and panels fixture SSE bytes remain exactly unchanged from assigned develop base. Keep old/new clients, mandatory first full, hidden accumulation, failed sends, malformed rejection, unchanged node/frame identity, focus/scroll/history/disclosure lifecycle and region fallback assertions intact.
- Record the new first-full regression RED before and GREEN after. Explain the root cause rather than weakening an assertion.
- With cached WS_PLAYWRIGHT_MODULE set, run scoped `go test ./internal/wsdashboard -run 'Tree|Entry|Panel|Rail|Board' -count=1` with offline Go modules/local toolchain and complete logs. Never run the full package/module/MCP suites. Run scoped package go vet and gofmt checks. No installs/downloads/external network, real providers/harnesses, SSH, process signals or unrelated package changes.
- Commit logical units with AI Context, including this plan. After a committed implementation, allocate at most one independent lite reviewer of the full E2 diff (correctness, fit and assertion integrity); it may read complete logs but must distinguish inspection from rerun. No repeated review unless a concrete blocker requires it. A bounded test leaf is allowed if useful, but avoid overlapping writes and unneeded delegation.
- Return HEAD READY only after all gates: full SHA/base, exact scope, T0 rationale, before/after crop/structure paths, unchanged wire evidence, RED/GREEN commands/logs, test/vet/gofmt outputs, independent review artifact/limits and clean status. Include any unresolved issue or omitted verification. MAIN owns the product merge-agent integration, daemon, pushes and CI. No merge, rebase, push, runtime swap or autonomous develop refresh. Retain the lane until MAIN reports integration. No next task.

## Reporting

Use complete English sentences, normal spacing, short bullets, VERIFIED/INFERRED and file:line citations. Send a baseline-gate checkpoint and revised ETA before editing production; surface blockers immediately. Small in-scope dogfood surprises may be fixed; others go to the lead, not idea tickets. The lead must inspect evidence before READY.
