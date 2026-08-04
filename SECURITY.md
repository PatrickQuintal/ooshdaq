# Security Policy

$OOSHDAQ serves trusted communities but treats every browser request and forwarded
header as untrusted input. See the [threat model](docs/security/threat-model.md).

## Supported code

Security fixes target the latest reviewed commit on the default branch. $OOSHDAQ
is source-distributed, so operators should pin, build, and update reviewed commits.

## Reporting a vulnerability

Do not open a public issue or pull request containing exploit details. Use the repository
host's private reporting feature when available. Otherwise, privately ask a maintainer
for a secure route without including sensitive details in the first message.

Include, where safe:

- the affected commit and configuration;
- reproduction steps or a minimal proof of concept;
- expected and observed impact;
- known mitigations and whether the issue is already public.

Maintainers should acknowledge a complete report within three business days and aim for an
initial assessment within ten business days. Disclosure and credit are coordinated with the reporter.

## Deployment boundary

Mock authentication is limited to explicitly allowed local use. Proxy authentication
requires an identity-aware proxy that strips inbound identity headers, prevents direct
access to $OOSHDAQ, and supplies trusted replacements. Keep secrets out of logs, images,
repository history, and client-visible configuration. Follow the
[authentication boundary](docs/architecture/authentication.md) and protect
[database backups](docs/operations/sqlite-backup-restore.md).

## Scope

$OOSHDAQ code, default configuration, authentication, persistence, container, and documented
deployment are in scope. Do not test denial of service against shared installations or report unrelated third-party issues here.
