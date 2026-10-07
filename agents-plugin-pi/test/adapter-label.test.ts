import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { ADAPTER_MESSAGE_LABEL, adapterLabeledBody, labelAdapterContent, labelAdapterText } from "../src/adapter-label.ts";

describe("adapter message label (261007)", () => {
  test("the label wording is pinned", () => {
    assert.equal(ADAPTER_MESSAGE_LABEL, "[system message from ws-pi-plugin]");
  });

  test("text gains the label as its own first line, and the body comes back out", () => {
    const sent = labelAdapterText("2 ws messages waiting");
    assert.equal(sent, "[system message from ws-pi-plugin]\n2 ws messages waiting");
    assert.equal(adapterLabeledBody(sent), "2 ws messages waiting");
    assert.equal(adapterLabeledBody(ADAPTER_MESSAGE_LABEL), "");
    assert.equal(adapterLabeledBody("2 ws messages waiting"), undefined);
    assert.equal(adapterLabeledBody(`${ADAPTER_MESSAGE_LABEL} same line`), undefined);
    assert.equal(adapterLabeledBody(`human text\n${ADAPTER_MESSAGE_LABEL}\nlater`), undefined);
  });

  test("custom content: string, leading text part, and a non-text first part", () => {
    assert.equal(labelAdapterContent("body"), `${ADAPTER_MESSAGE_LABEL}\nbody`);
    assert.deepEqual(labelAdapterContent([{ type: "text", text: "body" }, { type: "text", text: "more" }]), [
      { type: "text", text: `${ADAPTER_MESSAGE_LABEL}\nbody` },
      { type: "text", text: "more" },
    ]);
    const image = { type: "image", data: "AAAA", mimeType: "image/png" };
    assert.deepEqual(labelAdapterContent([image]), [{ type: "text", text: ADAPTER_MESSAGE_LABEL }, image]);
    assert.deepEqual(labelAdapterContent([]), [{ type: "text", text: ADAPTER_MESSAGE_LABEL }]);
  });
});
