# Bluefin Ops Control Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a read-only, stateless Goose Desktop MCP App that presents
Bluefin's current Hive and GitHub evidence in a dense Tactical Ledger view.

**Architecture:** A minimal Node MCP server publishes a bundled React resource
and owns all HTTP reads. The browser resource makes one aggregate, read-only
tool call on open or explicit refresh, then renders source-normalized evidence
with independent source failures. Hive and GitHub adapters never mutate
resources, and credentials remain process-local to the server.

**Tech Stack:** Node.js 18+, TypeScript, React, esbuild,
`@modelcontextprotocol/sdk`, Goose Desktop MCP App resources.

## Global Constraints

- The panel is read-only and stateless: no writes, persistence, polling,
  timers, WebSockets, task selection, tmux access, or services.
- Fetch Hive and GitHub once per open and only again after the one global
  refresh action.
- Keep `GITHUB_COPILOT_TOKEN`, `DONATE_CLANKER_GH_TOKEN`, `GH_TOKEN`, and all
  credential values out of UI payloads and logs.
- Resolve `DONATE_CLANKER_GH_TOKEN` before `GH_TOKEN` only in the server;
  never use a Copilot token for GitHub REST calls.
- Treat Hive completion and GitHub merge/review state as separate facts.
- Use Catppuccin Mocha CSS variables, WCAG-AA foreground/background pairs, and
  text-plus-shape status cues.
- Do not add automated tests in this visual-prototype pass; build and type
  validation are required.
- Document all non-authoritative Hive endpoint and host bridge assumptions in
  the MCP App README.

---

## File Structure

| Path | Responsibility |
| --- | --- |
| `mcp-app/package.json` | Isolated application scripts and dependencies. |
| `mcp-app/tsconfig.json` | Strict TypeScript compiler configuration. |
| `mcp-app/scripts/build.mjs` | Builds the browser bundle and server without a runtime dev service. |
| `mcp-app/src/contracts.ts` | Source-neutral evidence, data-state, and UI snapshot types. |
| `mcp-app/src/sources/hive.ts` | Configurable Hive HTTP adapter with response validation. |
| `mcp-app/src/sources/github.ts` | GitHub REST adapter with server-only token resolution. |
| `mcp-app/src/snapshot.ts` | Runs independent source reads and creates the aggregate snapshot. |
| `mcp-app/src/server.ts` | MCP tools/resources, restrictive CSP, and no-secret error handling. |
| `mcp-app/src/ui/main.tsx` | React startup, one-shot snapshot lifecycle, and global refresh. |
| `mcp-app/src/ui/components.tsx` | Small, accessible ledger components. |
| `mcp-app/src/ui/styles.css` | Catppuccin token set and Tactical Ledger layout. |
| `mcp-app/src/ui/index.html` | Resource HTML template with only the compiled client asset. |
| `mcp-app/README.md` | Launch instructions, endpoint/host assumptions, and security boundary. |
| `README.md` | Links the shipped Desktop MCP App as an optional presentation layer. |

### Task 1: Create the isolated MCP App package

**Files:**
- Create: `mcp-app/package.json`
- Create: `mcp-app/tsconfig.json`
- Create: `mcp-app/scripts/build.mjs`
- Create: `mcp-app/src/ui/index.html`

**Interfaces:**
- Produces: `npm run build`, `npm run typecheck`, and `dist/server.js`.
- Consumes: Node 18+ and Goose Desktop's stdio MCP server registration.

- [ ] **Step 1: Add package metadata and fixed build scripts**

```json
{
  "name": "@projectbluefin/ops-control-panel",
  "private": true,
  "type": "module",
  "scripts": {
    "build": "node scripts/build.mjs",
    "typecheck": "tsc --noEmit"
  }
}
```

Use only `react`, `react-dom`, `esbuild`, `typescript`, and
`@modelcontextprotocol/sdk`; do not add a framework or server framework.

- [ ] **Step 2: Add strict compiler settings**

```json
{
  "compilerOptions": {
    "strict": true,
    "target": "ES2022",
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "jsx": "react-jsx",
    "noEmit": true
  },
  "include": ["src/**/*.ts", "src/**/*.tsx"]
}
```

- [ ] **Step 3: Add the one-shot bundle script**

The script must build `src/server.ts` for Node and bundle
`src/ui/main.tsx` into an inline-safe browser asset. It reads the HTML template
at build time and writes only `dist/`; it must not start a watcher or dev
server.

- [ ] **Step 4: Build and type-check**

Run: `npm --prefix mcp-app run typecheck && npm --prefix mcp-app run build`

Expected: both commands exit zero and `mcp-app/dist/server.js` exists.

- [ ] **Step 5: Commit package foundation**

```bash
git add mcp-app/package.json mcp-app/tsconfig.json mcp-app/scripts/build.mjs mcp-app/src/ui/index.html
git commit -m "feat: scaffold Bluefin ops MCP app"
```

