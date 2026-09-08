import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

for (const root of [join(process.cwd(), "node_modules/@earendil-works/pi-coding-agent"), "/home/linuxbrew/.linuxbrew/lib/node_modules/@earendil-works/pi-coding-agent"]) {
  const version = JSON.parse(readFileSync(join(root, "package.json"), "utf8")).version;
  async function provider(prefix: string) {
    const directory = join(root, "dist/bundle/chunks");
    return import(join(directory, readdirSync(directory).find(name => name.startsWith(prefix + "-") && name.endsWith(".js"))!));
  }
  test(`${version}: exact Anthropic native marker and effort transitions`, async () => {
    const api = await provider("anthropic-messages");
    let sends = 0;
    for (const oauth of [false, true]) for (const retention of ["none", "short", "long"]) for (const effort of [false, true]) {
      const model = { provider: "anthropic", api: "anthropic-messages", id: "claude-offline", baseUrl: "https://api.anthropic.com", input: ["text", "image"], maxTokens: 8192, contextWindow: 128000, reasoning: true, compat: { supportsMidConvoEffort: effort, forceAdaptiveThinking: true } };
      const controls = retention === "none" ? undefined : retention === "long" ? { type: "ephemeral", ttl: "1h" } : { type: "ephemeral" };
      const capture = async (messages: any[]) => {
        let payload: any;
        for await (const _event of api.stream(model, { systemPrompt: "Exact Ω\r\n  ", messages }, { apiKey: oauth ? "sk-ant-oat-offline" : "offline", cacheRetention: retention, thinkingEnabled: true, effort: "medium", fetch: async () => { sends++; throw Error("network forbidden"); }, onPayload: (p: any) => { payload = structuredClone(p); throw Error("direct capture"); } })) {}
        assert.ok(payload); return payload;
      };
      const historical = await capture([{ role: "user", content: "original" }]);
      const continuation = await capture([{ role: "user", content: "original" }, { role: "user", content: "suffix" }]);
      const effortMessage = effort && version === "0.85.1" ? [{ role: "system", content: [], output_config: { effort: "medium" } }] : [];
      assert.deepEqual(historical.messages, [{ role: "user", content: controls ? [{ type: "text", text: "original", cache_control: controls }] : "original" }, ...effortMessage]);
      // This is the exact native transition, not annotation removal in a comparator:
      // historical final string -> marked block; continuation restores the original
      // string representation and marks only the new suffix.
      assert.deepEqual(continuation, { ...historical, messages: [{ role: "user", content: "original" }, { role: "user", content: controls ? [{ type: "text", text: "suffix", cache_control: controls }] : "suffix" }, ...effortMessage] });
      assert.deepEqual(continuation.output_config, { effort: effort && version === "0.85.1" ? "high" : "medium" });
      if (effort && version === "0.85.1") {
        const signed = { role: "assistant", api: model.api, provider: model.provider, model: model.id, stopReason: "stop", providerThinkingLevel: "low", content: [{ type: "thinking", thinking: "private", thinkingSignature: "signed" }, { type: "text", text: "answer" }] };
        const replay = await capture([{ role: "user", content: "original" }, signed, { role: "user", content: "suffix" }]);
        assert.deepEqual(replay.messages, [{ role: "user", content: "original" }, { role: "system", content: [], output_config: { effort: "low" } }, { role: "assistant", content: [{ type: "thinking", thinking: "private", signature: "signed" }, { type: "text", text: "answer" }] }, { role: "user", content: controls ? [{ type: "text", text: "suffix", cache_control: controls }] : "suffix" }, ...effortMessage]);
      }
    }
    assert.equal(sends, 0);
  });

  test(`${version}: Codex full-body to wire-delta uses child connection identity`, async () => {
    const api = await provider("openai-codex-responses");
    const original = globalThis.WebSocket;
    const wires: any[] = [];
    const sockets: any[] = [];
    let sends = 0;
    class Loopback extends EventTarget {
      readyState = 1;
      headers: any;
      constructor(_url: string, options: any) { super(); this.headers = options.headers; sockets.push(this); queueMicrotask(() => this.dispatchEvent(new Event("open"))); }
      send(text: string) {
        const body = JSON.parse(text); const id = `${this.headers["session-id"]}-response-${wires.length}`;
        wires.push({ body, id, headers: this.headers });
        queueMicrotask(() => {
          this.dispatchEvent(new MessageEvent("message", { data: JSON.stringify({ type: "response.created", response: { id, status: "in_progress" } }) }));
          this.dispatchEvent(new MessageEvent("message", { data: JSON.stringify({ type: "response.completed", response: { id, status: "completed", output: [], usage: { input_tokens: 1, output_tokens: 0, total_tokens: 1 } } }) }));
        });
      }
      close() { this.readyState = 3; }
    }
    globalThis.WebSocket = Loopback as never;
    const token = `e30.${Buffer.from(JSON.stringify({ "https://api.openai.com/auth": { chatgpt_account_id: "offline" } })).toString("base64url")}.x`;
    const model = { provider: "openai-codex", api: "openai-codex-responses", id: "offline", baseUrl: "https://offline.invalid", input: ["text", "image"], cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }, maxTokens: 8192, contextWindow: 128000 };
    async function request(sessionId: string, messages: any[]) {
      let full: any; let done = false;
      for await (const event of api.stream(model, { systemPrompt: "original exact instructions", tools: [], messages }, { apiKey: token, sessionId, transport: "websocket-cached", cacheRetention: "short", fetch: async () => { sends++; throw Error("network forbidden"); }, onPayload: (body: any) => { full = structuredClone(body); return { ...body, prompt_cache_key: "parent" }; } })) {
        if (event.type === "done") done = true;
        if (event.type === "error") assert.fail(event.error.errorMessage);
      }
      assert.ok(done); return full;
    }
    try {
      const history = [{ role: "user", content: "history" }];
      await request("parent", history);
      const first = await request("child", history);
      const next = await request("child", [...history, { role: "user", content: "suffix" }]);
      assert.equal(sockets.length, 2, "affinity does not share the parent's socket");
      assert.equal(wires[1].headers["session-id"], "child");
      assert.equal(wires[1].headers["x-client-request-id"], "child");
      assert.deepEqual(wires[1].body, { type: "response.create", ...first, prompt_cache_key: "parent" });
      assert.deepEqual(wires[2].body, { type: "response.create", ...next, prompt_cache_key: "parent", previous_response_id: wires[1].id, input: next.input.slice(first.input.length) });
      assert.deepEqual(next.input.slice(0, first.input.length), first.input);
      assert.equal(wires[2].body.input.length, 1);
      assert.notEqual(wires[2].body.previous_response_id, wires[0].id);
      assert.equal(sends, 0);
    } finally { api.closeOpenAICodexWebSocketSessions(); globalThis.WebSocket = original; }
  });
}
