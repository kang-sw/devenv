---
title: Pi compaction thresholds accept absolute token counts
related:
  261002-feat-pi-lead-ws-owned-compaction: introduced pi.compaction_hard_percent and the advisory/hard trigger this ticket generalizes
  261006-feat-pi-compaction-advisory-standing-intent-milestones: milestone rule (thirds between advisory and hard) kept and applied after resolution
  261002-feat-ws-config-adapter-schema-extension: adapter config manifest schema this ticket extends with a pattern constraint
  261016-research-claude-mod-pi-like-compaction: the Claude mod's future advisory threshold is expected to reuse this value grammar
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 26db5169a520f8ac
sage-review-completeness-reviewed: 26db5169a520f8ac
completed: 2026-10-17
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

  Accepted set, exactly: a percent term is an integer 1-100 with or without
  `%` (the old 1-100 range, so `0`, `0%`, and `101` stay rejected); a token
  term is a positive number with a lowercase `k` or `m` suffix, decimals
  allowed, rounded to the nearest whole token; the combined form is exactly
  one percent term and one token term joined by `|`, in either order, with no
  whitespace. Anything else (uppercase suffixes, `50%|60%`, three terms,
  `0k`) is rejected.
  Rejected: interpreting a bare integer above 100 as a token count (makes the
  unit invisible and `150` ambiguous); a JSON union/anyOf value type (the
  manifest schema has no union type, and storage is already a string).
- **Key type.** Both keys change from `integer` to `string` in the pi
  manifest; defaults stay 50% and 80% in the new string form. A stored bare
  number keeps its percent meaning, so no existing configuration changes
  behavior. The key names keep their `_percent` suffix; the descriptions state
  the token and combined forms. The pi resolver accepts both a number and a
  string, so a numeric value from an older reader or a test fixture still
  resolves.
- **Validation at tune time.** The ws-mcp adapter config manifest gains a
  generic `pattern` constraint for `string` keys: `config.tune` rejects a
  value whose whole value does not match, and a manifest whose declared
  default does not match is rejected at load like any other invalid manifest.
  The pi manifest declares a pattern for the grammar above on both keys.
  `pattern` uses Go `regexp` (RE2) syntax, is matched against the whole value
  whether or not it is anchored, is allowed only on `string` keys, and a
  pattern that fails to compile rejects the manifest at load. A patterned
  key's tune errors and constraint description name the pattern instead of
  "any string". The manifest pattern and the pi parser accept exactly the same
  set, so a value `config.tune` accepts never falls back silently in pi.
  Rejected: pi-side parsing only with the existing silent fallback to the
  default, because a typo such as `15ok` would then run at 50% without any
  signal, the worst failure mode for a cost knob.
- **Resolution.** At each trigger check, each threshold resolves to an
  absolute token count against the current window (percent x window, the
  smaller term for the combined form). The milestones keep the existing rule,
  one and two thirds of the way from the advisory to the hard point, computed
  between the resolved advisory and hard token counts. The existing rule that
  an advisory at or above the hard point fires neither the advisory nor the
  milestones applies to the resolved values. The window is
  `pi.context_window_override` when set, otherwise the model's
  `contextWindow`. Every caller that compares usage against a threshold
  resolves against that same window on each check: the compaction trigger and
  its latch baseline, and the goal reminder's advisory comparison. Usage and
  thresholds are compared in one unit per check, so the once-per-crossing latch
  and its re-arm below a threshold keep their current behavior when the
  window changes.
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
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 7fdb2b02 (2026-10-16, commit): "User request 2026-10-16: on 1M-window models the 50% advisory lands at 500k tokens while billing tracks tokens; the user wants token counts or a percent|tokens minimum." — bearing: supports
- 261006-feat-pi-compaction-advisory-standing-intent-milestones (2026-10-06, Decisions): "Two milestones sit at one third and two thirds of the way from the advisory threshold to the hard threshold (advisory + (hard - advisory) / 3 ...; 60% and 70% at the defaults)." — bearing: constrains
- 261006-feat-pi-compaction-advisory-standing-intent-milestones (2026-10-06, Decisions): "One message per observation, by priority hard > advisory > milestone. When one observation crosses several thresholds, only the highest-priority deliverable one is sent" — bearing: constrains
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, f065de0d): "Crossing flags also re-arm when usage is observed below the threshold, which keeps 'once per crossing' literal without depending only on compaction." — bearing: constrains
- 261004-feat-pi-compaction-interactive-ux (2026-10-04, Decisions): "D2 - Preserve non-advisory behavior. Keep existing advisory thresholds, once-per-crossing/rearming behavior, delivery and model wake semantics." — bearing: constrains
- 261002-chore-ws-pi-retire-ws-claude-and-soften-compaction-advisory (2026-10-02, Decisions): "The advisory trigger carries the hard percent. The advisory PreparationTrigger gains the resolved hard percent and sendPreparation's advisory call passes it" — bearing: constrains
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, Decisions): "Knob location. compaction_advisory_percent and the new compaction_hard_percent ... live in goal-loop-config.json for now; 261002-feat-ws-config-adapter-schema-extension migrates them into ws-mcp config." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/config_manifest.go, agents-plugin-pi/config-manifest.json, agents-plugin-pi/src/goal-loop.ts, agents-plugin-pi/src/lead-compaction.ts (message text) |
| scope.surface | cross-module | adapter manifest schema gains a pattern field consumed by ws-mcp and declared by the pi package; pi resolvers and trigger messages change in step |
| scope.new_public_symbol | yes | manifest key constraint pattern for string keys (no such field today: config_manifest.go declares only minimum, maximum, enum) |
| scope.new_type_contract | yes | pi.compaction_advisory_percent and pi.compaction_hard_percent change from integer to string; resolveCompaction*Percent no longer returns a bare percent |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/config_manifest_test.go, agents-plugin-pi/test/goal-loop.test.ts, agents-plugin-pi/test/adapter-config.test.ts, agents-plugin-pi/test/lead-compaction.test.ts |
| complexity.reuse_points | confirmed | declaredConfigKey parse/validation in config_manifest.go and compactionThresholds/highestThresholdAtOrBelow in goal-loop.ts were read |
| complexity.side_effect_risk | moderate | percent-based latch, thresholds and message text at goal-loop.ts call sites (L1074, L1154, L1430, L1489) must move to token counts; stored bare numbers must keep percent meaning |
| risk.correctness | moderate | grammar parsing, min-of-two resolution against a window that varies per check, and latch re-arm semantics must stay consistent across several call sites |
| risk.fit | moderate | follows the existing manifest-constraint pattern and the never-hard-fail resolver shape; the percent-typed trigger structures need rework rather than a drop-in change |
| risk.test | moderate | many resolution combinations (each side of the combined form winning, advisory at or above hard, beyond-window) need new cases beside existing suites |
| risk.security_or_contract | moderate | changes the declared type of two shipped config keys and extends the adapter manifest schema contract; a regex dialect for pattern must be fixed and validated at load |

