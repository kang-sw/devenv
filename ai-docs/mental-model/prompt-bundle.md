---
domain: prompt-bundle
description: "Embedded prompt loading (wsprompt/go:embed), call-time rsrc playbook loading (wsrsrc/filesystem), delegate orientation, and runtime bundle metadata."
sources:
  - agents-plugin-tool/internal/wsprompt/
  - agents-plugin-tool/internal/wsagent/
  - agents-plugin-tool/internal/wsrsrc/
  - agents-plugin/
related:
  plugin-runtime: "runtime.json and launcher checks consume prompt bundle hash/list metadata; rsrc text changes are decoupled from runtime.json."
  workflow-skills: "skills register delegate agents by embedded prompt stem."
  api-documentation-cache: "api.ask hard-codes API prompt stems and conditional prompt refs."
---

# Prompt Bundle

## Entry Points

- `wsprompt.Resolve` resolves embedded stems or absolute prompt paths into system text and optional model metadata plus legacy tier metadata.
- `wsprompt.Bundle` and `ContentSHA256` expose prompt bundle metadata to `runtime.info` and `runtime.capabilities`.
- `wsagent.Manager.Register` materializes prompt chains into each agent's `system.md`. {#260505-agent-prompt-registration-tier-resolution}
- `wsprompt.RenderSource` returns a single embedded prompt body by bare stem (absolute paths and `..` rejected) for the wsflow `prompt.render` MCP tool. {#260529-prompt-render-tool}
- `wsrsrc.Load` loads a named playbook from the filesystem rsrc tree (`agents-plugin/rsrc/`): validates manifest schema-version, verifies per-file hashes, applies harness overlay, resolves flat includes, and substitutes declared variables. Phase-2 MCP tool not yet wired. {#260609-rsrc-playbook-distribution}

## Module Contracts

- **wsprompt (go:embed)** and **wsrsrc (filesystem)** are deliberately parallel, non-overlapping loaders. Distributable playbooks belong in `agents-plugin/rsrc/`; runtime-embedded delegates belong in `prompts/`. Do not route one through the other.
- rsrc compatibility is gated on `manifest.schema_version` alone — not content-hash equality. Text-only edits to rsrc files ship without a binary bump as long as `SupportedSchemaVersion` is unchanged; increment it only when the playbook schema shape changes.
- rsrc failure is always loud: `ErrManifestMissing`, `ErrSchemaMismatch`, `ErrHashMismatch`, and `ErrFileMissing` are returned as typed errors with no embedded fallback. Per-file sha256 hashes in `manifest.json` are tree-integrity checks, not binary-coupling.
- rsrc variable substitution is declared-only: a key in `vars` absent from `variables:` frontmatter → `ErrUndeclaredVar`; a declared variable whose `{{.Name}}` appears in the body but is absent from `vars` → `ErrUnprovidedVar`; declared variables not used in the body are silently ignored. Substitution is single-pass; replacement values containing `{{.Name}}` literals are never re-expanded.
- rsrc includes are flat: names resolve to `<root>/<name>.md` only; nested playbook-subdirectory includes are not supported by design.
- Embedded prompt discovery includes only top-level Markdown under `prompts/*.md` and `infra/*.md`; nested files are invisible.
- Embedded prompt specs are bare stems with optional `.md` suffix; absolute paths are valid; relative/slashed specs and specs containing `..` are rejected.
- Prompt bodies concatenate in caller order, then inline `system_prompt_text` appends last with separators.
- Only the first prompt with usable `model:` frontmatter sets alias/model when explicit values are absent.
- Embedded prompt frontmatter uses `model: light|core|deep` for portable aliases; Claude compatibility aliases remain accepted inputs, not shared authoring style.
- If no explicit `model` or legacy `tier` and no prompt frontmatter sets one, registration defaults to the `core` alias before harness-aware backend/model resolution.
- Public named agents get `delegate-orientation` prepended unless suppressed or already first. {#260505-workflow-delegate-prompt-boundaries}
- Subquery uses inline `SubquerySystemPrompt` and suppresses orientation because it is self-contained. {#260505-async-subquery-ephemeral-agent}

## Coupling

- Editing rsrc text (`agents-plugin/rsrc/`) does NOT change the prompt bundle hash; `runtime.json` requires no refresh for rsrc-only changes. Regenerate `manifest.json` via `wsrsrc.GenerateManifest` instead.
- Prompt text, filenames, additions, deletions, and moves change the bundle hash; `agents-plugin/runtime.json` and `agents-plugin-wsflow/runtime.json` must be refreshed.
- Launcher fast-path and fallback checks must report the same prompt bundle hash: `runtime.capabilities`, `runtime.info`, and `runtime.json` drift makes otherwise compatible binaries fail validation.
- Release asset builds rewrite runtime prompt metadata from the built binary.
- Skills and API docs code name prompt stems directly; renaming stems requires updating those callers.
- Legacy skeleton prompt stems may remain bundled for compatibility; normal implementation routing does not register `skeleton-populator` or `skeleton-reviewer`.
- `reference-discovery` (renamed from `project-survey`) and `plan-populator-survey` cover different axes and are frequently conflated by callers: `reference-discovery` is a `light`, docs-only agent that returns a `[Must|Maybe]` list of spec/mental-model/ticket docs and never reads source; `plan-populator-survey` is a `core` agent that reads source and writes a file-backed source reference map. The discriminator is docs-vs-source, not the shared `survey` word; pick by what the caller needs (doc list for the brief vs source map for the plan). Because `tools:` and prose constraints are not runtime-enforced, a caller's free-form spawn prompt can override either agent's scope — match the spawn prompt to the agent's role.
- `plan-populator-survey` and `plan-populator-research` are stable prompt stems with different responsibilities: survey collects evidence-only risk signals or exits to research, research makes planner judgments and escalation calls.
- `prompts` is canonical while `prompt_refs` is a migration alias; when both are present, `prompts` wins.
- wsflow `prompt.render` owns render-time namespace substitution (word-boundary `\bws/` and `\bws:`) and the five-stem render-eligibility allowlist in the `internal/mcp` tool layer; `RenderSource` stays a generic bundle loader. Adding a render-eligible prompt updates the allowlist in `internal/mcp`, not `wsprompt`, and substitution never touches caller-injected context values. {#260529-prompt-render-tool}

## Extension Points & Change Recipes

- **Add a playbook**: create `agents-plugin/rsrc/<name>/<name>.md`, optional `<name>.<harness>.md` harness overlay, optional flat text deps at `agents-plugin/rsrc/<name>.md`; run `wsrsrc.GenerateManifest` to regenerate `manifest.json`; increment `SupportedSchemaVersion` only when the schema shape changes.
- **Add an embedded prompt**: create top-level `prompts/<stem>.md` or `infra/<stem>.md`, avoid duplicate stems, update tests/runtime metadata, then call it by bare stem.
- **Add conditional prompt behavior**: wire `ConditionalPromptRef`; missing binaries are skipped, empty `Binary` errors, empty `PromptRef` defaults to the binary name, and resolved conditional prompts append after primary prompts.
- **Change delegate orientation**: review all public named-agent workflows and internal suppressions, especially subquery and API docs managers.

## Common Mistakes

- Refreshing `runtime.json` after editing rsrc text files; rsrc content is intentionally decoupled from the prompt bundle hash — only `manifest.json` needs regeneration.
- Writing nested playbook includes (`includes:` entries referencing files inside a playbook subdirectory); rsrc includes are flat root-level text deps and nesting is not resolved.
- Editing prompt text without updating `runtime.json`; launcher compatibility and tests will see stale metadata.
- Expecting `tools:` or `name:` frontmatter to affect runtime behavior; only `model:` is interpreted.
- Registering an existing agent after prompt edits and expecting it to update while a call is active; active registrations cannot be reset.
- Adding prompt files in subdirectories.

## Technical Debt

- Static docs can drift from `runtime.json`; runtime metadata is the authoritative prompt bundle inventory.
- Root-relative prompt paths are not implemented.
