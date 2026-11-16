---
title: "Allow subagents to acquire and release pooled worktree leases"
related:
  261116-feat-ws-worktree-release-guard: existing lease-holder and dirty-unleased release guard
  260915-bug-ticket-worker-serial-route-acquires-extra-worktree: preserve the user-approved serial versus parallel worktree contract
---

# Allow subagents to acquire and release pooled worktree leases

## Context

The owner proposes letting native and Pi subagents manage pooled leases and
adding a merge-time reminder about checked-out source branches. In the triggering
incident, delegates could not acquire from the pool, so the lead directed them
to create raw worktrees under `D:/Temp`; the dashboard detected them outside the
pool. Its pending-worktree admission view excludes pooled paths; pooled
worktrees have a separate card (`agents-plugin-tool/internal/wsdashboard/rail_worktree_admission.go:292-314`,
`agents-plugin-tool/internal/mcp/dashboard_worktrees.go:94-135`).

Today acquire and release are lead-only both at dispatch and in their handlers;
Pi worker profiles also omit them (`agents-plugin-tool/internal/mcp/server.go:126-148,729-767,1442-1447,1492-1496`,
`agents-plugin-tool/internal/wsagent/builtin.go:83-112`). Acquire records the
caller as lease parent and mints a lead-scoped holder key at the acquired root
(`agents-plugin-tool/internal/mcp/server.go:1475-1479`,
`agents-plugin-tool/internal/mcp/worktree_tools.go:350-369`). The release guard
already admits the holder's own key or lease parent, but also accepts a supplied
holder `key` independent of caller identity (`agents-plugin-tool/internal/mcp/worktree_tools.go:769-792`).
The holder-key test is at `agents-plugin-tool/internal/mcp/worktree_release_guard_test.go:144-151`.

`git.merge` checks whether the target is held elsewhere; a checked-out source
can instead leave a successful merge with branch deletion refused and a cleanup
advisory (`agents-plugin-tool/internal/mcp/git_merge.go:232-270,398-421`).
`worktree.list` already gathers pooled branch, path and lease facts
(`agents-plugin-tool/internal/mcp/worktree_list.go:15-67,128-159`).

## Decisions

- P1: native subagents AND Pi profiles (pi-worker, pi-mini-lead) may call worktree.acquire and worktree.release. (1189)=a: open Pi profiles now too (Pi uses the acquired path as cwd for nested Pi).
- (1188)=b: force release is allowed ONLY for the root lead (the user-facing lead session), not the whole lead scope (native lead-run workers excluded). Rationale: force deletes uncommitted work; this morning's incident was another session's work being wiped.
- (1190) ok: the worktree key acquire mints never exceeds the caller's scope (a delegate's acquire yields a delegate-scope key) - blocks privilege escalation.
- (1191) ok: a non-lead caller may release only by identity (the holder itself or its lease parent); knowing the `key` argument alone is not enough - blocks sibling release. Root lead behavior and existing tests unchanged.
- (1192) ok: worktree.remove and worktree.list stay lead-only.
- (1193) ok: P2 nudge - after a successful git.merge, an advisory says the source branch is checked out in pooled worktree <path> (holder <key>); release it if done. No auto-release. Out-of-pool worktrees shown as fact too. Uses the existing `advisory` field -> T0.
- **Coordinator/owner-confirmed security boundary (2026-11-16):** identity-only
  ordinary release applies to every non-top-level caller, including a
  lead-scoped native ticket worker. Only `isTopLevelLead()` retains the
  supplied-holder-key shortcut and `force`. A non-top-level caller may release
  its own held lease, a lease it acquired as parent, and a descendant's lease
  whose acquiring `parent_key` has a verified session-key Parent chain to that
  caller. Never siblings, ancestors, or unrelated callers. This preserves a
  ticket worker's ability to clean its own delegates' checkouts without
  restoring sibling authority. Rejected: treating all lead-scoped workers as
  root leads or treating knowledge of a holder key as authority.
