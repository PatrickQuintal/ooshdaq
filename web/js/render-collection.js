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

const COLLECTION_CONTROLS = [
  { name: "available", label: "Available to swap" },
  { name: "wanted", label: "Want" },
];

function statusID(itemID) {
  return `collection-status-${itemID}`;
}

function syncUniverseOptions(catalog) {
  const selected = state.collection.filters.universe;
  const options = [
    node("option", { text: "All universes", attributes: { value: "" } }),
    ...(catalog?.universeOptions || []).map((universe) =>
      node("option", { text: universe, attributes: { value: universe } }),
    ),
  ];
  elements.collectionUniverse.replaceChildren(...options);
  elements.collectionUniverse.value = selected;
}

export function renderCollectionHeader() {
  const catalog = state.collection.catalog;
  if (!catalog) {
    elements.collectionTitle.textContent = "Collection";
    elements.collectionSubtitle.textContent =
      "Choose a catalog from your dashboard.";
    elements.collectionSummary.textContent = "";
    return;
  }
  if (state.route.name === "collection") {
    document.title = `${catalog.name} · $OOSHDAQ`;
  }
  const edition = catalog.edition ? ` · ${catalog.edition}` : "";
  elements.collectionTitle.textContent = catalog.name;
  elements.collectionSubtitle.textContent =
    `${catalog.brand.name}${edition} · ${number(catalog.itemCount)} items`;
  const summary = catalog.memberSummary;
  elements.collectionSummary.textContent =
    `${number(summary.available_designs)} available · ` +
    `${number(summary.wanted_designs)} wanted`;
}

function cardStateClasses(entry) {
  return [
    "collectible-card",
    entry.owned ? "is-owned" : "is-unowned",
    entry.available ? "is-available" : "",
    entry.wanted ? "is-wanted" : "",
  ]
    .filter(Boolean)
    .join(" ");
}

function stateControls(entry) {
  const item = entry.catalogItem;
  const saveStatusID = statusID(item.id);
  const pending = state.collection.pendingItemIDs.has(item.id);
  const error = state.collection.saveErrors.get(item.id) || "";
  const fieldset = node(
    "fieldset",
    {
      className: "collection-state-controls",
      attributes: {
        disabled: pending ? "" : null,
        "aria-busy": String(pending),
      },
    },
    [
      node("legend", {
        className: "sr-only",
        text: `Choices for ${item.name}`,
      }),
      ...COLLECTION_CONTROLS.map(({ name, label }) =>
        node("label", { className: `state-toggle state-toggle--${name}` }, [
          node("input", {
            attributes: {
              type: "checkbox",
              checked: entry[name] ? "" : null,
              "aria-describedby": saveStatusID,
            },
            dataset: {
              collectionState: name,
              itemId: item.id,
            },
          }),
          node("span", { text: label }),
        ]),
      ),
    ],
  );
  const status = node("p", {
    className: `collection-save-status${error ? " is-error" : ""}`,
    text: error || (pending ? "Saving…" : ""),
    attributes: {
      id: saveStatusID,
      role: error ? "alert" : "status",
      "aria-live": "polite",
    },
  });
  return [fieldset, status];
}

function ownershipButton(entry) {
  const item = entry.catalogItem;
  const pending = state.collection.pendingItemIDs.has(item.id);
  return node(
    "button",
    {
      className: "collectible-art collectible-art-toggle",
      attributes: {
        type: "button",
        disabled: pending ? "" : null,
        "aria-busy": String(pending),
        "aria-describedby": statusID(item.id),
        "aria-label": `Have ${item.name}`,
        "aria-pressed": String(Boolean(entry.owned)),
      },
      dataset: {
        collectionState: "owned",
        itemId: item.id,
      },
    },
    [
      artworkImage(item.artwork),
      node("span", {
        className: "item-number",
        text: `No. ${item.number}`,
        attributes: { "aria-hidden": "true" },
      }),
      node("span", {
        className: "ownership-status",
        text: entry.owned ? "On my shelf" : "Not on my shelf",
      }),
    ],
  );
}

function collectibleCard(entry) {
  const item = entry.catalogItem;
  return node(
    "article",
    {
      className: cardStateClasses(entry),
      dataset: { itemId: item.id },
    },
    [
      ownershipButton(entry),
      node("div", { className: "collectible-card__body" }, [
        node("div", { className: "tag-row" }, [
          item.universe
            ? node("span", { className: "tag", text: item.universe })
            : null,
          item.variant
            ? node("span", { className: "tag tag--soft", text: item.variant })
            : null,
          item.publicationStatus === "retired"
            ? node("span", { className: "tag tag--retired", text: "Retired" })
            : null,
        ]),
        node("h2", { text: item.name }),
        ...stateControls(entry),
      ]),
    ],
  );
}

