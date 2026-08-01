# Operating Requirements

- **Status:** Accepted
- **Date:** 2026-07-30

This document is the canonical operating envelope for $OOSHDAQ. Architecture and load tests should target these
requirements instead of assuming public-marketplace traffic or uninterrupted availability.

## Community and Load

- The deployment may admit approximately 10,000 staff identities.
- Fewer than 100 members are expected to use the application concurrently under normal conditions.
- Traffic is read-heavy. Writes are small, explicit collection-entry updates rather than a continuous event stream.
- A signed-in member may browse, update a collection, and request bounded
  possible-swap leads; there is no chat, presence,
  notification fan-out, payment processing, or trade settlement workload.
- Load tests should use the expected interaction mix and bounded responses rather than treating every admitted identity as
  simultaneously active.

Ten thousand eligible identities is a capacity and data-retention requirement, not a requirement for 10,000 concurrent
connections.

## Catalog Scope

- $OOSHDAQ is a multi-brand, multi-catalog tracker.
- The first Woolworths Ooshies catalog is seed content, not the product boundary.
- Plan for at least five comparable catalogs from different brands, with room for more finite catalogs over time.
- Brands, catalogs or editions, catalog items, source records, and artwork provenance are distinct concepts.
- A member who wants an item can discover another member who marked it
  available without requiring a return item. When the counterpart wants
  something, one request is attached: a fulfilable item is preferred, while an
  unfulfillable request remains visibly distinct from a genuine no-return
  offer. The request may cross catalogs or brands. Actionable results load
  independently from the member-opened unfulfillable section.
- Collection storage is sparse: an all-zero `(user, item)` entry need not be stored.
- Catalog import is an offline, reviewed, idempotent workflow. The serving process does not scrape third-party sites.

Catalog growth must not reintroduce a request-time scan of every user
multiplied by every pair of catalog items. Catalog and match endpoints require
explicit result bounds, and matching should use indexed offers and wants.
Popular items with hundreds of offerers must not force the initial actionable
page to enrich or return every unfulfillable lead.

## Durability and Availability

- Collection data must survive process restarts, pod replacement, image rollout, and Kubernetes node rolls.
- The service is not operationally critical. Brief downtime while a single pod stops, a volume detaches and reattaches,
  or a replacement pod becomes ready is acceptable.
- Zero-downtime rollout, active-active replicas, multi-region service, and automatic database failover are not initial
  requirements.
- A persistent volume is durable state, not a backup. Operators need a defined backup schedule, retention policy,
  integrity check, and tested restore procedure.
- No numeric recovery-time or recovery-point objective is claimed until a deployment owner explicitly sets and tests one.

Availability work should remain proportional to this service. Do not add a distributed database, cache, queue, or
replication layer solely because the eligible community or catalog contains many records.

## Accepted Runtime Target

The production shape is:

```text
browser
   |
TLS ingress and identity-aware proxy
   |
ClusterIP Service
   |
one $OOSHDAQ Go pod
   |
SQLite database on one persistent block volume
```

The application remains one Go process with an embedded browser UI. SQLite provides transactions, relational catalog
data, indexed availability-lead queries, schema migrations, and one-file operations. Kubernetes runs one writable pod and
recovers it with the same claim after a restart or node roll.

The current implementation uses SQLite exclusively. The nonvaluable pre-release bbolt prototype was removed under the
approved clean-reset authority. There is no bbolt data migrator, store selector, legacy API, retained reader, or rollback
path.

See [ADR 0002](../architecture/adr/0002-sqlite-single-replica-multi-catalog.md).
The dependency-ordered delivery and evidence gates are in the
[SQLite and multi-catalog uplift plan](../implementation/sqlite-multi-catalog-uplift.md).

## Validation Envelope

Before the SQLite deployment is called production-ready, automated evidence should cover:

- approximately 10,000 synthetic users;
- at least five catalogs and a representative multi-catalog item count;
- sparse owned, available, and wanted entries;
- a read-heavy workload with up to 100 concurrent simulated members;
- bounded, indexed availability leads with fulfilable, unfulfillable, and
  genuine no-request cases;
- separate readiness-feed bounds, capped-wait ordering, coarse stale-record
  demotion, and a high-fanout all-unfulfillable fixture;
- pod/container replacement with the same volume;
- abrupt process termination followed by successful database recovery;
- a rollout or node-drain rehearsal with accepted downtime and preserved data;
- online backup, restore into an isolated environment, integrity verification, and a complete user journey.

Synthetic data only is permitted in performance and recovery tests.

## Revisit Triggers

Reconsider the single-pod SQLite decision only when evidence shows one or more of:

- measured write contention or query latency misses an agreed service objective after indexing and query work;
- expected concurrency materially exceeds the tested envelope;
- rollout or node-recovery downtime becomes unacceptable;
- two or more simultaneously serving application replicas become a requirement;
- regional continuity or a numeric availability objective is adopted;
- backup, restore, volume attachment, or database-size constraints become operationally burdensome.

Catalog count or eligible-user count alone is not a trigger. Record a new ADR before adopting a client/server or
distributed database.
