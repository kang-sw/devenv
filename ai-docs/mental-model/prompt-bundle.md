---
domain: prompt-bundle
description: "Embedded prompt discovery, prompt resolution, delegate orientation, and runtime bundle metadata."
sources:
  - agents-plugin-tool/internal/wsprompt/
  - agents-plugin-tool/internal/wsagent/
  - agents-plugin/
related:
  plugin-runtime: "runtime.json and launcher checks consume prompt bundle hash/list metadata."
  workflow-skills: "skills register delegate agents by embedded prompt stem."
  api-documentation-cache: "api.ask hard-codes API prompt stems and conditional prompt refs."
---

# Prompt Bundle

## Entry Points

- `wsprompt.Resolve` resolves embedded stems or absolute prompt paths into system text and optional model metadata plus legacy tier metadata.
- `wsprompt.Bundle` and `ContentSHA256` expose prompt bundle metadata to `runtime.info` and `runtime.capabilities`.
- `wsagent.Manager.Register` materializes prompt chains into each agent's `system.md`. {#260505-agent-prompt-registration-tier-resolution}

## Module Contracts

- Embedded prompt discovery includes only top-level Markdown under `prompts/*.md` and `infra/*.md`; nested files are invisible.
- Embedded prompt specs are bare stems with optional `.md` suffix; absolute paths are valid; relative/slashed specs and specs containing `..` are rejected.
- Prompt bodies concatenate in caller order, then inline `system_prompt_text` appends last with separators.
- Only the first prompt with usable `model:` frontmatter sets alias/model when explicit values are absent.
- Embedded prompt frontmatter uses `model: light|core|deep` for portable aliases; Claude compatibility aliases remain accepted inputs, not shared authoring style.
- If no explicit `model` or legacy `tier` and no prompt frontmatter sets one, registration defaults to the `core` alias before harness-aware backend/model resolution.
- Public named agents get `delegate-orientation` prepended unless suppressed or already first. {#260505-workflow-delegate-prompt-boundaries}
- Subquery uses inline `SubquerySystemPrompt` and suppresses orientation because it is self-contained. {#260505-async-subquery-ephemeral-agent}

## Coupling

- Prompt text, filenames, additions, deletions, and moves change the bundle hash; `agents-plugin/runtime.json` must be refreshed.
- Launcher fast-path and fallback checks must report the same prompt bundle hash: `runtime.capabilities`, `runtime.info`, and `runtime.json` drift makes otherwise compatible binaries fail validation.
- Release asset builds rewrite runtime prompt metadata from the built binary.
- Skills and API docs code name prompt stems directly; renaming stems requires updating those callers.
- `lead-write-skeleton` registers `skeleton-populator` and `skeleton-reviewer`; no compatibility skeleton stem remains in the active prompt bundle.
- `prompts` is canonical while `prompt_refs` is a migration alias; when both are present, `prompts` wins.

## Extension Points & Change Recipes

- **Add an embedded prompt**: create top-level `prompts/<stem>.md` or `infra/<stem>.md`, avoid duplicate stems, update tests/runtime metadata, then call it by bare stem.
- **Add conditional prompt behavior**: wire `ConditionalPromptRef`; missing binaries are skipped, empty `Binary` errors, empty `PromptRef` defaults to the binary name, and resolved conditional prompts append after primary prompts.
- **Change delegate orientation**: review all public named-agent workflows and internal suppressions, especially subquery and API docs managers.

## Common Mistakes

- Editing prompt text without updating `runtime.json`; launcher compatibility and tests will see stale metadata.
- Expecting `tools:` or `name:` frontmatter to affect runtime behavior; only `model:` is interpreted.
- Registering an existing agent after prompt edits and expecting it to update while a call is active; active registrations cannot be reset.
- Adding prompt files in subdirectories.

## Technical Debt

- `ws-mcp.md` prompt stem lists can drift from `runtime.json`; runtime metadata is the authoritative list.
- Root-relative prompt paths are not implemented.
