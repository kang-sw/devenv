---
title: Pi lead read hatch drops image input
related:
  260904-feat-ws-pi-execute-approval-gateway: origin of the ugly-named read hatch and the lead `--tools` reshaping
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 519dff5b815c410e
sage-review-completeness-reviewed: 519dff5b815c410e
---

# Pi lead read hatch drops image input

## Background

`computeLeadActiveTools` (`agents-plugin-pi/src/execute-gateway.ts`) removes
native `read` from the lead's active tools and substitutes
`do-i-really-have-to-read-this-myself` (`UGLY_READ_TOOL_NAME`); it is applied
behind the `isLeadOrFork` gate in `computeSessionBootstrap`
(`src/lead-bootstrap.ts#L230-L245`, called from `src/index.ts#L921`), and a
fork inherits the lead's already-reshaped active tools verbatim
(`src/fork.ts#L67-L69`, `src/lead-bootstrap.ts#L237-L238`), so the fork has the
same substitution. That substitute reads every file with
`readFileSync(absolutePath, "utf8")` and returns a single text block through
`sliceLines`. An image file therefore reaches the model as mojibake text:
the lead cannot take image input from disk at all, even on a vision-capable
model, and the garbage bytes pollute its context.

Spawned workers are unaffected: the `full-worker` tool list in
`src/spawner.ts` keeps native `read`.

The host's native read (`createReadToolDefinition` in
`@earendil-works/pi-coding-agent`, 0.84.4 at time of writing) already
handles this: it detects supported image MIME types (jpg, png, gif, webp,
bmp) and returns `{ type: "image", data, mimeType }` content, auto-resizes to
2000x2000 max, appends a note when the current model (`ctx.model`) lacks image
input, and truncates text at `DEFAULT_MAX_LINES`/`DEFAULT_MAX_BYTES` (2000
lines / 50KB) with an offset-continuation hint.

## Decisions

- **Delegate the hatch's `execute` wholesale to the host native read.** The
  hatch calls `createReadToolDefinition(<cwd>).execute(...)` and returns its
  result unchanged, following the existing precedent in `src/write-scopes.ts`,
  which wraps `createEditToolDefinition`/`createWriteToolDefinition` the same
  way. Image detection, resizing, the non-vision note, and text truncation
  all come from the host and track host upgrades.
  - Consequences accepted with the wholesale choice: an `offset` past end of
    file now throws the host's `Offset N is beyond end of file` error instead
    of returning empty text, and `path` goes through the host's resolver,
    which also expands `~`, strips a leading `@`, and tolerates Unicode
    normalization variants.
  - This reverses 4455ec78, which built the hatch from scratch because Pi's
    built-in read internals were private to the installed package. That
    premise no longer holds: `createReadToolDefinition` is a public export
    of the host package.
  - Rejected: grafting only an image branch onto the current text read.
    It would keep the uncapped text read but re-implement MIME detection,
    resizing, and the non-vision note locally, drifting from the host.
- **Accept the host's text truncation as the new text-read behavior.** A text
  read is now capped at 2000 lines / 50KB with the host's continuation hint,
  replacing the current uncapped full-file return. The user confirmed this;
  it also fits the hatch's discouraged-fallback posture.
- **Keep the tool's name and discouraged-by-name posture.** The name
  `do-i-really-have-to-read-this-myself`, its `label`, the `path`/`offset`/
  `limit` parameter shape, and the "fallback, prefer delegating" wording stay.
  The description additionally states that supported image files are returned
  as image attachments.
- **Delete `sliceLines` and its unit tests.** Once the hatch delegates to the
  host read, `sliceLines` in `execute-gateway.ts` has no caller; host read owns
  `offset`/`limit`.
  - Rejected: keeping it as an unused tested export.
- **Update the hatch's documentation in the same phase.** The hatch rows in
  `agents-plugin-pi/pi-lead-guide.md` and `ai-docs/spec/pi-adapter-runtime.md`
  state that supported image files are returned as image attachments and that
  text reads are capped at 2000 lines / 50KB with an offset continuation.
  - Rejected: leaving both documents describing a plain uncapped text read.

## Constraints

- The path base stays as today: a relative `path` resolves against the lead
  session's cwd (`sessionCtx.cwd`), passed as the host factory's `cwd`.
- Copy only the `execute` call from the `src/write-scopes.ts` precedent, not
  its `{ ...definition }` spread. The hatch stays a `registerWsTool`
  definition with its own name, label, description, and parameters, whose
  `execute` calls `createReadToolDefinition(sessionCtx.cwd).execute(toolCallId,
  params, signal, onUpdate, ctx)`. Spreading the host definition would import
  host `name: "read"` and host renderers, and `registerWsTool` skips its
  logical-line preview wrapping when a definition already carries renderers.
- The tool-call `ctx` is passed through to the native `execute`, so the host's
  non-vision note sees the current model.
- Result rendering needs no change: `registerWsTool`'s `renderResult`
  (`src/tool-result-render.ts`) already defers to Pi's native text/image
  fallback when the result is not a single text block.
