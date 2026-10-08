---
title: Restore existing display summary rows after Pi reload or resume
related:
  261007-feat-pi-display-summary: original summary feature
  261008-feat-pi-display-summary-sidecar-persistence: existing persistence contract
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 75fd86b831a8fcfa
sage-review-completeness-reviewed: 75fd86b831a8fcfa
---

# Restore existing display summary rows after Pi reload or resume

## Background

After merging sidecar persistence in 0243f8cfe, the user observed that existing
rows did not return to their display summaries after reload and suspected the
same problem on resume/continue. The user subsequently clarified that newly
created rows summarize normally. This is an existing-row restoration/rendering
repair, not a provider-generation failure.

Metadata-only inspection found a matching sidecar containing a valid summary
for the git-status call used during dogfooding. Read-only reproductions using
actual installed Pi 1.0.4 lifecycle and UI components established three defects:

- Reload and in-process session replacement rebuild chat before `session_start`
  renderer registration. Mounted tool/custom-message components keep their
  constructor-time renderer references; late registration does not replace them.
- Summary startup clears the store's invalidators after rebuilt rows have
  captured them, preventing restored values from repainting those rows.
- Sidecar replay filters saved tool IDs before late bridge tool registration
  populates the owned tool names, then marks initialization complete. Valid ws
  rows excluded at that point are not reconsidered later.

Evidence pointers: installed Pi root
`/home/linuxbrew/.linuxbrew/lib/node_modules/@earendil-works/pi-coding-agent/`
(`package.json` declares 1.0.4): `dist/core/agent-session.js#L2932-L2963`
and `dist/modes/interactive/interactive-mode.js#L5365-L5377` establish reload
rebuild-before-start; the latter's `#L363-L369` and `#L1584-L1594` establish
replacement render-before-bind. Constructor-time references are retained in
`dist/modes/interactive/components/tool-execution.js#L34-L70` and
`dist/modes/interactive/components/custom-message.js#L15-L23`.
`agents-plugin-pi/src/display-summary-session.ts#L92-L101` clears the store
and registers renderers at startup; `agents-plugin-pi/src/display-summary.ts#L65-L80`
clears invalidators too. `agents-plugin-pi/src/display-summary-sidecar.ts#L52-L65`
and `#L283-L302` filter by owned tool names and complete replay once;
`agents-plugin-pi/src/index.ts#L667-L670`, `#L787-L807`, `#L845-L852` and
`agents-plugin-pi/src/tool-result-render.ts#L669-L677` establish summary-start
before bridge/one-liner tool ownership registration.
The relevant session/sidecar fixtures prepopulate tool names
(`agents-plugin-pi/test/display-summary-session.test.ts#L39-L42`,
`agents-plugin-pi/test/display-summary-sidecar.test.ts#L9-L12`), and the native
component regression registers wrappers before constructing the component
(`agents-plugin-pi/test/display-summary-render.test.ts#L412-L425`); these
fixtures do not exercise production rebuild-before-registration ordering.

## Decisions

- Repair the confirmed old/rebuilt/restored-row defects together, as approved
  by the user after read-only diagnosis. Newly generated rows are already
  working and are not the failure being repaired.
- Ensure summary-capable renderers are available when the host constructs the
  row, retain its active repaint linkage, and restore valid saved summaries
  whose owned tools register later. Source fixes must address all three
  production boundaries rather than only the stored map.
- Preserve the distinction between collapsed summary rendering and expanded
  raw rendering. No automatic collapsing of user-expanded rows is requested.
- (1) Register a public `registerToolRenderer(resolver)` presentation resolver
  before reconstruction, delegating raw rendering through its chain without
  double-wrapping. Keep execution definitions, child permission scopes and live
  lead-only activation unchanged. This explicitly replaces the prior deliberate
  session_start-only presentation timing while preserving its protection goal.
  Rejected: late same-name registration cannot repair mounted rows; private host
  UI patches violate the public-seam boundary.
- (2) Confirm summary eligibility through actual owned-tool registration, not
  tool-name/prefix inference or blanket admission. Early presentation availability
  does not grant ownership. Rejected: broad early inference expands the existing
  summarized-tool scope to foreign MCP or unconfigured native tools.
- (3) Await host-TUI loading and register adapter custom-message presentation
  before reconstruction with dynamic lead-summary gating and unchanged raw child
  rendering. Rejected: fire-and-forget session_start registration is too late
  for mounted custom-message components; a tool resolver cannot repair them.
- (4) Separate summary-value reset from session/generation-owned repaint
  subscriptions. Retire outgoing subscriptions before reconstruction and preserve
  subscriptions already mounted for the incoming session. Rejected: retaining
  all invalidators repaints outgoing sessions, while clearing all at startup
  discards incoming repaint links.
