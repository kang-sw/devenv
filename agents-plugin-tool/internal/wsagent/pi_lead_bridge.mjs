// servers and sessionKey are private JSON constants prepended by the runner.
export default function (pi) {
  for (const [name, config] of Object.entries(servers)) pi.registerMcpServer(name, config);
  pi.on("before_agent_start", (event) => ({
    systemPrompt: event.systemPrompt + "\n\nYour ws session key is " + JSON.stringify(sessionKey) +
      ". Pass it as session_key in ws tool calls. This hosted session already has a key; do not mint a replacement.",
  }));
}
