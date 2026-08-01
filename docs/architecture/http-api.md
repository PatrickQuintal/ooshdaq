# $OOSHDAQ HTTP API

This page describes the shipped HTTP surface and its architectural boundaries.
The byte-level product contract, query rules, response projections, and fixtures
are authoritative in the
[bounded `/api/v1` contract](../implementation/api-contract-v1.md).

## Boundary

- The authenticated principal comes from the configured auth adapter, never
  from a product path, query, or JSON body.
- Product handlers validate transport input, call the application service, and
  encode projections. They do not contain matching SQL or persistence policy.
- Every product response is scoped to the current member.
- There is no arbitrary-user collection route, contact-data route, trade
  reservation, messaging, payment, or auction API.
- Product list routes are keyset-paginated with hard limits. There is no
  unbounded catalog, collection, user, or match response.
- JSON uses `snake_case`, `Cache-Control: no-store`, and
  `Content-Type: application/json; charset=utf-8`.

## Route Set

The product route set is deliberately small:

| Method | Route | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/catalogs` | Browse bounded multi-brand catalog pages |
| `GET` | `/api/v1/catalogs/{catalog_id}/collection` | Browse and filter one catalog with the member's quantities |
| `GET` | `/api/v1/collection/summary` | Read aggregate current-member totals |
| `PUT` | `/api/v1/collection/{item_id}` | Replace one current-member collection entry |
| `GET` | `/api/v1/matches` | Find one independently bounded actionable or unfulfillable readiness page |
| `POST` | `/api/v1/activity` | Rate-limit the current member's private last-seen update |

The corresponding operational and authentication routes are:

| Method | Route | Purpose |
| --- | --- | --- |
| `GET` | `/health/live` | Process liveness |
| `GET` | `/health/ready` | Bounded database and migrated-schema readiness check |
| `GET` | `/api/v1/session` | Session state and CSRF bootstrap |
| `POST` | `/auth/mock-login` | Allowlisted fictional login in mock mode only |
| `POST` | `/auth/logout` | End a mock-mode session |

Unlisted product paths return `404 not_found`. A listed route with an
unsupported method returns `405 Method Not Allowed` and an `Allow` header.

## Product Contract

The [implemented contract](../implementation/api-contract-v1.md) defines:

- accepted filters, default limits, and maximum limits;
- stable error codes and status mappings;
- catalog, item, artwork, summary, collection, and match projections;
- cursor format, expiry, route and principal binding, and ordering tuples;
- quantity and identifier bounds; and
- disjoint actionable and unfulfillable availability feeds, capped-wait
  ordering, and genuine no-request semantics.

Catalog `q` search also matches published item names, universes, and variants,
allowing the bundled dashboard's **Find a collectible** search to return the
catalogs containing that item without adding an unbounded global-item route.

Contract fixtures live in [`testdata/contracts`](../../testdata/contracts).
Go handler tests and dependency-free browser contract tests consume those
fixtures so server and browser changes fail together.

Important current shapes include:

- globally unique `item_id` and `catalog_id` values, with no legacy
  `series_id` compatibility fields;
- a self-contained compact catalog and brand projection on every match leg;
- a required `you_receive` leg and an optional `they_request` object with
  `quantity` and `fulfillable`;
- a `readiness=actionable|unfulfillable` match selector, defaulting to
  actionable;
- an explicit `same_catalog` or `cross_catalog` match scope filter; and
- validated artwork metadata on every catalog and item, with unsafe image URLs
  omitted and provenance copy kept out of the member collection workflow.

## Authentication and CSRF

The exact trust boundary is documented in
[authentication](authentication.md).

Mock mode is for local use and browser qualification. It exposes only the
configured fictional profiles and uses signed, host-only session cookies.
Listening with mock mode on a non-loopback address requires an explicit unsafe
development override.

Proxy mode accepts identity only from configured headers supplied by a trusted
authenticating proxy. The application must be unreachable except through that
proxy, and the proxy must strip inbound copies of the identity headers before
setting verified values.

`GET /api/v1/session` establishes the CSRF cookie and returns the matching
token. The browser then sends an empty `POST /api/v1/activity`; the repository
writes only when the current member's private timestamp is at least one hour
old. Every product mutation and mock-auth mutation requires the exact token in
`X-CSRF-Token`. Upstream tokens, raw forwarded headers, email addresses,
groups, activity timestamps, and repository records are never returned.

## Request Validation

- JSON bodies are capped at 16 KiB.
- JSON routes require `application/json`.
- Unknown fields, `null`, trailing values, malformed JSON, and incomplete
  mutation bodies are rejected.
- Quantities are integers from `0` through `1,000,000`, and
  `available <= owned`.
- `owned` and `wanted` may both be positive: a member can own a copy and want
  more. Positive `available` and `wanted` values remain mutually exclusive.
- Unknown or repeated query parameters are rejected.
- Malformed query encoding, including invalid percent escapes or semicolon
  separators rejected by `url.ParseQuery`, is rejected rather than partially
  decoded.
- A continuation request supplies only its encrypted and authenticated cursor;
  combining it with filters is rejected.
- Product identifiers are validated before repository access.

The common safe error envelope is:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "available must not exceed owned.",
    "field": "available"
  }
}
```

`field` appears only when one request field caused the failure. Internal paths,
SQL, tokens, headers, request bodies, and collection contents are not exposed.

## Health Semantics

`GET /health/live` returns `200` while the HTTP process can serve requests. It
does not claim that SQLite is usable.

`GET /health/ready` applies a two-second timeout and performs bounded reads
against the migrated SQLite schema and catalog table. It verifies that the
known schema version is present, rather than relying on `SELECT 1`. A database
failure, missing or incompatible schema, or timeout returns
`503 service_unavailable`. Neither probe reveals build, dependency, or
filesystem detail.

Kubernetes uses liveness only to detect a wedged process and readiness to keep
traffic away until migrations and catalog import have completed.

## Cursors and Consistency

First-page requests carry filters and an optional limit. Continuations carry
only an AES-256-GCM encrypted and authenticated cursor containing normalized
filters, effective limit, authenticated member, route binding, expiry,
readiness, frozen ranking time, and the last deterministic keyset tuple. Route
and member are also authenticated as associated data. Cursors expire after 15
minutes and are invalidated by process restart.

The cursor is not a database snapshot. Keyset ordering avoids offset scans, but
concurrent collection writes can legitimately alter later match pages. A
tampered, expired, oversized, cross-user, cross-route, or pre-restart cursor is
the same safe `400 invalid_cursor` response.

## Versioning

Health and authentication paths are operational surfaces outside the product
version. `/api/v1` is the implemented pre-release product API. An incompatible
change after the first stable release requires a new version or an explicitly
documented migration window.

The discarded flat prototype is intentionally not retained as a legacy route or
projection.
