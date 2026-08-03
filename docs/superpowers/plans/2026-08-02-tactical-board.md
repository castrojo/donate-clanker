# Tactical Board Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bundle a read-only Tactical Board MCP App into the contributor image so Goose can display authoritative Hive status, knowledge freshness, prompt provenance, and reviewer links.

**Architecture:** A Node stdio MCP server fetches the canonical Hive contributor status only when Goose opens the board, then serves a sandboxed `ui://` HTML resource. A small pure snapshot module validates the configured Hive URL, redacts all secrets by omission, and reports only status metadata plus local `agent.md` freshness. The app neither reads tmux nor controls Hive or GitHub.

**Tech Stack:** Node 24, `@modelcontextprotocol/sdk`, Goose MCP Apps, static HTML/CSS, Node test runner, existing shell image contracts.

## Global Constraints

- Use the selected Tactical Board layout with Catppuccin Mocha-adjacent colors.
- Do not add a daemon, persistent state, task filters, queue controls, tmux scraping, or token output.
- Hive is authoritative for assignment and status; GitHub is authoritative for issue, PR, review, and merge state.
- “Hive complete” must never be represented as review or merge completion.
- The Bluefin policy source is `image/config/local-agent-policy.md`; expose it as **Improve this prompt**.
- Do not add the deferred YouTube player.
- Manual Mode is a visual fallback only: it cannot stop Hive or select, cache, rank, or assign work. It opens GitHub's canonical project issue search for the human to choose manually.

---

### Task 1: Implement safe snapshot collection

**Files:**
- Create: `image/bluefin-operations/package.json`
- Create: `image/bluefin-operations/snapshot.js`
- Create: `image/bluefin-operations/snapshot.test.js`

**Interfaces:**
- Produces `buildSnapshot({ hiveHub, agentMdPath, fetchImpl, now }) -> Promise<Snapshot>`.
- `Snapshot` has `{ hive: { state, activeContributors, registeredContributors, actionableItems, contributorUrl, dashboardUrl }, knowledge: { state, refreshedAt }, sources: { hivePromptUrl, bluefinPolicyUrl } }`.
- `Snapshot.hive.manualIssuesUrl` is `https://github.com/issues?q=is%3Aopen+is%3Aissue+org%3Aprojectbluefin`; it is shown only for `unavailable` or `suspended` Hive state.

- [ ] **Step 1: Write failing Node tests**

```js
import test from "node:test";
import assert from "node:assert/strict";
import { buildSnapshot } from "./snapshot.js";

test("converts a secure Hive contributor endpoint into canonical read-only links", async () => {
  const snapshot = await buildSnapshot({
    hiveHub: "wss://hive.example/contribute",
    agentMdPath: "/missing",
    fetchImpl: async () => new Response(JSON.stringify({
      hub: "online", active_contributors: 2, total_registered: 4, actionable_items: 8,
    })),
    now: () => new Date("2026-08-02T00:00:00Z"),
  });
  assert.equal(snapshot.hive.contributorUrl, "https://hive.example/contribute");
  assert.equal(snapshot.hive.activeContributors, 2);
  assert.equal(snapshot.knowledge.state, "unavailable");
});
```

- [ ] **Step 2: Run the tests and observe failure**

Run: `node --test image/bluefin-operations/snapshot.test.js`

Expected: FAIL because `snapshot.js` does not exist.

- [ ] **Step 3: Implement `buildSnapshot`**

```js
const contributorUrl = new URL(hiveHub);
contributorUrl.protocol = contributorUrl.protocol === "wss:" ? "https:" : "http:";
contributorUrl.pathname = "/contribute";
const statusUrl = new URL("/api/contribute/status", contributorUrl);
const response = await fetchImpl(statusUrl, { signal: AbortSignal.timeout(8_000) });
```

Reject a non-HTTP(S)/WS(S) hub, never include its query string or credentials in the result, map a fetch failure to `hive.state = "unavailable"`, and use `stat(agentMdPath).mtime` only for knowledge freshness.

- [ ] **Step 4: Run the focused tests**

