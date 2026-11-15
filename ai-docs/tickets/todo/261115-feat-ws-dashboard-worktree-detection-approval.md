---
title: "Dashboard session rail worktree detection and user approval"
related:
  261113-feat-ws-dashboard-session-rail-tree: "extends the landed session-rail worktree listing"
  261115-research-ws-effects-browser-unexpected-codex-child: "adjacent fixture-isolation evidence; no implementation dependency"
---

# Dashboard session rail worktree detection and user approval

## Background

The session rail already lists repository worktrees and excludes ws pool entries through `railPooled` and `worktreePoolRoots` (`agents-plugin-tool/internal/wsdashboard/pageview_rail.go:28-30, 292-300, 674-680`; pool exclusion is already landed and remains unchanged). The owner-approved follow-up is to make non-pool worktree discovery explicit and reviewable without turning the dashboard into a shell-prevention mechanism.

Owner decisions `(847)=a` and `(856)=ok`, relayed by MAIN, settle the visible behavior below. The approved reference is tracked at [the HTML mockup](../assets/261115-feat-ws-dashboard-worktree-detection-approval/847-worktree-approval.html), with the four accompanying PNG captures in the same folder. Future verification must compare a result capture side by side with that HTML reference.

## Decisions

- The compatibility target is **T0 additive**. Add one optional admission field to the existing daemon-owned `rail-layout.json` document while keeping `schema_version: 1`; its read-modify-write path preserves unknown fields (`agents-plugin-tool/internal/wsdashboard/rail_layout.go:1-53, 342-345, 387-428`).
- A successful `/rail/worktrees/add` records the canonical repository-root/worktree-path identity with `origin: user` and admitted state (`agents-plugin-tool/internal/wsdashboard/workspace_routes.go:346-392`; `railworktreeadd.go:159-232`).
- A worktree found by Git that has no admission record is `origin: detected` and pending. The repository root is implicitly admitted without a record. Pool filtering runs first through the existing `railPooled`/`worktreePoolRoots` rule, so pooled paths never become pending (`agents-plugin-tool/internal/wsdashboard/pageview_rail.go:292-300, 674-680`; `railworktreedelete.go:212-230`).
- Admission identity is the pair of canonical repository-root path and canonical worktree path.
- Apply uses the existing same-origin dashboard write gate, revalidates the latest rail facts, writes persistent admission state, and wakes the existing rail generation/render path (`agents-plugin-tool/internal/wsdashboard/rail_layout_route.go:70-120`; `rail.go:37-120`).
- If a selected path was removed or its identity was replaced before Apply, Apply rejects that entry. Apply never deletes, removes, or otherwise alters a worktree on disk.
- Pending entries have approve/reject selection toggles; any selection reveals the Apply capsule. This does not settle toggle clearing or mutual exclusion.
- Selected approvals persist across refresh. User-added worktrees remain admitted; unrecorded Git discoveries remain pending until Apply admits them.
- MAIN owner settlement `(888)=a` confirms that a removed and later recreated same path is a new detected/pending worktree; prune the admission record once the old path is confirmed gone.
- Apply admits approved entries and moves rejected entries into a collapsed rejected-worktree list.
- When rejected entries are applied, the dashboard copies the rejected-worktree list together with a prompt asking the agent to investigate unauthorized non-pool creation and report the finding to the user. Rejection does not delete or otherwise alter the worktree on disk.
- Detection and remediation are the scope; shell prevention or interception of worktree creation is not.

## Constraints

- Preserve the current rail's read-only worktree listing boundary: it reads repository worktree records and does not run a working-tree status read for every listed worktree (`agents-plugin-tool/internal/wsdashboard/railworktrees.go:7-17`; `pageview_rail.go:94-111`).
- Preserve pool-first filtering and root implicit admission even when the repository has no active session-backed pool source; the current helper derives configured, default, and legacy pool roots (`agents-plugin-tool/internal/wsdashboard/railworktreedelete.go:212-230`).
- Preserve canonical root/worktree identity and distinguish `origin: user` from `origin: detected`.
- The mockup's same-toggle clearing and mutual-exclusion behavior is visual reference only, not normative contract. Report any remaining owner-visible UI choice to the lead without silently resolving or minting a local decision ID.
- Keep this ticket in `todo/`; no ready promotion, fact population, or Sage review is part of this capture.

## Phases

### Phase 1: Detect, review, and remediate non-pool worktrees

Implement the settled T0 admission contract above while preserving root admission, canonical identity, and pool-first exclusion. Record dashboard-created additions as user/admitted, classify unrecorded Git discoveries as detected/pending, persist Apply outcomes, reject removed or replaced entries without touching disk, and wake the existing rail generation. Preserve rejected entries as a collapsed list plus the investigation prompt. A removed and later recreated same path is new detected/pending state; prune the admission record once the old path is confirmed gone. Do not prevent worktree creation at the shell or Git layer.

Focused verification must cover:

- pure rail classification for root, user/admitted, detected/pending, and pool-filtered worktrees using canonical root/worktree identity;
- schema-1 admission-field round-trip, unknown-field preservation, and persistence across refresh;
- reappearance and pruning: after a recorded path is confirmed gone, recreating the same path is new detected/pending state and the old admission record is pruned; do not invent or assert a timing mechanism;
- successful `/rail/worktrees/add` recording `origin: user` and admitted state;
- same-origin Apply revalidation against latest rail facts, persistent admitted/rejected writes, existing generation wake, and rejection of removed/replaced paths without deletion;
- rejected-list prompt contents and the no-delete guarantee;
- regression coverage that pool entries remain hidden before admission classification through `railPooled`/`worktreePoolRoots`;
- the rail's existing worktree-list cadence and no-per-worktree-status-read boundary; and
- a browser result capture compared side by side with `assets/261115-feat-ws-dashboard-worktree-detection-approval/847-worktree-approval.html` in pending and applied states. The toggled capture may be compared as visual reference only; mockup-only toggle clearing and mutual exclusion are not normative until separately settled.
