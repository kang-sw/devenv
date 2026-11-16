# Whiteboard: two test regressions on develop after the panel diff gutter merge

At develop `76705eb1a`, `go test ./internal/wswhiteboard -count=1` fails:

```
--- FAIL: TestServedTopicBodyRendersInASandboxedFrame (0.00s)
    regions_test.go:329: page holds "srcdoc"
--- FAIL: TestStandaloneClosedTopicWithHistoryKeepsBodyCollapsed (0.01s)
    stream_test.go:463: closed topic missing
```

Both pass at `ab2e32876`, which is before these merges:
- `f9c9e858f`: panel diff gutter (`c2e4bcf99`, plan `ai-docs/.plans/2026-11/16-2149-panel-diff-gutter.md`);
- `6abc5bc49`: workspace picker;
- `afe6c7aab`: select-all;
- `76705eb1a`: compact topic-state pills (`0c121e546`, `b288e97cf`, plan `16-2200-topic-state-compact.md`).

The topic-state change added `title="closed"` to the closed topic `<section>` tag on purpose; its follow-up updated one test for that. It may not have updated `stream_test.go`'s section regex expectations.

## What to do

1. Bisect across the four merges to find which change breaks each test.
2. **`srcdoc` on the standalone page:** the test asserts that a non-framed page has no `<iframe` or `srcdoc` text at all. If the panel diff script, or another merged inline script, contains the literal word `srcdoc` (for example, to reload a framed panel when compare toggles), decide which side is right:
   - If inline script legitimately references `srcdoc`, the assertion should target markup, not script text. Narrow it to attributes and elements, for example ` srcdoc=` and `<iframe`, and keep the intent: no frame is rendered for an unframed page.
   - If markup really emits a frame or `srcdoc`, fix the product.
3. **"closed topic missing":** the test's regex `<section class="topic[^>]* data-topic="t"[^>]*>` fails to match. Check how the closed section tag renders now, for example whether the attribute order changed or `title` was added before `data-topic`. Update the expectation to the intended markup, or fix the template if the change was unintended. The test's real assertions must still hold: a closed topic keeps its body collapsed and its history visible.
4. Keep every test's intent. Do not delete assertions.

## Verification

- `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wswhiteboard -count=1`: the whole package is allowed and must be green.
- `go test ./internal/wsdashboard -run 'TestDashboardFrame|TestWhiteboard' -count=1` (narrow).

## Common rules

- The module is `go 1.23`. Never run the whole `./internal/wsdashboard` or `./internal/mcp` package.
- Do not ssh, do not launch GUI, do not restart the shared daemon, do not run `scripts/dev.py rerun`.
- Public repository: no machine-local paths, hostnames, PIDs, or session keys in commits.
- Commit on your branch only with `## AI Context`; do not merge.
- Return: commits, which merge caused each failure, tests run with results.
