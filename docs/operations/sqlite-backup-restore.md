# SQLite Backup, Clean Reset, and Offline Restore

This runbook implements the storage decisions in
[ADR 0003](../architecture/adr/0003-clean-reset-modernc-and-bounded-api.md). It distinguishes the one authorized
pre-release bbolt reset from recovery of valuable SQLite data.

## Runtime Contract

The runtime uses:

| Setting | Container value | Purpose |
| --- | --- | --- |
| `OOSHDAQ_DATA_PATH` | `/data/ooshdaq.sqlite` | Live SQLite database; WAL and shared-memory files stay beside it |
| `OOSHDAQ_BACKUP_DIR` | `/backups` | Separate durable destination for completed online backups |
| `OOSHDAQ_BACKUP_INTERVAL` | `24h` | In-process backup schedule |
| `OOSHDAQ_BACKUP_RETENTION_COUNT` | `14` | Number of verified completed backups retained |

The SQLite repository provides online backup, full verification, hashing, isolated restore, file sync, and atomic
publication. The composition root starts one immediate backup, repeats on the
configured interval, retains only exact completed backup names, and parses
`maintenance restore` before serving configuration. Graceful shutdown prevents
another scheduled run and drains an active backup instead of canceling it; the
shutdown deadline remains the final bound.

The serving process:

1. create a uniquely named temporary file on the `/backups` filesystem;
2. use SQLite's online backup API against the open database;
3. verify schema-migration history, foreign keys, and full integrity;
4. calculate and log the byte size and SHA-256 digest;
5. sync the file, atomically publish a name such as `ooshdaq-20260730T120000.000000000Z.sqlite`, and sync the directory;
6. remove only older completed files matching the application's exact backup-name pattern; and
7. emit safe structured completion, digest, duration, and failure logs without
   logging collection data.

The process does not expose a metrics endpoint for backup age or filesystem
free space. The deployment owner must derive last-success age from completion
logs or completed filenames, monitor backup-PVC capacity through the platform,
and alert on the failure and retention log events.

There is no backup sidecar, CronJob, second pod, or raw live-file copy. The one serving Go process mounts both claims and is
the only process which opens the live database.

## Authorized Clean Reset

The following destructive reset is authorized only while the current bbolt data remains explicitly nonvaluable. It is not
a general SQLite recovery procedure and is not a rollback path.

### Local and Compose

Stop the current process before removing the disposable bbolt file:

```sh
rm -f data/ooshdaq.db
```

For Compose, remove the old named volumes before the SQLite-only first start:

```sh
docker compose down --volumes
docker compose up --build
```

`down --volumes` deletes both database and backup volumes. Never use it after SQLite contains valuable collection data.

### Kubernetes

Changing the existing claim from `ReadWriteOnce` to `ReadWriteOncePod` and selecting `standard-rwo` requires claim
replacement. Confirm the namespace and claim identity before running these destructive commands:

```sh
kubectl scale deployment/ooshdaq --replicas=0
kubectl wait --for=delete pod -l app.kubernetes.io/name=ooshdaq --timeout=5m
kubectl get pvc ooshdaq-data
kubectl delete pvc ooshdaq-data
```

Review the bound persistent volume and StorageClass reclaim policy if evidence of deletion is required. The old bbolt
volume is not backed up or migrated. Build `ooshdaq:local` from the selected
source revision and make it available to the cluster, or override the image in
an environment-specific overlay with an operator-built artifact. Then create
the new claims and SQLite-only pod:

```sh
kubectl apply -k deploy/kubernetes/base
kubectl wait --for=jsonpath='{.status.phase}'=Bound pvc/ooshdaq-data --timeout=5m
kubectl wait --for=jsonpath='{.status.phase}'=Bound pvc/ooshdaq-backups --timeout=5m
kubectl rollout status deployment/ooshdaq --timeout=10m
```

Do not reopen traffic until readiness, catalog counts, one collection update,
restart persistence, a genuine no-request lead, a fulfilable request, and an
unfulfillable request shown distinctly all pass. Include a cross-catalog
fulfilable request in that smoke test. A failure keeps traffic closed and is
repaired forward; it does not restore bbolt.

## Online Backup Operations

