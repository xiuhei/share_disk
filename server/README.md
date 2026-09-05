# Server

`server/` owns the Docker-deployed control plane. Executables live under
`cmd/`, private implementation under `internal/`, PostgreSQL schema changes
under `migrations/postgres/`, and deployment assets under `deploy/`.

Run the stack from the repository root:

```bash
./server/deploy/server.py init
./server/deploy/server.py up -d --build --wait
```

All server processes read the ignored `conf/server.json`. Ubuntu Agent source,
SQLite data and file bytes do not belong to this deployment boundary.
