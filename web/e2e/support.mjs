import { expect } from "@playwright/test";

export const catalogs = {
  disney: {
    id: "disney-ooshies-2026",
    name: "Disney Ooshies (2026)",
  },
  garden: {
    id: "garden-sprites-2026",
    name: "Garden Sprites",
  },
  harbor: {
    id: "harbor-signal-pins-2026",
    name: "Harbor Signal Pins",
  },
  pocket: {
    id: "pocket-constellations-2026",
    name: "Pocket Constellations",
  },
};

export async function makeDeterministic(page) {
  await page.route("https://cdn0.woolworths.media/**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "image/png",
      body: Buffer.from(
        "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
        "base64",
      ),
    });
  });
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (request.method() === "GET" && !url.searchParams.has("cursor")) {
      if (url.pathname === "/api/v1/catalogs") {
        if (url.searchParams.get("publication_status") !== "all") {
          url.searchParams.set("limit", "2");
        }
      } else if (
        /^\/api\/v1\/catalogs\/[^/]+\/collection$/.test(url.pathname)
      ) {
        url.searchParams.set("limit", "2");
      } else if (url.pathname === "/api/v1/matches") {
        url.searchParams.set("limit", "1");
      }
    }
    await route.continue({ url: url.href });
  });
}

export function watchBrowser(page) {
  const issues = [];
  page.on("pageerror", (error) => issues.push(`pageerror: ${error.message}`));
  page.on("console", (message) => {
    if (message.type() === "error") {
      issues.push(`console: ${message.text()}`);
    }
  });
  return issues;
}

export async function login(page, userID, firstName) {
  await expect(page.getByRole("heading", { name: "$OOSHDAQ" })).toBeVisible();
  await page
    .locator(`[data-action="mock-login"][data-user-id="${userID}"]`)
    .click();
  await expect(
    page.getByRole("heading", { name: `Hello, ${firstName}.` }),
  ).toBeVisible();
}

export async function resetCollectionEntries(page, members) {
  for (const member of members) {
    await page.goto("/");
    await login(page, member.userID, member.firstName);
    await page.evaluate(async (entries) => {
      const sessionResponse = await fetch("/api/v1/session", {
        credentials: "same-origin",
        cache: "no-store",
      });
      if (!sessionResponse.ok) {
        throw new Error(`Reset session failed (${sessionResponse.status}).`);
      }
      const session = await sessionResponse.json();
      for (const itemID of entries) {
        const saveResponse = await fetch(
          `/api/v1/collection/${encodeURIComponent(itemID)}`,
          {
            method: "PUT",
            headers: {
              "Content-Type": "application/json",
              "X-CSRF-Token": session.csrf_token,
            },
            credentials: "same-origin",
            body: JSON.stringify({ owned: 0, available: 0, wanted: 0 }),
          },
        );
        if (!saveResponse.ok) {
          throw new Error(`Reset collection save failed (${saveResponse.status}).`);
        }
      }
    }, member.entries);
    await page.getByRole("button", { name: "Sign out" }).click();
  }
}

export async function openCollection(page, catalog) {
  await page.goto(`/#/collection/${catalog.id}`);
  await expect(
    page.getByRole("heading", { name: catalog.name, exact: true }),
  ).toBeVisible();
}

export function collectibleCard(page, itemName) {
  return page.locator("article.collectible-card", {
    has: page.getByRole("heading", { name: itemName, exact: true }),
  });
}

const collectionStates = {
  neutral: {
    quantities: { owned: 0, available: 0, wanted: 0 },
    checked: { have: false, available: false, want: false },
  },
  owned: {
    quantities: { owned: 1, available: 0, wanted: 0 },
    checked: { have: true, available: false, want: false },
  },
  available: {
    quantities: { owned: 1, available: 1, wanted: 0 },
    checked: { have: true, available: true, want: false },
  },
  wanted: {
    quantities: { owned: 0, available: 0, wanted: 1 },
    checked: { have: false, available: false, want: true },
  },
  ownedWanted: {
    quantities: { owned: 1, available: 0, wanted: 1 },
    checked: { have: true, available: false, want: true },
  },
};

