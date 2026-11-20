---
title: "ws 도그푸딩 컨텍스트 바인딩·전달·브랜치 재사용 수정"
related:
  261119-bug-ws-agent-v2-dogfood-surprises: "Earlier broad dogfood capture; this ticket carries the source-diagnosed current fixes."
  261120-bug-ws-agent-dogfood-delivery-context: "Develop already contains its direct-parent and worker-delivery fixes."
  261115-feat-ws-permission-profile: "Its retained implementation branch reproduces stale branch reuse."
  261116-feat-ws-subagent-worktree-lease: "Phase 1's second valid B-key source depends on this lease contract landing."
  260912-bug-ws-pi-readonly-reviewer-artifact-contract: "Prior Pi-only scoped reviewer artifact contract."
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: f92b32b2745affc9
sage-review-completeness-reviewed: f92b32b2745affc9
completed: 2026-11-21
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
  (`agents-plugin-tool/internal/wsagent/agent_tools.go#L620-L658`,
  `agents-plugin-tool/internal/wsagent/runner.go#L907-L935`,
  `agents-plugin-tool/internal/wsagent/spawn.go#L103-L121`).
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
- **Minimal fix:** settle (1924). Keep keys immutable and make a mismatched
  A-bound key diagnose that the caller needs a key minted for B. That may be a
  child spawned directly into B or, after
  `261116-feat-ws-subagent-worktree-lease` lands, the B-bound key returned when
  the child legitimately acquires B under Decision (1190); respawn is not the
  only valid path. Test the refused A-key-in-B handoff and direct-spawn B-key
  source; test the acquired B-key source once that prerequisite lands and defer
  it otherwise.
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
  every spawned child's prompt when its profile has ws tools, and the prompt
  test covers both direct and nested parent addresses
  (`agents-plugin-tool/internal/wsagent/runner_process.go#L796-L824`,
  `agents-plugin-tool/internal/wsagent/agent_tools_test.go#L763-L781`). This landed in
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

- **VERIFIED:** at the diagnosis ref `596109dbe`,
  `git.resolve_impl_branch` resolves
  `261115-feat-ws-permission-profile` to
  `impl/develop/foyer-cause-shore`; that ref is an ancestor of `develop` and is
  `1800` commits behind with `0` commits ahead.
- **VERIFIED:** the resolver derives and validates only the canonical branch
  name; it does not inspect existence, ancestry, or divergence
  (`agents-plugin-tool/internal/mcp/impl_identity_tool.go#L10-L39`). Acquisition
  checks out an existing branch without resetting it; a known existing target
  behind base emits a warning and is not reset, while fact-read failures also
  emit warnings
  (`agents-plugin-tool/internal/mcp/worktree_tools.go#L73-L107`,
  `agents-plugin-tool/internal/mcp/worktree_tools.go#L530-L567`).
- **VERIFIED:** route observation measures current and leftover-target commits
  ahead of their merge root while retaining upstream `Ahead`/`Behind` fields;
  it does not record a merge-root-specific behind count
  (`agents-plugin-tool/internal/mcp/implement_resolver.go#L130-L145`,
  `agents-plugin-tool/internal/mcp/implement_resolver.go#L507-L543`). An exact
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
- **Minimal fix:** implement (1925) without replacing the resolver behavior
  vouched by `261116-bug-ws-worktree-acquire-stale-branch-facts` Phase 2
  (1184): retain the different-target `recreate` action at
  `implement_resolver.go:951-957` and the create-path `recreate` action at
  `:872-886`, because an in-root serial worker never calls
  `worktree.acquire`. In addition, during locked pooled-worktree acquisition,
  classify an existing name-rooted canonical target against its encoded merge
  root and non-force delete/recreate it when the target-only count is zero and
  the merge-root-only count is positive, first moving the pooled worktree to
  the merge-root commit with a detached checkout (`git switch --detach
  refs/heads/<merge-root>`; a plain branch switch fails while another worktree,
  usually the main root, has the merge-root branch checked out). Leave targets with target-only commits and equal
  targets checked out as today, with divergent targets retaining the existing
  behind warning. Route resolution adds only a defensive stop for the
  exact-target `continue` case at `:932-935`, telling a pooled caller to
  re-acquire or an in-root caller to switch the root to the merge root and route
  again so the kept create-path action can repair it. Regress the
  landed-or-empty-behind, active-ahead, equal, divergent, and unchanged
  in-root serial recreate cases.
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
- **VERIFIED:** hosted lead processes do not call `mailbox.recv` or poll for
  mail themselves; the daemon's `LeadMailbox` watches and claims queued direct
  replies, then injects them as user turns
  (`agents-plugin-tool/internal/mcp/lead_mail.go#L18-L23`,
  `agents-plugin-tool/internal/mcp/lead_mail.go#L81-L167`,
  `agents-plugin-tool/internal/wsagent/runner_conv.go:309`). A delivered batch
  explicitly says it has already been received through `mailbox.recv`, so a
  later manual receive must not duplicate it
  (`agents-plugin-tool/internal/mcp/lead_mail.go:353`).
