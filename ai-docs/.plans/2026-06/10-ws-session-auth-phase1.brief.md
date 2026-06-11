# Brief: ws session-auth model — Phase 1 (additive)

Ticket: `260609-refactor-ws-spawn-runtime-deletion-session-auth` (M3, epic 260605). Phase 1 ONLY.

## Intent

Introduce the ephemeral session-auth model **additively**, alongside the existing
actor/`sessionRoot` model. Add a lead login verb that mints an LLM-friendly
word-chain session key, a concurrency-safe in-memory `{key → root context}`
registry, mandatory-acceptance of a per-call `session_key` that authoritatively
resolves the call root, an `unknown_session → re-login` contract, and a reserved
optional capability/role-scope parameter with a minimal demonstrable gate. The
existing actor model and keyless root resolution keep working unchanged so callers
can migrate before the actor model is deleted in Phase 2.

## Scope Boundary

In scope (Phase 1):
- New generic word-chain key generator package (`internal/wskey`), separate from auth policy.
- New `ws.lead.login` MCP tool (namespace `ws.lead.*`) → returns a word-chain session key.
- Concurrency-safe in-memory session registry `{key → {root, capability scope}}`.
- `session_key` argument accepted on root-aware tools; when present it authoritatively
  resolves the call root (highest priority), bypassing the legacy resolver chain.
- `unknown_session` structured error + re-login contract when a passed `session_key` is unknown.
- Optional `capability`/`role_scope` param on login, stored, with a minimal keyed-path gate
  (reuse existing `roleAllowsTool`) so a scoped key demonstrably restricts tools.
- Integration + unit tests.

Explicitly OUT (later phases — do NOT touch):
- Deleting actor / authority / child-actor machinery, `gemini` runner, `subquery` runtime,
  reshaping `agents.*` into the mercenary surface (Phase 2).
- Removing `api.*` spawn/async (M4-owned).
- Exec statelessness rework, the full role-containment fold replacing `WS_MCP_TOOL_PROFILE`,
  dashboard build-fix (Phase 3).
- Hard-rejecting keyless calls. Phase 1 stays additive: keyless calls still fall through
  to the existing resolver chain (actor model intact). Mandatory-key enforcement that
  removes the keyless path is a Phase 2 migration concern.

## Caller-Visible Contract

- `ws.lead.login(root, capability?)`:
  - `root` (required): absolute repo path; canonicalized via the existing `canonicalGitRoot`.
  - `capability` (optional, reserved): a role/scope string. First cut accepts/stores it and
    honors a single mapping to the existing `toolRole` (omitted ⇒ `lead`, i.e. unrestricted).
  - Returns (text default; JSON when `format: "json"`): the minted `session_key` and the
    resolved canonical `root`.
- `session_key` argument on root-aware tools: when present and known, the call resolves its
  root from the registry entry (not from `sessionRoot`/host metadata/server root). Concurrent
  calls bearing distinct keys resolve distinct roots with no clobber.
- `unknown_session` contract: when `session_key` is present but not in the registry, the tool
  returns a structured error identifying `unknown_session` and instructing the caller to
  re-login via `ws.lead.login(root)` with its known root and retry. The error text must name
  the recovery verb. This is a `toolTextResponse` error (isError:true), not a JSON-RPC error.
- Capability gate: a `session_key` whose stored scope maps to a non-lead `toolRole` is gated by
  the existing `roleAllowsTool(scopeRole, toolName)` on the keyed `tools/call` path; a denied
  tool returns the same profile-style rejection shape already used for role denial.
- Key format (opaque to callers, but specified): 4 words + a 2-digit numeric suffix joined by
  `-`, e.g. `amber-tide-fox-river-42`. Callers MUST treat the key as opaque (do not parse it).

## Contract Instructions

Files / modules:
- NEW `internal/wskey/` package:
  - `eff_large_wordlist.txt` — ALREADY VENDORED in this branch (7772 pure-`[a-z]+` words, one
    per line, derived from the EFF large diceware list; the 4 hyphenated source entries
    `drop-down/felt-tip/t-shirt/yo-yo` were excluded so every token is hyphen-free and the `-`
    separator is unambiguous; pool size is ergonomic per the ticket decision, not load-bearing).
  - `wskey.go` — `//go:embed eff_large_wordlist.txt` (pattern mirrors `internal/wsprompt/prompts.go`
    and `internal/wsdoc/conventions.go`). Parse the embedded list once (package-level, split on
    newlines, trim, drop blanks). Use `crypto/rand` for word + digit selection (mirror the
    existing `crypto/rand` idiom at `internal/execjob/execjob.go:712`). Public API, generic and
    policy-free:
    - `func Generate() (string, error)` — mint one key (4 random words + 2-digit suffix).
    - `func GenerateUnique(exists func(string) bool) (string, error)` — mint, re-rolling while
      `exists` reports collision; `exists` is the caller's predicate (the session registry passes
      its own membership check, keeping the generator decoupled from auth state). Bound the
      re-roll loop and return an error if it cannot find a free key (defensive; effectively never).
  - The package MUST NOT import the mcp/session/auth packages (it produces strings only).
