---
name: document-dependency
description: Create or update a delta document for a specific dependency, recording where the model's training knowledge diverges from the actual API. For internal libraries with no training data, builds the document from scratch.
argument-hint: "<dependency name> [version]"
---

# Document Dependency

Target: $ARGUMENTS

## Overview

This skill orchestrates dependency documentation by delegating the heavy lifting
to the `document-dependency` subagent. The skill handles pre/post steps while the
agent does API exploration and document writing in an isolated context.

## Step 1: Pre-check

1. Determine the dependency version from the project manifest (e.g., `Cargo.toml`,
   `package.json`, `pyproject.toml`, `go.mod`).
2. Check if `ai-docs/deps/` already has a document for this dependency and version.
   If it exists and matches the current model, confirm with the user whether to
   regenerate or skip.

## Step 2: Delegate to subagent

Launch the `document-dependency` agent with the dependency name and version.
The agent will:
- Gather the actual API surface
- Assess known vs unknown
- Write the document to `ai-docs/deps/`
- Return a summary (file path, document type, corrections found, uncertainties)

Multiple dependencies can be documented in parallel by launching multiple agents.

## Step 3: Update CLAUDE.md

After the agent completes, add or update the entry in the project's
`CLAUDE.md` under `# MEMORY → Documented Dependencies`:

```
- <name> [v<version>/<model>]    ← known (delta)
- <name> [v<version>]            ← unknown (full summary)
```

## Step 4: Summary

Report to the user:
- Dependency name and version documented.
- Document type (delta or full summary) and file path.
- Number of corrections / additions found (for delta docs).
- Any areas of uncertainty flagged by the agent.
