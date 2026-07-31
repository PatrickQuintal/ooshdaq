# ADR 0003: Clean Reset, modernc SQLite, and a Bounded API

- **Status:** Accepted
- **Date:** 2026-07-30
- **Implements:** Milestone M0 in the
  [SQLite and multi-catalog uplift plan](../../implementation/sqlite-multi-catalog-uplift.md)
- **Partly superseded by:**
  [ADR 0004](0004-simple-states-and-availability-leads.md), which changes
  collection interaction, match qualification, match projections, and visible
  summary metrics

## Context

ADR 0002 selected SQLite, a single writable pod, and indexed multi-catalog matching. The implementation plan left several
cross-cutting decisions open because they affect the domain, repository, image, HTTP API, browser, and operations.

The application has not reached a valuable deployment state. Its bbolt database, flat API, current browser behavior, and
tests are disposable. The agreed operating envelope remains approximately 10,000 eligible members, fewer than 100
concurrent members, five or more finite catalogs, read-heavy traffic, and acceptable restart downtime.

## Decision

### Perform a clean reset

The SQLite release starts with a new database created by SQLite migrations and reviewed catalog manifests.

- Do not build a bbolt migrator, dual-store mode, retained bbolt reader, legacy API, or rollback path to bbolt.
- Remove the bbolt adapter, dependency, tests, license notice, configuration, and local data.
- After SQLite becomes current, protect SQLite data through forward migrations, schema-compatible images, and verified
  SQLite backups. The reset authority does not apply to future SQLite production data.

### Use globally unique item IDs

Catalog-item IDs are immutable, globally unique, and never reused. `catalog_id` remains an explicit foreign key and
`(catalog_id, number)` is also unique. This keeps collection keys, mutation paths, match IDs, and cursor positions simple
without hiding catalog ownership.

### Select modernc SQLite

Use these explicit module pins:

```text
modernc.org/sqlite v1.55.0
modernc.org/libc   v1.74.1
```

The matching `modernc.org/libc` pin is deliberate because the driver project requires downstream modules to use the
version selected by its module. Upgrade both together.

The selected driver:

- is CGO-free and preserves `CGO_ENABLED=0`, the Alpine builder, and a static scratch runtime;
- bundles SQLite 3.53.3, newer than the SQLite 3.51.3 WAL-reset fix required by ADR 0002;
- supports Linux amd64 and arm64;
- exposes online backup through a driver connection;
- exposes extended SQLite result codes for classified repository failures;
- honors `database/sql` context cancellation; and
- is BSD-3-Clause licensed.

The accepted source evidence is:

```text
modernc.org/sqlite module checksum:
h1:hIFh0MCH0rGinQ/4KYb5/UbCkRkb+UP+OkLCVWa5MTM=

modernc.org/libc module checksum:
h1:bdR4VTKFMC4966QSNZ05XLGI/VwzVa2kTUX51Dm0riQ=

SQLite runtime version:
3.53.3

SQLite runtime source ID:
2026-06-26 20:14:12 d4c0e51e4aeb96955b99185ab9cde75c339e2c29c3f3f12428d364a10d782c62
```

The dependency adds eleven transitive modules. Their licenses must be
represented in `LICENSES` and `NOTICE`; any operator-generated SBOM should
agree with the compiled binary and that inventory. The driver-specific backup
escape hatch remains inside the SQLite adapter.

The build baseline moves from Go 1.26.2 to Go 1.26.5 because the later point release contains standard-library security
fixes. Dependency and image updates remain pinned and reviewed.

### Use a small bounded `/api/v1`

All product routes require authentication. Session, mock login, logout, liveness, and readiness retain their current
security behavior. Product reads become:

