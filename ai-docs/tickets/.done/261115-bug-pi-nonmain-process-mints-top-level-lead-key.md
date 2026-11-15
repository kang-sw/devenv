---
title: "Pi non-main processes mint top-level lead keys and appear as dashboard root rows"
related:
  261114-bug-ws-fork-parent-identity-loss: "superseded: this ticket fixes the parentless fork mint that ticket captured"
  261114-bug-pi-fork-descendant-key-authority-rejected: "sibling, out of scope: bridge refuses keys a fork minted itself; separate authority question"
  261114-bug-ws-pi-fork-acquired-worktree-key-authority: "sibling, out of scope: bridge refuses a worktree key a fork acquired; separate authority question"
  261115-research-ws-pi-fork-acquired-worktree-key-authority-gap: "sibling, out of scope: research on the same acquired-key refusal"
  261007-bug-pi-fork-lineage-keeps-bootstrap-key-after-adoption: "neighbour, kept separate: forks name the abandoned bootstrap key as parent after adoption"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 9ab4663e26f2a0ce
sage-review-completeness-reviewed: 9ab4663e26f2a0ce
completed: 2026-11-15
---

# Pi non-main processes mint top-level lead keys and appear as dashboard root rows

## Background

A Pi process with a recognized role marker but no delegation policy takes
the parentless lead-scope bootstrap path; when dashboard registration is
enabled, its first keyed call can register a top-level root row
(`agents-plugin-pi/src/bridge.ts#L1007-L1016`, `agents-plugin-pi/src/bridge.ts#L1231-L1239`,
`agents-plugin-tool/internal/mcp/server.go#L766-L769`,
`agents-plugin-tool/internal/mcp/dashboard_sessions.go#L152-L183`). Observed 2026-11-15 04:15Z: two headless
`pi -p` workers launched with `WS_PI_SPAWN_ROLE=worker` produced root rows
(keys `morbidity-unskilled-bruising`, `shortlist-jumbo-trapper`); about 70
historical rows have the same shape.

Evidence (source verified on develop at 844a909ec; historical observations above were not independently reproduced):

- `agents-plugin-pi/src/bridge.ts:1007`: `ownsLeadKey` is false when a role
  marker is set, so `harnessReport` is `{}`.
- `bridge.ts:1239-1243`: with a role marker and no policy, bootstrap calls `ferrule` with
  only `root` and `format` - no `capability`, no `parent_session_key`.
- `agents-plugin-tool/internal/mcp/server.go:2628-2637`
  (`parseCapabilityScope`): an absent capability is `roleLead`;
  `agents-plugin-tool/internal/mcp/server.go#L2392-L2400` validates an optional
  parent and mints the key with the supplied parent string (empty when omitted).
  `session_auth.go:30-31` (`isTopLevelLead`) is lead scope with no parent.
- `agents-plugin-tool/internal/mcp/server.go#L766-L769`: a known top-level
  lead's keyed call runs `dashboardTouch`; registration remains conditional
  on dashboard enablement (`agents-plugin-tool/internal/mcp/dashboard_sessions.go#L152-L183`).
  Calls that omit `session_key` are filled by `resolveSessionKey`, not
  `normalizeSessionKey` (`agents-plugin-pi/src/bridge.ts#L886-L899`,
  `agents-plugin-pi/src/bridge.ts#L1112-L1122`), so an ordinary omitted-key
  call can register the bootstrap key's row.
- Forks: `fork.ts:233` passes only the legacy `parentSessionKey` and no
  `parentPolicy`, so `resolveSpawnAdmission` (`spawner.ts:3718`) falls back
  to `callerDelegationPolicy`, which carries no `sessionKey`. `childPolicy`
  (`delegation-policy.ts:108`) therefore sets no `parentSessionKey`, and the
  child bridge mints a parentless key at `bridge.ts:1239`. Normal spawn and
  explore children pass `leadParentPolicy(bridge)` (`spawner.ts:3693`) and are
  parent-bound. The discussion fork is a separate `spawnAgent` call, also
  without `parentPolicy` (`agents-plugin-pi/src/ask.ts#L1540-L1562`), not a
  caller of `buildForkSpawnCtx`. Admission currently defaults forks to lead
  authority unless a profile overrides it; adding `parentPolicy` alone does
  not select delegate authority (`agents-plugin-pi/src/spawner.ts#L3717-L3741`).
