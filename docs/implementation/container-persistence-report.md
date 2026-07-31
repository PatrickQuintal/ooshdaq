# Container persistence evidence

Status: **pass**

Date: 2026-07-31 Australia/Sydney

The current production Dockerfile completed the repository's container
persistence smoke on Docker Desktop 4.55.0 / Engine 29.1.3. The test enforces
one CPU, 256 MiB memory, a read-only root filesystem, a non-root process, and
separate data and backup volumes.

## Result

| Check | Result |
| --- | --- |
| Pinned multi-stage Dockerfile build | Pass |
| Static `scratch` image with no ELF interpreter | Pass |
| UID:GID `65532:65532` in the image and every serving process | Pass |
| Read-only root, all capabilities dropped, `no-new-privileges` | Pass |
| One CPU, 256 MiB memory, no additional swap | Pass |
| Separate `/data` and `/backups` volumes | Pass |
| Authenticated canonical collection write | Pass |
| Replacement using the same persistent volumes | Pass |
| Verified online backup | Pass |
| Isolated restore and API readback | Pass |
| Graceful SQLite checkpoint on every serving shutdown | Pass |
| Smoke-owned resource cleanup | Pass |

## Executed journey

The test:

1. built the checked-in Dockerfile;
2. inspected the exported filesystem and statically linked Go binary;
3. started generation 1, signed in as fictional `mock-alex`, and wrote:

   ```json
   {"owned": 1, "available": 1, "wanted": 0}
   ```

4. confirmed the value through the bounded collection API;
5. stopped generation 1 with SIGTERM and required a completed SQLite
   checkpoint;
6. started generation 2 on the same data and backup volumes and read the exact
   value;
7. copied the application's online backup and verified its logged byte count
   and SHA-256;
8. restored that backup into an isolated data volume with the same image;
9. started a fourth server on the restored database and read the same value;
   and
10. stopped every server cleanly and removed the temporary containers, volumes,
    image tag, and `/tmp` evidence directory.

The available copy is deliberately valid without a wanted item. This exercises
the collection invariant used by giveaway-style availability leads.

## Exact output

```text
Container persistence smoke passed
image=ooshdaq:container-smoke-20260731T082326Z-1154655
image_id=sha256:7d33b9684e3272e4d25126f2f5b22ed0f4c3baa98f202bd1c4ad9ff9933b48e1
image_repo_digest=ooshdaq@sha256:7d33b9684e3272e4d25126f2f5b22ed0f4c3baa98f202bd1c4ad9ff9933b48e1
image_size_bytes=4838734
item_id=disney-2026-01-moana
backup=ooshdaq-20260731T082352.959240765Z.sqlite
backup_sha256=c383499195ea31a701bf0c55a0d5418c7b2cbbb7cc2abea6cbb7a21dc91b8a6f
```

BuildKit resolved the pinned builder:

```text
docker.io/library/golang:1.26.5-alpine3.23
sha256:622e56dbc11a8cfe87cafa2331e9a201877271cbff918af53d3be315f3da88cc
```

It exported platform manifest
`sha256:37d4e1aa8ccfa547f4fcb07d7565432a0db8045abea19f7ff3f5922290f54aa2`
and image config
`sha256:a732a3bee522662fe98ef55728fb6791223f392c24d68189bee8cb6eae37c066`.
The extracted binary reported Go 1.26.5, `CGO_ENABLED=0`, Linux amd64,
`modernc.org/sqlite v1.55.0`, and no ELF interpreter.

These identities belong only to the local smoke build. `$OOSHDAQ` distributes
source and does not publish or sign an official binary or container image.
Each operator builds the selected source and owns its artifact identity.

## Reproduce

With a local Docker daemon available:

```sh
scripts/container-smoke.sh
```

The script creates uniquely named temporary resources and removes only those
resources on exit. Set `OOSHDAQ_SMOKE_KEEP=1` only when deliberately retaining
them for inspection.
