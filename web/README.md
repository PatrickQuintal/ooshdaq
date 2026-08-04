# $OOSHDAQ browser

The browser is a dependency-free ES module application embedded in the Go binary.
It renders authenticated projections and provides accessible collection and
possible-swap interactions without a build step.

## Boundaries

- The server owns identity, authorization, collection invariants, matching,
  ranking, pagination, and artwork approval.
- Browser modules own route state, bounded resource loading, rendering,
  optimistic interaction, focus restoration, and member-facing errors.
- Each resource has independent loading, error, cursor, and cancellation state;
  late responses cannot replace newer navigation or filter state.
- Collection controls write one canonical state and replace optimistic data with the server response.
- Match presentation keeps actionable and member-opened unfulfillable feeds
  independent.
- Artwork is rendered only from server projections and falls back to the
  embedded placeholder.
- The browser does not infer alternate API shapes, calculate totals from partial
  pages, or reconstruct matching policy.

The wire contract is documented in
[`http-api.md`](../docs/architecture/http-api.md).

## Checks

```sh
go test ./web
node --test web/tests/*.test.mjs
cd web && npm ci && npm run test:e2e
```

Node exercises parsing and state helpers without a browser build. Playwright is a pinned development dependency, not an application dependency.
