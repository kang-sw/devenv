# Plan: `ws-claude`: delegate bounded judgment tasks to Claude Code subagents (fan-out, edit-scoped, resumable) — Phase 1: read-only delegation with fan-out and stable handles; mandatory pre-build probe first

## Relevant Ticket Contract
- Phase 1 adds an adapter-registered `ws-claude` with only read-only `audit` and `consult` presets, array-in/index-aligned-array-out fan-out, `Promise.allSettled`, a small concurrency cap, and collision-free three-word public handles.
- The child is Claude Code's own harness: use its normal `claude_code` preset plus only a small natural task-frame append. Never forward Pi's system prompt, a ws session credential, or an account/lead MCP configuration.
- The child profile is closed: `strictMcpConfig: true`, with no Bash, git, or connector MCP servers. The prior spike found `settingSources: []` alone still attached account connectors.
- A real pre-build viability probe is mandatory and unresolved. It must run before source/dependency work, but is distinct from the owner-run post-build audit/consult and closed-profile acceptance checks.

## Out of Scope
- Phase 2 `rewrite`, `edit-targets`, canonical containment, `canUseTool`, and changed-file reporting.
- Phase 3 continuation/resume storage and `design-review` context/playbook adaptation.
- Restoring the reverted Claude-as-a-Pi-model provider, changing ws-mcp/shared playbooks, Pi native spawning, account setup, or billing-policy claims.

## Codebase Findings
- `agents-plugin-pi/package.json#L20-L24` — the Pi package has no Claude Agent SDK dependency, and `agents-plugin-pi/node_modules/` has no SDK package; the exact future SDK seam cannot be run from the current checkout without a dependency change.
- `agents-plugin-pi/src/index.ts#L1-L31` — the extension owns adapter-specific Pi registration while preserving ws-mcp as harness-neutral; a new leaf tool belongs in this package rather than ws-mcp.
- `agents-plugin-pi/src/spawner.ts#L186-L236` — existing Pi-native delegation is separately registered and has different lifecycle semantics; `ws-claude` must remain a distinct tool rather than extending the spawner.
- `agents-plugin-pi/test/native-tool-registration.test.ts#L1-L72` — registration can be tested by capturing `pi.registerTool` definitions without invoking a model.
- `ai-docs/tickets/.done/260908-research-ws-pi-claude-code-lead-provider.md#L144-L150` and `#L176-L181` — the prior isolated SDK spike established `strictMcpConfig: true` as necessary, but its provider conclusion was superseded by the later real-Pi classification failure; it cannot clear this ticket's new probe.
- `ai-docs/manuals/skill-authoring.md#L29-L98` — embedded audit/consult task frames are prompt authoring and must use its invariant/handler structure, with no tool-schema duplication.

## Implementation Plan
1. **Owner pre-build decision, before implementation:** run exactly one throwaway Claude-owned probe, then stop. In a new empty temporary directory, prepare a one-sentence natural task-frame file and an empty MCP-config file. Launch the installed `claude` CLI with its default Claude Code preset, `--append-system-prompt-file <task-frame>`, `--strict-mcp-config --mcp-config <empty-config>`, `--max-turns 1`, and a trivial text-only request such as “Reply with exactly: probe-ok”. Start it through a minimal environment that retains only normal CLI-auth prerequisites (including `HOME`, `PATH`, locale/user fields as needed) and deliberately omits all `WS_*`, Pi prompt, and host-integration variables; do not print that environment or auth/account data. Enforce a 120-second wall-clock timeout and capture only exit class, whether the literal `400 out of extra usage` classification occurred, and a bounded success/failure summary.
2. Treat the single probe result as a binary build gate: a normal text response permits the subsequent implementation plan to proceed; the known 400/refusal, timeout, or inconclusive launcher failure stops before code/dependency edits and returns the sanitized result to the owner. Do not trim, rewrite, or iterate the prompt to evade classification, and make no billing guarantee from a passing result.
3. If the gate passes, add a Pi-extension-local Claude execution module and registration in `agents-plugin-pi/src/index.ts`, following the captured-registration test pattern. Keep the Phase 1 item schema to `preset`, `request`, optional read paths/model, and no edit targets; implement only `audit`/`consult` embedded frames using the skill-authoring invariants.
4. In that module, use the Claude Agent SDK/CLI integration selected during implementation to preserve the Claude-owned preset and append only the embedded task frame. Construct an explicit closed tool/MCP profile (`strictMcpConfig: true`; no Bash, git, exec, or connector MCP) and scrub child process inheritance of ws credentials and Pi prompt material while retaining the user's ordinary Claude Code authentication path.
5. Implement a capped worker pool and `Promise.allSettled` join that preserves input order. Give each item a collision-free three-word public stem and return isolated per-item success/error results. Apply a finite per-item timeout and enclosing cancellation propagation; terminate/clean up every active child and listener while allowing settled siblings to remain reported.
6. Add fake-SDK tests beside the new module plus registration coverage in `agents-plugin-pi/test/`: result alignment under fan-out, one-item failure isolation, timeout of a never-settling child, cancellation/process cleanup, unique stable-handle formatting, and the absence of closed tools/MCP connectors in the child options. Keep model calls out of unit tests.
7. **Post-build owner-run acceptance, separate from the pre-build probe:** on subscription login, invoke the completed `ws-claude` against a real ticket with one `audit` and one `consult` item. Confirm returned outputs and index alignment, and inspect the spawned agent's available tools to confirm git, Bash, and connectors are absent. This acceptance does not substitute for the pre-build gate and should not be run during the survey.

## Verification Plan
- Pre-build: the single 120-second, text-only CLI probe described above is runnable in the current environment (`claude` 2.1.265 is installed; its authentication-status command succeeds without exposing account details; `--append-system-prompt-file` and `--strict-mcp-config` are available). It is pending owner execution; no model call ran in this survey.
- Unit: run the focused new `node --test` file(s), then `cd agents-plugin-pi && npm test` after implementation.
- Owner acceptance: run one real read-only `audit` plus `consult` against a ticket and verify the closed tool/MCP profile. Record only sanitized outcome/usage categories, never prompts, credentials, account details, or environment values.

## Escalations
- Confidence: medium.
- Reason: implementation design is bounded, but the ticket makes the subscription-path classification a hard pre-build runtime gate. The present worktree lacks the Agent SDK, so the currently runnable CLI probe can establish the Claude-owned prompt classification but cannot by itself prove the future SDK wrapper's option mapping.
- Research should decide: only if the one bounded probe is inconclusive for reasons other than the known refusal, whether a temporary non-repository SDK harness is necessary before adding the dependency. A refusal is a stop condition, not a prompt-iteration experiment.
- Owner decision required: whether to execute the described subscription-consuming probe. The required post-build live audit/consult acceptance remains owner-run and must not be treated as completed by this plan.