## Phases

### Phase 1: Token-capable compaction thresholds

Add the manifest `pattern` constraint to ws-mcp, switch both pi keys to
`string` with the grammar above, and resolve thresholds to token counts before
the advisory/milestone/hard comparison, per `## Decisions`.

Verification:

- ws-mcp unit tests: `pattern` accepted only on `string` keys; whole-value
  match; a non-compiling pattern rejects the manifest; `config.tune` rejects a non-matching value with an error naming the
  key; a manifest whose default fails its pattern is rejected.
- pi unit tests: grammar parsing (`50`, `50%`, `150k`, `1.5m`, `50%|150k`,
  `150k|50%`, malformed fallback); boundary values (`0`, `1`, `100`, `101`,
  `0k`, `50%|60%`) accepted or rejected identically by the manifest pattern
  and the pi parser; resolution against windows where each side of the
  combined form wins; milestones between resolved token counts; advisory at or
  above hard after resolution; a token threshold beyond the window never
  fires; message text shows tokens and percent.
- Existing stored bare-number values keep their percent behavior (a test
  that feeds `"50"` and `"80"` through the resolver).
- Full `agents-plugin-pi` and `agents-plugin-tool` suites pass.

### Result (2128d74a0) - 2026-10-17

Landed in 2128d74a0 (implementation) and 129b5ecf4 (baseline test from review).

- ws-mcp: `configManifestKey`/`declaredConfigKey` gain `pattern` (string keys
  only, RE2, compiled as `^(?:pattern)$` at load; a non-compiling pattern, a
  pattern on a non-string key, or a default that fails it rejects the
  manifest). A patterned key's tune errors and `config.list` value description
  read "a string matching the pattern ...".
- Pi manifest: both compaction keys are `string` with the grammar pattern,
  defaults `"50%"` / `"80%"`, descriptions state the percent, token, and
  combined forms.
- Pi: `parseCompactionThreshold` (string grammar, plus a finite number in
  (0, 100] as a percent); `resolveCompactionAdvisoryThreshold` /
  `resolveCompactionHardThreshold` return a `{percent?, tokens?}` spec with
  the never-hard-fail default fallback; `resolveContextWindow` (override,
  else the model window) and `resolveThresholdPercent` resolve each threshold
  at every check. The trigger check, the latch baseline, and the goal
  reminder (`goalReminderUsage`) all resolve against the same window. The
  advisory, hard, and milestone texts and the reminder show thresholds as
  `N% / Xk tokens` when the window is known.

Verification: `cd agents-plugin-pi && npm test` - 2335 tests, 0 fail (before
the review test; `node --test test/goal-loop.test.ts` after it - 219 pass, 0
fail). `cd agents-plugin-tool && go test ./...` - all packages ok. Manifest
pattern / parser parity is pinned by one corpus in
`config_manifest_test.go` (Go RE2 loader) and `adapter-config.test.ts` (JS
RegExp of the same pattern vs `parseCompactionThreshold`).

Decisions:
- Comparison is in percent space: each threshold resolves to
  min(percent, tokens / window * 100) against the same window as the usage
  percent. This is the window-linear equivalent of comparing token counts
  (milestones between resolved token counts land at the same points), keeps
  percent-only configs exact, and leaves the existing latch untouched.
- A token term with no known window resolves to +Infinity (never fires).
- The resolvers are renamed `resolveCompaction*Threshold` because they no
  longer return a bare percent.

Open (lite review Minors, not fixed):
- The latch is stored in percent, so a mid-session window shrink can re-fire
  a token advisory for the same token crossing (one extra nudge, no hard or
  data impact); a token-stored latch would instead move percent thresholds.
- `0.0001k` passes both the pattern and the parser and rounds to 0 tokens
  (fires on any reading); excluding it from a regex is awkward.
- With an unknown window a token-only threshold renders as `Infinity%`
  (practically unreachable: Pi always reports `contextWindow`).
