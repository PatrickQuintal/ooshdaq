# Contributing to $OOSHDAQ

Thank you for helping make collectible exchanges simpler and safer.

$OOSHDAQ is intentionally small. Before a large change, open a proposal describing the user problem, the narrowest useful
solution, security and privacy effects, and any new dependency or migration. Bug fixes and focused documentation
improvements can usually go straight to a pull request.

## Development Setup

Use a supported Go version from `go.mod`.

```sh
go run ./cmd/ooshdaq
go test ./...
```

The local service uses mock users only. Never use real identity data, credentials, production exports, or copied private
environment configuration in fixtures, screenshots, commit messages, issues, or pull requests. See
[local development](docs/operations/local-development.md).

## Making a Change

1. Read [AGENTS.md](AGENTS.md) and the relevant architecture or product document.
2. Add or identify a focused test that expresses the behavior.
3. Keep transport, use-case, domain, persistence, and presentation responsibilities separate.
4. Run `gofmt` on changed Go files and `go test ./...`.
5. Update documentation when an API, data shape, security boundary, deployment assumption, or user journey changes.
6. Explain risks, migration impact, verification, and any follow-up in the pull request.

Do not combine an unrelated refactor with a behavior change. Do not add compatibility scaffolding for an unpublished
internal shape; before the first stable release, prefer one clean transition.

## Testing Expectations

Tests should exercise observable behavior, not implementation trivia. Choose the smallest meaningful layer, then run the
relevant broader lane. Repository work should cover transactionality and restart behavior; authentication work should
cover header spoofing and mode isolation; matching work should cover both sides of a reciprocal exchange.

Full guidance is in [docs/architecture/testing-strategy.md](docs/architecture/testing-strategy.md).

## Dependencies and Generated Content

New dependencies require a concrete justification and the review described in
[docs/security/supply-chain.md](docs/security/supply-chain.md). Prefer the Go standard library and small local code when
that is easier to audit.

Do not hand-edit generated files. Commit the reviewed source input, generator version, and deterministic result together.
Do not add scraped or downloaded artwork without a provenance record and confirmed rights; see
[docs/catalog/artwork-provenance.md](docs/catalog/artwork-provenance.md).

## Security and Privacy

Report vulnerabilities privately according to [SECURITY.md](SECURITY.md), not in a public issue. Minimize personal data.
Logs and test fixtures must not contain authentication tokens, forwarded identity values, collection exports, private
contact details, or secrets.

All participants must follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Licensing

By contributing, you agree that your contribution is licensed under Apache-2.0 and that you have the right to submit it.
Third-party material must retain its original license and attribution and must not be represented as Apache-2.0 content.
