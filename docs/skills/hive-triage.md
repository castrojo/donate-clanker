---
name: hive-triage
version: "1.0"
last_updated: 2026-08-01
id: hive-triage
one_line_purpose: Diagnose why an attached contributor is never handed work.
entry_point: docs/skills/hive-triage.md
category: ci-ops
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [hive, triage, assignment, cooldown, websocket]
description: "Procedure for diagnosing a contributor that authenticates but never receives a task: the public hub endpoints to read, why the hub never pushes work, and the two wrong turns this diagnosis invites. Use when the container connects but no task arrives."
metadata:
  type: runbook
---

# Hive Triage

## When to Use

Load this when a contributor container starts, authenticates, and then sits
there — no `task_assign`, no error, no disconnect. Symptoms include "it says
it joined the hive but nothing happens" and a pane that reaches the agent's
idle prompt and stays there.

Not for a task that arrived and then went wrong: that is
[`hive-runtime.md`](hive-runtime.md). Not for a launch that never reaches the
hub at all — that is a credential or preflight failure, so run
`donate-clanker-doctor` first.

## Core Process

### 1. Confirm the client is actually asking

Read the relay's own log lines:

```bash
podman logs --tail 400 donate-clanker-container 2>&1 | tr '\r' '\n' \
  | grep -aiE 'connect|auth|ready|task|error' | grep -av 'starting [0-9]* extensions'
```

A healthy, waiting client shows `Connected to hub`, then
`Authenticated as <id> (tier: <tier>)`, then `CLI ready — accepting tasks`.

That sequence means the client did its part. `ready` is sent on `auth_ok`, so
it is on the wire before the agent CLI has even finished starting.

### 2. Read the hub's public endpoints

Two are unauthenticated and answer JSON; everything else under `/api/`
redirects to a login page or falls through to the landing HTML.

```bash
HUB=<hub-host>
curl -s "https://$HUB/api/contribute/status"    # actionable_items, hub, active_contributors
curl -s "https://$HUB/api/contribute/activity"  # last 50 joined/picked up/completed/failed/left
```

`actionable_items` is the hub's *scan* count, not the admissible pool. It is
routinely large while the admissible pool is empty, so it proves nothing on
its own.

### 3. Ask what was assigned, not whether

The activity feed is the diagnosis. Group the pickups:

```bash
curl -s "https://$HUB/api/contribute/activity" \
  | jq -r '[.activity[] | select(.action=="picked up") | .task]
           | map(sub(": .*"; "")) | group_by(.)[] | "\(length)\t\(.[0])"'
```

A healthy hub offers many distinct issues. A hub whose pickups collapse onto
one repeated issue is livelocked: a completion sets a 168-hour cooldown but a
failure sets none, and selection is unranked, so one unfinishable issue is
handed out over and over while everything else waits. Once that issue is
finally completed, the pool can go empty and every contributor starves.

### 4. Know that the hub never pushes

`selectTask` runs only in the `ready` handler. If it returns nothing, the
server sends **no** message — there is no "no work available" reply — and the
relay only sends `ready` again on a state transition (completion, failure,
reconnect), never on a timer.

Two consequences that decide what to do next:

- A client that connected while the pool was empty stays stranded even after
  work appears. **Reconnecting is the only retry.**
- Because idle clients never re-ask, whoever reconnects first after work
  becomes eligible gets it.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "`actionable_items` is 129, so there is work." | That is the scan count. Admissible is what the contribute filters, cooldowns and held issues leave — read the activity feed instead. |
| "Only my account gets nothing, so it is my credential or tier." | Check whether *anyone* was assigned anything in the same window before concluding that. |
| "I'll restart it a few times until it picks something up." | Restarting is a valid retry, but if the pool is empty it changes nothing. Diagnose first. |
| "The container is broken." | If the log reached `CLI ready — accepting tasks`, the client is fine and the cause is hub-side. |

## Red Flags

- Concluding "it is account-specific" from an activity window that predates a
  completion, a cooldown, or any other state change. Compare like with like.
- Diagnosing from `actionable_items` alone.
- Reading the relay's `CLI ready — accepting tasks` as proof that a task was
  offered. It only proves one was requested.
- Adding a retry, poll, or filter to this repository to work around it. The
  hub owns selection; a client-side workaround shadows it and
  `tests/just-onboarding.sh` fails the build for it.
- Reporting a completion to shake something loose. That burns 168 hours on an
  issue for nothing.

## Verification

You have a diagnosis, not a guess, when you can name which of these holds:

- [ ] The client never reached `CLI ready — accepting tasks` → client-side;
      go to `donate-clanker-doctor`.
- [ ] Pickups in the window collapse onto one repeated issue → hub livelock.
- [ ] No contributor was assigned anything in the window → admissible pool is
      empty, for everyone.
- [ ] Others were assigned work in the *same* window and you were not → then,
      and only then, look at anything account-specific.

```bash
bash tests/just-onboarding.sh   # confirms the launcher still filters nothing
```
