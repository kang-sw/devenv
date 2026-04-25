---
title: "ws-call-agent: Named Agent Registry + Auto-Compression"
spec:
  - 260424-ws-call-agent
spec-remove:
  - 260424-ws-agent
completed: 2026-04-25
related-mental-model:
  - workflow-routing
  - executor-wrapup
---

# ws-call-agent: Named Agent Registry + Auto-Compression

## Background

`ws-call-agent` currently addresses agent sessions via a deterministic UUID computed from
`repo + branch + agent-name`. This prevents refreshing an agent session independently of the
name — a requirement for auto-compression. When an agent's context exceeds ~100K tokens,
Claude's native auto-compression may trigger mid-task near 150K and cause catastrophic context
loss. A controlled compression-and-handoff mechanism prevents this at a conservative threshold.

## Decisions

- **Stored UUID over deterministic UUID.** Deterministic UUID cannot be refreshed without
  changing the agent name or branch. Stored UUID enables compression-triggered refresh while
  preserving the logical agent name.
- **100K compression threshold.** Conservative safety margin before Claude's native ~150K
  auto-compression. Actual trigger per cycle may be 110–130K due to check lag; this is
  intentional.
- **3-call compression flow.** (1) Haiku extracts original-prompt intent with a hardcoded
  "summarize intent, do not act" system-prompt; (2) existing agent produces a compression doc
  via agent-compression.md injection; (3) new agent receives compression doc + intent summary +
  original prompt.
- **compressed_at guard.** Set on the new agent.json after compression; cleared before the
  first subsequent call. Prevents re-compression when the handoff document itself is large.
- **ws-agent removed.** Its only caller was ws-call-agent. UUID generation moves into
  ws-new-agent; deterministic generation is no longer needed.
- **Haiku for intent extraction only.** Haiku is used for step (1) of compression — extracting
  the original prompt's intent in 2–3 sentences. The actual session compression (step 2) runs
  against the existing agent using its configured model.
- **agent-compression.md as user turn.** Injected as the next user turn to the existing agent,
  not as a system-prompt override. The agent's original system-prompt (role document) is
  maintained; the compression instruction leads with "Ignore your current task" to override.

## Phases

### Phase 1: agent-compression.md infra doc

Create `claude/infra/agent-compression.md`.

Purpose: a prompt injected as a user turn into the existing agent to trigger session
compression. The agent produces a structured handoff document from its current context.

Content structure:
- Lead: "Ignore your current task. Do not read any new files."
- Output sections the agent must produce:
  - Original purpose and action plan (as understood at session start)
  - Work summary — 1–2 lines per completed item
  - Skills for the next agent: `[Must]` / `[Maybe]` level entries
  - Docs for the next agent: `[Must]` / `[Maybe]` level entries
  - Execution log — concrete actions and findings relevant to the forwarded intent

The haiku intent-extraction framing is NOT in this document — it is hardcoded in
`ws-call-agent`.

**Success criteria:** The document, when injected into a live agent session, produces a
self-contained handoff that a fresh agent can use without reading any additional files.

### Result (7a58ed3) - 2026-04-25

Created `claude/infra/agent-compression.md`. Five-section structure: original purpose,
work summary, skills, docs, execution log. Leads with "Ignore your current task. Do not
read any new files." No haiku framing included (hardcoded in ws-call-agent per design).

### Phase 2: ws-new-agent script

Create `claude/bin/ws-new-agent`.

Signature:
```
ws-new-agent <agent-name>
  [--agent <type>]          # forwarded to claude CLI (Explore, general-purpose, etc.)
  [--system-prompt <path>]  # file read at registration time; content stored in JSON
  [--model <opus|sonnet|haiku>]
```

Writes `.git/ws@<repo-dir-name>/agents/<agent-name>.json` (directory created if absent).
The `<repo-dir-name>` is the basename of the git repo root.

JSON schema:
```json
{
  "uuid": "<random-uuid-v4>",
  "model": "sonnet",
  "agent_type": "",
  "system_prompt": "<file-content-or-empty>",
  "token_count": 0,
  "compressed_at": false
}
```

UUID is a fresh random UUID (not deterministic). If the JSON file already exists, the command
overwrites it (caller is responsible for not clobbering a live session).

**Success criteria:** File written to correct path; claude session file detection works for
the written UUID on subsequent `ws-call-agent` calls.

### Result (7a58ed3) - 2026-04-25

