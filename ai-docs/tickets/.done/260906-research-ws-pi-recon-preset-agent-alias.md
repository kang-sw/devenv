---
title: Replace lead-only one-shot explore with a recon-restricted preset alias over ordinary ws-agent lifecycle
related:
  260907-feat-ws-pi-persistent-explore-deep-research: actionable continuation of the confirmed two-mode persistent preset
  260906-feat-ws-pi-lead-explore-as-async-rpc-child: existing deliberately one-shot lead RPC contract to reconsider
  260906-feat-ws-pi-tool-and-push-tui-polish: coordinate dispatch presentation if tool exposure changes
completed: 2026-09-09
---

# Replace lead-only one-shot explore with a recon-restricted preset alias over ordinary ws-agent lifecycle

## Background

Dogfooding repeatedly attempted ws-agent-send to a lead explore child and received an explicit one-shot refusal. Owner requested investigation of removing the lead-facing standalone explore surface in favor of an alias/preset tool over ordinary ws-agent creation and resumption. Owner explicitly confirmed that the preset must bundle BOTH exploration system prompt and recon tool restrictions, not merely a prompt instruction. No implementation or final API/name was approved; ticket-only capture ends this session.

## Existing contract and source evidence

The existing 260906 ticket deliberately chose async RPC lead/fork explore with immediate id return, settle last_message delivery, send refusal, deletion after completion/stop/failure, and no dormant sidecar persistence. Its Result is frozen; any reversal needs an explicit new contract, not silent rewriting.

Source survey found the lead/fork preset sets oneShot:true; sendToAgent rejects it, attachEventListener deletes at settle, stop/failure paths delete, and agent-sidecar captureOrphans excludes it. Removing only the send guard cannot make it resumable. Commit `21488404` introduced the deliberate behavior; `1fed78ba` added failure deletion to avoid unreachable retained records. Ordinary alias retention and park/resume are established by `4060bc59`/`a8566a79`.

Internally spawnAgent supports spawnRole and toolGroup; recon excludes edit/write and recursive agent tools. Public ws-agent-spawn does not expose a documented caller-selectable toolGroup/restriction argument. Thus the owner's expectation of supported restrictions is supported internally, not established as a public API. A fixed adapter-owned preset can potentially reuse that internal seam without exposing arbitrary authority selection.

## Proposed investigation

Investigate a lead/fork-facing preset alias that selects an adapter-owned exploration prompt and recon tool group, then uses the ordinary persistent RPC registry, ws-agent-send, stop/park, dormant resume and sidecar paths. Preserve worker/execute-worker blocking exploreLeaf and depth/permission restrictions. Do not turn a read-only recon child into a general worker or infer permission expansion from resumability.

The alternative minimal change is removing oneShot:true only from the existing lead/fork preset; compare it with the requested renamed/replaced alias tool in terms of schema, prompt duplication and migration cost. Public name, preset acquisition interface, whether it directly spawns or provides inputs to ws-agent-spawn, and compatibility policy remain open. Do not assume a new public toolGroup parameter is desired.

## Lifecycle and verification boundaries

Ordinary non-one-shot records retain aliases, accept send, park on settle, and participate in sidecar restoration. Rehydration restores toolGroup/systemPromptPath/sessionPath/model fields; confirm recon restriction survives live send and restart. Survey found resumed role marker defaults to worker even though cached recon tool group remains; verify no capability widening through other role-sensitive paths.

Current fan-in already counts running RPC records and excludes settled dormant records, so no counting change appears necessary, but regression-test goal continuation. Decide how recon final answer reaches the lead: retaining last_message settle delivery versus adding report capability is not settled. Preserve read-only boundaries pending that decision.

Auto-explore alias counter resets after restart; restored explore-1 can collide with a new auto alias. Skip retained aliases or define an equivalent collision-safe strategy if alias continuity is promised. `fafe85bb` recorded this risk.

Tests should cover lead/fork preset restrictions, alias-based continuation, settle/stop/failure retention, sidecar capture/restore, resumed role/tool curation, collision after restart, fan-in, and unchanged worker blocking leaf. Update tool descriptions, lead guidance and runtime spec together once the API is decided. No source-level alias-tool implementation has been attempted.

## Confirmed direction - 2026-09-07

Owner approved retaining `explore` as an ordinary persistent agent preset, with `deep_research?: boolean` defaulting to false, rather than exposing a public tier/model override. False selects small with recon-only tools. True inherits the dispatching lead/fork model and effort at spawn time and permits small-tier blocking exploration for collection; the collection leaf cannot delegate. Both modes remain read-only. Spawn-time inheritance is retained across resume. A small-tier resolution failure must refuse instead of falling back to the expensive parent. The actionable contract and verification are owned by `260907-feat-ws-pi-persistent-explore-deep-research`; earlier open questions above are preserved as discussion history. The owner requested sage review and ready promotion of that implementation ticket, not source implementation in this session.

Follow-up source verification found that the existing `recon` group includes unrestricted bash. Owner approved using the adapter's existing no-bash `read-only` group for default researchers and collection leaves, and an adapter-internal `read-only` plus blocking `explore` composition for deep researchers. No public tool-list override or ws-mcp rsrc change is needed. Owner also approved exposing failure causes through the common resolver, with exploration-only refusal and unrelated caller policies preserved.


## Resolution (2026-09-09)

Owner approved closing this research as absorbed by 260907-feat-ws-pi-persistent-explore-deep-research. The confirmed two-mode persistent read-only explore decision and its verification contract belong to that existing ready ticket. This research closure does not clear that implementation ticket's remaining live acceptance.
