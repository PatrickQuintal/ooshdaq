# Kubernetes and GCP Deployment Direction

This document describes the accepted simple production shape for $OOSHDAQ on Kubernetes and GCP. It is governed by
[operating requirements](../product/operating-requirements.md) and
[ADR 0002](../architecture/adr/0002-sqlite-single-replica-multi-catalog.md), with the concrete driver, storage, backup,
and reset decisions fixed by
[ADR 0003](../architecture/adr/0003-clean-reset-modernc-and-bounded-api.md).

The checked-in manifests are a starting point, not a complete shared-environment deployment. They do not include an
identity-aware proxy, TLS ingress, network policy, secret-manager integration, external backup export, or
organization-specific policy. There is intentionally no backup Job, CronJob, sidecar, or second pod. The project
distributes source code and does not publish or sign an official container image.

## Operating Assumptions

- approximately 10,000 staff identities may be eligible;
- fewer than 100 members are expected to use the service concurrently;
- at least five finite multi-brand catalogs and availability leads that
  distinguish fulfilable, unfulfillable, and genuine no-request cases are
  required;
- writes are small collection-entry updates and traffic is read-heavy;
- data must survive process, pod, rollout, and node replacement;
- brief downtime during restart, rollout, or volume attachment is acceptable;
- zero-downtime rollout, active-active application replicas, and multi-region service are not initial requirements.

These assumptions deliberately favor one recoverable stateful pod over a managed or distributed database.

## Accepted Topology

```text
private DNS
    |
TLS ingress
    |
identity-aware auth proxy  <->  external IdP
    |
ClusterIP $OOSHDAQ Service
    |
one $OOSHDAQ Go pod
    |                         |
SQLite database              verified online backups
`/data/ooshdaq.sqlite`       `/backups`
    |                         |
standard-rwo RWOP PVC        separate standard-rwo RWOP PVC
```

Required properties:

- `OOSHDAQ_AUTH_MODE=proxy`;
- only the auth proxy can reach the application port;
- the proxy strips and injects the configured identity headers;
- exactly one $OOSHDAQ replica;
- `Recreate` or an equivalent update strategy that never overlaps writable pods;
- one `standard-rwo` `ReadWriteOncePod` claim mounted at `/data`;
- one separate durable `standard-rwo` `ReadWriteOncePod` claim mounted at `/backups` by the same container;
- online backup performed inside the serving Go process;
- TLS from browsers to ingress and protected proxy-to-service traffic;
- non-root pod, read-only root filesystem, dropped capabilities, seccomp, and no service-account token unless required;
- an operator-built image from the selected source revision, used consistently
  for serving and offline restore;
- liveness `/health/live` and readiness `/health/ready`.

The checked-in manifests and composition root encode the SQLite-only runtime: startup migrates the database, imports
reviewed manifests, verifies readiness, and starts the in-process backup scheduler. The same binary implements the
offline maintenance restore command. A deployment owner must still build or
select the image and add the organization-specific ingress, auth proxy, network
policy, and monitoring.

## Operator-Built Image

Build the repository Dockerfile locally:

```sh
make docker-build
```

This creates `ooshdaq:local`. The base Deployment and offline-restore template
both reference that tag with `imagePullPolicy: Never`, so Kubernetes fails
closed unless the operator has loaded the image onto the target node or
overridden the reference.

For Docker Desktop Kubernetes, the local image may already be available to the
cluster. For kind or another local cluster, use that cluster's image-load
command. A remote cluster such as GKE normally requires the deployment owner
to retag and push the self-built image to a registry they control, then replace
the image reference and pull policy in an environment-specific Kustomize
overlay.

There is no official `$OOSHDAQ` image, project registry, signature, or
project-issued binary checksum. An operator may add scanning, an SBOM,
signing, or digest pinning when their own policy requires it.

## Workload and Volume

Keep one replica. An autoscaler or additional replicas cannot safely share the SQLite file and are not useful for the
accepted load.

The checked-in GKE baseline uses two CSI-provisioned persistent disks with normal local filesystems:

