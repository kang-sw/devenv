---
title: "lead-revive's post-compaction trigger has low model-attention salience"
---

# lead-revive's post-compaction trigger has low model-attention salience

## Background

`lead-revive`'s `SKILL.md` description says: "Post-compaction recovery. If
this session was compacted or continued, invoke this BEFORE any other ws
lead skill, passing the session_key preserved in the compaction summary, to
restore agenda/todo state and reload the workflow primitives."

Dogfooded live: a session was compacted (`/compact`), and the very next
user turn invoked `/ws:lead-ship` directly. `lead-ship`'s own procedure
does not call `workflow_manual`, so the session ran an entire ship flow
(pre-flight, tag, build, publish) plus a later `/ws:lead-tune` call without
ever reloading `workflow_manual`. This meant a stored `UserPreferenceSection`
override (an instruction to never use the Claude Code harness's interactive
question tool) was silently absent from context, and the lead used that
tool anyway while confirming the tune write — the exact category of
behavior the stored preference existed to prevent.

The description text is correct and unambiguous; the model simply did not
act on it. Description-only, self-triggered conventions have a known
reliability ceiling — they compete with whatever skill the user's literal
slash command names, and lose when the user's command is more specific.

## Open question: enforcement mechanism

Should post-compaction workflow-manual reload be moved off "the model
notices and complies" and onto something structurally enforced? Candidate
directions to evaluate:

- A Claude Code hook (e.g. `SessionStart` with a `compact`/`resume` source,
  or a `UserPromptSubmit` hook) at the plugin level that injects a
  reminder — or directly re-runs `workflow_manual` — whenever a
  conversation resumes from compaction, independent of which skill the
  user's next message names.
- Whether an equivalent mechanism exists or is feasible for other harnesses
  (Codex) to keep this fix host-neutral rather than a Claude-only patch,
  per this repo's host-neutral-first architecture rule.
- Whether `lead-ship` (and other terminal-flow skills that don't route
  through `lead-proceed`/`lead-implement`) should unconditionally call
  `workflow_manual` themselves at invocation, independent of any
  compaction-detection mechanism, as a defense-in-depth measure.

No decision made yet; this ticket exists to hold the finding until a
session can evaluate hook feasibility and scope the fix.

