# Threat Model

## Scope

This model covers the $OOSHDAQ browser UI, Go HTTP service, authentication boundary, catalog, repository, container, and
documented Kubernetes deployment. The external identity provider, authentication proxy, Kubernetes control plane, cloud
provider, and out-of-band conversations are dependencies with their own security controls.

Review this model whenever authentication, authorization, public exposure, personal data, catalog ingestion, dependencies,
or storage topology changes.

The accepted operating envelope is approximately 10,000 eligible identities, fewer than 100 normally concurrent members,
and at least five finite catalogs from multiple brands. Members can exchange items across catalog boundaries. These are
capacity and authorization boundaries, not assumptions that every eligible identity is active at once. See
[operating requirements](../product/operating-requirements.md).

## Protected Assets

- integrity and confidentiality of each member's collection state;
- confidentiality of private last-seen and state-transition timestamps;
- stable identity-to-user mapping;
- the minimal counterpart identity exposed by an availability lead;
- session signing material and proxy configuration;
- database availability and recoverability;
- catalog integrity, provenance, and rights records;
- application and container build integrity;
- availability of the private community service.

There are no payment credentials, chat messages, trade contracts, or child profiles by design.

## Actors

- **Authenticated member:** may manage their own collection and view their own possible-swap leads.
- **Unauthenticated client:** may reach health checks and, depending on deployment policy, the static sign-in shell; has no
  collection access.
- **Malicious or curious member:** has a valid identity but tries to inspect or change another member's data.
- **Network attacker:** tries to bypass the auth proxy, spoof identity headers, replay requests, or observe traffic.
- **Catalog contributor/source:** may accidentally or deliberately supply malicious metadata, URLs, or files.
- **Dependency/build compromise:** introduces malicious code through modules, base images, CI actions, or tooling.
- **Operator:** can configure, back up, restore, and upgrade the service; accidental misconfiguration is in scope.

## Trust Boundaries

### Browser to ingress

Every path, header, cookie, and body is untrusted. TLS terminates at an approved boundary. Browser output encoding and a
restrictive content security policy are required even for catalog data.

### Auth proxy to $OOSHDAQ

Forwarded identity becomes trusted only after the proxy has authenticated the request, stripped client-provided identity
headers, and injected canonical values. $OOSHDAQ must be unreachable through any route that bypasses this proxy.

### Service to repository

Services pass validated domain values through repository interfaces. Encoded records are still untrusted on read because
files can be corrupt, stale, or restored from an incompatible version.

### Catalog input to published catalog

Remote pages and downloaded files are untrusted. Fetching, review, provenance, rights approval, transformation, and
publication are separate steps. The serving process performs no opportunistic scraping.

### Build inputs to runtime image

Source, Go modules, CI actions, compiler/base images, and catalog artifacts cross a supply-chain boundary. Pinning,
review, scanning, and reproducibility reduce but do not eliminate this risk.

## Threats and Controls

### Identity header spoofing

**Threat:** A client sends `X-Forwarded-User-ID` directly or reaches the Service/pod without the auth proxy.

**Controls:**

- ClusterIP-only service and ingress routing exclusively through the auth proxy;
- network policy allowing application ingress only from the proxy namespace/service account or selected pods;
- proxy strips configured headers before injection;
- $OOSHDAQ requires bounded, unambiguous subject and display-name values;
- no mock auth route in proxy mode;
- deployment smoke test attempts a direct spoof and expects failure.

**Residual risk:** A proxy or cluster-network compromise can impersonate users. Stronger cryptographic assertion validation
would be a separate auth adapter.

### Cross-member access

**Threat:** A member changes an ID in a path/body to read or update another collection.

**Controls:**

- collection routes contain no user selector;
- principal from authentication selects repository scope;
- request decoders reject unknown user fields;
- no arbitrary-user collection endpoint;
- match service returns only the minimal availability-lead projection;
- match projections and cursors expose neither exact activity nor ranking
  timestamps;
- authorization regression tests use two members.

### Session theft, fixation, and CSRF

**Threat:** An attacker steals or forces a mock session, or causes a logged-in browser to mutate collection state.

**Controls:**

- signed, expiring, `HttpOnly`, same-site cookie;
- fresh cookie on login and expired cookie on logout;
- exact double-submit CSRF cookie/header token and cross-site Fetch Metadata rejection when present;
- JSON content type where a body is accepted and no state-changing `GET`;
- `Secure` cookie when served with TLS;
- session values and secrets never logged.

Mock sessions are explicitly not suitable for shared deployment.

### Injection and browser script execution

**Threat:** Catalog text, display names, paths, or error details inject HTML/JavaScript or HTTP response content.

**Controls:**

- DOM text APIs or template escaping, never string-built HTML from data;
- strict JSON decoding and identifier validation;
- no user-authored rich text;
- content security policy with no remote script;
- `X-Content-Type-Options: nosniff`, frame restrictions, and a conservative referrer policy;
- generic external errors and structured redacted logs.

Remote images remain a privacy risk because their host can observe requests. Prefer reviewed local licensed assets or a
first-party image proxy with its own controls.

### Quantity abuse and resource exhaustion

**Threat:** Huge bodies, quantities, catalogs, or repeated match requests exhaust memory, CPU, disk, or write locks.

**Controls:**

- small per-route body limit;
- bounded integer quantities and identifier lengths;
- hard result limits, stable ordering, cancellation, and pagination where a response can grow;
- synthetic load tests with approximately 10,000 users, five or more catalogs across multiple brands, and fewer than 100
  concurrent members;
