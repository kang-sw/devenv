---
title: prompt override surface gaps — no unset path, and required harness
related-mental-model:
  - prompt-bundle
---

# prompt override surface gaps — no unset path, and required harness

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

A second, related gap surfaced in the same dogfood pass: `config.prompt.set`
requires `harness` (enum `claude`/`codex`/`*`), and `lead-tune` steers the caller
to pass `*` by default ("use `*` unless the user names one host"). But `*` means
*all hosts*, so an implicit-broad default gives a cross-host blast radius when a
user almost always means "the host I'm on." The sibling tool `config.agents_tier`
already models the conservative behavior: its `harness` is optional and, when
omitted, resolves to the detected MCP session harness. The prompt surface is
inconsistent with its own family.

## Resolution model (verified)

Override resolution precedence was confirmed empirically by setting both a `*`
and a `claude` bucket on `DelegationSection` and rendering under a Claude
session: the `claude` bucket won. So buckets are independent and layered:

> host-specific bucket (`claude`/`codex`) > `*` (all-hosts) bucket > shipped seed

Because buckets are independent, an unset must target one concrete bucket, and an
omitted harness can safely resolve to a single concrete bucket (the current host)
on both set and unset.

## Direction

Two coupled changes to the prompt-override surface; keep both lead-only behind
the `config.*` prefix gate.

**1. Add an unset / revert-to-seed path.** Likely a
`config.prompt.unset(session_key, pointId, harness?, scope?)` that removes the
stored override for the matching `(pointId, harness, scope)` so the point falls
back to the next layer (host-specific → `*` → seed). `config.prompt` must stop
listing the point as overridden once its last bucket is cleared. Decide whether
unset clears just the named scope entry or cascades; default to per-scope
(symmetry with how set writes one scope).

**2. Make `harness` optional with a current-host default — symmetric across
set and unset.**

- Omitted `harness` → the detected current session harness (matching
  `config.agents_tier`), not `*`. Conservative blast radius; matches the common
  "tune my current host" intent.
- `*` stays a deliberate, explicit value meaning "all hosts" — the fallback
  bucket that host-specific overrides shadow.
- set(omit) writes the current-host bucket; unset(omit) clears the current-host
  bucket. Never let an omitted harness on unset mean "wildcard-delete every
  bucket for this point" — that would be the footgun the symmetry avoids.
- Adding optionality to the already-shipped `config.prompt.set` is
  backward-compatible (existing explicit callers are unaffected).

This flips the `lead-tune` doctrine: omit harness for the common current-host
case, and pass `*` explicitly only for all-hosts tuning; teach the unset/revert
path alongside set. The skill/convention edits run under `lead-skill-authoring`.

Relates to `260611-research-ws-per-role-delegation-tuning-config` (the broader
per-role tuning-config surface) but is narrower: these are the missing inverse
and a default correction for an already-shipped write, not new tuning axes.
