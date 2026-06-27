# Brief: 260627-feat-lead-implement-rendered-delegate-prompts Phase 1

## Intent

Move initial delegated implementation dispatch to a file-first rendered
`implementer` prompt so `lead-implement` no longer carries the long initial
Implementer spawn prompt in its always-rendered body.

## Scope Boundary

Implement only Phase 1: parameterize initial implementer dispatch.

In scope:
- Add declared render variables to `implementer` for small mechanical context:
  brief path, optional plan path convention, verification hint, result
  expectations, and commit-range/reporting metadata.
- Move initial implementation task input into the rendered `implementer`
  playbook; the brief is the primary contract, and the plan path is optional.
- Replace the long `lead-implement` `Implementer spawn prompt` template with a
  minimal dispatch that names the rendered prompt path and tells the worker to
  execute it.
- Preserve `implementer` frontmatter `tier: medium` and the resulting
  `recommended-tier` behavior from `playbook.render`.
- Regenerate the shipped rsrc manifest and byte-identical wsflow rsrc mirror
  after canonical rsrc edits.
- Update focused render tests for `lead-implement`, `implementer`, and wsflow
  mirror/product-mode behavior.

Out of scope:
- Do not implement review-fix relay rendering or add `implementer-relay`.
- Do not change reviewer prompt frames, relay prompts, or re-review prompts.
- Do not change `ws.enter.implement` schema, resolver logic, or todo builders
  unless an existing test fixture needs only a narrow expected-text update.
- Do not change branch policy, review allocation, documentation closeout, merge
  policy, or ticket status.

## Caller-Visible Contract

For delegated initial implementation, `lead-implement` prepares a brief and
optionally a plan, renders `implementer` through `{{.McpNamespace}}/playbook.render`
with declared variables only, captures the returned prompt path and
`recommended-tier`, then gives the worker a short instruction to read and
execute that rendered prompt.

The implementer must not read the ticket directly. The brief owns the contract;
the plan refines it when present. Render context carries only pointers and
metadata, never the full ticket, acceptance contract, or design rationale.

## Contract Instructions

- Keep the file-first contract: brief path is required; plan path may be an
  empty string or another documented no-plan sentinel.
- Keep render context small and mechanical. Expected variable set should stay
  close to `BriefPath`, `PlanPath`, `VerificationHint`,
  `ResultExpectations`, and `CommitRangeHint`.
- Use declared variables only. `implementer` is not a wsflow legacy freeform
  Render Context stem, so undeclared context must continue to fail.
- Keep `RoleModel` declared and rendered from `tier: medium`; do not hard-code
  model names in prompt body text.
- Keep namespace text product-safe with `{{.McpNamespace}}` and
  `{{.SkillNamespace}}` where shared playbook text names ws MCP or skills.
- Keep the lead spawn prompt minimal: rendered prompt path, recommended tier,
  and "execute that rendered prompt" guidance. Do not copy the old acceptance
  list back into `lead-implement`.
- Do not make the worker read `ai-docs/tickets/ready/...` for this task. Any
  ticket decision needed by the worker belongs in this brief or the plan.
- Preserve same-source rsrc behavior: edit canonical `agents-plugin/rsrc/`
  first, regenerate `agents-plugin/rsrc/manifest.json`, then regenerate the
  byte-identical `agents-plugin-wsflow/rsrc/` copy.

## Integration Test Instructions

Extend existing Go tests under `agents-plugin-tool/internal/mcp/` and rsrc tests
under `agents-plugin-tool/internal/wsrsrc/`.

Required checks:
- `implementer` renders successfully with representative declared context,
  includes brief/plan/verification/result metadata, preserves `RoleModel`, and
  returns the existing recommended tier.
- Passing an undeclared context key to full ws `implementer` still returns
  `ErrUndeclaredVar`.
- In wsflow/no-agent mode, `implementer` still rejects undeclared context
  because it is not one of the five legacy freeform stems.
- `lead-implement` rendered body no longer contains the old long initial
  Implementer spawn prompt acceptance list, while still naming the minimal
  rendered-prompt dispatch contract.
- The wsflow rsrc mirror is byte-identical after regeneration.

