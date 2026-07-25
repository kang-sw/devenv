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
