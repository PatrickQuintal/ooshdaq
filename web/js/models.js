export class ContractError extends Error {
  constructor(message) {
    super(message);
    this.name = "ContractError";
  }
}

function object(value, label) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new ContractError(`${label} must be an object.`);
  }
  return value;
}

function list(value, label) {
  if (!Array.isArray(value)) {
    throw new ContractError(`${label} must be an array.`);
  }
  return value;
}

function string(value, label) {
  if (typeof value !== "string" || !value.trim()) {
    throw new ContractError(`${label} must be a non-empty string.`);
  }
  return value;
}

function optionalString(value, label) {
  return value === undefined ? "" : string(value, label);
}

function integer(value, label, minimum = 0) {
  if (!Number.isSafeInteger(value) || value < minimum) {
    throw new ContractError(`${label} must be an integer of at least ${minimum}.`);
  }
  return value;
}

function boolean(value, label) {
  if (typeof value !== "boolean") {
    throw new ContractError(`${label} must be a boolean.`);
  }
  return value;
}

function choice(value, allowed, label) {
  const parsed = string(value, label);
  if (!allowed.includes(parsed)) {
    throw new ContractError(`${label} has an unsupported value.`);
  }
  return parsed;
}

function quantity(value, label) {
  const parsed = integer(value, label);
  if (parsed > 1_000_000) {
    throw new ContractError(`${label} exceeds the quantity maximum.`);
  }
  return parsed;
}

const CANONICAL_COLLECTION_STATES = new Set([
  "0:0:0",
  "1:0:0",
  "1:1:0",
  "0:0:1",
  "1:0:1",
]);

function collectionStateKey(value) {
  return `${value.owned}:${value.available}:${value.wanted}`;
}

export function isCanonicalCollectionState(value) {
  return CANONICAL_COLLECTION_STATES.has(collectionStateKey(value));
}

function isValidCollectionState(value) {
  return (
    ["owned", "available", "wanted"].every(
      (field) =>
        Number.isSafeInteger(value[field]) &&
        value[field] >= 0 &&
        value[field] <= 1_000_000,
    ) &&
    value.available <= value.owned &&
    !(value.wanted > 0 && value.available > 0)
  );
}

export function nextCollectionQuantities(entry, control, checked) {
  if (!isValidCollectionState(entry)) {
    throw new ContractError("collection entry has an invalid state.");
  }
  if (!["owned", "available", "wanted"].includes(control)) {
    throw new ContractError("collection control has an unsupported value.");
  }
  if (typeof checked !== "boolean") {
    throw new ContractError("collection control state must be a boolean.");
  }
  if (control === "owned") {
    return {
      owned: checked ? 1 : 0,
      available: 0,
      wanted: entry.wanted > 0 ? 1 : 0,
    };
  }
  if (control === "available") {
    return checked
      ? { owned: 1, available: 1, wanted: 0 }
      : { owned: 1, available: 0, wanted: 0 };
  }
  return {
    owned: entry.owned > 0 ? 1 : 0,
    available: 0,
    wanted: checked ? 1 : 0,
  };
}

function parseArtwork(value, label) {
  const raw = object(value, label);
  const width = raw.width === undefined ? null : integer(raw.width, `${label}.width`, 1);
  const height =
    raw.height === undefined ? null : integer(raw.height, `${label}.height`, 1);
  if ((width === null) !== (height === null)) {
    throw new ContractError(`${label} dimensions must appear together.`);
  }
  const artwork = {
    imageUrl: optionalString(raw.image_url, `${label}.image_url`),
    altText: string(raw.alt_text, `${label}.alt_text`),
    width,
    height,
    sourceUrl: optionalString(raw.source_url, `${label}.source_url`),
    attribution: string(raw.attribution, `${label}.attribution`),
    rightsStatus: choice(
      raw.rights_status,
      [
        "original",
        "licensed",
        "permissioned-link",
        "source-hosted-link",
        "unknown",
        "restricted",
      ],
      `${label}.rights_status`,
    ),
    license: optionalString(raw.license, `${label}.license`),
    retrievedAt: string(raw.retrieved_at, `${label}.retrieved_at`),
  };
  if (
    ["unknown", "restricted"].includes(artwork.rightsStatus) &&
    (artwork.imageUrl || width !== null)
  ) {
    throw new ContractError(`${label} cannot publish restricted artwork.`);
  }
  return artwork;
}

function parseBrand(value, label) {
  const raw = object(value, label);
  return {
    id: string(raw.id, `${label}.id`),
    name: string(raw.name, `${label}.name`),
  };
}

