import { test } from "node:test";
import assert from "node:assert/strict";
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { RpcClient } from "@earendil-works/pi-coding-agent";

// Only the MCP and RPC transports are substituted. The copied adapter source,
// resource loader, extension runner, SessionManager and serializers are real.
// Copying isolates changed child resources and the test-only MCP launcher.
const GLOBAL_SDK = existsSync("/home/linuxbrew/.linuxbrew/lib/node_modules/@earendil-works/pi-coding-agent")
  ? "/home/linuxbrew/.linuxbrew/lib/node_modules/@earendil-works/pi-coding-agent"
  : "/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent";
for (const root of [join(process.cwd(), "node_modules/@earendil-works/pi-coding-agent"), GLOBAL_SDK]) for (const [providerName, apiName] of [["openrouter", "openai-completions"], ["openai-codex", "openai-codex-responses"], ["anthropic", "anthropic-messages"]]) {
  test(`production fork lifecycle ${JSON.parse(readFileSync(join(root, "package.json"), "utf8")).version}/${apiName}`,  async () => {
    const directory = mkdtempSync(join(tmpdir(), "ws-pi-lifecycle-"));
    const plugin = join(directory, "plugin");
    mkdirSync(plugin);
    for (const name of ["src", "runtime.json", "goal-loop-config.json", "pi-lead-guide.md", "execute-worker-guide.md", "explore-guide.md"]) cpSync(join(process.cwd(), name), join(plugin, name), { recursive: true });
    const childIndexPath = join(plugin, "src/index.ts");
    const childIndexSource = readFileSync(childIndexPath, "utf8");
    writeFileSync(childIndexPath, childIndexSource.replace(
      "    if (readSpawnRole(process.env) === \"fork\") {\n      try {\n        const unavailable = installMissingTaskForkTools(pi, durableForkContextRef.current);",
      "    if (process.env.WS_PI_TEST_OMIT_FORK_REPORT === \"1\") pi.setActiveTools(pi.getActiveTools().filter((name) => name !== \"ws-report-to-lead\"));\n\n    if (readSpawnRole(process.env) === \"fork\") {\n      try {\n        const unavailable = installMissingTaskForkTools(pi, durableForkContextRef.current);",
    ));
    symlinkSync(join(process.cwd(), "node_modules"), join(plugin, "node_modules"));
    mkdirSync(join(plugin, "bin"));
    const version = JSON.parse(readFileSync(join(plugin, "runtime.json"), "utf8")).plugin_version;
    writeFileSync(join(plugin, "bin/ws-mcp-launcher.py"), `import sys,json,uuid,os\nkey='own-'+str(uuid.uuid4())\nfor line in sys.stdin:\n q=json.loads(line); m=q['method']; p=q.get('params',{}); r={}\n if m=='initialize': r={'serverInfo':{'name':'offline','version':${JSON.stringify(version)}},'capabilities':{}}\n elif m=='tools/list': r={'tools':[{'name':'probe','description':'Offline routing probe','inputSchema':{'type':'object','properties':{'session_key':{'type':'string'}}}}]}\n elif m=='tools/call':\n  n=p['name']; a=p.get('arguments',{}); text=json.dumps({'session_key':key}) if n=='ferrule' else ('lead manual '+key if n=='workflow_manual' else (json.dumps(a) if n=='probe' else '{}'))\n  r={'content':[{'type':'text','text':text}],'isError':False}\n  if n=='ferrule' and os.path.exists(${JSON.stringify(join(directory, "fail-key"))}): r={'isError':True,'content':[{'type':'text','text':'offline failed ferrule'}]}\n  if n=='playbook.read' and os.path.exists(${JSON.stringify(join(directory, "fail-map"))}): r={'isError':True,'content':[{'type':'text','text':'offline failed mapping'}]}\n print(json.dumps({'jsonrpc':'2.0','id':q['id'],'result':r}),flush=True)\n`);
    const sdk = await import(join(root, "dist/index.js"));
    const spawner = await import(join(plugin, "src/spawner.ts"));
    const sidecar = await import(join(plugin, "src/agent-sidecar.ts"));
    const ask = await import(join(plugin, "src/ask.ts"));
    const originalEnv = { ...process.env };
    const prototypes = [...new Set([RpcClient.prototype, sdk.RpcClient.prototype])];
    const originals = prototypes.map(proto => Object.fromEntries(["start", "stop", "abort", "onEvent", "prompt", "getState", "setThinkingLevel", "steer", "followUp"].map(name => [name, (proto as any)[name]])));
    const sessions: any[] = [];
    const children: any[] = [];
    const errors: string[] = [];
    const payloads: any[] = [];
    let sends = 0;
    const apiKey = providerName === "openai-codex" ? `e30.${Buffer.from(JSON.stringify({ "https://api.openai.com/auth": { chatgpt_account_id: "offline" } })).toString("base64url")}.x` : "offline-test-key";
    const model = { provider: providerName, api: apiName, id: "offline-model", name: "offline", reasoning: false, input: ["text", "image"], contextWindow: 128000, maxTokens: 8192, baseUrl: "https://offline.invalid/v1", cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 } };
    const chunks = join(root, "dist/bundle/chunks");
    const serializer = await import(join(chunks, readdirSync(chunks).find(name => name.startsWith(apiName + "-") && name.endsWith(".js"))!));
    async function withEnv<T>(env: Record<string, string>, fn: () => Promise<T>): Promise<T> {
      const saved = { ...process.env };
      Object.assign(process.env, env);
      try { return await fn(); } finally { for (const key of Object.keys(process.env)) if (!(key in saved)) delete process.env[key]; Object.assign(process.env, saved); }
    }
    async function makeSession(sm: any, env: any = {}, tools?: string[], append = "Explicit append Ω\r\ntrailing  ", discovered = false, parentOnlyTool = false) {
      return withEnv(env, async () => {
        const agentDir = join(directory, `config-${sessions.length}`); mkdirSync(agentDir);
        const settings = sdk.SettingsManager.inMemory({ compaction: { enabled: false }, retry: { enabled: false }, ...(discovered ? { extensions: [join(plugin, "src/index.ts")] } : {}) });
        if (providerName === "openai-codex") writeFileSync(join(agentDir, "auth.json"), JSON.stringify({ "openai-codex": { type: "oauth", access: apiKey, refresh: "unused-offline", expires: Date.now() + 86400000 } }));
        const runtime = await sdk.ModelRuntime.create({ authPath: join(agentDir, "auth.json"), modelsPath: join(agentDir, "models.json"), modelsStorePath: join(agentDir, "store.json"), allowModelNetwork: false });
        if (providerName !== "openai-codex") await runtime.setRuntimeApiKey(providerName, apiKey);
        let api: any;
        let partialPrompt: string | undefined;
        const loader = new sdk.DefaultResourceLoader({ cwd: directory, agentDir, settingsManager: settings, noSkills: true, noThemes: true, noPromptTemplates: true, noContextFiles: true, additionalExtensionPaths: discovered ? [] : [join(plugin, "src/index.ts"), join(plugin, "src/index.ts")], systemPrompt: "Custom base\r\n", appendSystemPrompt: [append], extensionFactories: [(pi: any) => { api = pi; if (parentOnlyTool) pi.registerTool({ name: "parent-only-extension", label: "parent-only-extension", description: "A parent-only extension tool", parameters: { type: "object", properties: { query: { type: "string" } } }, async execute() { return { content: [{ type: "text", text: "parent-only" }] }; } }); pi.on("before_agent_start", (e: any) => { partialPrompt = e.systemPrompt; return env.WS_PI_SPAWN_ROLE === "fork" ? undefined : { systemPrompt: e.systemPrompt + "\nLater handler Ω  " }; }); }] });
        await loader.reload();
        assert.deepEqual(loader.getExtensions().errors, []);
        assert.equal(loader.getExtensions().extensions.filter((e: any) => e.path === join(plugin, "src/index.ts")).length, 1, "source/discovery deduplicates the adapter");
        const { session } = await sdk.createAgentSession({ cwd: directory, agentDir, sessionManager: sm, resourceLoader: loader, modelRuntime: runtime, model, thinkingLevel: "off", settingsManager: settings, ...(tools ? { tools } : {}) });
        const h: any = { session, sm, env, api, parentOnlyTool, get beforePrompt() { return partialPrompt; }, requests: [] as any[] };
        sessions.push(h);
        session.agent.streamFunction = async (m: any, context: any, options: any) => (h.rawStream = serializer.stream(m, context, { ...options, apiKey, cacheRetention: h.retention ?? "short", fetch: async () => { sends++; throw new Error("network forbidden"); }, onPayload: async (payload: any) => {
          const actual = await options.onPayload?.(payload, m) ?? payload;
          h.requests.push({ context: { ...context, messages: structuredClone(context.messages), tools: [...context.tools] }, payload: structuredClone(actual) }); payloads.push(actual);
          throw new Error("direct serializer capture before network");
        } }));
        const noop = () => {};
        const ui = new Proxy({ notify: (message: string) => { if (message.includes("not ready") || message.includes("differs")) errors.push(message); }, custom: async () => undefined }, { get: (target: any, key) => target[key] ?? noop });
        await session.bindExtensions({ mode: env.WS_PI_SPAWN_ROLE === "fork" ? "rpc" : "tui", uiContext: ui, onError: (e: any) => errors.push(String(e.message ?? e)) });
        return h;
      });
    }
    async function prompt(h: any, text: string) {
      const before = h.requests.length;
      const keys = h.sm.getEntries().findLast((e: any) => e.customType === "ws-pi-fork-keys" && e.data.sessionId === h.sm.getSessionId());
      // Independently defined new-message fixture, never copied from child payload.
      const framed = h.referenceHistory && !h.hasInput ? `Current fork-owned ws session_key: ${keys?.data.current}. Use this key, not the inherited parent or any historical own key. ws-fork, ws-queue-question, and ws-withdraw-question are refused in fork role; report to the lead instead.\n\n${text}${h.unavailableTools?.length ? `\n\nUnavailable tools in this fork: ${h.unavailableTools.join(", ")}. Their parent extension was not loaded; calling one fails deterministically.` : ""}` : text;
      await withEnv(h.env, () => h.session.prompt(text));
      if (h.referenceHistory && h.requests.length > before) {
        const user = { role: "user", content: [{ type: "text", text: framed }], timestamp: 1 };
        let expected: any;
        const actualContext = h.requests.at(-1).context.messages as any[];
        // Extension-pushed custom messages are materialized by Pi only in the
        // built model context, not the source SessionManager view. In that
        // transport case, take the provider-neutral prior-message ordering
        // from Pi while still independently serializing every provider field.
        const referenceHistory = JSON.stringify(actualContext).includes("[ws-agent-report]")
          ? structuredClone(actualContext.slice(0, -1))
          : [...h.referenceHistory];
        const context = { systemPrompt: h.oracle.systemPrompt, tools: h.oracle.tools, messages: [...referenceHistory, user] };
        for await (const _event of serializer.stream(h.oracleModel ?? model, context, { apiKey, sessionId: h.oracleSessionId ?? h.parentAffinityId, cacheRetention: h.retention ?? "short", fetch: async () => { sends++; throw Error("network forbidden"); }, onPayload: (body: any) => { expected = structuredClone(body); throw Error("independent direct capture"); } })) {}
        assert.ok(expected);
        assert.deepEqual(h.requests.at(-1).payload, expected, "entire raw provider request equals independently accumulated continuation");
        h.referenceHistory.push(user, structuredClone(await h.rawStream.result()));
        h.hasInput = true;
      }
    }
    async function stop(h: any) { if (!h.stopped) { h.stopped = true; await withEnv(h.env, () => h.session.extensionRunner.emit({ type: "session_shutdown", reason: "quit" })); h.session.dispose(); } }
    let omitCompletionReport = false;
    const transport = {
      async start(this: any) {
        const args = this.options.args; const env = this.options.env;
        assert.equal(args.includes("--append-system-prompt"), false);
        const extension = args.indexOf("--extension");
        assert.ok(args.includes("--no-extensions"), "RPC children disable ambient extension discovery");
        assert.deepEqual(args.slice(extension - 1, extension + 2), ["--no-extensions", "--extension", join(plugin, "src/index.ts")]);
        assert.equal(args.filter((arg: string) => arg === "--extension").length, 1, "the child loads exactly one adapter entry");
        const fork = args.indexOf("--fork");
        const sessionDir = args.indexOf("--session-dir");
        assert.ok(sessionDir >= 0, "the RPC argv owns its child session directory");
        const sm = fork >= 0 ? sdk.SessionManager.forkFrom(args[fork + 1], directory, args[sessionDir + 1]) : sdk.SessionManager.open(args[args.indexOf("--session") + 1]);
        const childEnv = omitCompletionReport ? { ...env, WS_PI_TEST_OMIT_FORK_REPORT: "1" } : env;
        const readinessPath = omitCompletionReport ? JSON.parse(readFileSync(env.WS_PI_FORK_CONTEXT, "utf8")).readinessPath : undefined;
        this.harness = await makeSession(sm, childEnv, args[args.indexOf("--tools") + 1].split(","), "CHANGED CHILD APPEND");
        this.harness.parentClient = this;
        if (omitCompletionReport) assert.match(readFileSync(readinessPath, "utf8"), /missing completion-critical callable tool: ws-report-to-lead/, "child bootstrap records the missing completion channel before readiness validation");
        const sourcePath = fork >= 0 ? args[fork + 1] : sm.getSessionFile();
        const sourceId = JSON.parse(readFileSync(sourcePath, "utf8").split("\n")[0]).id;
        const source = sessions.findLast(h => h !== this.harness && (h.sm.getSessionFile() === sourcePath || h.sm.getSessionId() === sourceId));
        this.harness.oracle = source.oracle ?? source.requests[0]?.context;
        this.harness.parentAffinityId = source.parentAffinityId ?? source.sm.getSessionId();
        this.harness.referenceHistory = this.harness.oracle ? structuredClone(source.referenceHistory ?? source.sm.buildSessionContext().messages) : undefined;
        this.harness.unavailableTools = source.unavailableTools ?? (source.parentOnlyTool ? ["parent-only-extension"] : []);
        children.push(this.harness);
      },
      async stop(this: any) { if (this.harness) await stop(this.harness); }, async abort() {},
      onEvent(this: any, listener: (event: unknown) => void) { (this.wsPiTestEventListeners ??= new Set()).add(listener); return () => this.wsPiTestEventListeners?.delete(listener); },
      async getState(this: any) { return { sessionFile: this.harness.sm.getSessionFile(), sessionId: this.harness.sm.getSessionId(), model, thinkingLevel: this.harness.session.thinkingLevel }; },
      async setThinkingLevel(this: any, level: string) { this.harness.session.setThinkingLevel(level); },
      async prompt(this: any, text: string) { await prompt(this.harness, text); },
      async steer(this: any, text: string) { await prompt(this.harness, text); }, async followUp(this: any, text: string) { await prompt(this.harness, text); },
    };
    for (const proto of prototypes) Object.assign(proto, transport);
    try {
      process.env.PI_OFFLINE = "1";
      for (const key of Object.keys(process.env)) if (key.startsWith("WS_PI_FORK_") || key === "WS_PI_PARENT_SESSION_KEY" || key === "WS_PI_SPAWN_ROLE") delete process.env[key];
      const lead = await makeSession(sdk.SessionManager.create(directory, join(directory, "sessions")), {}, undefined, "Explicit append Ω\r\ntrailing  ", false, true);
      if (!lead.api.getActiveTools().includes("ws-report-to-lead")) lead.api.setActiveTools([...lead.api.getActiveTools(), "ws-report-to-lead"]);
      await prompt(lead, "Original lead history Ω");
      assert.equal(lead.requests.length, 1, errors.join("\n") + JSON.stringify(lead.session.messages));
      const observed = lead.requests[0];
      const capture = lead.sm.getEntries().findLast((e: any) => e.customType === "ws-pi-lead-prompt").data;
      assert.equal(capture.effectiveSystemPrompt, observed.context.systemPrompt);
      assert.equal(capture.basePromptOptions.appendSystemPrompt, "Explicit append Ω\r\ntrailing  ");
      assert.ok(capture.effectiveSystemPrompt.endsWith("Later handler Ω  "));
      assert.notEqual(lead.beforePrompt, capture.effectiveSystemPrompt, "before-agent observation is partial; persisted capture is post-chain");
      // Original SessionManager branch oracle includes native metadata and mixed
      // content, not just a hand-authored provider payload.
      lead.sm.appendMessage({ role: "assistant", api: apiName, provider: providerName, model: model.id, providerThinkingLevel: "low", stopReason: "toolUse", timestamp: 2, content: [...(providerName === "anthropic" ? [{ type: "thinking", thinking: "original private reasoning", thinkingSignature: "original-signed-thinking" }] : []), { type: "toolCall", id: "call_original_1", name: "ws__probe", arguments: {} }, { type: "toolCall", id: "call_original_2", name: "ws__probe", arguments: {} }] });
      for (const toolCallId of ["call_original_1", "call_original_2"]) lead.sm.appendMessage({ role: "toolResult", toolCallId, toolName: "ws__probe", content: [{ type: "text", text: "original result Ω\r\n  " }], isError: false, timestamp: 3 });
      lead.sm.appendMessage({ role: "user", content: [{ type: "text", text: "" }, { type: "image", mimeType: "image/png", data: "AQ==" }], timestamp: 4 });
      writeFileSync(join(plugin, "pi-lead-guide.md"), "CHANGED GUIDE");
      writeFileSync(join(directory, "fail-map"), "fail");
      const sourceHistory = structuredClone(lead.sm.buildSessionContext().messages);
      const forkTool = lead.session.agent.state.tools.find((t: any) => t.name === "ws-fork");
      assert.ok(forkTool);
      const result = await forkTool.execute("task", { prompt: "Inspect task" });
      const id = JSON.parse(result.content[0].text).agent_id;
      assert.ok(id, JSON.stringify(result));
      const child = children.at(-1);
      assert.equal(child.requests.length, 1, errors.join("\n"));
      assert.equal(child.requests[0].context.systemPrompt, observed.context.systemPrompt);
      assert.deepEqual(child.requests[0].payload.tools, observed.payload.tools);
      assert.deepEqual(child.sm.buildSessionContext().messages.slice(0, sourceHistory.length), sourceHistory);
      const childContext = child.sm.getEntries().findLast((e: any) => e.customType === "ws-pi-fork-context").data.context;
      assert.equal(childContext.parentAffinityId, lead.sm.getSessionId());
      assert.equal(childContext.thinkingLevel, "off");
      const ownKey = (h: any) => h.sm.getEntries().findLast((e: any) => e.customType === "ws-pi-fork-keys").data.current;
      const firstKey = ownKey(child);
      assert.match(JSON.stringify(child.requests[0].context.messages.at(-1)), new RegExp(firstKey));
      assert.match(JSON.stringify(child.requests[0].context.messages.at(-1)), /Unavailable tools in this fork: parent-only-extension/);
      const parentOnly = child.session.agent.state.tools.find((tool: any) => tool.name === "parent-only-extension");
      const parentDefinition = lead.session.agent.state.tools.find((tool: any) => tool.name === "parent-only-extension");
      assert.ok(parentOnly && parentDefinition, "the parent-only registration remains provider-visible in the fork");
      assert.deepEqual({ name: parentOnly.name, description: parentOnly.description, parameters: parentOnly.parameters }, { name: parentDefinition.name, description: parentDefinition.description, parameters: parentDefinition.parameters });
      await withEnv(child.env, () => assert.rejects(() => parentOnly.execute("unavailable", { query: "test" }), /unavailable fork tool "parent-only-extension"/));
      assert.ok(child.session.agent.state.tools.find((tool: any) => tool.name === "ws-report-to-lead"), "the completion channel remains available");
      await prompt(child, "Second task turn");
      assert.equal(child.requests[1].context.systemPrompt, observed.context.systemPrompt);
      assert.deepEqual(child.requests[1].context.messages.at(-1).content, [{ type: "text", text: "Second task turn" }], "only the first new input of a process receives the frame");
      const callsBefore = children.length;
      const entriesBeforeRefusals = structuredClone(child.sm.getEntries());
      const childThreadsPath = ask.threadRegistryPath(child.sm.getSessionFile());
      const threadSnapshot = () => existsSync(childThreadsPath) ? readFileSync(childThreadsPath, "utf8") : undefined;
      const threadsBeforeRefusals = threadSnapshot();
      const listTool = child.session.agent.state.tools.find((t: any) => t.name === "ws-agent-list");
      const registryBeforeRefusals = await withEnv(child.env, () => listTool.execute("list", {}));
      const nestedFork = child.session.agent.state.tools.find((tool: any) => tool.name === "ws-fork");
      assert.ok(nestedFork, "the inherited fork tool remains active so its role handler can refuse recursive forks");
      await withEnv(child.env, () => assert.rejects(() => nestedFork.execute("refuse", {}), /unavailable in a fork/));
      // 260911 lifts the tool-surface hide (ac998f77/a8cf1183/5f366eff): the
      // fork's tool surface is the lead's exact active-tools snapshot
      // (`computeForkToolSurface` adds/removes/dedupes nothing), so
      // ws-queue-question/ws-withdraw-question stay VISIBLE with identical
      // metadata in a fork's own tool list — refused only at the handler
      // level, exactly like ws-fork itself just above.
      for (const name of ["ws-queue-question", "ws-withdraw-question"]) {
        const tool = child.session.agent.state.tools.find((t: any) => t.name === name);
        assert.ok(tool, `${name} remains visible in the inherited --tools allowlist`);
        await withEnv(child.env, () => assert.rejects(() => tool.execute("refuse", {}), /unavailable in a fork/));
      }
      assert.equal(children.length, callsBefore);
      assert.deepEqual(child.sm.getEntries(), entriesBeforeRefusals, "refusals do not mutate session state");
      assert.equal(threadSnapshot(), threadsBeforeRefusals, "refusals do not mutate the thread store");
      assert.deepEqual(await withEnv(child.env, () => listTool.execute("list", {})), registryBeforeRefusals, "refusals do not allocate or mutate the agent registry");
      const probe = child.session.agent.state.tools.find((t: any) => t.name === "ws__probe");
      await withEnv(child.env, () => assert.rejects(() => probe.execute("parent", { session_key: childContext.parentSessionKey }), /parent session key/));
      await withEnv(child.env, async () => {
        for (const value of [undefined, firstKey]) {
          const result = await probe.execute("forward", value ? { session_key: value } : {});
          assert.match(JSON.stringify(result), new RegExp(value ?? firstKey));
        }
        await assert.rejects(() => probe.execute("foreign", { session_key: "separately-issued-worker-key" }), /outside this agent's authority/);
      });
      const finalReport = "Outcome: degraded fork completed\nFiles changed: none\nVerification: lifecycle fixture\nBlockers: none\nCommit: none\nDecisions: report channel remained available";
      const reportTool = child.session.agent.state.tools.find((tool: any) => tool.name === "ws-report-to-lead");
      const reportResult = await withEnv(child.env, () => reportTool.execute("final", { kind: "final", message: finalReport }));
      assert.deepEqual(reportResult, { content: [{ type: "text", text: "reported" }], details: { subtreeRevision: 0 } });
      assert.ok(child.parentClient.wsPiTestEventListeners.size > 0, "the parent client owns the fork report relay");
      for (const listener of child.parentClient.wsPiTestEventListeners) listener({ type: "tool_execution_start", toolName: "ws-report-to-lead", toolCallId: "final", args: { kind: "final", message: finalReport } });
      for (const listener of child.parentClient.wsPiTestEventListeners) listener({ type: "tool_execution_end", toolName: "ws-report-to-lead", toolCallId: "final", isError: false, result: reportResult });
      for (const listener of child.parentClient.wsPiTestEventListeners) listener({ type: "agent_settled" });
      await new Promise((resolve) => setTimeout(resolve, 0));
      const leadListTool = lead.session.agent.state.tools.find((tool: any) => tool.name === "ws-agent-list");
      const listedAgents = JSON.parse((await leadListTool.execute("list", {})).content[0].text);
      const reportedChild = (Array.isArray(listedAgents) ? listedAgents : listedAgents.agents).find((agent: any) => agent.agent_id === id);
      assert.equal(reportedChild.status, "dormant", "the final report settles the degraded fork");
      assert.ok(reportedChild.last_report_at, "the final report reaches the parent registry");
      assert.ok(childContext.registeredTools.some((tool: any) => tool.name === "ws-report-to-lead"), "the parent capture treats the report channel as completion-critical");
      omitCompletionReport = true;
      await assert.rejects(() => forkTool.execute("missing completion", { prompt: "must not reach provider" }), /missing completion-critical callable tool: ws-report-to-lead/);
      const missingCompletionChild = children.at(-1);
      assert.equal(missingCompletionChild.requests.length, 0, "missing ws-report-to-lead rejects during child bootstrap before provider prompt");
      omitCompletionReport = false;
      await stop(lead); // Actual shutdown writes the task sidecar.
      let orphans = sidecar.readAndClearSidecar(lead.sm.getSessionFile()).filter((orphan: any) => orphan.agentId === id);
      assert.equal(orphans.length, 1);
      for (let generation = 0; generation < 2; generation++) {
        const registry = new Map(); sidecar.reviveOrphans(registry, sidecar.parseOrphans(sidecar.serializeOrphans(orphans)));
        await spawner.sendToAgent(registry, { cwd: directory, extensionPath: join(plugin, "src/index.ts") }, id, `Resume ${generation}`);
        const resumed = children.at(-1);
        assert.equal(resumed.requests[0].context.systemPrompt, observed.context.systemPrompt);
        assert.deepEqual(resumed.requests[0].payload.tools, observed.payload.tools);
        assert.notEqual(ownKey(resumed), firstKey);
        const tool = resumed.session.agent.state.tools.find((t: any) => t.name === "ws__probe");
        await withEnv(resumed.env, () => assert.rejects(() => tool.execute("stale", { session_key: firstKey }), /stale prior session key/));
        assert.match(JSON.stringify(resumed.requests[0].context.messages.at(-1)), new RegExp(ownKey(resumed)));
        orphans = sidecar.captureOrphans(registry);
        assert.deepEqual(orphans[0].forkContext, childContext);
        await stop(resumed);
      }
      // 260911 Phase 1: a lead-raised ("lead-ask") thread is fork-less end to
      // end. /answer on one never reaches the discussion-fork spawn below —
      // that branch of `ensureRespondent` is reachable only through
      // `openThread`'s pre-260911 fall-through, which its new "lead-ask"
      // early return (see Phase 2's `openLeadAskQueue`) now always
      // short-circuits before. ws-queue-question/ws-withdraw-question (renamed from
      // ws-ask/ws-resolve) are intentionally absent from active tool lists,
      // so seed the same persisted pending-thread contract that /answer
      // consumes, and assert the fork-less contract: no process spawns, no
      // respondent or forkResume is ever attached, and opening the thread
      // costs no paid model turn. (The discussion-fork-spawn scenario this
      // replaced, plus its dependent forkResume-rehydration-across-
      // generations loop, exercised code now unreachable for every thread
      // origin — fork-raised always already has a respondentAgentId at
      // registration, so it never takes ensureRespondent's spawn branch
      // either. captureForkResume/rehydrateForkRecord keep their own direct
      // unit coverage in ask.test.ts.)
      const restartedManager = sdk.SessionManager.open(lead.sm.getSessionFile(), join(directory, "sessions"));
      const now = new Date().toISOString();
      ask.saveThreadRegistryFile(ask.threadRegistryPath(restartedManager.getSessionFile()), [{
        threadId: "q1", title: "Choice", question: "Which choice?", entryId: restartedManager.getLeafId(),
        status: "pending", origin: "lead-ask", createdAt: now, touchedAt: now,
      }]);
      const restarted = await makeSession(restartedManager);
      restarted.oracle = { ...observed.context, tools: observed.context.tools.filter((tool: any) => tool.name !== "parent-only-extension") };
      restarted.parentAffinityId = lead.sm.getSessionId();
      const childrenBeforeAnswer = children.length;
      await prompt(restarted, "/answer");
      assert.equal(children.length, childrenBeforeAnswer, "a lead-raised question opens fork-less — /answer spawns no process");
      assert.equal(restarted.requests.length, 0, "opening a queued question is a local command, never a paid model turn");
      const openedThreads = ask.loadThreadRegistryFile(ask.threadRegistryPath(restarted.sm.getSessionFile()));
      assert.equal(openedThreads[0].status, "open", "the thread opens in place on the lead session, with no respondent to attach");
      assert.equal(openedThreads[0].respondentAgentId, undefined, "no respondent is ever assigned on the fork-less lead-ask path");
      assert.equal(openedThreads[0].forkResume, undefined, "no fork exists yet to capture a resume snapshot for");
      // The removed discussion-fork-rehydration-across-generations loop used
      // to also hand the drift/cache-override checks below a fresh live
      // child to exercise; a plain task fork off the (still-live) restarted
      // lead fills that same generic role now — none of what follows is
      // specific to a discussion fork.
      const restartedForkTool = restarted.session.agent.state.tools.find((t: any) => t.name === "ws-fork");
      assert.ok(restartedForkTool);
      const restartedForkResult = await restartedForkTool.execute("continuation", { prompt: "Continue after lead restart" });
      assert.ok(JSON.parse(restartedForkResult.content[0].text).agent_id);
      // `drifted` stays live (not stopped) here, deliberately: stopping its
      // lead (`restarted`) cascades into `index.ts`'s shutdown handler
      // (`persistShutdownAgentSnapshots` -> `stopAll()`), which stops every
      // non-threadBound child registered under that lead's own registry —
      // including this one. `restarted` is stopped once we are done using
      // `drifted`, below.
      const drifted = children.at(-1);
      assert.doesNotMatch(JSON.stringify(drifted.requests[0].context.messages.at(-1)), /Unavailable tools in this fork/, "no-mismatch forks receive no unavailable-tools notice");
      assert.equal(drifted.session.agent.state.tools.some((tool: any) => tool.name === "parent-only-extension"), false, "no-mismatch forks receive no unavailable stub");
      const noMismatchTools = structuredClone(drifted.requests[0].payload.tools);
      drifted.retention = "none";
      drifted.oracleSessionId = drifted.sm.getSessionId();
      await prompt(drifted, "Continue with caching disabled");
      assert.equal(drifted.requests.at(-1).context.systemPrompt, observed.context.systemPrompt);
      if (providerName === "openai-codex") assert.equal(drifted.requests.at(-1).payload.prompt_cache_key, undefined);
      drifted.retention = "short";
      drifted.oracleModel = { ...model, id: "explicit-model-override" };
      await withEnv(drifted.env, () => drifted.session.setModel(drifted.oracleModel));
      await prompt(drifted, "Continue with explicit model override");
      assert.equal(drifted.requests.at(-1).context.systemPrompt, observed.context.systemPrompt);
      assert.deepEqual(drifted.requests.at(-1).payload.tools, noMismatchTools);
      assert.equal(drifted.requests.at(-1).payload.model, "explicit-model-override");
      if (providerName === "openai-codex") assert.equal(drifted.requests.at(-1).payload.prompt_cache_key, drifted.sm.getSessionId(), "incompatible model preserves prompt/tools but receives no parent affinity");
      // Post-resource-merge registration drift is handled by the actual SDK input
      // hook, before any provider callback (throwing a hook would not suffice).
      const driftCount = drifted.requests.length;
      await withEnv(drifted.env, async () => { drifted.api.setActiveTools([...drifted.api.getActiveTools()].reverse()); await drifted.session.prompt("must block drift"); });
      assert.equal(drifted.requests.length, driftCount);
      await stop(restarted);
      // Fork-less /answer costs no paid turn even with zero prior lead
      // turns and under ambient extension auto-discovery (`discovered:
      // true`) — there is no composed discussion-fork prefix left to test
      // here (see the note above the restart scenario), only that opening
      // still spawns nothing and manufactures no turn.
      const noPriorManager = sdk.SessionManager.create(directory, join(directory, "sessions"));
      const noPriorNow = new Date().toISOString();
      ask.saveThreadRegistryFile(ask.threadRegistryPath(noPriorManager.getSessionFile()), [{
        threadId: "q1", title: "Before first turn", question: "Answer without prior turn", entryId: noPriorManager.getLeafId(),
        status: "pending", origin: "lead-ask", createdAt: noPriorNow, touchedAt: noPriorNow,
      }]);
      const noPrior = await makeSession(noPriorManager, {}, undefined, "Never-paid explicit Ω\r\n  ", true);
      const childrenBeforeNoPriorAnswer = children.length;
      await prompt(noPrior, "/answer");
      assert.equal(children.length, childrenBeforeNoPriorAnswer, "fork-less /answer spawns nothing even with no prior lead turn");
      assert.equal(noPrior.requests.length, 0, "no paid turn manufactured to open a fork-less thread");
      const noPriorThreads = ask.loadThreadRegistryFile(ask.threadRegistryPath(noPrior.sm.getSessionFile()));
      assert.equal(noPriorThreads[0].status, "open");
      const legacyEnv = { WS_PI_SPAWN_ROLE: "fork", WS_PI_FORK_CONTEXT: "", WS_PI_FORK_AFFINITY: "", WS_PI_PARENT_SESSION_KEY: "legacy-parent" };
      writeFileSync(join(directory, "fail-key"), "fail");
      const failedLegacy = await makeSession(sdk.SessionManager.create(directory, join(directory, "sessions")), legacyEnv);
      await prompt(failedLegacy, "No own key must block");
      assert.equal(failedLegacy.requests.length, 0);
      await withEnv(legacyEnv, () => assert.rejects(() => failedLegacy.session.agent.state.tools.find((t: any) => t.name === "ws__probe").execute("parent", { session_key: "legacy-parent" }), /bootstrap is not ready/));
      rmSync(join(directory, "fail-key"));
      const legacy = await makeSession(sdk.SessionManager.create(directory, join(directory, "sessions")), legacyEnv);
      await prompt(legacy, "Legacy local fallback remains enabled");
      assert.equal(legacy.requests.length, 1);
      assert.match(legacy.requests[0].context.systemPrompt, /CHANGED GUIDE/);
      assert.match(JSON.stringify(legacy.requests[0].context.messages.at(-1)), new RegExp(ownKey(legacy)));
      const malformedPath = join(directory, "bad.json"); writeFileSync(malformedPath, "{");
      const malformed = await makeSession(sdk.SessionManager.create(directory, join(directory, "sessions")), { ...legacyEnv, WS_PI_FORK_CONTEXT: malformedPath });
      await prompt(malformed, "Malformed present metadata must block");
      assert.equal(malformed.requests.length, 0);
      const worker = await makeSession(sdk.SessionManager.create(directory, join(directory, "sessions")), { ...legacyEnv, WS_PI_SPAWN_ROLE: "worker", WS_PI_FORK_CONTEXT: "/missing-poison-file" });
      await prompt(worker, "Worker ignores poisoned fork envelope");
      assert.equal(worker.requests.length, 1);
      assert.equal(sends, 0);
      assert.ok(payloads.length >= 8);
    } finally {
      for (const h of sessions.reverse()) await stop(h);
      prototypes.forEach((proto, i) => Object.assign(proto, originals[i]));
      for (const key of Object.keys(process.env)) if (!(key in originalEnv)) delete process.env[key];
      Object.assign(process.env, originalEnv);
      rmSync(directory, { recursive: true, force: true });
    }
  });
}
