# Plan: ws-mcp-hosted-key-doc

Close the one Important finding (F1) of the v0.50.7 release-gate re-review of
develop 94de8f53f..5c7797981: `ai-docs/manuals/ws-mcp.md` contradicts the
hosted-lead-key hotfix (fff872331, merge 5c7797981).

Findings: `C:/Users/user/.cache/ws@kang-sw-devenv/proj/de04b4ac/review-paths/d60a13f3-01-findings.md`

## Facts (agents-plugin-tool/internal/wsagent/)

- `runner_process.go:225-232` `startLead` (non-attach launch, Claude backend
  only) calls `composeLeadInstructions`, which writes `<conv dir>/instructions.md`
  (atomic, 0600) with the lead's session key, "do not mint a replacement",
  and "call workflow_manual at session start and after any compaction".
- `claude.go:132-134` `claudeLeadArgs` passes `--append-system-prompt-file
  <path>` when `StartSpec.Instructions` is non-empty. Resume launches rewrite
  and re-pass it; rejoin attach writes nothing. Pi leads are unchanged (the Pi
  bridge already injects the key).
- Purpose: on machines where policy blocks some of the plugin's Claude hooks,
  the hosted Claude lead still learns its key and how to load the manual.

## Scope (docs only)

- `ai-docs/manuals/ws-mcp.md`:
  - `:861-863`: the "no profile allowlist, plugins, or system prompt of the
    runner's" statement for leads becomes accurate: a Claude lead does get one
    appended daemon-written instruction file (key plus workflow_manual
    instruction); still no profile allowlist.
  - `:252-258`: `instructions.md` description covers the lead case (a Claude
    lead's file always carries the key and the manual instruction) besides the
    child profile case.
  - Identity bullet near `:884`: name the prompt file as a second key route
    beside the `WS_HOSTED_*` env.
  - Verify the exact line numbers yourself; keep edits to one or two
    sentences each, matching the manual's existing style and wrap.
- `ai-docs/manuals/performance.md:59-61`: rewrap the over-long line to the
  neighbouring 79-column wrap, no wording change.

## Constraints

- No code changes. Read `ai-docs/manuals/ws-mcp.md` around the edits and
  confirm facts against the code above before writing.
- No local go test / Go tools / ws executables / launchers / Electron.
- Temp files only under `D:/Temp/ws-mcp-hosted-key-doc/`.

## Done when

One commit on `impl/develop/ws-mcp-hosted-key-doc` with both doc edits and
this plan; its AI Context states it resolves F1 of the re-review and the
follow-up `clustered-grid-thirsty` (performance.md wrap; pass it in the
commit's resolves list if the commit tool supports it, else name it).
