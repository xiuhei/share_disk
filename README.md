# Share Disk - Personal Distributed Cloud Drive

A personal distributed cloud drive system that keeps file contents on user devices while providing a unified logical directory and cloud-drive-like browsing experience.

## Project Status

**Status: E0 engineering foundation in progress; the existing LAN V1 path remains a release candidate pending Android hardware acceptance.**

The repository contains one deliberately narrow vertical slice: a Docker Compose identity control plane, an Ubuntu Agent that persists verified object bytes and LAN metadata, and an Android 10+ APK that uploads through tus and downloads through HTTP Range using the system file picker. The broader distributed-drive roadmap is outside the current release scope.

LAN V1 evidence and remaining release boundary:

- DNS-SD/mDNS discovery provides only a candidate address; Android must still authenticate with the Control Server token.
- A target Android device is not attached to this checkout, so physical Wi-Fi, SAF-provider and process/network-interruption acceptance remains unverified.
- LAN V1 supports one account, one Ubuntu storage Agent and its first Android device. Additional device registration, global file metadata sync, public networking, Relay, Windows and multi-source transfer remain frozen.
- The shared desktop Agent and a two-node libp2p transfer prototype now exist, but control-plane ticket integration, Relay/public-network validation and the worker remain partial and must not be presented as completed product features.

## Quick Start

### Prerequisites

- Product deployment: Docker Engine, Docker Compose plugin and OpenSSL on Ubuntu 22.04/24.04. PostgreSQL runs in the server stack; the host-installed Ubuntu Agent owns its embedded SQLite database.
- Source development: Go 1.26.8+ and Node.js 24.18.x/npm 11.x; Android builds additionally require JDK 17 and Android SDK 35.
- Android 10+ device on the same trusted LAN; ADB is optional for side-loading.

### Build

```bash
make build          # build all Go binaries into build/bin/
make deb            # write the Ubuntu package under release/ubuntu/
make apk            # write a signed APK under release/android/
```

### Run the LAN product

```bash
./server/deploy/server.py init              # 首次生成 conf/server.json（Git 忽略，0600）
$EDITOR conf/server.json                    # 按需修改端口、数据库、日志和认证配置
./server/deploy/server.py up -d --build --wait
```

服务器只读取 `conf/server.json`；Compose 启动器仅把同一文件安全挂载给各服务器进程。不要提交该文件，也不要把明文 LAN 测试端口暴露到公网。Ubuntu Agent 客户端仍使用自身的 `/etc/share-disk/agent.env`。

监控台不是服务器基本依赖，默认关闭。执行 `./server/deploy/server.py monitoring true 8081 local` 可仅在服务器本机开放，或将 `local` 改为 `external` 允许外部访问；端口和访问范围都写入 `conf/server.json`。重建 Control Server 后打开 `http://<服务器地址>:8081/admin`。关闭时监控监听器不启动，文件同步等核心服务不受影响。

### Production-source maintenance

```bash
make fmt            # gofmt/goimports (write)
make fmt-check      # verify formatting
make lint           # go vet + staticcheck
make contract       # protobuf generation consistency + OpenAPI contract
make vuln           # govulncheck
make check          # production-source gate
```

### Configuration

服务器配置模板为 [`conf/server.example.json`](conf/server.example.json)，实际配置固定为被 Git 忽略的 `conf/server.json`，涵盖 HTTP、PostgreSQL、日志、迁移、Worker、初始化令牌和签名私钥。服务器不再从 systemd 或 Compose 环境变量读取业务配置。桌面 Agent 继续使用下列 `SHARE_DISK_*` 配置：

