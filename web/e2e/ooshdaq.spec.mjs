import { expect, test } from "@playwright/test";
import {
  assertCollectionState,
  catalogs,
  collectibleCard,
  login,
  makeDeterministic,
  openCollection,
  setCollectionState,
  watchBrowser,
} from "./support.mjs";

async function horizontalOverflow(page) {
  return page.locator("body *").evaluateAll((elements) =>
    elements
      .map((element) => {
        const bounds = element.getBoundingClientRect();
        return {
          element:
            element.id ||
            element.className ||
            element.tagName.toLowerCase(),
          left: Math.round(bounds.left + window.scrollX),
          right: Math.round(bounds.right + window.scrollX),
        };
      })
      .filter(
        ({ left, right }) =>
          left < -1 || right > document.documentElement.clientWidth + 1,
      ),
  );
}

test.describe.serial("$OOSHDAQ live browser journeys", () => {
  test("lazy navigation, collectible search, and keyboard-friendly inline states", async ({
    page,
  }) => {
    await makeDeterministic(page);
    const browserIssues = watchBrowser(page);
    const APIPaths = [];
    const catalogRequests = [];
    const activityRequests = [];
    page.on("request", (request) => {
      const url = new URL(request.url());
      if (url.pathname.startsWith("/api/")) APIPaths.push(url.pathname);
      if (url.pathname === "/api/v1/catalogs") catalogRequests.push(url);
      if (url.pathname === "/api/v1/activity") {
        activityRequests.push(request);
      }
    });

    await page.goto("/catalog/disney-ooshies-2026");
    await expect(page.locator("#mock-user-list .profile-button")).toHaveCount(4);
    const skipLink = page.getByRole("link", { name: "Skip to main content" });
    await page.keyboard.press("Tab");
    await expect(skipLink).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(page.locator("#main-content")).toBeFocused();
    await login(page, "mock-alex", "Alex");
    await expect.poll(() => activityRequests.length).toBe(1);
    expect(activityRequests[0].method()).toBe("POST");
    expect(activityRequests[0].postData()).toBeNull();
    expect(activityRequests[0].headers()["x-csrf-token"]).toBeTruthy();

    await expect(page.locator("article.catalog-card")).toHaveCount(2);
    await expect(page.locator(".catalog-card .status-badge")).toHaveCount(0);
    expect(APIPaths).toContain("/api/v1/catalogs");
    expect(
      APIPaths.filter((path) => path === "/api/v1/catalogs"),
    ).toHaveLength(1);
    expect(APIPaths).toContain("/api/v1/collection/summary");
    expect(APIPaths).not.toContain("/api/v1/matches");
    expect(
      APIPaths.some((path) => path.endsWith("/collection")),
    ).toBeFalsy();

    const moreCatalogs = page.getByRole("button", {
      name: "Load more catalogs",
    });
    await expect(moreCatalogs).toBeVisible();
    await moreCatalogs.click();
    await expect(page.locator("article.catalog-card")).toHaveCount(4);

    await expect(
      page.locator("#catalog-brand, #catalog-status"),
    ).toHaveCount(0);
    await page.getByLabel("Find a collectible").fill("Acorn Keeper");
    await page
      .locator("#catalog-filters")
      .getByRole("button", { name: "Search" })
      .click();
    await expect(page.locator("article.catalog-card")).toHaveCount(1);
    await expect(
      page.getByRole("heading", { name: catalogs.garden.name }),
    ).toBeVisible();
    const collectibleSearch = catalogRequests.find(
      (url) => url.searchParams.get("q") === "Acorn Keeper",
    );
    expect(collectibleSearch).toBeTruthy();
    expect(collectibleSearch.searchParams.get("publication_status")).toBe(
      "active",
    );
    expect(collectibleSearch.searchParams.has("brand_id")).toBeFalsy();

    const resultLink = page.getByRole("link", { name: "Open collection" });
    await expect(resultLink).toHaveAttribute(
      "href",
      `#/collection/${catalogs.garden.id}?q=Acorn+Keeper`,
    );
    await resultLink.click();
    await expect(page).toHaveURL(
      new RegExp(
        `#\\/collection\\/${catalogs.garden.id}\\?q=Acorn(?:\\+|%20)Keeper`,
      ),
    );
    await expect(
      page.getByRole("heading", { name: catalogs.garden.name, exact: true }),
    ).toBeVisible();
    await expect(page.getByLabel("Search collectibles")).toHaveValue(
      "Acorn Keeper",
    );
    await expect(page.locator("article.collectible-card")).toHaveCount(1);
    await expect(collectibleCard(page, "Acorn Keeper")).toBeVisible();

    await page.getByLabel("Search collectibles").fill("");
    await page
      .locator("#collection-filters")
      .getByRole("button", { name: "Apply" })
      .click();
    await expect(page.locator("article.collectible-card")).toHaveCount(2);
    await page
      .getByRole("button", { name: "Load more collectibles" })
      .click();
    await expect(page.locator("article.collectible-card")).toHaveCount(4);

    await page.getByLabel("Search collectibles").fill("Acorn");
    await page
      .locator("#collection-filters")
      .getByRole("button", { name: "Apply" })
      .click();
    await expect(page.locator("article.collectible-card")).toHaveCount(1);
    const card = collectibleCard(page, "Acorn Keeper");
    await expect(card).toBeVisible();
    await expect(card.getByRole("checkbox")).toHaveCount(2);
    await expect(
      card.getByRole("button", { name: "Have Acorn Keeper" }),
    ).toHaveAttribute("aria-pressed", "false");
    await expect(card).toHaveClass(/is-unowned/);

    await page.reload();
    await expect(
      page.getByRole("heading", { name: catalogs.garden.name, exact: true }),
    ).toBeVisible();
    await expect(page.getByLabel("Search collectibles")).toHaveValue("Acorn");
    await expect(card).toBeVisible();

    await setCollectionState(page, "Acorn Keeper", "wanted", {
      keyboard: true,
    });
    await expect(card).toHaveClass(/is-wanted/);
    await expect(card).toHaveClass(/is-unowned/);
    await setCollectionState(page, "Acorn Keeper", "ownedWanted", {
      keyboard: true,
    });
    await expect(card).toHaveClass(/is-owned/);
    await expect(card).toHaveClass(/is-wanted/);
    await expect(card).not.toHaveClass(/is-available/);
    await setCollectionState(page, "Acorn Keeper", "wanted", {
      keyboard: true,
    });
    await expect(card).toHaveClass(/is-unowned/);
    await expect(card).toHaveClass(/is-wanted/);
    await setCollectionState(page, "Acorn Keeper", "available", {
      keyboard: true,
    });
    await expect(card).toHaveClass(/is-available/);
    await expect(card).not.toHaveClass(/is-wanted/);
    await setCollectionState(page, "Acorn Keeper", "neutral", {
      keyboard: true,
    });
    await expect(card).toHaveClass(/is-unowned/);
    await setCollectionState(page, "Acorn Keeper", "owned", {
      keyboard: true,
    });
    await expect(card).toHaveClass(/is-owned/);
    await setCollectionState(page, "Acorn Keeper", "available", {
      keyboard: true,
    });
    await setCollectionState(page, "Acorn Keeper", "ownedWanted", {
      keyboard: true,
    });
    await expect(card).toHaveClass(/is-owned/);
    await expect(card).toHaveClass(/is-wanted/);
    await expect(card).not.toHaveClass(/is-available/);
    await setCollectionState(page, "Acorn Keeper", "owned", {
      keyboard: true,
    });
    await expect(card).toHaveClass(/is-owned/);
    await expect(card).not.toHaveClass(/is-wanted/);
    await setCollectionState(page, "Acorn Keeper", "neutral", {
      keyboard: true,
    });
    await expect(card).toHaveClass(/is-unowned/);

    await expect(page.locator("dialog")).toHaveCount(0);
    await expect(page.locator(".quantity-pill, .provenance")).toHaveCount(0);
    const hashBeforeSkip = await page.evaluate(() => window.location.hash);
    await skipLink.evaluate((link) => link.click());
    await expect(page.locator("#main-content")).toBeFocused();
    expect(await page.evaluate(() => window.location.hash)).toBe(hashBeforeSkip);

    const imageAltFailures = await page
      .locator(".collectible-card img")
      .evaluateAll(
        (images) => images.filter((image) => !image.alt.trim()).length,
      );
    expect(imageAltFailures).toBe(0);
    expect(browserIssues).toEqual([]);
  });

  test("a failed activity write does not block authenticated bootstrap", async ({
    page,
  }) => {
    await makeDeterministic(page);
    const browserIssues = watchBrowser(page);
    await page.route(
      "**/api/v1/activity",
      async (route) => {
        await route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({
            error: {
              code: "activity_unavailable",
              message: "Test activity write did not complete.",
            },
          }),
        });
      },
      { times: 1 },
    );

    await page.goto("/");
    const responsePromise = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new URL(response.url()).pathname === "/api/v1/activity",
    );
    await login(page, "mock-alex", "Alex");
    expect((await responsePromise).status()).toBe(503);
    await expect(page.locator("article.catalog-card").first()).toBeVisible();
    expect(browserIssues).toEqual([
      "console: Failed to load resource: the server responded with a status of 503 (Service Unavailable)",
    ]);
  });

  test("a failed inline save rolls back and keeps keyboard focus", async ({
    page,
  }) => {
    await makeDeterministic(page);
    const browserIssues = watchBrowser(page);
    await page.goto("/");
    await login(page, "mock-alex", "Alex");
    await openCollection(page, catalogs.garden);
    await page.getByLabel("Search collectibles").fill("Acorn Keeper");
    await page
      .locator("#collection-filters")
      .getByRole("button", { name: "Apply" })
      .click();
    await assertCollectionState(page, "Acorn Keeper", "neutral");

    await page.route(
      "**/api/v1/collection/**",
      async (route) => {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({
            error: {
              code: "save_failed",
              message: "Test save did not complete.",
            },
          }),
        });
      },
      { times: 1 },
    );
    const card = collectibleCard(page, "Acorn Keeper");
    const have = card.getByRole("button", {
      name: "Have Acorn Keeper",
      exact: true,
    });
    const responsePromise = page.waitForResponse(
      (response) =>
        response.request().method() === "PUT" &&
        response.url().includes("/api/v1/collection/"),
    );
    await have.focus();
    await page.keyboard.press("Space");
    expect((await responsePromise).status()).toBe(500);

    await assertCollectionState(page, "Acorn Keeper", "neutral");
    await expect(card.getByRole("alert")).toContainText(
      "Test save did not complete.",
    );
    await expect(have).toBeEnabled();
    await expect(have).toBeFocused();
    expect(browserIssues).toEqual([
      "console: Failed to load resource: the server responded with a status of 500 (Internal Server Error)",
    ]);
  });

  test("catalog changes clear stale identity on failure", async ({ page }) => {
    await makeDeterministic(page);
    const browserIssues = watchBrowser(page);
    await page.goto("/");
    await login(page, "mock-alex", "Alex");
    await openCollection(page, catalogs.garden);

    let releaseResponse;
    const responseGate = new Promise((resolve) => {
      releaseResponse = resolve;
    });
    await page.route(
      `**/api/v1/catalogs/${catalogs.harbor.id}/collection**`,
      async (route) => {
        await responseGate;
        await route.fulfill({
          status: 404,
          contentType: "application/json",
          body: JSON.stringify({
            error: {
              code: "not_found",
              message: "The requested catalog was not found.",
            },
          }),
        });
      },
    );

    try {
      await page.evaluate((catalogID) => {
        window.location.hash = `#/collection/${catalogID}`;
      }, catalogs.harbor.id);
      await expect(
        page.getByRole("heading", { name: "Collection", exact: true }),
      ).toBeVisible();
      await expect(page.locator("article.collectible-card")).toHaveCount(0);
    } finally {
      releaseResponse();
    }

    await expect(
      page.getByRole("heading", { name: "Collection unavailable" }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Collection", exact: true }),
    ).toBeVisible();
    await expect(page.locator("article.collectible-card")).toHaveCount(0);
    expect(browserIssues).toEqual([
      "console: Failed to load resource: the server responded with a status of 404 (Not Found)",
    ]);
  });

  test("expired CSRF and session state still complete local sign-out", async ({
    page,
  }) => {
    await makeDeterministic(page);
    const browserIssues = watchBrowser(page);
    await page.goto("/");
    await login(page, "mock-alex", "Alex");
    await openCollection(page, catalogs.garden);
    await expect(page.locator("article.collectible-card").first()).toBeVisible();

    const logoutStatuses = [];
    page.on("response", (response) => {
      if (
        response.request().method() === "POST" &&
        new URL(response.url()).pathname === "/auth/logout"
      ) {
        logoutStatuses.push(response.status());
      }
    });
    await page.context().clearCookies({ name: "ooshdaq_csrf" });
    await page.getByRole("button", { name: "Sign out" }).click();
    await expect.poll(() => logoutStatuses).toEqual([403, 204]);
    await expect(page.getByRole("heading", { name: "$OOSHDAQ" })).toBeVisible();
    await expect(page.locator("article.collectible-card")).toHaveCount(0);

    await login(page, "mock-alex", "Alex");
    await openCollection(page, catalogs.garden);
    await expect(page.locator("article.collectible-card").first()).toBeVisible();
    await page.context().clearCookies();
    await page.getByRole("button", { name: "Sign out" }).click();
    await expect.poll(() => logoutStatuses).toEqual([403, 204, 401]);
    await expect(page.getByRole("heading", { name: "$OOSHDAQ" })).toBeVisible();
    await expect(page.locator("article.collectible-card")).toHaveCount(0);
    expect(browserIssues).toEqual([
      "console: Failed to load resource: the server responded with a status of 403 (Forbidden)",
      "console: Failed to load resource: the server responded with a status of 401 (Unauthorized)",
    ]);
  });

  test("groups people by wanted item and explains what each person wants", async ({
    page,
  }) => {
    await makeDeterministic(page);
    const browserIssues = watchBrowser(page);
    const requestedPaths = [];
    const matchRequests = [];
    page.on("request", (request) => {
      const url = new URL(request.url());
      requestedPaths.push(url.pathname);
      if (url.pathname === "/api/v1/matches") matchRequests.push(url);
    });

    await page.goto("/");
    await login(page, "mock-alex", "Alex");
    await openCollection(page, catalogs.disney);
    await setCollectionState(page, "Maui", "wanted");
    await setCollectionState(page, "Moana", "wanted");

    await page.getByRole("button", { name: "Sign out" }).click();
    await login(page, "mock-bailey", "Bailey");
    await openCollection(page, catalogs.disney);
    await setCollectionState(page, "Maui", "available");

    await page.getByRole("button", { name: "Sign out" }).click();
    await login(page, "mock-alex", "Alex");
    const badge = page.locator("#matches-nav-badge");
    await expect(badge).toHaveText("1");
    await expect(badge).toHaveAttribute(
      "aria-label",
      "1 possible swap",
    );
    expect(requestedPaths).not.toContain("/api/v1/matches");

    await page.getByRole("link", { name: /Matches/ }).click();
    await expect(
      page.getByRole("heading", { name: "Possible swaps" }),
    ).toBeVisible();
    await expect(
      page.locator(
        "#match-filters, #match-give-catalog, #match-receive-catalog, #match-scope",
      ),
    ).toHaveCount(0);
    let group = page.locator("section.match-group");
    await expect(group).toHaveCount(1);
    await expect(
      group.getByRole("heading", { name: "Maui", exact: true }),
    ).toBeVisible();
    let people = group.locator("article.match-person");
    await expect(people).toHaveCount(1);
    await expect(people.first()).toContainText("Bailey Example");
    await expect(people.first()).toContainText(
      "They haven’t asked for anything back.",
    );
    await expect(people.first()).not.toContainText("They want your");
    let pending = page.locator("section.match-pending");
    await expect(pending).toBeVisible();
    const pendingToggle = pending.getByRole("button", {
      name: "Check swaps you can’t fulfil yet",
    });
    await expect(pendingToggle).toHaveAttribute("aria-expanded", "false");
    await expect(pending.locator("article.match-person")).toHaveCount(0);
    expect(
      matchRequests.filter(
        (url) => url.searchParams.get("readiness") === "unfulfillable",
      ),
    ).toHaveLength(0);
    await expect(
      page.locator("[data-view=\"matches\"] .provenance, [data-view=\"matches\"] .quantity-pill"),
    ).toHaveCount(0);

    const firstMatchRequest = matchRequests.find(
      (url) => !url.searchParams.has("cursor"),
    );
    expect(firstMatchRequest).toBeTruthy();
    expect(firstMatchRequest.searchParams.has("limit")).toBeTruthy();
    expect(firstMatchRequest.searchParams.get("readiness")).toBe("actionable");
    for (const parameter of [
      "give_catalog_id",
      "receive_catalog_id",
      "scope",
    ]) {
      expect(firstMatchRequest.searchParams.has(parameter)).toBeFalsy();
    }

    await page.goto(
      "/#/matches?give_catalog_id=garden-sprites-2026" +
        "&receive_catalog_id=disney-ooshies-2026&scope=same_catalog",
    );
    await expect(page).toHaveURL(/#\/matches$/);
    await expect(page.locator("section.match-group")).toHaveCount(1);

    await openCollection(page, catalogs.garden);
    await setCollectionState(page, "Acorn Keeper", "available");
    await page.getByRole("button", { name: "Sign out" }).click();
    await login(page, "mock-casey", "Casey");
    await openCollection(page, catalogs.disney);
    await setCollectionState(page, "Maui", "available");
    await openCollection(page, catalogs.garden);
    await setCollectionState(page, "Acorn Keeper", "wanted");

    await page.getByRole("button", { name: "Sign out" }).click();
    await login(page, "mock-devon", "Devon");
    await openCollection(page, catalogs.disney);
    await setCollectionState(page, "Maui", "available");
    await setCollectionState(page, "Moana", "available");
    await openCollection(page, catalogs.harbor);
    await setCollectionState(page, "Beacon Blue", "wanted");

    await page.getByRole("button", { name: "Sign out" }).click();
    await login(page, "mock-alex", "Alex");
    await expect(badge).toHaveText("4");
    await expect(badge).toHaveAttribute("aria-label", "4 possible swaps");
    await page.getByRole("link", { name: /Matches/ }).click();
    group = page.locator("section.match-group");
    await expect(group).toHaveCount(1);
    people = group.locator("article.match-person");
    await expect(people).toHaveCount(1);
    await expect(people.first()).toContainText("Casey Example");
    await expect(people.first()).toContainText(
      "They want your Acorn Keeper.",
    );

    await page.getByRole("button", { name: "Load more", exact: true }).click();
    await expect(group).toHaveCount(1);
    await expect(people).toHaveCount(2);
    await expect(people.nth(0)).toContainText("Casey Example");
    await expect(people.nth(0)).toContainText(
      "They want your Acorn Keeper.",
    );
    await expect(people.nth(1)).toContainText("Bailey Example");
    await expect(people.nth(1)).toContainText(
      "They haven’t asked for anything back.",
    );

    pending = page.locator("section.match-pending");
    await expect(pending).toBeVisible();
    await expect(pending.locator("article.match-person")).toHaveCount(0);
    expect(
      matchRequests.filter(
        (url) => url.searchParams.get("readiness") === "unfulfillable",
      ),
    ).toHaveLength(0);

    await pending
      .getByRole("button", {
        name: "Check swaps you can’t fulfil yet",
      })
      .click();
    await expect(group).toHaveCount(1);
    await expect(people).toHaveCount(2);
    await expect
      .poll(
        () =>
          matchRequests.filter(
            (url) => url.searchParams.get("readiness") === "unfulfillable",
          ).length,
      )
      .toBe(1);
    const unfulfillableInitial = matchRequests.find(
      (url) => url.searchParams.get("readiness") === "unfulfillable",
    );
    expect(unfulfillableInitial.searchParams.get("limit")).toBeTruthy();
    await expect(
      pending.getByRole("heading", { name: "Swaps you can’t fulfil yet" }),
    ).toBeVisible();
    let pendingPeople = pending.locator("article.match-person");
    await expect(pendingPeople).toHaveCount(1);
    await expect(pendingPeople.first()).toContainText("Devon Example");
    await expect(pendingPeople.first()).toContainText(
      /Devon has (?:Moana|Maui) available\./,
    );
    await expect(pendingPeople.first()).toContainText(
      "They’re looking for Beacon Blue.",
    );
    await expect(pendingPeople.first()).toContainText(
      "You don’t have one available to swap yet.",
    );
    await expect(pendingPeople.first()).not.toContainText(
      "They haven’t asked for anything back.",
    );
    await expect(pendingPeople.first()).not.toContainText(
      "They want your Beacon Blue.",
    );
    await expect(
      pendingPeople
        .first()
        .locator(".match-ask--unfulfillable .match-ask__art img"),
    ).toBeVisible();

    await pending
      .getByRole("button", { name: "Load more", exact: true })
      .click();
    await expect(pendingPeople).toHaveCount(2);
    await expect(pending).toContainText(
      "Devon has Moana available.",
    );
    await expect(pending).toContainText(
      "Devon has Maui available.",
    );
    await expect(pending.locator(".match-pending__results")).toContainText(
      "2 swaps loaded",
    );
    const lazyContinuation = matchRequests.at(-1);
    expect(lazyContinuation.searchParams.has("cursor")).toBeTruthy();
    expect(lazyContinuation.searchParams.has("readiness")).toBeFalsy();
    await expect(page.locator("#matches-results")).toContainText(
      "2 possibilities loaded for 1 wanted item",
    );
    const matchesText = await page
      .locator("[data-view=\"matches\"]")
      .innerText();
    expect(matchesText).not.toMatch(
      /\b(?:lead|reciprocal|scope)\b|same catalog|cross catalog/i,
    );

    for (const url of matchRequests) {
      for (const parameter of [
        "give_catalog_id",
        "receive_catalog_id",
        "scope",
      ]) {
        expect(url.searchParams.has(parameter)).toBeFalsy();
      }
      if (url.searchParams.has("cursor")) {
        expect([...url.searchParams.keys()]).toEqual(["cursor"]);
      } else {
        expect(["actionable", "unfulfillable"]).toContain(
          url.searchParams.get("readiness"),
        );
      }
    }

    const unfulfillableRequestCount = matchRequests.filter(
      (url) => url.searchParams.get("readiness") === "unfulfillable",
    ).length;
    await page.getByRole("button", { name: "Refresh" }).click();
    await expect(page.locator("section.match-group")).toHaveCount(1);
    await expect(page.locator("article.match-person")).toHaveCount(1);
    await expect(
      pending.getByRole("button", {
        name: "Check swaps you can’t fulfil yet",
      }),
    ).toHaveAttribute("aria-expanded", "false");
    await expect(pending.locator("article.match-person")).toHaveCount(0);
    expect(
      matchRequests.filter(
        (url) => url.searchParams.get("readiness") === "unfulfillable",
      ).length,
    ).toBe(unfulfillableRequestCount);
    await page.reload();
    await expect(
      page.getByRole("heading", { name: "Possible swaps" }),
    ).toBeVisible();
    await expect(page.locator("section.match-group")).toHaveCount(1);

    await openCollection(page, catalogs.disney);
    await assertCollectionState(page, "Maui", "wanted");
    await assertCollectionState(page, "Moana", "wanted");
    await openCollection(page, catalogs.garden);
    await assertCollectionState(page, "Acorn Keeper", "available");
    expect(browserIssues).toEqual([]);
  });

  test("badge, grouped matches, and inline controls stay usable on narrow and forced-color displays", async ({
    page,
  }) => {
    await makeDeterministic(page);
    const browserIssues = watchBrowser(page);
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/");
    await login(page, "mock-alex", "Alex");
    await expect(page.locator("#matches-nav-badge")).toHaveText("4");
    await page.getByRole("link", { name: /Matches/ }).click();
    await expect(page.locator("section.match-group")).toHaveCount(1);
    await expect(page.locator("article.match-person")).toHaveCount(1);
    await page.getByRole("button", { name: "Load more", exact: true }).click();
    const pending = page.locator("section.match-pending");
    await expect(pending).toBeVisible();
    await expect(pending.locator("article.match-person")).toHaveCount(0);
    await pending
      .getByRole("button", {
        name: "Check swaps you can’t fulfil yet",
      })
      .click();
    await expect(pending.locator("article.match-person")).toHaveCount(1);
    await pending
      .getByRole("button", { name: "Load more", exact: true })
      .click();
    await expect(pending.locator("article.match-person")).toHaveCount(2);
    await expect(
      pending.locator(".match-ask--unfulfillable .match-ask__art img").first(),
    ).toBeVisible();
    await expect(page.locator(".primary-nav")).toBeVisible();
    await expect(page.getByRole("button", { name: "Refresh" })).toBeVisible();
    expect(await horizontalOverflow(page)).toEqual([]);

    await page.emulateMedia({ forcedColors: "active" });
    expect(
      await page.evaluate(() =>
        window.matchMedia("(forced-colors: active)").matches,
      ),
    ).toBeTruthy();
    await expect(pending).toBeVisible();
    expect(
      await pending
        .locator(".match-ask--unfulfillable .match-ask__art")
        .first()
        .evaluate((element) => ({
          filter: getComputedStyle(element).filter,
          opacity: getComputedStyle(element).opacity,
        })),
    ).toEqual({ filter: "none", opacity: "1" });

    await openCollection(page, catalogs.garden);
    const card = collectibleCard(page, "Acorn Keeper");
    await expect(card.getByRole("checkbox")).toHaveCount(2);
    const haveButton = card.getByRole("button", {
      name: "Have Acorn Keeper",
    });
    await expect(haveButton).toHaveAttribute("aria-pressed", "true");
    const targets = await card
      .locator(".state-toggle, .collectible-art-toggle")
      .evaluateAll((controls) =>
        controls.map((control) => {
          const bounds = control.getBoundingClientRect();
          return { height: bounds.height, width: bounds.width };
        }),
      );
    expect(
      targets.every(({ height, width }) => height >= 44 && width >= 44),
    ).toBeTruthy();

    await expect(
      card.getByRole("checkbox", { name: "Available to swap" }),
    ).toBeVisible();
    await expect(haveButton).toBeVisible();

    await page.setViewportSize({ width: 320, height: 700 });
    expect(await horizontalOverflow(page)).toEqual([]);
    expect(browserIssues).toEqual([]);
  });
});
