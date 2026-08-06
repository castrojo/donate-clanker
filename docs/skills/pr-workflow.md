---
name: pr-workflow
version: "1.5"
last_updated: 2026-08-04
id: pr-workflow
one_line_purpose: Open review pull requests that merge cleanly.
entry_point: docs/skills/pr-workflow.md
category: meta
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [git, pullrequest, conventional, hooks, branches]
description: "Defines protected-branch pull request, branch, title, trailer, and validation requirements, and how to reconcile a long-lived branch with a squash-merged main. Use before branching, committing, opening a review pull request, or resolving merge conflicts."
metadata:
  type: policy
  context7-sources: [/pre-commit/pre-commit]
---

# PR Workflow

## When to Use

Load this before creating a branch, preparing a commit, or opening a pull
request in this repository.

## Core Process

1. Work on a branch. Hive-assigned work uses `clanker/<task_id>` and includes
   this commit trailer:

   ```text
   Hive-Task-Id: <task_id>
   ```

2. Open one logical change per pull request. Link its issue with `Closes #NNN`
   when appropriate. Size it for a tired maintainer: repair what is broken,
   leave unrelated fixes for their own change, and never bundle a refactor,
   feature, or new dependency with a fix. See
   [`contribution-culture.md`](contribution-culture.md).
3. Make the pull request title a Conventional Commit. Squash merging makes
   that title the permanent commit message.
4. Update the matching skill document when behavior changes. Do not treat
   documentation-only work as exempt from the protected-branch workflow.
5. Treat local hooks as feedback, not enforcement. GitHub rulesets and
   required checks determine whether a pull request can merge.
6. Push and open the pull request early for Hive work. The scoped token lasts
   55 minutes and is refreshed at 50 minutes only while the socket stays up,
   and a completion carrying a PR link starts a 168-hour cooldown; report
   completion only after the pull request or other required artifact is
   verifiable.
7. Never add or remove a task-admission label on an issue or pull request —
   including in this repository — to influence what work Hive assigns. Hive is
   the sole authority for task selection, and relabelling to attract or shed an
   assignment is task selection. See [`upstream-hive.md`](upstream-hive.md) for
   the full rule and for upstream triage boundaries.

## Reconciling A Long-Lived Branch

`main` squash-merges, which rewrites commit SHAs. A branch that predates
several merges therefore looks far more divergent than it is, and the usual
measurements all mislead:

- `git cherry main <branch>` reports content already on `main` as unmerged,
  because the squashed commit is a different object.
- The three-dot range `main...HEAD`, which is what the pull request renders,
  inflates the change by replaying work `main` already has.
- The two-dot range `main..HEAD` understates it by hiding what the merge
  will remove.

Read both ranges before judging the size of a reconciliation, and confirm
per file rather than trusting either total:

```bash
git diff --stat main...HEAD   # what the pull request shows
git diff --stat main..HEAD    # the true net difference
git log --oneline main --  <path>   # did this land on main already?
```

Merge `main` into the branch. Do not rebase: features that landed on `main`
after the merge base exist only on that side, and rebasing replays the stale
branch on top of them, reintroducing deletions of files the branch never had.

Resolve add/add conflicts hunk by hunk. `git checkout --ours` and
`--theirs` operate on the whole file and silently discard the hunks Git
already merged correctly from the other side. Keep the feature that landed on
`main` **and** the newer behavior from the branch, then confirm both survived
before committing. A merge can also duplicate an adjacent block that neither
side duplicated; re-read the resolved file rather than trusting the marker
count to reach zero.

When a pinned dependency conflicts, resolve by date rather than by side:

```bash
gh api repos/<owner>/<repo>/commits/<sha> --jq '.commit.committer.date'
```

## Red Flags

- Pushing directly to `main`.
- A non-Conventional pull request title.
- Combining unrelated changes in one pull request.
- Growing a diff because scoping it was harder than writing it.
- Adding a feature, dependency, or refactor to a repair.
- Omitting the Hive task trailer from assigned work.
- Using `--no-verify` to bypass a real failure.
- Reporting task completion before the required artifact exists.
- Adding or removing a task-admission label to influence a Hive assignment.
- Rebasing a long-lived branch onto `main` instead of merging `main` into it.
- Resolving a conflicted file with `--ours` or `--theirs` when both sides
  carry changes worth keeping.
- Declaring a reconciliation finished because no conflict markers remain,
  without re-running the suite that covers the resolved files.

## Verification

```bash
bash scripts/check-skill-frontmatter.sh
bash tests/generate-skills.sh
bash tests/image-contract.sh
bash tests/just-onboarding.sh
git diff --check
just --list
```

`pre-commit run --all-files` runs all socket-free contributor hygiene checks.
ShellCheck is a manual container-backed hook that the required `validate`
workflow invokes explicitly, so a missing local container socket does not
block the local gate.

After resolving a merge, confirm no marker survived anywhere. Anchor the
search, because shell here-strings legitimately contain `<<<`:

```bash
grep -rnE '^(<<<<<<< |=======$|>>>>>>> )' . && echo 'markers remain'
```

Before review, confirm the pull request title, branch, checks, and required
trailer with `gh pr view` and `gh pr checks`. A reconciliation is done when
`gh pr view --json mergeable,mergeStateStatus` reports `MERGEABLE` and
`CLEAN`.
