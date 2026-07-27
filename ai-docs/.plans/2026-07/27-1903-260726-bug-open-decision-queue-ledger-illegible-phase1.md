# Plan: The Open Decision Queue ledger can degrade to illegible while satisfying every stated rule — Phase 1: Make the ledger legible by construction

## Relevant Ticket Contract

- Queue item **subjects must be self-describing**: the subject carries the
  decision itself, not a label. `description` is optional detail that may not
  render and must never be load-bearing.
- **Restating each item in the response body is the documented default, not a
  recovery** — harness-independent, cannot silently degrade. The visible list
  is the record; prose is the channel.
- Do NOT mandate reprinting the whole queue every turn. Shape: full text of the
  item being asked, plus a one-line status roll-up of the rest.
- Two named copies to amend identically: `agents-plugin/rsrc/lead-write-ticket/task-list.md`
  and `agents-plugin/rsrc/lead-write-ticket/task-list.codex.md`. Not
  `lead-discuss` — it only routes and includes nothing (`includes: [task-list]`
  lives solely in `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`).
- Amend `lead-write-ticket`'s **On: Open Decision Queue** step 4 so asking an
  item includes restating it in the response body with a one-line roll-up.
- Keep the Markdown-checklist fallback path; apply the same self-describing
  rule to it.
- Do not weaken the queue mechanism itself — conveyance only.
- Both rsrc regen commands are mandatory after any `agents-plugin/rsrc/` edit,
  run in order from `agents-plugin-tool/`, both with `-count=1`:
  `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
  then
  `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`.
  Never hand-edit `agents-plugin-wsflow/rsrc/`.
- Verification boundary (ticket, three points): (1) both task-list files assert
  self-describing subjects, mark `description` non-load-bearing, and apply both
  rules to the Markdown-checklist fallback; (2) `lead-write-ticket`'s ODQ step 4
  carries the restate-in-body obligation; (3) both regen commands run clean and
  produce no diff on a second run.

## Out of Scope

- `ai-docs/spec/workflow-skills.md` lines 327-334 (the ODQ conveyance paragraph)
  is the ticket's declared, confirmed spec target and the wording there will
  eventually need the same caller-visible amendment, but Phase 1's own bullet
  list and its three-point verification boundary do not name a spec-text edit
  or spec check. `lead-implement`'s post-verification pipeline closes spec
  updates for caller-visible changes as a separate step (see Codebase Findings)
  — leave that edit to that pass rather than pre-empting it here. Flagged only
  as a pointer, not a plan step.
- `260726-feat-doc-organization-autonomy-odq-admission-filter` (related ticket,
  narrows what enters the queue) — unrelated to conveyance, not touched.
- Any change to `lead-discuss` — confirmed it does not include `task-list` and
  needs no edit.

## Codebase Findings

- `agents-plugin/rsrc/lead-write-ticket/task-list.md#L1-L8` — full-ws included
  guidance fragment, appended into `lead-write-ticket.md` via `includes:
  [task-list]` in its frontmatter (`lead-write-ticket.md#L1-L5`). Current text:
  ```
  ## Included Guidance: Open Decision Queue Task List

  - Use a visible task list when the harness exposes one; otherwise print a concise Markdown checklist.
  - One queue item equals one unresolved decision that could change persisted ticket, spec, focus, or note text.
  - Track each item as `open`, `confirmed`, `rejected`, or `deferred`.
  - Update the visible list after every user answer and before asking the next item.
  - Before closing an item, rewrite it with `[confirmed]`, `[rejected]`, or `[deferred]`.
  - Treat the list as the consent ledger; do not replace it with hidden notes.
  ```
  Says nothing about subject/description composition or the Markdown fallback's
  item shape — this is the exact gap the ticket names.

