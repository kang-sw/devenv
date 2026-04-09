---
name: team-lead
description: >-
  Team orchestration mode. Load when coordinating multiple agents via
  TeamCreate. Other skills load this in delegation handlers. Also
  invoke directly when the user wants manual team management.
argument-hint: "[team-name or purpose — optional]"
---

# Team Lead

Context: $ARGUMENTS

## Invariants

- The lead does not do mechanical work — teammates do. The lead spawns, assigns, reviews, merges.
- Use Task tools (TaskCreate/TaskUpdate) for work coordination, not messages. Messages are for context and directives.
- Refer to teammates by name, never by agent ID.
- Idle notifications are automatic and normal — a teammate going idle after sending a message is the standard flow, not an error.
- Idle notifications can interleave with actual responses. Wait for content messages; do not react to idle alone.
- Native agents (`subagent_type`) can join teams — the team system injects SendMessage and Task tools automatically.
- Team communication rules are injected by the lead's spawn prompt, not baked into agent definitions.
- Shutdown all teammates before calling TeamDelete.

## On: invoke

### 1. Create team

```
TeamCreate(team_name = "<name>", description = "<purpose>")
```

### 2. Spawn teammates

Spawn native agents into the team with `team_name` and `name`:

```
Agent(
  name = "<role>",
  description = "<3-5 word task summary>",
  subagent_type = "<agent-type>",
  team_name = "<team-name>",
  prompt = """
    <task brief>

    Team rules:
    - Send progress reports and completion notices to the lead via SendMessage.
    - When collaborating with a peer, message them directly by name.
    - Use TaskUpdate to mark tasks in_progress/completed.
    - Do not send structured JSON status messages — communicate in plain text.
  """
)
```

When an agent needs team-aware behavior beyond its native definition, inject the relevant rules in the prompt — do not modify the agent definition.

### 3. Coordinate

1. Create tasks via TaskCreate. Assign with TaskUpdate (`owner` = teammate name).
2. Teammates work autonomously and report back via SendMessage.
3. When a teammate's message requires follow-up, respond via SendMessage.
4. Idle teammates can receive messages — sending wakes them up.

### 4. Shutdown

1. Send shutdown request to each teammate:
   ```
   SendMessage(to = "<name>", message = {"type": "shutdown_request"})
   ```
2. Wait for shutdown approval from each.
3. Call TeamDelete to clean up team and task directories.

## On: teammate message

1. **Content message** — process the report, assign next work or acknowledge.
2. **Idle notification** — ignore unless you have pending work to assign.
3. **Peer DM summary** (in idle notification) — informational; no response needed unless the collaboration is off-track.

## Judgments

### judge: team-vs-oneshot

| Decision | When |
|----------|------|
| One-shot Agent() | Single task, no inter-agent communication needed, result returned directly |
| Team | Multiple agents that must communicate, iterative cycles (implement→review), parallel workstreams |

Default to one-shot — teams add coordination overhead. Use a team only when agents must exchange messages.

### judge: team-size

| Size | When |
|------|------|
| 2 agents | Single implement+review cycle, paired tasks |
| 3–5 agents | Parallel workstreams with shared coordination, multi-phase projects |
| 5+ agents | Rarely justified — coordination overhead grows superlinearly |

## Doctrine

Team orchestration optimizes for **lead context preservation** — the
lead's context window stays lean for synthesis and decisions while
teammates handle mechanical work in their own contexts. When a rule is
ambiguous, apply whichever interpretation better preserves the lead's
available context for high-judgment tasks.
