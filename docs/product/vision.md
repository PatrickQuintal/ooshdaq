# Product Vision

## Problem

Collectible sets are fun to complete, but duplicates are useful only when people
can find them. Spreadsheets and group messages obscure who has a spare item.

$OOSHDAQ answers that question for a trusted, authenticated community while collecting as little personal data as possible.

## Product promise

A member can:

1. browse finite, rights-reviewed collectible catalogs from multiple brands;
2. mark what they have, what is available to swap, and what they want;
3. find people who have a wanted collectible available, even when nothing is requested in return; and
4. understand what a counterpart wants without confusing an unavailable return item with a giveaway.

$OOSHDAQ introduces compatible members; it does not negotiate, message, reserve, value, or settle an exchange.

## Principles

### Delight without ceremony

Collection management should feel visual and immediate. Artwork toggles ownership; availability
and wanted state remain explicit controls. Members do not create listings or enter a trade flow.

### Useful leads first

A lead exists when one member wants an item and another explicitly makes it available.
A return item strengthens a lead but is not required. Results should be stable and fair.

### Private-community defaults

Catalog, collection, and match data require authentication. Members see only an approved
counterpart display identity and coordinate outside $OOSHDAQ. This is not a public directory.

### Minimal data

Store an opaque identity subject, approved display label, collection state, and limited timing
data required for fair matching. Avoid contact details, biographies, chat, location, and child
identity data. Never expose private activity information to another member.

### Portable and proportionate

Run as a small self-hosted service with recoverable local persistence. Prefer explicit bounds,
indexed queries, and measured evidence. See the [operating requirements](operating-requirements.md).

## Scope

- multiple finite catalogs and brands;
- local fictional members and explicit mock authentication for development;
- trusted reverse-proxy identity for deployed environments;
- owned, available, wanted, and owned-and-wanted collection states;
- actionable, genuine no-request, and separately loaded unfulfillable leads;
- source-built local and single-instance deployments.

## Deliberate non-goals

- public signup or password management;
- OAuth or OIDC protocol handling inside $OOSHDAQ;
- chat, notifications, contact imports, presence, prices, bidding, payments, postage, or ratings;
- binding offers, reservations, or trade lifecycle tracking;
- authenticity, condition grading, dispute resolution, behavioral profiling, or recommendations; and
- collection of identity data about children.

## Success signals

- A contributor can run the app and use fictional members without external infrastructure.
- A member can maintain several catalogs and understand useful swap leads in one session.
- Restarts preserve committed collection state.
- One member cannot mutate another member's collection.
- Deployment authentication fails closed when trusted identity is absent.
- Catalog content remains usable without adding unlicensed third-party assets to the distributed source.
