---
title: Persistent explore preset with an optional deep-research mode
related:
  260906-research-ws-pi-recon-preset-agent-alias: decision history; actionable continuation
  260906-feat-ws-pi-lead-explore-as-async-rpc-child: prerequisite; replaces its deliberate lead/fork one-shot lifecycle without editing its frozen Result
  260906-bug-ws-pi-tier-slug-rejected-children-inherit-parent-model: coordinate shared tier resolver and blocking-leaf effort; this ticket owns stricter exploration failure behavior and deep-mode effort inheritance
spec:
  - 260903-pi-explore-recon-leaf
plans:
  phase-1: 2026-09/07-1531-260907-feat-ws-pi-persistent-explore-deep-research
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 7e08cef8d289588b
sage-review-completeness-reviewed: 7e08cef8d289588b
---

# Persistent explore preset with an optional deep-research mode

## Background

Lead dogfooding tried to continue an explore child after stopping it and received `unknown agentId`. The existing async RPC explore contract deliberately deletes one-shot records and refuses continuation. The owner instead wants a thin exploration preset over the ordinary persistent agent pipeline, preserving actual read-only tool restrictions, not just a prompt requesting read-only behavior.

The owner also needs judgment-heavy research without making every file read run on an expensive model. A single boolean chooses the research method rather than exposing four tiers or arbitrary models. This ticket implements the confirmed direction from `260906-research-ws-pi-recon-preset-agent-alias`; that research remains historical context, not a prerequisite implementation unit.

## Decisions

### Public lead/fork interface

```ts
explore({ query: string, deep_research?: boolean })
// deep_research defaults to false
// Returns { agent_id, alias } immediately through the existing RPC spawn path.
```

Keep the public `explore` name. Do not add public `model`, `model_name`, `tier`, or effort selectors to this preset. Public `ws-agent-spawn` remains unchanged. This is an adapter-owned preset, not an arbitrary caller-selectable tool-group API.

| Behavior | Omitted / false | true |
|---|---|---|
| Model and effort | Resolve the configured `small` tier | Capture the dispatching lead/fork's model and effort at spawn time |
| Tools | Existing no-bash `read-only` group; no child-spawn capability | Adapter-owned composition of `read-only` plus blocking `explore` for small-tier collection |
| Work | Direct reconnaissance and summarization | Research direction, judgment and synthesis, with optional cheap collection delegation |
| Lifecycle | Ordinary persistent RPC agent | Ordinary persistent RPC agent |

Deep mode may read directly; delegation is available, not mandatory. Small collections can cost less when read directly than when duplicated across two contexts. The boolean does not promise a stronger or cheaper model: it inherits the caller's actual model. Do not silently replace it with the configured `large` tier.

Capture deep mode's model and effort together at creation. Retain that selection across parking, send, stop/resume and sidecar restoration; later changes to the parent's model/effort do not silently retune an existing child. This is a scoped exception to the sibling tier ticket's decision not to forward parent effort on ordinary inherit; unrelated spawn callers retain their existing behavior.

### Bounded read-only delegation

Bundle the exploration prompt and actual tool restrictions in the preset. The current adapter `recon` group includes unrestricted `bash` and is not a capability-enforced read-only profile. Reuse the existing `read-only` group (`read`, `grep`, `find`, `ls`) for default researchers and deep-mode collection leaves. Add an adapter-internal `read-only` plus blocking `explore` composition for deep researchers; do not expose an arbitrary tool-list override on public `ws-agent-spawn` or change ws-mcp rsrc inventory. Neither mode gains edit/write, unrestricted execution, arbitrary agent spawn, or fork tools. Deep-mode research receives only a blocking small-tier exploration child tool; that leaf cannot delegate further. Default exploration gets no blocking explore tool. Preserve existing worker/execute-worker blocking explore behavior rather than turning those callers into persistent-agent orchestrators.

