# $OOSHDAQ SQLite and Multi-Catalog Uplift Plan

- **Status:** Complete for repository scope; production deployment acceptance remains external
- **Date:** 2026-07-30
- **Implements:** [ADR 0002](../architecture/adr/0002-sqlite-single-replica-multi-catalog.md)
- **Current matching/UI semantics:**
  [ADR 0004](../architecture/adr/0004-simple-states-and-availability-leads.md),
  as amended by
  [ADR 0005](../architecture/adr/0005-ranked-split-match-feeds.md)

This is the canonical dependency-ordered delivery record for the clean SQLite, multi-brand rebuild. It turns the product
and architecture decisions into small, evidence-gated slices. The implementation tracker distinguishes completed
repository evidence from the credentialed deployment checks that can only run in a target environment.

The live status should remain in the implementation tracker below. Code, tests, and deployment evidence decide whether a
gate is complete.

## Outcome

The uplift is complete when one recoverable $OOSHDAQ pod can:

- persist approximately 10,000 eligible members and sparse collection state through restarts and node rolls;
- browse at least five finite catalogs without loading every item into the browser;
- discover separately bounded actionable and unfulfillable availability leads
  through indexed SQLite queries;
- serve the expected read-heavy workload with up to 100 concurrent members;
- back up a live database, restore it in isolation, pass integrity checks, and complete a user journey; and
- publish only independently reviewed catalog metadata and artwork references.

The accepted downtime remains intentional. This plan does not add application replicas, a client/server database, a
cache, a queue, chat, payments, or durable trade state. See the
[operating requirements](../product/operating-requirements.md).

## Implemented Baseline

| Surface | Current behavior |
| --- | --- |
| Catalog model | First-class brands, catalogs, globally unique items, sources, publication state, and artwork provenance |
| Catalog input | Five versioned, validated JSON manifests; fictional mock users remain separate |
| Repository | Narrow catalog, collection, user, health, import, and match ports with shared memory/SQLite conformance |
| Matching | Separate indexed actionable/unfulfillable queries with capped-wait ranking and bounded request selection |
| HTTP API | Exhaustive bounded catalog, collection, summary, update, activity, and split-match routes with encrypted keyset cursors |
| Browser | Embedded vanilla modules with lazy route pages, inline collection states, stale-request rejection, and playful original artwork |
| Runtime | Pure-Go SQLite in WAL/FULL mode, checksummed migrations, online backup, offline restore, and shutdown checkpoint |
| Kubernetes | One `Recreate` pod with separate `ReadWriteOncePod` data and backup claims |

The persistence cutover was a clean replacement. There is no prototype-store scan, runtime selector, compatibility route,
or legacy data reader.

## Settled Decisions and M0 Evidence

[ADR 0003](../architecture/adr/0003-clean-reset-modernc-and-bounded-api.md) records the cross-cutting implementation
decisions:

1. **SQLite driver and build model.** Use `modernc.org/sqlite v1.55.0` with its matching
   `modernc.org/libc v1.74.1` pin and SQLite 3.53.3. Preserve the CGO-free static image, support Linux amd64 and arm64,
   keep driver-specific backup access inside the adapter, and move the build baseline to Go 1.26.5.
2. **Item identity.** Item IDs are globally unique, immutable, and never reused. Retain `catalog_id` as a foreign key and
   enforce uniqueness of the catalog-local number. This preserves simple collection keys, match IDs, and client maps.
3. **Clean reset.** The disposable prototype store was removed. Startup creates a fresh SQLite database and imports
   reviewed catalog manifests. No prototype migrator, dual-store mode, retained offline reader, or rollback path exists.
4. **Bounded API contract.** Use catalog pages of 20/50, scoped collection pages of 40/100, and match pages of 20/50.
   Encrypted and authenticated 15-minute cursors bind the member, normalized
   filters, limit, route, frozen ranking time, and deterministic keyset
   position. Replace the flat API and browser together without compatibility
   routes or fields.
