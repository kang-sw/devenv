---
title: ws-mcp Git and reference tooling
related:
  260503-feat-agents-plugin-runtime-boundary: MCP runtime and launcher baseline that will host the new tools
  260503-feat-agents-plugin-write-code-port: core workflow that still needs portable diff, commit, review, and reference lookup primitives
  260503-epic-agents-plugin-skill-porting: skill migration roadmap that should eventually depend on MCP tools instead of host shell wording
---

# ws-mcp Git and reference tooling

## Background

The first host-neutral `agents-plugin` skills successfully moved ws-specific
helper dependencies from PATH-injected `ws-*` commands toward explicit MCP
tools. The remaining portability gap is broader than command replacement:
workflow skills and embedded prompts still assume that a lead or delegate can
run raw `git` commands, inspect ticket references manually, and discover spec or
ticket stems through ad hoc file search. That is workable on Unix-like developer
machines, but it weakens Windows compatibility and leaves too much
workflow-critical reference tracking to natural-language instructions.

This epic captures the next tooling track: add MCP primitives for Git status,
diff, log, commit, and workflow document reference lookup so shared skills can
depend on explicit `ws/<tool>` calls rather than shell command wording. The goal
is not to hide every project-specific build or test command. The goal is to make
the ws workflow's own document graph and Git-facing operations portable,
inspectable, and composable across Codex, Claude, and future hosts.

## Decisions

- Treat this as a separate epic from skill porting; it supports `write-code`,
  `implement`, `proceed`, and `sprint`, but should not block every draft skill
  from existing.
- Prefer MCP tools for ws-owned workflow operations: ticket lookup, spec stem
  lookup, reference graph queries, Git status/diff/log, and commit creation.
- Use the `ws/git.*` namespace for Git operations. It is more direct than a
  generic `ws/vcs.*` namespace and better matches the existing workflow history,
  including the old `ws-merge-commit` idea of composing a specialized workflow
  commit command rather than wrapping raw `git commit`.
- Treat `ws/git.commit` as a workflow-aware commit builder, not a thin Git
  wrapper. It should accept structured fields such as title, description,
  detailed AI context, explicit paths, and optional ticket/spec/mental-model
  update summaries, then assemble a compliant commit message and stage only the
  requested files.
- Keep project-specific verification commands out of scope for the first pass.
  Build/test/publish commands are inherently repository-defined and can remain
  explicit until a later command-runner policy exists.
- Keep CLI fallback subcommands for development and Claude compatibility where
  practical, but shared skill text should name the MCP tool surface first.
- Design tools around structured inputs and outputs instead of returning only
  shell-formatted text, while still keeping human-readable summaries available
  for model ergonomics.

## Constraints

- Do not update `ai-docs/spec/` or `ai-docs/mental-model/` on this branch; the
  current branch policy defers spec and mental-model writes until merge.
- Do not mutate `claude-plugin/` as part of the first MCP implementation unless
  a child ticket explicitly scopes Claude compatibility.
- Preserve the existing ticket stem convention and spec anchor convention.
- Commit creation must preserve detailed `## AI Context` bodies and must not
  accidentally stage unrelated files.
- Git operations must be path-safe on Windows and avoid shell parsing; use Go
  `exec.Command` with argument arrays or native parsers.
- Tools must make destructive or broad operations explicit. No reset, checkout,
  clean, merge, or push behavior belongs in the baseline unless a later child
  ticket scopes it carefully.

## Planned Child Tickets

- Git status and diff primitives: add `ws/git.status`, `ws/git.diff`, and
  `ws/git.log` plus CLI fallbacks. These should cover current skill needs such
  as start-commit capture, changed-file discovery, diff range inspection, merge
  base lookup, and commit-body inspection.
- Git commit primitive: add a constrained `ws/git.commit` that accepts explicit
  paths plus structured commit fields such as `title`, `description`,
  `ai_context`, `updated_tickets`, `updated_specs`, and
  `updated_mental_models`. It should refuse unrelated staged changes unless
  instructed by a narrow policy and preserve readable multi-paragraph commit
  messages without shell quoting issues.
- Ticket catalog and reference primitives:
  `260504-feat-ws-mcp-reference-discovery-tools` adds path-first tools for
  listing ticket paths/status, locating tickets that mention a ticket stem, and
  summarizing parent/related/spec references. It is not a ticket body read/edit
  API.
- Spec and stem reference primitives: extend the existing spec index surface so
  callers can find anchors by `spec_stem`, heading, source file, or text query,
  and can trace ticket frontmatter references to spec stems without ad hoc
  search.
- Skill and prompt cleanup: after the VCS/reference tools exist, normalize
  `agents-plugin` skills and embedded prompts away from direct `git`, `Bash`,
  `Grep`, `Glob`, `sed`, and manual stem-search wording where those operations
  are ws-owned rather than project-specific verification.
- Runtime metadata and smoke coverage: add the new MCP tools and CLI fallbacks
  to `runtime.json`, launcher drift checks, Go tests, CLI smoke, and plugin
  validation. Include Windows compile-only checks and defer native Windows
  runtime smoke until an environment is available.

## Implemented Slices

- `dcaa0366` - Added `ws/git.commit` and `ws-mcp git commit` as a constrained
  workflow commit builder. It stages only explicit paths, rejects unrelated
  staged paths, requires structured `## AI Context`, updates `runtime.json`,
  documents the MCP contract, and detects staged ticket moves plus added
  `### Result` headings as `ticket_changes`.
- `c9d2810` / `5a2c7b7` / `8c17ccd` / Phase 4 follow-up - Added path-first
  ticket, spec, mental-model, and cross-reference discovery tools through the
  `260504-feat-ws-mcp-reference-discovery-tools` child ticket. The surface uses
  direct scans, returns structured path/status/reference metadata, and avoids
  document body read/edit APIs.
- `260504-chore-skill-prompt-mcp-cleanup` - Centralized MCP primitive usage
  guidance in `ws:lead-workflow` and normalized shared `agents-plugin` skills
  plus embedded prompts away from ws-owned direct shell search/Git recipes.

## Open Questions

- Should reference graph output be optimized first for model-readable summaries
  or for structured downstream automation?
- Where should project-specific verification command metadata live if a later
  phase decides to make build/test invocation more portable?
