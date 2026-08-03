# Agent PR Queue Design

## Goal

Publish a small, public, machine-readable view of Project Bluefin's open pull
requests at `queue.projectbluefin.io`. It helps people and agents choose the
next review task without creating a second work queue, assignment mechanism,
or source of truth.

GitHub remains authoritative for pull requests, checks, reviews, and merge
state. Hive remains authoritative for agent coordination and task selection.

## Scope

The first version covers public open pull requests in `projectbluefin` only.
It publishes a human-readable Markdown view and an equivalent JSON document.
It is read-only: every action still happens through GitHub and the normal Hive
workflow.

The queue does not include issues, private repositories, claims, leases,
assignments, labels mutations, webhooks, a database, cache state, polling
clients, or a new dashboard. The existing MCP control panel remains the
on-demand, authenticated evidence view for a review contributor.

## Architecture

A scheduled GitHub Actions workflow in this repository fetches the configured
public repositories' open pull requests from GitHub. It normalizes and ranks
the evidence, then writes two tracked generated artifacts:

- `public/queue.md` is the human overview, grouped by repository.
- `public/queue.json` is the agent contract.

The workflow runs every 15 minutes, on `workflow_dispatch`, and when the
central `projectbluefin/renovate-config` workflow dispatches
`renovate-completed` after a successful scheduled or manual Renovate run. It
makes no commit when the generated files are unchanged. A static-hosting
configuration serves the files at:

- `/` for the Markdown overview;
- `/queue.md` for the Markdown artifact;
- `/queue.json` for the JSON artifact.

The custom-domain mapping for `queue.projectbluefin.io` is deployment
configuration outside this change. It must be approved and performed through
the established hosting and DNS process.

## Queue Contract

The JSON document contains a generation timestamp and a deterministic ordered
`items` array. Each item includes:

```json
{
  "id": "projectbluefin/bluefin#123",
  "repository": "projectbluefin/bluefin",
  "number": 123,
  "url": "https://github.com/projectbluefin/bluefin/pull/123",
  "title": "fix: handle upgrades",
  "updated_at": "2026-08-03T16:00:00Z",
  "labels": ["quality"],
  "review_state": "review_required",
  "mergeable_state": "clean",
  "check_state": "success",
  "recommended_action": "review",
  "ranking_reasons": ["repository has 4 open pull requests", "review is required"]
}
```

`id` is stable while the pull request exists. The action is one of:

- `fix-ci` when a check has failed;
- `resolve-conflicts` when GitHub reports the pull request as dirty;
- `review` when a clean pull request needs review;
- `investigate` when GitHub has not computed sufficient evidence or evidence
  is otherwise incomplete;
- `ready-for-human-merge` when the pull request is clean, checks pass, and
  GitHub reports approval.

The queue never emits `merge` as an instruction. A queue result cannot bypass
the repository's human review and merge gates.

## Ranking

Items are grouped by repository, ordered by the repository's open-pull-request
count descending. That lets a reviewer stay in one repository while reducing
the largest backlog.

Within a repository, action categories sort as `fix-ci`,
`resolve-conflicts`, `review`, `investigate`, and
`ready-for-human-merge`. Items within a category sort by oldest `updated_at`
first, then by `repository#number`. The generator records human-readable
reasons for every placement rather than inventing a numeric priority score.

Labels are retained as source context but do not rank items. The factory's
workflow labels describe lifecycle state, not a public priority taxonomy.

## Failure Handling and Freshness

The generator treats a failed GitHub request or malformed API response as an
error. Incomplete evidence for one otherwise valid pull request instead emits
`investigate` for that item. A source failure does not overwrite a previous
snapshot with an empty or success-shaped file: the workflow fails visibly and
leaves the last known generated artifacts available, including their
`generated_at` timestamp.

Consumers must check `generated_at` and verify a selected item directly in
GitHub before acting. The static queue is a recommendation snapshot, never an
authority.

## Validation

Tests use saved GitHub API fixtures and cover:

- normalization of pull-request evidence;
- recommended-action classification;
- deterministic grouping and ordering;
- JSON contract shape;
- Markdown and JSON agreement;
- no-change generation;
- failed-source preservation of the prior snapshot.

CI validates fixtures and generated output without making live GitHub API
requests. The scheduled generator is the only component that reads live public
GitHub data.

## Deferred Work

Do not add `/org`, `/repo`, `/batch`, `/next`, content negotiation,
`/.well-known/agent-queue`, webhooks, claims, leases, KV, Durable Objects, or
private-repository support until a consumer proves the need. The single JSON
document is intentionally small enough for clients to filter locally.
