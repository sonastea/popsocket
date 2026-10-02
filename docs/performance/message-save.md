# Single-transaction message saves

Follow-up: [Batching message-save statements](message-save-batching.md).

## Environment and method

Measured on 2026-10-01: Go 1.27.1, macOS/arm64, Apple M1, PostgreSQL 18.1
(aarch64, Docker), READ COMMITTED, `fsync=on`, `synchronous_commit=on`.
The baseline is `messageStore.Save` at `0becc79`; the comparison changes only
conversation resolution and removes the FK-error transaction restart.
Comparisons use benchstat from `golang.org/x/perf` revision `406019bb8b68`.

Both binaries use the same benchmark harness, eight database connections,
`-cpu=8`, and ten independent one-second measurements per workload. Each
measurement creates an isolated schema with the relevant unique/FK constraints
and warms the pool. Profiling runs are separate from timing runs.

- **New:** a unique conversation per save, with serial and eight-writer variants.
- **Existing:** repeated saves to one conversation, serial and eight writers.
- **Contended creation:** each group of eight saves shares a fresh conversation ID.
- **Delivery:** Save, protobuf serialization, and sender/recipient queue fanout
  with eight bounded writers. Healthy clients drain 1,024-slot queues immediately;
  saturated clients drain one-slot queues at one message per millisecond.
  These measure queue delivery, excluding WebSocket transport.

`latency-ns/op` measures mean per-request duration, including database pool and
queue waits; the table below gives benchstat's median across ten runs. Standard
`ns/op` measures aggregate throughput cost when writers run concurrently.
`db-ops/op` counts SQL executions, including BEGIN/COMMIT/ROLLBACK, excluding
protocol-level statement preparation and closed-transaction cleanup calls.

## Repeated benchmark results

| Workload | Request latency before → after | Change | Aggregate µs/op before → after |
| --- | ---: | ---: | ---: |
| New, serial | 1.917 → 1.925 ms | no significant change | 1,918 → 1,926 |
| New, concurrent | 3.920 → 3.871 ms | no significant change | 490.7 → 484.4 |
| Existing, serial | 2.792 → 1.887 ms | −32.42% | 2,792 → 1,887 |
| Existing, concurrent | 6.323 → 4.293 ms | −32.11% | 791.4 → 537.3 |
| Contended creation | 6.212 → 4.335 ms | −30.22% | 777.9 → 542.6 |
| Healthy delivery | 6.451 → 4.248 ms | −34.15% | 807.7 → 531.7 |
| Saturated delivery | 8.030 → 7.982 ms | −0.60% | 1,007 → 1,001 |

The existing/contended/delivery latency comparisons have `p < 0.001`, `n=10`.
New-conversation latency has `p=0.631` (serial) and `p=0.796` (concurrent).
Saturated delivery remains dominated by the configured receiver drain rate.

| Workload | KiB/op before → after | Allocs/op before → after | SQL ops/save before → after | Transactions/save before → after |
| --- | ---: | ---: | ---: | ---: |
| New, serial | 3.245 → 3.244 | 68 → 68 | 6 → 6 | 1 → 1 |
| New, concurrent | 3.251 → 3.249 | 68 → 68 | 6 → 6 | 1 → 1 |
| Existing, serial | 6.100 → 3.830 | 122 → 74 | 8 → 7 | 2 → 1 |
| Existing, concurrent | 6.098 → 3.823 | 122 → 74 | 8 → 7 | 2 → 1 |
| Contended creation | 5.758 → 3.781 | 116 → 76 | 7.749 → 6.875 | 1.875 → 1 |
| Healthy delivery | 6.162 → 3.886 | 123 → 75 | 8 → 7 | 2 → 1 |
| Saturated delivery | 6.163 → 3.888 | 123 → 75 | 8 → 7 | 2 → 1 |

Successful saves have zero SQL rollbacks after the change, versus one per
existing-conversation save and approximately 0.875 per contended-creation save.

Goroutine counts are identical before/after. Idle → peak → end counts are
5 → 6 → 5 for serial storage, 5 → 14 → 5 for concurrent storage,
4 → 13 → 4 for contended creation, and 6 → 15 → 6 for both delivery workloads.
Counts include the test harness, pool maintenance, queue readers, and monitor;
they describe the bounded workloads above.

