---
title: "Human-relay interactive gate: package TTY-only test scenarios for one-shot human execution"
related:
  260902-feat-ws-pi-native-mvp: the motivating case — /ws-discuss can only be exercised in a real interactive Pi TTY, which no non-interactive driver could reach
related-mental-model:
  - workflow-skills
---

# Human-relay interactive gate: package TTY-only test scenarios for one-shot human execution

## Background

Some acceptance gates cannot be driven non-interactively by an agent. The
Phase 4 `/ws-discuss` gate (`260902-feat-ws-pi-native-mvp`) is the concrete
example: three non-interactive driver paths were exhausted (`-p` drops the
handler-injected turn; interactive `--mode json`/`rpc` needs a real TTY a pipe
can't supply; session-resume fails because the injected turn is never persisted),
and the fully-literal proof was only obtained when the **user ran the command by
hand** in an interactive Pi TUI and pasted the model's output back.

The user proposed generalizing this into a reusable pattern: package the
interactive test scenario so the human is a **thin relay** — run one command,
type one line, copy one result block back — and the lead adjudicates the pasted
result against the gate criteria. The explicit design constraint is to minimize
how often, and how much, the work "touches the user's hands."

## Problem shape

- Agent-driven gates are the default and stay so; this pattern is only for gates
  that are structurally TTY-only.
- Today such a gate is ad-hoc: the lead improvises the command, the human
  improvises what to copy, and the evidence mapping is reconstructed by hand each
  time. That is exactly the friction the user flagged.

## Desired capability (to design, not yet build)

A skill/playbook that, given a named interactive scenario, emits a **copy-ready
human-relay block**:

1. **Run** — one exact, copy-pasteable command (e.g.
   `pi -e agents-plugin-pi/src/index.ts`), with any environment prerequisites
   (`--offline`, cwd) pre-resolved.
2. **Do** — the single line(s) to type in the TUI (e.g. `/ws-discuss <topic>`).
3. **Copy back** — a precise description of which output block to paste
   (ideally delimited so the paste is unambiguous).
4. **Expected evidence** — the gate criteria the lead will check the paste
   against (for `/ws-discuss`: skill expanded, a bridged `ws__*` tool called, one
   `explore` child harvested), so adjudication is mechanical, not improvised.

The lead then verifies the pasted block against (4) and records PASS/FAIL with
the human run attributed as first-hand evidence (as the Phase 4 Result now does).

## Open questions

- Skill vs. playbook vs. a lightweight lead-side convention — what is the lightest
  home that still standardizes the four-part block? (Authoring a new skill is
  "ask first" per AGENTS.md.)
- How are scenarios named/stored — inline in the requesting ticket, or a small
  registry?
- Can any of the human touch be reclaimed later (e.g. an `expect`/PTY harness for
  environments where a controlling terminal *is* available), or is the human
  relay the durable answer for genuinely interactive harnesses?
- Evidence trust: the human pastes model output — how much does the lead
  re-verify vs. trust the paste? (Phase 4 treated it as first-hand user evidence.)

## Non-goals

- Replacing agent-driven gates where non-interactive driving already works.
- Building the skill in this ticket — this is capture + design intent only;
  construction waits on an explicit go-ahead.
