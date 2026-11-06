# whiteboard: never repeat board content in the conversation

User directive (2026-11-06, whiteboard dogfood): while the whiteboard is in use,
the lead repeats in the terminal what it already wrote on the board (summaries
of items, recommendations, status). The user wants one strongly emphasized line
in the whiteboard playbook: content written on the whiteboard is only pointed
to ("see the board"), never repeated in the conversation.

## Assignment

1. In `agents-plugin/rsrc/whiteboard/whiteboard.md`, add one strongly
   emphasized line (bold or a short imperative sentence, placed where the
   lead reads it every turn, most likely `#### Each turn` near line 102)
   saying: what the update wrote on the board is not repeated in the
   terminal or chat; the turn's terminal output is the pointer (or embed)
   line plus any fixed-format lines a playbook mandates. One line; do not
   restructure surrounding text.
2. Mirror the same edit into `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`
   and, per precedent c6339b076, `agents-plugin-pi/rsrc/whiteboard/whiteboard.md`;
   regenerate any manifest the mirroring manual requires.
3. Before editing, read `ai-docs/manuals/shipped-surface-boundary.md`,
   `ai-docs/manuals/skill-authoring.md`, and `ai-docs/manuals/wsflow-mirroring.md`.
   The line is shipped text: host-neutral, no reference to this repository.

## Verification

The mirror and manifest tests `wsflow-mirroring.md` names, and any test that
pins the whiteboard playbook body (search `agents-plugin-tool` for
`whiteboard.md`). Report the exact line added.

## Return boundary

Stop and report before changing any other playbook sentence, the workflow
manual's `### Whiteboard` section, tool schemas, or rendering code.
