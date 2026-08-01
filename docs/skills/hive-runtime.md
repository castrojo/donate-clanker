---
name: hive-runtime
version: "1.0"
last_updated: 2026-07-31
id: hive-runtime
one_line_purpose: Operate inside Hive's tmux, token, and cooldown constraints.
entry_point: docs/skills/hive-runtime.md
category: ci-ops
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [hive, tmux, runtime, tokens, cooldown]
description: "Describes Hive's contributor runtime: the tmux session contract, prompt injection, the 15-line output budget, the 55-minute token, and the 168-hour cooldown. Use when debugging an attached session or planning task-length work."
metadata:
  type: reference
---

# Hive Runtime

## When to Use

Load this when attaching to a running contributor, when a task appears to
hang or produce no result, when reasoning about how long a task may run, or
before writing any code that touches session, prompt, or output handling.

## Core Process

### The tmux session contract

Hive's entrypoint creates a tmux session named `contributor` and launches the
agent CLI inside it by keystroke injection. donate-clanker does not create,
name, or manage this session. It only attaches.

Attach with Hive's own documented flow:

```bash
podman exec -it <container> tmux attach -t contributor
# docker exec -it <container> tmux attach -t contributor
```

Detach with the tmux prefix followed by `d`. Detaching leaves the agent
running; the task continues.

### How prompts arrive

Task prompts are injected into the pane the same way the CLI itself is
launched: as keystrokes. There is no API surface, no file drop, and no queue
you can inspect. What you see in the pane is the whole truth of what the
agent was told.

Consequence: if you type into the pane while a prompt is being injected, you
corrupt it. Attach to observe and steer deliberately, not reflexively.

### The 15-line output budget

Hive captures the **last 15 lines** of the pane to report results back. That
is the entire reporting channel.

Practical rules:

- Put the conclusion last. Anything scrolled past line 15 from the bottom is
  invisible to Hive.
- Avoid trailing verbose command output before a report; it evicts the
  summary.
- Do not design flows that assume a full transcript reaches the server.

### The 55-minute token

The scoped GitHub token Hive issues expires 55 minutes after assignment and
is **never refreshed**. Plan every task to push or open its pull request well
inside that window. A task that spends 50 minutes exploring and then tries to
push will fail at the last step with an authentication error that looks like
a permissions bug and is not.

### Self-reported completion and the 168-hour cooldown

Completion is self-reported by the agent. Once reported, the issue enters a
**168-hour cooldown**, whether the task passed or failed. There is no retry
loop, and a premature or mistaken completion report costs a week on that
issue.

Report completion only after the verifiable artifact exists — a pushed
branch, an opened pull request, a green check — not after the intent to
create one.

## Red Flags

- Code in this repository that creates a tmux session, injects keystrokes, or
  scrapes pane output. That is Hive's job; deleting the duplicate is the fix.
- Renaming or parameterizing the `contributor` session name.
- A summary longer than 15 lines, or a report followed by more output.
- Assuming the GitHub token can be refreshed, rotated, or re-requested.
- Reporting completion to unblock yourself from a stuck state. That burns 168
  hours.
- Treating a silent pane as a crash without attaching to look first.

## Verification

```bash
podman exec -it <container> tmux ls
podman exec -it <container> tmux attach -t contributor
```

`tmux ls` must show `contributor`. Before reporting completion, confirm the
artifact independently:

```bash
gh pr view --json url,state
git log --oneline -1
```

Confirm the final 15 lines of the pane contain the conclusion by scrolling to
the bottom of the pane after the run.
