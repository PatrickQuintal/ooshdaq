# Bounded Product API Contract (`/api/v1`)

- **Status:** Implemented
- **Date:** 2026-07-31
- **Authority:** [ADR 0003](../architecture/adr/0003-clean-reset-modernc-and-bounded-api.md)
  as amended by
  [ADR 0004](../architecture/adr/0004-simple-states-and-availability-leads.md)
  and [ADR 0005](../architecture/adr/0005-ranked-split-match-feeds.md)

This document fixes the product-facing JSON contract implemented together by the SQLite repository, HTTP handlers, and
bundled browser.

Authentication bootstrap and logout retain the boundary in the
[authentication document](../architecture/authentication.md). Every route below requires an authenticated member, and
the mutation also requires the established CSRF cookie/header pair. Brand records are nested in catalog projections;
they are not an independent product route.

## Route Set

The product route set is exhaustive:

| Method | Route | Default limit | Maximum limit |
| --- | --- | ---: | ---: |
| `GET` | `/api/v1/catalogs` | 20 | 50 |
| `GET` | `/api/v1/catalogs/{catalog_id}/collection` | 40 | 100 |
| `GET` | `/api/v1/collection/summary` | — | — |
| `PUT` | `/api/v1/collection/{item_id}` | — | — |
| `GET` | `/api/v1/matches` | 20 | 50 |
| `POST` | `/api/v1/activity` | — | — |

An unlisted product path receives the router's ordinary `404 not_found` response. Unsupported methods on a listed path
receive `405 Method Not Allowed` with an `Allow` header.

## Common JSON Rules

- Field names are `snake_case`. Successful bodies contain only the envelopes defined below.
- Responses use `Content-Type: application/json; charset=utf-8` and `Cache-Control: no-store`.
- Optional fields are omitted. `page.next_cursor` is the only nullable field in a successful response.
- Arrays and required objects are present even when empty.
- Brand, catalog, and item IDs are immutable, case-sensitive lowercase slugs. Clients treat all IDs as opaque.
- Brand, catalog, and item IDs in queries or paths match `^[a-z0-9](?:[a-z0-9-]{0,126}[a-z0-9])?$`.
- Member and derived match IDs are case-sensitive opaque strings and are never accepted as product query input.
- Counts and quantities are non-negative JSON integers. A quantity cannot exceed `1,000,000`.
- Dates are `YYYY-MM-DD`.
- Mutable names never determine result order or cursor position.

### Error envelope

Every error uses:

```json
{
  "error": {
    "code": "invalid_query",
    "message": "limit must be an integer from 1 through 50.",
    "field": "limit"
  }
}
```

`code` and, when present, `field` are stable. `message` is safe member-facing text but is not a byte-stable assertion
surface. `field` is omitted when no one request field caused the failure.

Common status mappings are:

| Status | Code | Use |
| ---: | --- | --- |
| 400 | `invalid_query` | A known query value is malformed, out of range, or repeated |
| 400 | `unknown_query_parameter` | The query contains a name not accepted by that route |
| 400 | `cursor_conflict` | `cursor` is combined with another query parameter |
| 400 | `invalid_cursor` | The cursor is malformed, too large, expired, tampered with, or bound elsewhere |
| 400 | `invalid_request` | A mutation body is malformed, null, incomplete, has unknown fields, or has trailing JSON |
| 401 | `authentication_required` | No authenticated member was resolved |
| 403 | `csrf_failed` | The mutation's CSRF proof is absent or invalid |
| 404 | `not_found` | A path catalog or item is not published or does not exist |
| 413 | `request_too_large` | A JSON body exceeds 16 KiB |
| 415 | `unsupported_media_type` | A mutation is not `application/json` |
| 422 | `validation_failed` | Quantities violate a domain bound or invariant |
| 503 | `service_unavailable` | The repository cannot safely serve the operation |

A valid query with no matches is `200 OK` with an empty array and `next_cursor: null`.

