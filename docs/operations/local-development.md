# Local Development and Operations

Local mode is designed to demonstrate the complete collection-and-match journey without an identity provider. It uses only
fictional seeded members and must remain bound to the developer machine.

## Prerequisites

- Go 1.26.5 as pinned by `go.mod` and the container builder
- optional Docker with Compose v2
- writable project-local `data/` and `backups/` directories for direct Go runs

## SQLite Runtime Contract

The local and deployed runtime is one Go process using:

- `./data/ooshdaq.sqlite` for the live SQLite database; and
- `./backups/` as a separate destination for in-process online backups.

The composition root, Dockerfile, Compose file, example environment, and Kubernetes manifests all use this SQLite-only
shape.

The clean reset, driver, backup, and storage decisions are fixed by
[ADR 0003](../architecture/adr/0003-clean-reset-modernc-and-bounded-api.md). Operational reset and restore steps are in
the [SQLite backup and restore runbook](sqlite-backup-restore.md).

## Run with Go

The routine entrypoint is:

```sh
make run
```

The equivalent explicit command is:

```sh
mkdir -p data backups
OOSHDAQ_AUTH_MODE=mock \
OOSHDAQ_DATA_PATH=./data/ooshdaq.sqlite \
OOSHDAQ_BACKUP_DIR=./backups \
OOSHDAQ_BACKUP_INTERVAL=24h \
OOSHDAQ_BACKUP_RETENTION_COUNT=14 \
go run ./cmd/ooshdaq
```

Open <http://localhost:8080>. Choose a seeded member on the local login screen,
update the collection, then switch members to exercise a genuine no-request
lead, a fulfilable request, and a request the current member cannot yet fulfil.

## Run with Docker Compose

```sh
docker compose up --build
```

Compose listens on `:8080` inside the container, explicitly opts into that non-loopback mock listener, and publishes it only
as `127.0.0.1:8080` on the host. It runs with a read-only root filesystem, stores SQLite under the `ooshdaq-data` named
volume, and stores in-process online backups under the separate `ooshdaq-backups` named volume.

Stop without deleting data:

```sh
docker compose down
```

Deleting the named volumes permanently removes local collections and backups:

```sh
docker compose down --volumes
```

Use the destructive command only for the authorized pre-release reset or an intentionally disposable local SQLite
database. It is never a backup or restore operation.

## Build Artifacts

The project distributes source rather than official binaries or container
images.

Build a static local binary:

```sh
make build
./bin/ooshdaq
```

Build the hardened `scratch` container under the local-only tag expected by
the Kubernetes templates:

```sh
make docker-build
```

This creates `ooshdaq:local`. Nothing pushes it to a registry. A self-hosting
operator may load that tag directly into a local cluster or retag it into an
operator-controlled registry when a remote cluster requires one.

## Configuration

| Variable | Local example | Purpose |
| --- | --- | --- |
| `OOSHDAQ_ADDR` | `127.0.0.1:8080` | HTTP listen address |
| `OOSHDAQ_AUTH_MODE` | `mock` | Select `mock` or `proxy` identity adapter |
| `OOSHDAQ_DATA_PATH` | `./data/ooshdaq.sqlite` | Live SQLite database file |
| `OOSHDAQ_BACKUP_DIR` | `./backups` | Separate durable online-backup directory |
| `OOSHDAQ_BACKUP_INTERVAL` | `24h` | In-process backup interval; accepted range `1m`–`744h` |
| `OOSHDAQ_BACKUP_RETENTION_COUNT` | `14` | Verified completed backups retained; accepted range 1–365 |
| `OOSHDAQ_SESSION_SECRET` | optional | Persists mock sessions across restarts; minimum 32 bytes |
| `OOSHDAQ_ALLOW_MOCK_AUTH` | `false` | Explicitly permits mock mode on a non-loopback listener |
| `OOSHDAQ_PROXY_USER_ID_HEADER` | `X-Forwarded-User-ID` | Proxy-mode stable subject header |
| `OOSHDAQ_PROXY_USER_NAME_HEADER` | `X-Forwarded-User-Name` | Proxy-mode display label header |
| `OOSHDAQ_COOKIE_SECURE` | `false` in mock, `true` in proxy | Requires cookies to travel over HTTPS when true |

`.env.example` documents values, but the Go process does not implicitly read dotenv files. Export values through the shell
or process supervisor. Never commit a real secret. Proxy mode does not need `OOSHDAQ_SESSION_SECRET`.

Proxy variables may be present in local configuration for documentation; mock mode must ignore identity headers.
`OOSHDAQ_ALLOW_MOCK_AUTH=true` weakens a startup safety check and does not make mock authentication safe for a shared
network. Compose uses it only because the container listener must be non-loopback while the published host port remains
loopback-only.

## Health

```sh
curl --fail http://localhost:8080/health/live
curl --fail http://localhost:8080/health/ready
```

- live means the HTTP process can respond;
- ready means the repository and required versions are usable.

Do not use a successful liveness response as evidence that data is writable.

## Quality Checks

```sh
make check
make test-race
make test-web
make test-browser
make container-smoke
```

The routine gate and broader test layers are described in
[testing strategy](../architecture/testing-strategy.md).

## Operating-Scale Verification

The small fictional local seed proves the browser journey; it is not capacity evidence. Production-readiness tests for the
SQLite target must use synthetic data representing approximately 10,000 eligible identities, at least five catalogs from
multiple brands, sparse collection entries, same-catalog and cross-catalog matches, and a read-heavy workload with fewer
than 100 concurrent members.

Keep those generated fixtures out of the default developer database. They belong in repeatable load and recovery tests
with bounded results and recorded timings.

`make qualify` runs the fixed 60-second release workload inside the documented
one-CPU/256-MiB user scope. It requires `systemd-run`; see the
[qualification report](../implementation/qualification-report.md) for the exact
command and accepted evidence.

## Data Reset, Backup, and Restore

Use the [SQLite backup and restore runbook](sqlite-backup-restore.md). It contains:

- the one authorized destructive reset for nonvaluable pre-release bbolt state;
- the in-process online-backup lifecycle;
- a Compose restore drill with the serving container stopped; and
- the Kubernetes scale-to-zero maintenance procedure.

Never use `docker compose down --volumes` after SQLite data becomes valuable. Never copy a live SQLite main file without
its WAL and assume it is a backup. Brief restart downtime is acceptable; losing committed SQLite collection data is not.

## Troubleshooting

### Address already in use

Set a different listener and browse to the corresponding port:

```sh
OOSHDAQ_ADDR=127.0.0.1:8081 make run
```

### Readiness stays unavailable

Check that the database directory exists and is writable by the process, that only one $OOSHDAQ process opens the file, and
that the file was created by a supported SQLite schema version. Create a SQLite-aware backup before attempting recovery;
do not fall back to a bbolt file.

### Container cannot write the database

The container runs as UID/GID `65532`. Ensure the mounted volume is writable by that identity. Keep the root filesystem
read-only and fix ownership of both the data and backup volumes rather than running the container as root.

### Scheduled backups do not appear

Confirm the `OOSHDAQ_BACKUP_*` values, then inspect safe structured logs, available space on the backup volume, and the
age of the newest atomically published backup. One backup runs immediately at startup; later backups use the configured
interval.

### Mock login appears in a shared environment

Stop the deployment. Set `OOSHDAQ_AUTH_MODE=proxy`, configure the trusted proxy, and verify that the application cannot be
reached around it before allowing traffic.