- use the Compute Engine persistent-disk CSI `standard-rwo` StorageClass;
- use `ReadWriteOncePod` for both the database and backup claims;
- mount both claims only into the one serving container;
- never use a shared NFS, Filestore, or multi-writer filesystem for SQLite WAL;
- use a regional persistent disk when recovery onto a second zone is worth the cost;
- ensure the allowed zones have capacity to run the replacement pod;
- set a reclaim and snapshot policy that cannot accidentally discard production data with a namespace or claim;
- monitor used space, inode availability, attachment failures, and filesystem errors.

`ReadWriteOncePod` is the required base access mode, not a claim that the application is highly available. It prevents a
backup or debug pod mounting either claim while the serving pod exists. An offline restore therefore scales the
Deployment to zero and waits for both claims to detach before starting one maintenance pod.

Give termination enough time for request drain, database close, and volume release. A strict PodDisruptionBudget on a
single replica does not create availability and may obstruct a node upgrade; maintenance policy should instead accept the
documented interruption and verify recovery.

## SQLite Runtime

The SQLite adapter establishes and verifies:

```text
PRAGMA journal_mode = WAL;
PRAGMA synchronous = FULL;
PRAGMA foreign_keys = ON;
```

It also implements:

- a bounded busy timeout;
- short write transactions;
- bounded connection pools;
- explicit migration serialization before readiness;
- measured checkpoint behavior in the qualification suite;
- typed handling of busy, disk-full, read-only, corrupt, and incompatible-schema states;
- an SQLite version containing the WAL-reset fix identified in ADR 0002.

The application must keep the database, WAL, and shared-memory files together on the mounted volume. Do not copy only the
main file while the service is live.

## Restart, Rollout, and Node Roll

The expected recovery sequence is:

1. traffic stops reaching the terminating or failed pod;
2. the process drains when possible and closes the database;
3. Kubernetes terminates the old pod and detaches its volume;
4. a replacement pod is scheduled where the claim can attach;
5. SQLite performs crash recovery if shutdown was abrupt;
6. migrations and integrity checks complete;
7. readiness succeeds and traffic resumes.

The service is unavailable during some or all of that sequence. This is accepted. Committed collection data disappearing
after the sequence is a failure.

Use `Recreate` for image rollout. Create and verify a SQLite-aware backup before a schema migration; take a volume snapshot
as optional additional evidence. Rollback is safe only when the previous binary can read the migrated schema; otherwise
restore the pre-migration backup under the accepted maintenance window.

The initial bbolt-to-SQLite cutover is different from a later SQLite schema rollout. It is an authorized clean reset:
discard the bbolt database or volume, deploy the SQLite-only image to a fresh `.sqlite` path, apply schema migrations, and
import reviewed catalog manifests. There is no rollback to bbolt. Keep traffic closed on failure and repair or redeploy
SQLite; after SQLite accepts writes, rollback uses only a schema-compatible SQLite image or a verified SQLite backup. Use
the exact destructive and recovery procedures in the
[SQLite backup and restore runbook](sqlite-backup-restore.md).

## Backups and Restoration

A persistent database volume is not a backup. The one serving process creates online backups directly on the separately
mounted `/backups` claim. It must use the SQLite online backup API, verify the temporary database, sync it, and atomically
publish a completed filename before applying retention. The production design requires:

- no backup sidecar, CronJob, separate pod, or second database writer;
- a configured in-process schedule and retention count;
- monitored completion time, backup age, file size, digest, failure, and free space;
- transfer or snapshot of completed backups according to deployment-owner disaster-recovery policy;
- encrypted volume snapshots as additional recovery material;
- defined retention and deletion policy;
- full integrity, foreign-key, and migration-history validation before publication;
- restoration into an isolated file or environment before controlled replacement;
- verification of schema version, brands, catalogs, item counts, users,
  collection counts, a one-way availability lead, and an optionally enriched
  cross-catalog lead.

Do not claim a recovery-time or recovery-point objective until the schedule and restore evidence support it.

## Capacity and Matching

Validate with synthetic data representing approximately 10,000 users, at least five catalogs, sparse collection state,
and up to 100 concurrent members. Capture request mix, storage class, database size, latency, errors, lock waits, WAL
growth, and checkpoint behavior.

The production matcher must use indexed non-zero offers and wants to find
counterparts who have a current member's wanted item available. It attaches at
most one deterministic counterpart request, preferring one the current member
can fulfil and otherwise marking the request unfulfillable. A request is not a
condition of discovery. The matcher must not list every user and perform every
catalog-item pair comparison in Go. Catalog and match responses require hard
bounds and pagination or another explicit continuation mechanism as their
result sets grow.