| Route | Purpose | Default limit | Maximum limit |
| --- | --- | ---: | ---: |
| `GET /api/v1/catalogs` | Catalog navigation and per-catalog member summaries | 20 | 50 |
| `GET /api/v1/catalogs/{catalog_id}/collection` | One catalog's item and collection projection | 40 | 100 |
| `GET /api/v1/collection/summary` | Current member's bounded aggregate summary | — | — |
| `PUT /api/v1/collection/{item_id}` | Replace one current-member entry | — | — |
| `GET /api/v1/matches` | Availability leads with optional reciprocal items | 20 | 50 |

Brands remain first-class records and appear inside catalog projections. A separate brand-list endpoint is unnecessary
for the initial browser.

The old flat catalog and collection reads are removed. They return the normal `404 not_found`; no `410` compatibility
handler, `series_id`, `series_name`, or browser fallback remains.

#### Catalog query

`GET /api/v1/catalogs` accepts:

- `brand_id`;
- `q`, trimmed and limited to 100 Unicode code points;
- `publication_status=active|retired|all`, defaulting to `active`;
- `limit`; and
- `cursor`.

Catalogs order by immutable `sort_order`, then catalog ID.

#### Collection query

`GET /api/v1/catalogs/{catalog_id}/collection` accepts:

- `q`, limited to 100 Unicode code points and matched against item name, universe, and variant;
- `universe`, an exact reviewed value;
- `state=all|owned|available|wanted`, defaulting to `all`;
- `limit`; and
- `cursor`.

Filters combine with AND. Results always order by item number, then item ID. Search does not introduce relevance ranking.
Every returned row contains the item projection plus zero or sparse quantities for the current member.

#### Match query

`GET /api/v1/matches` accepts:

- `give_catalog_id`;
- `receive_catalog_id`;
- `scope=all|same_catalog|cross_catalog`, defaulting to `all`;
- `limit`; and
- `cursor`.

Results order by immutable counterpart ID and received-item ID. One suggestion
contains one required `you_receive` leg and, when available, one optional
`you_give` leg; arrays and grouped-trade compatibility are not part of the
clean API.

#### Summaries

The API may retain aggregate fields for compatibility and internal use. Under
ADR 0004, the browser presents only available and wanted design counts plus an
exact distinct availability-lead count for the navigation badge.

#### Pagination

Paginated responses include:

```json
{
  "page": {
    "limit": 40,
    "returned": 40,
    "next_cursor": null
  }
}
```

Cursors are authenticated, base64url-safe tokens containing a version, route, authenticated member, normalized filters,
limit, expiry, and last immutable ordering tuple. They expire after 15 minutes. A continuation supplies only `cursor`;
combining a cursor with first-page filters is rejected.

At the time of this decision the process generated a fresh HMAC key at
startup. [ADR 0005](0005-ranked-split-match-feeds.md) later replaced the
signed plaintext payload with AES-256-GCM encryption and authentication under
a fresh process-local key. Restarting still invalidates an in-flight cursor,
which is acceptable; the browser returns to page one. Cursors provide
deterministic traversal for an unchanged data set but do not create a snapshot
or reservation while collections are changing.

Unknown or repeated scalar query parameters are rejected. Empty valid searches return an empty page. Invalid, tampered,
wrong-route, wrong-catalog, wrong-member, conflicting, or expired cursors return a safe `400` error.

### Keep artwork publication server-controlled

Catalog and item projections contain explicit artwork metadata: approved image URL, alt text, dimensions when known,
source URL, attribution, rights status, license when applicable, and retrieval date. Unknown or restricted artwork omits
the remote image URL. The browser uses the local placeholder and cannot convert an arbitrary manifest URL into an
allowlisted image.

### Distribute source, not official artifacts

The project publishes source code only. It does not publish or sign an
official binary or container image and does not operate a project container
registry. The Dockerfile is a reproducible build input for users and
self-hosting operators.

The Kubernetes templates reference the operator-built `ooshdaq:local` tag with
pulling disabled by default. An operator may load that image directly into a
cluster or replace it with an artifact from an operator-controlled registry.
Scanning, SBOM generation, signing, and digest pinning are operator-policy
choices rather than project release gates.

### Use one pod and one database writer

The base deployment uses:

