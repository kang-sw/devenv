# Tickets Workflow

A lightweight ticket system for a project run by a plain coding agent — no
specialized workflow tooling, only native file read/write/edit, shell
(`bash`, `git`), and a native search/explore agent. Place this file at the
project root and pull it into the project's agent context file (for example
`AGENTS.md` or `CLAUDE.md`) with an embed directive such as
`@TICKETS_WORKFLOW.md`, or paste it in directly if the host has no embed
mechanism. This project owns this copy; there is no automatic update path —
re-copy the file by hand when a newer version is wanted.

## Why This Exists

Two things quietly leak out of a project over time: what the user actually
decided, and why an approach was chosen over its alternatives. A ticket and a
commit message are the same memory, captured at two different moments. When a
decision forms, it lands in a ticket's decision ledger. When that decision
becomes code, the same *why* lands again in the commit that makes it real.
Everything below serves that one mechanism — recording a decision once when it
forms, and once more when it ships.

## The Ticket System

All tickets live under `ai-docs/tickets/`, one file per ticket, organized by
status directory:

- `ai-docs/tickets/todo/` — accepted, active work.
- `ai-docs/tickets/.done/` — completed work.
- `ai-docs/tickets/.dropped/` — abandoned work, kept for the record.

Status is the directory a ticket file lives in — never a separate status
field. Move a ticket between statuses with `git mv` when the project is a Git
repository; a plain file move otherwise.

### Filename

`YYMMDD-<category>-<slug>.md`, for example `250101-feat-retry-queue.md`.

- `YYMMDD` is the ticket's creation date and never changes, even after the
  ticket moves between status directories.
- `<category>` is one of exactly six words: `feat`, `bug`, `refactor`,
  `chore`, `research`, `epic`. Pick by plain-language fit: `feat` adds
  capability, `bug` corrects a deviation from intended behavior, `refactor`
  restructures internals without an intended external change, `chore` is
  maintenance or housekeeping, `research` is an investigation with no direct
  code change, `epic` groups related tickets under one outcome. Most
  day-to-day work is `feat`, `bug`, `refactor`, or `chore`; treat the category
  as a scanning label, not a process branch — every ticket in this system
  follows the same template and the same lifecycle regardless of category.
- `<slug>` is a short, kebab-case description.
- Once created, refer to a ticket by its filename stem
  (`250101-feat-retry-queue`), not by its full path — the stem survives a
  status move, a path does not.

### Template

```markdown
---
title: <short title>
related:             # optional — map of stem: relationship note
  250101-feat-retry-queue: prerequisite
completed:           # YYYY-MM-DD, added when moved to .done/
---

# <title>

## Background

<what this ticket is about, and why it exists>

<freeform notes as the work proceeds — findings, approach, implementation
detail, whatever the work needs. No fixed structure here; this section is
where the ticket earns its keep as a working document, not a form.>

## Outcome Ledger

### Confirmed Decisions
<!-- Normative choices the user explicitly confirmed. Never add your own
     inference or proposal here — see the invariant below. -->

### Open Questions
<!-- Unresolved choices. Log one here and keep working instead of blocking on
     an answer — see the rule below. -->

### Rejected Alternatives
<!-- Alternatives considered and turned down, with the reason when it
     clarifies later. -->
```

Open a new ticket for a new unit of work; append to an existing one when the
work is a continuation of a decision already on record there.

## Autonomous Operation

Record decisions and manage ticket status without asking first — waiting for
sign-off on every ticket update defeats the point of a running memory. Two
rules keep that autonomy safe:

- **Capture is search-first.** Before opening a new ticket, search
  `ai-docs/tickets/` for a ticket the new information belongs to: a plain
  `grep`/`glob` over ticket titles and content is the default and usually
  enough. Escalate to the native Explore agent only when the search turns up
  more than one plausible match and picking wrong would misfile the decision.
  When you do escalate, name the model explicitly — never leave it unset,
  since an unset model inherits the calling session's model, which is
  needlessly expensive for a bounded lookup. Use the cheapest available model
  (for example haiku) for a straightforward locate query, and a stronger
  model (for example sonnet) only when the read requires synthesizing
  ambiguous or conflicting evidence.
- **Status transitions are yours to make.** Move a ticket from `todo/` to
  `.done/` (adding the `completed:` date) once its work is actually finished,
  and to `.dropped/` once it is actually abandoned, without waiting for
  instruction to do the move itself.

## The Decision Ledger

The three headers under `## Outcome Ledger` carry the decision record; keep
their names exactly as given so the ticket stays readable by a future,
fuller-featured version of this same system.

- **`Confirmed Decisions` is user-only.** Only a choice the user explicitly
  stated goes here. Never promote your own inference, guess, or proposal into
  this section, even when it turns out right — a reader must be able to trust
  every line under this header as something the user actually decided.
- **`Open Questions` is log-and-continue, not block-and-ask.** When a real
  question comes up mid-work, do not stop and wait for an answer: write it
  under `Open Questions`, make the best call you can to keep moving, and
  mention the open question as one line in your next reply to the user. This
  is what makes the system autonomous instead of a chain of approval gates.
- **`Rejected Alternatives`** records what was considered and turned down.
  Skip the reason when it is obvious from the alternative's name; state it
  when a future reader would otherwise re-litigate the same option.

## Commit `## AI Context`

Every commit gets an `## AI Context` section — unconditionally, not only when
the change feels big enough to deserve one. A "when needed" judgment call is
the first thing that erodes under time pressure, and it erodes exactly when a
project has grown large enough to need this memory most. Make the body
proportional instead of optional: one line for a trivial commit, a fuller
account of rationale, rejected alternatives, and user directives for a
substantive one. Capture the *why* a diff cannot show, not a restatement of
the diff. Reference the related ticket's stem when the commit is ticket-driven.

```text
<type>(<scope>): <summary>

<what changed — brief>

## AI Context
- <decision rationale, rejected alternatives, user directives, etc.>
- ticket: <stem>
```

## Two Rules That Protect This Record

- **Evidence before claims.** Run the verification and read its actual output
  before telling the user something works, passes, or is done.
- **No performative agreement.** Don't just agree. Restate what was actually
  asked, verify it against reality, and then either act on it or push back —
  agreement that skips verification is how a wrong decision gets written down
  as a confirmed one.

## Language

Ticket bodies and commit `## AI Context` are written in English by default.
Human-facing UI strings are always exempt. If this project's `AGENTS.md` or
`CLAUDE.md` declares a different working language for AI-authored records,
follow that declaration instead — this file only sets the default.
