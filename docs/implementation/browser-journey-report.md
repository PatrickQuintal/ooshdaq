# Live browser journey report

Date: 2026-07-31

## Result

The browser qualification passed: seven Chromium journeys completed against the
real `$OOSHDAQ` HTTP server and a fresh SQLite database, with no unexpected
browser console errors or uncaught page errors.

```text
Running 7 tests using 1 worker
7 passed (41.8s)
```

The run used Playwright 1.62.0. The harness built and launched `cmd/ooshdaq`,
selected mock authentication, imported the five embedded catalogs, and created
its database and backups under a disposable directory. It removed the process
and directory when the run ended.

## Reproduce

From the repository root:

```bash
cd web
npm install
PLAYWRIGHT_BROWSERS_PATH=.cache/ms-playwright npx playwright install chromium
npm run test:e2e
```

`e2e/start-server.sh` sets the server configuration explicitly, waits on
`/health/ready`, and terminates the exact process it started.

## Journeys exercised

### Navigation and inline collection states

- Signed in as Alex through the rendered mock-login control and proved that
  login loads only bounded catalog navigation and the aggregate summary.
  Collection pages and matches remain lazy.
- Confirmed authenticated bootstrap sends one empty, CSRF-protected activity
  request, and that an induced activity failure does not delay or break the
  interface.
- Exercised catalog and collection cursor pagination, the
  **Find a collectible** search, deep links, page reload, and stale-request
  rejection. A collectible-name search finds the containing catalog and is
  carried into that catalog's item search.
- Used the collectible artwork's pressed-state **Have** control and the native
  **Available to swap** and **Want** checkboxes by keyboard. The tests assert
  the canonical browser transitions:
  neutral `0/0/0`, have `1/0/0`, available `1/1/0`, want `0/0/1`, and
  have-plus-want `1/0/1`.
- Proved pressing **Have** preserves **Want**, while selecting **Want**
  preserves **Have** and clears only an incompatible available choice.
- Proved **Available to swap** and **Want** cannot remain selected together.
  A failed save restores the server-confirmed state and keyboard focus.
- Confirmed cards without a copy have muted, translucent artwork while owned
  and available cards render normally.
- Confirmed there is no quantity dialog and no member-facing source,
  provenance, licensing, or review-date copy.

The SQLite and HTTP layers still accept bounded numeric quantities for
compatibility. The browser interprets any positive value as selected and writes
canonical `0` or `1` values only when the member interacts with a card
control.

### Fulfillable, unfulfillable, and no-request matches

The matching journey reproduces the reported Maui case:

1. Alex marks Maui as wanted.
2. Bailey marks Maui as available.
3. Alex receives one possible-swap lead for Bailey even though Bailey has no
   wanted item. The card says Bailey has the item available without suggesting
   that Alex owes an item in return.
4. The navigation badge reports one possible swap from
   `summary.possible_swap_count` without eagerly requesting the matches page.
5. Alex marks Acorn Keeper available; Casey marks Maui available and Acorn
   Keeper wanted. Casey is shown first with **They want your Acorn Keeper**.
6. Devon marks Maui available and Beacon Blue wanted while Alex has no Beacon
   Blue available. Devon appears in **Swaps you can't fulfil yet** with a muted
   Beacon Blue and is never described as asking for nothing.

The test asserts that the required receive quantity and currently offerable
request quantity are independent. The member-facing page has no
repository-oriented catalog or scope filters: it groups actionable results
under **You want Maui**, lists fulfilable requests first, describes a genuine
no-request row as **They haven't asked for anything back**, and keeps
unfulfillable requests separate. It also proves that the secondary feed makes
no request before the section opens, uses its own cursor-only continuation and
error/retry state, and is collapsed and reset by Refresh.

### Session, routing, and responsive behavior

- A failed in-place catalog change clears the previous catalog identity and
  keeps the new route's error state.
- Expired CSRF refreshes once during sign-out; a fully expired session still
  clears all private DOM and returns to the profile chooser.
- The skip link remains keyboard-operable without replacing the hash route.
- Every collectible image has meaningful alternative text and every primary
  touch control meets the 44-pixel target used by the test.
- Authenticated 390-pixel and 320-pixel layouts show the badge, leads, and
  inline controls without horizontal document overflow.
- Forced-colour mode preserves state and focus visibility. Reduced-motion
  preferences disable non-essential movement.

The deliberately induced `503`, `404`, `403`, and `401` responses produce
Chromium's normal failed-resource diagnostics. Tests allow those exact expected
lines; every other console error and every uncaught page error fails the run.

## Deterministic boundaries

The harness reduces first-page limits so visible continuation controls and real
encrypted cursors are exercised. Cursor requests themselves are not rewritten,
and all application HTML, JavaScript, CSS, authentication, CSRF, API,
persistence, summary, and matching behavior comes from the live Go server.

The test intercepts only third-party Woolworths image requests and returns a
small valid PNG. This removes external-host availability from a deterministic
repository test. Embedded bean artwork and fallback artwork are served by the
application itself.

This is a focused keyboard and journey qualification, not a complete WCAG
audit or an availability test of any external artwork host.

## Supporting regression checks

```text
node --test web/tests/*.test.mjs
13 passed, 0 failed

go test ./...
all Go packages passed
```

The Node suite separately fixes the bounded response parser, fulfilable,
unfulfillable, and absent request shapes, exact possible-swap count, canonical
state transitions, cursor behavior, stale generations, and deep-link round
trips.
