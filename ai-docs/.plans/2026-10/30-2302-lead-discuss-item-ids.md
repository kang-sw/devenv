# Plan: lead-discuss item IDs on every class

## Outcome

In `lead-discuss`, every item the lead puts to the user carries its `(nn)` ID,
announced defaults and technical decisions included, the same as in a ticket's
Open Decision Queue (`agents-plugin/rsrc/lead-ticket/lead-ticket.md` Response
format). Only the acknowledgement stays waived in discuss.

## Why

The workflow manual's `### Asking the user` already says "Every item carries an
ID unique within the conversation", and lead-discuss imports "the classes and
IDs" from it. But the following sentence, "Announced and technical items need no
acknowledgement here", was read as "no ID either", so a lead numbered only
policy questions and the user could not cite the others.

## Change

Files (byte-identical mirrors today; keep them identical):
- `agents-plugin/rsrc/lead-discuss/lead-discuss.md`
- `agents-plugin-wsflow/rsrc/lead-discuss/lead-discuss.md`
- `agents-plugin-pi/rsrc/lead-discuss/lead-discuss.md`

Replace the `## Conversation` bullet that begins "Ask in the classes and IDs"
with:

```text
- Ask in the classes and IDs of the workflow manual's `### Asking the user`,
  in prose: say which items you take as announced defaults, which are your
  technical calls with their reasoning, and which are policy questions the
  user must decide before the next topic. Every item carries its `(nn)` ID,
  announced and technical ones included, so the user can cite any of them;
  those two need no acknowledgement here. No block format and no queue;
  nothing here is persisted.
```

Out of scope: `lead-workflow-manual.md` (`### Asking the user` stays unchanged),
`lead-ticket.md`, and any other playbook.

## Constraints

- Read `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
  and `ai-docs/manuals/shipped-surface-boundary.md` before editing.
- Update any content hash or generated manifest entry the edit invalidates
  (e.g. `agents-plugin/rsrc/manifest.json`, and its wsflow/pi counterparts if
  they hash this file), using the repository's own procedure.

## Verification

Run the tests that guard playbook manifests and wsflow/pi mirror drift, and
report the commands and output.
