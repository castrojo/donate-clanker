# Agent PR Queue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish deterministic, public Markdown and JSON snapshots of Project
Bluefin open pull requests without introducing a new queue authority.

**Architecture:** A dependency-free Node ES module fetches configured public
GitHub pull requests, normalizes and ranks them, and writes `public/queue.md`
and `public/queue.json`. A scheduled GitHub Actions workflow runs that
generator and commits changed snapshots; static hosting serves those tracked
artifacts.

**Tech Stack:** Node.js built-in `fetch`, `node:test`, GitHub REST API, GitHub
Actions, static hosting.

## Global Constraints

- GitHub is authoritative for pull requests, checks, reviews, and merge state.
- Hive remains authoritative for agent coordination and task selection.
- Index public `projectbluefin` pull requests only; do not add authentication,
  private repository support, claims, leases, webhooks, databases, KV, Durable
  Objects, polling clients, or queue mutations.
- Keep `queue.projectbluefin.io` static. The custom-domain and DNS mapping is
  an approved operations handoff, not application code.
- Never emit `merge`; emit `ready-for-human-merge` only when GitHub evidence
  shows a clean, approved pull request with successful checks.
- On a GitHub source failure, leave prior generated artifacts untouched and
  fail the refresh. Per-item incomplete evidence emits `investigate`.
- Use no new npm dependencies and no new secrets.

---

## File Structure

| Path | Responsibility |
| --- | --- |
| `queue/lib/queue.mjs` | Validate GitHub pull-request data, classify actions, group and rank items, and render both output formats. |
| `queue/lib/github.mjs` | Fetch paginated public GitHub pull-request evidence with explicit errors. |
| `queue/generate.mjs` | Read configuration, call the GitHub client and queue library, then atomically write both generated artifacts. |
| `queue/test/queue.test.mjs` | Fixture-based unit and contract tests for classification, ranking, rendering, pagination, and source failures. |
| `public/queue.md` | Generated human-readable queue snapshot, committed only after a successful refresh. |
| `public/queue.json` | Generated agent queue snapshot, committed only after a successful refresh. |
| `.github/workflows/update-pr-queue.yml` | Scheduled, manually dispatchable, and Renovate-dispatched refresh workflow. |
| `.github/workflows/validate.yml` | Runs the queue's offline Node test suite in pull-request validation. |
| `README.md` | Documents the static queue contract, snapshot freshness, and the custom-domain operations handoff. |

### Task 1: Build the offline queue model and contract

**Files:**
- Create: `queue/lib/queue.mjs`
- Create: `queue/test/queue.test.mjs`

**Interfaces:**
- Consumes: GitHub pull-request objects containing `number`, `title`,
  `html_url`, `updated_at`, `labels`, `mergeable_state`, `reviewDecision`, and
  check conclusions.
- Produces: `buildQueue({ pullRequests, generatedAt })`, returning
  `{ items, markdown, json }`; each item has `id`, `repository`, `number`,
  `url`, `title`, `updated_at`, `labels`, `review_state`, `mergeable_state`,
  `check_state`, `recommended_action`, and `ranking_reasons`.

- [ ] **Step 1: Write classification and ordering tests**

Create fixtures for clean PRs needing review, dirty PRs, failed checks,
approved/clean/successful PRs, and a PR whose mergeability is unknown. Assert
these exact actions:

```js
assert.equal(itemsByNumber.get(1).recommended_action, 'fix-ci');
assert.equal(itemsByNumber.get(2).recommended_action, 'resolve-conflicts');
assert.equal(itemsByNumber.get(3).recommended_action, 'review');
assert.equal(itemsByNumber.get(4).recommended_action, 'investigate');
assert.equal(itemsByNumber.get(5).recommended_action, 'ready-for-human-merge');
assert.deepEqual(
  result.items.map((item) => item.id),
  ['projectbluefin/bluefin#1', 'projectbluefin/bluefin#2'],
);
```

- [ ] **Step 2: Run the new tests and confirm they fail**

Run:

```bash
node --test queue/test/queue.test.mjs
```

Expected: FAIL because `queue/lib/queue.mjs` does not exist.

- [ ] **Step 3: Implement the pure queue model**

