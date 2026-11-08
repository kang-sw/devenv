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

## Open Decision Queue

Last ID: (323)

(320) [open] [policy] The support agent may write to the open store only on the user's explicit request in the chat, as the viewer's own member identity with agent provenance (the `agent` chip); it never writes to another store, and cross-store search stays read-only.
- Context: epic decisions (114)/(122) say a local AI searches the user's stores read-only and writes nothing into another store; (123) b says an agent records automatically only for tasks assigned to its user and anything else on request.
- Alternative: read-only everywhere (answers and card references only).
- Alternative: also auto-record like a lead session's agent under (123) b.
> On-request writes to the open store only: a support agent that cannot create or update an item on request is of little use, this stays inside (122)'s "nothing into another store", and (123) b already allows on-request recording.
(321) [open] [announced] Blocked by both the agent runner and the next tracker caller child (dashboard drawer, MCP/skill exposure (115)) - epic Child Tickets: the drawer this card lives in and the tracker tools the agent calls both belong to the caller child.
(322) [open] [policy] One ongoing conversation per store, kept across drawer opens and days, with a "new conversation" action that starts a fresh backend session.
- Context: the runner keeps a sustained process per conversation with an idle timeout ((308) a); the backend session resumes after an idle exit.
- Alternative: a conversation list per store; a fresh conversation per drawer open.
> One ongoing conversation plus "new": matches a support-desk feel, keeps the card simple, and a list can come later.
(323) [open] [technical] The agent shows a card by calling a designated tracker tool (for example `tracker.show(item)`), and the renderer draws that call as the card-reference entry - over a text marker inside the reply, which is fragile to parse and to quote.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/`; `ai-docs/manuals/ws-mcp.md` if `internal/mcp/` is
  touched; UI strings go through the dashboard i18n tables.
- Model and effort choices offered by the modal come from the agent profile's
  backend; the runner owns how a change applies.

## Phases

### Phase 1: chat card

To be written once the queue settles and the prerequisites are planned.
