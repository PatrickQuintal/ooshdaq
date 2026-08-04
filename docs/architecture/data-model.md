# Data Model and Invariants

These are domain concepts, not a prescribed storage schema. Repository
adapters map durable records to these values and enforce equivalent
constraints transactionally. Wire projections are defined separately in the
[HTTP API](http-api.md).

## Catalog Identity

A brand identifies the issuer of one or more finite catalogs. A catalog is one
edition owned by a brand. A catalog item belongs to exactly one catalog.

| Type | Stable fields | Mutable or descriptive fields |
| --- | --- | --- |
| Brand | `id` | `name`, publication `status` |
| Catalog | `id`, `brand_id`, published `sort_order` | `name`, optional `edition`, `status` |
| Catalog item | globally unique `id`, `catalog_id`, published `number` | `name`, optional `universe` and `variant`, `status`, `retirement`, `artwork` |

IDs are lowercase slugs, immutable, and never reused. Item numbers are
positive and unique within a catalog. Draft, active, and retired are explicit
publication states. A retired item requires an effective date and reason;
non-retired items have no retirement record. Retired catalogs and items remain
addressable so existing collection entries keep their meaning.

Catalog manifests are complete, independently versioned snapshots. Each
manifest contains one brand, one catalog, and between one and 10,000 items.
Import validates ownership, unique IDs and numbers, publication state,
retirement data, and artwork before one transactional update. Items must be
explicitly retired rather than silently disappearing.

## Artwork

Each catalog item carries an `Artwork` value:

| Part | Fields |
| --- | --- |
| Reference | optional `asset_url`; explicit `placeholder` flag |
| Rights | `status`, optional `license`, `attribution`, and `permission_reference` |
| Provenance | optional `source_page_url` and `retrieved_at`; required `reviewed_at`, `review_reference`, and `transformation`; optional `sha256` |

Rights status is one of `original`, `licensed`, `permissioned-link`,
`source-hosted-link`, `unknown`, or `restricted`. Publishable artwork requires
a reviewed HTTPS asset and source, retrieval date, attribution, and the
rights-specific license or permission evidence. Unknown or restricted rights
require the local placeholder and prohibit an asset URL. A SHA-256 digest is
recorded only when bytes were reviewed or packaged. See the
[artwork policy](../catalog/artwork-provenance.md).

## User

| Field | Meaning |
| --- | --- |
| `id` | Stable local seed ID or trusted proxy subject |
| `display_name` | Bounded, escaped display label; never authorization identity |
| `last_seen_at` | Private, rate-limited activity metadata used only for coarse match ranking |

An installation uses one stable, collision-free subject namespace. Tokens,
email addresses, group claims, and raw identity headers are not domain data.
The display name may change. Exact activity is neither a presence feature nor
part of any member-facing projection.

## Collection Entry

One entry is identified by `(user_id, item_id)` and stores:

| Field | Meaning |
| --- | --- |
| `owned` | Copies physically owned |
| `available` | Owned copies explicitly offered |
| `wanted` | Additional copies wanted |
| `available_since_at` | Start of the current positive available state |
| `wanted_since_at` | Start of the current positive wanted state |

The invariants are:

```text
0 <= owned <= 1,000,000
0 <= available <= owned
0 <= wanted <= 1,000,000
available > 0 => wanted = 0
```

Owned and wanted may both be positive when a member wants additional copies.
Available and wanted are mutually exclusive. If every quantity is zero, the
repository may omit the row while callers still receive a canonical zero
projection.

A zero-to-positive transition sets the corresponding `since` timestamp. A
positive-to-zero transition clears it. Changes that leave the state positive
preserve it. Validation and replacement of one entry are atomic; failure
leaves quantities and transition times unchanged.

## Possible-Swap Lead

A lead is derived, not persisted. For current member `A`, counterpart `B`, and
receive item `Y`, a lead exists when:

```text
A.wanted(Y) > 0
B.available(Y) > 0
A != B
```

No reciprocal request is required. At most one published item `X` wanted by
`B` is attached to the lead:

- `actionable` contains a request that `A` can fulfil
  (`A.available(X) > 0`), or a genuine no-request lead when `B` has no
  published wants;
- `unfulfillable` contains one request when `B` has published wants but `A`
  can fulfil none of them.

The feeds are disjoint. `X` and `Y` may be in the same catalog or different
catalogs. Receive quantity is bounded by `A`'s want and `B`'s availability;
request quantity is bounded by `A`'s current availability and `B`'s want, and
is zero in the unfulfillable feed.

Exactly one suggestion exists per `(counterpart_id, receive_item_id)` in its
readiness class. Its ID is a deterministic projection key, not trade state.
Selection, ranking, privacy, and pagination are defined in
[ADR 0003](adr/0003-ranked-split-match-feeds.md) and the
[HTTP API](http-api.md).