- (5) Retain validated replay candidates per originating sidecar and reconsider
  them on confirmed ownership changes, notifying newly eligible mounted rows
  without rereading or regenerating summaries. Preserve newer-live precedence
  and origin isolation. Native-fork eligibility follows the same confirmation
  rule; retry restores only the child's own candidates, never parent hole-filling
  into authoritative empty/partial child caches. Rejected: permanent exclusion
  loses valid rows; repeated full-file replay adds unnecessary I/O.
- (6) Require a reproducible separate installed-Pi-1.0.4 lifecycle integration
  check, failing explicitly on absent/incompatible host or API without dependency
  upgrades. Keep package and host results separate; record exact host discovery
  command and version. Rejected: silent skipping or pinned-0.84.4 mocks do not
  establish the real resolver boundary.
- (7) Allow ticket closure after automated and actual-host offline verification,
  with live reload/resume acceptance explicitly pending for the user. Integration
  still uses normal merge approval, followed by the user-driven live check.
  Rejected: waiting for live UI acceptance before closure/integration prevents
  the worker from delivering a verified fix for the user to load. Never claim
  live acceptance passed without the user's confirmation.
- (8) Combine the lifecycle repair with fail-soft raw fallback and
  baseline-comparative failure tests for both new resolver logic and existing
  summary rendering. The user confirmed this safety direction after comparing
  current and proposed risks. Rejected: merely advancing registration and
  treating a thin UI wrapper as inherently crash-contained. Demonstrate that
  the repair does not worsen the tested failure behavior; do not claim universal
  crash containment or safety before implementation and verification.

## Constraints

- Target installed Pi 1.0.4, preserving the prior current-host-only support
  scope. Do not change dependencies, provider calls, output schema, summary
  configuration, or the existing delayed-flush behavior.
- Preserve the existing summary layout/status background, native/raw fallback,
  and the exclusion of the Previous conversation display-only block.
- Preserve the completed persistence ticket's ownership/foreign-file/symlink,
  native-fork, orderly drain, and newer-live-acceptance precedence contracts.
  Replaying old values must not overwrite newer accepted summaries or repaint
  an outgoing session through a replacement session's state.
- Use supported public Pi registration/lifecycle seams; do not patch installed
  host code or depend on private InteractiveMode object access. Installed Pi
  1.0.4 exposes `registerToolRenderer` with a `(toolName, next)` resolver callback
  that can supply renderers for not-yet-registered tools, including resumed MCP
  rows (installed `docs/extensions.md`, Tool rendering;
  `dist/core/extensions/types.d.ts#L501-L506`, `#L1220-L1221`). Its public API
  is `registerToolRenderer(resolver)`, not a per-tool definition registration;
  `dist/core/extensions/runner.js#L542-L546` resolves the chain and
  `dist/modes/interactive/interactive-mode.js#L1700-L1702` supplies built-in
  fallback. This establishes a supported pre-reconstruction presentation seam
  for built-ins and late tools; decision (1) selects this integration without
  replacing execution definitions.
- Do not expose private conversation/summary contents in diagnosis logs or
  introduce temporary probes/settings changes as part of the production fix.
- Treat missing mode/store/metadata/owned-tool confirmation and unavailable TUI
  dependencies as not ready, not as fabricated valid state. Our summary path
  must degrade to valid raw presentation rather than return an invalid component
  or propagate its own resolver/construction/delayed-render errors.
- Preserve raw-renderer delegation and legitimate component reuse, frame/status
  backgrounds and expansion. Do not double-wrap existing summary renderers or
  pass summary/guard components to native renderers expecting a native component.
