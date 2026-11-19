# Plan: whiteboard panel HTML blocks stay HTML until their matching close tag

## Request (owner, 2026-11-19, board item (1393)=a)

A mockup posted as a whiteboard panel rendered partly as an escaped code
block: a CommonMark type 6/7 HTML block (opened by e.g. `<div ...>`) ends at
the first blank line, and the following lines indented 4+ spaces became an
indented code block. The owner chose: inside an HTML block, keep HTML —
an HTML block opened by a tag stays HTML until the matching closing tag of
that element (depth-balanced), regardless of blank lines and indentation.
This extends the Markdown grammar to reduce surprise.

## Evidence

- Panel Markdown is rendered server-side with goldmark:
  `agents-plugin-tool/internal/wswhiteboard/markdown.go` (~100-161).
- The block parser is wrapped by `rawTextHTMLBlock` in
  `agents-plugin-tool/internal/wswhiteboard/htmlblock.go` (~13-112). It
  already keeps a type 6/7 block open across a blank line while a
  `script`/`style`/`pre`/`textarea` is still open (~96-100); otherwise a type
  6/7 block closes at a blank line (~75-78).
- Repro: `ai-docs/assets/mockups/261119/tracker-org-entity/editor.html`
  (blank line at 163 followed by an 8-space-indented `<!-- org section ... -->`
  inside the open `<div class="tkoo">`; again at 189).

## Outcome

- Generalize the existing repair: a type 6/7 HTML block whose opening line
  leaves non-void elements open stays open across blank lines until the
  element depth returns to zero (all opened non-void elements closed). Then
  the block ends as usual (at the next blank line after balance, per the
  existing logic). Void elements (`br`, `img`, `input`, `hr`, `meta`, `link`,
  `area`, `base`, `col`, `embed`, `source`, `track`, `wbr`) and self-closing
  `<x/>` do not count. Comments (`<!-- ... -->`, possibly multi-line) and the
  contents of `script`/`style`/`pre`/`textarea` must not be scanned for tags
  (keep the current raw-text handling).
- An element never closed keeps the block open to the end of the payload
  (same as a browser would treat it); that is accepted.
- Accepted consequence (owner-approved): GitHub-style
  `<details><summary>..</summary>` + blank line + Markdown body now renders
  the body as HTML/text, not Markdown.
- Markdown outside HTML blocks is unaffected; a payload with no open element
  after a block (e.g. `<p>x</p>` on one line) behaves as today.
- Add one line to the whiteboard playbook's panel payload rules saying that
  an HTML block opened by a tag stays HTML until its matching closing tag
  (blank lines and indentation inside it do not switch back to Markdown).
  Find the shipped whiteboard playbook source (search for "Payload: block
  Markdown with inline HTML" under agents-plugin/rsrc/ and any
  agents-plugin-wsflow mirror / embedded copies) and follow
  `ai-docs/manuals/skill-authoring.md` and `ai-docs/manuals/wsflow-mirroring.md`
  including any regeneration chain those manuals or repo tests require. Do
  NOT hand-edit generated native-pi/dist/workflow-bundle files; regenerate
  them with the documented script if the change requires it, or report if
  you cannot.
- Compatibility: T0 (render-only; stored payloads unchanged).

## Verification

- Go unit tests in `wswhiteboard` for: the editor.html repro shape (div with
  blank line + indented children renders no `<pre><code>`), nested divs with
  blank lines, a comment containing `<div>` text not counted, script/style
  containing `<div` strings not counted, void and self-closing elements,
  unclosed element runs to end, Markdown after a balanced block still renders
  as Markdown (after a blank line), existing tests unchanged.
- Run on w5090 only:
  `bash D:/sys_lock.sh remote w5090 run <who> 30 --sync '<this worktree path>' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wswhiteboard/... ./internal/wsdoc/... ./internal/rsrc/...'`
  (adjust the package list to whatever covers the playbook edit; redirect
  stdout to a file under D:/Temp/wb-html-block-balanced/ and read it, since
  piped output can be swallowed).

## Constraints

- NO local `go test`, Go tools, gofmt, ws/ws-mcp exes, launcher or Electron on
  this PC. Go only on w5090 (no Node there).
- Do NOT start, stop, rebuild or swap the shared ws daemon.
- Temp files only under D:/Temp/wb-html-block-balanced/.
- Never read or print credentials.
- Commit on branch impl/develop/wb-html-block-balanced in this worktree only,
  with no Co-Authored-By trailer (Pi worker).

## Return boundary

Stop and report before changing anything beyond the HTML block parser, its
tests, and the one playbook line (plus its mirrors/regenerated copies).
