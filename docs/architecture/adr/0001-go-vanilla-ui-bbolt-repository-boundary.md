# ADR 0001: Go, Vanilla UI, bbolt, and a Repository Boundary

- **Status:** Accepted in part; persistence direction superseded by
  [ADR 0002](0002-sqlite-single-replica-multi-catalog.md)
- **Date:** 2026-07-30

ADR 0001 remains authoritative for the Go service, embedded vanilla UI, and repository boundary. ADR 0002 replaces its
long-term bbolt and future-Spanner direction with SQLite, one recoverable pod, and indexed multi-catalog matching. The
sections below remain as the historical rationale for the first implementation.
[ADR 0004](0004-simple-states-and-availability-leads.md) replaces the
reciprocal-only product rule.

## Context

$OOSHDAQ needs a small full-stack application that:

- starts locally without an external identity provider or database;
- can be shipped as a minimal container to Kubernetes;
- handles trusted reverse-proxy identity in deployment;
- persists a modest finite catalog and member collection quantities;
- calculates current possible-swap leads consistently enough to avoid misleading partial state;
- keeps dependencies and supply-chain surface small;
- leaves a credible path to Google Cloud Spanner if higher availability or horizontal scale becomes necessary.

The initial user interface is a small collection dashboard, not a rich collaborative editor. The expected data set fits on
one node. A distributed database and frontend build framework would add operational and dependency cost before they solve
a demonstrated problem.

## Decision

### Go service

Use one Go module and one server process. Use the standard library for HTTP, JSON, static embedding, cryptographic
primitives, and tests where practical. Compile a static binary and ship it in a minimal non-root container.

### Vanilla embedded UI

Build the initial browser interface with semantic HTML, CSS, and small vanilla JavaScript modules served or embedded by the
Go binary. Do not introduce a runtime third-party script or Node-based production build until UI complexity demonstrates
that a framework/toolchain materially improves correctness or maintainability.

This is not a ban on browser testing or build-time asset tooling; those additions still require dependency review and a
clear job.

### bbolt embedded persistence

Use `go.etcd.io/bbolt` as the initial transactional embedded store. Keep one writable database file, one $OOSHDAQ replica,
and durable block storage. bbolt encoding, keys, buckets, migrations, and transaction mechanics live only in its adapter.

### Repository and service boundaries

Domain and application packages depend on small behavior-oriented repository interfaces. HTTP code depends on application
services, not directly on bbolt. Durable records map to domain values inside the adapter.

Repository semantics include:

- stable catalog and principal identity;
- atomic replacement of one collection entry;
- deterministic collection reads for possible-swap calculation;
- explicit schema versioning and stable catalog identifiers;
- visible errors for corruption or unsupported versions;
- deterministic ordering.

Create a repository conformance suite that can run against bbolt and future implementations.

### Historical future Spanner adapter (superseded)

Spanner is a future adapter, not an emulation target for bbolt's physical layout. When justified, implement the same
behavioral repository contracts using a Spanner schema, transactions, indexes, IAM, emulator tests, and migration tooling.
Do not leak Spanner timestamps, retry codes, record types, or query shapes into the domain or HTTP API.

## Consequences

### Positive

- Local startup needs only a Go toolchain and writable file.
- One static binary and no production JavaScript dependency graph reduce build and runtime surface.
- Go's HTTP and concurrency model suit a small network service and graceful Kubernetes operation.
- bbolt provides durable transactions without running a database service.
- Repository isolation makes persistence tests focused and a Spanner transition credible.
- The whole initial system remains understandable by a small contributor group.

### Costs and constraints

- bbolt has a single writer and the file must be opened by one application process.
- Kubernetes deployment uses one replica and `Recreate`, so upgrades and some failures cause downtime.
- Backup/restore and schema migration are application/operator responsibilities.
- In-process possible-swap discovery must remain explicitly bounded.
- Vanilla JavaScript requires discipline around modules, DOM safety, accessibility, and browser tests as interactions grow.
- A Spanner migration will still be real engineering: schema/index design, data export/import, retry behavior, IAM,
  consistency tests, rollout, and cost management.

## Rejected Alternatives

### Start with Spanner

Rejected for the initial version because it adds cloud provisioning, credentials, emulator/integration complexity, cost,
and distributed transaction behavior to local development before availability requirements justify them.

### Start with PostgreSQL or another server database

A sound future option, but it adds a local/deployed service and image lifecycle for a data set that currently fits an
embedded store. It does not better satisfy the stated Spanner direction than a clean repository contract does.

### SQLite

Also a credible embedded choice. bbolt was selected for its small transactional key/value model and straightforward static
Go deployment. This choice does not claim bbolt is wire- or schema-compatible with Spanner; portability comes from
behavioral adapters.

### SPA framework and Node production toolchain

Rejected for now because the initial UI does not need client-side routing/data complexity sufficient to offset a larger
dependency graph, build system, and patch surface.

### Browser-only local storage

Rejected because matches require authenticated shared state, server-side authorization, consistent persistence, backups,
and a deployment database path.

### Direct OIDC implementation in $OOSHDAQ

Rejected because deployment already has an identity-aware boundary and local development only needs deterministic
fictional users. Protocol handling, token validation, key rotation, callbacks, and logout would expand security scope. A
future direct OIDC adapter requires its own ADR and threat model.

## Revisit Triggers

Revisit this decision when evidence shows one or more of:

- required availability cannot tolerate a single pod or volume attachment;
- catalog/member volume makes bounded match reads exceed the latency or work budget;
- multi-region durability or concurrent replicas become requirements;
- the UI develops enough shared state or complex interaction that vanilla modules create repeated correctness problems;
- the trusted proxy identity model is unavailable or insufficient;
- bbolt backup, migration, or operational constraints outweigh its simplicity.

Revisiting a component does not erase the boundaries: transports, domain rules, services, and repositories should remain
separate unless a new ADR establishes a better tested model.
