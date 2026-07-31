import {
  artworkImage,
  happyBean,
  node,
  number,
} from "./ui.js";

function initials(displayName) {
  return displayName
    .split(/\s+/)
    .slice(0, 2)
    .map((part) => part[0])
    .join("")
    .toUpperCase();
}

function whatTheyWant(match) {
  const request = match.theyRequest;
  if (!request) {
    return node("div", { className: "match-ask match-ask--open" }, [
      happyBean("happy-bean--small"),
      node("p", {}, [
        node("strong", { text: "They haven’t asked for anything back." }),
      ]),
    ]);
  }

  const item = request.catalogItem;
  const unfulfillable = !request.fulfillable;
  return node(
    "div",
    {
      className: `match-ask ${
        unfulfillable ? "match-ask--unfulfillable" : "match-ask--item"
      }`,
    },
    [
      node("div", { className: "match-ask__art" }, [
        artworkImage(item.artwork),
      ]),
      node("p", {}, [
        node("strong", {
          text: unfulfillable
            ? `They’re looking for ${item.name}.`
            : `They want your ${item.name}.`,
        }),
        unfulfillable
          ? node("span", {
              className: "match-ask__note",
              text: " You don’t have one available to swap yet.",
            })
          : null,
      ]),
    ],
  );
}

export function personRow(match, wantedItem) {
  const otherName = match.otherUser.displayName;
  const firstName = otherName.split(/\s+/)[0];
  const requestState = !match.theyRequest
    ? "has-no-request"
    : match.theyRequest.fulfillable
      ? "has-item-request"
      : "has-unfulfillable-request";
  return node(
    "article",
    {
      className: `match-person ${requestState}`,
      attributes: { role: "listitem" },
    },
    [
      node("header", { className: "match-person__header" }, [
        node("span", {
          className: "match-avatar",
          text: initials(otherName),
          attributes: { "aria-hidden": "true" },
        }),
        node("div", {}, [
          node("h3", { text: otherName }),
          node("p", {
            text: `${firstName} has ${wantedItem.name} available.`,
          }),
        ]),
      ]),
      whatTheyWant(match),
    ],
  );
}

export function wantedItemGroup(group, index) {
  const item = group.catalogItem;
  const headingID = `wanted-item-${index + 1}`;
  const count = group.matches.length;
  return node(
    "section",
    {
      className: "match-group",
      attributes: { "aria-labelledby": headingID },
    },
    [
      node("header", { className: "match-group__header" }, [
        node("div", { className: "match-group__art" }, [
          artworkImage(item.artwork),
        ]),
        node("div", {}, [
          node("span", { className: "eyebrow", text: "You want" }),
          node("h2", { text: item.name, attributes: { id: headingID } }),
          node("p", {
            text:
              count === 1
                ? "1 person has this available."
                : `${number(count)} people have this available.`,
          }),
        ]),
      ]),
      node(
        "div",
        {
          className: "match-people",
          attributes: {
            role: "list",
            "aria-label": `People with ${item.name} available`,
          },
        },
        group.matches.map((match) => personRow(match, item)),
      ),
    ],
  );
}
