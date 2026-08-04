# AGENTS.md

Guidance for people and coding agents working in this repository.

## Stance

$OOSHDAQ is a small, privacy-conscious exchange matcher for collectible
communities. Keep it easy to run, easy to audit, and deliberately boring.

Build to the documented [operating requirements](docs/product/operating-requirements.md).
Do not add distributed infrastructure without measurements and a new ADR.

$OOSHDAQ is distributed as source. Do not add project-owned binary or container
publication, registry, or signing requirements without an explicit decision.

Prefer:

- plain Go domain rules and thin HTTP adapters
- repository interfaces between use cases and storage
- server-rendered or vanilla browser code before a frontend toolchain
- explicit input bounds, validation, authorization, and deterministic ordering
- concise documentation that links to one authoritative source
- the standard library or a small, justified direct dependency set

Avoid:

- messaging, payments, auctions, or marketplace feature creep
- client-supplied identity or deployment auth shortcuts
- persistence details in HTTP or domain types
- personal/contact data without a product requirement
- third-party artwork without recorded redistribution rights

## Start here

- [Product and quick start](README.md)
- [Documentation map](docs/README.md)
- [Product vision](docs/product/vision.md)
- [Architecture](docs/architecture/overview.md)
- [Data model](docs/architecture/data-model.md)
- [HTTP API](docs/architecture/http-api.md)
- [Authentication](docs/architecture/authentication.md)
- [Testing](docs/architecture/testing-strategy.md)
- [Release checklist](docs/operations/release-checklist.md)
- [Local operations](docs/operations/local-development.md)
- [Backup and restore](docs/operations/sqlite-backup-restore.md)
- [Threat model](docs/security/threat-model.md)
- [Artwork policy](docs/catalog/artwork-provenance.md)

## Module boundaries

- Composition roots parse configuration, wire dependencies, and manage shutdown.
- Domain packages own catalog and collection-state invariants.
- Application services own use cases and authorization-aware projections.
- HTTP packages decode, validate, call a service, and encode a response.
- Repository adapters own transactions, serialization, schema, and migrations.
- Query repositories own indexed matching, ranking, and bounded feeds.
- Browser modules own presentation, not security or matching policy.
- Catalog manifests are validated input data, not executable code.

If a file approaches 300 lines, check that it still has one responsibility.
Split mixed transport, service, persistence, and presentation concerns.

## Change rules

- Update the primary document with behavior, API, security, data, or deployment changes.
- Preserve the local/deployment authentication separation; new modes need an ADR and security review.
- Validate quantities, identifiers, body sizes, and list bounds server-side.
- Keep logs structured and free of secrets, identity headers, bodies, and collection contents.
- Regenerate generated files from reviewed inputs; never hand-edit them.
- Treat catalog names, descriptions, and images as licensed content with provenance.
- Justify dependencies; do not add one for a clearer local helper.

## Testing

Test behavior at the narrowest useful boundary, then run relevant wider checks:

- domain tests for collection invariants
- service tests for authorization and projections
- repository tests for transactions, matching, durability, and migrations
- HTTP tests for routing, validation, headers, and auth isolation
- browser tests for login, collection editing, and match discovery
- container and qualification tests for persistence, recovery, and bounded scale

Run `gofmt` on changed Go files and `go test ./...` before handoff. See the
[testing strategy](docs/architecture/testing-strategy.md) for broader gates.

## Dependencies

Before adding or upgrading a package, document its job and review maintenance,
license, releases, transitive dependencies, and known vulnerabilities. Keep
reproducible inputs and checksums committed. See the
[supply-chain policy](docs/security/supply-chain.md).
