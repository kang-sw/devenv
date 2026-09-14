---
title: "Pi delegated agents: bounded writable scopes with native edit/write semantics"
related:
  260912-bug-ws-pi-readonly-reviewer-artifact-contract: first consumer; grants one generated findings file to an otherwise read-only reviewer
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 87826949d28bfe20
sage-review-completeness-reviewed: 87826949d28bfe20
completed: 2026-09-13
---

# Pi delegated agents: bounded writable scopes with native edit/write semantics

## Background

Pi's standard child spawning chooses tools by static role groups, although lateral forks derive their surface from the parent's active tools (`agents-plugin-pi/src/spawner.ts#L152-L161`; `agents-plugin-pi/src/fork.ts#L107-L111`). An otherwise read-only child cannot receive authority to edit one report file or one generated-artifact tree without receiving broader filesystem tools, while a full worker already carries unrestricted write authority through native `edit` and `write`.

Add a reusable Pi-local delegation capability that lets a parent grant a child a bounded subset of its own effective write authority. This is a cooperative-agent workflow guardrail, not an adversarial operating-system sandbox. The first consumer is the reviewer findings artifact mismatch captured by `260912-bug-ws-pi-readonly-reviewer-artifact-contract`, but the capability is not reviewer-specific.

## Decisions

- Add an optional structured `write_scopes` field to Pi child spawning. It accepts exact-file and directory-tree grants rather than inferring kind from filesystem existence:

  ```ts
  type WriteScope =
    | { path: string; kind: "file" }
    | { path: string; kind: "tree"; include?: string[] };
  ```

- `path` is an absolute path. A file grant may name an existing file or a not-yet-created file whose parent exists. A tree root must exist when bound. A tree without `include` grants create/replace authority for descendant files; `include` contains root-relative glob patterns and is evaluated dynamically for each operation, including future matching files.
- Match `include` with Node's built-in `path.posix.matchesGlob` after narrow validation. Patterns and candidate paths use `/`, are root-relative and case-sensitive, and form a positive union. Support literals, backslash escaping, `*`, `?`, whole-segment `**`, and bracket classes/ranges. `*` and `?` do not cross `/`; `**` may cross components. Dot components require an explicit dot component in the pattern. Reject negation, braces, extglobs, NUL, empty/`.`/`..` segments, and invalid patterns before allocation. `*.md` matches the tree root only; `**/*.md` is recursive (`path.matchesGlob` is available since Node v22.5.0, while Pi requires Node >=22.19.0: `agents-plugin-pi/node_modules/@types/node/path.d.ts#L98-L105`; `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/package.json#L104`).
- Grants authorize only native edit/write create-or-replace semantics. They do not grant delete, rename, shell execution, or directory creation outside behavior already performed by the delegated native operation.
- Expose Pi's existing `edit` and `write` names and schemas through authorization wrappers. After a scope check succeeds, invoke the native Pi implementation so diffing, validation, and errors remain native. If safe native override or delegation is unavailable, fail child allocation; never fall back to unrestricted tools or a parallel public tool vocabulary (same-name built-in overrides retain native renderers, and Pi exports `createEditTool`/`createWriteTool`: `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/docs/extensions.md#L2078-L2110`; `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/core/tools/index.js#L1-L10`).
- Before every operation, normalize and canonicalize the candidate and scope root, verify containment and the applicable glob, and reject path traversal or a symlink resolution that escapes the canonical scope. Then delegate to the native operation. This is a best-effort guardrail; defending against a malicious concurrent process changing the filesystem between authorization and the native operation is explicitly out of scope.
- Capability delegation is monotonic. An unrestricted writer may grant narrower scopes; a scoped parent may grant only conservatively provable subsets of its own scopes; a read-only parent may grant none. Ambiguous glob-subset or canonicalization checks fail closed. The root lead retains only the repository's existing explicit privilege-expansion exception.
- Persist the normalized immutable scope binding with the child registry/sidecar lifecycle. Reload, resume, and descendant spawning may preserve or narrow it but never replace or widen it.
- If the child already has unrestricted native edit/write authority, `write_scopes` does not restrict it. Allocation returns a clear redundant/ignored diagnostic rather than silently implying confinement.
- Keep this capability Pi-local. Do not change shared ws-mcp, shared playbooks, or other harnesses.

## Constraints

