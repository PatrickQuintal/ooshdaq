# ADR 0004: Simple Collection States and Availability Leads

- **Status:** Accepted
- **Date:** 2026-07-31
- **Supersedes:** The reciprocal-only match requirement and quantity-first
  browser interaction in [ADR 0002](0002-sqlite-single-replica-multi-catalog.md)
  and [ADR 0003](0003-clean-reset-modernc-and-bounded-api.md)
- **Amended by:** Capped-wait ordering and separate readiness feeds in
  [ADR 0005](0005-ranked-split-match-feeds.md)

## Context

The quantity editor exposed more detail than the initial product needs. A
member's immediate questions are whether they have a collectible, have one
available, or want one. Offering and wanting the same copy produces an
incoherent initial state. Ownership and desire are different: a collector may
already own a figure and still want more copies of that design.

The reciprocal-only matcher hid useful connections. If Bailey has Maui
available and Alex wants Maui, that is already a useful lead even when Alex
has not marked something Bailey wants. A member may also offer an extra
without asking for anything in return.

Artwork rights and provenance remain important input controls, but their
license-review strings do not help a member manage a collection.

## Decision

### Present simple composable collection signals

The browser presents five canonical combinations per member and item:

| State | Stored `owned` | Stored `available` | Stored `wanted` |
| --- | ---: | ---: | ---: |
| Neutral | 0 | 0 | 0 |
| Owned | 1 | 0 | 0 |
| Available | 1 | 1 | 0 |
| Wanted | 0 | 0 | 1 |
| Owned and wanted | 1 | 0 | 1 |

The storage and HTTP fields remain non-negative integers so a future,
explicitly designed quantity feature does not require an immediate schema
replacement. Server-side rules continue to enforce `available <= owned` and
require `wanted = 0` whenever `available > 0`. `owned` and `wanted` may both be
positive: the first records current copies and the second records additional
copies the member would like.

The collectible artwork is an accessible pressed-state button for **Have**.
The browser keeps **Available to swap** and **Want** as the two explicit
checkboxes beneath it rather than opening a quantity dialog. Selecting
**Available to swap** clears **Want**; selecting **Want** clears
**Available to swap** but preserves **Have**. Pressing the artwork preserves
the wanted choice. An item the member does not have is visually muted;
available and wanted remain visually distinct.

Member-facing summaries show available and wanted design counts. Owned,
active-catalog, active-item, and catalogs-started statistics remain available
internally where useful but are not primary interface metrics.

### Discover availability leads

A possible-swap lead exists when:

```text
A.wanted(Y) > 0
B.available(Y) > 0
A != B
```

No return item is required. This covers swaps that have not yet become
reciprocal and extras offered without a recorded want.

For each lead, the repository looks for a published item the counterpart wants.
It first prefers a request the current member can fulfil:

```text
A.available(X) > 0
B.wanted(X) > 0
X != Y
```

If no such `X` exists but `B` has another published wanted item, the result
still includes one deterministic requested item marked unfulfillable. The
request is absent only when `B` has no published wants. `X` and `Y` may belong
to different catalogs or brands. The required **you receive** leg remains `Y`.

The repository performs this as a bounded, indexed query. It returns only the
counterpart identity, the item they have available, and at most one item they
have explicitly wanted. A fulfilable request is preferred; otherwise the one
deterministic unfulfillable request prevents the UI from incorrectly calling
the offer a giveaway. It does not reveal unrelated collection state. The
summary exposes a count of distinct
`(counterpart, wanted item)` leads for the navigation badge.

The bundled browser does not expose repository filters such as give catalog,
receive catalog, or reciprocal scope. It groups results by the collectible the
current member wants. Under that collectible, each person is described in
plain language as either wanting one named available item from the current
member or not having asked for anything in return. Fulfillable requests appear
first, followed by genuine no-return results. Leads with a request the current
member cannot fulfil appear in a separate muted **Swaps you can't fulfil yet**
section with the requested collectible; they are never described as a
giveaway. ADR 0005 makes that section an independently paginated feed which is
queried only when the member opens it.

A lead is not a reservation, promise, completed trade, or guarantee that an
item is free. Members agree on the exchange through their approved external
channel.

### Keep catalog discovery proportional

The bundled dashboard does not expose brand and publication filters for the
small catalog set. It offers one **Find a collectible** search instead. The
catalog query may retain broader API compatibility, but the member-facing
purpose is to find which catalog contains a collectible name. Once inside a
catalog, the existing collectible search and collection-state controls remain
available.

### Keep provenance out of the collection workflow

Catalog manifests still require rights and provenance review, and the server
continues to omit unsafe artwork URLs. The collection cards, catalog header,
and match cards do not render license classifications, source-hosting labels,
or review dates.

### Restore a playful visual language

The shell, placeholder, and empty states use original happy bean mascots.
These assets are project-owned decoration and do not copy third-party
collectible artwork. Motion remains optional and respects reduced-motion
preferences.

## Consequences

- The common owned update is one press on the artwork; availability and wants
  remain explicit choices rather than a modal form.
- Members can record that they own a design and still want additional copies.
- Available and wanted cannot conflict for one member/item.
- Members see useful availability immediately without constructing a complete
  barter pair.
- A counterpart's unfulfillable request is visible without exposing their
  wider collection, and is not misrepresented as asking for nothing.
- Members browse matches as wanted collectibles and people, not as database or
  catalog-filter terminology.
- Reciprocal cross-catalog opportunities remain visible as stronger leads.
- Existing rows that conflict with the new invariant must be normalized during
  migration before the stricter database constraint is applied.
- Earlier reciprocal-only qualification evidence remains historical; current
  tests and qualification must cover both one-way and reciprocal results.
