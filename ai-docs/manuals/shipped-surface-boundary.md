---
summary: Required before editing any text that ships to a downstream project - playbooks, skills, templates, embedded conventions, and every string the MCP tooling emits to an agent
---

# Shipped Surface Boundary

## Purpose

Every playbook, skill, convention, and template under `agents-plugin/` and
`agents-plugin-wsflow/`, every embedded convention, and every string
`agents-plugin-tool/` emits to an agent (todo instructions, advisories,
banners, doctor checks, tool descriptions) runs inside projects that are not
this one and that hold only what bootstrap installs.

This manual is the detail behind `AGENTS.md` `## Architecture Rules` rule 4.
That rule states the invariant; this file states how to apply it.

## Rules

The test for every sentence of shipped text: **does it depend on something a
downstream project does not have?** If yes, it is a leak.

Concretely, shipped text MUST NOT name:

- a ticket of this repository: a real `26xxxx-...` stem, an
  `ai-docs/tickets/...` path to one, an epic name, or a bare ticket number
  used as a citation. Example stems that resolve to nothing are fine;
  ticket-directory names and `<status>/<stem>` placeholders are ws
  conventions and fine.
- a commit hash.
- a specific file of this repository that bootstrap does not install
  (`ai-docs/ref/worktree-ticket-scope.md`, the `skill-authoring` manual, this
  manual), or this repository's own layout and tooling: `agents-plugin/`,
  `agents-plugin-tool/`, `agents-plugin-wsflow/`, `claude-plugin/`,
  `install.sh`, `wsflow-mirroring`.
- this repository's migration vocabulary as a rule: migration anchor,
  native-subagent pivot, spawn-removal, host-neutral migration, adapter
  boundaries, retired Claude tree, Codex-first.

## Applying It

A rule `AGENTS.md` imposes on sessions in this repository is a rule for this
repository, not a rule the shipped playbooks impose on every project.

When shipped text needs project-specific input it reads it through a generic
hook - `infra.read`, `convention.read`, a declared `AGENTS.md` section, a
`config.list` key - and this repository declares its own value behind that
hook.

A ticket that asks shipped playbooks to "honor" or "enforce" a rule from
`AGENTS.md` is asking for a leak; push back and redirect it to a hook.

A test that pins shipped text pins the leak too: a pinned devenv-only string
is a bug in the test as well as in the text.

Before committing any change under the shipped surfaces, re-read the changed
text as a lead in a project that has never heard of devenv.
