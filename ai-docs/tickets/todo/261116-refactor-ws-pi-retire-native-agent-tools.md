---
title: "Retire native Pi agent tools in favor of daemon Pi agents"
---

# Retire native Pi agent tools in favor of daemon Pi agents

## Context

The owner wants to retire the Pi extension's native `ws-agent-*` spawning path now that daemon-backed `pi.*` exists. Direct Pi TUI and daemon-independent operation are no longer requirements. Concrete model selection has repeatedly been a footgun; retain capability-tier selection, not per-spawn concrete model or effort selection. Of the native spawn-specific capabilities, port only `write_scopes` narrowly.

Start implementation only after the Pi background-shell idle signal work on `impl/develop/pi-lead-idle-stop`, followed by Pi bundling hardening (loud fallback failure and native-pi regeneration), has merged. Both touch `agents-plugin-pi/src` and `dist`; re-inspect their landed changes before editing. This is a sequencing prerequisite, not a claim that a particular ticket has landed.

Today native `registerAgentTools` owns a session `rpcRegistry` (`agents-plugin-pi/src/spawner.ts:4591`), shared with execute, fork, recovery and dashboard (`agents-plugin-pi/src/index.ts:901,960-998,1149`). The daemon `pi.spawn` has no `write_scopes`, alias, model_effort or concrete-model argument (`agents-plugin-tool/internal/mcp/pi_tools.go:529-543`); its profile and tier ceilings, child forwarding and depth limit remain relevant (`agents-plugin-tool/internal/wsagent/spawn.go:151-176`, `agents-plugin-tool/internal/wsagent/children.go:251-326`).

Dependency inventory and intended disposition, subject to the policy questions below:

- `explore` is a persistent read-only researcher preset using `spawnAgent` and the registry (`agents-plugin-pi/src/spawner.ts:4833-4900`). Recommend replace with tier-small `pi.spawn` and a read-only brief, not claim sandbox enforcement; whether to drop the preset is (1227).
- `ws-fork` spawns into that registry and arms fork-specific wiring (`agents-plugin-pi/src/fork.ts:355-378`). Recommend migrate its useful interaction to daemon Pi agents rather than silently lose it; whether to drop it is (1228).
- `ws-report-to-lead` returns from the child tool while the parent observes its RPC tool-start event and pushes progress (`agents-plugin-pi/src/spawner.ts:3074-3086,4816-4829`). It depends on the native RPC observation path. Migrate any required progress reporting to daemon messaging, not pretend completion mail is equivalent.
- `ws-execute` spawns a registry worker and `ws-approve` resolves that record (`agents-plugin-pi/src/execute-gateway.ts:711-740,774-795`); the approval request is relayed as a steerable `ws-agent-approval` (`agents-plugin-pi/src/execute-gateway.ts:595-634`). Migrate the approval workflow to daemon-backed agents and preserve actionable request/response correlation; completion mail is not approval transport. No orphaned approval or silent auto-approval.
- Sidecar shutdown snapshots the registry and startup revives entries, including execute/fork wiring (`agents-plugin-pi/src/index.ts:328-345,374-389,1165-1176`); it preserves selected metadata, not live clients or report history (`agents-plugin-pi/src/agent-sidecar.ts:140-167,354-358`). Replace native orphan revival with the daemon's lifecycle and explicitly handle already-persisted native sidecars; do not promise live process or report recovery. Decide migration/refusal behavior before deleting the reader.
- Dashboard feed projects the registry and relayed descendants, including approval state (`agents-plugin-pi/src/index.ts:960-988`, `agents-plugin-pi/src/dashboard-feed.ts:350-377`). Migrate to daemon agent state, preserving useful status and approval visibility without duplicate rows; do not keep a dead registry as a dashboard source.

Native push events include progress `ws-agent-report`, terminal/idle `ws-agent-settled`, approval, advisory, question and orphan notices (`agents-plugin-pi/src/spawner.ts:1003-1017`). Daemon settled-turn completion for a lead is system mail labelled `pi:<child-id>` with replies via `pi.send`, not `mailbox.send` (`agents-plugin-tool/internal/mcp/pi_tools.go:497-510`, `agents-plugin-tool/internal/mcp/lead_mail.go:227-229,266-280`); nested parents receive steering or queued delivery, and `pi.wait` may consume the report instead of redelivery (`agents-plugin-tool/internal/wsagent/children.go:509-543,729-770,832-883`). Completion mail replaces settled notification, not intermediate progress, approvals, questions, advisories or orphan recovery. Design explicit mappings for each retained behavior and test once-only delivery.

## Decisions

