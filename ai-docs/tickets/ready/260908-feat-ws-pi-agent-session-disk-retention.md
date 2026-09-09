---
title: "Pi adapter: bound subagent session files on disk (evict to disk, prune by age, drop empty spawn dirs)"
spec:
  - pi-adapter-runtime
  - 260909-pi-durable-child-session-homes
plans:
  phase-1: ai-docs/.plans/2026-09/09-2237-260908-feat-ws-pi-agent-session-disk-retention-phase1.md
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260905-feat-ws-pi-agent-alias-park-and-registry-cap: owns the in-memory registry cap and `evictForCapacity`
  260908-feat-ws-pi-subagent-audit-window-and-owner-steering: reads `record.sessionPath` for history; a pruned record must read as gone, not as a parse error
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: ca46ef187b7d1bd7
sage-review-completeness-reviewed: ca46ef187b7d1bd7
---

# Pi adapter: bound subagent session files on disk (evict to disk, prune by age, drop empty spawn dirs)

## Background

Owner ask (2026-09-08): a subagent that has not been referenced for a long
time, or that falls off the registry cap, should be deleted from disk as
well, not only forgotten in memory.

What the adapter does today (`agents-plugin-pi/src/spawner.ts`,
`agent-sidecar.ts`, verified on the 2026-09-08 tree):

- **The registry is an in-memory `Map`.** The cap (`WS_PI_AGENT_REGISTRY_CAP`,
  default 256) bounds only that map, and only lazily: `evictForCapacity`
  runs at spawn time when `size >= cap`, removes the dormant
  (`!client && !running && !threadBound`) record with the oldest
  `max(lastLeadPromptAt, last reportLog entry, lastReportAtOverride)`, and
  by design "only forgets the registry entry (never touches the evicted
  agent's on-disk session file)". The only other removal is the explore
  leaf's self-reap after its first harvest. Stop and automatic park keep
  the record.
- **Dormant records do not outlive the lead session.** The shutdown sidecar
  (`<lead session>.ws-agents.json`) persists every non-`threadBound`
  record and is deleted on the next `session_start` of the same lead
  session file. A fresh lead session starts with an empty registry, so the
  cap never sees the previous sessions' children at all; "256" is not a
  disk bound in any sense.
