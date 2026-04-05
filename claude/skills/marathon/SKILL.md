---
name: marathon
description: >
  Team-based implementation workflow. Delegates code interaction to
  team members, keeping the main context lean for long sessions.
argument-hint: "[ticket-path, topic, or description]"
---

# Marathon

User Argument: $ARGUMENTS

## Invariants

- Never read source code or diffs.
- Never open ticket files directly — all ticket access via clerk.
- Every response to a user message begins with a `## Delegation plan` block.
- Lead writes only `ai-docs/_index.md` directly.
- Never take over a teammate's work. Recover via message or fresh spawn.
- Never enter Session End without an explicit user signal.
- English only in code, commits, and docs.

## On: bootstrap

1. Run `bash ~/.claude/skills/marathon/bootstrap.sh`. Output is JSON:
   `{"branch", "team", "original_branch", "active_docs"}`. Record
   `original_branch` for Session End merge. If already on a `marathon/`
   branch, bootstrap resumes.
2. `TeamCreate(team_name="<team>")` using the `team` field.
3. Create protocol reminders:
   ```
   TaskCreate(
     subject="[PROTOCOL] Marathon rules",
     description="Delegate code R/W; never self-execute; emit delegation plan block per turn; doc update post-merge; coherence at wrap-up"
   )
   TaskCreate(
     subject="[PROTOCOL] Per-round checklist",
     description="Check teammate usage before dispatch; reuse or fresh spawn; dispatch fresh reviewer; dispatch clerk for any ticket touch; merge gate"
   )
   ```
4. If `$ARGUMENTS` references a ticket, spawn `clerk` and have it read
   the ticket. Receive summary and active phase from clerk. Do not open
   the file.

## On: user message

1. Emit `## Delegation plan` block first (see Templates). The block
   classifies the turn inside its `Decomposition` field.
2. Route per classification:

   - **Discussion.** Respond actively — propose approaches, surface
     risks, suggest alternatives.
     - Codebase lookup needed → dispatch Explore.
     - Recurring judgment query on a domain → see
       `judge: recurring-doc-query`.
     - Conclusion affects an unimplemented ticket phase → dispatch
       clerk with an edit directive.
     - Scope expansion (new concerns beyond current work) → dispatch
       clerk to create a ticket now; defer implementation unless
       trivially small.
     - Explicit user request for an isolated consultation → spawn
       `sub-lead` (see Templates). Turn ownership transfers to
       sub-lead; you remain idle until it delivers its Consultation
       summary or the user addresses you directly again.
   - **Non-code task.** Dispatch `worker`.
   - **Implementation.** Apply `judge: routing-check`:
     - complete brief possible → dispatch `implementer`
     - partial (gaps fillable by focused lookup) → Explore →
       `implementer`
     - fundamentally unclear → `planner` → `implementer`

     Branch: one-liner → direct commit on `marathon/<datetime>`;
     everything else → sub-branch `<type>/<round>`. Brief follows
     the Implementer brief template.

## On: implementer reports complete

1. Read the implementer's report (summary, files changed, test results).
2. For sub-branch work, check scope:
   `git diff --stat marathon/<datetime>...<type>/<round>`
3. Code review — apply `judge: review-triviality`:
   - trivial (typo/config/single-line) → skip
   - else → spawn a **fresh** reviewer with diff range
     `marathon/<datetime>...<type>/<round>`
   - on Critical/Important findings → implementer fixes → same
     reviewer re-reviews → loop until clean. Retire the reviewer
     after the round.
4. Merge decision:
   - **accept** → `git merge --no-ff <type>/<round>` into
     `marathon/<datetime>`, then delete sub-branch
   - **rollback** → delete sub-branch
