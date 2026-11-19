# Plan: `wb:panel src="<path>"` — snapshot a mockup file into a panel

## Request (owner, 2026-11-19 17:17, board item (1394)=a)

Leads and subagents keep ping-ponging long HTML mockups: the playbook tells
the lead to save text mockups under `ai-docs/assets/mockups/YYMMDD/<topic>/`
(`agents-plugin/rsrc/whiteboard/whiteboard.md:27-29`), but the only way to
show one on the board is to paste the whole HTML inline into a `wb:panel`
payload, so the HTML travels subagent -> lead -> whiteboard.update. The owner
chose a snapshot include: the lead names the file, the server reads it.

## Evidence (develop 5981d2e41, from a read-only lookup)

- Panel bodies are rendered unfiltered in a sandboxed srcdoc frame
  (`wswhiteboard/markdown.go:206-213`, `render.go:704-772`); there is no
  path-based HTML embed. Images resolve relative paths against the session
  project root (`wsdashboard/image_route.go:85-97`, symlink escape refused
  `:262-290`).
- Whiteboard commands are parsed from the `commands` XML string of
  `whiteboard.update` (find the `wb:panel` attribute parser in
  `agents-plugin-tool/internal/wswhiteboard/` or `internal/mcp/`).

## Outcome

- New optional attribute on `wb:panel`: `src="<path>"`. When present, the
  server reads that file once at update time and uses its content as the
  panel payload, exactly as if it had been sent inline (same Markdown/HTML
  rendering, same versioning; the stored panel holds the content, not the
  path). Later edits to the file need a new `wb:panel src=...` post.
- Path rules: relative to the calling session's project/worktree root (the
  same root image paths use); refuse absolute paths outside that root, `..`
  escapes and symlink escapes; refuse missing files and non-text files.
  Allowed extensions: `.html`, `.htm`, `.svg`, `.md` (keep it to these).
  Apply a size cap (pick a sane bound, e.g. 512 KiB, or reuse an existing
  payload cap if one exists) and refuse beyond it.
- `src` together with non-empty element content, or with `wb:change` bundles,
  is refused (one source per post). `recap` rules are unchanged.
- A refused `src` fails that command through the existing batch-refusal path
  (with position/line), naming the path and reason.
- Whole-document files: if the file starts with `<!doctype` or `<html`,
  either extract the `<body>` content (plus `<style>` from `<head>`) or
  refuse with a clear message — choose the simplest one that keeps the
  existing "no full HTML document" payload rule true, and record the choice
  in the commit AI Context.
- Shipped playbook: in the whiteboard playbook (the `agents-plugin/rsrc`
  source plus every mirror/regenerated copy, following
  `ai-docs/manuals/wsflow-mirroring.md` and `ai-docs/manuals/skill-authoring.md`):
  - add `src` to the `wb:panel` row of the commands table;
  - extend the "Text mockups" rule: write a complex mockup as a file in the
    mockups folder and post it with `wb:panel src="<path>"`; never pass
    mockup HTML bodies between agents or paste them inline.
  Regenerate manifests / zips / native-pi bundles with the documented
  scripts; never hand-edit generated files.
- Performance (`ai-docs/manuals/performance.md`): one bounded file read per
  `src` command on the update call only; no rendering-path I/O, no watching.
- Compatibility: state the tier with source evidence. Expected: an older
  daemon refuses the unknown `src` attribute (or ignores it and posts an
  empty panel — check which) and changes no stored data; record T0/T1 in the
  commit AI Context. Stored payload shape is unchanged.

## Verification

- Go unit tests: src snapshot stores the file content; relative path under
  root works; `..`/absolute-outside/symlink escape refused; missing file,
  disallowed extension, over-cap refused; src + inline content refused;
  full-document handling per the chosen rule.
- Run on w5090 only, stdout redirected to a file under
  `D:/Temp/wb-panel-src/` and read back:
  `bash D:/sys_lock.sh remote w5090 run wb-panel-src 30 --sync '<this worktree path>' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go build ./... && go vet ./internal/wswhiteboard/... ./internal/mcp/... && go test ./internal/wswhiteboard/... ./internal/mcp/... <rsrc/manifest test packages touched>'`
  (Node-dependent badge tests fail on w5090 for lack of Node; that is
  pre-existing — note and ignore).

## Constraints

- NO local `go test`, Go tools, gofmt, ws/ws-mcp exes, launcher or Electron on
  this PC (antivirus). Go only on w5090.
- Do NOT start, stop, rebuild or swap the shared ws daemon.
- Temp files only under `D:/Temp/wb-panel-src/`; delete when done.
- Never read or print credentials (`?login=` tokens, cookies, endpoint.json,
  LAN tokens).
- Commit on `impl/develop/wb-panel-src` in this worktree only, ending the
  message with `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`.

## Return boundary

Stop and report before changing anything beyond the `wb:panel` command
parsing/handling, its tests, and the playbook lines (plus mirrors and
regenerated copies) — e.g. renderer, sanitizer, image route, dashboard JS.
