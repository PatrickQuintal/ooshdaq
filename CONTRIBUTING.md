# Contributing to $OOSHDAQ

Thank you for helping make collectible exchanges simpler and safer. $OOSHDAQ
is intentionally small, privacy-conscious, and distributed as source.

Before a large change, open a proposal describing the user problem, the
narrowest useful solution, security and privacy effects, migration impact, and
any new dependency. Focused fixes and documentation improvements can usually
go straight to a pull request.

## Development and checks

Use the Go version declared in `go.mod` and the commands in the `Makefile`:

```sh
make run
make check
make test-web
(cd web && npm ci)
make test-browser
```

Run the narrowest relevant test first, then the broader lane. Browser changes
need the dependency-free contract tests and, when behavior changes, the live
browser journey. See the [testing strategy](docs/architecture/testing-strategy.md).

## Change expectations

1. Read [AGENTS.md](AGENTS.md) and the closest product or architecture document.
2. Add or identify a focused test for observable behavior.
3. Keep transport, use-case, domain, persistence, and presentation concerns separate.
4. Run `gofmt` on changed Go files and `go test ./...`.
5. Update documentation when behavior or an external boundary changes.
6. Explain risks, migration effects, verification, and follow-up work in the pull request.

Avoid unrelated refactors and compatibility scaffolding for unpublished
internal shapes. Never use real identities, credentials, private exports, or
private environment configuration in fixtures, screenshots, commits, issues,
or pull requests.

## Dependencies, content, and licensing

New dependencies require a concrete justification and the review in the
[supply-chain policy](docs/security/supply-chain.md). Do not hand-edit generated
files; commit reviewed inputs and deterministic outputs together.

Do not add copied or downloaded third-party artwork unless the project has the
right to redistribute it. Follow the [artwork policy](docs/catalog/artwork-provenance.md).

Report vulnerabilities privately according to [SECURITY.md](SECURITY.md).
All participants must follow the [Code of Conduct](CODE_OF_CONDUCT.md).

Contributions are licensed under Apache-2.0. Contributors must have the right
to submit every part of their contribution, and third-party material must
retain its own license and attribution.
