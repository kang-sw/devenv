---
title: "Redesign ws-ask as a fork-less async owner-question queue with a sequential prose-modal tier"
related:
  260908-research-ws-pi-ws-ask-removal: the cost/decision anchor this resolves — the fork-cost concern is answered by redesign (fork-less), not removal; that ticket records the settled direction and points here
  260904-feat-ws-pi-side-thread-fork-question-surface: the mechanism this redesigns (registry, widget, /thread, /answer, injection are reused); this reverses its "no fork-less quick-answer path / discussions dominate" decision on dogfood evidence and extends its §7 entry_id anchoring to the answer return path
  260903-feat-human-relay-interactive-gate: the "minimize how much, and how often, the work touches the user's hands" philosophy the pre-authored proposal/options follow
  260906-workset-ws-pi-dogfood-ux: the dogfood UX board this owner-friction fix belongs to
---

# Redesign ws-ask as a fork-less async owner-question queue with a sequential prose-modal tier

## Background

`ws-ask` / `ws-resolve` (`agents-plugin-pi/src/ask.ts`, landed by `260904`)
register an owner-facing question and, on `/answer`, spawn a **discussion fork**
at the lead's tip to run an overlay chat. Owner dogfood (`260908`) found the
fork rarely reuses the prefix cache and causes abrupt cost spikes; both tools
are currently hidden from the active tool surface (`ac998f77`, `a8cf1183`,
`5f366eff`) as a reversible mitigation, and `260908` left permanent removal
undecided.

The settled resolution (discuss, 2026-09-11) is **redesign, not removal**. The
cost driver is exactly one thing: the discussion fork spawned when a
**lead-raised** question is opened. That fork was always waste — for a
lead-raised question the main lead *is* the owner's conversation partner, so a
"discussion" simply continues in the main channel; a context-inheriting peer is
only ever needed for a **fork-raised** question, where the asker is a live
background worker that must be resumed with the answer (and that worker already
exists — attach, never spawn). Removing the lead-raised discussion-fork spawn
kills the cost with no capability loss.

This ticket reframes the tool as an **async, non-blocking question queue**
(Codex's queued-question shape, not Claude's blocking interview modal) and
replaces the live overlay chat with a new **single-shot sequential prose-modal
tier**, distinct from the existing `audit` (read-only viewer) and `interaction`
(live overlay chat) tiers.

## Decisions

Settled in discuss (2026-09-11); each maps to a confirmed decision (D1–D6).

- **D5/D6 — resolve `260908` by redesign.** The fork-cost concern is answered by
  making the path fork-less, so the removal question is moot. `260908` is
  updated to record this and point here; it is not deleted or promoted. This
  ticket is Pi-adapter-local and carries **no `epic/refound` ready-gate** — it is
  independent of that epic. Rejected: deleting `ws-ask` and its
  question/thread state (throws away the async-queue value that dogfood actually
  wanted).

- **D4 — rename to `ws-queue-question`.** The change is a **contract change**, not
  a cosmetic rename: the semantics become async / non-blocking / "answer when
  ready." The name `ask` drove the model to treat it as "ask now," which the
  owner would rather handle by discussing with the lead directly. `ws-resolve`
  is renamed consistently to signal "withdraw a queued question" (exact token an
  implementation detail, not load-bearing). The **blocking** question path is
  unaffected and stays `ws-report-to-lead(kind: "question")` (a worker actually
  parks and waits); the two contracts are now cleanly separated.