- separately bounded indexed non-zero offer and want lookups for actionable
  and member-opened unfulfillable feeds;
- capped waiting priority and coarse stale classification rather than an
  unbounded recency score;
- HTTP read/write/idle timeouts and graceful cancellation;
- ingress request and rate limits appropriate to the community;
- resource requests/limits, storage alerts, and disk quotas.

The matcher uses the bounded, indexed storage shape selected in
[ADR 0002](../architecture/adr/0002-sqlite-single-replica-multi-catalog.md)
and the availability-lead semantics in
[ADR 0004](../architecture/adr/0004-simple-states-and-availability-leads.md)
and [ADR 0005](../architecture/adr/0005-ranked-split-match-feeds.md).
Query-plan tests reject collection-table scans. Do not respond to future
pressure by adding an unmeasured cache or raising limits.

### Activity inference

**Threat:** Exact last-login data reveals working patterns or becomes a
member-visible popularity score.

**Controls:**

- store only one private `last_seen_at` value required for stale-lead ranking;
- advance it through an authenticated, CSRF-protected activity mutation and
  rate-limit durable writes to at most once per hour;
- use only a 90-day stale threshold, never exact recency, login counts, or
  presence;
- encrypt and authenticate cursors so their frozen ranking time and keyset are
  not client-readable, and omit activity/ranking timestamps from JSON, logs,
  UI copy, and counterpart profiles;
- keep display ordering explainable as waiting fairness rather than behavioral
  profiling.

**Residual risk:** An observer could infer broad activity from ordering among
otherwise tied capped-age leads. The coarse threshold limits but cannot
eliminate that inference.

### Data corruption, rollback, and loss

**Threat:** Abrupt termination, full disk, invalid migration, stale restore, or accidental volume deletion loses collection
state.

**Controls:**

- one SQLite writer pod uses one single-writer persistent block volume;
- use `ReadWriteOncePod` when supported, otherwise `ReadWriteOnce` with one replica and a non-overlapping replacement
  strategy; never place the writable SQLite file on NFS or another shared multi-writer filesystem;
- enable and verify SQLite WAL mode, full synchronous writes, foreign keys, a bounded busy timeout, short transactions,
  and an explicit checkpoint strategy;
- readiness fails until schema migrations complete and a real database read succeeds;
- graceful shutdown and adequate termination grace period;
- SQLite-aware online backups or `VACUUM INTO`, integrity checks, defined retention, monitored backup age/failure, and
  documented restoration drills;
- versioned, encrypted volume snapshots as supplementary recovery material rather than the only backup;
- clean-reset cutover, SQLite schema-migration failure, abrupt-termination recovery, pod replacement, and node-roll tests;
- globally unique item and catalog IDs never reused.

Brief downtime while the single pod restarts or its volume reattaches is accepted. The one-time clean reset removed only
disposable prototype state; committed SQLite collection data loss is not acceptable. SQLite is not a highly available
database here, and the baseline does not include PostgreSQL, Spanner, active-active replicas, or automatic database
failover.

### Catalog and artwork compromise

**Threat:** A source changes content, provides a malicious file, tracks members through remote images, or lacks
redistribution rights.

**Controls:**

- no request-path fetching;
- scheme/host allowlist during explicit ingestion;
- byte, MIME, dimension, and decode limits;
- checksums and retrieval timestamps;
- human-reviewed provenance and rights status;
- an independent provenance and rights decision for every catalog and brand;
- placeholders for unknown/restricted rights;
- sanitize or re-encode permitted raster assets; never serve source SVG/HTML as trusted artwork;
- browser policy limits image sources.

### Supply-chain compromise

**Threat:** A compromised Go module, CI action, base image, or developer tool changes the application.

**Controls:**

- small direct dependency set and standard-library preference;
- committed `go.sum`, minimal-version selection, and reviewed upgrades;
- pinned CI actions and builder inputs, plus operator-controlled deployment
  artifact identity;
- vulnerability, license, secret, and container scanning;
- non-root scratch runtime with no shell or package manager;
- reproducible source inputs and optional operator-generated SBOM/build
  evidence.

See [supply-chain policy](supply-chain.md).

### Sensitive logging and backup exposure

**Threat:** Logs, diagnostics, test fixtures, or backups expose identities and collections.

**Controls:**

- do not log identity headers, cookies, bodies, collection contents, or secrets;
- log only method, route path, status, duration, and safe error type; add generated correlation IDs before cross-system
  tracing requires them;
- restrict log and backup access and retention;
- encrypt disks, snapshots, and transport;
- use synthetic fixtures and screenshots;
- verify restored data only in an access-controlled environment.

## Security Assumptions

- the deployment is private and authenticated, not internet-anonymous;
- the auth proxy and cluster network are correctly administered;
- members use an approved external channel to contact a displayed counterpart;
- the eligible population is approximately 10,000 identities, normal concurrency remains below 100 members, and catalogs
  remain finite while supporting at least five catalogs across multiple brands;
- operators protect Kubernetes, any registry they choose to use, and backup
  administration;
- SQLite is mounted read-write by exactly one $OOSHDAQ pod and restored from tested backups when the
  persistent volume alone is insufficient.

An assumption becoming false is a reason to revisit architecture, not merely configuration.

## Abuse and Safety Boundaries

$OOSHDAQ is not directed at children and should not store child identities, photos, locations, or contact information.
Deployment owners are responsible for community admission and out-of-band conduct. A future public deployment would need
abuse reporting, moderation, data retention/deletion policy, and a new privacy assessment before launch.