### Query parsing

All accepted query parameters are scalar.

1. An unknown name returns `unknown_query_parameter`; `field` is that name.
2. Repeating a known name returns `invalid_query`; `field` is that name.
3. Supplying `cursor` with any other name returns `cursor_conflict`; `field` is `cursor`.
4. An invalid continuation returns `invalid_cursor`; `field` is `cursor`.
5. Any other invalid value returns `invalid_query`; `field` names that parameter.

`q` is trimmed. An empty result is normalized as though `q` were absent. It may contain at most 100 Unicode code points.
Search compares the Unicode-lowercased query as a literal substring of Unicode-lowercased reviewed fields; `%` and `_`
have no wildcard meaning. Other blank parameter values are invalid. Defaults apply only when a parameter is absent.

### Page envelope

Every paginated response includes:

```json
{
  "page": {
    "limit": 40,
    "returned": 2,
    "next_cursor": null
  }
}
```

`returned` equals the length of that response's resource array. `next_cursor` is an opaque string only when another page
was known to exist when the query ran. There is no total count and no server-issued previous cursor.

### Cursor contract

The first request supplies filters and an optional `limit`, but no `cursor`. A
continuation supplies only `cursor`; the server recovers the normalized
filters and effective limit from the encrypted and authenticated token.

The token is one no-padding Base64URL value containing a random AES-GCM nonce
and an AES-256-GCM encrypted and authenticated payload. The cursor key is
SHA-256-derived into the AES key; route and authenticated member are also
authenticated as associated data. Plaintext filters, identity, ranking
timestamps, and keyset values do not appear in the token.

The encrypted payload contains only:

- format version;
- route binding, including the scoped catalog ID where applicable;
- authenticated member ID;
- normalized filters, including defaults;
- effective limit;
- expiry instant; and
- the last deterministic keyset ordering tuple.

The process generates a fresh random key of at least 256 bits at startup. A
cursor expires 15 minutes after issue, and a token of more than 4,096 bytes is
invalid. Clients must treat it as opaque.

The cursor contains no mutable catalog or collection counters and creates no database snapshot. It gives deterministic
keyset traversal while the ordered rows are unchanged. Concurrent collection changes can affect later match pages.
Restarting the process invalidates outstanding cursors. Every such failure is the safe `400 invalid_cursor` response; the
browser returns to page one.

Ordering tuples are:

| Route | Ascending tuple |
| --- | --- |
| Catalogs | `(sort_order, catalog_id)` |
| Scoped collection | `(item_number, item_id)` |
| Actionable matches | `(request_class, capped_since_epoch, stale_flag, request_item_id, counterpart_id, receive_item_id)` at the cursor's frozen ranking time |
| Unfulfillable matches | `(request_class, capped_since_epoch, stale_flag, request_item_id, counterpart_id, receive_item_id)` at the cursor's frozen ranking time |

Catalog `sort_order` and item `number` become immutable when first published. IDs are never reused. Comparisons of ID
components use ascending UTF-8 byte order.

For match positions, class `0` is a displayed counterpart request and class
`1` is a genuine no-request extra. `request_item_id` is empty only for class
`1`. `capped_since_epoch` is the later of the relevant state transition and
the frozen ranking time minus 60 days, so every older state has equal waiting
priority. `stale_flag` sorts maintained rows before rows whose relevant state
and absent-or-old activity are both more than 90 days old.

## Shared Projections

### Artwork

Every catalog and item has an `artwork` object:

| Field | Presence | Meaning |
| --- | --- | --- |
| `image_url` | Optional | Server-approved HTTPS URL or same-origin absolute path |
| `alt_text` | Required | Non-empty description appropriate for the projected image or fallback |
| `width`, `height` | Optional pair | Positive pixel dimensions; both appear or neither appears |
| `source_url` | Optional | Reviewed source URI |
| `attribution` | Required | Validated source or creator credit retained as metadata; the collection workflow does not render it |
| `rights_status` | Required | `original`, `licensed`, `permissioned-link`, `source-hosted-link`, `unknown`, or `restricted` |
| `license` | Optional | SPDX identifier or reviewed rights statement |
| `retrieved_at` | Required | Date the source and publication decision were reviewed |

