---
title: "ws 도그푸딩 컨텍스트 바인딩·전달·브랜치 재사용 수정"
related:
  261119-bug-ws-agent-v2-dogfood-surprises: "Earlier broad dogfood capture; this ticket carries the source-diagnosed current fixes."
  261120-bug-ws-agent-dogfood-delivery-context: "Develop already contains its direct-parent and worker-delivery fixes."
  261115-feat-ws-permission-profile: "Its retained implementation branch reproduces stale branch reuse."
  260912-bug-ws-pi-readonly-reviewer-artifact-contract: "Prior Pi-only scoped reviewer artifact contract."
---

# Fix ws dogfood context routing gaps

## Background

Five dogfood surprises observed on 2026-11-21 share a boundary problem: a
caller-visible identity or delivery contract is either implicit, stale, or
split between host-specific and host-neutral paths. This ticket records the
source diagnosis against `develop` at `596109dbe` and groups the independent
fixes without changing source during diagnosis.

The intended end state is that a lead can safely hand work to a child, reuse a
ticket across phases, tune every shipped config key from Claude, and obtain a
durable Lite review report without guessing which identity, branch, or mailbox
owns the operation.

## Evidence and Diagnosis

### 1. Spawn-key binding prevents a later cross-worktree handoff

- **VERIFIED:** `agent.spawn` anchors a child to the caller root unless the
  spawn request names a worktree; the selected root, cwd, and minted ws key are
  persisted in the child record
  (`agents-plugin-tool/internal/wsagent/agent_tools.go:80`,
  `agents-plugin-tool/internal/wsagent/agent_tools.go:620`,
  `agents-plugin-tool/internal/wsagent/agent_tools.go:645`,
  `agents-plugin-tool/internal/wsagent/runner.go:866`,
  `agents-plugin-tool/internal/wsagent/runner.go:927`).
- **VERIFIED:** process startup revalidates the saved key against that root,
  and the child prompt requires that key on every ws call
  (`agents-plugin-tool/internal/wsagent/runner_process.go:94`,
  `agents-plugin-tool/internal/wsagent/runner_process.go:796`). Root-aware MCP
  calls then resolve the root authoritatively from the session key rather than
  a later cwd/path argument
  (`agents-plugin-tool/internal/mcp/server_sessions.go:286`). The fake key
  store explicitly rejects the same key at another root
  (`agents-plugin-tool/internal/wsagent/fake_test.go:336`).
- **INFERRED:** merely telling the running child to use a newly acquired
  worktree cannot rebind its key. A commit in that worktree therefore presents
  an identity rooted in the original worktree and is correctly refused.
- **Develop verdict:** **not fixed.** Current source has no live-child transfer
  or rebind operation. Spawning with `worktree: <B>` avoids the conflict.
- **Minimal fix:** settle (1924). The preferred small contract is to keep keys
  immutable, require a respawn for a different worktree, and make the mismatch
  diagnose the required respawn. Test both the refused A-to-B handoff and a
  successful child spawned directly into B.
- **Compatibility:** **T0** for the preferred diagnostic/prompt/workflow fix:
  tool schemas and stored key records do not change, and old calls retain their
  behavior. An additive rebind API could only be T0 with capability gating and
  is intentionally not assumed here
  (`ai-docs/manuals/release-compatibility.md:52`,
  `ai-docs/manuals/release-compatibility.md:73`).

### 2. Child-to-parent mailbox addressing was implicit

- **VERIFIED:** mailbox addresses accept only `name@scope` or
  `id:<reply-id>`; bare `parent` is invalid
  (`agents-plugin-tool/internal/wsmailbox/address.go:68`,
  `agents-plugin-tool/internal/mcp/mailbox_tools.go:27`). A worktree-key inbox
  is a named mailbox, whereas a reply id enters the direct reply queue
  (`agents-plugin-tool/internal/mcp/mailbox_tools.go:68`).
- **VERIFIED:** current `develop` injects the direct parent's reply id into
  every child prompt and tests both direct and nested parent routing
  (`agents-plugin-tool/internal/wsagent/runner_process.go:796`,
  `agents-plugin-tool/internal/wsagent/agent_tools_test.go:763`). This landed in
  `f9332e85c` under
  `261120-bug-ws-agent-dogfood-delivery-context`
  (`ai-docs/tickets/.done/261120-bug-ws-agent-dogfood-delivery-context.md:409`).