const stateTransitions = {
  neutral: { have: "owned", available: "available", want: "wanted" },
  owned: { have: "neutral", available: "available", want: "ownedWanted" },
  available: { have: "neutral", available: "owned", want: "ownedWanted" },
  wanted: { have: "ownedWanted", available: "available", want: "neutral" },
  ownedWanted: { have: "wanted", available: "available", want: "owned" },
};

function transitionPlan(from, to) {
  const queue = [{ state: from, controls: [] }];
  const visited = new Set([from]);
  while (queue.length) {
    const current = queue.shift();
    if (current.state === to) return current.controls;
    for (const [control, next] of Object.entries(
      stateTransitions[current.state],
    )) {
      if (visited.has(next)) continue;
      visited.add(next);
      queue.push({ state: next, controls: [...current.controls, control] });
    }
  }
  throw new Error(`No collection transition from ${from} to ${to}.`);
}

async function currentState(controls) {
  const have = (await controls.have.getAttribute("aria-pressed")) === "true";
  const available = await controls.available.isChecked();
  const want = await controls.want.isChecked();
  if (available) return "available";
  if (have && want) return "ownedWanted";
  if (have) return "owned";
  if (want) return "wanted";
  return "neutral";
}

function collectionControls(card, itemName) {
  return {
    have: card.getByRole("button", {
      name: `Have ${itemName}`,
      exact: true,
    }),
    available: card.getByRole("checkbox", {
      name: "Available to swap",
      exact: true,
    }),
    want: card.getByRole("checkbox", { name: "Want", exact: true }),
  };
}

export async function setCollectionState(
  page,
  itemName,
  targetState,
  { keyboard = false } = {},
) {
  const card = collectibleCard(page, itemName);
  await expect(card).toHaveCount(1);
  const target = collectionStates[targetState];
  if (!target) throw new Error(`Unknown collection state: ${targetState}`);
  const controls = collectionControls(card, itemName);
  let state = await currentState(controls);
  const plan = transitionPlan(state, targetState);
  if (!plan.length) {
    await assertCollectionState(page, itemName, targetState);
    return null;
  }

  let response = null;
  for (const controlName of plan) {
    const control = controls[controlName];
    const nextState = stateTransitions[state][controlName];
    const responsePromise = page.waitForResponse(
      (candidate) =>
        candidate.request().method() === "PUT" &&
        candidate.url().includes("/api/v1/collection/"),
    );
    if (keyboard) {
      await control.focus();
      await page.keyboard.press("Space");
    } else {
      await control.click();
    }
    response = await responsePromise;
    expect(response.status()).toBe(200);
    expect(response.request().headers()["x-csrf-token"]).toBeTruthy();
    expect(JSON.parse(response.request().postData())).toEqual(
      collectionStates[nextState].quantities,
    );
    await assertCollectionState(page, itemName, nextState);
    await expect(card.getByText("Saved.", { exact: true })).toBeVisible();
    if (keyboard) await expect(control).toBeFocused();
    state = nextState;
  }

  await assertCollectionState(page, itemName, targetState);
  return response;
}

export async function assertCollectionState(page, itemName, expectedState) {
  const expected = collectionStates[expectedState];
  if (!expected) throw new Error(`Unknown collection state: ${expectedState}`);
  const controls = collectionControls(collectibleCard(page, itemName), itemName);
  await expect(controls.have).toHaveAttribute(
    "aria-pressed",
    String(expected.checked.have),
  );
  await expect(controls.available).toBeChecked({
    checked: expected.checked.available,
  });
  await expect(controls.want).toBeChecked({ checked: expected.checked.want });
}
