# Plan: 260714-refactor-dashboard-active-root-atomic-select-pure-derivation — Phase 3: Cleanup + optional state promotion + doc/spec touch

## Relevant Ticket Contract

- Bullet 1: "Remove now-dead code: the duplicated mount logic, any unused
  branch of `resolveEffectiveActiveRootKey`, and stale comments referencing
  the deleted refs across `App.tsx` / `openRootLookup.ts`."
- Bullet 2 (OPTIONAL, reviewer's/executor's call, D5): "promote
  `lastNonNullResourcesByServer` from a render-time write-through ref to
  explicit state, removing the last render-time mutation smell (behaviorally
  identical; D5)."
- Bullet 3: "Update the keep-alive spec note under
  `{#260714-ws-dashboard-cross-server-workbench-keepalive}` in
  `ai-docs/spec/ws-web-dashboard/index.md` only if the derivation's
  caller-visible contract changed (expected: no new contract...). Fold the
  fragility record's disposition back into `260714-idea-...-fragility` (leave
  that idea ticket as the standing evidence log; do not delete it)."
- Constraint: "SCOPE-BOUNDED to the derivation/selection cluster: `App.tsx`
  selection handlers + `WorkbenchShell` active-root derivation,
  `workbench/openRootLookup.ts`, and the pure helpers in `resourceModel.ts`."
- Migration note: "Phase 3 is pure cleanup" / "Phases 2-3 ... revert
  independently without reopening any failure mode."
- Verification boundary (same as Phase 1/2): `npm run build`,
  `npm run test:workbench`, `test:resource-model`, `test:commands`,
  `test:open-work-root`.

## Out of Scope

- Any rewrite of workbench rendering, Dockview integration, pane lifecycle,
  or the resource-fetch layer (ticket-level Constraints).
- The unrelated `260714-*` terminal-visibility/socket-lifecycle "Phase 2
  Prong 2" comments at `App.tsx:8546`/`8590` and the agent-chat
  "Phase 3" comments belonging to a different ticket
  (`260711-feat-ws-dashboard-agent-activity-chat-ui`) — different cluster,
  different ticket, not this ticket's Phase 3.
- The sticky-selection machinery (`driveStickyWorkbenchSelection`,
  `resolveStickyWorkbenchSelection`, `resolveWorkbenchSelectionWithMatch`)
  added by the separate `260714-bug-linked-terminal-ws-relay-502` ticket
  (already merged onto this branch via `goal/drain-ready-queue`) — it is
  functioning production code with real callers, not dead code left over
  from this ticket's Phase 1/2, and is not named by this ticket's scope.
- Moving/closing `260714-idea-dashboard-workbench-active-root-derivation-fragility.md`
  out of `idea/` — ticket explicitly says leave it as the standing evidence
  log.

## Codebase Findings

