# Receiver verification batch for live summaries

## State and binding

**BLOCK — static scope artifact only. No executable START is authorized.**

- Ticket: `261114-feat-ws-live-reasoning-summary`.
- Worktree: `/Users/kang-sw/.ws-worktrees/devenv/safari-manicotti-litter`.
- Branch: `impl/feat/live-reasoning-summary-675/fruit-annex-hunk`.
- Receiver source binding: `5d018785c8caddc237cf24bb5229c871aac8783b`.
- Schema checkpoint: `a5b05c09a4e41a99c50fd8055108f2df82f8947d`.
- This replaces the provisional broad-pattern command sent before MAIN's
  14:09:50 swap-window restriction. It covers receiver checks only, not the
  Pi executable leaf, full suites, independent review, or visual acceptance.
- A later scoped START must name this artifact and an ownership/cleanup
  supervisor. The current bash tool exposes a timeout but no descendant
  ownership or quiescence receipts. No such supervisor is currently assigned;
  **that unknown blocks execution, rather than authorizing an exploratory run**.
- The swap window must first be explicitly closed by MAIN. Closing that window
  alone is not START. No new Pi/session/child launch occurs while it is active.
- Any source/test/dependency/embed change beyond the explicit gofmt operation
  invalidates this binding. Reconcile statically and obtain a new scoped START;
  do not expand a live batch, retry it, or repair failures inside it.

VERIFIED means static source/metadata evidence here, not a passing executable
check. All implementation and test behavior remains unverified.

## Fixed executable batch

The supervisor uses argv arrays, not a shell or a launcher. CWD is the worktree
for command 1 and its `agents-plugin-tool` directory for commands 2–5. Run each
command once, serially. Stop the batch on the first failure, timeout, unknown
child/owner, changed branch/input, or missing cleanup receipt. No retry, repair,
provider request, install, daemon swap, smoke, merge, or push is included.

Executables, inspected as installed filesystem entries without running them:

- Go: `/opt/homebrew/Cellar/go/1.26.2/libexec/bin/go`.
- gofmt: `/opt/homebrew/Cellar/go/1.26.2/libexec/bin/gofmt`.
- Git: `/Library/Developer/CommandLineTools/usr/bin/git`.
- No Python supervisor executable is selected or authorized by this artifact.

### Command 1: formatting (30 seconds maximum)

Executable: the absolute gofmt path above. Arguments are `-w` followed by exactly
these 23 paths, relative to the worktree:

```text
agents-plugin-tool/internal/mcp/intake_order.go
agents-plugin-tool/internal/mcp/intake_order_test.go
agents-plugin-tool/internal/mcp/live_snapshot.go
agents-plugin-tool/internal/mcp/server.go
agents-plugin-tool/internal/mcp/summary_intake_test.go
agents-plugin-tool/internal/mcp/summary_owner.go
agents-plugin-tool/internal/wsagent/backend.go
agents-plugin-tool/internal/wsagent/pi.go
agents-plugin-tool/internal/wsagent/record.go
agents-plugin-tool/internal/wsagent/runner.go
agents-plugin-tool/internal/wsagent/runner_summary.go
agents-plugin-tool/internal/wsagent/summary_hosted_test.go
agents-plugin-tool/internal/wsagent/wake.go
agents-plugin-tool/internal/wsdashboard/live_hosted.go
agents-plugin-tool/internal/wsdashboard/live_hosted_test.go
agents-plugin-tool/internal/wslive/reasoning_summary.go
agents-plugin-tool/internal/wslive/reasoning_summary_test.go
agents-plugin-tool/internal/wslive/ring.go
agents-plugin-tool/internal/wslive/snapshot.go
agents-plugin-tool/internal/wslive/state.go
agents-plugin-tool/internal/wslive/summary_commit.go
agents-plugin-tool/internal/wslive/summary_commit_test.go
agents-plugin-tool/internal/wslive/summary_compat_test.go
```

No children or listeners are expected. Only these files may change. Record their
post-format hashes before command 2; subsequent commands must not change inputs.

### Command 2: reducer, frozen candidates, compatibility (240 seconds maximum)

