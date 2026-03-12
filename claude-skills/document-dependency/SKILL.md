---
name: document-dependency
description: Create or update a delta document for a specific dependency, recording where the model's training knowledge diverges from the actual API. For internal libraries with no training data, builds the document from scratch.
argument-hint: "<dependency name> [version]"
---

# Document Dependency

Target: $ARGUMENTS

## Overview

Orchestrates dependency documentation by delegating to the `document-dependency`
agent. This skill handles pre/post steps; the agent does API exploration and
document writing in an isolated context.

## Step 1: Pre-check

1. Find the dependency version from the project manifest (`Cargo.toml`,
   `package.json`, `pyproject.toml`, `go.mod`, etc.).
2. Check `ai-docs/deps/` for an existing document matching this dependency and
   version. If found and current, confirm with the user whether to regenerate.

## Step 2: Gather context for the agent

Before launching the agent, collect context that will be included in the prompt.
The agent runs in a fresh context with NO knowledge of the project, so this
information is critical for quality output.

### 2a. Source location

Find where the dependency's source code lives:
- **Rust**: Run `find ~/.cargo/registry/src -maxdepth 1 -name '<crate>-<version>*'`
  to get the exact path. For re-export crates, also locate implementation crates
  (e.g., `godot` → `godot-core`, `godot-macros`).
- **Node**: Check `node_modules/<pkg>/` for type definitions.
- **Python**: Check site-packages or venv.

### 2b. Project usage scan

Run a targeted grep for the dependency's imports and key symbols:
```
grep -r "use <crate>::\|<crate>::" --include="*.rs" src/ lib/
```
Summarize which types, traits, macros, and functions the project actively uses.

### 2c. Ecosystem hints

Note any relevant tooling:
- Rust: whether `cargo brief` works for this crate
- Whether the crate is a thin re-export wrapper vs. monolithic
- Known quirks (proc-macro heavy, codegen, etc.)

## Step 3: Delegate

Launch the `document-dependency` agent. The prompt MUST include all of the
following (this is a checklist — verify before launching):

- [ ] **Dependency name and exact version** (e.g., `godot v0.4.5`)
- [ ] **Source paths** — absolute paths to source directories found in Step 2a
      (e.g., `~/.cargo/registry/src/.../godot-core-0.4.5/`)
- [ ] **Project root** — the working directory path
- [ ] **Project usage summary** — the grep results from Step 2b, listing which
      types/macros/traits the project actually uses
- [ ] **Ecosystem notes** — from Step 2c (tooling hints, crate structure)
- [ ] **Output file path** — `ai-docs/deps/<name>[v<version>].md` or delta

If the dependency has sub-crates or implementation crates, list them all with
paths so the agent doesn't waste tool calls searching for them.

Multiple dependencies can be documented in parallel by launching multiple agents.

## Step 4: Verify

Launch the `verify-dependency-document` agent to check the written document
against actual source. This runs in a **fresh context** with no carry-over from
the writing agent, which is critical for catching confirmation bias.

The prompt MUST include:
- **Document path** — the file written in Step 3
- **Source path(s)** — the same source paths gathered in Step 2a
- **Language** — Rust, TypeScript, Python, Go, etc.

### Handling verification results

- **CRITICAL errors**: Fix them in the document immediately (use Edit tool).
  These are factual inversions or fundamentally wrong claims.
- **ERROR-level issues**: Fix them in the document.
- **MISSING items**: Add to the document if they are used by the project
  (check Step 2b usage scan). Otherwise note in the summary but don't add.
- **EXAMPLE issues**: Fix or remove the broken example.

If there are 3+ CRITICAL errors, consider whether the document should be
regenerated from scratch rather than patched.

## Step 5: Update CLAUDE.md

After verification and fixes, add or update the entry in the project's
`CLAUDE.md` under `# MEMORY → Documented Dependencies`:

```
- <name> [v<version>/<model>]    ← known (delta)
- <name> [v<version>]            ← unknown (full summary)
```

## Step 6: Summary

Report to the user:
- Dependency name, version, document type (delta or full), and file path.
- Number of corrections / additions (for delta docs).
- Coverage assessment: what percentage of project-used APIs are documented.
- Verification result: errors found and fixed, remaining uncertainties.
- Any areas where the verifier flagged issues that need human review.