- `ws-dashboard/frontend/src/workbench/openRootLookup.ts:225-232` —
  `resolveEffectiveActiveRootKey` does not exist as a function anymore
  anywhere in the repo (repo-wide grep: zero hits besides this one comment).
  It and the `lastActiveRootKey*`/`lastActiveRootServerId*` refs were already
  fully deleted in Phase 1 (confirmed by Phase 1's own Result note and by
  direct grep — no `lastActiveRootKeyRef`/`lastActiveRootServerIdRef` symbol
  exists in `App.tsx` or `openRootLookup.ts`). The one remaining comment
  reference is historically accurate ("deleted with its `lastActiveRootKey*`
  refs"), not stale/misleading. **Bullet 1's "unused branch of
  `resolveEffectiveActiveRootKey`" target is already fully resolved — nothing
  to delete.**
- `App.tsx:1072-1083` (`resource.select` command handler) and
  `App.tsx:688-716`, `718-735`, `737-745`, `747-767`
  (`handleWorkRootOpened`, the `resourcesByServer` normalize effect,
  `handleServerSelected`, `applyServerConnection`) — all six selection call
  sites (plus the `server.off` handler at `App.tsx:1250`) already route
  through `selectRoot(...)` per Phase 2's landed result; grepped
  `setSelectedServerId`/`setSelectedId`/`selectedServerIdRef.current =` shows
  no remaining duplicated triple-setting code outside the single
  `selectRoot` definition (`App.tsx:662-667`) and one unrelated
  `useEffect` that mirrors `selectedServerId` into a ref (`App.tsx:646-648`,
  not part of the triple). The two other bare `setSelectedId` call sites
  (`App.tsx:800`, `806` — the entity-reconcile effect; `App.tsx:1425` — git
  worktree creation) are same-server, `selectedId`-only updates never named
  by the ticket's call-site list and out of the "selectedServerId triple"
  concern entirely. **Bullet 1's "duplicated mount logic" target is already
  fully resolved by Phase 2 — nothing to delete.**
- Repo-wide grep for `260714` across `App.tsx`, `openRootLookup.ts`,
  `resourceModel.ts`, `layoutRestore.ts` — every comment referencing the
  deleted refs (e.g. `App.tsx:1211-1221`, `App.tsx:6024-6030`) correctly
  describes the current, already-cleaned-up state; none are stale.
  **No comment cleanup needed.**
- `resourceModel.ts:590-602` — `resolveWorkbenchSelection` (the plain,
  non-match wrapper) has no production caller anymore (only
  `resolveWorkbenchSelectionWithMatch` and the sticky driver are used in
  `App.tsx`/`openRootLookup.ts`). **This looked like a dead-code candidate
  but is NOT**: `workbench/deriveWorkbenchView.test.ts:1-133` deliberately
  imports and uses it as `plainSelectionFor(...)`, an explicit test-only
  stand-in mirroring "what a caller with no sticky/transient-omission
  concern would compute" (see the doc comment at
  `resourceModel.ts:526-532` and the test file's own comment at
  `deriveWorkbenchView.test.ts:116-120`). Removing it would break
  `test:workbench`. Leave as-is.
- `App.tsx:563-573` — `lastNonNullResourcesByServerRef` (a `useRef`) is
  written unconditionally every render via `withLastNonNullResourcesByServer`
  (a render-time mutation) immediately before `resolveActiveResources` reads
  it; also threaded to `WorkbenchShell` as a prop (`App.tsx:1449`,
  `App.tsx:3628/3660`) and cleared on server-off (`App.tsx:1225-1228`). This
  is the D5 cache the optional bullet 2 targets.
- `ai-docs/spec/ws-web-dashboard/index.md:155-196`
  (`{#260714-ws-dashboard-cross-server-workbench-keepalive}`, anchor at line
  179) — the section is a pure black-box behavioral contract (per-server
  On/Off, hide-not-unmount keep-alive, single-active right-side workbench,
  refetch-on-reselect); it names no implementation detail (no ref, no
  derivation function, no "watermark"/"flash" wording) that this refactor
  changed. The refactor's own stated goal across all three phases is
  preserving this exact contract more robustly, not changing it. **No spec
  edit needed** — matches the ticket's own "expected: no new contract".
- `ai-docs/tickets/idea/260714-idea-dashboard-workbench-active-root-derivation-fragility.md`
  — freeform idea ticket (no `## Phases`), currently ends at "## Not
  actionable yet (TBD)" with no closing/disposition section. Precedent for a
  `## Disposition` section on a ticket that stays in its current directory
  exists elsewhere (e.g. `ai-docs/tickets/.done/260429-research-host-neutral-ws-plugin.md`
  uses the same heading, though that one also moved directories — this
  ticket must NOT move, per explicit instruction).
- `ws-dashboard/frontend/package.json` — `test:workbench` already runs
  `workbenchModel.test.js`, `openRootLookup.test.js`,
  `deriveWorkbenchView.test.js`, `layoutRestore.test.js`; `test:resource-model`,
  `test:commands`, `test:open-work-root` all present and match the ticket's
  verification boundary. No script wiring changes needed.

## Implementation Plan

