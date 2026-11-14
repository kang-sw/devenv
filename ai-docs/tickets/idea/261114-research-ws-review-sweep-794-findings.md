---
title: Review sweep 794 findings and repair routing
---

# Review sweep 794 findings and repair routing

## Background

This is the findings/routing anchor for the static review of `ffc60b4bf78e6801093673fdf540a865b9ec4d45..459fd070b2043fa45430eed12a1379f95fc630d5`. MAIN requested this ticket as the full-range `block` reference. Nine independent package/lens partitions covered UI, Core, and Distribution/Landing under correctness, fit and test lenses. No tests, builds, browser runs or daemon operations were performed by the sweep.

The reviewed head is immutable. Later `cacd771c1` (652), subsequent develop merges and repair commits are not reviewed by this verdict. Repair acceptance requires a new review retaining the original base, or the independently cleared prefix cut after it is stamped. This research ticket is not an implementation dispatch target and selects no repair mechanism.

## Outcome Ledger

### Verified Findings

All source citations below refer to `459fd070b2043fa45430eed12a1379f95fc630d5`. Static source traces are verified; runtime occurrence, frequency and exact geometry/duration were not reproduced.

- **UI-C1 — Important: Web popover coordinates use the wrong containing block.** `agents-plugin-tool/internal/wsdashboard/assets/webcard.js:420-477` assigns button/column viewport coordinates directly to a popover appended inside the toolbar slot. `assets/dashboard.css:781-791` makes the slot relative and `.tpop` absolute. A nonzero slot origin therefore displaces the input popover and can clip it in normal split layouts. UI correctness and test reviewers independently confirmed the trace; existing Web tests do not mount/exercise this entry. Introducing landing: `7d57eb834aaf34e3aa8a5fcf7cf996e350406f21`; source `cba911ad6fd766d039e7f4de1cf522c24ec8ead0`, branch `impl/develop/chug-case-thigh`. **Owner: EXEC1.** MAIN reports proposed fix `4d302c5a5` on `fix/review794-web-popover`, explicitly untested and outside this review. Required repair evidence includes the actual popover in an offset slot at wide/narrow widths, containment, submission and focus behavior; no passing result is claimed.

- **UI-C2 — Important: republishing an expanded history boundary can stall Older.** `agents-plugin-tool/internal/wswhiteboard/assets/stream.js:33-35` excludes entries under `.panel-versions`, while `:396-403` restores saved expanded chunk IDs unchanged. After a saved boundary becomes a historical version, `:98-101` cannot find it in active rows and returns with pending state retained; the handler at `:427-441` rejects later Older clicks until reset. Renderer projection at `internal/wswhiteboard/stream_view.go:240-269` and `assets/page.tmpl:63` supplies that transition. Both UI reviewers corroborated it; same-ID height swaps and log appends do not test panel republishing. Exact defect landing: `28d8576226cc62312bf2d85d9c4cb306882f3df0`; source `8f24cec5b9ee9e09d7c2d58e5002ddab03842152`, branch `fix/history-fit-panels`. **Owner: EXEC2**, queued after 649 per MAIN. No repair commit or runtime reproduction was reported. Do not attribute the earlier fold-reopening incident to this trace.

