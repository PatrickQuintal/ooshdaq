import {
  API,
  PAGE_LIMITS,
  abortResource,
  resetMemberState,
  resetPage,
  state,
} from "./core.js";
import { ApiError, isAbort, pageURL, request } from "./api.js";
import {
  parseCatalogPage,
  parseCollectionPage,
  parseMatchPage,
  parseOverallSummary,
  parseSession,
} from "./models.js";
import { parseHash, routeHash } from "./router.js";
import {
  acceptPage,
  beginRequest,
  finishRequest,
  isCurrent,
  queryKey,
} from "./page-state.js";
import { renderCollection } from "./render-collection.js";
import { renderDashboard, renderCatalogs, renderSummary } from "./render-dashboard.js";
import { renderMatches } from "./render-matches.js";
import {
  announce,
  applyVisibleRoute,
  hideStartup,
  renderAuthenticatedShell,
  renderLogin,
  showStartup,
  showStartupError,
  toast,
} from "./render-shell.js";

const loadedKeys = {
  catalogs: "",
  collection: "",
  matches: "",
  unfulfillableMatches: "",
};

function firstCatalogQuery() {
  const filters = state.catalogs.filters;
  return {
    q: filters.q,
    publication_status: "active",
    limit: state.catalogs.page.limit,
  };
}

function firstCollectionQuery() {
  const filters = state.collection.filters;
  return {
    q: filters.q,
    universe: filters.universe,
    state: filters.state,
    limit: state.collection.page.limit,
  };
}

function firstMatchQuery(resource, readiness) {
  return {
    readiness,
    limit: resource.page.limit,
  };
}

function sessionExpired(error) {
  if (!(error instanceof ApiError) || error.status !== 401) {
    return false;
  }
  resetMemberState();
  clearLoadedKeys();
  state.session.status = "loading";
  showStartup("Your session changed. Reopening $OOSHDAQ…");
  queueMicrotask(() => void loadSession());
  return true;
}

async function recordActivity() {
  try {
    await request(API.activity, {
      method: "POST",
      headers: state.session.csrfToken
        ? { "X-CSRF-Token": state.session.csrfToken }
        : {},
    });
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      sessionExpired(error);
    }
  }
}

async function recoverCursor(name, cursor, recovered, reload) {
  if (!cursor || recovered) {
    return false;
  }
  announce("Those results expired, so $OOSHDAQ returned to page one.");
  toast("Results refreshed from page one.");
  resetPage(state[name], PAGE_LIMITS[name]);
  await reload();
  return true;
}

export async function loadCatalogs({ append = false, recovered = false } = {}) {
  const resource = state.catalogs;
  const cursor = append ? resource.page.nextCursor : null;
  if (append && !cursor) return;
  if (!append) {
    resetPage(resource, PAGE_LIMITS.catalogs);
    loadedKeys.catalogs = "";
  }
  const { controller, generation } = beginRequest("catalogs");
  renderCatalogs();
  try {
    const payload = await request(
      pageURL(API.catalogs, firstCatalogQuery(), cursor),
      { signal: controller.signal },
    );
    if (!isCurrent("catalogs", generation, controller)) return;
    const parsed = parseCatalogPage(payload);
    acceptPage(resource, parsed.catalogs, parsed.page, cursor, (item) => item.id);
    loadedKeys.catalogs = queryKey(resource.filters);
  } catch (error) {
    if (isAbort(error) || !isCurrent("catalogs", generation, controller)) return;
    if (sessionExpired(error)) return;
    if (
      error instanceof ApiError &&
      error.code === "invalid_cursor" &&
      (await recoverCursor("catalogs", cursor, recovered, () =>
        loadCatalogs({ recovered: true }),
      ))
    ) return;
    resource.error = error;
  } finally {
    finishRequest("catalogs", generation, controller, renderCatalogs);
  }
}

export async function loadOverallSummary() {
  const resource = state.overallSummary;
  const { controller, generation } = beginRequest("overallSummary");
  renderSummary();
  try {
    const payload = await request(API.summary, { signal: controller.signal });
    if (!isCurrent("overallSummary", generation, controller)) return;
    resource.value = parseOverallSummary(payload);
  } catch (error) {
    if (isAbort(error) || !isCurrent("overallSummary", generation, controller)) return;
    if (sessionExpired(error)) return;
    resource.error = error;
  } finally {
    finishRequest("overallSummary", generation, controller, renderSummary);
  }
}

export async function loadCollection({ append = false, recovered = false } = {}) {
  const resource = state.collection;
  if (!resource.catalogID) return;
  const cursor = append ? resource.page.nextCursor : null;
  if (append && !cursor) return;
  if (!append) {
    resetPage(resource, PAGE_LIMITS.collection);
    loadedKeys.collection = "";
  }
  const { controller, generation } = beginRequest("collection");
  renderCollection();
  try {
    const payload = await request(
      pageURL(API.collection(resource.catalogID), firstCollectionQuery(), cursor),
      { signal: controller.signal },
    );
    if (!isCurrent("collection", generation, controller)) return;
    const parsed = parseCollectionPage(payload);
    resource.catalog = parsed.catalog;
    acceptPage(
      resource,
      parsed.entries,
      parsed.page,
      cursor,
      (entry) => entry.catalogItem.id,
    );
    loadedKeys.collection = queryKey({
      catalogID: resource.catalogID,
      ...resource.filters,
    });
  } catch (error) {
    if (isAbort(error) || !isCurrent("collection", generation, controller)) return;
    if (sessionExpired(error)) return;
    if (
      error instanceof ApiError &&
      error.code === "invalid_cursor" &&
      (await recoverCursor("collection", cursor, recovered, () =>
        loadCollection({ recovered: true }),
      ))
    ) return;
    resource.error = error;
  } finally {
    finishRequest("collection", generation, controller, renderCollection);
  }
}