5. **Operational choices.** Use one `ReadWriteOncePod` database claim on the GKE persistent-disk CSI driver. Run online
   backups through the serving process to separately mounted durable storage. Qualify the fixed 10,000-user,
   five-catalog, 100-worker, 60-second request mix and latency/error thresholds in ADR 0003.

**Gate complete:** the dependency and license review, automated driver assertions, implemented
[API contract](api-contract-v1.md), contract fixtures, and five embedded catalog manifests are present. The decisions
themselves are no longer open.

## Dependency Map

```text
M0 decisions and dependency proof
        |
        +---- M1 first-class catalog model and manifests
        |
        +---- M2 narrow repository ports and conformance suite
                       |
M1 + M2 --------------+---- M3 SQLite schema, adapter, and build
                                      |
                                      +---- M4 indexed availability-lead discovery
                                      |
                                      +---- M5 clean reset and operations
                                               |
M4 -------------------- M6 bounded API and catalog-aware browser
                                      |
M5 + M6 -----------------------------+---- M7 catalog expansion and qualification
                                                   |
                                                   +---- M8 staged cutover and cleanup
```

M1 and M2 may proceed in parallel after M0. Later milestones should land as reviewable vertical slices, not one database
rewrite branch.

M3 through M5 are logical evidence gates, but their persistence changes must reach the default branch as one SQLite-only
replacement. Stacked review branches are acceptable; an intermediate default-branch state containing both persistence
implementations is not.

## Milestones

### M0 — Close Cross-Cutting Decisions

- Run the driver/build spike and dependency review.
- Record the globally unique item-ID rule and the approved clean-reset procedure.
- Commit pagination/filter, retirement, storage, backup-execution, and release-threshold decisions.
- Add representative five-catalog JSON fixtures for contract and capacity tests.
- Update the build dependency list so embedded `.sql` and catalog manifests trigger rebuilds.

**Exit evidence:** exact SQLite version and compile options, amd64 and arm64 build results, backup/crash-reopen spike,
license and linked-module review, API fixtures, storage choice, release thresholds, and the recorded clean-reset authority. Driver
selection and operational choices are recorded in ADR 0003.

### M1 — Introduce Catalog Domain Data and Reviewed Manifests

- Add plain Go `Brand`, `Catalog`, `CatalogItem`, publication status, source, and artwork-provenance types.
- Remove the old `series_id` and `series_name` model rather than carrying compatibility projections.
- Move production catalog input out of executable Go arrays into a versioned manifest format.
- Build a validator and transactional, idempotent importer keyed by stable IDs and a manifest digest.
- Validate the complete manifest before mutation; reject duplicate IDs or numbers and invalid provenance.
- Require explicit retirement directives. Missing input must never silently retire or delete historical items.
- Keep fictional mock users and collection examples separate from production catalog data.
- Do no scraping or third-party fetching in the serving process.

**Exit evidence:** manifest schema and examples, validator tests, repeat-import tests, atomic rollback tests, explicit
retirement tests, and the current forty-item catalog reproduced without changing stable item IDs.

### M2 — Narrow Repository Ports and Add Conformance Tests

Introduce behavior-focused ports equivalent to:

- bounded catalog listing and item paging;
- bounded current-member collection reads and atomic entry writes;
- minimal user resolution;
- query-specific availability-lead discovery; and
- schema-aware repository health.

Add typed repository failures for not found, busy or retryable, incompatible schema, corruption, disk full or read only,
and unavailable storage. Keep HTTP status selection out of the adapters.

Share applicable conformance tests across memory and SQLite for ordering, reference checks, sparse zero-entry deletion,
user upsert, cancellation, restart durability, and concurrent reads and writes. Do not add bbolt compatibility behavior
or a selectable persistence mode. Preparatory port refactors may preserve the current application, but the SQLite
replacement slice removes bbolt rather than carrying both adapters.

**Exit evidence:** current HTTP and browser behavior remains green, services no longer depend on the broad repository
interface, and the memory contract is ready to exercise the SQLite replacement.