- **Core-C1 — Important: predecessor cleanup can undo successor registration liveness.** `agents-plugin-tool/internal/wsdashboard/proxy.go:320-347` closes an idle proxy stream before old `ServeStdio` cleanup necessarily finishes. The successor restores a live row at `internal/mcp/dashboard_sessions.go:245-253`; `dashboardWriteRow` at `:357-374` fences hosted takeover but not proxy generations, allowing predecessor `dashboardEnd` at `:461-465` to write an end mark later. A cached non-idle successor registration avoids immediate keyed-call rewriting (`:315-317`). `internal/wsdashboard/registry.go:205-221` rejects any end mark immediately; heartbeat cadence is two minutes (`:42-53`). Wrong ended status can therefore survive normal interactive use until heartbeat; no guaranteed two-minute outage or permanent loss is claimed. This is explicitly documented deferred hardening in source `fb1074fb7d9ee14de36eef39d79ba7ac67736ba1`, not an undisclosed race or assertion that the primitive originated here. Changed repair landing: `5a3758f36ae4681b9dedf4cd2f7fd66734134ab5`, branch `fix/rail-reload-ended`. **Owner: EXEC3.** MAIN requested capture only: `261114-bug-ws-proxy-predecessor-end-write-after-reconnect`, commit `00774152ce76963adca472c6c738e0157a106372` on isolated `goal/selection-bundle-comment`, pending later integration. T0 in-process ownership is the target; no patch or persisted-generation design is approved.

- **D-C1 — Important: board takeover instructions copy source render bookkeeping.** `agents-plugin/rsrc/whiteboard/whiteboard.md:28-29` prescribes copying a stopped source's folder and updating to regenerate HTML; wsflow and Pi carry identical guidance. `agents-plugin-tool/internal/wswhiteboard/folders.go:359-361,557-568` clones/persists session state without clearing its `EmbedDir` and `Embeds` fields (`store.go:284-287`). A copied missing host directory refuses ordinary updates (`store.go:646-665`). The update defaults omitted commands to empty text and explicitly renders with embed output (`internal/mcp/whiteboard_tools.go:240-272`). That render selects the copied host target or appends to the copied repository retention list (`:460-481`); `:567-581` constructs eviction paths from retained names, including source-named snapshots, and publication removes those paths (`:552-553`). A destination-folder backup does not protect shared-parent snapshots. Introducing landing: `d1c3b9d6baf433c48b4ec61313dbf6c26b577cdf`; source `85d2008e4b2fa10c83b8c710bf543f77d17e3568`, branch `docs/whiteboard-handoff-line`, owner decision 637. **Owner: EXEC1** per MAIN. No takeover/filesystem reproduction, deletion or repair was performed. Repair must protect authoritative board content and source history; no state-sanitization mechanism is selected here.

### Confirmed Decisions

- MAIN assigned UI-C1 and D-C1 to EXEC1, UI-C2 to EXEC2, and Core-C1 to EXEC3. Its reports of proposed fixes are routing state, not implementation acceptance.
- MAIN authorized one idea findings/routing ticket and a develop commit carrying it with the review ledger. This ticket anchors the full-range block and authorizes no source repair or shared daemon operation.
- MAIN confirmed the immutable review head and excluded later 652/other merges. Next ordinary sweep cadence is ten first-parent merges or three hours; repairs still require the lead-review repair re-review procedure.
- The conservative first-parent path rule maps history to its earliest touched segment `4d869df399af8084296fb5554dde51b0e33b769c`, although the precise defect is introduced later. The candidate cut is its first parent, `c55f9f3f1ac9f24489077b920a8fdc44f5ed1dde`. Independent review of base-to-cut returned LGTM for exactly one previous-ledger append. Only that record is cleared; no substantive product prefix is newly cleared.
- Preserve existing shared-card visual acceptance, C670 merge-first/no-further-diagnosis authority and disclosed browser residuals. Test-validation gaps are not proof of production terminal/selection defects and do not authorize browser retries or assertion relaxation.

### Proposals

None are selected by this routing capture. Bounded repair mechanisms and verification execution remain with the owning lane and its approval gates.

### Open Questions

Repair results, test evidence, precise runtime incidence and the final repair-inclusive review remain open. D-C1's embed-generation/publication trace was independently checked against numbered Git source; root-verified coordinates above supersede stale coordinates in the original reviewer report. Update findings only from evidence, without treating later source integration as delivery or acceptance.

### Rejected Alternatives

No new product alternative was rejected. Counting duplicated regression-test obligations as extra product defects, claiming a two-minute guaranteed outage, or treating a ledger-only prefix as substantive product clearance would exceed the evidence.
