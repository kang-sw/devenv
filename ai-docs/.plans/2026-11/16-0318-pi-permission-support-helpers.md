# Pi permission fallback and support availability helper consolidation

## Authority and goal

MAIN assigned one bounded follow-up task on 2026-11-15 at 18:13:18Z and released it after the permission Phase 2a merge report at 18:18:14Z. Consolidate two duplicated predicates without changing behavior. Phase 2a is merged as develop ca5e86689; this task starts from current develop ea80c8103866ef1cfcc05fc33a6886b20144af72 on refactor/pi-permission-support-helpers.

Resolve these existing review follow-ups in the commit:
- savior-rubdown-bubbling: Pi legacy permission fallback repeated across registration.go, profile.go and runner.go.
- undress-clapping-managing: ChoiceLister plus executable availability predicate repeated in support.go.

## Scope

Production scope is agents-plugin-tool/internal/wsagent/{registration,profile,runner,support}.go, plus narrowly related wsagent tests where needed. Re-read the actual moved sites on this base. Reuse existing helpers where appropriate; add only unexported helpers, not a new type, exported symbol, module or external contract. If Phase 2a already removed a duplicate, report the source evidence rather than recreating a change.

Preserve the exact Pi compatibility fallback, extension backend semantics, support ordering and availability/error behavior. This is a transparent T0 internal refactor, not a protocol/schema/tool-result/refusal/stored-shape change. Preserve all Phase 2a admission, native mode, selection and replay behavior. No ticket, dashboard UI asset, worker lineage, permission policy, daemon or packaging changes.

## Verification

Read all applicable AGENTS.md instructions and implementation manuals before editing. Use GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local for Go commands. Run from agents-plugin-tool:
1. go test ./internal/wsagent
2. go test ./internal/wsagent -race -count=1 -run '^TestPermission'
3. go test ./internal/wsdashboard -count=1 -run 'Support|Choices'
4. go build ./...
5. go vet ./...
6. git diff --check

All commands must succeed, and focused selectors must select tests. The old report of 128 stale wsagent failures is obsolete: MAIN reports current develop is green. Treat any failure as a regression and report its exact evidence; do not expand into unrelated repairs. Never run the full dashboard, full mcp or repository/module test suite. No provider/harness launch, external network, install/download, Windows SSH, daemon operation, push or CI. Test-owned fake subprocesses and local fixtures are permitted.

## Delivery

Commit one logical refactor including this immutable plan. Use workflow Git commit with expected_branch refactor/pi-permission-support-helpers, AI Context explaining behavior preservation, and resolves containing both exact follow-up IDs. Keep unrelated files out. Report full head SHA, actual helper/call-site source citations, verification results, any omitted scope and clean status. MAIN alone integrates develop; retain the pooled lane until its merge report. Do not start another task.
