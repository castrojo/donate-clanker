#!/usr/bin/env python3
"""Test-only guest consumer for the host bootstrap socket."""

import json
import os
import socket
import sys
import time


def fail(message):
    raise SystemExit(message)


socket_path = os.environ.get("DONATE_CLANKER_BOOTSTRAP_SOCKET")
marker = os.environ.get("DONATE_CLANKER_TEST_CONSUMED")
if not socket_path or not marker:
    fail("bootstrap consumer requires socket and marker paths")

client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
client.connect(socket_path)
time.sleep(float(os.environ.get("DONATE_CLANKER_TEST_CONSUMER_DELAY", "0")))
payload = json.loads(client.recv(65536).splitlines()[0])

required = {"version", "hive_endpoint", "registration_token", "backend", "run_id"}
optional = {"goose_provider", "goose_model", "provider_secret"}
if not required <= set(payload) or not set(payload) <= (required | optional):
    fail("invalid bootstrap envelope fields")
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
client.sendall((json.dumps(ack, separators=(",", ":")) + "\n").encode())
client.close()
with open(marker, "w", encoding="utf-8") as stream:
    stream.write("acknowledged\n")