- **Technical: descendant proof.** Follow existing ws session-key ancestry from
  the lease's acquiring `parent_key`, with cycle detection and refusal on an
  unreadable/missing chain before reaching the caller. Existing records make
  full ancestry cheap and checkable (`session_auth.go:556-579`), so the
  direct-child-only fallback is unnecessary. A direct matching holder/parent
  needs no inferred harness relationship. Pi token-spawned keys are currently
  parentless; do not infer ws ancestry from conversation topology or widen the
  Pi token contract. Rejected: scanning every session or inventing ownership
  when a lineage record is unavailable.
- **Compatibility: T0 against `e525b0738`.** Permission widening adds no tool,
  argument, or response member; authorization hardening implements the
  owner-approved root/identity boundary, not a version refusal. Existing root
  lead calls retain their behavior and arguments. Old servers/profiles may
  still refuse or omit newly available subagent calls: gate new use on
  capability detection or retain the lead-owned fallback. Both versions use
  existing `scope`/`parent` session fields (`session_auth.go:53-56,209-215`)
  and schema-1 lease fields (`worktree_lease.go:22-35,50-64,96-116`); the new
  writer records only already-known lead/delegate/leaf scope values and parent
  keys, and old readers can read them. No unknown field is added or lost by an
  old rewrite; reopening the new version reads the same ancestry/lease data
  and reapplies authorization. Merge writes the existing result's `advisory`
  (`git_merge.go:37-46,68-70`) through unchanged text/JSON dispatch
  (`server.go:1436-1442`); old consumers already accept that field. Verify exact
  source references and both directions in the implementation Results; the
  highest affected surface controls.

- **Technical: reuse existing identity metadata.** Root-lead authorization uses
  `sessionEntry.isTopLevelLead()` (lead scope and no parent), not `roleLead`
  alone. Acquire mints with the caller's existing scope and records the caller
  as parent; a leaf remains a leaf and a delegate remains a delegate. The
  existing `scope` and `parent` fields suffice; no new public identity field,
  token, scope, or lease schema is needed. Rejected: minting lead keys for
  delegates or adding a separate role hierarchy.
- **Technical: nested Pi checkout ownership.** Preserve Pi's existing cwd and
  launched-profile subset checks. A worker passes the acquired path as its
  nested child's cwd; the child's MCP key binds to that cwd while its profile
  remains within the parent's launched ceiling. The acquiring parent retains
  its lease-parent identity and releases the lease after its child is done.
  Rejected: widening a nested child's profile or treating a path argument as
  release authority.
- **Technical: merge advisory is observational.** Reuse the successful merge's
  source branch identity, Git worktree inventory, existing pool classification,
  and lease reader. Name a pooled checkout and its recorded holder when
  present; report an absent lease as unknown, not free. Preserve source
  cleanup diagnostics and merge success even if the observational read fails.
  Rejected: auto-release, refusing an otherwise valid merge, or inferring that
  a leaseholder is idle from the fact that its branch merged.

## Promotion Status

Parked in `todo/` at the owner's priority change on 2026-11-16. Planning is
captured; implementation has not started. Fact population, Route Facts, Sage
design/completeness review, ready promotion and implementation verification
remain outstanding. No review pass or implementation approval is implied by
this planning commit. Keep the assigned implementation worktree leased until
the coordinating lead resumes or disposes of this track.

On resumption, design review must explicitly verify the parentless Pi token-key
corner before ready promotion: `AgentKeyStore.Mint` and `Ensure` use delegate
scope (`agent_keys.go:28-29,50-51`), and ordinary Pi spawn uses that key mint,
not `MintLead` (`wsagent/runner.go:664-680`). The builtin Pi mini-lead is a
non-lead profile (`wsagent/builtin.go:170-179`); being parentless does not make
a delegate key satisfy `isTopLevelLead()` (`session_auth.go:30-32`). Verify
profile override/spawn ceilings cannot make a non-lead token conversation
parentless lead-scoped. If such a path exists, raise the security ambiguity
before promotion; do not infer user-facing identity from UI or conversation
topology.

## Constraints

- Convention: `ai-docs/manuals/ws-mcp.md` for `agents-plugin-tool/internal/mcp/`;
  retain compact text by default and equivalent explicit JSON results.
