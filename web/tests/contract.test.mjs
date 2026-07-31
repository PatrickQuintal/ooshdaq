import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { pageURL } from "../js/api.js";
import { groupMatchesByWantedItem } from "../js/match-groups.js";
import {
  isCanonicalCollectionState,
  nextCollectionQuantities,
  parseCatalogPage,
  parseCollectionPage,
  parseMatchPage,
  parseOverallSummary,
  parseSavedEntry,
} from "../js/models.js";
import {
  acceptPage,
  beginRequest,
  finishRequest,
  isCurrent,
} from "../js/page-state.js";
import { parseHash, routeHash } from "../js/router.js";

async function fixture(name) {
  const url = new URL(`../../testdata/contracts/${name}`, import.meta.url);
  return JSON.parse(await readFile(url, "utf8"));
}

test("parses the five-catalog navigation fixture exactly", async () => {
  const page = parseCatalogPage(await fixture("catalogs-page.json"));
  assert.equal(page.catalogs.length, 5);
  assert.equal(new Set(page.catalogs.map((item) => item.brand.id)).size, 3);
  assert.deepEqual(
    page.catalogs.map((item) => item.sortOrder),
    [10, 20, 30, 40, 50],
  );
  assert.equal(page.page.nextCursor, null);
});

test("accepts the documented source-hosted artwork status", async () => {
  const payload = await fixture("catalogs-page.json");
  payload.catalogs[0].artwork.rights_status = "source-hosted-link";
  payload.catalogs[0].artwork.image_url =
    "https://cdn0.woolworths.media/example.png";
  const page = parseCatalogPage(payload);
  assert.equal(page.catalogs[0].artwork.rightsStatus, "source-hosted-link");
});

test("parses bounded collection pages and restricted artwork", async () => {
  const first = parseCollectionPage(await fixture("collection-page-1.json"));
  const second = parseCollectionPage(await fixture("collection-page-2.json"));
  assert.equal(first.entries.length, 2);
  assert.equal(first.page.limit, 2);
  assert.ok(first.page.nextCursor);
  assert.equal(second.entries[0].catalogItem.publicationStatus, "retired");
  assert.equal(second.entries[0].catalogItem.artwork.imageUrl, "");
  assert.equal(second.page.nextCursor, null);
});

test("parses bounded numeric mutation and aggregate summary", async () => {
  const entry = parseSavedEntry(await fixture("collection-put.json"));
  const combinedPayload = await fixture("collection-put.json");
  combinedPayload.entry.owned = 3;
  combinedPayload.entry.available = 0;
  combinedPayload.entry.wanted = 2;
  const combined = parseSavedEntry(combinedPayload);
  const invalidPayload = await fixture("collection-put.json");
  invalidPayload.entry.owned = 3;
  invalidPayload.entry.available = 1;
  invalidPayload.entry.wanted = 1;
  const summary = parseOverallSummary(await fixture("collection-summary.json"));
  assert.equal(entry.catalogItem.catalogID, "paper-orbit-stargazers-2026");
  assert.equal(entry.available, 2);
  assert.deepEqual(
    {
      owned: combined.owned,
      available: combined.available,
      wanted: combined.wanted,
    },
    { owned: 3, available: 0, wanted: 2 },
  );
  assert.throws(
    () => parseSavedEntry(invalidPayload),
    /wanted cannot be combined with available/,
  );
  assert.equal(summary.active_catalog_count, 4);
  assert.equal(summary.active_item_count, 11);
  assert.equal(summary.possible_swap_count, 2);
});

test("parses fulfillable, unfulfillable, and absent counterpart requests", async () => {
  const actionablePayload = await fixture("matches-actionable-page.json");
  const unfulfillablePayload = await fixture(
    "matches-unfulfillable-page.json",
  );
  const mixedPayload = {
    ...actionablePayload,
    matches: [
      actionablePayload.matches[0],
      unfulfillablePayload.matches[0],
      actionablePayload.matches[1],
    ],
    page: { ...actionablePayload.page, returned: 3 },
  };
  const page = parseMatchPage(mixedPayload);
  assert.equal(page.matches.length, 3);
  const [fulfillable, unfulfillable, noRequest] = page.matches;
  assert.equal(
    fulfillable.theyRequest.catalogItem.catalogID,
    fulfillable.youReceive.catalogItem.catalogID,
  );
  assert.equal(Array.isArray(fulfillable.theyRequest), false);
  assert.equal(fulfillable.theyRequest.quantity, 1);
  assert.equal(fulfillable.theyRequest.fulfillable, true);
  assert.equal(fulfillable.youReceive.quantity, 2);
  assert.equal(unfulfillable.theyRequest.catalogItem.name, "Coral Token");
  assert.equal(unfulfillable.theyRequest.quantity, 0);
  assert.equal(unfulfillable.theyRequest.fulfillable, false);
  assert.equal(noRequest.theyRequest, null);
  assert.equal(noRequest.youReceive.catalogItem.name, "Summit Badge");
});

