---
name: hive-triage
version: "1.1"
last_updated: 2026-08-02
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
2. Check the hub's current contributor status and activity through its
   supported operator interface. Compare assignments made during the same
   window; do not infer availability from a displayed backlog count alone.
3. Classify the condition: no admissible work for any contributor, work held
   by another live contributor, repeated failures returning the same work to
   selection, or a contributor-specific connectivity/authorization problem.
4. Reconnect only as the normal request retry after the relevant hub state
   changed. Do not add polling, selection logic, or an assignment retry loop
   to review.
5. Escalate the observed condition to the hub operator with the time window
   and classification. Hub configuration and selection behavior are fixed
   there, not in this launcher.

## Red Flags

- Treating a generic backlog count as proof that work is assignable.
- Diagnosing an account-specific failure without comparing the same time
  window for other contributors.
- Repeatedly restarting a healthy contributor instead of checking hub state.
- Adding a local filter, poller, or retry mechanism to compensate for hub
  selection.
- Reporting completion merely to alter assignment availability.

## Verification

- [ ] The contributor reached a ready session.
- [ ] The comparison covers the current hub state, not an earlier window.
- [ ] The outcome is classified before any reconnect.
- [ ] No change was made to launcher or image task-selection behavior.

```bash
bash tests/just-onboarding.sh
```
