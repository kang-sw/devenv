---
title: agents-plugin lead skill namespace
related:
  260503-epic-agents-plugin-skill-porting: parent roadmap for host-neutral skill migration
  260504-feat-agents-plugin-api-docs-mcp: dogfood run that showed delegated agents can see the available skill list
parent: 260503-epic-agents-plugin-skill-porting
completed: 2026-05-04
---

# agents-plugin lead skill namespace

## Background

Delegated agents receive role prompts and bounded task briefs, but Codex still
shows the available skill list in their session context. The recent ask-api
dogfood run did not show implementers or reviewers invoking `ws:` skills
directly, but the visible names `implement`, `edit`, `proceed`, and similar
orchestration skills still create avoidable attention pressure.

The Agents plugin has no downstream consumers yet. Most current references use
the `ws:<skill>` form, so a mechanical rename is cheaper now than after merge or
downstream adoption.

## Decisions

- Rename all Agents plugin skills to a `lead-` namespace.
- Do not keep aliases in the first pass; aliases would preserve the ambiguous
  names and become migration debt.
- Keep MCP tool notation unchanged: `ws/<tool-name>` is not part of this rename.
- Keep `claude-plugin/` unchanged; this ticket covers the Agents plugin
  candidate only.
- Update the local Codex plugin cache copy during implementation so the current
  workspace sees the renamed skills immediately.
- Do not edit `ai-docs/spec/` on this branch. A later forge-spec pass will
  reconcile skill namespace specs in one batch.

## Rename Map

```text
add-rule            -> lead-add-rule
bootstrap           -> lead-bootstrap
discuss             -> lead-discuss
edit                -> lead-edit
exit-session        -> lead-exit-session
forge-mental-model  -> lead-forge-mental-model
forge-spec          -> lead-forge-spec
implement           -> lead-implement
proceed             -> lead-proceed
ship                -> lead-ship
skill-authoring     -> lead-skill-authoring
sprint              -> lead-sprint
update-spec         -> lead-update-spec
workflow            -> lead-workflow
write-code          -> lead-write-code
write-skeleton      -> lead-write-skeleton
write-spec          -> lead-write-spec
write-ticket        -> lead-write-ticket
```

## Constraints

- `lead-*` skills are lead-session entry points; delegated agents do not invoke
  them.
- Worker-facing prompts should state the boundary once, preferably in
  `delegate-orientation`.
- Host-neutral MCP pseudo-call notation stays as `ws/tool.name(...)`.
- Do not rename agent prompt stems such as `implementer`, `code-reviewer`, or
  `api-doc-manager`.
- Do not rewrite historical `claude-plugin/` skill names or Claude slash-command
  references.

## Phases

### Phase 1: Rename skill directories and frontmatter

Rename every `agents-plugin/skills/<old>/` directory to
`agents-plugin/skills/lead-<old>/` according to the map above.

Update each `SKILL.md` frontmatter `name:` field to match the new directory
stem.

Acceptance criteria:

- `find agents-plugin/skills -maxdepth 2 -name SKILL.md` lists only `lead-*`
  skill directories.
- No `agents-plugin/skills/<old>/SKILL.md` directory remains for the old names.
- The plugin cache skill directories mirror the repository skill directories.

### Result (22251b5) - 2026-05-04

Renamed every Agents plugin skill directory to `lead-*`, updated `SKILL.md`
frontmatter, and mirrored the same directory set into the local Codex plugin
cache. No alias directories were kept.

### Phase 2: Rewrite Agents plugin references

Mechanically rewrite references in Agents plugin materials from `ws:<old>` to
`ws:lead-<old>` and from old skill paths to the new paths.

In scope:

- `agents-plugin/`
- `agents-plugin-tool/` prompt or reference text that names plugin skills
- `ai-docs/_index.md` inventory and queue text
- active tickets that reference Agents plugin skill names

Out of scope:

- `claude-plugin/`
- `ai-docs/spec/`
- `ai-docs/mental-model/`
- old historical reference snapshots unless they are active operational inputs

Acceptance criteria:

- `rg 'ws:(add-rule|bootstrap|discuss|edit|exit-session|forge-mental-model|forge-spec|implement|proceed|ship|skill-authoring|sprint|update-spec|workflow|write-code|write-skeleton|write-spec|write-ticket)\\b'` returns no in-scope hits.
- `rg 'agents-plugin/skills/(add-rule|bootstrap|discuss|edit|exit-session|forge-mental-model|forge-spec|implement|proceed|ship|skill-authoring|sprint|update-spec|workflow|write-code|write-skeleton|write-spec|write-ticket)\\b'` returns no in-scope hits.

### Result (22251b5) - 2026-05-04

Rewrote Agents plugin skill invocations and path references to `lead-*` in the
plugin candidate, prompt/reference text, active project memory, and the active
host-neutral plugin research anchor. Historical Claude-plugin references and
spec/mental-model documents were left untouched.

### Phase 3: Strengthen delegate boundary wording

Update `delegate-orientation` so delegated workers are told that `lead-*`
skills are lead-owned orchestration entry points. Workers may use visible MCP
tools and role prompts, but should not invoke lead skills unless the caller
explicitly assigns lead-session work.

Acceptance criteria:

- Delegate orientation contains one concise lead-skill boundary rule.
- The rule does not enumerate every skill name; the prefix carries the taxonomy.
- Prompt bundle metadata is updated if embedded prompt text changes.

### Result (22251b5) - 2026-05-04

Added a concise `lead-*` boundary rule to `delegate-orientation` and updated the
embedded prompt bundle hash in `agents-plugin/runtime.json`.

### Phase 4: Verify plugin and cache consistency

Run repository checks and current-cache consistency checks after the rename.

Acceptance criteria:

- `cd agents-plugin-tool && go test ./...`
- `git diff --check`
- Repository and local plugin cache skill directory sets match.
- No broad alias or compatibility shim is introduced.
- Completion report notes that specs were intentionally deferred to a later
  forge-spec pass.

### Result (22251b5) - 2026-05-04

Verified the rename with `go test ./...`, `git diff --check`, old-name rg
checks, and a repository-vs-cache skill directory comparison. Spec updates are
intentionally deferred to the later forge-spec pass for this branch.