- **VERIFIED:** for a `ParentConversation` child, ws persists the complete
  `TurnResult` in its pending-report record and `ReportText` appends the final
  message without an explicit source-side truncation cap; a `ParentLead` child
  instead reports through `Options.Report` and mailbox delivery
  (`agents-plugin-tool/internal/wsagent/record.go#L130-L144`,
  `agents-plugin-tool/internal/wsagent/record.go#L242-L259`,
  `agents-plugin-tool/internal/wsagent/spawn.go#L305-L365`,
  `agents-plugin-tool/internal/wsagent/children.go#L1001-L1020`).
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
- **Minimal fix:** implement (1926): add the harness-neutral MCP tool
  `path.write(path, content)`, authorize it only for a cache path that the same
  session key previously minted through `path.generate`, and expose it through
  `ChildSafeWSTools`. The read-only reviewer, not its invoker, calls
  `path.generate`, writes the detailed report through `path.write` with that
  same key, and returns a one-line status containing the path for the invoker
  to read. The render-minted child key at
  `agents-plugin-tool/internal/mcp/playbook_tools.go:1082-1099` or another
  reviewer key is valid, but one key must perform both calls. Test clean,
  ordinary non-clean, oversized, unminted-path, cross-session, and
  parent-minted-path attempts.
- **Compatibility:** **T0.** `path.write` is an additive tool, and the playbooks
  that call it ship embedded with the same binary. Existing tools, arguments,
  hello data, mailbox records, and persisted-history shapes remain unchanged.

### 6. Worker instructions require tools their child profile withholds

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
- **VERIFIED:** the non-blocked `route.resolve_implement` Prep todo requires
  `infra.read("impl-playbook")`, and `infra.read` is a read-only bundled-document
  tool that delegate authorization permits
  (`agents-plugin-tool/internal/mcp/session_state.go#L454-L474`,
  `agents-plugin-tool/internal/mcp/server_schemas.go#L557-L568`,
  `agents-plugin-tool/internal/mcp/server_dispatch.go#L1125-L1128`,
  `agents-plugin-tool/internal/mcp/server_inventory.go#L220-L236`). It is also
  absent from `PiMiniLeadWSTools`, `ChildSafeWSTools`, and `AgentWSTools`, so the
  common Codex, Claude, agy, and Pi agent-worker profiles cannot satisfy that
  emitted instruction
  (`agents-plugin-tool/internal/wsagent/builtin.go#L111-L190`,
  `agents-plugin-tool/internal/wsagent/builtin.go#L276-L290`).
- **Develop verdict:** **not fixed.** The canary worker's missing tools match
  the current playbook/profile mismatch exactly.
- **Minimal fix:** implement (1927), (1928), and their source-derived
  `infra.read` consequence. Remove the impossible `workflow_manual` call
  from worker instructions and treat the rendered Worker Protocol, ticket,
  AGENTS guidance, declared manuals, and route-emitted infra document as the
  worker's context. Add the already-authorized `config.resolve_agent` and
  `infra.read` to the harness-neutral child-safe and agent-worker surfaces.
  Regress required ws tool references in worker playbooks and deterministic
  route output against the effective Codex, Claude, agy, and Pi child profiles.
- **Compatibility:** **T0.** This exposes existing read-only tools to a role
  that the server already authorizes and removes a bad instruction; it changes
  no MCP schema, argument, hello, or persisted record. `workflow_manual`
  remains excluded, so no child gains a lead-key mint path
  (`ai-docs/manuals/release-compatibility.md:73`).

## Decisions

- **(1924) Child worktree binding:** A child key stays bound to the root where
  it was minted. An attempt to use an A-bound key in B receives an actionable
  error requiring a B-bound key. The caller may spawn directly into B or, once
  `261116-feat-ws-subagent-worktree-lease` lands, use the B-bound key returned
  by a legitimate child acquisition under its Decision (1190); the old key is
  never rebound. Rejected: an adopt/rebind lifecycle.