The depth policy permits lead/fork -> deep researcher -> blocking recon leaf, and stops there. Recon restrictions and the selected mode survive continuation and process restart; do not recover a research child as a general worker merely because its process is being rehydrated.

### Cost safety

A simple explore or deep researcher's collection leaf launches only after `small` resolves to an available authenticated model. Missing Pi tier configuration, unknown model, missing auth, empty catalog, or failed/unparseable resolution must produce an explicit failure rather than silently inheriting the expensive parent. A refused launch must not leave an active process, session directory, registry entry or alias reservation.

The deep research parent intentionally inherits and does not require `small` until it actually requests a collection leaf; failure of that leaf is returned to the researcher, not transformed into an expensive inherited launch. This ticket's exploration-only fail-closed rule is stricter than the sibling tier ticket's transport-failure fallback. Extend the shared resolver outcome to expose transport/parse failure distinctly from intentional inheritance and accepted tier resolution; exploration callers refuse on that failure, while unrelated callers retain their existing fallback policy. Do not duplicate the resolver in a preset-local implementation. Preserve compatibility with the sibling's `source` discriminator rather than treating all `source: inherit` outcomes as equivalent. Honor resolved small-tier effort on the collection path.

### Ordinary lifecycle, not another runtime

Reuse the established RPC registry, automatic alias/title creation, send, park, stop, resume, sidecar restoration, transcript and push machinery. Stop retains a resumable record, and settled records are not auto-deleted solely because they were created by `explore`. Keep the recon answer on the existing settle `last_message` path; no reporting capability is added merely to imitate worker reports. Repeated sends must deliver repeated answers and must not keep the goal-loop fan-in permanently busy after settling.

Allocate auto aliases without colliding with retained/restored aliases, including after the counter restarts. Preserve mode, permission profile, model/effort and identity across lifecycle transitions. Prefer fixing/reusing common preservation paths to constructing an exploration-specific lifecycle.

Rejected: a public tier/model override, because the owner chose a simpler two-mode schema; a fixed large-tier deep mode, superseded by explicit lead-model inheritance; making all recon children recursive; widening the child to a full worker just to enable delegation; retaining one-shot disposal for the lead-facing preset.

## Constraints

- Adapter implementation stays in `agents-plugin-pi/`; no ws-mcp Go or shared rsrc semantic edits authored on the Pi track. Adapter-owned prompt composition can express the two modes without forking shared playbook content.
- Preserve the predecessor's frozen Result as evidence of intentional former behavior. Do not describe the existing one-shot contract as an accidental registry bug.
- The sibling tier-resolution ticket is coordination, not a whole-ticket prerequisite: implement this preset's fail-closed checks and effort contract against the available shared seams, reconciling with that ticket if it lands first. Its unrelated advisory work is not required here.
- No changes to the separately tracked workflow-manual static-body warning.

## Spec Impact

Update `ai-docs/spec/pi-adapter-runtime.md` at `260903-pi-explore-recon-leaf` and affected tool-group, bounded-depth, model-inheritance, lifecycle/sidecar, widget and delegation-tool passages. Document the boolean schema, ordinary retained lifecycle, settle-answer delivery, permission differences between modes, frozen spawn-time inheritance, and fail-closed small selection. Update adapter tool descriptions and lead guidance together, removing the obsolete one-shot/non-resumable guidance for lead/fork exploration while preserving worker leaf guidance.

## Phases

### Phase 1: Persistent two-mode exploration over the existing pipeline

Implement the public contract and permissions above as one complete behavior slice. Replace the lead/fork preset's one-shot lifecycle with ordinary persistent RPC handling; provide read-only deep-mode blocking collection without widening default exploration or changing ordinary worker leaf registration. Reuse common lifecycle machinery and make mode/role/permissions and effective model/effort survive both in-process continuation and sidecar restoration. Make automatic aliases collision-safe and update the spec and guidance in the same slice.

Verification must cover:

