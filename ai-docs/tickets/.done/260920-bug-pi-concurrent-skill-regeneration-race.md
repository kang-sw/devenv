---
title: "Concurrent Pi resource discovery can race while regenerating the shared skills directory"
related:
  260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter: increased routine nested-process concurrency exposed the existing race; not the direct cause
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3e8830d258e40407
sage-review-completeness-reviewed: 3e8830d258e40407
completed: 2026-09-21
---

# Concurrent Pi resource discovery can race while regenerating the shared skills directory

## Background

After merging the recursive subagent-tree work, the TUI surfaced a native crash:

```text
libc++abi: terminating due to uncaught exception of type std::__1::__fs::filesystem::filesystem_error: filesystem error: in create_directory: No such file or directory ["/Users/kang-sw/devenv/agents-plugin-pi/skills/lead-delegate"]
```

The reported directory exists in the checkout, but each Pi RPC process loads the
extension and registers `resources_discover`. `prepareSkillsDir()` in
`agents-plugin-pi/src/skills-dir.ts` unconditionally removes the shared generated
`agents-plugin-pi/skills` tree and copies it again. Concurrent lead, worker, and
grandchild startup can therefore interleave one process's recursive copy with
another process's recursive removal, making a destination parent disappear
while the first process is creating a child such as `lead-delegate`.

The merged subtree code watches only each child's private `subtree.json`
directory and does not write or watch `agents-plugin-pi/skills`. Recursive agent
usage increased concurrent startup opportunities, exposing an existing shared
generation hazard rather than directly introducing the skill-path mutation.

## Decisions

- Gate runtime synchronization on a deterministic source/generated content hash.
  A matching hash is a strict no-op; resource discovery must not remove, copy,
  or rewrite the generated tree when source content is unchanged.
- On a hash mismatch, run one exact mirror synchronization: copy changed and new
  entries, remove entries absent from source, validate the result, and publish
  the generated hash marker only after the synchronization succeeds.
- Preserve the supported installation modes: npm tarballs continue using their
  bundled package-local skills when canonical source is absent; local and full
  git checkouts with canonical source may refresh the generated mirror on a
  mismatch.
- Inter-process locking is a non-goal for this first fix. The hash gate is the
  mitigation boundary; simultaneous first generation or simultaneous mismatch
  writers remain deferred unless implementation evidence shows the unlocked
  path cannot be made acceptably safe.

## Open Questions

- What deterministic hash input and marker format best cover relative paths,
  file contents, additions, removals, and renames without depending on mtimes?
