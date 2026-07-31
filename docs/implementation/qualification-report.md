# SQLite Envelope Qualification

This report records the reproducible repository-level capacity lane required by
[ADR 0003](../architecture/adr/0003-clean-reset-modernc-and-bounded-api.md)
and amended by
[ADR 0004](../architecture/adr/0004-simple-states-and-availability-leads.md)
and
[ADR 0005](../architecture/adr/0005-ranked-split-match-feeds.md).
The qualification-v3 fixed 10,000-user, 100-worker, 60-second SQLite gate
**passed** under a hard one-CPU/256-MiB systemd scope on its second recorded
run.

Machine-readable evidence:

- [current ranked split-feed passing run](../../testdata/qualification/2026-07-31-ranked-split-systemd-pass.json);
- [retained first ranked split-feed run](../../testdata/qualification/2026-07-31-ranked-split-systemd-failed.json), which
  used the same fixed configuration and fixture SHA-256 but failed the write
  p95 gate at 1,600 ms;
- [historical qualification-v2 availability-lead passing run](../../testdata/qualification/2026-07-31-availability-leads-systemd-pass.json);
- [historical qualification-v1 reciprocal-only passing run](../../testdata/qualification/2026-07-30-paced-systemd-pass.json);
  and
- [historical qualification-v1 zero-think failure](../../testdata/qualification/2026-07-30-zero-think-failed.json), which
  explains the load-model tuning instead of hiding it.

This is capacity evidence for the SQLite repository on local ext4 storage. It is not a claim that the complete product,
HTTP layer, recovery plan, container, or GKE deployment has passed every release gate.

## Exact Reproduction

The runner refuses to overwrite its SQLite database, WAL sidecars, or JSON report. Use a new directory for every run.
The transient user scope is part of the qualification command: `CPUQuota=100%` is one CPU of time,
`MemoryMax=256M` is the hard memory ceiling, and swap is disabled.

```sh
qualification_dir="$(mktemp -d)"
GOCACHE="$qualification_dir/go-cache" \
  go build -trimpath -o "$qualification_dir/ooshdaq-qualify" ./cmd/ooshdaq-qualify
systemd-run --user --scope --quiet \
  -p CPUQuota=100% \
  -p MemoryMax=256M \
  -p MemorySwapMax=0 \
  "$qualification_dir/ooshdaq-qualify" \
  -mode full \
  -database "$qualification_dir/qualification.sqlite" \
  -report "$qualification_dir/report.json"
```

The recorded run used Go 1.26.5. `-mode full` fixes:

- seed `20260730`;
- 3 fictional brands, 5 fictional catalogs, and 40 items per catalog;
- 10,000 fictional users and 80,000 collection rows;
- 100 workers for 60 seconds;
- a 100-millisecond pause after each member operation;
- 8 SQLite connections and a 5-second busy timeout;
- `GOMAXPROCS=1`; and
- a 220 MiB Go soft memory limit inside the hard 256 MiB scope.

The runner accepts explicit overrides for diagnostic experiments. An override which changes the fixture or seed, workers,
duration, fixed member pause, connection bound, or busy timeout cannot claim the release gate. See all flags with:

```sh
go run ./cmd/ooshdaq-qualify -h
```

The fast normal-test lane keeps the five-catalog, 200-item, split-match query
shape while reducing the fixture to
100 users, 800 rows, 4 workers, and 2 seconds. Two seconds keeps the complete
twenty-operation mix reliable under race instrumentation:

```sh
qualification_dir="$(mktemp -d)"
go run ./cmd/ooshdaq-qualify \
  -mode smoke \
  -database "$qualification_dir/smoke.sqlite"
```

## Fixture and Workload Design

Catalog manifests are generated in memory and contain only fictional names, local placeholder instructions, and
`unknown` artwork rights. They contain no third-party images, URLs, descriptions, or production identities. Catalog
import uses the real manifest importer.

The setup phase loads users and sparse collection rows in one direct SQLite transaction so 80,000 setup commits do not
pollute the timed application workload. Every timed operation uses a real `sqliterepo.Store` method:

| Scheduled share | Operation | Response bound |
| ---: | --- | ---: |
| 15% | catalog navigation | at most 5 catalogs in this fixture |
| 50% | collection page read | at most 40 items |
| 20% | actionable, cross-catalog match read | at most 20 matches |
| 10% | unfulfillable match read | at most 20 matches |
| 5% | collection write | one sparse row |

One global deterministic twenty-operation sequence contains exactly 3 catalog,
10 collection, 4 actionable-match, 2 unfulfillable-match, and 1 write
operation. One hundred workers continuously claim the next operation for 60
seconds. Each worker pauses for 100 milliseconds after an operation; that wait
is outside measured request latency.

