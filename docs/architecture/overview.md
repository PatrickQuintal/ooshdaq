# Architecture Overview

## Context

$OOSHDAQ is one Go service with an embedded browser UI and a transactional embedded database. It sits behind a trusted
identity-aware reverse proxy when deployed.

The accepted operating envelope is approximately 10,000 eligible staff, fewer than 100 concurrent members, at least five
finite catalogs from multiple brands, and acceptable downtime during restart or node roll. Data loss is not acceptable.
See [operating requirements](../product/operating-requirements.md).

```text
                  accepted target deployment shape

┌─────────┐   ┌──────────────────────────┐   ┌─────────────────────┐
│ browser │──>│ TLS ingress + auth proxy │──>│ one $OOSHDAQ Go pod │
└─────────┘   │ strips/injects identity  │   └──────────┬──────────┘
              └──────────────────────────┘              │
                                                        ▼
                                            ┌──────────────────────┐
                                            │ SQLite database file │
                                            │ persistent volume    │
                                            └──────────────────────┘
```

ADR 0002 selected SQLite, ADR 0003 authorized the completed clean reset from
the superseded bbolt implementation, and ADR 0004 simplified member state and
possible-swap discovery. There is no legacy reader, data migrator, dual-store
mode, or bbolt fallback.

Mock auth and trusted-proxy auth are mutually exclusive composition-time choices. They share downstream authorization
and business behavior but not identity acquisition.

## Responsibilities

### Composition root

- parse and validate configuration;
- select exactly one authentication strategy;
- open and migrate the repository;
- construct services and handlers;
- start the server and coordinate graceful shutdown.

It does not contain matching rules or HTTP-specific business decisions.

### HTTP and static UI adapter

- serve the vanilla HTML, CSS, and JavaScript UI;
- enforce request size and method constraints;
- resolve the authenticated principal through the configured auth adapter;
- decode and validate request shapes;
- call application services;
- encode stable JSON errors and responses;
- set defensive browser headers.

### Application services

- load the current member and catalog projections;
- apply collection updates atomically;
- calculate member-visible availability leads and one preferred counterpart
  request projection;
- prevent identity or repository implementation details leaking to clients.

### Domain

- define quantity and collection invariants;
- model offers and wants;
- determine whether another member has a wanted item available and whether
  their preferred request is fulfilable;
- provide deterministic ordering and deduplication rules.

The domain has no knowledge of HTTP, cookies, storage engines, Kubernetes, or forwarded headers.

### Repository boundary

- store and retrieve brands, catalogs, catalog items, users, and collection entries;
- provide atomic collection-entry writes and consistent individual reads;
- perform bounded, indexed availability-lead discovery without exposing
  storage-specific query types;
- map durable records to domain types;
- validate schema metadata and return deterministic order.

The current SQLite adapter satisfies the behavior-focused repository ports with relational constraints, schema
migrations, transactional manifest import, and query-specific indexed matching. The memory adapter implements the same
ports for tests; production has no runtime storage selector.

## Request Flow

For an authenticated collection update:

1. middleware resolves the principal from a mock session or trusted proxy headers;
2. the handler reads a bounded JSON body and rejects unknown or invalid fields;
3. the service builds a proposed collection state for that principal;
4. domain validation checks quantity invariants;
5. the repository commits the entry atomically;
6. the service returns the canonical member-owned projection;
7. the handler emits JSON without caching private data.

No body or path value can select a different member.

## Data and Consistency

Brand, catalog or edition, catalog item, source, and artwork provenance are separate concepts. Catalog-item identifiers
are globally unique, immutable, and never reused. Collection entries use `(user_id, item_id)` as their identity
and remain sparse when all quantities are zero.

An availability lead requires only that another member has an item the current
member wants. One counterpart request may come from a different catalog. The
matcher exposes disjoint readiness feeds. `actionable` contains either a
request the current member can fulfil or a genuine no-request extra;
`unfulfillable` contains a deterministic request only when the counterpart has
published wants but none the current member can offer. The result is derived
from current collection entries and is not persisted as a trade record.

The SQLite matcher uses separate bounded repository queries over indexed
non-zero availability and wants. It ranks waiting age with a 60-day cap,
applies only a coarse 90-day stale tie-break, supports deterministic catalog
filters, and paginates with an encrypted frozen-time keyset cursor. The browser
loads actionable results first and requests the unfulfillable query only when
that section is opened. Add a maintained projection or cache only after these
indexed queries are measured and shown insufficient.

See [data model](data-model.md).

## Runtime Characteristics

Implemented and locally qualified:

- one HTTP process and one writable embedded database file;
- SQLite runs in WAL mode with full synchronous durability;
- graceful shutdown before the pod termination grace period;
- liveness indicates that the process event loop is serving;
- readiness verifies a repository read transaction; startup validates the supported schema;
- process/container replacement on the same local volume preserves committed
  state, and an online backup has been restored into an isolated volume;
- no outbound network dependency is required on the request path;
- catalog artwork failures degrade to placeholders.

Accepted target deployment requirements:

- one application replica, one single-writer data claim, and one separate
  backup claim mounted only by that pod;
- brief downtime during replacement, rollout, and volume attachment is
  accepted;
- the persistent volume must preserve committed collection state across pod
  and node replacement;
- consistent backups and rehearsed target-environment restoration must protect
  against volume loss or operator error.

These target bullets describe acceptance criteria, not evidence that a
particular cluster, auth proxy, or persistent disk has been provisioned.

## Privacy Boundary

Collection contents are private authenticated data. Match results reveal only
the minimum needed to describe one availability lead: counterpart display
identity, the item that counterpart marked available, and at most one
published item the counterpart explicitly wants. Neither exact waiting age nor
last activity is projected; those private timestamps influence only the
documented capped/stale ordering. There is no endpoint for arbitrary member
collection or activity lookup.

## Evolution Rules

- Keep `/api/v1` compatible after a tagged release; before then, make one clean documented transition.
- Add a schema migration for durable shape changes and test forward migration from supported versions.
- Share repository conformance tests between memory and SQLite.
- Do not reintroduce the superseded bbolt adapter, migration, compatibility projections, or rollback path.
- Load-test synthetic multi-catalog data at the accepted 10,000-user and 100-concurrent-member envelope.
- Preserve the one-pod design unless measured contention or a changed availability requirement justifies more.
- Capture a new ADR when changing language, UI architecture, identity trust model, or persistence boundary.
