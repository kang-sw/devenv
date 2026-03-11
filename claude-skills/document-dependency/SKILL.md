---
name: document-dependency
description: Create or update a delta document for a specific dependency, recording where the model's training knowledge diverges from the actual API. For internal libraries with no training data, builds the document from scratch.
argument-hint: "<dependency name> [version]"
---

# Document Dependency

Target: $ARGUMENTS

## Principles

- **One dependency per invocation.** This skill documents a single dependency at a
  time. The user specifies which one.
- **Known vs unknown.** Before documenting, determine whether you have prior
  knowledge of this dependency's API. This is not about internal vs external — an
  obscure external package you have never seen is just as unknown as a workspace
  member.
  - **Known** → produce a delta document: only corrections, additions, and removals
    relative to your training knowledge. Omit what you already know correctly.
  - **Unknown** → produce a complete API summary from the consumer's perspective:
    key types, functions, usage patterns, and important constraints. The goal is that
    a reader can use the library without exploring its source code.
- **Filename convention distinguishes document type.**
  - Known (delta): `<name>[v<version>/<model>].md` — model tag present, because the
    document is relative to that model's training knowledge. Stale on model upgrade or
    version bump.
  - Unknown (full summary): `<name>[v<version>].md` — no model tag, because the
    document is built from scratch and valid for any model.

## Step 0: Identify the dependency

1. Determine whether the dependency is **external** (published package) or **internal**
   (workspace member / local library).
2. Find the current version from the project manifest (e.g., `Cargo.toml`,
   `package.json`, `pyproject.toml`, `go.mod`).
3. Check if `ai-docs/deps/` already has a document for this dependency and version.
   If it exists and matches the current model, confirm with the user whether to
   regenerate or skip.

## Step 1: Gather actual API

Obtain the real, current API surface of the dependency. Approaches by ecosystem
(use whichever is available):

- **Rust** — `cargo brief <crate>`, or read `docs.rs` / crate source.
- **Node/JS/TS** — Read type definitions from `node_modules/`, or official docs.
- **Python** — Read package source or official docs.
- **Go** — `go doc <package>`, or read source.
- **Other** — Use available documentation, source code, or web resources.

For internal libraries, delegate source exploration to a subagent if the codebase is
large enough to warrant it.

## Step 2: Produce the delta document

First, assess: do you have prior knowledge of this dependency's API?

- **Known** — Before looking at Step 1 results, recall what you believe the API looks
  like (mentally, not written to file). Then compare your expectations against the
  actual API from Step 1. The output document records only the delta: corrections
  (things you got wrong), additions (things you did not know), and removals (API that
  no longer exists). Omit anything you already know correctly.
- **Unknown** — No prior knowledge exists. Write a complete API summary from the
  consumer's perspective: key types, functions, common usage patterns, and important
  constraints. The goal is that a reader can use the library without exploring its
  source code.

## Step 3: Write the document

Save to `ai-docs/deps/` using the filename convention from Principles:
- Known: `<name>[v<version>/<model>].md`
- Unknown: `<name>[v<version>].md`

Example filenames:
- `ai-docs/deps/hecs[v0.11.0/opus-4.6].md` — known, delta for Opus 4.6
- `ai-docs/deps/libgame[v0.1.0].md` — unknown, full API summary

## Step 4: Update CLAUDE.md

Add or update the entry in `# MEMORY → Documented Dependencies`:

```
- <name> [v<version>/<model>]    ← known (delta)
- <name> [v<version>]            ← unknown (full summary)
```

## Step 5: Summary

Report to the user:
- Dependency name and version documented.
- Number of corrections / additions found (for external deps).
- Any areas of uncertainty — flag for manual verification.
