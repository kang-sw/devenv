import json
import os
import subprocess
import unittest
from pathlib import Path


PLUGIN_DIR = Path(__file__).resolve().parents[1]
REPO_ROOT = PLUGIN_DIR.parent
TOOL_DIR = REPO_ROOT / "agents-plugin-tool"

HIDDEN_TOOLS = {
    "agents.register",
    "agents.call",
    "agents.wait",
    "agents.result",
    "agents.status",
    "agents.interrupt",
    "agents.tail",
    "agents.debug.tail",
    "agents.debug.stdout",
    "agents.debug.stderr",
    "agents.debug.runtime_log",
    "agents.debug.events",
    "agents.cancel",
    "agents.print",
    "agents.erase",
    "subquery",
    "config.agents_tier",
    "api.ask",
    "api.ask_async",
    "api.status",
    "api.result",
    "api.cancel",
    "ws.setup",
    "exec.spawn",
    "exec.shell",
    "exec.status",
    "exec.result",
    "exec.abort",
    "exec.raw.tail",
    "exec.raw.read",
    "exec.raw.grep",
}

HIDDEN_COMMANDS = {
    "agents.register",
    "agents.call",
    "agents.run-current",
    "agents.wait",
    "agents.result",
    "agents.status",
    "agents.interrupt",
    "agents.check-inbox",
    "agents.tail",
    "agents.debug.tail",
    "agents.debug.stdout",
    "agents.debug.stderr",
    "agents.debug.runtime-log",
    "agents.debug.events",
    "agents.cancel",
    "agents.print",
    "agents.erase",
    "subquery",
    "config.agents-tier",
}


class WsflowRuntimeContractTest(unittest.TestCase):
    def load_mcp_env(self):
        config = json.loads((PLUGIN_DIR / ".mcp.json").read_text(encoding="utf-8"))
        return config["mcpServers"]["wsflow"]["env"]

    def load_contract(self):
        return json.loads((PLUGIN_DIR / "runtime.json").read_text(encoding="utf-8"))

    def test_mcp_config_selects_agentless_wsflow_runtime(self):
        config = json.loads((PLUGIN_DIR / ".mcp.json").read_text(encoding="utf-8"))
        server = config["mcpServers"]["wsflow"]

        self.assertEqual(server["command"], "python3")
        self.assertEqual(server["cwd"], ".")
        self.assertEqual(server["args"], ["./bin/ws-mcp-launcher.py", "serve", "--stdio"])
        self.assertEqual(
            server["env"],
            {
                "WS_MCP_NO_AGENT": "1",
                "WS_MCP_NAMESPACE": "wsflow",
                "WS_MCP_SETUP_TOOL": "setup",
            },
        )

    def test_runtime_contract_matches_agentless_capabilities(self):
        contract = self.load_contract()
        env = os.environ.copy()
        env.update(self.load_mcp_env())

        self.assertEqual(contract["runtime_capabilities"], {"match": "exact"})

        proc = subprocess.run(
            ["go", "run", "./cmd/ws-mcp", "runtime", "capabilities"],
            cwd=TOOL_DIR,
            env=env,
            text=True,
            capture_output=True,
            timeout=60,
            check=False,
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        payload = json.loads(proc.stdout)

        self.assertEqual(set(contract["tools"]), set(payload["tools"]))
        self.assertEqual(set(contract["commands"]), set(payload["commands"]))
        self.assertFalse(HIDDEN_TOOLS & set(contract["tools"]))
        self.assertFalse(HIDDEN_COMMANDS & set(contract["commands"]))
        self.assertIn("setup", contract["tools"])
        self.assertIn("api.list", contract["tools"])
        self.assertIn("prompt.render", contract["tools"])

    def test_prompt_render_absent_from_full_ws_capabilities(self):
        env = os.environ.copy()
        env.pop("WS_MCP_NO_AGENT", None)
        env.pop("WS_MCP_NAMESPACE", None)
        env.pop("WS_MCP_SETUP_TOOL", None)

        proc = subprocess.run(
            ["go", "run", "./cmd/ws-mcp", "runtime", "capabilities"],
            cwd=TOOL_DIR,
            env=env,
            text=True,
            capture_output=True,
            timeout=60,
            check=False,
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        payload = json.loads(proc.stdout)
        self.assertNotIn("prompt.render", payload["tools"])


if __name__ == "__main__":
    unittest.main()
