---
name: exit-session
description: >
  Seal a work session by compressing the current conversation's
  working memory into ai-docs/_continue.local.md so the next
  session's enter-session can fast-path the Briefing from the
  payload. Invoke when the user signals wrap-up ("let's wrap this
  session", "session wrap-up", "I'll continue next session"). Sources
  are conversation-only — no scans of `wip/`, tickets, logs, or
  filesystem beyond the git-status check required for auto-commit.
  Auto-commits dirty tracked state as WIP before sealing so the
  payload's HEAD anchor is meaningful.
---

# Exit Session

## Invariants

- The payload is a compression of the current conversation's working memory — what the owner is holding in attention right now. No filesystem exploration, no scans of `wip/`, no ticket-body reads, no `git log`, nothing beyond the `git status --porcelain` required for the auto-commit step. If it wasn't in this conversation, it does not belong in the payload; surfacing orphan tickets or latent threads is enter-session's job, not this one's.
- The payload header is produced by `seal.sh`, not composed by the agent. Format is exactly `<!-- HEAD: <sha7> · Written: <ISO> -->` — a minimal script-only contract. HEAD is the fast-path gate for enter-session's dispatcher; Written drives elapsed-time flavor at resume. Branch, active ticket stem, and any other contextual fields live in the body prose where the reading agent extracts them naturally. The agent copies the header verbatim from its Read of the staged stub into the final Write — never types it from scratch.
- Overwrite `ai-docs/_continue.local.md` — never append, never merge with prior content.
- Auto-commit dirty tracked state before writing the payload so the captured HEAD SHA reflects the full state. Use `git add -u` — never `git add .`.
- Untracked files (`??` in `git status --porcelain`) are reported in the final summary but never auto-staged, to avoid committing secrets or unintended artifacts.
- No edits to `_index.md`, `_index.local.md`, or ticket files. Canonical sources are updated only by their own dedicated skills.
- If every content section of the payload would be empty, skip writing the file entirely and report "nothing to capture."
- All output in English regardless of conversation language.

## On: invoke

1. Run `git status --porcelain`. Classify entries into tracked-dirty (any non-`??` line) and untracked (`??` lines).
2. If tracked-dirty is non-empty, auto-commit as WIP:
   - Run `git diff --stat HEAD` to see scope.
   - Compose commit message `wip(<scope>): <one-line session intent>` from session memory, with an `## AI Context` body noting the auto-commit by `/exit-session`.
   - `git add -u` to stage tracked changes only.
   - Commit.
3. Decide whether there is anything to capture by mentally composing the payload body from the current conversation — what the owner actually held in attention during this session. The sections are Mental state, Next concrete step, Open threads, User directives pending (see the **Payload template** below). Do not scan the filesystem, read tickets, or run git commands to pad content; padding with scanned observations corrupts the compression contract. If every section would be empty, stop here and report "nothing to capture, continuation file not written" — do not run `seal.sh`, do not touch the file.
4. Run `bash <skill-dir>/seal.sh ai-docs/_continue.local.md` to stage the mechanical header, where `<skill-dir>` is this skill's base directory shown at the top of this skill invocation. The script creates or overwrites the file with exactly one line: `<!-- HEAD: <sha7> · Written: <ISO> -->`.
5. Read `ai-docs/_continue.local.md` via the Read tool. This loads only the single-line header stub just staged — no stale prior-session content — and satisfies the Write tool's read-before-write requirement for the next step.
6. Write `ai-docs/_continue.local.md` via the Write tool. The content is the exact header line from step 5 (copied verbatim, do not retype), a blank line, then the composed body sections from step 3.
7. Report: WIP commit SHA if created, untracked files left alone, payload sections written, and "session sealed."

## Payload template

The final file is the header line from `seal.sh` (copied verbatim from the Read in step 5), a blank line, then the body sections below.

```
<!-- HEAD: <sha7> · Written: <ISO> -->

## Mental state
<1-3 bullets: what the owner is currently holding in attention — tentative reasoning that has not crystallized into commits or tickets. If an active ticket stem matters for resume, name it here. Omit section if nothing to capture.>

## Next concrete step
<single line: the exact next action on resumption. Omit if unclear.>

## Open threads
<bullets: things noticed but not actioned during this conversation. Omit section if empty.>

## User directives pending
<bullets: user instructions issued but not yet acted on. Omit section if empty.>
```

## Doctrine

The skill optimizes for **owner context conservation** — the same finite resource enter-session targets. exit-session moves the next bootstrap's cost from "clerk re-synthesizes committed state while the prior session's mental state is lost to the session boundary" to "owner reads a pre-digested payload covering the slice only this session's owner can produce." The single control is **conversation-only sourcing**: the payload is a compression of what was actually said and decided in this session, nothing more. Scanning the filesystem at seal time to find "things the owner might also want to know" fabricates context rather than preserving it — that widening of scope belongs to enter-session's clerk fork or an explicit post-bootstrap user request, never here. When a rule is ambiguous, apply whichever interpretation keeps the payload a faithful compression of the conversation's working memory.
