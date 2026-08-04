# $OOSHDAQ Documentation

Documentation describes enduring behavior, boundaries, and operating
procedures. Code and tests are authoritative for implementation detail.

Each topic has one primary home:

## Product

- [Vision and scope](product/vision.md)
- [Operating requirements](product/operating-requirements.md)
- [User journeys](product/user-journeys.md)

## Architecture

- [System overview](architecture/overview.md)
- [Data model](architecture/data-model.md)
- [HTTP API](architecture/http-api.md)
- [Authentication](architecture/authentication.md)
- [Testing strategy](architecture/testing-strategy.md)
- [ADRs](architecture/adr/)

## Operations

- [Local development](operations/local-development.md)
- [SQLite backup and restore](operations/sqlite-backup-restore.md)
- [Release checklist](operations/release-checklist.md)

## Security and catalog content

- [Threat model](security/threat-model.md)
- [Supply-chain policy](security/supply-chain.md)
- [Artwork and provenance](catalog/artwork-provenance.md)

## Documentation rules

- Link to the primary document instead of repeating it.
- Keep commands and runbooks executable; keep generated results out of prose.
- Record durable design rationale in ADRs, not completed delivery history.
- Update documentation with behavior, API, security, data, or deployment changes.
