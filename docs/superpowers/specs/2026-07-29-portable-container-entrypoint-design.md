# Portable donate-clanker Container Entrypoint

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

The published image owns the contributor launcher, Goose runtime, local
RamaLama integration, bundled policy, model catalog, and default paths. The
host supplies only:

- `/config`: read-only Hive/GitHub authentication state
- `/workspace`: the repository or working directory to donate against

The entrypoint validates the mounted configuration before starting. Missing,
malformed, or unauthenticated state produces an actionable error and never
starts a worker. The container remains foreground-only and removes itself on
exit.

The image must use a non-root runtime user, must not require a host container
socket, and must not mount the host home directory wholesale. README examples
use the immutable production image reference or the explicitly documented
`stable` release channel.

## Runtime flow

1. Podman starts the published image with the two documented mounts.
2. The entrypoint resolves `/config` and `/workspace` using container-local
   defaults.
3. Hive credentials and GitHub authentication are validated.
4. The launcher starts the local inference helper and worker in the same pod
   network namespace.
5. The worker connects to Hive, executes assignments through Goose, and exits
   cleanly when the terminal or container is stopped.

## Error handling

- Missing `/config` or required files: identify the missing host setup command.
- Invalid Hive URL or registration data: report the exact field without secrets.
- Missing `/workspace`: fail before starting inference.
- Missing helper or contributor image configuration: use image-bundled defaults
  or fail with the exact override variable, never silently fall back.
- Shutdown signals: stop the worker and helper, then remove the ephemeral pod.

## Validation

- Add image contract tests for `/config`, `/workspace`, the non-root user, and
  the default entrypoint.
- Add launcher tests proving container-local defaults work without host-path
  environment overrides.
- Update README and factory skill documentation with the canonical command.
- Run `go test ./...`, `git diff --check`, `gofmt -l .`, and the existing image
  validation workflow.
