# ADR 0002: SQLite, One Recoverable Pod, and Multi-Catalog Matching

- **Status:** Accepted
- **Date:** 2026-07-30
- **Supersedes:** The bbolt and future-Spanner persistence direction in
  [ADR 0001](0001-go-vanilla-ui-bbolt-repository-boundary.md)
- **Partly superseded by:**
  [ADR 0004](0004-simple-states-and-availability-leads.md), which replaces
  reciprocal-only discovery with one-way availability leads and optional
  reciprocal enrichment

## Context

$OOSHDAQ is expected to admit approximately 10,000 staff identities, with fewer than 100 concurrent members in normal
use. It must support at least five finite catalogs from multiple brands and
possible swaps across catalog boundaries.
Collection writes are small and infrequent. Data must survive restarts and Kubernetes node rolls, but brief downtime is
acceptable because this is not a critical service.

The original prototype used a key/value store and an all-user in-process matcher. Multi-catalog discovery would have
required custom secondary indexes or increasingly broad application scans, with work growing with users multiplied by
the square of total catalog size.

A managed PostgreSQL or Spanner service could provide concurrent application replicas and automated database failover, but
those properties are not current requirements. They would add credentials, network dependencies, provisioning, cost, and
operational ownership before they solve a measured problem.

The accepted operating envelope is documented in
[operating requirements](../../product/operating-requirements.md).

## Decision

### Keep the modular Go application

Retain the single Go service, embedded vanilla browser UI, authentication adapters, domain rules, application services, and
repository boundary accepted by ADR 0001. Persistence-specific SQL, migrations, connection behavior, and backup mechanics
remain inside the repository adapter and composition root.

The deployed application remains one Go process containing that UI. It does not gain a second service, a frontend
toolchain, or a selectable persistence mode.

### Use SQLite as the production persistence target

Replace bbolt with one SQLite database file for deployed and local persistence. SQLite is selected for:

- relational brand, catalog, item, provenance, user, and collection data;
- transactions and foreign-key constraints;
- indexed joins for availability discovery and optional reciprocal matching;
- schema migrations and integrity checking;
- one-file deployment and recovery without a separate database service.

The driver and bundled SQLite version require a separate dependency review. The selected version must contain the
WAL-reset corruption fix available in SQLite 3.51.3 or a documented fixed backport.

### Run one writable pod

Run exactly one $OOSHDAQ pod against one persistent block-volume claim.

- Use `ReadWriteOncePod` when the selected CSI storage class supports it.
- Otherwise use `ReadWriteOnce`, one replica, and an update strategy that cannot overlap two writable pods.
- Use a filesystem on a single-writer block volume, not NFS, Filestore, or another shared multi-writer filesystem.
- Use `Recreate` or equivalent ordered replacement and accept a short service interruption.
- Give termination enough time to drain HTTP requests, close SQLite, and release the volume.
- Readiness must fail until migrations complete and the database can serve a real read.

A regional persistent disk may be selected when recovery onto another zone is worth its additional cost. That improves
volume durability and placement options; it does not make the single application pod continuously available.

### Configure SQLite for bounded server use

The adapter should establish and verify at least:

```text
PRAGMA journal_mode = WAL;
PRAGMA synchronous = FULL;
PRAGMA foreign_keys = ON;
```

It must also use a bounded busy timeout, short write transactions, bounded connection pools, and an explicit checkpoint
strategy informed by load tests. `SQLITE_BUSY`, disk-full, corruption, and migration failures must surface rather than
silently dropping a write.

### Make catalogs first-class data

The durable model separates:

- brand or issuer;
- catalog or edition;
- catalog item;
- source and artwork provenance;
- user;
- sparse collection entry.

Catalog items have globally unique, immutable, never-reused identifiers. `catalog_id` remains an explicit foreign key, and
catalog-local item numbers are unique within their catalog. Removal from current display retires an item rather than
remapping historical collection data. Catalogs arrive through an offline, reviewed, idempotent import; they are not
hard-coded as executable Go arrays in the long-term design.

### Replace bbolt through a clean reset

The current pre-release bbolt databases contain no valuable data. The SQLite uplift therefore performs an authorized clean
reset: discard the current bbolt store, create a fresh SQLite database, apply SQLite schema migrations, and import the
reviewed catalog manifests.

