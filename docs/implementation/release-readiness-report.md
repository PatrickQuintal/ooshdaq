# Source Readiness and Self-Hosting Handoff

Status: **repository implementation, local build/recovery rehearsal, and
qualification-v3 repository envelope pass**

No production rollout is claimed. This repository began with no valuable
state or target cluster. The completed repository gates prove that the source
builds and operates as documented. `$OOSHDAQ` is source-distributed: the
project does not publish or sign an official binary or container image.
Self-hosting operators build their own artifact and own their deployment
boundary.

The current capacity result is the second recorded qualification-v3 run. The
first run used the same fixed configuration and fixture digest but failed the
write p95 threshold; both artifacts are retained. These capacity measurements
call repository methods directly and do not claim a fresh HTTP, container, or
GKE runtime qualification.

## Repository Gate Summary

| Gate | Result | Evidence |
| --- | --- | --- |
| Format, static analysis, unit, and integration | Pass | Go 1.26.5 `gofmt`, `go vet ./...`, and `go test ./...` |
| Race detector | Pass | `go test -race ./...`, including process and real SQLite tests |
| Browser contracts | Pass | 13 dependency-free Node tests |
| Live browser | Pass | 7 Chromium journeys against the real HTTP server and SQLite; see [browser report](browser-journey-report.md) |
| SQLite migration and recovery | Pass | Failed migration rollback, typed failure, checkpoint, graceful restart, abrupt kill, online backup, and isolated restore tests |
| Indexed matching | Pass | Separate actionable/unfulfillable production-query plans, capped-wait ordering, a 320-counterpart fan-out regression, and current qualification-v3 evidence |
| Fixed capacity envelope | Pass | Second fixed qualification-v3 run: 10,000 users, 5 catalogs, 80,000 sparse rows, 100 workers, 60 seconds; see [qualification report](qualification-report.md) |
| Container runtime and persistence | Pass | Hardened scratch image, replacement on the same volume, verified backup, and isolated restore; see [container report](container-persistence-report.md) |
| Kubernetes source validation | Pass | Kustomize 5.8.1 render and strict Kubeconform 0.8.0 validation against Kubernetes 1.35.0: 4 valid serving resources plus 1 separately rendered maintenance pod |
| Module integrity | Pass | `go mod verify`: all modules verified |
| Linux architectures | Pass | Static, stripped, interpreter-free Linux amd64 and arm64 builds |
| Catalog platform | Pass | 5 manifests, 4 brands, 56 items, 40 reviewed source-hosted image links, and 16 local placeholders |
| Legacy removal | Pass | No runtime prototype adapter, dependency, selector, compatibility route, or rollback path |

The final cross-build commands used `CGO_ENABLED=0`, `-mod=readonly`,
`-trimpath`, `-buildvcs=false`, and `-ldflags='-s -w -buildid='`.

```text
linux/amd64
sha256 c4aea3e450747b11811b80e3561b371867851ec99cd3fff8fb1f144dad421885

linux/arm64
sha256 1a1997312e042a32a9401b2fbe492188e98feaf2ddb177aa9fe7deaa345e319c
```

The amd64 build was repeated from the same workspace and compared byte for
byte. These are local binary identities, not published release checksums.

## Measured Envelope

The fixed qualification ran under a hard one-CPU/256-MiB cgroup scope with
swap disabled:

- 10,000 fictional users, 3 fictional brands, 5 catalogs, 200 items, and
  80,000 sparse collection rows;
- qualification-v3 fixture SHA-256
  `f61ca842d76c964af4a63bfa5e4b539afc4710d8dc1b654bf060afcc406efc26`,
  with 1,000 same-catalog and 3,998 cross-catalog reciprocal pairs plus
  verified one-way and unfulfillable journeys;
- 100 workers for 60 seconds with the exact 15/50/20/10/5 catalog,
  collection, actionable-match, unfulfillable-match, and write mix;
- 25,009 attempts at 416.80 attempts/second and 24,962 successes at 416.02
  successes/second;
- zero unexpected errors, exposed `SQLITE_BUSY` results, or response-limit
  violations;
- p95 catalog 460 ms, collection 240 ms, actionable matches 120 ms,
  unfulfillable matches 120 ms, and writes 330 ms;
- peak RSS 64,184,320 bytes;
- successful readiness, foreign keys, full integrity, both production
  split-feed query plans, passive and truncate checkpoints, hard resource
  checks, and every field in the report's `gates` object.

