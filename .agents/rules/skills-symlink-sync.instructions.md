---
name: 'Skills Symlink Sync'
description: 'Require .claude/skills symlinks for every skill added under .agents/skills'
applyTo: '.agents/skills/**'
---
`.agents/skills/` is the single source of truth for repo skill content.
`.claude/skills/` and `.github/agents/` must never hold real copies — they
only exist so each tool can discover the skill.

- When adding a new skill folder under `.agents/skills/<name>/`, add a
  matching symlink `.claude/skills/<name> -> ../../.agents/skills/<name>` in
  the same change.
- Symlink the whole skill folder, not individual files inside it.
- When removing a skill from `.agents/skills/`, remove its `.claude/skills/`
  symlink too.
- To edit a skill, edit its files under `.agents/skills/` — the symlink
  needs no further changes.
