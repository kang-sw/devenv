---
title: prompt override surface has no unset/revert-to-seed path
related-mental-model:
  - prompt-bundle
---

# prompt override surface has no unset/revert-to-seed path

## Background

Found while dogfooding the recently-landed prompt-replacement surface
(`config.prompt` / `config.prompt.set`, resolved at `playbook.print` and
`playbook.render`). Setting an override works end-to-end and replaces the seed
block for the matching `(pointId, harness)`. But the surface offers no inverse:

- `config.prompt.set` requires a non-empty `prompt` (schema: "Must be
  non-empty"), so it cannot clear an override by writing empty text.
- The `config.*` tool surface exposes `config.show`, `config.prompt`,
  `config.prompt.set`, and `config.agents_tier` — no `unset`/`clear`/`remove`.
- `lead-tune` documents only the set path; it never tells the user how to revert
  a knob to its shipped default.

Consequence by scope:

- `session` scope is low-harm: the override lives in the session key file
  (`keys/<key>.json` `overrides` map) and is irrelevant once that key is unused.
- `project`/`global` scope is sticky. A user who sets an override can never
  return that point to its shipped seed through the surface. Overwriting with the
  original seed text is not a true revert either: `config.prompt` still reports
  the point as overridden (an override whose text merely equals the seed), and
  the user would have to hand-copy the seed block out of the shipped playbook
  between its `ws:override` markers to do even that.

This is a set-with-no-inverse asymmetry in a write surface — the kind of footgun
that strands a project/global config in a state the tools can't undo.

## Direction

Add a revert path to the prompt-override surface. Likely shapes:

- A `config.prompt.unset(session_key, pointId, harness, scope)` that removes the
  stored override for the matching key so the point falls back to the next scope
  (and ultimately the shipped seed).
- Or let `config.prompt.set` accept an explicit clear sentinel (an empty-string
  write meaning "remove this override"), guarded so it cannot be confused with a
  genuine empty-extension override.

Either way: keep it lead-only behind the `config.*` prefix gate like the set
path, make `config.prompt` stop listing the point as overridden once cleared,
and teach the revert path in `lead-tune` alongside set. Decide whether unset is
per-scope (clear just the session/project/global entry) or cascading.

Relates to `260611-research-ws-per-role-delegation-tuning-config` (the broader
per-role tuning-config surface) but is narrower: the gap is the missing inverse
of an already-shipped write, not new tuning axes.
