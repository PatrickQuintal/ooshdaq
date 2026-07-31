# Testing Strategy

Tests should prove user-visible behavior and security boundaries without requiring a deployed identity provider or shared
cloud infrastructure. Capacity and recovery tests target the accepted envelope in
[operating requirements](../product/operating-requirements.md): approximately 10,000 eligible users, at least five
catalogs, fewer than 100 concurrent members, durable single-pod storage, and accepted restart downtime.

## Routine Local Gate

```sh
make check
```

This formats Go sources, runs `go vet`, and runs all Go tests. Before merging concurrency, persistence, or shutdown changes,
also run:

```sh
make test-race
```

`go test ./...` remains the portable minimum when `make` is unavailable.
Use `make test-web` for dependency-free browser contracts, `make test-browser`
for the live Chromium lane, `make container-smoke` for container replacement
and restore, and `make qualify` only for the fixed qualification-scale lane.

## Test Layers

### Domain unit tests

Use table-driven tests with no I/O for:

- quantity bounds, `available <= owned`, and available/wanted exclusivity;
- canonical neutral, owned, available, wanted, and owned-plus-wanted browser
  states;
- one-way wanted-to-available leads with no reciprocal requirement;
- optional same-catalog and cross-catalog reciprocal enrichment;
- deterministic unfulfillable disclosure in its separate feed when the
  counterpart has wants the current member cannot fulfil, distinct from a
  genuine no-request lead;
- 60-day waiting-age saturation and the 90-day two-signal stale boundary;
- self-match prevention;
- deterministic ordering and deduplication;
- independently bounded receive and immediately offerable request quantities;
- zero and maximum boundaries.

### Application service tests

Use small fakes implementing narrow repository ports for:

- current-principal ownership of writes;
- catalog existence checks;
- brand and catalog scoping without preventing cross-catalog trade legs;
- canonical zero projections;
- minimal match disclosure;
- propagation of readiness, match result bounds, filters, ranked cursors,
  cancellation, and timeouts;
- propagation of cancellation and classified errors;
- no partial update after validation or repository failure.

Fakes should model the contract, not expose storage-engine details.

### Repository conformance tests

Run the same behavior-focused suite against the memory adapter and a real SQLite database under `t.TempDir()`. Cover:

- bounded catalog and collection pages with deterministic keyset positions;
- catalog-scoped summaries and sparse entry replacement;
- genuine no-request leads, fulfilable and unfulfillable requests, and
  same/cross-catalog filters;
- disjoint actionable and unfulfillable pages with independent positions;
- preservation and reset of wanted/available-since timestamps plus
  rate-limited last-seen writes;
- manifest import idempotence and conflict handling;
- typed failure classification and cancellation;
- defensive copying and hard result limits.

### SQLite repository integration tests

In addition to shared conformance, cover:

- schema creation and every supported forward migration;
- brand, catalog, item, source, asset, user, and sparse collection constraints;
- foreign-key enforcement and stable retired-item references;
- atomic entry replacement and zero-row removal;
- close, reopen, abrupt-termination recovery, and integrity checks;
- WAL configuration, bounded lock waiting, serialized write behavior, and checkpoint handling;
- visible handling of busy, disk-full, corrupt, incompatible, and failed-migration states;
- indexed, independently bounded actionable and unfulfillable queries with
  capped-wait ranking and stable frozen-time cursors;
- a high-fanout fixture where hundreds of counterparts are all
  unfulfillable, proving the actionable page remains fast and empty;
- online backup of a live database and restoration into an isolated store;
- clean creation from schema migrations and reviewed catalog manifests.

Applicable repository behavior belongs in the shared conformance suite rather than duplicated as implementation-shaped
expectations. No test should introduce a runtime adapter selector, dual-store mode, legacy reader, or storage rollback.

### HTTP tests

Use `httptest` against the real router and services to verify:

- every route, method, content type, and status code in the API document;
- body-size, malformed JSON, unknown-field, and trailing-value rejection;
- stable safe error envelopes;
- authentication required for private resources;
- path/body fields cannot select another member;
- mock-login is absent in proxy mode;
- spoofed proxy headers do not work through a mock-mode server;
- duplicated or malformed forwarded headers fail closed;
- cache, cookie, content security, framing, and MIME-sniffing headers;
- liveness and readiness differ when the repository is unavailable.

Never assert tokens, signed cookies, or raw identities in failure output.

