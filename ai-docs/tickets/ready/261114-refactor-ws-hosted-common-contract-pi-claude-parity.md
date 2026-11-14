---
title: "Hosted agents: common internal capability/control contract with Pi and Claude behavior preserved"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261113-bug-ws-hosted-session-sentinel-bootstrap-mints-foreign-key: "identity preservation reference; not permission to change bootstrap authority"
  261114-bug-ws-fork-parent-identity-loss: "lineage preservation reference; existing policy is not changed here"
  261114-feat-ws-conversation-audit-event-store: "future consumer; no new blocking dependency"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 43441150447687c9
sage-review-completeness-reviewed: 43441150447687c9
---

# Hosted agents: common contract with Pi and Claude parity first

## Background

The owner wants hosted-agent commonality before adding another harness. On 2026-11-14, whiteboard (665)=b explicitly selected common functionality with Pi/Claude behavior preservation as the first implementation. The owner has no Antigravity account and deferred actual Antigravity integration to a substantial separate follow-up.

The existing `wsagent` package already supplies `Backend`, `StartSpec`, `Process`, optional process-control interfaces and a shared `Event` model. This is an internal consolidation of that boundary, not a replacement runtime or a migration away from native harness integrations.

## Decisions

1. **Pi/Claude parity is the release criterion, not simply compiling a generic interface.** Preserve their existing launch, controls, lifecycle and integration differences while centralizing the common capability/control contract. Rejected: implementing Antigravity in this first unit, or flattening backend-specific behavior to the weakest common subset. Authority: owner (665)=b.
2. **Reuse `Backend`/`Process`/`Runner`.** The worker may choose internal types and decomposition within this package, but must leave existing caller-visible behavior unchanged. Rejected: a new universal runtime beside the existing runner.
3. **Preserve native integrations.** Claude mods/plugins and Pi extensions, bundled-resource resolution and user configuration remain active under their existing isolation rules. MCP attachment is not a substitute for hosted process control or native integration.
4. **Compatibility is T0.** This ticket changes only internal structure and regression coverage. Existing MCP/HTTP semantics, on-disk profile/record fields, event meanings, session identities and protocol versions remain unchanged. A newly discovered need to change one returns to the lead; this ticket does not authorize it.
5. **Deferred scope is separate.** No Antigravity install, authentication, provider call or live probe; no voice feature, root-key mint restriction, fork-authority policy reversal or API removal. These are not prerequisites for this work.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Preserve one owner of input/mail delivery for hosted sessions, existing ordering and duplicate-prevention behavior, approval request/result correlation, key adoption and parent lineage, holder attachment, replay gaps and recovery holds. Commonality must not create a second delivery path.
- Preserve each backend's actual supported controls and `ErrUnsupported` fallback. In particular, an internal capability description must not advertise controls merely because another backend implements them.
- Keep the default behavior of existing profiles and the backend names `pi`/`claude`; unknown backend and unsupported-control errors remain explicit.
- No daemon replacement, live session restart, installer execution or credential-dependent acceptance is part of this implementation ticket. Use existing isolated fixtures and fake processes for contract verification.
- Manuals for implementation: `ai-docs/manuals/shipped-surface-boundary.md`, `ai-docs/manuals/release-compatibility.md`; also `ai-docs/manuals/ws-mcp.md` if MCP package wiring is touched. Any skill/prompt/convention change additionally requires `ai-docs/manuals/skill-authoring.md` and applicable mirroring guidance.

## Prior Art

- `agents-plugin-tool/internal/wsagent/backend.go`: `Backend`, `StartSpec`, `LeadSpec`, `Process`, optional approval/mode/MCP/steer/report/command/turn-probe controls and `Event`.
- `agents-plugin-tool/internal/wsagent/profile.go`: `BackendClaude`, `BackendPi`, strict `Profile` parsing and current backend-dependent defaults.
- Existing adapter implementations: `PiBackend.Start/startLead` in `pi.go`, `ClaudeBackend.Start` in `claude.go`; shared orchestration in `runner.go` and holder/lead paths.
- Existing test anchors: `TestClaudeLeadPluginArgsModes`, `TestClaudeLeadRejoinIgnoresChangedPluginLaunchInputs`, `TestLeadRejoinsItsProcessAfterADaemonRestart`, wrapped-ring recovery tests, `TestSteeredReportTheRunMissedGoesAsTheNextTurn`, and `TestHostedPiRowUsesOwnUsageWhileRunningAndAfterRecovery`.

## Prior Decisions