function parseMemberSummary(value, label) {
  const raw = object(value, label);
  const result = {};
  for (const field of [
    "owned_quantity",
    "owned_designs",
    "available_quantity",
    "available_designs",
    "wanted_quantity",
    "wanted_designs",
  ]) {
    result[field] = integer(raw[field], `${label}.${field}`);
  }
  return result;
}

export function parseCatalog(value, label = "catalog") {
  const raw = object(value, label);
  return {
    id: string(raw.id, `${label}.id`),
    brand: parseBrand(raw.brand, `${label}.brand`),
    name: string(raw.name, `${label}.name`),
    edition: optionalString(raw.edition, `${label}.edition`),
    publicationStatus: choice(
      raw.publication_status,
      ["active", "retired"],
      `${label}.publication_status`,
    ),
    sortOrder: integer(raw.sort_order, `${label}.sort_order`),
    itemCount: integer(raw.item_count, `${label}.item_count`),
    universeOptions: list(
      raw.universe_options,
      `${label}.universe_options`,
    ).map((item, index) => string(item, `${label}.universe_options[${index}]`)),
    artwork: parseArtwork(raw.artwork, `${label}.artwork`),
    memberSummary: parseMemberSummary(
      raw.member_summary,
      `${label}.member_summary`,
    ),
  };
}

function parseCompactCatalog(value, label) {
  const raw = object(value, label);
  return {
    id: string(raw.id, `${label}.id`),
    name: string(raw.name, `${label}.name`),
    edition: optionalString(raw.edition, `${label}.edition`),
    publicationStatus: choice(
      raw.publication_status,
      ["active", "retired"],
      `${label}.publication_status`,
    ),
    brand: parseBrand(raw.brand, `${label}.brand`),
  };
}

export function parseCatalogItem(value, label = "catalog_item") {
  const raw = object(value, label);
  const catalogID = string(raw.catalog_id, `${label}.catalog_id`);
  const catalog = parseCompactCatalog(raw.catalog, `${label}.catalog`);
  if (catalog.id !== catalogID) {
    throw new ContractError(`${label}.catalog.id must equal catalog_id.`);
  }
  return {
    id: string(raw.id, `${label}.id`),
    catalogID,
    catalog,
    number: integer(raw.number, `${label}.number`, 1),
    name: string(raw.name, `${label}.name`),
    universe: optionalString(raw.universe, `${label}.universe`),
    variant: optionalString(raw.variant, `${label}.variant`),
    publicationStatus: choice(
      raw.publication_status,
      ["active", "retired"],
      `${label}.publication_status`,
    ),
    artwork: parseArtwork(raw.artwork, `${label}.artwork`),
  };
}

export function parseEntry(value, label = "entry") {
  const raw = object(value, label);
  const entry = {
    catalogItem: parseCatalogItem(raw.catalog_item, `${label}.catalog_item`),
    owned: quantity(raw.owned, `${label}.owned`),
    available: quantity(raw.available, `${label}.available`),
    wanted: quantity(raw.wanted, `${label}.wanted`),
  };
  if (entry.available > entry.owned) {
    throw new ContractError(`${label}.available cannot exceed owned.`);
  }
  if (entry.wanted > 0 && entry.available > 0) {
    throw new ContractError(
      `${label}.wanted cannot be combined with available.`,
    );
  }
  return entry;
}

function parsePage(value, returned, maximum, label = "page") {
  const raw = object(value, label);
  const limit = integer(raw.limit, `${label}.limit`, 1);
  if (limit > maximum) {
    throw new ContractError(`${label}.limit exceeds ${maximum}.`);
  }
  if (integer(raw.returned, `${label}.returned`) !== returned) {
    throw new ContractError(`${label}.returned does not match the resource array.`);
  }
  if (
    raw.next_cursor !== null &&
    (typeof raw.next_cursor !== "string" || !raw.next_cursor)
  ) {
    throw new ContractError(`${label}.next_cursor must be a string or null.`);
  }
  return { limit, returned, nextCursor: raw.next_cursor };
}

export function parseCatalogPage(payload) {
  const raw = object(payload, "catalog response");
  const catalogs = list(raw.catalogs, "catalogs").map((item, index) =>
    parseCatalog(item, `catalogs[${index}]`),
  );
  return { catalogs, page: parsePage(raw.page, catalogs.length, 50) };
}