Cross-catalog request selection is normal behavior, not a separate batch
workload. Do not introduce a cache, queue, or materialized match service until
the indexed SQLite query is measured and shown insufficient.

## Identity Integration

Use an approved identity-aware proxy or gateway that can:

- perform OIDC or SAML integration outside $OOSHDAQ;
- restrict admission to the intended community;
- remove all inbound instances of `X-Forwarded-User-ID` and `X-Forwarded-User-Name`;
- inject one stable subject and one bounded display label after authentication;
- prevent response caching of authenticated pages;
- expose a real upstream logout route.

Header names are configurable, but customization does not make an untrusted proxy safe. Add a NetworkPolicy and, where
available, protected proxy-to-service identity so only the proxy can write to the application socket. Do not create a
second ingress, external load balancer, debug port, or service that bypasses authentication.

## Configuration and Secrets

Non-secret configuration belongs in a ConfigMap or deployment values:

- listener;
- auth mode;
- data path (`/data/ooshdaq.sqlite`);
- backup directory (`/backups`), interval, and retention count;
- exact proxy identity header names.

Proxy mode uses the proxy/network trust boundary and does not require `OOSHDAQ_SESSION_SECRET`. Any future credentials
belong in an approved secret manager, not an image layer, committed manifest, log, or broadly readable pod description.
SQLite itself requires no network database credential.

For GKE:

- use an access-restricted cluster consistent with the threat model;
- build from the selected source revision and, if node-local loading is not
  suitable, store that operator-built image in an operator-controlled Artifact
  Registry;
- use the Compute Engine Persistent Disk CSI driver and the checked-in `standard-rwo` storage class;
- use Workload Identity Federation only if backup or other approved Google API access requires it;
- send redacted logs and bounded operational metrics to approved sinks;
- enforce Pod Security, admission policy, resources, namespace quotas, and restrictive network policy.

## Network Policy

Default-deny ingress and egress for the namespace or workload. Allow:

- ingress to the auth proxy from the designated ingress controller;
- ingress to $OOSHDAQ only from the auth proxy;
- DNS and explicitly required backup or telemetry egress;
- no serving-pod access to third-party catalog sites.

Catalog ingestion is an offline, reviewed maintenance workflow. Remote catalog images, when approved, are fetched by the
member's browser rather than the serving pod; their privacy and rights implications remain documented separately.

## Production-Readiness Checklist

- [x] The image composition root is SQLite-only and honors the documented data and backup environment.
- [x] The clean-reset cutover removes bbolt code, dependency, notices, data, and rollback instructions.
- [ ] Auth proxy and identity-provider admission are configured.
- [ ] Direct application access and identity-header spoofing are blocked and tested.
- [ ] TLS, cookie, and browser security headers are verified.
- [x] The base templates reference the locally built `ooshdaq:local` image and
      never pull an unrelated image by default.
- [ ] The target environment uses one operator-built image consistently for
      serving and offline restore.
- [ ] Mock auth is absent.
- [x] Exactly one writable pod and the intended volume access mode are encoded.
- [ ] Both `standard-rwo` RWOP claims are bound and mounted only by the serving pod in the target cluster.
- [x] Database migrations, WAL settings, lock handling, and readiness are verified in repository tests.
- [x] In-process backup schedule, retention, integrity checks, and offline restore drill are implemented and tested.
- [x] No backup sidecar, Job, CronJob, or second database writer exists.
- [ ] Pod replacement, rollout, and node-drain recovery are rehearsed in the target cluster.
- [ ] Synthetic 10,000-user, five-catalog, 100-concurrent-member capacity evidence is current.
- [x] Indexed availability-lead and preferred request-selection queries are
      bounded and query-plan tested.
- [ ] Resource, disk, readiness, restart, attachment, and backup alerts are active.
- [ ] Catalog provenance and artwork rights have been reviewed independently for every published catalog.

## Revisit Triggers

PostgreSQL, Spanner, concurrent replicas, or another availability design require measured evidence and a new ADR. Use the
triggers in [operating requirements](../product/operating-requirements.md); do not promote catalog count or eligible-user
count alone into a distributed-systems requirement.