- **(1925) Stale implementation branch ownership:** Preserve both existing
  resolver-owned `recreate` actions: the vouched different-target action from
  `261116-bug-ws-worktree-acquire-stale-branch-facts` Phase 2 (1184) at
  `implement_resolver.go:951-957`, and the create-path action at `:872-886`.
  They remain necessary for in-root serial workers, which never call
  `worktree.acquire`; this decision supersedes nothing in `261116` and
  complements it. Add classify-and-recreate during locked pooled-worktree
  acquisition. A name-rooted canonical target is stale and safe to recreate
  only when comparison with its encoded merge root, not an arbitrary caller
  base, succeeds with target-only `0` and merge-root-only `>0` (ahead `0`,
  behind `>0`); `0/0` stays equal, and any target-only `>0` branch is preserved
  and checked out as today, including a divergent branch and its existing
  behind warning. This predicate also covers a freshly created empty canonical
  branch left behind after its merge root advances, so neither classification
  nor diagnostics may claim it proves previously landed work or work loss. Use
  non-forced deletion after switching the pooled worktree to the encoded merge
  root; if another pooled worktree has the branch checked out, acquisition fails
  actionably, leaves the branch and both worktrees intact, and tells the caller
  to release that checkout and re-acquire. Route resolution adds no repair: it
  changes only the exact-target `continue` case at `:932-935` to refuse this
  stale predicate and tell a pooled caller to re-acquire or an in-root caller
  to switch the root to the merge root and route again, which reaches the kept
  create-path `recreate`. The scope-limited rejection of auto-resetting stale
  branches in `261116-bug-ws-worktree-acquire-stale-branch-facts` Phase 1 is
  lifted only for the target-only `0` case. Rejected: removing the
  existing route recreation paths or broad route-owned repair.
- **(1926) Read-only reviewer artifact write:** Add the harness-neutral ws MCP
  tool `path.write(path, content)`. It writes only to a ws-cache path that the
  same session key previously minted through `path.generate`; it is included in
  `ChildSafeWSTools` so read-only children can call it. The daemon-side write
  runs outside the harness sandbox, while the child retains zero source-tree
  write authority. The reviewer mints its own findings path with
  `path.generate` under the key it uses for ws calls, writes the full report
  with `path.write` under that same key, and returns only a one-line status that
  includes the path; the invoker does not pre-generate a path and reads it from
  that line. `playbook_tools.go:1082-1099` may splice a render-minted child key,
  and a spawn-issued reviewer key may also be used, but path generation and
  writing must use one identical key. Compatibility is T0: the tool is additive
  and the calling playbooks ship embedded with the same binary. Rejected:
  having the handler persist a bounded handoff; authorizing parent-minted paths
  by session-key lineage; a session allow-list tool (`allowed.edit`), because it
  creates a new authority path for deciding who sets the list.
- **(1927) Worker bootstrap context:** Remove the lead-only `workflow_manual`
  requirement from worker instructions; workers rely on the rendered Worker
  Protocol, ticket, AGENTS guidance, and declared manuals. Rejected: a new
  child-safe context tool.
- **(1928) Delegate tier resolution:** Expose the existing read-only
  `config.resolve_agent` to delegate children. The same settled child-surface
  rule covers the existing read-only, delegate-authorized `infra.read` required
  by deterministic route output. Rejected: redesigning delegation.

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

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)

## Prior Decisions

