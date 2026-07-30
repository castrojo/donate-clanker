---
name: donate-clanker-skill-improvement
description: Use when a session discovers a durable donate-clanker workaround, convention, failure mode, or agent workflow improvement.
---

# Skill Improvement

## When to Use

Use this before closing any non-trivial session when implementation or
investigation revealed knowledge that a future agent would otherwise need to
rediscover.

## When NOT to Use

Do not use this for ephemeral task status, resolved issue lists, session notes,
or plans that belong in the session workspace.

## Core Process

1. Identify the nearest existing skill for the changed area.
2. Add only the timeless procedure, constraint, or verification evidence.
3. Keep the skill focused on how to operate, not what happened in one session.
4. Keep `image/config/agent-contract.json` aligned whenever the durable rule changes the required repository entry points, rule clauses, or validation commands.
5. Update `docs/SKILL.md` when adding a new skill or routing entry.
6. Commit the skill update with the related implementation change.

## Common Rationalizations

- “The workaround is obvious.”  
  If it took investigation to find, record the reusable rule.
- “I will document it later.”  
  The next session needs the learning immediately.
- “Put it in a changelog.”  
  Changelogs become stale; skills are the agent-facing source of truth.
- “The skill doc changed, the contract manifest can catch up later.”
  Runtime enforcement reads the manifest, not your intent; drift there drops required entry points or stale validation commands.

## Red Flags

- Session ends with a repeated discovery undocumented
- Durable workflow guidance changed, but `image/config/agent-contract.json` still points at old entry points or checks
- Skill contains dates, issue status, or resolved-item tables
- New skill is not reachable from `docs/SKILL.md`
- Documentation claims a command or path that does not exist

## Verification

- [ ] The content is timeless and procedural.
- [ ] It has clear trigger and exclusion conditions.
- [ ] The nearest skill document changed in the same change as the durable implementation update.
- [ ] `image/config/agent-contract.json` still matches the required repository entry points, rule clauses, and validation commands.
- [ ] It includes red flags and checkable verification criteria.
- [ ] Links and commands refer to this repository or canonical factory docs.
