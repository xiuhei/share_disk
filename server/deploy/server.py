#!/usr/bin/env python3
"""Initialize the one server config file and run the production Compose stack."""

import json
import os
from pathlib import Path
import secrets
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[2]
CONFIG = ROOT / "conf" / "server.json"
RUNTIME_DIR = ROOT / "conf" / ".runtime"
RUNTIME_CONFIG = RUNTIME_DIR / "server.json"
RUNTIME_PUBLIC_KEY = RUNTIME_DIR / "access_public_key.pem"
RUNTIME_MONITORING_COMPOSE = RUNTIME_DIR / "monitoring.compose.yaml"
COMPOSE_DIR = Path(__file__).resolve().parent


def initialize() -> None:
    if CONFIG.exists():
        raise SystemExit(f"refusing to overwrite existing {CONFIG}")
    private_key = subprocess.run(
        ["openssl", "genpkey", "-algorithm", "ED25519"],
        check=True,
        stdout=subprocess.PIPE,
        text=True,
    ).stdout.strip()
    value = {
        "server": {
            "host": "0.0.0.0",
            "port": 8080,
            "read_timeout": "30s",
            "write_timeout": "30s",
            "idle_timeout": "2m",
            "shutdown_timeout": "30s",
            "migrations_dir": "/app/migrations/postgres",
            "worker_interval": "30s",
        },
        "database": {
            "host": "postgres",
            "port": 5432,
            "name": "share_disk",
            "user": "share_disk",
            "password": secrets.token_hex(32),
            "ssl_mode": "disable",
            "max_open_conns": 25,
            "max_idle_conns": 10,
            "conn_max_lifetime": "5m",
        },
        "logging": {"level": "info", "format": "json", "output": "stdout"},
        "monitoring": {"enabled": False, "port": 8081, "external_access": False},
        "auth": {
            "bootstrap_token": secrets.token_hex(32),
            "access_private_key": private_key,
            "access_token_ttl": "15m",
        },
    }
    CONFIG.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    descriptor = os.open(CONFIG, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, "w", encoding="utf-8") as output:
        json.dump(value, output, ensure_ascii=False, indent=2)
        output.write("\n")
    print(f"created {CONFIG} (mode 0600)")


def load() -> dict:
    if not CONFIG.is_file():
        raise SystemExit(f"missing {CONFIG}; run: {Path(__file__).name} init")
    if CONFIG.stat().st_mode & 0o077:
        raise SystemExit(f"{CONFIG} must use mode 0600")
    with CONFIG.open(encoding="utf-8") as source:
        return json.load(source)


def prepare_runtime() -> dict:
    value = load()
    RUNTIME_DIR.mkdir(mode=0o700, parents=True, exist_ok=True)
    os.chmod(RUNTIME_DIR, 0o700)
    runtime_config_tmp = RUNTIME_DIR / ".server.json.tmp"
    runtime_config_tmp.write_bytes(CONFIG.read_bytes())
    os.chmod(runtime_config_tmp, 0o444)
    os.replace(runtime_config_tmp, RUNTIME_CONFIG)
    result = subprocess.run(
        ["openssl", "pkey", "-pubout"],
        input=value["auth"]["access_private_key"] + "\n",
        check=True,
        stdout=subprocess.PIPE,
        text=True,
    )
    runtime_key_tmp = RUNTIME_DIR / ".access_public_key.pem.tmp"
    runtime_key_tmp.write_text(result.stdout, encoding="utf-8")
    os.chmod(runtime_key_tmp, 0o444)
    os.replace(runtime_key_tmp, RUNTIME_PUBLIC_KEY)
    monitoring = value.get("monitoring", {})
    monitoring_compose_tmp = RUNTIME_DIR / ".monitoring.compose.yaml.tmp"
    if monitoring.get("enabled", False):
        port = int(monitoring.get("port", 8081))
        if port < 1 or port > 65535:
            raise SystemExit(f"invalid monitoring.port: {port}")
        host = "0.0.0.0" if monitoring.get("external_access", False) else "127.0.0.1"
        monitoring_compose_tmp.write_text(
            "services:\n"
            "  control-server:\n"
            "    ports:\n"
            f'      - "{host}:{port}:{port}"\n',
            encoding="utf-8",
        )
    else:
        monitoring_compose_tmp.write_text("services: {}\n", encoding="utf-8")
    os.chmod(monitoring_compose_tmp, 0o444)
    os.replace(monitoring_compose_tmp, RUNTIME_MONITORING_COMPOSE)
    return value