- **D1 — drop the discussion-fork spawn entirely (fork-less).** `/answer` on a
  lead-raised queued question no longer spawns a fork; the owner's prose answer
  is injected back into the lead session on idle (the existing `followUp`
  custom-message path). A genuine multi-turn discussion of a lead-raised
  question continues in the main lead channel, or the lead explicitly calls
  `ws-fork` / `ws-execute` if it decides it needs a peer. Rejected: an opt-in
  "escalate to discussion fork" tier — a non-goal until dogfood shows a concrete
  gap (keeps the surface small; reversal of `260904`'s "discussions dominate,
  one path" premise on dogfood evidence).

- **D2 — one unified modal for both origins.** The prose modal serves both
  lead-raised and fork-raised questions. On submit, a lead-raised answer is
  injected into the lead session; a fork-raised answer is routed to the live
  waiting worker via the existing `ws-agent-send` / report-wait path — **no new
  process** in either case. The `260904` overlay-chat component is retired for
  this surface, but the fork-raised **live-attach** semantics survive in the
  cheaper form (route prose to the existing agent).

- **D3 — the new prose-modal tier is reversible within a single edit; no
  accidental commit.** (Explicitly *not* an Enter-to-accept fast path.)
  - Response channel is **prose by default**. The agent may embed enumerated
    options as `#1. #2. ...` in the question text; the owner answers in prose
    referencing them ("1번이요", "#1") or freeform.
  - Questions are laid out **sequentially**; the owner moves between them with a
    keybinding (e.g. `ctrl+[` / `ctrl+]`) and edits each freely.
  - **Submission is gated.** Pressing Enter on the final question raises a
    **confirmation prompt whose cursor defaults to "No"**; nothing is committed
    until that confirm. Everything before it is reversible inside the single
    edit session. An accidental Enter must never submit.
  - Never auto-pop (carried from `260904` §5): widget + notify only; the owner
    opens the queue when ready — that is the whole point of the async reframe.

- **D3 (return path) — anchor answers with ask-time context.** On completion the
  queue re-reports to the lead **per question, with each question's surrounding
  context**. Because the answer may arrive after the lead has lost that context
  (compaction), the ask-time snapshot — the offered `#1/#2` options and an
  ask-time anchor (short commit hash and/or the `260904` `entry_id`) — is
  captured **at queue time** and travels with the answer so the lead can recover
  where the question came from. This is the mirror of `260904` §7 (which
  anchored the fork's *inbound* context) applied to the *return* path.

## Constraints

- **Pi-track-local authorship (AGENTS.md clause 1).** Everything here is
  `agents-plugin-pi/` (the adapter's own tools and TUI). No `agents-plugin-tool/`
  (ws-mcp Go) or shared `agents-plugin/skills/` change; not gated on
  `epic/refound`.
- **Cross-harness note (flag, not scope).** "Queue a question to the owner" is a
  pattern Codex also has; it is a candidate future ws-mcp harness-peer surface
  (which would then be develop-authored per the harness-peer clause). Kept
  adapter-local here; revisit promotion only once the contract is stable.
- Reuse, do not rebuild: the persisted thread registry
  (`<lead session>.ws-threads.json`), the `aboveEditor` "N pending" widget,
  `/thread` / `/answer` / reopen shortcut, never-auto-pop, `MAX_CONTEXT_CHARS`
  warning, and idle-`followUp` injection all carry over from `260904`.
- Lifting the current tool-surface hide (`ac998f77` / `a8cf1183`) is part of
  landing the fork-less path, not a separate re-enable of the old fork behavior.

## Prior Art

- `260904` `ask.ts` (registry, `/answer` / `/thread`, widget, injection),
  `overlay-chat.ts` (the live chat this retires for the question surface),
  `conversation-view.ts`; `audit.ts` (the read-only viewer tier the new tier is
  distinct from). The `260904` Prior Art `questionnaire.ts` (a custom component
  raised from a tool `execute()`) is the building block for the sequential prose
  modal.
- `260904` §7 `entry_id` anchoring + post-compaction excerpt insertion — the
  inbound analog of D3's return-path anchoring.

## Phases

### Phase 1: Async contract — rename, fork-less lead-raised path, unified routing

Rename `ws-ask` → `ws-queue-question` and `ws-resolve` consistently; make the
contract async/non-blocking. Remove the discussion-fork spawn: a lead-raised
queued question's answer is injected into the lead session on idle; a
fork-raised question's answer routes to the live waiting worker via
`ws-agent-send` (no spawn in either case). Capture the ask-time snapshot
(offered options + short-hash/`entry_id` anchor) on the thread record and carry
it on the return report. Lift the tool-surface hide as part of landing the
fork-less path. Headless (`--mode rpc`) baselines from `260904` §8 must keep
working.

Verification: opening a lead-raised queued question spawns **no** fork process
(`ws-agent-list` / `ps` shows none) and its answer appears as an injected lead
message on idle; a fork-raised question's answer reaches the waiting worker and
its `kind: "final"` report reflects it, with no new process; the registry
records the ask-time anchor and it appears on the returned report; the adapter
suite is green.

### Phase 2: Sequential prose-modal tier

Depends on Phase 1. Build the new modal tier (distinct from `audit` /
`interaction`): sequential questions navigable with a keybinding
(`ctrl+[` / `ctrl+]`), prose editor per question, agent-embedded `#1. #2.`
options answerable in prose, **gated submission** (Enter on the last question →
confirm prompt defaulting to "No"; reversible within the single edit), never
auto-pop. Retire the overlay-chat component for the question surface. Wire the
completion path to the Phase 1 per-question re-report with ask-time anchors.

Verification (agent-driven where possible; TTY-only items packaged as a
`260903`-shape owner runbook): the modal renders the queued questions with their
options; navigation moves between questions without submitting; an accidental
Enter mid-queue does not commit; Enter on the last question raises the
No-defaulted confirm and only that confirm submits; each answer re-reports to
the lead with its question context and, when the lead is compacted past the ask
point, the recovered options + ask-time anchor.
