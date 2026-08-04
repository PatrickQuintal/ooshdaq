# ADR 0001: Go, Vanilla UI, and SQLite

## Context

$OOSHDAQ serves a small collectible community under the documented
[operating requirements](../../product/operating-requirements.md). Collection
data must remain durable, but the product does not require continuous
availability or concurrent application replicas.

The system should be easy for an operator to build, inspect, back up, and
restore without a frontend build service or separately managed database. The
domain and HTTP contract must remain independent of physical storage.

## Decision

Build one Go executable that:

- uses plain Go types for domain rules and application services;
- embeds static HTML plus vanilla CSS and JavaScript;
- uses standard-library HTTP facilities and a small direct dependency set;
- exposes storage through narrow, behavior-focused repository ports; and
- uses SQLite as the only durable application repository.

Use the pure-Go `modernc.org/sqlite` driver so normal builds remain
CGO-independent. Pin dependencies and review their licenses, maintenance,
transitive graph, and vulnerabilities under the supply-chain policy.

SQLite owns one database file opened by one application process. A deployment
runs one writable instance with exclusive durable storage. Use foreign
keys, explicit schema migrations, transactional catalog import, WAL
journaling, full synchronous durability, bounded connections, and bounded
busy waits. Startup validates and migrates before readiness succeeds.

Repositories map storage records to domain values. The SQLite adapter owns
SQL, transactions, migrations, backup mechanics, and classified storage
failures. Query-specific repository operations own bounded indexed match
discovery and ranking. Services must not rebuild those queries as all-user or
all-item scans. Tests exercise the same SQLite implementation used by the
application, using an isolated temporary database where persistence is not the
subject of the test.

Operators build the executable or container from source. The project does not
require a project-owned binary or image registry. Deployment-specific artifact
controls remain the operator's responsibility.

## Consequences

- Local and deployed persistence have the same transactional semantics.
- One process and database file keep operations and recovery understandable.
- Restart, replacement, or storage attachment can cause brief downtime.
- Durable volumes, online SQLite backups, integrity checks, and rehearsed
  isolated restores are mandatory because there is no database service
  failover.
- Match performance depends on reviewed indexes and bounded keyset queries;
  performance qualification is part of acceptance.
- Repository isolation costs mapping and contract-test code, but prevents
  HTTP and domain packages from depending on SQLite shapes.
- Vanilla browser code avoids a package toolchain but still requires modules,
  accessibility tests, and explicit state management.
- A pure-Go SQLite stack has a nontrivial transitive graph that must be kept
  under supply-chain review.

## Rejected Alternatives

### Managed PostgreSQL or globally distributed SQL by default

These add network dependencies, credentials, service provisioning, higher
recovery complexity, and cost without a measured need for multiple writers or
continuous availability.

### Multiple writable application replicas over one SQLite volume

Volume access, lock contention, and rollout behavior would violate the simple
single-writer model. More HTTP replicas require moving the write authority to
a database designed and operated for that topology.

### Storage-shaped domain and service APIs

Exposing tables, SQL positions, or driver errors would make persistence
changes affect security and product contracts. Portability comes from
behavioral boundaries, not a common physical schema.

### A frontend framework and bundler by default

The interaction surface does not justify a second dependency graph
and build pipeline. Reusable vanilla modules and semantic HTML are sufficient.

## Revisit When

Reconsider the database or replica model when measured contention violates
the accepted latency gates, downtime is no longer acceptable, recovery
objectives require managed failover, or the workload requires concurrent
writers. Reconsider the browser toolchain when interaction complexity cannot
be kept accessible and testable with small dependency-free modules.
