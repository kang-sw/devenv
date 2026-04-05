---
title: "Marathon delegation hardening — resident advisor/clerk agents, stricter lead discipline"
related:
  - 260404-research-marathon-skill-improvements  # predecessor; checkpoint model foundation
---

# Marathon Delegation Hardening

Follow-up to `260404-research-marathon-skill-improvements`. After running
marathon in practice, multiple **lead self-execution drift** patterns
emerged where the lead bypassed delegation and performed work itself.
Two patches already landed in this session (code-review prohibition and
member recovery protocol). This ticket captures the remaining structural
changes — introducing resident **advisor** and **clerk** roles and
tightening the rules around large document reads.

## Problem

The current marathon skill treats the lead as the sole reader of tickets,
mental-model docs, and plans. In long sessions this breaks down:

1. **Code review drift.** Lead performed reviews directly instead of
   dispatching a fresh reviewer. (Fixed in commit `6334cd2`.)
2. **Member recovery drift.** When a teammate became unresponsive, the
   lead took over the work. (Fixed in commit `fdf57be`.)
3. **Token blow-up from direct doc reads.** An opus lead session read
   100K tokens of ticket content directly — most of it completed phase
   history irrelevant to routing decisions. The doctrine permits this,
   even though it contradicts marathon's token-efficiency doctrine.
4. **Ambiguous ticket ownership.** The skill has five implicit points
   where the lead reads or writes tickets (bootstrap, discussion-time
   updates, post-merge Result append, phase split, scope expansion),
   all assuming direct file edits. There is no role accountable for
   ticket consistency or `/write-ticket` convention enforcement.
5. **One-shot Explore cost for recurring queries.** When the lead
   needs repeated judgment-based lookups against the same domain's
   mental-model docs, spawning a fresh Explore agent each time wastes
   tokens re-reading the same files.

## Goals

- Keep the lead's context lean even as sessions grow.
- Eliminate the "lead reads the whole document" failure mode without
  forcing every query through expensive one-shot Explore spawns.
- Give tickets a single writer accountable for convention.
- Preserve the lead's role as decision-maker and discussion partner —
  new agents advise and record; they do not decide.

## Non-Goals

- Replacing the lead with a hierarchy. Advisor/clerk answer queries and
  execute directives — they do not propose approaches or make decisions.
- Changing the planner/implementer/reviewer/worker role contracts.
- Modifying `/write-ticket` skill itself. Clerk loads and obeys it;
  the skill is unchanged.
- Retroactively splitting the existing 260404 research ticket. This
  ticket is a distinct follow-up thread.

## Design Decisions

### Two new roles