- Tool-name presence alone is not the authority model: track an explicit effective write capability separately from the child-visible active tool list.
- A restricted child receives native edit/write wrappers only when its normalized scope set is nonempty and authorized by its direct parent.
- Scope diagnostics must not expose unrelated filesystem inventory.
- Existing unrestricted worker behavior remains unchanged.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/delegation-policy.ts, agents-plugin-pi/src/spawner.ts, agents-plugin-pi/src/agent-sidecar.ts |
| scope.surface | public-interface | ws-agent-spawn gains the write_scopes input contract |
| scope.new_public_symbol | yes | write_scopes |
| scope.new_type_contract | yes | WriteScope |
| scope.test_surface | existing | agents-plugin-pi/test/spawner.test.ts, agents-plugin-pi/test/recursive-worker.test.ts, agents-plugin-pi/test/agent-sidecar.test.ts |
| complexity.reuse_points | confirmed | delegation policy, spawn admission, RPC agent records, ownership metadata, and orphan sidecar |
| complexity.side_effect_risk | high | authorization runs before every native edit or write operation |
| risk.correctness | high | a containment or subset error can deny intended work or widen write access |
| risk.fit | high | the Pi extension must retain native edit/write behavior while narrowing authority |
| risk.test | high | spawned-child, persistence, and native tool-surface behavior need coverage |
| risk.security_or_contract | high | the public scope contract controls filesystem write authority |

## Phases

### Phase 1: Add monotonic write-scope delegation

Extend Pi spawn admission, delegation policy, agent records, and sidecar persistence with normalized `WriteScope` bindings and explicit effective-write capability. Add the authorization wrapper around native edit/write execution and fail allocation when a restricted wrapper cannot be installed safely.

Verify exact existing and new file grants, recursive tree grants, default-all descendants, dynamic include globs, create/replace behavior, path traversal and canonical escape rejection, symlink escape rejection, absent-parent and ambiguous subset refusal, unrestricted-to-scoped and scoped-to-narrower delegation, read-only refusal, nested monotonicity, reload/resume preservation, redundant unrestricted-child diagnostics, and the absence of Bash/delete/rename authority. Exercise the real child tool surface and native edit/write behavior rather than only pure path predicates. Run the committed glob-contract fixture against Pi's Node 22.19 version floor as well as the development runtime; do not infer floor semantics solely from a newer Node release.

### Result (d00e66d3) - 2026-09-13

- Added Pi-local `write_scopes` admission, explicit effective-write capability, immutable persisted bindings, conservative descendant narrowing, and same-name native edit/write wrappers. Unrestricted children retain their existing authority and receive an ignored diagnostic for valid redundant scopes; malformed scopes are rejected before allocation.
- Native tool registration, scoped sidecar revival/resume, denial before widened recovery allocation, filesystem containment, dynamic glob matching, and native create/replace/edit behavior are covered by tests. Shared ws-mcp and other harness surfaces remain unchanged.
- Review fixes landed in `10d1840e` and `94b1dc86`. The escalated Critical was a second native normalization of an authorized absolute path: a Unicode-space cwd could redirect execution into an ASCII-space sibling. The handoff now uses Node's file-URL encoding of the checked canonical target, which Pi decodes after convenience normalization. Regression coverage exercises all 15 normalized Unicode spaces from cwd, symlinks, and decoded URLs, plus percent/hash names and explicit sibling denials.
- Decisions: use exact syntactic glob projection and exact-file membership to prove scope subsets, rejecting ambiguous subset claims; preserve native execution rather than reimplement filesystem operations; encode valid canonical paths rather than deny Unicode-space grants.
- Verification: the new regression reproduced the original escape before the fix. `node --test test/write-scopes.test.ts` passed all 11 tests on Node 25.9.0; `npx --yes node@22.19.0 --test test/write-scopes.test.ts` passed all 11 tests, including the committed glob fixture, on the exact version floor. `npm test -- --test-reporter=dot` passed the full Pi suite; `git diff --check` passed. Commands ran from `agents-plugin-pi/` except the Git check; the package declares no separate build script.
- Independent correctness, fit, and test verification of the escalation fix (`10d1840e..94b1dc86`) is clean. Correctness inspection covered both package-local Pi 0.84.4 and installed Pi 0.85.1; that reviewer could not execute tests under its restricted tool surface, so runtime evidence comes from the worker's commands above.
- Unresolved: none. Omitted: none.