- The server already supports a parentless delegate or leaf mint ("a child
  launched without its lead's key"; comment at
  `agents-plugin-tool/internal/mcp/server.go#L2401-L2406`). Such a
  key is not a top-level lead, never rebinds the mailbox owner, and never
  registers a dashboard row.
- `WS_PI_PARENT_SESSION_KEY` is already defined
  (`agents-plugin-pi/src/process-role.ts#L74-L79`). The RPC environment
  populates it only for forks and clears it for other roles
  (`agents-plugin-pi/src/spawner.ts#L2913-L2922`). The bridge reads it in
  normalization/refusal handling (`agents-plugin-pi/src/bridge.ts#L1104-L1119`),
  whose refusal predicate is fork-only (`agents-plugin-pi/src/bridge.ts#L750-L756`);
  bootstrap does not currently read it (`agents-plugin-pi/src/bridge.ts#L1234-L1239`).

## Decisions

- **A Pi process with a role marker and no delegation policy binds to its
  parent when one is supplied.** When `WS_PI_PARENT_SESSION_KEY` is set (any
  role, not only fork), the bootstrap passes it as `parent_session_key` and
  keeps lead capability: a parent-bound lead key, the same shape a
  spawner-launched worker already gets, which keeps the lead-only
  `workflow_manual` and `workflow_state` the `ticket-worker` route and fork
  flows call. A parent-bound key is not a top-level lead, so it never
  registers a dashboard row nor rebinds the mailbox owner. Source: owner
  (924)=a via MAIN, as amended by MAIN 04:41Z.
  Rejected: delegate capability for every non-main process - `ticket-worker`
  requires lead-only `workflow_manual` and forks call `workflow_state`, so a
  delegate key would break both routes. Rejected: changing ferrule's
  server-side default for a missing capability - an API semantic change
  (owner (924)=a, house rule).
- **Without a parent, or with a parent the server refuses as unknown, keep
  today's behavior and warn.** No `WS_PI_PARENT_SESSION_KEY`: the bootstrap
  mints exactly as today (no capability, no parent: a top-level lead key) and
  emits a one-time warning telling the operator to set
  `WS_PI_PARENT_SESSION_KEY` to the launching lead's key. An unknown-parent
  refusal: retry once with that same legacy call and emit the same one-time
  warning. Source: MAIN 04:41Z change to the (924) decision note.
  Rejected: a parentless delegate fallback - it cannot call the lead-only
  workflow tools, so a headless worker launched without a parent would lose
  the `ticket-worker` route (owner via MAIN 04:41Z).
- **A key the environment already supplies is reused, not minted.** The
  existing `WS_PI_DELEGATION_POLICY.sessionKey` path stays as it is. A key a
  prompt names explicitly passes through unchanged for a no-policy worker;
  policy-bearing children still reject keys outside `knownKeys`
  (`agents-plugin-pi/src/bridge.ts#L1121-L1122`,
  `agents-plugin-pi/src/delegation-policy.ts#L113-L117`). Only omitted-key
  calls fall back to the bootstrap key, which registers no row when it is
  parent-bound. Rejected: parsing keys out of the prompt before bootstrap - bootstrap
  runs before the first turn, and admitted explicit keys reach the server as
  written (`agents-plugin-pi/src/bridge.ts#L886-L899`).
