---
title: Async exec job surface
related:
  260513-feat-async-exec-output-reader: absorbed original broad ticket
related-mental-model:
  - mcp-runtime
  - named-agent-runtime
  - plugin-runtime
---

# Async exec job surface

## Scope

Create a ws MCP execution-job surface that lets trusted lead workflows run
bounded shell or argv commands without forcing large stdout or stderr into the
lead context.

The epic owns decomposition for:

- durable file-backed exec job records scoped to the current ws worktree;
- launch, lifecycle, abort, and bounded output result tools;
- reusable text readers over persisted output files;
- later model-backed questions over command output;
- runtime contract, capability, CLI, and wsflow visibility alignment for each
  introduced tool surface.

## Non-Scope

- Interactive PTY or terminal session management.
- Dashboard Activity Console UI or transcript projection work.
- Remote, multi-user, or authority-bound execution semantics.
- Treating MCP profile filters as a security boundary.
- Model-backed output synthesis in the first implementation child.

## Child Tickets

- `260524-feat-exec-job-core-text-readers` - first implementation child for
  `exec.spawn`, `exec.shell`, `exec.status`, `exec.result`, `exec.abort`, and
  text readers over persisted output files.
- Planned: `exec.ask` output-question reader - add model-backed questions over
  persisted exec output after the core text-reader surface is proven.

## Cross-Child Decisions

- Public tool arguments should use `working_dir` for execution location. The
  term `root` remains reserved for the ws MCP repository/worktree context and
  should not be overloaded as an exec command parameter.
- Omitted `working_dir` resolves to the current ws project/worktree root through
  the existing ws root resolver. Relative `working_dir` values resolve beneath
  that ws worktree root rather than the plugin cache cwd.
- `exec.spawn` is structured argv execution. `cmd` means executable or
  `argv[0]`, not a shell command line.
- `exec.shell` is explicit shell execution and accepts one command string.
  Optional shell selection must be named as shell executable or shell profile
  semantics in the child ticket before implementation.
- Launch tools create a durable job record first, then wait up to a fixed short
  foreground window before responding. That foreground wait is not a caller-set
  timeout and does not replace async job recovery.
- Normal tool responses never return more than the fixed small-output budget.
  Larger output is inspected through bounded text readers or later `exec.ask`.
- Command output is untrusted input. Any model-backed reader must be designed as
  a separate child with explicit prompt-injection and stale-context controls.
- All introduced `exec.*` tools remain hidden from wsflow no-agent mode.

## Completion Criteria

- Done: child tickets deliver the core exec job surface and any accepted reader
  layers, with specs, runtime metadata, tests, and wsflow visibility aligned.
- Dropped: the repo chooses not to expose arbitrary execution through ws MCP.
- Deferred: dashboard or harness integrations may remain outside this epic when
  they only consume the exec job surface.
