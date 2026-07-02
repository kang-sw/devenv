---
title: Pre-release cleanup — epic merge gate items before main
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260619-research-ws-delegate-continuity-host-neutral-fallback: item 1
  260620-bug-mercenary-path-visible-when-prefer-off: item 2
  260620-bug-ws-delegate-playbook-output-language-unbound: item 3
  260620-bug-ws-prompt-override-no-unset-path: item 4
  260622-bug-bump-version-script-edits-legacy-launcher: item 5
  260622-bug-wsflow-launcher-coldload-divergence: item 6
  260624-feat-prefer-mercenary-hide-option: item 7
---

# Pre-release cleanup — epic merge gate items before main

Workset grouping cleanup items that should land on the epic branch before merging
to `main`. All are idea-stage tickets promoted to this workset; individual items
are independent and can be implemented in any order.

The single remaining hard gate before main is `260622-chore-windows-shipping-hardening`
Phase C (real-Windows acceptance). These cleanup items run in parallel with or before
Phase C, whichever is convenient.

## Items

### 1. `260619-research-ws-delegate-continuity-host-neutral-fallback`
**Type:** docs  
**Scope:** `lead-implement`, `lead-write-spec` relay/re-review prompts  
**Work:** SendMessage is already absent from rsrc (grep confirmed). Add explicit
fresh-spawn fallback guidance inline in relay and re-review prompts so operators
never dead-end. Note the mercenary path as host-neutral stateful continuation option.

### 2. `260620-bug-mercenary-path-visible-when-prefer-off`
**Type:** bug / playbook  
**Work:** When `prefer_mercenary=off`, the rendered `lead-implement` procedure still
shows full mercenary dispatch idiom. Fix conditional rendering so mercenary idiom
is suppressed when `prefer_mercenary` is off or unset.

### 3. `260620-bug-ws-delegate-playbook-output-language-unbound`
**Type:** bug / playbook  
**Work:** Delegate playbooks carry no explicit output-language binding, so native
subagents inherit session language. Add an English output-language binding to
bundled delegate playbooks (complement to `workflow.lang` which binds the lead).

### 4. `260620-bug-ws-prompt-override-no-unset-path`
**Type:** bug / config surface  
**Work:** `config.prompt.set` cannot clear an override (requires non-empty value).
Add `config.prompt.unset` (or `clear`) operation and wire it through MCP + runtime
contract.

### 5. `260622-bug-bump-version-script-edits-legacy-launcher`
**Type:** bug / script  
**Work:** `agents-plugin-tool/scripts/bump-ws-version.sh` edits the old POSIX shell
launcher path instead of the live `.py` launcher. Update the script to target the
correct file.

### 6. `260622-bug-wsflow-launcher-coldload-divergence`
**Type:** bug / launcher  
**Work:** `agents-plugin-wsflow/bin/ws-mcp-launcher.py` never received the `260524`
`wait_for_runtime_contract` materialization wait or the `260622` Phase B cold-load
hardening. Port the canonical Phase B diff (commits `ab1460d4`, `da1047fb`) to the
wsflow launcher, keeping wsflow text non-ws-aware.

### 7. `260624-feat-prefer-mercenary-hide-option`
**Type:** feature / config  
**Work:** Extend `prefer_mercenary` to accept `on | off | hide`. `hide` removes
`ws.mercenary.*` from `filteredTools()` (discovery) and `toolAllowed()` (call-gate),
mirroring the exec permanent-hide pattern. Default remains `off`.

## Dropped from workset

- `260619-research-claude-teammate-mode-subagent-collection-doc-gap` — dropped;
  Claude teammate-mode API surface is in active flux; re-evaluate post-stabilization.
