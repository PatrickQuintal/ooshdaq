# Data Model and Invariants

The names below describe domain concepts, not a required storage layout. Persistence records must be mapped at the
repository boundary.

## Implementation Status

The Go domain, SQLite schema, bounded API, and browser use explicit brands, catalogs, and globally unique catalog items.
The superseded flat-series and bbolt compatibility models have been removed.

See [operating requirements](../product/operating-requirements.md) and
[ADR 0002](adr/0002-sqlite-single-replica-multi-catalog.md). The
member-facing state and match semantics are refined by
[ADR 0004](adr/0004-simple-states-and-availability-leads.md) and ranked,
separately loaded feeds are defined by
[ADR 0005](adr/0005-ranked-split-match-feeds.md).

## Brand

A brand identifies the issuer or promoter of one or more catalogs. It is not interchangeable with a fictional universe,
artwork rights holder, retailer source page, or authentication tenant.

| Field | Meaning |
| --- | --- |
| `id` | Stable opaque identifier; never reused |
| `name` | Reviewed display name |
| `status` | Whether the brand may receive newly published catalogs |

## Catalog

A catalog is one finite collection or edition owned by a brand. A new annual edition is a new catalog rather than an
in-place reuse of identifiers.

| Field | Meaning |
| --- | --- |
| `id` | Stable opaque identifier; never reused |
| `brand_id` | Owning brand |
| `name` | Display name |
| `edition` | Optional reviewed year or edition label |
| `status` | Draft, active, or retired publication state |
| `sort_order` | Deterministic display order |

## Catalog Item

| Field | Meaning |
| --- | --- |
| `id` | Globally unique, immutable lowercase slug; never reused |
| `catalog_id` | Stable owning catalog identifier |
| `number` | Positive deterministic order within the catalog |
| `name` | Display name |
| `universe` | Optional grouping label such as a fictional universe |
| `variant` | Optional variant label |
| `image_url` | Optional reviewed artwork URL |
| `source_url` | Optional source page used to verify the catalog fact |

Renaming or reordering an item must not change its ID. Removing an item from active display retires it and must not cause
an existing collection entry to point to a different item. Item IDs are globally unique across every catalog;
`catalog_id` remains explicit ownership metadata rather than part of item identity.

## Artwork Provenance

| Field | Meaning |
| --- | --- |
| `source_url` | Page from which the asset or metadata originated |
| `asset_url` | Optional permitted remote or packaged asset |
| `rights_status` | `original`, `licensed`, `permissioned-link`, `source-hosted-link`, `unknown`, or `restricted` |
| `license` | SPDX identifier or a precise non-SPDX rights statement |
| `attribution` | Required reviewed attribution metadata |
| `retrieved_at` | Date the source was checked |
| `checksum` | Digest for a packaged asset |

Provenance is associated with the specific brand, catalog, item, and asset it supports. Every added catalog receives an
independent rights decision and manifest; approval for one brand does not carry to another.

The initial 2026 catalog uses reviewed, source-hosted HTTPS links for the 40 individual character renders exposed by
Woolworths' collection page. The browser falls back to its bundled placeholder when a remote image is unavailable. No
third-party image bytes are stored in the repository or container. See
[artwork policy](../catalog/artwork-provenance.md) and the
[2026 provenance manifest](../catalog/disney-ooshies-2026-artwork.json).

## User

| Field | Meaning |
| --- | --- |
| `id` | Stable identity key: a seed ID locally or the trusted proxy subject in deployment |
| `display_name` | Deployment-approved label shown on relevant possible-swap leads |
| `last_seen_at` | Private, rate-limited application-bootstrap timestamp used only as a coarse stale-record signal |

`id` is unique within a $OOSHDAQ installation. A deployment must give its proxy subjects one stable namespace; supporting
multiple issuers would require a composite identity design and migration. Do not use a display name as identity. Email,
tokens, group lists, and raw upstream claims are not stored by default.
`last_seen_at` is not returned to another member and exact activity never
appears in match ordering copy.

## Collection Entry

One entry is identified by `(user_id, item_id)`. The globally unique item ID makes catalog identity unambiguous, while
the referenced item still carries `catalog_id`.

| Field | Meaning |
| --- | --- |
| `owned` | Quantity physically owned |
| `available` | Quantity the member explicitly offers |
| `wanted` | Quantity the member wants to acquire |
| `available_since_at` | Stable start of the current positive available state; empty when available is zero |
| `wanted_since_at` | Stable start of the current positive wanted state; empty when wanted is zero |

Initial invariants:

