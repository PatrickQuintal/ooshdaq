# SQLite Backup and Offline Restore

This is the recovery runbook for valuable $OOSHDAQ data. Restoration is offline: stop the serving process so only one maintenance process mounts the storage.

## Safety Invariants

- Keep `/data` and `/backups` on separate durable volumes.
- Never copy only a live `.sqlite` file; its WAL and shared-memory files may contain committed state.
- Never start a restore while a serving process or container has the database open.
- Restore only from an immutable, verified backup selected by exact filename.
- Retain the replaced live database until the restored service is verified.
- Keep traffic closed after any migration, integrity, or restore failure.
- Use dedicated data and backup directories, not a shared working directory.

On systems with Unix file modes, the application creates missing data and
backup directories as `0700`. It does not change an existing operator-managed
directory. The live database, WAL, shared-memory files, completed backups,
restore staging files, and retained pre-restore database are `0600`; startup or
maintenance fails if an artifact cannot be made both private and usable.

## Runtime Configuration

| Setting | Container value | Purpose |
| --- | --- | --- |
| `OOSHDAQ_DATA_PATH` | `/data/ooshdaq.sqlite` | live SQLite database |
| `OOSHDAQ_BACKUP_DIR` | `/backups` | completed online backups |
| `OOSHDAQ_BACKUP_INTERVAL` | `24h` | in-process backup schedule |
| `OOSHDAQ_BACKUP_RETENTION_COUNT` | `14` | verified backups retained |

The serving process starts one backup immediately and repeats on the configured
interval. It uses SQLite's online backup API, verifies migration history,
foreign keys, and full integrity, records size and SHA-256 digest, syncs the
temporary file, and atomically publishes a completed `ooshdaq-*.sqlite` file.
Retention removes only older files matching the application's completed-backup
pattern. A retryable online-backup failure is retried once after one minute
before the scheduler returns to its configured interval. Cancellation ends the
retry immediately. Backup failure logs contain only the operation, a safe
failure class, attempt number, and retry timing; they do not contain paths or
database contents.

Graceful shutdown stops future scheduled runs and allows an active backup to
drain within the shutdown deadline. No sidecar or separate scheduled process
should open the live database.

## Monitor Backups

Alert on:

- a newest completed backup older than policy permits;
- backup failure or retention failure logs;
- an empty or unexpected backup size or digest;
- temporary files accumulating after failures;
- insufficient backup-volume space for another atomic publication; and
- an unsuccessful periodic restore drill.

The application logs safe completion metadata but exposes no backup-age or
filesystem-capacity metrics. Deployment owners must derive those signals from
logs, completed filenames, and platform volume metrics. Protect and retain
exported backups or snapshots according to local disaster-recovery policy.

## Restore Command

The offline maintenance command is:

```text
/ooshdaq maintenance restore \
  --backup=/backups/VERIFIED_BACKUP.sqlite \
  --database=/data/ooshdaq.sqlite \
  --retain-replaced=/backups/UNIQUE_PRE_RESTORE_COPY.sqlite
```

The command validates that paths are distinct, refuses existing temporary or
retained destinations, opens and verifies the source, restores into an
isolated sibling file, syncs it, preserves the previous live database, and
atomically promotes the verified result. A failure before promotion leaves the
live database unchanged.

Set `OOSHDAQ_AUTH_MODE=maintenance` as a fail-closed guard against accidentally
starting the HTTP server. It is not a serving authentication mode.

## Docker Compose Restore

Choose exact, unique backup and retention filenames, then stop the serving
container:

```sh
docker compose stop ooshdaq
docker compose run --rm --no-deps -e OOSHDAQ_AUTH_MODE=maintenance ooshdaq \
  maintenance restore \
  --backup=/backups/VERIFIED_BACKUP.sqlite \
  --database=/data/ooshdaq.sqlite \
  --retain-replaced=/backups/UNIQUE_PRE_RESTORE_COPY.sqlite
```

Inspect the successful restore output. Only then restart the serving container:

```sh
docker compose up -d ooshdaq
```

Do not use `docker compose down --volumes`; that deletes both the database and
backup volumes.

## Other Runtimes

Use the same offline sequence on any hosting platform:

1. Record the selected backup filename and digest and the application version.
2. Stop every serving instance and confirm nothing has the data storage open.
3. Run the same binary or operator-built image in maintenance mode with
   exclusive access to the data and backup paths.
4. Inspect the successful restore output before restarting one serving
   instance.

Adapt process supervision, storage attachment, and command invocation to the
chosen platform. The application does not require or supply a particular
orchestrator.

## Verification and Failure Handling

Before reopening traffic, verify readiness, expected catalog counts, login,
collection read/write, persistence through one restart, and representative
same-catalog and cross-catalog matching behavior.

If restoration fails, keep the serving process stopped. Preserve logs, source
backup, retained database, and temporary output. Diagnose without modifying the
backup; resume only with verified data and a schema-compatible image.
