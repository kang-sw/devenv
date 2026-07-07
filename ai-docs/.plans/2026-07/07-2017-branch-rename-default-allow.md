# Plan: 260703-chore-implement-branch-rename-default-allow — Phase 1: Flip the branch-rename default and update the playbook rule

## Relevant Ticket Contract
- Default `policy.branch.allow_rename` to `yes` when the lead gives no
  explicit signal, instead of requiring explicit per-invocation opt-in.
- The `TargetExists` / `Upstream`/`Ahead`/`Behind` guardrails in
  `deriveImplementBranchPlan` are the safety net and must NOT change.
- Ticket's own guidance: prefer the resolver-side default (over playbook-prose
  wording) "if it doesn't collapse the `unknown` state's meaning for other
  future consumers of that fact."
- Phase 1 scope explicitly includes: resolver default flip, playbook Policy
  rule update, resolver/session-state tests (default-allow reaches `rename`;
  explicit `no` still stops), and the `enter.implement` spec entry update —
  phase is not complete without the spec update.
- Out of scope (ticket-level): `TargetExists`/`Upstream`/`Ahead`/`Behind`
  guardrail logic; the fresh-branch `action: "create"` path.

## Out of Scope
- Any guardrail logic changes in `deriveImplementBranchPlan` beyond the
  `AllowRename` default (lines checking `TargetExists`, `Upstream`/`Ahead`/`Behind`).
- The `create` branch-plan path (current branch not `impl/*`/`implement/*`).
- `lead-write-code`, `lead-write-skeleton`, `lead-salvage`,
  `lead-skill-authoring` (wsflow-excluded skills) — not touched by this ticket.

## Codebase Findings

- `agents-plugin-tool/internal/mcp/implement_resolver.go#L510` — the exact
  default site: `AllowRename: factOr(policy.Branch.AllowRename, "unknown")`
  inside `normalizeImplementFacts`. `AllowRename` (normalized) is consumed at
  exactly one place, `implement_resolver.go#L638` (`if n.AllowRename != "yes"`),
  so flipping the fallback to `"yes"` here does not collapse meaning for any
  other consumer — this is the resolver-side default the ticket prefers.
- `agents-plugin-tool/internal/mcp/proceed_resolver.go#L575-L579` —
  `factOr(f factString, fallback string) string`: returns `fallback` only when
  the fact is absent/null/empty; an explicit value (`"yes"` or `"no"`) always
  wins. Confirms changing the fallback cannot override an explicit `"no"`.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L614-L652` —
  `deriveImplementBranchPlan`: the guardrail order is unchanged by this ticket
  — `AllowRename != "yes"` check (line 638) precedes `TargetExists` (643) and
  `Upstream`/`Ahead`/`Behind` (648). No reordering needed.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L373` —
  `parseEnumFact(gm, "allow_rename", []string{"yes", "no", "unknown"})`: input
  parsing/validation is unaffected; only the post-normalization fallback
  changes.
- `agents-plugin-tool/internal/mcp/server.go#L2857` —
  `"allow_rename": nullableEnumStringProperty("Whether MCP may choose a safe
  branch rename verdict.", []string{"yes", "no", "unknown"})` — tool schema
  description; consider whether it should mention the new default (optional,
  low-risk wording touch-up, not required for correctness).
- `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L240-L339` —
  `TestResolveImplementBranchPlanRules` calls `deriveImplementBranchPlan`
  directly with explicit `AllowRename` values in every case (`"no"`/`"yes"`);
  none rely on `normalizeImplementFacts`'s fallback, so this table is
  unaffected by the default flip.
- `agents-plugin-tool/internal/mcp/implement_resolver_test.go#L151-L174` —
  `TestResolveImplementBranchStopOmitsPlannerInstructions` already covers
  "explicit `allow_rename: no` still stops" end-to-end through
  `resolveImplement`. No new explicit-`no` test is strictly required, but add
  one only if the existing one doesn't provide a clean "still stops" assertion
  at the `BranchPlan.Reason` level — it does (`result.Verdict.BranchPlan.Action
  != "stop"`).
- Missing coverage: no existing end-to-end test calls `resolveImplement` /
  `normalizeImplementFacts` with `Policy.Branch.AllowRename` **absent** (zero
  value `factString{}`) while on a mismatched `impl/*` branch to assert the
  verdict is `rename` by default. This is the new test Phase 1 must add.
- `agents-plugin-tool/internal/mcp/session_state_test.go#L1340-L1410,#L1804-L1810` —
  existing session-state tests set `allow_rename` explicitly (`"no"` or
  `"yes"`); no change needed there for this phase unless a default-path
  end-to-end session-state test is wanted (not required by ticket phrasing,
  which names `implement_resolver_test.go` primarily and `session_state_test.go`
  as an alternate location).