- 261113-feat-ws-compaction-before-queued-followup (2026-11-14, commit): "The hold gates mail and child reports too: a mail-only turn would push the compaction back the same way (Decision 3, hold scope)." — bearing: constrains
- 261113-bug-ws-release-gate-sweep-fixes (2026-11-14, Result): "bundledPluginRoot has no runtime.Caller source-tree fallback. Without WS_RSRC_ROOT it returns an error that names the launcher." — bearing: constrains
- 261113-bug-ws-pi-subagent-turn-dies-on-daemon-restart (2026-11-13, commit): "Reuse the existing holder pipe transport and publish/settle path, not a new delivery mechanism. Lead privileges remain independent of holder selection;" — bearing: constrains
- 52dc582b (2026-11-13, commit): "Pi restoration wraps its own in-process SDK resolver instead of pre-enumerating extensions, preserving project trust hooks, ordering, package filters and reload without initializing extensions twice." — bearing: constrains
- 261113-bug-ws-hosted-sessions-show-no-pi-or-batch-token-usage (2026-11-13, Result): "Hosted Pi's rate is an approximate first-update-to-message-end figure. Align it with native Pi's visible-block/reasoning-excluded estimator if exact cross-surface rate parity is needed." — bearing: constrains
- 261112-bug-ws-hosted-pi-subagents-missing-from-dashboard (2026-11-12, commit): "status/permission/label/model stay immediate; batches, contextTokens, outputTps coalesce to 2 s; generating.chars ignored. CLI feed untouched." — bearing: constrains
- 261110-feat-ws-hosted-session-mode-flag (2026-11-10, Decisions): "A successful RPC compaction is unverified and does not block this ticket, since Decision 1 leaves compaction untouched." — bearing: constrains
- 261108-feat-ws-daemon-agent-runner (2026-11-08, Decisions): "Control through the daemon's REST API (user direction): consumers start, send to, interrupt, and stop agent conversations over daemon routes." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Existing boundary spans agents-plugin-tool/internal/wsagent/backend.go, profile.go, pi.go, claude.go and runner.go. |
| scope.surface | internal | Consolidation stays in agents-plugin-tool/internal/wsagent; REST consumers remain unchanged (profile.go#L1-L11; runner.go#L109-L167). |
| scope.new_public_symbol | no | No new caller-visible surface is authorized by Decisions 2 and 4; Backend and Process already exist (agents-plugin-tool/internal/wsagent/backend.go#L19-L109). |
| scope.new_type_contract | yes | Phase 1 makes the internal capability/control contract explicit; existing optional interfaces are agents-plugin-tool/internal/wsagent/backend.go#L112-L172. Exact internal decomposition is worker-owned. |
| scope.test_surface | existing | Named anchors exist in agents-plugin-tool/internal/wsagent/claude_lead_plugins_test.go#L43-L98, claude_lead_recovery_test.go, lead_runner_test.go#L783-L1019, children_test.go#L401-L441 and hosted_usage_test.go#L142-L207; new common expectations can extend these existing fixtures. |
| complexity.reuse_points | confirmed | Read Backend/StartSpec/Process/Event in agents-plugin-tool/internal/wsagent/backend.go; Runner uses the same StartSpec for adapters (runner.go#L1961-L2189). Fake processes and ordered holder streams exist (fake_test.go#L116-L135; lead_fake_test.go#L21-L51). |
| complexity.side_effect_risk | high | Runner consolidation intersects process launch, key checks, capability tokens and holder attachment (agents-plugin-tool/internal/wsagent/runner.go#L1989-L2189). |
| risk.correctness | high | Replay holds differ by harness: Claude waits for a boundary, Pi probes running state (agents-plugin-tool/internal/wsagent/lead_runner_test.go#L902-L952 and #L1121-L1212); untaken reports must be requeued once (children_test.go#L401-L441). |
| risk.fit | moderate | Reuse is available, but actual controls depend on lead status and arguments, not backend name alone (agents-plugin-tool/internal/wsagent/claude.go#L415-L455; pi.go#L583-L617). Pi launch also has SDK and bundled-resource fallbacks (pi_lead.go#L26-L103). |
| risk.test | moderate | Existing fixtures cover plugin modes, rejoin, approval correlation, mail ordering and isolation (agents-plugin-tool/internal/wsagent/claude_lead_plugins_test.go#L43-L98; lead_runner_test.go#L154-L207, #L457-L512, #L1220-L1264; lead_isolation_test.go#L36-L124). They have not been executed in fact population; no live parity is established. |
| risk.security_or_contract | high | Profile JSON, persisted identities and approval correlation are protected contracts (agents-plugin-tool/internal/wsagent/profile.go#L69-L130, #L238-L246; record.go#L22-L111; lead_takeover_test.go#L141-L212). Native fork lineage and sentinel authority remain separate related ideas, not authorized policy changes. |

## Phases

### Phase 1: Consolidate the common hosted capability/control boundary without behavior drift

Make the common internal contract explicit and consolidate duplicated backend-selection/capability decisions at the existing runner/adapter boundary. Add focused contract tests that run the same common expectations against Pi and Claude fixtures while retaining separate assertions for backend-specific supported operations and fallback behavior. Migrate existing call sites to the common boundary rather than leaving a parallel unused abstraction.

Verification:

- Run the existing `wsagent` tests plus focused new common-contract tests with fake processes/transcript fixtures; report actual commands and results.
- Cover fresh start/resume, model/effort update or existing restart fallback, send/steer/interrupt, unsupported controls, tool approvals where supported, native plugin/extension launch inputs, and isolation/default settings.
- Cover ordered event delivery, input/mail single ownership, identity/key/parent preservation, holder rejoin, wrapped replay rings, approval correlation and no duplicate queued input.
- Assert that profile JSON shape/backend values, persisted session/holder records and externally served event meanings are unchanged. Run affected dashboard/MCP tests only where corresponding wiring is touched.
- A passing source suite is source verification, not evidence of live delivery. Record any pre-existing failure separately; do not repair an unrelated frozen lane to make this ticket pass.
