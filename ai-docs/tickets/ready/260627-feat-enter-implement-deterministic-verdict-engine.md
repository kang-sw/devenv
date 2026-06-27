---
title: Deterministic enter.implement strategy and branch verdict resolution
sage-review: recommended
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260627-feat-enter-proceed-deterministic-verdict-engine: predecessor optimization pattern for moving deterministic verdict resolution into typed enter tools
  260625-feat-ws-session-state-machine: introduced typed enter tools and session agenda/todo persistence
  260523-bug-implement-merge-target-discovery: branch preflight must avoid unsafe merge-target inference on nested implementation branches
related-mental-model:
  - workflow-skills
  - mcp-runtime
  - git-workflow-tools
---

# Deterministic enter.implement strategy and branch verdict resolution

## Background

The `lead-implement` route stage currently asks the LLM to choose final
implementation labels: delegation mode, branch mode, plan depth, review
allocation, review need, and documentation need. `ws.enter.implement` then stores
those labels as agenda payload and derives todo titles. That is useful session
state, but it leaves deterministic strategy composition in playbook prose.

The same optimization chosen for `ws.enter.proceed` should apply here:
`lead-implement` should gather normalized implementation facts, then call one
typed mode-switch MCP tool at the fact-complete boundary. MCP should own every
deterministic implementation verdict rule it can derive from those facts,
including agenda payload, todo replacement, canonical JSON output, and stable raw
Implementation Verdict text.

Branch handling is the main refactor boundary. Branch state is observable from
the repository, so the input schema should not make the LLM mirror Git preflight.
The caller should provide only branch intent or policy that MCP cannot observe,
such as the requested merge target while already on an `implement/*` branch and
whether a safe pre-edit rename is allowed.

## Decisions

- Keep `ws.enter.implement` as the only public MCP mode-switch call for
  implementation strategy verdict resolution.
- Do not add a separate public implementation route or strategy helper. Reusable
  strategy and branch-plan logic may exist as private Go helpers behind
  `ws.enter.implement`.
- Move deterministic `lead-implement` verdict rules into MCP wherever the rule
  can be derived from normalized facts or repository branch preflight.
- Keep ambiguous judgments in the playbook. The LLM still reads the ticket,
  caller request, relevant docs, and source-independent context to fill fact
  fields that cannot be observed mechanically.
- Split input into `target`, `facts`, and `policy`. The LLM fills facts and
  explicit policy overrides; MCP returns verdict labels.
- Do not ask the LLM to input final labels such as `delegation`, `plan_depth`,
  `review_alloc`, `need_review`, or `need_doc` under the new schema. Those are
  derived verdict fields.
- Remove branch observation fields from the schema. MCP observes current branch,
  current HEAD, implementation branch existence, upstream/tracking ambiguity, and
  start commit from the session key's repository root.
- Replace the vague `branch_mode` label with a precise `branch_plan.action`
  verdict: `create`, `continue`, `rename`, or `stop`.
- Stop rather than infer when a required merge target is missing on an existing
  `implement/*` branch. This preserves the safety boundary captured by the
  merge-target discovery follow-up.
- Record branch execution results separately from route-time facts when needed.
  If the implementation branch is created or renamed after the verdict, runtime
  context may be persisted through agenda state without a second
  `enter.implement` call.
- Emit final MCP output that lets the LLM clearly know the implementation
  direction to proceed next, without recomputing the strategy.
- Preserve the existing enter-tool mode-switch semantics: the call stores the
  `implement` agenda and replaces the todo list with the derived implement
  checklist.

## Contract Sketch

`ws.enter.implement` should accept the lead `session_key`, a `target` object, a
grouped optional/nullable `facts` object, and a small `policy` object:

```json
{
  "session_key": "<lead-session-key>",
  "target": {
    "kind": "ticket | inline | unknown",
    "label": "string",
    "ticket_stem": "string | null",
    "ticket_path": "string | null",
    "scope_label": "Phase 1: ... | whole target | caller slice | unknown",
    "scope_slug": "string | null"
  },
  "facts": {
    "scope": {
      "span": "single-file | multi-file | unknown",
      "surface": "internal | public-interface | cross-module | unknown",
      "new_public_symbol": "yes | no | unknown",
      "new_type_contract": "yes | no | unknown",
      "test_surface": "none | existing | new-files | unknown",
      "explicit_delegation_request": "yes | no | unknown"
    },
    "complexity": {
      "change_points": "clear | partially-known | unknown",
      "reuse_points": "confirmed | unconfirmed | not-applicable | unknown",
      "strategy_shape": "single-obvious | multiple-viable | unknown",
      "side_effect_risk": "low | moderate | high | unknown",
      "cold_context": "yes | no | unknown"
    },
    "risk": {
      "correctness": "low | moderate | high | unknown",
      "fit": "low | moderate | high | unknown",
      "test": "low | moderate | high | unknown",
      "security_or_contract": "low | moderate | high | unknown"
    }
  },
  "policy": {
    "branch": {
      "merge_target": "string | null",
      "allow_rename": "yes | no | unknown"
    },
    "review": {
      "override": "auto | lead-only | single | partitioned | null"
    },
    "docs": {
      "mode": "standard | skip-with-reason | unknown",
      "reason": "string | null"
    }
  },
  "format": "text | json"
}
```

