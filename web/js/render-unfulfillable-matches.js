import { personRow } from "./render-match-rows.js";
import {
  errorMessage,
  loadingPanel,
  node,
  number,
  statePanel,
} from "./ui.js";

function feedBody(resource) {
  const attributes = {
    id: "unfulfillable-matches-content",
    hidden: resource.expanded ? null : "",
    "aria-busy": String(resource.loading),
  };
  if (resource.loading && !resource.page.items.length) {
    return node(
      "div",
      { className: "match-pending__content", attributes },
      [loadingPanel("Checking the swaps you can’t fulfil yet…")],
    );
  }
  if (resource.error && !resource.page.items.length) {
    return node(
      "div",
      { className: "match-pending__content", attributes },
      [
        statePanel({
          title: "These swaps couldn’t be checked",
          message: errorMessage(
            resource.error,
            "The extra swaps could not be loaded.",
          ),
          action: "retry-unfulfillable-matches",
          actionLabel: "Try checking again",
          error: true,
        }),
      ],
    );
  }
  if (!resource.page.items.length) {
    return node(
      "div",
      { className: "match-pending__content", attributes },
      [
        statePanel({
          title: "Nothing waiting here",
          message:
            "There are no requested collectibles blocking your other possible swaps.",
        }),
      ],
    );
  }

  const count = resource.page.items.length;
  const children = [
    node("p", {
      className: "results-meta match-pending__results",
      text: `${number(count)} ${count === 1 ? "swap" : "swaps"} loaded`,
      attributes: { "aria-live": "polite" },
    }),
    node(
      "div",
      {
        className: "match-pending__people",
        attributes: {
          role: "list",
          "aria-label": "Swaps you can’t fulfil yet",
        },
      },
      resource.page.items.map((match) =>
        personRow(match, match.youReceive.catalogItem),
      ),
    ),
  ];
  if (resource.error) {
    children.push(
      node(
        "div",
        {
          className: "match-pending__error",
          attributes: { role: "alert" },
        },
        [
          node("p", {
            text: errorMessage(
              resource.error,
              "More of these swaps could not be loaded.",
            ),
          }),
          node("button", {
            className: "button button--secondary",
            text: "Retry loading more",
            attributes: { type: "button" },
            dataset: { action: "load-more-unfulfillable-matches" },
          }),
        ],
      ),
    );
  } else if (resource.page.nextCursor) {
    children.push(
      node("div", { className: "match-pending__more" }, [
        node("button", {
          className: "button button--secondary",
          text: resource.loading ? "Loading…" : "Load more",
          attributes: {
            type: "button",
            disabled: resource.loading ? "" : null,
          },
          dataset: { action: "load-more-unfulfillable-matches" },
        }),
      ]),
    );
  }
  return node(
    "div",
    { className: "match-pending__content", attributes },
    children,
  );
}

export function unfulfillableSection(resource) {
  const headingID = "unfulfillable-matches-title";
  return node(
    "section",
    {
      className: "match-pending",
      attributes: { "aria-labelledby": headingID },
    },
    [
      node("header", { className: "match-pending__header" }, [
        node("div", {}, [
          node("span", { className: "eyebrow", text: "Not ready to swap" }),
          node("h2", {
            text: "Swaps you can’t fulfil yet",
            attributes: { id: headingID },
          }),
          node("p", {
            text:
              "Some collectors may request an item you don’t have available. Check these separately when you’re ready.",
          }),
        ]),
        node("button", {
          className: "button button--secondary match-pending__toggle",
          text: resource.expanded
            ? "Hide swaps you can’t fulfil yet"
            : "Check swaps you can’t fulfil yet",
          attributes: {
            type: "button",
            "aria-expanded": String(resource.expanded),
            "aria-controls": "unfulfillable-matches-content",
          },
          dataset: { action: "toggle-unfulfillable-matches" },
        }),
      ]),
      feedBody(resource),
    ],
  );
}