- **Every spawn creates `mkdtempSync(join(tmpdir(), "ws-pi-agent-"))`
  unconditionally** (`spawnAgent`, before the fork/noSession decision) and
  sets `sessionPath = <dir>/session.jsonl`. Nothing ever removes that
  directory. Persistent workers write their session there. Explore leaves
  run `--no-session`, and forks are launched with `--fork` so
  `record.sessionPath` is reassigned to the child's real file
  (`record.sessionPath = state.sessionFile`, which Pi places in its own
  `~/.pi/agent/sessions/<cwd>/` next to the lead's sessions); for both, the
  mkdtemp directory stays behind empty (it is still used as
  `WS_PI_APPROVAL_DIR`).
- **Observed on the owner machine (2026-09-08):**

  | item | value |
  |---|---|
  | `/tmp/ws-pi-agent-*` directories | 542 |
  | of which without any `session.jsonl` | 493 |
  | total size | 45M |
  | oldest present | 2026-09-07 (earlier ones removed by OS tmp policy, not by the adapter) |
  | `/tmp/ws-pi-ask-test-*` left by the test suite | 1224 dirs, 9.4M |

  Fork session files (`--fork` copies) sit in `~/.pi/agent/sessions/<cwd>/`
  interleaved with lead session files, indistinguishable by name.

So the current bound is whatever the OS applies to `tmpdir()`, which is
accidental (WSL `/tmp` survives restarts of the shell; a long-lived host
keeps everything).

## Decisions

Owner confirmed the full proposed policy on 2026-09-09:

- Store children beneath `<Pi agent dir>/ws-agents/<lead session id>/<agent id>/`,
  outside Pi's ordinary session listing. Resolve the configured Pi agent dir;
  do not hard-code `~/.pi/agent`. Workers, fork copies and approval scratch
  material share this owned home; no-session explores need only scratch material.
  Preserve the lead-keyed identity across reopen and sidecar revival.
- Default retention is **30 days since last activity**, configurable and
  disableable. Disabling age pruning does not disable explicit cap eviction.
  References/resume, prompts, reports and session writes refresh activity;
  persist enough ownership/activity metadata for cross-lead-session pruning.
  Directory creation time alone is not activity. Unknown or unreadable ownership,
  activity or liveness must be retained rather than guessed stale.
- Never prune running children, open question or approval waits, or owner-held
  children. Apply the same protection to cap eviction and sidecar-orphan cleanup.
  Another live lead session's protected children remain protected; a new lead
  must not infer inactivity merely from absence in its own registry.
- Cap eviction removes the evicted child's owned disk material as well as its
  registry entry. Remove no-session scratch homes after their last consumer ends,
  including pending approval consumers. Do not delete an active fork's approval
  directory merely because fork readiness has completed.
- Prune eligible stale children across lead sessions at session start. Delete
  an entire lead subtree only if every contained child is eligible; disappearance
  of the lead file alone never overrides TTL or liveness protection.
- Existing legacy paths remain readable/resumable. Delete legacy material only
  when recorded ownership proves that exact path belongs to a child. Prefix-only
  temp scans and scans of Pi's normal sessions directory are not authorization.
  Do not mass-migrate unknown historical files.

## Constraints

- Never delete the lead session, unrelated Pi sessions, or arbitrary directories.
  Canonical path containment and symlink handling must prevent deletion outside
  an authorized owned home. Recheck eligibility immediately before removal.
- Deletion is best effort: failures surface through existing diagnostics without
  failing spawn or session start. Retain enough metadata to retry safely.
- Missing history is an explicit unavailable/gone state in `/audit`, not a parse
  failure. Owner-held protection integrates when the audit steering state lands;
  it is not a prerequisite for relocating existing workers and forks.
- The historical measurements above are dated evidence, not current machine
  counts. Test-fixture leaks are outside this runtime ticket.

## Spec Impact

Update `pi-adapter-runtime` session lifecycle coverage with the durable root,
legacy compatibility, 30-day configurable retention, shared exclusion rules,
cap eviction deletion and unavailable-history behavior. No live spec behavior
is claimed implemented by this ticket preparation.

## Phases

### Phase 1: Relocate child sessions to a durable owned home

Create new worker, fork and explore homes under the confirmed root, preserving
fork ancestry, approval access, recorded session paths, dormant resume and
sidecar revival. Persist ownership/activity and liveness information needed for
later safe cleanup. Legacy recorded paths remain usable.
Fork launch must pass `--session-dir <owned-agent-home>` and validate that the
reported fork session file stays within that home; current `--fork` alone uses
Pi's ordinary session location. Preserve the fork copy and ancestry semantics.

Verification: temporary-directory integration tests for all spawn kinds,
configured agent-dir override, fork copy and ancestry, approval lifetime,
reopen/resume and sidecar recovery. Owner live check: new children do not enter
Pi's ordinary resume list or create legacy `ws-pi-agent-*` temp homes.

### Result (5b525cfe) - 2026-09-09

Phase 1 implementation checkpoint, through `15c2fa89`; owner live acceptance
remains pending. The researched plan resolved storage identity to the immediate
dispatching Pi session's SDK identity, including no-session dispatchers, and
included the separate terminal collector and owner-thread recovery paths.
No disk deletion, TTL configuration, cross-session scan, or audit steering was
introduced. Phases 2 and 3 remain unimplemented.

Review dispositions:

- Correctness C1 [fixed]: complete canonical session-path validation, including
  terminal traversal, directories, symlinks, and invalid readiness without
  state mutation. Review 3 retained C1; the required elevated implementer fixed
  it in `15c2fa89` with targeted regressions. No fourth review was scheduled.
- Correctness C2 [fixed]: recovery binds descriptors to complete on-disk
  ownership identity; Critical re-review resolved it.
- Correctness C3 [fixed]: commit fork readiness paths only after validation;
  Critical re-review resolved it.
- Correctness C4 [fixed]: preserve question and approval protection across
  transitions and decision consumption; Critical re-review resolved it.
- Correctness C5 [fixed]: failed stops and stale launch generations cannot
  fabricate stopped state; Critical re-review resolved it.
- Correctness C6 [fixed]: later metadata failures are nonfatal and diagnostic.
  Re-review resolved the runtime-breaking concern and downgraded its remaining
  diagnostic concern to Important; final implementation verification covers
  pending first writes versus actual observation failures.
- Correctness I1/I2/I3 [fixed], implementer self-reports: all dispatching roles
  recover registries; durable process/protection facts are validated; activity
  boundaries and observer lifecycle are retained. These Important dispositions
  are not independent re-review verdicts.
- Test T1/T2/T3 [fixed]: configured-root dispatch integration, valid/invalid
  recovery and resume through both registries, and temporary fixture cleanup.
  The terminal collector test exercises real process launch/harvest with a
  disposable fake Pi executable and no model call. Fit review was clean.

Verification at `15c2fa89`: 559 focused tests passed. Full suite: 1,417 tests,
1,287 passed and 130 existing baseline failures (129 Linux-specific SDK fixture
failures on macOS and one stale `ws-ask` surface expectation). No unexpected
observer diagnostic lines; `git diff --check` passed. `ws-ask` remains hidden.

An isolated installed Pi 0.85.1 RPC probe verified worker session targeting and
fork `--session-dir`, physical fork history and parent ancestry outside the
ordinary session directory. It used `--no-extensions` and no model prompt:
this verifies SDK/CLI compatibility, not the adapter's live owner acceptance.
The worker did not flush a physical transcript before its first assistant turn.
Probe roots and processes were removed. An initial SDK signature probe created
one synthetic file in the owner's default session directory; that exact new
file was immediately removed, without changing pre-existing history.

### Phase 2: Clean up scratch homes and cap-evicted sessions

Build on Phase 1 metadata to remove unused scratch homes and safely remove
cap-evicted or deliberately discarded sidecar children. Use the common eligibility
and ownership checks; all protected states survive both memory and disk eviction.

Verification: eligible worker/fork deletion, explore cleanup after approval use,
protected-state exclusion, canonical-path/symlink escapes, known legacy paths,
unrelated files untouched, and deletion failure without spawn failure.

### Phase 3: Prune stale children across lead sessions

Apply the 30-day last-activity default at session start over recorded owned homes,
with a documented adapter configuration to change or disable TTL. Safely handle
concurrent live leads, recent activity, missing metadata and partial deletion.

Verification: fake-clock boundary tests, disabled/overridden TTL, activity refresh,
protected children in another live lead, mixed-age subtrees, unknown legacy homes,
missing lead files, missing audit history, and permission failures. Owner live
check uses disposable owned fixtures with a short TTL; production history is not
needed to verify deletion.

## Blocked (2026-09-09)

Awaiting the Phase 1 owner live adapter check: launch fresh worker, fork, and
explore children through the actual extension, confirm their configured owned
homes and fork ancestry, and confirm they neither enter Pi's ordinary resume
list nor allocate legacy `ws-pi-agent-*` homes. The extension-disabled CLI probe
does not satisfy this gate. Record acceptance before advancing to Phase 2;
the ticket stays open in `ready/` and autonomous selection should skip it while
this condition remains outstanding. No production-history deletion is needed.
