# AGENTS.md

Guidance for people and coding agents working in this repository.

## Stance

$OOSHDAQ is a small, privacy-conscious exchange matcher for collectible communities. Keep it easy to run, easy to audit,
and deliberately boring.

$OOSHDAQ is source-distributed. Do not add a project-owned container registry,
binary release pipeline, image publication, or image-signing requirement
without an explicit new decision. Users build the binary or container
themselves; deployment-specific artifact controls belong to the operator.

Design to the accepted operating envelope: approximately 10,000 eligible staff, fewer than 100 concurrent members, several
finite catalogs, acceptable restart downtime, and durable single-pod storage. Do not introduce distributed infrastructure
without a measured requirement and a new ADR.

Prefer:

- domain rules in plain Go
- HTTP handlers as transport adapters, not business logic
- repository interfaces between use cases and storage
- server-rendered or vanilla browser code before adding a frontend toolchain
- explicit bounds, validation, and authorization at every input boundary
- concise documentation with links to the authoritative detail
- standard-library code or a small, justified direct dependency set

Avoid:

- messaging, payments, auctions, or marketplace feature creep
- trusting client-supplied user identity
- deployment-only authentication shortcuts in local development, or local mock authentication in deployment
- persistence details leaking into HTTP or domain types
- storing contact details or other personal data without a product requirement
- copying third-party artwork into this Apache-licensed repository without redistribution rights

## Breadcrumbs

Start here:

- product and quick start: [README.md](README.md)
- documentation map: [docs/README.md](docs/README.md)
- product intent: [docs/product/vision.md](docs/product/vision.md)
- operating envelope: [docs/product/operating-requirements.md](docs/product/operating-requirements.md)
- implementation plan: [docs/implementation/sqlite-multi-catalog-uplift.md](docs/implementation/sqlite-multi-catalog-uplift.md)
- source/self-hosting handoff: [docs/implementation/release-readiness-report.md](docs/implementation/release-readiness-report.md)
- user journeys: [docs/product/user-journeys.md](docs/product/user-journeys.md)
- architecture: [docs/architecture/overview.md](docs/architecture/overview.md)
- data model: [docs/architecture/data-model.md](docs/architecture/data-model.md)
- HTTP API: [docs/architecture/http-api.md](docs/architecture/http-api.md)
- authentication boundary: [docs/architecture/authentication.md](docs/architecture/authentication.md)
- testing: [docs/architecture/testing-strategy.md](docs/architecture/testing-strategy.md)
- threat model: [docs/security/threat-model.md](docs/security/threat-model.md)
- supply chain: [docs/security/supply-chain.md](docs/security/supply-chain.md)
- local operations: [docs/operations/local-development.md](docs/operations/local-development.md)
- Kubernetes and GCP direction: [docs/operations/kubernetes-gcp.md](docs/operations/kubernetes-gcp.md)
- artwork policy: [docs/catalog/artwork-provenance.md](docs/catalog/artwork-provenance.md)
- foundational ADR: [docs/architecture/adr/0001-go-vanilla-ui-bbolt-repository-boundary.md](docs/architecture/adr/0001-go-vanilla-ui-bbolt-repository-boundary.md)
- persistence target: [docs/architecture/adr/0002-sqlite-single-replica-multi-catalog.md](docs/architecture/adr/0002-sqlite-single-replica-multi-catalog.md)
- implementation decisions: [docs/architecture/adr/0003-clean-reset-modernc-and-bounded-api.md](docs/architecture/adr/0003-clean-reset-modernc-and-bounded-api.md)
- simplified states and availability leads: [docs/architecture/adr/0004-simple-states-and-availability-leads.md](docs/architecture/adr/0004-simple-states-and-availability-leads.md)
- ranked and split match feeds: [docs/architecture/adr/0005-ranked-split-match-feeds.md](docs/architecture/adr/0005-ranked-split-match-feeds.md)

## Module Rules

- Composition roots parse configuration, wire dependencies, and manage shutdown.
- Domain packages own collectible, canonical collection-state, availability,
  wanted, and possible-swap rules.
- Application services own use cases and authorization-aware projections.
- HTTP packages decode, validate, call a service, and encode a response.
- Repository implementations own keys, transactions, serialization, and schema migration.
- Query-specific repository operations own indexed availability-lead
  discovery, capped-wait ranking, and the separate actionable and
  unfulfillable feeds; do not rebuild an all-user, all-item scan in a service
  or browser.
- Browser modules own presentation and interaction; they must not reconstruct security or matching policy.
- Static catalog seed data is input data, not executable code. Validate it before import.

If a file approaches 300 lines, check whether it still has one responsibility. Split mixed transport, service, persistence,
and presentation concerns before they become difficult to test.

## Change Rules

- Update the relevant document in the same change when behavior, API shapes, authentication, data ownership, or deployment
  assumptions change.
- Preserve the local/deployment authentication separation. A new auth mode needs a security review and an ADR.
- Validate quantities, identifiers, body sizes, and list bounds server-side.
- Keep logs structured and free of tokens, forwarded identity headers, request bodies, and collection contents.
- Do not hand-edit generated files. Regenerate them from reviewed source inputs.
- Treat catalog images and descriptions as licensed content with recorded provenance.
- Do not add a dependency for a convenience function that is clearer to implement locally.

## Testing Rules

Behavior changes need tests at the narrowest useful boundary, followed by the relevant wider checks:

- domain tests for collection-state and possible-swap invariants
- service tests for authorization and projections
- repository tests for transactions, restart durability, and migration
- scale tests with synthetic multi-catalog data for the accepted 10,000-user and 100-concurrent-member envelope
- HTTP tests for routing, validation, status codes, headers, and auth isolation
- browser journey tests for login, collection editing, and match discovery
- container smoke tests for startup, probes, persistence, and graceful shutdown

Run `gofmt` on changed Go files and `go test ./...` before handing work off. Use
[docs/architecture/testing-strategy.md](docs/architecture/testing-strategy.md) for broader gates.

## Dependency Rules

Before adding or upgrading a package, record the job it performs and check its maintenance, license, release history,
transitive graph, and known vulnerabilities. Pin reproducible inputs and keep checksums committed. See
[docs/security/supply-chain.md](docs/security/supply-chain.md).
