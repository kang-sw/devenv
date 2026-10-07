/**
 * A stand-in Pi RPC child whose session bootstrap fails: it runs the real
 * `bootstrapOrFailLoud` with a worker role and an RPC-mode-shaped `ui.notify`
 * (an `extension_ui_request` line written to stdout on a later turn, as Pi's
 * output guard does), then exits through the guard's real `process.exit`.
 * `FAKE_NOTIFY_PAD` pads the message so the pipe write cannot complete
 * synchronously. Inert unless `WS_PI_FAKE_BOOTSTRAP_FAIL_CHILD=1`, since the
 * test runner's default glob also loads files under `test/`.
 */
import { bootstrapOrFailLoud } from "../../src/index.ts";

const pad = "x".repeat(Number(process.env.FAKE_NOTIFY_PAD ?? "0"));
const ui = {
  notify(message: string, type?: string) {
    setImmediate(() => {
      process.stdout.write(JSON.stringify({ type: "extension_ui_request", id: "notify-1", method: "notify", message, notifyType: type }) + "\n");
    });
  },
};

if (process.env.WS_PI_FAKE_BOOTSTRAP_FAIL_CHILD === "1") {
  await bootstrapOrFailLoud(ui, "worker", async () => {
    throw new Error(`simulated ws-mcp bootstrap failure ${pad}`);
  });
}
