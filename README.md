# devenv

Personal developer environment and local `ws` workflow workspace.

This repo started as dotfiles and a workstation bootstrap script. It now also
contains the local `ws` and `wsflow` plugin packages, the native `ws-mcp`
runtime source, workflow docs, and the dashboard scaffold used while developing
that system.

## Quick Start

```sh
git clone https://github.com/kang-sw/devenv.git ~/devenv
cd ~/devenv
bash install.sh          # idempotent; safe to re-run
bash install.sh update   # skip packages & sudo, refresh config and symlinks
```

> Neovim 0.10+ is required but not installed by the script.

## What's Inside

```
nvim/                  Neovim config
shell/                 tmux, WezTerm, Starship, zsh, helper scripts
tools/                 local Claude session TUIs
agents-plugin/         ws plugin package
agents-plugin-wsflow/  agentless wsflow plugin package
agents-plugin-tool/    native ws MCP runtime and tooling source
ws-dashboard/          local web dashboard scaffold
ai-docs/               workflow specs, tickets, mental models, references
```

- **Workstation config**: Neovim, tmux, WezTerm, Starship, zsh, shell helpers,
  and editor/tooling defaults for day-to-day development.
- **Local TUIs**: `claude-watch` and `claude-dash` live under `tools/`.
- **`ws` plugin**: Codex-first workflow skills plus the `ws-mcp` runtime.
  Claude Code compatibility uses the `agents-plugin/` package metadata; there is
  no live `claude-plugin/` source tree.
- **`wsflow` plugin**: Agentless workflow skills that reuse the shared runtime
  without ws managed-agent orchestration.
- **Runtime source**: Go tooling and MCP server code under `agents-plugin-tool/`.
- **Workflow docs**: specs, tickets, mental models, and references under
  `ai-docs/`.

## install.sh

Detects macOS, WSL, and Linux. Full mode installs packages and writes config.
`update` mode skips package installation and refreshes dotfiles, symlinks, and
local plugin setup.

## ws Plugin

The active `ws` plugin package lives in `agents-plugin/`.

Codex installs use the repository marketplace entries under `.agents/plugins/`.
Claude-compatible marketplace metadata lives under `.claude-plugin/`.

Local bootstrap registers a Claude-compatible snapshot from `agents-plugin/`
when Claude Code is available:

```sh
claude plugin marketplace add kang-sw/devenv
claude plugin install ws@kang-sw-devenv
```

The agentless `wsflow` package lives in `agents-plugin-wsflow/` for separate
marketplace installation. `install.sh` does not install `wsflow` into Claude.

## License

Personal configuration. Use freely, no warranty implied.
