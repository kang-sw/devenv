---
title: delegate playbooks do not bind subagent output language to English
related-mental-model:
  - prompt-bundle
  - named-agent-runtime
  - workflow-skills
---

# delegate playbooks do not bind subagent output language to English

## Background

Found while dogfooding `lead-implement` (260620 Phase 1). A native implementer
subagent, spawned from the `implementer` playbook rendered by `playbook.render`,
produced its narration / self-talk in Korean — inheriting the host session's
language setting (the lead operates under an AGENTS.md `# Language` binding to
한국어).

The bundled delegate playbooks (`implementer`, `reviewer` / review partitions,
`plan-populator-*`, `mental-model-updater`, `reference-discovery`) carry role
orientation but say nothing about the agent's **output language**, so a delegate
inherits whatever language the host/lead operates in.

## Why it matters

The repo invariant is that AI-authored docs, plans, commits, tickets, and code
comments are English (AGENTS.md `# Language` / `## Project Knowledge`; only
human-facing UI strings are exempt). A delegate that thinks and writes in a
non-English language risks leaking non-English text into English-only artifacts
it authors **directly**:

- commit `## AI Context` bodies — the implementer commits its own checkpoints via
  `ws/git.commit`;
- brief / plan prose and review-path findings files written by delegates.

`lead-workflow-manual` already binds the **input** side ("Write prompts sent to
native Explore-style subagents in English"), but nothing binds the **output**
side of the delegate itself. The Korean narration observed in this run is the
benign tail of the surface; the load-bearing risk is a non-English commit body
or plan/review file.

## Direction (sketch)

Bind delegate output language to English at the playbook layer so it travels with
every render regardless of host language:

- Add an explicit line to the shared **`delegate-orientation`** fragment that
  `playbook.render` injects (the rsrc-sourced orientation joined with the caller's
  `SystemPromptText`, per `named-agent-runtime.md:56`): produce all output —
  narration, reports, commit messages, and file contents — in English. Injecting
  once in the orientation covers every delegate (DRY) rather than repeating the
  rule per playbook.
- Keep the artifact-language rule (always English, per repo invariant) distinct
  from any human-facing channel — delegates have none; they return data to the
  lead, not prose to the user.
- Edits touch rsrc delegate prompt text / orientation, so they run under
  `lead-skill-authoring`; regen the `agents-plugin-wsflow/rsrc` mirror.

## Open questions

- Injection point: shared `delegate-orientation` fragment (one place, all
  delegates) vs per-playbook frontmatter. Orientation is the DRY choice and the
  intended home.
- Does this ever generalize to a config-driven workflow output language, or is
  English hardcoded for AI-authored artifacts? The repo invariant currently fixes
  English; keep it hardcoded until a ticket changes the invariant.

Relates to `prompt-bundle` (delegate-orientation injection) and the broader
playbook-factory render path under epic `260605-epic-ws-playbook-factory-pivot`.
