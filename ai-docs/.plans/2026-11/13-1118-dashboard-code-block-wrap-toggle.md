# Dashboard code blocks: a line-wrap toggle button at the top right

Owner 2026-11-13, screenshot `C:\Users\user\.cache\ws@kang-sw-devenv\dashboard\sessions\shrankhumpbackuncannyelaborate\inbox\attachments\17ebe6474d306319-1.png`:
a dashboard code block with line numbers (gutter 1..11, dark theme, a
horizontal scrollbar because long lines do not wrap). The owner wants a small
button at the block's top right that toggles line wrapping.

## Do

1. Find the renderer that produces that line-numbered code block (the
   dashboard assets under `agents-plugin-tool/internal/wsdashboard/assets/`
   and any shared markdown renderer the whiteboard page uses; the screenshot
   shows prompt-like text, so a config/prompt-override view or a markdown
   code fence are the likely sources). Apply the toggle to every place that
   renders this same code-block component, not one view only; name them in
   the report.
2. Add a top-right toggle button on the block (icon-sized; if a copy button
   exists, sit next to it and match its show-on-hover behaviour). Toggled on:
   lines wrap (`white-space: pre-wrap; overflow-wrap: anywhere` or
   equivalent) and the line-number gutter stays aligned with the logical
   line starts (each number at the top of its wrapped line). Toggled off:
   today's behaviour.
3. Default off (today's behaviour). Optionally remember the choice in
   `localStorage` (one global key) — technical call, state it.
4. Tooltip / aria-label for the button, with any new string in all
   `agents-plugin-tool/internal/wsi18n/tables/*.json` (or the page's existing
   i18n path for JS strings).
5. Works on blocks rendered after page load (live-streamed content), not only
   on the initial render.

## Out of scope

- Restarting the daemon (the lead does the live check after merge).

## Verification

- Go tests on the remote w5090 (`D:/sys_lock.sh remote w5090 run <who> <min>
  --sync <worktree> -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd
  agents-plugin-tool && go test ./internal/wsdashboard/... ./internal/wsi18n/...
  ./internal/wswhiteboard/... -count=1'`).
- Touched node-backed JS tests run locally with node at
  `C:/Program Files/nodejs` under the local lock (`bash D:/sys_lock.sh run
  <who> <min> -- <cmd>`); add a test for the toggle (class flip and gutter
  alignment structure) in the existing test style.
- Commit in the worktree with `## AI Context`; state what was not checked
  live. Temp files only under D:/Temp/<purpose>/, deleted when done.
