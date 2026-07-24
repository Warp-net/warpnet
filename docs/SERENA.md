# Serena + gopls: symbolic LSP tooling for Claude Code

[Serena](https://github.com/oraios/serena) is an MCP server that gives an AI
agent *symbolic* code operations backed by a real language server, instead of
plain text search. For this Go repo the backend runs
[`gopls`](https://pkg.go.dev/golang.org/x/tools/gopls), so the agent can call:

- `find_symbol` — jump to a type/func/method by name
- `find_referencing_symbols` — find every caller/reference of a symbol
- `replace_symbol_body` — edit a whole symbol body precisely

instead of grepping over text. This makes navigation and refactors on the node
codebase (`core/`, `database/`, `event/`, `cmd/node/...`) far more accurate.

## Prerequisites

- **`uv`** — Serena is launched through it:
  ```bash
  curl -LsSf https://astral.sh/uv/install.sh | sh
  ```
- **`gopls`** — Serena downloads and manages its own language servers, so this
  is usually automatic. If you prefer a system `gopls`, check for it and install
  when missing:
  ```bash
  which gopls || go install golang.org/x/tools/gopls@latest
  # make sure it is on PATH:
  export PATH="$PATH:$(go env GOPATH)/bin"
  ```

## Connect it to Claude Code (local scope)

Run this **from the repo root** so Serena indexes this project:

```bash
claude mcp add serena -- \
  uvx --from git+https://github.com/oraios/serena \
  serena start-mcp-server --context ide-assistant --project "$(pwd)"
```

`claude mcp add` registers the server in **local scope** by default: it is
stored in your per-project user settings, private to you, and is *not* committed
to the repo. Once added, Claude Code connects to Serena automatically every time
you open a session in this directory. Verify with:

```bash
claude mcp list        # should show: serena
```

## Gotchas

- **`.serena/` is gitignored.** Serena writes its project config and memories to
  `.serena/` in the repo. It is listed in `.gitignore` on purpose — this repo is
  public and the cache does not belong in history.
- **After `/compact`**, the agent sometimes forgets how to use Serena's tools.
  Ask it to *"read Serena's initial instructions"* to reload them.

## Reference

- Serena: https://github.com/oraios/serena
- Guide: https://mcp.directory/blog/serena-mcp-complete-guide-2026
