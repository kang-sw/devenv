import { test } from "node:test";
import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { capRpcClientStderr } from "../src/rpc-stderr-cap.ts";

function fakeClient() {
  const stderr = new EventEmitter();
  const client = { process: { stderr }, stderr: "" };
  // Mimics the host's own listener, registered first.
  stderr.on("data", (d: Buffer) => { client.stderr += d.toString(); });
  return { client, stderr };
}

test("capRpcClientStderr trims accumulated stderr to its tail", () => {
  const { client, stderr } = fakeClient();
  assert.equal(capRpcClientStderr(client, 10), true);
  stderr.emit("data", Buffer.from("abcde"));
  assert.equal(client.stderr, "abcde");
  stderr.emit("data", Buffer.from("fghijklmno\npq"));
  assert.equal(client.stderr, "ghijklmno\npq".slice(-10));
  assert.equal(client.stderr.length, 10);
  assert.ok(client.stderr.endsWith("o\npq"));
});

test("capRpcClientStderr is a no-op on an unexpected client shape", () => {
  assert.equal(capRpcClientStderr(undefined), false);
  assert.equal(capRpcClientStderr({}), false);
  assert.equal(capRpcClientStderr({ process: {}, stderr: "" }), false);
  assert.equal(capRpcClientStderr({ process: { stderr: new EventEmitter() }, stderr: 5 }), false);
});