For `unknown` or `restricted`, `image_url`, `width`, and `height` are omitted. The browser then uses its bundled
placeholder. `source-hosted-link` records a reviewed link to an asset hosted by the source owner; it grants no
redistribution right and $OOSHDAQ does not copy those bytes. A manifest URL is never published merely because it parsed;
allowlisting and the independent provenance review in the
[artwork policy](../catalog/artwork-provenance.md) remain server responsibilities.
The browser uses the approved image and alt text but does not render the
source URL, rights classification, license, attribution, or review date in the
collection and possible-swap workflow.

### Member summary

Catalog and overall summaries use the same six current-member measures:

```json
{
  "owned_quantity": 8,
  "owned_designs": 5,
  "available_quantity": 4,
  "available_designs": 3,
  "wanted_quantity": 4,
  "wanted_designs": 3
}
```

A quantity is the sum of that field. A design count is the number of distinct items for which that field is positive.
Catalog summaries cover all published items in that catalog and are not changed by list filters.

### Catalog

A catalog projection contains:

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | string | Immutable catalog ID |
| `brand` | object | Required `id` and `name` |
| `name` | string | Reviewed catalog name |
| `edition` | string, optional | Reviewed edition label |
| `publication_status` | string | `active` or `retired` |
| `sort_order` | integer | Immutable published navigation order |
| `item_count` | integer | Active plus retired published items; draft items are excluded |
| `universe_options` | string array | Exact filter values, sorted by ascending UTF-8 bytes |
| `artwork` | object | Artwork projection above |
| `member_summary` | object | Six unfiltered member measures above |

Draft records never appear. A retired catalog remains explicitly browseable and preserves member state.

### Catalog item

Every `catalog_item` contains:

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | string | Globally unique immutable item ID |
| `catalog_id` | string | Owning catalog ID |
| `catalog` | object | Compact `id`, `name`, optional `edition`, `publication_status`, and nested brand |
| `number` | integer | Positive immutable catalog-local order |
| `name` | string | Reviewed item name |
| `universe` | string, optional | Exact reviewed grouping |
| `variant` | string, optional | Reviewed variant |
| `publication_status` | string | `active` or `retired` |
| `artwork` | object | Artwork projection above |

The compact catalog makes both sides of a cross-catalog match self-contained. Its `id` must equal `catalog_id`.

## `GET /api/v1/catalogs`

Accepted first-page parameters are:

| Parameter | Rule |
| --- | --- |
| `brand_id` | Exact valid brand ID; an unknown valid ID produces an empty page |
| `q` | Literal substring of brand name, catalog name, edition, or a published item's name, universe, or variant |
| `publication_status` | `active`, `retired`, or `all`; default `active` |
| `limit` | Integer from 1 through 50; default 20 |

Filters combine with AND. `all` includes active and retired catalogs, never drafts. Results order by `sort_order`, then
catalog ID. The response is:

```text
{ "catalogs": [Catalog...], "page": Page }
```

See the five-catalog [catalog page fixture](../../testdata/contracts/catalogs-page.json).

## `GET /api/v1/catalogs/{catalog_id}/collection`

The path selects exactly one known active or retired catalog. Accepted first-page parameters are:

| Parameter | Rule |
| --- | --- |
| `q` | Literal substring of item name, universe, or variant |
| `universe` | Exact, case-sensitive value from the catalog's `universe_options` |
| `state` | `all`, `owned`, `available`, or `wanted`; default `all` |
| `limit` | Integer from 1 through 100; default 40 |

An unknown reviewed universe is `400 invalid_query`; valid filters combine with AND. State predicates mean the
corresponding quantity is positive. An owned-and-wanted row can therefore
appear under either filter. `all` has no quantity predicate.