function visibleCard(itemID) {
  return (
    [...elements.collectionContent.querySelectorAll(".collectible-card")].find(
      (card) => card.dataset.itemId === itemID,
    ) || null
  );
}

function syncVisibleCard(entry, message = "") {
  const card = visibleCard(entry.catalogItem.id);
  if (!card) return;
  card.className = cardStateClasses(entry);
  const pending = state.collection.pendingItemIDs.has(entry.catalogItem.id);
  for (const control of card.querySelectorAll("[data-collection-state]")) {
    const pressed = Boolean(entry[control.dataset.collectionState]);
    if (control instanceof HTMLInputElement) {
      control.checked = pressed;
    } else if (control instanceof HTMLButtonElement) {
      control.disabled = pending;
      control.setAttribute("aria-busy", String(pending));
      control.setAttribute("aria-pressed", String(pressed));
      control.querySelector(".ownership-status").textContent = pressed
        ? "On my shelf"
        : "Not on my shelf";
    }
  }
  const error = state.collection.saveErrors.get(entry.catalogItem.id) || "";
  const controls = card.querySelector(".collection-state-controls");
  controls.disabled = pending;
  controls.setAttribute("aria-busy", String(pending));
  const status = card.querySelector(".collection-save-status");
  status.classList.toggle("is-error", Boolean(error));
  status.setAttribute("role", error ? "alert" : "status");
  status.textContent = error || (pending ? "Saving…" : message);
}

export function findVisibleEntry(itemID) {
  return (
    state.collection.page.items.find(
      (entry) => entry.catalogItem.id === itemID,
    ) || null
  );
}

export function replaceVisibleEntry(entry, message = "") {
  const index = state.collection.page.items.findIndex(
    (candidate) => candidate.catalogItem.id === entry.catalogItem.id,
  );
  if (index >= 0) {
    state.collection.page.items[index] = entry;
  }
  syncVisibleCard(entry, message);
}

export function refreshVisibleEntry(itemID, message = "") {
  const entry = findVisibleEntry(itemID);
  if (entry) syncVisibleCard(entry, message);
}

export function findVisibleControl(itemID, control) {
  return (
    [...elements.collectionContent.querySelectorAll("[data-collection-state]")].find(
      (input) =>
        input.dataset.itemId === itemID &&
        input.dataset.collectionState === control,
    ) || null
  );
}

export function applyCollectionSummaryDelta(previous, next) {
  if (state.collection.catalog?.id !== next.catalogItem.catalogID) return;
  const summary = state.collection.catalog.memberSummary;
  if (!summary) return;
  for (const field of ["owned", "available", "wanted"]) {
    summary[`${field}_quantity`] += next[field] - previous[field];
    summary[`${field}_designs`] +=
      Number(next[field] > 0) - Number(previous[field] > 0);
  }
  renderCollectionHeader();
}

export function renderCollection() {
  const resource = state.collection;
  elements.collectionContent.setAttribute("aria-busy", String(resource.loading));
  renderCollectionHeader();
  syncUniverseOptions(resource.catalog);
  elements.collectionSearch.value = resource.filters.q;
  elements.collectionState.value = resource.filters.state;
  elements.collectionResults.textContent = resource.page.items.length
    ? `${number(resource.page.items.length)} collectibles loaded${
        resource.error ? " · More results could not be loaded" : ""
      }`
    : "";
  elements.collectionMore.hidden = !resource.page.nextCursor;
  elements.collectionMore.disabled = resource.loading;
  elements.collectionMore.textContent = resource.loading
    ? "Loading…"
    : "Load more collectibles";

  if (resource.loading && !resource.page.items.length) {
    elements.collectionContent.replaceChildren(
      loadingPanel("Loading this catalog…", { cards: true }),
    );
    return;
  }
  if (resource.error && !resource.page.items.length) {
    elements.collectionContent.replaceChildren(
      statePanel({
        title: "Collection unavailable",
        message: errorMessage(
          resource.error,
          "This catalog and your collection states could not be loaded.",
        ),
        action: "retry-collection",
        actionLabel: "Retry collection",
        error: true,
      }),
    );
    return;
  }
  if (!resource.catalog && !resource.loading) {
    elements.collectionContent.replaceChildren(
      statePanel({
        title: "Choose a collection",
        message: "Return to your collections and choose one to open.",
        action: "go-dashboard",
        actionLabel: "Browse catalogs",
      }),
    );
    return;
  }
  if (!resource.page.items.length) {
    elements.collectionContent.replaceChildren(
      statePanel({
        title: "No collectibles match",
        message: "Try another search, universe, or filter.",
        action: "clear-collection-filters",
        actionLabel: "Clear filters",
      }),
    );
    return;
  }
  elements.collectionContent.replaceChildren(
    node(
      "div",
      { className: "card-grid" },
      resource.page.items.map(collectibleCard),
    ),
  );
}
