---
title: Pi lead cannot see or self-load ws skills because removing native read/bash drops Pi's skills block from the system prompt
related:
  260905-feat-ws-pi-lead-one-liner-exec-escape-hatch: landed the lead tool-surface reshaping that removes native read and bash
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 2b63821756147d8f
sage-review-completeness-reviewed: 2b63821756147d8f
---

# Pi lead cannot see or self-load ws skills because removing native read/bash drops Pi's skills block from the system prompt

## Background

Owner dogfood question, 2026-09-06: is `lead-drain-ready-queue` a skill the
Pi lead cannot run on its own, so that the user must invoke it? In practice
yes, and not only that skill: the Pi lead currently cannot see any ws skill,
and could not load one even if it knew the name.

Pi's skill mechanism is progressive disclosure. At session start Pi renders an
`<available_skills>` block (name, description, SKILL.md `location`) into the
system prompt and tells the model to load a skill's file with the `read`
tool, or with `bash` when `read` is absent (`docs/skills.md`, "How Skills
Work"). Users can also force a skill with `/skill:<name>`, which loads the
SKILL.md and runs it as a prompt with any arguments appended. The adapter
relies on this: `{#260903-pi-bridge-skill-exposure}` hands Pi the ws skills
tree through `resources_discover` "so ws skills load as native Pi skills".

The lead tool-surface reshaping (`{#260905-pi-lead-tool-surface-execute-gateway}`)
removes native `bash` and `read` from the lead's active tools via
`pi.setActiveTools(computeLeadActiveTools(...))` (`LEAD_REMOVED_TOOL_NAMES` in
`agents-plugin-pi/src/execute-gateway.ts`), leaving
`do-i-really-have-to-read-this-myself` as the only direct read. Pi rebuilds
the base system prompt from the active tool names on every
`setActiveTools`, and `buildSystemPrompt` (`dist/core/system-prompt.js`)
computes `skillFileReadTool = ["read", "bash"].find(tools.includes)`; when
that is undefined the skills block is **omitted entirely**. So on the
reshaped lead surface:

- the model never sees the skill list or any SKILL.md location;
- the "load with read" instruction is gone, and the model has no idea the
  ugly-named read tool is the way in;
- `/skill:<name>` typed by the owner still works, because Pi's slash
  command reads the file itself and injects the content as the prompt.

That is the observed behavior: the lead can run `lead-drain-ready-queue` only
when the owner types `/skill:lead-drain-ready-queue`. The `/goal` loop makes
this worse. A goal directive that names a skill re-injects only the goal
text (`buildGoalReminder`, `goal-loop.ts`); the model must load the skill
again each cycle, and cannot.

Worker and explore children are not affected: only the lead surface is
reshaped, so their prompts keep Pi's skills block and their `read` tool.
Fork sessions inherit the reshaped surface and are affected like the lead.

## Proposed direction

Adapter-only. Give the lead its own skill list and its own skill loader,
independent of Pi's `read`/`bash` presence check.

- **Skill source is Pi's own loaded set, not a ws-tree scan.**
  `pi.getCommands()` lists every skill Pi's resource loader found (user,
  project, package, and settings locations alike) as `{name: "skill:<n>",
  description, sourceInfo}` with `source: "skill"`, and `sourceInfo.path`
  is the SKILL.md path. Both the block and `ws-skill` resolve against that
  list, so the lead sees and can load exactly what Pi would have listed,
  ws skills and any other installed skill alike. No directory scan, no
  reliance on `resolveSkillsDir` existing.
- **`ws-skill` tool** on the lead surface (and forks): `ws-skill(name,
  args?)` looks `name` up in that list, reads the SKILL.md at
  `sourceInfo.path`, and returns the body (frontmatter stripped) with
  `User: <args>` appended when `args` is given, mirroring what Pi's
  `/skill:<name>` injects. Unknown name returns the available names. This is
  the model-invocable equivalent of Claude's Skill tool and of Pi's
  user-typed slash command. A skill whose frontmatter sets
  `disable-model-invocation: true` still loads by name, as Pi's slash
  command does.
