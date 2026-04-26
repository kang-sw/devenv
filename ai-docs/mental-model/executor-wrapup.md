---
domain: executor-wrapup
description: "Shared wrapup playbook for executor-series skills: doc pipeline, commit gate, ticket update."
sources:
  - claude/infra/
  - claude/bin/
  - claude/skills/edit/
  - claude/skills/implement/
  - claude/skills/sprint/
related:
  workflow-routing: "Executor skills are the implementation-phase targets that /proceed routes to; wrapup runs after their implementation commits."
---

# Executor Wrapup

`executor-wrapup.md` is a shared infra playbook loaded by all executor-series
skills (`ws:edit`, `ws:implement`) at the end of their
doc-pipeline step. It handles three responsibilities:

- `§Doc Pipeline` — refresh `ai-docs/_index.md`.
- `§Doc Commit Gate` — auto-commit any dirty `ai-docs/` state after doc-pipeline outputs.
- `§Ticket Update` — append result entries and move tickets when ticket-driven.

## Entry Points

- `claude/infra/executor-wrapup.md` — the canonical playbook; the single source of truth for all three responsibilities above.
- `claude/skills/edit/SKILL.md` §Step 5 — shows how `ws:edit` dispatches updaters before calling executor-wrapup.
- `claude/skills/implement/SKILL.md` §Step 7 — shows how `ws:implement` calls executor-wrapup directly post-merge.
- `claude/skills/sprint/SKILL.md` §On:wrap-up — shows how `ws:sprint` defers the doc pipeline to a single session-end wrap-up and calls executor-wrapup after spec/mental-model updaters complete.

## Module Contracts

