# Reviewing Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make validation prerequisites visible at contributor startup, retain explicit VM identity compatibility reporting, and publish a validated contributor image.

**Architecture:** Hive remains responsible for assignment and repository checkout. The contributor entrypoint only examines locally installed validation commands and reports missing capabilities before starting Hive. The launcher/image contract prevents a VM identity token from being exposed until a compatible guest consumes its versioned bootstrap field.

**Tech Stack:** Bash, Containerfile, Podman, existing shell contract tests.

## Global Constraints

- Do not add task-selection, checkout, or retry logic outside Hive.
- Do not expose, log, persist, or place credentials in Podman arguments.
- Keep both launch paths foreground-only.
- Do not attempt VM GitHub identity delivery until the FSDK guest artifact supports the versioned field.
- Do not add a general-purpose validation distribution to the contributor image.

---

### Task 1: Report validation capability gaps

**Files:**
- Modify: `image/entrypoint.sh:99-112`
- Test: `tests/image-contract.sh:150-181`

**Interfaces:**
- Consumes: commands on `PATH` for `bats`, `shellcheck`, `systemd-analyze`, `pre-commit`, `just`, and `podman`.
- Produces: one `donate-clanker:` startup line listing unavailable validation commands, without preventing Hive startup.

- [ ] **Step 1: Write the failing contract assertion**

Add the expected helper and message fragments:

```bash
require image/entrypoint.sh \
  'validation_tools=(bats shellcheck systemd-analyze pre-commit just podman)' \
  'validation tools unavailable:'
```

- [ ] **Step 2: Run the focused contract test to verify it fails**

Run: `bash tests/image-contract.sh`

Expected: FAIL reporting the missing required entrypoint fragments.

- [ ] **Step 3: Add the minimal startup report**

Insert this block immediately after the org-skills report:

```bash
validation_tools=(bats shellcheck systemd-analyze pre-commit just podman)
missing_validation_tools=()
for validation_tool in "${validation_tools[@]}"; do
	if ! command -v "$validation_tool" >/dev/null 2>&1; then
		missing_validation_tools+=("$validation_tool")
	fi
done
if ((${#missing_validation_tools[@]})); then
	note "validation tools unavailable: ${missing_validation_tools[*]}"
fi
```

- [ ] **Step 4: Run the focused contract test to verify it passes**

Run: `bash tests/image-contract.sh`

Expected: `✓ image contract holds.`

- [ ] **Step 5: Commit**

```bash
git add image/entrypoint.sh tests/image-contract.sh
git commit -m "fix: report contributor validation tool gaps"
```

### Task 2: Preserve external runtime boundaries

**Files:**
- Modify: `README.md:68-96`
- Test: `tests/just-onboarding.sh:511-640`

**Interfaces:**
- Consumes: the guest’s version-2 bootstrap capability and the pinned Hive contributor runtime.
- Produces: actionable output that distinguishes container GitHub identity from the VM guest’s unavailable identity field.

- [ ] **Step 1: Write the failing launcher test**

Assert that a resolved host token is withheld from the VM bootstrap envelope:

```bash
assert_contains "VM GitHub identity is blocked" "$OUT"
assert_file_contains "github_token:absent" "$consumed_marker"
```

- [ ] **Step 2: Run the launcher contract to verify it fails**

Run: `bash tests/just-onboarding.sh`

Expected: FAIL until the launcher reports the guest capability prerequisite and omits the token.

- [ ] **Step 3: Implement the versioned-envelope boundary**

Use the existing `report_vm_github_identity_blocked` path and omit any GitHub
identity field from the envelope until the guest release advertises a
compatible field. Document the container-only fallback in `README.md`.

- [ ] **Step 4: Run the launcher contract to verify it passes**

Run: `bash tests/just-onboarding.sh`

Expected: all launcher contract sections pass or skip only the KVM-dependent
sections when `/dev/kvm` is unavailable.

- [ ] **Step 5: Commit**

```bash
git add just/61-donate-clanker.just README.md tests/guest-bootstrap-consumer.py tests/just-onboarding.sh
git commit -m "fix: keep VM GitHub identity behind guest compatibility"
```

### Task 3: Build the contributor image

**Files:**
- Modify: `image/Containerfile:8-94`
- Test: `tests/image-contract.sh`

**Interfaces:**
- Consumes: the full Hive commit declared by `just/61-donate-clanker.just`.
- Produces: `donate-clanker:reviewing-runtime`, built without secret build arguments.

- [ ] **Step 1: Assert matching Hive pins**

Keep this test in `tests/image-contract.sh`:

```bash
[[ -n "$launcher_hive_pin" || "$launcher_hive_pin" != "$image_hive_pin" ]]
```

Replace it with the correct failure condition:

```bash
if [[ -z "$launcher_hive_pin" || "$launcher_hive_pin" != "$image_hive_pin" ]]; then
  fail=1
fi
```

- [ ] **Step 2: Run the image contract**

Run: `bash tests/image-contract.sh`

Expected: `✓ image contract holds.`

- [ ] **Step 3: Build the image**

Run: `podman build -f image/Containerfile -t donate-clanker:reviewing-runtime .`

Expected: successful image build with the matching pinned Hive runtime.

- [ ] **Step 4: Run final repository validation**

Run: `bash tests/just-onboarding.sh && git diff --check && just --justfile just/61-donate-clanker.just --list && pre-commit run --all-files`

Expected: all commands succeed, with KVM-only launcher assertions skipped when
the host lacks usable KVM.

- [ ] **Step 5: Commit**

```bash
git add image/Containerfile .github/workflows/validate.yml
git commit -m "build: align contributor runtime image"
```
