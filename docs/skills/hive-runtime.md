---
name: hive-runtime
version: "1.9"
last_updated: 2026-08-04
id: hive-runtime
one_line_purpose: Operate inside Hive's tmux, token, and cooldown constraints.
entry_point: docs/skills/hive-runtime.md
category: ci-ops
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [hive, tmux, runtime, tokens, cooldown, workspace]
description: "Explains Hive's tmux session, workspace directory, prompt and output contract, contributor credentials, token lifetime, cooldown, and exclusive task selection. Use when operating or debugging a session."
metadata:
  type: reference
---

# Hive Runtime

## When to Use

Load this before changing code near a Hive session boundary or when an
assigned contributor session behaves unexpectedly.

## Core Process

1. Let Hive own the WebSocket protocol, assignment selection, `contributor`
   tmux session, prompt injection, result capture, and external documentation
   lookup. Context7 is Hive's: the hub queries it server-side
   (`v2/pkg/knowledge/context7.go`) and delivers the result through its
   knowledge export, so the image must never configure a second path to it.
   review starts the runtime and attaches to it; it does not reproduce any of
   those jobs.
2. Attach only to inspect or deliberately steer a live session:

   ```bash
   podman exec -it <container> tmux attach -t contributor
   ```

   Detaching tmux changes the display, not the foreground run. Ctrl-C or
   closing the original terminal ends the contributor.
3. Expect the agent to start in Hive's prepared workspace. `contributor-agent.sh`
   exports `HIVE_WORKSPACE_DIR` (default `$HOME/workspace`), creates it, and
   starts the tmux session rooted there with `tmux new-session -c`. Clone
   assigned work into that directory; no `cd` step is required, and the
   launcher must not create or mount a workspace of its own.
4. Put the final result in the final 15 pane lines. Hive captures only those
   lines for its report.
5. Plan around the scoped assignment token's 55-minute lifetime. The hub
   proactively re-mints and pushes a fresh token to an active task after 50
   minutes, so a long task survives expiry only while its socket stays up.
   Report completion only after its verifiable artifact exists: a completion
   carrying a PR link applies the 168-hour issue cooldown, a completion with
   no PR link only 4 hours, and a failure or disconnect books the short
   10-minute failure cooldown — 6 hours once an issue is quarantined.
6. Do not filter, decline, rank, or retry assignments in this repository.
   Hive selection is the sole authority. The relay's own negative-ack handling
   is Hive's, not ours: when the hub declines to assign work it sends
   `task_unavailable` with a reason (`no_work`, `token_mint_failed`,
   `tier_disabled`, `concurrency_limit`), which the relay logs before re-asking
   after a fixed delay. A relay revision predating that case logs the message
   as an unknown type and then has no path back to asking, because every
   `ready` it sends is event-driven and none is timed; it wedges idle. No task
   was assigned in that state, so nothing is held. Move the pin rather than
   adding a downstream retry.

### Periodic Goose restart loop

If the relay repeatedly logs `CLI ready — accepting tasks`, `CLI restarted:
goose`, and `Restarting goose CLI for memory cleanup (task 3)`, leave the
running contributor alone and report it upstream. In the pinned relay,
`tmuxSendKeys()` starts the periodic restart while `tasksCompletedCount` is
three; when `relaunchCLI()` becomes ready, `flushPendingTask()` re-enters
`tmuxSendKeys()` before that count changes, so it starts the same restart
again instead of delivering the pending assignment. This is upstream relay
behavior, not a launcher, image, or tmux lifecycle concern. Include the relay
SHA and the alternating log lines in an upstream report; do not add a client
retry, a session recreation, or a local relay patch.

### GitHub identity

Container-only mode can pass one contributor GitHub token as inherited
`GH_TOKEN`; it does not mount the host GitHub configuration. VM mode can pass
the Copilot provider secret but the current guest cannot map a host GitHub
identity to `GH_TOKEN`. Use `review-container` for work requiring
fork, push, or pull-request access until the guest supports that mapping.
Never log or persist either credential.

To inspect earlier review output, enter tmux copy-mode with `Ctrl-b [`.
PageUp or the mouse wheel scrolls, tmux search finds text, and `q` returns to
the live pane. Copy-mode changes only your view; Hive still owns output
capture.

When configuring a derived contributor image, preserve the attach client's
recognized `TERM`; tmux's pane terminal is configured separately. Enable tmux
mouse support so the wheel enters copy-mode for long output. Do not alter
Hive's session creation to accomplish either behavior.

## Red Flags

- Creating or naming tmux sessions, injecting prompts, or scraping pane output
  in the launcher or image.
- Adding assignment selectors or client-side retry loops.
- Treating a tmux detach as background execution.
- Preparing, mounting, or renaming a contributor workspace in the launcher or
  image instead of using Hive's `HIVE_WORKSPACE_DIR`.
- Adding a local retry, poll, or timeout to compensate for a relay revision
  that ignores `task_unavailable`.
- Reporting completion before the required artifact is independently visible.
- Assuming an abruptly killed contributor strands its assigned task. The hub
  releases `currentTask` in its disconnect handler and books a cooldown, and
  its heartbeat loop closes a half-open socket, so no downstream release,
  timeout, or slot-reclaim step belongs here.
- Mounting `~/.config/gh` or printing a token to provide agent identity.

## Verification

```bash
podman exec -it <container> tmux ls
podman exec -it <container> tmux attach -t contributor
bash tests/just-onboarding.sh
```

Confirm that `contributor` exists, the final pane lines contain the result,
and no launcher change duplicates Hive lifecycle behavior.

## Sources

Cite upstream by pinned permalink, never a branch path.

- Relay message cases, including `task_unavailable`:
  [`bin/contributor-relay.sh` @ 4d61ad7c](https://github.com/kubestellar/hive/blob/4d61ad7ce8b646a4e380865c521d5b12677240c9/bin/contributor-relay.sh)
- Workspace preparation and tmux rooting:
  [`bin/contributor-agent.sh` @ 4d61ad7c](https://github.com/kubestellar/hive/blob/4d61ad7ce8b646a4e380865c521d5b12677240c9/bin/contributor-agent.sh)
- Task release on disconnect:
  [`v2/pkg/dashboard/contribute_ws.go#L1057-L1090` @ 4d61ad7c](https://github.com/kubestellar/hive/blob/4d61ad7ce8b646a4e380865c521d5b12677240c9/v2/pkg/dashboard/contribute_ws.go#L1057-L1090)
- tmux terminal and mouse configuration: Context7 `/tmux/tmux`