### M3 — Build the SQLite Schema and Adapter

Add ordered, checksummed, transactional migrations for tables equivalent to:

```text
schema_migrations  catalog_imports  brands  catalogs  catalog_items
catalog_sources    item_assets      users   collection_entries
```

Enforce foreign keys, immutable non-empty IDs, unique catalog item numbers,
valid publication states, quantity bounds, `available <= owned`, the
available-versus-wanted exclusivity rule, support for owned-and-wanted rows,
sparse entries, and deletion
restrictions that preserve history. Add collection and catalog-order indexes
plus partial or covering indexes for non-zero offers and wants.

Configure and verify required settings on every physical connection:

```text
journal_mode = WAL
synchronous = FULL
foreign_keys = ON
```

Use a bounded busy timeout, bounded connection pools, short transactions, explicit write serialization if required by
the selected driver, and a measured checkpoint policy. Migrate and import before readiness.

**Exit evidence:** empty-to-latest and every-supported-version migration tests, migration checksum and newer-schema
rejection, pragma verification per connection, foreign-key and integrity checks, contention and cancellation tests,
restart and abrupt-termination recovery, and visible disk-full/corruption handling.

### M4 — Move Availability Discovery Into an Indexed Query

Implement `FindMatches` in the SQLite adapter by joining the current member's
wants to other members' available copies. Do not require the counterpart to
want anything. Attach at most one deterministic counterpart request,
preferring an item the current member has available and otherwise falling back
to a published wanted item marked unfulfillable. Omit it only when the
counterpart has no published wants. Preserve self-user and same-item exclusion;
never add a catalog-equality condition.

- Support separate give-catalog and receive-catalog filters.
- Apply a hard server maximum and keyset pagination.
- Bind an opaque cursor to the complete immutable sort tuple and filter fingerprint.
- Return only the minimal counterpart, required receive leg, and optional
  requested item.
- Respect request cancellation and a bounded deadline.
- Do not precompute or cache matches until measurement proves the indexed query inadequate.

**Exit evidence:** same- and cross-catalog tests, retired/filter behavior, no duplicates, complete multi-page traversal,
`EXPLAIN QUERY PLAN` assertions using the offer/want indexes, including a
counterpart with no wanted entries, and proof the service no longer loads all
users or all item pairs.

### M5 — Implement Clean Reset, Backup, and Runtime Integration

The clean reset discards the current bbolt database and creates a fresh `.sqlite` destination from schema migrations and
reviewed catalog manifests. There is no valuable bbolt state to preserve. Do not add a data migrator, data import from
bbolt, a dual-store configuration, or a retained bbolt reader.

Then:

- switch the composition root, local defaults, Compose, and Kubernetes path to `.sqlite`;
- remove the bbolt adapter, dependency, tests, license notice, configuration wording, and disposable data in the same
  replacement slice;
- expose only the SQLite repository in the composition root, with no adapter selector or fallback;
- reject non-SQLite bytes at a SQLite path and never silently replace a valid SQLite database;
- add integrity, backup, restore, schema-migration, and recovery commands;
- use the online backup API or `VACUUM INTO`, never a raw live-file copy;
- restore to an isolated destination and verify schema, counts, integrity, and a cross-catalog journey;
- keep one pod with `Recreate`, prefer `ReadWriteOncePod` where supported, and retain block-volume filesystem semantics;
- keep liveness process-only and readiness false through schema migration, import, and a real bounded database read; and
- drain HTTP, stop maintenance work, checkpoint as designed, and close SQLite within the termination grace period.

**Exit evidence:** documented destructive reset, clean start, close/reopen, container replacement on the same volume,
SIGTERM and abrupt-kill tests, backup/isolated-restore drill, rendered manifests, no bbolt code or dependency, and an
SQLite-only rollback procedure.

### M6 — Transition the API and Browser Together

Implement the exact contract in the [HTTP API document](../architecture/http-api.md). The expected shape is:

