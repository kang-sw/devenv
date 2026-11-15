NEEDS FIX. 1 Critical (C-B class, on the new wrapper surface), 2 Important (superlinear), 0 Minor (the Minor variant is folded into C-1)

## Review findings: panel-first Phase 1 parser repair, 1b7e7cd65..4c875057b (round 4, dynamic)

Method: extracted 4c875057b into /tmp/pfr4. Copied the v0.50.0 parser from testdata/panel-v050 (hash-checked tarball, 4be663726) into a sibling package `internal/wsxmlcmdold` so it runs in-process. Set up the v0.50.0 grammar alongside it. The harness and probes were /tmp-only `_test.go` files in package wswhiteboard, named zz_round4_test.go, zz_round4_perf_test.go and zz_round4_wrap2_test.go. The key snippets are quoted below because /tmp/pfr4 was deleted. Every go command used `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` and ran from /tmp/pfr4/agents-plugin-tool.

Root cause shared by C-1 and I-1: `codeBodyBoundary` always starts with an empty frame stack (literal_code.go:158), and `codeLiteralEnd` (literal_code.go:359) passes it no enclosing context. That stack is correct when E is a leaf or a container field. It is wrong when E is a wrapper-child field (`wb:old`/`wb:new` inside `<wb:change>` inside `<wb:panel>`, read via wsxmlcmd.go:811 → readFields → readContent at wsxmlcmd.go:1150). After `</wb:change>` the walk believes it is at enclosing level, but the real reader is back in the panel's raw wrapper body. The walk then sees the next `<wb:change>` at an empty stack. `wb:change` is not a command and not in `fields`, so literal_code.go:228-238 reads it as an unknown name, and :251-253 pushes it as a RAW leaf frame with the wrong bodyStart, lineStart and close. It should be a wrapper container whose fields are raw frames, which is what literal_code.go:229-232 does when the walk is inside a panel frame.

### Critical

