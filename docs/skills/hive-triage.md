---
name: hive-triage
version: "1.2"
last_updated: 2026-08-04
id: hive-triage
one_line_purpose: Diagnose why an attached contributor is never handed work.
entry_point: docs/skills/hive-triage.md
category: ci-ops
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [hive, triage, assignment, cooldown, websocket]
description: "Diagnoses a contributor that connects but receives no assignment without adding client-side selection or retry behavior. Use when a ready contributor remains idle."
metadata:
  type: runbook
---

# Hive Triage

## When to Use

Load this when a contributor has connected successfully but remains idle before
receiving an assignment. Use [`hive-runtime.md`](hive-runtime.md) when work
arrived and the session then failed.

## Core Process

1. Confirm the launcher reached a ready contributor session. Resolve local
   preflight, configuration, and connectivity failures before diagnosing the
   hub.
2. Read the relay log first. When the hub declines to assign work it sends a
   `task_unavailable` negative-ack and the relay prints the reason
   (`no_work`, `token_mint_failed`, `tier_disabled`, `concurrency_limit`)
   before re-asking on its own fixed delay. That reason is the classification;
   do not reconstruct it from the hub dashboard. Silence where a reason is
   expected means the pinned relay predates that case, which is a pin problem,
   not a hub problem.
3. Check the hub's current contributor status and activity through its
   supported operator interface. Compare assignments made during the same
   window; do not infer availability from a displayed backlog count alone.
4. Classify the condition: no admissible work for any contributor, work held
   by another live contributor, repeated failures returning the same work to
   selection, or a contributor-specific connectivity/authorization problem.
5. Reconnect only as the normal request retry after the relevant hub state
   changed. Do not add polling, selection logic, or an assignment retry loop
   to review.
6. Escalate the observed condition to the hub operator with the time window
   and classification. Hub configuration and selection behavior are fixed
   there, not in this launcher.

## Red Flags

- Treating a generic backlog count as proof that work is assignable.
- Diagnosing a silent idle contributor as a hub fault before confirming the
  pinned relay handles `task_unavailable`.
- Treating an assigned-but-idle session with no checkout on disk as a triage
  case. That was an upstream workspace gap, fixed by `HIVE_WORKSPACE_DIR`;
  check the pin instead.
- Diagnosing an account-specific failure without comparing the same time
  window for other contributors.
- Repeatedly restarting a healthy contributor instead of checking hub state.
- Adding a local filter, poller, or retry mechanism to compensate for hub
  selection.
- Reporting completion merely to alter assignment availability.

## Verification

- [ ] The contributor reached a ready session.
- [ ] The relay log was read for a `task_unavailable` reason before any hub
      comparison.
- [ ] The comparison covers the current hub state, not an earlier window.
- [ ] The outcome is classified before any reconnect.
- [ ] No change was made to launcher or image task-selection behavior.

```bash
bash tests/just-onboarding.sh
```
