---
title: "git.merge: warn when a merge grows a source file past a size limit"
sage-review-design: required
---

# git.merge: warn when a merge grows a source file past a size limit

## Background

Owner idea, 2026-11-13. Agents grow source files without bound (for example
`dashboard.js` at ~6.9k lines). Agents also restructure aggressively when they
assume they own the project, so the warning must route the refactor decision
to the user, not trigger it.

The `git.merge` handler (lead-owned) lives in
`agents-plugin-tool/internal/mcp/git_merge.go`.

## Behavior

`git.merge` result carries warnings computed over merge-base..branch, for
files with known source extensions:

- **Soft threshold N**: a file that was <= N lines at the merge base and is
  > N after the merge ->
  "Warning: this change pushed <file> past N lines (now L); splitting it is
  recommended."
- **Hard threshold M**: any file >= M lines that the branch edited ->
  "Warning: edited <file> at L lines (>= M); splitting it is recommended."
- Every warning text tells the reader (the lead): do not split on your own;
  ask the user whether refactoring this project is permitted and whether to do
  it now.

## Constraints

- T0 additive: new optional result field/text plus new config keys in both the
  `agents-plugin` and `agents-plugin-wsflow` config manifests; the wsflow
  mirroring manual (`ai-docs/manuals/wsflow-mirroring.md`) applies.
- The warning text ships to downstream agents: read
  `ai-docs/manuals/shipped-surface-boundary.md`.
- `ai-docs/manuals/release-compatibility.md` applies.

## Decisions

- (424)=a: warnings appear only in the `git.merge` result. Rejected:
  `git.commit` / edit hooks (deep workers receive them but cannot ask the user
  and may refactor unilaterally); ticket ready-promotion pre-check (owner:
  injects refactoring into the discussion context and lowers ticket quality);
  `review.stamp` / lead-review (owner: too many changed files per range to be
  useful).
- (425)=b: a file the user declines to split is recorded and not warned again
  (unless it grows past a recorded limit). Rejected: ask every time (warning
  fatigue on files like `dashboard.js`); a single project-wide
  refactor-permission setting.
- (426): N and M are config keys (defaults N=1000, M=2000; 0 disables each);
  the extension list is configurable; files marked generated or vendored in
  `.gitattributes` (`linguist-generated` / `linguist-vendored`) are excluded.
  Rejected: hard-coded values. Owner note 2026-11-13: N, M and the extension list
  are ordinary config keys that follow the existing config layers (manifest
  default, then project/user overrides) and are tunable through lead-tune like
  any other knob; no separate mechanism.
- (427): the warning text explicitly instructs the lead not to split on its
  own and to ask the user about refactor permission and timing. Rejected: a
  bare "splitting recommended".
- Compatibility tier: T0 (additive).
- (428): the suppression list lives in .gitattributes as a `ws-size-guard` attribute (`-ws-size-guard` disables the warning for matching paths; `ws-size-guard=<lines>` overrides the limit for them). Rejected: a new `.wsignored` file (owner's initial suggestion) - .gitattributes already gives gitignore-style per-directory patterns evaluated by `git check-attr`, and holds the generated/vendored exclusion. A per-path limit value gives (425) b's "warn again only after notable growth".
