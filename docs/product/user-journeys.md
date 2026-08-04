# User Journeys

These journeys define enduring member-visible behavior; labels may improve, but authorization and outcomes remain stable.

## 1. Start locally as a fictional member

1. A contributor starts $OOSHDAQ in local mock-auth mode.
2. The sign-in screen lists clearly fictional members.
3. The contributor chooses one member.
4. $OOSHDAQ opens that member's catalog dashboard.
5. Signing out clears rendered member data and returns to sign-in.

Mock login is unavailable in proxy-auth mode, and the browser cannot invent a member outside the configured local set.

## 2. Browse catalogs

1. An authenticated member opens the dashboard.
2. They browse catalog cards or search for a collectible by name.
3. A search result opens the relevant catalog with the same query applied.
4. Each collectible card prioritizes its name and artwork or a safe placeholder.
5. Pressing the artwork toggles **Have**.
6. **Available to swap** and **Want** remain explicit controls beneath it.
7. Items not on the member's shelf are visually muted.

Catalog and item order is stable, pages are bounded, and missing artwork never blocks collection management.

## 3. Maintain a collection

The member updates state directly on a card. **Have** records ownership. **Available to swap**
records a copy that may be exchanged or given away. **Want** records interest in another copy,
including when the member already owns one.

The resulting state follows these rules:

- available implies owned;
- available and wanted cannot be selected together;
- owned and wanted may be selected together;
- turning Have off clears availability but preserves Want; and
- the signed-in member, never a URL or body identity, selects the collection being changed.

The interface saves inline, reports failure on the affected card, and preserves keyboard focus for retry.

## 4. Discover possible swaps

Given that Alex wants Maui and Bailey has marked Maui **Available to swap**,
Alex sees a **Maui** group with Bailey beneath it.

If Bailey wants something Alex has available, the row names it. These fulfilable requests appear
before people who have not asked for anything back. Only a counterpart with no published wants
is described as not asking for anything in return.

If Bailey requests something Alex cannot offer, Bailey is not presented as a giveaway. The muted
**Swaps you can't fulfil yet** panel is always available and loads its results only when opened.
Loading or retrying it does not replace the actionable results already on screen.

Possible swaps:

- use current collection state and are not binding trades;
- never list the signed-in member as their own counterpart;
- avoid duplicate counterpart/item leads;
- show only the approved counterpart display identity;
- reveal at most one relevant counterpart request; and
- may disappear when either member changes the relevant state.

Members coordinate elsewhere; $OOSHDAQ does not reserve items or mark exchanges complete.
Matching order is defined in [ADR 0003](../architecture/adr/0003-ranked-split-match-feeds.md).

## 5. Find no match yet

If nobody has marked a wanted item available, the Matches view reports no possible swaps yet.
It does not imply that no copy exists or reveal unrelated holdings.

## 6. Return after a restart

1. A member updates their collection.
2. The process or container stops and a replacement uses the same data volume.
3. The collection and calculated matches remain consistent.

Brief downtime is acceptable; losing committed collection data is not. Catalog updates must not remap stored identifiers.

## 7. Sign in through an identity proxy

1. The member authenticates with the deployment's identity provider.
2. A trusted proxy removes browser-supplied identity headers.
3. The proxy supplies the authenticated subject and display name.
4. $OOSHDAQ resolves the minimal local member record.
5. Upstream tokens and claims are never returned to the browser.

Missing or invalid trusted identity fails closed. See the [authentication boundary](../architecture/authentication.md).
