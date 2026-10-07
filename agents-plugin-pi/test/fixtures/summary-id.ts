import assert from "node:assert/strict";
import { SUMMARY_ID_KEY, summaryIdOf } from "../../src/summary-id.ts";

/** Asserts `details` carries a display-summary row id and returns the rest for exact comparison. */
export function stripSummaryId(details: unknown): Record<string, unknown> {
  assert.equal(typeof summaryIdOf(details), "string", "details carry a display-summary row id");
  const { [SUMMARY_ID_KEY]: _id, ...rest } = details as Record<string, unknown>;
  return rest;
}