- **Both fork spawn sites pass the lead's parent policy.** `buildForkSpawnCtx`
  (`fork.ts`) and the discussion fork's `spawnAgent` call (`ask.ts`) pass
  `parentPolicy: leadParentPolicy(bridge)`, so the fork's policy carries
  `parentSessionKey` and its key is a parent-bound lead key; admission keeps
  the fork's lead authority. The legacy `parentSessionKey` field and
  `WS_PI_PARENT_SESSION_KEY` stay for the refusal guard. A fork relaunched from a
  dormant record is re-stamped the same way: `relaunchDelegation`'s fork
  exemption (`agents-plugin-pi/src/spawner.ts#L3991-L3994`, launched at
  `#L4128-L4140`) is removed so a resumed fork's policy carries the current
  lead's key as `parentSessionKey`, keeping pre-minted keys and the refusal
  guard. Every dormant-fork relaunch caller supplies the current lead key it
  now lacks: the `ask.ts` owner-thread send (`agents-plugin-pi/src/ask.ts#L1442`)
  and the `/done` Finish path (`closeThreadOnDone` -> `finishForkRaisedThread`
  -> `startForkFinish`, whose `finishResumeCtx` carries only cwd and
  extensionPath today: `agents-plugin-pi/src/spawner.ts#L2042-L2064`,
  `#L2128-L2130`, `agents-plugin-pi/src/ask.ts#L1217-L1266`). This supersedes the
  lateral-lead fork exemption of `260926-bug-pi-execute-worker-steals-mailbox-owner`
  (5fa2e7cb8) per owner (924)=a. Consequence accepted: a fork is no longer a
  top-level lead, so the top-level-only note, whiteboard, and tracker writes
  are refused to it, as they already are to workers. The clerk role design is
  deferred (owner (924)=a). Rejected: delegate authority for forks - fork
  flows call lead-only `workflow_manual`/`workflow_state`.
- **Main lead unchanged.** A process with no role marker and no policy keeps
  today's lead bootstrap: relogin, hosted key, recorded-key reuse, and harness
  report.
- **Historical rows stay** until the dashboard's 7-day retention removes them;
  no cleanup (owner (925)=c).
- **Compatibility: T0.** Client-side only. `capability` and
  `parent_session_key` are existing ferrule arguments; no server, protocol,
  or store change. If implementation finds the fix cannot stay T0, stop and
  report to the lead.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- No change under `agents-plugin-tool/internal/mcp/` except tests; ferrule
  semantics stay as they are.
- Session authority is not widened: the bridge's `assertSessionAuthority`
  and fork refusal guard keep their behavior. The fork acquired-key and
  descendant-key refusals (sibling tickets) are out of scope.
- A headless worker with a parent key, and a fork, must still complete the
  `ticket-worker` route and the side-thread flows (`ask.ts`, `fork.ts`, the
  workflow_manual mapping at `bridge.ts:1159`). Those routes call lead-only
  tools: `ticket-worker` requires `workflow_manual`
  (`agents-plugin/rsrc/ticket-worker/ticket-worker.md#L26-L28`), and fork
  mapping dispatches `workflow_manual` and, on an anchor-cut miss,
  `workflow_state` (`agents-plugin-pi/src/bridge.ts#L489-L516`,
  `#L1275-L1284`; lead-only per `server.go:143-145` and `roleAllowsTool` at
  `server.go:5714`). The parent-bound lead key keeps that access by design;
  do not demote these processes to delegate or leaf capability. Stop and
  report only if a needed tool is refused to a parent-bound lead key.
  `piByToken` bypasses the keyed role gate only for `pi.*`
  (`agents-plugin-tool/internal/mcp/server.go#L747-L752`,
  `agents-plugin-tool/internal/mcp/pi_forward.go#L48-L55`).
- Rebuild the committed `agents-plugin-pi/dist/index.js` with
  `npm run bundle` in `agents-plugin-pi/` after source edits. The package
  ships `dist/` and defines that script (`agents-plugin-pi/package.json`);
  `agents-plugin-pi/scripts/bundle.mjs#L2-L12` requires rebuilding and
  recording its source-input hash, which the launcher checks before using
  the bundle (`agents-plugin-tool/internal/wsagent/pi_bundle.go#L15-L25`,
  `agents-plugin-tool/internal/wsagent/pi_bundle.go#L85-L104`).
