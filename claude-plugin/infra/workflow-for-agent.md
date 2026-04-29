# Doc System Orientation

This project maintains three documentation layers. Each has a distinct role;
understanding their relationships helps you navigate context and report
accurately.

## The Three Layers

**`ai-docs/spec/`** — External-facing contracts.
Each file documents caller-visible behavior for a domain. Features are identified
by `{#YYMMDD-slug}` anchors (spec stems). A 🚧 marker on an entry means the
feature is planned but not yet implemented. Stems are the shared key that
connects spec entries to tickets.

**`ai-docs/mental-model/`** — Current implementation understanding.
These docs describe how the codebase actually works: domain structure, extension
points, invariants, coupling. They are kept in sync with code by the
`ws:mental-model-updater` agent after merges. Read them to understand existing
behavior before implementing.

**`ai-docs/tickets/`** — Work units.
Each ticket tracks a bounded scope of work through `idea/ → todo/ → wip/ → .done/`.
A ticket's `spec:` frontmatter field (optional) lists the spec stems it covers.

## How They Connect

```
ticket (spec: field)  →  spec stem {#YYMMDD-slug}  →  spec entry in ai-docs/spec/
                                                         (🚧 = not yet implemented)
mental-model docs  ←  updated after code lands  →  reflect current implementation
```

## Your Role

Read these docs for context. Do not modify them unless your brief explicitly
tasks you to do so — lifecycle management (stripping 🚧, updating mental models,
advancing ticket status) is the lead's responsibility.

If a doc appears stale or contradicts code you encounter, note it in your output.
Do not silently work around it.

## Available Primitives

These read-only tools are safe to call from any agent context:

```bash
ws-print-infra <stem>        # print an infra doc to stdout
ws-infra-path <stem>         # resolve infra doc path
ws-list-mental-model [paths] # list relevant mental-model docs
ws-list-spec-stems           # list all spec stems in ai-docs/spec/
ws-proj-tree                 # print project structure overview
ws-spec-build-index          # rebuild spec features: frontmatter; may write spec files
ws-subquery "<question>"     # haiku-backed read-only codebase search
ws-subquery --deep-research "<question>"  # sonnet-backed, for cross-module traces
```

Do NOT call these from sub-agent context — they are lead-only orchestration
primitives:

```
ws-new-named-agent    # registers a new agent session
ws-call-named-agent   # dispatches a registered agent
ws-interrupt-named-agent  # injects a message into a running agent
ws-merge-branch       # merges a sprint branch into main
```

## API Documentation

When you need external library API information (function signatures, usage patterns,
threading models, error handling — anything from a third-party library), use
`ws-ask-api`. Do NOT use WebSearch or WebFetch for API lookup.

```bash
ws-ask-api "<question>"                  # domain resolved automatically
ws-ask-api <domain-hint> "<question>"    # hint for a specific library
```

Examples:
```bash
ws-ask-api "how does asio strand prevent data races?"
ws-ask-api asio "executor vs strand difference"
ws-ask-api "grpc ClientContext deadline semantics"
```

`ws-ask-api` returns a structured answer citing which cached doc sections were used.
The cache is maintained automatically; you do not need to manage it.
