<!-- Fragment concatenated below SKILL.md by dispatch.sh on the resume path.
     Universal invariants live in SKILL.md above the splitter; only
     resume-path-specific invariants and handler steps live here. -->

## Invariants

- The Continuation payload appended below by the dispatcher is authoritative — the dispatcher pre-gated this path on HEAD SHA match, so the payload reflects the current commit state.
- Do not spawn clerk. Do not rescan `git log`, `git diff`, or ticket bodies. The payload is the sole recent-work source.
- Read `ai-docs/_index.md` directly; it is small and mandated.
- Never delete or modify `ai-docs/_continue.local.md` — it is consumed non-destructively so re-invocation is idempotent.

## On: invoke

1. Read `ai-docs/_index.md` directly.
2. Parse the **Continuation payload** block appended to this skill by the dispatcher. The header (`<!-- HEAD: ... · Written: ... -->`) is a script contract — only `HEAD` and `Written` live there, and the dispatcher has already used them for gating and the elapsed-time flavor line. All contextual fields (Branch, active ticket stem, purpose) are inferred from the body sections: `Mental state`, `Next concrete step`, `Open threads`, `User directives pending`.
3. Consult the **Workflow Map** below against the payload's `Next concrete step` to pick the next skill. Apply `judge: scope-complexity` and `judge: parallelizable` if the mapping is ambiguous.
4. Emit the **Briefing** template, mapping payload fields:
   - `Branch` — read current branch from `git rev-parse --abbrev-ref HEAD` (one grep-equivalent call, allowed under delegation posture). The payload no longer carries it.
   - `Recent work` — 1-3 bullets paraphrased from `Mental state`.
   - `Active` — active ticket stem extracted from `Mental state` / `Next concrete step` prose if present; otherwise "none". Purpose drawn from the same prose.
   - `Open threads` — merge `Open threads` and `User directives pending`.
   - `Queue` — omit in resume path.
   - `Recommended next` — skill chosen from Workflow Map, reason copied from `Next concrete step`.
5. Stop. Do not proceed into the recommended next step — the user decides.
