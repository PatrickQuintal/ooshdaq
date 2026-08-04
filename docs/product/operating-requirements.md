# Operating Requirements

This is the canonical operating envelope for $OOSHDAQ. Architecture, tests, and deployment
choices should target it rather than assume public-marketplace traffic or uninterrupted availability.

## Community and load

- A deployment may admit approximately 10,000 staff identities.
- Fewer than 100 members are expected to use the application concurrently.
- Traffic is read-heavy; writes are small, explicit collection updates.
- Responses and browser pages are bounded.
- There is no chat, presence, notification fan-out, payment, or settlement workload.

Ten thousand eligible identities is a capacity and retention requirement, not 10,000 concurrent
connections. Load evidence should use a representative interaction mix.

## Catalog and query scale

- The service supports at least five finite catalogs across multiple brands.
- No single catalog or brand defines the product boundary.
- Collection storage is sparse; an all-zero member/item row need not persist.
- Catalog import is offline, reviewed, deterministic, and idempotent.
- Catalog and match queries use explicit limits and opaque continuation cursors.
- Match discovery uses indexed availability and wanted state, not an all-user, all-item scan.
- Actionable and unfulfillable feeds are independently bounded.

A popular item may have hundreds of offerers; its initial page must not enrich every candidate or wait for unfulfillable leads.

## Durability and availability

- Committed collection data survives process restarts, application replacement,
  and host maintenance when the same durable storage is retained.
- Brief downtime while the application or its storage moves is acceptable.
- Zero-downtime rollout, active-active serving, multi-region continuity, and automatic database failover are not requirements.
- Durable application storage is not a backup.
- Operators define backup frequency and retention, verify integrity, and test restoration.
- No recovery-time or recovery-point objective is implied without an explicit operator decision.

Do not add a distributed database, cache, queue, or replication layer merely because the community or catalogs contain many records.

## Runtime shape

```text
browser
   |
TLS termination and identity-aware proxy
   |
one $OOSHDAQ process or container
   |
SQLite database on exclusive durable storage
```

The application is one Go process with an embedded browser UI. SQLite provides transactions,
relational catalog data, indexed matching, migrations, and one-file operations. The hosting
platform runs only one writable instance and retains or reconnects its storage after replacement.

## Browser support

The bundled interface supports desktop viewports at least 900 CSS pixels wide.
Desktop keyboard, pointer, focus, forced-color, and error-recovery behavior remain
supported. Mobile and narrower layouts are unsupported: incidental rendering does
not create a compatibility commitment, dedicated navigation, or release gate.

## Validation envelope

Evidence should cover:

- synthetic users and concurrency matching the community and load envelope;
- a multi-brand catalog set with sparse collection state;
- bounded actionable and unfulfillable feeds, including popular-item fan-out;
- concurrent reads and collection updates under the expected workload;
- restart and abrupt-termination recovery;
- replacement using the same durable data path; and
- online backup, isolated restore, integrity verification, and a complete user journey.

Performance and recovery evidence uses synthetic data only.

## Revisit triggers

Record a new architecture decision before changing the runtime shape. Revisit single-instance
SQLite only when evidence shows write contention, missed latency objectives, concurrency beyond
the tested envelope, unacceptable recovery downtime, a need for simultaneous serving replicas,
a regional continuity objective, or operationally burdensome storage and restoration.

Catalog count or eligible-user count alone is not a trigger.
