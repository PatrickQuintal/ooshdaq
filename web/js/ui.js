const PLACEHOLDER_URL = new URL(
  "../assets/collectible-placeholder.svg",
  import.meta.url,
).href;

export function node(tagName, options = {}, children = []) {
  const element = document.createElement(tagName);
  if (options.className) {
    element.className = options.className;
  }
  if (options.text !== undefined) {
    element.textContent = String(options.text);
  }
  for (const [name, value] of Object.entries(options.attributes || {})) {
    if (value !== null && value !== undefined) {
      element.setAttribute(name, String(value));
    }
  }
  for (const [name, value] of Object.entries(options.dataset || {})) {
    element.dataset[name] = String(value);
  }
  for (const child of children.flat()) {
    if (child instanceof Node) {
      element.append(child);
    } else if (child !== null && child !== undefined) {
      element.append(document.createTextNode(String(child)));
    }
  }
  return element;
}

export function initials(name) {
  return (
    String(name || "?")
      .trim()
      .split(/\s+/)
      .slice(0, 2)
      .map((part) => part[0]?.toUpperCase() || "")
      .join("") || "?"
  );
}

export function number(value) {
  return Number(value || 0).toLocaleString();
}

export function errorMessage(error, fallback) {
  if (error?.name === "ApiError" && error.message) {
    return error.message;
  }
  if (error?.name === "ContractError") {
    return "The server returned data $OOSHDAQ could not safely display.";
  }
  if (error instanceof TypeError) {
    return "$OOSHDAQ could not reach the server. Check your connection and retry.";
  }
  return fallback;
}

function approvedURL(value) {
  if (!value) {
    return null;
  }
  try {
    const url = new URL(value, window.location.origin);
    const sameOrigin = url.origin === window.location.origin;
    if (sameOrigin && ["http:", "https:"].includes(url.protocol)) {
      return url;
    }
    return url.protocol === "https:" ? url : null;
  } catch {
    return null;
  }
}

export function artworkImage(artwork, className = "") {
  const approved = approvedURL(artwork?.imageUrl);
  const image = node("img", {
    className: `artwork-image ${className}`.trim(),
    attributes: {
      alt: artwork?.altText || "Collectible artwork unavailable.",
      loading: "lazy",
      decoding: "async",
      referrerpolicy: "no-referrer",
      width: artwork?.width || (approved ? null : 600),
      height: artwork?.height || (approved ? null : 600),
    },
  });
  let usingFallback = !approved;
  image.src = approved?.href || PLACEHOLDER_URL;
  image.addEventListener("error", () => {
    if (usingFallback) {
      image.remove();
      return;
    }
    usingFallback = true;
    image.src = PLACEHOLDER_URL;
  });
  return image;
}

export function loadingPanel(message, { cards = false } = {}) {
  if (cards) {
    return node(
      "div",
      {
        className: "card-grid",
        attributes: { "aria-hidden": "true" },
      },
      [1, 2, 3].map(() => node("div", { className: "skeleton-card" })),
    );
  }
  return node(
    "div",
    { className: "state-panel state-panel--loading", attributes: { role: "status" } },
    [
      node("span", { className: "spinner", attributes: { "aria-hidden": "true" } }),
      node("p", { text: message }),
    ],
  );
}

export function happyBean(modifier = "") {
  return node(
    "span",
    {
      className: `happy-bean ${modifier}`.trim(),
      attributes: { "aria-hidden": "true" },
    },
    [
      node("span", { className: "happy-bean__eyes" }),
      node("span", { className: "happy-bean__smile" }),
    ],
  );
}

export function statePanel({
  title,
  message,
  action = "",
  actionLabel = "",
  error = false,
}) {
  const children = [
    happyBean(error ? "happy-bean--error" : ""),
    node("h2", { text: title }),
    node("p", { text: message }),
  ];
  if (action && actionLabel) {
    children.push(
      node("button", {
        className: "button button--secondary",
        text: actionLabel,
        attributes: { type: "button" },
        dataset: { action },
      }),
    );
  }
  return node("div", {
    className: `state-panel${error ? " state-panel--error" : ""}`,
    attributes: error ? { role: "alert" } : {},
  }, children);
}