In `queue/lib/queue.mjs`, export `buildQueue`. Reject malformed required
fields rather than silently guessing. Classify `fix-ci` before
`resolve-conflicts`, then `review`, `investigate`, and
`ready-for-human-merge`. Group by repository size descending, then action
order, ascending `updated_at`, and stable `repository#number`. Include the
source evidence and a human-readable reason for every ranking decision.

Serialize JSON with:

```js
JSON.stringify({ generated_at: generatedAt, items }, null, 2) + '\n'
```

Render Markdown from the same `items` array, with repository headings, direct
PR links, the recommended action, and ranking reasons. Do not implement an
independent Markdown ranking path.

- [ ] **Step 4: Add output-contract tests**

Assert a fixed `generatedAt` appears identically in Markdown and JSON, JSON
has only the documented fields, Markdown groups items by repository, labels
remain informational, and two runs with the same fixture produce byte-identical
outputs.

- [ ] **Step 5: Run the offline model tests**

Run:

```bash
node --test queue/test/queue.test.mjs
```

Expected: PASS.

- [ ] **Step 6: Commit the model and tests**

```bash
git add queue/lib/queue.mjs queue/test/queue.test.mjs
git commit -m "feat: add deterministic PR queue model"
```

### Task 2: Fetch GitHub evidence and generate snapshots safely

**Files:**
- Create: `queue/lib/github.mjs`
- Create: `queue/generate.mjs`
- Modify: `queue/test/queue.test.mjs`

**Interfaces:**
- Consumes: `QUEUE_OWNER`, `QUEUE_REPOSITORIES`, and optional `GH_TOKEN`
  environment variables.
- Produces: `fetchOpenPullRequests({ fetch, owner, repositories, token })`,
  which returns validated PR evidence, and
  `generateSnapshots({ fetch, owner, repositories, token, outputDirectory })`,
  which writes both queue artifacts only after all source reads and rendering
  succeed. `node queue/generate.mjs` supplies the live environment values to
  that exported function.

- [ ] **Step 1: Write GitHub pagination and failure tests**

Pass a fake `fetch` to `fetchOpenPullRequests` that returns two pages for one
repository. Assert both pages are present. Add a test where one response is
non-200 and a test where its JSON is not an array:

```js
await assert.rejects(
  () => fetchOpenPullRequests({ fetch: failingFetch, owner: 'projectbluefin', repositories: ['bluefin'] }),
  /GitHub request failed/,
);
```

Also test that a failed generation leaves pre-existing `public/queue.md` and
`public/queue.json` byte-identical.

- [ ] **Step 2: Run the focused tests and confirm they fail**

Run:

```bash
node --test queue/test/queue.test.mjs
```

Expected: FAIL because `queue/lib/github.mjs` and `queue/generate.mjs` do not
exist.

- [ ] **Step 3: Implement the GitHub client**

In `queue/lib/github.mjs`, build the URL
`https://api.github.com/repos/<owner>/<repository>/pulls?state=open&per_page=100&page=<n>`
with `URL` and `URLSearchParams`. Request all pages until a page has fewer
than 100 items. Send `Accept: application/vnd.github+json` and send
`Authorization: Bearer <token>` only when a token is supplied. Reject
non-success status codes and malformed JSON with repository-specific errors.

Fetch the additional review/check evidence needed by Task 1 through explicit
GitHub REST requests. If individual review/check evidence cannot be obtained
for an otherwise valid PR, preserve the PR and mark its derived action
`investigate`; do not fail the full snapshot.

- [ ] **Step 4: Implement atomic generation**

Export `generateSnapshots` from `queue/generate.mjs`. It must accept an
injected `fetch` and `outputDirectory` for tests, while the CLI entry point
requires `QUEUE_OWNER=projectbluefin` and parses `QUEUE_REPOSITORIES` as a
non-empty comma-separated list of simple repository names. Fetch all
configured repositories, call `buildQueue`, then write Markdown and JSON to
temporary files in `outputDirectory` and rename both only after both
renderings are ready. Do not delete or truncate an existing artifact before
every source request succeeds.

Use a temporary directory and fixture-backed `fetch` in tests. The first real
snapshot is created by the manually dispatched Task 3 workflow, so test
fixtures never become public queue content.

- [ ] **Step 5: Run all queue tests**

Run:

```bash
node --test queue/test/queue.test.mjs
```

Expected: PASS.