## Load profiles

Separate 12-second-per-workload runs captured CPU profiles plus live heap and
goroutine snapshots for every workload, with a 16-KiB heap sampling rate.
Aggregate CPU samples were 51.48 seconds over 144.37 seconds before, and
43.06 seconds over 133.65 seconds after (including benchmark calibration/setup).
Both profiles attribute about 91% of samples to `runtime.kevent`,
`syscall.rawsyscalln`, and pthread wait/signal operations. The profile durations
and completed-operation counts differ; the repeated benchmarks above provide
the per-operation comparison.

Last under-load retained-heap snapshots (`inuse_space`, after GC):

| Workload | Before → after, MiB |
| --- | ---: |
| New, serial | 2.042 → 1.928 |
| New, concurrent | 2.109 → 2.031 |
| Existing, serial | 2.215 → 1.972 |
| Existing, concurrent | 2.041 → 2.269 |
| Contended creation | 2.188 → 2.133 |
| Healthy delivery | 2.447 → 2.190 |
| Saturated delivery | 2.342 → 2.143 |

These are individual sampled snapshots, dominated by pool statement caches and
the CPU profiler's 1.13-MiB buffer. They show no consistent retained-heap reduction;
the repeatable memory improvement is the reduction in allocations per save.
The saturated after-profile contains 16 goroutines, including the CPU profiler.

## Correctness checks

The real-PostgreSQL tests cover new/existing and self conversations, missing
membership links on an existing ID of 42, sender metadata/username fallback,
16 concurrent saves, and a deterministically observed INSERT lock wait whose
competing creator either commits or rolls back. They assert one transaction per
successful save. Real errors during conversation, membership, message, sender
lookup, and deferred-constraint commit verify complete rollback. Unit tests
also exercise BEGIN errors, failed/missing ID resolution, wrapped no-rows errors,
and deferred cleanup.

`go test -race -buildvcs ./...`, `golangci-lint run ./...`, and `gofumpt -l .`
passed. CI starts PostgreSQL and sets the opt-in test URL.

## Reproduction

Start PostgreSQL and point tests at it:

```sh
docker run --detach --rm --name popsocket-save-validation \
  -e POSTGRES_PASSWORD=popsocket-test -e POSTGRES_DB=popsocket_test \
  -p 127.0.0.1:55432:5432 postgres:18.1
export POPSOCKET_TEST_DATABASE_URL='postgres://postgres:popsocket-test@127.0.0.1:55432/popsocket_test?sslmode=disable'
go test -race -buildvcs ./...
```

Build each revision with the same benchmark files using
`go test -c -o tmp/save-validation/before.test ./pkg/popsocket` and the equivalent
`after.test` output. Run them sequentially:

```sh
for revision in before after; do
  tmp/save-validation/$revision.test -test.run='^$' \
    -test.bench='BenchmarkMessage(StoreSave|Delivery)Postgres' \
    -test.benchmem -test.benchtime=1s -test.count=10 -test.cpu=8 \
    > tmp/save-validation/$revision.txt
done
benchstat tmp/save-validation/before.txt tmp/save-validation/after.txt
```

Capture profiles in separate load runs:

```sh
for revision in before after; do
  mkdir -p tmp/save-validation/$revision-profiles
  POPSOCKET_PROFILE_DIR=tmp/save-validation/$revision-profiles \
    tmp/save-validation/$revision.test -test.run='^$' \
    -test.bench='BenchmarkMessage(StoreSave|Delivery)Postgres' \
    -test.benchmem -test.benchtime=12s -test.count=1 -test.cpu=8 \
    -test.cpuprofile=tmp/save-validation/$revision-profiles/cpu.pprof \
    -test.memprofilerate=16384 \
    > tmp/save-validation/$revision-profiles/load.txt
done
```

The monitor captures heap (after GC) and goroutine snapshots every five seconds
while workers are active. Inspect heap profiles with `go tool pprof -inuse_space`.
Raw timing output, benchstat output, binaries, and profiles are retained locally
in the git-ignored `tmp/save-validation/` directory.

```sh
docker stop popsocket-save-validation
```
