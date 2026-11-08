---
title: "ws tracker support-agent chat card: a daemon-hosted agent conversation inside the tracker drawer"
parent: 261107-epic-ws-personal-common-store
related:
  261108-feat-ws-daemon-agent-runner: prerequisite (agent launch, profiles, wslive output, REST control)
---

# ws tracker support-agent chat card: a daemon-hosted agent conversation inside the tracker drawer

## Background

The user (2026-11-08) wants a local support agent in the tracker: a chat
card in the drawer's cards layout where the user talks to a daemon-hosted
agent about the open store. The mockup already places it (board (300) a,
(303) ok, merged `1d7fbce0a`): a "Support agent" card with fixed height,
internal scroll, a disabled composer, and a sample conversation that
includes a tracker card reference. This child makes it real on top of the
agent runner (`261108-feat-ws-daemon-agent-runner`).

## Decisions

- **Placement** (board (303) ok, (304) a, (305) ok): the chat is one card in
  the drawer's cards layout, fixed height with internal scroll and the newest
  message at the bottom. It has no view-mode picker.
- **Reuse the dashboard conversation view** (board (302) ok): the agent's
  conversation is rendered by the existing conversation renderer from the
  runner's `wslive` session directory; no chat-only renderer. The renderer
  gains one entry kind, a tracker card reference: a mini card (title,
  status, priority, due, assignees) of one item from the store's view state
  that opens the item's thread sheet on click.
- **Reuse the dashboard composer without its bottom badges** (user, board
  topic): the composer is shared, but the usage and other marks under it are
  left out and replaced by a model and effort selection modal.
- **Mockup scope stays placement only** (board (300) a): chat behaviour and
  the API are settled here, not in the mockup.
- **Prerequisites** (board (321) ok): blocked by the agent runner and by the
  next tracker caller child (the dashboard drawer this card lives in and the
  tracker MCP/skill tools the agent calls, (115)).
- **One ongoing conversation per store** (board (322) a): kept across drawer
  opens and days, with a "new conversation" action that starts a fresh
  backend session. Rejected for now: a conversation list per store; a fresh
  conversation per drawer open.
- **Card references come from a designated tool call** (board (323) ok): the
  agent shows a card by calling a tracker tool (for example
  `tracker.show(item)`), and the renderer draws that call as the
  card-reference entry. Rejected: a text marker inside the reply, fragile to
  parse and to quote.

## Open Decision Queue

Last ID: (325)

(320) [confirmed] [policy] The support agent may write to the open store only on the user's explicit request in the chat, as the viewer's own member identity with agent provenance (the `agent` chip); it never writes to another store, and cross-store search stays read-only. User note (board (320) a): also open a limited, read-only git search over the user's other local projects (git log, commit search, listing).
(324) [open] [technical] Local git search is a dedicated read-only tool the support profile allows (log, show, grep, blame, ls-files over a repository path; argument-validated, no write-capable options), over Bash prefix rules such as `Bash(git log:*)` - prefix rules cannot block write options (`git log --output=<file>`, `git diff --output`), `git -C <repo>` and `cd <repo> &&` forms escape a fixed prefix, and Pi's `--tools` has no per-command granularity, while one tool behaves the same on both backends.
(325) [open] [policy] The git search tool reaches the projects the dashboard daemon already knows (every project root that has run a ws session) plus extra roots from a config knob, and refuses any other path.
- Context: the user wants commit search in "제 로컬의 다른 프로젝트"; the epic's local-AI rule ((114)/(122)) allows read-only search to answer the user's own query.
- Alternative: only roots listed in a config knob (default empty).
- Alternative: any git repository path on the machine.
> Daemon-known projects plus a knob: zero setup for the projects the user works in, a bounded and visible set, and the knob covers repositories ws never ran in.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/`; `ai-docs/manuals/ws-mcp.md` if `internal/mcp/` is
  touched; UI strings go through the dashboard i18n tables.
- Model and effort choices offered by the modal come from the agent profile's
  backend; the runner owns how a change applies.

## Phases

### Phase 1: chat card

To be written once the queue settles and the prerequisites are planned.
