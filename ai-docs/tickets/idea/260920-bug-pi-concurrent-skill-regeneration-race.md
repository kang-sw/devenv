---
title: "Concurrent Pi resource discovery can race while regenerating the shared skills directory"
related:
  260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter: increased routine nested-process concurrency exposed the existing race; not the direct cause
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

## Open Questions

- Should resource discovery use an inter-process lock, an atomic prepared-tree
  swap, or an idempotent no-delete synchronization strategy?
- Which other generators, including `scripts/copy-skills.mjs`, can race with Pi
  startup and must share the same serialization boundary?
- Which native copy layer emits the observed uncaught libc++ filesystem error?

## Phases

### Phase 1: Make shared skill generation safe across concurrent Pi processes

Create a deterministic concurrent discovery reproduction, identify every writer
to the generated skills tree, and make regeneration safe when multiple Pi
processes initialize simultaneously.

Verification must run overlapping resource discovery/copy operations repeatedly,
assert that no process observes a missing destination parent or crashes, and
prove that the resulting generated tree matches the source without stale or
partial entries.