- 261120-bug-ws-agent-dogfood-delivery-context (2026-11-20, Result): "Every spawned worker context now names its direct parent's `id:<reply-id>` at the shared instruction seam." — bearing: supports
- 261116-bug-ws-worktree-acquire-stale-branch-facts (2026-11-16, commit): "Recreation starts from the encoded merge root, not the vouched current tip; deletion stays non-forced." — bearing: contradiction-candidate
- 114f104d4 (2026-11-20, commit): "Shipped agy settings in a separate namespace manifest because adding an agy key to the existing claude namespace manifest would make older readers reject it." — bearing: supports
- 260912-bug-ws-pi-readonly-reviewer-artifact-contract (2026-09-13, Result): "Reviewers retain the read-only tool surface plus scoped native edit/write wrappers for only that file." — bearing: constrains
- c6a2d1b3 (2026-11-19, commit): "Defined one harness-neutral allowlist (ChildSafeWSTools in internal/wsagent/builtin.go) and ensured AgentWSTools includes all entries." — bearing: constrains
- c4dcf460 (2026-11-20, commit): "Rejected a review-handler-only profile because agent.spawn has one common backend-neutral loadout and no handler role signal." — bearing: supports
- 924d473e (2026-09-09, commit): "The key is still a child (parent = caller key) bound to the caller's root." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp, agents-plugin-tool/internal/wsagent, agents-plugin/rsrc, agents-plugin-wsflow/rsrc, agents-plugin-pi/src, and package manifest tests |
| scope.surface | public-interface | new MCP tool path.write(path, content), child tool visibility, diagnostics, and shipped worker/reviewer instructions are caller-visible |
| scope.new_public_symbol | yes | additive MCP tool path.write |
| scope.new_type_contract | yes | path.write(path, content) plus same-session path.generate provenance and exact generated-cache-path authorization |
| scope.test_surface | existing | internal/mcp server and route tests, internal/wsstate generated-path tests, internal/wsagent child-surface tests, package runtime/hosted-package tests, and Pi policy tests |
| complexity.reuse_points | confirmed | path.generate allocation, session-key root lookup, worktree acquisition lock, ChildSafeWSTools/AgentWSTools, and existing manifest/config loaders |
| complexity.side_effect_risk | high | the ticket combines branch recreation, daemon-side file writes, mailbox delivery, manifest loading, and four harness child surfaces |
| risk.correctness | high | a stale-branch misclassification can discard or reuse work, and an artifact or delivery error can lose review findings |
| risk.fit | high | six phases cross MCP dispatch, wsstate, wsagent profiles, shipped playbooks, plugin packaging, and Pi compatibility |
| risk.test | high | branch topology, same-session path provenance, clean/non-clean/oversized reports, once-only mail, manifest parity, and four harness profiles need coverage |
| risk.security_or_contract | high | path.write adds a session-scoped file-write authority whose cache containment and source-tree exclusion are security boundaries |

## Phases

### Phase 1: Make child worktree binding explicit

Implement the settled lifecycle without weakening root authority: retain
immutable child keys, reject later cross-worktree assignments with a
diagnostic that requires a key minted for the destination, and teach lead/child
guidance both valid paths: spawn directly into the destination or, after
`261116-feat-ws-subagent-worktree-lease` lands, use the destination-bound key
returned by the child's own legitimate acquisition. Do not present respawn as
the sole remedy.

Acceptance:

- A child spawned in worktree A cannot ws-commit in worktree B and receives an
  actionable diagnostic to obtain and use a B-bound key.
- A child spawned with `worktree: B` can use its minted key for B.
- If `261116-feat-ws-subagent-worktree-lease` has landed, a child that
  legitimately acquires B can use the returned key for B without privilege
  escalation under that ticket's Decision (1190); otherwise this second valid
  B-key source and its regression test are deferred.
- Existing same-root child calls remain unchanged.
- Compatibility remains T0, or the phase records and settles a revised tier
  before implementation.

### Result (c06d6031a) - 2026-11-21

Spawned-worker context now states that its session key is immutably bound to
the selected root and directs a moved assignment to use a destination-minted
key. Explicit `worktree: B` spawn is regressed as minting B's root, while
same-root behavior remains unchanged. Lead guidance covers direct spawn into B
and the future legitimate B-key acquisition path without implying key rebind.
`261116-feat-ws-subagent-worktree-lease` remains in `ready`, so its acquired
B-key source and regression are deferred exactly as Decision (1924) permits.
Focused `internal/wsagent` spawn/context tests passed with the required offline
Go settings. Compatibility remains T0.

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

### Result (3d7d4ffed) - 2026-11-21

End-to-end regressions now send from direct and nested children to the exact
injected direct-parent reply id, exclude worktree-inbox fallback, and prove the
hosted lead claims the message once as either an active steer or a later turn.
A subsequent manual receive is empty. The tests exposed no production wording
ambiguity, so this phase is test-only and remains T0. Focused `internal/mcp`
mail routing and `internal/wsagent` hosted-delivery tests passed with the MCP
package using `-timeout 45m`.

### Phase 3: Retire fully landed canonical implementation branches

Make pooled provisioning and route resolution distinguish a retained active
branch from a stale canonical branch with no commits outside its encoded merge
root. Perform pooled branch recreation under the existing acquisition lock,
retain the resolver's existing recreate instructions for in-root serial work,
and never discard unmerged work.

