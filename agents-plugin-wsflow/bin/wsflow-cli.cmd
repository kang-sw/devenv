@echo off
set WS_MCP_NO_AGENT=1
set WS_MCP_NAMESPACE=wsflow
set WS_MCP_SETUP_TOOL=setup
python3 "%~dp0ws-mcp-launcher.py" %*