### Task 2: Define the source-neutral evidence contracts and adapters

**Files:**
- Create: `mcp-app/src/contracts.ts`
- Create: `mcp-app/src/sources/hive.ts`
- Create: `mcp-app/src/sources/github.ts`
- Create: `mcp-app/src/snapshot.ts`

**Interfaces:**
- Produces: `getSnapshot(config: SnapshotConfig): Promise<OpsSnapshot>`.
- Produces: `DataState<T> = Known<T> | Empty | Unknown | Stale<T>`.
- Consumes: `HIVE_API_BASE_URL`, `BLUEFIN_GITHUB_OWNER`, and
  `BLUEFIN_GITHUB_REPOSITORIES` as non-secret configuration.

- [ ] **Step 1: Define explicit state and evidence types**

```ts
export type DataState<T> =
  | { kind: "known"; value: T; observedAt: string; source: "hive" | "github" }
  | { kind: "empty"; observedAt: string; source: "hive" | "github" }
  | { kind: "unknown"; source: "hive" | "github"; reason: string }
  | { kind: "stale"; value: T; observedAt: string; source: "hive" | "github"; reason: string };

export interface OpsSnapshot {
  mode: "live" | "manual";
  generatedAt: string;
  hive: HiveEvidence;
  github: GitHubEvidence;
  githubAuthAvailable: boolean;
}
```

Model counts as `DataState<number>`, never as nullable numbers, so the UI can
render zero, unknown, and stale facts differently.

- [ ] **Step 2: Implement the Hive adapter with local response guards**

Define endpoint paths in one `HiveEndpoints` configuration object. Use the
documented `/api/knowledge/export` path for knowledge timestamp extraction.
Keep currently undocumented connectivity, contributor, count, and provenance
paths configurable through `HIVE_*_PATH` environment names. For each request,
use `fetch` once with an `AbortSignal.timeout(8_000)` and convert malformed,
missing, or unavailable responses into `unknown` with a safe reason. Do not
retain an old value in the process.

- [ ] **Step 3: Implement GitHub REST evidence**

Read `DONATE_CLANKER_GH_TOKEN ?? GH_TOKEN` inside the adapter, construct an
authorization header only when it is non-empty, and omit authorization
otherwise. Request `/repos/{owner}/{repo}/pulls?state=open` and
`/repos/{owner}/{repo}/issues?state=open` for configured repositories. Return
only title, number, URL, labels, review status, and observation timestamp. Do
not return request headers or token-presence source names.

- [ ] **Step 4: Aggregate independent source reads**

```ts
export async function getSnapshot(config: SnapshotConfig): Promise<OpsSnapshot> {
  const [hive, github] = await Promise.all([
    collectHiveEvidence(config.hive),
    collectGitHubEvidence(config.github),
  ]);
  return {
    mode: hive.connectivity.kind === "known" ? "live" : "manual",
    generatedAt: new Date().toISOString(),
    hive,
    github,
    githubAuthAvailable: config.github.tokenAvailable,
  };
}
```

Do not let either promise reject after normalization. Manual Mode depends on
Hive connectivity only, not GitHub availability.

- [ ] **Step 5: Type-check and build**

Run: `npm --prefix mcp-app run typecheck && npm --prefix mcp-app run build`

Expected: both commands exit zero.

- [ ] **Step 6: Commit source adapters**

```bash
git add mcp-app/src/contracts.ts mcp-app/src/sources mcp-app/src/snapshot.ts
git commit -m "feat: add read-only ops evidence sources"
```

### Task 3: Publish the Goose MCP resource

**Files:**
- Create: `mcp-app/src/server.ts`

**Interfaces:**
- Produces: `show_bluefin_ops_control_panel` and
  `get_ops_control_panel_snapshot` MCP tools.
- Produces: `ui://bluefin-ops-control-panel/main` as
  `text/html;profile=mcp-app`.
- Consumes: `getSnapshot(config)`.

- [ ] **Step 1: Register resource and tool capabilities**

Use `Server` with both `{ tools: {}, resources: {} }`. Register exactly the
open-panel tool and snapshot tool. The open-panel result must set:

```ts
_meta: {
  ui: { resourceUri: "ui://bluefin-ops-control-panel/main" }
}
```

The snapshot tool has an empty object input schema and returns the JSON
serialized `OpsSnapshot` only.

- [ ] **Step 2: Serve the bundled resource with restrictive CSP**

Read the generated HTML once at server start. The resource must declare:

```ts
_meta: {
  ui: {
    csp: { connectDomains: [], resourceDomains: [], frameDomains: [], baseUriDomains: [] },
    prefersBorder: true
  }
}
```

The sandbox receives no browser network permission because all reads happen in
the server.

- [ ] **Step 3: Keep server failures safe**

Write operational server startup errors to stderr without serializing
environment variables, headers, or thrown provider response bodies. Return
source-category errors from adapters, not raw network errors.

