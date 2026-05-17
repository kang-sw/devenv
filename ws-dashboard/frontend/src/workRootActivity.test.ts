import { describe, expect, it } from "vitest";

import { workRootActivityEndpoint } from "./workRootActivity.js";

describe("workRootActivityEndpoint", () => {
  it("addresses activity by encoded opaque workRoot id", () => {
    expect(workRootActivityEndpoint("root/local test")).toBe(
      "/api/dashboard/work-roots/root%2Flocal%20test/activity",
    );
  });
});
