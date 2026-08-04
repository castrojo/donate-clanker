---
name: mcp-app
version: "1.1"
last_updated: 2026-08-04
id: mcp-app
one_line_purpose: Build read-only Goose Desktop MCP Apps as typed UI resources.
entry_point: docs/skills/mcp-app.md
category: meta
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [goose, mcp, ui, react, desktop]
description: "Builds a stateless Goose Desktop MCP App resource with a server-only data boundary. Use when adding a Goose Desktop panel, dashboard, or interactive MCP resource."
metadata:
  type: reference
  context7-sources: [/aaif-goose/goose]
---

# Goose Desktop MCP Apps

## When to Use

Load this when adding a browser-based Goose Desktop MCP App or exposing a
`ui://` resource from an MCP server.

## When NOT to Use

Do not use this for contributor runtime behavior, Hive assignment, tmux
inspection, or a persistent web service. Hive owns those boundaries.

## Core Process

1. Keep the MCP server as the data boundary. The browser resource calls a
   read-only MCP tool through the host bridge; it does not receive credentials
   or make direct network requests.
2. Declare both `tools` and `resources` capabilities. Serve the panel as a
   `ui://` resource with MIME type `text/html;profile=mcp-app`, then set the
   opening tool result's `_meta.ui.resourceUri` to that resource.
3. Give the resource a restrictive CSP with empty connect, resource, frame,
   and base URI domain lists when the UI has no browser network requirement.
4. Normalize source payloads into explicit known, empty, unknown, and stale
   states before rendering. Independently failing providers must leave each
   other visible. Model a count as that state type, never as a nullable
   number, so the view can distinguish zero from unavailable. Use a stale value
   only when the server returns an explicit last-success timestamp; never
   silently reuse a prior result.
5. Validate every nested host payload before putting it in UI state. Invalid
   data remains unknown; it must never crash a rendered resource.
6. Authorize GitHub reads in the server from `REVIEW_GH_TOKEN`, then
   `GH_TOKEN`. Never use a Copilot token for GitHub REST calls, and return only
   a boolean capability indicator to the UI. Redact provider errors to a source
   and status category.
7. Label every fact with its source and observation time, and distinguish
   self-reported Hive completion from GitHub-observed state. Encode status with
   a text label and a shape as well as color, check foreground/background pairs
   against WCAG AA, and do not animate, blink, or pulse.
8. Build the client once into the resource and run the MCP server over stdio.
   Do not create a watcher, daemon, polling loop, or persistence layer.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "The UI can fetch the provider directly." | That moves auth and CSP risk into the browser; use the server tool boundary. |
| "A top-level JSON check is enough." | Partial nested evidence can still crash rendering; guard every rendered field. |
| "A refresh timer is harmless." | It changes a snapshot panel into a background lifecycle; require explicit refresh. |
| "An empty count and a failed source both render as nothing." | They mean opposite things to a reviewer. Keep zero and unavailable distinct in the model and in the view. |
| "An undocumented endpoint shape is close enough." | Isolate it in an adapter and label an unrecognized response `unknown`; do not present a guess as evidence. |

## Red Flags

- A credential value, authorization header, or raw provider error crosses into
  the resource response.
- The UI uses `fetch`, a WebSocket, `setInterval`, local storage, or a
  background worker.
- A resource advertises a browser `connectDomain` despite server-side data
  access being available.
- A status view mutates Hive, GitHub, contributor, or launcher state.
- A Copilot token is used for a GitHub REST call.
- A count renders identically whether it is zero or unavailable.
- Status is conveyed by color alone.
- The panel lists or selects actionable work rather than reporting its count.

## Verification

- [ ] Server declares `{ tools: {}, resources: {} }`.
- [ ] Resource is `text/html;profile=mcp-app` and opens through
  `_meta.ui.resourceUri`.
- [ ] Browser CSP permits no direct provider connection.
- [ ] Every UI payload is nested-shape validated before rendering.
- [ ] Source failures render an explicit unknown or stale state independently.
- [ ] The package type-checks and builds.

## Sources

- Goose MCP App tutorial (Context7: `/aaif-goose/goose`).
