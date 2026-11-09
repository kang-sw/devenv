# Merge develop into the file-mention impl branch

Outcome: branch `impl/develop/exes-drier-awoke` (ticket 261109-feat-ws-dashboard-composer-file-mention, closed) merges cleanly into `develop`.

- `develop` gained the subagent badge (merge c80edb923, ticket 261109-feat-ws-dashboard-subagent-badge). Both branches edit the composer: `assets/dashboard.tmpl`, `dashboard.js`, `dashboard.css`, `pages_test.go`, and the 11 i18n tables.
- `git merge-tree` reports content conflicts in `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl` and `agents-plugin-tool/internal/wsdashboard/pages_test.go`; the rest auto-merge.
- Merge `develop` into this branch (a merge commit, no rebase, no stash). Keep both features: the badge row (interrupt badge + subagent badge sharing a three-column row on the composer's top border, interrupt badge hidden by visibility) and the file-mention popup/pill layer (assets/mention.js inlined before dashboard.js). Check auto-merged files for semantic interaction too (composer layout CSS, key handlers in dashboard.js, i18n keys from both sides all present).
- Verification: `go vet ./internal/wsdashboard/ ./internal/wsi18n/`, `go test ./internal/wsdashboard/ ./internal/wsi18n/ ./internal/wsagent/ -count=1`, `node --check` on assets/mention.js and assets/dashboard.js.
- Return when the merge commit is on the branch and verification passes, or when a conflict needs a product decision.
