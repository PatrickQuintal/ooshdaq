# ADR 0002: Collection States and Availability Matching

## Context

Collectors need to record what they own, what they can give away, and what
additional copies they want. An available item is useful to another member
even when its owner has not requested anything in return. Treating every lead
as a reciprocal trade would hide valid opportunities and imply commitments
that the product does not make.

## Decision

Store three bounded quantities for each `(user, item)` collection entry:
`owned`, `available`, and `wanted`. Enforce:

```text
0 <= available <= owned <= 1,000,000
0 <= wanted <= 1,000,000
available > 0 => wanted = 0
```

Owned and wanted may coexist because wanted means additional desired copies.
The bundled UI writes five canonical states while the API preserves valid
quantities:

```text
neutral          (0, 0, 0)
owned            (1, 0, 0)
available        (1, 1, 0)
wanted           (0, 0, 1)
owned and wanted (1, 0, 1)
```

Maintain `available_since_at` and `wanted_since_at` as transition times. Set a
timestamp only on a zero-to-positive transition, preserve it while the state
remains positive, and clear it on a positive-to-zero transition.

For current member `A`, counterpart `B`, and item `Y`, create an availability
lead whenever `A` wants `Y` and `B` offers `Y`. No reciprocal item is
required. Exclude self matches and emit at most one lead for each
`(B, Y)` pair.

Attach at most one published item wanted by `B`:

- a request that `A` can currently fulfil makes the lead actionable;
- if `B` has no published wants, omit the request and keep the lead
  actionable as a genuine no-request extra;
- if `B` has wants but `A` can fulfil none, attach one with zero offerable
  quantity in the unfulfillable feed.

The receive and request items may cross brands and catalogs. Project only the
counterpart's display identity, the offered item, bounded receive quantity,
and at most one requested item with the amount `A` can offer now. A lead ID is
derived projection identity, not a reservation, negotiation, or trade record.

## Consequences

- Members can own an item while seeking more, but cannot offer and seek the
  same item simultaneously.
- One-way availability remains discoverable without manufacturing a return
  request.
- Actionable and unfulfillable semantics are explicit and disjoint.
- Quantity replacement and transition-time changes must be atomic.
- Results can become stale immediately because they describe current
  collection state and reserve nothing.
- Matching requires indexed repository queries rather than browser or service
  reconstruction.

## Rejected Alternatives

- Reciprocal pairs only: hides useful giveaway leads.
- One overloaded enum: loses quantity information and the valid owned-plus-
  wanted state.
- Omitting every unfulfillable request: makes “asks nothing” indistinguishable
  from “asks for something unavailable.”
- Persisted matches: suggests durable trade state and creates invalidation and
  privacy obligations the product does not need.

## Revisit When

Revisit the model before adding reservations, acceptance, messaging,
administrative visibility, partial commitments, or marketplace features.
Those capabilities need explicit lifecycle, authorization, retention, and
abuse decisions rather than more fields on an availability lead.
