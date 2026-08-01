#!/usr/bin/env python3
"""Project projectbluefin skill docs into Goose's native Agent Skills layout.

The org keeps skills as ``docs/skills/<id>.md`` plus a generated
``docs/skills/index.json`` manifest. Goose discovers skills only as directories
containing a ``SKILL.md``, under fixed roots such as ``~/.agents/skills``.

This script reads the manifest and writes ``<out>/<id>/SKILL.md`` for each
active skill. It is a projection, not a migration: ``docs/skills`` stays the
source of truth and no org repository changes.

Only ``name``, ``description`` and a nested ``metadata`` block are emitted.
The factory frontmatter carries a further ten top-level keys that Goose has no
use for; regenerating rather than copying keeps them away from Goose's parser
entirely.

Goose loads only ``name`` and ``description`` into the system prompt at session
start, then fetches a body on demand via ``load_skill`` -- so the description is
what actually drives selection, and it is copied verbatim.
"""

from __future__ import annotations

import argparse
import json
import pathlib
import sys
import urllib.error
import urllib.request

DEFAULT_INDEX = (
    "https://raw.githubusercontent.com/projectbluefin/common/main/docs/skills/index.json"
)
DEFAULT_RAW_BASE = "https://raw.githubusercontent.com/projectbluefin/common/main/"

# Goose's own guidance is that fewer tools and shorter always-on context perform
# better; every skill costs name+description tokens in every request.
MAX_DESCRIPTION = 256


def read_source(location: str) -> str:
    if location.startswith(("http://", "https://")):
        with urllib.request.urlopen(location, timeout=30) as response:  # noqa: S310
            return response.read().decode("utf-8")
    return pathlib.Path(location).read_text(encoding="utf-8")


def yaml_quote(value: str) -> str:
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"') + '"'


def render(skill: dict, body: str) -> str:
    name = skill["name"]
    description = " ".join(str(skill.get("description", "")).split())
    if len(description) > MAX_DESCRIPTION:
        description = description[: MAX_DESCRIPTION - 1].rstrip() + "\u2026"

    tags = [str(tag) for tag in skill.get("tags", [])]
    lines = [
        "---",
        f"name: {name}",
        f"description: {yaml_quote(description)}",
        "metadata:",
        f"  source: {skill.get('entry_point', '')}",
        f"  category: {skill.get('category', '')}",
        f"  version: {yaml_quote(str(skill.get('version', '')))}",
    ]
    if tags:
        lines.append("  tags: [" + ", ".join(tags) + "]")
    lines += ["---", "", body.rstrip(), ""]
    return "\n".join(lines)


def strip_frontmatter(text: str) -> str:
    """Remove the factory YAML frontmatter, keeping the prose body."""
    if not text.startswith("---"):
        return text
    end = text.find("\n---", 3)
    if end == -1:
        return text
    newline = text.find("\n", end + 1)
    return text[newline + 1 :] if newline != -1 else ""


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--index", default=DEFAULT_INDEX, help="index.json path or URL")
    parser.add_argument(
        "--raw-base",
        default=DEFAULT_RAW_BASE,
        help="base path or URL that entry_point values are relative to",
    )
    parser.add_argument("--out", required=True, help="output skills root")
    parser.add_argument(
        "--category",
        action="append",
        default=[],
        help="only emit skills in this category (repeatable; default all)",
    )
    parser.add_argument(
        "--allow-empty",
        action="store_true",
        help="succeed even when no skills are emitted",
    )
    args = parser.parse_args()

    try:
        manifest = json.loads(read_source(args.index))
    except (OSError, urllib.error.URLError, json.JSONDecodeError) as err:
        print(f"generate-skills: cannot read index {args.index}: {err}", file=sys.stderr)
        return 1

    out_root = pathlib.Path(args.out)
    out_root.mkdir(parents=True, exist_ok=True)

    emitted = 0
    skipped = []
    for skill in manifest.get("skills", []):
        skill_id = skill.get("id") or skill.get("name")
        if not skill_id:
            continue
        if skill.get("status", "active") != "active":
            skipped.append((skill_id, "status is not active"))
            continue
        if args.category and skill.get("category") not in args.category:
            skipped.append((skill_id, "category filtered out"))
            continue

        entry_point = skill.get("entry_point")
        if not entry_point:
            skipped.append((skill_id, "no entry_point"))
            continue

        location = (
            args.raw_base + entry_point
            if args.raw_base.startswith(("http://", "https://"))
            else str(pathlib.Path(args.raw_base) / entry_point)
        )
        try:
            body = strip_frontmatter(read_source(location))
        except (OSError, urllib.error.URLError) as err:
            # A skill listed in the manifest but missing on disk is a factory
            # bug, not a reason to ship an image with no skills at all.
            skipped.append((skill_id, f"unreadable: {err}"))
            continue

        target = out_root / skill_id
        target.mkdir(parents=True, exist_ok=True)
        (target / "SKILL.md").write_text(render(skill, body), encoding="utf-8")
        emitted += 1

    for skill_id, reason in skipped:
        print(f"generate-skills: skipped {skill_id}: {reason}", file=sys.stderr)
    print(f"generate-skills: wrote {emitted} skills to {out_root}", file=sys.stderr)

    if emitted == 0 and not args.allow_empty:
        print("generate-skills: no skills emitted", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
