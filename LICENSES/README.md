# Third-Party License Inventory

The repository-root `LICENSE` contains the Apache License 2.0 terms for
$OOSHDAQ and for `github.com/google/pprof`. The remaining notices are
reproduced in this directory.

Build and runtime:

| Component | Selected version | License notice |
| --- | --- | --- |
| Go toolchain and standard library | 1.26.5 | `go-BSD-3-Clause.txt` |

SQLite implementation:

| Component | Selected version | License notice |
| --- | --- | --- |
| SQLite | 3.53.3 | `sqlite-Public-Domain.txt` |
| `modernc.org/sqlite` | v1.55.0 | `modernc-sqlite-BSD-3-Clause.txt` |
| `modernc.org/libc` | v1.74.1 | `modernc-libc-BSD-3-Clause.txt`, `modernc-libc-THIRD-PARTY.md` |
| `github.com/google/pprof` | v0.0.0-20250317173921-a4b03ec1a45e | `google-pprof-Apache-2.0.txt` and root `LICENSE` |
| `golang.org/x/sys` | v0.46.0 | `x-sys-BSD-3-Clause.txt` |
| `modernc.org/fileutil` | v1.4.0 | `modernc-fileutil-BSD-3-Clause.txt` |
| `modernc.org/mathutil` | v1.7.1 | `modernc-mathutil-BSD-3-Clause.txt`, `modernc-mathutil-mersenne-BSD-3-Clause.txt` |
| `github.com/dustin/go-humanize` | v1.0.1 | `go-humanize-MIT.txt` |
| `github.com/google/uuid` | v1.6.0 | `google-uuid-BSD-3-Clause.txt` |
| `github.com/mattn/go-isatty` | v0.0.20 | `go-isatty-MIT.txt` |
| `github.com/ncruces/go-strftime` | v1.0.0 | `go-strftime-MIT.txt` |
| `github.com/remyoudompheng/bigfft` | v0.0.0-20230129092748-24d4a6f8daec | `bigfft-BSD-3-Clause.txt` |
| `modernc.org/memory` | v1.11.0 | `modernc-memory-BSD-3-Clause.txt`, `modernc-memory-GO-BSD-3-Clause.txt`, `modernc-memory-MMAP-BSD-3-Clause.txt` |

If an operator generates a compiled-binary SBOM, compare it with this
inventory; a module selected by Go's module graph is not necessarily linked
into a particular target binary. The project distributes source and does not
publish an official binary or container SBOM.
