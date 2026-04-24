---
domain: executor-wrapup
description: "Shared wrapup playbook for executor-series skills: doc pipeline, commit gate, ticket update."
sources:
  - claude/infra/
  - claude/bin/
  - claude/skills/edit/
  - claude/skills/implement/
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

## Module Contracts

- `executor-wrapup` guarantees: the doc-commit gate always runs, even when no doc changes are expected. Prior steps may have dirtied `ai-docs/` without the executor knowing.
- `executor-wrapup` guarantees: it does **not** dispatch `ws:spec-updater` or `ws:mental-model-updater`. Updater dispatch is the caller's responsibility, following the pattern for that executor series.
- The doc-commit gate uses `git status --porcelain ai-docs/` as the trigger. A non-empty result mandates a commit; an empty result is a no-op.
- `executor-wrapup §Ancestor Loading` invariant: when a skill loads a sub-domain mental-model doc (`mental-model/<domain>/<sub>.md`), it must also load `mental-model/<domain>/index.md` first. `list-mental-model` auto-emits the parent `index.md` alongside any direct-child sub-domain — callers using it need no manual action. Callers using manual paths must read the parent before the child. Subagent spawn prompts must include the ancestor loading rule verbatim so the subagent observes it inside its own read ordering.
- `ws:implement` file-based review protocol: reviewers write full findings to pre-allocated file paths; each reviewer returns only a `[clean|non-clean]: <one-line summary>` to the lead. The lead reads summaries only. When non-clean, the lead relays the file paths (not content) to the implementer. The implementer reads files directly. On re-review, reviewers overwrite the same paths.
- `review-path` guarantees: paths are non-reproducible after the call returns (they embed a per-call `run_id`). The lead must invoke `review-path <stem1> <stem2> ...` once in a single Bash call and capture all output lines. Re-invoking `review-path` with the same stems yields different paths.
- `review-path` cleanup: `ws:implement` step 8 issues `rm -f <correctness-path> <fit-path> <test-path>` using the literal paths stored from the single allocation call. Cleanup is mandatory; paths are in `/tmp/claude-reviews/`.
- `ws-infra-path` portability: all `--system-prompt` arguments referencing infra docs use `$(ws-infra-path <doc-name>)`, never bare `claude/infra/<doc-name>` literals. `ws-infra-path` resolves to the absolute path regardless of CWD. It exits 1 if the named doc does not exist under `claude/infra/`.
- `implementer.md` review findings guidance: when review findings arrive as a file path, the implementer reads the file then applies judgment — address correctness, contract, and security findings; deprioritize style or naming feedback that conflicts with established codebase patterns; never apply a finding without understanding why it matters for the specific change.

## Coupling

- `ws:edit` → `ws:spec-updater` + `ws:mental-model-updater` → `executor-wrapup`: edit dispatches both updaters in parallel first, waits for them, then calls executor-wrapup. The updaters run before the commit gate so their outputs are captured by the gate.
- `ws:implement` → `executor-wrapup`: this skill dispatches updaters in its pre-merge pre-pass (before the merge commit). After merging, it calls executor-wrapup directly. The commit gate captures any post-merge doc changes.

## Extension Points & Change Recipes

- **Add a new executor skill**: decide which pattern to follow:
  - Edit-like (single implementation commit, no merge): dispatch updaters explicitly before calling executor-wrapup.
  - Implement-like (multi-branch with merge): dispatch updaters in the pre-merge pre-pass, then call executor-wrapup post-merge.
  Mixing the patterns — calling executor-wrapup before updaters finish, or dispatching updaters inside executor-wrapup — breaks the commit-gate capture guarantee.
  Include the ancestor loading rule in any implementer spawn prompt when that implementer may read sub-domain mental-model docs. Use `list-mental-model` rather than manual paths where possible — it handles ancestor emission automatically.
- **Change wrapup responsibilities**: edit `claude/infra/executor-wrapup.md` only. Do not duplicate wrapup logic in individual skill files.

## Common Mistakes

- Dispatching `ws:spec-updater` or `ws:mental-model-updater` inside executor-wrapup — the playbook intentionally excludes updater dispatch. Adding it there causes double-dispatch for `ws:implement`, whose updaters already ran pre-merge.
- Calling executor-wrapup before updaters finish in an edit-like skill — the commit gate fires before updater outputs exist, leaving ai-docs/ changes uncommitted.
- Assuming the commit gate is a no-op when no doc changes were planned — prior steps such as updater agents always produce at least a checkpoint commit that touches `ai-docs/`.
- Calling `review-path` twice expecting the same paths — paths embed a per-call `run_id` and are non-reproducible. Allocate once; store the paths; use them throughout the review loop.
- Using bare `claude/infra/<doc>` as a `--system-prompt` value — this path is relative to the plugin repo and fails in downstream projects. Always use `$(ws-infra-path <doc>)`.
- Relaying reviewer file contents to the implementer instead of file paths — the protocol sends paths. The implementer reads files directly. Sending content bypasses the file-based protocol and inflates lead context unnecessarily.
- Omitting the `rm -f` cleanup at step 8 — review files persist in `/tmp/claude-reviews/` across runs if not deleted, potentially leaking findings from a prior review cycle.