```text
0 <= owned <= MAX_QUANTITY
0 <= available <= owned
0 <= wanted <= MAX_QUANTITY
wanted > 0 => available = 0
```

`MAX_QUANTITY` is the server-side constant `1,000,000` and must not be
client-controlled. `owned` describes the member's current copies while
`wanted` describes additional copies they would like to acquire, so both may
be positive for a collector accumulating one design. A copy cannot be offered
and wanted at the same time.

Numeric fields remain available for future quantity work, but the initial
browser writes only:

```text
neutral          (owned=0, available=0, wanted=0)
owned            (owned=1, available=0, wanted=0)
available        (owned=1, available=1, wanted=0)
wanted           (owned=0, available=0, wanted=1)
owned and wanted (owned=1, available=0, wanted=1)
```

If all three quantities are zero, the repository may omit the entry while the
API still projects zeroes.

Changing another compatible signal does not reset either positive-state
timestamp. A timestamp is replaced only on a zero-to-positive transition and
cleared on a positive-to-zero transition.

Updating one entry is atomic. A failed validation or persistence operation
leaves its previous value and state-transition times unchanged.

## Possible-Swap Lead

A lead is a derived projection, not durable trade state.

For current member `A` and counterpart `B`, an opportunity for item `Y`
exists when:

```text
A.wanted(Y) > 0
B.available(Y) > 0
A != B
```

No reciprocal item is required. This includes a counterpart who has an extra
available and has not marked anything wanted.

A lead also projects at most one published item `X` that `B` wants. A lead
with a request `A` can offer belongs to the `actionable` feed:

```text
A.available(X) > 0
B.wanted(X) > 0
X != Y
```

If none is fulfilable but `B` has a published want, the lead belongs only to
the `unfulfillable` feed and projects one such request with zero offerable
quantity. Only when `B` has no published wants is the request omitted; that
genuine no-request lead belongs to `actionable`. The classes are disjoint, so
one `(counterpart, receive item)` lead never appears in both feeds.

`X` and `Y` may belong to the same catalog or to different brands and
catalogs. A request may filter these catalogs, but must not change the
underlying meaning of an offer or want. Receive and immediately fulfilable
request quantities are bounded independently; an unfulfillable request has a
zero offerable quantity.

Actionable fulfilable requests precede genuine no-request extras. Within a
class, waiting age is oldest first but saturates after 60 days. A coarse stale
flag is applied next only when both the relevant state and the counterpart's
last activity are older than 90 days. Immutable item, counterpart, and receive
IDs break remaining ties. The same capped-age rule selects one request when
several qualify; a giveaway uses `available_since_at` as its relevant state.

Exactly one suggestion is emitted for each unique
`(counterpart_id, receive_item_id)` lead in its readiness class. A result ID is
derived from the current user, counterpart, and received item; it is not
durable trade state. The ranking clock is frozen into an opaque encrypted and
authenticated cursor for stable continuation.

## Repository Operations

The application needs behavior equivalent to:

- resolve or create a minimal user from a trusted principal and rate-limit its
  private last-seen update;
- list brands and active catalogs in stable, bounded order;
- list or page one catalog's items in stable order;
- get one user's bounded collection projection for a selected catalog;
- atomically put one collection entry after validation;
- find independently bounded actionable and unfulfillable leads through
  indexed wants and availability, attach at most one ranked request, and
  preserve deterministic cursors and catalog filters;
- read repository health and validate the schema version while opening the store.

These are behavioral ports, not one broad CRUD interface. The memory and SQLite adapters share applicable conformance
tests. Match results are suggestions that can already be stale when returned, never reservations.

## SQLite Durable Layout

The relational shape includes:

```text
brands
catalogs
catalog_items
catalog_sources
item_assets
users
collection_entries
schema_migrations
catalog_imports
```

Required properties include:

- primary and foreign keys enforce brand, catalog, item, user, and collection-entry identity;
- a unique `(user_id, item_id)` key referencing a globally unique, immutable catalog-item ID;
- sparse collection entries, deleting the row when all three quantities are zero;
- indexes supporting one user's collection, positive state-transition times,
  and items with `available > 0` or `wanted > 0`;
- separate bounded indexed joins for ranked actionable and unfulfillable
  availability leads;
- transactional, idempotent catalog imports and explicit schema migrations;
- retired catalogs and items remain addressable by existing collection records;
- schema and foreign-key validation complete before readiness succeeds.

SQLite connection, journaling, checkpoint, backup, and single-writer volume requirements are defined in
[ADR 0002](adr/0002-sqlite-single-replica-multi-catalog.md).