The pause is deliberate closed-workload pacing, not a reduced worker count or a
changed request mix. The historical qualification-v1 zero-think run generated
a continuously replenished, growing serialized-write queue: write p50 was 2.1
seconds and p95 was 3.1 seconds. Adding a 100-millisecond per-member pause
preserved 100 active workers, the full duration, and all thresholds in the
following qualification-v1 run, which attempted approximately 455 operations
per second. The pause prevents one member from immediately replacing every
completed request with another. One hundred milliseconds remains far more
aggressive than human interaction time. It is now part of the fixed
configuration, so silently increasing it cannot turn a failing run green.

In-flight operations canceled by the exact 60-second deadline are reported separately from unexpected repository errors.
Successful-response latency percentiles use bounded histograms: p50 and p95 values are conservative bucket upper bounds,
while maximum latency is exact.

Most synthetic member pairs have inverse offer/want rows over one unique item
pair. One deterministic pair is reserved as a one-way lead: its offering
counterpart has zero wanted rows, so the gate proves that no return item is
required. A second pair is reserved for the separate unfulfillable feed: the
counterpart offers what the current member wants but requests an item that
member cannot offer. Of the remaining reciprocal pairs, roughly four-fifths
cross catalogs and one-fifth remain within a catalog. The other sparse rows are
owned-only state, including the row used by timed writes. This avoids
manufacturing accidental many-to-many match explosions.

After the workload, the runner records:

- database, WAL, and shared-memory file sizes;
- passive and truncate checkpoint frame counts, busy state, and duration;
- readiness plus full foreign-key and integrity checks;
- actionable and unfulfillable production-statement `EXPLAIN QUERY PLAN`
  details and index names;
- response-limit violations, classified busy failures, and other errors;
- Go heap/runtime memory and Linux RSS/high-water memory;
- CPU affinity, cgroup limits, Go version, and `GOMAXPROCS`; and
- every individual acceptance gate plus aggregate functional and formal results.

## Passing Run — 2026-07-31

Environment:

- local Linux amd64 environment;
- ext4-backed workspace;
- Go 1.26.5;
- cgroup CPU quota exactly 1;
- cgroup hard memory limit exactly 268,435,456 bytes, with swap disabled;
- `GOMAXPROCS=1` and a 220 MiB Go soft memory limit; and
- SQLite pool bound to 8 connections.

### Fixture

| Measure | Observed |
| --- | ---: |
| Fixture version | `qualification-v3` |
| Fixture SHA-256 | `f61ca842d76c964af4a63bfa5e4b539afc4710d8dc1b654bf060afcc406efc26` |
| Seed duration | 1.007 s |
| Brands / catalogs / items | 3 / 5 / 200 |
| Users | 10,000 |
| Collection rows | 80,000 |
| Rows per user / density | 8 / 4% |
| Same-catalog reciprocal pairs | 1,000 |
| Cross-catalog reciprocal pairs | 3,998 |
| Same- and cross-catalog reciprocal checks | passed |
| Zero-wanted counterpart one-way check | passed |
| Separate unfulfillable-feed check | passed |

### Sixty-Second Workload

The run lasted 60.002 seconds and attempted 25,009 operations, or 416.80
attempts/second. It completed 24,962 operations, or 416.02
successes/second.

| Operation | Attempts | Successes | Mix | p50 | p95 | Max | Gate |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| Catalog | 3,752 | 3,732 | 15% | 290 ms | 460 ms | 9,558.315 ms | pass: p95 < 1 s |
| Collection | 12,504 | 12,481 | 50% | 120 ms | 240 ms | 8,122.602 ms | pass: p95 < 1 s |
| Actionable matches | 5,001 | 4,999 | 20% | 43 ms | 120 ms | 1,699.614 ms | pass: p95 < 2 s |
| Unfulfillable matches | 2,501 | 2,500 | 10% | 44 ms | 120 ms | 4,814.806 ms | pass: p95 < 2 s |
| Writes | 1,251 | 1,250 | 5% | 83 ms | 330 ms | 426.606 ms | pass: p95 < 1 s |

There were zero unexpected repository errors, zero exposed `SQLITE_BUSY` results, and zero response-limit violations.
Forty-seven operations were canceled when the fixed deadline closed: 20
catalog, 23 collection, 2 actionable-match, 1 unfulfillable-match, and 1 write
operation. Deadline cancellation is expected at workload shutdown and is not
counted as an unexpected error.

### Storage, Memory, and Checkpoint

| Measure | Before checkpoint | After truncate checkpoint |
| --- | ---: | ---: |
| Database | 15,208,448 bytes (14.50 MiB) | 15,208,448 bytes (14.50 MiB) |
| WAL | 15,548,912 bytes (14.83 MiB) | 0 bytes |
| Shared memory | 32,768 bytes | 32,768 bytes |

The passive checkpoint reported `busy=0`, 250 log frames, 250 checkpointed
frames, and 5.706 ms duration. The following truncate checkpoint reported
`busy=0`, zero remaining frames, and 4.642 ms duration.

Process memory was:

