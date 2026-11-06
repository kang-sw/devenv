# whiteboard: board text follows the conversation language

User decision (2026-11-06, whiteboard item (207) a): resolve idea ticket
`261106-feat-whiteboard-board-text-language` by delegating one line in the
whiteboard playbook. Background is in that ticket.

## Assignment

1. In `agents-plugin/rsrc/whiteboard/whiteboard.md`, add one line, placed
   where the lead reads it while writing payload text (most likely under
   `#### Payload` or `#### Each turn`), saying: board text (titles, options,
   rationale, recommendations, answer readings, and the Now fragment) is
   written in the conversation language, the `workflow.lang` value when set;
   item IDs, class and status tags, `ok`/`no` answers, and option letters stay
   as written. One line; do not restructure surrounding text.
2. Mirror the same edit into `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`
   and `agents-plugin-pi/rsrc/whiteboard/whiteboard.md` (precedent c6339b076,
   ebf89cb07); regenerate any manifest the mirroring manual requires.
3. Before editing, read `ai-docs/manuals/shipped-surface-boundary.md`,
   `ai-docs/manuals/skill-authoring.md`, and `ai-docs/manuals/wsflow-mirroring.md`.
   The line is shipped text: host-neutral, no reference to this repository.
4. Close the idea ticket with `ws/tickets.close` (a resolution line naming
   the commit) in the same commit or a following ticket commit.

## Verification

The mirror and manifest tests `wsflow-mirroring.md` names
(`TestWsflowRsrcMirrorUpToDate`, `TestPiMirrorUpToDate`), and any test that
pins the whiteboard playbook body. Report the exact line added.

## Return boundary

Stop and report before changing any other playbook sentence, the workflow
manual's `### Whiteboard` section, tool schemas, or rendering code.