- **Adapter-rendered skills block** appended to the ws system-prompt block
  (`{#260905-pi-lead-bootstrap-system-prompt}`, the `before_agent_start`
  append): the same `<available_skills>` shape Pi would have rendered (name,
  description, location), preceded by one line telling the model to load a
  skill with `ws-skill <name>` rather than `read`. Skills with
  `disable-model-invocation: true` are left out of the block, as in Pi's own
  renderer (`skills.js` `formatSkillsForPrompt`); the frontmatter is read
  from `sourceInfo.path` at block-build time. Pi's "resolve relative paths
  against the skill directory" line is dropped: ws skills reach their
  bundled assets through ws-mcp, not relative reads.
- **No `read`/`bash` gate.** The block is emitted wherever the ws
  system-prompt block is, that is for lead and fork roles
  (`computeBeforeAgentStartResult`, `isLeadOrFork`), and every lead/fork
  gets `computeLeadActiveTools`, which removes `read` and `bash`
  unconditionally, headless `--mode rpc` included. So Pi's own block is
  never present where the adapter's is, and a gate on the active-tool set
  would be always-true at best and, if evaluated before the reshape at
  `index.ts:425` (the ws block is built earlier, around `index.ts:405-415`),
  always-false, reproducing this bug. The role gate the ws block already
  has is the whole condition.
- **Lead guide row**: add `Load and follow a ws skill (lead-proceed,
  lead-drain-ready-queue, lead-write-ticket, ...)` → `ws-skill <name>` to the
  `## Verb-routing table` in `agents-plugin-pi/pi-lead-guide.md`, and note that `/goal` directives naming a skill are pursued by
  calling `ws-skill` at the start of every cycle.
- Rejected: keeping a native `read` on the lead so Pi renders its block. The
  reshaping removed it deliberately (`{#260905-pi-lead-tool-surface-execute-gateway}`),
  and the ugly-named read tool exists so that reading is a last resort, not
  the skill-loading path.
- Rejected: teaching the model to call `do-i-really-have-to-read-this-myself`
  on the SKILL.md path. It works, but the model would need the path (gone
  with Pi's block) and the tool's name is meant to discourage exactly that
  reflex.

## Spec Impact

`pi-adapter-runtime`: amend `{#260903-pi-bridge-skill-exposure}` to state
that on the reshaped lead surface Pi omits its skills block and the adapter
supplies its own list plus the `ws-skill` loader; add a `ws-skill` bullet to
the lead surface under `{#260905-pi-lead-tool-surface-execute-gateway}`; add
the skills block as a third ordered item of the ws system-prompt block under
`{#260905-pi-lead-bootstrap-system-prompt}`.

## Constraints

- Adapter-only change in `agents-plugin-pi/`; no change to ws skill text,
  which stays host-neutral.
- The skill list (names, descriptions, paths) comes from `pi.getCommands()`
  once per session start; `ws-skill` reads the body on call so an edited
  SKILL.md is picked up without restart.
- Missing or unreadable SKILL.md at a listed path: the block skips the entry
  and `ws-skill` returns an error naming the path; neither throws out of
  `before_agent_start`.
- Worker/explore surfaces are untouched; the tool and the block are lead/fork
  only.

## Phases

### Phase 1: ws-skill tool and lead skills block

Add the `ws-skill` tool, the skill list taken from `pi.getCommands()`
entries with `source: "skill"`, the adapter-rendered `<available_skills>`
block as the third ordered item of the ws system-prompt block (role-gated
like the rest of it), and the lead guide row. Tests: `ws-skill` returns a
known skill's body with frontmatter stripped; args are appended as
`User: <args>`; an unknown name lists available names; a
`disable-model-invocation: true` skill is loadable by name but absent from
the block; the block lists every `source: "skill"` entry (ws and non-ws
alike) with name, description, and location, and skips an entry whose path
is unreadable; the block is present for lead and fork and absent for
worker/explore, verified on the post-reshape tool surface (a test that
drives `before_agent_start` through the real `index.ts` ordering, not only
the pure helper); worker/explore surfaces carry neither the tool nor the
block. Amend the three spec passages under Spec Impact. Live check
(owner-run): in a Pi lead session, ask the lead to drain the ready queue
without typing `/skill:...` and confirm it calls `ws-skill
lead-drain-ready-queue` and proceeds; arm `/goal` with a directive naming
the skill and confirm each cycle starts with a `ws-skill` call.
