# Development Guidelines

## Backend
- Explore current backend implementation https://github.com/Warp-net/warpnet and its API details

## Build Artifacts
- Do NOT modify the `dist` directory manually.
- If a build changes `dist`, restore it to its previous state before committing.

## Code Changes
- Make the smallest possible changes required to solve the task.
- Avoid refactoring or unrelated edits.

## AI-generated Comments
- Validate all comments and suggestions from Codex and Copilot:
    - Ensure correctness.
    - Ensure relevance.
    - Discard low-value or incorrect suggestions.

## Versioning
- Increment the patch version in the `version` file on every commit. Create according git tag.