- Import the host factory statically from `@earendil-works/pi-coding-agent`,
  as `src/write-scopes.ts` does. The
  `gotcha.pi-tui-dual-package` rule (resolve `pi-tui` through the host) is
  specific to `pi-tui` and does not apply to `pi-coding-agent`.

## Prior Decisions

- 4455ec78 (2026-09-05, commit): "The ugly-named read tool ... is a minimal from-scratch reimplementation (offset/limit line-slicing, no image handling, no truncation-by-bytes) rather than a wrapped copy of Pi's built-in `read`" — bearing: contradiction-candidate
- 260904-feat-ws-pi-execute-approval-gateway (2026-09-04, Decisions): "Keep read as a soft-discouraged escape hatch — an \"ugly-named\" direct read tool ... so the lead reaches for delegation by default but retains a legitimate must-look path." — bearing: supports
- 3c404963 (2026-09-05, commit): "Owner question during dogfooding (2026-09-05): the hatch was discoverable only through the tool schema list; the guide contradicted it. Doc-only, semantics unchanged; no test snapshots the guide text." — bearing: constrains
- 260905-feat-ws-pi-lead-one-liner-exec-escape-hatch (2026-09-05, Decisions): "Surface. Added through the same lead tool-surface reshaping step as the read hatch (`LEAD_ADDED_TOOL_NAMES` in `execute-gateway.ts`), so it is present for the lead and a lateral fork" — bearing: supports
- 260906-feat-ws-pi-tool-and-push-tui-polish (2026-09-06, Decisions and boundaries): "Direct-tool result previews cover `do-i-really-have-to-read-this-myself` and `do-i-really-have-to-run-this-myself`. Preserve their titles, arguments, read offset/limit semantics" — bearing: constrains
- 260906-feat-ws-pi-tool-and-push-tui-polish (2026-09-09, Result 465d8c31): "additive `lineBudget: \"physical\" | \"logical\"` seam in `tool-result-render.ts` ... The two direct tools ... set logical" — bearing: constrains
- bbf9a29a (2026-09-06, commit): "Do not transform execute inputs, results, details, mixed content, errors, or image fallback data; render-only unsupported cases intentionally defer to Pi's existing fallback." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/execute-gateway.ts hatch body and sliceLines, test/execute-gateway.test.ts sliceLines tests and header comments, a new or extended hatch test with an image fixture, agents-plugin-pi/pi-lead-guide.md, ai-docs/spec/pi-adapter-runtime.md |
| scope.surface | internal | no exported TS symbol changes; the model-visible hatch description and result content shape change |
| scope.new_public_symbol | no | none added; the sliceLines export (src/execute-gateway.ts) is removed with its tests per Decisions |
| scope.new_type_contract | no | parameters path/offset/limit unchanged; result becomes host read's AgentToolResult with text or image content |
| scope.test_surface | existing | agents-plugin-pi/test/execute-gateway.test.ts, agents-plugin-pi/test/native-tool-registration.test.ts#L86-L117 |
| complexity.reuse_points | confirmed | createReadToolDefinition exported from @earendil-works/pi-coding-agent 0.84.4 dist/index.js#L30, execute at dist/core/tools/read.js#L135-L146; wrap precedent src/write-scopes.ts#L275-L290 |
| complexity.side_effect_risk | low | read-only lead/fork tool; no writes or process state touched |
| risk.correctness | low | wholesale delegation to host read; only cwd base sessionCtx.cwd and ctx passthrough via registerWsTool spread in src/tool-result-render.ts#L681-L685 must be wired |
| risk.fit | low | reverses the 4455ec78 from-scratch-reimplementation rationale, cited in Decisions; host read is now public API so the premise no longer holds |
| risk.test | moderate | image unit test runs the host processImage resize path under node --test; the hatch body was previously deferred to live gates per 45fc7948 |
| risk.security_or_contract | low | model-visible text reads become capped at 2000 lines or 50KB, a decided change; path resolution stays relative to sessionCtx.cwd |

## Phases

### Phase 1: Route the lead read hatch through the host native read

Replace the hatch's `execute` body in `registerExecuteGateway` with a call to
the host native read's `execute`, update the tool description to mention
image support, delete `sliceLines` and its tests, update the hatch's
descriptions in `agents-plugin-pi/pi-lead-guide.md` and the repo-root
`ai-docs/spec/pi-adapter-runtime.md`,
and cover the change with tests.

Verification:

- A unit test that registers the hatch and executes it against a small image
  fixture asserts the result contains an `image` content block with the
  expected `mimeType`, and that a text file read still returns text honoring
  `offset`/`limit`, and that a text file over 2000 lines comes back
  truncated with the host's offset-continuation hint.
- `sliceLines` has no remaining reference in `agents-plugin-pi/src` or
  `agents-plugin-pi/test` (including test header comments).
- The hatch entries in `agents-plugin-pi/pi-lead-guide.md` and
  `ai-docs/spec/pi-adapter-runtime.md` mention image attachments and the
  2000-line / 50KB text cap.
- `npm test` in `agents-plugin-pi/` passes, including the existing
  `native-tool-registration` preview-budget tests for the hatch.
