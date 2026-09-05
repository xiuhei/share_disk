# Ubuntu client

The Ubuntu client consists of the host-installed Agent and CLI. It owns local
SQLite metadata, verified file bytes, LAN discovery/API behavior and the
loopback management page. It is delivered as a `.deb` and is not part of the
Docker Compose server stack.

From the repository root:

```bash
make deb
```

The package is written to `release/ubuntu/`; Debian packaging source remains
under `packaging/deb/` and package assembly uses the ignored `build/` tree.
