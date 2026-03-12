## Execution Protocol (follow strictly)

> Source: @.claude/skills/implement/SKILL.md — do NOT skip these steps.

1. **Task tracking**: Before writing any code, create tasks with `TaskCreate` for
   each implementation unit, testing, doc updates, and commit. Track progress with
   `TaskUpdate` (`in_progress` → `completed`).
2. **Understand first**: Read `ai-docs/_index.md` and relevant
   `ai-docs/mental-model/` domain docs before coding.
3. **Code standards**: Follow CLAUDE.md Code Standards (simplicity, surgical changes).
4. **Testing**: Run the project test suite after implementation
   (see CLAUDE.md `# MEMORY → Build & Workflow`). All tests must pass.
5. **Doc updates (MANDATORY — do not skip)**:
   - `ai-docs/_index.md` if capabilities changed
   - Mental-model updates: delegate to a **background subagent** using
     `mental-model-update-agent.md` prompt (in the implement skill directory)
   - `# MEMORY` section in `CLAUDE.md`
   - Append `### Result` to ticket doc if completing a phase
6. **Commit format**: `<type>(<scope>): <summary>` + body + `## AI Context` block.
   Include doc changes in the commit.