- `agents-plugin-wsflow/rsrc/lead-implement/lead-implement.md#L46-L52` and
  `agents-plugin/rsrc/lead-implement/lead-implement.md#L46-L52` — byte-identical
  today (`diff` confirmed empty). Canonical is `agents-plugin/rsrc/...`; the
  wsflow copy is a **generated mirror**, not hand-edited — see
  `ai-docs/ref/wsflow-mirroring.md` "Rsrc Tree Provisioning". The Policy rules
  bullet to change: `- Set policy.branch.allow_rename=yes only when the
  caller accepts pre-edit branch rename.` (line 49 in both files).
- `ai-docs/ref/wsflow-mirroring.md#L181-L189` — mandatory after-edit checklist
  for any canonical `agents-plugin/rsrc/` change:
  1. `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
  2. `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
  Both `-count=1` flags are mandatory (test-cache silently skips the write
  side effect otherwise). Do not hand-edit
  `agents-plugin-wsflow/rsrc/lead-implement/lead-implement.md` directly.
- `ai-docs/spec/mcp-tools.md#L233-L243` — the `enter.implement` spec prose
  currently says only that callers provide "whether safe branch rename is
  allowed" as policy MCP cannot observe; this line needs a short update
  describing the new default-yes posture (e.g., defaults to allowed unless the
  caller explicitly withholds consent), satisfying the ticket's mandatory
  Phase 1 spec-update bullet.

## Implementation Plan
1. `agents-plugin-tool/internal/mcp/implement_resolver.go#L510`: change
   `factOr(policy.Branch.AllowRename, "unknown")` to
   `factOr(policy.Branch.AllowRename, "yes")`.
2. `agents-plugin-tool/internal/mcp/implement_resolver_test.go`: add a new
   test (or a new case within `TestResolveImplementBranchPlanRules`'s sibling
   end-to-end tests near `TestResolveImplementBranchStopOmitsPlannerInstructions`)
   that calls `resolveImplement` with `Policy.Branch.AllowRename` left as the
   zero-value `factString{}` (absent), `MergeTarget` set, current branch
   `impl/old` mismatching the resolved target scope slug, and no
   `TargetExists`/`Upstream`/`Ahead`/`Behind` set — assert
   `result.Verdict.BranchPlan.Action == "rename"`. Keep the existing explicit-
   `"no"` stop test as-is (already covers that case).
3. `agents-plugin/rsrc/lead-implement/lead-implement.md#L49`: update the
   Policy rules bullet from "Set `policy.branch.allow_rename=yes` only when
   the caller accepts pre-edit branch rename." to describe default-allow with
   an explicit-withhold escape hatch, e.g.: "`policy.branch.allow_rename`
   defaults to `yes`; set it to `no` only when the caller has explicitly asked
   to keep the current branch name."
4. Run the wsflow rsrc regen checklist (`ai-docs/ref/wsflow-mirroring.md`
   after-edit checklist, both commands in order) so
   `agents-plugin-wsflow/rsrc/lead-implement/lead-implement.md` picks up the
   same edit byte-for-byte. Do not hand-edit the wsflow copy.
5. `ai-docs/spec/mcp-tools.md#L242` (`enter.implement` entry): update the
   "whether safe branch rename is allowed" clause to state the new
   default-yes posture, matching the resolver behavior from step 1.
6. Optional low-risk polish: `agents-plugin-tool/internal/mcp/server.go#L2857`
   `allow_rename` tool-schema description — consider adding "(defaults to
   yes)" if it doesn't complicate the enum description; skip if it reads
   awkwardly, this is not required by the ticket.

## Verification Plan
- `cd agents-plugin-tool && go test ./internal/mcp/... -run 'Implement|BranchPlan' -v`
  — covers the new default-allow test plus existing `AllowRename` table/e2e
  tests (`TestResolveImplementBranchPlanRules`,
  `TestResolveImplementBranchStopOmitsPlannerInstructions`).
- `cd agents-plugin-tool && go build ./...` — sanity build after the one-line
  resolver change.
- `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
  then `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
  (from `agents-plugin-tool/`) — regenerates manifest and wsflow rsrc mirror;
  re-run `go test ./internal/wsrsrc/...` afterward to confirm
  `TestWsflowRsrcMirrorUpToDate` and manifest-drift tests are green.
- `python3 -m unittest discover agents-plugin-wsflow/tests` — required by
  `ai-docs/ref/wsflow-mirroring.md` static verification for any lead-implement
  playbook change.
- Manual: confirm `ai-docs/spec/mcp-tools.md` prose no longer contradicts the
  new default (no automated spec-check tool identified for this file).

## Escalations
- None.