5. Doc updates (skip for config/typo):
   - Spawn two parallel one-shot doc-update agents (not team
     members; fresh `general-purpose` Agents): one to refresh
     `ai-docs/mental-model/` against the merged diff, one to
     refresh `ai-docs/spec/` (skip the spec one if `ai-docs/spec/`
     does not exist). Apply the parallel spawn addendum.
   - Wait for both.
   - Update `ai-docs/_index.md` if project capabilities changed.
   - If completing a ticket phase → dispatch clerk to append `### Result`.
   - Refresh active advisors with the names of files the doc-update
     agents touched (selective re-read).
   - Commit doc changes.
6. Report results to the user.

When splitting a ticket phase into subphases, dispatch clerk with the
split directive before proceeding.

## On: session end

0. **REQUIRE** an explicit user signal of completion. Phase completion
   or task exhaustion is NOT a signal — ask what's next.
1. Final checkpoint — if the last round did not already run the
   doc-update + commit sequence from "On: implementer reports
   complete", run it now for any pending changes on
   `marathon/<datetime>`.
2. Coherence review — spawn a one-shot `general-purpose` Agent
   (sonnet, not a team member) to read `ai-docs/mental-model/` and
   `ai-docs/spec/` (if exists) and look for cross-document
   contradictions from incremental updates. Fix session-caused
   issues; flag pre-existing ones in the report. Skip if trivial
   session, no mental-model dir, or no docs updated.
3. Final commit for coherence fixes and any remaining changes.
4. Summary report to user: what was implemented, coherence findings,
   process issues, ticket status.
5. Mark all `[PROTOCOL]` tasks completed.
6. Shutdown team members.
7. Ask user for confirmation, then merge `marathon/<datetime>` into
   `<original_branch>` with `--no-ff`. Commit format:
   ```
   <type>(<scope>): <summary>
   <what changed>
   ## AI Context
   - <decisions, alternatives, directives>
   ```
   Delete `marathon/<datetime>` after merge. Skip if no commits were
   made.

## Judgments

Soft signals, not deterministic branches. The event handlers above
invoke these by name (`judge: <name>`) or reference them implicitly;
the criteria live here.

- **routing-check** — *"Can I write a complete Description for the
  implementer — specific enough to know which files to touch and what
  approach to take?"* Yes → implementer. Partial (gaps fillable by a
  focused lookup) → Explore + implementer. Fundamentally unclear →
  planner.
- **recurring-doc-query** — Expect two or more judgment-based queries
  against the same mental-model/plan domain in this session? → spawn
  `advisor.<domain>`. Single one-shot lookup → Explore.
- **read-mode (soft-lock docs)** — Mental-model, plans. Small and
  one-shot → direct read. Large or recurring → advisor. Tickets are
  hard-lock (always clerk). Source/diffs are never.
- **review-triviality** — Typo, config-only, single-line → skip.
  Otherwise fresh reviewer.
- **reuse-or-fresh** — Before dispatching to an existing member, read
  `~/.claude/usage/<team-name>.md` (entries: `"@name": "42%/150K"`).
  Prefer fresh spawn if the `%` exceeds ~80, or on domain contamination
  (prior context would mislead), or on user-initiated refresh. Resident
  roles (advisor, clerk, sub-lead) bypass — they persist until user
  refresh or Session End.
- **model** — Default sonnet. Opus for novel architecture or complex
  cross-module logic; mark the name `.expert`. Haiku for mechanical
  worker tasks and simple Explore lookups. Upgrade = spawn a fresh
  `.expert` peer, not reuse.
- **member-recovery** — Teammate stuck or unresponsive → SendMessage
  nudge. Still unresponsive → spawn fresh replacement with the same
  brief. Never self-execute.

## Templates

**Delegation plan block.** Emitted first in every response to a user
message, before any tool calls.
```
## Delegation plan
Intent: <what the user actually wants, in your own words>
Decomposition:
  - <step 1> → <role or "lead-direct (discussion)" or "lead-direct (_index.md)">
  - <step 2> → <role>
Routing: <concrete next action — which agent gets the next message, or "respond in discussion">
```
`lead-direct` is valid only for discussion turns, `_index.md` updates,
and one-shot reads of small soft-lock documents (mental-model, plans).
Tickets, source, and diffs never qualify. If the plan proves wrong
mid-turn, emit a revised block with `## Delegation plan (revision)` on
the header line.

