# Development Guidelines

## Documentation
First find and read the relevant codebase, then act.

## Glossary
- "thin client", "thin node", "alias", "mobile client", "mobile node", "device" - Android or IOS implementations of Warpnet
- "fat node", "backend", "server node" - main node of Warpnet hosted on computer serving pairing with devices

## Code exploration
Always start to explore the codebase from the files:
  - cmd/node/member/main.go
  - cmd/node/member/app.go
Prefer Serena's symbolic tools (`find_symbol`, `find_referencing_symbols`, `replace_symbol_body`, backed by `gopls`) over text search for Go code. If they seem unavailable (e.g. after `/compact`), ask to "read Serena's initial instructions". Setup: `.claude/SERENA.md`.

## Build Artifacts
- Read `vendor`, `dist` directory only if you need context of a code dependency
- Do NOT modify the `vendor`, `dist` directory manually.
- If a build changes `vendor`, `dist`, restore it to its previous state before committing.

## Code Changes
- Make the smallest possible changes required to solve the task.
- Avoid refactoring or unrelated edits.
- Do not add fat comment blocks

## AI-generated Comments
- Validate all comments and suggestions from Codex and Copilot:
    - Ensure correctness.
    - Ensure relevance.
    - Discard low-value or incorrect suggestions.

## AI Attribution Ban
- NEVER mention "Claude" or any AI assistant anywhere in this repository: no `claude/` branch prefixes, no AI co-author or session trailers in commit messages, no AI references in code, comments, or docs.
- Commits are authored under the repository owner's identity only.
- If tooling auto-creates a `claude/...` working branch, rename it (e.g. `feature/<topic>`) before pushing and delete the prefixed branch.
