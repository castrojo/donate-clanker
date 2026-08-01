#!/usr/bin/env python3
"""Unattended real-QEMU VM boot feedback loop.

This is intentionally opt-in: it never downloads an artifact or chooses a
cached image. Pass exactly one explicit raw disk or immutable runner image.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import pathlib
import re
import shutil
import signal
import socket
import struct
import subprocess
import sys
import tempfile
import threading
import time
import uuid
from typing import BinaryIO, Iterable


STATUS_VERSION = 1
READINESS_TYPES = ("control_ack", "network", "hive", "worker_ready")
IMMUTABLE_REF = re.compile(r"^[^@\s]+@sha256:[0-9a-f]{64}$")
EFI_SYSTEM_PARTITION = uuid.UUID("c12a7328-f81f-11d2-ba4b-00a0c93ec93b")
ROOT_UUID = re.compile(
    rb"root=UUID=([0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12})", re.IGNORECASE
)
ROOT_DEVICE_FAILURES = (
    re.compile(rb"ALERT!\s+UUID=[0-9a-f-]+\s+does not exist", re.IGNORECASE),
    re.compile(rb"Gave up waiting for root file system device", re.IGNORECASE),
    re.compile(rb"Timed out waiting for device .*/dev/disk/by-uuid/", re.IGNORECASE),
    re.compile(rb"(?:Failed to mount|Dependency failed for) /sysroot", re.IGNORECASE),
)
BOOT_MARKERS = (
    re.compile(rb"Linux version "),
    re.compile(rb"systemd\[1\]: Started systemd-journald\.service"),
    re.compile(rb"Reached target basic\.target"),
)
SECRET_FIELD = re.compile(
    r"(?i)((?:registration[_-])?token|password|api[_-]?key|secret)"
    r"(\s*[:=]\s*)([\"']?)([^\"'\s,}\n]+)"
)


class HarnessFailure(Exception):
    def __init__(self, stage: str, message: str):
        super().__init__(message)
        self.stage = stage


def readiness_stage(status_type: str) -> str:
    return "control-ack" if status_type == "control_ack" else f"readiness:{status_type}"


class StatusTracker:
    def __init__(self) -> None:
        self.index = 0
        self.saw_optional_boot = False

    @property
    def complete(self) -> bool:
        return self.index == len(READINESS_TYPES)

    def accept(self, status: object) -> str | None:
        if not isinstance(status, dict) or status.get("version") != STATUS_VERSION:
            raise HarnessFailure("readiness", "guest sent an unsupported status version")
        status_type = status.get("type")
        if status_type == "boot" and not self.saw_optional_boot and self.index == 0:
            self.saw_optional_boot = True
            return None
        expected = READINESS_TYPES[self.index]
        if status_type != expected:
            raise HarnessFailure(
                readiness_stage(expected),
                f"guest sent {status_type!r}, waiting for {expected!r}",
            )
        self.index += 1
        return status_type


def redact(text: str, secrets: Iterable[str] = ()) -> str:
    for secret in sorted({value for value in secrets if value}, key=len, reverse=True):
        text = text.replace(secret, "<redacted>")
    return SECRET_FIELD.sub(r"\1\2<redacted>", text)


class OutputCapture:
    def __init__(self, secrets: Iterable[str]):
        self._secrets = tuple(secrets)
        self._lock = threading.Lock()
        self._data = bytearray()
        self.booted = threading.Event()
        self.closed = threading.Event()

    def feed(self, data: bytes) -> None:
        text = redact(data.decode(errors="replace"), self._secrets)
        clean = text.encode()
        with self._lock:
            self._data.extend(clean)
            if len(self._data) > 64 * 1024:
                del self._data[: len(self._data) - 64 * 1024]
        if any(marker.search(clean) for marker in BOOT_MARKERS):
            self.booted.set()

    def tail(self, limit: int = 4000) -> str:
        with self._lock:
            return bytes(self._data[-limit:]).decode(errors="replace")

    def root_uuid_diagnostic(self) -> str | None:
        self.closed.wait(0.1)
        with self._lock:
            data = bytes(self._data)
        if not any(marker.search(data) for marker in ROOT_DEVICE_FAILURES):
            return None
        requested = sorted(
            {match.group(1).decode().lower() for match in ROOT_UUID.finditer(data)}
        )
        detail = ", ".join(requested) if requested else "the configured root filesystem"
        return (
            "guest reported an unavailable root UUID "
            f"({detail}). This indicates a stale EFI root=UUID contract in the "
            "signed guest artifact. The artifact is owned by "
            "projectbluefin/fsdk-containers; publish a corrected signed artifact "
            "there. Keep the existing checksum/signature gates fail-closed; no "
            "local repair or mutable workaround was attempted."
        )


def read_env_file(path: pathlib.Path) -> dict[str, str]:
    values: dict[str, str] = {}
    if not path.is_file():
        return values
    for raw_line in path.read_text().splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
            value = value[1:-1]
        values[key.strip()] = value
    return values


def sha256(path: pathlib.Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def verify_raw(path: pathlib.Path) -> None:
    if not path.is_file():
        raise HarnessFailure("artifact", f"raw artifact not found: {path}")
    sidecar = pathlib.Path(f"{path}.sha256")
    if not sidecar.is_file():
        raise HarnessFailure("artifact", f"checksum sidecar not found: {sidecar}")
    fields = sidecar.read_text().split()
    if not fields or not re.fullmatch(r"[0-9a-fA-F]{64}", fields[0]):
        raise HarnessFailure("artifact", f"invalid checksum sidecar: {sidecar}")
    actual = sha256(path)
    if actual.lower() != fields[0].lower():
        raise HarnessFailure("artifact", f"checksum mismatch for {path}")


def _efi_root_uuids(image: BinaryIO, first_lba: int, last_lba: int) -> set[str]:
    image.seek(first_lba * 512)
    remaining = (last_lba - first_lba + 1) * 512
    overlap = b""
    found: set[str] = set()
    while remaining:
        chunk = image.read(min(1024 * 1024, remaining))
        if not chunk:
            break
        remaining -= len(chunk)
        data = overlap + chunk
        found.update(match.group(1).decode().lower() for match in ROOT_UUID.finditer(data))
        overlap = data[-128:]
    return found


def _ext4_uuid(image: BinaryIO, first_lba: int) -> str | None:
    image.seek(first_lba * 512 + 1024)
    superblock = image.read(120)
    if len(superblock) < 120 or superblock[56:58] != b"\x53\xef":
        return None
    return str(uuid.UUID(bytes=superblock[104:120]))


def raw_boot_contract(path: pathlib.Path) -> tuple[set[str], set[str]] | None:
    """Read the EFI root UUID and ext4 root UUID without mounting the image."""

    image_size = path.stat().st_size
    with path.open("rb") as image:
        image.seek(512)
        header = image.read(92)
        if header[:8] != b"EFI PART":
            return None
        entry_lba, entry_count, entry_size = struct.unpack_from("<QII", header, 72)
        if not 0 < entry_count <= 4096 or not 128 <= entry_size <= 4096:
            return None
        entries_end = (entry_lba * 512) + (entry_count * entry_size)
        if entries_end > image_size:
            return None
        image.seek(entry_lba * 512)
        entries = image.read(entry_count * entry_size)
        if len(entries) != entry_count * entry_size:
            return None

        efi_partition: tuple[int, int] | None = None
        root_partitions: list[tuple[int, str]] = []
        ext4_partitions: list[tuple[int, str]] = []
        for index in range(entry_count):
            entry = entries[index * entry_size : (index + 1) * entry_size]
            if entry[:16] == b"\0" * 16:
                continue
            first_lba, last_lba = struct.unpack_from("<QQ", entry, 32)
            if first_lba > last_lba or (last_lba + 1) * 512 > image_size:
                continue
            name = entry[56:128].decode("utf-16le", errors="ignore").rstrip("\0")
            if uuid.UUID(bytes_le=entry[:16]) == EFI_SYSTEM_PARTITION:
                efi_partition = (first_lba, last_lba)
                continue
            filesystem_uuid = _ext4_uuid(image, first_lba)
            if filesystem_uuid is not None:
                ext4_partitions.append((first_lba, filesystem_uuid))
                if name.lower() == "root":
                    root_partitions.append((first_lba, filesystem_uuid))

        if efi_partition is None:
            return None
        configured = _efi_root_uuids(image, *efi_partition)
        actual = {value for _, value in (root_partitions or ext4_partitions)}
        if not configured or not actual:
            return None
        return configured, actual


def verify_raw_boot_contract(path: pathlib.Path) -> None:
    contract = raw_boot_contract(path)
    if contract is None:
        return
    configured, actual = contract
    if configured & actual:
        return
    requested = ", ".join(sorted(configured))
    found = ", ".join(sorted(actual))
    raise HarnessFailure(
        "artifact",
        "raw VM artifact boot contract mismatch: EFI requests root UUID "
        f"{requested}, but the ext4 root filesystem UUID is {found}. "
        "The signed guest artifact is owned by projectbluefin/fsdk-containers; "
        "publish a corrected signed artifact there. Checksum verification "
        "passed; no local repair or mutable workaround was attempted.",
    )


def find_firmware(architecture: str) -> pathlib.Path:
    configured = os.environ.get("DONATE_CLANKER_QEMU_FIRMWARE")
    if configured:
        firmware = pathlib.Path(configured)
        if not firmware.is_file():
            raise HarnessFailure("host", f"configured firmware is not a file: {firmware}")
        return firmware

    names = (
        ("OVMF_CODE.fd", "OVMF_CODE_4M.fd", "edk2-x86_64-code.fd")
        if architecture == "x86_64"
        else ("AAVMF_CODE.fd", "QEMU_EFI.fd", "edk2-aarch64-code.fd")
    )
    roots = (
        pathlib.Path("/usr/share"),
        pathlib.Path("/usr/lib"),
        pathlib.Path("/usr/libexec"),
        pathlib.Path("/usr/local/share"),
        pathlib.Path("/opt/homebrew/share"),
    )
    qemu = shutil.which(f"qemu-system-{architecture}")
    if qemu:
        roots += (pathlib.Path(qemu).parent.parent / "share" / "qemu",)
    for root in roots:
        if not root.is_dir():
            continue
        for name in names:
            matches = root.rglob(name)
            try:
                return next(matches)
            except StopIteration:
                pass
    raise HarnessFailure(
        "host",
        f"matching {architecture} firmware not found; set DONATE_CLANKER_QEMU_FIRMWARE",
    )


def require_kvm() -> None:
    kvm = pathlib.Path("/dev/kvm")
    if not kvm.is_char_device() or not os.access(kvm, os.R_OK | os.W_OK):
        raise HarnessFailure("host", "/dev/kvm is unavailable or not usable by this user")


def build_qemu_command(
    raw: pathlib.Path,
    firmware: pathlib.Path,
    control_socket: pathlib.Path,
    qemu: str = "qemu-system-x86_64",
) -> list[str]:
    return [
        qemu,
        "-enable-kvm",
        "-machine",
        "q35",
        "-cpu",
        "host",
        "-smp",
        "2",
        "-m",
        "2048",
        "-snapshot",
        "-drive",
        f"if=pflash,format=raw,readonly=on,file={firmware}",
        "-drive",
        f"file={raw},format=raw,if=virtio",
        "-nic",
        "user,model=virtio",
        "-chardev",
        f"socket,id=control,path={control_socket}",
        "-device",
        "virtio-serial-pci",
        "-device",
        "virtserialport,chardev=control,name=org.projectbluefin.donate-clanker.bootstrap",
        "-nographic",
    ]


def build_runner_command(
    runner: str, state: pathlib.Path, socket_name: str, run_id: str
) -> list[str]:
    return [
        "podman",
        "run",
        "--rm",
        "--name",
        f"donate-clanker-vm-e2e-{run_id}",
        "--device",
        "/dev/kvm",
        "--mount",
        f"type=bind,src={state},dst=/run/donate-clanker,rw",
        "--env",
        f"DONATE_CLANKER_BOOTSTRAP_SOCKET=/run/donate-clanker/{socket_name}",
        "--env",
        f"DONATE_CLANKER_RUN_ID={run_id}",
        "--env",
        "DONATE_CLANKER_VM=1",
        runner,
    ]


def consume_statuses(
    connection: socket.socket,
    deadline: float,
    process: subprocess.Popen[bytes],
    capture: OutputCapture,
) -> None:
    tracker = StatusTracker()
    received = bytearray()
    connection.settimeout(0.2)
    while not tracker.complete:
        if process.poll() is not None:
            expected = READINESS_TYPES[tracker.index]
            raise_boot_failure(
                capture,
                readiness_stage(expected),
                f"VM runner exited waiting for {expected} (status {process.returncode})",
            )
        if time.monotonic() >= deadline:
            expected = READINESS_TYPES[tracker.index]
            raise_boot_failure(
                capture,
                readiness_stage(expected), f"timed out waiting for {expected}"
            )
        try:
            chunk = connection.recv(4096)
        except socket.timeout:
            continue
        if not chunk:
            expected = READINESS_TYPES[tracker.index]
            raise_boot_failure(
                capture,
                readiness_stage(expected), f"control channel closed waiting for {expected}"
            )
        received.extend(chunk)
        while b"\n" in received:
            line, _, remainder = received.partition(b"\n")
            received = bytearray(remainder)
            try:
                status = json.loads(line)
            except json.JSONDecodeError:
                raise HarnessFailure("readiness", "guest sent malformed status JSON")
            status_type = tracker.accept(status)
            if status_type is None:
                continue
            if status_type == "control_ack":
                print("stage=control-ack")
            if status_type == "network":
                print("stage=network-ready")
            elif status_type == "hive":
                print("stage=hive-ready")
            elif status_type == "worker_ready":
                print("stage=worker-ready")


def pump_output(process: subprocess.Popen[bytes], capture: OutputCapture) -> None:
    assert process.stdout is not None
    try:
        while True:
            chunk = process.stdout.read(4096)
            if not chunk:
                return
            capture.feed(chunk)
    finally:
        capture.closed.set()


def raise_boot_failure(capture: OutputCapture, stage: str, message: str) -> None:
    diagnostic = capture.root_uuid_diagnostic()
    if diagnostic is not None:
        raise HarnessFailure("artifact-contract", diagnostic)
    raise HarnessFailure(stage, message)


def terminate_process(process: subprocess.Popen[bytes] | None, timeout: float) -> None:
    if process is None or process.poll() is not None:
        return
    try:
        os.killpg(process.pid, signal.SIGINT)
    except ProcessLookupError:
        return
    try:
        process.wait(timeout=timeout)
        return
    except subprocess.TimeoutExpired:
        pass
    try:
        os.killpg(process.pid, signal.SIGTERM)
        process.wait(timeout=1)
        return
    except (ProcessLookupError, subprocess.TimeoutExpired):
        pass
    try:
        os.killpg(process.pid, signal.SIGKILL)
    except ProcessLookupError:
        return
    try:
        process.wait(timeout=1)
    except subprocess.TimeoutExpired as exc:
        raise HarnessFailure("cleanup", "VM runner did not exit after SIGKILL") from exc


def remove_runner_container(name: str, timeout: float) -> None:
    try:
        exists = subprocess.run(
            ["podman", "container", "exists", name],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            timeout=timeout,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise HarnessFailure("cleanup", f"unable to inspect runner container: {exc}") from exc
    if exists.returncode == 1:
        return
    if exists.returncode != 0:
        raise HarnessFailure("cleanup", "unable to inspect runner container")
    try:
        removed = subprocess.run(
            ["podman", "rm", "-f", name],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.PIPE,
            timeout=timeout,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise HarnessFailure("cleanup", f"unable to remove runner container: {exc}") from exc
    if removed.returncode != 0:
        detail = redact(removed.stderr.decode(errors="replace"))
        raise HarnessFailure("cleanup", f"runner container removal failed: {detail}")


def self_test() -> None:
    global find_firmware, require_kvm

    secret = "test-registration-token"
    assert secret not in redact(f'registration_token="{secret}"', [secret])
    assert "<redacted>" in redact(f'registration_token="{secret}"', [secret])
    assert "control_ack" in READINESS_TYPES
    tracker = StatusTracker()
    for status_type in ("boot", *READINESS_TYPES):
        tracker.accept({"version": STATUS_VERSION, "type": status_type})
    assert tracker.complete
    tracker = StatusTracker()
    tracker.accept({"version": STATUS_VERSION, "type": "control_ack"})
    try:
        tracker.accept({"version": STATUS_VERSION, "type": "hive"})
    except HarnessFailure as exc:
        assert exc.stage == "readiness:network"
    else:
        raise AssertionError("out-of-order readiness status was accepted")

    state = pathlib.Path(".vm-boot-e2e-self-test")
    shutil.rmtree(state, ignore_errors=True)
    state.mkdir(mode=0o700)
    try:
        def write_raw_fixture(
            path: pathlib.Path, configured_uuid: str, root_uuid: str
        ) -> None:
            sector = 512
            efi_first, efi_last = 34, 41
            root_first, root_last = 42, 63
            with path.open("wb") as image:
                image.truncate((root_last + 1) * sector)
                header = bytearray(92)
                header[:8] = b"EFI PART"
                struct.pack_into("<QII", header, 72, 2, 128, 128)
                image.seek(sector)
                image.write(header)

                entries = bytearray(128 * 128)

                def add_entry(
                    index: int,
                    type_uuid: uuid.UUID,
                    first: int,
                    last: int,
                    name: str,
                ) -> None:
                    offset = index * 128
                    entries[offset : offset + 16] = type_uuid.bytes_le
                    entries[offset + 16 : offset + 32] = uuid.UUID(
                        "00000000-0000-0000-0000-000000000001"
                    ).bytes_le
                    struct.pack_into("<QQQ", entries, offset + 32, first, last, 0)
                    entries[offset + 56 : offset + 128] = name.encode(
                        "utf-16le"
                    ).ljust(72, b"\0")

                add_entry(0, EFI_SYSTEM_PARTITION, efi_first, efi_last, "efi")
                add_entry(
                    1,
                    uuid.UUID("4f68bce3-e8cd-4db1-96e7-fbcaf984b709"),
                    root_first,
                    root_last,
                    "root",
                )
                image.seek(2 * sector)
                image.write(entries)
                image.seek(efi_first * sector)
                image.write(f"root=UUID={configured_uuid}".encode())
                superblock = bytearray(120)
                superblock[56:58] = b"\x53\xef"
                superblock[104:120] = uuid.UUID(root_uuid).bytes
                image.seek(root_first * sector + 1024)
                image.write(superblock)

        raw = state / "guest.raw"
        firmware = state / "firmware.fd"
        command = build_qemu_command(raw, firmware, state / "control.sock")
        joined = "\0".join(command)
        assert "registration_token" not in joined
        runner = build_runner_command(
            "ghcr.io/example/runner@sha256:" + "a" * 64,
            state,
            "control.sock",
            "self-test",
        )
        assert runner.count("--mount") == 1
        assert "/workspace" not in "\0".join(runner)

        actual_uuid = "a5e5b74b-7aa6-58b1-8408-e4147a36da17"
        stale_uuid = "9e71ad99-5ddc-5b20-8b9c-f3f6b4e570e1"
        corrected = state / "corrected.raw"
        write_raw_fixture(corrected, actual_uuid, actual_uuid)
        corrected.with_name(f"{corrected.name}.sha256").write_text(
            f"{sha256(corrected)}  {corrected.name}\n"
        )
        assert raw_boot_contract(corrected) == ({actual_uuid}, {actual_uuid})
        verify_raw_boot_contract(corrected)
        stale = state / "stale.raw"
        write_raw_fixture(stale, stale_uuid, actual_uuid)
        stale.with_name(f"{stale.name}.sha256").write_text(
            f"{sha256(stale)}  {stale.name}\n"
        )
        try:
            verify_raw_boot_contract(stale)
        except HarnessFailure as exc:
            assert exc.stage == "artifact"
            assert "projectbluefin/fsdk-containers" in str(exc)
            assert stale_uuid in str(exc)
            assert actual_uuid in str(exc)
        else:
            raise AssertionError("stale EFI root UUID was not rejected")

        stale_popen_calls: list[object] = []
        original_popen = subprocess.Popen

        def unexpected_popen(*args: object, **kwargs: object) -> None:
            stale_popen_calls.append((args, kwargs))
            raise AssertionError("stale artifact attempted to start QEMU")

        subprocess.Popen = unexpected_popen  # type: ignore[assignment]
        try:
            try:
                run(
                    argparse.Namespace(
                        raw=stale,
                        runner=None,
                        hive_env=state / "missing.env",
                        timeout=1.0,
                        cleanup_timeout=1.0,
                        state_root=state,
                        self_test=False,
                    )
                )
            except HarnessFailure as exc:
                assert exc.stage == "artifact"
                assert stale_uuid in str(exc)
            else:
                raise AssertionError("stale artifact reached the QEMU launch seam")
        finally:
            subprocess.Popen = original_popen
        assert not stale_popen_calls

        original_which = shutil.which
        original_firmware = find_firmware
        original_require_kvm = require_kvm
        corrected_popen_calls: list[list[str]] = []

        def corrected_popen(
            command: list[str], *args: object, **kwargs: object
        ) -> None:
            corrected_popen_calls.append(command)
            raise OSError("self-test QEMU launch seam")

        shutil.which = lambda name: f"/fake/{name}"
        find_firmware = lambda architecture: firmware
        require_kvm = lambda: None
        subprocess.Popen = corrected_popen  # type: ignore[assignment]
        try:
            try:
                run(
                    argparse.Namespace(
                        raw=corrected,
                        runner=None,
                        hive_env=state / "missing.env",
                        timeout=1.0,
                        cleanup_timeout=1.0,
                        state_root=state,
                        self_test=False,
                    )
                )
            except HarnessFailure as exc:
                assert exc.stage == "qemu-start"
            else:
                raise AssertionError("corrected artifact did not reach QEMU")
        finally:
            subprocess.Popen = original_popen
            shutil.which = original_which
            find_firmware = original_firmware
            require_kvm = original_require_kvm
        assert len(corrected_popen_calls) == 1
        assert str(corrected) in "\0".join(corrected_popen_calls[0])
        assert not any(state.glob(".vm-boot-e2e-*"))

        runner_text = "\0".join(runner)
        assert runner[:2] == ["podman", "run"]
        assert runner[runner.index("--device") + 1] == "/dev/kvm"
        assert "registration_token" not in runner_text
        assert "/workspace" not in runner_text

        capture = OutputCapture([secret])
        capture.feed(f"registration_token={secret}\n".encode())
        assert secret not in capture.tail()
        capture.feed(f"Kernel command line: root=UUID={stale_uuid}\n".encode())
        capture.feed(f"ALERT! UUID={stale_uuid} does not exist\n".encode())
        diagnostic = capture.root_uuid_diagnostic()
        assert diagnostic is not None
        assert "projectbluefin/fsdk-containers" in diagnostic
        assert stale_uuid in diagnostic
        print("vm-boot-e2e: deterministic host seam passed")
    finally:
        shutil.rmtree(state, ignore_errors=True)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--raw", type=pathlib.Path, help="explicit local raw guest artifact")
    parser.add_argument("--runner", help="explicit immutable OCI runner reference")
    parser.add_argument(
        "--hive-env",
        type=pathlib.Path,
        default=pathlib.Path.home() / ".config/hive/contributor.env",
        help="optional Hive env file (default: ~/.config/hive/contributor.env)",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=float(os.environ.get("DONATE_CLANKER_VM_E2E_TIMEOUT", "60")),
    )
    parser.add_argument("--cleanup-timeout", type=float, default=10.0)
    parser.add_argument("--state-root", type=pathlib.Path, default=pathlib.Path.cwd())
    parser.add_argument("--self-test", action="store_true")
    return parser.parse_args()


def run(args: argparse.Namespace) -> int:
    if args.self_test:
        self_test()
        return 0
    if bool(args.raw) == bool(args.runner):
        raise HarnessFailure(
            "artifact",
            "pass exactly one of --raw PATH or --runner IMAGE@sha256:DIGEST",
        )
    if args.timeout <= 0 or args.cleanup_timeout <= 0:
        raise HarnessFailure("host", "timeouts must be positive")

    if args.raw:
        raw = args.raw.expanduser().resolve()
        verify_raw(raw)
        verify_raw_boot_contract(raw)
        qemu = shutil.which("qemu-system-x86_64")
        if not qemu:
            raise HarnessFailure("host", "qemu-system-x86_64 is not installed")
        firmware = find_firmware("x86_64")
    else:
        assert args.runner is not None
        if not IMMUTABLE_REF.fullmatch(args.runner):
            raise HarnessFailure("artifact", "runner must be an immutable OCI digest reference")
        if not shutil.which("podman"):
            raise HarnessFailure("host", "podman is required for the runner path")
    require_kvm()

    env_values = read_env_file(args.hive_env.expanduser())
    endpoint = (
        os.environ.get("DONATE_CLANKER_VM_E2E_HIVE_ENDPOINT")
        or env_values.get("HIVE_WS_URL")
        or env_values.get("HIVE_HUB")
        or "wss://example.invalid"
    )
    token = (
        os.environ.get("DONATE_CLANKER_VM_E2E_REGISTRATION_TOKEN")
        or env_values.get("HIVE_REGISTRATION_TOKEN")
        or "e2e-probe-token"
    )
    backend = os.environ.get("DONATE_CLANKER_VM_E2E_BACKEND", "goose")
    run_id = f"{int(time.time())}-{os.getpid()}"

    state_root = args.state_root.expanduser().resolve()
    if not state_root.is_dir():
        raise HarnessFailure("host", f"state root is not a directory: {state_root}")
    state = pathlib.Path(tempfile.mkdtemp(prefix=".vm-boot-e2e-", dir=state_root))
    control_socket = state / "bootstrap.sock"
    server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    try:
        server.bind(str(control_socket))
        os.chmod(control_socket, 0o600)
        server.listen(1)
    except OSError as exc:
        server.close()
        shutil.rmtree(state, ignore_errors=True)
        raise HarnessFailure("control-channel", f"unable to create control socket: {exc}") from exc
    server.settimeout(0.2)
    process: subprocess.Popen[bytes] | None = None
    capture = OutputCapture([token, endpoint])
    pump: threading.Thread | None = None
    connection: socket.socket | None = None
    failure: HarnessFailure | None = None
    cleanup_failure: HarnessFailure | None = None
    runner_name = f"donate-clanker-vm-e2e-{run_id}"
    started = time.monotonic()
    print("stage=artifact")
    print("stage=host")
    try:
        if args.raw:
            command = build_qemu_command(raw, firmware, control_socket, qemu)
        else:
            command = build_runner_command(args.runner, state, control_socket.name, run_id)
        try:
            process = subprocess.Popen(
                command,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                start_new_session=True,
            )
        except OSError as exc:
            raise HarnessFailure("qemu-start", f"unable to start VM runner: {exc}") from exc
        pump = threading.Thread(target=pump_output, args=(process, capture), daemon=True)
        pump.start()
        print("stage=qemu-start")

        deadline = started + args.timeout
        while connection is None:
            if time.monotonic() >= deadline:
                raise_boot_failure(
                    capture, "control-channel", "QEMU did not connect the control socket"
                )
            if process.poll() is not None:
                raise_boot_failure(
                    capture,
                    "qemu-start",
                    f"VM runner exited before control connection (status {process.returncode})",
                )
            try:
                connection, _ = server.accept()
            except socket.timeout:
                continue
        print("stage=control-channel")
        connection.settimeout(0.2)
        payload = {
            "version": STATUS_VERSION,
            "hive_endpoint": endpoint,
            "registration_token": token,
            "backend": backend,
            "run_id": run_id,
        }
        connection.sendall((json.dumps(payload, separators=(",", ":")) + "\n").encode())

        boot_deadline = min(deadline, time.monotonic() + args.timeout)
        while not capture.booted.is_set():
            if time.monotonic() >= boot_deadline:
                raise_boot_failure(
                    capture,
                    "guest-boot",
                    "no Linux/systemd boot marker was observed on the QEMU console",
                )
            if process.poll() is not None:
                raise_boot_failure(
                    capture,
                    "guest-boot",
                    f"VM runner exited before guest boot (status {process.returncode})",
                )
            time.sleep(0.1)
        print("stage=guest-boot")
        consume_statuses(connection, deadline, process, capture)
        print("stage=readiness")
        return 0
    except HarnessFailure as exc:
        failure = exc
        raise
    finally:
        if connection is not None:
            connection.close()
        server.close()
        try:
            terminate_process(process, args.cleanup_timeout)
        except HarnessFailure as exc:
            cleanup_failure = exc
            if failure is None:
                failure = exc
            else:
                print(f"vm-boot-e2e: cleanup warning: {exc}", file=sys.stderr)
        if args.runner:
            try:
                remove_runner_container(runner_name, args.cleanup_timeout)
            except HarnessFailure as exc:
                cleanup_failure = exc
                if failure is None:
                    failure = exc
                else:
                    print(f"vm-boot-e2e: cleanup warning: {exc}", file=sys.stderr)
        if pump is not None:
            pump.join(timeout=1)
        try:
            shutil.rmtree(state)
        except OSError as exc:
            cleanup_failure = HarnessFailure("cleanup", f"unable to remove run state: {exc}")
            if failure is None:
                failure = cleanup_failure
            else:
                print(
                    f"vm-boot-e2e: cleanup warning: {redact(str(exc), [token])}",
                    file=sys.stderr,
                )
        if state.exists() and failure is None:
            cleanup_failure = HarnessFailure("cleanup", "run state remains after cleanup")
            failure = cleanup_failure
        if cleanup_failure is None:
            print("stage=cleanup")
        if failure is not None:
            tail = capture.tail()
            if tail:
                print(
                    "vm-boot-e2e: redacted console tail:\n" + redact(tail, [token]),
                    file=sys.stderr,
                )
        if cleanup_failure is not None and cleanup_failure is failure:
            raise cleanup_failure


def main() -> int:
    try:
        args = parse_args()
        run(args)
        if not args.self_test:
            print("vm-boot-e2e: PASS")
    except HarnessFailure as exc:
        print(f"vm-boot-e2e: FAIL stage={exc.stage} error={redact(str(exc))}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("vm-boot-e2e: FAIL stage=cleanup error=interrupted", file=sys.stderr)
        return 130
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
