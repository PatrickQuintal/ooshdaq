export class ApiError extends Error {
  constructor(message, status, code = "", field = "") {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.field = field;
  }
}

function fallbackMessage(status) {
  if (status === 401) {
    return "Your session has ended. Please sign in again.";
  }
  if (status === 403) {
    return "This request could not be verified. Refresh and try again.";
  }
  if (status === 404) {
    return "The requested catalog or collectible was not found.";
  }
  if (status >= 500) {
    return "$OOSHDAQ could not complete that request. Please try again.";
  }
  return `The request could not be completed (${status}).`;
}

function errorFromPayload(payload, status) {
  const detail =
    payload?.error && typeof payload.error === "object" ? payload.error : null;
  return new ApiError(
    typeof detail?.message === "string" && detail.message.trim()
      ? detail.message.trim()
      : fallbackMessage(status),
    status,
    typeof detail?.code === "string" ? detail.code : "",
    typeof detail?.field === "string" ? detail.field : "",
  );
}

export async function request(path, options = {}) {
  const headers = new Headers(options.headers || {});
  headers.set("Accept", "application/json");

  let body = options.body;
  if (body !== undefined) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(body);
  }

  const response = await fetch(path, {
    method: options.method || "GET",
    headers,
    body,
    signal: options.signal,
    credentials: "same-origin",
    cache: "no-store",
  });

  let payload = null;
  if (response.status !== 204) {
    const contentType = response.headers.get("content-type") || "";
    try {
      payload = contentType.includes("application/json")
        ? await response.json()
        : null;
    } catch {
      payload = null;
    }
  }

  if (!response.ok) {
    throw errorFromPayload(payload, response.status);
  }
  return payload;
}

export function pageURL(path, filters, cursor = null) {
  const query = new URLSearchParams();
  if (cursor) {
    query.set("cursor", cursor);
  } else {
    for (const [name, value] of Object.entries(filters)) {
      if (value !== "" && value !== null && value !== undefined) {
        query.set(name, String(value));
      }
    }
  }
  const encoded = query.toString();
  return encoded ? `${path}?${encoded}` : path;
}

export function isAbort(error) {
  return error?.name === "AbortError";
}
