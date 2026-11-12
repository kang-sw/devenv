# Plan: whiteboard output while the user is watching the board

## Intent

User directive (2026-11-12, from the whiteboard): rework the whiteboard
playbook's output rule into a two-way rule keyed on whether the user is
watching the board. When the user is watching the whiteboard, the lead puts
as much of the conversation as practical on the board and does not repeat
board content in the chat. When the user is not watching, the chat carries
the conversation as usual. Keep the edit small.

## Current state

`agents-plugin/rsrc/whiteboard/whiteboard.md` `## Each turn` ends with one
bold line (added in ebf89cb07): "Never repeat in the terminal or chat what the
update wrote on the board ...". It applies unconditionally. Replace that line
with the two-way rule; leave the surrounding pointer-line text as is unless
the new rule needs a small adjustment for coherence.

## The "watching" signal

The lead treats the user as watching the board when the user's current
message arrived from the whiteboard, which both hosts label with a first line
`[user message from the ws whiteboard]`
(`agents-plugin-pi/src/adapter-label.ts` `WHITEBOARD_MESSAGE_LABEL`;
`agents-plugin/hooks/claude-mod/dashboard.ts`), or when the user says they
are watching it. Otherwise the lead treats the user as reading the chat.
Name the label literally in the playbook so the reader can detect it.

## Mirrors and manifests

- Mirror the identical edit to `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`
  and `agents-plugin-pi/rsrc/whiteboard/whiteboard.md`.
- Regenerate rsrc manifests the way ebf89cb07 did (ws and wsflow regen; Pi
  manifest synced from the canonical one); follow
  `ai-docs/manuals/wsflow-mirroring.md`.

## Constraints

- Read `ai-docs/manuals/skill-authoring.md` and
  `ai-docs/manuals/shipped-surface-boundary.md` first; the line ships
  downstream and must not reference this repository.
- No other playbook or skill changes. If the rule seems to need a change
  elsewhere (a skill, the workflow manual), stop and report instead.
- Run the rsrc/manifest tests that cover these files.
