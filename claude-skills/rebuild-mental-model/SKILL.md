---
name: rebuild-mental-model
description: Rebuild or update the ai-docs/mental-model/ directory so it reflects the current source code. Delegates source exploration to subagents to keep the main context window small.
argument-hint: "[target path or special instruction] (omit for full rebuild)"
---

# Rebuild Mental Model

Target: $ARGUMENTS

## Principles

- **Do not read source directly.** Delegate all source exploration to subagents
  (Explore agents, general-purpose Agents, Agent teams — choose the approach that
  fits the scope). Read source yourself only when a subagent summary is clearly
  insufficient. This is critical for keeping the main context window small.
- **Fractal document tree.** The mental-model directory mirrors the project's module
  hierarchy. The recursive rule is simple:
  - A leaf module becomes `<name>.md`.
  - A module with children becomes `<name>/index.md`.
  - Example (depth 2):
    ```
    ai-docs/mental-model/
      index.md                  ← project-wide overview
      networking/
        index.md                ← networking module: purpose, public API, children
        transport.md            ← leaf: transport implementation details
        protocol/
          index.md              ← protocol sub-module
          handshake.md          ← leaf
      game-logic/
        index.md
        combat.md
        inventory.md
    ```
  - Each `index.md` contains: one-paragraph purpose, public interface summary
    (key types, functions, abstractions), and a list of child modules with one-line
    descriptions.
  - Depth follows source complexity. Most projects stay within 2 levels.
- **Incremental by default.** Only rebuild what has actually changed, unless a full
  rebuild is explicitly requested.

## Step 0: Determine dirty scope

1. Check whether `ai-docs/mental-model/` already exists.
   - **Exists →** Find the oldest last-committed date across all mental-model
     documents (e.g., `git log -1 --format="%aI" -- ai-docs/mental-model/`). Collect
     source files changed since that date (`git diff --name-only <commit> HEAD`).
     This produces a conservatively wide "dirty set." Subagents in Step 1 will
     naturally skip files that turn out to be irrelevant.
   - **Does not exist →** The entire project is dirty.
2. If `$ARGUMENTS` names a specific path, narrow the dirty set to that subtree.
3. If `$ARGUMENTS` contains special instructions (e.g., "migrate flat docs to fractal
   structure", "focus on public APIs only"), carry them forward as additional
   directives for all subsequent steps.

## Step 1: Explore source (subagent-delegated)

Dispatch subagents to read and summarize the dirty source files. A suggested
progression — adapt as you see fit:

1. **Macro pass** — Project-wide structure: top-level modules or packages, dependency
   graph, entry points, build targets.
2. **Module pass** — Per-module: public interface, key types and functions, internal
   structure, dependencies on sibling modules.
3. **Detail pass** (only when needed) — Implementation specifics for subsystems too
   complex to summarize at the module level alone.

How many agents to use, whether to run them in parallel or in sequence, and how to
batch the work is your judgment call. Optimize for thorough coverage while minimizing
token consumption on the main context.

## Step 2: Write / update mental-model documents

Using subagent summaries, create or update documents under `ai-docs/mental-model/`.

- Follow the fractal structure described in Principles.
- Leaf documents should only cover implementation details that would be non-obvious
  to a reader who already understands the module's public interface.
- If a module no longer exists in source, remove or merge its document.
  When uncertain, flag it for the user rather than deleting silently.
- Prefer concrete type/function names over vague descriptions.

## Step 3: Update _index.md

Update `ai-docs/_index.md` to reflect current project-wide architecture:

- Project structure and module boundaries.
- Cross-module dependency relationships.
- Operational state (what works, what is in progress).
- A reference to the mental-model document tree so future sessions know where to look.

## Step 4: Summary

Print a concise summary for the user:

- Dirty set size (files examined).
- Documents created / updated / removed.
- Areas where subagent exploration felt insufficient — flag for manual review.
