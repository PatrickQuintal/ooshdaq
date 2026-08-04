# Threat Model

## Scope

This model covers the $OOSHDAQ browser UI, Go service, authentication boundary,
catalog manifests, SQLite repository, container, and documented operational
boundaries. The identity provider, authentication proxy, network edge, hosting
platform, operator registry, backup export, and out-of-band conversations are
external dependencies with their own controls.

Review this model when authentication, authorization, exposure, personal data,
catalog handling, dependencies, or storage topology changes. Capacity and
availability assumptions are owned by
[operating requirements](../product/operating-requirements.md).

## Protected Assets

- each member's collection state and private activity timestamps;
- stable identity-to-member mapping and the minimal counterpart identity shown
  in a match lead;
- session signing material and trusted-proxy configuration;
- database integrity, availability, backups, and recovery capability;
- catalog integrity, provenance, and rights decisions; and
- source, dependency, build, and deployment integrity.

The product intentionally stores no payments, messages, trade contracts,
locations, or member contact details.

## Actors and Trust Boundaries

Authenticated members may update only their own state and view their own match
leads. A malicious member may manipulate identifiers or inputs. Unauthenticated
and network clients may try to bypass authentication, spoof identity, replay
requests, or exhaust resources. Catalog sources and build inputs may be
malformed or compromised. Operators can recover and reconfigure the service,
and accidental operator error is in scope.

Every browser request is untrusted. Forwarded identity becomes trusted only at
an operator-provided proxy that authenticates the request, removes
client-supplied identity headers, and injects canonical values. Database records
and restored files are validated on read. Catalog publication is a reviewed
maintenance action; the serving process does not scrape source sites.

## Control Ownership

Repository controls include authentication adapters, bounded header parsing,
authorization-aware services, input validation, CSRF protection, browser
headers, SQLite transactions and migrations, verified backups, hardened
container defaults, and tests.

Deployment owners must provide TLS termination, an identity-aware proxy,
network isolation, external backup protection, monitoring, and any artifact
scanning or signing required locally. None of the network edge, proxy,
platform policy, secret scanning, or container vulnerability scanning is
supplied by the repository. A control below is not present until its stated
owner has configured and tested it.

## Threats and Controls

### Identity spoofing and authentication bypass

**Threat:** A client supplies a forwarded identity header or reaches the
application around the trusted proxy.

The application validates configured identity values, has no mock-login route
in proxy mode, and refuses unsafe mock listeners unless explicitly overridden.
The deployment owner must make the application reachable only through the
proxy, remove inbound identity headers there, inject one stable subject and
bounded display name, and test direct-header spoofing.

Compromise of the proxy or trusted network can still impersonate members.

### Cross-member access

**Threat:** A member changes an identifier to inspect or update another
member's collection.

Collection use cases derive ownership from the authenticated principal rather
than a client-supplied user ID. Decoders reject unexpected identity fields,
there is no arbitrary-member collection endpoint, and match responses expose
only the bounded counterpart projection needed to start a conversation.
Authorization tests use distinct members to detect scope regressions.

### Session theft and cross-site requests

**Threat:** An attacker steals or fixes a session, or causes a browser to
submit a mutation from another site.

Mock sessions are signed, expiring, `HttpOnly`, same-site cookies and are
replaced on login and expired on logout. Mutations require the CSRF cookie and
header token and reject cross-site Fetch Metadata when supplied. State changes
do not use `GET`; accepted request bodies require JSON. Secure cookies are
required behind TLS. Mock authentication remains local-development-only.

### Injection and browser execution

**Threat:** Catalog text, display names, paths, or errors become executable
HTML, script, or response content.

The browser uses text-safe DOM operations, request decoders validate identifiers
and JSON shape, and the product accepts no member-authored rich text. Response
headers include a restrictive content security policy, `nosniff`, frame
restrictions, and a conservative referrer policy. External errors are generic
and logs use safe error categories.

Remote artwork can reveal a member's IP and browser metadata to its host. Use
only reviewed hosts, prefer licensed local assets where possible, and revisit
the browser image policy whenever a source changes.

### Resource exhaustion and activity inference

**Threat:** Large inputs or repeated catalog and match queries consume memory,
CPU, disk, or SQLite write capacity; ranking leaks working patterns.

Repository controls bound body sizes, identifiers, quantities, page sizes,
cursors, and indexed offer/want queries. HTTP timeouts and cancellation limit
abandoned work. Query-plan and qualification tests protect the accepted access
shape. Deployment owners set edge rate limits, runtime resources, disk alerts,
and quotas appropriate to their community.

Only one private `last_seen_at` value supports stale-lead ranking. Authenticated
activity advances it no more than once per hour. Ranking uses a coarse stale
threshold and capped waiting priority; exact activity, scores, and ranking
timestamps are absent from member responses and logs. Broad activity may still
be inferred from otherwise tied ordering.

### Data corruption and loss

**Threat:** Abrupt termination, full disk, invalid migration, stale restore, or
volume deletion loses collection state.

The application requires one writer process with exclusive durable data
storage. WAL mode, full synchronous writes, foreign keys, bounded busy handling,
short transactions, migration serialization, and readiness checks protect the
database. The serving process creates verified, atomically published online
backups; restore validates an isolated file before replacing the live database
and retains the previous file. On Unix, directories created by the application
are owner-only and every generated database, sidecar, backup, and restore
artifact is restricted to the owning account.

Deployment owners must protect storage from accidental deletion, monitor disk
and backup age, export or snapshot completed backups, and rehearse the
[offline restore](../operations/sqlite-backup-restore.md). Do not use shared
multi-writer storage or let overlapping processes open the database. Brief
recovery downtime is accepted; committed collection loss is not.

### Catalog and artwork compromise

**Threat:** A catalog contains malicious metadata, changes remotely, tracks
members through images, or lacks redistribution rights.

The serving process loads bounded, reviewed embedded manifests and fetches no
artwork bytes. It validates catalog structure and HTTPS URL shape, escapes text,
and restricts browser image sources. Human review records provenance and rights
per catalog; uncertain artwork uses a placeholder. The repository does not
claim MIME inspection, malware scanning, decoding, or re-encoding of remote
images. Those controls are required if a future maintenance workflow downloads
asset bytes. See [artwork provenance](../catalog/artwork-provenance.md).

### Supply-chain and sensitive-output exposure

**Threat:** A compromised dependency, build environment, log, fixture, or
backup exposes or modifies member data.

The repository pins module checksums and builder inputs, prefers a small
dependency graph, runs a non-root scratch image, and keeps secrets out of its
build. Contributors run the checks listed in the
[supply-chain policy](supply-chain.md) locally. Operators control their built
artifact and add locally required image scanning, SBOM, signing, and deployment
records.

Logs must omit identity headers, cookies, request bodies, collection contents,
activity timestamps, and secrets. Restrict and encrypt log, backup, snapshot,
and restore access. Tests and screenshots use synthetic identities.

## Residual Risk and Review Triggers

The service depends on correct proxy, network, and host administration, member
use of an approved external contact channel, and exactly one writable SQLite
process. It is not a highly available or anonymous public marketplace.

$OOSHDAQ is not directed at children and must not store child profiles, photos,
locations, or contact details. Public access, messaging, moderation, additional
personal data, multiple writers, a new authentication mode, or a new storage
topology requires a fresh privacy/security review and, where architectural, a
new ADR.
