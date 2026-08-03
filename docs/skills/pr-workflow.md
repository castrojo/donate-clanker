---
name: pr-workflow
version: "1.3"
last_updated: 2026-08-02
id: pr-workflow
one_line_purpose: Open review pull requests that merge cleanly.
entry_point: docs/skills/pr-workflow.md
category: meta
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [git, pullrequest, conventional, hooks, branches]
description: "Defines protected-branch pull request, branch, title, trailer, and validation requirements. Use before branching, committing, or opening a review pull request."
metadata:
  type: policy
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
   when appropriate.
3. Make the pull request title a Conventional Commit. Squash merging makes
   that title the permanent commit message.
4. Update the matching skill document when behavior changes. Do not treat
   documentation-only work as exempt from the protected-branch workflow.
5. Treat local hooks as feedback, not enforcement. GitHub rulesets and
   required checks determine whether a pull request can merge.
6. Push and open the pull request early for Hive work. The scoped token
   expires after 55 minutes and task completion starts a 168-hour cooldown;
   report completion only after the pull request or other required artifact is
   verifiable.

## Red Flags

- Pushing directly to `main`.
- A non-Conventional pull request title.
- Combining unrelated changes in one pull request.
- Omitting the Hive task trailer from assigned work.
- Using `--no-verify` to bypass a real failure.
- Reporting task completion before the required artifact exists.

## Verification

```bash
bash tests/image-contract.sh
bash tests/just-onboarding.sh
bash scripts/check-skill-frontmatter.sh
git diff --check
just --list
pre-commit run --all-files
```

Before review, confirm the pull request title, branch, checks, and required
trailer with `gh pr view` and `gh pr checks`.