| Environment Variable | Description | Required for |
|---------------------|-------------|-------------|
| `SHARE_DISK_ACCESS_PUBLIC_KEY` / `_FILE` | Ed25519 access-token verification key (PKIX PEM) | agent |
| `SHARE_DISK_SQLITE_PATH` | SQLite database path | agent |
| `SHARE_DISK_STORAGE_ROOT` | Agent object storage root | agent / CLI |
| `SHARE_DISK_LAN_ENABLED` / `_HOST` / `_PORT` | Explicit LAN data API | agent |
| `SHARE_DISK_LAN_MAX_UPLOAD_SIZE` | Per-upload byte limit | agent |
| `SHARE_DISK_LAN_DISCOVERY_ENABLED` | Publish the enabled Agent with DNS-SD/mDNS | agent |
| `SHARE_DISK_CONTROL_URL` | Pinned control-plane origin for Agent provisioning and outbox delivery | agent |
| `SHARE_DISK_LAN_ADVERTISE_URL` | Client-reachable Agent origin stored with verified replicas | agent |
| `SHARE_DISK_SYNC_INTERVAL` | Base interval for durable catalog delivery and retry | agent |
| `SHARE_DISK_TRASH_RETENTION` | Recoverable LAN trash retention as a Go duration (default `168h`) | agent |
| `SHARE_DISK_DESKTOP_UI_ENABLED` / `_HOST` / `_PORT` | Ubuntu loopback browser client (DEB default `127.0.0.1:9191`) | agent |
| `SHARE_DISK_LOG_LEVEL` / `_FORMAT` / `_OUTPUT` | Logging | all |

### API Endpoints

- `GET /livez` - liveness probe
- `GET /readyz` - readiness probe (database + schema)
- `GET /version` - version/build information
- `POST /v1/auth/bootstrap` - initialize the first account (one-time)
- `POST /v1/auth/login` - login and bind a device
- `POST /v1/auth/refresh` - rotate refresh token
- `POST /v1/auth/logout` - revoke the current session (requires auth)
- `GET /v1/account` / `PATCH /v1/account/password` - account profile and secure password change
- `GET /v1/auth/jwks` - access-token verification public key
- `POST/HEAD/PATCH /v1/lan/uploads/*` - authenticated tus uploads (Agent)
- `GET /v1/lan/files` - authenticated local file list (Agent)
- `PATCH/DELETE /v1/lan/files/{id}` - rename or move a logical file to trash (Agent)
- `GET/HEAD /v1/lan/files/{id}/content` - verified Range download (Agent)
- `GET /v1/lan/trash` - list recoverable files (Agent)
- `POST /v1/lan/trash/{id}/restore` / `DELETE /v1/lan/trash/{id}` - restore or permanently delete (Agent)

See `contracts/openapi/openapi.yaml` for the control API contract. The remaining endpoints in the design (`devices`, `folders`, `files`, `transfers`, `sync`, `trash`, `replicas`, `events`, `metrics`) are specified but not yet implemented.

## Architecture

The system uses a "modular control service + per-device agent + libp2p data plane + PostgreSQL/SQLite dual metadata layer" architecture:

- **Control Server**: Stateless HTTP/WebSocket instances (stateless; partial)
- **Control Worker**: Task scanning, outbox, redundancy, and GC (skeleton)
- **Device Agent**: Shared Go process managing SQLite, verified objects, local IPC and the opt-in authenticated LAN HTTP data path, hosted by Ubuntu and Windows launchers
- **PostgreSQL**: Authoritative source for accounts, logical directory, devices, tasks, and events
- **SQLite**: Local authority for agent state, chunk progress, and pending operations

### Source ownership

- [`server/`](server/) contains the Docker control plane, PostgreSQL migrations and server deployment assets.
- [`client/android/`](client/android/) contains the complete Android Gradle project.
- [`client/agent/`](client/agent/) contains the cross-platform Agent core, CLI and SQLite migrations.
- [`client/ubuntu/`](client/ubuntu/) contains the Ubuntu launcher and Debian packaging source.
- [`client/windows/`](client/windows/) contains the Windows launcher and packaging source.
- [`contracts/`](contracts/) contains cross-process OpenAPI and Protobuf contracts.
- [`internal/`](internal/) contains Go infrastructure shared by the server and clients; product-private packages stay below their owning product.
- [`release/`](release/) is the ignored output location for signed/versioned `.apk`, `.deb` and future Windows installers. Temporary files belong in `build/`.

## Contracts and client guide

- Control API: [`contracts/openapi/openapi.yaml`](contracts/openapi/openapi.yaml)
- Local IPC and transfer protocol: [`contracts/proto/sharedisk/v1/`](contracts/proto/sharedisk/v1/)
- Android build and installation guide: [`client/android/README.md`](client/android/README.md)
- Ubuntu Agent source and packaging guide: [`client/ubuntu/README.md`](client/ubuntu/README.md)

## License

MIT
