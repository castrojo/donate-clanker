# pi + DeepSeek container
#
# Adds pi, the extensible coding agent, to the review contributor ecosystem
# with DeepSeek as the AI provider. pi runs as a foreground container or a
# podman quadlet (systemd) so it stays alive across terminal sessions.
#
# pi is a coding agent harness that reads, writes, edits files and runs bash.
# It supports 30+ providers including DeepSeek, and can be extended with
# skills, prompt templates, and TypeScript extensions.
#
# See https://pi.dev for full documentation.

## Quick Start

Build the image:

```bash
podman build -f pi/image/Containerfile -t pi-deepseek:latest .
```

### Direct container run

```bash
DEEPSEEK_API_KEY=sk-your-key podman run -it --rm --name pi \
  -v "$(pwd):/home/dev/work:Z" \
  -e DEEPSEEK_API_KEY \
  pi-deepseek:latest
```

This attaches you directly to pi's interactive terminal inside the container.
Ctrl-C stops it.

### Podman quadlet (systemd service)

1. Copy the quadlet file:

```bash
mkdir -p ~/.config/containers/systemd
cp pi/quadlet/pi.container ~/.config/containers/systemd/
```

2. Create the env file with your DeepSeek API key:

```bash
mkdir -p ~/.config/containers
echo "DEEPSEEK_API_KEY=sk-your-key-here" > ~/.config/containers/pi.env
chmod 600 ~/.config/containers/pi.env
```

3. Reload systemd and start:

```bash
systemctl --user daemon-reload
systemctl --user start pi
```

4. Attach to the pi session:

```bash
podman exec -it pi tmux attach -t pi
```

5. Stop the service:

```bash
systemctl --user stop pi
```

## Configuration

| Variable              | Purpose                                                | Default         |
|-----------------------|--------------------------------------------------------|-----------------|
| `DEEPSEEK_API_KEY`    | DeepSeek API key (pi reads it automatically)           | (required)      |
| `PI_PROVIDER`         | AI provider for pi                                     | `deepseek`      |
| `PI_MODEL`            | Model name (e.g. `deepseek-chat`, `deepseek-reasoner`) | pi default      |
| `PI_THINKING`         | Thinking level: off, minimal, low, medium, high, max   | pi default      |
| `PI_MODE`             | `interactive` (tmux), `headless` (one-shot), `rpc`     | `interactive`   |
| `PI_PROMPT`           | Prompt for headless mode                               | (required)      |
| `PI_EXTRA_ARGS`       | Additional pi CLI arguments                            | (none)          |

## Modes

### Interactive (default)

pi runs in a foreground tmux session named `pi`. You attach with `podman exec`:

```bash
podman exec -it pi tmux attach -t pi
```

Detach with `Ctrl-b d`. The container keeps running; reattach any time.

### Headless

Set `PI_MODE=headless` and `PI_PROMPT="Your task here"`. pi processes the
prompt in `-p` (print) mode and exits. Useful for one-shot automation.

### RPC

Set `PI_MODE=rpc`. pi serves JSON-RPC on stdin/stdout. For process
integration; see pi's RPC documentation.

## Session persistence

The quadlet mounts `~/.local/share/pi/sessions/` into the container so pi
sessions survive container restarts. Resume with:

```bash
podman exec -it pi pi --resume
```

## Other providers

pi supports 30+ providers. Override `PI_PROVIDER` and set the appropriate env
var:

```bash
# Anthropic
Environment=PI_PROVIDER=anthropic
ANTHROPIC_API_KEY=sk-ant-...

# OpenAI
Environment=PI_PROVIDER=openai
OPENAI_API_KEY=sk-...

# Google Gemini
Environment=PI_PROVIDER=google
GEMINI_API_KEY=...
```

## Development

### Iterating on the image

```bash
podman build -f pi/image/Containerfile -t pi-deepseek:dev .
podman run -it --rm -e DEEPSEEK_API_KEY pi-deepseek:dev
```

### Inspecting the image

```bash
podman run --rm --entrypoint /usr/bin/bash -it pi-deepseek:latest
```
