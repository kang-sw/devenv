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
- raw fallback text readers over persisted output files;
- lead-facing model-backed questions over command output;
- runtime contract, capability, CLI, and wsflow visibility alignment for each
  introduced tool surface.

## Non-Scope

- Interactive PTY or terminal session management.
- Dashboard Activity Console UI or transcript projection work.
- Remote, multi-user, or authority-bound execution semantics.
- Treating MCP profile filters as a security boundary.
- Advanced reader memory, cross-job synthesis, or dashboard presentation of
  exec answers before the basic `exec.ask` layer exists.

## Child Tickets

- `260524-feat-exec-job-core-text-readers` - done; first implementation child for
  `exec.spawn`, `exec.shell`, `exec.status`, `exec.result`, `exec.abort`, and
  raw fallback text readers over persisted output files.
- `260524-feat-exec-output-ask` - add lead-facing `exec.ask` questions over
  persisted exec output.
- `260526-bug-exec-readable-result-affordance` - ready; improve the basic
  exec launch/status/result/abort readability, key length, and result wait
  affordance before layering model-backed output questions on top.
- `260524-chore-exec-surface-runtime-contract` - finalize runtime
  capabilities, manifests, CLI mirror policy, and wsflow package drift after the
  accepted `exec.*` surface is stable.

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
  Larger output guidance should point to `exec.ask` first and `exec.raw.*`
  fallback readers second.
- Raw text readers are fallback tools, not the primary lead-facing large-output
  UX. Name them under `exec.raw.*` so callers distinguish direct raw output
  inspection from model-backed answering.
- Command output is untrusted input. `exec.ask` must include explicit
  prompt-injection boundaries and default to a fresh reader context unless the
  caller opts into resumed context.
- All introduced `exec.*` tools remain hidden from wsflow no-agent mode.

## Completion Criteria

- Done: child tickets deliver the core exec job surface and any accepted reader
  layers, with specs, runtime metadata, tests, and wsflow visibility aligned.
- Dropped: the repo chooses not to expose arbitrary execution through ws MCP.
- Deferred: dashboard or harness integrations may remain outside this epic when
  they only consume the exec job surface.
