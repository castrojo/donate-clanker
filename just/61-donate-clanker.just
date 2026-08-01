# Direct donate-clanker launcher.
#
# The compatibility image owns Hive credential loading, the Goose worker, and
# the attached contributor session. The host contributes only the two mounts
# below and the fixed agent settings.

donate-clanker:
    #!/usr/bin/env bash
    set -euo pipefail
    image="${DONATE_CLANKER_IMAGE:-localhost/donate-clanker:latest}"
    exec podman run --rm -it --userns=keep-id \
      -e AGENT_BACKEND=goose \
      -e GOOSE_PROVIDER=github_copilot \
      -e GOOSE_MODEL=gpt-5.6-luna \
      -e AGENT_MODEL=gpt-5.6-luna \
      -e GOOSE_DISABLE_KEYRING=1 \
      -e GOOSE_TELEMETRY_ENABLED=false \
      -v "$HOME/.config/hive:/config:ro,Z" \
      -v "$HOME/.config/goose:/home/dev/.config/goose:Z" \
      -v "$PWD:/workspace:Z" \
      "$image"
