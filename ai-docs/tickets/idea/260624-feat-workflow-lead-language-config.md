---
title: Configurable user conversation language for the lead agent
related-mental-model:
  - workflow-skills
  - mcp-runtime
related:
  - 260620-bug-ws-delegate-playbook-output-language-unbound
---

# Configurable user conversation language for the lead agent

## Background

The lead agent currently inherits whatever language the host session operates in.
`260620-bug-ws-delegate-playbook-output-language-unbound` addresses the
complementary problem: subagent delegates leak non-English into English-only
artifacts. This ticket addresses the lead's output channel: there is no
first-class ws-level mechanism for a non-English-speaking user to configure the
lead to respond in their language while keeping CoT and subagent outputs in
English.

The current workaround is host-specific (e.g., `AGENTS.md ## Language` binding in
Claude-specific CLAUDE.md shims). A ws-native config key would work across hosts
and survive compaction correctly, without spreading language rules into
project-level AGENTS.md.

## Direction (sketch)

Add `workflow.lang` config key at global or project scope. When set:

1. The `lead-workflow-manual` playbook's `### User preferences` section
   (currently empty) receives an injected line:
   > User conversation language: `<lang>`. Keep internal reasoning and all
   > subagent prompts in English; final user-facing responses must be in
   > `<lang>`.
2. Subagent isolation is automatic: delegate subagents do not load
   `lead-workflow-manual`, so the instruction stays lead-only.
3. CoT / mid-process output stays English; only the lead's final response to the
   user changes language.

Implementation requires:
- Go: add `workflow.lang` to the config schema and layered-config resolution.
- Go: inject the resolved value as a template variable when
  `playbook.print(name: "lead-workflow-manual")` renders with a session key.
- Playbook: add the conditional language block to `### User preferences` in
  `lead-workflow-manual.md`.

## Open questions

- Config key name: `workflow.lang` vs `workflow.user_language` vs plain `lang`.
- Scope: global (user-wide preference) vs project (project-specific language).
  Global is the natural default; project override may be useful for multilingual
  deployments.
- Interaction with `260620-bug-ws-delegate-playbook-output-language-unbound`:
  that ticket hardcodes English for delegates. This ticket must not break that
  invariant — the language config applies only to the lead output channel.

## Related

- `260620-bug-ws-delegate-playbook-output-language-unbound` — complementary fix
  (delegates output English); implementation ordering: either can go first.
- `260605-epic-ws-playbook-factory-pivot` — the playbook render path that would
  carry the injected variable.
