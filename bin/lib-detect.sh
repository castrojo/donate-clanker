#!/usr/bin/env bash
# lib-detect.sh — shared host-side CLI detection for donate-clanker.
# Sourced by both donate-clanker-bootstrap.sh and the `donate-clanker-doctor`
# just recipe so the two never drift out of sync.
#
# Every tool here must have a matching quadlet/tools/<name>.conf fragment.

# Deterministic detection order. Earlier entries win when auto-picking.
DONATE_CLANKER_TOOL_ORDER=(claude copilot goose)

# tool_installed <name> — is the CLI present at all on the host?
tool_installed() {
  case "$1" in
    claude)  command -v claude &>/dev/null ;;
    copilot) command -v gh &>/dev/null ;;   # copilot backend rides on gh auth, no separate CLI required
    goose)   command -v goose &>/dev/null ;;
    *)       return 1 ;;
  esac
}

# tool_authenticated <name> — is it actually usable right now (logged in)?
# Kept cheap/local-only (file and command existence checks) — never makes a
# network call, so detection stays fast even with several tools installed.
tool_authenticated() {
  case "$1" in
    claude)
      # Claude Code stores its session under ~/.claude / ~/.claude.json once
      # `claude` -> `/login` has been completed.
      [[ -d "${HOME}/.claude" || -f "${HOME}/.claude.json" ]]
      ;;
    copilot)
      gh auth status &>/dev/null
      ;;
    goose)
      # Goose is configured via env vars (GOOSE_PROVIDER/GOOSE_MODEL) or
      # `goose configure`'s on-disk config.
      [[ -n "${GOOSE_PROVIDER:-}" || -f "${HOME}/.config/goose/config.yaml" ]]
      ;;
    *)
      return 1
      ;;
  esac
}

# tool_fixit_hint <name> — one-line, actionable fix for "installed but not authenticated".
tool_fixit_hint() {
  case "$1" in
    claude)  echo "Run: claude   (then type /login and follow the prompts)" ;;
    copilot) echo "Run: gh auth login --web --scopes repo,read:org" ;;
    goose)   echo "Run: goose configure   (or: export GOOSE_PROVIDER=... GOOSE_MODEL=...)" ;;
    *)       echo "See quadlet/tools/${1}.conf for what this tool needs." ;;
  esac
}

# tool_install_hint <name> — one-line install pointer for "not installed at all".
tool_install_hint() {
  case "$1" in
    claude)  echo "Install: npm i -g @anthropic-ai/claude-code" ;;
    copilot) echo "Install: see https://cli.github.com" ;;
    goose)   echo "Install: https://github.com/block/goose/releases" ;;
    *)       echo "" ;;
  esac
}

# detect_first_ready_tool — echoes the first installed+authenticated tool in
# DONATE_CLANKER_TOOL_ORDER, or nothing if none qualify.
detect_first_ready_tool() {
  local t
  for t in "${DONATE_CLANKER_TOOL_ORDER[@]}"; do
    if tool_installed "$t" && tool_authenticated "$t"; then
      echo "$t"
      return 0
    fi
  done
  return 1
}
