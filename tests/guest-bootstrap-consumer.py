#!/usr/bin/env python3
"""Test-only guest consumer for the host bootstrap socket."""

import json
import os
import socket
import sys
import time


def fail(message):
    raise SystemExit(message)


socket_path = os.environ.get("REVIEW_BOOTSTRAP_SOCKET")
marker = os.environ.get("REVIEW_TEST_CONSUMED")
if not socket_path or not marker:
    fail("bootstrap consumer requires socket and marker paths")

client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
client.connect(socket_path)
time.sleep(float(os.environ.get("REVIEW_TEST_CONSUMER_DELAY", "0")))


def recv_line(sock):
    data = bytearray()
    recv_size = int(os.environ.get("REVIEW_TEST_RECV_SIZE", "4096"))
    if not 1 <= recv_size <= 4096:
        fail("invalid bootstrap receive size")
    while b"\n" not in data:
        chunk = sock.recv(recv_size)
        if not chunk:
            fail("bootstrap payload ended before newline")
        data.extend(chunk)
        if len(data) > 65536:
            fail("bootstrap payload exceeds 65536 bytes")
    return bytes(data).split(b"\n", 1)[0]


payload = json.loads(recv_line(client))

required = {"version", "hive_endpoint", "registration_token", "backend", "run_id"}
optional = {"goose_provider", "goose_model", "provider_secret"}
if not required <= set(payload):
    fail("invalid bootstrap envelope fields")
if set(payload) - required - optional:
    fail("unexpected bootstrap envelope fields")
if payload["version"] != 2:
    fail("unsupported bootstrap version")
if payload["backend"] != "goose":
    fail("unsupported backend")
if not payload["hive_endpoint"].startswith(("https://", "wss://")):
    fail("invalid bootstrap endpoint")
if not all(payload[key] for key in ("registration_token", "backend", "run_id")):
    fail("incomplete bootstrap envelope")

# The acknowledgement intentionally contains no credential material.
ack = {"version": 2, "type": "control_ack"}
ack_bytes = (json.dumps(ack, separators=(",", ":")) + "\n").encode()
if os.environ.get("REVIEW_TEST_SPLIT_ACK") == "1":
    client.sendall(ack_bytes[:1])
    time.sleep(0.01)
    client.sendall(ack_bytes[1:])
else:
    client.sendall(ack_bytes)
client.close()
# Record only whether a credential was present, never its value: the harness
# needs to prove the VM path hands the agent a Copilot token, and proving it by
# writing the token to a file would defeat the point.
with open(marker, "w", encoding="utf-8") as stream:
    stream.write("acknowledged\n")
    stream.write(
        "provider_secret:%s\n"
        % ("present" if payload.get("provider_secret") else "absent")
    )
    stream.write(
        "github_token:%s\n"
        % ("present" if payload.get("github_token") else "absent")
    )
