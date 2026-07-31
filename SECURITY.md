# Security Policy

$OOSHDAQ is designed for a trusted, authenticated community, but every browser request and forwarded header is still
untrusted input. The security model is described in [docs/security/threat-model.md](docs/security/threat-model.md).

## Supported Versions

$OOSHDAQ is under active construction and has no stable release yet. Security fixes are applied to the latest commit on the
default branch. Once releases exist, this section will list supported release lines.

## Reporting a Vulnerability

Please do not open a public issue or pull request containing exploit details.

Use the repository host's private vulnerability reporting feature when it is enabled. Otherwise, contact a repository
maintainer through a private channel and ask for a secure reporting route without including sensitive details in the first
message.

Include, where safe:

- affected version or commit
- configuration and authentication mode
- reproduction steps or a minimal proof of concept
- expected and observed impact
- known mitigations
- whether the issue is already public

Maintainers should acknowledge a complete report within three business days and aim to provide an initial assessment
within ten business days. These are response targets, not guarantees. We will coordinate disclosure and credit with the
reporter and may ask for additional time when downstream fixes are involved.

## Deployment Security Notes

- Mock authentication is for loopback/local development only and must fail closed in deployment mode.
- Proxy authentication is safe only when $OOSHDAQ is unreachable except through a trusted proxy that strips inbound
  identity headers before injecting its own.
- Use TLS at the ingress, a non-root container, a read-only root filesystem, least-privilege service accounts, restrictive
  network policy, and encrypted persistent storage.
- Keep secrets out of environment dumps, logs, images, repository history, and client-visible configuration.
- Back up and test restoration of the data store before upgrades.

Detailed operational guidance is in [docs/operations/kubernetes-gcp.md](docs/operations/kubernetes-gcp.md).

## Scope

Reports about $OOSHDAQ's code, default configuration, container, authentication boundary, persistence, or documented
deployment are in scope. Social engineering, denial-of-service testing against shared installations, and vulnerabilities
in unrelated third-party services should be reported to their respective owners.