- **Develop verdict:** **fixed.** Bare `parent` remains intentionally outside
  the address grammar; the prompt now supplies the address that works.
- **Minimal fix:** retain production behavior and pin it with an end-to-end
  test proving a direct child and nested child send to the injected parent id,
  do not fall back to a worktree inbox, and reach the direct parent exactly
  once. Adjust wording only if that regression exposes ambiguity.
- **Compatibility:** **T0.** This is a launch-time prompt/context correction;
  it changes no tool, argument, hello, or mailbox record
  (`ai-docs/tickets/.done/261120-bug-ws-agent-dogfood-delivery-context.md:438`).

### 3. A fully merged implementation branch is reused as if active

- **VERIFIED:** on `develop`, `git.resolve_impl_branch` resolves
  `261115-feat-ws-permission-profile` to
  `impl/develop/foyer-cause-shore`; that ref is an ancestor of `develop` and is
  `1800` commits behind with `0` commits ahead.
- **VERIFIED:** the resolver derives and validates only the canonical branch
  name; it does not inspect existence, ancestry, or divergence
  (`agents-plugin-tool/internal/mcp/impl_identity_tool.go:10`). Acquisition
  checks out an existing branch without resetting it and only warns when it is
  behind base
  (`agents-plugin-tool/internal/mcp/worktree_tools.go:73`,
  `agents-plugin-tool/internal/mcp/worktree_tools.go:530`).
- **VERIFIED:** route observation records commits ahead of the merge root but
  not commits behind it
  (`agents-plugin-tool/internal/mcp/implement_resolver.go:130`,
  `agents-plugin-tool/internal/mcp/implement_resolver.go:470`,
  `agents-plugin-tool/internal/mcp/implement_resolver.go:507`). An exact
  canonical-target match returns `continue` unconditionally
  (`agents-plugin-tool/internal/mcp/implement_resolver.go:931`). The separate
  create path already distinguishes an empty leftover branch from one carrying
  work
  (`agents-plugin-tool/internal/mcp/implement_resolver.go:866`).
- **Develop verdict:** **not fixed.** Existing tests cover a leftover target
  while current is base, but the same-ticket exact-target case is deliberately
  treated as active
  (`agents-plugin-tool/internal/mcp/implement_resolver_test.go:446`,
  `agents-plugin-tool/internal/mcp/implement_resolver_test.go:994`).
- **Minimal fix:** settle (1925), then classify an existing canonical target
  against base under the acquisition lock. Recreate a fully landed target at
  base, preserve/refuse any target with unmerged work, and retain an equal
  target. Route resolution must also refuse or repair a stale exact target.
  Regress the merged-behind, active-ahead, equal, and divergent cases.
- **Compatibility:** **T0.** The correction can preserve current tool names,
  required arguments, and stored records. Any optional result fact remains an
  additive field that old callers ignore
  (`ai-docs/manuals/release-compatibility.md:73`).

### 4. Claude did not expose the agy timeout config key

- **VERIFIED:** current Claude packaging includes
  `${CLAUDE_PLUGIN_ROOT}/agy-config-manifest.json`, and that manifest declares
  `agy.model_wait_timeout_seconds`
  (`agents-plugin/.claude-plugin/plugin.json:17`,
  `agents-plugin/agy-config-manifest.json:1`). Commit `114f104d4` added that
  third manifest on 2026-11-20; the preceding Claude launcher listed only the
  core and dashboard manifests.
- **VERIFIED:** manifest knowledge is connection-scoped. The server loads only
  paths supplied in the caller's hello environment
  (`agents-plugin-tool/internal/mcp/config_manifest.go:18`,
  `agents-plugin-tool/internal/mcp/config_manifest.go:99`), and the proxy
  preserves the caller-owned path list
  (`agents-plugin-tool/cmd/ws-mcp/proxy.go:146`). An external connection does
  not inherit the daemon's hosted package policy
  (`agents-plugin-tool/cmd/ws-mcp/hosted_plugin_package_test.go:127`).
- **INFERRED:** the observed lead used a pre-update or not-yet-restarted Claude
  launcher/session, or its manifest failed to load. A daemon predating the
  manifest is not independently sufficient to explain the error because the
  generic loader already consumes the connection's supplied paths; daemon age
  cannot be established from repository evidence.