- While editing, fix the stale "merge into" comments at
  `agents-plugin-pi/src/index.ts:655` and `mailbox-waiter.ts:179` and `:316`.
- Tests: `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`; Go tests run with
  focused `-run` patterns only.

## Prior Decisions

- 261110-feat-ws-dashboard-workspaces / 29a561a2 (2026-11-12, commit): "The report is computed once per bridge (harnessSessionArgs, gated on ownsLeadKey) and threaded into every lead login" — bearing: constrains
- 261110-feat-ws-hosted-session-mode-flag / 4add08ee (2026-11-11, commit): "Only an owner lead (no spawn role, no policy) reads the hosted key, so a policy child spawned from a hosted lead keeps its own child key." — bearing: constrains
- ee0520fa (2026-11-06, commit): "Only top-level leads register, since only they have boards and live snapshots" — bearing: supports
- 261007-bug-pi-session-key-reissued-on-resume / dc698ea0 (2026-10-07, commit): "reloginLeadKey verifies the returned key equals the requested one; a mismatch counts as a refusal so a misbehaving server can never swap the default key." — bearing: constrains
- 260926-bug-pi-execute-worker-steals-mailbox-owner / 5fa2e7cb8 (2026-09-26, commit): "ws-fork and the ask discussion fork are fork-exempt (lateral leads, parent key only via WS_PI_PARENT_SESSION_KEY" — bearing: contradiction-candidate
- 260912-feat-ws-pi-recursive-worker-subtree-lifecycle / 7228fac3 (2026-09-13, commit): "explicit session keys cannot grant non-root processes broader capabilities." — bearing: constrains
- 3ec8d5af (2026-09-08, commit): "Role safety is execution-time refusal, not schema hiding: forks retain the lead's provider-visible tools while ws-fork/ws-ask/ws-resolve reject before allocation or mutation." — bearing: constrains
- 0c7c0f50 (2026-06-11, commit): "Child-key minting is render-time and lead-gated: playbook.render is the single mint+inject point for both native and mercenary delegates" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/bridge.ts, agents-plugin-pi/src/fork.ts, agents-plugin-pi/src/index.ts, agents-plugin-pi/src/mailbox-waiter.ts, committed agents-plugin-pi/dist/index.js, agents-plugin-pi/src/ask.ts (discussion fork spawn and owner-thread send), agents-plugin-pi/src/spawner.ts (fork relaunch re-stamp), and Go/Pi tests including agents-plugin-pi/test/relaunch-parent-key.test.ts |
| scope.surface | cross-module | Pi bootstrap, delegation admission, fork/discussion dispatch, and the existing MCP capability gate interact; no new MCP argument is proposed |
| scope.new_public_symbol | no | no new public symbol is named; capability and parent_session_key already exist |
| scope.new_type_contract | no | existing DelegationPolicy.authority and parentSessionKey, plus existing ferrule argument contracts |
| scope.test_surface | existing | agents-plugin-pi/test/bridge.test.ts, agents-plugin-pi/test/fork.test.ts, agents-plugin-pi/test/spawner.test.ts; agents-plugin-tool/internal/mcp/dashboard_sessions_test.go#L151-L186 covers top-level registration and a lead-scope child, not the requested delegate matrix |
| complexity.reuse_points | confirmed | readSpawnRole, leadParentPolicy, childPolicy, ferrule capability/parent handling, isTopLevelLead, and the committed bundle build script were read |
| complexity.side_effect_risk | high | changing fork lineage alters dashboard registration, mailbox ownership and top-level-only writes; capability stays lead so workflow-tool admission is unchanged |
| risk.correctness | high | no-policy bootstrap selection, unknown-parent legacy retry, one-time warning, main-key reuse, and the fork spawn and relaunch sites must agree |
| risk.fit | high | parent-bound lead keys keep workflow_manual/workflow_state; the fork entry points (buildForkSpawnCtx, ask.ts discussion fork, dormant-fork relaunch via owner-thread send and /done Finish) must all carry the parent; the no-parent path must stay byte-for-byte legacy |
| risk.test | high | current Pi tests use fake-MCP responses that do not prove server-side registration; focused Go assertions must show a parent-bound lead key registers no row while keeping workflow_manual, and Pi assertions must distinguish default keys from explicit rendered worker keys |
| risk.security_or_contract | high | capability demotion meets existing lead-only and adapter session-authority boundaries; the ticket forbids widening them or changing server semantics |

