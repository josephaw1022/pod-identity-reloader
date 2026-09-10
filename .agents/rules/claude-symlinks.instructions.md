---
name: 'Claude/GitHub Agent Symlinks'
description: 'Keep .claude and .github agent config as symlinks into .agents, the single source of truth'
applyTo: '.agents/**'
---
`.agents/` is the single source of truth for repo-wide AI agent configuration
(`agents/`, `rules/`, `skills/`). `.claude/` and `.github/` must never hold
real copies of this content — they exist only so each tool can discover it.

- Every file under `.claude/agents/`, `.claude/rules/`, `.claude/skills/`,
  `.github/agents/`, and `.github/instructions/` must be a symlink pointing
  back at the matching file/folder under `.agents/`, e.g.
  `.claude/rules/foo.instructions.md -> ../../.agents/rules/foo.instructions.md`.
- Symlink individual files/folders, never whole directories.
- When adding a new agent/rule/skill under `.agents/`, add matching symlinks
  in both `.claude/` and `.github/` in the same change.
- When removing something from `.agents/`, remove its symlinks too.
- To edit content, edit the file under `.agents/` — the symlinks need no
  further changes.