- **Develop verdict:** **production packaging fixed, regression coverage
  incomplete.** The hosted package fixture still models only two manifests and
  does not assert the exact shipped Claude list
  (`agents-plugin-tool/cmd/ws-mcp/hosted_plugin_package_test.go:21`,
  `agents-plugin-tool/cmd/ws-mcp/hosted_plugin_package_test.go:105`).
- **Minimal fix:** assert the exact three-manifest list and file existence for
  ws and wsflow under Claude and Codex; update the stale hosted-package fixture;
  and add a focused integration test proving that a Claude hello can list/get
  and project-tune the agy timeout. Make unknown-key diagnostics identify
  rejected manifests or a required plugin-session restart where available.
- **Compatibility:** **T0.** A separate schema-1 manifest path is additive and
  leaves old calls and existing manifest/store shapes intact. The loader's
  strict unknown-field validation is precisely why the new namespace remains
  separate
  (`agents-plugin-tool/internal/mcp/config_manifest.go:172`,
  `ai-docs/manuals/release-compatibility.md:73`).

### 5. A read-only Lite reviewer cannot own the required findings artifact

- **VERIFIED:** the reviewer contract is read-only yet also says the reviewer
  always writes the detailed report to the generated findings path
  (`agents-plugin/rsrc/code-reviewer.md:1`,
  `agents-plugin/rsrc/code-reviewer.md:43`). The Lite stop protocol renders and
  spawns that reviewer without supplying a findings path or scoped write grant
  (`agents-plugin/rsrc/worker-stop-protocol.md:70`). Common `agent.spawn` has no
  write-scope argument
  (`agents-plugin-tool/internal/wsagent/agent_tools.go:123`).
- **VERIFIED:** the legacy Pi path has an explicit exception: identify a review
  artifact, require exactly one file write scope, and grant only that file
  (`agents-plugin-pi/src/delegation-policy.ts:127`,
  `agents-plugin-pi/src/spawner-admission.ts:289`,
  `agents-plugin-pi/src/write-scopes.ts:180`). The common daemon-agent adapter
  explicitly states that legacy write scopes are not `agent.spawn` arguments
  (`agents-plugin-pi/src/daemon-agent-compat.ts:17`).
- **VERIFIED:** hosted leads do not wait on mailbox polling. The daemon claims
  queued direct replies and injects them as user turns
  (`agents-plugin-tool/internal/mcp/lead_mail.go:18`,
  `agents-plugin-tool/internal/mcp/lead_mail.go:154`,
  `agents-plugin-tool/internal/wsagent/runner_conv.go:309`). A delivered batch
  explicitly says it has already been received through `mailbox.recv`, so a
  later manual receive must not duplicate it
  (`agents-plugin-tool/internal/mcp/lead_mail.go:353`).
- **INFERRED:** the empty artifact is the direct consequence of assigning its
  write to a read-only common child. A successful direct send followed by an
  empty manual `mailbox.recv` is not evidence of loss when the hosted lead has
  already auto-claimed that reply into a turn.
- **Develop verdict:** **partly fixed.** `f9332e85c` fixed the direct-parent id
  and worker steering, but the host-neutral reviewer artifact ownership remains
  broken. The prior ticket itself left this recheck forward
  (`ai-docs/tickets/.done/261120-bug-ws-agent-dogfood-delivery-context.md:451`).
- **Minimal fix:** settle (1926). Prefer returning full findings from the
  read-only reviewer and making the write-capable handler own the generated
  artifact. Test non-clean and clean Lite runs for a non-empty artifact, no
  reviewer source writes, direct-parent delivery exactly once, and no duplicate
  manual receive after auto-claim.
- **Compatibility:** **T0** for moving artifact persistence to the existing
  handler and clarifying current mailbox semantics. No tool, hello, mailbox
  record, or persisted-history shape changes.

## Open Decision Queue

Last ID: (1926)

- (1924) [open] [policy] Should a live child remain permanently bound to its
  spawn root, requiring respawn for a different worktree (recommended), or
  should ws introduce an explicit adopt/rebind lifecycle that atomically moves
  key, root, mail, config, and process authority?
- (1925) [open] [technical] Should stale canonical implementation branches be
  classified and recreated during locked worktree acquisition, with route
  resolution retaining a defensive stale-target refusal (recommended), or
  should route resolution own all repair before acquisition?
- (1926) [open] [technical] Should the write-capable review handler persist the
  full report returned by a read-only reviewer (recommended), or should common
  `agent.spawn` gain a narrowly scoped review-artifact write capability across
  every host?

## Constraints

