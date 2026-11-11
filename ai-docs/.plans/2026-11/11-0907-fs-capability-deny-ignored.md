# fs-capability-deny-ignored

## Intent

User decision (2026-11-11 whiteboard, item 138 option c): the HTML preview's
capability route keeps the whole session root as its read scope, so relative
assets such as `../assets/x.css` keep working, but it refuses to serve any
path that is gitignored or that has a dot-named segment (`.env`, `.git/...`,
`.ssh/...`, `ai-docs/.plans/...`). Secrets usually live in exactly those
places, and a previewed HTML file runs script that could otherwise fetch them.
Option b, narrowing the scope to the previewed file's folder, was rejected
because it breaks documentation sites that load assets from a parent folder.

## Scope

- `agents-plugin-tool/internal/wsdashboard/fs_capability.go`,
  `handleFSCapability`: after the containment check, answer 404 (the route's
  single refusal shape) when:
  - any segment of the root-relative path starts with `.`, or
  - git reports the path as ignored.
- Reuse the existing ignored-path logic (`gitIgnored` and `badgeGitCommand`
  in `fs_routes.go`, with its timeout) instead of a second git invocation
  style. Fail closed: if the ignore check errors or times out inside a repo,
  refuse the read. A root that is not a git repository has no ignored set,
  so only the dot rule applies there.
- Also apply the rule where the capability is minted (the preview route
  that calls `fsCapabilityURL`/`mintFSCapability`). A previewed HTML or SVG
  file whose own path the rule denies gets no capability and shows as source.
  This matches how a file from a changed root already shows.
- Update the file comment's bullet list to state the new refusal.
- Tests in `fs_capability_test.go`, and in the preview route's tests if the
  mint path changes. Cover:
  - a dot file (`.env`), a file under a dot folder, and a gitignored file are
    all 404 through a valid capability;
  - a normal sibling asset and a `../`-style parent-folder asset still serve;
  - a preview of an HTML file inside a dot folder shows source, not a
    capability frame;
  - a non-repo root still serves normal files and still refuses dot paths.

## Verification

Remote only; never run heavy go build or test locally:
`~/.local/bin/ws-rgo -C agents-plugin-tool -m 15 -- test ./internal/wsdashboard/ ./cmd/ws-mcp/`

## Boundary

Commit on branch `fix/fs-capability-deny-ignored` in this worktree. Return to
the lead without committing if the change needs:

- a new config knob,
- a change to the explorer listing's ignored semantics, or
- a change to any route other than the capability route and its mint site.
