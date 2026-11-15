# Explicit CI long-test group

## Authority

MAIN assigned one bounded CI and project ship-config change from the complete approved brief `/tmp/ci-long-tests-brief.md`. Owner directive: wsledger and wstrackerhost tests run only when explicitly requested. Branch `ci/long-test-group`, based on local develop, uses a full pooled worktree. Target handoff is approximately 22:18 KST on 2026-11-15. MAIN owns merge, push, CI dispatch, and shared runtime; retain the lease until its merge report.

## Intended changes

1. Define the long group once in `agents-plugin-tool/scripts/ci-test-shard.sh`: `internal/wsledger` and `internal/wstrackerhost`. `all` and `rest` exclude it; new `long` runs only it. Preserve total coverage by rest + every MCP shard + long, documenting the partition. Keep macOS /bin/bash 3.2 compatibility (no mapfile, associative arrays, or newer Bash syntax).
2. Add boolean `workflow_dispatch.long_tests`, default false, to `.github/workflows/ws-mcp-ci.yml`; add Ubuntu/Windows long jobs, with existing setup and Defender handling, conditional on that input and never on PRs. Inspect `.github/workflows/ws-mcp-release.yml` and cite why a successful CI run with skipped long jobs satisfies its tagged-commit gate; adjust only if actually required.
3. Update only this project's `ai-docs/ship/ws.md`, not shipped lead-ship playbooks. Before the final release gate, ask the owner via the whiteboard whether to run long tests. Default no: local pre-flight uses the shard script excluding the group, long CI legs may be skipped, every other leg must succeed. Yes: add local long tests and dispatch `-f long_tests=true`; require long legs to succeed. Preserve all other gates.
4. Update directly affected contract tests and drifted manuals referencing these commands or matrix. No unrelated semantics.

## Verification and boundaries

Read applicable project manuals before edits; use exact source and discovered tests as evidence. Run `rest -list .`, `all -list .`, and `long -list .` under macOS `/bin/bash`, capturing package result lines to demonstrate exclusion and exact long membership. These are inventory runs, not authorization to execute the long suites. Parse workflow YAML with available actionlint or a YAML parser. Run focused relevant Go tests with `-run` and `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`; never entire internal/mcp, internal/wsdashboard, or the full Go suite. No network, providers, credentials, downloads, installation, branch push, CI dispatch, shared daemon operations, or unrelated process signals. Preserve any unrelated stashes; do not apply, drop, or clean them.

Commit this plan unchanged with the implementation. Commit AI Context records the owner directive verbatim from the brief and T0 compatibility (CI/docs only, no cross-version surface). Report exact head, complete commands/output artifacts, partition evidence, new input/job condition, release gate file:line citations, and any limitations. Return at a clean checkpoint for the lead's evaluation and MAIN handoff. Stop before scope expansion.
