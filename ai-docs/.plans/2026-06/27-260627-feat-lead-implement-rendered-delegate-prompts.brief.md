# Brief: 260627-feat-lead-implement-rendered-delegate-prompts Phase 2

## Intent

Move delegated review-fix relay dispatch to a rendered, file-first
`implementer-relay` prompt so `lead-implement` no longer carries the long relay
prompt in its always-rendered body.

## Scope Boundary

Implement only Phase 2: add the review-fix relay render surface.

In scope:
- Add an `implementer-relay` rsrc playbook when that keeps relay inputs clearer
  than overloading `implementer`.
- Render relay prompts from review findings paths, current commit range, lead
  disposition notes, verification metadata, and cycle metadata.
- Replace the long `lead-implement` Review relay prompt with a minimal rendered
  prompt path dispatch.
- Preserve stateless relay semantics and existing option to reuse an
  implementer or reviewer only as a latency optimization.
- Regenerate the shipped rsrc manifest and byte-identical wsflow rsrc mirror.
- Update focused render tests for `lead-implement`, `implementer-relay`, and
  wsflow mirrored rendering behavior.

Out of scope:
- Do not change initial `implementer` dispatch from Phase 1 except to keep shared
  wording consistent if needed.
- Do not change `ws.enter.implement` schema, verdict logic, branch policy, or
  todo builders.
- Do not close Phase 3 documentation beyond minimum source-test support for this
  phase.
- Do not implement the older review-fix ownership ticket, except avoid
  contradicting its settled intended contract.

## Caller-Visible Contract

For delegated non-clean review, `lead-implement` renders an
`implementer-relay` prompt with declared file-first inputs, captures the returned
prompt path and `recommended-tier`, then gives a worker a short instruction to
read and execute that rendered prompt. Reviewer findings remain in files and are
passed by path. The lead supplies only disposition/adjudication notes and
verification metadata.

The implementation owner applies fixes:
- Direct-edit mode: the lead applies fixes.
- Delegated mode: the implementer receives non-clean review path files through
  the rendered relay prompt.

The lead still owns triage, rejected/deferred/won't-fix rationale, verification,
re-review orchestration, and final clean judgment.

## Contract Instructions

- Keep relay context small and mechanical: brief path, optional plan path,
  review cycle, commit range, non-clean review paths, disposition notes,
  verification hint, and result expectations.
- Use declared variables only. `implementer-relay` is not a wsflow legacy
  freeform Render Context stem.
- Keep reviewer findings in files. Do not copy full findings into
  `lead-implement`.
- Keep the relay prompt self-contained. It must not rely on prior implementer
  conversation or inherited lead context.
- Preserve disposition vocabulary: `[fixed]`, `[won't fix: <reason>]`,
  `[deferred: <reason>]`, and reviewer-side `[accepted]` or `[maintained:
  <reason>]`.
- State that won't-fix is allowed for style suggestions conflicting with local
  patterns or scope expansion, and is not allowed for correctness, security, or
  contract violations.
- Preserve commit-log durability: fix commits must record dispositions in
  `## AI Context`.
- Use `{{.McpNamespace}}` and `{{.SkillNamespace}}` for shared namespace text.
- Keep canonical edits under `agents-plugin/rsrc/`; regenerate
  `agents-plugin/rsrc/manifest.json` and `agents-plugin-wsflow/rsrc/`.

## Integration Test Instructions

Extend existing Go tests under `agents-plugin-tool/internal/mcp/` and rsrc tests
under `agents-plugin-tool/internal/wsrsrc/`.

Required checks:
- `implementer-relay` renders successfully with representative declared context.
- Rendered relay output includes review paths, commit range, cycle, disposition
  notes, verification hint, result expectations, and the role model hint.
- Passing undeclared context to full ws `implementer-relay` returns
  `ErrUndeclaredVar`.
- In wsflow/no-agent mode, `implementer-relay` still rejects undeclared context
  because it is not a legacy freeform stem.
- `lead-implement` rendered body no longer carries the old long Review relay
  prompt body while still naming the rendered relay dispatch contract.
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

- Prefer a separate `implementer-relay` playbook over adding relay mode flags to
  `implementer`; relay input differs materially from initial implementation.
- Keep lead-owned relay text to a short rendered-prompt dispatch.
- Treat review findings paths and disposition notes as the relay contract.
- Preserve the existing partitioned review loop and cycle caps; this phase only
  changes prompt materialization.
