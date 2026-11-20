# Brief: raise the agy model-wait stall timeout default from 45s to 120s

## Outcome

The owner directed that the hosted agy "model stream stalled" watchdog
default change from 45 seconds to 120 seconds in the ws package. The retries
default (3) and the wsflow default (0, off) stay as they are. Behavior is
otherwise unchanged.

## Known sites

- agents-plugin-tool/internal/wsagent/agy_watchdog.go:19:
  `DefaultAgyModelWaitTimeoutSeconds = 45`
- agents-plugin/agy-config-manifest.json: the
  `agy.model_wait_timeout_seconds` default. Leave
  agents-plugin-wsflow/agy-config-manifest.json at 0.
- agents-plugin-tool/internal/mcp/config_manifest_parity_test.go:189, which
  pins 45.
- ai-docs/manuals/ws-mcp.md:~1961, which says "default 45 in ws".
- Grep the whole repo (excluding ai-docs/tickets/.done, ai-docs/.plans and
  ai-docs/whiteboards) for other copies of this default: other manuals, Pi
  package mirrors, playbooks or skills that mention 45s, and tests using
  DefaultAgyModelWaitTimeoutSeconds or a literal 45 for this purpose. Update
  every live copy together. Do not change tests that set their own explicit
  timeout.

## Compatibility

A manifest default change: T0. Each binary reads its own package's
manifest, and nothing stored changes. Confirm by reading where the manifest
default is consumed, and cite it.

## Permitted actions

- Edit the files above and any mirror or pin you find.
- One commit on impl/develop/agy-model-wait-120, with `## AI Context` and no
  Co-Authored-By line.
- No push, no branch switch, no daemon, no Electron, no real agy binary.

## Verification

Run every Go command from agents-plugin-tool/ with the prefix
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local:
- `go build ./...`
- `go vet ./internal/wsagent ./internal/mcp ./cmd/ws-mcp`
- `go test -count=1 -timeout 20m -run 'Manifest|Parity' ./internal/mcp`
- `go test -count=1 -timeout 20m -run 'Agy|Watchdog|ModelWait' ./internal/wsagent`
- Any narrow test that pins a file you touched. Never run the whole
  ./internal/mcp or ./internal/wsdashboard package.

If you touched agents-plugin-pi, also run its suite:
`npm ci --ignore-scripts && npm test` there. There are 17 known pre-existing
failures; compare failing test names against develop.

## Return boundary

Return after the single commit, or earlier if the change needs more than a
default value. Report:
- the commit sha
- every file changed, with before and after
- every command run, with its result
- the compatibility citation