export function parseCollectionPage(payload) {
  const raw = object(payload, "collection response");
  const entries = list(raw.entries, "entries").map((item, index) =>
    parseEntry(item, `entries[${index}]`),
  );
  return {
    catalog: parseCatalog(raw.catalog),
    entries,
    page: parsePage(raw.page, entries.length, 100),
  };
}

export function parseOverallSummary(payload) {
  const raw = object(payload, "summary response");
  const summary = parseMemberSummary(raw.summary, "summary");
  summary.active_catalog_count = integer(
    raw.summary.active_catalog_count,
    "summary.active_catalog_count",
  );
  summary.active_item_count = integer(
    raw.summary.active_item_count,
    "summary.active_item_count",
  );
  summary.catalogs_started = integer(
    raw.summary.catalogs_started,
    "summary.catalogs_started",
  );
  summary.possible_swap_count = integer(
    raw.summary.possible_swap_count,
    "summary.possible_swap_count",
  );
  return summary;
}

function parseLeg(value, label) {
  const raw = object(value, label);
  return {
    catalogItem: parseCatalogItem(raw.catalog_item, `${label}.catalog_item`),
    quantity: integer(raw.quantity, `${label}.quantity`, 1),
  };
}

function parseTheyRequest(value, label) {
  const raw = object(value, label);
  const request = {
    catalogItem: parseCatalogItem(raw.catalog_item, `${label}.catalog_item`),
    quantity: integer(raw.quantity, `${label}.quantity`),
    fulfillable: boolean(raw.fulfillable, `${label}.fulfillable`),
  };
  if (request.fulfillable !== (request.quantity > 0)) {
    throw new ContractError(
      `${label}.fulfillable must be true exactly when quantity is positive.`,
    );
  }
  return request;
}

export function parseMatchPage(payload, readiness = "") {
  const expectedReadiness = readiness
    ? choice(
        readiness,
        ["actionable", "unfulfillable"],
        "match readiness",
      )
    : "";
  const raw = object(payload, "match response");
  const matches = list(raw.matches, "matches").map((value, index) => {
    const label = `matches[${index}]`;
    const match = object(value, label);
    const otherUser = object(match.other_user, `${label}.other_user`);
    if (match.you_give !== undefined) {
      throw new ContractError(
        `${label}.you_give is no longer supported; use they_request.`,
      );
    }
    const theyRequest =
      match.they_request === undefined
        ? null
        : parseTheyRequest(match.they_request, `${label}.they_request`);
    const youReceive = parseLeg(match.you_receive, `${label}.you_receive`);
    return {
      id: string(match.id, `${label}.id`),
      otherUser: {
        id: string(otherUser.id, `${label}.other_user.id`),
        displayName: string(
          otherUser.display_name,
          `${label}.other_user.display_name`,
        ),
      },
      theyRequest,
      youReceive,
    };
  });
  for (const [index, match] of matches.entries()) {
    if (
      expectedReadiness === "actionable" &&
      match.theyRequest &&
      !match.theyRequest.fulfillable
    ) {
      throw new ContractError(
        `matches[${index}] is not actionable.`,
      );
    }
    if (
      expectedReadiness === "unfulfillable" &&
      (!match.theyRequest || match.theyRequest.fulfillable)
    ) {
      throw new ContractError(
        `matches[${index}] is not unfulfillable.`,
      );
    }
  }
  return { matches, page: parsePage(raw.page, matches.length, 50) };
}

export function parseSavedEntry(payload, expectedItemID = "") {
  const entry = parseEntry(object(payload, "saved response").entry);
  if (expectedItemID && entry.catalogItem.id !== expectedItemID) {
    throw new ContractError("saved entry item ID does not match the request.");
  }
  return entry;
}

export function parseSession(payload) {
  const raw = object(payload, "session response");
  const authenticated = Boolean(raw.authenticated);
  const user = authenticated ? object(raw.user, "session.user") : null;
  const mockUsers = raw.mock_users === undefined ? [] : list(raw.mock_users, "mock_users");
  return {
    status: authenticated ? "authenticated" : "anonymous",
    user: user
      ? {
          id: string(user.id, "session.user.id"),
          displayName: string(user.display_name, "session.user.display_name"),
        }
      : null,
    authMode: string(raw.auth_mode, "session.auth_mode"),
    csrfToken: string(raw.csrf_token, "session.csrf_token"),
    mockUsers: mockUsers.map((value, index) => {
      const profile = object(value, `mock_users[${index}]`);
      return {
        id: string(profile.id, `mock_users[${index}].id`),
        displayName: string(
          profile.display_name,
          `mock_users[${index}].display_name`,
        ),
      };
    }),
  };
}
