---
title: ws-ask-api — 2-layer API documentation cache system
spec:
  - 260429-ws-ask-api-tool
  - 260429-api-deps-cache-layout
  - 260429-pre-router-domain-resolution
  - 260429-per-domain-executor-session
  - 260429-per-domain-lock
  - 260429-worker-agent-contract
related-mental-model:
  - executor-wrapup
  - personal-devenv
---

# ws-ask-api — 2-layer API documentation cache system

## Background

Downstream project agents frequently need external library API information (e.g. ASIO, Boost, gRPC). Without a managed system, agents browse the web directly — producing inconsistent, unaudited, non-reusable results and leaking web-browsing latency into implementation sessions.

This ticket builds a filesystem-backed API doc cache under `ai-docs/.deps/` and a `ws-ask-api` bin tool that downstream agents use as their sole interface. The cache is structured per-domain with complexity levels (l1–l3 + on-demand subdomain drill-down), managed autonomously by an `api-doc-manager` agent that bootstraps, queries, and updates docs. A lightweight Haiku pre-router normalizes caller domain names and enables parallel cross-domain queries. Worker agents are directed to `ws-ask-api` via `workflow-for-agent.md` and `ws:workflow`; they have no visibility into the `.deps/` structure.

## Decisions

- **2-layer design**: pre-router (Haiku oneshot) resolves canonical domain names before any per-domain work, preventing aliasing (`asio-net` → `asio`) and enabling parallel executor dispatch. Rejected: passing domain name directly from caller — agents would produce inconsistent canonical names.
- **Persistent per-domain sessions**: executor sessions are named `api-doc-<domain>` and survive across invocations using `ws-named-agent` auto-resume. Rejected: one-shot per call — loses loaded context and re-fetches unnecessarily.
- **`.deps/` dot-prefix**: suppresses default ripgrep/ag scans. Cache is committed to VCS (version-pinned knowledge). Rejected: global `~/.claude/api-deps/` — not reproducible across machines.
- **Lock at bin-tool layer**: `ws-ask-api-internal` acquires the flock before the agent call, not delegated to an agent tool call. Any access (read or write) holds the lock; rw distinction dropped since stale checks can always trigger a write.
- **Subdomain = on-demand drill-down**: `<domain>/<subdomain>/l1-l3.md` created by the executor when root l3.md cannot answer the query. Subdomain dirs represent L4 depth; within them, l1–l3 mirror the root convention scoped to that subdomain.

## Constraints

- Every new bin script under `claude-plugin/bin/` requires a Windows-compatible `.cmd` shim (CLAUDE.md architecture rule).
- `ws-ask-api-internal` must check the `ws-named-agent` registry before calling `ws-new-named-agent` — calling `new` on a live session overwrites its UUID and destroys session state.
- `ai-docs/.deps/` must not appear in CLAUDE.md or any agent-visible project index.
- Lock timeout: 60 seconds; emit `lock timeout: <domain> is being updated by another agent` on failure.

## Phases

### Phase 1: `api-doc-manager` agent prompt

Write `claude-plugin/infra/prompts/api-doc-manager.md` — the per-domain executor agent.

Behaviors to implement:
- **Bootstrap** (domain absent): web-search for official docs, fetch and parse into l1–l3.md, write `README.md` (management contract: source URLs, version detection method, subdomain map), write `meta.yaml` (cached-version, source-url, last-fetched), write `scripts/detect-version` (parses project files like conanfile.txt / vcpkg.json / CMakeLists.txt → prints version string), `scripts/fetch` (re-fetches and rewrites doc levels), `scripts/check-stale` (runs detect-version, compares with meta.yaml cached-version; exit 0=fresh, 1=stale). All scripts must be executable shell.
- **Query** (domain fresh): load l1.md + l2.md first; load l3.md and/or subdomain docs only when the prompt requires deeper specificity. Return a structured answer citing which doc sections were used.
- **Update** (domain stale): run `scripts/fetch`, update doc files in-place, update `meta.yaml`, then answer.
- **Subdomain bootstrap**: when l3.md is insufficient, create `<domain>/<subdomain>/` with its own l1–l3.md and add the subdomain to `README.md`'s subdomain map.

