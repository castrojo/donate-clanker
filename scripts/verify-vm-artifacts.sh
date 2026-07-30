#!/usr/bin/env bash
set -euo pipefail

manifest="${VM_MANIFEST:-vm/manifest.json}"
fail() {
  echo "verify-vm-artifacts: $*" >&2
  exit 1
}

command -v jq >/dev/null || fail "jq is required"
[[ -r "$manifest" ]] || fail "missing manifest: $manifest"
jq -e '.schema_version == 1 and (.architectures | sort == ["amd64", "arm64"])' \
  "$manifest" >/dev/null || fail "unsupported manifest schema or architectures"

immutable_ref() {
  local name="$1" ref="${!2-}"
  [[ -n "$ref" ]] || fail "$name is unset"
  [[ "$ref" =~ ^[^@[:space:]]+@sha256:[0-9a-f]{64}$ ]] ||
    fail "$name must be an immutable digest reference"
  printf '%s\n' "$ref"
}

refs=()
while IFS= read -r env_name; do
  refs+=("$(immutable_ref "$env_name" "$env_name")")
done < <(jq -r '
  .artifact_contract.runner.reference_env,
  .artifact_contract.guest.reference_env,
  (.artifact_contract.guest.artifacts[] | .reference_env)
' "$manifest")

if [[ "${VM_VERIFY_REMOTE:-true}" == false ]]; then
  echo "static VM artifact contract validation passed"
  exit 0
fi

command -v docker >/dev/null || fail "docker is required for remote validation"
command -v cosign >/dev/null || fail "cosign is required for remote validation"
identity="${COSIGN_CERT_IDENTITY_REGEXP:-}"
issuer="${COSIGN_CERT_ISSUER:-}"
[[ -n "$identity" && -n "$issuer" ]] ||
  fail "COSIGN_CERT_IDENTITY_REGEXP and COSIGN_CERT_ISSUER are required"

for ref in "${refs[@]}"; do
  metadata="$(docker manifest inspect "$ref")" ||
    fail "artifact is missing or unreadable: $ref"
  for arch in amd64 arm64; do
    jq -e --arg arch "$arch" '
      (.manifests // []) | any(.platform.architecture == $arch)
    ' <<<"$metadata" >/dev/null ||
      fail "artifact $ref is missing architecture $arch"
  done

  cosign verify \
    --certificate-identity-regexp "$identity" \
    --certificate-oidc-issuer "$issuer" \
    "$ref" >/dev/null ||
    fail "signature verification failed: $ref"
  cosign verify-attestation \
    --type spdxjson \
    --certificate-identity-regexp "$identity" \
    --certificate-oidc-issuer "$issuer" \
    "$ref" >/dev/null ||
    fail "SPDX SBOM attestation missing: $ref"
  cosign verify-attestation \
    --type slsaprovenance \
    --certificate-identity-regexp "$identity" \
    --certificate-oidc-issuer "$issuer" \
    "$ref" >/dev/null ||
    fail "provenance attestation missing: $ref"
done

echo "VM artifact contract validated for ${#refs[@]} immutable artifacts"
