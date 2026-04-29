---
title: API Dependency Docs
summary: Filesystem-based external API documentation cache with 2-layer agent routing, consumed by downstream project agents via ws-ask-api.
---

# API Dependency Docs

`ws-ask-api` provides downstream project agents with structured, cached external API documentation. Agents query it in natural language; the system resolves relevant domains, fetches and caches docs on demand, and returns answers without exposing the cache layout or web browsing to callers.

## `ws-ask-api` Bin Tool {#260429-ws-ask-api-tool}

PATH-accessible script installed by the `ws` plugin.

```
ws-ask-api [<domain-hint>] "<prompt>"
ws-ask-api --refresh <domain>
ws-ask-api --check-stale <domain>
ws-ask-api --list
```

- No domain hint: the pre-router resolves relevant domains from the prompt and existing cache.
- With domain hint that exactly matches an existing `ai-docs/.deps/<hint>/` directory: pre-router is skipped; `ws-ask-api-internal <hint>` is called directly.
- With domain hint that does not match any existing directory: hint is passed to the pre-router as a strong prior; the pre-router may expand to additional domains.
- `--refresh <domain>`: forces re-fetch of all doc levels for the named domain.
- `--check-stale <domain>`: runs `scripts/detect-version` against the project and reports whether the cached version matches. No fetch.
- `--list`: emits all domain names present in `ai-docs/.deps/`.
- Extra positional arguments beyond `[hint] prompt` are rejected with a usage message and exit 1.

Exit code: non-zero when the underlying domain executor fails, when all domains in a parallel dispatch fail, or when any management command fails. Callers may rely on exit code to detect failure. {#260429-ws-ask-api-exit-code}

> [!note] Constraints
> - A `.cmd` Windows shim must accompany every Unix script addition under `claude-plugin/bin/`.
> - `ws-ask-api-internal` is not PATH-exposed; it is invoked only by `ws-ask-api`.

## `ai-docs/.deps/` Cache Layout {#260429-api-deps-cache-layout}

Dependency documentation lives under `ai-docs/.deps/<domain>/`. The `.deps` directory name starts with `.` to suppress inclusion in default ripgrep/ag scans. The directory is committed to version control.

```
ai-docs/.deps/
  <domain>/
    README.md          # management contract: source URLs, version detection method, subdomain map
    meta.yaml          # cached-version, source-url, last-fetched (ISO8601)
    scripts/
      detect-version   # reads project files (conanfile.txt, vcpkg.json, …) → prints version string
      fetch            # fetches from source URL and rewrites doc levels
      check-stale      # runs detect-version, compares with meta.yaml cached-version; exit 0=fresh, 1=stale
    l1.md              # concepts and architecture overview
    l2.md              # commonly used API reference
    l3.md              # idioms, patterns, error handling
    <subdomain>/       # created on demand when l3.md is insufficient for the query
      l1.md
      l2.md
      l3.md
    .lockfile          # flock target; content is irrelevant
```

`README.md` and all scripts are authored by the `api-doc-manager` agent on first bootstrap. `meta.yaml` is updated after every fetch.

> [!note] Constraints
> - `ai-docs/.deps/` is never listed in CLAUDE.md. Callers interact only through `ws-ask-api`.
> - Subdomain directories represent on-demand drill-down (equivalent depth to L4). Within them, `l1–l3` mirror the root level convention scoped to that subdomain.
> - `scripts/` files are executable shell scripts; the `api-doc-manager` writes them during bootstrap.

## Pre-Router Domain Resolution {#260429-pre-router-domain-resolution}

A Haiku-tier one-shot agent that maps an incoming prompt to a list of canonical domain names before any domain-level work begins.

Input:
- The caller's prompt text.
- Optional domain hint from the CLI argument.
- Current directory listing of `ai-docs/.deps/` (existing canonical names).

Output: one canonical domain name per line.

Resolution rules:
1. Match against existing `.deps/` directory names first. Fuzzy caller input (e.g. `asio-net`) resolves to the existing `asio` directory.
2. When no existing directory matches, derive a canonical slug from the prompt context.
3. The domain hint is treated as a strong prior but does not suppress additional domains when the prompt spans multiple libraries.

Invoked only when no domain hint is given, or when the hint does not exactly match an existing `.deps/` directory. Exact-match detection is performed by `ws-ask-api` at the bin-tool level before the pre-router is called.

`ws-ask-api` launches per-domain executors in parallel for each name in the output list and concatenates their responses.

> [!note] Constraints
> - Pre-router is always one-shot; it holds no session state.
> - `workflow-for-agent.md` is not injected (`--no-doc-system`).

## Per-Domain Executor Session {#260429-per-domain-executor-session}

Each canonical domain runs as a persistent `ws-named-agent` session named `api-doc-<domain>`. Session persistence is tied to the canonical domain name, not to the caller's invocation style — any `ws-ask-api` call that resolves to the same domain resumes the existing session.

Executor behavior per call:
1. Acquire the per-domain lock (see below).
2. Run `scripts/check-stale`. If stale or the domain directory is absent: run `scripts/fetch` (bootstrap on first call).
3. Load relevant doc levels (`l1.md`, `l2.md`, and selectively `l3.md` or subdomain files).
4. Answer the prompt. Release lock.

Session compression follows the standard `ws-call-named-agent` 120K auto-compression mechanism. Executor sessions are not erased at sprint wrap-up; they persist until explicitly erased via `ws-named-agent erase api-doc-<domain>`.

> [!note] Constraints
> - `ws-ask-api-internal` checks the registry before calling `ws-new-named-agent`; it calls `new` only when no registry entry exists for the domain. Calling `new` on a live session overwrites the registry and destroys session state.
> - Direct reads or writes to `ai-docs/.deps/` by agents other than the per-domain executor are not permitted.

## Per-Domain Lock {#260429-per-domain-lock}

`ws-ask-api-internal` acquires an exclusive advisory lock on `.deps/<domain>/.lockfile` before any read or write operation for that domain. Lock implementation:

- Unix: `flock -x -w 60 <lockfile>` — OS releases the lock automatically on process exit; no stale lock risk.
- Windows: atomic `mkdir .deps/<domain>/.lock` with EXIT trap `rmdir`; 60-second poll timeout.

Lock granularity is per domain. Concurrent `ws-ask-api` invocations targeting different domains proceed in parallel without contention.

On lock timeout: `ws-ask-api-internal` exits non-zero with the message `lock timeout: <domain> is being updated by another agent`.

> [!note] Constraints
> - Lock scope covers both reads and writes. There is no shared read mode — any access may trigger a fetch.
> - The lock is acquired by the bin tool layer, never delegated to an agent tool call.

## Worker Agent Contract {#260429-worker-agent-contract}

Agents in downstream projects obtain external API information exclusively through `ws-ask-api`. Direct use of `WebSearch` or `WebFetch` for API documentation lookup is prohibited.

`workflow-for-agent.md` and the `ws:workflow` skill prominently surface `ws-ask-api` so agents encounter it before considering web browsing. `ai-docs/.deps/` is not referenced in `CLAUDE.md` or any agent-visible project index; agents have no path to the cache except through the tool.
