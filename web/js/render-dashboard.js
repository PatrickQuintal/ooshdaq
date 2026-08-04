import { state } from "./core.js";
import { elements } from "./dom.js";
import {
  artworkImage,
  errorMessage,
  loadingPanel,
  node,
  number,
  statePanel,
} from "./ui.js";
import { routeHash } from "./router.js";

function metric(label, designs, tone = "") {
  return node("article", { className: `metric-card ${tone}`.trim() }, [
    node("span", { text: label }),
    node("strong", { text: number(designs) }),
    node("small", {
      text: designs === 1 ? "collectible" : "collectibles",
    }),
  ]);
}

function renderMatchesBadge(count = 0) {
  elements.matchesNavBadge.hidden = count === 0;
  elements.matchesNavBadge.textContent = number(count);
  elements.matchesNavBadge.setAttribute(
    "aria-label",
    count === 0
      ? "No possible swaps"
      : `${number(count)} possible ${count === 1 ? "swap" : "swaps"}`,
  );
}

export function renderSummary() {
  const resource = state.overallSummary;
  elements.summaryContent.setAttribute("aria-busy", String(resource.loading));
  if (resource.loading && !resource.value) {
    renderMatchesBadge();
    elements.summaryContent.replaceChildren(
      loadingPanel("Loading your collection summary…"),
    );
    return;
  }
  if (resource.error && !resource.value) {
    renderMatchesBadge();
    elements.summaryContent.replaceChildren(
      statePanel({
        title: "Summary unavailable",
        message: errorMessage(
          resource.error,
          "Your overall totals could not be loaded.",
        ),
        action: "retry-summary",
        actionLabel: "Retry summary",
        error: true,
      }),
    );
    return;
  }
  const summary = resource.value;
  if (!summary) {
    renderMatchesBadge();
    elements.summaryContent.replaceChildren();
    return;
  }
  renderMatchesBadge(summary.possible_swap_count);
  const metrics = node(
    "div",
    {
      className: "metric-grid",
      attributes: { "aria-label": "Overall collection summary" },
    },
    [
      metric(
        "Available",
        summary.available_designs,
        "metric-card--available",
      ),
      metric("Wanted", summary.wanted_designs, "metric-card--wanted"),
    ],
  );
  const children = [metrics];
  if (resource.error) {
    children.unshift(
      node("p", {
        className: "notice",
        text: "These totals may be out of date. Retry the summary.",
        attributes: { role: "alert" },
      }),
    );
  }
  elements.summaryContent.replaceChildren(...children);
}

function collectionHref(catalogID) {
  return routeHash({
    name: "collection",
    catalogID,
    filters: {
      q: state.catalogs.filters.q,
      universe: "",
      state: "all",
    },
  });
}

function catalogStats(summary) {
  return node(
    "dl",
    {
      className: "catalog-stats",
      attributes: { "aria-label": "Your catalog summary" },
    },
    [
      node("div", { className: "catalog-stat catalog-stat--available" }, [
        node("dt", { text: "Available" }),
        node("dd", { text: number(summary.available_designs) }),
      ]),
      node("div", { className: "catalog-stat catalog-stat--wanted" }, [
        node("dt", { text: "Wanted" }),
        node("dd", { text: number(summary.wanted_designs) }),
      ]),
    ],
  );
}

function catalogCard(catalog) {
  const edition = catalog.edition ? ` · ${catalog.edition}` : "";
  const href = collectionHref(catalog.id);
  return node("article", { className: "catalog-card" }, [
    node("div", { className: "catalog-art" }, [
      artworkImage(catalog.artwork),
      catalog.publicationStatus === "active"
        ? null
        : node("span", {
            className: `status-badge status-badge--${catalog.publicationStatus}`,
            text: catalog.publicationStatus,
          }),
    ]),
    node("div", { className: "catalog-card__body" }, [
      node("span", { className: "eyebrow", text: catalog.brand.name }),
      node("h3", {}, [
        node("a", { text: catalog.name, attributes: { href } }),
      ]),
      node("p", {
        className: "muted",
        text: `${number(catalog.itemCount)} items${edition}`,
      }),
      catalogStats(catalog.memberSummary),
      node("a", {
        className: "button button--secondary button--full",
        text: "Open collection",
        attributes: { href },
      }),
    ]),
  ]);
}

export function renderCatalogs() {
  const resource = state.catalogs;
  elements.catalogContent.setAttribute("aria-busy", String(resource.loading));
  elements.catalogSearch.value = resource.filters.q;

  const first = resource.page.items[0];
  const navCatalogID = first?.id || state.collection.catalogID || "";
  elements.collectionNavLink.hidden = !navCatalogID;
  if (navCatalogID) {
    elements.collectionNavLink.href = collectionHref(navCatalogID);
  }

  elements.catalogResults.textContent = resource.page.items.length
    ? `${number(resource.page.items.length)} catalogs loaded${
        resource.error ? " · More results could not be loaded" : ""
      }`
    : "";
  elements.catalogMore.hidden = !resource.page.nextCursor;
  elements.catalogMore.disabled = resource.loading;
  elements.catalogMore.textContent = resource.loading
    ? "Loading…"
    : "Load more catalogs";

  if (resource.loading && !resource.page.items.length) {
    elements.catalogContent.replaceChildren(
      loadingPanel("Loading catalogs…", { cards: true }),
    );
    return;
  }
  if (resource.error && !resource.page.items.length) {
    elements.catalogContent.replaceChildren(
      statePanel({
        title: "Collections unavailable",
        message: errorMessage(
          resource.error,
          "Collections could not be loaded.",
        ),
        action: "retry-catalogs",
        actionLabel: "Retry catalogs",
        error: true,
      }),
    );
    return;
  }
  if (!resource.page.items.length) {
    elements.catalogContent.replaceChildren(
      statePanel({
        title: "No collections match",
        message: "Try another collectible name.",
        action: "clear-catalog-filters",
        actionLabel: "Clear search",
      }),
    );
    return;
  }

  elements.catalogContent.replaceChildren(
    node(
      "div",
      { className: "catalog-grid" },
      resource.page.items.map(catalogCard),
    ),
  );
}

export function renderDashboard() {
  renderSummary();
  renderCatalogs();
}