Created `claude/bin/ws-new-agent`. Uses Python for JSON generation to handle multi-line
system_prompt safely. REPO_DIR_NAME uses `basename` of repo root. Smoke test confirmed:
correct JSON schema written, "Agent registered" output.

### Phase 3: ws-call-agent rewrite

Replace `claude/bin/ws-call-agent`. New signature:

```
ws-call-agent <agent-name> <prompt>
```

Behavior:

1. Locate agent.json at `.git/ws@<repo-dir-name>/agents/<agent-name>.json`. If absent, print
   a clear error (`agent '<name>' not found — run ws-new-agent first`) and exit 1.
2. Read `uuid`, `model`, `agent_type`, `system_prompt`, `token_count`, `compressed_at` from
   JSON.
3. If `compressed_at == true`: clear the flag (write JSON back) before proceeding. Skip
   compression check for this call only.
4. Build claude args: `--model <model>`, `--output-format json`, `--dangerously-skip-permissions`.
   If `agent_type` is non-empty: `--agent <agent_type>`.
   If `system_prompt` is non-empty: `--system-prompt "<system_prompt>"`.
   Auto-route session: `--resume <uuid>` if `~/.claude/projects/<escaped>/<uuid>.jsonl` exists,
   `--session-id <uuid>` otherwise.
5. Run claude, capture JSON output to temp file.
6. Compute new `token_count = input_tokens + cache_creation_input_tokens` from JSON. Write
   back to agent.json.
7. Print to stderr: context fill percentage when `token_count > 50000`.
8. If `token_count > 100000` and `compressed_at == false` (after step 3 clear): run
   compression flow:
   - **Step a — intent extraction (haiku):**
     `claude -p --model haiku --system-prompt "Summarize the user's intent in 2-3 sentences. Do not perform the task." "<original-prompt>"` → `$intent_summary`
   - **Step b — session compression:**
     Inject `$(cat $(ws-infra-path agent-compression.md))` as next user turn to the existing
     agent (same args as step 4 but forced `--resume <uuid>`). Capture result as
     `$compression_doc`.
   - **Step c — new agent:**
     `ws-new-agent <agent-name> --model <model>` (plus `--agent` and `--system-prompt` if
     set; re-read from pre-compression agent.json values).
     Set `compressed_at=true` in the newly written agent.json.
   - **Step d — handoff call:**
     Send `$compression_doc + "\n\n---\n\n" + $intent_summary + "\n\n---\n\n" + $original_prompt`
     as first prompt to the new agent. Output this result to stdout.
   - Exit after compression flow (do not also print step 5 result).
9. Output `jq -r '.result'` from step 5 (or step d on compression). Exit 1 if
   `is_error == true`.

**Rejected alternative — ws-call-agent calls itself recursively after compression.** Avoided
because it re-enters compression-check logic on the handoff call; the compressed_at guard
already handles this, but recursive invocation makes the control flow harder to reason about.

**Success criteria:**
- Compression does not trigger on the first call of a newly-compressed agent.
- New agent after compression uses the same `model`, `agent_type`, `system_prompt` as the
  original.
- `ws-call-agent nonexistent "prompt"` fails with a clear message.

### Result (c374a31) - 2026-04-25

Rewrote `claude/bin/ws-call-agent`. Key deviations from plan:
- TMPFILE created before CLEANUP_FILES array (not after) to avoid `set -u` empty-array error.
- CONTEXT_LIMIT set to 200K (full Claude window) rather than leaving unspecified.
All three acceptance criteria confirmed via smoke tests.

### Phase 4: ws-agent removal + spec update

1. Delete `claude/bin/ws-agent`.
2. Update `workflow-skills.md` spec entries via `/write-spec`:
   - Replace `{#260424-ws-call-agent}` with updated interface (new signature, registry-based,
     compression behavior).
   - Replace `{#260424-ws-agent}` with `{#260425-ws-new-agent}` entry.
   - Add new stem for named agent registry and compression behavior.
3. Update any skill or agent doc that references `ws-agent` or the old `ws-call-agent <model>`
   interface.

**Success criteria:** No references to the old `ws-call-agent <model> [--agent ...]` signature
remain in docs or skill files. `ws-agent` is deleted with no dead references.

### Result (96640f4) - 2026-04-25

Deleted `claude/bin/ws-agent`. Updated ws-orchestration.md (full rewrite), implement/SKILL.md,
sprint/SKILL.md (delegation cycle split into two steps: register + allocate-paths), and
ws-infra-path example in workflow-skills.md spec. grep confirmed zero stale references.
ws-declare-agent preserved — still useful for orphaned session cleanup without re-registration.