Acceptance:

- During pooled-worktree acquisition, a name-rooted canonical target with
  target-only `0` and merge-root-only `>0` against its encoded merge root is
  handled by first detaching the pooled worktree at that merge-root commit
  (`git switch --detach refs/heads/<merge-root>`), then non-force deleting
  (`git branch -d`) and recreating the target from it (`git switch -c <target>
  <merge-root>`) under the acquisition lock instead of checking it out stale.
  The real-Git regression runs with the merge-root branch checked out in the
  main root. This safe-to-recreate class includes a
  formerly empty branch after the merge root advances and is not evidence that
  the branch previously carried landed work.
- Equal targets and targets with target-only `>0`, whether actively ahead or
  divergent, are checked out as today and are neither recreated nor refused;
  divergent targets retain the existing behind warning.
- If another pooled worktree checks out the stale target, non-forced deletion
  fails the acquisition without mutation and the error identifies the blocking
  checkout and tells the caller to release it and re-acquire.
- Route resolution retains the existing create-path and vouched
  different-target `recreate` actions from
  `261116-bug-ws-worktree-acquire-stale-branch-facts`; regress that an in-root
  serial worker still receives and can execute those actions. Only an
  exact-target `continue` that meets the stale predicate becomes a defensive
  refusal whose message tells a pooled caller to re-acquire and an in-root
  caller to switch the root to the merge root and route again, reaching the
  kept create-path `recreate`.
- The exact `261115-feat-ws-permission-profile` topology is covered by a
  regression fixture.
- Compatibility remains T0.

### Result (1565da461) - 2026-11-21

Locked pooled acquisition now classifies an existing canonical target against
an existing encoded local merge root, preflights every registered checkout,
and safely recreates only `ahead=0, behind>0` branches by detaching at the
merge-root ref, non-force deleting, and recreating. Equal, ahead, divergent,
and legacy `impl/*` names without a local encoded root preserve prior behavior.
Exact-target routing defensively stops the stale predicate while both existing
resolver recreation paths remain intact. Real-Git coverage includes the
permission-profile topology and held-branch no-mutation case. The full focused
worktree/resolver MCP matrix passed with `-timeout 45m`; compatibility remains
T0.

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
- The shipped `lead-tune` caller performs `config.list` before using the
  additive key against an unknown daemon version
  (`agents-plugin/rsrc/lead-tune/lead-tune.md:29` and its wsflow and Pi
  mirrors); tests pin that catalog gate before its `config.tune` call. There is
  no separate agy-key-specific production caller to gate. This preserves a T0
  old-call path instead of relying on a T3 refusal.
- An unavailable shipped key points to rejected manifests or the need to
  restart/update the plugin session where that evidence exists.
- Compatibility remains T0 for old calls and stored data; any ungated
  new-caller-to-old-daemon path is recorded as T3 and requires explicit owner
  approval.

### Result (ee8af28ee) - 2026-11-21

Exact ws/wsflow Claude and Codex contracts now require the three shipped
manifests and their files; the hosted fixture includes the agy manifest. Claude
hello coverage lists, gets, and project-tunes
`agy.model_wait_timeout_seconds`, while absent/rejected catalog diagnostics and
the shipped `config.list` gate are pinned. Targeted `internal/mcp`,
`cmd/ws-mcp`, ws, and wsflow package tests passed. Old calls and stored data
remain T0; the caller gate avoids an ungated new-call/old-daemon path.

### Phase 5: Add a cache-scoped reviewer report writer

Add `path.write(path, content)` as a harness-neutral daemon-side MCP operation.
The session key may write only an exact ws-cache path that it previously minted
through `path.generate`; no repository path or path minted by another session
is eligible. Include the tool in `ChildSafeWSTools`. Update
`agents-plugin/rsrc/code-reviewer.md:45` and its wsflow/Pi mirrors so the
read-only reviewer mints its own findings path, writes the detailed report, and
returns only a one-line status containing that path. The review handler at
`agents-plugin/rsrc/review-handler/review-handler.md:53,90` and its wsflow/Pi
mirrors is not a pre-generated-path invoker: it keeps minting and writing its
own graded findings report with `path.generate` and `path.write` under its own
key, even when it runs read-only, and keeps returning `findings: <path>`. It
reads each partition reviewer's returned findings path as input. Update the Lite
invoker in
`agents-plugin/rsrc/worker-stop-protocol.md:70-84` and its mirrors, which does
not currently pass a findings path, to read the returned path rather than add a
parent-generated one.