Run: `node --test image/bluefin-operations/snapshot.test.js`

Expected: PASS.

### Task 2: Implement the MCP App and Tactical Board resource

**Files:**
- Create: `image/bluefin-operations/server.js`
- Create: `image/bluefin-operations/index.html`
- Create: `image/bluefin-operations/server.test.js`

**Interfaces:**
- Consumes `buildSnapshot`.
- Produces a model-visible `show_bluefin_operations` tool and `ui://bluefin-operations/tactical-board` resource with MIME type `text/html;profile=mcp-app`.

- [ ] **Step 1: Write failing protocol tests**

```js
test("advertises the Tactical Board resource", async () => {
  const resources = await listResources();
  assert.deepEqual(resources.resources[0], {
    uri: "ui://bluefin-operations/tactical-board",
    name: "Bluefin Operations Tactical Board",
    mimeType: "text/html;profile=mcp-app",
  });
});
```

- [ ] **Step 2: Run the tests and observe failure**

Run: `node --test image/bluefin-operations/server.test.js`

Expected: FAIL because the server does not exist.

- [ ] **Step 3: Implement the stdio server**

Use `@modelcontextprotocol/sdk` request schemas for tools and resources. On `show_bluefin_operations`, collect an ephemeral snapshot, return text stating when it was retrieved, and attach `_meta.ui.resourceUri`. On resource read, interpolate only escaped snapshot fields into the static Tactical Board HTML. Set a restrictive resource CSP with no `connectDomains`, no `resourceDomains`, and no `frameDomains`.

The HTML must contain the original Bluefin ASCII banner, the original line “The tide remembers every horizon.”, Hive/knowledge states, a reviewer warning, and links labeled **Open hosted contributor dashboard**, **Open full Hive dashboard**, **View Hive prompt source**, and **Improve this prompt**.

When `hive.state` is `unavailable` or `suspended`, render a compact **Manual Mode** panel: “Hive assignments are unavailable. Stop or resume Hive in its hosted dashboard; choose work directly in GitHub.” Its sole control is an `Open project issues` link to `manualIssuesUrl`. It must not call any Hive mutation endpoint and must not list issue titles.

- [ ] **Step 4: Run focused tests**

Run: `node --test image/bluefin-operations/server.test.js`

Expected: PASS.

### Task 3: Package the app into the contributor image

**Files:**
- Modify: `image/Containerfile`
- Modify: `image/config/goose.yaml`
- Modify: `tests/image-contract.sh`
- Modify: `README.md`

**Interfaces:**
- Installs `/opt/bluefin/bluefin-operations` and enables the `bluefin_operations` command extension from the controlled Goose config.

- [ ] **Step 1: Extend image-contract assertions first**

Assert that the Containerfile copies the app, installs `@modelcontextprotocol/sdk`, and the Goose config contains an enabled `bluefin_operations` stdio extension pointing to `node /opt/bluefin/bluefin-operations/server.js`.

- [ ] **Step 2: Run the image contract and observe failure**

Run: `bash tests/image-contract.sh`

Expected: FAIL because the app is not packaged or configured.

- [ ] **Step 3: Install and configure**

Copy the package files before running `npm --prefix /opt/bluefin/bluefin-operations ci --omit=dev`, then copy `server.js`, `snapshot.js`, and `index.html`. Preserve ownership by creating `/opt/bluefin/bluefin-operations` as `dev`. Add a standard I/O extension in `image/config/goose.yaml`; do not put its configuration under `~/.config/goose`.

Document the app’s purpose, source links, lack of task controls, and `show_bluefin_operations` entry point in `README.md`.

- [ ] **Step 4: Run validation**

Run: `node --test image/bluefin-operations/*.test.js && bash tests/image-contract.sh && bash tests/just-onboarding.sh && git diff --check`

Expected: all commands pass.

- [ ] **Step 5: Commit**

```bash
git add image/bluefin-operations image/Containerfile image/config/goose.yaml tests/image-contract.sh README.md
git commit -m "feat: add Bluefin Operations Tactical Board"
```
