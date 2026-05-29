---
title: Rename project-survey to reference-discovery
related-mental-model:
  - prompt-bundle
  - workflow-skills
completed: 2026-05-29
---

# Rename project-survey to reference-discovery

## Problem

`project-survey` (docs-only, `light`, `tools: Read`, "never read source code",
returns a `[Must|Maybe]` list of spec/mental-model/ticket docs) was repeatedly
conflated with `plan-populator-survey` (`core`, reads source, writes a
source-level reference map). The shared `survey` token, the opaque `project-`
prefix, and the `lead-implement` Prep "survey project" verb pushed callers to
select `project-survey` and inject a source-mapping spawn prompt. Because
`tools:` and prose constraints are not runtime-enforced, the spawn prompt
overrode the prompt body and produced off-contract source-level output.

## Goal

Move the disambiguation to the selection layer (agent name + spine verb) rather
than the already-explicit prompt body. Rename `project-survey` to
`reference-discovery` to drop the `survey` token entirely; keep
`plan-populator-survey`/`plan-populator-research` to preserve the planner pair.

## Phases

### Phase 1: Rename and re-thread callers

Rename the bundled prompt and update every live caller and doc; preserve
historical records (CHANGELOG, `.old/`, `.done/`, `.dropped/`).

#### Result (1dd50787) - 2026-05-29

- `prompts/project-survey.md` -> `reference-discovery.md`; frontmatter `name`,
  description, and body restated as docs-only with an explicit contrast to
  `plan-populator-survey`.
- `internal/mcp` render-eligibility allowlist + schema example; `prompts_test.go`
  bundle expectations.
- Both `runtime.json`: `prompt_bundle.prompts` entry + `content_sha256`
  regenerated from the built binary (`ec854872...`).
- ws `lead-implement`/`lead-discuss`/`lead-workflow-manual` and wsflow
  `lead-implement` stem references; Prep step 10 now reads "discover reference
  docs (docs only)" and points to step 12 (`plan-populator-survey`) for source
  mapping.
- `ref/wsflow-mirroring`, `spec/mcp-tools`, `spec/workflow-skills`,
  `mental-model/prompt-bundle` (docs-vs-source coupling note), and live idea
  ticket `260524-bug-subquery-non-head-history-evidence`.
- Verification: `go test ./...` and the wsflow python suite (10 tests) pass.
  Mechanical, behavior-preserving rename.
