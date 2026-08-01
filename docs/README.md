# $OOSHDAQ Documentation

This directory describes product intent, current interfaces, security boundaries, and the intended deployment path. Code
and tests are authoritative when a document accidentally drifts; fixing the document is part of fixing the drift.

## Product

- [Vision and boundaries](product/vision.md)
- [Operating requirements](product/operating-requirements.md)
- [User journeys](product/user-journeys.md)

## Architecture

- [System overview](architecture/overview.md)
- [Data model and invariants](architecture/data-model.md)
- [HTTP API](architecture/http-api.md)
- [Authentication boundary](architecture/authentication.md)
- [Testing strategy](architecture/testing-strategy.md)
- [ADR 0001: Go, vanilla UI, bbolt, and a repository boundary (persistence superseded)](architecture/adr/0001-go-vanilla-ui-bbolt-repository-boundary.md)
- [ADR 0002: SQLite, one recoverable pod, and multi-catalog matching](architecture/adr/0002-sqlite-single-replica-multi-catalog.md)
- [ADR 0003: Clean reset, modernc SQLite, and a bounded API](architecture/adr/0003-clean-reset-modernc-and-bounded-api.md)
- [ADR 0004: Simple collection states and availability leads](architecture/adr/0004-simple-states-and-availability-leads.md)
- [ADR 0005: Ranked and split match feeds](architecture/adr/0005-ranked-split-match-feeds.md)

## Implementation

- [SQLite and multi-catalog uplift plan](implementation/sqlite-multi-catalog-uplift.md)
- [Implemented bounded API contract](implementation/api-contract-v1.md)
- [Live browser journey report](implementation/browser-journey-report.md)
- [Container persistence report](implementation/container-persistence-report.md)
- [10,000-user envelope qualification](implementation/qualification-report.md)
- [Source readiness and self-hosting handoff](implementation/release-readiness-report.md)

## Security

- [Threat model](security/threat-model.md)
- [Supply-chain policy](security/supply-chain.md)
- [modernc SQLite dependency review](security/modernc-sqlite-review.md)

## Operations

- [Local development](operations/local-development.md)
- [Kubernetes and GCP deployment direction](operations/kubernetes-gcp.md)
- [SQLite backup, reset, and offline restore runbook](operations/sqlite-backup-restore.md)

## Catalog

- [Artwork, licensing, and provenance](catalog/artwork-provenance.md)
- [Disney Ooshies 2026 artwork manifest](catalog/disney-ooshies-2026-artwork.json)

## Documentation Conventions

- **Current** describes behavior implemented by the repository source.
- **Target** or **Direction** describes an accepted boundary or next adapter, not a delivered capability. SQLite is the
  current repository. References to bbolt appear only as superseded decision history or the one-time clean-reset record;
  there is no migration, compatibility, or rollback path.
- **Distribution** is source-only. The project does not publish or sign an
  official binary or container image; operators build and deploy their own
  artifact.
- **Decision** is captured in an ADR and should not be changed accidentally.

New documents should link from this page and from the closest existing topic. Do not repeat secrets, private environment
details, real identity data, or copied third-party content in documentation or examples.
