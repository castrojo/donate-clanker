---
name: hive-runtime
version: "1.5"
last_updated: 2026-08-02
id: hive-runtime
one_line_purpose: Operate inside Hive's tmux, token, and cooldown constraints.
entry_point: docs/skills/hive-runtime.md
category: ci-ops
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [hive, tmux, runtime, tokens, cooldown]
description: "Explains Hive's tmux session, prompt and output contract, contributor credentials, token lifetime, cooldown, and exclusive task selection. Use when operating or debugging a session."
metadata:
  type: reference
---

# Hive Runtime

## When to Use

Load this before changing code near a Hive session boundary or when an
assigned contributor session behaves unexpectedly.

## Core Process

1. Let Hive own the WebSocket protocol, assignment selection, `contributor`
   tmux session, prompt injection, and result capture. review starts
   the runtime and attaches to it; it does not reproduce any of those jobs.
2. Attach only to inspect or deliberately steer a live session:

   ```bash
   podman exec -it <container> tmux attach -t contributor
   ```

   Detaching tmux changes the display, not the foreground run. Ctrl-C or
   closing the original terminal ends the contributor.
3. Put the final result in the final 15 pane lines. Hive captures only those
   lines for its report.
4. Plan around the scoped assignment token's 55-minute lifetime. It is not
   refreshed. Report completion only after its verifiable artifact exists;
   completion applies the 168-hour issue cooldown, while failure does not.
5. Do not filter, decline, rank, or retry assignments in this repository.
   Hive selection is the sole authority.

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

## Red Flags

- Creating or naming tmux sessions, injecting prompts, or scraping pane output
  in the launcher or image.
- Adding assignment selectors or client-side retry loops.
- Treating a tmux detach as background execution.
- Reporting completion before the required artifact is independently visible.
- Mounting `~/.config/gh` or printing a token to provide agent identity.

## Verification

```bash
podman exec -it <container> tmux ls
podman exec -it <container> tmux attach -t contributor
bash tests/just-onboarding.sh
```

Confirm that `contributor` exists, the final pane lines contain the result,
and no launcher change duplicates Hive lifecycle behavior.