Schema direction:

- `target.scope_slug` names the desired implementation branch suffix. If absent,
  MCP may derive a sanitized slug from `target.label` or `target.scope_label` and
  emit a warning.
- `facts` groups are optional so partial callers can still enter implement mode.
  Missing fields normalize to `unknown` or `not-applicable` according to
  deterministic resolver rules.
- `policy` carries explicit caller or user choices. It should stay small and
  should not duplicate observable Git state.
- `policy.review.override=auto` or `null` means MCP derives review allocation.
  Explicit overrides are allowed but should still emit warnings when they reduce
  coverage below the risk facts.
- `policy.docs.mode=standard` is the default. Documentation steps may be skipped
  only with an explicit reason, and the raw verdict must make the skip visible.
- Hard-block only malformed JSON/type failures, authentication or session-key
  failures, enum-level values outside the accepted schema, or branch safety stops
  where continuing could merge into the wrong target.

## Deterministic Verdict Rules

MCP should derive these verdict fields from facts and observed Git state:

```json
{
  "verdict": {
    "delegation": "direct-edit | delegated",
    "branch_plan": {
      "action": "create | continue | rename | stop",
      "current_branch": "string",
      "target_branch": "implement/<scope_slug> | null",
      "merge_target": "string | null",
      "start_commit": "hex commit | null",
      "reason": "string",
      "warnings": []
    },
    "plan_depth": "none | brief | survey | research",
    "review_alloc": "lead-only | single | partitioned: correctness | partitioned: fit | partitioned: test | partitioned: correctness, fit | partitioned: correctness, test | partitioned: fit, test | partitioned: correctness, fit, test",
    "need_review": true,
    "doc_mode": "standard | skipped"
  }
}
```

Required rule shape:

- Delegation is `direct-edit` only when the work is single-file, internal-only,
  has no public interface or cross-module surface, introduces no public symbol or
  type contract, adds no new test files, and has no explicit delegation request.
  Otherwise it is `delegated`.
- Plan depth is `none` only for narrow direct edits with clear change points and
  low side-effect risk. Delegated work has a minimum depth of `brief`.
  Multi-file or unconfirmed reuse defaults to `survey`; multiple viable
  strategies or high/non-obvious side-effect risk defaults to `research`.
- Review allocation is `lead-only` only for mechanical low-risk work.
  Direct-edit moderate work uses `single`. Delegated, public-interface, contract,
  cross-module, or high-risk work uses partitioned review.
- Partitioned review selects the smallest set of `correctness`, `fit`, and
  `test` partitions that covers material risk. Unknown risk is conservative and
  should not silently remove review coverage.
- `need_review` is derived from review allocation: `lead-only` means no reviewer
  stage; every other allocation includes review.
- Documentation mode defaults to `standard`, which includes Doc pre-pass, Doc
  commit gate, and Doc closeout. A skip must be explicit in policy and visible in
  raw output.

Branch preflight rules:

- If the current branch is not `implement/*`, set `merge_target` to the current
  branch and `branch_plan.action=create` for `implement/<scope_slug>`.
- If the current branch is `implement/*` and no merge target is provided through
  policy, set `branch_plan.action=stop` with a merge-target-required reason.
- If the current branch is `implement/*` and the branch scope matches the target
  scope, set `branch_plan.action=continue`.
- If the current branch is `implement/*` and the branch scope differs, set
  `branch_plan.action=rename` only when `allow_rename=yes`, the target branch is
  absent, and upstream/tracking state is safe. Otherwise stop with a precise
  branch safety reason.
- MCP may observe `start_commit` at enter time, but branch creation or rename
  itself remains an execution step owned by `lead-implement` after reading the
  verdict.

## Raw Verdict Format

Default text output should render canonical raw verdict text. JSON output, when
requested, should include the same `raw` field.

