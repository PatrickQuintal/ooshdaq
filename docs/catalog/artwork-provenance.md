# Catalog Artwork, Licensing, and Provenance

## Core Rule

The Apache-2.0 license covers $OOSHDAQ's original code and documentation. It does not grant rights to third-party product
photographs, character art, logos, names, descriptions, or trademarks. A public source URL is not permission to copy,
redistribute, modify, or hotlink an asset.

The initial catalog may use the public
[Disney Ooshies information page](https://www.woolworths.com.au/shop/discover/disney-ooshies) as a factual research source.
Before fetching or publishing anything from that site, a maintainer must review the current terms, robots policy, asset
rights, and intended use. $OOSHDAQ is not affiliated with or endorsed by the source site or relevant rights holders.

When redistribution rights are uncertain, prefer a reviewed remote link or publish factual original metadata with the
$OOSHDAQ placeholder—never copy the uncertain image into the repository or release image.

## Current Seed Behavior

The initial catalog represents the 40-character 2026 set shown on Woolworths' current Disney Ooshies page. That page
exposes a distinct transparent character render for every picker entry. Each catalog `image_url` links directly to its
corresponding source-hosted asset on `cdn0.woolworths.media`; $OOSHDAQ does not splice a group image or generate a
lookalike.

- No third-party image bytes are committed, embedded in the binary, or redistributed in the container.
- The browser loads each HTTPS image with `Referrer-Policy: no-referrer` behavior, lazy loading, and a graceful local
  placeholder/fallback.
- The content security policy allows that specific image host, not arbitrary remote scripts or media.
- The source can change or remove an image; collection management remains usable when it does.
- Deployers must re-check source terms, linking permission, privacy policy, and organizational policy before shared use.

The mapping and review date are recorded in
[`disney-ooshies-2026-artwork.json`](disney-ooshies-2026-artwork.json). This source-hosted-link decision is narrower than a
license or permission to copy, modify, redistribute, or necessarily hotlink the images. It does not imply affiliation or
endorsement. A deployment whose policy does not permit third-party image requests must clear the `image_url` values and
use the bundled placeholders.

On 2026-07-30, an availability probe using an ordinary browser user agent and no referrer received `200 image/png` for
all 40 recorded asset URLs. No probed bytes were retained in the repository. The CDN rejected a generic command-line user
agent, so this check proves current browser availability rather than an availability guarantee or permission grant; the
fallback remains required.

## Multi-Catalog Baseline

The Disney Ooshies manifest is the first implemented catalog, not the product
boundary. The accepted target is at least five finite catalogs across multiple
brands, with possible swaps and optional reciprocal items allowed between
catalogs. Additional catalogs are not complete merely because item names and
URLs have been collected.

Before a catalog is enabled:

- give its brand, catalog or edition, items, sources, and provenance records stable, distinct identifiers;
- perform a rights, terms, privacy, and source-integrity review specific to that brand and source host;
- record a reviewed manifest for every non-placeholder asset and an explicit placeholder decision for the rest;
- review every new remote image host before adding it to Content Security Policy;
- validate an offline, idempotent import and ensure it cannot remap existing item IDs;
- verify each cross-catalog item retains the correct internal rights metadata
  without adding license-review copy to member-facing cards;
- test unavailable or removed source assets without blocking collection or match workflows.

Permission, linking practice, attribution, or source-host approval for one catalog does not carry over to another. A
five-catalog fixture without five independent provenance decisions is useful test data, not publishable catalog content.
See [operating requirements](../product/operating-requirements.md).

## Serving Policy

Preferred order:

1. original $OOSHDAQ placeholder or illustration;
2. asset under an explicit compatible license, with attribution;
3. asset covered by written permission for this use, with the permission reference;
4. a reviewed remote link only when linking terms and the privacy impact are permitted;
5. placeholder for `unknown` or `restricted` rights.

Do not copy third-party assets into the repository based on fair-use assumptions. Legal exceptions are jurisdiction- and
context-specific and require an explicit project decision.

Remote images have additional drawbacks: the host observes member IP/browser requests, content can change, tracking can
be introduced, availability is outside $OOSHDAQ's control, and a source may prohibit hotlinking. A first-party cached copy
still requires redistribution rights.

## Provenance Record

Every non-placeholder image needs a reviewed manifest record equivalent to:

```json
{
  "brand_id": "brand-example",
  "catalog_id": "catalog-example-2026",
  "item_id": "collectible-example-red",
  "source_page": "https://example.invalid/catalog/example-red",
  "source_asset": "https://example.invalid/assets/example-red.jpg",
  "retrieved_at": "2026-07-30",
  "rights_status": "licensed",
  "license": "CC-BY-4.0",
  "attribution": "Example artwork by Example Creator",
  "permission_reference": "",
  "sha256": "hex-digest-of-published-bytes",
  "transformation": "decoded, metadata removed, resized, re-encoded as WebP",
  "review_reference": "maintainer-role-or-review-id",
  "reviewed_at": "2026-07-30"
}
```

Use stable review identifiers or roles rather than personal contact details. For written permission, store only a
non-public reference in the public manifest; keep the evidence in an access-controlled record.

`brand_id`, `catalog_id`, and `item_id` are project-owned identifiers. They must remain globally unambiguous because
one possible-swap response can contain items from two different catalogs.

Rights statuses:

| Status | Publish behavior |
| --- | --- |
| `original` | Bundle under the repository's stated license |
| `licensed` | Bundle/link according to the recorded third-party license |
| `permissioned-link` | Link only within the recorded permission and privacy constraints |
| `source-hosted-link` | Link only after an explicit project review; no copying or redistribution rights are implied |
| `unknown` | Show placeholder |
| `restricted` | Show placeholder; retain source only for factual review |

## Controlled Ingestion Direction

Catalog maintenance may eventually have a separate ingestion command. It must never run on an end-user request or as an
unreviewed production startup side effect.

An ingestion workflow should:

1. accept allowlisted HTTPS source pages/assets;
2. use explicit timeouts, byte limits, redirect limits, and a descriptive user agent;
3. respect source access rules and stop on authentication or anti-bot controls;
4. store downloads only in a temporary review area, not directly in served assets;
5. inspect content-derived MIME, dimensions, decode cost, and malware signals;
6. reject HTML, scripts, and active SVG; re-encode approved raster content;
7. strip unnecessary metadata and tracking parameters;
8. calculate a digest and produce a provenance diff;
9. require a human rights/provenance approval;
10. reject duplicate or remapped brand, catalog, and collectible identifiers;
11. publish catalog metadata and approved asset in one versioned change.

Scraping around access controls, accepting a source's labels without review, or silently replacing changed bytes is not an
acceptable ingestion strategy.

## Catalog Metadata

Factual names and release facts should be expressed in original wording and kept to what the product needs. Do not copy
marketing paragraphs. Record the page used to verify facts and the retrieval date. Trademark use should be nominative and
must not imply sponsorship.

Brand, catalog or edition, and collectible IDs are $OOSHDAQ-owned stable
identifiers. A source name change may update display text but must not remap
stored collections. Matching across catalogs changes neither asset rights nor
attribution: each projected item retains the provenance of its own catalog
record, while the member-facing workflow omits license-review copy.

## Browser and Privacy Controls

- Always provide meaningful `alt` text or empty alt text for decorative imagery.
- Do not put member identifiers, sessions, or referrer query data in image URLs.
- Prefer `Referrer-Policy: no-referrer` or an equivalently conservative application policy.
- Restrict `img-src` with Content Security Policy.
- Use lazy loading and explicit dimensions to avoid layout shifts.
- Fall back to the bundled placeholder on load/decode error.
- Do not proxy arbitrary user-supplied URLs.
- Treat approximately 10,000 eligible identities and fewer than 100 concurrent members as potential third-party image
  request volume during source-host and privacy review; the private deployment must not shift an unreviewed load to a
  catalog owner's infrastructure.

## Removal and Dispute Process

If rights are questioned:

1. replace the asset with the placeholder promptly;
2. preserve the provenance record and relevant private evidence;
3. assess whether source references and self-built deployments also need replacement;
4. coordinate privately through the repository's maintainer/security route;
5. restore an asset only after a documented rights decision.

Removing an image must not remove or change the collectible ID or a member's collection state.
