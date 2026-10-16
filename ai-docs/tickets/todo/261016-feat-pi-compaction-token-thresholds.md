---
title: Pi compaction thresholds accept absolute token counts
related:
  261002-feat-pi-lead-ws-owned-compaction: introduced pi.compaction_hard_percent and the advisory/hard trigger this ticket generalizes
  261006-feat-pi-compaction-advisory-standing-intent-milestones: milestone rule (thirds between advisory and hard) kept and applied after resolution
  261002-feat-ws-config-adapter-schema-extension: adapter config manifest schema this ticket extends with a pattern constraint
  261016-research-claude-mod-pi-like-compaction: the Claude mod's future advisory threshold is expected to reuse this value grammar
---

# Pi compaction thresholds accept absolute token counts

## Background

`pi.compaction_advisory_percent` (default 50) and `pi.compaction_hard_percent`
(default 80) are declared `type: "integer"`, range 1-100, in
`agents-plugin-pi/config-manifest.json`, and resolved by
`resolveCompactionAdvisoryPercent` / `resolveCompactionHardPercent` in
`agents-plugin-pi/src/goal-loop.ts`. A percentage scales with the model's
window: on a 1M-window model the advisory point is 500k tokens. Billing scales
with tokens, not with percent, so the user wants to state a threshold as a
token count, or as "whichever comes first" of a percent and a token count.

The existing workaround, `pi.context_window_override`, shrinks the window the
percent is computed against, which also changes the meaning of every percent
the lead is shown.

Storage and transport already carry strings: ws-mcp stores every config value
as a string in every scope, and the declared type only governs `config.tune`
validation and `config.get`'s typed read
(`agents-plugin-tool/internal/mcp/config_manifest.go`, the declared-type
comment). `config.tune`'s `value` is published as anyOf string/object, and
scalar values already arrive as strings (`d67dce5d0`, the vLLM tool-call
parser case). A `string` key type therefore needs no union and loses no stored
value.

## Decisions

- **Value grammar.** Both keys take one of:

  ```text
  50          bare number  -> percent (backward compatible with stored "50")
  50%         percent
  150k        tokens (k = 1,000)
  1.5m        tokens (m = 1,000,000)
  50%|150k    whichever is reached first, i.e. the smaller resolved token count
  ```

  Rejected: interpreting a bare integer above 100 as a token count (makes the
  unit invisible and `150` ambiguous); a JSON union/anyOf value type (the
  manifest schema has no union type, and storage is already a string).
- **Key type.** Both keys change from `integer` to `string` in the pi
  manifest; defaults stay 50% and 80% in the new string form. A stored bare
  number keeps its percent meaning, so no existing configuration changes
  behavior.
- **Validation at tune time.** The ws-mcp adapter config manifest gains a
  generic `pattern` constraint for `string` keys: `config.tune` rejects a
  value whose whole value does not match, and a manifest whose declared
  default does not match is rejected at load like any other invalid manifest.
  The pi manifest declares a pattern for the grammar above on both keys.
  Rejected: pi-side parsing only with the existing silent fallback to the
  default, because a typo such as `15ok` would then run at 50% without any
  signal, the worst failure mode for a cost knob.
- **Resolution.** At each trigger check, each threshold resolves to an
  absolute token count against the current window (percent x window, the
  smaller term for the combined form). The milestones keep the existing rule,
  one and two thirds of the way from the advisory to the hard point, computed
  between the resolved advisory and hard token counts. The existing rule that
  an advisory at or above the hard point fires neither the advisory nor the
  milestones applies to the resolved values.
- **No clamp.** A token threshold beyond the window is not clamped; it simply
  never fires and Pi's native automatic compaction takes over. The combined
  form is how a user bounds a token threshold by a percent. Rejected: a hidden
  clamp rule.
- **Messages.** Lead-facing nudge and reminder text reports the threshold in
  both tokens and percent, so a token-configured user can match the message to
  the setting.
- The pi resolver keeps its never-hard-fail shape for a malformed value that
  reaches it anyway (a value written outside `config.tune`), falling back to
  the default.

## Constraints

- `pi.context_window_override` is unchanged and out of scope.
- The Claude mod compaction thresholds
  (`261016-research-claude-mod-pi-like-compaction`) are out of scope; this
  ticket only defines the grammar they are expected to reuse.
- `agents-plugin-tool/` edits follow `ai-docs/manuals/shipped-surface-boundary.md`;
  `agents-plugin-tool/internal/mcp/` edits follow `ai-docs/manuals/ws-mcp.md`.
  Key descriptions in the pi manifest are agent-facing shipped text.

## Phases

### Phase 1: Token-capable compaction thresholds

Add the manifest `pattern` constraint to ws-mcp, switch both pi keys to
`string` with the grammar above, and resolve thresholds to token counts before
the advisory/milestone/hard comparison, per `## Decisions`.

Verification:

- ws-mcp unit tests: `pattern` accepted only on `string` keys; whole-value
  match; `config.tune` rejects a non-matching value with an error naming the
  key; a manifest whose default fails its pattern is rejected.
- pi unit tests: grammar parsing (`50`, `50%`, `150k`, `1.5m`, `50%|150k`,
  malformed fallback); resolution against windows where each side of the
  combined form wins; milestones between resolved token counts; advisory at or
  above hard after resolution; a token threshold beyond the window never
  fires; message text shows tokens and percent.
- Existing stored bare-number values keep their percent behavior (a test
  that feeds `"50"` and `"80"` through the resolver).
- Full `agents-plugin-pi` and `agents-plugin-tool` suites pass.
