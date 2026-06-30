---
title: "Clarify render→spawn pattern: rendered path must be forwarded to subagent, not read by lead"
---

# Clarify render→spawn pattern: rendered path must be forwarded to subagent, not read by lead

## Background

`ws/playbook.render` returns a file path, not content. The design intent is:

1. Lead calls `playbook.render(name: "<delegate>")` → receives a file path.
2. Lead includes the path in the subagent kickoff prompt.
3. Subagent reads the file itself and applies it as its system prompt.

The lead must **not** read the rendered file. The architecture exists precisely to
keep the subagent's system prompt out of the lead's context window.

Two failure modes were observed during dogfooding of the `lead-write-ticket` Sage
Review Gate:

- **Failure A:** Lead called `playbook.render`, received path, but did not forward
  the path to the subagent kickoff prompt at all. Subagent received no system
  prompt and misinterpreted the task (completeness agent executed the ticket
  instead of reviewing it).
- **Failure B (incorrect recovery):** Lead read the rendered file content and
  embedded it directly in the Agent() prompt parameter. This loads the subagent
  system prompt into the lead's context window, defeating the purpose of
  `playbook.render`.

Root cause: the `lead-write-ticket` Sage Review Gate step that spawns reviewers
does not explicitly say "include the path in the kickoff prompt; do not read it."
The `lead-workflow-manual` Persistent agents section has the same ambiguity:
"Hand the rendered prompt to a native subagent" is unclear about whether "hand"
means forwarding the path or embedding the content.

## Phases

### Phase 1: Fix render→spawn guidance in workflow_manual and lead-write-ticket

Clarify both locations to make the forwarding contract unambiguous.

Changes required:

1. **`lead-workflow-manual.md` — Persistent agents section:**
   After "Hand the rendered prompt to a native subagent (default)", add a
   clarifying note:
   > Pass the returned file path in the subagent's kickoff prompt so the subagent
   > reads its own system prompt. Do not read the rendered file in the lead context.

2. **`lead-write-ticket` Sage Review Gate spawn step:**
   The step that spawns design/completeness reviewers must explicitly say:
   > Include the rendered path in the subagent kickoff prompt. The subagent reads
   > the file; the lead does not.

Verification: a fresh read of both files shows the forwarding contract is
unambiguous to a reader who has not seen this bug before.