- `executor-wrapup` guarantees: the doc-commit gate always runs, even when no doc changes are expected. Prior steps may have dirtied `ai-docs/` without the executor knowing.
- `executor-wrapup` guarantees: it does **not** dispatch `ws:spec-updater` or `ws:mental-model-updater`. Updater dispatch is the caller's responsibility, following the pattern for that executor series.
- The doc-commit gate uses `git status --porcelain ai-docs/` as the trigger. A non-empty result mandates a commit; an empty result is a no-op.
- `executor-wrapup §Ancestor Loading` invariant: when a skill loads a sub-domain mental-model doc (`mental-model/<domain>/<sub>.md`), it must also load `mental-model/<domain>/index.md` first. `ws-list-mental-model` auto-emits the parent `index.md` alongside any direct-child sub-domain — callers using it need no manual action. Callers using manual paths must read the parent before the child. Subagent spawn prompts must include the ancestor loading rule verbatim so the subagent observes it inside its own read ordering.
- `ws:implement` file-based review protocol: reviewers write full findings to pre-allocated file paths; each reviewer returns only a `[clean|non-clean]: <one-line summary>` to the lead. The lead reads summaries only. When non-clean, the lead relays the file paths (not content) to the implementer. The implementer reads files directly. On re-review, reviewers overwrite the same paths.
- `ws-review-path` guarantees: paths are non-reproducible after the call returns (they embed a per-call `run_id`). The lead must invoke `ws-review-path <stem1> <stem2> ...` once in a single Bash call and capture all output lines. Re-invoking `ws-review-path` with the same stems yields different paths.
- `ws-review-path` cleanup: `ws:implement` step 8 issues `rm -f <correctness-path> <fit-path> <test-path>` using the literal paths stored from the single allocation call. Cleanup is mandatory; paths are in `/tmp/claude-reviews/`.
- `ws-infra-path` portability: all `--system-prompt` arguments referencing infra docs use `$(ws-infra-path <doc-name>)`, never bare `claude/infra/<doc-name>` literals. `ws-infra-path` resolves to the absolute path regardless of CWD. It exits 1 if the named doc does not exist under `claude/infra/`.
- `ws-new-named-agent` / `ws-call-named-agent` named registry: agent slots must be registered via `ws-new-named-agent <name> --model <level> --system-prompt "$(ws-infra-path <doc>)"` before any `ws-call-named-agent` call for that name. `ws-new-named-agent` stores model, agent type, and system prompt in `$(git rev-parse --git-dir)/ws@<repo-dir>/agents/<name>.json` and issues a fresh session UUID. The path uses `git rev-parse --git-dir` (not `$REPO_ROOT/.git`) so the registry is worktree-safe. `ws-call-named-agent <name> <prompt>` reads all config from the registry — model, system prompt, session routing — with no per-call flags. Calling `ws-call-named-agent` before `ws-new-named-agent` exits 1 with "agent not found". Output is written to both stdout and `<registry-dir>/<name>.output.txt`; the file is overwritten on every call.
- `ws-print-named-agent-output <name>` prints the last response written by the named agent. Use this after a background `ws-call-named-agent` completes to read its output. Exits 1 if no output file exists for the agent.
- `ws-interrupt-named-agent <name> <message>` queues a message for the named agent as a new user turn. The message is appended to `<registry-dir>/<name>.outbox.txt`. If the agent is currently running, the `PostToolBatch` hook (`ws-agent-check-mailbox`) exits 2 at the next tool boundary; `ws-call-named-agent`'s drain loop then resumes the session with the queued content. If the agent is idle, the message is delivered on the next `ws-call-named-agent` call. Multiple `ws-interrupt-named-agent` calls before the drain loop fires append without a separator — they arrive as one concatenated user turn. The `--settings` flag on the underlying `claude` invocation accepts a raw JSON string (not only a file path); this is how the `PostToolBatch` hook is injected inline without a temp file.
- `ws:implement` background-mode default: all `ws-call-named-agent` Bash calls in the implement skill (implementer spawn, reviewer spawns, re-review loop) use `run_in_background: true` and `timeout: 600000`. The lead reads each agent's output via `ws-print-named-agent-output <name>` after the completion notification arrives — not from stdout.
- `ws-call-named-agent` retry: when the `claude` subprocess exits non-zero and stderr contains "already in use", `ws-call-named-agent` retries up to 3 times with a 3-second delay before propagating the error. The retry loop is transparent to callers; no special handling is required. If all retries fail for any other reason, the script exits 1 immediately.
- `ws-call-named-agent` auto-compression: when `token_count > 120K` tokens, `ws-call-named-agent` transparently replaces the active session with a fresh one. The 3-step flow is: extract intent via haiku, compress session state via the model's own context, hand off to a new agent session with compressed context prepended. The lead observes no behavioral difference — the handoff result appears on stdout as normal. Re-compression is suppressed on the immediate next call via a `compressed_at` flag in the registry. Token count includes `input_tokens + cache_creation_input_tokens + output_tokens`; the registry also records a `context-window` field with the same value.
- `implementer.md` review findings guidance: when review findings arrive as a file path, the implementer reads the file then applies judgment — address correctness, contract, and security findings; deprioritize style or naming feedback that conflicts with established codebase patterns; never apply a finding without understanding why it matters for the specific change.
- `ws:sprint` Delegation Cycle uses two reviewers only (`reviewer-correctness` and `reviewer-fit`). There is no `reviewer-test` partition in sprint sessions; the Delegation Cycle registers only two agents via `ws-new-named-agent`. The ws-review-path allocation call is `ws-review-path correctness fit` (two paths, not three). The cleanup step removes the two allocated paths after each task.
- `ws:sprint` doc-pipeline deferral: doc pipeline does not run after individual tasks. It runs once at `On:wrap-up` in this order: (1) spec-update pass — dispatch `ws:spec-updater` in Suggestion mode with the commit range; the agent returns `### Proposed strips` without editing files; the sprint lead applies strips directly and runs `ws-spec-build-index`; commit immediately, (2) `ws:mental-model-updater` (after the spec-update pass so it sees the updated spec); commit immediately, (3) executor-wrapup. Each updater's output is committed in its own commit right after it completes — do not defer or batch both into a single commit.

## Coupling