Do not build a bbolt-to-SQLite data migrator, dual-store mode, retained bbolt reader, legacy flat API, or rollback path to
bbolt. The replacement release removes the bbolt adapter, module dependency, tests, notices, and runtime configuration.
Rollback after cutover uses only a schema-compatible SQLite image, a forward fix, or a verified SQLite backup.

### Query possible swaps through indexes

Replace the all-user repository scan with a query-specific repository
operation. The current rule, as superseded by ADR 0004, first joins:

```text
current member wants Y   <-> counterpart offers Y
```

It may then enrich that lead when `current member offers X` and `counterpart
wants X`; this second leg is optional. Indexes target non-zero offers and
wants. The query supports catalog filters and cross-catalog item pairs, has a
hard result limit, deterministic order, timeout/cancellation, and cursor
pagination when the HTTP contract adds it. Matches remain derived suggestions
rather than durable trades.

Do not add a cache or precomputed match service until the indexed query is measured against the accepted load envelope.

### Back up through SQLite-aware tooling

Use the SQLite online backup API or `VACUUM INTO` to create a consistent backup of the live database before transferring
it to approved backup storage. Volume snapshots are useful additional recovery material, not the sole backup mechanism.
Define retention, monitor backup age and failures, run integrity checks, and rehearse restoration.

## Consequences

### Positive

- One application binary and one durable file remain easy to run and audit.
- SQL joins and indexes fit multi-brand catalogs and cross-catalog matching.
- The deployment survives pod and node replacement without operating a database service.
- The accepted downtime and concurrency profile are explicit.
- The repository boundary preserves a future migration path without designing for it prematurely.

### Costs and constraints

- Only one application pod serves at a time.
- Restarts, image rollouts, volume reattachment, and some node events cause downtime.
- SQLite permits one writer at a time; writes must remain short and contention must be measured.
- Operators own migrations, checkpoints, backups, restores, disk capacity, and recovery testing.
- The clean reset intentionally discards the nonvaluable pre-release bbolt state; collection history does not cross the
  replacement boundary.

## Rejected Alternatives

### Keep the prototype key/value store long term

Rejected because its key/value model would require custom relational indexes
or broad scans for multi-catalog availability discovery. SQLite gives the
required query and migration tools without changing the one-file operating
model.

### Start with PostgreSQL

PostgreSQL is a credible future adapter, especially if concurrent replicas or sustained write concurrency become
requirements. It is rejected as the baseline because the current load and availability requirements do not justify a
separate database service.

### Start with Spanner

Spanner is rejected as the baseline because neither global distribution nor uninterrupted regional availability is a
requirement. Eligible-user and catalog counts alone do not justify a distributed database.

### Replicate or share the SQLite file between active pods

Rejected. WAL assumes processes accessing the database are on the same host, and a shared filesystem does not turn SQLite
into a safe multi-node writer. The deployment enforces one writable pod instead.

## Implementation Status

The decision is implemented: `go.mod`, the composition root, local operations, and manifests use only SQLite; the bounded
HTTP API and embedded browser are catalog-aware; and availability-lead
discovery is an indexed repository query. Five reviewed
manifests import transactionally with globally unique item IDs. The clean reset removed the prototype adapter, dependency,
tests, notices, and compatibility surface; there is no data inventory or migration branch.

The dependency order, milestone gates, SQLite-only rollback boundary, and required evidence are tracked in the
[SQLite and multi-catalog uplift plan](../../implementation/sqlite-multi-catalog-uplift.md).
The driver, clean API, cursor, storage, and measurable release decisions are recorded in
[ADR 0003](0003-clean-reset-modernc-and-bounded-api.md).

## Revisit Triggers

Use the triggers in [operating requirements](../../product/operating-requirements.md). A new database choice requires a new
ADR with measured evidence.

## References

- [SQLite write-ahead logging](https://sqlite.org/wal.html)
- [SQLite online backup API](https://sqlite.org/backup.html)
- [Kubernetes persistent-volume access modes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#access-modes)
- [GKE persistent-volume concepts](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/persistent-volumes)
