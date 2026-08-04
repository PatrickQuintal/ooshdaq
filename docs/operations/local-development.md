# Local Development

Local mode uses fictional seeded members and mock authentication. Keep it on a
developer-controlled loopback interface, never a shared deployment.

## Prerequisites

- the Go toolchain declared by `go.mod`;
- optional Docker with Compose v2; and
- writable `data/` and `backups/` directories for direct Go runs.

## Run with Go

```sh
make run
```

Open <http://localhost:8080>, choose a seeded member, update their collection,
and switch members to inspect matching behavior. The live database is
`./data/ooshdaq.sqlite`; verified online backups are written to `./backups`.
Local runs import only published catalogs by default.

To use another loopback port:

```sh
OOSHDAQ_ADDR=127.0.0.1:8081 make run
```

## Run with Docker Compose

```sh
docker compose up --build
```

Compose publishes only `127.0.0.1:8080`, runs as UID/GID `65532`, uses a
read-only root filesystem, and stores data and backups in separate named
volumes.

Stop the service without deleting its data:

```sh
docker compose down
```

The following command permanently deletes both local volumes:

```sh
docker compose down --volumes
```

Use it only for an intentionally disposable local environment. It is never a
backup or restore procedure.

Build a binary with `make build` and run `./bin/ooshdaq`. `make docker-build`
creates the local tag `ooshdaq:local`; it does not publish an image.

## Configuration

| Variable | Meaning |
| --- | --- |
| `OOSHDAQ_ADDR` | HTTP listener; local default is `127.0.0.1:8080` |
| `OOSHDAQ_AUTH_MODE` | `mock` locally or `proxy` behind a trusted proxy |
| `OOSHDAQ_DATA_PATH` | SQLite database path |
| `OOSHDAQ_DEMO_CATALOGS` | loads fictional catalogs for an explicit local demo |
| `OOSHDAQ_BACKUP_DIR` | separate online-backup directory |
| `OOSHDAQ_BACKUP_INTERVAL` | backup interval, from `1m` to `744h` |
| `OOSHDAQ_BACKUP_RETENTION_COUNT` | completed backups retained, from 1 to 365 |
| `OOSHDAQ_SESSION_SECRET` | optional mock-session key of at least 32 bytes |
| `OOSHDAQ_ALLOW_MOCK_AUTH` | permits mock mode on a non-loopback listener |
| `OOSHDAQ_PROXY_USER_ID_HEADER` | trusted proxy's stable-subject header |
| `OOSHDAQ_PROXY_USER_NAME_HEADER` | trusted proxy's display-name header |
| `OOSHDAQ_COOKIE_SECURE` | requires HTTPS transport for the session cookie |

[`.env.example`](../../.env.example) contains development values. The Go
process does not load dotenv files automatically; export variables through the
shell or supervisor. Never commit a real session secret.

`OOSHDAQ_ALLOW_MOCK_AUTH=true` removes only a startup safeguard. It does not
make mock authentication safe on a shared network. Proxy mode ignores the mock
login flow and requires an independently secured proxy boundary.

`OOSHDAQ_DEMO_CATALOGS=true` adds the manifests under
`internal/catalog/testdata/manifests`. Leave it unset for an ordinary
self-hosted instance. Catalog imports are additive: turning the option off does
not remove catalogs already imported. Use a separate disposable data path for
demos, for example:

```sh
OOSHDAQ_DEMO_CATALOGS=true \
OOSHDAQ_DATA_PATH=./data/ooshdaq-demo.sqlite \
OOSHDAQ_BACKUP_DIR=./backups/demo \
go run ./cmd/ooshdaq
```

The option does not generate members or collection state.

## Health and Checks

```sh
curl --fail http://localhost:8080/health/live
curl --fail http://localhost:8080/health/ready
make check
make test-race
make test-web
make test-browser
make container-smoke
make qualify
```

Liveness means the HTTP process responds. Readiness also checks repository
usability; it does not prove a future write will succeed. Browser tests require
the packages in `web/package-lock.json`; `make qualify` requires user-level
`systemd-run` support. See the
[testing strategy](../architecture/testing-strategy.md) for test scope.

## Data Safety

Do not copy the live SQLite main file while the process is running; its WAL and
shared-memory files are part of the live database. Use the
[backup and restore runbook](sqlite-backup-restore.md) for recovery. Brief
restart downtime is expected, but committed collection data must persist.
Use dedicated data and backup directories. On Unix the process creates missing
directories as owner-only and keeps every database artifact owner-only; it does
not change an existing directory's operator-managed permissions.

## Troubleshooting

If readiness remains unavailable, verify both directories are writable, only
one process opens the database, and the schema is supported. If a container
cannot write, fix ownership of both volumes for UID/GID `65532`; do not run the
container as root.

If scheduled backups do not appear, verify the backup settings, free space,
directory ownership, and structured failure logs. One backup runs at startup;
a transient failure is retried once before the configured interval resumes.

If mock login appears in a shared environment, stop traffic, switch to
`OOSHDAQ_AUTH_MODE=proxy`, and verify the application cannot be reached around
the trusted proxy before reopening access.