The database PVC and backup PVC both use `ReadWriteOncePod` and are mounted by the serving pod at `/data` and `/backups`.
This enforces the single-pod rule, but it does not protect against project-wide deletion or credential compromise. Apply
deployment-owner snapshot, export, encryption, and retention policy in addition to the in-process backup schedule.

Check at least:

- the newest completed backup age is within policy;
- its recorded size and SHA-256 digest are non-empty;
- temporary files do not accumulate after failures;
- the backup PVC has enough free space for another backup and atomic publication;
- periodic isolated restore drills succeed; and
- alerts reach an owner before retention or disk exhaustion threatens recovery.

Do not read the live `/data/ooshdaq.sqlite`, `-wal`, or `-shm` files with ordinary copy tools while the process is running.
Volume snapshots are supplementary recovery material, not a substitute for the verified online backup.

## Offline Restore

Restoration is deliberately offline. Exactly one maintenance process may mount the database and backup claims, and the
serving Deployment must have no pods.

The command used by the checked-in maintenance template is:

```text
/ooshdaq maintenance restore \
  --backup=/backups/<verified-backup>.sqlite \
  --database=/data/ooshdaq.sqlite \
  --retain-replaced=/backups/<pre-restore-copy>.sqlite
```

The template sets `OOSHDAQ_AUTH_MODE=maintenance`. The binary parses the maintenance subcommand before serving-mode
configuration. The value remains a fail-closed guard against accidentally starting a serving process; it is not a third
authentication mode.

The command must refuse to run when:

- the source and live destination resolve to the same path;
- the backup is missing, malformed, incompatible, or fails migration, foreign-key, or full-integrity verification;
- a temporary or retained destination would overwrite an existing file; or
- it cannot sync the restored file and containing directories.

It must restore through SQLite into an isolated sibling file, verify that file, preserve the prior live database under the
explicit `--retain-replaced` name, and only then atomically promote the verified restore. A failed pre-promotion step must
leave the live database unchanged.

### Compose restore drill

Stop the serving container, select exact unique filenames, and run the image as the sole maintenance process:

```sh
docker compose stop ooshdaq
docker compose run --rm --no-deps -e OOSHDAQ_AUTH_MODE=maintenance ooshdaq \
  maintenance restore \
  --backup=/backups/REPLACE_WITH_VERIFIED_BACKUP.sqlite \
  --database=/data/ooshdaq.sqlite \
  --retain-replaced=/backups/REPLACE_WITH_PRE_RESTORE_NAME.sqlite
docker compose up -d ooshdaq
```

### Kubernetes restore drill

1. Record the selected backup path, its logged SHA-256 digest, the source
   revision and operator-built image identity in use, and the change owner.
2. Scale the Deployment to zero and wait for its pod to disappear:

   ```sh
   kubectl scale deployment/ooshdaq --replicas=0
   kubectl wait --for=delete pod -l app.kubernetes.io/name=ooshdaq --timeout=5m
   ```

3. Copy `deploy/kubernetes/maintenance/restore-pod.yaml` to a controlled
   temporary location. Use the exact same operator-built image as the serving
   Deployment, and replace the backup filename and unique pre-restore
   filename. Do not add this Pod to the base kustomization.
4. Confirm no $OOSHDAQ pod exists, then create the one maintenance pod:

   ```sh
   kubectl apply -f /path/to/reviewed-ooshdaq-restore-pod.yaml
   kubectl wait --for=jsonpath='{.status.phase}'=Succeeded pod/ooshdaq-offline-restore --timeout=30m
   kubectl logs pod/ooshdaq-offline-restore
   ```

5. Delete the completed Pod so it releases both RWOP claims, then restore service:

   ```sh
   kubectl delete pod/ooshdaq-offline-restore
   kubectl scale deployment/ooshdaq --replicas=1
   kubectl rollout status deployment/ooshdaq --timeout=10m
   ```

6. Verify readiness, schema and catalog counts, login, collection read/write,
   persistence through one restart, and all three match presentations: a
   genuine no-request lead, a fulfilable request, and a distinctly
   unfulfillable request. Include a cross-catalog fulfilable request before
   closing the change.

If restoration fails, keep the Deployment at zero, retain logs and both source files, and diagnose without modifying the
selected backup. After SQLite cutover, recovery uses a verified SQLite backup, a schema-compatible image, or a forward
fix—never bbolt.
