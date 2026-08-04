import {
  abortResource,
  requestControllers,
  state,
} from "./core.js";

export function queryKey(value) {
  return JSON.stringify(value);
}

export function beginRequest(name) {
  abortResource(name);
  const resource = state[name];
  const controller = new AbortController();
  requestControllers[name] = controller;
  resource.requestGeneration += 1;
  resource.loading = true;
  resource.error = null;
  return {
    controller,
    generation: resource.requestGeneration,
  };
}

export function isCurrent(name, generation, controller) {
  return (
    state[name].requestGeneration === generation &&
    requestControllers[name] === controller
  );
}

export function finishRequest(name, generation, controller, render) {
  if (!isCurrent(name, generation, controller)) {
    return;
  }
  state[name].loading = false;
  requestControllers[name] = null;
  render();
}

export function acceptPage(resource, values, page, cursor, key) {
  const seen = new Set(cursor ? resource.page.items.map(key) : []);
  const accepted = values.filter((value) => {
    const valueKey = key(value);
    if (seen.has(valueKey)) return false;
    seen.add(valueKey);
    return true;
  });
  if (!cursor) {
    resource.page.items = accepted;
    resource.page.requestCursor = null;
    resource.page.backStack = [];
    resource.page.number = 1;
  } else {
    resource.page.items.push(...accepted);
    resource.page.backStack.push(resource.page.requestCursor);
    resource.page.requestCursor = cursor;
    resource.page.number += 1;
  }
  resource.page.limit = page.limit;
  resource.page.nextCursor = page.nextCursor;
}
