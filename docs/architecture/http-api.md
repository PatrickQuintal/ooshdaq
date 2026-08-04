# $OOSHDAQ HTTP API

This is the canonical JSON wire contract. Handlers resolve the current
principal, validate bounded input, invoke a service, and encode these
projections; they never accept a member ID or contain persistence queries.

## Route Map

| Method | Route | Authentication | Purpose |
| --- | --- | --- | --- |
| `GET` | `/health/live` | None | Process liveness |
| `GET` | `/health/ready` | None | Database and schema readiness |
| `GET` | `/api/v1/session` | Optional | Session and CSRF bootstrap |
| `POST` | `/auth/mock-login` | CSRF; mock mode | Select an allowlisted fictional user |
| `POST` | `/auth/logout` | Member and CSRF; mock mode | End the local session |
| `GET` | `/api/v1/catalogs` | Member | List catalogs; default/max 20/50 |
| `GET` | `/api/v1/catalogs/{catalog_id}/collection` | Member | List one catalog; default/max 40/100 |
| `GET` | `/api/v1/collection/summary` | Member | Current-member totals |
| `PUT` | `/api/v1/collection/{item_id}` | Member and CSRF | Replace one entry |
| `GET` | `/api/v1/matches` | Member | List one readiness feed; default/max 20/50 |
| `POST` | `/api/v1/activity` | Member and CSRF | Rate-limited private activity update |

Unlisted reserved paths return JSON `404 not_found`. An unsupported method on
an available route returns `405 method_not_allowed` with `Allow`. In proxy
mode, mock login is always `404` and local logout is `501`.

## Common Rules

- JSON names are `snake_case`; successful JSON and errors use
  `Content-Type: application/json; charset=utf-8` and `Cache-Control: no-store`.
- Optional fields are omitted; required objects and arrays remain present when
  empty. Only `page.next_cursor` is nullable on success.
- Brand, catalog, and item IDs are case-sensitive lowercase slugs matching
  `^[a-z0-9](?:[a-z0-9-]{0,126}[a-z0-9])?$`; clients otherwise treat IDs as
  opaque. Member and match IDs are opaque strings and never query input.
- Counts and quantities are non-negative JSON integers; each collection-entry
  quantity is at most `1,000,000`. Dates are `YYYY-MM-DD`.
- JSON bodies have a 16 KiB cap, require `application/json`, and reject `null`,
  unknown or missing fields, trailing values, and malformed JSON.
- Query values are scalar; unknown names, repetition, invalid encoding, blank
  non-`q` values, and invalid enums are errors.
- `q` is trimmed, limited to 100 Unicode code points, and matched as a
  Unicode-lowercased literal substring; `%` and `_` have no wildcard meaning.
- Valid empty queries return `200`, an empty array, and `next_cursor: null`.

## Authentication and CSRF

`GET /api/v1/session` issues or reuses the CSRF cookie and returns:

```text
Session = {
  "authenticated": boolean, "user"?: User, "auth_mode": "mock" | "proxy",
  "csrf_token": string, "mock_users"?: [User...]
}
User = { "id": string, "display_name": string }
```

`user` appears only when authenticated; `mock_users` appears only in mock mode.
Anonymous session bootstrap still returns `200`.

Mock login accepts exactly `{"user_id": string}` and returns an authenticated
`Session`; an ID outside the allowlist is `401 invalid_mock_user`. Mock logout
accepts no body and returns `204`. Product mutations and both mock-auth
mutations require the CSRF cookie plus an identical `X-CSRF-Token` value and
reject `Sec-Fetch-Site: cross-site`. See [authentication](authentication.md)
for cookie properties and the proxy trust topology.

## Error Envelope

```json
{"error":{"code":"validation_failed","message":"The collection quantities are invalid.","field":"available"}}
```

`code` and an included `field` are stable; `message` is safe but not
byte-stable. `field` is omitted without one cause. Private details stay hidden.

| Status | Codes |
| ---: | --- |
| 400 | `invalid_query`, `unknown_query_parameter`, `cursor_conflict`, `invalid_cursor`, `invalid_request` |
| 401 | `authentication_required`, `invalid_mock_user` |
| 403 | `csrf_failed` |
| 404 | `not_found` |
| 405 | `method_not_allowed` |
| 413 | `request_too_large` |
| 415 | `unsupported_media_type` |
| 422 | `validation_failed` |
| 500 | `internal_error` |
| 501 | `logout_not_supported` |
| 503 | `service_unavailable` |

An unknown query name uses that name as `field`; a repeated or invalid value
uses its parameter name. `cursor` combined with anything else is
`cursor_conflict`. All cursor decoding failures are the same safe
`invalid_cursor` response.

## Pages and Cursors

```text
Page = { "limit": integer, "returned": integer, "next_cursor": string | null }
```

`returned` equals the array length; there is no total or previous cursor. First
pages supply filters and optional `limit`; continuations supply only `cursor`.

The no-padding Base64URL cursor carries a random nonce and AES-256-GCM payload
binding version, route, scoped catalog, member, normalized filters, limit,
expiry, and keyset tuple. Route and member are associated data. Cursors expire
after 15 minutes, cannot exceed 4,096 bytes, and are invalid after restart.

| Resource | Ascending keyset tuple |
| --- | --- |
| Catalogs | `(sort_order, catalog_id)` |
| Collection | `(item_number, item_id)` |
| Either match feed | `(request_class, capped_since_epoch, stale_flag, request_item_id, counterpart_id, receive_item_id)` |

Match cursors freeze the ranking clock. Cursors are not snapshots; concurrent
writes can change later pages.