- `agents-plugin/rsrc/lead-write-ticket/task-list.codex.md#L1-L8` — Codex host
  variant, same include mechanism, same gap. Current text:
  ```
  ## Included Guidance: Codex Open Decision Queue

  - Use Codex's visible plan/task-list surface for the Open Decision Queue.
  - Create one task per unresolved decision before ticket cleanup starts.
  - Mark an item complete only after the user confirms, rejects, or defers it.
  - Before marking an item complete, rewrite its text with `[confirmed]`, `[rejected]`, or `[deferred]`.
  - Refresh the task list after each user answer before asking about the next item.
  - Treat the task list as the consent ledger; do not persist an item that remains open.
  ```
  No Markdown-checklist fallback mentioned here (Codex always has the plan
  surface) — only the self-describing/non-load-bearing rule needs adding, in
  the equivalent structural position (second bullet, mirroring `task-list.md`).

- `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md#L90-L98` — `On:
  Open Decision Queue` handler, `kind: print` choreography playbook (Layer 3
  applies fully; no `enter.*` routing here, so Layer 2 does not apply). Current
  step 4:
  ```
  4. Ask about one queue item at a time; after each answer, update the visible queue status before asking the next item.
  ```
  This is the step the ticket names for the restate-in-body amendment.

- `agents-plugin/skills/lead-skill-authoring/SKILL.md` → rendered via
  `ws/playbook.print(name: "lead-skill-authoring")` — invariant checklist
  (Falsifiable, Actionable, One line, Context-free, Non-redundant,
  Doctrine-aligned) applies to every new/changed line below. `task-list.md` /
  `task-list.codex.md` are `Templates`-style include fragments rendered into
  the parent playbook, not a standalone skill file with its own Doctrine
  section — they inherit `lead-write-ticket`'s doctrine (recoverability of
  intent), which the self-describing-subject rule directly re-derives from.

- `ai-docs/ref/wsflow-mirroring.md#L175-L206` (Rsrc Tree Provisioning) —
  `agents-plugin-wsflow/rsrc/` is a generated byte-identical mirror of
  `agents-plugin/rsrc/`; `lead-write-ticket` is in the Shipped wsflow Skills
  list (`#L37`), so this edit is mirrored automatically by the regen commands,
  not hand-edited. Confirmed present at
  `agents-plugin-wsflow/rsrc/lead-write-ticket/{lead-write-ticket.md,task-list.md,task-list.codex.md}`.

- `ai-docs/spec/workflow-skills.md#L327-L334` — the confirmed spec target named
  in the ticket's `## Spec Impact`. Current paragraph:
  ```
  Discussion-derived ticket persistence is consent-gated. Before ticket cleanup
  writes mechanism decisions, rejected alternatives, future-scope hints, Result
  Forward notes, focus "Next" lines, or note/comment proposals, `lead-write-ticket`
  builds a visible Open Decision Queue, asks whether to persist the discussion
  when persistence was not already approved, resolves one queue item at a time,
  updates the visible queue after each answer, and writes only user-confirmed
  items. Rejected, deferred, unanswered, or otherwise unconfirmed items are omitted
  unless the user explicitly approves recording their status.
  ```
  No mention of subject composition or response-body restatement. Left for
  `lead-implement`'s post-verification "closes spec ... updates" pass
  (`ai-docs/spec/workflow-skills.md#L631`, `lead-update-spec` audits recent
  commits for caller-visible behavior changes,
  `ai-docs/spec/workflow-skills.md#L817`) rather than pre-written here, per Out
  of Scope above.

- `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go` — not touched:
  `lead-write-ticket` is not in the `Substitution-Mirrored Skill Generation`
  exception list (`ai-docs/ref/wsflow-mirroring.md#L131-L173`); its wsflow
  skill shim stays a thin `playbook.print` entry and needs no edit for this
  wording-only rsrc change.

## Implementation Plan

1. Edit `agents-plugin/rsrc/lead-write-ticket/task-list.md`: replace the block
   quoted in Codebase Findings with:
   ```
   ## Included Guidance: Open Decision Queue Task List

   - Use a visible task list when the harness exposes one; otherwise print a concise Markdown checklist, applying the same item rules below to it.
   - One queue item equals one unresolved decision that could change persisted ticket, spec, focus, or note text.
   - Write the subject as the decision itself, self-describing without opening the item; `description` is optional detail that may not render and must never carry load-bearing content.
   - Track each item as `open`, `confirmed`, `rejected`, or `deferred`.
   - Update the visible list after every user answer and before asking the next item.
   - Before closing an item, rewrite it with `[confirmed]`, `[rejected]`, or `[deferred]`.
   - Treat the list as the consent ledger; do not replace it with hidden notes.
   ```
   (Only the first bullet's fallback clause and the new third bullet change;
   the rest is unchanged and reproduced verbatim to keep the file whole.)