Active and retired published items are eligible; draft items are not. Every eligible row is projected, including an
all-zero member entry. Results order by item number, then item ID. The catalog metadata and member summary are repeated
unchanged on every page and are not narrowed by item filters:

```text
{ "catalog": Catalog, "entries": [CollectionEntry...], "page": Page }
```

Each entry contains `catalog_item`, `owned`, `available`, and `wanted`. See
[page one](../../testdata/contracts/collection-page-1.json) and
[page two](../../testdata/contracts/collection-page-2.json).

## `GET /api/v1/collection/summary`

This unpaginated route returns:

```text
{ "summary": OverallSummary }
```

`OverallSummary` contains `active_catalog_count`, `active_item_count`, `catalogs_started`, and the six member-summary
fields, plus `possible_swap_count`. Active item count includes active items in
active catalogs. Member totals include active and retired published catalogs
and items. `catalogs_started` counts published catalogs in which any of the
member's three quantities is positive. `possible_swap_count` is the exact
number of distinct `(counterpart_id, receive_item_id)` availability leads and
is not limited to the first match page.

The bundled browser presents only available and wanted design counts and uses
`possible_swap_count` for its navigation badge. The other aggregate fields
remain transport-compatible internal measures, not member-facing dashboard
statistics.

See the [overall summary fixture](../../testdata/contracts/collection-summary.json).

## `PUT /api/v1/collection/{item_id}`

The path uses the globally unique item ID. Active and retired published items can be updated; draft or unknown items
return `404 not_found`.

The required body has exactly three integer fields:

```json
{
  "owned": 3,
  "available": 2,
  "wanted": 0
}
```

Validation is `0 <= available <= owned <= 1000000` and
`0 <= wanted <= 1000000`. `owned` and `wanted` may both be positive because
the wanted quantity represents additional desired copies. `available` and
`wanted` cannot both be positive. Replacement is atomic. All zeroes may delete
the sparse durable row, but success still returns its canonical zero
projection:

```text
{ "entry": CollectionEntry }
```

See the [saved-entry fixture](../../testdata/contracts/collection-put.json).

## `POST /api/v1/activity`

This authenticated, CSRF-protected route accepts no body or query parameters
and returns `204 No Content`. It records only that the current member opened an
authenticated application session. The repository performs a durable update
only when `last_seen_at` is empty or at least one hour old.

The route returns no timestamp, presence state, user projection, or ranking
data. It is a coarse stale-record input, not analytics or a member-visible
last-login feature. A browser may send it once after successful authenticated
bootstrap without delaying the rest of the interface.

## `GET /api/v1/matches`

Accepted first-page parameters are:

| Parameter | Rule |
| --- | --- |
| `readiness` | `actionable` or `unfulfillable`; default `actionable` |
| `give_catalog_id` | Fulfillable requested item from the current member must belong to this valid catalog ID |
| `receive_catalog_id` | Current member's wanted item must belong to this valid catalog ID |
| `scope` | Fulfillable requested item must be `same_catalog` as or `cross_catalog` from the receive item; default `all` |
| `limit` | Integer from 1 through 50; default 20 |

An unknown but valid catalog filter produces an empty page. Filters combine
with AND. `receive_catalog_id` is valid for either readiness feed.
`give_catalog_id` and reciprocal `scope` values only describe a fulfilable
leg, so combining either with `readiness=unfulfillable` returns
`400 invalid_query` on `readiness`. `same_catalog` requires the two item
catalog IDs to be equal; `cross_catalog` requires them to differ.
Contradictory otherwise-valid filter combinations are valid empty queries.

The two readiness values select disjoint queries:

- `actionable` contains a counterpart request the current member can fulfil,
  or a genuine giveaway from a counterpart with no published wants.
  Fulfillable requests always precede giveaways.
- `unfulfillable` contains a counterpart who has at least one published want,
  but none that the current member can currently offer.