- **C-1: agents-plugin-tool/internal/wsxmlcmd/literal_code.go:158 and :228-253 (the enclosing wrapper frame is lost). A structurally clean, Apply-accepted 2-change named patch is silently merged into 1 change with a different stored payload.**
  The walk's unknown raw `wb:change` frame pops at the first `</wb:change>`, even when that close is plain text inside a later `wb:old` raw field. The walk then believes it is at enclosing level while the real reader is inside that field. A tick there lets the earlier field's candidate be accepted.
  Repro:
  ```
  <wb:topic name="t"><wb:panel name="p"><wb:change><wb:old>a ` b</wb:old><wb:new>c</wb:new></wb:change><wb:change><wb:old>x </wb:change> y ` z</wb:old><wb:new>w</wb:new></wb:change></wb:panel></wb:topic>
  ```
  - Structural reading (same grammar, LiteralCode=false): Parse has 0 problems. It gives 1 panel with 2 changes: old=`a ` b` new=`c`, and old=`x </wb:change> y ` z` new=`w`. Body is "".
  - New Parse and ParseOrdered (0 problems, complete=true): 1 panel with 1 change. old=``a ` b</wb:old><wb:new>c</wb:new></wb:change><wb:change><wb:old>x </wb:change> y ` z`` new=`w`.
  - Control: the same input with no tick in the first old (`a b`) gives 2 changes in new mode, identical to the structural reading. So one unmatched tick silently changes a clean reading. This is the same class as round-3 C-B.
  - Apply on a State seeded by `<wb:topic name="t"><wb:panel name="p" recap="r"><![CDATA[S[a ` b</wb:old><wb:new>c</wb:new></wb:change><wb:change><wb:old>x </wb:change> y ` z]]]></wb:panel></wb:topic>`:
    - literal mode: accepted=1, problems=[]. Stored p1 text becomes `S[w]`.
    - LiteralCode=false: accepted=1, problems=[]. Stored text becomes `S[c</wb:old><wb:new>c</wb:new></wb:change><wb:change><wb:old>w]`.
    - Both are silent acceptances with different immutable payloads.
  - Severity note: v0.50.0 has no wrapper and rejects `name=` (1 Parse problem), so the rubric's literal "v0.50.0-valid" clause does not apply. It is rated Critical on the round-3 C-B precedent: silent merge of input the new grammar parses cleanly without the tick. The lead may re-rate it.
  - Variant, fuzz-found and Minor on its own: e lands in panel-body text after `</wb:change>`. Example: `<wb:topic name="0"><wb:panel name=""><wb:change><wb:old>a ` b</wb:old><wb:new>c</wb:new></wb:change> d ` e</wb:old><wb:new>f</wb:new></wb:change></wb:panel></wb:topic>`. The structural Parse is clean (1 change plus body text) but Apply refuses it as a mixed payload. Literal mode returns 1 change (old swallows `</wb:change> d ` e`) with an empty body, which Apply accepts. This contradicts the design's claim that "enclosing-level text ⇒ F reports a problem" at the Parse level. My I3-with-wrappers oracle flagged 106-188 such cases per 60k run. The committed I3 test excludes `<wb:change` inputs, so it cannot see this.
  - Commands: `go test ./internal/wswhiteboard -run 'TestR4WrapMerge|TestR4WrapApply' -count=1 -v`. These parse the input with new Parse/ParseOrdered, with the structural grammar, and with v0.50.0, then run `Apply(&State{}, seed, Env{})` and `Apply(st, patch, Env{})` with `grammar.LiteralCode` toggled true/false.
  - Fix direction: seed the walk's stack with the enclosing frames of E. At minimum, when E is a wrapper-child field, start with the wrapper raw frame and the `wb:change` container frame, so `</wb:change>` pops to a raw panel frame and the next `<wb:change` becomes a container frame. Add a parser and Apply regression, and add wrapper inputs to I3.

### Important

- **I-1: same root cause (literal_code.go:158, :228-253, raw-frame matcher at :184-205). Quadratic parse on a Parse-clean named-patch shape.**
  Each `wb:old` body opens a tilde fence at its own body start, so the real reader matches a fence. The candidate crosses `</wb:old>` and starts a walk. Inside the walk, the following changes are wrong-context raw `wb:change` frames. In those frames lineStart and bodyStart are after `<wb:change>`, so `<wb:old>~~~x` is not at line start, and a tilde is never inline code. The walk therefore never meets an "ambiguous frame" and runs to the far closer. The disjoint-walk argument (design claim 3) fails.
  Generator: `<wb:topic name="t"><wb:panel name="p">` + repeat(`<wb:change><wb:old>~~~x\n</wb:old><wb:new>n</wb:new></wb:change>`) + `<wb:change><wb:old>\n~~~\n</wb:old><wb:new>n</wb:new></wb:change></wb:panel></wb:topic>`. The input parses with 0 problems in both new modes.
  - Best of 3, Apple M-series:

    | shape | 50KB | 100KB | 200KB | v0.50.0 Parse at 200KB |
    |---|---|---|---|---|
    | wrapper-old-tilde, Parse | 295 ms | 1147 ms | 4660 ms | 0.34 ms |
    | wrapper-old-tilde, ParseOrdered | 293 ms | 1156 ms | 4637 ms | 0.34 ms |
    | wrapper-new-tilde (fence in `wb:new`) | 287 ms | 1157 ms | 4635 ms | 0.31 ms |

    The control shape with `\n```x` (leading newline, backticks) is linear: 1.1/2.4/4.9 ms.
  - Command: `R4K=wrapper-old-tilde,wrapper-new-tilde go test ./internal/wswhiteboard -run 'TestR4Perf$' -count=1 -v`.
  - Fix: the same stack seeding as C-1 restores correct raw field frames, and with them the stop-at-originator bound. Add this shape to BenchmarkLiteralCodeStructuralWalk.

- **I-2: literal_code.go:175-218 (group frames never run the candidate matcher) vs wsxmlcmd.go:515 (ParseOrdered reads a rejected group's body with literal code on). Quadratic in ParseOrdered, and so in whiteboard Apply.**
  In ParseOrdered, a group whose first content is not a command has its whole body read by readBodyContent → readContent with `t.literalCode`. That body is an originator: its fence candidate crosses `</wb:topic>`. The walk models every later `<wb:topic ...>` as a group frame. Text in a group frame is skipped without the raw-frame candidate check, so the walk never stops at the next rejected group's originator and runs to the far closer every time. New Parse uses `strings.Index` for rejected groups and stays linear.
  Generator: repeat(`<wb:topic name="a">\n```x\n</wb:topic>`) + `<wb:topic name="z"><wb:log>\n```\n</wb:log></wb:topic>`. A `~~~x` variant and a variant with no final log behave the same.

  | shape | 50KB | 100KB | 200KB | new Parse at 200KB | v0.50.0 at 200KB |
  |---|---|---|---|---|---|
  | ParseOrdered | 176 ms | 711 ms | 2867 ms | 4.3 ms | 13.4 ms |
  | Apply(&State{}, s, Env{}) | 189 ms | 721 ms | 2864 ms | — | — |

  - Apply returns accepted=0 with 5688 problems at 200KB. The input is invalid (v0.50.0 refuses it), so this is a cost and DoS issue only, not a correctness issue. There is no total-size cap before parsing.
  - Commands: `R4K=rejected-group-fence,rejected-group-tilde,rejected-group-fence-nolog go test ./internal/wswhiteboard -run 'TestR4Perf$' -count=1 -v` and `go test ./internal/wswhiteboard -run 'TestR4ApplyPerf$' -count=1 -v`.
  - Fix direction: give the rejected-group body the raw-frame matcher in the walk (frame type "group, before first command"), or bound the walk to the next group boundary. Alternatively, disable literal code for rejected group bodies, as non-ordered Parse already does.

### Minor
None beyond the C-1 variant above.

### Differential fuzz (held for I1 and non-wrapper I3)
- Harness: in-process v0.50.0 Parse vs new Parse and ParseOrdered.
  - I1: when v0.50.0 has 0 problems and the input has no `<wb:change`, new Parse must have 0 problems and identical commands. New ParseOrdered must have 0 problems, complete=true, and identical non-scope commands.
  - I3: when LiteralCode=false Parse has 0 problems, literal Parse must be identical. Wrapper inputs were included and reported separately.
  - Comparison: Pos, Line, Tag, Attrs, Body, Fields, Children, Scope tag and value. Failures were delta-minimized with the author's minimizer.
- Generators, different from the author's:
  - a token generator with comments (`<!-- `` ` ``), PI-like `<?pi `?>`, DOCTYPE, CDATA holding closes, ticks and fences, and dangling `<![CDATA[`/`]]>`;
  - attributes holding backticks, tildes and quotes (recap, label, tickets, close result);
  - `wb:edit`, `wb:add`, root `wb:patch` and `wb:answer`, `wb:state` with an explicit close, self-closing fields, and unknown `wb:foo`;
  - CRLF fences, 1/3/4-space indents, info strings, and framed fences (```` ```</wb:log> ````, `~~~</wb:old>`);
  - an "old-clean-dense" generator;
  - mutation of the author's corpus plus 1,056 `<wb:` string literals harvested via go/ast from the package's own tests (real payloads): token insertion, tick-run insertion, deletion, cross-corpus splicing, and segment duplication.
- Runs: seeds 9137, 424242, 1, 77777, 31337 and 5, 60,000 cases each, **360,000 cases total**.
  - About 78k cases were v0.50.0-clean.
  - In the instrumented run (seed 5), 11,199 of 16,974 v0.50.0-clean cases ran at least one crossing-candidate walk.
  - The slowest single case was 1.07 ms.
- Results: **I1-Parse 0, I1-Ordered 0, non-wrapper I3 0.** I3-with-wrapper had 106-188 per run, all the C-1 root cause.
- Command: `R4SEED=<seed> R4N=60000 go test ./internal/wswhiteboard -run TestR4Fuzz -count=1 -v`. The instrumentation was a temporary counter in the /tmp copy of codeLiteralEnd. The source was restored from git before the package tests.

### Tested and held
- Prior defects:
  - C-A closed: the exact repro gives 2 panels, as on v0.50.0. The panel→log and why→link variants match v0.50.0.
  - Round-3 rejected-group swallow variant: the "ok" log is retained under ParseOrdered.
  - C-B closed: the 3-command split is preserved, and the log body is the intended Decision-10 literal.
  - I-A closed: the author's ten shapes pass TestLiteralCodeNearLinearGrowth.
  - My extra multi-body shapes are linear:
    - unknown-command fence per body: 15.7 ms at 200KB in ParseOrdered;
    - `wb:edit` old fence: 5.9 ms;
    - root `wb:answer` fence: 3.9 ms;
    - CDATA per log: 5.0 ms;
    - known tag with unterminated attribute: 7.7 ms;
    - unterminated CDATA after a tick: 0.01 ms;
    - group-text ticks: 6.6 ms;
    - rejected-group distinct inline runs: 2.3 ms;
    - wrapper panel-body fence: 0.5 ms.
  - P1-P4 are covered by the existing tests, which pass.
- Walk vs reader by reasoning, then confirmed by fuzz:
  - literalTagEnd matches readOpen on known names (limit=end makes a cross-e known tag reject). Unknown names at an empty stack can only land in F-problem territory.
  - Comments and PI at enclosing level are a v0.50.0 fatal error. Inside raw bodies they are text in both.
  - CDATA consumption (`literalConsumedEnd`) mirrors readContent.
  - firstBodyClose cannot be pushed past a close by quotes.
- lineAt forward cache (wsxmlcmd.go:602): I found no backward-call pattern on hot paths.
- `go vet ./internal/wsxmlcmd ./internal/wswhiteboard`: clean.
- `go test ./internal/wsxmlcmd -count=1`: ok (0.61s).
- `go test ./internal/wswhiteboard -p=1 -count=1 -timeout 10m`: ok (64.0s). This ran on unmodified source, with my zz_ files moved out.

omitted: full ./internal/mcp and ./internal/wsdashboard (forbidden); Windows; 2 MiB timings (I-1 extrapolates to roughly 7-8 minutes at 2 MiB, not measured); tracker grammar (LiteralCode=false); store/entries/folders T0 writer matrix (unchanged in this range); browser/export opt-in tests.