- Which other generators, including `agents-plugin-pi/scripts/copy-skills.mjs`, must reuse the
  same hash and exact-sync implementation so their markers cannot diverge? (The script already
  calls `syncGeneratedSkillsDir` at agents-plugin-pi/scripts/copy-skills.mjs#L9-L23.)
- Which native copy layer emits the observed uncaught libc++ filesystem error?

## Prior Decisions

- 260912-bug-ws-pi-generated-skill-shim-stale-after-workflow-sync (2026-09-13, Result ef6316e): "Pi's resources_discover lifecycle now cleanly regenerates the ignored package-local skill tree from canonical agents-plugin/skills on startup and reload." — bearing: constrains
- 979f883 (2026-09-20, commit): "The subtree merge did not write the skills path; routine nested-process concurrency increased exposure to the pre-existing race." — bearing: supports
- fb8e156d (2026-05-23, commit): "Concurrent Codex MCP startup against an empty shared runtime directory reproduced the intermittent failure: fixed temporary download paths were deleted or replaced by another launcher process." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/skills-dir.ts, agents-plugin-pi/scripts/copy-skills.mjs, agents-plugin-pi/test/skills-dir.test.ts |
| scope.surface | internal | package-private resource-discovery and pack-time synchronization paths; no external API symbol is named |
| scope.new_public_symbol | no | no external symbol is required; syncGeneratedSkillsDir is already the script's internal helper |
| scope.new_type_contract | yes | deterministic generated-tree hash marker shared by resources_discover and agents-plugin-pi/scripts/copy-skills.mjs |
| scope.test_surface | existing | agents-plugin-pi/test/skills-dir.test.ts covers discovery, source-absent installed packages, and the pack-time entrypoint |
| complexity.reuse_points | confirmed | syncGeneratedSkillsDir, validateGeneratedSkillTargets, prepareSkillsDir, and the copy-skills entrypoint are present in agents-plugin-pi/src/skills-dir.ts and agents-plugin-pi/scripts/copy-skills.mjs |
| complexity.side_effect_risk | high | a mismatch updates the live generated skills tree while concurrent startup remains intentionally unlocked |
| risk.correctness | high | stale-entry removal, hash-marker publication after validation, and concurrent readers must preserve the established generated-skill contract |
| risk.fit | moderate | the current shared helper already serves both resource discovery and pack-time copy, but the hash and exact-mirror boundary need design |
| risk.test | high | existing tests cover replacement and installed-package fallback but not unchanged-source mutation absence, failed-marker publication, or concurrent discovery |
| risk.security_or_contract | moderate | published package skills and the source-absent fallback remain a user-visible installation contract |

## Phases

### Phase 1: Add hash-gated mismatch-only skill synchronization

Create a deterministic reproduction for unchanged and changed source trees,
centralize the package/runtime sync path, and make an unchanged generated tree a
strict no-op. On mismatch, perform an exact incremental mirror and publish its
hash marker only after validation succeeds. Do not add inter-process locking or
an agent/process lifetime timeout.

Verification must prove that repeated lead, worker, and nested-child discovery
against an unchanged source performs no generated-tree mutations; additions,
content changes, removals, and renames each produce an exact mirror and updated
marker; a failed sync never publishes the new marker; installed-package behavior
with canonical source absent remains unchanged. Include a bounded concurrent
unchanged-source run to cover the observed high-frequency spawn path, while
recording simultaneous mismatch writers as deferred risk rather than claiming
serialization.

### Result (7a7954b9) - 2026-09-21

- Runtime discovery and the pack-time script now share hash-gated incremental
  synchronization. Unchanged discovery only reads and validates; mismatches
  preserve unchanged entries, remove stale entries, and atomically replace
  changed files. Source-absent packages retain their bundled skills and target
  validation.
- The reserved root `.ws-skills-hash` marker contains
  `ws-skills-v1:sha256:<digest>` plus a newline. Sorted names, entry types, file
  bytes and permission bits, literal link targets, and empty directories feed
  the hash; mtimes do not. Comparing actual generated contents as well as the
  marker repairs generated drift. Publication follows target validation and
  source/generated equality checks, using a unique temporary file and rename.
- Regression commit c8da7a8b guards filesystem mutation calls in isolated
  discovery processes, including six barrier-synchronized processes repeatedly
  invoking lead/worker/nested-child callbacks. It also covers additions,
  same-mtime content changes, removals, renames, type changes, marker recovery,
  failed validation/I/O/publication, and pack/runtime agreement.
- Independent correctness and fit reviews were clean. Test review's one
  Important finding, the empty-source-root boundary, was fixed in 7ab267da;
  round two confirmed closure. No review findings remain.
- Verification: `node --test test/skills-dir.test.ts` passed 15/15;
  `npm test -- --test-reporter=dot` passed the full Pi suite; `git diff --check`
  passed. Initial full-suite failures were environment-only: a symlinked
  dependency tree violated the existing web-search realpath guard. A physical
  worktree-local dependency copy resolved them without source/test changes.
- Native-layer evidence: the installed Node v26 `internal/fs/cp/cp-sync`
  implementation dispatches unfiltered recursive copying to
  `fsBinding.cpSyncCopyDir`. This fix removes that path; the original macOS
  crash's exact native stack was not available for confirmation.
- Deferred by design: simultaneous first-generation/mismatch writers and
  whole-tree reader atomicity during a mismatch. No lock, process-lifetime
  timeout, or serialization claim was introduced.
