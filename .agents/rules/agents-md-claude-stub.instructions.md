---
name: 'AGENTS.md CLAUDE.md Stub'
description: 'Require a CLAUDE.md stub pointing to every AGENTS.md'
applyTo: '**/AGENTS.md'
---
- Whenever a new `AGENTS.md` file is added anywhere in the repo, create a `CLAUDE.md` file in that same directory containing only `@AGENTS.md`.
- `CLAUDE.md` files must never contain anything other than a pointer to an `AGENTS.md` file in the same directory. Do not add prose, instructions, or other content directly to a `CLAUDE.md` file — put that content in `AGENTS.md` instead.
- If an `AGENTS.md` file is removed and no longer needed, remove its corresponding `CLAUDE.md` stub in the same change.
