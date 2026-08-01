---
name: pr-workflow
version: "1.0"
last_updated: 2026-07-31
id: pr-workflow
one_line_purpose: Open donate-clanker pull requests that merge cleanly.
entry_point: docs/skills/pr-workflow.md
category: meta
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [git, pullrequest, conventional, hooks, branches]
description: "Defines Conventional Commit PR titles under squash-only merging, the clanker/<task_id> branch convention, the Hive-Task-Id trailer, and why git hooks are ergonomics not enforcement. Use before branching, committing, or opening a PR."
metadata:
  type: policy
---

# PR Workflow

## When to Use

Load this before creating a branch, writing a commit message, or opening a
pull request in this repository — especially for work assigned by Hive.

## Core Process

### Branch naming

Hive-assigned work uses `clanker/<task_id>`, where `<task_id>` is the task
identifier Hive supplied with the assignment. Human-initiated work may use
any descriptive branch name.

```bash
git switch -c "clanker/${TASK_ID}"
```

### Commit trailers

Commits produced from a Hive assignment carry the task identifier:

```
Hive-Task-Id: <task_id>
```

The trailer is what links a merged squash commit back to the assignment after
the branch is deleted. Add it to the pull request body as well, since the
squash commit body is editable by whoever merges.

### Conventional Commits in the PR title

Merges are **squash-only**, and the squash commit inherits the pull request
title. The title is therefore the permanent commit message. It must follow
Conventional Commits:

```
feat(launcher): add container-only mode
fix(image): pin hive-contributor by digest
docs(skills): add goose-context skill
chore(deps): bump pinned hive commit
```

Individual commits on the branch do not need to conform; they disappear on
squash. Do not spend effort curating them.

### One logical change per pull request

Split unrelated changes. A pull request that both bumps a pinned digest and
refactors a recipe cannot be reverted cleanly and cannot be reviewed
honestly.

Link the issue with `Closes #NNN` when the work closes one. When behavior
changes, update the matching `docs/skills/*.md` file in the same pull
request.

### Hooks are ergonomics, not enforcement

Git hooks ship at `/opt/bluefin/git-hooks` and are wired through a global
`core.hooksPath`. They give fast local feedback. They are **not** a gate:
`git commit --no-verify` bypasses them completely, and a contributor without
the image never runs them at all.

Deterministic enforcement is **GitHub rulesets and required status checks**.
Only those block a merge. Never document a hook as a guarantee, and never
weaken a required check on the theory that a hook already covers it.

### Timing under Hive

The scoped GitHub token expires 55 minutes after assignment and is never
refreshed. Push the branch and open the pull request early — a draft is fine
— rather than holding everything until the end. Completion is self-reported
and applies a 168-hour cooldown per issue regardless of outcome, so report
only after the pull request exists and you have verified its URL.

## Red Flags

- A pull request title that is not a Conventional Commit. It becomes the
  permanent commit message.
- Two unrelated changes in one pull request.
- A Hive-assigned branch not named `clanker/<task_id>`, or a missing
  `Hive-Task-Id` trailer.
- `--no-verify` used to get past a real failure rather than a broken hook.
- Treating a green local hook run as merge readiness.
- Reporting completion before confirming the pull request URL.

## Verification

```bash
go test ./...
gofmt -l .
git diff --check
just --justfile just/61-donate-clanker.just --list
pre-commit run --all-files
```

Then confirm the pull request itself:

```bash
gh pr view --json title,url,headRefName,state
git log -1 --format='%B'      # Hive-Task-Id trailer present
gh pr checks
```

`gofmt -l .` and `git diff --check` must produce no output. The pull request
title must parse as a Conventional Commit before you ask for review.
