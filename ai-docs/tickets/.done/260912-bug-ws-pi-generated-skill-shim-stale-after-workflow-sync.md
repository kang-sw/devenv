---
title: "Pi reload can retain generated skill shims for removed playbook names"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: cbbfd7fce8370800
sage-review-completeness-reviewed: cbbfd7fce8370800
completed: 2026-09-13
---

# Pi reload can retain generated skill shims for removed playbook names

## Background

After synchronizing `track/pi-agent` with the new develop workflow and reloading Pi, the exposed `lead-write-ticket` skill instructed the lead to call `playbook.read(name: "lead-write-ticket")`. The synchronized rsrc inventory contains the replacement `lead-ticket` playbook and no `lead-write-ticket`, so the call failed with `no such rsrc playbook`. Calling `lead-ticket` directly succeeded.

This indicates that the package-local generated skills visible after reload can remain stale relative to the synchronized rsrc/entry workflow. The runtime/rsrc byte-identity tests remained green, so the current guard does not cover this loaded generated-skill seam.

## Decisions

- The supported local sync/reload path cleanly regenerates the package-local generated skill tree from the current shipped skill and playbook inventory; removed entry names must disappear rather than survive as stale files.
- Validation fails loudly when any generated skill shim targets a playbook absent from the current rsrc manifest. Regeneration is the normal repair; validation is the guard against an incomplete copy, cache, or reload result.
- Preserve the package topology in which generated skills are ignored build artifacts. Do not restore removed playbook aliases or silently fall back to a stale installed shim.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/scripts/copy-skills.mjs, agents-plugin-pi/src/skills-dir.ts, agents-plugin-pi/test/version-check.test.ts |
| scope.surface | public-interface | generated skills under agents-plugin-pi/skills/ are published package files |
| scope.new_public_symbol | no | no new symbol named in the phase |
| scope.new_type_contract | no | no type or signature change named in the phase |
| scope.test_surface | existing | agents-plugin-pi/test/version-check.test.ts; new regression coverage is required |
| complexity.reuse_points | confirmed | copy-skills.mjs replaces the generated tree; resolveSkillsDir selects the package-local tree; version-check.test.ts guards rsrc only |
| complexity.side_effect_risk | high | sync or reload behavior changes which skill instructions a Pi lead receives |
| risk.correctness | high | a stale entry shim directs a valid user action to a nonexistent playbook |
| risk.fit | moderate | clean regeneration follows the existing copy-skills path and validation extends the existing package identity guard |
| risk.test | moderate | existing adapter tests cover rsrc identity but not generated skill-to-playbook references |
| risk.security_or_contract | high | published generated skill names and their playbook targets are a caller-visible package contract |

## Phases

### Phase 1: Keep reloaded Pi skill shims aligned with the shipped playbook inventory

Reproduce the stale shim after a workflow rename or removal and identify whether the source is the ignored pack-time `skills/` copy, installed Pi package cache, or reload lifecycle. Make the supported local sync/reload path replace the generated tree from the current inventory so removed entries disappear, and fail validation when any generated shim targets a playbook absent from the current rsrc manifest. Add regressions for rename/removal, stale extra files, and nonexistent playbook targets. Preserve the package topology in which generated skills are not committed, and do not restore removed playbook aliases merely to hide stale generated state.

### Result (ef6316e) - 2026-09-13

Pi's `resources_discover` lifecycle now cleanly regenerates the ignored package-local skill tree from canonical `agents-plugin/skills/` on startup and reload, removing renamed entries and unrelated stale files before Pi rebuilds its command inventory. Pack-time copying uses the same replacement and validates literal `playbook.read`/`playbook.render` targets against the package-local rsrc manifest; installed packages without a canonical sibling validate and expose their carried generated tree.

Regression coverage executes the registered resource-discovery callback, the real copy-skills entrypoint in an isolated package fixture, source-absent installed-package success and failure paths, stale entry removal, and the canonical skill-to-rsrc target inventory. `npm test -- --test-reporter=dot` passed, and `npm pack --dry-run` regenerated, validated, and listed the skills tree in the tarball.

Correctness and fit reviews were clean. Round-one test review found that helper-only tests did not prove the production reload and pack entrypoints remained wired and omitted the valid installed-package path; `ef6316e` added those seams, and round two verified both Important findings fixed with no remaining observations.

Validation intentionally covers only static literal playbook names that can be proven from generated text; dynamic names continue through runtime playbook resolution.


## Resolution (2026-09-13)

Completed Phase 1: Pi startup/reload and package lifecycle paths now cleanly regenerate and validate generated skill shims against the current rsrc manifest, with independent review and regression coverage.
