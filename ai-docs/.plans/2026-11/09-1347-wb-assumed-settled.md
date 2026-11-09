# Whiteboard: assumed items render and count as settled

User request (board, 2026-11-09, verbatim): "이거 assumed는 그냥 완료 처리로 두고, 접혔을 때 표현만 좀 다르게 하죠. assumed가 너무 크게 보여서요. 취급을 "선택된 정책 항목"이랑 같은 레벨로 두게요."
Board items (92)-(94) state the settled scope below.

## Outcome

An item with status `assumed` is treated like an answered policy item everywhere the page decides "waiting on the user", and renders as a settled row, distinguished only by a small marker.

- **Render (92):** in `agents-plugin-tool/internal/wswhiteboard/view.go` an assumed item goes to the topic's settled list (the same Expanded/Collapsed rotation as answered/withdrawn/superseded, ordered by its settle time), not `TopicView.Assumed`. Its row uses the settled-row template with the picked-option chip, plus a small "assumed" marker in the row head (muted assumed-color text, not the full magenta bar). Remove the big `card assumed` template and its CSS/JS if nothing else uses it.
- **Counts (93):** assumed no longer counts as waiting: header and topic assumed chips (`CountAssumed`, `chip.assumed`), the open-question pill (`assets/pill.js` item selector and answered()), the tab-title count, quiet/waiting topic ordering in view.go, the nudge (`nudge.go`: a topic whose only non-settled items were assumed is now idle), and the bulk "confirm all" in `assets/form.js` (assumed cards no longer exist, so drop that path and the reply-builder segment for them). Remove now-unused i18n keys only if the i18n tests require it; otherwise leave keys that other tables share alone (the wsi18n key tests must pass).
- **Close (93):** `wb:close` (store.go ~:483-527) withdraws only `open` items; assumed items stay assumed. Update the `info:` wording accordingly.
- **Store (94):** keep `StatusAssumed` and `wb:answer assumed="true"` unchanged; a later `wb:answer` without assumed still turns it `answered`.
- **Playbook (94):** update `agents-plugin/rsrc/whiteboard/whiteboard.md` (the `assumed="true"` sentence says the item "stays highlighted until..."; the `wb:close` sentence says it withdraws items still `open` or `assumed`) to describe the new behavior: an assumed item shows as a settled row with an assumed marker; the lead still records the user's later confirming/correcting words on that ID. Mirror the same edit in `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`. Read `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`, and `ai-docs/manuals/shipped-surface-boundary.md` first. Regenerate any manifest/digest the repo tests check for these files (see the tests that fail).
- Dashboard embedding: the dashboard re-renders the whiteboard regions; check `internal/wsdashboard` for any selector on `.card.assumed` and update it.

## Verification

From `agents-plugin-tool/`: `go test ./internal/wswhiteboard/ ./internal/wsdashboard/ ./internal/wsi18n/ ./internal/wsdoc/... -count=1` plus any package whose test pins the playbook files (run `go test ./... -count=1 -timeout 40m` once at the end), `go vet ./internal/...`, `node --check` on changed JS. Update or add render tests: an assumed item renders as a settled row with the marker, is not counted in header chips, the topic is not waiting, and wb:close leaves it assumed. A headless browser screenshot of a board with an assumed item is welcome if a dev render path exists; otherwise say it was not checked.

Return when committed on branch `wb-assumed-settled` with verification passing.