async function loadMatchFeed(
  name,
  readiness,
  { append = false, recovered = false } = {},
) {
  const resource = state[name];
  const cursor = append ? resource.page.nextCursor : null;
  if (append && !cursor) return;
  if (!append) {
    resetPage(resource, PAGE_LIMITS[name]);
    loadedKeys[name] = "";
  }
  const { controller, generation } = beginRequest(name);
  renderMatches();
  try {
    const payload = await request(
      pageURL(API.matches, firstMatchQuery(resource, readiness), cursor),
      { signal: controller.signal },
    );
    if (!isCurrent(name, generation, controller)) return;
    const parsed = parseMatchPage(payload, readiness);
    acceptPage(resource, parsed.matches, parsed.page, cursor, (item) => item.id);
    loadedKeys[name] = readiness;
  } catch (error) {
    if (isAbort(error) || !isCurrent(name, generation, controller)) return;
    if (sessionExpired(error)) return;
    if (
      error instanceof ApiError &&
      error.code === "invalid_cursor" &&
      (await recoverCursor(name, cursor, recovered, () =>
        loadMatchFeed(name, readiness, { recovered: true }),
      ))
    ) return;
    resource.error = error;
  } finally {
    finishRequest(name, generation, controller, renderMatches);
  }
}

export async function loadMatches(options = {}) {
  await loadMatchFeed("matches", "actionable", options);
}

export async function loadUnfulfillableMatches(options = {}) {
  await loadMatchFeed(
    "unfulfillableMatches",
    "unfulfillable",
    options,
  );
}

export async function toggleUnfulfillableMatches() {
  const resource = state.unfulfillableMatches;
  if (resource.expanded) {
    abortResource("unfulfillableMatches");
    resource.expanded = false;
    renderMatches();
    return;
  }
  resource.expanded = true;
  renderMatches();
  if (loadedKeys.unfulfillableMatches !== "unfulfillable") {
    await loadUnfulfillableMatches();
  }
}

export async function refreshMatches() {
  abortResource("unfulfillableMatches");
  state.unfulfillableMatches.expanded = false;
  resetPage(
    state.unfulfillableMatches,
    PAGE_LIMITS.unfulfillableMatches,
  );
  loadedKeys.unfulfillableMatches = "";
  await loadMatches();
}

export async function syncRoute({ focus = true, force = false } = {}) {
  if (state.session.status !== "authenticated") {
    return;
  }
  const route = parseHash(window.location.hash);
  const canonicalHash = routeHash(route);
  if (window.location.hash && window.location.hash !== canonicalHash) {
    window.history.replaceState(
      null,
      "",
      `${window.location.pathname}${window.location.search}${canonicalHash}`,
    );
  }
  state.route = { name: route.name, catalogID: route.catalogID };
  if (route.name === "dashboard") {
    abortResource("collection");
    abortResource("matches");
    abortResource("unfulfillableMatches");
    state.unfulfillableMatches.expanded = false;
    state.catalogs.filters = route.filters;
    renderDashboard();
    if (force || loadedKeys.catalogs !== queryKey(route.filters)) {
      void loadCatalogs();
    }
  } else if (route.name === "collection") {
    abortResource("matches");
    abortResource("unfulfillableMatches");
    state.unfulfillableMatches.expanded = false;
    if (state.collection.catalogID !== route.catalogID) {
      abortResource("collection");
      state.collection.catalog = null;
      resetPage(state.collection, PAGE_LIMITS.collection);
      loadedKeys.collection = "";
    }
    state.collection.catalogID = route.catalogID;
    state.collection.filters = route.filters;
    renderCatalogs();
    renderCollection();
    const key = queryKey({ catalogID: route.catalogID, ...route.filters });
    if (force || loadedKeys.collection !== key) void loadCollection();
  } else {
    abortResource("collection");
    renderMatches();
    if (force || loadedKeys.matches !== "actionable") void loadMatches();
  }
  applyVisibleRoute({ focus });
}

let sessionGeneration = 0;

export async function loadSession() {
  const generation = ++sessionGeneration;
  showStartup();
  try {
    const session = parseSession(await request(API.session));
    if (generation !== sessionGeneration) return;
    state.session = session;
    if (session.status !== "authenticated") {
      resetMemberState();
      renderDashboard();
      renderCollection();
      renderMatches();
      renderLogin();
      hideStartup();
      return;
    }
    renderAuthenticatedShell();
    void recordActivity();
    await Promise.all([
      loadCatalogs(),
      loadOverallSummary(),
    ]);
    if (generation !== sessionGeneration) return;
    hideStartup();
    await syncRoute({ focus: false });
  } catch (error) {
    if (generation === sessionGeneration) {
      showStartupError(error);
    }
  }
}

export function invalidateAfterWrite() {
  abortResource("matches");
  abortResource("unfulfillableMatches");
  resetPage(state.matches, PAGE_LIMITS.matches);
  state.unfulfillableMatches.expanded = false;
  resetPage(
    state.unfulfillableMatches,
    PAGE_LIMITS.unfulfillableMatches,
  );
  loadedKeys.matches = "";
  loadedKeys.unfulfillableMatches = "";
  loadedKeys.catalogs = "";
  loadedKeys.collection = "";
  renderMatches();
}

export function clearLoadedKeys() {
  loadedKeys.catalogs = "";
  loadedKeys.collection = "";
  loadedKeys.matches = "";
  loadedKeys.unfulfillableMatches = "";
}
