---
title: Pi bootstrap can elide an undelivered workflow manual
---

# Pi bootstrap and manual-elision readiness

## Background

The hosted-manual investigation found a conditional instruction-loss path: native Pi can retain the static manual-body snapshot while failing to obtain the full bootstrap manual. System-prompt construction then receives no manual, but a later `workflow_manual` call can still remove the static body and claim it is already in the system prompt.

Impact is **High, conditional instruction loss**, following section 4 of MAIN's `shared/docs/hosted-manual-injection.md`. The branch mismatch is source-verified; the failure mode is inferred, not reproduced in a live session. Successful manual injection and compaction recovery were observed in the report's sampled native Pi leads, so this is not a claim that all hosted leads currently lack instructions.

MAIN assigned this bug capture and its degraded-fallback follow-up to EXEC3 on 2026-11-14 under docs/tickets-only direction, describing (799)=a as assumed while the owner was asleep. This records that dispatch provenance, not an independently confirmed owner answer or implementation/runtime grant.

## Decisions

1. **Make bootstrap and elision readiness atomic.** A later manual read must not claim static instructions are present unless that session's effective prompt actually contains the corresponding manual. Partial snapshot success must not authorize loss of the only available copy. Preserve full manual delivery when readiness is absent or uncertain.
2. **Preserve valid inherited prompts and role boundaries.** The pinned source has an explicit immutable-fork-prompt path. Do not fix the fresh-lead failure by replacing a delivered fork prompt, assuming every missing local snapshot means missing inherited instructions, or enabling lead-only bootstrap/mapping for workers and explorers. Establish readiness from the actual supported bootstrap/inheritance contract.
3. **Make the MCP-only fallback visibly degraded.** In the same follow-up, expose that fallback lacks native automatic-manual injection, preload expansion, body mapping and dedup. Do not advertise native workflow readiness merely because MCP connectivity or a session key exists. Preserve the existing fallback and key-adoption behavior; no new transport or fallback removal is selected.
4. **Compatibility target: T0.** Prefer internal readiness correction and additive/clarified status presentation, preserving existing tool/API/store behavior. New required fields, a changed protocol or T2 migration require separate approval. Exact status placement/wording and any serialized shape are not settled by this idea capture.

## Source Evidence

The report's initial inspected revision was `951af006a`. EXEC3 rechecked the decisive branches using read-only `git show` at MAIN's requested baseline `1771e2140`:

- `agents-plugin-pi/src/bridge.ts:1272–1294` says snapshot readiness is all-or-nothing, but assigns `manualSnapshotRef` and `staticBodySnapshotRef` in independent success/catch branches. Empty/error results can leave only the static body present.
- `agents-plugin-pi/src/index.ts:1278–1295` passes the full manual snapshot into system-bootstrap computation; static-body availability alone does not create that prompt block.
- `agents-plugin-pi/src/bridge.ts:1146–1161` enables workflow-manual mapping based on the static-body snapshot and role. This does not itself establish that the system prompt received the full manual.
- `agents-plugin-pi/src/bridge.ts:1277–1279` deliberately skips replacing an inherited fork prompt; preserve this distinct path rather than equating all forks with failed fresh bootstrap.
- `agents-plugin-tool/internal/wsagent/pi_lead.go:35–64` selects builtin MCP plus a generated bridge when the bundled extension is unavailable and retains a `PluginWarning`.
- `agents-plugin-tool/internal/wsagent/pi_lead_bridge.mjs` only registers MCP servers and appends the hosted key instruction. That fallback is not equivalent to native bootstrap/preload/dedup.

These are static source observations, not executed bootstrap/fallback acceptance. The shared report is supporting local evidence, not a downstream dependency.

## Constraints

- Preserve initial and post-compaction live-state reads. Injected static instructions are not agenda/todo/note restoration, and active-context dedup is not a persistent per-key delivery ledger.
- Preserve correct full-result fallback, valid active-context dedup and existing native Pi/Claude distinctions. Do not broaden this bug into unconditional Claude manual injection or a universal runtime rewrite.
- Read `ai-docs/manuals/shipped-surface-boundary.md` before shipped tooling/string changes, `ai-docs/manuals/release-compatibility.md` for tooling internals, and `ai-docs/manuals/ws-mcp.md` if MCP wiring changes. Apply authoring/mirroring manuals when editing prompts, playbooks or mirrored surfaces.
- Capture only. No install, login, credential operation, executable/provider probe, tests, build, daemon request/replacement or source patch is authorized by the current assignment. MAIN alone controls deployment under corrected (778).

## Phases

### Phase 1: Align bootstrap/elision readiness and disclose fallback degradation

Implement the confirmed readiness invariant using the actual native bootstrap and inherited-prompt paths, then make MCP-only degradation visible without deleting the fallback or pretending it provides native capabilities. Settle exact mechanism/status presentation and actual compatibility effects before ready promotion. Preserve role gating, hosted identity adoption, immutable inherited prompts and full manual recovery on uncertain readiness.

Verification: isolated fixtures for full success, each independent thrown/error/empty snapshot outcome, both failures, fresh lead versus valid inherited fork, worker/explorer exclusion and later manual reads. Assert that no result says the manual is in the system prompt when absent, and that already-present instructions still support safe elision/dedup without losing mutable state. Cover missing-extension fallback status and retained key/MCP access. Separately retain the report's successful native samples and inferred defect; no live reproduction or equivalent-Claude claim follows from fixtures. Account-backed/live acceptance and daemon delivery require separate authorization and exact source/served pins.
