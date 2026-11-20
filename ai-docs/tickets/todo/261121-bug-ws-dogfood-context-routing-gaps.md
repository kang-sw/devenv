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

Six dogfood surprises observed on 2026-11-21 share a boundary problem: a
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
- **VERIFIED:** a hosted Claude lead uses the daemon build's embedded package
  policy to construct its launcher, and each hosted connection passes that
  launcher's manifest paths to a new server that loads them at construction
  (`agents-plugin-tool/internal/wsagent/hosted_plugins.go:14`,
  `agents-plugin-tool/cmd/ws-mcp/agent_mcp.go:19`,
  `agents-plugin-tool/cmd/ws-mcp/proxy.go:75`,
  `agents-plugin-tool/internal/mcp/process_env.go:159`).
- **INFERRED:** for the reported hosted lead, a daemon built before
  `114f104d4` would embed a Claude launcher that omits the agy path: “daemon
  predates the manifest” and “launcher omits the manifest” are then the same
  build-staleness cause. A daemon that can receive the current path is not
  independently unable to parse it. The live daemon build, selected package,
  and manifest rejection diagnostics were not inspected, so the particular
  refusal cannot be marked verified.
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
- **Compatibility:** **T0 for old calls and persisted data** because the
  separate schema-1 manifest path is additive and project overrides use the
  existing generic key map. However, a new caller sending the agy key to a
  pre-manifest hosted build receives `unknown config key` before any write;
  that direction is a **T3 old-version boundary unless the caller gates the
  attempt on `config.list` or requires an updated daemon**
  (`agents-plugin-tool/internal/mcp/config_manifest.go:106`,
  `agents-plugin-tool/internal/mcp/config_manifest.go:172`,
  `agents-plugin-tool/internal/mcp/server_dispatch.go:411`,
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
- **VERIFIED:** ws persists a settled child's complete `TurnResult` in its
  pending-report record and `ReportText` appends the final message without an
  explicit source-side truncation cap
  (`agents-plugin-tool/internal/wsagent/record.go:242`,
  `agents-plugin-tool/internal/wsagent/spawn.go:307`,
  `agents-plugin-tool/internal/wsagent/children.go:1002`).
- **INFERRED:** the canary worker's truncated reports therefore do not have a
  demonstrated truncation point in this ws delivery path. The fact that all
  three findings were recovered through compact follow-ups proves that a
  single large final report is not a sufficient durable-artifact protocol,
  but does not distinguish harness output truncation from host presentation
  truncation.
- **INFERRED:** the empty artifact is the direct consequence of assigning its
  write to a read-only common child. A successful direct send followed by an
  empty manual `mailbox.recv` is not evidence of loss when the hosted lead has
  already auto-claimed that reply into a turn.
- **Develop verdict:** **partly fixed.** `f9332e85c` fixed the direct-parent id
  and worker steering, but the host-neutral reviewer artifact ownership remains
  broken. The prior ticket itself left this recheck forward
  (`ai-docs/tickets/.done/261120-bug-ws-agent-dogfood-delivery-context.md:451`).
- **Minimal fix:** settle (1926). Prefer making the write-capable handler own
  the generated artifact and requiring a bounded, completeness-checked
  findings handoff from the read-only reviewer rather than assuming one final
  report is untruncated. Test clean, ordinary non-clean, and oversized
  multi-finding Lite runs for a complete non-empty artifact, no reviewer source
  writes, direct-parent delivery exactly once, and no duplicate manual receive
  after auto-claim.
- **Compatibility:** **T0** for moving artifact persistence to the existing
  handler and clarifying current mailbox semantics. No tool, hello, mailbox
  record, or persisted-history shape changes.

### 6. The worker playbook requires tools its child profile withholds

- **VERIFIED:** the shared Worker Protocol tells every ticket worker to call
  `workflow_manual`, while the elevated ticket-worker playbook tells it to
  call `config.resolve_agent` before delegated leaves
  (`agents-plugin/rsrc/worker-stop-protocol.md:18`,
  `agents-plugin/rsrc/ticket-worker-elevated/ticket-worker-elevated.md:49`).
- **VERIFIED:** `workflow_manual` is intentionally a lead-key-minting tool that
  agent profiles may never name, and child-surface tests explicitly require it
  to remain excluded
  (`agents-plugin-tool/internal/wsagent/profile.go:67`,
  `agents-plugin-tool/internal/wsagent/child_safe_mcp_tools_test.go:29`). A
  spawned child receives its key and direct-parent address in its system
  instructions, but not a lead workflow manual
  (`agents-plugin-tool/internal/wsagent/runner_process.go:796`).
- **VERIFIED:** `config.resolve_agent` is a read-only tool that the server
  deliberately authorizes for delegate keys because worker playbooks use it
  before spawning
  (`agents-plugin-tool/internal/mcp/server_schemas.go:352`,
  `agents-plugin-tool/internal/mcp/server_inventory.go:229`,
  `agents-plugin-tool/internal/mcp/session_auth_test.go:514`). Despite that
  contract, it is absent from both `ChildSafeWSTools` and the lists that build
  `AgentWSTools`
  (`agents-plugin-tool/internal/wsagent/builtin.go:119`,
  `agents-plugin-tool/internal/wsagent/builtin.go:171`). Codex filters all
  spawned children to the child-safe subset, and Claude does the same for
  read-only children
  (`agents-plugin-tool/internal/wsagent/codex.go:258`,
  `agents-plugin-tool/internal/wsagent/claude.go:224`).
- **Develop verdict:** **not fixed.** The canary worker's missing tools match
  the current playbook/profile mismatch exactly.
- **Minimal fix:** settle (1927) and (1928). Remove the impossible
  `workflow_manual` call
  from worker instructions and treat the rendered Worker Protocol, ticket,
  AGENTS guidance, and declared manuals as the worker's context. Add the
  already-authorized `config.resolve_agent` to the harness-neutral child-safe
  and agent-worker surfaces. Regress every worker playbook's required ws tool
  references against the effective Codex, Claude, agy, and Pi child profiles.
- **Compatibility:** **T0.** This exposes an existing read-only tool to a role
  that the server already authorizes and removes a bad instruction; it changes
  no MCP schema, argument, hello, or persisted record. `workflow_manual`
  remains excluded, so no child gains a lead-key mint path
  (`ai-docs/manuals/release-compatibility.md:73`).

## Open Decision Queue

Last ID: (1928)

- (1924) [open] [policy] Should a live child remain permanently bound to its
  spawn root, requiring respawn for a different worktree (recommended), or
  should ws introduce an explicit adopt/rebind lifecycle that atomically moves
  key, root, mail, config, and process authority?
- (1925) [open] [technical] Should stale canonical implementation branches be
  classified and recreated during locked worktree acquisition, with route
  resolution retaining a defensive stale-target refusal (recommended), or
  should route resolution own all repair before acquisition?
- (1926) [open] [technical] Should the write-capable review handler persist a
  bounded, completeness-checked findings handoff from the read-only reviewer
  (recommended), or should common `agent.spawn` gain a narrowly scoped
  review-artifact write capability across every host?
- (1927) [open] [technical] Should worker instructions stop requiring the
  lead-key-minting `workflow_manual` and rely on the rendered Worker Protocol,
  ticket, AGENTS guidance, and declared manuals (recommended), or should ws
  introduce a new child-safe workflow-context tool?
- (1928) [open] [technical] Should the child surface expose the existing
  read-only `config.resolve_agent` that the server already authorizes for
  delegates (recommended), or should every host's worker delegation contract
  be redesigned to avoid separate tier resolution?

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
  cross-version surfaces, and preserve the documented verdicts and gates or
  reopen the tier.
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
- A caller checks `config.list` before using the additive key against an
  unknown daemon version, preserving a T0 old-call path instead of relying on
  a T3 refusal.
- An unavailable shipped key points to rejected manifests or the need to
  restart/update the plugin session where that evidence exists.
- Compatibility remains T0 for old calls and stored data; any ungated
  new-caller-to-old-daemon path is recorded as T3 and requires explicit owner
  approval.

### Phase 5: Give Lite review artifacts one write-capable owner

After (1926) settles, remove the contradiction between a read-only reviewer and
mandatory durable findings. Under the recommended contract, let the handler
generate and write the artifact, and use a bounded handoff with an explicit
finding count or equivalent completeness check instead of trusting one large
final report, including the clean-report case.

Acceptance:

- A non-clean read-only Lite review produces a non-empty findings artifact
  containing every returned finding.
- An oversized multi-finding review remains complete when one host-visible
  report would be truncated; the handler detects a missing chunk or count
  before reporting success.
- A clean Lite review produces a non-empty clean report.
- The reviewer receives no source-tree write authority.
- The direct-parent report reaches the hosted lead exactly once without
  relying on a worktree inbox or duplicate manual receive.
- ws/wsflow mirrors and Pi compatibility guidance remain aligned.
- Compatibility remains T0, or any proposed `agent.spawn` API addition is
  separately tiered and settled before implementation.

### Phase 6: Align worker instructions with the spawned-child tool surface

After (1927) and (1928) settle, remove the lead-only bootstrap call from the
Worker Protocol and expose the existing read-only tier resolver that elevated
workers actually need for delegation. Keep all lead-key mint paths unavailable
to children.

Acceptance:

- Rendered ticket-worker instructions do not tell spawned children to call
  `workflow_manual` or any other intentionally unavailable lead-only tool.
- `config.resolve_agent` is visible and callable with a delegate key under the
  effective Codex, Claude, agy, and Pi agent-worker profiles, including the
  Codex child-safe MCP map.
- A contract test compares required ws tool references in worker playbooks
  against each effective child surface and fails on another prose/exposure
  drift.
- Leaf keys retain their current config restrictions, and no child can mint or
  adopt a lead key.
- ws/wsflow mirrors stay aligned.
- Compatibility remains T0.
