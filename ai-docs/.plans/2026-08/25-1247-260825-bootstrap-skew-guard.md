# Plan: 260825-refactor-ws-wsflow-bootstrap-artifact-convergence — Phase 3: Fail-loud version-skew guard (above-head / unknown tag)

## Relevant Ticket Contract

- Guard a tag **above the running package's own template head**, or a tag
  absent from its known lineage / unparseable, across two surfaces: (1)
  detect/warn (Go staleness banner) and (2) refuse (agent-run
  `lead-bootstrap` skill instruction to stop rather than reconcile/restamp).
- **Honest enforcement contract (binding):** bootstrap reconcile is
  agent-executed; no Go code performs it. The refuse is a skill-level
  instruction backed by the code-level warning, **not a mechanical
  hard-block** — every new instruction/comment must state this limit, never
  imply a code-enforced block.
- Direction semantics (from the ticket's Decisions section, binding):
  - `project tag < package head` → normal upgrade → **allowed**, unchanged.
  - `project tag == package head` → current → no action, unchanged.
  - `project tag > package head` → **above-head** → fire guard (warn + refuse).
  - tag unknown/unparseable/not-in-lineage → fire guard.
  - The comparison is against the **running package's own** template head
    (well-defined as `v0047` for both packages after Phase 2).
- Depends on Phase 2 (own template head is well-defined against the shared
  counter) — already landed on this branch (`ee4bc6a5`).
- Verification boundary (ticket text): the extended staleness detection is
  unit-tested on an above-head tag; wsflow opening a shared-head ws project
  surfaces the warning and the skill leaves artifact + tag unchanged; ws
  opening a below-head project proceeds to a clean re-stamp.
- Spec deferral (binding): updating `mcp-tools.md
  {#260703-bootstrap-staleness-warning}` is Phase 4-owned. This ticket merges
  as one unit after Phase 4, so `develop` never sees the intra-branch doc
  drift. Phase 3 implements + unit-tests code and skill instruction only.

## Out of Scope

- Phase 4: test inversion of `test_bootstrap_template_uses_wsflow_local_version_lineage`,
  spec anchor updates (`mcp-tools.md #260703`, `workflow-skills.md #260513`),
  `wsflow-mirroring.md` Bootstrap Template Rules inversion, the `260728`
  Non-Scope override note.
- Phase 1/2 content: the converged `AGENTS.template.md`/`WORKFLOW.md` bodies
  and the unified `v0047` migration counter — do not touch template bodies or
  version tags.
- Any mechanical/code-level hard-block of the reconcile/restamp step — the
  refuse is skill-instruction-only, backed by the warning, per the honest
  enforcement contract.
- `mcp-tools.md #260703` spec text update (explicitly Phase 4-owned; the
  fit reviewer should read this as intentional deferral, not missing-spec
  drift).

## Codebase Findings

- `agents-plugin-tool/internal/mcp/bootstrap_alarm.go#L53-L86` — `bootstrapStalenessWarning(root, skillsRoot string, resolver *wsconfig.Resolver, sessionKey string) string`. Today: reads installed tag via `readTemplateVersion(filepath.Join(root, "AGENTS.md"))` (returns `(0, false)` uniformly for "no file", "no tag", and "malformed tag" — cannot currently distinguish "never opted in" from "opted in with a broken tag"); reads `latest` via `latestKnownTemplateVersion(skillsRoot)` (already package-scoped — no branching needed, confirmed both packages' shipped `AGENTS.template.md` carry `v0047`); early-returns `""` when `installed >= latest`. This early-return is exactly the branch that must split: `installed == latest` stays silent, `installed > latest` must now fire.
- `agents-plugin-tool/internal/mcp/bootstrap_alarm.go#L12-L32` — `templateVersionTag` regex (`<!--\s*Template Version:\s*v(\d+)\s*-->`) and `parseTemplateVersionTag`. The regex is strict: a marker-like comment that doesn't match this exact digit format (e.g. hand-edited, truncated, or non-numeric) is indistinguishable today from "no marker at all" — both yield `(0, false)`. Implementing the "unknown/unparseable" fire case requires a second, looser marker-presence check (e.g. `<!--\s*Template Version:` without the value capture) to split "marker absent" (must stay silent — untagged project never opted in, per the existing invariant at L58-60) from "marker present, value unparseable" (must now fire).
- `agents-plugin-tool/internal/mcp/bootstrap_alarm_test.go#L129-L166` — **direct behavioral conflict, must be updated as part of this phase, not treated as a regression.** `TestBootstrapStalenessWarningSilentWhenUpToDate` has a subtest literally named `"downstream ahead of latest"` (root `v0003`, template `v0002`) that asserts the warning **stays silent**. This is the exact above-head case Phase 3 must invert to fire. Confirmed via `go test ./internal/mcp/... -run TestBootstrapStalenessWarning -v`: currently 5/5 green, including this subtest. This subtest must be removed from `TestBootstrapStalenessWarningSilentWhenUpToDate` (keep only the `"equal versions"` subtest there) and replaced by a new fire-assertion test.
- `agents-plugin-tool/internal/mcp/bootstrap_alarm.go#L88-L96` — `injectBootstrapStalenessWarning` (prepend helper) and its two callers (`agents-plugin-tool/internal/mcp/server.go#L1717` for `ferrule`, `agents-plugin-tool/internal/mcp/workflow_manual.go#L279-L321` for `workflow_manual` FRESH-with-root and CONTINUE branches) need **no signature or call-site changes** — the function shape and both call sites already pass whatever string `bootstrapStalenessWarning` returns; only the returned string's content/conditions change.
- `agents-plugin-tool/internal/mcp/bootstrap_alarm.go` — `readTemplateVersion` is used at exactly two sites: `latestKnownTemplateVersion` (package's own head — leave untouched, still wants strict-parse-or-fail-safe) and the installed-tag read inside `bootstrapStalenessWarning` (must change to also detect marker-present-but-unparseable). No other caller in the package (confirmed via `grep -rn "readTemplateVersion\b"`).
- `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md#L27-L32` — `## On: invoke` step 4 mode detection: `fresh` / `upgrade` (any tag present) / `adopt` (no tag) / `claude-migrate`. There is currently **no mode boundary** for above-head or unparseable tags — they fall into the same `upgrade` bucket as below-head tags, which is exactly the "currently unspecified... masked only by the executing agent's ad-hoc judgment" gap the ticket's Background names. This is the shared playbook (confirmed via `diff` that `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md` and `agents-plugin-wsflow/rsrc/lead-bootstrap/lead-bootstrap.md` are currently byte-identical — the wsflow copy is a **generated mirror**, never hand-edited).
- `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md#L48-L58` — `## On: upgrade` steps: step 1 parses current version, step 2 walks checklist items `version > current` (empty walk when current is already above every checklist entry — this is the silent-corruption path the ticket's Background describes), step 4 "Update `AGENTS.md` template-managed sections from `AGENTS.template.md`" and step 8 "Update the template version tag" are the two steps that must never execute for an above-head/unparseable tag.
- `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md#L9-L20` (`## Invariants`) — good injection point for the honest-enforcement statement (skill-level instruction backed by the code-level warning, not a mechanical block), consistent with how the existing invariants are phrased (short, declarative, one per bullet).
- `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md#L46,94-103` — existing precedent for package-neutral templating: skill-name references use `{{.SkillNamespace}}:...`, never a literal `ws:`/`wsflow:` token. The new refuse instruction needs no skill-name reference at all (it only compares the on-disk tag against "this skill directory's own `AGENTS.template.md` tag", already read at step 1 of `## On: invoke`), so no templating is needed, but any new instruction text must still avoid inventing a literal `ws`/`wsflow` token if one were added later.
- `ai-docs/manuals/wsflow-mirroring.md#L244-L275` (**Rsrc Tree Provisioning**) — `agents-plugin-wsflow/rsrc/` is a **generated byte-identical copy** of `agents-plugin/rsrc/`; edit only the canonical file, never `agents-plugin-wsflow/rsrc/lead-bootstrap/lead-bootstrap.md` by hand. After-edit checklist (must run both, in order):
  1. `WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest` — confirmed necessary: `agents-plugin/rsrc/manifest.json:20` stores a SHA-256 content hash for `lead-bootstrap/lead-bootstrap.md`, so any edit changes that hash.
  2. `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror` — syncs `agents-plugin-wsflow/rsrc/` byte-for-byte from canonical; drift is caught by `TestWsflowRsrcMirrorUpToDate` (`agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L54-L81`).
  Both `-count=1` flags are mandatory (env-gated test bodies, Go test cache can return a stale green `ok` without running the write side effect).
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L1-L13,120-129` — the forbidden-pattern scan (`test_skill_files_do_not_reference_full_ws_agent_surface`) walks only `SKILLS_DIR = agents-plugin-wsflow/skills`, **not** `agents-plugin-wsflow/rsrc/`. Editing only the shared `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md` (and its generated mirror) is therefore outside this test's scanned surface — confirmed by reading the test source directly, not inferred. No `agents-plugin-wsflow/skills/lead-bootstrap/*` file needs editing for this phase (its `SKILL.md` is already a thin `playbook.print` shim; `AGENTS.template.md`/`WORKFLOW.md` are Phase 1/2 territory, out of scope here).
- `ai-docs/spec/mcp-tools.md#L672-L689` (`{#260703-bootstrap-staleness-warning}`) — current spec text describes only the behind-direction warning ("carries a ... tag behind the version shipped"). Confirmed this anchor is the one the ticket's Spec Impact section defers to Phase 4; do not edit it in this phase.

### Risk signal: marker-presence heuristic is an implementation-level judgment call

The ticket says "unknown/unparseable tag not in the package's known lineage"
but does not spell out how the code should tell "tag genuinely absent" (must
stay silent, existing invariant) apart from "tag present but malformed" (must
now fire). The plan above resolves this with a second, looser regex that
detects the `<!-- Template Version:` marker prefix independent of whether the
value parses — a narrow, mechanically bounded extension of the existing
`readTemplateVersion` collapse-into-one-bucket behavior, not a new product
policy. This is flagged for reviewer attention but does not block
implementation: it is testable, low blast radius (one function, two call
sites unaffected), and consistent with the stated invariant ("untagged
project never opted in" stays true only for the *fully absent* case).

## Implementation Plan

1. `agents-plugin-tool/internal/mcp/bootstrap_alarm.go` — restructure `bootstrapStalenessWarning`:
   - Add a second regex, e.g. `templateVersionMarker = regexp.MustCompile(`<!--\s*Template Version:`)`, to detect marker presence independent of strict parse success.
   - Replace the `installed, ok := readTemplateVersion(...)` line with a direct read (`os.ReadFile` + `parseTemplateVersionTag`) that also computes `markerPresent` (`parsed || templateVersionMarker.MatchString(content)`), or add a small local helper (e.g. `readInstalledVersionState(path) (version int, parsed, markerPresent bool)`) next to the existing helpers. Keep `readTemplateVersion`/`latestKnownTemplateVersion` unchanged (still used for the package's own head at L49-51).
   - Keep the existing `bootstrap_alarm off` short-circuit and the "no AGENTS.md at all" silent case first (unchanged order).
   - After the marker-presence check (return `""` when no marker at all — preserves the "never opted in" invariant) and the existing "`latest` unreadable → fail-safe silent" check, branch three ways instead of the current single `installed >= latest` early return:
     - `!parsed` (marker present, value unparseable) → new "unknown tag" warning.
     - `installed > latest` → new "above-head" warning.
     - `installed < latest` → existing "Bootstrap template is stale" message, **text unchanged**.
     - `installed == latest` → `""` (unchanged).
   - Both new messages must (a) name the relevant version number(s), (b) point to `config.tune(key: "bootstrap_alarm", value: "off")` (matches existing test-checked pattern), and (c) explicitly state the honest-enforcement limit — e.g. "this is a detector only; lead-bootstrap must stop and report, not auto-fix" — never phrase it as a code-enforced block. Draft:
     - Above-head: `"> **Bootstrap template tag is ahead of this package's own template head.** This project's AGENTS.md is at v%04d; this package's shipped lead-bootstrap template head is v%04d. Do not reconcile or restamp — this check is a code-level detector only; lead-bootstrap must stop and report rather than auto-fix. Leave the artifact and tag unchanged, or run `config.tune(key: \"bootstrap_alarm\", value: \"off\")` to silence this permanently."`
     - Unknown/unparseable: same shape, opening with `"> **Bootstrap template tag is unrecognized.**"` and describing the marker as present but not parseable as `vNNNN`, omitting the installed-version number (it has none) but still naming the package's own head.
2. `agents-plugin-tool/internal/mcp/bootstrap_alarm_test.go`:
   - In `TestBootstrapStalenessWarningSilentWhenUpToDate` (L134-166), remove the `"downstream ahead of latest"` subtest (root `v0003` vs template `v0002`) — this exact fixture must now assert **fire**, not silence. Keep the `"equal versions"` subtest as-is.
   - Add a new test, e.g. `TestBootstrapStalenessWarningFiresOnAboveHeadTag`, reusing the `writeTemplateVersionFixture`/`callLogin`/`toolText` helpers already in this file: root `AGENTS.md` tag `v0003`, template fixture `v0002`; assert the response contains distinguishing above-head wording (whatever exact phrase step 1 lands on) and still names both version numbers and the `config.tune(key: "bootstrap_alarm"` setter, mirroring `TestBootstrapStalenessWarningFiresOnFerrule`'s assertion shape (L27-49).
   - Add a subtest or sibling test for the unparseable-tag case: root `AGENTS.md` with a marker-shaped but malformed tag (e.g. `<!-- Template Version: vXYZ -->` or `<!-- Template Version: -->`), asserting the warning fires with the "unrecognized" wording and does not crash/panic on the malformed value.
   - Re-run `go test ./internal/mcp/... -run TestBootstrapStalenessWarning -v` and confirm all subtests (existing + new) pass.
3. `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md` (shared canonical playbook — do **not** edit the wsflow mirror by hand):
   - `## Invariants` (L9-20): add one bullet stating the honest enforcement limit, e.g. "A project tag above this skill's own `AGENTS.template.md` head, or one that does not parse, is a stop-and-report condition (see `## On: refuse`) enforced only by this instruction and the code-level staleness warning — there is no mechanical block on reconcile/restamp."
   - `## On: invoke` step 4 mode detection (L27-32): split the current `upgrade` bullet into two:
     - `upgrade` — `AGENTS.md` has a `<!-- Template Version: vNNNN -->` tag **at or below** this skill directory's own `AGENTS.template.md` head.
     - `refuse` — `AGENTS.md` has a `Template Version` marker whose value is **above** this skill directory's own `AGENTS.template.md` head, or one that does not parse as `vNNNN` at all.
   - Add a new `## On: refuse` section (parallel to `## On: fresh`/`## On: upgrade`/etc.): stop before making any change; do not update `AGENTS.md`/`WORKFLOW.md`/`CLAUDE.md`, do not update the version tag, do not commit; report the mismatch (on-disk tag vs. this package's own template head) to the user and ask how to proceed; explicitly restate that this is an agent-followed instruction, not a code-enforced block.
4. Regenerate the generated mirrors (mechanical, no hand edits to generated files) — run in this order after step 3:
   - `cd agents-plugin-tool && WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest`
   - `cd agents-plugin-tool && WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
   - Confirm `agents-plugin/rsrc/manifest.json`'s `lead-bootstrap/lead-bootstrap.md` hash changed and `agents-plugin-wsflow/rsrc/lead-bootstrap/lead-bootstrap.md` is byte-identical to the canonical file again (`diff` the two files).
5. Do not touch `AGENTS.template.md`, `WORKFLOW.md` (either package), the migration checklist, or any `## Spec` frontmatter/spec anchor text — all out of scope per the ticket's Phase 4 charter.

## Verification Plan

- `cd agents-plugin-tool && go test ./internal/mcp/... -run TestBootstrapStalenessWarning -v` — new above-head/unknown-tag tests pass; existing below-head, equal-version, off, and no-tag tests still pass unchanged.
- `cd agents-plugin-tool && go test ./internal/mcp/...` — full package suite green (confirms no other test depended on the old silent-above-head behavior; a repo-wide `grep -rn "ahead of latest\|downstream ahead" internal/mcp` before editing is a cheap extra check).
- `cd agents-plugin-tool && go test ./internal/wsrsrc/... -run TestWsflowRsrcMirrorUpToDate` — passes after the regen steps, confirming the wsflow rsrc copy is back in sync with the edited canonical playbook.
- `diff agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md agents-plugin-wsflow/rsrc/lead-bootstrap/lead-bootstrap.md` — empty (byte-identical) after regen.
- `python3 -m unittest discover agents-plugin-wsflow/tests` — full wsflow package suite stays green (regression check; the edited files are outside `SKILLS_DIR`, so no forbidden-pattern hits are expected, but this confirms the manifest/mirror regen did not disturb anything else scanned by wsflow tests).
- Manual/agent-reasoning check (per ticket verification boundary, not automatable by a Go test):
  - Below-head case: read the updated `## On: invoke` + `## On: upgrade` sections and confirm a tag strictly below this package's own template head still routes to `upgrade` (unchanged bucket) and proceeds through the existing migration walk to a clean re-stamp — no new branch intercepts it.
  - Above-head/unknown case: confirm the new `refuse` mode bullet and `## On: refuse` section are reachable from step 4/5 of `## On: invoke`, and that following them leaves `AGENTS.md`/`WORKFLOW.md`/the version tag byte-unchanged and produces a user-facing report rather than a silent reconcile.
- `gofmt -l agents-plugin-tool/internal/mcp/bootstrap_alarm.go agents-plugin-tool/internal/mcp/bootstrap_alarm_test.go` — no output (formatting clean).

## Escalations

- None.