2. Edit `agents-plugin/rsrc/lead-write-ticket/task-list.codex.md`: replace the
   block quoted in Codebase Findings with:
   ```
   ## Included Guidance: Codex Open Decision Queue

   - Use Codex's visible plan/task-list surface for the Open Decision Queue.
   - Create one task per unresolved decision before ticket cleanup starts.
   - Write each task's visible text as the decision itself, self-describing without opening the item; treat any secondary note or description field as optional detail that may not render and must never carry load-bearing content.
   - Mark an item complete only after the user confirms, rejects, or defers it.
   - Before marking an item complete, rewrite its text with `[confirmed]`, `[rejected]`, or `[deferred]`.
   - Refresh the task list after each user answer before asking about the next item.
   - Treat the task list as the consent ledger; do not persist an item that remains open.
   ```
   (New third bullet added in the same structural position as `task-list.md`'s
   new bullet; no fallback clause needed here since Codex always has the plan
   surface.)

3. Edit `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`, `On: Open
   Decision Queue` step 4 (currently `4. Ask about one queue item at a time;
   after each answer, update the visible queue status before asking the next
   item.`) — replace with:
   ```
   4. Ask about one queue item at a time by restating its full text in the response body, followed by a one-line status roll-up of the remaining items; after each answer, update the visible queue status before asking the next item.
   ```

4. From `agents-plugin-tool/`, run both regen commands in order (each with
   `-count=1`):
   ```
   WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest
   WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
   ```
   The second command syncs `agents-plugin-wsflow/rsrc/lead-write-ticket/*`
   byte-for-byte from the three edited canonical files. Do not hand-edit the
   wsflow mirror.

5. After edits, run the skill-authoring **On: Fresh-Reader Audit** (from the
   already-loaded `lead-skill-authoring` playbook) against the three edited
   files, scoped to just the new/changed lines, since this is a wording-only
   guidance change: spawn a fresh reviewer with only the changed file content,
   classify findings `fix` / `risk accepted` / `intentional difference` / `out
   of scope`, and apply only `fix` findings before verification.

## Verification Plan

- Artifact check 1 (ticket point 1): `git diff -- agents-plugin/rsrc/lead-write-ticket/task-list.md agents-plugin/rsrc/lead-write-ticket/task-list.codex.md` and manually confirm both files assert self-describing subjects, mark `description`/notes non-load-bearing, and that `task-list.md` applies the rule to the Markdown-checklist fallback bullet.
- Artifact check 2 (ticket point 2): `git diff -- agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md` and confirm step 4 under `On: Open Decision Queue` carries the restate-in-body-plus-roll-up obligation.
- Regen check (ticket point 3), from `agents-plugin-tool/`:
  1. Run both regen commands from Implementation Plan step 4; confirm both report `ok`.
  2. `git status --short` / `git diff` over `agents-plugin/rsrc/manifest.json` and `agents-plugin-wsflow/rsrc/` to confirm the expected write side effects landed (manifest hash update, mirrored `lead-write-ticket/*` files updated to match).
  3. Re-run both regen commands a second time (`-count=1` again) and confirm `git status --short` / `git diff` show **no further changes** — proves the regen is idempotent and the first run fully captured the edits.
- Package check: `python3 -m unittest discover agents-plugin-wsflow/tests` from the repo root — must pass, confirming the wsflow distributed skill bundle and rsrc mirror stay consistent (`lead-write-ticket` is a shipped wsflow skill per `ai-docs/ref/wsflow-mirroring.md#L37`).
- Manual-only: no runtime probe exists for prose guidance; the above artifact/regen/package checks are the full verification boundary per the ticket's own framing ("an artifact check, since this is a text-only change with no runtime probe").

## Escalations

- None.