- The safety boundary covers adapter-owned summary behavior; it is not a promise
  to contain every downstream extension/native-renderer failure. Installed Pi
  catches tool/message construction faults but does not generally catch resolver
  or returned-component render faults (dist/core/extensions/runner.js#L542-L545,
  dist/modes/interactive/components/tool-execution.js#L213-L258,
  dist/modes/interactive/components/custom-message.js#L49-L64). Current adapter
  stack/createSummarySwitch render paths also invoke components without a catch.
  These pre-existing hazards must be distinguished from new resolver/readiness
  exposure in verification.

## Prior Decisions

- 261008-feat-pi-display-summary-sidecar-persistence (2026-10-08, commit 0243f8cf): "Live provider/TUI dogfood remains for user reload verification; no re-review was allocated. Previous conversation display-only blocks remain unsummarized." — bearing: constrains
- 261007-feat-pi-display-summary (2026-10-07, commit 4bc2d79e): "ctx.mode is only known at session_start, which is why registration lives there rather than at factory time, keeping child scoped edit/write overrides (write-scopes.ts) unshadowed." — bearing: deliberately revised by confirmed decision (1) for presentation timing only; child execution-scope protection remains constraining
- 261004-feat-pi-compaction-interactive-ux (2026-10-04, commit b49929e0): "Supported plain custom entries and renderers can persist this display without adding model context or waking the model" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/display-summary-session.ts, display-summary-sidecar.ts, index.ts, tool-result-render.ts; installed Pi lifecycle/component evidence above |
| scope.surface | cross-module | adapter store, sidecar ownership/eligibility, tool/message presentation and installed public Pi lifecycle/renderer API |
| scope.new_public_symbol | no | no new user command, configuration key or provider surface requested |
| scope.new_type_contract | yes | internal resolver delegation, session-owned repaint subscriptions and deferred eligibility/replay notifications are confirmed in Decisions; persisted record/output schema stays unchanged |
| scope.test_surface | existing | agents-plugin-pi/test/display-summary-session.test.ts, display-summary-sidecar.test.ts, display-summary-render.test.ts; actual installed-host lifecycle coverage required |
| complexity.reuse_points | confirmed | createDisplaySummaryStore, registerDisplaySummarySession, createSummarySidecar, registerWsTool and wrapToolRenderersWithSummary read; installed registerToolRenderer resolver confirmed |
| complexity.side_effect_risk | high | startup/replacement replay and repaint must remain bound to their originating session; native-fork cache seeding shares eligibility logic |
| risk.correctness | high | rebuilt component identity, invalidator retention, late tool ownership and newer-live-acceptance ordering interact across runtime replacement |
| risk.fit | high | public resolver is available in installed 1.0.4 but absent from pinned 0.84.4 declarations; replacing existing same-name presentation wrappers must preserve delegation and lead/child gating under Decisions (1)-(4) |
| risk.test | high | current fixtures establish renderers and tool names too early; installed-host offline lifecycle and user-driven live verification must distinguish old and fresh rows |
| risk.security_or_contract | moderate | replay changes must retain foreign-file/symlink, retained-row eligibility and session isolation contracts without exposing private contents or altering execution |

## Phases

### Phase 1: Restore existing row summaries across reload and resume

Correct registration/reconstruction order, mounted invalidator lifetime, and
late-owned-tool replay so valid summaries for retained rows render after reload,
in-process resume/session replacement, and cold continuation. Missing or invalid
cached values must continue rendering their original rows. Keep normal fresh-row
summarization unchanged.

Verification must reproduce actual Pi 1.0.4 lifecycle ordering rather than only
asserting store contents. Cover native tool rows, bridge ws tool rows including
one-liner tools, and adapter custom-message rows. Assert old/rebuilt rows and
freshly created rows separately; include cached replay before tool registration,
restored-row repaint, newer acceptance during replay, originating-session
isolation during replacement, collapsed/expanded rendering, and raw fallback.
Preserve Previous conversation exclusion and summary background/layout
regressions. Include actual-host offline component/lifecycle integration and a
subsequent user-driven live reload/resume check; offline tests alone do not
establish the final live TUI result.

Compare the current baseline and repaired adapter under actual-host negative
cases, not just successful summary rendering: resolver delegation missing or
throwing; TUI absent/delayed/incompatible; mode/store/row metadata absent;
unknown/late tools and missing raw renderer slots; adapter summary construction,
delayed render and invalidation failures; raw-to-summary-to-expanded transitions
with legitimate raw component reuse; default/self-frame width parity and
other-extension resolver composition. Assert valid fallback components, no new
adapter-owned escaping errors, no duplicate summary wrapping, and unchanged
execution/schema/tool exposure and child scoped-write enforcement. Keep malformed
or unsupported state raw, and distinguish existing downstream failures from
new adapter failures. Do not run a destructive live crash test in the user's
session; isolated/offline fault injection provides the failure evidence.

## Execution continuation (2026-10-08)

The user authorized correction and continuation after the verifier discovery
stop recorded in `7d6b7fafb`. ESM package discovery now resolves the installed
host's import-only pi-ai entry without dependency changes. The preserved repair
continues on `impl/develop/catty-hertz-trout`; `cc986da44` retains its unit tests.

The separate installed-Pi-1.0.4 verifier passes actual production-factory
registration, reload, replacement and cold-continuation checks, including
baseline-comparative summary construction, delayed-render and invalidation
faults. It exposed an invalidation guard that forgot a failed summary when Pi
rebuilt tool slots; per-row state now fences that failed value until a newer
summary arrives. Full package verification reports 2,234 passes, zero failures
and three existing explicit skips. Live reload/resume acceptance remains
user-pending, independently of these offline results.