- Preserve the session-key/root authority boundary; no fallback to native git
  when ws refuses an operation.
- Preserve the canonical mailbox grammar and once-only hosted-lead auto-claim
  semantics. Do not make a named worktree inbox an alias for a direct parent.
- Preserve the portions already fixed by `f9332e85c`: injected direct-parent
  ids, nested-parent routing, and active worker steering.
- Read `ai-docs/manuals/shipped-surface-boundary.md` before changing shipped
  plugin, wsflow, or tooling surfaces.
- Read `ai-docs/manuals/skill-authoring.md` before changing reviewer, stop
  protocol, prompt, or convention text, and apply its invariant checklist.
- Read `ai-docs/manuals/wsflow-mirroring.md` before changing mirrored ws/wsflow
  resources.
- Read `ai-docs/manuals/ws-mcp.md` before changing MCP code.
- Read `ai-docs/manuals/release-compatibility.md` before changing internal
  cross-version surfaces, and preserve the T0 verdicts or reopen the tier.
- Read `ai-docs/manuals/performance.md` before changing tooling paths; keep
  interactive resolution and delivery off avoidable file I/O.
- Run narrow Go tests with
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`; do not restart, replace, or
  otherwise touch the shared daemon during diagnosis or verification.

## Phases

### Phase 1: Make child worktree binding explicit

After (1924) settles, implement the chosen lifecycle without weakening root
authority. Under the recommended contract, retain immutable child keys, reject
later cross-worktree assignments with a respawn-specific diagnostic, and teach
lead/child guidance to spawn directly into the acquired worktree.

Acceptance:

- A child spawned in worktree A cannot ws-commit in worktree B and receives an
  actionable respawn diagnostic.
- A child spawned with `worktree: B` can use its minted key for B.
- Existing same-root child calls remain unchanged.
- Compatibility remains T0, or the phase records and settles a revised tier
  before implementation.

### Phase 2: Pin direct-parent and once-only mailbox delivery

Add an end-to-end regression around the already-landed parent-id prompt and
hosted-lead auto-claim path. Change production wording only where the test
shows a remaining ambiguity.

Acceptance:

- Direct and nested children send to the exact injected direct-parent reply id.
- Neither path substitutes a worktree-key inbox.
- A hosted lead receives the message exactly once as an active steer or later
  turn; a subsequent manual `mailbox.recv` does not duplicate it.
- Compatibility remains T0.

### Phase 3: Retire fully landed canonical implementation branches

After (1925) settles, make provisioning and route resolution distinguish a
retained active branch from a canonical branch whose work is wholly contained
in base. Perform any branch recreation under the existing acquisition lock and
never discard unmerged work.

Acceptance:

- A canonical target that is behind base with zero commits ahead is recreated
  at base instead of checked out stale.
- Equal and actively-ahead same-ticket targets retain their intended behavior.
- Divergent or otherwise unmerged targets are preserved and produce a stop or
  explicit conflict instead of reset.
- The exact `261115-feat-ws-permission-profile` topology is covered by a
  regression fixture.
- Compatibility remains T0.

### Phase 4: Lock Claude agy manifest exposure

Turn the current production packaging fix into an exact contract and improve
the error path enough to distinguish an unknown declared key from a stale or
rejected caller manifest.

Acceptance:

- ws and wsflow tests assert the exact three shipped manifest paths for Claude
  and Codex and verify every file exists.
- The hosted-package fixture no longer models the obsolete two-manifest list.
- A Claude-hello integration test lists, reads, and project-tunes
  `agy.model_wait_timeout_seconds`.
- An unavailable shipped key points to rejected manifests or the need to
  restart/update the plugin session where that evidence exists.
- Compatibility remains T0.

### Phase 5: Give Lite review artifacts one write-capable owner

After (1926) settles, remove the contradiction between a read-only reviewer and
mandatory durable findings. Under the recommended contract, require complete
findings in the reviewer result and let the handler generate and write the
artifact, including the clean-report case.

Acceptance:

- A non-clean read-only Lite review produces a non-empty findings artifact
  containing every returned finding.
- A clean Lite review produces a non-empty clean report.
- The reviewer receives no source-tree write authority.
- The direct-parent report reaches the hosted lead exactly once without
  relying on a worktree inbox or duplicate manual receive.
- ws/wsflow mirrors and Pi compatibility guidance remain aligned.
- Compatibility remains T0, or any proposed `agent.spawn` API addition is
  separately tiered and settled before implementation.
