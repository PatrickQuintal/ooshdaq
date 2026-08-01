import { state } from "./core.js";
import { elements } from "./dom.js";
import { groupMatchesByWantedItem } from "./match-groups.js";
import { wantedItemGroup } from "./render-match-rows.js";
import { unfulfillableSection } from "./render-unfulfillable-matches.js";
import {
  errorMessage,
  loadingPanel,
  node,
  number,
  statePanel,
} from "./ui.js";

export function renderMatches() {
  const resource = state.matches;
  const { wantedItemGroups, wantedItemCount } =
    groupMatchesByWantedItem(resource.page.items);
  elements.matchesContent.setAttribute("aria-busy", String(resource.loading));
  elements.matchesResults.textContent = resource.page.items.length
    ? `${number(resource.page.items.length)} ${
        resource.page.items.length === 1 ? "possibility" : "possibilities"
      } loaded for ${number(wantedItemCount)} wanted ${
        wantedItemCount === 1 ? "item" : "items"
      }${resource.error ? " · More results could not be loaded" : ""}`
    : "";
  elements.matchesMore.hidden = !resource.page.nextCursor;
  elements.matchesMore.disabled = resource.loading;
  elements.matchesMore.textContent = resource.loading
    ? "Loading…"
    : "Load more";
  elements.refreshMatches.disabled = resource.loading;

  let actionableContent;
  if (resource.loading && !resource.page.items.length) {
    actionableContent = loadingPanel("Looking for possible swaps…");
  } else if (resource.error && !resource.page.items.length) {
    actionableContent = statePanel({
      title: "Matches unavailable",
      message: errorMessage(
        resource.error,
        "Possible swaps could not be loaded.",
      ),
      action: "retry-matches",
      actionLabel: "Retry matches",
      error: true,
    });
  } else if (!resource.page.items.length) {
    actionableContent = statePanel({
      title: "No possible swaps yet",
      message:
        "Keep your Available to swap and Want choices current, then check again as other collectors update theirs.",
    });
  } else {
    actionableContent = wantedItemGroups.map(wantedItemGroup);
  }

  elements.matchesContent.replaceChildren(
    node(
      "div",
      { className: "match-list" },
      [
        actionableContent,
        unfulfillableSection(state.unfulfillableMatches),
      ],
    ),
  );
}
