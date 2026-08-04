# ADR 0003: Ranked, Split Match Feeds

## Context

Availability can fan out across many counterparts. A single combined query
would let unfulfillable requests consume the actionable page and make an
empty actionable result expensive to prove. Pure oldest-first ordering also
lets abandoned state dominate indefinitely, while exact activity disclosure
would reveal more than the product needs.

## Decision

Query `actionable` and `unfulfillable` as separate, independently bounded
repository operations with independent encrypted keyset cursors.
`actionable` contains fulfillable requests followed by genuine no-request
leads. `unfulfillable` contains only a selected request the current member
cannot offer. A `(counterpart, receive item)` lead belongs to exactly one feed.

Rank each feed by this ascending tuple at a clock frozen on the first page:

```text
request class
capped relevant-state time
stale flag
request item ID
counterpart ID
receive item ID
```

Class zero is a displayed request; class one is a genuine no-request lead.
Relevant state is `wanted_since_at` for a displayed request and
`available_since_at` for a no-request lead. Cap waiting priority at 60 days so
all older states tie on age.

Set the stale flag only when the relevant state is older than 90 days and the
counterpart's private `last_seen_at` is also older than 90 days or absent.
Authenticated application bootstrap may advance `last_seen_at` at most once
per hour. Never project activity or state timestamps.

When several counterpart requests qualify, use the same capped-time, stale,
and item-ID ordering to choose one. Apply give-catalog and reciprocal-scope
filters only to fulfillable requests; receive-catalog filtering applies to
both feeds.

The cursor binds member, route, readiness, filters, limit, frozen clock, and
the six-part keyset position. It is not a snapshot; collection changes may
affect later pages.

## Consequences

- Actionable latency and page capacity are isolated from unfulfillable
  fanout.
- Members can load either feed without fetching or counting the other.
- Long-waiting members retain priority without allowing abandoned state to
  dominate forever.
- Activity is a coarse private ranking input, not presence or analytics.
- SQLite ordering needs focused contract, ranking, and query-plan tests.
- Indexed partial joins and keyset positions are more complex than an
  unbounded in-memory sort.

## Rejected Alternatives

- One combined feed: obscures readiness and couples its cost and pagination.
- Offset pagination: becomes unstable under writes and scales with offset.
- Unbounded all-user scans or browser ranking: violates response and trust
  boundaries.
- Exact waiting age or last-seen labels: disclose unnecessary private timing.
- Permanent oldest-first priority: lets abandoned rows lead indefinitely.

## Revisit When

Revisit the cap, stale threshold, or maintained projections only with measured
workload evidence and a privacy review. Revisit feed separation if the product
changes what “actionable” means or introduces durable commitments.
