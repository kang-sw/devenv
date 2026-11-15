# Pinned summary compatibility source

These fixtures retain the actual historical Go packages and their historical
local dependency closure, rather than approximating old structs or mixing them
with current dependencies. The test injects the same bounded drivers as before.
No production code, compatibility assertion, or CI fetch policy is changed.

| Archive | Original full commit | Files | Compressed bytes |
| --- | --- | ---: | ---: |
| `wslive-9854ff350fa1b0eccfb5c71649c5190c405925b9.tar.gz` | `9854ff350fa1b0eccfb5c71649c5190c405925b9` | 60 | 139584 |
| `wsagent-54b252036f430ddd0886af90ba8de767bc4b1a6a.tar.gz` | `54b252036f430ddd0886af90ba8de767bc4b1a6a` | 361 | 3887840 |

The inspectable `wslive.json` and `wsagent.json` manifests record original
repository paths and SHA-256 digests for every file and each archive. The Go
test checks those digests, the full revision pins, and exact archive membership
before building. Compression avoids hundreds of duplicate source files,
preserves original bytes through Windows checkout conversion, and retains the
embedded assets required by the real mapper's dashboard/whiteboard imports.
There are no compiled executables, caches, downloaded dependencies, or unrelated
package payloads in these archives. External modules use the existing offline cache.

## Source selection

Each archive contains that revision's `agents-plugin-tool/go.mod` and `go.sum`,
plus the union of Go source and embedded files reported by
`go list -deps -test -json ./internal/<package>` for darwin, linux, and windows,
with amd64 and arm64, `CGO_ENABLED=0`, and offline module resolution. The
historical writer needs no existing test files. The mapper retains whole,
unmodified historical helper files needed by `newLeadHarness` and
`piWidgetLine`, including their transitive test-helper references:

```text
children_test.go             claude_lead_plugins_test.go
claude_test.go               cmdshim_test.go
fake_test.go                 http_test.go
lead_bundled_plugin_test.go   lead_fake_test.go
pi_test.go                   profile_test.go
runner_test.go               spawn_test.go
standin_test.go
```

Other historical tests and all dependency-package tests are omitted. All
retained files were loaded with `git show <full-revision>:<original-path>`;
none were rewritten. Tar members are sorted regular files with fixed metadata.
The archive contains no current-checkout dependency substitutions.

## Independent provenance verification

In a full clone containing the two original commits, run:

```sh
python3 agents-plugin-tool/internal/wslive/testdata/historical-summary/verify-provenance.py
```

This compares every decompressed file byte-for-byte with its original Git blob
and checks all manifest hashes. Runtime tests require neither Python nor any
historical Git objects, so shallow CI and source-only checkouts work offline.