Run:
- `go test ./internal/mcp -count=1 -run 'TestPlaybook.*Implementer|TestRenderPlaybook.*Implementer|TestPlaybookPrintGoldenLeadImplement|TestPlaybookPrintWsflowLeadImplementOmitsMercenaryCommands'`
- `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`
- `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
- `go test ./internal/wsrsrc -count=1`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `go test ./... -count=1`
- `git diff --check`

## Implementation Strategy Decisions

- Treat the brief as the contract source of truth. The rendered implementer
  prompt tells the worker which files to read and how to report, not what the
  whole ticket says.
- Use explicit declared variables rather than a generic freeform context block.
- Keep initial implementation and review-fix relay as separate surfaces. Phase 1
  changes only `implementer`; Phase 2 can add `implementer-relay`.
- Prefer one compact "initial dispatch" template in `lead-implement` over
  duplicating acceptance and verification instructions there.
- Preserve current render mechanics. This phase should be mostly rsrc text,
  manifest/mirror regeneration, and focused render-test expectation updates.

## Rejected Alternatives

- Passing the full brief text through `playbook.render` context is rejected
  because large prose contracts belong in files.
- Keeping the old long Implementer spawn prompt in `lead-implement` is rejected
  because it keeps unreachable or already-file-backed task input in the lead's
  always-rendered context.
- Using wsflow's legacy `## Render Context` bridge for `implementer` is rejected
  because that bridge intentionally applies only to five legacy stems.
- Combining initial dispatch and review-fix relay behind mode flags is deferred;
  split surfaces are safer when variable sets differ.

## Approach

- Inspect the current `implementer` frontmatter/body and add only variables used
  by the body.
- Rewrite `implementer` input instructions around file-first Mode A: read the
  brief, read the plan only when the rendered plan path is non-empty, and do not
  read the ticket.
- Replace `lead-implement`'s `Implementer spawn prompt` template with a short
  rendered-prompt dispatch block.
- Update tests to cover declared context rendering, undeclared-context rejection,
  recommended-tier preservation, and removal of the old long spawn template.
- Regenerate manifest and wsflow rsrc mirror after rsrc edits.

## Constraints

- AI-authored text stays English.
- Skill/playbook edits follow the skill-authoring checklist: concise,
  falsifiable, actionable, context-free rules before rationale.
- Fresh-reader audit is required after editing `agents-plugin/rsrc/lead-*` or
  delegate prompt text; if no subagent is allowed in the execution context, stop
  and report that blocker instead of pretending the audit happened.
- Do not hand-edit `agents-plugin-wsflow/rsrc/`; regenerate it from canonical
  rsrc.
- Do not update `runtime.json`; rsrc text changes require manifest and mirror
  regeneration only.

## Details

Likely target prompt shape:

```text
Rendered implementer prompt: <prompt-path>
Recommended tier: <recommended-tier>

Read that prompt file and execute it. It contains the brief path, optional plan
path, verification expectations, and reporting requirements.
```

Likely `implementer` render variables:

| variable | purpose |
| --- | --- |
| `RoleModel` | existing tier-derived model hint |
| `BriefPath` | required implementation contract path |
| `PlanPath` | optional plan path or empty/no-plan sentinel |
| `VerificationHint` | concise command or test hint |
| `ResultExpectations` | concise output/commit reporting expectation |
| `CommitRangeHint` | expected commit-range reporting metadata |

## Verification Contract

Acceptance requires:
- Source render tests prove `implementer` uses declared context and preserves
  recommended tier.
- Negative render tests prove undeclared context still fails in full ws and
  wsflow for non-legacy `implementer`.
- Lead playbook render tests prove the old long initial Implementer spawn prompt
  is gone from always-rendered `lead-implement`.
- Manifest and wsflow mirror tests pass after regeneration.
- wsflow package tests pass after the shared rsrc edit.

## References

Ticket decisions have been incorporated into this brief. The implementer should
not read the ticket directly.

- [Must] `ai-docs/spec/workflow-skills.md` - lead-implement delegated mode, brief/plan contract, and verdict/todo boundary.
- [Must] `ai-docs/spec/mcp-tools.md` - `playbook.render`, declared context, recommended-tier, and wsflow legacy-context bridge.
- [Must] `ai-docs/mental-model/workflow-skills.md` - workflow playbook ownership and delegated implementation boundaries.
- [Must] `ai-docs/mental-model/prompt-bundle.md` - rsrc playbook variables, role/tier frontmatter, manifest, and wsflow mirror coupling.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - product-mode rendering, session key, and enter-tool ownership constraints.
- [Must] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` - native-subagent pivot, playbook.print/render split, and rsrc prompt-factory direction.
- [Must] `ai-docs/ref/wsflow-mirroring.md` - wsflow rsrc mirror regeneration and product-boundary checks.
- [Must] `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md` - skill/playbook authoring and fresh-reader audit rules.
- [Maybe] `ai-docs/spec/named-agent-runtime.md` - rendered prompt registration and tier pass-through context.
- [Maybe] `ai-docs/mental-model/named-agent-runtime.md` - mercenary registration behavior if tests touch the full-ws mercenary path.
- [Maybe] `ai-docs/mental-model/plugin-runtime.md` - package-level runtime/mirror context if wsflow package checks fail.