## Shared Projections

These are the complete field sets; `?` marks an omitted absent field.

```text
Artwork = { image_url?: string, alt_text: string, width?: integer,
  height?: integer, source_url?: string, attribution: string,
  rights_status: string, license?: string, retrieved_at: date }
MemberSummary = { available_designs, wanted_designs: integer }
CompactCatalog = { id, name: string, edition?: string,
  publication_status: "active" | "retired", brand: { id, name: string } }
Catalog = { id: string, brand: { id, name: string }, name: string,
  edition?: string, publication_status: "active" | "retired",
  sort_order, item_count: integer, universe_options: [string...],
  artwork: Artwork, member_summary: MemberSummary }
CatalogItem = { id, catalog_id: string, catalog: CompactCatalog,
  number: integer, name: string, universe?: string, variant?: string,
  publication_status: "active" | "retired", artwork: Artwork }
CollectionEntry = { catalog_item: CatalogItem,
  owned, available, wanted: integer }
```

Artwork dimensions appear together; `image_url` is server-approved and
`alt_text` is non-empty. Rights are `original`, `licensed`,
`permissioned-link`, `source-hosted-link`, `unknown`, or `restricted`.
Unsafe, unknown, and restricted images omit `image_url`. Draft records are
never projected. Member summaries use distinct positive-item counts;
`item_count` includes published active and retired items.

### Catalogs

`GET /api/v1/catalogs` accepts:

| Parameter | Rule |
| --- | --- |
| `brand_id` | Exact ID; an unknown valid ID produces an empty page |
| `q` | Searches brand, catalog, edition, and published item name/universe/variant |
| `publication_status` | `active` (default), `retired`, or `all` |
| `limit` | 1–50; default 20 |

Filters combine with AND. Results order by catalog `sort_order`, then ID.
Response: `{"catalogs":[Catalog...],"page":Page}`. Catalog summaries are for
the current member and are not narrowed by `q`.

### Scoped Collection

`GET /api/v1/catalogs/{catalog_id}/collection` accepts:

| Parameter | Rule |
| --- | --- |
| `q` | Searches item name, universe, and variant |
| `universe` | Exact reviewed value advertised by the catalog |
| `state` | `all` (default), `owned`, `available`, or `wanted` |
| `limit` | 1–100; default 40 |

The path may name an active or retired catalog. Filters combine with AND;
state means the named quantity is positive. Active and retired published
items, including canonical all-zero entries, order by number then ID. An
unknown universe is `invalid_query`. Response:
`{"catalog":Catalog,"entries":[CollectionEntry...],"page":Page}`. Catalog
metadata and summary repeat unchanged on each page and ignore item filters.

### Summary and Replacement

`GET /api/v1/collection/summary` accepts no query and returns:

```text
{ "summary": { possible_swap_count, available_designs,
  wanted_designs: integer } }
```

Design counts include retired published records. `possible_swap_count` counts
distinct `(counterpart_id, receive_item_id)` leads across all pages.

`PUT /api/v1/collection/{item_id}` accepts exactly:

```json
{ "owned": 3, "available": 2, "wanted": 0 }
```

Active and retired published items may be updated; draft or unknown IDs are
`404`. Replacement is atomic: `available <= owned`, available and wanted
cannot coexist, and owned and wanted may. All zeroes may remove the sparse row.
Success returns `{"entry":CollectionEntry}` with canonical values.

### Activity

`POST /api/v1/activity` accepts no query or body and returns `204`. It may
advance only the current member's private `last_seen_at`, at most once per
hour, and never returns activity or presence data.

### Matches

`GET /api/v1/matches` accepts:

| Parameter | Rule |
| --- | --- |
| `readiness` | `actionable` (default) or `unfulfillable` |
| `give_catalog_id` | Fulfillable requested item must be in this catalog |
| `receive_catalog_id` | `you_receive` item must be in this catalog |
| `scope` | `all` (default), `same_catalog`, or `cross_catalog` for a fulfillable request |
| `limit` | 1–50; default 20 |

Scope compares request and receive catalog IDs. Unknown valid IDs produce an
empty page. `unfulfillable` cannot combine with `give_catalog_id` or non-`all`
scope. The feeds are disjoint:

- `actionable` contains a request the current member can fulfil, followed by
  genuine no-request leads from counterparts with no published wants;
- `unfulfillable` contains counterparts with published wants when none can be
  fulfilled by the current member.

One row is returned per `(counterpart_id, receive_item_id)`. Oldest relevant
state sorts first, capped at 60 days: request `wanted_since_at`, or offered
`available_since_at` for no-request leads. A row is stale only when that state
and absent-or-old counterpart activity exceed 90 days. IDs break remaining
ties; the same rule chooses among several requests.

Response: `{"matches":[Match...],"page":Page}`.

```text
Match = { id: string, other_user: User, you_receive: { catalog_item:
  CatalogItem, quantity: integer }, they_request?: { catalog_item:
  CatalogItem, quantity: integer, fulfillable: boolean } }
```

`you_receive.quantity` is the minimum wanted/available quantity.
`they_request.quantity` is the amount the current member can offer now; it is
positive with `fulfillable:true` and zero in the unfulfillable feed. Omission
of `they_request` means the counterpart has no published wants. Result IDs are
derived projection keys, not durable trades. Timestamps and ranking values are
not returned.

## Health

Both health routes return `{"status":"ok"}` on `200`. Liveness asserts only
that HTTP is serving. Readiness uses a two-second bounded SQLite, schema, and
catalog check; failure is a redacted `503 service_unavailable`.