- NEW `internal/mcp/session_auth.go` (or similarly named) in the `mcp` package:
  - A `sessionRegistry` type: `sync.RWMutex` + `map[string]sessionEntry` where
    `sessionEntry{ root string; scope toolRole }`. Concurrency-safe. Methods: `mint(root, scope) (key, error)`
    (uses `wskey.GenerateUnique` with a membership predicate under the lock — take care to avoid a
    lock-ordering deadlock: generate the candidate, then check+insert under the write lock, looping
    if taken), `lookup(key) (sessionEntry, bool)`. No logout/eviction.
  - Add the registry to the `Server` struct (`internal/mcp/server.go:31`). Initialize in `NewServer`.
- `ws.lead.login` dispatch:
  - Register schema in `tools()` and a dispatch case in `callTool` (`internal/mcp/server.go:328`).
    Follow the mcp-runtime "Add an MCP tool" recipe: schema in `tools()`, dispatch in `callTool`,
    `runtime.json` update, profile/visibility considered, tests. ALSO update `agents-plugin/runtime.json`
    (and any wsflow runtime contract that enumerates the lead tool surface) per the launcher
    compatibility-check coupling — confirm which `runtime.json` the launcher checks before editing.
  - `ws.lead.login` does NOT participate in the setup fence (`isSetupFenceRequest`). The key is
    returned in the response and carried explicitly on later calls, so there is no shared-mutable-state
    ordering dependency. Leave the existing `ws.setup` fence untouched (Phase 2 removes it).
  - Namespace: respect the existing namespace override mechanics (`RuntimeNamespace()`/`WS_MCP_NAMESPACE`)
    the same way other `ws.*` tools do; the literal default is `ws.lead.login`.
- Keyed root resolution:
  - In `resolveToolRoot` (`internal/mcp/server.go:2044`), BEFORE the existing chain: if a
    `session_key` argument is present (non-empty string), look it up in the registry. Found ⇒ return
    its root. Present-but-unknown ⇒ return the `unknown_session` error. Absent ⇒ fall through to the
    existing chain unchanged (additive). `resolveToolRoot` does not currently see the registry — thread
    it via the `Server` receiver (it is already a method on `*Server`).
  - Keyed capability gate: on the `tools/call` path, when a known `session_key` carries a non-lead
    scope, enforce `roleAllowsTool(scope, toolName)` and reject denied tools. Keep this minimal and
    additive; do not refactor the existing `s.role`/`roleAllowsTool`/profile machinery.
- Forbidden: no SQLite/`wsstore` persistence for sessions (in-memory only); no runtime network fetch
  of the wordlist; no hand-enumerated word pool in source; no temporary/mock key generator; do not
  weaken or remove the existing actor path or the `ws.setup` fence.

## Integration Test Instructions

- Unit tests `internal/wskey/wskey_test.go`:
  - Format: minted key matches `^[a-z]+(-[a-z]+){3}-[0-9]{2}$` (4 words + 2 digits).
  - Pool loaded: parsed word count == 7772 and all words are pure `[a-z]+`.
  - `GenerateUnique` re-rolls: a predicate returning true for the first N candidates yields a key
    not in the seen set (drive determinism by seeding the predicate, not the RNG).
  - go:embed content is compiled in and tracked by the build cache, so `-count=1` is NOT required
    for wskey tests. (The `-count=1` caveat applies to tests that read the on-disk `rsrc/` tree at
    runtime, not to embedded assets.)
- Integration tests at the MCP tool boundary in `internal/mcp/` (extend `server_test.go` or add
  `session_auth_test.go`):
  1. `ws.lead.login` returns a word-chain-formatted key and the canonical root; two logins for
     distinct roots return distinct keys mapping to distinct roots.
  2. A root-aware tool call carrying a valid `session_key` resolves that key's root, independent of
     and not clobbering `sessionRoot`; two concurrent calls with distinct keys resolve distinct roots.
  3. A call carrying an unknown `session_key` returns the `unknown_session` error naming the re-login
     recovery verb.
  4. A capability-scoped key (non-lead scope) is denied a lead-only tool via the keyed-path gate,
     while a default (lead) key is allowed — demonstrating "a capability-scoped key restricts the
     intended tools."
  5. Additive guarantee: a keyless call still resolves through the existing chain (actor/`sessionRoot`
     path unchanged).
- Run: `cd agents-plugin-tool && go test ./internal/wskey/... ./internal/mcp/...` (add `-count=1` only
  if a test reads the runtime rsrc tree). Also `go build ./...` and `go vet ./...` clean.

## Implementation Strategy Decisions