def public_key(output: str) -> None:
    value = load()
    result = subprocess.run(
        ["openssl", "pkey", "-pubout"],
        input=value["auth"]["access_private_key"] + "\n",
        check=True,
        stdout=subprocess.PIPE,
        text=True,
    )
    target = Path(output)
    target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    target.write_text(result.stdout, encoding="utf-8")
    os.chmod(target, 0o600)
    print(target)


def rotate_database_password() -> None:
    value = load()
    value["database"]["password"] = secrets.token_hex(32)
    temporary = CONFIG.with_suffix(".json.tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    os.chmod(temporary, 0o600)
    os.replace(temporary, CONFIG)
    print("rotated database password in conf/server.json; use only before first database startup")


def set_monitoring(raw: str, port_raw: str = None, access: str = None) -> None:
    if raw not in {"true", "false"}:
        raise SystemExit("usage: server.py monitoring true|false [port] [local|external]")
    value = load()
    current = value.get("monitoring", {})
    port_raw = port_raw or str(current.get("port", 8081))
    access = access or ("external" if current.get("external_access", False) else "local")
    try:
        port = int(port_raw)
    except ValueError as error:
        raise SystemExit(f"invalid monitoring port: {port_raw}") from error
    if port < 1 or port > 65535 or access not in {"local", "external"}:
        raise SystemExit("usage: server.py monitoring true|false [port] [local|external]")
    value["monitoring"] = {
        "enabled": raw == "true",
        "port": port,
        "external_access": access == "external",
    }
    temporary = CONFIG.with_suffix(".json.tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    os.chmod(temporary, 0o600)
    os.replace(temporary, CONFIG)
    print(f"monitoring enabled={raw} port={port} access={access} in conf/server.json; recreate control-server to apply")


def compose(arguments: list[str]) -> None:
    value = prepare_runtime()
    database = value["database"]
    server = value["server"]
    environment = os.environ.copy()
    environment.update({
        "POSTGRES_DB": database["name"],
        "POSTGRES_USER": database["user"],
        "POSTGRES_PASSWORD": database["password"],
        "CONTROL_SERVER_BIND_ADDRESS": server["host"],
        "CONTROL_SERVER_PORT": str(server["port"]),
    })
    command = [
        "docker", "compose", "--env-file", "/dev/null",
        "-f", "compose.yaml",
        "-f", str(RUNTIME_MONITORING_COMPOSE),
        *arguments,
    ]
    raise SystemExit(subprocess.run(command, cwd=COMPOSE_DIR, env=environment).returncode)


def main() -> None:
    if len(sys.argv) < 2:
        raise SystemExit("usage: server.py init | public-key <output> | monitoring true|false [port] [local|external] | rotate-uninitialized-db-password | <docker compose arguments...>")
    if sys.argv[1] == "init":
        initialize()
        return
    if sys.argv[1] == "public-key":
        if len(sys.argv) != 3:
            raise SystemExit("usage: server.py public-key <output>")
        public_key(sys.argv[2])
        return
    if sys.argv[1] == "rotate-uninitialized-db-password":
        rotate_database_password()
        return
    if sys.argv[1] == "monitoring":
        if len(sys.argv) < 3 or len(sys.argv) > 5:
            raise SystemExit("usage: server.py monitoring true|false [port] [local|external]")
        set_monitoring(sys.argv[2], *(sys.argv[3:]))
        return
    compose(sys.argv[1:])


if __name__ == "__main__":
    main()
