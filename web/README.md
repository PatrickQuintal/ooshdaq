# $OOSHDAQ browser

The browser is a dependency-free ES module application embedded into the Go binary. It implements the bounded product
contract in [`docs/implementation/api-contract-v1.md`](../docs/implementation/api-contract-v1.md).

## Runtime assumptions

- Authentication uses `GET /api/v1/session`; mock login and logout retain their existing CSRF behavior.
- Each authenticated bootstrap starts one best-effort, non-blocking
  `POST /api/v1/activity` with the current CSRF token. A transient activity
  failure does not delay or break the browser.
- Authenticated bootstrap fetches one bounded active-catalog dashboard page
  and `GET /api/v1/collection/summary`.
- The dashboard exposes one **Find a collectible** search. A matching catalog
  link carries the submitted query into its collection page so the matching
  item is visible immediately.
- `#/collection/{catalog_id}` lazily fetches the scoped collection page.
- Each collection card's artwork is a native pressed-state button for
  **Have**. **Available to swap** and **Want** remain native checkboxes below
  it. Each interaction writes one canonical state; there is no quantity
  dialog.
- **Have** and **Want** may be on together. The Have button preserves Want;
  Want preserves Have and clears Available; Available forces Have on and
  clears Want. Turning Have off clears Available while preserving Want.
- `#/matches` fetches only the bounded `readiness=actionable` feed. Results are
  grouped by the item the current member wants, with fulfillable requests first
  and people who have not asked for anything back next.
- A separate muted **Swaps you can’t fulfil yet** panel stays collapsed and
  makes no request until opened. It independently pages
  `readiness=unfulfillable`, keeps its loading and error states inside the
  panel, and never implies the requested item is already in the current
  member’s collection. Refreshing Matches collapses and resets this lazy feed.
- A match omits `they_request` only when the counterpart has no published
  wants. When present, its `fulfillable` flag must be true exactly when its
  currently offerable `quantity` is positive; an unfulfillable request has
  quantity zero. The browser rejects ambiguous or inconsistent shapes instead
  of mislabelling a real request as absent.
- The Matches navigation badge comes from the summary
  `possible_swap_count`; it does not prefetch the matches page.
- A continuation request for either match feed sends only the opaque cursor
  supplied by the server.
- `400 invalid_cursor` resets that resource to page one.
- A successful item `PUT` refreshes the overall summary and current catalog
  navigation page, refreshes a state-filtered collection when membership may
  have changed, and clears both match feeds and their cursors.
- Catalog and item artwork is rendered only from server projections. Unsafe or failed URLs use the embedded placeholder.
- Artwork provenance remains a server-side input control and is not rendered
  in member collection, catalog, or match cards.
- An in-place catalog change clears the old catalog identity and cards before
  loading the next projection.
- A local logout `401` is treated as an already-ended session and still clears
  rendered member data. An expired CSRF token refreshes and retries once.

The UI does not infer alternate response fields, calculate global totals from partial pages, or prefetch the
unfulfillable match feed.

## Checks

From the repository root:

```sh
go test ./web
node --test web/tests/*.test.mjs
cd web && npm ci && npm run test:e2e
```

The Node tests exercise pure contract parsing, match grouping, pagination URL
construction, and deep-link routing without adding a browser build step.
Playwright is a pinned development-only test dependency; it is not shipped in
the embedded browser application.
