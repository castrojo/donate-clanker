# Bluefin Agentic Factory Feedback Loop

This document is the canonical local model for `review`. It adapts
[`projectbluefin/common`'s Agentic Operating Model][common-model] to this
repository's thin foreground launcher and read-only MCP app. Read it after
`AGENTS.md` and before task-specific skills.

The model is documentation: the launcher, image, MCP app, tests, skills, and
user-facing instructions must describe the same roles and authority
boundaries. When source evidence changes the model, update this document and
the affected local contract together. Do not preserve superseded plans,
session logs, or design scratchpads as competing explanations.

## Roles and authority

| Term | Meaning | Authority |
|---|---|---|
| **Bluefin Agentic Factory Feedback Loop** | The lifecycle that turns agent work and test feedback into reviewed Bluefin changes. | The model for this repository. |
| **Toil** | Repetitive, low-novelty maintenance work an under-maintained project needs: broken CI, stale pins, drifted documentation, unreproduced reports, untriaged issues, stalled branches. | Toil is the work this factory exists to absorb. |
| **Factory Worker** | A contributor using the worker configuration to receive and complete Hive-assigned work. | Hive assigns work; the worker implements only its assigned scope. |
| **Maintainer Reviewer** | A maintainer assessing an incoming pull request. | The human decides review, approval, and merge. |
| **Review Evidence** | Read-only pull-request, issue, verification, and merge-state context shown before a review. | Evidence informs a human; it never makes a decision. |
| **Managed Reviewer Client** | A foreground, preconfigured Goose session that a Maintainer Reviewer may choose after examining Review Evidence. | It prepares a Review Draft; it cannot submit, approve, or merge. |
| **Portable Reviewer Prompt** | Markdown Review Evidence and queue instructions for a maintainer's own client. | It is context, not an assignment. |
| **Bluefin PR Queue** | A generated read-only snapshot of open factory pull requests and suggested next actions. | GitHub is authoritative; the queue does not assign work or merge. |
| **Review Draft** | Analysis, review text, or commands prepared for a Maintainer Reviewer. | A human explicitly considers and submits it. |

Avoid calling Factory Workers "reviewers" or "maintainers"; avoid calling
Review Evidence a decision or approval; and avoid calling the PR Queue an
assignment queue or merge authority.

## Scope of work

This is a toil-reduction factory for under-maintained open-source projects,
not a feature factory. Factory Workers repair what is already broken and
finish what a project already decided to do; they do not add features,
dependencies, configuration surfaces, or architecture.

Well-staffed projects restrict large agent-authored pull requests because
those consume more maintainer attention than they return. That reasoning is
the model here too: the reviewer's attention is the scarce resource, so a
change is sized to be reviewable rather than to be complete in one pass. When
an assigned task can only be finished by out-of-scope work, the deliverable is
an evidenced written finding. That is completed work, not a declined
assignment; Hive's authority over what gets worked on is unchanged.

[`docs/skills/contribution-culture.md`](../skills/contribution-culture.md)
carries the operational form of this section.

## Repository boundary

`review` owns only foreground VM boot, credential handoff, and review context.
Hive owns the contributor WebSocket protocol, task selection, assignment prompt
injection, the `contributor` tmux session, and output capture. The launcher
must not filter, prioritize, decline, retry, or otherwise manage assignments.

`mcp-app/` presents read-only Review Evidence over foreground stdio MCP. It
does not select work, control Hive, read tmux, expose credentials, persist
state, poll, or mutate GitHub or launcher state.

The human Maintainer Reviewer is the decision point. A Factory Worker,
Managed Reviewer Client, Portable Reviewer Prompt, Review Evidence view, or
Bluefin PR Queue must never claim approval, merge, queue-management, or
task-selection authority.

## Documentation discipline

Keep the model executable and compact:

1. Treat local code and tests as evidence for implementation behavior.
2. Treat `AGENTS.md`, this document, and the matching skill as the
   agent-facing contract.
3. Record durable operational knowledge in `docs/skills/` and mirror skill
   frontmatter in `docs/skills/index.json`.
4. Delete stale changelogs, session notes, plans, design scratchpads, and
   append-only status documents. They are historical noise, not the model.
5. Use the pinned `projectbluefin/common` catalog as a shared sidecar after
   local documentation; it complements but does not override local authority.

## Verification

```bash
bash scripts/check-skill-frontmatter.sh
bash tests/generate-skills.sh
bash tests/image-contract.sh
bash tests/just-onboarding.sh
git diff --check
just --list
pre-commit run --all-files
```

[common-model]: https://github.com/projectbluefin/common/blob/main/docs/factory/agentic-model.md