- a bounded catalog index with brand, edition, publication, and member-summary metadata;
- a cursor-paged item or collection projection scoped to one catalog;
- a cursor-paged match response with optional give/receive catalog filters; and
- the item update route keyed by the settled globally unique item ID.

Because the browser ships in the same binary and the project is pre-release, make one coordinated transition. Remove the
flat endpoint and compatibility projections when the new browser and bounded contract land. Do not retain or version a
legacy API for this pre-release uplift.

In the browser:

- replace flat eager state with catalog-index, selected-catalog, summary, and per-query page state;
- load only navigation and summaries after login, then fetch a route's bounded data on demand;
- cancel requests or reject stale generations when the member switches catalog or filters;
- offer one server-scoped **Find a collectible** search that locates the
  containing catalog by item name and carries the search into that catalog;
- keep catalog and collectible searches, selected catalogs, and pagination in
  deep-linkable route state;
- keep rights/provenance validation in the manifest and API boundary without
  rendering licensing copy in collection or match cards;
- make the artwork itself the pressed-state **Have** control, with only
  **Available to swap** and **Want** as explicit checkboxes;
- group matches under each wanted collectible, put fulfilable requests first,
  describe genuine no-return leads plainly, and separate unfulfillable
  requests into a muted section;
- invalidate summaries and match cursors after an entry update without refetching every catalog; and
- preserve keyboard operation and focus across inline saves, meaningful
  loading/empty/error states, zoom, and narrow mobile layouts.

**Exit evidence:** HTTP limit/cursor/auth tests and a real browser lane covering login, five-catalog navigation,
pagination, editing, inverse cross-catalog setup, match discovery, broken or restricted artwork, keyboard use, mobile
layout, sign-out, and zero unexpected page, console, or CSP errors.

### M7 — Expand Catalogs and Qualify the Accepted Envelope

Prove the platform with at least five synthetic or redistribution-safe catalogs before relying on third-party approvals.
Each real catalog is a separate content change with its own source, rights, attribution, CSP-host, stable-ID, and review
decision. Synthetic fixtures prove scale and behavior; they do not grant permission to publish real brand content.

Run a recorded, read-heavy test with approximately 10,000 synthetic users, five or more catalogs, sparse state, and up to
100 concurrent members. Capture fixture shape, request mix, duration, hardware/storage class, database and WAL size,
latency distribution, errors, busy/lock events, checkpoint behavior, and query plans.

Also test SQLite schema-migration failure, process kill, disk full, container replacement, backup age/failure handling,
and isolated restoration. Required invariants are no committed-data loss, bounded responses, no prohibited application
scans, correct readiness, successful integrity checks, and documented accepted downtime. Repeat replacement, node-drain,
volume-reattachment, and capacity tests against the chosen target storage before claiming deployment acceptance.

**Repository exit evidence:** the reproducible [capacity report](qualification-report.md), recovery tests, and
[container persistence report](container-persistence-report.md) pass the thresholds recorded in M0. The fixed runner is
kept as a release-triggered command; schema-migration, query-plan, and browser regressions remain in the normal CI gate.

**Target deployment acceptance:** rerun the unchanged qualification and recovery procedures on the target
`standard-rwo` volume, including node drain and volume reattachment. That credentialed check is deliberately not
represented as repository evidence.

### M8 — Rehearse Cutover and Remove Transitional Paths

Rehearse the cutover locally and repeat it with the production storage class before deployment. During either controlled
maintenance window:

1. stop traffic and writes;
2. discard the current bbolt database or volume under the approved clean-reset authority;
3. deploy the SQLite-only image and configuration to a fresh `.sqlite` path;
4. apply schema migrations and import the reviewed catalog manifests;
5. verify schema, manifest digests, record counts, foreign keys, and integrity;
6. complete login, edit, persistence, genuine no-request, fulfilable
   cross-catalog request, and distinctly unfulfillable request smoke tests; and
7. reopen traffic.

