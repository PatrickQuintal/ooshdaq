# Testing Strategy

Tests prove domain behavior, privacy, durable SQLite behavior, and the accepted operating envelope without production data or a deployed identity provider.

## Commands

| Command | Purpose |
| --- | --- |
| `make check` | Check Go formatting, run `go vet`, and run all Go tests |
| `go test ./...` | Portable minimum Go gate |
| `make test-race` | Race-enabled Go suite for concurrency or shutdown changes |
| `make test-web` | Dependency-free browser contract tests |
| `make test-browser` | Live Chromium journeys after installing `web/` dependencies |
| `make container-smoke` | Container hardening, persistence, backup, and restore |
| `make qualify` | Fixed synthetic capacity and SQLite qualification lane |

Run the narrowest relevant test during development, then `make check`. Run the
wider lanes whenever their boundary changes.

## Test Layers

### Domain

Use table-driven tests without I/O for catalog identity and retirement,
artwork-rights combinations, user validation, quantity bounds,
`available <= owned`, available/wanted exclusivity, owned-plus-wanted, and
canonical zero state. Domain tests do not recreate repository matching queries
or pagination.

### Application services

Use focused fakes for isolated failure and cancellation paths. Use temporary
SQLite databases for end-to-end port orchestration instead of maintaining a
second repository implementation for tests. Match service tests verify
delegation and safe projection boundaries, not ranking algorithms.

### Repository behavior

Run repository behavior tests against a temporary SQLite database. Cover
bounded keyset pages, catalog filters and summaries, sparse atomic
replacements, timestamp transitions, rate-limited activity, catalog import
idempotence, typed failures, cancellation, and hard limits.

Matching belongs here: one-way leads, self-match prevention, same- and
cross-catalog requests, genuine no-request results, disjoint actionable and
unfulfillable feeds, request selection, deduplication, capped-wait/stale
ranking, frozen-time continuation, and independently bounded quantities.

### SQLite integration

Exercise schema creation and every supported forward migration, constraints
and foreign keys, transactional import, zero-row removal, close/reopen and
abrupt-restart recovery, integrity checks, WAL and checkpoint behavior,
bounded lock waiting, and classified busy/full/corrupt/incompatible failures.
Verify indexed match plans and high-fanout unfulfillable data without an
application-level all-user scan. Test online backup and isolated restore.

### HTTP

Use `httptest` with the real router, services, and a temporary SQLite database
for every route, query shape, method, status, projection, and header in the
[HTTP API](http-api.md). Include
body caps, malformed or extra JSON, repeated and unknown queries, cursor
binding, authentication isolation, CSRF, proxy-header ambiguity, safe error
envelopes, and differing liveness/readiness behavior. Never assert raw tokens,
cookies, identity headers, SQL, or collection contents in failure output.

### Browser and container

Keep a small keyboard-accessible desktop browser lane for login, catalog paging
and filtering, collection state edits, actionable and unfulfillable match
discovery, cursor recovery, image fallback, forced colors, errors, and logout.
Exercise the supported desktop viewport boundary and a typical wide display.
Fail on uncaught page or unexpected console errors; prefer roles and visible
text over CSS selectors. Mobile and narrower layouts are outside this lane.

Container smoke tests run the non-root image with a read-only root filesystem
and temporary volumes. Verify probes, loopback-only mock access, persistence
across replacement, graceful termination, resource limits, online backup, and
restore with the same image.

## Reproducible Qualification

`make qualify` creates a fresh deterministic synthetic database and refuses to
overwrite an existing database or WAL sidecar. It exercises the documented
[operating envelope](../product/operating-requirements.md) under one CPU,
256 MiB memory, and no swap. The executable fixture and workload constants in
`internal/qualification` are authoritative.

Accept a qualification run only when its report shows:

- the fixture shape and request mix are exact;
- same-catalog, cross-catalog, one-way, and unfulfillable journeys succeed;
- there are no unexpected, busy, or response-bound errors;
- p95 catalog and collection latency is below one second; write and match-feed
  latency is below two seconds;
- required indexes produce bounded plans;
- readiness, foreign keys, integrity, and checkpoints succeed; and
- the hard CPU/memory limits, peak-memory ceiling, and expected Go toolchain
  are verified.

Qualification reports to stdout. It is evidence for that host and workload,
not a promise of continuous availability or multi-replica failover.

## Security Regression Matrix

| Scenario | Expected result |
| --- | --- |
| Mock route in proxy mode | `404`; no local login surface |
| Identity header sent to mock mode | Ignored; mock session still required |
| Proxy request without a subject | Unauthorized |
| Proxy request without a display name | Accepted with the safe fallback label |
| Repeated or malformed proxy identity | Unauthorized |
| `available > owned` or available plus wanted | Validation error; no write |
| Owned plus wanted, with zero available | Accepted atomically |
| Request supplies another member ID | Unknown-field or query rejection |
| Cross-site mutation | CSRF rejection |
| Counterpart has no wants | Actionable lead omits `they_request` |
| Counterpart has only unofferable wants | Appears only in unfulfillable feed |
| Repository cannot be opened or read | Startup or readiness fails safely |

## Fixtures and Coverage

Use fictional identities and generated collection data. Catalog tests retain
reviewed provenance metadata but not unlicensed image bytes. Control clocks
and IDs where ordering matters; never use production exports or real auth
headers.

Published catalog tests use `internal/catalog/manifests`. Multi-catalog unit
and browser journeys opt into the fictional manifests under
`internal/catalog/testdata/manifests`; qualification generates its own five-
catalog fixture.

Coverage is diagnostic; require behavioral coverage for each invariant,
authorization decision, decoder, transaction, migration, and match class.