- Convention: `ai-docs/manuals/release-compatibility.md` for
  `agents-plugin-tool/internal/`; record directional, source-backed T0 against
  `e525b0738` and verify existing key/lease/advisory readers remain compatible.
- Convention: `ai-docs/manuals/shipped-surface-boundary.md` for all changed
  shipped code descriptions and guidance; emitted text is downstream-neutral.
- Convention: `ai-docs/manuals/skill-authoring.md` for changed skills,
  playbooks, prompts, or conventions; apply its invariant checklist to changed
  Constraints/Invariants lines and avoid duplicating schema-owned policy.
- Convention: `ai-docs/manuals/wsflow-mirroring.md` for canonical rsrc/skill
  changes; regenerate the manifest and byte-identical wsflow mirror, never
  hand-edit generated copies. Check product-mode output and applicable bundle
  guards. Include the Pi rsrc copy only where its existing packaging contract
  requires the changed source to be mirrored.
- Keep acquire/release's pool lock, hygiene, hooks, profiles, checkout facts,
  lease publication/removal order, and refusal-with-no-mutation behavior.
  Non-root `force: true` is refused before destructive work, even for the
  caller's own lease. No expansion of `worktree.remove` or `worktree.list`.
- Tool availability is not provisioning permission. An assigned serial ticket
  worker stays in its assigned checkout unless the lead/user authorized an
  additional tree. No new default acquisition or serial-to-parallel transition.
  The related serial-route bug is context, not an implementation prerequisite;
  this ticket preserves its contract rather than solving unrelated routing.
- A non-root caller with no valid lease identity may not release an unleased
  clean tree merely because the historical root-lead cleanup path allows it.
  Preserve root-lead clean-unleased and dirty-unleased guarded behavior.
- Existing tool arguments and response/storage shapes remain unchanged. Do not
  add force to acquire, auto-release any merge source, remove a checkout, or
  broaden Pi spawn depth/tier/profile authority.
- Verification uses scoped families on the configured remote Go worker only:
  no local Go test/build or test executable. Local static/Node/Python checks
  must not create test executables. Stop below the session's disk-space floor;
  use only the session-approved temporary location and clean temporary files.
- One passing scoped check closes each phase; do not run whole slow packages.
  A proven pre-existing slow test is excluded and recorded once as Forward.
  Independent R1, then fixes-only R2 only for substantive findings; minor/style
  findings become Forward. Do not replace the shared daemon, merge, or push.

## Prior Decisions

- `261116-feat-ws-worktree-release-guard`: preserve the serialized holder check,
  dirty-unleased refusal, non-forced refusal without mutation, and root-lead
  cleanup behavior; owner decisions here refine who may force or use a key.
- `260915-bug-ticket-worker-serial-route-acquires-extra-worktree`: preserve the
  lead/user-approved serial versus parallel provisioning contract; widening
  tool access does not independently authorize another checkout.

## Phases

### Phase 1: Enable scoped subagent leases

Allow acquire and release through the MCP role gate, handler authorization,
Pi `pi-worker` and `pi-mini-lead` allowlists, and any existing adapter exposure
needed for those profiles. Keep list/remove lead-only. Enforce root-only force
using existing top-level metadata, and mint acquired keys at the caller's
exact capability scope. Keep lease parent equal to the acquiring caller.
Ordinary non-root release requires the caller to be the lease holder, direct
acquiring parent, or a verified ancestor of that acquiring parent. `path` and
`key` locate a checkout but do not grant that authority. Use existing key
ancestry; missing, unreadable or cyclic ancestry fails closed without release
hooks, hygiene or lease removal.

Update tool descriptions and the applicable shared worktree/delegate guidance
for the approved capability, root-only force, serial-checkout constraint and
Pi cwd handoff. Reuse existing generic hooks and package mirror generators;
no new skills, profiles, or cross-module API are required. Native worktree
users use the acquired root/key; Pi nested workers use the acquired path as
cwd and the existing profile subset ceiling. The acquiring parent releases
only after nested work is done.

Acceptance:

