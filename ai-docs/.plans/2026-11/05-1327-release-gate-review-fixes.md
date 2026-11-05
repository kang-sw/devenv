# release-gate review fixes

Repair of the three Important findings from the release-gate range review
daae12ec7a7e68811eb343bae0bd1acd49f26da9..3ca89792843e37d6e999c1ecfe29060987a9a434
(findings file: ~/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/6d9001d7-01-findings.md).
The user pre-approved review, autonomous fixes, and publish for this ship.
Contract for the whiteboard behavior: ai-docs/tickets/.done/261030-feat-session-whiteboard-mode.md.

## Assignment

1. Whiteboard manual variants reach workers. `workflow_manual.go` applies
   `applyWhiteboardVariants` (the `### Whiteboard` section and the
   discussion-counter replacement) for every key. Apply them only for the
   FRESH bootstrap or a top-level lead key (`sessionEntry.isTopLevelLead()`,
   the gate the occupied-root banner and notes cleanup warning already use);
   a spawned key (lead scope with a parent, or a leaf) gets the knob-off
   manual. Add a test: knob on, a spawned key's `workflow_manual` omits the
   whiteboard section and keeps "Discussion keeps no counter", while the
   parent lead's manual has it.
2. Whiteboard tool descriptions hardcode `ws/playbook.read`
   (`server.go` near 4808 and 4820). Build them with `RuntimeNamespace()`, as
   `whiteboardRefusal` does, so wsflow's `tools/list` names its own namespace.
   The pre-existing `ws/git.commit` in the `tickets.sage_stamp` description
   (near 5011) may be fixed the same way if trivial. Add or extend a test
   that the descriptions carry the runtime namespace.
3. Full-page live reload race. `internal/wswhiteboard/assets/live.js` adopts
   the first polled stamp as the page's version, so a render landing between
   page load and the first poll leaves the tab stale. Give the script the
   page's own seq from the template (the page already prints `#{{.Seq}}`) and
   reload whenever the polled stamp differs from it. Add a render test that
   the full page carries its seq for the script.
4. On contact: fix the stale doc comment at
   `internal/mcp/prompt_override_test.go:1078` (names deleted
   `promptOverrideHarnessBuckets (server.go)`; now `harnessBuckets` in
   `config_registry.go`).

## Verification

Targeted: `go test ./internal/mcp -run '<new and touched tests>|TestWhiteboard' -count=1`
and `go test ./internal/wswhiteboard -count=1`. Then the whole module:
`go test ./... -timeout 40m` (internal/mcp is slow, over 10 minutes). If
`go test` hangs after PASS, run `go test -c` for internal/mcp and execute the
binary with `-test.timeout 40m`, and say so in the commit's AI Context.
No shipped rsrc text should change; if any does, follow
`ai-docs/manuals/wsflow-mirroring.md` and regenerate manifests.

## Return boundary

Stop and report before changing the whiteboard store, the store contract,
any tool's input schema, or shipped playbook text.
