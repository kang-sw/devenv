# Whiteboard file:// probe checklist

Phase 2 of `261030-feat-session-whiteboard-mode`. The kit checks that a local
`file://` page can watch a stamp script for changes and reload itself, the
change signal the whiteboard full page depends on.

Kit files: `probe.html` (the page), `probe.stamp.js` (its stamp),
`rewrite-stamp.sh` (macOS, Linux, WSL), `rewrite-stamp.ps1` (native Windows).
Each rewriter run is one board update: it rewrites the page version, then the
stamp, through a temp file and a rename.

## Platforms

Run every check on each row. Use a fresh tab per row: `sessionStorage` is per tab.

| row | where the kit lives | open in | rewriter |
|-----|---------------------|---------|----------|
| W-Chrome | a local folder, e.g. `C:\wbprobe\` | Windows Chrome | `powershell -ExecutionPolicy Bypass -File C:\wbprobe\rewrite-stamp.ps1` |
| W-Edge | same folder | Windows Edge | same |
| WSL-Chrome | inside the distro, e.g. `~/wbprobe/` | Windows Chrome, at `\\wsl.localhost\<distro>\home\<user>\wbprobe\probe.html` | `sh ~/wbprobe/rewrite-stamp.sh` inside WSL |
| WSL-Edge | same | Windows Edge, same path | same |
| macOS | a local folder | Chrome (Safari optional, as an extra row) | `sh <folder>/rewrite-stamp.sh` |

Copy the whole folder; the rewriters edit the files next to them.

## Checks

Open `probe.html`, wait 3 s, then:

1. **Stamp.** Row 1 reads `polling; run the rewriter`, `errors=0`. Run the
   rewriter once. Within about 2 s the page reloads and row 1 reads `ok` with
   `last-latency` under 2500 ms. `SLOW`, or no reload within 5 s, is a fail.
   The latency compares the rewriter's clock with the browser's; on a WSL row
   a negative or very large value is clock skew, so judge by the reload timing.
2. **Reload and scroll.** Scroll down until `row 40` is in view, run the
   rewriter. After the reload `row 40` is still in view and row 2 reads `ok`.
   (The probe turns off the browser's own scroll restoration, so this tests
   the `sessionStorage` path the whiteboard uses.)
3. **sessionStorage.** Type a word in the note box and open the panel, run the
   rewriter. After the reload the word and the open panel are still there and
   row 3 reads `ok: survived N reload(s)`.
4. **Page fresh.** After any rewriter-triggered reload, row 4 reads `ok`:
   the reloaded page is the rewritten file, not a cached copy.
5. **Copy.** Click `copy test string`: row 5 reads `execCommand returned
   true`. Paste into the paste box: it reads `match`.
6. **Rewriter.** Run the rewriter five times about 1 s apart while the page
   polls. It never fails (on Windows, watch for a sharing or access-denied
   error). Note row 1's `errors` count; `0` is expected.

## Report

For each row, click `copy results` and send the line, plus any rewriter error
and anything the checks did not show, to the lead. The lead records them in
Phase 2's `### Result`.
