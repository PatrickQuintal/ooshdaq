# Product Vision

## Problem

Collectible sets are fun to complete, but duplicates are only useful when
people can find them. Informal spreadsheets and group messages make it hard to
answer a simple question: “Who has a spare item I want?”

$OOSHDAQ answers that question for a trusted, authenticated community while collecting as little personal data as possible.

## Product Promise

A member can:

1. browse finite, attributed collectible catalogs from multiple brands;
2. press a collectible's artwork to toggle ownership, mark it available, or
   independently record that they want more with one inline action;
3. browse matches grouped by the collectible they want and see another member
   who has it available, even when no return item has been requested; and
4. see an optional reciprocal item when the other member wants something they
   have available, including across catalogs, or see the requested item muted
   in a separate section when they cannot currently fulfil it.

The application introduces compatible members. It does not negotiate, message, reserve, value, or settle a trade.

## Principles

### Delight without complexity

The collection view should feel visual and quick. The artwork itself toggles
ownership; updating an item should not require a multi-step listing workflow.
The Matches view should speak in terms of wanted collectibles and people, not
catalog directions or reciprocal-query terminology.

### Explicit availability without barter ceremony

Owning an item does not automatically advertise it. A useful lead exists when
one member explicitly wants an item and another explicitly marks it available.
The available member does not need to request anything in return. A reciprocal
item makes the lead stronger but is not required. If they have asked for
something the current member cannot offer, that request is shown separately
rather than being mistaken for a giveaway. That secondary section is checked
only when the member opens it, so a popular collectible does not bury or delay
useful results.

### Fair waiting without rewarding app opens

Longer-standing requests receive priority, but the advantage stops growing
after 60 days. Exact login frequency is not a ranking signal. Only a coarse
90-day stale-record check breaks capped-age ties, reducing abandoned leads
without penalising someone for not repeatedly opening a still-current
tracker.

### Private-community defaults

Catalog browsing may be deployment-configurable, but collection and match data require authentication. Members see only
the counterpart identity needed to make contact through an approved external channel. $OOSHDAQ is not a public directory.

### Minimal data

Store an opaque identity subject, a display label approved by the deployment,
collection state, stable state-transition times, and a private rate-limited
last-seen timestamp. Avoid email addresses, phone numbers, free-form
biographies, chat history, location, or child identity data. Never expose
exact activity to another member.

### Catalogs are data with rights

Characters, product names, images, and descriptions can be protected by copyright or trademark. A catalog entry must carry
source and rights metadata. The Apache-2.0 license for this repository does not relicense third-party material.

### Portable persistence

Use SQLite on a persistent volume for a simple, recoverable single-replica deployment. Brief downtime during pod
replacement and node rolls is acceptable; collection durability and tested restoration are required. Domain and
application code depend on repository interfaces so a client/server database remains possible if measured needs change.

### Proportionate operations

Approximately 10,000 staff may be eligible, but normal concurrency is expected to remain below 100 members. Prefer indexed
queries, explicit bounds, and load evidence over distributed infrastructure. See
[operating requirements](operating-requirements.md).

## Initial Scope

- at least five finite catalogs from multiple brands, beginning with Ooshies
- seeded local users and an explicit mock-login development flow
- trusted reverse-proxy identity in deployed environments
- simple owned, available, and wanted signals—including owning one design
  while wanting more—backed by bounded numeric storage
- availability-lead discovery with fulfilable, unfulfillable, and genuine
  no-request cases across catalogs, with unfulfillable leads loaded separately
- local container and recoverable single-replica Kubernetes deployment

## Deliberate Non-Goals

- public signup or password management
- OAuth/OIDC protocol handling inside $OOSHDAQ
- chat, notifications, contact imports, or presence
- prices, bidding, payments, postage, or ratings
- binding offers, reservations, or trade lifecycle tracking
- item authenticity, condition grading, or dispute resolution
- recommendations based on behavioral profiling
- direct use by or collection of identity data about children

## Success Signals

For the initial release:

- a new contributor can start the app locally and use seeded members without external infrastructure;
- a member can update multiple catalogs and discover available wanted items,
  including stronger fulfilable requests and clearly separated unfulfillable
  requests, in one browser session;
- restarting the app or replacing its pod with the same volume preserves collection state;
- one member cannot mutate another member's collection;
- deployment mode cannot enable mock login and cannot trust forwarded headers from arbitrary clients;
- every third-party image reference has a reviewed provenance and rights
  decision without cluttering the member collection workflow.
