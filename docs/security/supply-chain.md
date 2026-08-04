# Supply-Chain Policy

$OOSHDAQ favors a small, inspectable build and deliberately avoids dependencies
for convenience alone. Versions and checksums are authoritative in `go.mod`,
`go.sum`, `web/package-lock.json`, and the Dockerfile; this policy does not
duplicate that inventory.

## Dependency Admission

Before adding or replacing a direct dependency, record:

1. the concrete job it performs and why local or standard-library code is not
   appropriate;
2. ownership, maintenance activity, release cadence, and security reporting
   route;
3. license compatibility with Apache-2.0;
4. direct, transitive, native, and runtime components;
5. known vulnerabilities and source/API review proportional to risk; and
6. how the input is pinned, tested, upgraded, and removed.

Reject dependencies with unclear ownership, incompatible licensing,
unnecessary install-time execution, an excessive graph for a small task, or
unsupported code in a security-sensitive path.

## Go Modules and SQLite

- Commit `go.mod` and `go.sum`; run `go mod tidy` only as an intentional,
  reviewed change.
- Inspect `go mod graph` and `go list -m -json all` for graph changes.
- Use the supported toolchain declared by `go.mod` and keep its builder image
  digest aligned.
- Run `go mod verify`, `go vet ./...`, `go test ./...`, and relevant race tests.
  Review current Go security advisories when changing the toolchain or module
  graph.
- Do not commit an unreviewed local-path or fork replacement.
- Keep required third-party notices in `LICENSES/` and ship the applicable
  notices with the runtime image.

An SQLite-driver update also requires adapter tests for connection settings,
busy handling, migrations, abrupt recovery, online backup, offline restore,
integrity checks, supported architectures, and static-container linkage.
Review the bundled SQLite release and compile configuration, not only the Go
wrapper version. Module checksums prove content identity, not trustworthiness.

Do not add a network database client as a speculative fallback. A different
persistence design requires measured need and an ADR.

## Container Inputs

- Pin build images by immutable digest while retaining a readable tag.
- Use a multi-stage build and copy only the static binary, reviewed assets, and
  required notices into the runtime.
- Keep the runtime non-root, without a shell, package manager, compiler,
  capabilities, writable root filesystem, or embedded secret.
- Build with `-trimpath` and avoid local paths or non-reproducible timestamps.
- If a dependency adds CGO or native artifacts, pin and review the compiler,
  sysroot, linkage, target architectures, runtime libraries, and licenses.

The project distributes source and does not publish or sign an official image
or binary. Each operator builds the selected revision and may add scanning,
SBOM generation, signing, attestation, and digest policy required by its own
environment. The repository defines local build and smoke-test commands, not a
hosted build or test workflow.

## Local Automation

- Keep registry, signing, and deployment credentials out of project scripts.
- Never print credentials, environment dumps, cookies, or forwarded identity
  values.
- Review automated dependency changes like any other source change.
- Treat tests and smoke checks as reproducible gates, not permanent prose
  reports of individual runs.

The repository intentionally has no hosted workflow. Contributors run module
verification, Go analysis and tests, desktop browser tests, and the container
smoke test locally. Operators add vulnerability, secret, or container scanning
and hosted automation when their own policy requires it.

## Catalog Inputs

Catalog metadata and artwork URLs are supply-chain inputs. The current service
loads reviewed embedded manifests and does not download artwork bytes during
startup or an end-user request. Remote image hosts can still observe browser
requests, and every catalog needs an independent provenance and rights review.

If maintenance tooling later downloads assets, it must allowlist HTTPS hosts,
bound size and time, limit redirects, verify type from bytes, reject active
content, decode within resource limits, re-encode permitted raster formats,
record checksums and transformations, and require a reviewed manifest diff.
Use a placeholder whenever rights or integrity are uncertain. See
[artwork provenance](../catalog/artwork-provenance.md).

## Upgrade and Incident Response

For an update, read release notes and advisories, verify the owner and immutable
identifier, inspect source and graph changes, update notices, and run the
focused tests plus relevant wider gates.

For a disclosed vulnerability, determine whether the affected path is
reachable, upgrade to a supported fix or remove the component, rebuild from
clean inputs, and rotate credentials if exposure is plausible. Record any
reachability-based exception and follow [SECURITY.md](../../SECURITY.md) for
disclosure.

Operators should retain the source revision, dependency locks, build input
digests, catalog manifest identity, their image digest, and any locally
required scan, SBOM, signature, and deployment records. Those artifacts remain
operator evidence and do not create a project distribution channel.
