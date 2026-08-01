---
name: pr-workflow
version: "1.1"
last_updated: 2026-08-01
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

Not for what to build or which checks exist; load the skill for the area you
are changing and read `.github/workflows/validate.yml` for the checks.

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

### `main` is protected — there is no direct-push path

A ruleset on `refs/heads/main` rejects direct pushes outright:

```
! [remote rejected] main -> main (push declined due to repository rule violations)
remote: - Changes must be made through a pull request.
remote: - 2 of 2 required status checks are expected.
```

The two required checks are `validate` and `conventional-title`. This applies
to every change, including docs-only ones: an org convention that lets skill
updates go straight to `main` does not apply in this repository, because the
server refuses them. Branch, open a pull request, and let the checks run.

After `gh pr merge --squash`, the local post-merge fast-forward fails if the
local branch holds the pre-squash commit. That is not a failed merge --
confirm with `gh pr view --json state`, then `git fetch -p && git reset --hard
origin/main`.

### Timing under Hive

The scoped GitHub token expires 55 minutes after assignment and is never
refreshed. Push the branch and open the pull request early — a draft is fine
— rather than holding everything until the end. Completion is self-reported
and applies a 168-hour cooldown per issue regardless of outcome, so report
only after the pull request exists and you have verified its URL.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "It's docs-only, I'll push straight to main." | The ruleset rejects it. Every change branches, including docs. |
| "The local hooks passed, so it is ready to merge." | `--no-verify` bypasses every hook. Only required checks gate a merge. |
| "I'll tidy the branch commits before review." | They vanish on squash. Spend the effort on the PR title, which becomes permanent. |
| "I'll report the task done, the PR is basically up." | Completion burns a 168-hour cooldown whether or not it worked. Confirm the URL first. |

## Red Flags

- A pull request title that is not a Conventional Commit. It becomes the
  permanent commit message.
- Two unrelated changes in one pull request.
- A Hive-assigned branch not named `clanker/<task_id>`, or a missing
  `Hive-Task-Id` trailer.
- `--no-verify` used to get past a real failure rather than a broken hook.
- Treating a green local hook run as merge readiness.
- Reporting completion before confirming the pull request URL.
- Attempting `git push origin main`. The ruleset rejects it; branch instead.
- Reading a failed post-merge fast-forward as a failed merge.

## Verification

```bash
bash tests/image-contract.sh
bash tests/just-onboarding.sh
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

`git diff --check` must produce no output. The pull request
title must parse as a Conventional Commit before you ask for review.
