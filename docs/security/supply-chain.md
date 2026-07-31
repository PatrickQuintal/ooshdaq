# Supply-Chain Policy

$OOSHDAQ favors a small, inspectable build over a large convenience dependency graph. “Latest” means a currently supported,
reviewed release—not an automatic upgrade to unexamined code.

## Dependency Admission

Before adding a direct dependency, record in the pull request:

1. the concrete job it performs;
2. why the standard library or a small local implementation is insufficient;
3. maintenance activity, release cadence, ownership, and security reporting route;
4. license compatibility with Apache-2.0;
5. transitive dependency count and any native/runtime components;
6. known vulnerabilities and the result of a source/API review;
7. how it will be pinned, tested, upgraded, and removed.

Reject packages that are abandoned for a security-sensitive job, obscure ownership, execute install-time code without a
need, pull a disproportionate graph for a trivial helper, or use an incompatible license.

## Current Dependency Stance

- Go's standard library provides HTTP, JSON, embedding, cryptography primitives, and testing.
- `modernc.org/sqlite` v1.55.0 and its required matching `modernc.org/libc` v1.74.1 pin are the current SQLite driver
  pair. The production composition, migrations, backup path, and recovery tests all use it. See
  [ADR 0003](../architecture/adr/0003-clean-reset-modernc-and-bounded-api.md) and the
  [driver review](modernc-sqlite-review.md).
- `LICENSES/` records the selected driver's immediate module requirements and linked embedded-code notices. The Docker
  build copies that inventory into the runtime image. The removed prototype store and its notices are absent.
- Go 1.26.5 is the current build baseline because it contains security fixes after Go 1.26.2. The toolchain, builder tag,
  and builder digest remain a reviewed unit.
- The browser UI has no runtime package manager or third-party script
  dependency. `@playwright/test` 1.62.0 is pinned in `web/package-lock.json`
  solely for development and CI browser journeys; it is not embedded or copied
  into the runtime image.
- The runtime image is `scratch`; build-only images are pinned by tag and digest.

If this list drifts from `go.mod`, the compiled module graph, `LICENSES/`, or the shipped browser assets, update it in the
same change.

## Go Module Controls

- Commit `go.mod` and `go.sum`.
- Use the Go version declared in `go.mod`; keep it on a supported patch release.
- Run `go mod tidy` intentionally and review every graph/checksum change.
- Inspect `go mod graph` and `go list -m -json all` for new transitive modules.
- Run `go vet ./...`, `go test ./...`, `go test -race ./...` where applicable, and a current Go vulnerability scan.
- Do not use `replace` to an unreviewed fork or local path in committed release state.
- Do not vendor by default. If an environment requires vendoring, generate it deterministically and scan/review the
  vendored sources.
- Treat checksum-database or proxy bypass configuration as a documented exceptional decision.

Checksums prove content identity, not trustworthiness.

## Selected SQLite Driver

The accepted pins are:

```text
modernc.org/sqlite v1.55.0
modernc.org/libc   v1.74.1
```

They provide a CGO-free SQLite 3.53.3 build and preserve the static Linux scratch-image design. The exact runtime source
ID, Go module checksums, transitive graph, licenses, isolated spike results, and remaining implementation gates are in the
[modernc SQLite dependency review](modernc-sqlite-review.md).

The adapter classifies busy, disk-full, read-only, corrupt, not-a-database, migration, checkpoint, and foreign-key
failures; configures and verifies every pooled connection; and proves online backup, isolated restore, abrupt recovery,
and transactional migration rollback. Linux amd64/arm64 builds, the fixed
release qualification, and the executed scratch-image persistence smoke are
recorded in the implementation reports. The project publishes source rather
than an image. A self-hosting operator may scan or generate an SBOM for their
own build when local policy requires it.

Do not add PostgreSQL or Spanner clients as speculative fallbacks; they are not part of the accepted baseline. Do not
reintroduce the removed prototype store, a retained reader, dual-store mode, or a data-preservation branch.

