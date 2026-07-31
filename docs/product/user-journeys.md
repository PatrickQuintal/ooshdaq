# User Journeys

These journeys define the initial behavior. UI labels may evolve, but the
authorization and possible-swap outcomes should remain stable.

## 1. Start Locally as a Seeded Member

1. A contributor starts $OOSHDAQ in local mock-auth mode.
2. The sign-in screen lists clearly fictional seeded members.
3. The contributor selects one member.
4. $OOSHDAQ creates a local session and opens that member's catalog dashboard.
5. Signing out clears the session and returns to the sign-in screen.

Important boundaries:

- the mock endpoint is unavailable in deployed proxy-auth mode;
- a client cannot invent a user ID outside the configured seed set;
- local cookies are still `HttpOnly` and use an explicit same-site policy.

## 2. Browse Catalogs

1. An authenticated member opens the catalog dashboard.
2. They can browse the small catalog set directly or use **Find a
   collectible** to search item names across catalogs.
3. They open the relevant catalog.
4. Each card prioritizes the collectible name and image or happy safe
   placeholder. Pressing the artwork toggles **Have**; **Available to swap**
   and **Want** remain explicit controls beneath it.
5. Items not in the member's collection are visibly muted.
6. Missing or failed artwork does not prevent collection management.

Catalog and item order are stable and deterministic. Every list is bounded; adding catalogs must not create an unbounded
response or one browser page containing every item from every catalog.

Rights and provenance remain validated manifest/API metadata. License
classifications, hosting labels, and review dates are not rendered on
collection or match cards.

## 3. Maintain a Collection

1. The member uses the controls directly on a collectible card.
2. Pressing the artwork toggles **Have** and records or clears an owned item.
3. **Available** records an owned item that may be swapped or given away.
4. **Want** records an item the member does not have.
5. The server validates the complete state, saves it atomically, and returns
   the canonical result.

Invariants:

- quantities are non-negative integers and have a configured upper bound;
- available cannot exceed owned;
- owned and wanted may coexist when the member has one copy and wants more;
- available and wanted are mutually exclusive;
- the initial browser writes only neutral `(0,0,0)`, owned `(1,0,0)`,
  available `(1,1,0)`, wanted `(0,0,1)`, or owned-and-wanted `(1,0,1)`;
- zero values may remove an otherwise empty entry;
- the authenticated subject, never a URL or body field, selects the collection being changed.

## 4. Discover an Available Item

Given:

- Alex wants Maui; and
- Bailey marks Maui available.

Then Alex opens Matches and sees a **Maui** group with Bailey listed beneath
it. Bailey does not need to mark anything wanted. If Bailey also wants an item
Alex has available, Bailey's row plainly names that item; it may belong to
another catalog or brand. If Bailey wants an item Alex cannot currently offer,
the actionable page does not mislabel or return Bailey. Alex may open the
muted **Swaps you can't fulfil yet** section, which then runs its own bounded
query and shows Bailey with that requested collectible. Only when Bailey has
no published wants does the actionable row say Bailey has not asked for
anything back.

The member interface does not ask Alex to understand give-catalog,
receive-catalog, reciprocal-scope, or same/cross-catalog filters. When both
kinds exist for one wanted item, people who want one of Alex's available items
appear before people who have not asked for anything back. Unfulfillable
requests remain separate from both and are not fetched until Alex opens their
section.

Within each kind, older relevant states are preferred but waiting advantage
stops growing after 60 days. Only then may the private coarse stale signal move
a record whose state and last activity are both older than 90 days downward.
The interface never displays waiting age, last login, or an activity label.

The result:

- is computed from current collection state rather than treated as a binding trade;
- never includes the current member as their own counterpart;
- is deterministic and does not duplicate the same counterpart/item lead;
- shows no private contact details beyond the deployment-approved display identity;
- exposes at most one deterministic counterpart want and no unrelated
  holdings; and
- can disappear immediately when either member changes the relevant available
  or wanted state.

The members coordinate elsewhere. $OOSHDAQ does not mark the exchange complete.

## 5. No Match Yet

If a member wants an item but no one has marked that item available, the UI
explains that there are no possible swaps yet. It must not imply that no copy
exists anywhere or reveal unrelated holdings.

## 6. Return After Restart or Node Roll

1. A member updates their collection.
2. The process or pod stops and Kubernetes starts a replacement using the same data volume.
3. Their collection and calculated matches remain consistent.

The service may be briefly unavailable during replacement or volume attachment. That downtime is accepted. Losing
committed collection data is not.

Catalog version changes must not silently remap a stored collectible ID to a different item.

## 7. Sign In Through a Deployed Identity Proxy

1. The member authenticates with the deployment's identity provider.
2. A trusted auth proxy removes any identity headers sent by the browser.
3. The proxy injects the authenticated subject and display name.
4. $OOSHDAQ validates those headers and resolves or provisions the minimal local user record.
5. The response does not expose upstream tokens or claims.

If the proxy does not supply a valid identity, the request fails closed. See
[authentication](../architecture/authentication.md).
