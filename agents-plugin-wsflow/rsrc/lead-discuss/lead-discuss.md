---
kind: print
delegates: true
---

# Discuss

Topic: user request

## Invariants

Scope
- No source edits during discussion.
- Documentation writes are allowed only in Capture, Ticket Status Transition, or user-approved persistence handlers.
- With user agreement, unimplemented ticket phases may be edited during Capture. Phase plan text before a `### Result` is frozen; append a `#### Edition` for later tweaks.

Evidence
- Architecture/migration/spawn-removal/adapter-boundary topics → read `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` before answering.
- Commit history is a project memory tier: `## AI Context` bodies carry decision rationale docs may not yet reflect. Access via Explore-type subagent dispatch.
- When docs are stale or insufficient, say so; do not speculate.

Conversation
- Act like a careful senior engineer: stress-test premises, trade-offs, and failure modes before endorsing a direction.
- Evaluate each claim independently; call out unaddressed risks; do not parrot back risks already discussed and resolved.
- When responding to proposals, design questions, or trade-off requests: embed reading of the request, options considered, and stance naturally in the response before giving advice.
- When direction is unclear, ask the single highest-leverage question; descend to detail only after the parent is resolved.
- Summarize decision rationale when explaining stances; do not expose raw hidden reasoning.
- Never proactively ask to wrap up or persist; wait for the user's explicit signal.
- Discussion persistence writes only confirmed decisions; ticket cleanup goes through `lead-write-ticket`'s Open Decision Queue.
- Ticket creation must route through `lead-write-ticket` (`ws/tickets.create`); do not create ticket files directly.
- "Save a preference" / "remember a setting" → `{{.SkillNamespace}}:lead-tune`; `{{.SkillNamespace}}:lead-add-rule` is for repo-level rules only.

## On: invoke

1. Call `{{.McpNamespace}}/project_tree(session_key: <key>)` and `{{.McpNamespace}}/git.status(session_key: <key>)` in parallel.
2. If `user request` references a ticket, read it.
3. Enter user-message handling.

Post-compaction: call `{{.McpNamespace}}/workflow_manual(session_key: <key>)` before step 1; use `{{.SkillNamespace}}:lead-revive` first if key is lost.

## On: user message

1. If the user explicitly wants implementation to start, hand off to `{{.SkillNamespace}}:lead-proceed` and stop.
2. Apply `judge: needs-survey` and `judge: needs-cascade-lookup`; run **Cascade Lookup** if triggered.

### Cascade Lookup

1. Search loaded tickets and docs first.
2. For each loaded stem, call `{{.McpNamespace}}/references.trace`.
3. Query `{{.McpNamespace}}/tickets.find`, `{{.McpNamespace}}/specs.find`, and `{{.McpNamespace}}/mental_models.find` with concrete terms.
4. Stop when a documented answer is found.
5. If no documented answer, say that before inferring.

## On: Ticket Status Transition

Triggers on user request to change ticket status.

1. **Triage (idea/ → todo/)**: `{{.McpNamespace}}/tickets.move(stem, to: "todo")`; fall back to `git mv`. Do not require spec creation.
2. **Ready promotion (todo/ → ready/)**: call `{{.McpNamespace}}/playbook.print(name: "lead-write-ticket")` inline (Edit path). It owns spec addressing, frontmatter, move, focus update, and commit. Stop after it returns.
3. **Drop (→ .dropped/)**:
   a. Read the ticket. For each `spec:` field and `{#YYMMDD-slug}` reference: check if any other non-dropped ticket also references it.
   b. No other ticket → `{{.McpNamespace}}/playbook.print(name: "lead-write-spec")` inline to close the linked spec entry.
   c. Other tickets reference it, or coverage is ambiguous → ask before removing.
   d. `{{.McpNamespace}}/tickets.close(stem, status: "dropped")`; fall back to `git mv`.
4. Commit through `{{.McpNamespace}}/git.commit`.

## On: user signals done

1. If implementation, hand off to `{{.SkillNamespace}}:lead-proceed` and stop.
2. If durable capture requested and artifact not approved, ask whether to persist; stop until answered.
3. Call `{{.McpNamespace}}/playbook.print(name: "lead-write-ticket")` inline; it handles ticket creation/update and any required spec addressing.
4. If artifact is unclear, ask one clarifying question and stop.
5. Write only what the user approves. If nothing written, report current conclusion and any unresolved decision.

## Judgments

### judge: needs-survey
Spawn explorer subagent when the question names a component, skill, agent, spec, or ticket whose doc has NOT been loaded this session (regardless of confidence), or when the discussion direction shifts to a domain with no loaded docs. Prefer triggering over skipping.

Prompt: "Researching [topic] for discussion. Find related tickets, specs, and mental models in `ai-docs/` relevant to [specific question]. Return concise findings in English."

Does NOT fire for session-continuity queries ("what were we doing?") — those draw from session state or `{{.McpNamespace}}/git.log`.

### judge: needs-cascade-lookup
Run Cascade Lookup when the answer depends on a documented decision, prior rejection, architecture fact, or cross-ticket constraint not already loaded, or when answering otherwise requires inferring project direction from memory.

Does NOT fire for status from already-loaded context or purely local implementation detail.

## Doctrine

This skill optimizes for **decision quality per conversation turn**. Sharpen
reasoning with risks, reuse opportunities, and concrete alternatives; capture
only what the user approves. When ambiguous, preserve decision quality per turn.