If any counterpart request is fulfilable, that availability lead appears only
in `actionable`; it is never duplicated in `unfulfillable`. Omitting
`they_request` therefore means the counterpart has no published wants, not
merely that the current member lacks their requested item.

Within a result class, the oldest relevant state sorts first, with all waiting
age beyond 60 days treated equally. A lead sorts behind maintained peers only
when both its relevant state is more than 90 days old and the counterpart's
private `last_seen_at` is more than 90 days old or absent. Immutable request
item, counterpart, and receive item IDs finish the order. A displayed
request's `wanted_since_at` is the relevant state; a genuine giveaway uses the
offered item's `available_since_at`. If several requests qualify, the same
capped-wait, stale, and item-ID ordering selects one.

Active and retired published catalogs and items remain matchable. Draft
records never match. One result is emitted for each unique
`(counterpart_id, receive_item_id)` availability lead in the selected feed:

```text
{ "matches": [Match...], "page": Page }
```

A match contains:

- opaque deterministic `id`, which is derived projection identity rather than durable trade state;
- `other_user` with only `id` and `display_name`;
- one required `you_receive` object; and
- an optional `they_request` object.

`you_receive` contains one full `catalog_item` and a `quantity`, which is the
minimum of the current member's want and the counterpart's availability.
`they_request` contains one full `catalog_item`, a numeric `quantity`, and a
boolean `fulfillable`. Its quantity is the amount the current member could
offer now—the minimum of their availability and the counterpart's want—rather
than disclosure of the counterpart's raw wanted quantity. It is positive
exactly when `fulfillable` is true, and zero for a request the current member
cannot currently fulfil. State-transition times, ranking time, capped-age
values, and activity data are not projected. See the separate
[actionable availability-lead fixture](../../testdata/contracts/matches-actionable-page.json)
and
[unfulfillable availability-lead fixture](../../testdata/contracts/matches-unfulfillable-page.json).

## Browser State and Loading Contract

The browser owns this JSON-shaped state; names are normative even though the implementation is JavaScript:

```js
{
  session: { status, user, authMode, csrfToken },
  route: { name, catalogID },
  catalogs: {
    filters: { q },
    page: { items, limit, requestCursor, nextCursor, backStack, number },
    requestGeneration, loading, error
  },
  overallSummary: { value, requestGeneration, loading, error },
  collection: {
    catalogID,
    catalog,
    pendingItemIDs,
    saveErrors,
    filters: { q, universe, state },
    page: { items, limit, requestCursor, nextCursor, backStack, number },
    requestGeneration, loading, error
  },
  matches: {
    page: { items, limit, requestCursor, nextCursor, backStack, number },
    requestGeneration, loading, error
  },
  unfulfillableMatches: {
    expanded,
    page: { items, limit, requestCursor, nextCursor, backStack, number },
    requestGeneration, loading, error
  }
}
```

`items` holds the corresponding envelope array. `requestCursor` is null on page one. Advancing pushes the current
request cursor, including null, onto `backStack`; going back pops it. `number` is one-based presentation state, not an API
parameter. Cursor tokens never enter the URL.

The browser behavior is fixed as follows:

1. After authentication, start one best-effort, non-blocking
   `POST /api/v1/activity`, then fetch the first active-catalog page and the
   overall summary. A transient activity failure does not block the
   application. The dashboard exposes one collectible-oriented `q` search; it
   does not fetch an independent catalog directory for hidden filter choices.
2. Collection route `#/collection/{catalog_id}` lazily fetches that catalog's first page.
3. Matches route `#/matches` fetches only the first bounded
   `readiness=actionable` page. Its **Swaps you can't fulfil yet** section stays
   collapsed and makes no `readiness=unfulfillable` request until opened.
   Each feed owns independent loading, error, retry, cursor, and page state.
4. Dashboard collectible search and collection filters live in the URL;
   changing one clears page state and starts at page one. The bundled Matches
   view explicitly selects each readiness feed and exposes no
   give/receive/scope filters.