1. **Bullet 1 (dead-code sweep) — verification pass, expect no edits.** Grep
   `App.tsx` and `workbench/openRootLookup.ts` for `lastActiveRootKey`,
   `lastActiveRootServerId`, `resolveEffectiveActiveRootKey` to reconfirm
   zero live symbols (only the one historical comment at
   `openRootLookup.ts:229` remains, which is accurate — leave it). Reconfirm
   all `setSelectedServerId`/`setSelectedId` call sites are either inside
   `selectRoot` (`App.tsx:662-667`) or are legitimately out-of-triple
   (`App.tsx:800`, `806`, `1425`, the `selectedServerId` mirror effect at
   `646-648`). If this matches the survey (it should), make **no changes**
   for this bullet — do not invent cleanup busywork. If the executor's own
   pass turns up something the survey missed, fix it surgically and note it
   in the plan/commit.
2. **Bullet 2 (OPTIONAL D5 state promotion) — executor's call.** If chosen:
   replace `const lastNonNullResourcesByServerRef = useRef<ResourcesByServer>({})`
   (`App.tsx:563`) with `useState<ResourcesByServer>({})`, and replace the
   unconditional render-time write
   (`lastNonNullResourcesByServerRef.current = withLastNonNullResourcesByServer(...)`,
   `App.tsx:564-568`) with the React-sanctioned "adjusting state during
   render" pattern: compute `nextLastNonNull` synchronously, compare by
   reference to the current state value, and call the setter only when it
   differs (avoids an infinite render loop; `withLastNonNullResourcesByServer`
   already returns the same reference on a no-op update per its existing
   contract — confirm this in `resourceModel.ts` before relying on it).
   Update the one read site (`App.tsx:572`), the `WorkbenchShell` prop
   threading (`App.tsx:1449`, `3628`, `3660`), and the server-off clear
   (`App.tsx:1225-1228`, becomes a setter call) accordingly. Behaviorally
   identical — no test assertions should need to change since
   `test:workbench`/`test:resource-model` exercise the pure functions, not
   the ref/state wrapper. If the executor judges this not worth the diff in
   a "low-risk cleanup pass," it is equally acceptable to skip it entirely
   and say so in the result note — the ticket explicitly leaves this
   optional.
3. **Bullet 3a (spec) — no edit.** Do not touch
   `ai-docs/spec/ws-web-dashboard/index.md` — the survey confirms the
   caller-visible contract at
   `{#260714-ws-dashboard-cross-server-workbench-keepalive}` (lines 155-196)
   is unchanged. Record this "verified, no edit" finding in the phase
   Result note so a future reader does not think it was overlooked.
4. **Bullet 3b (fold fragility disposition back)** — append a
   `## Disposition` section to the END of
   `ai-docs/tickets/idea/260714-idea-dashboard-workbench-active-root-derivation-fragility.md`
   (do not move the file out of `idea/`, do not delete it). Summarize: this
   idea ticket's structural-fragility diagnosis was answered by
   `260714-refactor-dashboard-active-root-atomic-select-pure-derivation`
   (Phases 1-3); reference the traceability table in that ticket
   (failure-mode -> structural-property mapping) rather than restating it;
   note the ticket intentionally stays open in `idea/` as the standing
   evidence log per its own instruction, not because the fragility is
   unresolved.
5. Commit per the repo's commit-rules (one logical unit; `## AI Context`
   explaining the "already clean from Phase 1/2" finding for bullet 1 and
   the go/no-go decision on bullet 2). Since this branch already contains
   Phase 1/2 commits plus the unrelated relay-502 sticky-selection commits
   (verified via `git log --oneline`), keep the Phase 3 commit scoped only
   to what this plan's bullets actually touch — do not re-touch
   relay-502/sticky-selection code.

## Verification Plan

- `npm run build` (from `ws-dashboard/frontend`) — must stay clean.
- `npm run test:workbench` — must stay green; if bullet 2 is done, re-run to
  confirm no regression (expected: unaffected, since these are pure-function
  tests).
- `npm run test:resource-model`, `npm run test:commands`,
  `npm run test:open-work-root` — must stay green (ticket's verification
  boundary).
- If bullet 1 finds and fixes anything the survey missed: re-grep afterward
  to confirm zero remaining `lastActiveRootKey*`/`resolveEffectiveActiveRootKey`
  symbols.
- Manual/live-dogfooding confirmation is NOT required for Phase 3 (D6 LIMIT
  applies to Phases 1-2's correctness properties; Phase 3 is non-behavioral
  cleanup + optional refactor + docs).

## Escalations

- None.
