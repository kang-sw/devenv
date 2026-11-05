# config bucket follow-up minors

Follow-up fixes for minors left open by 261105-feat-lead-ticket-review-auto-ack
(merged 03a23ab5f) and 261105-feat-whiteboard-knob-harness-and-session-scope
(merged b16d2bf8b). Lead decisions from the user conversation, 2026-11-05.

## Assignment

1. (149) `agents-plugin-tool/internal/mcp/ticket_review_text_test.go:83`
   builds its expected off-text (`wantOff`) by calling the function under test.
   Replace that with an independent expectation (literal text or a check that
   does not route through the function being tested), so the test can fail
   when the function is wrong.
2. (150) `config.get` now resolves harness-bucketed keys bucket-aware, which
   also changed how `prompt.*` keys resolve through it
   (`internal/mcp/config_manifest.go` near line 509, `server.go` near 3082).
   Add a test pinning `config.get` for a `prompt.<point>` key: a harness
   bucket value wins over `.all`, and the result names the answering scope.
   Pin the current behavior; do not change it.
3. (151) In `config.list`'s scoped overrides view (`formatConfigView`,
   `server.go` near 2515, fed by `wsconfig.View.ResolvedOverrides`), the
   `workflow.whiteboard_enabled` row resolves with a plain `Get` of the bare
   key, so it can show the builtin `off` while the effective value for this
   harness is `on` from a bucket key. Make that row resolve through
   `resolveHarnessBucketed` for the session harness, matching the catalog's
   `current`. Keep the per-bucket rows (`workflow.whiteboard_enabled.<harness>`)
   as they are: they show where stored values live. Add a test.

## Out of scope (user decisions)

- A reset cannot clear a legacy bare `workflow.whiteboard_enabled` value; left
  as-is because the knob never shipped (153).
- 9ca6f0039's missing Co-Authored-By trailer is not rewritten (152).

## Verification

`go test ./...` in `agents-plugin-tool`. If any shipped text or manifest
changes (not expected), follow `ai-docs/manuals/wsflow-mirroring.md`.

## Return boundary

Stop and report before changing any resolution order, write path, reset
behavior, or shipped playbook text.