There is no rollback to bbolt. If a check fails before reopening traffic, keep traffic closed and repair or redeploy the
SQLite release. After SQLite accepts writes, use only a schema-compatible prior image, a forward fix, or restoration from
a verified SQLite backup. Rehearse that SQLite-only rollback before the maintenance window.

The cutover release removes the runtime bbolt adapter, dependency, tests, notices, transitional API projections, and
compatibility normalizers. Update licenses, dependency inventory, local
instructions, manifests, API docs, and architecture status in the same change;
no retention window or offline bbolt reader remains.

**Repository exit evidence:** the current source completes the local staged cutover, same-volume container replacement,
fresh online backup and isolated restore, Kubernetes render validation, release-gate audit, and removal of transitional
behavior. See the [release-readiness handoff](release-readiness-report.md).

**Target deployment acceptance:** build from the selected source revision,
use the same operator-built artifact for serving and restore, rehearse on the
target storage, and retain the fresh backup/restore result and rollout record.
The project publishes no official binary or container image. An operator may
add a private registry, SBOM, scan, signature, or digest policy, but none is a
project release requirement.

## Suggested Pull Request Slices

1. M0 decisions, fixtures, and build-input correction.
2. Catalog domain types, manifest validator, and importer.
3. Narrow repository ports and memory conformance suite.
4. Stacked M3 SQLite dependency, schema, adapter, and repository tests; do not merge it alone.
5. Stacked M4 indexed match query, cursor contract, query-plan tests, and scale fixture.
6. M5 clean reset, bbolt removal, SQLite-only composition root, backup/restore, and local/Compose commands; merge the
   complete M3–M5 stack as one persistence replacement.
7. Bounded HTTP contract and handler tests.
8. Catalog-aware browser state, inline controls, original decorative artwork,
   and browser journeys, removing the old flat contract.
9. Kubernetes hardening, container/recovery tests, and CI lanes.
10. Five-catalog qualification, staged cutover, and final documentation audit.

Do not merge a slice that leaves startup capable of silently damaging an existing data file. A temporary feature flag is
not an accepted persistence design: deployable mainline has one SQLite repository, one Go process with its vanilla UI,
and one writable pod.

## Implementation Tracker

| Milestone | Status | Depends on | Completion artifact |
| --- | --- | --- | --- |
| M0 decisions and dependency proof | Complete | — | [ADR 0003](../architecture/adr/0003-clean-reset-modernc-and-bounded-api.md), [driver review](../security/modernc-sqlite-review.md), and [contract fixtures](../../testdata/contracts) |
| M1 catalog model and manifests | Complete | M0 | Embedded validator plus transactional SQLite import tests |
| M2 narrow ports and conformance | Complete | M0 | Shared memory/SQLite behavioral suite |
| M3 SQLite schema and adapter | Complete | M1, M2 | Migration, pragma, failure, contention, checkpoint, and abrupt-recovery tests |
| M4 indexed matching | Complete | M3 | Split readiness feeds, capped/stale ranking, no-request, same/cross/filter pagination, production query plans, and high-fanout tests |
| M5 clean reset and operations | Complete | M3 | SQLite-only runtime, [backup/restore runbook](../operations/sqlite-backup-restore.md), process lifecycle tests, and [container persistence evidence](container-persistence-report.md) |
| M6 API and browser transition | Complete | M4 | [Implemented HTTP contract](api-contract-v1.md), handler/contract suites, and [live browser journeys](browser-journey-report.md) |
| M7 catalog and envelope qualification | Complete — repository scope | M5, M6 | [Capacity qualification](qualification-report.md), recovery suites, and [container persistence evidence](container-persistence-report.md) |
| M8 cutover rehearsal and cleanup | Complete — repository scope | M7 | [Release-readiness handoff](release-readiness-report.md) and final documentation audit |

Change a milestone to `In progress` only when its dependencies are complete. Change it to `Complete` only when its exit
evidence is present or linked; code completion without recovery, rights, or operational evidence does not close the
corresponding gate. Repository completion does not imply that credentialed target-deployment acceptance has run.