5. Each resource owns an `AbortController` or monotonically increasing `requestGeneration`; late responses cannot replace
   newer route or filter state.
6. A collection card makes its artwork an accessible pressed-state **Have**
   control and exposes native inline **Available to swap** and **Want**
   checkboxes beneath it. The browser accepts all valid numeric server states
   but writes only the canonical zero/one combinations in ADR 0004. **Have**
   and **Want** may remain selected together; **Available to swap** and
   **Want** clear one another. Each card has its own pending and error state; a
   failed optimistic update rolls back and preserves keyboard focus.
7. A successful mutation replaces the visible row with the canonical entry,
   refreshes the overall summary and currently loaded catalog page, and clears
   both match resources and their cursors. If an active collection filter may
   no longer include the edited row, that collection query reloads from page
   one. While Matches is active, actionable results reload immediately and the
   unfulfillable section collapses to its untouched state.
8. `400 invalid_cursor` clears that resource's page state, announces that results were refreshed, and requests page one
   once. A second failure is shown as an error rather than retried.
9. Sign-out clears all member-derived rows, rendered private nodes, summaries,
   cursors, pending writes, save errors, and request generations. A logout `401`
   means the session is already gone and completes the same local cleanup. A
   `403 csrf_failed` refreshes session/CSRF state and retries once; an anonymous
   refresh performs the cleanup without retrying.
10. Changing catalog identity clears the previous catalog projection before the
    next request renders.
11. Cards render approved artwork and alt text from server projections. They do
    not render license, hosting, attribution, or review boilerplate. Collection
    artwork is the pressed-state **Have** button; **Available to swap** and
    **Want** remain checkboxes. Matches are grouped by the receive item the
    member wants. Fulfillable requests are preferred, genuine no-request rows
    say the person has not asked for anything back, and unfulfillable requests
    appear in a separate muted section with the requested collectible.

The browser must render loading, valid-empty, and error states without treating a partial page as the complete filtered
data set. Keyboard focus, checkbox semantics, zoom, and narrow-screen behavior remain required by the
[browser testing strategy](../architecture/testing-strategy.md).

## Contract Fixtures

All fixture identities and brands are fictional, and the only image is the repository's Apache-licensed placeholder.
Opaque cursor text in a fixture is illustrative and is intentionally not valid under a running process's random key.

| Fixture | Representative request |
| --- | --- |
| [Five catalogs](../../testdata/contracts/catalogs-page.json) | `GET /api/v1/catalogs?publication_status=all` |
| [Collection page one](../../testdata/contracts/collection-page-1.json) | `GET /api/v1/catalogs/paper-orbit-stargazers-2026/collection?limit=2` |
| [Collection page two](../../testdata/contracts/collection-page-2.json) | Continuation using page one's cursor |
| [Overall summary](../../testdata/contracts/collection-summary.json) | `GET /api/v1/collection/summary` |
| [Saved entry](../../testdata/contracts/collection-put.json) | `PUT /api/v1/collection/paper-orbit-stargazers-2026-001-comet-pin` |
| [Actionable matches](../../testdata/contracts/matches-actionable-page.json) | `GET /api/v1/matches?readiness=actionable` |
| [Unfulfillable matches](../../testdata/contracts/matches-unfulfillable-page.json) | `GET /api/v1/matches?readiness=unfulfillable` |
| [Invalid query](../../testdata/contracts/error-invalid-query.json) | `GET /api/v1/catalogs?limit=51` |
| [Unknown query](../../testdata/contracts/error-unknown-query-parameter.json) | `GET /api/v1/catalogs?sort=name` |
| [Cursor conflict](../../testdata/contracts/error-cursor-conflict.json) | `GET /api/v1/matches?cursor=opaque&limit=20` |
| [Invalid cursor](../../testdata/contracts/error-invalid-cursor.json) | Any rejected continuation |
