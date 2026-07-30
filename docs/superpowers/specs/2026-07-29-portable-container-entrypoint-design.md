# Portable donate-clanker Container Entrypoint

> **Implementation note:** The original monolithic Goose/RamaLama design below
> was superseded by the published compatibility wrapper. The current image
> wraps the pinned `ghcr.io/kubestellar/hive-contributor` runtime, starts its
> relay and Copilot process, and attaches the contributor tmux session to the
> invoking terminal. The direct Podman command in `README.md` is the canonical
> portable contract.

## Goal

Replace the README's raw upstream Hive `podman run` example with a short,
portable command for the published `donate-clanker` image:

```bash
podman run --rm -it --userns=keep-id \
  -v "$HOME/.config/hive:/config:ro,Z" \
  -v "$PWD:/workspace:Z" \
  ghcr.io/projectbluefin/donate-clanker:stable
```

## Contract

The published compatibility image owns the upstream contributor launcher and
supplies the terminal-facing session. The host supplies only:

- `/config`: read-only Hive/GitHub authentication state
- `/workspace`: the repository or working directory to donate against

The entrypoint maps the portable mounts, loads `gh-auth.env` for the upstream
GitHub token contract, starts the upstream contributor process, and attaches
the `contributor` tmux session. Missing mounts fail before startup. Detaching
or interrupting the terminal cleans up the relay and session; Podman removes
the ephemeral container on exit.

The image must use a non-root runtime user, must not require a host container
socket, and must not mount the host home directory wholesale. README examples
use the immutable production image reference or the explicitly documented
`stable` release channel.

## Runtime flow

1. Podman starts the published image with the two documented mounts.
2. The entrypoint resolves `/config` and `/workspace` using container-local
   defaults.
3. The entrypoint starts the upstream relay and CLI inside a `contributor`
   tmux session.
4. The entrypoint attaches the invoking terminal to that session.
5. The worker connects to Hive, executes assignments through the configured
   upstream backend, and exits cleanly when the terminal or container stops.

## Error handling

- Missing `/config` or required files: identify the missing host setup command.
- Missing `/workspace`: fail before starting inference.
- Missing `gh-auth.env`: preserve the upstream authentication error without
  printing secret values.
- Shutdown signals or tmux detach: stop the contributor process and relay, then
  remove the ephemeral container.

## Validation

- Add image contract tests for `/config`, `/workspace`, the non-root user, and
  the default entrypoint.
- Add launcher tests proving container-local defaults work without host-path
  environment overrides.
- Update README and factory skill documentation with the canonical command.
- Run `go test ./...`, `git diff --check`, `gofmt -l .`, and the existing image
  validation workflow.