1. Through actual MCP dispatch, native delegate/leaf callers can acquire and
   release. Missing/unknown session keys still refuse. Minted keys bind to the
   acquired root with the exact caller scope and caller parent, in memory and
   after persisted-key reload; using a minted key cannot bootstrap or reach
   list/remove, root-only force, or a stronger capability.
2. Holder self-release, direct acquiring-parent release and verified ancestor
   cleanup of a child's/grandchild's acquired lease succeed. A sibling,
   ancestor's child trying to release upward, or unrelated caller supplied the
   correct holder `key` or `path` still refuses without running release
   hooks/hygiene or removing the lease. Cover lead-scoped native workers as
   well as delegate/leaf callers, including reloaded persisted lineage.
   Missing/unreadable/cyclic ancestry and non-root unleased clean/dirty targets
   refuse; no harness-topology inference substitutes for proof. Other-holder
   and dirty-tree contents survive refusals.
3. `force: true` succeeds only for a parent-less lead. Delegate/leaf and
   parent-carrying lead callers are refused, even for their own lease. Include
   parentless Pi worker/mini-lead delegate keys: neither force nor the
   supplied-holder-key shortcut is granted merely because parent is empty.
   Existing root-lead key-based release, force, clean-unleased, dirty-unleased
   refusal, pool ownership and acquire/release-lock tests remain green.
4. Both Pi builtin profiles list acquire/release but not list/remove. Existing
   nested cwd/key binding and launched-profile ceiling remain enforced; add
   regression assertions at those boundaries rather than launching real
   harnesses or widening tools to make a fixture pass. Keep the assigned
   serial-worker guidance and route behavior without extra acquisition.
5. Run scoped MCP worktree/role tests and affected builtin/profile/cwd tests
   (for example MCP `-run 'Worktree|RoleAllowsTool'` and wsagent
   `-run 'BuiltinPi|TokenSpawnCwd'`, each `-count=1`); extend their selectors
   only to include newly named acceptance tests. Generate/verify changed
   rsrc mirrors with the documented scoped generators and fetch only changed
   artifacts back if generation is remote. Run applicable mirror/product-mode
   guards and `git diff --check`; record actual commands and outcomes.
6. Independent review covers privilege ceilings, ordinary release identity,
   root-only force, refusal-before-mutation and preserved serial behavior.
   Append Phase 1 Result and source-backed compatibility evidence.

### Phase 2: Advise on checked-out merge sources

Independent of Phase 1's implementation. After a successful `git.merge`, add
an advisory for any checkout holding the exact source branch, before returning
from either generic promotion or impl/goal cleanup paths. For a pooled source,
name its path and recorded holder (or unknown lease) and suggest releasing it
only if work is done. An out-of-pool source is a fact only: never suggest pool
release. No checked-out source means no new nudge. Keep existing dirty-worktree,
cleanup-failure, size and release-boundary diagnostics rather than replacing
them. Advisory reads are not permission to mutate a source checkout and do not
convert an already-landed merge into a failure.

Acceptance:

1. Successful merges from a pooled held source report path and holder; unknown
   lease does not imply unused. Checkout files, branch, lease and hook state
   stay untouched. Include a generic source branch as well as impl/goal source
   cleanup so the existing early return cannot hide the new advisory.
2. Out-of-pool checked-out sources are named without pool-release instructions;
   no checked-out source emits no new nudge. A failed/conflicted merge emits
   no success-only nudge. Observational read failure cannot misreport a landed
   merge as failed or erase existing diagnostics.
3. MCP text and explicit JSON carry the same existing `advisory` field; retain
   merge success, cleanup semantics, existing diagnostics and branch-deletion
   behavior. No new response member or persistent field.
4. Run the affected merge family only, for example
   `go test ./internal/mcp -run 'ImplMerge|GenericMerge' -count=1` on the
   configured remote worker; record the passing scoped command and
   `git diff --check`. Independent R1 covers source classification, advisory
   parity and absence of checkout mutation; fixes-only R2 only if substantive.
   Append Phase 2 Result and close the ticket after both phases are complete.