- [ ] **Step 6: Commit the generator**

```bash
git add queue/lib/github.mjs queue/generate.mjs queue/test/queue.test.mjs
git commit -m "feat: add PR queue generator"
```

### Task 3: Automate refresh, validate offline, and document operations

**Files:**
- Create: `.github/workflows/update-pr-queue.yml`
- Modify: `.github/workflows/validate.yml`
- Modify: `README.md`

**Interfaces:**
- Consumes: the Task 2 generator and the repository-local
  `QUEUE_REPOSITORIES` workflow environment value.
- Produces: a public snapshot refresh every 15 minutes, manually on demand,
  and after the central Renovate workflow succeeds; validation that never calls
  GitHub's live API.

- [ ] **Step 1: Write a workflow contract test**

Extend `queue/test/queue.test.mjs` to read
`.github/workflows/update-pr-queue.yml` as text and assert it contains:

```js
assert.match(workflow, /cron: '\*\/15 \* \* \* \*'/);
assert.match(workflow, /contents: write/);
assert.match(workflow, /node --test queue\/test\/queue\.test\.mjs/);
assert.match(workflow, /QUEUE_OWNER: projectbluefin/);
```

Also assert the workflow checks out `main`, never a pull request head ref, and
uses `git diff --quiet -- public/queue.md public/queue.json` before committing.

- [ ] **Step 2: Run the test and confirm it fails**

Run:

```bash
node --test queue/test/queue.test.mjs
```

Expected: FAIL because `.github/workflows/update-pr-queue.yml` does not exist.

- [ ] **Step 3: Add the refresh workflow**

Create `.github/workflows/update-pr-queue.yml` with `schedule` cron
`*/15 * * * *`, `workflow_dispatch`, and
`repository_dispatch` type `renovate-completed`. Do not use a local
`pull_request` trigger: it cannot observe Renovate pull requests opened in
other repositories.

Set top-level `permissions` to `contents: write`, check out `main` explicitly,
set `QUEUE_OWNER: projectbluefin`, list the approved public repositories in
`QUEUE_REPOSITORIES`, run the offline test suite, run the generator with
`${{ github.token }}`, and commit only changed `public/queue.md` and
`public/queue.json` as `ci: refresh PR queue`. Do not add a secret, use
`pull_request_target`, or run code from a pull-request head.

In `projectbluefin/renovate-config`, add a post-Renovate job that runs only
when the scheduled or manually dispatched Renovate job succeeds. It creates
the existing Renovate GitHub App token and sends
`repository_dispatch` event `renovate-completed` to
`projectbluefin/review`. The payload is informational only; review always
checks out `main`.

Before enabling the workflow, a maintainer must confirm that the protected
branch permits the repository `GITHUB_TOKEN` to commit these generated files.
If it does not, stop rather than adding a personal token or GitHub App.

- [ ] **Step 4: Add offline validation and user documentation**

Add a `node --test queue/test/queue.test.mjs` step to `validate.yml`. Update
the README with the three static endpoints, the `generated_at` freshness rule,
the five recommended actions, the instruction to verify each selected PR on
GitHub, and the explicit fact that the custom-domain/DNS mapping is an
operations task rather than part of the repository automation.

- [ ] **Step 5: Run repository checks**

Run:

```bash
node --test queue/test/queue.test.mjs
git diff --check
just --list
pre-commit run --all-files
```

Expected: all commands pass. Confirm the validation workflow does not make
live GitHub API requests.

- [ ] **Step 6: Commit automation and documentation**

```bash
git add .github/workflows/update-pr-queue.yml .github/workflows/validate.yml README.md queue/test/queue.test.mjs public/queue.md public/queue.json
git commit -m "ci: refresh public PR queue"
```

## Final Acceptance Check

- [ ] Run `node --test queue/test/queue.test.mjs`.
- [ ] Run `git diff --check` and `pre-commit run --all-files`.
- [ ] Dispatch the refresh workflow from `main` and confirm it either writes
  both updated artifacts together or leaves both prior artifacts untouched on
  a source error.
- [ ] Confirm `/`, `/queue.md`, and `/queue.json` through the selected static
  host after a maintainer completes the custom-domain and DNS mapping.
- [ ] Verify a queue-selected pull request directly in GitHub before taking
  any review action.