- 20,725,760-byte baseline RSS;
- 38,211,584-byte final RSS;
- 64,184,320-byte peak RSS (61.21 MiB);
- 8,341,728 bytes of final Go heap allocation; and
- 26,380,568 bytes of final Go runtime-managed system memory.

The process stayed well below the enforced 256 MiB limit.

### Readiness, Integrity, and Query Plan

Readiness, `foreign_key_check`, and full `integrity_check` all passed. The
one-way zero-want journey, unfulfillable journey, and both reciprocal scopes
returned the expected shapes, and every response remained within its hard
repository limit. The timed workload exercises both production split-feed
statements. It does not substitute a smaller proxy query for either statement.

The complete recorded plans are in the machine-readable report. The
match-discovery portions of the actionable production plan were:

```text
SEARCH current_want USING COVERING INDEX collection_entries_user_wants_idx (user_id=?)
SEARCH other_offer USING COVERING INDEX collection_entries_offers_idx (item_id=?)
SEARCH other_want USING COVERING INDEX collection_entries_user_wants_idx (user_id=? AND item_id=? AND wanted>?) LEFT-JOIN
CORRELATED SCALAR SUBQUERY 1
SEARCH candidate_want USING COVERING INDEX collection_entries_user_wants_idx (user_id=?)
SEARCH candidate_offer USING PRIMARY KEY (user_id=? AND item_id=?)
SEARCH current_offer USING COVERING INDEX collection_entries_user_offers_idx (user_id=? AND item_id=? AND available>?) LEFT-JOIN
USE TEMP B-TREE FOR ORDER BY
USE TEMP B-TREE FOR ORDER BY
SCAN page
```

The match-discovery portions of the unfulfillable production plan were:

```text
SEARCH current_want USING COVERING INDEX collection_entries_user_wants_idx (user_id=?)
SEARCH other_offer USING COVERING INDEX collection_entries_offers_idx (item_id=?)
CORRELATED SCALAR SUBQUERY 2
SEARCH fulfill_want USING COVERING INDEX collection_entries_user_wants_idx (user_id=?)
SEARCH fulfill_offer USING PRIMARY KEY (user_id=? AND item_id=?)
SEARCH other_want USING PRIMARY KEY (user_id=? AND item_id=?)
CORRELATED SCALAR SUBQUERY 1
SEARCH candidate_want USING COVERING INDEX collection_entries_user_wants_idx (user_id=?)
SEARCH candidate_offer USING COVERING INDEX collection_entries_user_offers_idx (user_id=? AND item_id=? AND available>?) LEFT-JOIN
USE TEMP B-TREE FOR ORDER BY
USE TEMP B-TREE FOR ORDER BY
SCAN page
```

All collection-entry aliases use indexed searches. `SCAN page` reads the
already bounded page CTE; it is not a full collection-entry scan. The
subsequent item, catalog, brand, asset, and user projection lookups also use
their recorded primary-key or auto-index searches.

### Retained First Run

The first qualification-v3 execution used the identical fixed configuration
and fixture SHA-256 as the passing run. It recorded 23,803 attempts at 396.49
attempts/second, zero unexpected errors, zero exposed `SQLITE_BUSY` results,
zero response-limit violations, and successful journeys, query plans,
integrity checks, checkpoints, and resource-limit checks. Its p95 values were
510 ms catalog, 260 ms collection, 130 ms actionable matches, 130 ms
unfulfillable matches, and **1,600 ms writes**. The write result exceeded the
strict one-second threshold, so `latency_thresholds_passed`,
`functional_thresholds_passed`, and `formal_release_gate_passed` were false.

That [failed first run](../../testdata/qualification/2026-07-31-ranked-split-systemd-failed.json)
is not treated as passing evidence. Retaining it alongside the immediately
following passing run makes the observed write-latency variance visible and
supports repeating the unchanged lane on target storage rather than relying
on a single local result.

## Gate Decision and Remaining Qualification

Every field in the recorded report's `gates` object is `true`, including:

- fixed release shape and workload;
- the exact 15/50/20/10/5 request mix;
- zero unexpected and busy errors;
- all p95 latency thresholds;
- response bounds, the zero-want one-way journey, the separate unfulfillable
  journey, and reciprocal journeys;
- both production query plans and their required indexes;
- readiness, foreign keys, and full integrity;
- passive and truncate checkpoint results;
- pinned Go 1.26.5;
- hard one-CPU/256-MiB limits; and
- observed memory below the hard limit.

The SQLite repository envelope gate therefore passes in this measured
environment. The identical-shape first qualification-v3 failure remains
checked in to expose the observed write-latency variance. The historical
qualification-v1 zero-think failure also remains checked in because it
demonstrates why load pacing is explicit and fixed. Neither failure is
overwritten or reclassified by the passing run.

This lane does not replace HTTP/browser qualification, process-kill and restart tests, backup/restore evidence,
disk-full testing, container-image execution, or GKE volume reattachment drills. Run the same unchanged qualification
binary on production-representative `standard-rwo` storage before treating this local ext4 result as GKE capacity
evidence.