Agent must operate with `--no-doc-system` suppressed (i.e., inject `workflow-for-agent.md` by default) since it needs doc-layer orientation to write README.md and scripts/ correctly.

Suggested approach: the prompt should declare the `.deps/<domain>/` layout as its operational context (not read from outside), so the agent can bootstrap without prior knowledge of the cache.

### Phase 2: `pre-router` agent prompt

Write `claude-plugin/infra/prompts/pre-router.md` — the Haiku domain-resolution agent.

- Model: `haiku` (declared in frontmatter).
- `--no-doc-system` (narrow role, no doc orientation needed).
- Input contract: receives prompt text, optional hint, and newline-separated list of existing `.deps/` directory names.
- Output contract: one canonical domain name per line, nothing else. No prose, no explanation.
- Resolution rules: fuzzy-match hint and prompt context against existing directory names first; derive a new slug (lowercase, hyphens) when no match.
- Domain hint is a strong prior but may be supplemented with additional domains when the prompt spans multiple libraries.

### Phase 3: `ws-ask-api` and `ws-ask-api-internal` bin tools

Write `claude-plugin/bin/ws-ask-api` (Python, like `ws-named-agent`) and `claude-plugin/bin/ws-ask-api-internal`, plus `.cmd` shims for both.

**`ws-ask-api`**:
- Arg parsing: `[<domain-hint>] "<prompt>"` / `--refresh <domain>` / `--check-stale <domain>` / `--list`.
- `--list`: print directory names under `ai-docs/.deps/`.
- `--check-stale <domain>`: run `scripts/check-stale` for that domain and report.
- `--refresh <domain>`: call `ws-ask-api-internal <domain> --force-refresh`.
- Default mode (routing logic):
  1. If `<domain-hint>` given and `ai-docs/.deps/<hint>/` exists as a directory → skip pre-router, call `ws-ask-api-internal <hint> "<prompt>"` directly.
  2. Otherwise → enumerate `ai-docs/.deps/` dirs → call pre-router oneshot (`ws-oneshot-agent -p pre-router --no-doc-system`) with prompt + hint? + dir list → read resolved domain list → spawn `ws-ask-api-internal <domain> "<prompt>"` in parallel for each domain → concatenate outputs to stdout.

**`ws-ask-api-internal <domain> "<prompt>" [--force-refresh]`**:
- Acquire `flock` lock on `ai-docs/.deps/<domain>/.lockfile` (60s timeout, emit message on failure).
- Registry check: if `api-doc-<domain>` entry exists in the `ws-named-agent` registry, call it. If not, call `ws-new-named-agent api-doc-<domain> -p api-doc-manager` first.
- Deliver prompt to the executor session via `ws-call-named-agent api-doc-<domain> "<prompt>"`.
- Release lock after the call returns.
- Windows lock fallback: `mkdir .deps/<domain>/.lock` atomic; EXIT trap `rmdir`.

### Phase 4: `workflow-for-agent.md` and `ws:workflow` integration

Update two files to surface `ws-ask-api` to worker agents:

- `claude-plugin/infra/workflow-for-agent.md`: add a clearly marked `## API Documentation` section instructing agents to use `ws-ask-api` for any external library API questions and prohibiting direct `WebSearch`/`WebFetch` for API lookup.
- `claude-plugin/skills/workflow/SKILL.md` (the `ws:workflow` primitives reference): add `ws-ask-api` to the bin tool table with its signature and the same prohibition note.

After both edits, run `install.sh update` is not required here (the workflow skill is loaded via Skill tool, not the snapshot cache path for this content), but note that downstream snapshot users will need `claude plugin update ws@ws`.