- Omitted and false `deep_research` are identical; true captures the actual dispatching lead/fork model and effort. The schema has no public tier/model override.
- Simple researchers lack blocking explore; deep researchers have it; collection leaves have no recursion. Both modes lack mutation tools on first launch, live send and restart.
- Changing the parent's model/effort after spawn does not change a continued or restored researcher. Small model and effort reach collection leaves.
- Small unset/unknown/no-auth/empty-catalog/transport/parse failures refuse without expensive fallback or launch artifacts. Deep parent creation remains possible without small; a requested collection reports failure explicitly.
- Immediate id/alias return, settle answer delivery, subsequent send and second answer, dormant park, explicit stop followed by resume, failure handling via ordinary lifecycle, transcript access, sidecar capture/restore, and collision-free aliases after restart.
- Goal-loop and widget/fan-in behavior follows ordinary agents: a running research child counts, settled dormant records do not spin the goal loop, and retained records are inspectable as ordinary agents.
- Existing worker/execute-worker blocking exploration remains blocking, read-only and self-reaping. Shared resolver changes do not accidentally change unrelated spawn callers' policies.
- Run the adapter tests with realistic tool-surface/lifecycle fixtures, not only flag assertions. Owner-run dogfood: simple explore -> settle -> send follow-up; deep explore -> small blocking collection -> synthesis; stop/resume; restart and follow-up with recorded role/model/effort intact. No implementation completion claim substitutes mocked coverage for the live check.

## Implementation checkpoint - 2026-09-07

Implementation is committed on `impl/track/pi-agent/suave-kooky-halt`: initial implementation `048a9cb2`, review corrections `6998a5b0`, against plan baseline `1083e01a`. The unrelated research-ticket commit `5a962f0a` is preserved and excluded from implementation review scope.

Partitioned review found two Critical issues (effective model/effort verification and accidental worker recon permission removal), plus Important restoration/spec/test gaps. Relay 1 reports every Important fixed; the independent Critical-only second review resolved both Critical findings, with seven focused tests passing. Important dispositions are implementer self-reports, not independently re-reviewed. Detailed dispositions: `ai-docs/.plans/2026-09/07-1531-260907-feat-ws-pi-persistent-explore-deep-research.review-dispositions-cycle-1.md`.

Relay verification reports 946/946 tests passing both normally and with inherited worker/deep environment markers, a no-model dynamic allowlist probe, and successful package dry-run including the new exploration guide. Initial 941-pass evidence was insufficient: review exposed environment-dependent failures and missing coverage, corrected in the relay. Spec index verification passed after the documentation audit.

**Phase 1 remains unfinished pending the mandatory owner-run live provider/restart dogfood above.** Automated tests and the no-model probe do not clear this gate. No Result is recorded and the ticket remains ready until live evidence is recorded. Merge has not been authorized or performed. Spec and adapter guidance are updated; no shared rsrc or ws-mcp code was changed.

## Blocked (2026-09-09)

Blocked on a human-only gate, not on agent-doable work. Phase 1's automated
slice is already complete on `impl/track/pi-agent/suave-kooky-halt` (impl
`048a9cb2`, review corrections `6998a5b0`, plan baseline `1083e01a`):
partitioned review closed both Critical findings, Important dispositions are
implementer self-reports, 946/946 adapter tests pass, and the spec plus adapter
guidance are updated. The sole remaining item is the **mandatory owner-run live
provider/restart dogfood** the Phase 1 verification list names (simple explore →
settle → send follow-up; deep explore → small blocking collection → synthesis;
stop/resume; restart and follow-up with recorded role/model/effort intact) —
explicitly a live check that mocked coverage cannot substitute. No Result can be
recorded and no merge authorized until the owner runs that dogfood and records
the evidence, so this queue turn cannot advance it further.

Unblocks when: the owner runs the live dogfood and records the evidence (append
the Phase 1 Result and clear this note). Drain-queue selector should skip this
ticket until then.