## Phases

### Phase 1: Parent-bound bootstrap for non-main Pi processes and forks

- `bridge.ts` bootstrap, role marker set and no policy: read
  `WS_PI_PARENT_SESSION_KEY` (any role); with it, call `ferrule` with
  `parent_session_key` and no capability (lead, parent-bound). Without it,
  call exactly as today (no capability, no parent) and emit a one-time
  warning to set `WS_PI_PARENT_SESSION_KEY`. On an unknown-parent refusal,
  retry once with that legacy call and emit the same warning. The fork
  refusal guard keeps its current inputs.
- `fork.ts` `buildForkSpawnCtx` and the `ask.ts` discussion-fork `spawnAgent`
  call: pass `parentPolicy: leadParentPolicy(bridge)`; fork authority stays
  lead. `spawner.ts` `relaunchDelegation`: drop the fork exemption so a
  resumed fork is re-stamped with the current lead key as parent; every
  relaunch caller passes the live lead key - the `ask.ts` owner-thread send
  and the `/done` Finish path (`startForkFinish` / `finishResumeCtx`). Replace the
  parentless-fork expectations in `agents-plugin-pi/test/relaunch-parent-key.test.ts`.
- Fix the stale comments named in `## Constraints`; regenerate the committed
  Pi bundle (`npm run bundle` in `agents-plugin-pi/`).

Verification:
- Pi unit tests: a role marker with no policy and a parent env calls ferrule
  with `parent_session_key` (lead, parent-bound); without the env it calls
  with today's arguments (no capability, no parent) and warns once; the
  unknown-parent refusal retries with the legacy call and warns once; both
  fork spawn sites and a relaunched dormant fork carry the lead's session key
  as parent, through both the owner-thread send and a dormant `/done` Finish,
  for a stored parentless policy and a stale-parent policy alike; the main lead's bootstrap arguments are unchanged.
- Go test (focused `-run`): a parent-bound lead key never registers a
  dashboard row on a keyed call (an omitted `session_key` filled with it
  included), it can still call `workflow_manual`, and a top-level lead still
  registers.
- The existing Pi test suite for bridge, fork, spawner, and relaunch passes.

### Result

- `bridge.ts`: a role-marker process with no policy mints with
  `parent_session_key` from `WS_PI_PARENT_SESSION_KEY` (lead capability);
  without it, or on the server's unknown-parent refusal, it mints the legacy
  top-level key and warns once: "ws-pi-bridge: non-main Pi process has no
  known parent; set WS_PI_PARENT_SESSION_KEY to the launching lead's session
  key. Falling back to a top-level lead key."
- `fork.ts` and the `ask.ts` discussion fork pass `leadParentPolicy(bridge)`;
  `relaunchDelegation` no longer exempts forks; the owner-thread send and the
  `/done` Finish path carry the live lead key. `buildRpcClientOptions` sets
  `WS_PI_PARENT_SESSION_KEY` for every child to its immediate parent (the fork
  refusal guard stays fork-only).
- Stale comments fixed; Pi bundle regenerated.
- Verification: Pi `child-key-bootstrap`, `relaunch-parent-key`, `bridge`,
  `fork`, `spawner` suites 361/361 pass; Go `-run TestDashboard` (mcp,
  including the new parent-bound no-registration test) and wsagent bundle-hash
  tests pass.
- Headless `pi -p` launches outside the spawner have no checked-in launch
  site; their launchers must set `WS_PI_PARENT_SESSION_KEY`.
