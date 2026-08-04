import { state } from "./core.js";
import { elements } from "./dom.js";
import { errorMessage, initials, node } from "./ui.js";

export function showStartup(message = "Opening $OOSHDAQ…") {
  elements.header.hidden = true;
  elements.loginView.hidden = true;
  elements.authenticatedViews.hidden = true;
  elements.globalStatus.replaceChildren(
    node("span", { className: "spinner", attributes: { "aria-hidden": "true" } }),
    node("span", { text: message }),
  );
  elements.globalStatus.className = "global-status";
  elements.globalStatus.hidden = false;
  elements.app.setAttribute("aria-busy", "true");
}

export function showStartupError(error) {
  elements.globalStatus.replaceChildren(
    node("div", { className: "state-panel state-panel--error", attributes: { role: "alert" } }, [
      node("h1", { text: "$OOSHDAQ could not open" }),
      node("p", {
        text: errorMessage(error, "Something went wrong while starting $OOSHDAQ."),
      }),
      node("button", {
        className: "button button--secondary",
        text: "Try again",
        attributes: { type: "button" },
        dataset: { action: "retry-session" },
      }),
    ]),
  );
  elements.globalStatus.className = "global-status global-status--error";
  elements.globalStatus.hidden = false;
  elements.app.setAttribute("aria-busy", "false");
}

export function hideStartup() {
  elements.globalStatus.hidden = true;
  elements.app.setAttribute("aria-busy", "false");
}

export function renderLogin() {
  elements.header.hidden = true;
  elements.authenticatedViews.hidden = true;
  elements.loginView.hidden = false;
  elements.loginError.hidden = true;
  elements.mockUserList.replaceChildren();

  for (const user of state.session.mockUsers) {
    elements.mockUserList.append(
      node("button", {
        className: "profile-button",
        attributes: { type: "button" },
        dataset: { action: "mock-login", userId: user.id },
      }, [
        node("span", {
          className: "avatar avatar--large",
          text: initials(user.displayName),
          attributes: { "aria-hidden": "true" },
        }),
        node("span", { className: "profile-copy" }, [
          node("strong", { text: user.displayName }),
          node("small", { text: "Open this collection" }),
        ]),
        node("span", { text: "→", attributes: { "aria-hidden": "true" } }),
      ]),
    );
  }

  const mockMode = state.session.authMode === "mock";
  elements.identityNote.hidden = mockMode && state.session.mockUsers.length > 0;
  elements.identityNote.textContent = mockMode
    ? "No local profiles are configured."
    : "Sign in through your organisation’s access portal, then return here.";
}

export function renderAuthenticatedShell() {
  const user = state.session.user;
  elements.loginView.hidden = true;
  elements.header.hidden = false;
  elements.authenticatedViews.hidden = false;
  elements.headerAvatar.textContent = initials(user.displayName);
  elements.headerUserName.textContent = user.displayName;
  elements.headerAuthMode.textContent =
    state.session.authMode === "mock" ? "Local profile" : "Signed in";
  elements.logoutButton.hidden = state.session.authMode !== "mock";
  elements.dashboardName.textContent = user.displayName.split(/\s+/)[0];
}

export function applyVisibleRoute({ focus = true } = {}) {
  for (const view of document.querySelectorAll("[data-view]")) {
    view.hidden = view.dataset.view !== state.route.name;
  }
  for (const link of document.querySelectorAll("[data-route-link]")) {
    const active = link.dataset.routeLink === state.route.name;
    link.classList.toggle("is-active", active);
    if (active) {
      link.setAttribute("aria-current", "page");
    } else {
      link.removeAttribute("aria-current");
    }
  }
  document.title = {
    dashboard: "$OOSHDAQ · Your catalogs",
    collection: "Collection · $OOSHDAQ",
    matches: "Possible swaps · $OOSHDAQ",
  }[state.route.name];
  if (focus) {
    const heading = document.querySelector(
      `[data-view="${state.route.name}"] h1`,
    );
    heading?.focus({ preventScroll: true });
    window.scrollTo({ top: 0, behavior: "smooth" });
  }
}

export function announce(message) {
  elements.announcer.textContent = "";
  window.setTimeout(() => {
    elements.announcer.textContent = message;
  }, 20);
}

export function toast(message, isError = false) {
  const item = node("div", {
    className: `toast${isError ? " toast--error" : ""}`,
    text: message,
    attributes: { role: isError ? "alert" : "status" },
  });
  elements.toastRegion.append(item);
  window.setTimeout(() => item.remove(), 4500);
}

export function showLoginError(error) {
  elements.loginError.textContent = errorMessage(
    error,
    "That profile could not be opened.",
  );
  elements.loginError.hidden = false;
}