- Daemon Pi agents replace native `ws-agent-*`; daemon independence is not a goal. No concrete-model selection or per-spawn model effort; use existing `agents.tier` Pi mapping and tier ceilings. Do not port native aliases or other spawn options merely for parity.
- Port `write_scopes` only as the native edit/write capability, not as a shell/process sandbox. Existing native admission validates absolute file/tree grants, existing parents/tree roots, canonical paths and restricted positive includes; it validates grants even when unrestricted edit/write already exists, then ignores them with a diagnostic in that case (`agents-plugin-pi/src/spawner.ts:3698-3747`, `agents-plugin-pi/src/write-scopes.ts:54-99,180-211`). Scoped native edit/write calls canonicalize and check each target; descendant grants must be a conservative subset, with a uniform denial (`agents-plugin-pi/src/write-scopes.ts:213-293`). Bash and other write mechanisms are not confined.
- Enable daemon `pi.*` by default for Pi leads, while respecting explicit configuration, host availability and version gates. Pi currently defaults off (`agents-plugin-pi/dashboard-config-manifest.json:35-38`; compare `agents-plugin/dashboard-config-manifest.json:35-38`); inspect all three manifests and packaged mirrors. Switch Pi playbook idioms, including explorer wording, from native names (`agents-plugin-tool/internal/mcp/playbook_tools.go:46-47,160-170`) to namespace-correct `pi.spawn`/`pi.send` and capability tiers.
- Compatibility: an optional compatible argument is normally T0; a new caller must capability-gate against an old server. MCP removal without a working alias is T3, requiring a minor boundary and explicit owner approval (`ai-docs/manuals/release-compatibility.md:42-68`). Native extension-local, version-pinned tools need a separately justified directional verdict, not an automatic MCP classification; (1229) remains open. A manifest *default value* change is not automatically the T3 unknown-manifest-field case: trace old/new readers, explicit overrides, persistence and reopen behavior, grade the highest actual path and obtain approval if T3. Do not assert a tier without that evidence.

## Open Decision Queue

- (1227) `explore`: (a) drop the native preset and use small-tier `pi.spawn` with a read-only brief; (b) retain an equivalent daemon-backed preset. Recommend (a), matching the narrow port, while clearly stating a brief is not enforcement.
- (1228) `ws-fork`: (a) drop it outright; (b) migrate its question/thread behavior to daemon Pi agents. Recommend (b) if still needed by workflows; inventory users and owner-confirm before implementing. Neither option is authorized yet.
- (1229) Removal compatibility: (a) treat extension-local removal as T3 and ship at an approved minor boundary; (b) approve a lower tier only after proving version-pinned client/extension/dist and persisted-session directions cannot issue removed calls. Recommend (a) conservatively; determine the actual paths and get explicit owner approval for any T3 release. Do not conflate this decision with the owner direction to retire the tools.

## Phases

### Phase 1: Scoped daemon spawn and Pi defaults

Add optional `write_scopes` to `pi.spawn`, carry its validated effective capability through daemon runner launch and nested forwarding into actual Pi edit/write enforcement. Preserve the native grant, canonicalization, include, unrestricted-profile diagnostic and conservative delegation semantics above; enforce at tool execution, not only by prompt or server admission. Ensure a scoped child cannot gain unrestricted edit/write through profile choice or descendant spawn. Define omitted scopes and existing profile behavior compatibly; preserve tier/profile/depth ceilings. Use an enforceable worker-side mechanism even for daemon-hosted Pi sessions without the native extension, or fail closed rather than claim enforcement. Document the Bash limitation.

Enable default Pi-lead availability and switch all relevant rendered playbook idioms. Trace the manifest default compatibility direction and old-server capability detection before release. Acceptance: tests cover accepted/invalid file and tree grants, include patterns, existing and new targets, symlink/canonical escape, uniform denial, scoped and unrestricted profiles, no escalation on nested spawn, actual worker edit/write calls and explicit shell limitation; legacy no-scope calls and configured opt-out work. Test rendered Pi lead and daemon-worker playbooks, host/version disabled cases, and mixed-version behavior.

### Phase 2: Migrate registry dependents

After Phase 1 and resolution of (1227)-(1228), implement the dispositions above. Specify and test progress versus completion, questions, actionable approval correlation and responses, advisory delivery, daemon restart/reconnect, persisted native orphan handling, dashboard state and descendant visibility; verify fresh spawn and dormant/recovery paths. Keep `ws-execute`/`ws-approve` behavior safe, or explicitly obtain an owner decision before changing its contract. No lost or duplicate settled mail when `pi.wait` consumes a result. Do not remove the registry until every retained consumer has an equivalent source.

### Phase 3: Remove native agent registrations and ship mirrors

After Phase 2 and compatibility decision (1229), remove native `ws-agent-spawn`, send, list, stop and transcript registrations, obsolete registry machinery and tests, plus other native surfaces only as resolved above. Update `pi-lead-guide.md`, push/render/compaction references and workflow instructions to the actual retained transports. Regenerate `agents-plugin-pi/dist` and first-party `agents-plugin/native-pi` and `agents-plugin-wsflow/native-pi` source/dist/guide mirrors using the landed bundling process; satisfy the dist freshness guard. Test registration absence, retained flows, mirrored assets/freshness, and the approved old/new compatibility boundary. No merge or release is implied by this ticket.