**Implementer brief.**
```
Brief:       <one-line summary>
Files:       <target files if known>
Constraints: <any constraints from discussion>
Branch:      <branch per routing>
[Plan:       ai-docs/plans/YYYY-MM/DD-hhmm.<name>.md]
Description: <detailed guidance — approach, specific files/patterns, edge cases>
```

**Planner brief.** Same shape as implementer brief, with:
- `Plan` is required.
- `Description` explains what is uncertain — what to research, not
  implementation guidance.

Planner flow: if the plan does not converge with mental-model docs
after two rounds, dispatch implementer with inline guidance.

**Spawn signature.**
```
Agent(
  description = "<3-5 words: this round's goal>",  -- e.g., "implement login validation"
  subagent_type = "general-purpose",
  team_name = "marathon-<datetime>",
  name = "<role>.<label>[.expert]",                -- labels: alpha, beta, gamma...
  model = "sonnet",                                -- "opus" for .expert
  prompt = "Read ~/.claude/skills/marathon/agents/<role>.md.
            Your lead's name is '<your-agent-name>'. Then:
            <brief or plan reference>"
)
```
Do not encode domain in the name — `description` carries the round's
topic (verb-led, 3–5 words), `name` carries the stable team identity.

**Explore agents** are the lead's direct tool, not team members:
```
Agent(
  description = "<3-5 words: the lookup>",  -- e.g., "find auth handlers"
  subagent_type = "Explore",
  prompt = "<question>"
)
```
Default haiku. Use `model="sonnet"` for relational queries or
cross-module tracing. Insufficient even with sonnet → escalate to
planner.

**Parallel spawn addendum.** When spawning any agents in parallel
(implementers, updaters, or mixed), append to each prompt:
> You are working in parallel with other agents. Before every git
> commit, message me and wait for approval.

Serialize commit approvals one at a time — the git index is shared.

**Consultation summary.** Format sub-lead uses when reporting back
to you at wrap-up, inside SendMessage `message`:
```
## Consultation summary: <topic>
Conclusions: <what was decided, 2-5 bullets>
Tickets touched: <paths and brief change summary, or "none">
Open questions: <raised but unresolved, or "none">
Follow-ups for lead: <what the lead should pick up, or "none">
```
Ingest this into your own context at wrap-up; do not re-read the
sub-lead's transcript. The summary is the handoff.

## Team roles

Role descriptions live in `~/.claude/skills/marathon/agents/`.

| Role | Purpose | Lifespan |
|------|---------|----------|
| `planner` | Deep codebase research → plan file | multi-round |
| `implementer` | Code implementation from plan or brief | multi-round |
| `reviewer` | Code review on diffs (read-only) | fresh per round |
| `worker` | Non-code tasks (documents, config, research output) | multi-round |
| `advisor.<domain>` | Read-only domain oracle — mental-model, plans, `_index.md` | **resident** |
| `clerk` | Ticket owner (R/W); loads `/write-ticket` conventions | **resident** |
| `sub-lead` | Discussion-only consultation; explicit user spawn | **resident** |

**multi-round** members are reused by default across rounds; respawn
decision follows `judge: reuse-or-fresh` (token-aware). **fresh per
round** retires after the round. **resident** spans the whole session.

Clerk spawn: at bootstrap if `$ARGUMENTS` references a ticket;
otherwise on the first ticket-touching operation. Single clerk per
session, handles multiple active tickets. Advisor spawn: on
`judge: recurring-doc-query`. Sub-lead spawn: only on explicit user
request for an isolated consultation — never auto-spawned. Single
sub-lead per session; replacement only on explicit user request.
All three are resident and bypass the normal reuse heuristics.

## Doctrine

The lead has one finite resource: its context window. Every rule
above preserves it for decisions — delegate reads, encode mechanisms,
externalize state. When a rule looks ambiguous, apply whichever
interpretation keeps the window freer for decisions.
