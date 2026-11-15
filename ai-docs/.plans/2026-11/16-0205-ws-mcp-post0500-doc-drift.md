# Correct ws-mcp manual drift after v0.50.0

## Authorized outcome

MAIN assigned one documentation-only task on 2026-11-15 at 17:05 UTC. Correct the three Important findings in `/tmp/review-sweep-0501-fit.md` items 1-3 in `ai-docs/manuals/ws-mcp.md`, preserving the document's structure and existing workflow semantics. MAIN owns integration.

## Scope

1. Replace the obsolete held-slash-command steer refusal with the current command-to-steered-text behavior. Verify wsinbox.Steer and relevant callers/source.
2. Correct Claude streaming documentation for `--include-partial-messages`, the stream_event mapper, 100ms throttling and partial reset on block/message start. Explain the holder ring-skip behavior accurately: whole lines, skipped lines consume no sequence number.
3. Update GET /support/choices for optional `lead` bool, per-harness `permission_modes`, appended registered lead-only backends and a brief explanation of BackendRegistration. Verify the precise field placement and registration behavior in current source rather than guessing from the finding summary.

## Constraints and verification

- Read `ai-docs/manuals/skill-authoring.md` in full before changing the manual. Apply its tone and invariant checklist; prefer positive ownership/default statements and avoid defensive over-negation. Read applicable local instructions and the relevant manual sections with enough surrounding context.
- Edit only `ai-docs/manuals/ws-mcp.md` and commit this unchanged plan alongside it. No source, test, protocol, API or workflow behavior changes; do not fix the findings file's Minor follow-ups.
- Verify every new factual statement against current source. Include exact path:line evidence in the commit's `## AI Context` and final report. Check the diff for obsolete contradictory claims and run `git diff --check`. No Go tests/builds, downloads, daemon/shared-runtime/Windows actions, push, merge or swap are needed or authorized.
- Use targeted edits and one logical docs commit on `docs/ws-mcp-post0500-drift`. No independent fresh-read audit is authorized by this assignment; the lead will offer it after the draft as required.
- Return full head SHA, changed paths, concise source-backed summary and verification outcome. Keep the clean checkout held until MAIN reports integration and the lead releases it. Do not self-start another task.
