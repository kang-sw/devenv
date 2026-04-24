---
title: /discuss on-demand project-survey + enriched output
spec:
  - 260424-discuss-on-demand-survey
related-mental-model:
  - workflow-routing
---

# /discuss on-demand project-survey + enriched output

## Background

`/discuss` currently auto-invokes `project-survey` on every invocation, before
any discussion begins. This fires unconditionally — even for trivial queries like
"what were we working on?" or single-turn exploratory questions — adding agent
spawn latency and context overhead to every discuss session.

The survey exists to orient the model on relevant spec, mental-model, and ticket
context before it engages with a topic. This value is real when the topic warrants
broad context, but wasted when the user is asking a short clarifying question or
continuing from existing session state.

Separately, the survey's current output is stem-only (a `[Must|Maybe]` list of
document identifiers), which requires the reader to separately open each doc to
understand relevance. Enriching the output with spec entry summaries and ticket
phase titles would let the model (and user) assess relevance at a glance.

## Decisions

- **On-demand trigger: model judgment, not user invocation**: the model running
  `/discuss` decides when to spawn the survey, not the user via a slash command.
  Trigger signals: (a) the topic spans multiple spec domains or mentions unfamiliar
  components, (b) the discussion direction shifts significantly mid-session, (c) the
  model lacks confidence about which specs or tickets are relevant.

- **Not a session-continuity tool**: queries like "what were we doing?" or "remind
  me where we left off" are answered from current session state or `git log`, not
  from a project-survey spawn. The survey answers "what docs are relevant to this
  topic?" not "what did we do before?"

- **Enriched output scope**: spec entries include the entry title and one-line
  summary (from the spec body, not synthesized). Active ticket entries include the
  ticket title and the titles of unresolved phases. Stems remain present as
  identifiers. The `[Must|Maybe]` tier structure is preserved.

- **No auto-fire retained for /implement and /edit**: those skills retain the current
  auto-invoke behavior. This ticket only changes `/discuss`.

## Phases

### Phase 1: Remove auto-invoke from discuss/SKILL.md; add on-demand trigger logic

Modify `discuss/SKILL.md`:

1. Remove step 0 ("Context survey: spawn `project-survey`...").
2. Add a judgment to the discussion loop handler:

   > **judge: needs-survey** — spawn `project-survey` when any of the following hold:
   > - The topic references components, specs, or tickets the model has not read
   >   this session and cannot confidently assess scope from session context alone.
   > - The discussion direction shifts to a new domain mid-session.
   > Explicitly does NOT fire for session-continuity queries ("what were we doing?",
   > "where were we?") — those draw from session state or git log.

3. When `judge: needs-survey` fires, spawn `project-survey` inline during the
   discussion loop turn (not as a blocking pre-step) and incorporate the returned
   reference list into the current turn's reasoning.

Success: starting `/discuss` with "what were we doing?" does not spawn a
`project-survey` agent. Starting `/discuss` with a topic spanning multiple spec
domains does spawn the survey on the first relevant turn.

### Phase 2: Enrich project-survey agent output

Modify `claude/agents/project-survey.md` (the agent instruction file):

The returned `[Must|Maybe]` list currently contains stems only. Enrich each entry:

- **Spec entries**: `<stem> — <entry title>: <one-line summary from spec body>`
- **Ticket entries**: `<stem> — <ticket title> [phases: <unresolved phase titles>]`
- **Mental-model entries**: unchanged (stems + section titles are sufficient)

The agent reads the spec files and ticket files it identifies as relevant to extract
titles and summaries. No synthesis — copy verbatim from source documents.

Success: a survey invocation returns entries with titles and summaries inline, not
just stems. The model reading the list can assess relevance without opening each
document.

Phase 2 is independent of Phase 1 and may ship first.
