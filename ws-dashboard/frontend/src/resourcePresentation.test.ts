import {
  countByRootKey,
  formatOpenSurfaceCounts,
} from "./resourcePresentation.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
  }
}

function assertDeepEqual<T>(actual: T, expected: T, label: string) {
  const actualJson = JSON.stringify(actual);
  const expectedJson = JSON.stringify(expected);
  if (actualJson !== expectedJson) {
    throw new Error(`${label}: expected ${expectedJson}, got ${actualJson}`);
  }
}

// countByRootKey: 260725 nav-row-two-line-open-state Phase 1, Decision 1 -
// shared grouping used by both the App()-level document-count memo and
// WorkbenchShell's signature-gated terminal-count memo.
assertDeepEqual(
  countByRootKey([], (item: string) => item),
  {},
  "empty input groups to an empty map",
);

assertDeepEqual(
  countByRootKey(
    [
      { workRootId: "root-a" },
      { workRootId: "root-b" },
      { workRootId: "root-a" },
      { workRootId: "root-a" },
    ],
    (item) => item.workRootId,
  ),
  { "root-a": 3, "root-b": 1 },
  "groups and counts items by the derived key",
);

assertDeepEqual(
  countByRootKey(["a", "b", "c"], () => "same-key"),
  { "same-key": 3 },
  "all items collapsing to one key still count correctly",
);

// formatOpenSurfaceCounts: exact wording is an implementation choice (per
// the plan), but the zero-vs-populated branch and singular/plural counts
// are the load-bearing behavior for the browser-gate assertions.
assertEqual(
  formatOpenSurfaceCounts(0, 0),
  "no open surfaces",
  "both counts zero renders the empty-state text",
);
assertEqual(
  formatOpenSurfaceCounts(1, 0),
  "1 terminal, 0 documents",
  "singular terminal count, zero documents",
);
assertEqual(
  formatOpenSurfaceCounts(0, 1),
  "0 terminals, 1 document",
  "zero terminals, singular document count",
);
assertEqual(
  formatOpenSurfaceCounts(2, 3),
  "2 terminals, 3 documents",
  "plural terminal and document counts",
);

// 260725 Phase 7: the agent split. The zero-agent strings above are asserted
// BYTE-IDENTICAL through the three-argument form below, because
// dashboard-acceptance.spec.ts compares live row text against this
// function's own output on its existing two-argument call sites.
assertEqual(
  formatOpenSurfaceCounts(0, 0, { agents: 0, working: 0, ready: 0 }),
  "no open surfaces",
  "an explicit all-zero agent triple leaves the empty-state text byte-identical",
);
assertEqual(
  formatOpenSurfaceCounts(2, 3, { agents: 0, working: 0, ready: 0 }),
  "2 terminals, 3 documents",
  "an explicit all-zero agent triple leaves the populated text byte-identical",
);
assertEqual(
  formatOpenSurfaceCounts(1, 0, { agents: 2, working: 1, ready: 1 }),
  "2 agents: 1 working, 1 ready · 1 terminal, 0 documents",
  "the agent segment reports the split, working before ready, and LEADS the line so the ~225px nowrap ellipsis box spends its width on the agent numbers",
);
// Review cycle 1, Important 1: a root whose only open surface is an agent
// terminal must not read "no open surfaces · 1 agent: ...". That state is
// this feature's primary flow - the moment right after an agent is spawned
// into a freshly opened root - not an exotic corner.
assertEqual(
  formatOpenSurfaceCounts(0, 0, { agents: 1, working: 0, ready: 0 }),
  "1 agent: 0 working, 0 ready",
  "an agents-only root reports its agent alone rather than contradicting itself with 'no open surfaces'",
);
assertEqual(
  formatOpenSurfaceCounts(0, 0, { agents: 1, working: 1, ready: 0 }),
  "1 agent: 1 working, 0 ready",
  "the working half is rendered as its own number (Phase 3 spike), not folded into a single total",
);
assertEqual(
  formatOpenSurfaceCounts(0, 2, { agents: 1, working: 0, ready: 1 }),
  "1 agent: 0 working, 1 ready · 0 terminals, 2 documents",
  "a root with agents and documents but no shell terminal still reports the surfaces half",
);
