# ADR 0005: Ranked and Split Match Feeds

- **Status:** Accepted
- **Date:** 2026-07-31
- **Amends:** Match ordering, loading, and cursor encoding in
  [ADR 0004](0004-simple-states-and-availability-leads.md)

## Context

One popular collectible may be available from hundreds of members. Many of
those members may have recorded a request the current member cannot fulfil.
Returning and enriching every such lead before finding useful swaps wastes
work, can bury actionable results, and can make an old abandoned request look
more relevant than a recently maintained collection.

Exact last-login ordering would create a different problem: it would reward
members for repeatedly opening the application and penalise someone whose
collection is still accurate but who has no reason to visit. Waiting time
should matter, but an untouched request must not gain priority forever.

## Decision

### Preserve state-transition times

`collection_entries` records separate `wanted_since_at` and
`available_since_at` timestamps.

- A timestamp is set when its quantity changes from zero to positive.
- It is preserved while that quantity remains positive, including when another
  compatible collection signal changes.
- It is cleared when the quantity returns to zero.
- Existing positive rows are backfilled from their durable `updated_at` value.

The generic row update time is not used as waiting age because changing
ownership while preserving a want must not make that want appear new.

`users.last_seen_at` is private ranking metadata. After authenticated
bootstrap, the browser sends a CSRF-protected activity `POST`; the repository
may advance the timestamp at most once per hour. Session reads remain
side-effect free. Exact activity timestamps are never returned to another
member or rendered in the browser.

### Use capped waiting age and a coarse stale signal

Within each result class, the repository prefers the oldest relevant state but
caps its ordering advantage at 60 days. Every state older than the cap has the
same waiting score.

Only after the capped waiting score does the repository apply a stale flag. A
lead is stale when both:

- its relevant `wanted_since_at` or `available_since_at` is more than 90 days
  old; and
- the counterpart's `last_seen_at` is more than 90 days old or absent.

The flag is deliberately coarse. Exact or frequent activity does not improve
rank. A member is not penalised merely for failing to revisit a still-young
request. Stable opaque IDs finish the ordering.

For a fulfilable or unfulfillable lead, the relevant state is the one
counterpart request selected for display. When several requests qualify, the
same capped-age, stale, and immutable-item ordering chooses one. For a genuine
no-request extra, the relevant state is when the offered item became
available.

### Query actionable and unfulfillable leads separately

`GET /api/v1/matches` has two internally selected, independently paginated
readiness feeds:

- `actionable` is the default and contains a fulfilable counterpart request or
  a genuine no-request extra; fulfilable requests rank before no-request
  extras.
- `unfulfillable` contains only counterparts who have a published request but
  no request the current member can currently offer.

The bundled browser loads only `actionable` when Matches opens. It renders a
plain-language control for **Swaps you can't fulfil yet** and requests the
`unfulfillable` feed only when the member opens that section. Each feed owns
its own opaque cursor, error, retry, and continuation state.

Ranking time is frozen into the encrypted and authenticated continuation
cursor so crossing a 60-day or 90-day boundary while paging does not duplicate
or skip results. Ranking timestamps and activity status remain server-private.

## Consequences

- Hundreds of unfulfillable candidates do not delay rendering the initial
  actionable page or consume browser memory unless the member asks to see
  them.
- An abandoned request stops accumulating priority after 60 days and loses a
  capped-age tie to a maintained profile after 90 days.
- A member with a still-young request is not demoted just because they have not
  repeatedly logged in.
- Match pagination is deterministic but has a richer encrypted position than
  the earlier counterpart/item tuple.
- SQLite stores three additional private timestamps and performs a
  rate-limited write on authenticated application bootstrap.
- Query-plan and high-fanout tests must cover both readiness feeds; normal
  capacity qualification must continue to use bounded response sizes.
