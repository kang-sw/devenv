/**
 * Unit tests for fork.ts's pure-logic seams (260904 Phase 1, side-thread
 * fork ticket): `computeForkToolSurface`/`addForkToolIfLead` (§3's dynamic
 * tool-surface formula, including the role-differentiation fix — a fork
 * must never regain `ws-fork`), `shouldNudge`/`classifyForkTurnOutcome`/
 * `isIdleWithoutFinal` (§4's anti-bleed disambiguation), `extractReportField`/
 * `validateFinalReportShape`/`checkExpectsCommitCompletion` (§4's required
 * report shape + `expects_commit` non-completion rule), `tailLines`, and
 * `getForkSourceSessionFile`.
 *
 * NOT covered here — genuinely live-gate only, mirroring
 * test/execute-gateway.test.ts's own pure/IO split: `registerFork`'s tool
 * `execute()` body and `wireAntiBleedLoop` (both need a live `pi --mode rpc`
 * session or a real `RpcClient`). Exercised only by the plan's documented
 * manual verification gate (no provider credentials in this sandbox —
 * deferred, not faked).
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  FORK_TOOL_NAME,
  FORK_EXCLUDED_TOOL_NAMES,
  computeForkToolSurface,
  addForkToolIfLead,
  MAX_FORK_NUDGES,
  shouldNudge,
  classifyForkTurnOutcome,
  isIdleWithoutFinal,
  REQUIRED_FINAL_REPORT_FIELDS,
  extractReportField,
  validateFinalReportShape,
  checkExpectsCommitCompletion,
  tailLines,
  getForkSourceSessionFile,
  buildForkDirectiveText,
} from "../src/fork.ts";
import { REPORT_TO_LEAD_TOOL_NAME } from "../src/spawner.ts";

describe("FORK_TOOL_NAME / FORK_EXCLUDED_TOOL_NAMES", () => {
  test("FORK_TOOL_NAME is the literal ws-fork", () => {
    assert.equal(FORK_TOOL_NAME, "ws-fork");
  });

  test("Phase 1 excludes only ws-fork itself (ws-ask/ws-resolve don't exist yet)", () => {
    assert.deepEqual([...FORK_EXCLUDED_TOOL_NAMES], [FORK_TOOL_NAME]);
  });
});

describe("computeForkToolSurface", () => {
  test("removes ws-fork from the lead's active tools and adds ws-report-to-lead", () => {
    const result = computeForkToolSurface(["bash", "edit", FORK_TOOL_NAME, "ws-agent-spawn"]);
    assert.ok(!result.includes(FORK_TOOL_NAME), "a fork's own surface must never include ws-fork (no recursive forking)");
    assert.ok(result.includes(REPORT_TO_LEAD_TOOL_NAME));
    assert.ok(result.includes("bash"));
    assert.ok(result.includes("edit"));
    assert.ok(result.includes("ws-agent-spawn"));
  });

  test("never duplicates ws-report-to-lead if the lead's active tools already carry it", () => {
    const result = computeForkToolSurface(["bash", REPORT_TO_LEAD_TOOL_NAME]);
    assert.equal(result.filter((name) => name === REPORT_TO_LEAD_TOOL_NAME).length, 1);
  });

  test("an empty lead tool list still ends up with exactly ws-report-to-lead", () => {
    assert.deepEqual(computeForkToolSurface([]), [REPORT_TO_LEAD_TOOL_NAME]);
  });

  test("a lead surface with no ws-fork present is unaffected besides the ws-report-to-lead addition", () => {
    const result = computeForkToolSurface(["bash", "edit", "write"]);
    assert.deepEqual([...result].sort(), ["bash", "edit", REPORT_TO_LEAD_TOOL_NAME, "write"].sort());
  });
});

describe("addForkToolIfLead (risk-signal fix: role-differentiated, never folded into computeLeadActiveTools)", () => {
  test("role undefined (true top lead) gains ws-fork when absent", () => {
    const result = addForkToolIfLead(["bash", "edit"], undefined);
    assert.ok(result.includes(FORK_TOOL_NAME));
  });

  test("role undefined does not duplicate ws-fork if already present", () => {
    const result = addForkToolIfLead(["bash", FORK_TOOL_NAME], undefined);
    assert.equal(result.filter((name) => name === FORK_TOOL_NAME).length, 1);
  });

  test('role "fork" NEVER regains ws-fork, even if somehow present in the input (defense in depth)', () => {
    const withoutIt = addForkToolIfLead(["bash", "edit"], "fork");
    assert.ok(!withoutIt.includes(FORK_TOOL_NAME));
    const alreadyPresent = addForkToolIfLead(["bash", FORK_TOOL_NAME], "fork");
    assert.deepEqual(alreadyPresent, ["bash", FORK_TOOL_NAME], "an existing entry is left as-is, but never newly added");
  });

  test('role "worker" and "explore" never gain ws-fork', () => {
    assert.ok(!addForkToolIfLead(["bash"], "worker").includes(FORK_TOOL_NAME));
    assert.ok(!addForkToolIfLead(["bash"], "explore").includes(FORK_TOOL_NAME));
  });

  test("an empty active-tools list for the true lead ends up with exactly ws-fork", () => {
    assert.deepEqual(addForkToolIfLead([], undefined), [FORK_TOOL_NAME]);
  });
});

describe("shouldNudge", () => {
  test("allows nudging while nudgeCount is below MAX_FORK_NUDGES", () => {
    for (let i = 0; i < MAX_FORK_NUDGES; i++) {
      assert.equal(shouldNudge(i), true, `expected shouldNudge(${i}) to be true`);
    }
  });

  test("refuses once nudgeCount has reached MAX_FORK_NUDGES (the fail-loud transition)", () => {
    assert.equal(shouldNudge(MAX_FORK_NUDGES), false);
    assert.equal(shouldNudge(MAX_FORK_NUDGES + 1), false);
  });
});

describe("classifyForkTurnOutcome (§4 disambiguation table)", () => {
  test('reportKind "question" always classifies as "question", tool call or not', () => {
    assert.equal(classifyForkTurnOutcome({ hadToolCall: true, reportKind: "question" }), "question");
    assert.equal(classifyForkTurnOutcome({ hadToolCall: false, reportKind: "question" }), "question");
  });

  test('reportKind "final" always classifies as "final", tool call or not', () => {
    assert.equal(classifyForkTurnOutcome({ hadToolCall: true, reportKind: "final" }), "final");
    assert.equal(classifyForkTurnOutcome({ hadToolCall: false, reportKind: "final" }), "final");
  });

  test('no report but a tool call happened -> "acknowledge-and-return" (not itself a bleed signal)', () => {
    assert.equal(classifyForkTurnOutcome({ hadToolCall: true }), "acknowledge-and-return");
  });

  test('neither a report nor a tool call -> "no-signal" (the actual bleed condition)', () => {
    assert.equal(classifyForkTurnOutcome({ hadToolCall: false }), "no-signal");
  });
});

describe("isIdleWithoutFinal", () => {
  test("true when no kind in the list is \"final\"", () => {
    assert.equal(isIdleWithoutFinal([undefined, "question", undefined]), true);
  });

  test("false as soon as any kind is \"final\"", () => {
    assert.equal(isIdleWithoutFinal(["question", "final"]), false);
  });

  test("an empty list is vacuously true (no final report was ever seen)", () => {
    assert.equal(isIdleWithoutFinal([]), true);
  });
});

describe("extractReportField", () => {
  const message = ["Outcome: did the thing", "Files changed: a.ts, b.ts", "Commit: none", "  Blockers:   none  "].join("\n");

  test("extracts the trimmed value after a matching '<field>:' line", () => {
    assert.equal(extractReportField(message, "Outcome"), "did the thing");
    assert.equal(extractReportField(message, "Files changed"), "a.ts, b.ts");
  });

  test("tolerates leading whitespace on the line itself", () => {
    assert.equal(extractReportField(message, "Blockers"), "none");
  });

  test("returns undefined when the field's line is absent", () => {
    assert.equal(extractReportField(message, "Verification"), undefined);
  });

  test("does not match a field name that is a substring of another field's line", () => {
    // "Commit" must not accidentally match inside some other longer field.
    assert.equal(extractReportField("Recommit: no", "Commit"), undefined);
  });
});

describe("validateFinalReportShape (§4 required field-prefix check)", () => {
  const wellFormed = [
    "Outcome: did the thing",
    "Files changed: a.ts",
    "Verification: ran npm test, all green",
    "Blockers: none",
    "Commit: abc123",
    "Decisions: chose X over Y",
  ].join("\n");

  test("a message carrying every required field passes", () => {
    assert.deepEqual(validateFinalReportShape(wellFormed), { ok: true });
  });

  test("a message missing one or more required fields lists exactly the missing ones", () => {
    const partial = ["Outcome: did the thing", "Commit: none"].join("\n");
    const result = validateFinalReportShape(partial);
    assert.equal(result.ok, false);
    if (!result.ok) {
      assert.deepEqual(result.missing.sort(), ["Blockers", "Decisions", "Files changed", "Verification"].sort());
    }
  });

  test("an empty message is missing every required field", () => {
    const result = validateFinalReportShape("");
    assert.equal(result.ok, false);
    if (!result.ok) {
      assert.deepEqual(result.missing.sort(), [...REQUIRED_FINAL_REPORT_FIELDS].sort());
    }
  });

  test("Commit: none is valid SHAPE-wise (the expects_commit non-completion rule is a separate check)", () => {
    assert.deepEqual(validateFinalReportShape(wellFormed.replace("Commit: abc123", "Commit: none")), { ok: true });
  });
});

describe("checkExpectsCommitCompletion (§4 expects_commit non-completion rule)", () => {
  test("expects_commit:false never flags anything, regardless of commitLine", () => {
    assert.deepEqual(checkExpectsCommitCompletion(false, undefined), { ok: true });
    assert.deepEqual(checkExpectsCommitCompletion(false, "none"), { ok: true });
    assert.deepEqual(checkExpectsCommitCompletion(false, "abc123"), { ok: true });
  });

  test("expects_commit:true with a missing Commit line is flagged non-completion", () => {
    const result = checkExpectsCommitCompletion(true, undefined);
    assert.equal(result.ok, false);
  });

  test('expects_commit:true with the literal "none" (case/whitespace tolerant) is flagged non-completion', () => {
    for (const commitLine of ["none", "None", "NONE", "  none  "]) {
      const result = checkExpectsCommitCompletion(true, commitLine);
      assert.equal(result.ok, false, `expected commitLine=${JSON.stringify(commitLine)} to be flagged`);
    }
  });

  test("expects_commit:true with a real commit value is accepted", () => {
    assert.deepEqual(checkExpectsCommitCompletion(true, "abc123"), { ok: true });
  });
});

describe("tailLines", () => {
  const text = ["l1", "l2", "l3", "l4", "l5"].join("\n");

  test("returns the last n lines", () => {
    assert.equal(tailLines(text, 2), "l4\nl5");
  });

  test("n beyond the total line count returns the whole text unchanged", () => {
    assert.equal(tailLines(text, 100), text);
  });

  test("n <= 0 returns an empty string", () => {
    assert.equal(tailLines(text, 0), "");
    assert.equal(tailLines(text, -3), "");
  });
});

describe("getForkSourceSessionFile", () => {
  test("extracts the session file path from a well-formed toolCtx.sessionManager.getSessionFile()", () => {
    const toolCtx = { sessionManager: { getSessionFile: () => "/tmp/lead-session.jsonl" } };
    assert.equal(getForkSourceSessionFile(toolCtx), "/tmp/lead-session.jsonl");
  });

  test("returns undefined when toolCtx, sessionManager, or getSessionFile is missing", () => {
    assert.equal(getForkSourceSessionFile(undefined), undefined);
    assert.equal(getForkSourceSessionFile({}), undefined);
    assert.equal(getForkSourceSessionFile({ sessionManager: {} }), undefined);
  });

  test("returns undefined when getSessionFile returns an empty string or a non-string", () => {
    assert.equal(getForkSourceSessionFile({ sessionManager: { getSessionFile: () => "" } }), undefined);
    assert.equal(getForkSourceSessionFile({ sessionManager: { getSessionFile: () => undefined } }), undefined);
  });
});

describe("buildForkDirectiveText", () => {
  test("names the report tool and both kind values, with no identity framing or ALL-CAPS override language", () => {
    const text = buildForkDirectiveText();
    assert.ok(text.includes(REPORT_TO_LEAD_TOOL_NAME));
    assert.ok(text.includes('kind:"question"'));
    assert.ok(text.includes('kind:"final"'));
    for (const field of REQUIRED_FINAL_REPORT_FIELDS) {
      assert.ok(text.includes(`${field}:`), `expected the directive to name required field "${field}"`);
    }
  });

  test("carries no identity-framing persona opener", () => {
    const text = buildForkDirectiveText();
    assert.ok(!/\byou\s+are\s+a\b/i.test(text), `directive text must not open with "you are a ..." identity framing: ${text}`);
  });

  test("carries no ALL-CAPS override-style words", () => {
    const text = buildForkDirectiveText();
    const allCapsWords = text.match(/\b[A-Z]{4,}\b/g) ?? [];
    assert.deepEqual(allCapsWords, [], `directive text must not carry ALL-CAPS override word(s): ${JSON.stringify(allCapsWords)}`);
  });
});
