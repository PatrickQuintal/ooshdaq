export function groupMatchesByWantedItem(matches) {
  const groups = new Map();
  const seen = new Set();
  const wantedItems = new Set();

  for (const match of matches) {
    if (seen.has(match.id)) continue;
    seen.add(match.id);
    const item = match.youReceive.catalogItem;
    wantedItems.add(item.id);
    let group = groups.get(item.id);
    if (!group) {
      group = { catalogItem: item, withItemRequest: [], withoutRequest: [] };
      groups.set(item.id, group);
    }
    group[match.theyRequest ? "withItemRequest" : "withoutRequest"].push(match);
  }

  return {
    wantedItemGroups: [...groups.values()].map(
      ({ catalogItem, withItemRequest, withoutRequest }) => ({
        catalogItem,
        matches: [...withItemRequest, ...withoutRequest],
      }),
    ),
    wantedItemCount: wantedItems.size,
  };
}