- one replica with `Recreate`;
- a CSI-backed `ReadWriteOncePod` database claim;
- a normal filesystem on persistent block storage;
- readiness held false through migrations, catalog import, integrity checks, and a real database read; and
- enough termination time to drain HTTP and close SQLite.

Use the GKE Compute Engine persistent-disk CSI driver and its `standard-rwo` storage class as the checked-in default.
Regional storage remains an environment choice, not a baseline availability claim.

Online backups run through the serving Go process so a second pod or writer never mounts the database claim. A configured
backup directory is mounted from separate durable storage. Backup creation uses SQLite's online backup API, writes a
temporary destination, verifies it, and atomically publishes it. Restoration is an offline maintenance operation into an
isolated file before controlled replacement.

### Fix measurable release gates

The capacity fixture contains:

- 10,000 fictional users;
- five catalogs from at least three fictional or redistribution-safe brands;
- at least 200 items;
- at least 80,000 sparse collection rows; and
- one-way availability leads, including counterparts who want nothing, and
  optional same-catalog and cross-catalog reciprocal items.

The release workload runs for 60 seconds with 100 concurrent workers and this request mix:

```text
15% catalog navigation
50% collection page reads
30% possible-swap reads
 5% collection writes
```

Under a Linux process with hard cgroup limits of one CPU and 256 MiB—either a
container or an equivalent transient scope—the gate requires:

- zero unexpected HTTP or repository errors;
- no exposed `SQLITE_BUSY`, corrupt, or partial-write result;
- p95 below one second for catalog, collection, and write requests;
- p95 below two seconds for match requests;
- every response at or below its hard item limit;
- required indexes present in recorded query plans;
- a final successful `foreign_key_check` and `integrity_check`; and
- recorded database, WAL, checkpoint, memory, and latency results.

These are reproducible release thresholds, not a user-facing availability or latency service-level objective. Change them
only with recorded evidence and an explicit documentation update.

## Consequences

### Positive

- The implementation has no migration or compatibility branch.
- The selected driver preserves a small static image while providing relational queries and online backup.
- The HTTP and browser workload stays bounded as catalogs grow.
- Cursor and match behavior is deterministic without introducing snapshot infrastructure.
- The release envelope can be tested locally and repeated on the selected production storage class.

### Costs and constraints

- Modernc's transitive dependency and license inventory is larger than bbolt's.
- A restart invalidates browser cursors; the UI must recover by returning to page one.
- SQLite data becomes valuable after cutover and must no longer be treated as disposable.
- A separate durable backup destination and production identity proxy remain deployment-owner integrations.

## Rejected Alternatives

- **CGO driver:** mature, but it adds a native toolchain and runtime/linkage work without a demonstrated benefit here.
- **Wasm-based pure-Go driver:** smaller module graph, but its new translation path and custom VFS are less mature for this
  first production store.
- **Legacy API window:** rejected because there are no valuable consumers.
- **Revision-bound cursor snapshots:** rejected because global collection revisions make match cursors fragile and add
  machinery without making suggestions durable.
- **Separate backup pod:** rejected because `ReadWriteOncePod` deliberately prevents a second pod mounting the live claim.
- **Client/server database or multiple pods:** rejected by the accepted load and downtime requirements.

## References

- [modernc SQLite package](https://pkg.go.dev/modernc.org/sqlite)
- [modernc SQLite v1.55.0 changelog](https://gitlab.com/cznic/sqlite/-/blob/v1.55.0/CHANGELOG.md)
- [SQLite WAL-reset bug](https://sqlite.org/wal.html#the_wal_reset_bug)
- [SQLite online backup API](https://sqlite.org/backup.html)
- [Go 1.26 release history](https://go.dev/doc/devel/release#go1.26.0)
- [Kubernetes `ReadWriteOncePod`](https://kubernetes.io/docs/tasks/administer-cluster/change-pv-access-mode-readwriteoncepod/)
- [GKE persistent volumes](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/persistent-volumes)
