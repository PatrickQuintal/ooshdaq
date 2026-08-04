export const API = Object.freeze({
  session: "/api/v1/session",
  activity: "/api/v1/activity",
  catalogs: "/api/v1/catalogs",
  summary: "/api/v1/collection/summary",
  collection: (catalogID) =>
    `/api/v1/catalogs/${encodeURIComponent(catalogID)}/collection`,
  collectionEntry: (itemID) =>
    `/api/v1/collection/${encodeURIComponent(itemID)}`,
  matches: "/api/v1/matches",
  mockLogin: "/auth/mock-login",
  logout: "/auth/logout",
});

export const PAGE_LIMITS = Object.freeze({
  catalogs: 20,
  collection: 40,
  matches: 20,
  unfulfillableMatches: 20,
});

export function emptyPage(limit) {
  return {
    items: [],
    limit,
    requestCursor: null,
    nextCursor: null,
    backStack: [],
    number: 1,
  };
}

function loadable(extra = {}) {
  return {
    requestGeneration: 0,
    loading: false,
    error: null,
    ...extra,
  };
}

export const state = {
  session: {
    status: "loading",
    user: null,
    authMode: "",
    csrfToken: "",
    mockUsers: [],
  },
  route: {
    name: "dashboard",
    catalogID: null,
  },
  catalogs: loadable({
    filters: {
      q: "",
    },
    page: emptyPage(PAGE_LIMITS.catalogs),
  }),
  overallSummary: loadable({
    value: null,
  }),
  collection: loadable({
    catalogID: null,
    catalog: null,
    pendingItemIDs: new Set(),
    saveErrors: new Map(),
    filters: {
      q: "",
      universe: "",
      state: "all",
    },
    page: emptyPage(PAGE_LIMITS.collection),
  }),
  matches: loadable({
    page: emptyPage(PAGE_LIMITS.matches),
  }),
  unfulfillableMatches: loadable({
    expanded: false,
    page: emptyPage(PAGE_LIMITS.unfulfillableMatches),
  }),
};

export const requestControllers = {
  catalogs: null,
  overallSummary: null,
  collection: null,
  matches: null,
  unfulfillableMatches: null,
};

export function resetPage(resource, limit) {
  resource.page = emptyPage(limit);
  resource.error = null;
}

export function abortResource(name) {
  requestControllers[name]?.abort();
  requestControllers[name] = null;
  const resource = state[name];
  if (resource) {
    resource.requestGeneration += 1;
    resource.loading = false;
  }
}

export function abortAll() {
  for (const name of Object.keys(requestControllers)) {
    abortResource(name);
  }
}

export function resetMemberState() {
  abortAll();
  state.catalogs.filters = {
    q: "",
  };
  resetPage(state.catalogs, PAGE_LIMITS.catalogs);
  state.overallSummary.value = null;
  state.overallSummary.error = null;
  state.collection.catalogID = null;
  state.collection.catalog = null;
  state.collection.pendingItemIDs.clear();
  state.collection.saveErrors.clear();
  state.collection.filters = { q: "", universe: "", state: "all" };
  resetPage(state.collection, PAGE_LIMITS.collection);
  resetPage(state.matches, PAGE_LIMITS.matches);
  state.unfulfillableMatches.expanded = false;
  resetPage(
    state.unfulfillableMatches,
    PAGE_LIMITS.unfulfillableMatches,
  );
}
