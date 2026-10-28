# Pi batch-summary language hotfix

## User request

After enabling agent-widget batch summaries, the user reported Japanese output despite workflow.lang being Korean and requested a prompt hotfix. The user explicitly requires execution in a separate worktree.

## Existing behavior and settled scope

`agents-plugin-pi/src/batch-summary.ts` already includes workflow.lang through languageLine(config.lang) in the user message. The system prompt is currently fixed. Strengthen the existing language instruction by including the same effective configured language in the system prompt for each request. Preserve the user-message behavior and unset-language fallback. This is prompt hardening, not a verified diagnosis of the Japanese output.

## Allowed changes

- agents-plugin-pi/src/batch-summary.ts
- agents-plugin-pi/test/batch-summary.test.ts
- This write-once plan, committed with the change.

Keep the implementation inline or reuse existing private helpers; introduce no new exported symbol, API, knob, module, or style-setting behavior. Do not change configuration-read failure semantics, retry behavior, summary caches, model/effort selection, or widget rendering.

## Verification

Extend focused request-construction coverage to assert that the provider system prompt contains the effective workflow.lang, that successive requests follow a changed language rather than capturing an old value, and that the existing unset-language fallback remains intact. Run focused batch-summary tests plus appropriate package type checking if available. Use the package's documented commands and report exact results. No live provider calls or image generation.

## Boundaries

Work only in /Users/kang-sw/.ws-worktrees/devenv/decree-thinness-strum on fix/pi-batch-summary-language. Read AGENTS.md and relevant authoring/shipped-surface manuals before editing prompt strings. Do not alter the root checkout, tickets, unrelated files, or configuration. Commit the minimal change with AI Context and return the commit, tests, and any limitations. No merge or push is authorized. Stop and report if the minimal scope is insufficient.
