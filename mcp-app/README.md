# Bluefin Ops Control Panel MCP App

This package is a read-only Goose Desktop MCP App. It is a presentation layer
only: it does not replace `just review`, does not participate in Hive
task selection, and never mutates launcher state.

## Registering the app

Run the bundled server over stdio:

```bash
node mcp-app/dist/server.js
```

In Goose Desktop, register that command as an MCP app entry. The server exposes:

- `show_bluefin_ops_control_panel` — opens the UI resource.
- `get_ops_control_panel_snapshot` — returns one JSON evidence snapshot.
- `ui://bluefin-ops-control-panel/main` — the rendered `text/html;profile=mcp-app`
  resource.

The UI loads once, renders the current evidence, and refreshes only when the
user activates the single **Refresh all evidence** control. It is stateless and
foreground-only.

## Configuration

All configuration comes from environment variables.

### Hive

- `HIVE_API_BASE_URL` — Hive API base URL.
- `HIVE_CONNECTIVITY_PATH`
- `HIVE_CONTRIBUTORS_PATH`
- `HIVE_ACTIONABLE_COUNT_PATH`
- `HIVE_PROVENANCE_PATH`

`/api/knowledge/export` is the only fixed Hive path. The other Hive endpoint
paths are configurable because this app does not assume an authoritative
response contract for them. If a configured response is missing, malformed, or
uses an unsupported shape, the server labels that evidence `unknown`.

### GitHub

- `BLUEFIN_GITHUB_OWNER`
- `BLUEFIN_GITHUB_REPOSITORIES` — comma-separated repository names.
- `REVIEW_GH_TOKEN`
- `GH_TOKEN`

For GitHub API requests, the server uses `REVIEW_GH_TOKEN` first and
then `GH_TOKEN`. It never uses a Copilot token, never exposes credential values
to the UI, and never logs them.

## Notes

- The app observes evidence only.
- It does not replace the launcher or its foreground-only behavior.
- When an endpoint or payload shape is unsupported, the UI should show
  `unknown`, not invent a value.