- Additive only: keyed path is the highest-priority resolver branch; keyless path is untouched.
- Generator/policy separation is a hard boundary: `internal/wskey` produces strings and knows nothing
  about roots, auth, or capabilities; the registry owns uniqueness predicate + scope.
- Reuse before adding: `canonicalGitRoot` for root canonicalization; `crypto/rand` idiom from execjob;
  `go:embed` idiom from wsprompt/wsdoc; `roleAllowsTool`/`toolRole` for the capability gate;
  `toolTextResponse` for the `unknown_session` error; existing `format: "json"` response helpers.
- `unknown_session` is a stable caller-visible contract token; keep the wording recovery-actionable.
- Backend-swap invariance: the caller-visible contract (`login(root)→key`; `<tool>(key,…)`;
  re-login-on-reject) must not leak that the backend is an in-memory map, so a later persistent
  backend is a pure implementation swap.

## Rejected Alternatives

- Runtime REST fetch of the wordlist — rejected (offline/CI/airgap regression + supply-chain risk;
  login is the per-session bootstrap verb). Embed at build time.
- Hand-enumerated word pool in source — rejected (token-wasteful, error-prone).
- Pool-size-as-correctness — rejected; mint-time uniqueness check is the correctness mechanism, so
  excluding the 4 hyphenated words (7776→7772) is immaterial.
- Hard-rejecting keyless calls now — rejected for Phase 1 (would break the still-live actor model);
  deferred to Phase 2.
- SQLite/wsstore session persistence — rejected (in-memory map; bounded tiny rows; no logout/eviction).

## Approach

1. Vendor wordlist (DONE) + write `internal/wskey` generator with embed + crypto/rand + uniqueness.
2. Add `sessionRegistry` type + `Server` field + `NewServer` init.
3. Add `ws.lead.login` schema + dispatch (mint key, store {root, scope}, return key+root).
4. Branch `resolveToolRoot` on `session_key` (found → root; unknown → unknown_session; absent → fall through).
5. Add the keyed-path capability gate via `roleAllowsTool`.
6. Update `runtime.json` lead tool surface.
7. Unit + integration tests; build/vet/test clean.

## Constraints

- Phase 1 hard boundary (see Scope Boundary OUT list). Additive; do not regress the actor model.
- AI-authored code/comments in English.
- Follow mcp-runtime "Add an MCP tool" + "tools()/callTool both reviewed" + runtime.json coupling.
- Do not strip any spec `🚧` markers (the M3 spec stems span all three phases).

## Out of scope

See Scope Boundary OUT list. Notably: actor-model deletion, mercenary reshape, api.* removal,
exec rework, full role-containment fold, dashboard fix, keyless hard-rejection.

## Details

- `Server` struct: `internal/mcp/server.go:31` (`root`, `rootMu sync.RWMutex`, `sessionRoot`, … —
  add `sessions *sessionRegistry`).
- `callTool`: `internal/mcp/server.go:328`; dispatch switch on `params.Name`.
- `resolveToolRoot`: `internal/mcp/server.go:2044` (explicit `root` → `sessionRoot` → host workspace →
  server root → env). Insert the `session_key` branch at the very top.
- `roleAllowsTool`: `internal/mcp/server.go:3095`; `toolRole` enum `roleLead/roleDelegate/roleLeaf`
  at `:46`.
- `canonicalGitRoot`: `internal/mcp/server.go:2084`.
- exec key idiom: `internal/execjob/execjob.go:712` (`crypto/rand` + hex).
- embed idiom: `internal/wsprompt/prompts.go:18`, `internal/wsdoc/conventions.go:11`.
- Tool-add recipe + runtime.json coupling: mental-model `mcp-runtime` "Add an MCP tool" / "Coupling".

## Verification Contract

- `go build ./...`, `go vet ./...` clean (no new warnings).
- `go test ./internal/wskey/... ./internal/mcp/...` passes, including the 5 integration cases + wskey
  unit cases above.
- Concurrent distinct-root keyed calls demonstrably do not clobber (test 2).
- Unknown key yields the `unknown_session` re-login contract (test 3).
- Capability-scoped key restricts intended tools (test 4).
- Keyless path unchanged (test 5) — additive guarantee.

## References
<!-- [Must] entries: read before starting. [Maybe] entries: consult if uncertain. -->
- `ai-docs/mental-model/mcp-runtime.md` — [Must] server protocol surface, root resolver chain,
  Add-an-MCP-tool recipe, tools()/callTool + runtime.json coupling, fence semantics.
- `ai-docs/mental-model/named-agent-runtime.md` — [Must] WS_MCP_TOOL_PROFILE non-functional finding;
  containment must be a server-side keyed-handler role check, not schema/env filtering.
- `ai-docs/mental-model/prompt-bundle.md` — [Maybe] embed/rsrc loading idioms.
- `ai-docs/spec/mcp-tools.md` `#260610-ephemeral-session-auth-model` — [Must] the planned 🚧 contract
  this phase begins to realize (do not strip the marker).
