# modernc SQLite Dependency Review

- **Decision:** accepted for the SQLite implementation
- **Reviewed:** 2026-07-30
- **Runtime status:** current production adapter and composition root
- **Architecture decision:** [ADR 0003](../architecture/adr/0003-clean-reset-modernc-and-bounded-api.md)

## Selection

Pin both modules explicitly:

```text
modernc.org/sqlite v1.55.0
modernc.org/libc   v1.74.1
```

`modernc.org/sqlite` documents that downstream users must select the exact
`modernc.org/libc` version in its `go.mod`. Treat the pair as one update. Do
not allow Minimal Version Selection to move `modernc.org/libc` independently.

The selected driver is a CGO-free Go translation of SQLite. It preserves the
existing `CGO_ENABLED=0`, Alpine builder, statically linked Linux binary, and
scratch runtime design. The driver is BSD-3-Clause licensed; bundled SQLite is
public domain.

## Immutable Evidence

The accepted module identities are:

```text
modernc.org/sqlite v1.55.0
module: h1:hIFh0MCH0rGinQ/4KYb5/UbCkRkb+UP+OkLCVWa5MTM=
go.mod: h1:4ntCLuNmnH8+GNqjka1wNg7KJd5/Hi5FYp8K+XQ7GZw=
origin commit: cfb97341944de3bfa93dd4d7ef30bc921e924229

modernc.org/libc v1.74.1
module: h1:bdR4VTKFMC4966QSNZ05XLGI/VwzVa2kTUX51Dm0riQ=
go.mod: h1:uH4t5bOx3G3g9Xcmj10YKlTcVISlRDwv8VoQJG9n8Os=
```

The isolated runtime query returned:

```text
sqlite_version():
3.53.3

sqlite_source_id():
2026-06-26 20:14:12 d4c0e51e4aeb96955b99185ab9cde75c339e2c29c3f3f12428d364a10d782c62
```

SQLite documents that the rare multi-connection WAL-reset corruption bug is
fixed in 3.51.3 and later. The selected 3.53.3 source is beyond that boundary.
Version ordering alone is not enough for future reviews: record and compare the
full source ID.

## Isolated Spike Evidence

The review used a temporary Go 1.26.2 module outside the repository and made no
application-code changes.

- `CGO_ENABLED=0 go test` passed on Linux amd64.
- Runtime configuration returned `journal_mode=wal`, `synchronous=2` (FULL),
  `foreign_keys=1`, and the configured 250 ms busy timeout.
- A unique-constraint failure surfaced as `*sqlite.Error` with extended result
  code 2067 (`SQLITE_CONSTRAINT_UNIQUE`).
- A deliberately long recursive query stopped at its context deadline and
  returned `context deadline exceeded`.
- `NewBackup`, `Step`, and `Finish` produced a backup that reopened and returned
  the expected data.
- `go test -race` passed. The Go race runtime itself uses CGO on supported
  platforms; this does not make the production driver or binary depend on CGO.
- `CGO_ENABLED=0` cross-compilation produced statically linked Linux amd64 and
  arm64 ELF test binaries. The amd64 binary was executed; arm64 was
  cross-compiled and inspected, not emulated.

These results admit the dependency. They do not replace repository contention,
failure-injection, crash-recovery, backup/restore, or capacity tests.

## Selected Module Graph and Licenses

The v1.55.0 driver manifest names eleven immediate module requirements. Their
own manifests add build, test, and generator modules to the complete MVS graph;
those graph-only modules are not linked into the default application binary.
The “linked in spike” column reflects the default Linux database/sql test
binary, not every possible build tag. Deployment evidence should come from the
operator's compiled binary; an SBOM is optional operator tooling rather than a
project-published artifact.