```text
/opt/homebrew/Cellar/go/1.26.2/libexec/bin/go test ./internal/wslive -run ^Test(ReasoningSummaryValidation|ReasoningSummaryFreshnessAndRealEnd|ReasoningSummaryLeadOnlyAndAttemptPreservation|SummaryCommitFreezesRetriesAndNeverRecounts|SummaryCommitCapacityNeverEvictsAcceptedCandidate|SummaryImmutableCommitRefusesOverwrite|SummaryHistoricalReaderWriterPreservesOriginalBytes|SummaryPagingPruningAndNoResurrection)$ -count=1 -vet=off -p=1 -parallel=1 -timeout=210s
```

Exactly eight outer tests. The historical test additionally compiles and runs
one test in the archived historical package:

```text
go test ./internal/wslive -run ^TestSummaryHistoricalWriterFixture$ -count=1 -vet=off -p=1 -parallel=1 -timeout=90s
```

That child is bounded by the parent's single two-minute context, which also
covers `git rev-parse --show-toplevel` and `git archive 9854ff350
agents-plugin-tool`. It receives `summaryHistoricalEnv`, an explicit offline
allowlist (`internal/wslive/summary_compat_test.go`). The fixed historical
revision is real prior source, not a simulated old writer. Archive extraction,
new summary records, old-reader/writer append, snapshot reconstruction and
2000-file pruning occur only in test temporary directories. No listener or
holder is expected. No fixture is skipped or substituted on failure.

### Command 3: intake ordering, ownership, filesystem faults (150 seconds maximum)

```text
/opt/homebrew/Cellar/go/1.26.2/libexec/bin/go test ./internal/mcp -run ^TestSummary(IngressTracksAbandonedLegacyAndActualDeferredWrite|IngressFailsClosedWithinAdmissionBudget|IngressFIFOAndUnrelatedToolsRemainIndependent|QuiescenceWaitsForRunningSnapshotCallback|IntakeNegotiatesOnlyOwnedDaemonReceiver|IntakeFreezesRealEndBeforeAcceptanceAndRetriesOnlyCandidate|IntakeRetiredConnectionAndHostedTakeoverCannotWrite)$ -count=1 -vet=off -p=1 -parallel=1 -timeout=120s
```

Exactly seven tests. `newLiveFixture` uses temporary Git repositories;
`initGit` runs only `git init` and three repository-local `git config` writes
(`internal/mcp/server_test.go:647`). Session/state resolution and snapshot repo
reads may run Git read commands (`rev-parse`, `symbolic-ref`, `rev-list`, `status`,
and local configuration/root queries), confined to those temporary repositories.
There is no origin remote, fetch, push, credential-helper use, shell script,
Node, Python holder, or installed harness launch in these selected tests.

The hosted slot in these tests is a controlled in-memory fake; it does not
constitute proof of the real daemon generation implementation. Command 5 covers
that separate implementation. Snapshot timers and RAM retry timers are goroutines
inside `mcp.test`, not detached OS processes. The receiver-fault fixture creates
a regular file where its own temporary ring directory should be, then removes
that owned blocker and explicitly retries the frozen candidate. This is test
behavior, not authorization to repair an operational environment.

### Command 4: hosted mapper and native boundary (150 seconds maximum)

```text
/opt/homebrew/Cellar/go/1.26.2/libexec/bin/go test ./internal/wsagent -run ^TestSummaryHosted(WidgetIsProjectionNotLifecycleAuthority|FinalPrecedesNativeEndAndQuietReplayNeverWrites|NativeAbortReasonIsActualFact)$ -count=1 -vet=off -p=1 -parallel=1 -timeout=120s
```

Exactly three tests. `newLeadHarness` replaces `preparePiLeadLaunch`, `lookPath`
and the holder host; its `/opt/harness/pi` and `/opt/harness/claude` are
non-executed strings (`internal/wsagent/lead_fake_test.go:706–736`).
`fakeHolders` has no process behind it (`lead_fake_test.go:80–84`); its launch,
spawn, rejoin, exit and restart are in-memory fixture operations. No real holder,
Python, Node, Pi, Claude, RPC client, provider, SDK discovery, launcher or daemon
process is authorized by this command. Runner goroutines close through the
registered test cleanup; the test binary's process exit also retires its
in-memory fake streams. `WSAGENT_STANDIN_OUT` must be absent, so the package's
TestMain cannot enter stand-in harness mode (`standin_test.go:22`).

### Command 5: actual daemon slot generation (60 seconds maximum)

