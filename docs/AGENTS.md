# Repository Guidelines

## Project Structure & Module Organization

This checkout contains the design baseline plus the LAN V1 Go and Android implementation for the personal distributed cloud drive. Read `README.md` first; it defines document precedence and links the numbered specifications. Keep requirements in `01_需求基线与验收追踪.md`, architecture decisions in `02_架构设计方案.md`, coding rules in `03_代码规范.md`, implementation contracts in `04_详细设计与实现方案.md`, milestone gates in `05_开发阶段性目标.md`, and user-facing deployment guidance in `06_局域网部署与验收.md`. Executables live under `cmd/`, capability-oriented packages under `internal/`, contracts in `api/openapi/` and `proto/sharedisk/v1/`, and migrations under `migrations/`. The M2-D gate requires server container assets under `deploy/docker/` and `deploy/compose/`. Keep temporary test output and process notes outside Git; do not treat a build, a container health check, or a document claim as completion evidence.

## Build, Test, and Development Commands

The repository Makefile provides the baseline commands documented in `03_代码规范.md`:

- `make fmt`: verify `gofmt` and `goimports` output.
- `make lint`: run `go vet` and `staticcheck`.
- `make test` / `make test-race`: run unit tests and race-sensitive core tests.
- `make test-int`: exercise real PostgreSQL, SQLite, and process integrations.
- `make contract`: validate OpenAPI, Proto, and migration consistency.
- `make check`: run the complete CI gate before review.

The Docker targets `make image`, `make compose-config`, `make container-test`, and `make image-scan` exist but the container E2E/scan gates have not yet been run end-to-end in CI. Do not report planned or unexecuted Docker commands as executed deployment evidence.

## Coding Style & Naming Conventions

Use standard Go formatting and lowercase, capability-based package names; avoid catch-all packages such as `utils` or `common`. Preserve initialisms (`ID`, `URL`, `HTTP`), use `snake_case` for JSON fields, RFC 3339 UTC timestamps, and versioned forward-only migrations. Keep I/O APIs context-aware, concurrency bounded, and generated files paired with their generator command. Number new design documents consistently and update the README index.

Server deliverables must use multi-stage OCI image builds, separate single-process targets, non-root and read-only runtime defaults, stdout/stderr logs, external configuration and secret files, bounded probes, explicit one-shot migrations, and graceful `SIGTERM` handling. Docker Compose is the V1 single-host production and E2E baseline; do not introduce Kubernetes or microservices without new evidence.

## Testing Guidelines

Use table-driven Go tests named `TestXxx`; assert business state, persistence, error paths, idempotency, and illegal state transitions rather than mock calls. Integration and E2E tests must use isolated databases, object directories, processes, and real sockets where required. Run race tests for concurrent state machines and avoid timing assertions based on `time.Sleep`.

## Commit & Pull Request Guidelines

Git history is unavailable in this checkout, so no repository-specific commit convention can be inferred. Prefer short, imperative, scoped subjects such as `docs: clarify transfer ticket lifecycle`. Pull requests should link the requirement or defect, explain observable behavior and security/concurrency impact, list commands actually run, and identify unverified platform or network checks. Update affected contracts, migrations, tests, and documentation together; never commit credentials, editor lock files, caches, or unsupported performance claims.