test("rejects ambiguous or inconsistent counterpart request shapes", async () => {
  const cases = [
    { quantity: 0, fulfillable: true },
    { quantity: 1, fulfillable: false },
  ];
  for (const request of cases) {
    const payload = await fixture("matches-actionable-page.json");
    Object.assign(payload.matches[0].they_request, request);
    assert.throws(
      () => parseMatchPage(payload),
      /fulfillable must be true exactly when quantity is positive/,
    );
  }

  const missingFlag = await fixture("matches-actionable-page.json");
  delete missingFlag.matches[0].they_request.fulfillable;
  assert.throws(
    () => parseMatchPage(missingFlag),
    /fulfillable must be a boolean/,
  );

  const explicitNull = await fixture("matches-actionable-page.json");
  explicitNull.matches[0].they_request = null;
  assert.throws(
    () => parseMatchPage(explicitNull),
    /they_request must be an object/,
  );

  const legacy = await fixture("matches-actionable-page.json");
  legacy.matches[0].you_give = legacy.matches[0].they_request;
  delete legacy.matches[0].they_request;
  assert.throws(() => parseMatchPage(legacy), /you_give is no longer supported/);
});

test("enforces the actionable and unfulfillable feed boundaries", async () => {
  const actionablePayload = await fixture("matches-actionable-page.json");
  const unfulfillablePayload = await fixture(
    "matches-unfulfillable-page.json",
  );

  const actionable = parseMatchPage(actionablePayload, "actionable");
  const unfulfillable = parseMatchPage(
    unfulfillablePayload,
    "unfulfillable",
  );
  assert.equal(actionable.matches.length, 2);
  assert.equal(unfulfillable.matches.length, 1);
  assert.equal(unfulfillable.matches[0].theyRequest.fulfillable, false);
  assert.throws(
    () => parseMatchPage(unfulfillablePayload, "actionable"),
    /is not actionable/,
  );
  assert.throws(
    () => parseMatchPage(actionablePayload, "unfulfillable"),
    /is not unfulfillable/,
  );
  assert.throws(
    () => parseMatchPage(actionablePayload, "waiting"),
    /match readiness has an unsupported value/,
  );
});

test("groups actionable item requests before genuine no-request rows", async () => {
  const page = parseMatchPage(
    await fixture("matches-actionable-page.json"),
    "actionable",
  );
  const withItemRequest = page.matches[0];
  const withoutRequest = {
    ...page.matches[1],
    id: "same-item-without-request",
    youReceive: withItemRequest.youReceive,
  };
  const { wantedItemGroups, wantedItemCount } = groupMatchesByWantedItem([
    withoutRequest,
    withItemRequest,
    { ...withoutRequest },
    page.matches[1],
  ]);

  assert.equal(wantedItemGroups.length, 2);
  assert.equal(wantedItemGroups[0].catalogItem.name, "Moon Glider");
  assert.deepEqual(
    wantedItemGroups[0].matches.map((match) => match.id),
    [withItemRequest.id, withoutRequest.id],
  );
  assert.equal(wantedItemGroups[1].catalogItem.name, "Summit Badge");
  assert.equal(wantedItemCount, 2);
});

