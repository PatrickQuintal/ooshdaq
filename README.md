# $OOSHDAQ

$OOSHDAQ helps a trusted community compare collectible collections and find
people with a wanted item available. A member marks what they have, what they
can offer, and what they want. A return item is optional; when both people have
something the other wants, the stronger reciprocal lead may cross catalog and
brand boundaries. Conversation and the physical exchange happen outside the
application.

The name is a playful nod to an exchange board. The project begins with Disney Ooshies, but the model is intended to support
other finite collectible catalogs.

> $OOSHDAQ is under active construction. The initial local application and its interfaces may change before the first
> tagged release.

## Product Boundaries

$OOSHDAQ does:

- show a catalog of collectibles with reviewed artwork metadata
- let an authenticated member press the artwork to toggle ownership, mark a
  copy available, or keep looking for more of a design they already own
- group matches by wanted item, list members who have it available, and plainly
  distinguish a fulfilable request, an unfulfillable request, and someone who
  genuinely has not asked for anything back; the unfulfillable list is checked
  only when opened
- identify the other member by a deployment-approved display identity

$OOSHDAQ does not provide:

- chat, notifications, payments, auctions, shipping, or trade settlement
- a public user directory
- assurance that a physical item is genuine or that an exchange was completed
- permission to redistribute third-party catalog artwork

The service is designed for an authenticated private community, not as a public marketplace or a service directed at
children.

The accepted operating envelope is approximately 10,000 eligible staff, fewer than 100 concurrent members, at least five
finite multi-brand catalogs, and acceptable downtime during restarts and node rolls. See
[operating requirements](docs/product/operating-requirements.md).

## Local Quick Start

Prerequisites:

- a currently supported Go toolchain (see `go.mod`)
- a writable directory for the embedded database

Run the application:

```sh
make run
```

Then open <http://localhost:8080>. (`go run ./cmd/ooshdaq` is the equivalent
direct command.) Local development uses the explicit mock-login flow and
seeded example users. It does not emulate a real identity provider.

Run the test suite:

```sh
go test ./...
```

Configuration and reset instructions are in
[docs/operations/local-development.md](docs/operations/local-development.md). The current HTTP contract is documented in
[docs/architecture/http-api.md](docs/architecture/http-api.md).

## Source Distribution

$OOSHDAQ is distributed as source code. The project does not publish an official
binary or container image and does not require users to consume a project-owned
registry artifact or image signature.

Build the standalone binary:

```sh
make build
./bin/ooshdaq
```

Or build the local container:

```sh
make docker-build
```

Self-hosting operators own the resulting artifact and may load
`ooshdaq:local` directly into a cluster or retag it into their own registry.
Scanning, SBOM generation, signing, and digest pinning are optional
operator-policy choices, not project publication steps.

## Deployment Shape

The accepted production target is a single, non-root container behind an authentication reverse proxy, with one SQLite
database on a persistent block volume:

```text
browser -> IdP/auth proxy -> one $OOSHDAQ pod -> SQLite -> persistent volume
```

The proxy authenticates the member, removes any inbound identity headers, and injects trusted identity headers. The
application must not be directly reachable by clients in this mode. Kubernetes may briefly interrupt service while it
replaces the pod or reattaches the volume; that downtime is accepted, while collection durability, backups, and tested
restoration are required.

The current runtime is one Go process with an embedded vanilla UI and one
SQLite database. It uses separately bounded, SQLite-indexed actionable and
unfulfillable match queries, capped waiting-age ordering, and a coarse private
stale-record signal. It is configured for exactly one writable Kubernetes pod. The
repository manifests are the accepted baseline, not
evidence of a target-cluster rollout. The disposable pre-release bbolt
prototype was removed through the approved clean reset; there is no store selector, legacy reader, or rollback to bbolt.
See [ADR 0002](docs/architecture/adr/0002-sqlite-single-replica-multi-catalog.md) for the architecture and the
[implementation tracker](docs/implementation/sqlite-multi-catalog-uplift.md) for its qualification evidence. The
[source-readiness handoff](docs/implementation/release-readiness-report.md) separates completed source/build gates from
the target-environment checks that remain for each self-hosting operator.

See [authentication](docs/architecture/authentication.md), [threat model](docs/security/threat-model.md), and
[Kubernetes/GCP direction](docs/operations/kubernetes-gcp.md).

## Catalog Content

Application code is Apache-2.0 licensed. Third-party collectible names, characters, product images, and descriptions may
have different owners and are not automatically covered by that license. $OOSHDAQ records source and rights metadata and
uses reviewed source-hosted links for the current 40-character catalog, with local placeholders when those links are
unavailable. It does not package the third-party image bytes. See
[docs/catalog/artwork-provenance.md](docs/catalog/artwork-provenance.md).

## Project Documentation

[docs/README.md](docs/README.md) is the documentation map. Contributions are welcome; read
[CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) first.

## License

Source code and original documentation are licensed under the [Apache License 2.0](LICENSE). Third-party catalog content is
excluded unless its own provenance record explicitly says otherwise.