```text
/opt/homebrew/Cellar/go/1.26.2/libexec/bin/go test ./internal/wsdashboard -run ^TestHostedSlot(RendersLiveOnly|OwnerFencesActualGeneration|Takeover)$ -count=1 -vet=off -p=1 -parallel=1 -timeout=40s
```

Exactly three tests. These call `StartDaemon` **inside `wsdashboard.test`**, with
an owned `t.TempDir`, `Port: 0`, a fake page source and no Agents, Holders, Proxy,
Support or Worktrees host (`index_test.go:434–445`; `pages_test.go:147–184`).
There is one loopback listener per fixture, not a `ws-mcp` child or shared daemon.
The fixture disables the Git badge. The SSE test opens an owned HTTP connection
and stream; `openStream` registers cleanup, and `startIndexedDaemon` registers
`d.Close`. No `/shutdown`, installed endpoint, provider, browser or page script
is invoked. Rendering templates/CSS does not execute JavaScript. No plan-usage
route is requested; its credential/provider probes are not part of these tests.
`WS_TEST_USAGE_CODEX` must be absent to disable the unrelated test-binary init
stand-in (`plan_usage_probe_test.go:46`).

## Transitive process and resource inventory

| Owner | Allowed descendants/resources | Excluded |
|---|---|---|
| Future supervisor | The five commands above; an explicitly reviewed metadata/ownership helper, if MAIN assigns one | No supervisor is currently assigned; do not guess one |
| gofmt | No process child, listener or holder | Every unlisted executable |
| Go 1.26.2 | Its own `pkg/tool/darwin_arm64/{asm,compile,link,preprofile}` and generated `wslive.test`, `mcp.test`, `wsagent.test`, `wsdashboard.test` binaries under the batch temporary directory | `cgo`, external C compiler/linker, vet, coverage/fix tools, downloaded toolchains, shell, Node, provider executables |
| Outer `wslive.test` | Two read-only Git children, one nested Go invocation and its compile/link/test children; historical checkout and fixture files under `t.TempDir` | Real checkout writes; historical fixture replacement; network/download fallback |
| `mcp.test` | Git init/config/read children over fixture roots; in-process timers and test state | SSH, remote Git, real session store, installed launcher, real daemon |
| `wsagent.test` | In-process fake holders and runner goroutines only | Actual holder/SDK/RPC/harness children; Python; Node |
| `wsdashboard.test` | In-process loopback listeners, SSE/HTTP connections and timers under temporary daemon directories | Shared daemon endpoint/lock, real holder, provider probe, browser or JavaScript execution |

`-p=1`, `-parallel=1`, `CGO_ENABLED=0`, `-vet=off` and `-buildvcs=false` bound
compiler/process fan-out. All test files still compile, but only the exact
anchored names above execute. Unselected Node/Python/browser/provider tests and
package TestMain/init stand-ins are not silently added. An unexpected executable
or listener is BLOCK, even if the top-level command exits zero.

## Environment: construct from empty, never inherit

Create exactly one new private batch root with exclusive creation and mode 0700:
`/tmp/ws-live-summary-675-timothy-unlikable-tingle-01`. If it already exists or
ownership cannot be established, BLOCK; do not remove or reuse it.

The only entries in its private `bin` directory are symlinks `go` and `git` to
the absolute executable paths above. No parent PATH component is inherited.
Make `home`, `tmp`, `gocache`, `config`, `cache`, and `logs` beneath this root.
The allowlisted environment is exactly:

```text
PATH=<batch>/bin
HOME=<batch>/home
TMPDIR=<batch>/tmp
TMP=<batch>/tmp
TEMP=<batch>/tmp
LANG=C
LC_ALL=C
GOROOT=/opt/homebrew/Cellar/go/1.26.2/libexec
GOCACHE=<batch>/gocache
GOMODCACHE=/Users/kang-sw/go/pkg/mod
GOOS=darwin
GOARCH=arm64
GOMAXPROCS=2
GOTOOLCHAIN=local
GOENV=off
GOWORK=off
CGO_ENABLED=0
GOPROXY=off
GOSUMDB=off
GOVCS=*:off
GOTELEMETRY=off
GOFLAGS=-mod=readonly -buildvcs=false
GIT_CONFIG_NOSYSTEM=1
GIT_CONFIG_GLOBAL=/dev/null
GIT_CONFIG_SYSTEM=/dev/null
GIT_NO_REPLACE_OBJECTS=1
GIT_ATTR_NOSYSTEM=1
GIT_TERMINAL_PROMPT=0
GIT_ASKPASS=/usr/bin/false
SSH_ASKPASS=/usr/bin/false
GIT_SSH_COMMAND=/usr/bin/false
WS_CACHE_HOME=<batch>/cache
WS_CONFIG_HOME=<batch>/config
WS_RSRC_ROOT=/Users/kang-sw/.ws-worktrees/devenv/safari-manicotti-litter/agents-plugin/rsrc
```

