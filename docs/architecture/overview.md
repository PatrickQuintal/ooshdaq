# Architecture Overview

## Context

$OOSHDAQ is one Go HTTP service with an embedded vanilla browser UI and one
SQLite database. A deployment places the service behind a trusted
identity-aware reverse proxy; local development uses a separate mock identity
adapter. The accepted load, durability, and availability boundaries are in the
[operating requirements](../product/operating-requirements.md).

```text
┌─────────┐   ┌──────────────────────────┐   ┌─────────────────┐
│ browser │──>│ TLS + auth proxy         │──>│ one Go process  │
└─────────┘   │ strips/injects identity  │   │ HTTP + UI       │
              └──────────────────────────┘   └────────┬────────┘
                                                      ▼
                                             ┌─────────────────┐
                                             │ SQLite on one   │
                                             │ durable volume  │
                                             └─────────────────┘
```

The stack rationale is in [ADR 0001](adr/0001-go-vanilla-sqlite.md); collection
and matching decisions are in
[ADR 0002](adr/0002-collection-states-and-matching.md) and
[ADR 0003](adr/0003-ranked-split-match-feeds.md).

## Components and Responsibilities

### Composition root

The composition root validates configuration, selects exactly one
authentication adapter, opens and migrates SQLite, imports reviewed catalog
manifests, wires services and handlers, and coordinates graceful shutdown. It
contains no transport or matching policy.

Normal startup imports only manifests in `internal/catalog/manifests`.
Fictional catalogs under `internal/catalog/testdata/manifests` are included
only when the local demo option is explicitly enabled.

### HTTP and browser adapters

HTTP handlers resolve the principal, enforce method, size, media-type, query,
CSRF, and identifier bounds, call an application service, and encode the
[wire contract](http-api.md). They do not issue matching queries or expose
persistence records. Embedded HTML, CSS, and JavaScript own presentation and
interaction only; they do not reconstruct authorization or ranking policy.

### Application services

Services coordinate use cases for the current principal. They validate
proposed domain values, invoke narrow repository ports, translate failures,
and return authorization-aware projections. Matching is delegated to the
query-specific repository port so a service cannot fall back to an all-user or
all-item scan.

### Domain

Plain Go types define catalog publication, artwork provenance, user, and
collection invariants. The domain has no dependency on HTTP, cookies, SQLite,
deployment platforms, or forwarded headers. See the [data model](data-model.md).

### Repository

Behavior-focused ports separate use cases from storage. The SQLite adapter
owns schema migrations, relational constraints, transactions, catalog import,
failure classification, backup primitives, and bounded indexed matching. The
tests use temporary SQLite databases so matching has one authoritative
implementation. Storage records and query positions do not cross the
repository boundary.

## Request Flow

An authenticated collection replacement follows one path:

1. the configured adapter resolves a principal;
2. the handler validates the item path, CSRF proof, and bounded JSON body;
3. the service builds a collection entry owned by that principal;
4. domain validation enforces quantity invariants;
5. the repository commits the replacement atomically;
6. the handler returns the canonical current-member projection with no-store
   caching.

No path, query, or body field can select another member.

## Data and Consistency

Brand, catalog, catalog item, artwork rights, and artwork provenance are
separate concepts. Published IDs and ordering keys are immutable. Collection
rows are sparse and identified by the current user plus a globally unique item
ID.

Possible swaps are derived suggestions, not reservations or durable trade
records. Separate indexed repository queries produce independently bounded
actionable and unfulfillable feeds. Encrypted keyset cursors preserve filters
and a frozen ranking clock, but do not create database snapshots; concurrent
collection changes may alter later pages.

SQLite uses transactions, foreign keys, explicit schema migrations, WAL
journaling, and full synchronous durability. One process owns the writable
database and volume. Restart downtime is accepted; online backups and
rehearsed restores protect against storage loss. Operators choose the hosting
platform while preserving the boundaries in
[operating requirements](../product/operating-requirements.md),
[authentication](authentication.md), and the
[restore runbook](../operations/sqlite-backup-restore.md).

## Trust and Privacy Boundaries

Mock and proxy authentication are mutually exclusive startup choices and
share the downstream principal shape. Proxy identity is trusted only under the
topology in [authentication](authentication.md).

Collection contents are private. Match projections reveal only a counterpart
display identity, one offered item, and at most one item that counterpart
wants. Exact state age and activity timestamps affect coarse ordering but are
never returned. There is no arbitrary-member collection, activity, contact,
messaging, payment, auction, or reservation endpoint.
