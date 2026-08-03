---
name: pi-agent
version: "1.0"
last_updated: 2026-08-03
id: pi-agent
one_line_purpose: Build and run the pi + DeepSeek Hive contributor container.
entry_point: docs/skills/pi-agent.md
category: ci-ops
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [pi, deepseek, container, agent, quadlet, hive]
description: "Build and run the pi + DeepSeek Hive contributor container. Use when setting up pi as an alternative agent backend, building the pi image, or debugging pi container or Hive contributor issues."
metadata:
  type: procedure
---

# pi Agent

## When to Use

Load this before working on the pi container image, entrypoint, quadlet files,
or when debugging pi container issues.

## Core Process

1. Build the pi image from `pi/image/Containerfile`. It derives from the same
   FSDK runner base as the Goose contributor image and installs Node.js, tmux,
   and pi via npm. No Goose, no Hive runtime — just the coding agent.

2. pi reads `DEEPSEEK_API_KEY` from the environment automatically. Pass it at
   container launch by env-var name (not value) per review convention:
   ```bash
   export DEEPSEEK_API_KEY=sk-...
   just pi-container
   ```

3. pi runs in a foreground tmux session named `pi`. Attach, detach (`Ctrl-b d`),
   and stop with Ctrl-C — same foreground guarantee as the Goose recipes.

4. The pi image is local-only until a publish workflow is added. Build it from
   the Containerfile or set `REVIEW_PI_IMAGE` to a registry reference.

## pi versus Goose

| Property | Goose | pi |
|---|---|---|
| Default provider | GitHub Copilot | DeepSeek |
| Credential env var | `GITHUB_COPILOT_TOKEN` | `DEEPSEEK_API_KEY` |
| Permission model | `--no-confirm` auto-approve | Tool configuration via CLI flags |
| Hive contributor | Yes (contributor-agent.sh) | Via hanthor/hive v2 backends.conf |
| VM mode | Yes (QEMU VM + runner) | Container only (no VM image yet) |
| Quadlet | Not shipped | Ships `pi/quadlet/pi.container` |
| Extensibility | Skills only (Goose native) | Skills + extensions + prompt templates |

## Provider Configuration

pi supports 30+ providers. Override `PI_PROVIDER` at container launch:

```bash
# Anthropic
PI_PROVIDER=anthropic ANTHROPIC_API_KEY=sk-ant-... just pi-container

# OpenAI
PI_PROVIDER=openai OPENAI_API_KEY=sk-... just pi-container
```

The pi image entrypoint accepts `PI_PROVIDER`, `PI_MODEL`, `PI_THINKING`, and
`PI_EXTRA_ARGS` environment variables.

## Modes

- **interactive** (default): tmux session, attach with `podman exec`
- **headless**: one-shot with `PI_PROMPT`, exits when done
- **rpc**: JSON-RPC on stdin/stdout for process integration

Set `PI_MODE` to switch modes. See `pi/README.md` for details.

## Quadlet (systemd service)

pi ships a podman quadlet for always-on use. Install:

```bash
cp pi/quadlet/pi.container ~/.config/containers/systemd/
echo "DEEPSEEK_API_KEY=sk-..." > ~/.config/containers/pi.env
systemctl --user daemon-reload && systemctl --user start pi
podman exec -it pi tmux attach -t pi
```

The quadlet mounts `~/.local/share/pi/sessions/` for session persistence across
container restarts.

## Common Rationalizations

- "pi should use Goose's provider." pi is a different agent; it uses its own
  provider configuration. Don't mix credentials between agents.
- "pi should participate in Hive contributor protocol." hanthor/hive v2 already
  adds pi to backends.conf. The review container pulls from kubestellar/hive;
  update the HIVE_COMMIT pin to a fork that includes pi when ready.

## Verification

```bash
just review-doctor | grep -A6 "pi + DeepSeek"
podman build -f pi/image/Containerfile -t pi-deepseek:latest .
DEEPSEEK_API_KEY=sk-... just pi-container
```

## Sources

- pi documentation: https://pi.dev
- DeepSeek API: https://platform.deepseek.com/api_keys
- hanthor/hive (pi backend): https://github.com/hanthor/hive (v2 branch)