`<batch>` means only the exact private root above. Cached modules are inputs;
missing dependencies fail offline, with no install/repair. Do not inherit API
keys, auth tokens, `SSH_AUTH_SOCK`, keychain selections, proxy variables,
`NODE_OPTIONS`, `PYTHONPATH`, `DYLD_*`, user Go settings, Git trace/config command
injection, `PI_*`, `CLAUDE*`, `CODEX*`, `WS_HOSTED_*`, `WS_MCP_*`, mailbox/session
credentials, `WSAGENT_STANDIN_OUT`, `WS_TEST_USAGE_CODEX`, or launcher variables.
The two test-only stand-in switches are absent, not forwarded as user values.

No Node or Python child belongs to the selected tests. If MAIN later assigns a
Python ownership supervisor, its absolute executable, complete source, isolated
`-I -S` invocation, imports, subprocess graph, environment and cleanup must be
added to this same artifact before START. Nothing here approves an implicit
Python wrapper or installed harness launcher.

## Ownership registration and cleanup receipts

**These are admission requirements, not claims that receipts already exist.**
The assigned supervisor must register its own identity and every command before
admitting the next command. Receipt records must carry:

- Batch nonce, expected branch/source binding, command ordinal and exact argv,
  CWD, allowed environment key list and environment/input hashes.
- A real owned child handle, PID, PPID, process-group/session identity, and an
  OS creation/ownership discriminator; do not infer identity from PID or command
  name alone. Registration must cover generated test binaries and all nested
  Go/Git children, not just the outer `go test` parent.
- Owned temporary roots, compiled binary paths, historical fixture root, and
  each loopback listener/HTTP connection/fixture-holder owner. An in-memory fake
  holder must be recorded as such, never reported as a stopped OS process.
- Start/deadline/end monotonic times, complete stdout/stderr log paths, exit
  status, and child wait/reap results. A zero parent exit is not a quiescence
  receipt.
- Before each next command and at batch end, the exact owned descendants still
  live, listener/stream close evidence, and cleanup disposition. Unknown owners
  or non-quiescent descendants/listeners produce BLOCK and retain evidence.

The command ceilings sum to **630 seconds**. Reserve **90 seconds** for bounded
cleanup and receipts, giving an absolute **720-second / 12-minute** batch cap.
Do not consume cleanup reserve to retry, repair, build another target, inspect a
provider, or continue the next command. At a failure, stop the command sequence
immediately; cleanup is restricted to identities established by the supervisor.
A signal is allowed only after that particular owner is verified still to be
the registered process. Never signal by a broad name match, guessed PID, stale
process group, or shared daemon identity. If safe ownership or quiescence cannot
be established within the reserve, report BLOCK with the exact retained owner
records and paths; do not send an unverified signal or claim cleanup success.

Remove only proven-owned scratch files after descendants/listeners are proven
quiescent. Keep complete logs and ownership/cleanup receipts as the run's
verification evidence. There is no ownership/cleanup implementation assigned
currently, so this prerequisite remains BLOCK; executing to discover the missing
scope or supervisor behavior is not authorized.

## Still required after this batch

- Pi tests, shipped-bundle/version/override proof and exact 1.5.2 offline source
  and transport fixtures, with their own scoped command/child inventory.
- Remaining receiver/late-call/ownership/fault coverage, full `npm test` and
  `go test ./...`, after static inventory and MAIN's later slots. This focused
  artifact is not a waiver or a completion claim.
- Actual conversation UI captures at 1280 and 360, compared side by side with
  the unchanged 717 reference files, including live, ordinary fallback and
  retained collapsed/expanded states. No visual implementation/capture is
  claimed here.
- One independent lite review and its required fixes, phase Result evidence,
  ticket closure, and a retained implementation-branch report. MAIN alone owns
  integration and any later shared daemon swap.

omitted: All executable verification, runtime/provider activity, new child
launches, full suites and captures are intentionally not performed by authoring
this static artifact. No verification obligation is waived.
