---
title: "Give the Pi execute-worker structured git read tools so routine inspection stops elevating"
related:
  260904-feat-ws-pi-execute-approval-gateway: origin; its Decisions §2 (per-mutation gate invariant) and §5 (tool-shape allowlist, no command-string matching) bind this ticket
  261003-research-typed-decision-model-new-domains: context; this structural fix was taken first in that discussion
  261003-feat-pi-execute-readonly-auto-approval: follow-up; read-only auto-approval classifier sequenced after this ticket
---

# Give the Pi execute-worker structured git read tools so routine inspection stops elevating

## Background

A `ws-execute`-spawned execute-worker reads freely only through `read`,
`grep`, `find` and `ls` (`TOOL_GROUPS["execute-worker"]`,
`agents-plugin-pi/src/spawner.ts:163`). Every other shell command goes through
`ws-worker-exec`, which pauses the worker and injects an approval request into
the lead's session. That costs a lead turn per command, and the binding anchor
`260909-research-ws-refoundation-evidence-audit` (A3) names lead turns as the
scarce resource.

**What the history shows.** Local Pi lead sessions hold 327 historical approval
requests. A rough regex split:
- 115 requests (~35%) were non-mutating inspection.
- 76 of those were git-only inspection (`git status` / `log` / `diff` and similar).
- Of the 76, 18 targeted another worktree via `git -C <path>`.

So this ticket can remove about 58 requests (~18%).

**Why this gap is unintended.** `260904` §5 already chose structured,
mutation-incapable read tools as the way to let the worker inspect without
elevation. ws-mcp has read-only git tools, and full-worker children already use
them heavily (`ws__git_diff` 911 calls and `ws__git_log` 480 calls across 751
child sessions). The execute-worker group simply never received them.

## Decisions

- **Tool-shape allowlist only.**
  - The execute-worker receives ws tools through the existing positive read-only inventory, `READ_WS` / `readOnlyWsTools` in `agents-plugin-pi/src/delegation-policy.ts`.
  - No command-string matching is introduced anywhere.
  - `READ_WS` is the inventory under which newly shipped mutators never become read authority by default.
  - Rejected: a command-string allowlist of "safe" shell commands. `260904` §5 rejects it as a smuggling arms race.
- **Git subset only.**
  - The execute-worker gets `git_status`, `git_diff`, `git_log` and `git_merge_base` from `READ_WS`, not the whole inventory.
  - The approval history shows demand only for git inspection, and the execute-worker runs on a light model by default, so every extra tool schema costs prompt tokens. Widening later is a one-line change.
  - Rejected: all of `READ_WS`, or the git subset plus `project_tree`.
- **`ws-worker-exec` is unchanged.**
  - Every free-form shell command still elevates to lead approval, including non-git inspection (`date`, `env`, smoke scripts).
  - This keeps `260904` §2's per-mutation gate invariant within this ticket. Read-only auto-approval is owned by `261003-feat-pi-execute-readonly-auto-approval`.
- **Worker guide.**
  - `agents-plugin-pi/execute-worker-guide.md` "Your tools" lists the granted git read tools and directs routine git inspection to them instead of `ws-worker-exec`.
  - The guide is the worker's fixed system prompt and today says `ws-worker-exec` is the only way to run a shell command. Without this line the worker keeps elevating `git status`.
- **Separate classifier ticket.**
  - The decision-model classifier is split out to its own ticket, which takes this one as prerequisite.
  - This ticket stays adapter-only with no external dependency.

## Constraints

- **Cross-worktree inspection is out of scope.**
  - `git -C <other worktree>` requests stay gated here.
  - The reason is that ws git read tools resolve their root only from `session_key` (`resolveToolRoot`, `agents-plugin-tool/internal/mcp/server.go:3511`) and expose no root argument. Covering the case needs a ws-mcp schema change to shipped tools, which deserves its own ticket once the residual is measured.
- **Shipped-surface rules apply.** Edits under `agents-plugin-pi/` follow `ai-docs/manuals/shipped-surface-boundary.md`.

## Phases

### Phase 1: Grant git read tools to the execute-worker

**Scope:**
- Extend the execute-worker tool group and its spawn admission
  (`resolveTools` / `resolveSpawnAdmission` in `agents-plugin-pi/src/spawner.ts`)
  so the child receives the four granted `ws__` git read tools, filtered
  through `readOnlyWsTools`.
- Update the execute-worker guide's tool list.

**What stays unchanged:**
- `ws-worker-exec` gating.
- The `ws-execute` / `ws-approve` protocol.
- The full-worker group.

**Verification:**
- A unit test shows execute-worker spawn admission (tool list and delegation
  ceiling) includes exactly the four granted `ws__` git read tools and no `ws__`
  mutator (e.g. `ws__git_commit`, `ws__git_merge`).
- A test or probe shows an execute-worker's bridged git read call resolves the
  lead's repository root through its session key. Execute-worker session-key
  injection is unverified, while full-worker children demonstrably work. A
  granted tool that cannot resolve a root removes no approvals.

**Rejected alternatives:**
- A decision-model auto-approval classifier in this ticket. It is split out to `261003-feat-pi-execute-readonly-auto-approval`.
- A command-string allowlist (`260904` §5).