Acceptance:

- `path.write` accepts an exact generated cache path only when that same session
  key previously obtained it from `path.generate`.
- Unminted paths, paths minted by another session (including a parent key in
  the writer's lineage), repository paths, traversal, aliases, and symlink
  escapes are refused without modifying a file.
- A read-only child can call `path.write` through every effective
  `ChildSafeWSTools` host surface but receives no source-tree write authority.
- Non-clean, clean, and oversized Lite reviews each leave a complete non-empty
  reviewer-minted findings artifact; the reviewer final response is one status
  line containing its path, and each invoker reads the full artifact from that
  returned path.
- The review handler continues to create and write its own graded findings
  report with `path.generate` and `path.write` under its own key when it runs
  read-only, returns `findings: <path>`, and reads each partition reviewer's
  report from that reviewer's returned path; `lead-review` continues forwarding
  the handler's findings path.
- Whether the reviewer uses the render-minted child key from
  `playbook_tools.go:1082-1099` or another reviewer key, the same key performs
  both `path.generate` and `path.write`.
- The write occurs daemon-side and does not depend on a harness-specific file
  write grant; ws/wsflow mirrors and Pi guidance remain aligned.
- Compatibility remains T0 because the tool is additive and its calling
  playbooks ship embedded with the same binary.

### Result (1b0899f66) - 2026-11-21

Added additive `path.write(path, content)` with exact same-key provenance that
survives per-connection server replacement but is not persisted. Only generated
review/prompt cache files are eligible; plan/clone, repository, alias,
cross-session, parent-minted, and symlink-replaced paths are refused. A held
descriptor closes the parent-symlink race, grants are oldest-first bounded at
64 and consumed on write, and the transport accepts complete 17 MiB reports
under a finite 64 MiB request ceiling. Reviewers mint and write their own
artifacts; the handler separately writes its consolidated report and invokers
read returned paths. Focused MCP, wsagent, wsstate, resource-mirror, runtime,
and package tests passed. The live pre-change daemon did not yet expose
`path.write`, so the required Lite reviewer returned its full findings inline;
both Important findings were fixed in this result. Compatibility remains T0.

### Phase 6: Align worker instructions with the spawned-child tool surface

Remove the lead-only bootstrap call from the Worker Protocol and expose the
existing read-only tier resolver that elevated workers need for delegation,
plus the read-only infra document reader required by deterministic route output.
Keep all lead-key mint paths unavailable to children.

Acceptance:

- Rendered ticket-worker instructions do not tell spawned children to call
  `workflow_manual` or any other intentionally unavailable lead-only tool.
- `config.resolve_agent` and `infra.read` are visible and callable with a
  delegate key under the effective Codex, Claude, agy, and Pi agent-worker
  profiles, including the Codex child-safe MCP map.
- A contract test compares required ws tool references in worker playbooks and
  deterministic route output against each effective child surface and fails on
  another instruction/exposure drift.
- Leaf keys retain their current config restrictions, and no child can mint or
  adopt a lead key.
- ws/wsflow mirrors stay aligned.
- Compatibility remains T0.

### Result (c06d6031a) - 2026-11-21

The rendered Worker Protocol no longer requires the intentionally unavailable
`workflow_manual`. `config.resolve_agent` and `infra.read` are now in the
harness-neutral child-safe surface inherited by Codex, Claude, agy, and Pi
agent-worker profiles; `workflow_manual`, config mutation, and lead-key minting
remain excluded. Render/profile regressions pin the elevated resolver call and
all four effective profiles. Required wsagent, rendered-playbook, wsflow/Pi
mirror, and runtime-contract tests passed. Dogfooding reproduced the original
gap: this worker's pre-change live profile lacked `workflow_manual`,
`config.resolve_agent`, and `infra.read`, so it proceeded from the rendered
protocol, ticket, AGENTS guidance, and declared manuals as Decision (1927)
requires. Compatibility remains T0.


## Resolution (2026-11-21)

Implemented all six phases on `impl/develop/stung-sushi-peso`. Immutable root-bound worker keys now carry destination-key guidance; parent mail delivery is pinned exactly once; stale canonical implementation branches are safely recreated under lock; Claude agy manifest exposure is exact; read-only reviewers own bounded cache artifacts through additive `path.write`; and worker instructions match the child-safe tool surface. The lease-dependent acquired-B-key regression remains deferred by Decision (1924) because its prerequisite ticket has not landed.
