# Catalog Artwork, Licensing, and Provenance

## Rights boundary

Apache-2.0 covers $OOSHDAQ's original work, not third-party photographs, character
art, logos, names, descriptions, or trademarks. A public URL is not permission to
copy, redistribute, modify, or hotlink an asset.

When rights are uncertain, publish factual original metadata with the placeholder or use a
separately reviewed remote link. Never copy uncertain bytes into the repository, binary, or container.

The serving catalog manifest is the canonical machine record for each item's source, rights
decision, review, and transformation. See the seed [catalog manifest](../../internal/catalog/manifests/disney-ooshies-2026.v1.json).
A source-hosted-link decision permits only the recorded link; it does not imply copying rights,
affiliation, or endorsement.

## Serving policy

Use the first applicable option:

1. original $OOSHDAQ placeholder or illustration;
2. an asset under a compatible license, with required attribution;
3. an asset covered by written permission for this use;
4. a reviewed remote link whose terms and privacy effects are acceptable; or
5. the placeholder for unknown or restricted rights.

Do not rely on fair-use assumptions; legal exceptions require an explicit project decision.

## Required catalog record

For every non-placeholder asset, the serving manifest records:

- stable brand, catalog, and item identifiers;
- the source page and exact asset URL;
- retrieval and review dates;
- rights status, license statement, and required attribution;
- a stable review role or decision reference; and
- any transformation applied to published bytes.

Use stable roles or review identifiers, not personal contact details. Keep written permission
evidence in access-controlled storage and publish only its non-sensitive reference.

| Status | Publish behavior |
| --- | --- |
| `original` | Bundle under the repository license. |
| `licensed` | Bundle or link within the recorded license. |
| `permissioned-link` | Link within the recorded permission and privacy constraints. |
| `source-hosted-link` | Link after explicit review; no copying rights are implied. |
| `unknown` | Use the placeholder. |
| `restricted` | Use the placeholder and retain only factual review data. |

Project-owned identifiers remain stable; a display name or source change must not remap stored collections.

## Catalog approval

Before enabling a catalog:

- review rights, source terms, privacy effects, and source integrity;
- record a decision for every non-placeholder asset;
- review each remote image host before changing Content Security Policy;
- validate deterministic, idempotent import without identifier remapping;
- verify unavailable images fall back without blocking member workflows; and
- keep rights metadata out of member-facing collection and match cards.

Approval for one catalog or host does not extend to another.

## Browser and remote-image controls

- Use meaningful alternative text, or empty alternative text for decoration.
- Never place member identifiers, sessions, or referrer data in image URLs.
- Send no referrer information;
- Allow only reviewed image hosts through Content Security Policy;
- Lazy-load images with explicit dimensions;
- Fall back to the bundled placeholder on load or decode failure; and
- Never proxy arbitrary user-supplied URLs.

Remote hosts observe requests, can change content, and may prohibit linking. A first-party cached copy still requires redistribution rights.

## Controlled ingestion

Any ingestion tool runs offline with allowlisted HTTPS sources, timeouts, byte and redirect
limits, content-derived type checks, and temporary review storage. It rejects active content,
strips unnecessary metadata, calculates a digest, produces a reviewable diff, and requires human
approval. It stops on authentication or anti-bot controls and never runs from a user request or startup.

## Removal and disputes

If rights are questioned, use the placeholder, preserve non-sensitive evidence, assess every
distributed reference, and coordinate privately. Restore the image only after a new documented
decision. Removing artwork must not change its identifier or a member's collection state.