- [ ] **Step 4: Build and inspect the public artifact**

Run:

```bash
npm --prefix mcp-app run typecheck
npm --prefix mcp-app run build
rg -n "DONATE_CLANKER_GH_TOKEN|GITHUB_COPILOT_TOKEN|GH_TOKEN" mcp-app/dist
```

Expected: type-check and build pass. Any credential identifiers in the server
bundle are never emitted into the browser asset or resource response.

- [ ] **Step 5: Commit MCP host integration**

```bash
git add mcp-app/src/server.ts
git commit -m "feat: expose Bluefin ops MCP resource"
```

### Task 4: Implement the Tactical Ledger React view

**Files:**
- Create: `mcp-app/src/ui/main.tsx`
- Create: `mcp-app/src/ui/components.tsx`
- Create: `mcp-app/src/ui/styles.css`

**Interfaces:**
- Consumes: `OpsSnapshot`, `DataState<T>`, and the host bridge call
  `tools/call` for `get_ops_control_panel_snapshot`.
- Produces: an initial unknown state, then one rendered evidence snapshot.

- [ ] **Step 1: Implement bridge initialization and one-shot lifecycle**

Render the complete ledger immediately with unknown evidence. After bridge
initialization, call the aggregate snapshot tool once. Store the result in
React component state only. Provide one `<button>` named **Refresh all
evidence** that repeats the same call; do not use `setInterval`, effects that
re-fetch on render, timers, local storage, or per-panel buttons.

- [ ] **Step 2: Build reusable accessible evidence components**

Implement `StatusMark`, `EvidenceCell`, `LedgerPanel`, `FactValue`, and
`ExternalLink`. `FactValue` maps data states to:

```tsx
known  -> <>{value}</>
empty  -> <>None reported</>
unknown -> <>Unknown — {reason}</>
stale  -> <>Stale — {value}</>
```

All status marks include a text state and an `aria-label`; links have
descriptive accessible names and use `target="_blank"` with
`rel="noreferrer"`.

- [ ] **Step 3: Render all required labeled regions**

Render Hive connectivity (endpoint and last contact), active contributors,
actionable count only, knowledge age and skill/doc drift, prompt provenance,
GitHub PR/review links, explicit Hive-complete and GitHub-truth reconciliation,
Manual Mode issue links, and both footer links. The reconciliation copy must
state that VM guests have no GitHub identity mapping and that Hive completion
can precede GitHub-visible state.

- [ ] **Step 4: Apply the CSS token system**

Define Catppuccin Mocha values exclusively in `:root` custom properties:
`--base`, `--mantle`, `--surface-0`, `--surface-1`, `--text`, `--subtext-1`,
`--blue`, `--green`, `--yellow`, `--red`, and `--mauve`. Use a tight grid,
monospaced tabular numerics, aligned label/value columns, visible focus rings,
and a narrow-screen single-column layout. Do not animate, blink, or pulse.

- [ ] **Step 5: Build and visually inspect**

Run: `npm --prefix mcp-app run typecheck && npm --prefix mcp-app run build`

Expected: both commands exit zero and the resource contains no direct external
fetch calls.

- [ ] **Step 6: Commit the ledger UI**

```bash
git add mcp-app/src/ui
git commit -m "feat: add Tactical Ledger panel UI"
```

### Task 5: Document configuration and integrate the presentation layer

**Files:**
- Create: `mcp-app/README.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: the executable `mcp-app/dist/server.js` and environment-only
  configuration.
- Produces: reproducible Goose Desktop MCP App registration instructions.

- [ ] **Step 1: Document local registration and one-shot behavior**

Document a stdio server command that runs `node mcp-app/dist/server.js`.
Explain the two exposed tools, the `ui://` resource, the single refresh
behavior, required optional configuration, and that the app is not a
replacement for `ujust donate-clanker`.

- [ ] **Step 2: Document endpoint and token assumptions explicitly**

List each configurable Hive endpoint, state which currently lacks an
authoritative response contract, and state that unsupported shapes render
unknown. Explain that the server uses only `DONATE_CLANKER_GH_TOKEN` then
`GH_TOKEN` for GitHub API requests, never a Copilot token, and never exposes
credentials to the UI or logs.

- [ ] **Step 3: Link the MCP App from the repository README**

Add a concise optional **Bluefin Ops Control Panel** subsection after the
launcher command surface. Keep foreground launcher guarantees unchanged and
say that the panel observes evidence only.

- [ ] **Step 4: Run final validation**

Run:

```bash
npm --prefix mcp-app run typecheck
npm --prefix mcp-app run build
bash tests/image-contract.sh
bash tests/just-onboarding.sh
git diff --check
```

Expected: all commands exit zero. If a launcher contract already fails before
these changes, report it without changing unrelated launcher behavior.

- [ ] **Step 5: Commit documentation**

```bash
git add mcp-app/README.md README.md
git commit -m "docs: describe Bluefin ops MCP app"
```
