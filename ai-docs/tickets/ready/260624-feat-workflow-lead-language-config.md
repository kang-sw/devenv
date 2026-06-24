---
title: Configurable user conversation language for the lead agent
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260620-bug-ws-delegate-playbook-output-language-unbound: complementary fix — delegates output English; independent ordering
related-mental-model:
  - workflow-skills
  - mcp-runtime
---

# Configurable user conversation language for the lead agent

## Background

The lead agent's output language is inherited from the host session with no ws-native config
channel. Users operating in a non-English language must use host-specific workarounds (e.g.,
AGENTS.md `## Language` shims) that don't survive compaction portably.

`260620-bug-ws-delegate-playbook-output-language-unbound` addresses the complementary
problem: delegates leak non-English into English-only artifacts. This ticket addresses the
lead's output channel: a `workflow.lang` config key injects a language binding into
`lead-workflow-manual` so the lead uses the user's preferred language for final responses
while keeping CoT, mid-process output, and all subagent prompts in English.

Subagent isolation is natural: delegate subagents do not load `lead-workflow-manual`, so the
instruction never reaches them.

## Spec Impact

Target spec area: `plugin-runtime.md` (config schema — `workflow.lang` key) and
`workflow-skills.md` (lead-workflow-manual `### User preferences` — new conditional block).
Expected caller-visible change: when `workflow.lang` is set, `playbook.print(name:
"lead-workflow-manual")` output includes a language binding instruction that scopes only the
lead agent's final user-facing responses to the configured language.
Contract-first spec: no

## Phases

### Phase 1: Config key + render injection + workflow-manual template

**Goal:** Setting `workflow.lang: <lang>` in ws config causes
`lead-workflow-manual` output to instruct the lead to respond in `<lang>` while
keeping internal reasoning and subagent prompts in English.

**Changes:**

1. Go config schema — add `workflow.lang` string field to the layered config
   struct. Support global and project scope; global is the natural user-level
   preference; project scope allows per-repo language overrides following the
   existing `session > project > global > builtin` resolution order.

2. Go playbook render path — when `playbook.print(name: "lead-workflow-manual")`
   is called with a valid session key, resolve `workflow.lang` from the session's
   layered config and inject it as a `.WorkflowLang` template variable. Follow the
   same session-key-based resolution pattern as existing tier vars; no new render
   pipeline is needed.

3. Playbook template — add a conditional block to the empty `### User preferences`
   section in `lead-workflow-manual.md`:

   ```
   {{- if .WorkflowLang }}
   User conversation language: '{{.WorkflowLang}}'. Keep internal reasoning and
   all subagent prompts in English; final user-facing responses must be in
   '{{.WorkflowLang}}'.
   {{- end }}
   ```

**Constraints:**
- No effect when `workflow.lang` is unset or empty; `### User preferences`
  stays empty.
- Subagent isolation holds by design: delegates never load `lead-workflow-manual`.
- The language instruction must not propagate to `playbook.render` delegate
  outputs; `render` targets implementer/reviewer/etc. prompts, not the
  workflow-manual, so isolation is already structural.

**Rejected alternatives:**
- Host system-prompt injection (AGENTS.md): host-specific, not portable.
- Global `lang` key without namespace: risks collision with future non-workflow
  language settings; `workflow.lang` namespace is explicit.

**Verification:**
- Set `workflow.lang: ko` in a test config; call `playbook.print(name:
  "lead-workflow-manual")` with a session key bound to that config; assert output
  contains the Korean language instruction in `### User preferences`.
- Leave `workflow.lang` unset; assert `### User preferences` section is empty.
- Call `playbook.render` on an implementer delegate with the same session key;
  assert the rendered output does NOT contain the language instruction.
