---
name: donate-clanker-factory-onboarding
description: Use when onboarding donate-clanker to the Project Bluefin factory, auditing factory documentation, or checking repository agent entry points.
---

# Factory Onboarding

## When to Use

Use this when adding or auditing `AGENTS.md`, `docs/SKILL.md`, factory
workflow references, or agent-facing documentation in this repository.

## When NOT to Use

Do not use this for ordinary README edits, worker credential changes, or
FSDK implementation work that does not change factory onboarding.

## Core Process

1. Read local `AGENTS.md` and `docs/SKILL.md` first.
2. Read the shared contract in
   [`projectbluefin/common/docs/skills/factory-onboarding.md`](https://github.com/projectbluefin/common/blob/main/docs/skills/factory-onboarding.md).
3. Keep local documentation focused on donate-clanker's launcher, worker,
   onboarding, and validation commands; link to common for org-wide policy.
4. Update the nearest skill when a durable repository-specific pattern is
   discovered.
5. For VM release documentation, distinguish this repository's validation gate
   from external artifact publication and keep the contract in `vm/README.md`.
6. Run the smallest relevant checks and verify links and changed paths.

## Common Rationalizations

- “Copy the entire common policy tree here.”  
  Duplicated policy drifts; link to common and keep local authority local.
- “A README is enough for agents.”  
  Agents need an explicit `AGENTS.md` contract and `docs/SKILL.md` router.
- “Factory onboarding is only metadata.”  
  The entry points must name build checks, ownership boundaries, and the
  self-improvement rule.

## Red Flags

- Missing `AGENTS.md` or `docs/SKILL.md`
- Local docs contradicting common factory rules
- Release docs claim that this repository builds or publishes externally owned
  VM artifacts
- Skill docs containing session logs, status ledgers, or append instructions
- A changed implementation surface with no matching durable skill guidance

## Verification

- [ ] `AGENTS.md` points to the local router and common shared contract.
- [ ] `docs/SKILL.md` routes worker, onboarding, and skill-improvement tasks.
- [ ] Factory-wide policy is linked rather than duplicated.
- [ ] VM release docs distinguish artifact verification from publication.
- [ ] `git diff --check` passes.
- [ ] Repository validation commands pass for implementation changes.
