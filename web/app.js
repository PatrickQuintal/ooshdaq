import { state } from "./js/core.js";
import { elements } from "./js/dom.js";
import {
  loadCatalogs,
  loadCollection,
  loadMatches,
  loadOverallSummary,
  loadSession,
  loadUnfulfillableMatches,
  refreshMatches,
  syncRoute,
  toggleUnfulfillableMatches,
} from "./js/controller.js";
import { logout, mockLogin, saveCollectionState } from "./js/actions.js";
import { routeHash } from "./js/router.js";

let focusNextHashRoute = true;

function navigate(route, { focus = false } = {}) {
  const target = routeHash(route);
  if (window.location.hash === target) {
    void syncRoute({ force: true, focus });
  } else {
    focusNextHashRoute = focus;
    window.location.hash = target;
  }
}

function dashboardRoute() {
  return {
    name: "dashboard",
    catalogID: null,
    filters: {
      q: elements.catalogSearch.value.trim(),
    },
  };
}

function collectionRoute(filters = null) {
  return {
    name: "collection",
    catalogID: state.collection.catalogID,
    filters: filters || {
      q: elements.collectionSearch.value.trim(),
      universe: elements.collectionUniverse.value,
      state: elements.collectionState.value,
    },
  };
}

function handleAction(button) {
  const action = button.dataset.action;
  if (action === "mock-login") {
    void mockLogin(button.dataset.userId);
  } else if (action === "retry-session") {
    void loadSession();
  } else if (action === "retry-summary") {
    void loadOverallSummary();
  } else if (action === "retry-catalogs") {
    void loadCatalogs();
  } else if (action === "retry-collection") {
    void loadCollection();
  } else if (action === "retry-matches") {
    void loadMatches();
  } else if (action === "retry-unfulfillable-matches") {
    void loadUnfulfillableMatches();
  } else if (action === "toggle-unfulfillable-matches") {
    void toggleUnfulfillableMatches();
  } else if (action === "load-more-catalogs") {
    void loadCatalogs({ append: true });
  } else if (action === "load-more-collection") {
    void loadCollection({ append: true });
  } else if (action === "load-more-matches") {
    void loadMatches({ append: true });
  } else if (action === "load-more-unfulfillable-matches") {
    void loadUnfulfillableMatches({ append: true });
  } else if (action === "clear-catalog-filters") {
    navigate(
      {
        name: "dashboard",
        catalogID: null,
        filters: { q: "" },
      },
      { focus: true },
    );
  } else if (action === "clear-collection-filters") {
    navigate(collectionRoute({ q: "", universe: "", state: "all" }), {
      focus: true,
    });
  } else if (action === "go-dashboard") {
    navigate(
      {
        name: "dashboard",
        catalogID: null,
        filters: state.catalogs.filters,
      },
      { focus: true },
    );
  }
}

function bindForms() {
  elements.catalogFilters.addEventListener("submit", (event) => {
    event.preventDefault();
    navigate(dashboardRoute());
  });

  elements.collectionFilters.addEventListener("submit", (event) => {
    event.preventDefault();
    navigate(collectionRoute());
  });
  elements.collectionUniverse.addEventListener("change", () =>
    elements.collectionFilters.requestSubmit(),
  );
  elements.collectionState.addEventListener("change", () =>
    elements.collectionFilters.requestSubmit(),
  );
}

window.addEventListener("hashchange", () => {
  const focus = focusNextHashRoute;
  focusNextHashRoute = true;
  void syncRoute({ focus });
});
elements.skipLink.addEventListener("click", (event) => {
  event.preventDefault();
  elements.mainContent.focus({ preventScroll: true });
  elements.mainContent.scrollIntoView();
});
elements.logoutButton.addEventListener("click", () => void logout());
elements.refreshMatches.addEventListener("click", () => void refreshMatches());
elements.collectionContent.addEventListener("change", (event) => {
  const input = event.target.closest("[data-collection-state]");
  if (input instanceof HTMLInputElement) {
    void saveCollectionState(input);
  }
});
elements.collectionContent.addEventListener("click", (event) => {
  const button = event.target.closest(
    "button[data-collection-state=\"owned\"]",
  );
  if (button instanceof HTMLButtonElement) {
    void saveCollectionState(button);
  }
});
document.addEventListener("click", (event) => {
  const button = event.target.closest("[data-action]");
  if (button) handleAction(button);
});

bindForms();
void loadSession();