```text
Implementation Verdict
Mode: delegated
Branch Action: create implement/enter-implement-deterministic-verdict-engine
Merge Target: feature/ferrule
Plan Depth: survey
Review Allocation: partitioned: correctness, fit, test
Doc Mode: standard

Target: 260627-feat-enter-implement-deterministic-verdict-engine
Scope: Phase 1: MCP-owned implement strategy verdict
Reason: public-interface=yes; cross-module=yes; reuse-points=unconfirmed

Conditions:
- span=multi-file
- surface=public-interface
- explicit-delegation-request=no
- side-effect-risk=moderate
- correctness-risk=high
- fit-risk=moderate
- test-risk=moderate

Warnings:
- none

Agenda:
- delegation: delegated
- branch_plan.action: create
- branch_plan.target_branch: implement/enter-implement-deterministic-verdict-engine
- merge_target: feature/ferrule
- plan_depth: survey
- review_alloc: partitioned: correctness, fit, test
- need_review: true
- doc_mode: standard
```

Rules:

- The first non-empty line is always `Implementation Verdict`.
- The next lines always include `Mode`, `Branch Action`, `Plan Depth`, `Review
  Allocation`, and `Doc Mode`.
- `Branch Action: stop` must include a short reason that tells the LLM what to
  ask or fix before implementation can continue.
- `Conditions` renders normalized facts used by the resolver, not raw caller
  input.
- `Warnings` renders deterministic normalization notes; use `- none` when empty.
- `Agenda` renders the values written to the `implement` agenda blob.
- Do not include long explanatory prose in raw output. The lead may add
  user-facing explanation after reading the verdict, but MCP output should remain
  stable and easy to follow.

## Spec Impact

Target spec areas:

- `ai-docs/spec/workflow-skills.md`: `lead-implement` route/verdict behavior and
  the timing of `ws.enter.implement`.
- `ai-docs/spec/mcp-tools.md`: typed enter-tool contract, implement agenda/todo
  derivation, JSON result shape, raw verdict output, and branch preflight safety.

Expected caller-visible change:

- `lead-implement` becomes lighter after route facts are gathered: it calls
  `ws.enter.implement`, follows the returned implementation verdict, and stops
  on branch safety blockers instead of recomputing final strategy labels in
  prose.
- `ws.enter.implement` accepts `target + facts + policy`, observes branch state
  from the repository root, returns deterministic JSON and raw verdict output,
  writes the implement agenda, and replaces the todo list.

Contract-first spec: no. This ready ticket pins the implementation slice in
enough detail for development; the implementation closeout should update
`workflow-skills` and `mcp-tools` to match the shipped contract.

## Phases

### Phase 1: MCP-owned implement strategy verdict

Implement deterministic strategy and branch verdict resolution inside
`ws.enter.implement`.

Required behavior:

- Replace final-label inputs with the `target + facts + policy` schema while
  preserving `session_key`.
- Add private pure resolver helpers for implementation strategy derivation and
  branch plan construction.
- Make MCP observe current branch, HEAD/start commit, target branch existence,
  and upstream/tracking ambiguity instead of asking the LLM to provide those as
  facts.
- Derive `delegation`, `branch_plan`, `plan_depth`, `review_alloc`,
  `need_review`, and documentation mode deterministically.
- Emit canonical JSON and raw Implementation Verdict output that makes the next
  execution direction clear to the LLM.
- Store the normalized implement agenda and replace the todo list using the
  derived verdict labels.
- Update `lead-implement` so Route gathers normalized facts and policy, calls
  `ws.enter.implement`, and follows the returned verdict instead of carrying the
  extracted deterministic tables in prose.
- Keep branch creation, branch rename, source edits, delegate dispatch, review,
  documentation, and merge as `lead-implement` execution steps after the verdict.
- Do not call `ws.enter.implement` a second time after branch execution; use
  agenda state for later runtime context if needed.
- Preserve wsflow-visible behavior and evaluate wsflow mirror requirements for
  any shared playbook edits.

Deferred scope:

- Do not change `ws.enter.proceed`; that sibling optimization is covered by
  `260627-feat-enter-proceed-deterministic-verdict-engine`.
- Do not implement branch cleanup after merge; that remains separate from
  strategy verdict resolution.
- Do not introduce a public helper surface outside `ws.enter.implement`.

Verification:

- Unit-test resolver precedence for direct/delegated mode, plan depth, review
  allocation, documentation mode, and warning generation.
- Unit-test branch preflight for create, continue, rename, stop, missing merge
  target, target branch already exists, and ambiguous upstream/tracking cases.
- Verify `ws.enter.implement` text output includes canonical raw verdict lines
  and that JSON output includes the same `raw` field.
- Verify agenda storage and derived todo replacement still work through the
  session-state flow tests.
- Verify `lead-implement` playbook output no longer carries duplicated
  deterministic strategy tables after the MCP resolver owns them.
- Run the relevant MCP and rsrc/wsflow mirror tests after source/playbook edits.
