# Brief: 260622-sage-review-config-p3

## Intent
Register the four `sage_review*` config keys in `scope.go` following the
`ItemPreferMercenary` pattern, making them appear in `config.show` output and
ensuring the inline `wsconfig.Resolver` call in `server.go` uses constants
instead of a raw string literal.

## Scope Boundary
Phase 3 only:
- `internal/wsconfig/scope.go`: add 4 Item* constants + 4 `init()` registrations.
- `internal/mcp/server.go`: replace 1 string literal (`"sage_review"`) with
  `wsconfig.ItemSageReview`.
- No new test files; existing `scope_test.go` pattern covers scope registration
  behavior.

Deferred: builtin default value emission (Resolver returns empty when unset;
callers already treat empty as "off"), any config.set/config.prompt changes.

## Caller-Visible Contract
- `config.show` now lists all four keys with scope=builtin and empty value when
  unset, matching the `prefer_mercenary` behavior.
- `tickets.move` resolver now uses the typed constant; behavior is identical.

## Contract Instructions
In `scope.go`:
```go
const (
    ItemPreferMercenary            = "prefer_mercenary"
    ItemSageReview                 = "sage_review"
    ItemSageReviewDesignTier       = "sage_review_design_tier"
    ItemSageReviewCompleteness     = "sage_review_completeness"
    ItemSageReviewCompletenessTier = "sage_review_completeness_tier"
)

func init() {
    RegisterDefaultScope(ItemPreferMercenary, ScopeSession)
    RegisterDefaultScope(ItemSageReview, ScopeProject)
    RegisterDefaultScope(ItemSageReviewDesignTier, ScopeProject)
    RegisterDefaultScope(ItemSageReviewCompleteness, ScopeProject)
    RegisterDefaultScope(ItemSageReviewCompletenessTier, ScopeProject)
}
```

In `server.go`: replace `r.Get(sessionKey, "sage_review")` with
`r.Get(sessionKey, wsconfig.ItemSageReview)`.

## Integration Test Instructions
- `go test ./internal/wsconfig/...` — scope registration + config.show path.
- `go test ./...` — full suite green.

## Implementation Strategy Decisions
- All 4 keys default to ScopeProject explicitly, even though ScopeProject is the
  implicit fallback for unregistered keys. Explicit registration ensures
  `config.show` includes them when unset (see `scoped_show.go:63`).
- No builtin-default-value mechanism needed: Resolver returns Value="" for
  unset keys; callers treat "" as "off" (see checkSageReview and gate logic).

## Rejected Alternatives
- Registering with ScopeSession: wrong — sage_review is a project-level setting,
  not a session-ephemeral toggle like prefer_mercenary.

## Approach
- Edit scope.go: expand const block + expand init() block.
- Edit server.go: 1-line string→constant substitution.
- Run go test ./... to verify.

## Constraints
- Do not add a new test file; extend scope_test.go only if the existing
  RegisterDefaultScope tests do not already cover this.
- Do not change tickets_mutate.go; SageReview field already accepts the resolved
  string value from the Resolver.

## Out of scope
- builtin default value registration
- config.prompt for sage_review* keys
- lead-write-ticket playbook reading the configured tiers (Phase 3 spec is only
  schema registration)

## Verification Contract
- `go test ./...` green.
- `go build ./...` clean.
- `config.show` (manual spot-check): the 4 keys appear with scope=builtin.

## References
- `internal/wsconfig/scope.go` — [Must] ItemPreferMercenary pattern to follow
- `internal/wsconfig/scoped_show.go` — [Must] scopeRegistry enumeration in config.show
- `internal/mcp/server.go:855-871` — [Must] tickets.move dispatch (sage_review string literal)