The [current passing evidence](../../testdata/qualification/2026-07-31-ranked-split-systemd-pass.json)
is accompanied by the [retained first run](../../testdata/qualification/2026-07-31-ranked-split-systemd-failed.json).
That first run had the identical fixed configuration and fixture SHA-256, but
its 1,600 ms write p95 exceeded the one-second threshold; its latency,
functional, and formal release gates are therefore false. It is not
reclassified as a pass by the following successful run. Qualification-v1 and
qualification-v2 artifacts remain historical evidence in the
[qualification report](qualification-report.md).

The local ext4 result is the accepted repository capacity gate. The unchanged
runner must still be repeated on the target `standard-rwo` volume before a GKE
rollout claims target-storage capacity. It neither reruns nor supersedes the
separate browser, local container, or manifest-validation evidence above.

## Local Cutover and Recovery Rehearsal

The current source completed the repository-scoped M8 rehearsal:

1. A fresh path was opened as SQLite; checksummed migrations ran and all five
   embedded manifests imported transactionally.
2. The live browser signed in, navigated bounded catalog pages, used the inline
   collection states, created a one-way Maui availability lead with no return
   item, enriched it with an optional cross-catalog reciprocal item, and
   rediscovered the state after reload.
3. Graceful process restart and abrupt process kill preserved committed state.
4. The production Dockerfile built a 4,838,734-byte `scratch` image. Runtime
   inspection proved UID:GID 65532, no dynamic interpreter, no operating-system
   tree, a read-only root filesystem, dropped capabilities,
   `no-new-privileges`, and enforced one-CPU/256-MiB limits.
5. Generation 1 wrote a known value and checkpointed on SIGTERM. Generation 2
   mounted the same data and backup volumes and read the exact value.
6. The process published an online backup whose copied bytes matched SHA-256
   `c383499195ea31a701bf0c55a0d5418c7b2cbbb7cc2abea6cbb7a21dc91b8a6f`.
7. The same image restored that backup into an isolated data volume; a fourth
   server read the exact pre-backup value and shut down cleanly.
8. The serving base and offline-maintenance Kubernetes manifests rendered
   separately without schema errors and retained the one-writer/RWOP topology.

The local smoke build had transient image identity
`sha256:7d33b9684e3272e4d25126f2f5b22ed0f4c3baa98f202bd1c4ad9ff9933b48e1`.
It was built from an uncommitted workspace and removed after the test. It is
evidence for the repository Dockerfile, not an official `$OOSHDAQ` image. The
Kubernetes templates instead expect an operator-built `ooshdaq:local` image
and use `imagePullPolicy: Never` until the deployment owner deliberately
overrides that choice.

## Self-Hosting Boundary

Before running a production installation, its operator should:

1. select a reviewed source revision and require its CI workflow to pass,
   including full-repository `govulncheck`;
2. build the binary with `make build` or the local container with
   `make docker-build`;
3. load `ooshdaq:local` into the cluster, or retag the same self-built image
   into an operator-controlled registry and override the templates;
4. apply any scan, SBOM, signature, or digest policy required by that
   operator—none is a project publication requirement;
5. add the approved TLS ingress, auth proxy, default-deny network policy,
   secret integration, monitoring, and alert routes;
6. bind the two target `standard-rwo` `ReadWriteOncePod` claims and prove that
   only the serving pod can mount them;
7. rerun the unchanged capacity binary on the target volume;
8. rehearse pod replacement, node drain, volume detach/reattach, backup export,
   and isolated restore in staging;
9. execute the maintenance-window checklist in the
   [backup and restore runbook](../operations/sqlite-backup-restore.md); and
10. record the production rollout, fresh backup digest, restore result, owner,
    and rollback decision before reopening traffic.

The local dependency-admission spike reported no reachable vulnerability in
the selected SQLite driver. A final full-repository vulnerability result is
intentionally delegated to CI because this environment did not authorize
sending the private workspace's module metadata to the external vulnerability
database. That limitation is not represented as a passing local scan.

## Decision

M0 through M8 are complete for repository scope: the clean SQLite
implementation, five-catalog platform, bounded API/browser, recovery tooling,
single-pod manifests, local qualification-v3 capacity evidence, cutover
rehearsal, and handoff documentation are present. The passing repository
capacity artifact does not establish HTTP performance, container performance,
or GKE runtime/volume behavior. Building, validating, and operating a
deployment remains the responsibility of each self-hosting environment.