**`advisor.<domain>` — read-only domain oracle.**
- Reads: `ai-docs/mental-model/`, plans, `ai-docs/_index.md`, and any
  reference docs in the given domain. Never reads source code or diffs.
  Never reads tickets (clerk's domain).
- Answers: structured responses to the lead's queries, with file path
  citations so the lead can spot-check.
- Must not: propose approaches, make decisions, modify files.
- Naming: `advisor.<domain>` (e.g., `advisor.auth`, `advisor.indexing`).
- Lifecycle: on-demand spawn; persists across rounds within a session.
  **Residency is load-bearing** — see caching notes below.
- Refresh protocol: initial Read at spawn. Subsequent Reads only on
  lead instruction after known doc updates (e.g., after merge gate
  runs mental-model-updater). Advisor does not auto-re-read every
  query, and the lead does not blindly ask for refresh — both would
  duplicate content in context and waste tokens.
- Model: sonnet default; haiku acceptable for simple lookups.
- Spawn trigger: **reactive** — spawn when recurring queries on the
  same domain are expected (rule of thumb: two or more queries in the
  session). A single one-shot lookup still goes to Explore.

**Read-tool caching note (verified via claude-code-guide).** Claude
Code's Read tool has no dirty-read or diff mechanism: it always returns
full file content. The only cost reduction is prompt caching, which
makes the cached prefix ~10x cheaper to **reprocess** on later turns —
it does **not** make duplicate Read calls free. Calling Read on the
same unchanged file twice adds the full content to context twice.
This is why advisor must be resident (to keep the first Read in its
own cached prefix across rounds) and why selective lead-directed
refresh beats "always re-read at query start".

**`clerk` — ticket owner (read/write).**
- Scope: the active session's ticket(s). Single instance per session,
  handles one or multiple tickets if multiple are active.
- Reads: the active ticket(s) in full; reads the `write-ticket` skill
  file at spawn time (see skill-loading note below).
- Writes: applies the lead's edit directives — phase description
  updates, phase splits, Result appends, new ticket creation, status
  transitions via `git mv`.
- Must not: read source code, read mental-model docs (that's advisor),
  make design decisions. Clerk converts the lead's directives into
  convention-compliant ticket edits; it does not decide what to write.
- Answers: the lead's queries about ticket state (current active phase,
  past decisions, Result entries).
- Lifecycle: spawned at bootstrap if a ticket exists; persists for the
  session. If no ticket exists at bootstrap, clerk is spawned when
  scope expansion or explicit ticket creation triggers it.
- Model: sonnet default.

**Skill-loading mechanism (verified via claude-code-guide).** Claude
Code supports a `skills:` frontmatter field on custom agent definitions
under `.claude/agents/<name>.md`, which auto-injects skill content at
spawn. Marathon currently spawns team members as
`subagent_type="general-purpose"` with role files passed in the spawn
prompt — which means the `skills:` frontmatter mechanism does **not**
apply to marathon's role files. Practical path for this ticket: clerk's
role file instructs the agent to `Read` the `write-ticket` skill file
directly at startup and follow its conventions. This is a simple,
working fallback. Migrating marathon roles to first-class custom agent
definitions so they can use the `skills:` frontmatter is a separate,
larger refactor tracked in Open Questions.

### Lead's revised reading policy

The doctrine changes from "the lead reads mental-model docs, tickets,
plans, diffs, team reports, explore results" to a layered policy:

- **Always direct-read:** briefs, reports, reviewer verdicts, explore
  results, monologue notes, bootstrap output, **`ai-docs/_index.md`**
  (see team-board note below).
- **Soft-lock (mental-model / plans):** direct read allowed when the
  document is small and the query is one-shot. For large documents or
  recurring queries, prefer `advisor.<domain>`.
- **Hard-lock (tickets):** the lead never opens a ticket file directly.
  All ticket access — read or write — goes through `clerk`.
- **Never:** source code, diffs (already committed in this session).

Soft-lock vs hard-lock reflects usage pattern: mental-model/plans are
reference artifacts the lead may glance at; tickets are live documents
that need a single writer to stay consistent.

### `ai-docs/_index.md` as the lead's team board

The lead is the sole decision-maker but has no durable external memory
in the current design. Everything it "knows" lives in its context
window, which decays over long sessions. `_index.md` is promoted from
a static capability index to the **lead's team board** — a concise,
live dashboard the lead owns and edits directly.

Scope of the team board:
- Current session focus and active ticket pointers.
- Per-domain advisor pointers (which advisors are spawned, what they
  know).
- Short status/next-step annotations the lead wants to carry across
  context compaction.
- Long-lived project capability summary (the original `_index.md` role).

**This is the single carve-out to the "never self-execute" rule.**
It is not a code/doc edit in the delegation-worthy sense — it is the
lead externalizing its own working memory. Without this carve-out, the
lead has no place to park state, and either hoards it in context
(token bloat) or forgets it (drift). The rule is phrased as a distinct
principle rather than an exception:

> The lead maintains `_index.md` as its working memory. Everything
> else is delegated.

Clerk feeds the team board with ticket summaries; advisor can read
the team board for domain context. The lead is the only writer.

### Rationale for clerk-as-writer (not just as oracle)

Tickets are read-write and read-heavy from multiple points in the
marathon loop. A read-only advisor for tickets would leave the lead
still editing files directly — the exact drift we are trying to
eliminate. Clerk owns both sides: it is the single entity that knows
the current ticket state and applies changes, which removes the
read/write skew and keeps the lead out of `/write-ticket` skill loads.

### Rejected alternatives

- **Single `librarian` role covering both mental-model and tickets.**
  Rejected: tickets need a writer with `/write-ticket` convention
  knowledge; mental-model/plans are read-only reference. Merging the
  roles would force every advisor instance to load `/write-ticket`
  unnecessarily, and would blur the domain-based naming (`advisor.auth`
  vs the session-wide ticket owner).
- **Ticket size threshold instead of hard-lock.** Rejected: thresholds
  drift in practice ("this one is only 15K, I'll read it directly"),
  and once the lead is reading tickets at all, the five implicit
  touch-points remain ambiguous. A hard rule is easier to enforce.
- **Preemptive advisor spawn at bootstrap.** Rejected: many sessions
  touch only one domain, so preemptive spawning wastes a teammate slot.
  Reactive (spawn on second query) is the conservative default.
- **`scribe` instead of `clerk`.** Rejected: "scribe" semantically
  implies passive transcription ("say it and I'll write it exactly"),
  which would push the lead toward verbose dictation. "Clerk" carries
  a light-judgment connotation — form-filling, convention compliance,
  filing — which matches the convert-directive-to-edit role better.
  Both fit the existing functional-role naming (reviewer, implementer,
  planner, worker).
- **Advisor allowed to propose approaches.** Rejected: that is the
  planner's role. Conflating them would blur ownership of design
  decisions.
- **Advisor always re-reads mental-model at the start of each query
  (relying on Read tool dirty-detection).** Rejected on fact-check:
  Claude Code's Read tool has no dirty-read mechanism; it always
  returns full content. The naive always-re-read approach duplicates
  unchanged content in the agent's context. See the caching note
  under advisor's definition.
- **Migrate marathon team roles to first-class custom agent
  definitions (`.claude/agents/<name>.md`) to use the `skills:`
  frontmatter.** Deferred, not rejected. This would let clerk declare
  `skills: [write-ticket]` and get the skill auto-injected, which is
  cleaner than Read-at-startup. But the migration touches every
  marathon role (implementer, reviewer, planner, worker, advisor,
  clerk) and collides with marathon's dynamic naming system
  (`impl.alpha`, `impl.beta.expert`, etc.) that relies on
  `subagent_type="general-purpose"` with inline labels. Out of scope
  for this ticket — tracked in Open Questions.

### Protocol task split (already staged, uncommitted)

The single `[PROTOCOL]` task is split into two for stronger attention:
1. **Marathon rules** (session-level): delegate code R/W, never
   self-execute, doc update post-merge, coherence at wrap-up.
2. **Per-round checklist**: check teammate usage before dispatch,
   reuse or fresh spawn, dispatch fresh reviewer, merge gate.

This change is already in the working tree; it rolls up into Phase 1.

## Phases

### Phase 1: SKILL.md revision

Apply all doctrinal and procedural changes to
`claude/skills/marathon/SKILL.md` in a single pass:

- **Doctrine section.** Rewrite the reading policy to the layered
  always-direct / soft-lock / hard-lock / never model described above.
  Keep the "token-efficient" opening sentence. Add the team-board
  principle: the lead maintains `_index.md` as its working memory,
  everything else is delegated.
- **Bootstrap step.** Add: "If `$ARGUMENTS` references a ticket, spawn
  `clerk` and have it read the ticket; receive the summary and active
  phase from clerk — do not open the file directly." Move the existing
  "If `$ARGUMENTS` references a ticket, read it" line into clerk's
  responsibility.
- **Discussion section.** Replace "update unimplemented phases to
  reflect discussion conclusions in real-time" with a clerk-dispatched
  flow: the lead summarizes the decision and sends an edit directive
  to clerk. Similarly, "load `/write-ticket` for conventions" is
  removed — clerk owns the skill.
- **Merge gate post-doc-update step.** Replace "append `### Result`
  to the ticket" with a clerk directive. Remove the `/write-ticket`
  skill load from the lead's side.
- **Phase split rule.** Dispatched via clerk.
- **Scope expansion rule.** Dispatched via clerk; if no clerk exists
  yet, spawn one as part of the ticket creation step.
- **Team Management table.** Add `advisor.md` and `clerk.md` rows
  alongside existing roles.
- **Spawning team members section.** Add a subsection for advisor
  (reactive spawn policy, naming, refresh-on-instruction) and clerk
  (spawned at bootstrap if ticket present, or on first
  ticket-touching operation otherwise).
- **Reuse policy section.** Note that advisor and clerk are resident
  by default and bypass the normal reuse heuristics — they persist
  for the session barring user refresh.
- **Protocol reminders.** Keep the two-task split staged in the
  working tree. Update the per-round checklist to include
  "dispatch clerk for any ticket touch" if it fits cleanly.
- **Rules section.** Add:
  - "Never open ticket files directly. All ticket access flows
    through clerk."
  - "Prefer advisor over direct reads for mental-model and plans
    when queries recur or documents are large."
  - "The lead maintains `_index.md` as its working memory — this is
    the only file the lead writes directly."
- **Merge gate doc updates.** After mental-model/spec updates land
  post-merge, the lead sends a refresh directive to any active
  advisor: "files X, Y were updated, please re-read." This is the
  selective refresh pattern from the advisor definition.

Success criterion: a fresh read of the doctrine, merge gate, and
rules sections makes it impossible to justify the lead opening a
ticket file directly.

### Phase 2: New role files

Create the two role definitions under
`claude/skills/marathon/agents/`:

- **`advisor.md`** — Purpose, read scope (mental-model / plans /
  `_index.md`, never tickets or code), query response format
  (structured + cited), explicit non-scope (no code, no tickets,
  no decisions), refresh protocol (lead-initiated re-read only, with
  the caching rationale inline so the agent understands why it must
  not self-refresh).
- **`clerk.md`** — Purpose, ticket scope (full R/W), **skill load via
  direct Read** of `~/.claude/skills/write-ticket/SKILL.md` at
  startup (since marathon uses general-purpose spawns, not custom
  agent definitions, the `skills:` frontmatter mechanism is
  unavailable; document this fallback inline), edit directive
  handling, explicit non-scope (no code, no mental-model, no design
  decisions), query response format for ticket state questions,
  multi-ticket handling.

Both files must direct the agent to read
`claude/skills/marathon/agents/_common.md` first (existing convention).

Success criterion: both files are self-contained enough that a
freshly spawned agent can operate from role file + `_common.md`
alone, without guidance from the lead.

### Phase 3: `_common.md` touch-ups (if needed)

Inspect `claude/skills/marathon/agents/_common.md` for anything that
assumes executor semantics (planner/implementer/reviewer/worker) and
doesn't apply to advisor/clerk. Likely small edits:

- Clarify that some roles are **resident** (advisor, clerk) and do
  not follow the "report and retire" pattern.
- Any shared instructions that would confuse advisor (e.g., "run
  tests") should be scoped to executor roles.

Phase may be dropped after inspection if no changes are required.
Mark `[dropped]` instead of removing if so.

### Phase 4: Migration / housekeeping

- Note in `claude/migration-guide/` if the changes affect downstream
  projects. Marathon is used by downstream projects via symlink, so
  any new role files appear automatically, but the SKILL.md changes
  might warrant a one-paragraph guide entry.
- If `ai-docs/_index.md` exists in this repo, update it; otherwise
  skip.

## Dependencies Between Phases

Phase 1 and Phase 2 are mutually referenced — SKILL.md mentions the
role files, and the role files assume SKILL.md's doctrine. Implement
together in a single round if possible, otherwise Phase 2 first so
SKILL.md's references resolve to real files.

Phase 3 depends on Phase 2 (need roles defined to check common file
coverage).

Phase 4 is standalone and may come last.

## Open Questions

- Should `clerk` handle `/write-spec` invocation too, or is that left
  to the lead's judgment? `/write-ticket` already cross-calls
  `/write-spec` in its conventions, so clerk inherits it transitively.
  Leaning toward: clerk handles spec as a cascade of its ticket
  responsibility.
- **Follow-up epic (deferred):** migrate marathon team roles to
  first-class custom agent definitions under `.claude/agents/<role>.md`
  so they can declare `skills:` frontmatter and drop the Read-at-startup
  pattern. Blockers to resolve first: (1) marathon's dynamic naming
  (`impl.alpha`, `impl.beta.expert`) currently relies on
  `subagent_type="general-purpose"` with inline labels; custom agents
  have fixed names per definition file. (2) Downstream consumption via
  symlink needs to still work. (3) `.expert` model upgrade pattern
  needs a story (same role, different model — custom agent definitions
  pin a single model). This is an epic-sized refactor.
- `_index.md` team-board format — this ticket promotes `_index.md` to
  lead's working memory, but doesn't specify the new layout. A
  follow-up will likely need a small format spec (sections for session
  focus, active ticket pointers, advisor pointers, long-lived project
  summary). Addressing this inline in Phase 1 is acceptable if the
  shape emerges naturally; otherwise spin off.