- `ws:edit` → `ws:spec-updater` + `ws:mental-model-updater` → `executor-wrapup`: edit dispatches both updaters in parallel first, waits for them, then calls executor-wrapup. The updaters run before the commit gate so their outputs are captured by the gate.
- `ws:implement` → `executor-wrapup`: this skill dispatches updaters in its pre-merge pre-pass (before the merge commit). After merging, it calls executor-wrapup directly. The commit gate captures any post-merge doc changes.
- `ws:sprint` → spec-update pass → `ws:mental-model-updater` → `executor-wrapup`: the session suppresses per-task doc pipeline. At wrap-up, sprint dispatches `ws:spec-updater` in Suggestion mode, applies the proposed strips, commits that output, then dispatches `ws:mental-model-updater`, commits that output, then calls executor-wrapup. The merge into main is suggested after executor-wrapup completes, not before.

## Extension Points & Change Recipes

- **Add a new executor skill**: decide which pattern to follow:
  - Edit-like (single implementation commit, no merge): dispatch updaters explicitly before calling executor-wrapup.
  - Implement-like (multi-branch with merge): dispatch updaters in the pre-merge pre-pass, then call executor-wrapup post-merge.
  - Sprint-like (session container, feature branch): suppress per-task doc pipeline; at session end dispatch `ws:spec-updater` in Suggestion mode, apply proposed strips, then run `ws:mental-model-updater`, then call executor-wrapup. Suggest merge after executor-wrapup.
  Mixing the patterns — calling executor-wrapup before updaters finish, or dispatching updaters inside executor-wrapup — breaks the commit-gate capture guarantee.
  Include the ancestor loading rule in any implementer spawn prompt when that implementer may read sub-domain mental-model docs. Use `ws-list-mental-model` rather than manual paths where possible — it handles ancestor emission automatically.
- **Change wrapup responsibilities**: edit `claude/infra/executor-wrapup.md` only. Do not duplicate wrapup logic in individual skill files.

## Common Mistakes

- Dispatching `ws:spec-updater` or `ws:mental-model-updater` inside executor-wrapup — the playbook intentionally excludes updater dispatch. Adding it there causes double-dispatch for `ws:implement`, whose updaters already ran pre-merge.
- Calling executor-wrapup before updaters finish in an edit-like skill — the commit gate fires before updater outputs exist, leaving ai-docs/ changes uncommitted.
- Assuming the commit gate is a no-op when no doc changes were planned — prior steps such as updater agents always produce at least a checkpoint commit that touches `ai-docs/`.
- Calling `ws-review-path` twice expecting the same paths — paths embed a per-call `run_id` and are non-reproducible. Allocate once; store the paths; use them throughout the review loop.
- Using bare `claude/infra/<doc>` as a `--system-prompt` value — this path is relative to the plugin repo and fails in downstream projects. Always use `$(ws-infra-path <doc>)`.
- Relaying reviewer file contents to the implementer instead of file paths — the protocol sends paths. The implementer reads files directly. Sending content bypasses the file-based protocol and inflates lead context unnecessarily.
- Omitting the `rm -f` cleanup at step 8 — review files persist in `/tmp/claude-reviews/` across runs if not deleted, potentially leaking findings from a prior review cycle.
- Calling `ws-interrupt-named-agent` multiple times before the drain loop cycles expecting separate user turns — the outbox is a flat file; multiple appends concatenate without a separator, and the drain loop delivers all pending content as one user turn.
- Running doc pipeline per task inside a `ws:sprint` session — the session's invariant defers all doc pipeline to wrap-up. Dispatching `ws:spec-updater` or `ws:mental-model-updater` mid-session creates partial checkpoints that confuse the wrap-up updater run.
- Batching spec-updater and mental-model-updater outputs into one deferred commit at wrap-up — each updater must be committed immediately after it completes. Batching obscures which updater produced which change and makes the `(mental-model-updated)` checkpoint stamp appear on a mixed commit.
- Allocating three review paths (`ws-review-path correctness fit test`) in a sprint Delegation Cycle — sprint uses two reviewers only; there is no `reviewer-test` agent registered via `ws-new-named-agent` in the sprint session.