test("inline controls always emit one canonical collection state", () => {
  const numericAvailable = { owned: 3, available: 2, wanted: 0 };
  const numericWanted = { owned: 0, available: 0, wanted: 4 };
  const numericOwned = { owned: 3, available: 0, wanted: 0 };
  const numericOwnedWanted = { owned: 3, available: 0, wanted: 4 };
  const transitions = [
    [numericWanted, "owned", true, { owned: 1, available: 0, wanted: 1 }],
    [numericOwnedWanted, "owned", false, { owned: 0, available: 0, wanted: 1 }],
    [numericAvailable, "owned", false, { owned: 0, available: 0, wanted: 0 }],
    [numericWanted, "available", true, { owned: 1, available: 1, wanted: 0 }],
    [numericOwnedWanted, "available", true, { owned: 1, available: 1, wanted: 0 }],
    [numericAvailable, "available", false, { owned: 1, available: 0, wanted: 0 }],
    [numericAvailable, "wanted", true, { owned: 1, available: 0, wanted: 1 }],
    [numericOwned, "wanted", true, { owned: 1, available: 0, wanted: 1 }],
    [numericOwnedWanted, "wanted", false, { owned: 1, available: 0, wanted: 0 }],
    [numericWanted, "wanted", false, { owned: 0, available: 0, wanted: 0 }],
  ];
  for (const [entry, control, checked, expected] of transitions) {
    const next = nextCollectionQuantities(entry, control, checked);
    assert.deepEqual(next, expected);
    assert.equal(isCanonicalCollectionState(next), true);
  }
  assert.throws(
    () =>
      nextCollectionQuantities(
        { owned: 1, available: 1, wanted: 1 },
        "owned",
        false,
      ),
    /collection entry has an invalid state/,
  );
});

test("match feed URLs select readiness only on the first page", () => {
  assert.equal(
    pageURL("/api/v1/matches", {
      readiness: "actionable",
      limit: 20,
    }),
    "/api/v1/matches?readiness=actionable&limit=20",
  );
  assert.equal(
    pageURL("/api/v1/matches", {
      readiness: "unfulfillable",
      limit: 20,
    }),
    "/api/v1/matches?readiness=unfulfillable&limit=20",
  );
  const continuation = pageURL(
    "/api/v1/matches",
    { readiness: "unfulfillable", limit: 20 },
    "signed.cursor",
  );
  assert.equal(continuation, "/api/v1/matches?cursor=signed.cursor");
});

test("load-more state appends unique rows and advances the cursor stack", () => {
  const resource = {
    page: {
      items: [],
      limit: 2,
      requestCursor: null,
      nextCursor: null,
      backStack: [],
      number: 1,
    },
  };
  acceptPage(
    resource,
    [{ id: "one" }, { id: "two" }],
    { limit: 2, nextCursor: "next" },
    null,
    (item) => item.id,
  );
  acceptPage(
    resource,
    [{ id: "two" }, { id: "three" }, { id: "three" }],
    { limit: 2, nextCursor: "next-2" },
    "next",
    (item) => item.id,
  );
  acceptPage(
    resource,
    [{ id: "three" }, { id: "three" }],
    { limit: 2, nextCursor: null },
    "next-2",
    (item) => item.id,
  );
  assert.deepEqual(resource.page.items.map((item) => item.id), [
    "one",
    "two",
    "three",
  ]);
  assert.deepEqual(resource.page.backStack, [null, "next"]);
  assert.equal(resource.page.number, 3);
});

test("match feeds have independent request generations and controllers", () => {
  const first = beginRequest("matches");
  const lazy = beginRequest("unfulfillableMatches");
  assert.equal(first.controller.signal.aborted, false);
  assert.equal(lazy.controller.signal.aborted, false);
  const second = beginRequest("matches");
  assert.equal(first.controller.signal.aborted, true);
  assert.equal(lazy.controller.signal.aborted, false);
  assert.equal(isCurrent("matches", first.generation, first.controller), false);
  assert.equal(isCurrent("matches", second.generation, second.controller), true);
  assert.equal(
    isCurrent(
      "unfulfillableMatches",
      lazy.generation,
      lazy.controller,
    ),
    true,
  );
  finishRequest("matches", second.generation, second.controller, () => {});
  finishRequest(
    "unfulfillableMatches",
    lazy.generation,
    lazy.controller,
    () => {},
  );
});

test("collection links retain filters while matches always open the default view", () => {
  const collection = {
    name: "collection",
    catalogID: "paper-orbit-stargazers-2026",
    filters: { q: "moon", universe: "Moon", state: "wanted" },
  };
  assert.deepEqual(parseHash(routeHash(collection)), collection);

  const matches = {
    name: "matches",
    catalogID: null,
  };
  assert.equal(routeHash(matches), "#/matches");
  assert.deepEqual(
    parseHash(
      "#/matches?give_catalog_id=paper-orbit-stargazers-2026" +
        "&receive_catalog_id=cedar-kite-trail-signs-2026&scope=cross_catalog",
    ),
    matches,
  );

  assert.deepEqual(
    parseHash("#/dashboard?q=Acorn+Keeper&brand_id=ignored&publication_status=all"),
    {
      name: "dashboard",
      catalogID: null,
      filters: { q: "Acorn Keeper" },
    },
  );
});
