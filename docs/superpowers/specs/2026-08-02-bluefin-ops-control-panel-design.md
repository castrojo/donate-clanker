# Bluefin Ops Control Panel Design

## Goal

Add a read-only Goose Desktop MCP App that gives a contributor one dense,
single-view answer to: "what is the state of Bluefin work right now, and where
do I go next?" It is a presentation layer over Hive and GitHub only. It does
not participate in Hive's contributor protocol, task selection, or lifecycle.

## Selected approach

The panel will be a small Node MCP server plus a bundled React/TypeScript
resource:

- The server exposes one `show_bluefin_ops_control_panel` tool and a
  `ui://bluefin-ops-control-panel/main` resource. Goose Desktop renders the
  resource as `text/html;profile=mcp-app`.
- The server is the only component that makes HTTP requests. The browser UI
  calls one read-only MCP tool through the MCP App JSON-RPC bridge to request a
  snapshot. This keeps all credential material outside the sandboxed UI.
- The React bundle is embedded in the resource HTML. A data normalizer converts
  source-specific response documents to an explicit `known` / `empty` /
  `unknown` / `stale` data model before rendering.

This is preferred over a browser-direct implementation because credentials
never enter browser code, CSP can remain restrictive, and source failure
handling stays in one auditable boundary. It is preferred over embedding static
HTML because the ledger needs independently updating regions and clear data
state semantics.

## Scope and boundaries

The app is stateless and foreground-only. It makes one Hive request group and
one GitHub request group when opened, then makes no further requests until the
user activates the single **Refresh all evidence** control. It creates no
files, stores no browser data, keeps no timer, opens no WebSocket, and offers
no mutation controls.

The panel never lists or selects actionable work. It presents only the
actionable count. It never reads a tmux pane and never reaches the contributor
WebSocket. Hive completion is labeled as self-reported workflow completion;
GitHub PR/review state remains separately labeled as GitHub truth. The
reconciliation copy explains that the VM guest has no GitHub identity mapping,
so GitHub-visible state can legitimately follow Hive completion.

## Data flow

`show_bluefin_ops_control_panel` opens the resource. On initialization the
React client calls `get_ops_control_panel_snapshot` once through the host
bridge. The server starts Hive and GitHub reads in parallel, isolates errors,
redacts error messages to source and status category, and returns a typed
snapshot. The client renders an initial unknown state synchronously and replaces
only the regions represented by returned evidence. The refresh control repeats
that one aggregate tool call.

Hive data covers connectivity, active contributor descriptors, actionable
count, knowledge-export age, skill/doc drift, and prompt provenance. GitHub
data covers pull request/review links and canonical issue links for Manual
Mode. A Hive source failure changes the overall mode to the visibly distinct
Manual Mode and keeps GitHub panels available; a GitHub source failure leaves
Hive evidence intact and labels GitHub facts unknown.

The server obtains GitHub API authorization only from the existing container
identity path: `DONATE_CLANKER_GH_TOKEN`, then `GH_TOKEN`. It does not use
`GITHUB_COPILOT_TOKEN` for GitHub API calls and never returns, logs, or
serializes an authorization value. The UI receives only a boolean capability
indicator.

## Ledger layout

The view uses Catppuccin Mocha design tokens defined once in CSS custom
properties. It is a compact, responsive grid of labeled ledger regions:

1. Header: Bluefin Ops Control Panel, mode badge, snapshot timestamp, refresh.
2. Signal strip: Hive connectivity, active contributors, actionable work, and
   knowledge freshness.
3. Evidence ledger: prompt provenance, skill/doc drift, GitHub PR/review
   links, and the Hive-complete versus GitHub-truth reconciliation.
4. Manual Mode: a strongly labeled fallback panel with canonical GitHub issue
   links when Hive is unavailable.
5. Footer: Bluefin factory policy and hosted Hive dashboard links.

Every fact shows a source and observation timestamp. Count rows distinguish
zero from unavailable, and stale data is only used when the server returns an
explicit last-success timestamp; the app never silently caches a prior result.
Status uses a short text label and a shape in addition to color. Catppuccin
Mocha `text` (#cdd6f4) on `base` (#1e1e2e) and semantic foregrounds are checked
against WCAG AA contrast before use.

## Host and endpoint assumptions

- Goose Desktop follows its documented MCP App convention: a `ui://` resource
  with MIME type `text/html;profile=mcp-app`, exposed by a tool result's
  `_meta.ui.resourceUri`; browser-to-server calls use the MCP App JSON-RPC
  bridge.
- The known Hive references document `/api/knowledge/export` and
  `/api/contribute/ws`, but do not document the HTTP status endpoint set or
  response shapes needed by this panel. The implementation will isolate them
  in a configured `HiveApi` adapter and label unsupported/malformed responses
  `unknown`; it will not pretend those shapes are authoritative.
- GitHub calls use documented REST endpoints selected by configured repository
  scope. The initial prototype reads public review/PR data without exposing a
  token and uses `DONATE_CLANKER_GH_TOKEN` / `GH_TOKEN` only in the server when
  available.
- The existing repository has no JavaScript build convention. The app will
  introduce a minimal isolated package under `mcp-app/`, rather than change the
  launcher or image lifecycle.

## Validation

Validate the package build and TypeScript check, inspect the generated resource
for absent credential names/values, run the repository's existing launcher and
image contracts unchanged, and run `git diff --check`. Automated test coverage
is intentionally deferred for this visual-prototype pass.