### Browser journey tests

Keep a small browser lane for:

1. select a seeded local member;
2. browse cards with a missing image fallback;
3. switch between multiple brand catalogs;
4. use inline controls to move between owned, available, wanted, and
   owned-plus-wanted states while keeping available and wanted exclusive;
5. sign in as another member and mark a wanted item available without marking
   a return item wanted;
6. return to the first member, see the possible-swap badge and the other person
   grouped beneath the wanted item, then add an inverse state in another
   catalog and see the named item that person wants;
7. prove fulfilable rows precede genuine no-return rows without requesting the
   unfulfillable feed;
8. open **Swaps you can't fulfil yet**, then prove its independent request,
   pagination, muted result, error handling, and retry;
9. prove technical API filters are not exposed and bounded match and catalog
   pagination still works;
10. sign out and lose access to private views;
11. use keyboard-only controls and verify inline-control focus behavior.

Browser tests should fail on uncaught page errors and unexpected console errors. Prefer semantic roles and user-facing
text over CSS implementation selectors.

### Container smoke tests

Build the local image from the pinned Dockerfile inputs, run it with a
temporary data volume, and verify:

- it runs as the declared non-root UID;
- the read-only root filesystem works;
- `/health/live` and `/health/ready` become healthy;
- local mock login and one collection update work through the published loopback port;
- data survives container replacement with the same volume;
- SQLite schema migrations and readiness complete before traffic;
- the built image, module graph, license inventory, and composition root contain no bbolt path;
- SIGTERM causes bounded graceful shutdown.
- one CPU and 256 MiB limits are actually applied;
- an online backup's logged digest matches its copied bytes; and
- an isolated restore made with the same built image returns the pre-backup value.

### Capacity and recovery tests

Use generated synthetic data, never production exports, to validate:

- approximately 10,000 user records;
- five or more catalogs with representative item counts and sparse collection state;
- a read-heavy mix of catalog, collection, update, and match requests with up to 100 concurrent simulated members;
- bounded match response size and stable latency without all-user or all-item-pair application scans;
- separately measured actionable and high-fanout unfulfillable match latency;
- database and WAL growth, checkpoint behavior, disk headroom, and write-lock wait time;
- process kill and restart against the same volume;
- pod replacement or node-drain rehearsal with accepted downtime and preserved committed data;
- backup creation, isolated restore, integrity verification, record counts, and an end-to-end match journey.

Record the fixture shape, request mix, hardware/storage class, test duration, error rate, and latency distribution. A test
that opens 100 idle connections is not evidence for the expected workload.

Continuous availability, multi-replica failover, and zero-downtime rollout are not baseline acceptance gates.

## Security Regression Matrix

| Scenario | Expected result |
| --- | --- |
| Mock route requested in proxy mode | Not found |
| Identity header supplied directly to mock mode | Ignored; valid mock session still required |
| Proxy request without subject or name | Unauthorized |
| Repeated/conflicting proxy identity header | Unauthorized |
| Available greater than owned | Validation error; no write |
| Wanted combined with owned, but not available | Accepted and stored atomically |
| Wanted combined with available | Validation error; no write |
| Request attempts to include another user ID | Unknown-field validation error |
| Cross-origin collection mutation | Rejected |
| Unknown catalog item ID | Not found; no durable entry |
| Counterpart has the wanted item available but wants nothing | Lead has no `they_request` object |
| Cross-catalog reciprocal pair | Returned as a fulfilable `they_request` on the lead |
| Counterpart wants an item the current member cannot offer | One request is returned as unfulfillable, never as a no-return lead |
| Database unavailable at startup | Startup fails |
| Database becomes unreadable | Readiness fails; safe API error |

## Fixtures

- Use fictional names and identifiers.
- Keep catalogs small in unit tests; use the dedicated capacity fixture to catch accidental quadratic or unbounded
  behavior.
- Store expected third-party provenance metadata, not copied restricted images.
- Set deterministic clocks and IDs at service boundaries where assertions need them.
- Never use production exports or real authentication headers.

## Coverage Standard

Coverage is a diagnostic, not the acceptance criterion. Every meaningful branch of a domain invariant, authorization
decision, decoder, persistence transaction, and SQLite schema migration should have a behavioral test. Uncovered code
should be limited to generated data, trivial wiring, or defensive paths that cannot be triggered safely, with the reason
explained in review.
