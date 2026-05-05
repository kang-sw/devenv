---
domain: git-workflow-tools
description: "Constrained Git MCP and CLI tools for status, diff, log, merge-base, and structured commits."
sources:
  - agents-plugin-tool/internal/wsgit/
  - agents-plugin-tool/internal/mcp/
  - agents-plugin-tool/cmd/ws-mcp/
related:
  mcp-runtime: "git.* tools are exposed through MCP schemas and CLI mirrors."
  documentation-system: "git.commit detects ticket moves and Result headings from documentation conventions."
---

# Git Workflow Tools

## Entry Points

- `internal/wsgit/git.go` is the core client and test seam around `git -C <root>`.
- `git.status`, `git.diff`, `git.log`, `git.merge_base`, and `git.commit` are the MCP surface. {#260505-git-workflow-tools}
- `ws-mcp git ...` is a CLI mirror with separate flag parsing.

## Module Contracts

- All Git execution goes through `wsgit.Runner`; tests rely on runner injection.
- Status parsing depends on `git status --porcelain=v2 --branch`. Changing flags changes branch and file-state parsing.
- Range-less diffs append untracked files; ranged diffs do not. This is user-visible behavior.
- Path filters are appended after `--`; commit paths reject absolute paths, `..`, and option-like values.
- `git.commit` stages only explicit paths, expands ticket moves by stem, rejects unrelated staged paths, detects ticket Result additions from cached diffs, builds a structured message, then commits.

## Coupling

- Git operation additions require `wsgit`, MCP dispatch/schema, CLI handler/usage, tests, optional profile filtering, and docs.
- Diff modes are shared constants across core, MCP enum, CLI default/help, and docs.
- Ticket move expansion depends on `ai-docs/tickets/{idea,todo,wip,.done,.dropped}` layout.
- `git.commit` is hidden only when the optional leaf profile filter is active.

## Extension Points & Change Recipes

- **Add a diff mode**: add constants, `DiffArgs`, MCP enum, CLI help/default handling, tests, and documentation.
- **Add a write Git operation**: decide optional profile access explicitly; prompt-level role rules remain the primary containment mechanism.
- **Change commit message format**: update structured message builder, ticket update detection, commit rules, and tests.

## Common Mistakes

- Assuming `git.diff` is just `git diff`; untracked files are appended for range-less calls.
- Passing directories to `git.commit` while unrelated staged files already exist under that directory; validation treats requested paths as roots.
- Using unconstrained staging; the primitive stages only requested path roots, with `git add -A -- <paths>` for non-deleted paths and explicit cached removal for deleted paths.
- Expecting general Git operations like reset, checkout, clean, merge, push, or arbitrary commit mutation; this is a constrained workflow wrapper.

## Technical Debt

- Ticket Result detection is textual and narrow: only cached diff lines beginning `+### Result` under tickets are summarized.
- Revision validation mainly prevents option injection; it does not fully parse Git revision syntax.
