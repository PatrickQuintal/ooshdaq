const STATES = new Set(["all", "owned", "available", "wanted"]);

function decoded(value) {
  try {
    return decodeURIComponent(value);
  } catch {
    return "";
  }
}

function queryPart(hash) {
  const index = hash.indexOf("?");
  return index < 0 ? "" : hash.slice(index + 1);
}

export function parseHash(hash) {
  const clean = hash.replace(/^#\/?/, "");
  const path = clean.split("?")[0];
  const parts = path.split("/").filter(Boolean).map(decoded);
  const query = new URLSearchParams(queryPart(clean));

  if (parts[0] === "collection" && parts.length === 2 && parts[1]) {
    const state = query.get("state") || "all";
    return {
      name: "collection",
      catalogID: parts[1],
      filters: {
        q: (query.get("q") || "").trim(),
        universe: query.get("universe") || "",
        state: STATES.has(state) ? state : "all",
      },
    };
  }
  if (parts[0] === "matches" && parts.length === 1) {
    return {
      name: "matches",
      catalogID: null,
    };
  }

  return {
    name: "dashboard",
    catalogID: null,
    filters: {
      q: (query.get("q") || "").trim(),
    },
  };
}

function add(query, name, value, defaultValue = "") {
  if (value && value !== defaultValue) {
    query.set(name, value);
  }
}

export function routeHash(route) {
  const query = new URLSearchParams();
  if (route.name === "collection") {
    add(query, "q", route.filters.q);
    add(query, "universe", route.filters.universe);
    add(query, "state", route.filters.state, "all");
    return withQuery(
      `#/collection/${encodeURIComponent(route.catalogID)}`,
      query,
    );
  }
  if (route.name === "matches") {
    return "#/matches";
  }
  add(query, "q", route.filters.q);
  return withQuery("#/dashboard", query);
}

function withQuery(path, query) {
  const encoded = query.toString();
  return encoded ? `${path}?${encoded}` : path;
}
