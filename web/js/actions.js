import { API, resetMemberState, state } from "./core.js";
import { ApiError, request } from "./api.js";
import { elements } from "./dom.js";
import {
  nextCollectionQuantities,
  parseSavedEntry,
} from "./models.js";
import {
  clearLoadedKeys,
  invalidateAfterWrite,
  loadCatalogs,
  loadCollection,
  loadOverallSummary,
  loadSession,
} from "./controller.js";
import {
  applyCollectionSummaryDelta,
  findVisibleControl,
  findVisibleEntry,
  refreshVisibleEntry,
  replaceVisibleEntry,
} from "./render-collection.js";
import { showLoginError, toast } from "./render-shell.js";
import { errorMessage } from "./ui.js";

async function finishLocalLogout() {
  resetMemberState();
  clearLoadedKeys();
  state.session = {
    status: "loading",
    user: null,
    authMode: "",
    csrfToken: "",
    mockUsers: [],
  };
  window.history.replaceState(
    null,
    "",
    `${window.location.pathname}#/dashboard`,
  );
  await loadSession();
}

function csrfHeaders() {
  return state.session.csrfToken
    ? { "X-CSRF-Token": state.session.csrfToken }
    : {};
}

async function postLogout() {
  await request(API.logout, {
    method: "POST",
    headers: csrfHeaders(),
  });
}

export async function saveCollectionState(controlElement) {
  const itemID = controlElement.dataset.itemId || "";
  const control = controlElement.dataset.collectionState || "";
  const previous = findVisibleEntry(itemID);
  if (!previous) return;
  if (state.collection.pendingItemIDs.has(itemID)) {
    refreshVisibleEntry(itemID);
    return;
  }

  const checked =
    controlElement instanceof HTMLInputElement
      ? controlElement.checked
      : control === "owned"
        ? !Boolean(previous.owned)
        : false;
  const restoreFocus = controlElement.matches(":focus");
  const quantities = nextCollectionQuantities(
    previous,
    control,
    checked,
  );
  const optimistic = { ...previous, ...quantities };
  state.collection.pendingItemIDs.add(itemID);
  state.collection.saveErrors.delete(itemID);
  replaceVisibleEntry(optimistic);
  try {
    const payload = await request(API.collectionEntry(itemID), {
      method: "PUT",
      headers: csrfHeaders(),
      body: quantities,
    });
    const saved = parseSavedEntry(payload, itemID);
    replaceVisibleEntry(saved);
    applyCollectionSummaryDelta(previous, saved);
    state.collection.pendingItemIDs.delete(itemID);
    refreshVisibleEntry(itemID, "Saved.");

    invalidateAfterWrite();
    const refreshes = [
      loadOverallSummary(),
      loadCatalogs(),
    ];
    if (
      state.route.name === "collection" &&
      state.collection.filters.state !== "all"
    ) {
      refreshes.push(loadCollection());
    }
    await Promise.all(refreshes);
  } catch (error) {
    state.collection.pendingItemIDs.delete(itemID);
    replaceVisibleEntry(previous);
    if (error instanceof ApiError && error.status === 401) {
      void loadSession();
      return;
    }
    state.collection.saveErrors.set(
      itemID,
      errorMessage(
        error,
        "This collection state could not be saved. Please retry.",
      ),
    );
    refreshVisibleEntry(itemID);
  } finally {
    state.collection.pendingItemIDs.delete(itemID);
    if (restoreFocus && state.session.status === "authenticated") {
      const controlToFocus = findVisibleControl(itemID, control);
      if (controlToFocus instanceof HTMLElement) {
        controlToFocus.focus({ preventScroll: true });
      } else if (state.route.name === "collection") {
        elements.collectionTitle.focus({ preventScroll: true });
      }
    }
  }
}

export async function mockLogin(userID) {
  elements.loginError.hidden = true;
  for (const button of elements.mockUserList.querySelectorAll("button")) {
    button.disabled = true;
  }
  try {
    await request(API.mockLogin, {
      method: "POST",
      headers: csrfHeaders(),
      body: { user_id: userID },
    });
    clearLoadedKeys();
    await loadSession();
  } catch (error) {
    showLoginError(error);
    for (const button of elements.mockUserList.querySelectorAll("button")) {
      button.disabled = false;
    }
  }
}

export async function logout() {
  elements.logoutButton.disabled = true;
  try {
    await postLogout();
    await finishLocalLogout();
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      await finishLocalLogout();
      return;
    }
    if (
      error instanceof ApiError &&
      error.status === 403 &&
      error.code === "csrf_failed"
    ) {
      await loadSession();
      if (state.session.status !== "authenticated") {
        return;
      }
      try {
        await postLogout();
        await finishLocalLogout();
        return;
      } catch (retryError) {
        if (retryError instanceof ApiError && retryError.status === 401) {
          await finishLocalLogout();
          return;
        }
        error = retryError;
      }
    }
    toast(errorMessage(error, "Sign out failed. Please retry."), true);
  } finally {
    elements.logoutButton.disabled = false;
  }
}
