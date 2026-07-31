# Direct Container Launch Design

## Goal

Make `ujust donate-clanker` launch the published compatibility container
directly with Podman instead of starting the QEMU/VM bootstrap path.

## Launch contract

The recipe will run:

```bash
podman run --pull=always --rm -it --userns=keep-id \
  -e AGENT_BACKEND=goose \
  -e GOOSE_PROVIDER=githubcli \
  -e GOOSE_MODEL=gpt-5.6-luna \
  -e AGENT_MODEL=gpt-5.6-luna \
  -v "$HOME/.config/hive:/config:ro,Z" \
  -v "$PWD:/workspace:Z" \
  ghcr.io/projectbluefin/donate-clanker:stable
```

The host Hive configuration remains read-only, and the current directory is
the worker workspace. Podman owns the foreground lifecycle and removes the
container on exit.

## Scope and compatibility

The public `donate-clanker` recipe will use this direct-container contract.
The existing diagnostic and stale-container recipes remain available unless
their behavior is directly coupled to the removed default VM path. VM-specific
implementation code and unrelated worktree changes are not reverted or
rewritten. Documentation and onboarding tests will be updated so they describe
and assert the direct image invocation.

## Error handling

Podman remains the single launch boundary. Its exit status is returned by the
recipe, and `--pull=always` surfaces image pull failures rather than silently
using a stale image. No host workspace, home directory, or container socket is
added beyond the two mounts explicitly required by the compatibility image.

## Validation

Update the focused launcher/onboarding assertions for the direct Podman
arguments, then run the repository's existing checks:

```bash
just --justfile just/61-donate-clanker.just --list
git diff --check
go test ./...
gofmt -l .
```