| Component | Version | Driver graph role | Linked in spike | License record |
| --- | --- | --- | --- | --- |
| `modernc.org/sqlite` | v1.55.0 | application direct | yes | `LICENSES/modernc-sqlite-BSD-3-Clause.txt` |
| `modernc.org/libc` | v1.74.1 | required | yes | `LICENSES/modernc-libc-BSD-3-Clause.txt`, `LICENSES/modernc-libc-THIRD-PARTY.md` |
| `github.com/google/pprof` | v0.0.0-20250317173921-a4b03ec1a45e | required | no | `LICENSES/google-pprof-Apache-2.0.txt`, root `LICENSE` |
| `golang.org/x/sys` | v0.46.0 | required | yes | `LICENSES/x-sys-BSD-3-Clause.txt` |
| `modernc.org/fileutil` | v1.4.0 | required | no | `LICENSES/modernc-fileutil-BSD-3-Clause.txt` |
| `modernc.org/mathutil` | v1.7.1 | required | yes | `LICENSES/modernc-mathutil-BSD-3-Clause.txt`, mersenne notice |
| `github.com/dustin/go-humanize` | v1.0.1 | indirect | yes | `LICENSES/go-humanize-MIT.txt` |
| `github.com/google/uuid` | v1.6.0 | indirect | yes | `LICENSES/google-uuid-BSD-3-Clause.txt` |
| `github.com/mattn/go-isatty` | v0.0.20 | indirect | no | `LICENSES/go-isatty-MIT.txt` |
| `github.com/ncruces/go-strftime` | v1.0.0 | indirect | no | `LICENSES/go-strftime-MIT.txt` |
| `github.com/remyoudompheng/bigfft` | v0.0.0-20230129092748-24d4a6f8daec | indirect | yes | `LICENSES/bigfft-BSD-3-Clause.txt` |
| `modernc.org/memory` | v1.11.0 | indirect | yes | module, Go-derived, and mmap-go notices in `LICENSES/` |
| SQLite | 3.53.3 | bundled source | yes | `LICENSES/sqlite-Public-Domain.txt` |

The removed prototype store is absent from the module and license inventories.

## Vulnerability and Toolchain Review

An official `govulncheck` v1.6.0 scan against the isolated spike and the
2026-07-30 Go vulnerability database reported no reachable vulnerabilities.
Its verbose output identified three imported-package and ten module-level
advisories in the Go 1.26.2 standard library, rather than the selected driver.

Go 1.26.5, released 2026-07-07, includes the later `crypto/tls` and `os`
security fixes and is the current build baseline. The repository pins the Go
toolchain and builder tag and digest to 1.26.5; future changes keep those inputs
in one review.

`govulncheck` analyzes Go module and symbol reachability. It may not report a
vulnerability in SQLite source translated into Go until the relevant advisory
is represented in the Go vulnerability database. Every update and release
therefore also reviews the SQLite release log, SQLite security notices, the
driver changelog, retractions, and the exact runtime source ID.

## Update Policy

For a driver update:

1. update `modernc.org/sqlite` and its exact required `modernc.org/libc`
   version in one review;
2. verify tag ownership, module and `go.mod` checksums, origin commits, and
   retractions;
3. compare `go mod graph`, the compiled module list for Linux amd64 and arm64,
   license files, and any generated SBOM;
4. query and record `sqlite_version()` and `sqlite_source_id()`, and confirm the
   source is not older than or missing the SQLite 3.51.3 WAL fix;
5. review changed SQLite compile flags, extensions, VFS, backup, cancellation,
   transaction, error-code, and connection-hook behavior;
6. rerun unit, repository, race, cross-build, WAL-contention, crash-recovery,
   backup/restore, and accepted-capacity tests;
7. run a current `govulncheck` and image scan, while separately reviewing
   upstream SQLite advisories; and
8. retain the evidence with the release. Dependency-bot output is a prompt for
   review, never approval to merge automatically.

For a Go patch update, review the official release history, pin the module
toolchain and builder image/digest together, rerun the same application and
container gates, and refresh vulnerability evidence.

## Implementation Evidence

The accepted pins and checksums are committed; startup verifies every pooled connection's WAL, FULL synchronous,
foreign-key, and busy-timeout settings; repository tests cover cancellation, contention, typed full/read-only/corrupt
failures, transactional migrations, online backup, isolated restore, graceful restart, and abrupt recovery. The
qualification report records the accepted 10,000-user workload and checkpoint
behavior. The release-readiness report records static Linux amd64/arm64 build
results, and the container report proves the driver in the locally executed
scratch runtime image built from the production Dockerfile.
The project publishes no official container image. A self-hosting operator may
scan and inventory their own build and should compare any generated SBOM with
`NOTICE` and `LICENSES/`.

## Primary Sources

- [modernc SQLite v1.55.0 source](https://gitlab.com/cznic/sqlite/-/tree/v1.55.0)
- [modernc SQLite v1.55.0 changelog](https://gitlab.com/cznic/sqlite/-/blob/v1.55.0/CHANGELOG.md)
- [SQLite 3.53.3 release](https://sqlite.org/releaselog/3_53_3.html)
- [SQLite WAL-reset bug](https://sqlite.org/wal.html#the_wal_reset_bug)
- [SQLite online backup API](https://sqlite.org/backup.html)
- [SQLite public-domain statement](https://sqlite.org/copyright.html)
- [Go 1.26 release history](https://go.dev/doc/devel/release#go1.26.0)
- [`govulncheck`](https://go.dev/security/vuln/)
