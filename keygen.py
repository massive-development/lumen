#!/usr/bin/env python3
"""
Generate cryptographically secure secrets for the BitNet stack.

    python keygen.py           # print new values for all secrets
    python keygen.py --write   # fill change-me placeholders in .env in place
"""

import re
import secrets
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Literal

KeyType = Literal["password", "secret", "api_key", "public_key", "private_key"]

_ENTROPY_BYTES = 32  # 256-bit — used for keys and secrets

_SHORTHANDS: dict[KeyType, str] = {
    "secret":      "sk",
    "api_key":     "ak",
    "public_key":  "pk",
    "private_key": "sk",
}


@dataclass
class EnvVar:
    name: str
    key_type: KeyType


@dataclass
class Service:
    name: str
    prefix: str
    vars: list[EnvVar]

    def generate(self, v: EnvVar) -> str:
        if v.key_type == "password":
            return secrets.token_urlsafe(16)
        shorthand = _SHORTHANDS[v.key_type]
        return f"mdat_{self.prefix}_{shorthand}_{secrets.token_hex(_ENTROPY_BYTES)}"


SERVICES: list[Service] = [
    Service("postgres", "pg", [
        EnvVar("POSTGRES_PASSWORD", "password"),
    ]),
    Service("searxng", "sg", [
        EnvVar("SEARXNG_SECRET", "secret"),
    ]),
    Service("pipelines", "pl", [
        EnvVar("PIPELINES_API_KEY", "api_key"),
    ]),
    Service("openwebui", "ow", [
        EnvVar("WEBUI_SECRET_KEY", "secret"),
    ]),
    Service("langfuse", "lf", [
        EnvVar("LANGFUSE_NEXTAUTH_SECRET", "secret"),   # NextAuth JWT signing
        EnvVar("LANGFUSE_SALT", "secret"),              # password hashing — must differ from NEXTAUTH_SECRET
        EnvVar("LANGFUSE_PUBLIC_KEY", "public_key"),
        EnvVar("LANGFUSE_SECRET_KEY", "private_key"),
        EnvVar("LANGFUSE_ADMIN_PASSWORD", "password"),
    ]),
    Service("open-terminal", "ot", [
        EnvVar("OPEN_TERMINAL_API_KEY", "api_key"),
    ]),
]


def _items() -> list[tuple[Service, EnvVar]]:
    return [(svc, v) for svc in SERVICES for v in svc.vars]


def cmd_print() -> None:
    for svc in SERVICES:
        print(f"\n# {svc.name}")
        for v in svc.vars:
            print(f"{v.name}={svc.generate(v)}")


def cmd_write(env_path: Path) -> None:
    if not env_path.exists():
        sys.exit(f"error: {env_path} not found")

    text = env_path.read_text()
    updated: list[str] = []
    skipped: list[str] = []

    for svc, v in _items():
        pat = re.compile(rf"^({re.escape(v.name)}=)(.*)$", re.MULTILINE)
        m = pat.search(text)
        if not m:
            skipped.append(f"  {v.name}: not present in file")
            continue
        if "change-me" not in m.group(2):
            skipped.append(f"  {v.name}: already set, skipped")
            continue
        text = pat.sub(rf"\g<1>{svc.generate(v)}", text)
        updated.append(f"  {v.name}")

    env_path.write_text(text)

    if updated:
        print(f"Set {len(updated)} secret(s) in {env_path}:")
        print("\n".join(updated))
    if skipped:
        print(f"\nSkipped {len(skipped)}:")
        print("\n".join(skipped))


def main() -> None:
    if "--write" in sys.argv:
        cmd_write(Path(__file__).parent / ".env")
    else:
        cmd_print()


if __name__ == "__main__":
    main()
