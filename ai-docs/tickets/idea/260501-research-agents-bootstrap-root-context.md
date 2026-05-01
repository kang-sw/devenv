---
title: AGENTS bootstrap-managed root workflow context
related:
  260429-research-host-neutral-ws-plugin: first slice for making the workflow usable from host-neutral agents
---

# AGENTS bootstrap-managed root workflow context

## Background

The open-conventions migration needs a fast path from session start to effective
automation. A fresh agent should be able to read `AGENTS.md` and immediately
understand the project documentation system, ticket lifecycle, current source of
truth, and safe next actions without having to rediscover scattered conventions
from skill and infra documents.

The existing knowledge is split across `AGENTS.md`, `CLAUDE.md`,
`ai-docs/_index.md`, `claude-plugin/infra/workflow-for-agent.md`,
`claude-plugin/skills/workflow/SKILL.md`, and
`claude-plugin/skills/bootstrap/`. That split made sense for Claude-specific
skills and subagent prompts, but document-system rules change over time and need
to be synchronized into the root agent context rather than hidden behind a
skill invocation.

## Direction

Treat `AGENTS.md` as the host-neutral root workflow context for project and
ticket operation. It should contain the stable rules that a lead agent needs
before touching documents:

- session-start read order and compatibility fallback rules;
- documentation layer roles for `_index.md`, `tickets/`, `spec/`, and
  `mental-model/`;
- ticket status directories, stem usage, phase/result immutability, and queue
  expectations;
- pointers to the convention documents that must be read before editing
  tickets, specs, mental models, skills, agents, or infra prompts;
- the current mixed Claude/open-conventions authority boundary;
- the fact that broad host-neutral migration work should be split into
  actionable tickets before structural changes.

Keep `workflow-for-agent.md` as the compact subagent orientation document. It
can summarize document layers and safe read-only primitives for delegated
agents, but it should not be the only source of lead-agent document-system
truth.

Keep `workflow/SKILL.md` focused on runtime orchestration primitive signatures:
named-agent registration, calls, interrupts, review paths, and current `ws-*`
command behavior. It should not own the meaning or lifecycle of project
documents.

Extend `/bootstrap` so root workflow context is synchronized as a managed
template, likely through an `AGENTS.template.md` or equivalent source. Fresh,
upgrade, and adopt modes should maintain `AGENTS.md` with the same idempotent
discipline currently used for `CLAUDE.md`, while preserving project-specific
sections and surfacing conflicts inline.

## Open Questions

- Should `AGENTS.md` be generated from a standalone `AGENTS.template.md`, or
  should the canonical text live in `bootstrap/SKILL.md` until the host-neutral
  plugin layout exists?
- Which rules remain in `CLAUDE.md` as Claude compatibility behavior after
  `AGENTS.md` becomes the root context?
- Should `workflow-for-agent.md` be generated from the same source as
  `AGENTS.md`, or kept as a separate intentionally smaller subagent prompt?
- What is the first actionable ticket boundary: AGENTS template only,
  bootstrap support only, or both in one phase?

## Next Step

Promote this research into an actionable `todo/` ticket after choosing the first
implementation boundary and confirming whether any caller-visible behavior needs
a spec entry. The expected first implementation slice is small: make
`AGENTS.md` bootstrap-managed and move durable document-system orientation there
without changing the broader plugin directory layout.