## Container Controls

- Pin builder images by immutable digest while retaining the human-readable tag.
- Use a multi-stage build and copy only the static binary and explicitly reviewed assets into the runtime.
- Run as a numeric non-root UID with no capabilities, no privilege escalation, and a read-only root filesystem.
- Include no shell, package manager, compiler, source tree, or secret in the runtime image.
- Build with `-trimpath`; avoid embedding local paths and non-reproducible timestamps.
- Do not publish or sign an official `$OOSHDAQ` image. CI builds and exercises
  the Dockerfile without pushing it.
- Let each self-hosting operator scan, inventory, sign, or digest-pin their own
  build when their environment requires those controls.
- Keep the repository Kubernetes default local-only and fail closed with
  `imagePullPolicy: Never`; a remote deployment explicitly overrides it.
- If the selected SQLite driver introduces CGO or native artifacts, pin and scan the compiler/sysroot inputs, verify the
  final linkage and target architectures, and keep every required runtime library and license notice explicit.

## CI and Automation Controls

- Give workflows the minimum token permissions and no write permission for untrusted pull-request code.
- Pin reusable actions by full commit SHA and retain the release tag in a comment for reviewability.
- Keep project CI free of registry, signing, and deployment credentials.
- Never print secrets, environment dumps, forwarded identities, or signed cookies.
- Review dependency-bot pull requests like any other source change; passing automation is not approval.
- Retain test and local build evidence with the source revision.

## Catalog Inputs Are Supply-Chain Inputs

Remote metadata and artwork can carry code-like risk through malformed parsers, active SVG, tracking URLs, or compromised
source pages.

- Ingest only in an explicit, reviewable workflow, never during an end-user request or application startup.
- Allowlist HTTPS sources, cap downloads, verify MIME by content, and set deadlines and redirect limits.
- Re-encode permitted raster images; do not publish downloaded HTML or SVG as trusted assets.
- Record source URL, retrieval date, checksum, transformation, attribution, and rights decision.
- Keep brand, catalog/edition, item, source, and artwork provenance identifiers distinct and stable across imports.
- Require a reviewed manifest for every one of the five or more catalogs across multiple brands; approval for one brand or
  source does not transfer to another.
- Review generated catalog diffs before publication.
- Use a placeholder when rights or integrity is uncertain.

See [artwork provenance](../catalog/artwork-provenance.md).

## Upgrade Response

For a routine update:

1. read release notes and security advisories;
2. verify the expected module/image/action owner and immutable identifier;
3. inspect source and graph changes proportional to risk;
4. run focused tests, the routine gate, and relevant smoke tests;
5. document behavior or migration changes.

For a disclosed vulnerability, determine whether reachable code/configuration is affected, apply a supported fix or remove
the component, rebuild from clean inputs, rotate credentials if exposure is plausible, and follow
[SECURITY.md](../../SECURITY.md) for disclosure. Do not dismiss a scanner finding without recording the reachability
reason.

## Source and Operator Build Evidence

A source revision must remain buildable from:

- source commit;
- pinned Go toolchain/builder digest;
- reviewed module checksums;
- the selected SQLite library version, compile configuration, driver provenance, and schema migration set;
- catalog manifest version and asset digests;
- CI action digests; and
- repository test, cross-build, container, and recovery results.

The repository currently provides pinned inputs, module checksum verification,
cross-build evidence, executed image hardening/persistence evidence, and the
completed local repository, browser, and container qualifications. The
[release-readiness report](../implementation/release-readiness-report.md)
separates those completed repository gates from target-cluster work owned by a
self-hosting operator.

The project does not issue official binary checksums, container digests,
signatures, or attestations. An operator may record the source revision, local
binary checksum or image ID, vulnerability result, SBOM, signature, and
deployment record for their own build without turning those artifacts into a
project distribution channel.