- Keep same-agent reuse optional and non-normative; correctness comes from files,
  commit ranges, and self-contained prompt inputs.

## Rejected Alternatives

- Passing full review findings through `playbook.render` context is rejected
  because findings belong in files.
- Keeping the long Review relay prompt in `lead-implement` is rejected because it
  keeps fix-loop branch detail in the always-rendered lead playbook.
- Combining initial and relay dispatch behind a mode variable is rejected for
  this phase because it would create optional-variable noise.
- Making relay correctness depend on host conversation continuation is rejected;
  retained agents are a latency optimization only.

## Approach

- Inspect current `lead-implement`, `implementer`, render tests, and rsrc
  manifest/mirror tests.
- Add `agents-plugin/rsrc/implementer-relay/implementer-relay.md` with declared
  variables and direct-execution delegate metadata.
- Replace the `lead-implement` relay template with a compact rendered relay
  handoff.
- Update render tests and negative undeclared-context tests.
- Regenerate manifest and wsflow rsrc mirror.
- Run focused, package, full, and diff checks.
- Run a fresh-reader audit on edited prompt/playbook text and apply accepted
  fixes.

## Constraints

- AI-authored text stays English.
- Skill/playbook edits follow the skill-authoring checklist.
- Fresh-reader audit is required after editing prompt/playbook text.
- Do not hand-edit `agents-plugin-wsflow/rsrc/`; regenerate it from canonical
  rsrc.
- Do not update `runtime.json`; rsrc text changes require manifest and mirror
  regeneration only.

## Out of scope

- Phase 3 documentation closeout beyond source-facing tests.
- Review owner wording cleanup outside the relay surface.
- Changes to reviewer partition prompts unless a test fixture requires a narrow
  expected-text update.

## Details

Likely lead relay handoff shape:

```text
Rendered review relay prompt: <prompt-path>
Recommended tier: <recommended-tier>

Read that prompt file and execute it. It contains the review findings paths,
disposition notes, verification expectations, and reporting requirements.
```

Likely `implementer-relay` render variables:

| variable | purpose |
| --- | --- |
| `RoleModel` | tier-derived model hint |
| `BriefPath` | required implementation contract path |
| `PlanPath` | optional plan path or empty/no-plan sentinel |
| `ReviewCycle` | current review-fix cycle |
| `CommitRange` | implemented or updated diff range |
| `ReviewPaths` | non-clean review findings paths |
| `DispositionNotes` | lead triage and prior accepted/deferred dispositions |
| `VerificationHint` | concise command or test hint |
| `ResultExpectations` | concise output/commit/disposition reporting expectation |

## Verification Contract

Acceptance requires:
- Source render tests prove `implementer-relay` uses declared context, rejects
  undeclared context, and preserves recommended tier.
- Lead playbook render tests prove the long Review relay prompt is gone from
  always-rendered `lead-implement`.
- Manifest and wsflow mirror tests pass after regeneration.
- wsflow package tests pass after the shared rsrc edit.
- Full Go tests pass unless an unrelated stale test is documented with evidence.

## References

- [Must] `ai-docs/spec/workflow-skills.md` - lead-implement delegated mode,
  stateless review-fix relay, review allocation, and documentation gates.
- [Must] `ai-docs/spec/mcp-tools.md` - `playbook.render`, declared context,
  recommended-tier, product-mode rendering, and wsflow legacy-context bridge.
- [Must] `ai-docs/mental-model/workflow-skills.md` - workflow playbook ownership,
  review relay, and file-path relay mistake guidance.
- [Must] `ai-docs/mental-model/prompt-bundle.md` - rsrc playbook variables,
  role/tier frontmatter, manifest, and wsflow mirror coupling.
- [Must] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` -
  native-subagent pivot, playbook.print/render split, and prompt-factory
  direction.
- [Must] `ai-docs/ref/wsflow-mirroring.md` - wsflow rsrc mirror regeneration and
  product-boundary checks.
- [Must] `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md` -
  skill/playbook authoring and fresh-reader audit rules.
- [Must] `ai-docs/tickets/todo/260525-bug-implement-review-fix-owner.md` -
  implementation-owner contract for delegated review fixes.
- [Maybe] `ai-docs/mental-model/mcp-runtime.md` - keyed capability and
  enter-tool ownership constraints.
- [Maybe] `ai-docs/mental-model/named-agent-runtime.md` - mercenary registration
  behavior if tests touch full-ws mercenary relay.
